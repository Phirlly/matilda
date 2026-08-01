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
		return runner.report(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_backfill_context_unavailable"), "AWS CUR 2.0 backfill context could not be resolved without unsafe assumptions.", false, blockedStep(), failCheck(providerErrorCode(err, "aws_backfill_context_unavailable"), "AWS CUR 2.0 backfill context", "AWS CUR 2.0 backfill context could not be resolved without unsafe assumptions."))
	}
	if !context.MissingDataPartition && !context.MissingManifest {
		return runner.report(request, workflow.RunStatusReady, workflow.SupportSupported, "aws_backfill_not_required", "Previous-month AWS CUR 2.0 billing data is already present.", false, reuseStep(), passCheck("aws_backfill_not_required", "AWS previous-month billing data", "Previous-month AWS CUR 2.0 billing data is already present.", workflow.PlanEvidence{Key: "previous_billing_period", Value: context.Period}))
	}

	if !workflow.HasAWSBackfillSupportCaseApproval(options) {
		return runner.report(request, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_backfill_support_case_approval_required", "AWS Support case creation requires explicit backfill approval.", false, approvalRequiredStep(), warnCheck("aws_backfill_support_case_approval_required", "AWS Support case approval", "Run apply-prereqs with explicit backfill support-case approval flags to request AWS backfill.", context.evidence()...))
	}

	classification, err := resolveSupportClassification(ctx, client)
	if err != nil {
		code := providerErrorCode(err, "aws_support_case_manual_fallback_required")
		if manualFallbackProviderError(code) {
			return runner.report(request, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_support_case_manual_fallback_required", "AWS Support API case creation is unavailable; use the manual AWS Support request path.", false, manualSupportCaseStep(), warnCheck("aws_support_case_manual_fallback_required", "AWS Support API availability", "AWS Support API case creation is unavailable for this account or support plan.", manualSupportRequestEvidence(context)...))
		}
		return runner.report(request, workflow.RunStatusManualSteps, workflow.SupportGuided, code, "AWS Support case could not be created automatically; use the manual AWS Support request path.", false, manualSupportCaseStep(), warnCheck(code, "AWS Support case manual fallback", "AWS Support service, category, severity, or create-case options could not be resolved safely enough for automation.", manualSupportRequestEvidence(context)...))
	}

	ref := backfillRequestReference(context.Export.ExportARN, context.Period)
	existing, ok, err := findExistingOpenCase(ctx, client, ref)
	if err != nil {
		return runner.report(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_backfill_duplicate_check_failed", "AWS Support duplicate-case check failed; no case was created.", false, blockedStep(), failCheck("aws_backfill_duplicate_check_failed", "AWS Support duplicate-case check", "Existing AWS Support cases could not be checked safely before mutation."))
	}
	if ok {
		return runner.report(request, workflow.RunStatusManualSteps, workflow.SupportSupported, "aws_backfill_support_case_already_open", "An existing matching AWS Support case is already open.", false, reuseStep(), passCheck("aws_backfill_support_case_already_open", "AWS Support duplicate-case check", "An existing matching AWS Support case is already open.", supportCaseEvidence(existing, context.Period)...))
	}

	caseRequest := buildCreateCaseRequest(classification, context, ref)
	created, err := client.CreateCase(ctx, caseRequest)
	if err != nil {
		return runner.report(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_support_create_case_failed"), "AWS Support case could not be created.", false, blockedStep(), failCheck(providerErrorCode(err, "aws_support_create_case_failed"), "AWS Support case creation", "AWS Support case could not be created."))
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
	return runner.report(request, workflow.RunStatusManualSteps, workflow.SupportSupported, "aws_backfill_support_case_created", "AWS Support case was created; rerun preflight after AWS completes the backfill.", true, supportCaseCreatedStep(), checks...)
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
		return runner.report(request, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_cur2_creation_required", "No AWS CUR 2.0 export exists yet; CUR 2.0 export creation is required before previous-month backfill can be requested.", false, guideStep(), warnCheck("aws_cur2_creation_required", "AWS CUR 2.0 export discovery", "No AWS CUR 2.0 export exists yet."))
	case preflight.Status == workflow.RunStatusReady:
		return runner.report(request, workflow.RunStatusReady, workflow.SupportSupported, "aws_backfill_not_required", "AWS CUR 2.0 previous-month billing data does not require backfill.", false, reuseStep(), passCheck("aws_backfill_not_required", "AWS previous-month billing data", "AWS CUR 2.0 previous-month billing data does not require backfill."))
	default:
		code := preflight.Code
		if strings.TrimSpace(code) == "" {
			code = "aws_backfill_preflight_not_ready"
		}
		return runner.report(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_backfill_preflight_not_ready", "AWS Support backfill was not requested because CUR 2.0 preflight is not in a backfill-ready state.", false, blockedStep(), failCheck(code, "AWS CUR 2.0 preflight", "AWS CUR 2.0 preflight must reach previous-month backfill state before a support case can be requested."))
	}
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

var _ workflow.CapabilityRunner = Runner{}
