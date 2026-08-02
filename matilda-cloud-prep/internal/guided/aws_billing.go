package guided

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func isAWSBilling(outcome outcomeOption, cloud cloudOption) bool {
	return outcome.goal == assessment.RapidAssessment &&
		outcome.collectionPath == assessment.CollectionBilling &&
		cloud.provider == assessment.ProviderAWS
}

func runAWSBilling(reader *bufio.Scanner, stdout io.Writer, config Config) error {
	ctx, cancel := guidedContext(config)
	defer cancel()

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Connect AWS account")
	fmt.Fprintln(stdout, "Discovering safe local AWS credential sources.")

	sources, err := config.AWSBilling.DiscoverCredentialSources(ctx)
	if err != nil {
		writeAWSDiscoveryError(stdout, err)
		return nil
	}
	if len(sources) == 0 {
		fmt.Fprintln(stdout, "No AWS credential sources were found.")
		fmt.Fprintln(stdout, "Sign in or configure an AWS profile, then run matilda-prep start again.")
		return nil
	}

	verified, deferred, blocked := inspectAWSSources(ctx, config.AWSBilling, sources)
	if len(verified) == 0 && len(deferred) == 0 {
		writeNoVerifiedAWSSources(stdout, blocked)
		return nil
	}

	selected, proceed, err := selectAWSSource(ctx, reader, stdout, config.AWSBilling, verified, deferred)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Inspect AWS CUR 2.0 billing exports")
	options, err := awsBillingOptions(selected.Identity.Source)
	if err != nil {
		fmt.Fprintln(stdout, "Selected AWS credential source contains unsafe selector metadata.")
		return nil
	}
	result := config.Registry.ExecuteContext(ctx, awsBillingRequest(), options)
	return handleAWSBillingResult(ctx, reader, stdout, config, selected, result)
}

func guidedContext(config Config) (context.Context, context.CancelFunc) {
	timeout := config.TimeoutSeconds
	if timeout <= 0 {
		timeout = workflow.DefaultExecutionTimeoutSeconds
	}
	return context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
}

type awsVerifiedSource struct {
	Source   billingguide.CredentialSource
	Identity billingguide.VerifiedIdentity
}

type awsBlockedSource struct {
	Source         billingguide.CredentialSource
	Code           string
	UnsafeSource   bool
	CanRunAWSLogin bool
}

func inspectAWSSources(ctx context.Context, guide AWSBillingGuide, sources []billingguide.CredentialSource) ([]awsVerifiedSource, []billingguide.CredentialSource, []awsBlockedSource) {
	verified := []awsVerifiedSource{}
	deferred := []billingguide.CredentialSource{}
	blocked := []awsBlockedSource{}
	for _, source := range sources {
		safeSource, sourceOK := safeAWSCredentialSource(source)
		if !sourceOK {
			blocked = append(blocked, awsBlockedSource{Code: "aws_config_invalid_selector", UnsafeSource: true})
			continue
		}
		if shouldDeferAWSVerification(safeSource) {
			deferred = append(deferred, safeSource)
			continue
		}
		identity, err := guide.VerifyIdentity(ctx, source)
		if err != nil {
			code := verificationCode(err)
			blocked = append(blocked, awsBlockedSource{
				Source:         safeSource,
				Code:           code,
				CanRunAWSLogin: canRunAWSLogin(safeSource, code),
			})
			continue
		}
		if _, err := awsBillingOptions(identity.Source); err != nil {
			blocked = append(blocked, awsBlockedSource{Source: safeSource, Code: "aws_config_invalid_selector"})
			continue
		}
		verified = append(verified, awsVerifiedSource{Source: safeSource, Identity: identity})
	}
	return verified, deferred, blocked
}

func shouldDeferAWSVerification(source billingguide.CredentialSource) bool {
	return source.Kind == billingguide.CredentialSourceProfile && source.HasCredentialProcess
}

func canRunAWSLogin(source billingguide.CredentialSource, code string) bool {
	return source.Kind == billingguide.CredentialSourceProfile &&
		source.HasLoginSession &&
		code == "aws_config_missing_credentials"
}

func verificationCode(err error) string {
	var verificationErr billingguide.VerificationError
	if errors.As(err, &verificationErr) && verificationErr.Code != "" {
		return verificationErr.Code
	}
	return "aws_auth_failed"
}

func writeAWSDiscoveryError(stdout io.Writer, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(stdout, "AWS credential discovery timed out.")
		fmt.Fprintln(stdout, "Retry after AWS responds, or run the direct preflight command with a longer timeout.")
		return
	}
	fmt.Fprintln(stdout, "AWS credential discovery could not complete.")
}

func writeNoVerifiedAWSSources(stdout io.Writer, blocked []awsBlockedSource) {
	fmt.Fprintln(stdout, "No verified AWS credential source is available.")
	for _, source := range blocked {
		if source.UnsafeSource {
			fmt.Fprintf(stdout, "  AWS credential source blocked: %s\n", source.Code)
			continue
		}
		fmt.Fprintf(stdout, "  %s blocked: %s\n", credentialSourceLabel(source.Source), source.Code)
		if source.CanRunAWSLogin {
			fmt.Fprintf(stdout, "  Remediation: aws login --profile %s\n", shellArg(source.Source.Profile))
		}
	}
	if len(blocked) == 0 {
		fmt.Fprintln(stdout, "  Configure an AWS profile or environment credentials, then run matilda-prep start again.")
	}
}

func selectAWSSource(ctx context.Context, reader *bufio.Scanner, stdout io.Writer, guide AWSBillingGuide, verified []awsVerifiedSource, deferred []billingguide.CredentialSource) (awsVerifiedSource, bool, error) {
	if len(deferred) == 0 {
		return selectAWSIdentity(reader, stdout, verified)
	}
	if len(verified) == 0 && len(deferred) == 1 {
		return verifyDeferredAWSSource(ctx, reader, stdout, guide, deferred[0], "Verify this AWS credential source now? [y/N] ")
	}

	fmt.Fprintln(stdout, "Select AWS credential source")
	for index, item := range verified {
		fmt.Fprintf(stdout, "  %d. %s\n", index+1, credentialSourceLabel(item.Identity.Source))
		fmt.Fprintf(stdout, "     %s, caller %s\n", item.Identity.AccountLabel, item.Identity.CallerRef)
	}
	for index, source := range deferred {
		fmt.Fprintf(stdout, "  %d. %s\n", len(verified)+index+1, credentialSourceLabel(source))
		fmt.Fprintln(stdout, "     Verification requires confirmation before this source is used.")
	}

	index, err := readChoice(reader, stdout, fmt.Sprintf("Select AWS credential source [1-%d]: ", len(verified)+len(deferred)), "AWS credential source", len(verified)+len(deferred))
	if err != nil {
		return awsVerifiedSource{}, false, err
	}
	if index < len(verified) {
		return verified[index], true, nil
	}
	return verifyDeferredAWSSource(ctx, reader, stdout, guide, deferred[index-len(verified)], "Verify selected AWS credential source now? [y/N] ")
}

func selectAWSIdentity(reader *bufio.Scanner, stdout io.Writer, verified []awsVerifiedSource) (awsVerifiedSource, bool, error) {
	if len(verified) == 1 {
		return confirmAWSIdentity(reader, stdout, verified[0])
	}

	fmt.Fprintln(stdout, "Select AWS account")
	for index, item := range verified {
		fmt.Fprintf(stdout, "  %d. %s\n", index+1, credentialSourceLabel(item.Identity.Source))
		fmt.Fprintf(stdout, "     %s, caller %s\n", item.Identity.AccountLabel, item.Identity.CallerRef)
	}
	index, err := readChoice(reader, stdout, fmt.Sprintf("Select AWS account [1-%d]: ", len(verified)), "AWS account", len(verified))
	if err != nil {
		return awsVerifiedSource{}, false, err
	}
	return verified[index], true, nil
}

func verifyDeferredAWSSource(ctx context.Context, reader *bufio.Scanner, stdout io.Writer, guide AWSBillingGuide, source billingguide.CredentialSource, prompt string) (awsVerifiedSource, bool, error) {
	fmt.Fprintf(stdout, "AWS credential source requires verification: %s\n", credentialSourceLabel(source))
	fmt.Fprintln(stdout, "This source uses a configured credential process; verification may run that local credential command.")
	ok, err := readConfirmation(reader, stdout, prompt)
	if err != nil {
		return awsVerifiedSource{}, false, err
	}
	if !ok {
		fmt.Fprintln(stdout, "AWS billing preflight was not run.")
		return awsVerifiedSource{}, false, nil
	}

	identity, err := guide.VerifyIdentity(ctx, source)
	if err != nil {
		code := verificationCode(err)
		writeNoVerifiedAWSSources(stdout, []awsBlockedSource{{
			Source:         source,
			Code:           code,
			CanRunAWSLogin: canRunAWSLogin(source, code),
		}})
		return awsVerifiedSource{}, false, nil
	}
	if _, err := awsBillingOptions(identity.Source); err != nil {
		writeNoVerifiedAWSSources(stdout, []awsBlockedSource{{Source: source, Code: "aws_config_invalid_selector"}})
		return awsVerifiedSource{}, false, nil
	}
	return confirmAWSIdentity(reader, stdout, awsVerifiedSource{Source: source, Identity: identity})
}

func confirmAWSIdentity(reader *bufio.Scanner, stdout io.Writer, selected awsVerifiedSource) (awsVerifiedSource, bool, error) {
	fmt.Fprintf(stdout, "AWS account verified: %s, caller %s, source %s\n", selected.Identity.AccountLabel, selected.Identity.CallerRef, credentialSourceLabel(selected.Identity.Source))
	ok, err := readConfirmation(reader, stdout, "Continue with this AWS account? [y/N] ")
	if err != nil {
		return awsVerifiedSource{}, false, err
	}
	if !ok {
		fmt.Fprintln(stdout, "AWS billing preflight was not run.")
		return awsVerifiedSource{}, false, nil
	}
	return selected, true, nil
}

func readConfirmation(reader *bufio.Scanner, stdout io.Writer, prompt string) (bool, error) {
	fmt.Fprint(stdout, prompt)
	if !reader.Scan() {
		if err := reader.Err(); err != nil {
			return false, fmt.Errorf("%w: guided setup cancelled while reading confirmation: %v", ErrInputCancelled, err)
		}
		return false, fmt.Errorf("%w: guided setup cancelled before confirmation", ErrInputCancelled)
	}
	switch strings.ToLower(strings.TrimSpace(reader.Text())) {
	case "y", "yes":
		return true, nil
	case "", "n", "no":
		return false, nil
	case "q", "quit", "cancel":
		return false, fmt.Errorf("%w: guided setup cancelled by user", ErrInputCancelled)
	default:
		return false, fmt.Errorf("%w: expected y or n", ErrInvalidSelection)
	}
}

func awsBillingOptions(source billingguide.CredentialSource) (workflow.ExecutionOptions, error) {
	options := workflow.ExecutionOptions{
		InterfaceMode: workflow.InterfaceModeGuided,
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				Profile: source.Profile,
				Region:  source.Region,
			},
		},
	}
	return workflow.NormalizeExecutionOptions(options)
}

func awsBillingRequest() workflow.Request {
	return workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}
}

func safeAWSCredentialSource(source billingguide.CredentialSource) (billingguide.CredentialSource, bool) {
	if _, err := awsBillingOptions(source); err != nil {
		return billingguide.CredentialSource{}, false
	}
	return source, true
}

func writeAWSBillingSummary(stdout io.Writer, source billingguide.CredentialSource, result workflow.Result) {
	writeAWSBillingSummaryWithFacts(stdout, source, result, true)
}

func writeAWSBillingSummaryWithoutFacts(stdout io.Writer, source billingguide.CredentialSource, result workflow.Result) {
	writeAWSBillingSummaryWithFacts(stdout, source, result, false)
}

func writeAWSBillingSummaryWithFacts(stdout io.Writer, source billingguide.CredentialSource, result workflow.Result, includeFacts bool) {
	fmt.Fprintf(stdout, "Result: %s\n", result.Status)
	if code := safeCandidateLabelValue(result.Code); code != "" {
		fmt.Fprintf(stdout, "Support code: %s\n", code)
	}
	if includeFacts {
		writeAWSBillingSummaryFacts(stdout, result)
	}
	if result.Message != "" {
		fmt.Fprintln(stdout, result.Message)
	}
	fmt.Fprintln(stdout)
	label, command := awsBillingFollowupCommand(source, result)
	fmt.Fprintln(stdout, label)
	fmt.Fprintf(stdout, "  %s\n", command)
}

func selectedExportRef(result workflow.Result) string {
	if result.ExecutionOptions.Selectors != nil && result.ExecutionOptions.Selectors.AWS != nil {
		return result.ExecutionOptions.Selectors.AWS.CUR2ExportRef
	}
	return ""
}

func directAWSBillingCommand(source billingguide.CredentialSource, exportRef string) string {
	parts := []string{"matilda-prep", "rapid-assessment", "billing", "aws", "preflight"}
	return directAWSBillingCommandWithParts(parts, source, exportRef)
}

func directAWSBillingBackfillCommand(source billingguide.CredentialSource, exportRef string) string {
	parts := []string{"matilda-prep", "rapid-assessment", "billing", "aws", "apply-prereqs"}
	parts = directAWSBillingSelectorParts(parts, source, exportRef)
	parts = append(parts, "--request-backfill")
	return strings.Join(parts, " ")
}

func directAWSBillingCreateCUR2Command(source billingguide.CredentialSource) string {
	parts := []string{"matilda-prep", "rapid-assessment", "billing", "aws", "apply-prereqs"}
	parts = directAWSBillingSelectorParts(parts, source, "")
	parts = append(parts, "--create-cur2-export")
	return strings.Join(parts, " ")
}

func awsBillingFollowupCommand(source billingguide.CredentialSource, result workflow.Result) (string, string) {
	switch result.Code {
	case "aws_backfill_manual_step_required":
		return "Next command:", directAWSBillingBackfillCommand(source, selectedExportRef(result))
	case "aws_cur2_export_not_found", "aws_non_cur2_source_out_of_scope":
		return "Next command:", directAWSBillingCreateCUR2Command(source)
	default:
		return "Reproduce with:", directAWSBillingCommand(source, selectedExportRef(result))
	}
}

func directAWSBillingCommandWithParts(parts []string, source billingguide.CredentialSource, exportRef string) string {
	parts = directAWSBillingSelectorParts(parts, source, exportRef)
	return strings.Join(parts, " ")
}

func directAWSBillingSelectorParts(parts []string, source billingguide.CredentialSource, exportRef string) []string {
	if source.Profile != "" {
		parts = append(parts, "--profile", shellArg(source.Profile))
	}
	if source.Region != "" {
		parts = append(parts, "--region", shellArg(source.Region))
	}
	if exportRef != "" {
		parts = append(parts, "--export-ref", shellArg(exportRef))
	}
	return parts
}

func credentialSourceLabel(source billingguide.CredentialSource) string {
	if source.Kind == billingguide.CredentialSourceEnvironment {
		if source.Region != "" {
			return "environment credentials in " + source.Region
		}
		return "environment credentials"
	}
	label := "profile " + source.Profile
	if source.Region != "" {
		label += " in " + source.Region
	}
	if source.HasLoginSession {
		label += " with login session"
	}
	if source.HasCredentialProcess {
		label += " with credential process"
	}
	return label
}

func shellArg(value string) string {
	if shellBareArg(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func shellBareArg(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("._-/:=@%,+", r):
		default:
			return false
		}
	}
	return true
}
