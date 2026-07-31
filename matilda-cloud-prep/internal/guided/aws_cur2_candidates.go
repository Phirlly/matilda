package guided

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func handleAWSBillingResult(ctx context.Context, reader *bufio.Scanner, stdout io.Writer, config Config, selected awsVerifiedSource, result workflow.Result) error {
	if result.Code != "aws_cur2_export_ambiguous" {
		writeAWSBillingSummary(stdout, selected.Identity.Source, result)
		return nil
	}

	candidates := cur2Candidates(result)
	if len(candidates) == 0 {
		writeAWSBillingSummary(stdout, selected.Identity.Source, result)
		return nil
	}

	fmt.Fprintf(stdout, "Classifying %d CUR 2.0 export candidates\n", len(candidates))
	classified := classifyCUR2Candidates(ctx, config.Registry, selected.Identity.Source, candidates)
	selectable := selectableCUR2Candidates(classified)
	switch len(selectable) {
	case 0:
		repairable := repairableCUR2Candidates(classified)
		if len(repairable) > 0 {
			writeRepairableCUR2Candidates(stdout, repairable, classified)
			return nil
		}
		fmt.Fprintln(stdout, "No AWS CUR 2.0 export is ready or repairable yet.")
		writeBlockedClassifications(stdout, classified)
		return nil
	case 1:
		item := selectable[0]
		fmt.Fprintf(stdout, "Auto-selected CUR 2.0 export %s\n", item.Candidate.Ref)
		writeSelectableCUR2Candidate(stdout, item)
		writeBlockedClassifications(stdout, classified)
		writeAWSBillingSummaryWithoutFacts(stdout, selected.Identity.Source, item.Result)
		return nil
	default:
		fmt.Fprintln(stdout, "Select AWS CUR 2.0 export")
		for index, item := range selectable {
			fmt.Fprintf(stdout, "  %d. %s\n", index+1, candidateLabel(item.Candidate))
			writeSelectableCUR2CandidateOption(stdout, item)
		}
		index, err := readChoice(reader, stdout, fmt.Sprintf("Select AWS CUR 2.0 export [1-%d]: ", len(selectable)), "AWS CUR 2.0 export", len(selectable))
		if err != nil {
			return err
		}
		writeBlockedClassifications(stdout, classified)
		writeAWSBillingSummary(stdout, selected.Identity.Source, selectable[index].Result)
		return nil
	}
}

type cur2Candidate struct {
	Index       int
	Ref         string
	Health      string
	Output      string
	Compression string
	Granularity string
	Destination string
}

type classifiedCUR2Candidate struct {
	Candidate cur2Candidate
	Result    workflow.Result
}

func cur2Candidates(result workflow.Result) []cur2Candidate {
	byIndex := map[int]*cur2Candidate{}
	if result.Plan == nil {
		return nil
	}
	for _, check := range result.Plan.Checks {
		for _, evidence := range check.Evidence {
			index, field, ok := candidateEvidenceKey(evidence.Key)
			if !ok {
				continue
			}
			candidate := byIndex[index]
			if candidate == nil {
				candidate = &cur2Candidate{Index: index}
				byIndex[index] = candidate
			}
			switch field {
			case "export_ref":
				if safeCUR2ExportRef(evidence.Value) {
					candidate.Ref = strings.TrimSpace(evidence.Value)
				}
			case "health":
				candidate.Health = safeCandidateLabelValue(evidence.Value)
			case "output_format":
				candidate.Output = safeCandidateLabelValue(evidence.Value)
			case "compression":
				candidate.Compression = safeCandidateLabelValue(evidence.Value)
			case "time_granularity":
				candidate.Granularity = safeCandidateLabelValue(evidence.Value)
			case "destination_region":
				candidate.Destination = safeCandidateLabelValue(evidence.Value)
			}
		}
	}

	indexes := make([]int, 0, len(byIndex))
	for index := range byIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	candidates := make([]cur2Candidate, 0, len(indexes))
	for _, index := range indexes {
		candidate := *byIndex[index]
		if candidate.Ref != "" {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func safeCUR2ExportRef(value string) bool {
	options := workflow.ExecutionOptions{
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				CUR2ExportRef: value,
			},
		},
	}
	_, err := workflow.NormalizeExecutionOptions(options)
	return err == nil
}

func candidateEvidenceKey(key string) (int, string, bool) {
	const prefix = "candidate_"
	if !strings.HasPrefix(key, prefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(key, prefix)
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	index, err := strconv.Atoi(parts[0])
	if err != nil || index <= 0 {
		return 0, "", false
	}
	return index, parts[1], true
}

func classifyCUR2Candidates(ctx context.Context, registry workflow.Registry, source billingguide.CredentialSource, candidates []cur2Candidate) []classifiedCUR2Candidate {
	classified := make([]classifiedCUR2Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		options, err := awsBillingOptions(source)
		if err != nil {
			continue
		}
		if options.Selectors == nil {
			options.Selectors = &workflow.ExecutionSelectors{}
		}
		if options.Selectors.AWS == nil {
			options.Selectors.AWS = &workflow.AWSExecutionSelectors{}
		}
		options.Selectors.AWS.CUR2ExportRef = candidate.Ref
		options, err = workflow.NormalizeExecutionOptions(options)
		if err != nil {
			continue
		}
		result := registry.ExecuteContext(ctx, awsBillingRequest(), options)
		classified = append(classified, classifiedCUR2Candidate{Candidate: candidate, Result: result})
	}
	return classified
}

func selectableCUR2Candidates(classified []classifiedCUR2Candidate) []classifiedCUR2Candidate {
	selectable := []classifiedCUR2Candidate{}
	for _, item := range classified {
		if item.Result.Status == workflow.StatusReady || item.Result.Status == workflow.RunStatusManualSteps {
			selectable = append(selectable, item)
		}
	}
	return selectable
}

func repairableCUR2Candidates(classified []classifiedCUR2Candidate) []classifiedCUR2Candidate {
	repairable := []classifiedCUR2Candidate{}
	for _, item := range classified {
		if isRepairableCUR2Candidate(item) {
			repairable = append(repairable, item)
		}
	}
	return repairable
}

func isRepairableCUR2Candidate(item classifiedCUR2Candidate) bool {
	if item.Result.Status != workflow.RunStatusBlocked {
		return false
	}
	switch item.Result.Code {
	case "aws_s3_delivery_policy_missing", "aws_s3_bucket_policy_inaccessible":
		return true
	default:
		return false
	}
}

func writeBlockedClassifications(stdout io.Writer, classified []classifiedCUR2Candidate) {
	for _, item := range classified {
		if item.Result.Status == workflow.StatusReady || item.Result.Status == workflow.RunStatusManualSteps {
			continue
		}
		writeNonReadyCUR2Candidate(stdout, item)
	}
}

func writeRepairableCUR2Candidates(stdout io.Writer, repairable []classifiedCUR2Candidate, classified []classifiedCUR2Candidate) {
	fmt.Fprintln(stdout, "No AWS CUR 2.0 export is ready yet.")
	fmt.Fprintln(stdout, "Repairable CUR 2.0 export candidates")
	for _, item := range repairable {
		writeRepairableCUR2Candidate(stdout, item)
	}
	if len(repairable) == len(classified) {
		return
	}
	fmt.Fprintln(stdout, "Other CUR 2.0 candidates")
	for _, item := range classified {
		if isRepairableCUR2Candidate(item) {
			continue
		}
		if item.Result.Status == workflow.StatusReady || item.Result.Status == workflow.RunStatusManualSteps {
			continue
		}
		writeNonReadyCUR2Candidate(stdout, item)
	}
}

func writeRepairableCUR2Candidate(stdout io.Writer, item classifiedCUR2Candidate) {
	writeCUR2CandidateDetails(stdout, item, "repair required", repairableCUR2NextAction(item))
}

func writeSelectableCUR2Candidate(stdout io.Writer, item classifiedCUR2Candidate) {
	writeCUR2CandidateDetails(stdout, item, selectableCUR2Readiness(item), selectableCUR2NextAction(item))
}

func writeSelectableCUR2CandidateOption(stdout io.Writer, item classifiedCUR2Candidate) {
	writeCUR2CandidateFactLines(stdout, item, selectableCUR2Readiness(item), selectableCUR2NextAction(item), "     ")
}

func writeNonReadyCUR2Candidate(stdout io.Writer, item classifiedCUR2Candidate) {
	writeCUR2CandidateDetails(stdout, item, "not ready", nonReadyCUR2NextAction(item))
}

func writeCUR2CandidateDetails(stdout io.Writer, item classifiedCUR2Candidate, readiness string, nextAction string) {
	fmt.Fprintf(stdout, "  %s\n", item.Candidate.Ref)
	writeCUR2CandidateFactLines(stdout, item, readiness, nextAction, "    ")
}

func writeCUR2CandidateFactLines(stdout io.Writer, item classifiedCUR2Candidate, readiness string, nextAction string, indent string) {
	writeCUR2CandidateFactLinesWithOptions(stdout, item, readiness, nextAction, indent, true)
}

func writeCUR2CandidateFactLinesWithOptions(stdout io.Writer, item classifiedCUR2Candidate, readiness string, nextAction string, indent string, includeSupportCode bool) {
	facts := cur2CandidateFactsFromResult(item)
	fmt.Fprintf(stdout, "%sReadiness: %s\n", indent, readiness)
	if code := safeCandidateLabelValue(item.Result.Code); includeSupportCode && code != "" {
		fmt.Fprintf(stdout, "%sSupport code: %s\n", indent, code)
	}
	if export := exportSummary(facts); export != "" {
		fmt.Fprintf(stdout, "%sExport: %s\n", indent, export)
	}
	if facts.DeliveryStatus != "" {
		fmt.Fprintf(stdout, "%sAWS delivery: %s\n", indent, facts.DeliveryStatus)
	}
	if facts.PolicyStatus != "" {
		fmt.Fprintf(stdout, "%sS3 delivery policy: %s\n", indent, facts.PolicyStatus)
	}
	if facts.PreviousBillingPeriod != "" && len(facts.MissingPreviousMonth) > 0 {
		fmt.Fprintf(stdout, "%sPrevious month: %s missing %s\n", indent, facts.PreviousBillingPeriod, strings.Join(facts.MissingPreviousMonth, ", "))
	}
	if facts.Blocker != "" {
		fmt.Fprintf(stdout, "%sBlocker: %s\n", indent, facts.Blocker)
	}
	fmt.Fprintf(stdout, "%sNext action: %s\n", indent, nextAction)
}

func writeAWSBillingSummaryFacts(stdout io.Writer, result workflow.Result) {
	item, ok := summaryCUR2Candidate(result)
	if !ok {
		return
	}
	readiness, nextAction := summaryCUR2ReadinessAndNextAction(item)
	writeCUR2CandidateFactLinesWithOptions(stdout, item, readiness, nextAction, "", false)
}

func summaryCUR2Candidate(result workflow.Result) (classifiedCUR2Candidate, bool) {
	if !resultHasCUR2PlanFacts(result) {
		return classifiedCUR2Candidate{}, false
	}
	return classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: selectedExportRef(result)},
		Result:    result,
	}, true
}

func resultHasCUR2PlanFacts(result workflow.Result) bool {
	if result.Plan == nil {
		return false
	}
	for _, check := range result.Plan.Checks {
		if cur2SummaryCheckCode(planCheckCode(check)) {
			return true
		}
		for _, evidence := range check.Evidence {
			switch evidence.Key {
			case "output_format", "compression", "time_granularity", "overwrite", "previous_billing_period", "missing_previous_month_component", "policy_gap":
				return true
			}
		}
	}
	return false
}

func cur2SummaryCheckCode(code string) bool {
	return strings.HasPrefix(code, "aws_cur2_") ||
		strings.HasPrefix(code, "aws_s3_delivery_policy_") ||
		code == "aws_s3_bucket_policy_inaccessible" ||
		code == "aws_backfill_manual_step_required"
}

func summaryCUR2ReadinessAndNextAction(item classifiedCUR2Candidate) (string, string) {
	if isRepairableCUR2Candidate(item) {
		return "repair required", repairableCUR2NextAction(item)
	}
	if item.Result.Status == workflow.StatusReady || item.Result.Status == workflow.RunStatusManualSteps {
		return selectableCUR2Readiness(item), selectableCUR2NextAction(item)
	}
	return "not ready", nonReadyCUR2NextAction(item)
}

type cur2CandidateFacts struct {
	Format                string
	Compression           string
	Granularity           string
	OutputVersioning      string
	DeliveryStatus        string
	PolicyStatus          string
	PreviousMonthStatus   string
	PreviousBillingPeriod string
	MissingPreviousMonth  []string
	PolicyGap             string
	Blocker               string
}

func cur2CandidateFactsFromResult(item classifiedCUR2Candidate) cur2CandidateFacts {
	facts := cur2CandidateFacts{
		Format:      item.Candidate.Output,
		Compression: item.Candidate.Compression,
		Granularity: item.Candidate.Granularity,
	}
	if item.Result.Plan == nil {
		return facts
	}
	for _, check := range item.Result.Plan.Checks {
		checkCode := planCheckCode(check)
		facts.observeCheck(check.Status, checkCode)
		for _, evidence := range check.Evidence {
			value := safeCandidateLabelValue(evidence.Value)
			if value == "" {
				continue
			}
			switch evidence.Key {
			case "output_format":
				facts.Format = value
			case "compression":
				facts.Compression = value
			case "time_granularity":
				facts.Granularity = value
			case "overwrite":
				facts.OutputVersioning = value
			case "previous_billing_period":
				facts.PreviousBillingPeriod = value
			case "missing_previous_month_component":
				facts.MissingPreviousMonth = append(facts.MissingPreviousMonth, previousMonthComponentLabel(value))
			case "policy_gap":
				facts.PolicyGap = value
			}
		}
	}
	facts.deriveBlocker(item.Result.Status, item.Result.Code)
	return facts
}

func planCheckCode(check workflow.PlanCheck) string {
	if id := safeCandidateLabelValue(check.ID); id != "" {
		return id
	}
	for _, evidence := range check.Evidence {
		if evidence.Key == "code" {
			return safeCandidateLabelValue(evidence.Value)
		}
	}
	return ""
}

func (facts *cur2CandidateFacts) observeCheck(status workflow.CheckStatus, code string) {
	switch code {
	case "aws_cur2_delivery_ready":
		if status == workflow.CheckPass {
			facts.DeliveryStatus = "ready"
		}
	case "aws_cur2_delivery_not_started":
		if status == workflow.CheckWarn {
			facts.DeliveryStatus = "in progress"
		}
		if status == workflow.CheckFail {
			facts.DeliveryStatus = "not started"
		}
	case "aws_s3_delivery_policy_ready":
		if status == workflow.CheckPass {
			facts.PolicyStatus = "ready"
		}
	case "aws_s3_bucket_policy_inaccessible":
		if status == workflow.CheckWarn || status == workflow.CheckFail {
			facts.PolicyStatus = "not inspected"
		}
	case "aws_s3_delivery_policy_missing":
		if status == workflow.CheckWarn || status == workflow.CheckFail {
			facts.PolicyStatus = "action needed"
		}
	case "aws_cur2_previous_month_ready":
		if status == workflow.CheckPass {
			facts.PreviousMonthStatus = "ready"
		}
	}
}

func (facts *cur2CandidateFacts) deriveBlocker(status workflow.RunStatus, resultCode string) {
	if status != workflow.RunStatusBlocked {
		return
	}
	if resultCode == "aws_cur2_output_settings_blocked" && facts.OutputVersioning == "OVERWRITE_REPORT" {
		facts.Blocker = "overwrite file versioning is not verified for this Matilda path."
		return
	}
	if facts.PolicyGap != "" && (facts.PolicyStatus == "action needed" || resultCode == "aws_s3_delivery_policy_missing") {
		facts.Blocker = "S3 delivery policy does not satisfy " + policyGapRequirement(facts.PolicyGap) + "."
	}
}

func repairableCUR2NextAction(item classifiedCUR2Candidate) string {
	facts := cur2CandidateFactsFromResult(item)
	switch item.Result.Code {
	case "aws_s3_delivery_policy_missing":
		if facts.PolicyGap != "" {
			return "update the S3 delivery policy to include " + policyGapRequirement(facts.PolicyGap) + ", then rerun preflight."
		}
		return "update the S3 delivery policy using the direct preflight finding, then rerun preflight."
	case "aws_s3_bucket_policy_inaccessible":
		return "grant read access to inspect the S3 bucket policy, then rerun preflight."
	default:
		return "review the direct preflight result and rerun after remediation."
	}
}

func selectableCUR2Readiness(item classifiedCUR2Candidate) string {
	switch item.Result.Status {
	case workflow.StatusReady:
		return "ready"
	case workflow.RunStatusManualSteps:
		return "manual step required"
	default:
		return "selected"
	}
}

func selectableCUR2NextAction(item classifiedCUR2Candidate) string {
	facts := cur2CandidateFactsFromResult(item)
	switch item.Result.Status {
	case workflow.StatusReady:
		switch item.Result.Code {
		case "aws_s3_delivery_policy_missing", "aws_s3_bucket_policy_inaccessible":
			return "continue with this CUR 2.0 export; review the S3 delivery policy before relying on future delivery or backfill."
		}
		return "continue with this CUR 2.0 export."
	case workflow.RunStatusManualSteps:
		if facts.previousMonthMissing() {
			return "request or complete previous-month billing data backfill, then rerun preflight."
		}
		return "complete the manual step shown by preflight, then rerun preflight."
	default:
		return "review the direct preflight result and rerun after remediation."
	}
}

func previousMonthComponentLabel(component string) string {
	switch component {
	case "data_partition":
		return "data partition"
	case "manifest":
		return "manifest"
	default:
		return "previous-month component"
	}
}

func policyGapRequirement(gap string) string {
	switch gap {
	case "service_principal_missing":
		return "the AWS Data Exports service principal"
	case "put_object_action_missing":
		return "permission for AWS Data Exports to write CUR objects"
	case "put_object_resource_not_covered":
		return "the selected CUR 2.0 S3 destination prefix"
	case "source_account_condition_missing":
		return "the expected aws:SourceAccount condition"
	case "source_arn_condition_missing":
		return "the expected aws:SourceArn condition"
	case "policy_unparseable":
		return "a parsable S3 bucket policy"
	case "matching_allow_statement_missing":
		return "a matching AWS Data Exports delivery statement"
	default:
		return "the required AWS Data Exports S3 delivery policy"
	}
}

func nonReadyCUR2NextAction(item classifiedCUR2Candidate) string {
	facts := cur2CandidateFactsFromResult(item)
	switch item.Result.Code {
	case "aws_cur2_output_settings_blocked":
		if facts.OutputVersioning == "OVERWRITE_REPORT" {
			return "confirm Matilda support for OVERWRITE_REPORT, or select a CREATE_NEW_REPORT CUR 2.0 export."
		}
		return "review the CUR 2.0 output settings and rerun after they match a Matilda-supported AWS-standard shape."
	case "aws_data_exports_transient":
		return "retry preflight after the transient AWS Data Exports issue clears."
	default:
		return "review the direct preflight result and rerun after remediation."
	}
}

func (facts cur2CandidateFacts) previousMonthMissing() bool {
	return facts.PreviousBillingPeriod != "" && len(facts.MissingPreviousMonth) > 0
}

func exportSummary(facts cur2CandidateFacts) string {
	parts := []string{}
	if facts.Format != "" || facts.Compression != "" {
		parts = append(parts, displayFact(facts.Format)+" / "+displayFact(facts.Compression))
	}
	if facts.Granularity != "" {
		parts = append(parts, facts.Granularity)
	}
	if facts.OutputVersioning != "" {
		parts = append(parts, facts.OutputVersioning)
	}
	return strings.Join(parts, ", ")
}

func displayFact(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unverified"
	}
	return value
}

func candidateLabel(candidate cur2Candidate) string {
	parts := []string{candidate.Ref}
	if health := safeCandidateLabelValue(candidate.Health); health != "" {
		parts = append(parts, "health "+health)
	}
	if output := safeCandidateLabelValue(candidate.Output); output != "" {
		parts = append(parts, "output "+output)
	}
	if compression := safeCandidateLabelValue(candidate.Compression); compression != "" {
		parts = append(parts, "compression "+compression)
	}
	if granularity := safeCandidateLabelValue(candidate.Granularity); granularity != "" {
		parts = append(parts, "granularity "+granularity)
	}
	if destination := safeCandidateLabelValue(candidate.Destination); destination != "" {
		parts = append(parts, "region "+destination)
	}
	return strings.Join(parts, ", ")
}

func safeCandidateLabelValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"/users/",
		"/private/",
		"/tmp/",
		"/home/",
		"\\",
		".pem",
		"access_key",
		"apikey",
		"api_key",
		"arn:",
		"bearer ",
		"client-secret",
		"client_secret",
		"ocid1.",
		"passphrase",
		"password",
		"plain-secret",
		"plain-token",
		"private-key",
		"private_key",
		"projects/",
		"refresh_token",
		"secret_key",
		"service_account_json",
		"session_token",
		"token=",
	} {
		if strings.Contains(lower, forbidden) {
			return ""
		}
	}
	if sensitiveCandidateIdentifierLikeValue(value) {
		return ""
	}
	if strings.ContainsAny(value, `/\`) {
		return ""
	}
	return value
}

func sensitiveCandidateIdentifierLikeValue(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 12 && allDigits(value) {
		return true
	}
	upper := strings.ToUpper(value)
	return len(upper) == 20 &&
		(strings.HasPrefix(upper, "AKIA") || strings.HasPrefix(upper, "ASIA")) &&
		allUpperAlphaNumeric(upper)
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func allUpperAlphaNumeric(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return value != ""
}
