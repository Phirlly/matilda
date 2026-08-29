package billingcur2setup

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

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
	if !isAWSBillingApplyPrereqsRequest(request) {
		return runner.blocked(request, "aws_billing_apply_prereqs_request_required", "AWS CUR 2.0 setup runner requires the AWS Rapid Assessment - Billing Based apply-prereqs request.")
	}
	if options.AWSBillingOperation != workflow.AWSBillingOperationCreateCUR2Export {
		return runner.blocked(request, "aws_cur2_create_export_operation_required", "AWS CUR 2.0 create-export setup runner requires the create-cur2-export operation intent.")
	}

	client := runner.clientFor(options)
	if isNilClient(client) {
		return runner.blocked(request, "aws_provider_capability_blocked", "AWS CUR 2.0 setup client is not configured.")
	}

	plan, err := runner.buildPlan(ctx, client, options)
	if err != nil {
		code := providerErrorCode(err, "aws_cur2_create_export_plan_failed")
		if plan.identityVerified() {
			return runner.blockedAfterIdentity(request, code, "AWS CUR 2.0 create-export setup plan could not be built safely.", plan)
		}
		return runner.blocked(request, code, "AWS CUR 2.0 create-export setup plan could not be built safely.")
	}
	steps := planSteps(plan)
	input := setupPlanInput(request, plan, steps, []workflow.PlanCheck{planFactsCheck(plan)})
	input.ExecutionOptions = options

	if plan.ManagedExport != nil {
		setPlanEvidenceValue(&input, "selected_export_ref", cur2preflight.SafeCUR2ExportRef(plan.ManagedExport.ExportARN))
	}
	if plan.ManagedExport != nil && !plan.PolicyNeedsMerge {
		setPlanEvidenceValue(&input, "selected_export_ref", cur2preflight.SafeCUR2ExportRef(plan.ManagedExport.ExportARN))
		if len(options.Approvals) > 0 {
			previewInput := input
			previewInput.ExecutionOptions = createPlanPreviewOptions(options)
			preview, err := workflow.BuildExecutionPlan(previewInput)
			if err != nil {
				return runner.blocked(request, "aws_cur2_create_export_plan_failed", "AWS CUR 2.0 create-export setup plan could not be built safely.")
			}
			switch approval := createApprovalState(options, preview.PlanID, preview.Steps); approval {
			case approvalStale:
				return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_stale", "Approved AWS CUR 2.0 setup plan does not match the current plan. Review the new plan before applying.", false, input)
			case approvalMismatch, approvalReady:
				return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_approval_mismatch", "Approved AWS CUR 2.0 setup steps do not match the current mutating step set.", false, input)
			case approvalMissing:
			default:
				return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_approval_mismatch", "Approved AWS CUR 2.0 setup steps do not match the current mutating step set.", false, input)
			}
		}
		return reportWithPlan(request, workflow.RunStatusReady, workflow.SupportSupported, "aws_cur2_create_export_reused", "An existing Matilda-generated AWS CUR 2.0 export matches the setup contract.", false, input)
	}
	if blockedCode := blockedPlanCode(plan); blockedCode != "" {
		return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, blockedCode, "AWS CUR 2.0 create-export setup is blocked before mutation.", false, input)
	}

	previewInput := input
	previewInput.ExecutionOptions = createPlanPreviewOptions(options)
	preview, err := workflow.BuildExecutionPlan(previewInput)
	if err != nil {
		return runner.blocked(request, "aws_cur2_create_export_plan_failed", "AWS CUR 2.0 create-export setup plan could not be built safely.")
	}
	switch approval := createApprovalState(options, preview.PlanID, preview.Steps); approval {
	case approvalMissing:
		return reportWithPlan(request, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_cur2_create_export_approval_required", "Review the AWS CUR 2.0 setup plan and approve each mutating step before cloud changes are made.", false, input)
	case approvalStale:
		return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_stale", "Approved AWS CUR 2.0 setup plan does not match the current plan. Review the new plan before applying.", false, input)
	case approvalMismatch:
		return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_approval_mismatch", "Approved AWS CUR 2.0 setup steps do not match the current mutating step set.", false, input)
	case approvalReady:
		input.ApprovedExecutionPlanID = preview.PlanID
		return runner.apply(ctx, request, client, plan, input)
	default:
		return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_approval_mismatch", "Approved AWS CUR 2.0 setup steps do not match the current mutating step set.", false, input)
	}
}

func (runner Runner) buildPlan(ctx context.Context, client Client, options workflow.ExecutionOptions) (setupPlan, error) {
	config, err := client.CheckConfiguration(ctx)
	if err != nil {
		return setupPlan{}, err
	}
	region := selectedRegion(options, config.Region)
	if region == "" {
		return setupPlan{}, NewProviderError("aws_config_missing_region", "AWS region is required for S3 bucket placement.")
	}
	identity, err := client.GetCallerIdentity(ctx)
	if err != nil {
		return setupPlan{}, err
	}
	identityContext := identityContext{
		AccountID: strings.TrimSpace(identity.AccountID),
		Partition: partitionFromARN(identity.CallerARN),
	}
	if identityContext.AccountID == "" {
		return setupPlan{}, NewProviderError("aws_auth_failed", "AWS caller account could not be resolved.")
	}
	if identityContext.Partition == "" {
		return setupPlan{}, NewProviderError("aws_auth_partition_unresolved", "AWS caller partition could not be resolved from the caller ARN.")
	}
	coverage := classifyCoverage(ctx, client, identityContext.AccountID)

	candidates := generatedNameCandidates(identityContext, region)
	exports, err := listFullExports(ctx, client)
	if err != nil {
		return setupPlan{
			Facts:    candidates[0],
			Identity: identityContext,
			Region:   region,
			Coverage: coverage,
		}, err
	}

	cur2Count := 0
	for _, export := range exports {
		if cur2preflight.IsCUR2ExportCandidate(export) {
			cur2Count++
		}
	}

	for _, facts := range candidates {
		plan := setupPlan{
			Facts:    facts,
			Identity: identityContext,
			Region:   region,
			Coverage: coverage,
		}
		for _, export := range exports {
			if isManagedExport(export, plan) {
				exportCopy := export
				plan.ManagedExport = &exportCopy
				return verifyManagedExportReuse(ctx, client, plan)
			}
		}
	}
	if cur2Count >= 5 {
		plan := setupPlan{
			Facts:    candidates[0],
			Identity: identityContext,
			Region:   region,
			Coverage: coverage,
		}
		plan.Steps = []plannedStep{{ID: "aws_cur2_export_quota_full", Intent: "blocked"}}
		return plan, nil
	}

	for _, facts := range candidates {
		plan := setupPlan{
			Facts:    facts,
			Identity: identityContext,
			Region:   region,
			Coverage: coverage,
		}
		access, err := client.HeadBucket(ctx, HeadBucketRequest{
			Bucket:        facts.BucketName,
			Region:        region,
			ExpectedOwner: identityContext.AccountID,
		})
		if err != nil {
			code := providerErrorCode(err, "aws_s3_bucket_inaccessible")
			switch code {
			case "aws_s3_bucket_owner_mismatch":
				continue
			case "aws_s3_bucket_not_found":
				plan.BucketExists = false
				plan.PolicyNeedsMerge = true
				return plan, nil
			default:
				return plan, err
			}
		}
		if !access.Accessible {
			if access.StatusCode == 409 {
				continue
			}
			return plan, NewProviderError("aws_s3_bucket_inaccessible", "AWS S3 bucket candidate access could not be verified safely.")
		}
		if access.Region != region {
			continue
		}
		plan.BucketExists = true
		policy, err := client.GetBucketPolicy(ctx, BucketPolicyRequest{
			Bucket:        facts.BucketName,
			ExpectedOwner: identityContext.AccountID,
		})
		if err != nil {
			return plan, err
		}
		merged, changed, err := mergeDataExportsPolicy(policy, plan)
		if err != nil {
			return plan, NewProviderError("aws_s3_bucket_policy_unmergeable", "S3 bucket policy cannot be merged safely.")
		}
		plan.PolicyNeedsMerge = changed
		plan.Policy = merged
		return plan, nil
	}
	return setupPlan{
		Facts:    candidates[0],
		Identity: identityContext,
		Region:   region,
		Coverage: coverage,
	}, NewProviderError("aws_cur2_bucket_name_candidates_exhausted", "No generated bucket name candidate is safely available.")
}

func verifyManagedExportReuse(ctx context.Context, client Client, plan setupPlan) (setupPlan, error) {
	if plan.ManagedExport == nil {
		return setupPlan{}, NewProviderError("aws_cur2_managed_export_validation_failed", "Managed AWS CUR 2.0 export is unavailable.")
	}
	if err := validateManagedExportARN(plan.ManagedExport.ExportARN, plan); err != nil {
		return plan, err
	}
	access, err := client.HeadBucket(ctx, HeadBucketRequest{
		Bucket:        plan.Facts.BucketName,
		Region:        plan.Region,
		ExpectedOwner: plan.Identity.AccountID,
	})
	if err != nil {
		return plan, err
	}
	if err := validateBucketAccessForPlan(access, plan); err != nil {
		return plan, err
	}
	policy, err := client.GetBucketPolicy(ctx, BucketPolicyRequest{
		Bucket:        plan.Facts.BucketName,
		ExpectedOwner: plan.Identity.AccountID,
	})
	if err != nil {
		return plan, err
	}
	merged, changed, err := mergeDataExportsPolicy(policy, plan)
	if err != nil {
		return plan, NewProviderError("aws_s3_bucket_policy_unmergeable", "Managed AWS CUR 2.0 export bucket policy cannot be parsed safely.")
	}
	if changed {
		plan.BucketExists = true
		plan.PolicyNeedsMerge = true
		plan.Policy = merged
		return plan, nil
	}
	plan.BucketExists = true
	plan.PolicyNeedsMerge = false
	plan.Policy = merged
	return plan, nil
}

func (runner Runner) apply(ctx context.Context, request workflow.Request, client Client, plan setupPlan, input workflow.ExecutionPlanInput) workflow.CapabilityReport {
	mutated := false
	if !plan.BucketExists {
		if err := client.CreateBucket(ctx, CreateBucketRequest{Bucket: plan.Facts.BucketName, Region: plan.Region}); err != nil {
			code := providerErrorCode(err, "aws_s3_create_bucket_failed")
			switch code {
			case "aws_s3_bucket_already_owned_by_caller":
				plan.BucketExists = true
			case "aws_s3_bucket_already_exists":
				return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_plan_stale", "Approved AWS CUR 2.0 setup bucket candidate is no longer available. Review the new plan before applying.", false, input)
			default:
				return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, code, "AWS S3 bucket could not be created.", false, input)
			}
		} else {
			mutated = true
			plan.BucketExists = true
		}
		access, err := client.HeadBucket(ctx, HeadBucketRequest{Bucket: plan.Facts.BucketName, Region: plan.Region, ExpectedOwner: plan.Identity.AccountID})
		if err != nil {
			return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_s3_bucket_owner_mismatch"), "AWS S3 bucket ownership could not be verified after creation.", mutated, input)
		}
		if err := validateBucketAccessForPlan(access, plan); err != nil {
			return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_s3_bucket_validation_failed"), "AWS S3 bucket ownership and region could not be verified after creation.", mutated, input)
		}
	}
	if plan.PolicyNeedsMerge {
		policy, err := client.GetBucketPolicy(ctx, BucketPolicyRequest{Bucket: plan.Facts.BucketName, ExpectedOwner: plan.Identity.AccountID})
		if err != nil {
			return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_s3_bucket_policy_inaccessible"), "AWS S3 bucket policy could not be inspected before merge.", mutated, input)
		}
		merged, changed, err := mergeDataExportsPolicy(policy, plan)
		if err != nil {
			return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, "aws_s3_bucket_policy_unmergeable", "AWS S3 bucket policy could not be merged safely.", mutated, input)
		}
		if changed {
			if err := client.PutBucketPolicy(ctx, PutBucketPolicyRequest{Bucket: plan.Facts.BucketName, ExpectedOwner: plan.Identity.AccountID, Policy: merged}); err != nil {
				return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_s3_put_bucket_policy_failed"), "AWS S3 bucket policy could not be updated.", mutated, input)
			}
			mutated = true
		}
	}
	if plan.ManagedExport != nil {
		if err := runner.validateManagedExport(ctx, client, plan); err != nil {
			return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_cur2_managed_export_repair_validation_failed"), "AWS CUR 2.0 managed export policy was updated but could not be validated against the approved plan.", mutated, input)
		}
		setPlanEvidenceValue(&input, "selected_export_ref", cur2preflight.SafeCUR2ExportRef(plan.ManagedExport.ExportARN))
		return reportWithPlan(request, workflow.RunStatusManualSteps, workflow.SupportSupported, "aws_cur2_create_export_repaired", "AWS CUR 2.0 managed export delivery policy was repaired. Initial delivery and previous-month data availability can still require follow-up validation.", mutated, input)
	}
	createResult, err := client.CreateExport(ctx, buildCreateExportRequest(plan))
	if err != nil {
		return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_cur2_create_export_failed"), "AWS CUR 2.0 export could not be created.", mutated, input)
	}
	mutated = true
	if err := runner.validateCreatedExport(ctx, client, plan, createResult); err != nil {
		return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, providerErrorCode(err, "aws_cur2_create_export_validation_failed"), "AWS CUR 2.0 export was created but could not be validated against the approved plan.", mutated, input)
	}
	setPlanEvidenceValue(&input, "selected_export_ref", cur2preflight.SafeCUR2ExportRef(createResult.ExportARN))
	return reportWithPlan(request, workflow.RunStatusManualSteps, workflow.SupportSupported, "aws_cur2_create_export_created", "AWS CUR 2.0 export was created. Initial delivery and previous-month data availability can still require follow-up validation.", true, input)
}

func (runner Runner) validateManagedExport(ctx context.Context, client Client, plan setupPlan) error {
	if plan.ManagedExport == nil || strings.TrimSpace(plan.ManagedExport.ExportARN) == "" {
		return NewProviderError("aws_cur2_managed_export_repair_validation_failed", "Managed AWS CUR 2.0 export ARN is unavailable.")
	}
	if err := validateManagedExportARN(plan.ManagedExport.ExportARN, plan); err != nil {
		return err
	}
	export, err := client.GetExport(ctx, plan.ManagedExport.ExportARN)
	if err != nil {
		return err
	}
	if !isManagedExport(export, plan) {
		return NewProviderError("aws_cur2_managed_export_repair_validation_failed", "Managed AWS CUR 2.0 export settings do not match the approved plan after repair.")
	}
	access, err := client.HeadBucket(ctx, HeadBucketRequest{
		Bucket:        plan.Facts.BucketName,
		Region:        plan.Region,
		ExpectedOwner: plan.Identity.AccountID,
	})
	if err != nil {
		return err
	}
	if err := validateBucketAccessForPlan(access, plan); err != nil {
		return err
	}
	policy, err := client.GetBucketPolicy(ctx, BucketPolicyRequest{
		Bucket:        plan.Facts.BucketName,
		ExpectedOwner: plan.Identity.AccountID,
	})
	if err != nil {
		return err
	}
	if _, changed, err := mergeDataExportsPolicy(policy, plan); err != nil {
		return NewProviderError("aws_s3_bucket_policy_unmergeable", "AWS S3 bucket policy could not be parsed during managed export repair validation.")
	} else if changed {
		return NewProviderError("aws_s3_delivery_policy_validation_failed", "AWS S3 bucket policy did not contain the approved Data Exports delivery statement after repair.")
	}
	return nil
}

func (runner Runner) validateCreatedExport(ctx context.Context, client Client, plan setupPlan, createResult CreateExportResult) error {
	if strings.TrimSpace(createResult.ExportARN) == "" {
		return NewProviderError("aws_cur2_create_export_validation_failed", "AWS Data Exports did not return a created export ARN.")
	}
	if err := validateReturnedExportARN(createResult.ExportARN, plan); err != nil {
		return err
	}
	export, err := client.GetExport(ctx, createResult.ExportARN)
	if err != nil {
		return err
	}
	if !isManagedExport(export, plan) {
		return NewProviderError("aws_cur2_create_export_validation_failed", "Created AWS CUR 2.0 export settings do not match the approved plan.")
	}
	access, err := client.HeadBucket(ctx, HeadBucketRequest{
		Bucket:        plan.Facts.BucketName,
		Region:        plan.Region,
		ExpectedOwner: plan.Identity.AccountID,
	})
	if err != nil {
		return err
	}
	if err := validateBucketAccessForPlan(access, plan); err != nil {
		return err
	}
	policy, err := client.GetBucketPolicy(ctx, BucketPolicyRequest{
		Bucket:        plan.Facts.BucketName,
		ExpectedOwner: plan.Identity.AccountID,
	})
	if err != nil {
		return err
	}
	if _, changed, err := mergeDataExportsPolicy(policy, plan); err != nil {
		return NewProviderError("aws_s3_bucket_policy_unmergeable", "AWS S3 bucket policy could not be parsed during post-create validation.")
	} else if changed {
		return NewProviderError("aws_s3_delivery_policy_validation_failed", "AWS S3 bucket policy did not contain the approved Data Exports delivery statement after setup.")
	}
	return nil
}

func validateBucketAccessForPlan(access cur2preflight.BucketAccess, plan setupPlan) error {
	if !access.Accessible || access.Region != plan.Region {
		return NewProviderError("aws_s3_bucket_validation_failed", "AWS S3 bucket was not reachable in the approved region.")
	}
	return nil
}

func listFullExports(ctx context.Context, client Client) ([]cur2preflight.Export, error) {
	exports := []cur2preflight.Export{}
	token := ""
	for page := 0; page < 10; page++ {
		pageResult, err := client.ListExports(ctx, token)
		if err != nil {
			return nil, err
		}
		for _, summary := range pageResult.Exports {
			if strings.TrimSpace(summary.ExportARN) == "" {
				return nil, NewProviderError("aws_data_exports_incomplete_export_summary", "AWS Data Exports returned an export summary without an export ARN.")
			}
			export, err := client.GetExport(ctx, summary.ExportARN)
			if err != nil {
				return nil, err
			}
			exports = append(exports, export)
		}
		if pageResult.NextToken == "" {
			return exports, nil
		}
		token = pageResult.NextToken
	}
	return nil, NewProviderError("aws_data_exports_pagination_unbounded", "AWS Data Exports pagination did not converge.")
}

func selectedRegion(options workflow.ExecutionOptions, configured string) string {
	if options.Selectors != nil && options.Selectors.AWS != nil && strings.TrimSpace(options.Selectors.AWS.Region) != "" {
		return strings.TrimSpace(options.Selectors.AWS.Region)
	}
	return strings.TrimSpace(configured)
}

type approvalState string

const (
	approvalMissing  approvalState = "missing"
	approvalReady    approvalState = "ready"
	approvalStale    approvalState = "stale"
	approvalMismatch approvalState = "mismatch"
)

func createApprovalState(options workflow.ExecutionOptions, planID string, steps []workflow.PlanStep) approvalState {
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
		if !approval.Confirmed || approval.Intent != "" {
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

func createPlanPreviewOptions(options workflow.ExecutionOptions) workflow.ExecutionOptions {
	options.Approvals = nil
	return options
}

func blockedPlanCode(plan setupPlan) string {
	for _, step := range plan.Steps {
		if step.Intent == "blocked" {
			return step.ID
		}
	}
	return ""
}

func isAWSBillingApplyPrereqsRequest(request workflow.Request) bool {
	return request.Goal == assessment.RapidAssessment &&
		request.CollectionPath == assessment.CollectionBilling &&
		request.Provider == assessment.ProviderAWS &&
		request.Action == assessment.ActionApplyPrereqs
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

func (plan setupPlan) identityVerified() bool {
	return strings.TrimSpace(plan.Identity.AccountID) != "" && strings.TrimSpace(plan.Identity.Partition) != ""
}

func (runner Runner) blockedAfterIdentity(request workflow.Request, code string, message string, plan setupPlan) workflow.CapabilityReport {
	step := blockedSetupStep(code, true)
	check := workflow.PlanCheck{
		ID:       code,
		Status:   workflow.CheckFail,
		Title:    "AWS CUR 2.0 setup blocker",
		Message:  message,
		Evidence: []workflow.PlanEvidence{{Key: "code", Value: code}},
	}
	input := setupPlanInput(request, plan, []workflow.PlanStep{step}, []workflow.PlanCheck{check})
	if strings.TrimSpace(input.CoverageRecommendation.Summary) == "" {
		input.CoverageRecommendation = workflow.CoverageRecommendation{
			CoverageStatus: workflow.CoverageUnknown,
			Summary:        "AWS billing coverage could not be classified because setup stopped before discovery completed.",
		}
	}
	return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, code, message, false, input)
}

func (runner Runner) blocked(request workflow.Request, code string, message string) workflow.CapabilityReport {
	handles := sourceHandles()
	input := workflow.ExecutionPlanInput{
		Request: request,
		OperatorIdentitySummary: workflow.OperatorIdentitySummary{
			IdentityStatus: "unknown",
			Summary:        "AWS CUR 2.0 setup could not verify caller identity before stopping.",
			SourceHandles:  handles,
		},
		CoverageRecommendation: workflow.CoverageRecommendation{
			CoverageStatus: workflow.CoverageUnknown,
			Summary:        "AWS billing coverage could not be classified because setup stopped before discovery completed.",
		},
		PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
		Steps:               []workflow.PlanStep{blockedSetupStep(code, false)},
		Checks: []workflow.PlanCheck{{
			ID:            code,
			Status:        workflow.CheckFail,
			Title:         "AWS CUR 2.0 setup blocker",
			Message:       message,
			Evidence:      []workflow.PlanEvidence{{Key: "code", Value: code}},
			SourceHandles: handles,
		}},
		SourceHandles: handles,
	}
	return reportWithPlan(request, workflow.RunStatusBlocked, workflow.SupportBlocked, code, message, false, input)
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

var _ workflow.CapabilityRunner = Runner{}
