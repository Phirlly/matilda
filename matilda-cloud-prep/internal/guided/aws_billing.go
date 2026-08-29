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
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Connect AWS account")

	for attempt := 0; ; attempt++ {
		selection, err := discoverAndSelectAWSSource(reader, stdout, config, attempt)
		if err != nil {
			return err
		}
		switch selection.Action {
		case awsSourceSelectionRescan:
			continue
		case awsSourceSelectionProceed:
			fmt.Fprintln(stdout)
			fmt.Fprintln(stdout, "Inspect AWS CUR 2.0 billing exports")
			options, err := awsBillingOptions(selection.Source.Identity.Source)
			if err != nil {
				fmt.Fprintln(stdout, "Selected AWS credential source contains unsafe selector metadata.")
				return nil
			}

			ctx, cancel := guidedContext(config)
			result := config.Registry.ExecuteContext(ctx, awsBillingRequest(), options)
			err = handleAWSBillingResult(ctx, reader, stdout, config, selection.Source, result)
			cancel()
			return err
		default:
			return nil
		}
	}
}

func discoverAndSelectAWSSource(reader *bufio.Scanner, stdout io.Writer, config Config, attempt int) (awsSourceSelection, error) {
	ctx, cancel := guidedContext(config)
	defer cancel()

	if attempt == 0 {
		fmt.Fprintln(stdout, "Discovering safe local AWS credential sources.")
	} else {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Re-scanning safe local AWS credential sources.")
	}

	sources, err := config.AWSBilling.DiscoverCredentialSources(ctx)
	if err != nil {
		writeAWSDiscoveryError(stdout, err)
		return awsSourceSelection{Action: awsSourceSelectionStop}, nil
	}
	if len(sources) == 0 {
		fmt.Fprintln(stdout, "No AWS credential sources were found.")
	}

	verified, deferred, blocked := inspectAWSSources(ctx, config.AWSBilling, sources)
	loginActions := awsLoginActions(ctx, config, blocked)
	if len(verified) == 0 && len(deferred) == 0 {
		writeNoVerifiedAWSSources(stdout, blocked, len(loginActions) == 0)
	}
	return selectAWSSource(reader, stdout, config, verified, deferred, loginActions)
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

type awsLoginAction struct {
	Source billingguide.CredentialSource
}

type awsSourceSelectionAction int

const (
	awsSourceSelectionStop awsSourceSelectionAction = iota
	awsSourceSelectionRetry
	awsSourceSelectionRescan
	awsSourceSelectionProceed
)

type awsSourceSelection struct {
	Source awsVerifiedSource
	Action awsSourceSelectionAction
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

func awsLoginActions(ctx context.Context, config Config, blocked []awsBlockedSource) []awsLoginAction {
	if config.AWSLogin == nil {
		return nil
	}

	candidates := []awsLoginAction{}
	for _, item := range blocked {
		if !item.CanRunAWSLogin {
			continue
		}
		source, ok := safeAWSCredentialSource(item.Source)
		if !ok || source.Kind != billingguide.CredentialSourceProfile || source.Profile == "" {
			continue
		}
		candidates = append(candidates, awsLoginAction{Source: source})
	}
	if len(candidates) == 0 {
		return nil
	}

	if support := config.AWSLogin.SupportsLogin(ctx); !support.Available {
		return nil
	}
	return candidates
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

func writeNoVerifiedAWSSources(stdout io.Writer, blocked []awsBlockedSource, includeExternalLoginRemediation bool) {
	fmt.Fprintln(stdout, "No verified AWS credential source is available.")
	for _, source := range blocked {
		if source.UnsafeSource {
			fmt.Fprintf(stdout, "  AWS credential source blocked: %s\n", source.Code)
			continue
		}
		fmt.Fprintf(stdout, "  %s blocked: %s\n", credentialSourceLabel(source.Source), source.Code)
		if source.CanRunAWSLogin && includeExternalLoginRemediation {
			fmt.Fprintf(stdout, "  Remediation: aws login --profile %s\n", shellArg(source.Source.Profile))
		}
	}
	if len(blocked) == 0 {
		fmt.Fprintln(stdout, "  Sign in or configure an AWS profile outside this tool, then choose re-scan.")
	}
}

func selectAWSSource(reader *bufio.Scanner, stdout io.Writer, config Config, verified []awsVerifiedSource, deferred []billingguide.CredentialSource, loginActions []awsLoginAction) (awsSourceSelection, error) {
	if len(verified) == 1 && len(deferred) == 0 {
		selected, proceed, err := confirmAWSIdentity(reader, stdout, verified[0])
		if err != nil {
			return awsSourceSelection{}, err
		}
		if proceed {
			return awsSourceSelection{Source: selected, Action: awsSourceSelectionProceed}, nil
		}
		fmt.Fprintln(stdout, "Choose how to connect another AWS account.")
	}
	if len(verified) == 0 && len(deferred) == 1 {
		selected, proceed, err := verifyDeferredAWSSource(reader, stdout, config, deferred[0], "Verify this AWS credential source now? [y/N] ")
		if err != nil {
			return awsSourceSelection{}, err
		}
		if proceed {
			return awsSourceSelection{Source: selected, Action: awsSourceSelectionProceed}, nil
		}
		fmt.Fprintln(stdout, "Choose how to connect another AWS account.")
	}

	for {
		selection, err := selectAWSIdentity(reader, stdout, config, verified, deferred, loginActions)
		if err != nil {
			return awsSourceSelection{}, err
		}
		switch selection.Action {
		case awsSourceSelectionProceed, awsSourceSelectionRescan, awsSourceSelectionStop:
			return selection, nil
		}
		fmt.Fprintln(stdout, "Choose how to connect another AWS account.")
	}
}

func selectAWSIdentity(reader *bufio.Scanner, stdout io.Writer, config Config, verified []awsVerifiedSource, deferred []billingguide.CredentialSource, loginActions []awsLoginAction) (awsSourceSelection, error) {
	sourceCount := len(verified) + len(deferred)
	loginStartIndex := sourceCount
	rescanIndex := sourceCount + len(loginActions)
	manualIndex := rescanIndex + 1
	choiceCount := manualIndex + 1
	if sourceCount == 0 {
		fmt.Fprintln(stdout, "Select AWS credential source")
	} else {
		fmt.Fprintln(stdout, "Select AWS account")
	}
	for index, item := range verified {
		fmt.Fprintf(stdout, "  %d. %s\n", index+1, credentialSourceLabel(item.Identity.Source))
		fmt.Fprintf(stdout, "     %s, caller %s\n", item.Identity.AccountLabel, item.Identity.CallerRef)
	}
	for index, source := range deferred {
		fmt.Fprintf(stdout, "  %d. %s\n", len(verified)+index+1, credentialSourceLabel(source))
		fmt.Fprintln(stdout, "     Verification requires confirmation before this source is used.")
	}
	for index, action := range loginActions {
		fmt.Fprintf(stdout, "  %d. Sign in to profile %s, then re-scan\n", loginStartIndex+index+1, action.Source.Profile)
		fmt.Fprintln(stdout, "     Opens AWS CLI browser login after confirmation.")
	}
	fmt.Fprintf(stdout, "  %d. Sign in or configure another AWS profile, then re-scan\n", rescanIndex+1)
	fmt.Fprintf(stdout, "  %d. Use an existing AWS profile name manually (advanced)\n", manualIndex+1)

	promptName := "AWS account"
	if sourceCount == 0 {
		promptName = "AWS credential source"
	}
	index, err := readChoice(reader, stdout, fmt.Sprintf("Select %s [1-%d]: ", promptName, choiceCount), promptName, choiceCount)
	if err != nil {
		return awsSourceSelection{}, err
	}
	if index < len(verified) {
		selected, proceed, err := confirmAWSIdentity(reader, stdout, verified[index])
		if err != nil {
			return awsSourceSelection{}, err
		}
		if proceed {
			return awsSourceSelection{Source: selected, Action: awsSourceSelectionProceed}, nil
		}
		return awsSourceSelection{Action: awsSourceSelectionRetry}, nil
	}
	if index < sourceCount {
		selected, proceed, err := verifyDeferredAWSSource(reader, stdout, config, deferred[index-len(verified)], "Verify selected AWS credential source now? [y/N] ")
		if err != nil {
			return awsSourceSelection{}, err
		}
		if proceed {
			return awsSourceSelection{Source: selected, Action: awsSourceSelectionProceed}, nil
		}
		return awsSourceSelection{Action: awsSourceSelectionRetry}, nil
	}
	if index < rescanIndex {
		return runAWSLoginForSource(reader, stdout, config, loginActions[index-loginStartIndex].Source)
	}
	if index == rescanIndex {
		if err := waitForAWSCredentialRescan(reader, stdout); err != nil {
			return awsSourceSelection{}, err
		}
		return awsSourceSelection{Action: awsSourceSelectionRescan}, nil
	}

	source, ok, err := readManualAWSProfileSource(reader, stdout)
	if err != nil {
		return awsSourceSelection{}, err
	}
	if !ok {
		return awsSourceSelection{Action: awsSourceSelectionRetry}, nil
	}
	selected, proceed, err := verifyManualAWSProfile(reader, stdout, config, source)
	if err != nil {
		return awsSourceSelection{}, err
	}
	if proceed {
		return awsSourceSelection{Source: selected, Action: awsSourceSelectionProceed}, nil
	}
	return awsSourceSelection{Action: awsSourceSelectionRetry}, nil
}

func waitForAWSCredentialRescan(reader *bufio.Scanner, stdout io.Writer) error {
	fmt.Fprintln(stdout, "Sign in or configure the AWS profile for the account you want outside this tool.")
	fmt.Fprintln(stdout, "For AWS login profiles, run: aws login --profile <profile-name>")
	fmt.Fprintln(stdout, "This option waits for a login or configuration change you complete outside this prompt.")
	fmt.Fprint(stdout, "Press Enter after the AWS profile is ready to re-scan, or type cancel: ")
	if !reader.Scan() {
		if err := reader.Err(); err != nil {
			return fmt.Errorf("%w: guided setup cancelled while waiting to re-scan AWS credentials: %v", ErrInputCancelled, err)
		}
		return fmt.Errorf("%w: guided setup cancelled before AWS credential re-scan", ErrInputCancelled)
	}
	switch strings.ToLower(strings.TrimSpace(reader.Text())) {
	case "q", "quit", "cancel":
		return fmt.Errorf("%w: guided setup cancelled by user", ErrInputCancelled)
	default:
		return nil
	}
}

func runAWSLoginForSource(reader *bufio.Scanner, stdout io.Writer, config Config, source billingguide.CredentialSource) (awsSourceSelection, error) {
	if config.AWSLogin == nil {
		fmt.Fprintln(stdout, "In-flow AWS login is unavailable. Choose the re-scan option after signing in outside this tool.")
		return awsSourceSelection{Action: awsSourceSelectionRetry}, nil
	}

	fmt.Fprintf(stdout, "AWS CLI will open browser login for profile %s and return here when login completes.\n", source.Profile)
	fmt.Fprintln(stdout, "Matilda Cloud Prep will re-scan and ask you to confirm the verified AWS account before inspecting billing exports.")
	ok, err := readConfirmation(reader, stdout, fmt.Sprintf("Run AWS login for profile %s now? [y/N] ", source.Profile))
	if err != nil {
		return awsSourceSelection{}, err
	}
	if !ok {
		return awsSourceSelection{Action: awsSourceSelectionRetry}, nil
	}

	ctx, cancel := guidedContext(config)
	defer cancel()
	if err := config.AWSLogin.Login(ctx, source); err != nil {
		fmt.Fprintln(stdout, "AWS login did not complete. No AWS billing inspection was run.")
		return awsSourceSelection{Action: awsSourceSelectionRetry}, nil
	}

	fmt.Fprintln(stdout, "AWS login completed. Re-scanning safe local AWS credential sources.")
	return awsSourceSelection{Action: awsSourceSelectionRescan}, nil
}

func verifyDeferredAWSSource(reader *bufio.Scanner, stdout io.Writer, config Config, source billingguide.CredentialSource, prompt string) (awsVerifiedSource, bool, error) {
	fmt.Fprintf(stdout, "AWS credential source requires verification: %s\n", credentialSourceLabel(source))
	fmt.Fprintln(stdout, "This source uses a configured credential process; verification may run that local credential command.")
	ok, err := readConfirmation(reader, stdout, prompt)
	if err != nil {
		return awsVerifiedSource{}, false, err
	}
	if !ok {
		return awsVerifiedSource{}, false, nil
	}

	ctx, cancel := guidedContext(config)
	defer cancel()

	identity, err := config.AWSBilling.VerifyIdentity(ctx, source)
	if err != nil {
		code := verificationCode(err)
		writeNoVerifiedAWSSources(stdout, []awsBlockedSource{{
			Source:         source,
			Code:           code,
			CanRunAWSLogin: canRunAWSLogin(source, code),
		}}, true)
		return awsVerifiedSource{}, false, nil
	}
	if _, err := awsBillingOptions(identity.Source); err != nil {
		writeNoVerifiedAWSSources(stdout, []awsBlockedSource{{Source: source, Code: "aws_config_invalid_selector"}}, true)
		return awsVerifiedSource{}, false, nil
	}
	return confirmAWSIdentity(reader, stdout, awsVerifiedSource{Source: source, Identity: identity})
}

func readManualAWSProfileSource(reader *bufio.Scanner, stdout io.Writer) (billingguide.CredentialSource, bool, error) {
	profile, err := readPromptLine(reader, stdout, "Enter AWS profile name: ", "AWS profile name", false)
	if err != nil {
		return billingguide.CredentialSource{}, false, err
	}
	source := billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: profile,
	}
	if _, ok := safeAWSCredentialSource(source); !ok {
		fmt.Fprintln(stdout, "AWS profile name is not safe to use.")
		return billingguide.CredentialSource{}, false, nil
	}

	region, err := readPromptLine(reader, stdout, "Enter AWS region for this profile [leave blank to use profile configuration]: ", "AWS region", true)
	if err != nil {
		return billingguide.CredentialSource{}, false, err
	}
	source.Region = region
	if _, ok := safeAWSCredentialSource(source); !ok {
		fmt.Fprintln(stdout, "AWS region is not safe to use.")
		return billingguide.CredentialSource{}, false, nil
	}
	return source, true, nil
}

func verifyManualAWSProfile(reader *bufio.Scanner, stdout io.Writer, config Config, source billingguide.CredentialSource) (awsVerifiedSource, bool, error) {
	fmt.Fprintln(stdout, "AWS profile verification may run normal AWS SDK credential resolution for this profile, including a configured credential process.")
	ok, err := readConfirmation(reader, stdout, "Verify this AWS profile now? [y/N] ")
	if err != nil {
		return awsVerifiedSource{}, false, err
	}
	if !ok {
		return awsVerifiedSource{}, false, nil
	}

	ctx, cancel := guidedContext(config)
	defer cancel()

	identity, err := config.AWSBilling.VerifyIdentity(ctx, source)
	if err != nil {
		writeManualAWSProfileVerificationError(stdout, source, verificationCode(err))
		return awsVerifiedSource{}, false, nil
	}
	if _, err := awsBillingOptions(identity.Source); err != nil {
		writeManualAWSProfileVerificationError(stdout, source, "aws_config_invalid_selector")
		return awsVerifiedSource{}, false, nil
	}
	return confirmAWSIdentity(reader, stdout, awsVerifiedSource{Source: source, Identity: identity})
}

func readPromptLine(reader *bufio.Scanner, stdout io.Writer, prompt string, name string, allowEmpty bool) (string, error) {
	fmt.Fprint(stdout, prompt)
	if !reader.Scan() {
		if err := reader.Err(); err != nil {
			return "", fmt.Errorf("%w: guided setup cancelled while reading %s: %v", ErrInputCancelled, name, err)
		}
		return "", fmt.Errorf("%w: guided setup cancelled before %s", ErrInputCancelled, name)
	}
	value := strings.TrimSpace(reader.Text())
	switch strings.ToLower(value) {
	case "q", "quit", "cancel":
		return "", fmt.Errorf("%w: guided setup cancelled by user", ErrInputCancelled)
	}
	if value == "" && !allowEmpty {
		return "", fmt.Errorf("%w: %s cannot be empty", ErrInvalidSelection, name)
	}
	return value, nil
}

func writeManualAWSProfileVerificationError(stdout io.Writer, source billingguide.CredentialSource, code string) {
	fmt.Fprintf(stdout, "%s blocked: %s\n", credentialSourceLabel(source), code)
	switch code {
	case "aws_config_missing_credentials":
		fmt.Fprintf(stdout, "Run aws login --profile %s if this is an AWS login profile, or configure credentials for profile %s.\n", shellArg(source.Profile), shellArg(source.Profile))
		fmt.Fprintln(stdout, "Then choose this profile again after login or configuration is complete.")
	case "aws_config_missing_region":
		fmt.Fprintf(stdout, "Enter an AWS Region when choosing this profile again, or configure a Region for profile %s outside this tool.\n", shellArg(source.Profile))
	case "aws_config_profile_shadowed":
		fmt.Fprintln(stdout, "AWS credential environment variables would take precedence over the selected profile.")
		fmt.Fprintln(stdout, "Unset AWS credential environment variables and start a new shell before retrying this profile.")
	}
}

func confirmAWSIdentity(reader *bufio.Scanner, stdout io.Writer, selected awsVerifiedSource) (awsVerifiedSource, bool, error) {
	fmt.Fprintf(stdout, "AWS account verified: %s, caller %s, source %s\n", selected.Identity.AccountLabel, selected.Identity.CallerRef, credentialSourceLabel(selected.Identity.Source))
	ok, err := readConfirmation(reader, stdout, "Continue with this AWS account? [y/N] ")
	if err != nil {
		return awsVerifiedSource{}, false, err
	}
	if !ok {
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

func readConfirmationDefaultYes(reader *bufio.Scanner, stdout io.Writer, prompt string) (bool, error) {
	fmt.Fprint(stdout, prompt)
	if !reader.Scan() {
		if err := reader.Err(); err != nil {
			return false, fmt.Errorf("%w: guided setup cancelled while reading confirmation: %v", ErrInputCancelled, err)
		}
		return false, fmt.Errorf("%w: guided setup cancelled before confirmation", ErrInputCancelled)
	}
	switch strings.ToLower(strings.TrimSpace(reader.Text())) {
	case "", "y", "yes":
		return true, nil
	case "n", "no":
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

func awsBillingApplyPrereqsRequest() workflow.Request {
	return workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
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

func writeAWSBillingSummaryWithFacts(stdout io.Writer, source billingguide.CredentialSource, result workflow.Result, includeFacts bool) {
	fmt.Fprintf(stdout, "Result: %s\n", result.Status)
	if code := safeCandidateLabelValue(result.Code); code != "" {
		fmt.Fprintf(stdout, "Support code: %s\n", code)
	}
	if includeFacts {
		if isCreateCUR2SetupResult(result) {
			writeCreateCUR2SetupPlanSummary(stdout, source, result)
		} else {
			writeAWSBillingSummaryFacts(stdout, result)
		}
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
	if isCreateCUR2SetupResult(result) {
		return "Next command:", directAWSBillingCreateCUR2Command(source)
	}
	switch result.Code {
	case "aws_backfill_manual_step_required":
		return "Next command:", directAWSBillingBackfillCommand(source, selectedExportRef(result))
	case "aws_cur2_export_not_found", "aws_non_cur2_source_out_of_scope":
		return "Next command:", directAWSBillingCreateCUR2Command(source)
	default:
		return "Reproduce with:", directAWSBillingCommand(source, selectedExportRef(result))
	}
}

func isCreateCUR2SetupResult(result workflow.Result) bool {
	if result.ExecutionOptions.AWSBillingOperation == workflow.AWSBillingOperationCreateCUR2Export {
		return true
	}
	return result.Request.Action == assessment.ActionApplyPrereqs &&
		strings.HasPrefix(result.Code, "aws_cur2_create_export_")
}

func writeCreateCUR2SetupPlanSummary(stdout io.Writer, source billingguide.CredentialSource, result workflow.Result) {
	if result.Plan == nil {
		return
	}
	fmt.Fprintln(stdout, "Setup plan:")
	for _, step := range result.Plan.Steps {
		if step.RequiresApproval {
			fmt.Fprintf(stdout, "  - Approval required: %s\n", step.Title)
			writeCreateCUR2SetupPlanStepDetail(stdout, "Step ID", step.ID)
		} else {
			fmt.Fprintf(stdout, "  - No approval required: %s\n", step.Title)
		}
		writeCreateCUR2SetupPlanStepDetail(stdout, "Current state", step.CurrentState)
		writeCreateCUR2SetupPlanStepDetail(stdout, "Target state", step.TargetState)
		writeCreateCUR2SetupPlanStepDetail(stdout, "Required permission", step.RequiredPermission)
		writeCreateCUR2SetupPlanStepDetail(stdout, "Validation", step.Validation)
		writeCreateCUR2SetupPlanStepDetail(stdout, "Rollback", step.Rollback)
	}
	if result.Mutated {
		fmt.Fprintln(stdout, "Cloud changes were made for the approved setup plan.")
		return
	}
	fmt.Fprintln(stdout, "No cloud changes were made.")
	if result.Plan.Approval.Blocked {
		fmt.Fprintln(stdout, "This setup plan is blocked and cannot be approved until the blocker is resolved.")
		return
	}
	if result.Plan.Approval.Required {
		fmt.Fprintln(stdout, "Cloud changes require plan-bound approval before they are made.")
		if result.Plan.Approval.ApprovalPlanID != "" {
			fmt.Fprintf(stdout, "Approval plan: %s\n", result.Plan.Approval.ApprovalPlanID)
			if command := directAWSBillingCreateCUR2ApprovalCommand(source, result); command != "" {
				fmt.Fprintln(stdout, "Approve with:")
				fmt.Fprintf(stdout, "  %s\n", command)
			}
		}
		return
	}
	fmt.Fprintln(stdout, "No mutation approval is required for this result.")
}

func writeCreateCUR2SetupPlanStepDetail(stdout io.Writer, label string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(stdout, "    %s: %s\n", label, value)
}

func directAWSBillingCreateCUR2ApprovalCommand(source billingguide.CredentialSource, result workflow.Result) string {
	if result.Plan == nil || !result.Plan.Approval.Required || result.Plan.Approval.Blocked || result.Plan.Approval.ApprovalPlanID == "" {
		return ""
	}
	parts := []string{"matilda-prep", "rapid-assessment", "billing", "aws", "apply-prereqs"}
	parts = directAWSBillingSelectorParts(parts, source, "")
	parts = append(parts, "--create-cur2-export", "--approve-plan", shellArg(result.Plan.Approval.ApprovalPlanID))
	for _, step := range result.Plan.Steps {
		if step.RequiresApproval {
			parts = append(parts, "--approve-step", shellArg(step.ID))
		}
	}
	return strings.Join(parts, " ")
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
