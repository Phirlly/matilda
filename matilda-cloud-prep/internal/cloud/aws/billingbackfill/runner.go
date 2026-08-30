package billingbackfill

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const maxListObjectPages = 5

type RunnerConfig struct {
	Client        Client
	ClientFactory func(workflow.ExecutionOptions) Client
	Now           time.Time
}

type Runner struct {
	client        Client
	clientFactory func(workflow.ExecutionOptions) Client
	now           time.Time
}

func NewRunner(config RunnerConfig) Runner {
	now := config.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Runner{
		client:        config.Client,
		clientFactory: config.ClientFactory,
		now:           now.UTC(),
	}
}

func (runner Runner) Run(ctx context.Context, request workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
	if request != AWSBillingApplyPrereqsRequest() {
		return runner.report(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_billing_apply_prereqs_request_required", "AWS billing backfill runner requires the AWS Rapid Assessment - Billing Based apply-prereqs request.", false, blockedStep(), failCheck("aws_billing_apply_prereqs_request_required", "AWS billing apply-prereqs request", "AWS billing backfill runner was called outside the AWS Rapid Assessment - Billing Based apply-prereqs path."))
	}
	if options.AWSBillingOperation != workflow.AWSBillingOperationRequestBackfill {
		return runner.report(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_backfill_operation_required", "AWS billing backfill runner requires the request-backfill operation intent.", false, blockedStep(), failCheck("aws_backfill_operation_required", "AWS billing backfill operation", "AWS billing backfill runner was called without the request-backfill operation intent."))
	}
	if awsCUR2ExportRefOption(options) == "" {
		return runner.report(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_cur2_export_selection_required", "Select one AWS CUR 2.0 export with --export-ref before requesting previous-month backfill.", false, blockedStep(), failCheck("aws_cur2_export_selection_required", "AWS CUR 2.0 export selection", "Rerun AWS billing preflight and pass the selected CUR 2.0 export ref with --export-ref before requesting previous-month backfill."))
	}

	client := runner.clientFor(options)
	if isNilClient(client) {
		return runner.report(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_provider_capability_blocked", "AWS billing apply-prereqs client is not configured.", false, blockedStep(), failCheck("aws_provider_capability_blocked", "AWS billing apply-prereqs", "AWS billing apply-prereqs client is not configured."))
	}

	preflightReport := cur2preflight.NewRunner(cur2preflight.RunnerConfig{
		Client: client,
		Now:    runner.now,
	}).Run(ctx, cur2preflight.AWSBillingPreflightRequest(), options)
	if preflightReport.Code != "aws_backfill_manual_step_required" {
		return runner.preflightNotBackfillReport(request, preflightReport)
	}

	context, err := runner.resolveBackfillContext(ctx, client, options)
	if err != nil {
		return runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_backfill_context_unavailable"), "AWS CUR 2.0 backfill context could not be resolved without unsafe assumptions.", false, blockedStep(), failCheck(providerErrorCode(err, "aws_backfill_context_unavailable"), "AWS CUR 2.0 backfill context", "AWS CUR 2.0 backfill context could not be resolved without unsafe assumptions."))
	}
	if !context.MissingDataPartition && !context.MissingManifest {
		return runner.verifiedReport(request, workflow.RunStatusReady, workflow.SupportSupported, "aws_backfill_not_required", "Previous-month AWS CUR 2.0 billing data is already present.", false, reuseStep(), passCheck("aws_backfill_not_required", "AWS previous-month billing data", "Previous-month AWS CUR 2.0 billing data is already present.", workflow.PlanEvidence{Key: "previous_billing_period", Value: context.Period}))
	}

	classification, err := resolveSupportClassification(ctx, client)
	if err != nil {
		code := providerErrorCode(err, "aws_support_case_manual_fallback_required")
		if manualFallbackProviderError(code) {
			return runner.verifiedReport(request, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_support_case_manual_fallback_required", "AWS Support API case creation is unavailable; use the manual AWS Support request path.", false, manualSupportCaseStep(), warnCheck("aws_support_case_manual_fallback_required", "AWS Support API availability", "AWS Support API case creation is unavailable for this account or support plan.", manualSupportRequestEvidence(context)...))
		}
		return runner.verifiedReport(request, workflow.RunStatusManualSteps, workflow.SupportGuided, code, "AWS Support case could not be created automatically; use the manual AWS Support request path.", false, manualSupportCaseStep(), warnCheck(code, "AWS Support case manual fallback", "AWS Support service, category, severity, or create-case options could not be resolved safely enough for automation.", manualSupportRequestEvidence(context)...))
	}
	ref := backfillRequestReference(context.Export.ExportARN, context.Period)
	approvalEvidence := context.approvalEvidence(supportCaseBindingRef(classification, context, ref))
	approvalStep := approvalRequiredStep()
	approvalCheck := warnCheck("aws_backfill_support_case_approval_required", "AWS Support case approval", "Review the current plan, then rerun apply-prereqs with a plan-bound backfill support-case approval.", approvalEvidence...)
	approvalInput := runner.verifiedReport(request, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_backfill_support_case_approval_required", "AWS Support case creation requires explicit plan-bound backfill approval.", false, approvalStep, approvalCheck).PlanInput
	if approvalInput == nil {
		return runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_backfill_plan_build_failed", "AWS Support backfill approval plan could not be built safely.", false, blockedStep(), failCheck("aws_backfill_plan_build_failed", "AWS Support case approval plan", "AWS Support backfill approval plan could not be built safely."))
	}
	previewInput := *approvalInput
	previewInput.ExecutionOptions = backfillPlanPreviewOptions(options)
	preview, err := workflow.BuildExecutionPlan(previewInput)
	if err != nil {
		return runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_backfill_plan_build_failed", "AWS Support backfill approval plan could not be built safely.", false, blockedStep(), failCheck("aws_backfill_plan_build_failed", "AWS Support case approval plan", "AWS Support backfill approval plan could not be built safely."))
	}

	switch approval := backfillApprovalState(options, preview.PlanID, preview.Steps); approval {
	case approvalMissing:
		return runner.verifiedReport(request, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_backfill_support_case_approval_required", "AWS Support case creation requires explicit plan-bound backfill approval.", false, approvalStep, approvalCheck)
	case approvalStale:
		return runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_stale", "Approved AWS Support backfill plan does not match the current plan. Review the new plan before creating a support case.", false, approvalStep, failCheck("aws_plan_stale", "AWS Support case approval", "The supplied approval plan ID does not match the current AWS backfill plan.", approvalEvidence...))
	case approvalMismatch:
		return runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_approval_mismatch", "Approved AWS Support backfill steps do not match the current mutating step set.", false, approvalStep, failCheck("aws_plan_approval_mismatch", "AWS Support case approval", "The supplied approved step set does not match the current AWS backfill support-case step.", approvalEvidence...))
	case approvalReady:
	default:
		return runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_approval_mismatch", "Approved AWS Support backfill steps do not match the current mutating step set.", false, approvalStep, failCheck("aws_plan_approval_mismatch", "AWS Support case approval", "The supplied approved step set does not match the current AWS backfill support-case step.", approvalEvidence...))
	}
	approvedPlanID := preview.PlanID

	existing, ok, err := findExistingOpenCase(ctx, client, ref)
	if err != nil {
		return withApprovedExecutionPlanID(runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_backfill_duplicate_check_failed", "AWS Support duplicate-case check failed; no case was created.", false, blockedStep(), failCheck("aws_backfill_duplicate_check_failed", "AWS Support duplicate-case check", "Existing AWS Support cases could not be checked safely before mutation.")), approvedPlanID)
	}
	if ok {
		return withApprovedExecutionPlanID(runner.verifiedReport(request, workflow.RunStatusManualSteps, workflow.SupportSupported, "aws_backfill_support_case_already_open", "An existing matching AWS Support case is already open.", false, reuseStep(), passCheck("aws_backfill_support_case_already_open", "AWS Support duplicate-case check", "An existing matching AWS Support case is already open.", supportCaseEvidence(existing, context.Period)...)), approvedPlanID)
	}

	caseRequest := buildCreateCaseRequest(classification, context, ref)
	created, err := client.CreateCase(ctx, caseRequest)
	if err != nil {
		code := providerErrorCode(err, "aws_support_create_case_failed")
		mutated := code == "aws_support_create_case_response_incomplete"
		return withApprovedExecutionPlanID(runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, code, createCaseFailureMessage(mutated), mutated, blockedStep(), failCheck(code, "AWS Support case creation", createCaseFailureCheckMessage(mutated))), approvedPlanID)
	}
	if strings.TrimSpace(created.CaseID) == "" {
		return withApprovedExecutionPlanID(runner.verifiedReport(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_support_create_case_response_incomplete", "AWS Support case creation returned an incomplete response after the create request.", true, blockedStep(), failCheck("aws_support_create_case_response_incomplete", "AWS Support case creation", "AWS Support case creation was attempted, but AWS did not return a case ID.")), approvedPlanID)
	}

	caseDetails := SupportCase{CaseID: created.CaseID, Status: "created"}
	statusLookupCheck := workflow.PlanCheck{}
	if strings.TrimSpace(created.CaseID) != "" {
		if details, err := client.DescribeCases(ctx, DescribeCasesRequest{
			CaseIDs:                  []string{created.CaseID},
			IncludeCommunications:    false,
			IncludeCommunicationsSet: true,
			MaxResults:               1,
		}); err == nil && len(details) > 0 {
			caseDetails = details[0]
			if caseDetails.CaseID == "" {
				caseDetails.CaseID = created.CaseID
			}
		} else if err != nil {
			statusLookupCheck = warnCheck("aws_support_case_status_lookup_unavailable", "AWS Support case status lookup", "AWS Support case was created, but display ID and status could not be retrieved automatically.", supportCaseEvidence(caseDetails, context.Period)...)
		}
	}

	checks := []workflow.PlanCheck{passCheck("aws_backfill_support_case_created", "AWS Support case creation", "AWS Support case was created without exposing case body details.", supportCaseEvidence(caseDetails, context.Period)...)}
	if statusLookupCheck.ID != "" {
		checks = append(checks, statusLookupCheck)
	}
	return withApprovedExecutionPlanID(runner.verifiedReport(request, workflow.RunStatusManualSteps, workflow.SupportSupported, "aws_backfill_support_case_created", "AWS Support case was created; rerun preflight after AWS completes the backfill.", true, supportCaseCreatedStep(), checks...), approvedPlanID)
}

func (runner Runner) clientFor(options workflow.ExecutionOptions) Client {
	if !isNilClient(runner.client) {
		return runner.client
	}
	if runner.clientFactory == nil {
		return nil
	}
	return runner.clientFactory(options)
}

func isNilClient(client Client) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (runner Runner) preflightNotBackfillReport(request workflow.Request, preflight workflow.CapabilityReport) workflow.CapabilityReport {
	switch {
	case preflight.Code == "aws_cur2_export_not_found":
		return runner.preflightReport(request, preflight, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_cur2_creation_required", "No AWS CUR 2.0 export exists yet; CUR 2.0 export creation is required before previous-month backfill can be requested.", false, guideStep(), warnCheck("aws_cur2_creation_required", "AWS CUR 2.0 export discovery", "No AWS CUR 2.0 export exists yet."))
	case preflight.Code == "aws_data_exports_incomplete_export_summary":
		return runner.preflightReport(request, preflight, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_data_exports_incomplete_export_summary", "AWS Data Exports returned incomplete export metadata; no backfill action was taken.", false, blockedStep(), failCheck("aws_data_exports_incomplete_export_summary", "AWS Data Exports export metadata", "AWS Data Exports returned an export summary without an export ARN."))
	case preflight.Code == "aws_cur2_export_ambiguous" || preflight.Code == "aws_cur2_export_selection_required":
		return runner.preflightReport(request, preflight, workflow.RunStatusBlocked, workflow.SupportBlocked, preflight.Code, "Select one AWS CUR 2.0 export with --export-ref before requesting previous-month backfill.", false, blockedStep(), failCheck(preflight.Code, "AWS CUR 2.0 export selection", "Multiple AWS CUR 2.0 exports were found. Rerun apply-prereqs --request-backfill with the selected --export-ref from preflight output.", preflightSelectionEvidence(preflight)...))
	case preflight.Status == workflow.RunStatusReady:
		return runner.preflightReport(request, preflight, workflow.RunStatusReady, workflow.SupportSupported, "aws_backfill_not_required", "AWS CUR 2.0 previous-month billing data does not require backfill.", false, reuseStep(), passCheck("aws_backfill_not_required", "AWS previous-month billing data", "AWS CUR 2.0 previous-month billing data does not require backfill."))
	default:
		code := preflight.Code
		if strings.TrimSpace(code) == "" {
			code = "aws_backfill_preflight_not_ready"
		}
		return runner.preflightReport(request, preflight, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_backfill_preflight_not_ready", "AWS Support backfill was not requested because CUR 2.0 preflight is not in a backfill-ready state.", false, blockedStep(), failCheck(code, "AWS CUR 2.0 preflight", "AWS CUR 2.0 preflight must reach previous-month backfill state before a support case can be requested."))
	}
}

func preflightSelectionEvidence(preflight workflow.CapabilityReport) []workflow.PlanEvidence {
	if preflight.PlanInput == nil {
		return nil
	}
	evidence := []workflow.PlanEvidence{}
	for _, check := range preflight.PlanInput.Checks {
		for _, item := range check.Evidence {
			if strings.HasPrefix(item.Key, "candidate_") || item.Key == "selected_export_ref" {
				evidence = append(evidence, item)
			}
		}
	}
	return evidence
}

func providerErrorCode(err error, fallback string) string {
	var providerErr ProviderError
	if errors.As(err, &providerErr) && providerErr.Code != "" {
		return providerErr.Code
	}
	var preflightErr cur2preflight.ProviderError
	if errors.As(err, &preflightErr) && preflightErr.Code != "" {
		return preflightErr.Code
	}
	return fallback
}

func manualFallbackProviderError(code string) bool {
	switch code {
	case "aws_support_subscription_required", "aws_support_access_denied", "aws_support_api_unavailable":
		return true
	default:
		return false
	}
}

func createCaseFailureMessage(mutated bool) string {
	if mutated {
		return "AWS Support case creation returned an incomplete response after the create request."
	}
	return "AWS Support case could not be created."
}

func createCaseFailureCheckMessage(mutated bool) string {
	if mutated {
		return "AWS Support case creation was attempted, but AWS did not return a case ID."
	}
	return "AWS Support case could not be created."
}

var _ workflow.CapabilityRunner = Runner{}

type approvalState string

const (
	approvalMissing  approvalState = "missing"
	approvalReady    approvalState = "ready"
	approvalStale    approvalState = "stale"
	approvalMismatch approvalState = "mismatch"
)

func backfillApprovalState(options workflow.ExecutionOptions, planID string, steps []workflow.PlanStep) approvalState {
	expected := map[string]int{}
	for _, step := range steps {
		if step.RequiresApproval {
			expected[step.ID] = 1
		}
	}
	if len(options.Approvals) == 0 {
		return approvalMissing
	}
	actual := map[string]int{}
	for _, approval := range options.Approvals {
		if approval.PlanID != planID {
			return approvalStale
		}
		if !approval.Confirmed || approval.Intent != workflow.ApprovalIntentRequestBackfillSupportCase {
			return approvalMismatch
		}
		actual[approval.OperationID]++
	}
	if len(actual) != len(expected) {
		return approvalMismatch
	}
	for id, count := range expected {
		if actual[id] != count {
			return approvalMismatch
		}
	}
	return approvalReady
}

func backfillPlanPreviewOptions(options workflow.ExecutionOptions) workflow.ExecutionOptions {
	options.Approvals = nil
	return options
}
