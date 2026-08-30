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
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/s3handoff"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func handleAWSBillingResult(ctx context.Context, reader *bufio.Scanner, stdout io.Writer, config Config, selected awsVerifiedSource, result workflow.Result) error {
	if !isCUR2CandidateSelectionResult(result) {
		writeAWSBillingSummary(stdout, selected.Identity.Source, result)
		if shouldOfferCreateCUR2SetupFromBillingResult(result) {
			return offerCreateCUR2SetupPlanAfterBillingResult(reader, stdout, config, selected.Identity.Source)
		}
		return maybeRunBackfillPreviewAfterPreflight(stdout, config, selected.Identity.Source, result)
	}

	candidates := cur2Candidates(result)
	if len(candidates) == 0 {
		writeAWSBillingSummary(stdout, selected.Identity.Source, result)
		return nil
	}

	ranked := rankedCUR2Candidates(candidates)
	switch len(ranked) {
	case 1:
		candidate := ranked[0]
		if !isAutoSelectableCUR2Candidate(candidate) {
			return selectSingleCUR2CandidateAction(reader, stdout, config, selected.Identity.Source, candidate)
		}
		fmt.Fprintf(stdout, "Detected one usable CUR 2.0 export %s\n", candidate.Ref)
		writeCUR2CandidateSelectionFacts(stdout, candidate, "  ", isRecommendedCUR2Candidate(candidate))
		useDetected, err := readConfirmationDefaultYes(reader, stdout, "Use this detected CUR 2.0 export? [Y/n] ")
		if err != nil {
			return err
		}
		if !useDetected {
			return runCreateCUR2SetupPlanWithConfig(reader, stdout, config, selected.Identity.Source)
		}
		fmt.Fprintf(stdout, "Running readiness preflight for selected CUR 2.0 export %s\n", candidate.Ref)
		selectedResult := runSelectedCUR2PreflightWithConfig(config, selected.Identity.Source, candidate.Ref)
		writeAWSBillingSummary(stdout, selected.Identity.Source, selectedResult)
		return maybeRunBackfillPreviewAfterPreflight(stdout, config, selected.Identity.Source, selectedResult)
	default:
		fmt.Fprintln(stdout, "Select AWS CUR 2.0 export")
		for index, candidate := range ranked {
			recommended := index == 0 && isRecommendedCUR2Candidate(candidate)
			fmt.Fprintf(stdout, "  %d. %s\n", index+1, candidateLabel(candidate, recommended))
			writeCUR2CandidateSelectionFacts(stdout, candidate, "     ", recommended)
		}
		createNewIndex := len(ranked)
		choiceCount := len(ranked) + 1
		fmt.Fprintf(stdout, "  %d. Prepare a new Matilda CUR 2.0 setup plan\n", createNewIndex+1)
		fmt.Fprintln(stdout, "     Use this when none of the discovered exports is the one you want.")
		index, err := readChoice(reader, stdout, fmt.Sprintf("Select AWS CUR 2.0 export [1-%d]: ", choiceCount), "AWS CUR 2.0 export", choiceCount)
		if err != nil {
			return err
		}
		if index == createNewIndex {
			return runCreateCUR2SetupPlanWithConfig(reader, stdout, config, selected.Identity.Source)
		}
		selectedCandidate := ranked[index]
		fmt.Fprintf(stdout, "Running readiness preflight for selected CUR 2.0 export %s\n", selectedCandidate.Ref)
		selectedResult := runSelectedCUR2PreflightWithConfig(config, selected.Identity.Source, selectedCandidate.Ref)
		writeAWSBillingSummary(stdout, selected.Identity.Source, selectedResult)
		return maybeRunBackfillPreviewAfterPreflight(stdout, config, selected.Identity.Source, selectedResult)
	}
}

func selectSingleCUR2CandidateAction(reader *bufio.Scanner, stdout io.Writer, config Config, source billingguide.CredentialSource, candidate cur2Candidate) error {
	writeSingleCUR2CandidateNeedsReview(stdout, candidate)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Select AWS CUR 2.0 action")
	fmt.Fprintln(stdout, "  1. Review this CUR 2.0 export with full readiness preflight")
	fmt.Fprintln(stdout, "  2. Prepare a new Matilda CUR 2.0 setup plan")
	fmt.Fprintln(stdout, "     Use this when the discovered export is not the one you want.")
	index, err := readChoice(reader, stdout, "Select AWS CUR 2.0 action [1-2]: ", "AWS CUR 2.0 action", 2)
	if err != nil {
		return err
	}
	if index == 1 {
		return runCreateCUR2SetupPlanWithConfig(reader, stdout, config, source)
	}

	fmt.Fprintf(stdout, "Running readiness preflight for selected CUR 2.0 export %s\n", candidate.Ref)
	selectedResult := runSelectedCUR2PreflightWithConfig(config, source, candidate.Ref)
	writeAWSBillingSummary(stdout, source, selectedResult)
	return maybeRunBackfillPreviewAfterPreflight(stdout, config, source, selectedResult)
}

func isCUR2CandidateSelectionResult(result workflow.Result) bool {
	switch result.Code {
	case "aws_cur2_export_ambiguous", "aws_cur2_export_selection_required":
		return true
	default:
		return false
	}
}

func shouldOfferCreateCUR2SetupFromBillingResult(result workflow.Result) bool {
	switch result.Code {
	case "aws_cur2_export_not_found", "aws_non_cur2_source_out_of_scope":
		return true
	default:
		return false
	}
}

func offerCreateCUR2SetupPlanAfterBillingResult(reader *bufio.Scanner, stdout io.Writer, config Config, source billingguide.CredentialSource) error {
	prepare, err := readConfirmationDefaultYes(reader, stdout, "Prepare a new Matilda CUR 2.0 setup plan now? [Y/n] ")
	if err != nil {
		return err
	}
	if !prepare {
		fmt.Fprintln(stdout, "Guided setup stopped. No cloud changes were made.")
		return nil
	}
	return runCreateCUR2SetupPlanWithConfig(reader, stdout, config, source)
}

type cur2Candidate struct {
	Index           int
	Ref             string
	Health          string
	Output          string
	Compression     string
	Granularity     string
	Overwrite       string
	OutputType      string
	RefreshCadence  string
	IncludeResource string
	Destination     string
	ProviderSource  string
	CloudValidity   string
	MatildaSupport  string
	MetadataStatus  string
	PrimaryIssue    string
	RequiredAction  string
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
			case "overwrite":
				candidate.Overwrite = safeCandidateLabelValue(evidence.Value)
			case "output_type":
				candidate.OutputType = safeCandidateLabelValue(evidence.Value)
			case "refresh_cadence":
				candidate.RefreshCadence = safeCandidateLabelValue(evidence.Value)
			case "include_resources":
				candidate.IncludeResource = safeCandidateLabelValue(evidence.Value)
			case "destination_region":
				candidate.Destination = safeCandidateLabelValue(evidence.Value)
			case "provider_source_type":
				candidate.ProviderSource = safeCandidateLabelValue(evidence.Value)
			case "cloud_validity":
				candidate.CloudValidity = safeCandidateLabelValue(evidence.Value)
			case "matilda_support":
				candidate.MatildaSupport = safeCandidateLabelValue(evidence.Value)
			case "pre_selection_metadata_status":
				candidate.MetadataStatus = safeCandidateLabelValue(evidence.Value)
			case "primary_issue":
				candidate.PrimaryIssue = safeCandidateLabelValue(evidence.Value)
			case "required_next_action":
				candidate.RequiredAction = safeCandidateLabelValue(evidence.Value)
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

func rankedCUR2Candidates(candidates []cur2Candidate) []cur2Candidate {
	ranked := append([]cur2Candidate(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return cur2CandidatePreferenceScore(ranked[i]) > cur2CandidatePreferenceScore(ranked[j])
	})
	return ranked
}

func cur2CandidatePreferenceScore(candidate cur2Candidate) int {
	if strings.EqualFold(candidate.Health, "UNHEALTHY") {
		return -1000 + cur2CandidateShapePreferenceScore(candidate)
	}

	score := 0
	switch {
	case hasSupportedCUR2SelectionMetadata(candidate):
		score += 3000
	case hasUnsupportedCUR2SelectionMetadata(candidate):
		score += 0
	case hasCompleteCUR2SelectionMetadata(candidate):
		score += 2000
	case hasIncompleteCUR2SelectionMetadata(candidate):
		score += 1000
	default:
		score += 0
	}
	score += cur2CandidateHealthScore(candidate)
	score += cur2CandidateShapePreferenceScore(candidate)
	return score
}

func cur2CandidateHealthScore(candidate cur2Candidate) int {
	switch strings.ToUpper(candidate.Health) {
	case "HEALTHY":
		return 100
	default:
		return 0
	}
}

func cur2CandidateShapePreferenceScore(candidate cur2Candidate) int {
	score := 0
	if strings.EqualFold(candidate.Output, "TEXT_OR_CSV") && strings.EqualFold(candidate.Compression, "GZIP") {
		score += 40
	}
	if strings.EqualFold(candidate.Output, "PARQUET") && strings.EqualFold(candidate.Compression, "PARQUET") {
		score += 25
	}
	switch strings.ToUpper(candidate.Granularity) {
	case "MONTHLY":
		score += 30
	case "DAILY":
		score += 15
	case "HOURLY":
		score += 5
	}
	switch strings.ToUpper(candidate.Overwrite) {
	case "CREATE_NEW_REPORT":
		score += 10
	case "OVERWRITE_REPORT":
		score += 5
	}
	return score
}

func hasSupportedCUR2SelectionMetadata(candidate cur2Candidate) bool {
	if candidate.MetadataStatus != "" {
		return strings.EqualFold(candidate.MetadataStatus, "preferred") ||
			strings.EqualFold(candidate.MetadataStatus, "supported")
	}
	return false
}

func hasSupportedCUR2Settings(candidate cur2Candidate) bool {
	return hasSupportedCUR2Output(candidate) &&
		hasSupportedCUR2Granularity(candidate) &&
		hasSupportedCUR2Overwrite(candidate) &&
		hasSupportedCUR2OutputType(candidate) &&
		hasSupportedCUR2RefreshCadence(candidate) &&
		hasSupportedCUR2IncludeResources(candidate)
}

func hasUnsupportedCUR2SelectionMetadata(candidate cur2Candidate) bool {
	return len(unsupportedCUR2CandidateSettings(candidate)) > 0
}

func hasCompleteCUR2SelectionMetadata(candidate cur2Candidate) bool {
	return !hasIncompleteCUR2SelectionMetadata(candidate)
}

func hasIncompleteCUR2SelectionMetadata(candidate cur2Candidate) bool {
	return safeCandidateLabelValue(candidate.Health) == "" ||
		safeCandidateLabelValue(candidate.Output) == "" ||
		safeCandidateLabelValue(candidate.Compression) == "" ||
		safeCandidateLabelValue(candidate.Granularity) == "" ||
		safeCandidateLabelValue(candidate.Overwrite) == "" ||
		safeCandidateLabelValue(candidate.OutputType) == "" ||
		safeCandidateLabelValue(candidate.RefreshCadence) == "" ||
		safeCandidateLabelValue(candidate.Destination) == ""
}

func hasSupportedCUR2Output(candidate cur2Candidate) bool {
	return strings.EqualFold(candidate.Output, "TEXT_OR_CSV") && strings.EqualFold(candidate.Compression, "GZIP") ||
		strings.EqualFold(candidate.Output, "PARQUET") && strings.EqualFold(candidate.Compression, "PARQUET")
}

func hasSupportedCUR2Granularity(candidate cur2Candidate) bool {
	switch strings.ToUpper(candidate.Granularity) {
	case "HOURLY", "DAILY", "MONTHLY":
		return true
	default:
		return false
	}
}

func hasSupportedCUR2Overwrite(candidate cur2Candidate) bool {
	switch strings.ToUpper(candidate.Overwrite) {
	case "CREATE_NEW_REPORT", "OVERWRITE_REPORT":
		return true
	default:
		return false
	}
}

func hasSupportedCUR2OutputType(candidate cur2Candidate) bool {
	outputType := safeCandidateLabelValue(candidate.OutputType)
	return strings.EqualFold(outputType, "CUSTOM")
}

func hasSupportedCUR2RefreshCadence(candidate cur2Candidate) bool {
	refreshCadence := safeCandidateLabelValue(candidate.RefreshCadence)
	return strings.EqualFold(refreshCadence, "SYNCHRONOUS")
}

func hasSupportedCUR2IncludeResources(candidate cur2Candidate) bool {
	includeResources := safeCandidateLabelValue(candidate.IncludeResource)
	return includeResources == "" ||
		strings.EqualFold(includeResources, "TRUE") ||
		strings.EqualFold(includeResources, "FALSE")
}

func isRecommendedCUR2Candidate(candidate cur2Candidate) bool {
	return strings.EqualFold(candidate.MetadataStatus, "preferred") &&
		strings.EqualFold(candidate.MatildaSupport, "preferred")
}

func isAutoSelectableCUR2Candidate(candidate cur2Candidate) bool {
	return hasSupportedCUR2SelectionMetadata(candidate) &&
		cur2CandidateSelectionBlocker(candidate) == ""
}

func runSelectedCUR2PreflightWithConfig(config Config, source billingguide.CredentialSource, exportRef string) workflow.Result {
	ctx, cancel := guidedContext(config)
	defer cancel()
	return runSelectedCUR2Preflight(ctx, config.Registry, source, exportRef)
}

func maybeRunBackfillPreviewAfterPreflight(stdout io.Writer, config Config, source billingguide.CredentialSource, result workflow.Result) error {
	if result.Code != "aws_backfill_manual_step_required" {
		return nil
	}
	exportRef := selectedExportRef(result)
	if exportRef == "" {
		writeBackfillPreviewRefUnavailable(stdout, source)
		return nil
	}
	return runBackfillPreview(stdout, config, source, exportRef)
}

func continueAfterCreateCUR2Setup(stdout io.Writer, config Config, source billingguide.CredentialSource, result workflow.Result) error {
	exportRef := selectedExportRef(result)
	if exportRef == "" {
		writeCreateCUR2FollowupRefUnavailable(stdout, source)
		return nil
	}
	fmt.Fprintf(stdout, "Validating selected Matilda AWS CUR 2.0 export %s.\n", exportRef)
	selectedResult := runSelectedCUR2PreflightWithConfig(config, source, exportRef)
	writeAWSBillingSummary(stdout, source, selectedResult)
	return maybeRunBackfillPreviewAfterPreflight(stdout, config, source, selectedResult)
}

func runBackfillPreview(stdout io.Writer, config Config, source billingguide.CredentialSource, exportRef string) error {
	fmt.Fprintln(stdout, "Preparing AWS Support backfill request plan.")
	ctx, cancel := guidedContext(config)
	defer cancel()
	result := runBackfillPreviewWithConfig(ctx, config.Registry, source, exportRef)
	writeAWSBillingSummary(stdout, source, result)
	return nil
}

func runBackfillPreviewWithConfig(ctx context.Context, registry workflow.Registry, source billingguide.CredentialSource, exportRef string) workflow.Result {
	request := awsBillingApplyPrereqsRequest()
	options, err := awsBillingOptions(source)
	if err != nil {
		return workflow.Result{
			Status:  workflow.RunStatusBlocked,
			Code:    "aws_config_invalid_selector",
			Message: "Selected AWS credential source contains unsafe selector metadata.",
			Request: request,
		}
	}
	if options.Selectors == nil {
		options.Selectors = &workflow.ExecutionSelectors{}
	}
	if options.Selectors.AWS == nil {
		options.Selectors.AWS = &workflow.AWSExecutionSelectors{}
	}
	options.Selectors.AWS.CUR2ExportRef = exportRef
	options.AWSBillingOperation = workflow.AWSBillingOperationRequestBackfill
	options, err = workflow.NormalizeExecutionOptionsForRequest(request, options)
	if err != nil {
		return workflow.Result{
			Status:  workflow.RunStatusBlocked,
			Code:    "aws_cur2_export_ref_invalid",
			Message: "Selected AWS CUR 2.0 export reference is invalid.",
			Request: request,
		}
	}
	return registry.ExecuteContext(ctx, request, options)
}

func writeBackfillPreviewRefUnavailable(stdout io.Writer, source billingguide.CredentialSource) {
	fmt.Fprintln(stdout, "Backfill request planning needs a safe CUR 2.0 export ref. Rerun AWS billing preflight to select the export explicitly.")
	fmt.Fprintln(stdout, "Next command:")
	fmt.Fprintf(stdout, "  %s\n", directAWSBillingCommand(source, ""))
}

func writeCreateCUR2FollowupRefUnavailable(stdout io.Writer, source billingguide.CredentialSource) {
	fmt.Fprintln(stdout, "Follow-up validation needs a safe CUR 2.0 export ref. Rerun AWS billing preflight to rediscover the created export.")
	fmt.Fprintln(stdout, "Next command:")
	fmt.Fprintf(stdout, "  %s\n", directAWSBillingCommand(source, ""))
}

func runCreateCUR2SetupPlanWithConfig(reader *bufio.Scanner, stdout io.Writer, config Config, source billingguide.CredentialSource) error {
	destinationMode, err := selectCreateCUR2Destination(reader, stdout)
	if err != nil {
		return err
	}
	bucketRef := ""
	if destinationMode == workflow.AWSCUR2DestinationExistingSameAccount {
		fmt.Fprintln(stdout, "Discovering existing S3 buckets in the selected AWS account.")
		ctx, cancel := guidedContext(config)
		selection := runCreateCUR2SetupPlanWithDestination(ctx, config.Registry, source, destinationMode, "")
		cancel()
		if !isExistingBucketSelectionResult(selection) {
			writeAWSBillingSummary(stdout, source, selection)
			if shouldOfferGeneratedBucketFallbackAfterExistingBucketSelection(selection) {
				return offerGeneratedBucketFallbackAfterExistingBucketSelection(reader, stdout, config, source)
			}
			return nil
		}
		candidates := existingS3BucketCandidates(selection)
		if len(candidates) == 0 {
			writeAWSBillingSummary(stdout, source, selection)
			return offerGeneratedBucketFallbackAfterExistingBucketSelection(reader, stdout, config, source)
		}
		selectedRef, err := selectExistingS3Bucket(reader, stdout, candidates)
		if err != nil {
			return err
		}
		bucketRef = selectedRef
	}

	return runCreateCUR2SetupPlanForDestination(reader, stdout, config, source, destinationMode, bucketRef)
}

func runCreateCUR2SetupPlanForDestination(reader *bufio.Scanner, stdout io.Writer, config Config, source billingguide.CredentialSource, destinationMode workflow.AWSCUR2DestinationMode, bucketRef string) error {
	fmt.Fprintln(stdout, "Preparing a new Matilda AWS CUR 2.0 setup plan.")
	ctx, cancel := guidedContext(config)
	result := runCreateCUR2SetupPlanWithDestination(ctx, config.Registry, source, destinationMode, bucketRef)
	cancel()
	writeAWSBillingSummary(stdout, source, result)
	if isCompletedCreateCUR2SetupResult(result) {
		return continueAfterCreateCUR2Setup(stdout, config, source, result)
	}
	if !shouldOfferCreateCUR2GuidedApply(result) {
		return nil
	}
	apply, err := readConfirmation(reader, stdout, "Apply this AWS CUR 2.0 setup plan now? [y/N] ")
	if err != nil {
		return err
	}
	if !apply {
		fmt.Fprintln(stdout, "Guided apply skipped. No cloud changes were made.")
		return nil
	}

	fmt.Fprintln(stdout, "Applying approved Matilda AWS CUR 2.0 setup plan.")
	applyCtx, applyCancel := guidedContext(config)
	defer applyCancel()
	applied := runApprovedCreateCUR2SetupPlan(applyCtx, config.Registry, source, result)
	writeAWSBillingSummary(stdout, source, applied)
	return continueAfterCreateCUR2Setup(stdout, config, source, applied)
}

func shouldOfferGeneratedBucketFallbackAfterExistingBucketSelection(result workflow.Result) bool {
	switch result.Code {
	case "aws_s3_list_buckets_failed", "aws_s3_list_buckets_pagination_unbounded":
		return true
	default:
		return false
	}
}

func offerGeneratedBucketFallbackAfterExistingBucketSelection(reader *bufio.Scanner, stdout io.Writer, config Config, source billingguide.CredentialSource) error {
	useGenerated, err := readConfirmationDefaultYes(reader, stdout, "Use a generated same-account Matilda S3 bucket instead? [Y/n] ")
	if err != nil {
		return err
	}
	if !useGenerated {
		fmt.Fprintln(stdout, "Guided setup stopped. No cloud changes were made.")
		return nil
	}
	return runCreateCUR2SetupPlanForDestination(reader, stdout, config, source, workflow.AWSCUR2DestinationGenerated, "")
}

func selectCreateCUR2Destination(reader *bufio.Scanner, stdout io.Writer) (workflow.AWSCUR2DestinationMode, error) {
	fmt.Fprintln(stdout, "Select AWS CUR 2.0 destination")
	fmt.Fprintln(stdout, "  1. Use a generated same-account Matilda S3 bucket")
	fmt.Fprintln(stdout, "     Recommended when you do not need to reuse an existing bucket.")
	fmt.Fprintln(stdout, "  2. Use an existing S3 bucket owned by this AWS account")
	fmt.Fprintln(stdout, "     Matilda Cloud Prep will list owned buckets and bind your selection by safe reference.")
	index, err := readChoice(reader, stdout, "Select AWS CUR 2.0 destination [1-2]: ", "AWS CUR 2.0 destination", 2)
	if err != nil {
		return "", err
	}
	if index == 1 {
		return workflow.AWSCUR2DestinationExistingSameAccount, nil
	}
	return workflow.AWSCUR2DestinationGenerated, nil
}

type existingS3BucketCandidate struct {
	Index  int
	Ref    string
	Label  string
	Region string
}

func existingS3BucketCandidates(result workflow.Result) []existingS3BucketCandidate {
	if result.Plan == nil {
		return nil
	}
	byIndex := map[int]*existingS3BucketCandidate{}
	for _, check := range result.Plan.Checks {
		for _, evidence := range check.Evidence {
			index, field, ok := candidateEvidenceKey(evidence.Key)
			if !ok {
				continue
			}
			candidate := byIndex[index]
			if candidate == nil {
				candidate = &existingS3BucketCandidate{Index: index}
				byIndex[index] = candidate
			}
			switch field {
			case "bucket_ref":
				if safeS3BucketRef(evidence.Value) {
					candidate.Ref = strings.TrimSpace(evidence.Value)
				}
			case "bucket_label":
				candidate.Label = safeCandidateLabelValue(evidence.Value)
			case "bucket_region":
				candidate.Region = safeCandidateLabelValue(evidence.Value)
			}
		}
	}

	indexes := make([]int, 0, len(byIndex))
	for index := range byIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	candidates := make([]existingS3BucketCandidate, 0, len(indexes))
	for _, index := range indexes {
		candidate := *byIndex[index]
		if candidate.Ref == "" {
			continue
		}
		if candidate.Label == "" {
			candidate.Label = candidate.Ref
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func selectExistingS3Bucket(reader *bufio.Scanner, stdout io.Writer, candidates []existingS3BucketCandidate) (string, error) {
	fmt.Fprintln(stdout, "Select S3 bucket for new Matilda CUR 2.0 export")
	for index, candidate := range candidates {
		label := candidate.Label
		if candidate.Region != "" {
			label += " in " + candidate.Region
		}
		fmt.Fprintf(stdout, "  %d. %s (%s)\n", index+1, label, candidate.Ref)
	}
	index, err := readChoice(reader, stdout, fmt.Sprintf("Select S3 bucket [1-%d]: ", len(candidates)), "S3 bucket", len(candidates))
	if err != nil {
		return "", err
	}
	return candidates[index].Ref, nil
}

func runSelectedCUR2Preflight(ctx context.Context, registry workflow.Registry, source billingguide.CredentialSource, exportRef string) workflow.Result {
	options, err := awsBillingOptions(source)
	if err != nil {
		return workflow.Result{
			Status:  workflow.RunStatusBlocked,
			Code:    "aws_config_invalid_selector",
			Message: "Selected AWS credential source contains unsafe selector metadata.",
		}
	}
	if options.Selectors == nil {
		options.Selectors = &workflow.ExecutionSelectors{}
	}
	if options.Selectors.AWS == nil {
		options.Selectors.AWS = &workflow.AWSExecutionSelectors{}
	}
	options.Selectors.AWS.CUR2ExportRef = exportRef
	options, err = workflow.NormalizeExecutionOptions(options)
	if err != nil {
		return workflow.Result{
			Status:  workflow.RunStatusBlocked,
			Code:    "aws_cur2_export_ref_invalid",
			Message: "Selected AWS CUR 2.0 export reference is invalid.",
		}
	}
	return registry.ExecuteContext(ctx, awsBillingRequest(), options)
}

func runCreateCUR2SetupPlan(ctx context.Context, registry workflow.Registry, source billingguide.CredentialSource) workflow.Result {
	return runCreateCUR2SetupPlanWithDestination(ctx, registry, source, workflow.AWSCUR2DestinationGenerated, "")
}

func runCreateCUR2SetupPlanWithDestination(ctx context.Context, registry workflow.Registry, source billingguide.CredentialSource, destinationMode workflow.AWSCUR2DestinationMode, bucketRef string) workflow.Result {
	request := awsBillingApplyPrereqsRequest()
	options, err := awsBillingOptions(source)
	if err != nil {
		return workflow.Result{
			Status:  workflow.RunStatusBlocked,
			Code:    "aws_config_invalid_selector",
			Message: "Selected AWS credential source contains unsafe selector metadata.",
		}
	}
	options.AWSBillingOperation = workflow.AWSBillingOperationCreateCUR2Export
	if options.Selectors == nil {
		options.Selectors = &workflow.ExecutionSelectors{}
	}
	if options.Selectors.AWS == nil {
		options.Selectors.AWS = &workflow.AWSExecutionSelectors{}
	}
	if destinationMode != "" && destinationMode != workflow.AWSCUR2DestinationGenerated {
		options.Selectors.AWS.CUR2DestinationMode = destinationMode
	}
	if bucketRef != "" {
		options.Selectors.AWS.CUR2S3BucketRef = bucketRef
	}
	options, err = workflow.NormalizeExecutionOptionsForRequest(request, options)
	if err != nil {
		return workflow.Result{
			Status:  workflow.RunStatusBlocked,
			Code:    "execution_options_invalid",
			Message: "AWS CUR 2.0 setup options are invalid or unsafe.",
		}
	}
	return registry.ExecuteContext(ctx, request, options)
}

func isExistingBucketSelectionResult(result workflow.Result) bool {
	return result.Code == "aws_cur2_existing_bucket_selection_required"
}

func shouldOfferCreateCUR2GuidedApply(result workflow.Result) bool {
	if !isCreateCUR2SetupResult(result) || result.Mutated || result.Plan == nil {
		return false
	}
	approval := result.Plan.Approval
	if !approval.Required || approval.Blocked || approval.Approved || approval.ApprovalPlanID == "" {
		return false
	}
	for _, step := range result.Plan.Steps {
		if step.RequiresApproval {
			return true
		}
	}
	return false
}

func runApprovedCreateCUR2SetupPlan(ctx context.Context, registry workflow.Registry, source billingguide.CredentialSource, preview workflow.Result) workflow.Result {
	request := awsBillingApplyPrereqsRequest()
	options, err := createCUR2SetupApprovalOptions(source, preview)
	if err != nil {
		return workflow.Result{
			Status:  workflow.RunStatusBlocked,
			Code:    "aws_cur2_create_export_approval_unavailable",
			Message: "AWS CUR 2.0 setup approval could not be built from the current guided plan.",
			Request: request,
		}
	}
	return registry.ExecuteContext(ctx, request, options)
}

func createCUR2SetupApprovalOptions(source billingguide.CredentialSource, preview workflow.Result) (workflow.ExecutionOptions, error) {
	request := awsBillingApplyPrereqsRequest()
	options, err := awsBillingOptions(source)
	if err != nil {
		return workflow.ExecutionOptions{}, err
	}
	options.AWSBillingOperation = workflow.AWSBillingOperationCreateCUR2Export
	preserveCreateCUR2DestinationSelectors(&options, preview)
	if preview.Plan == nil {
		return workflow.ExecutionOptions{}, fmt.Errorf("create CUR 2.0 setup plan is missing")
	}
	approval := preview.Plan.Approval
	if !approval.Required || approval.Blocked || approval.Approved {
		return workflow.ExecutionOptions{}, fmt.Errorf("create CUR 2.0 setup plan is not approvable")
	}
	planID := strings.TrimSpace(approval.ApprovalPlanID)
	if planID == "" {
		return workflow.ExecutionOptions{}, fmt.Errorf("create CUR 2.0 setup plan approval ID is missing")
	}
	for _, step := range preview.Plan.Steps {
		if !step.RequiresApproval {
			continue
		}
		options.Approvals = append(options.Approvals, workflow.ExecutionApproval{
			OperationID: step.ID,
			PlanID:      planID,
			Confirmed:   true,
		})
	}
	if len(options.Approvals) == 0 {
		return workflow.ExecutionOptions{}, fmt.Errorf("create CUR 2.0 setup plan has no mutating steps to approve")
	}
	return workflow.NormalizeExecutionOptionsForRequest(request, options)
}

func preserveCreateCUR2DestinationSelectors(options *workflow.ExecutionOptions, preview workflow.Result) {
	if options == nil || preview.ExecutionOptions.Selectors == nil || preview.ExecutionOptions.Selectors.AWS == nil {
		return
	}
	if options.Selectors == nil {
		options.Selectors = &workflow.ExecutionSelectors{}
	}
	if options.Selectors.AWS == nil {
		options.Selectors.AWS = &workflow.AWSExecutionSelectors{}
	}
	previewAWS := preview.ExecutionOptions.Selectors.AWS
	options.Selectors.AWS.CUR2DestinationMode = previewAWS.CUR2DestinationMode
	options.Selectors.AWS.CUR2S3BucketRef = previewAWS.CUR2S3BucketRef
}

func safeCUR2ExportRef(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
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

func safeS3BucketRef(value string) bool {
	options := workflow.ExecutionOptions{
		AWSBillingOperation: workflow.AWSBillingOperationCreateCUR2Export,
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				CUR2DestinationMode: workflow.AWSCUR2DestinationExistingSameAccount,
				CUR2S3BucketRef:     value,
			},
		},
	}
	_, err := workflow.NormalizeExecutionOptionsForRequest(awsBillingApplyPrereqsRequest(), options)
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
	case "aws_s3_delivery_policy_missing":
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

func writeNonReadyCUR2Candidate(stdout io.Writer, item classifiedCUR2Candidate) {
	writeCUR2CandidateDetails(stdout, item, "not ready", nonReadyCUR2NextAction(item))
}

func writeSingleCUR2CandidateNeedsReview(stdout io.Writer, candidate cur2Candidate) {
	fmt.Fprintln(stdout, "One AWS CUR 2.0 export candidate needs review.")
	fmt.Fprintf(stdout, "  %s\n", candidateLabel(candidate))
	writeCUR2CandidateSelectionFacts(stdout, candidate, "    ", false)
}

func writeCUR2CandidateSelectionFacts(stdout io.Writer, candidate cur2Candidate, indent string, recommended bool) {
	if recommended {
		fmt.Fprintf(stdout, "%sRecommendation: preferred Rapid Assessment billing export shape.\n", indent)
	}
	if reason := cur2CandidateSelectionBlocker(candidate); reason != "" {
		fmt.Fprintf(stdout, "%sBlocker: %s\n", indent, reason)
	}
	switch strings.ToUpper(candidate.Granularity) {
	case "DAILY":
		fmt.Fprintf(stdout, "%sNote: daily is valid AWS CUR 2.0; monthly is preferred for Rapid Assessment billing when available.\n", indent)
	case "HOURLY":
		fmt.Fprintf(stdout, "%sNote: hourly is valid AWS CUR 2.0; monthly is preferred and hourly can increase file volume.\n", indent)
	}
	fmt.Fprintf(stdout, "%sFull readiness checks run after selection.\n", indent)
}

func cur2CandidateSelectionBlocker(candidate cur2Candidate) string {
	if candidate.PrimaryIssue != "" && !strings.EqualFold(candidate.PrimaryIssue, "none") {
		switch strings.ToLower(candidate.MetadataStatus) {
		case "incomplete", "unsupported", "unhealthy":
			return candidate.PrimaryIssue
		}
	}
	if strings.EqualFold(safeCandidateLabelValue(candidate.Health), "UNHEALTHY") {
		return "AWS reports this export as unhealthy."
	}
	if unsupported := unsupportedCUR2CandidateSettings(candidate); len(unsupported) > 0 {
		return "pre-selection metadata has unsupported settings: " + strings.Join(unsupported, ", ") + "."
	}
	if missing := missingCUR2CandidateMetadata(candidate); len(missing) > 0 {
		return "pre-selection metadata is incomplete: missing " + strings.Join(missing, ", ") + "."
	}
	return ""
}

func missingCUR2CandidateMetadata(candidate cur2Candidate) []string {
	missing := []string{}
	if safeCandidateLabelValue(candidate.Health) == "" {
		missing = append(missing, "health status")
	}
	if safeCandidateLabelValue(candidate.Output) == "" {
		missing = append(missing, "output format")
	}
	if safeCandidateLabelValue(candidate.Compression) == "" {
		missing = append(missing, "compression")
	}
	if safeCandidateLabelValue(candidate.Granularity) == "" {
		missing = append(missing, "time granularity")
	}
	if safeCandidateLabelValue(candidate.Overwrite) == "" {
		missing = append(missing, "file versioning")
	}
	if safeCandidateLabelValue(candidate.OutputType) == "" {
		missing = append(missing, "output type")
	}
	if safeCandidateLabelValue(candidate.RefreshCadence) == "" {
		missing = append(missing, "refresh cadence")
	}
	if safeCandidateLabelValue(candidate.Destination) == "" {
		missing = append(missing, "destination region")
	}
	return missing
}

func unsupportedCUR2CandidateSettings(candidate cur2Candidate) []string {
	unsupported := []string{}
	health := safeCandidateLabelValue(candidate.Health)
	if health != "" && !strings.EqualFold(health, "HEALTHY") && !strings.EqualFold(health, "UNHEALTHY") {
		unsupported = append(unsupported, "health status "+health)
	}
	if !hasSupportedCUR2Output(candidate) {
		output := safeCandidateLabelValue(candidate.Output)
		compression := safeCandidateLabelValue(candidate.Compression)
		if output != "" && compression != "" {
			unsupported = append(unsupported, "output/compression "+output+"/"+compression)
		}
	}
	if granularity := safeCandidateLabelValue(candidate.Granularity); granularity != "" && !hasSupportedCUR2Granularity(candidate) {
		unsupported = append(unsupported, "time granularity "+granularity)
	}
	if overwrite := safeCandidateLabelValue(candidate.Overwrite); overwrite != "" && !hasSupportedCUR2Overwrite(candidate) {
		unsupported = append(unsupported, "file versioning "+overwrite)
	}
	if outputType := safeCandidateLabelValue(candidate.OutputType); outputType != "" && !hasSupportedCUR2OutputType(candidate) {
		unsupported = append(unsupported, "output type "+outputType)
	}
	if refreshCadence := safeCandidateLabelValue(candidate.RefreshCadence); refreshCadence != "" && !hasSupportedCUR2RefreshCadence(candidate) {
		unsupported = append(unsupported, "refresh cadence "+refreshCadence)
	}
	if includeResources := safeCandidateLabelValue(candidate.IncludeResource); includeResources != "" && !hasSupportedCUR2IncludeResources(candidate) {
		unsupported = append(unsupported, "include resources "+includeResources)
	}
	return unsupported
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
	if shouldRenderCUR2HandoffLocationFacts(item) {
		writeCUR2HandoffLocationFacts(stdout, facts, indent)
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
			case "output_format", "compression", "time_granularity", "overwrite", "previous_billing_period", "missing_previous_month_component", "s3_bucket", "s3_prefix", "s3_region", "cur2_data_prefix", "cur2_manifest_prefix", "policy_gap":
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

func shouldRenderCUR2HandoffLocationFacts(item classifiedCUR2Candidate) bool {
	if item.Result.Status == workflow.StatusReady {
		return true
	}
	return item.Result.Status == workflow.RunStatusManualSteps &&
		item.Result.Code == "aws_backfill_manual_step_required"
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
	S3Bucket              string
	S3Prefix              string
	S3Region              string
	DataPrefix            string
	ManifestPrefix        string
	UnsafeHandoffEvidence bool
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
		facts.observeCheck(check, checkCode)
		for _, evidence := range check.Evidence {
			switch evidence.Key {
			case "s3_bucket":
				if value := s3handoff.Bucket(evidence.Value); value != "" {
					facts.S3Bucket = value
				} else if strings.TrimSpace(evidence.Value) != "" {
					facts.UnsafeHandoffEvidence = true
				}
				continue
			case "s3_prefix":
				if value := s3handoff.ConfiguredPrefix(evidence.Value); value != "" {
					facts.S3Prefix = value
				} else if strings.TrimSpace(evidence.Value) != "" {
					facts.UnsafeHandoffEvidence = true
				}
				continue
			case "s3_region":
				if value := s3handoff.Region(evidence.Value); value != "" {
					facts.S3Region = value
				} else if strings.TrimSpace(evidence.Value) != "" {
					facts.UnsafeHandoffEvidence = true
				}
				continue
			case "cur2_data_prefix":
				if value := s3handoff.ReportPrefix(evidence.Value); value != "" {
					facts.DataPrefix = value
				} else if strings.TrimSpace(evidence.Value) != "" {
					facts.UnsafeHandoffEvidence = true
				}
				continue
			case "cur2_manifest_prefix":
				if value := s3handoff.ReportPrefix(evidence.Value); value != "" {
					facts.ManifestPrefix = value
				} else if strings.TrimSpace(evidence.Value) != "" {
					facts.UnsafeHandoffEvidence = true
				}
				continue
			}
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

func (facts *cur2CandidateFacts) observeCheck(check workflow.PlanCheck, code string) {
	switch code {
	case "aws_cur2_delivery_ready":
		if check.Status == workflow.CheckPass {
			facts.DeliveryStatus = "ready"
		}
	case "aws_cur2_delivery_not_started":
		if check.Status == workflow.CheckWarn {
			facts.DeliveryStatus = deliveryWarningLabel(check.Message)
		}
		if check.Status == workflow.CheckFail {
			facts.DeliveryStatus = "not started"
		}
	case "aws_s3_delivery_policy_ready":
		if check.Status == workflow.CheckPass {
			facts.PolicyStatus = "ready"
		}
	case "aws_s3_bucket_policy_inaccessible":
		if check.Status == workflow.CheckWarn || check.Status == workflow.CheckFail {
			facts.PolicyStatus = "not inspected"
		}
	case "aws_s3_delivery_policy_missing":
		if check.Status == workflow.CheckWarn || check.Status == workflow.CheckFail {
			facts.PolicyStatus = "action needed"
		}
	case "aws_cur2_previous_month_ready":
		if check.Status == workflow.CheckPass {
			facts.PreviousMonthStatus = "ready"
		}
	}
}

func deliveryWarningLabel(message string) string {
	normalized := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(normalized, "still in progress"):
		return "in progress"
	case strings.Contains(normalized, "has started yet"):
		return "not started yet"
	case strings.Contains(normalized, "not conclusive"):
		return "not conclusive"
	default:
		return "not conclusive"
	}
}

func (facts *cur2CandidateFacts) deriveBlocker(status workflow.RunStatus, resultCode string) {
	if status != workflow.RunStatusBlocked {
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
		if facts.PolicyStatus == "action needed" || facts.PolicyStatus == "not inspected" {
			return "continue with this CUR 2.0 export; review the S3 delivery policy before relying on future delivery or backfill."
		}
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
	switch item.Result.Code {
	case "aws_cur2_output_settings_blocked":
		return "review the CUR 2.0 output settings and rerun after they match a Matilda-supported AWS-standard shape."
	case "aws_data_exports_throttled":
		return "AWS throttled the Data Exports read-only check. Wait briefly, then rerun preflight."
	case "aws_data_exports_transient":
		return "retry preflight after the transient AWS Data Exports issue clears."
	case "aws_s3_bucket_policy_inaccessible":
		return "grant read access to inspect the S3 bucket policy, then rerun preflight."
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

func candidateLabel(candidate cur2Candidate, recommended ...bool) string {
	parts := []string{candidate.Ref}
	if len(recommended) > 0 && recommended[0] {
		parts = append(parts, "recommended")
	}
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
	if overwrite := safeCandidateLabelValue(candidate.Overwrite); overwrite != "" {
		parts = append(parts, "versioning "+overwrite)
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
	if s3handoff.SensitiveIdentifierLike(value) {
		return ""
	}
	if strings.ContainsAny(value, `/\`) {
		return ""
	}
	return value
}
