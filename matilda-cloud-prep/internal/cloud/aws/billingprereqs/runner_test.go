package billingprereqs

import (
	"context"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestRunnerRoutesDefaultToNonMutatingSetupRecommendation(t *testing.T) {
	backfill := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_backfill_support_case_approval_required")}
	setup := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_cur2_create_export_plan_ready")}
	runner := NewRunner(RunnerConfig{
		BackfillRunner: backfill,
		SetupRunner:    setup,
	})

	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), workflow.DefaultExecutionOptions())

	if result.Code != "aws_billing_prereqs_operation_required" {
		t.Fatalf("Code = %q, want operation-required guide", result.Code)
	}
	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if backfill.calls != 0 {
		t.Fatalf("backfill calls = %d, want 0", backfill.calls)
	}
	if setup.calls != 0 {
		t.Fatalf("setup calls = %d, want 0", setup.calls)
	}
	if result.PlanInput == nil || len(result.PlanInput.Steps) == 0 {
		t.Fatalf("PlanInput missing guide step: %#v", result.PlanInput)
	}
	if result.PlanInput.Steps[0].Intent != workflow.PlanStepGuide {
		t.Fatalf("guide step intent = %q, want %q", result.PlanInput.Steps[0].Intent, workflow.PlanStepGuide)
	}
}

func TestRunnerDefaultGuideDoesNotRequireOperationRunners(t *testing.T) {
	result := NewRunner(RunnerConfig{}).Run(context.Background(), AWSBillingApplyPrereqsRequest(), workflow.DefaultExecutionOptions())

	if result.Code != "aws_billing_prereqs_operation_required" {
		t.Fatalf("Code = %q, want operation-required guide", result.Code)
	}
	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if result.PlanInput == nil || len(result.PlanInput.Checks) == 0 {
		t.Fatalf("PlanInput missing guide check: %#v", result.PlanInput)
	}
}

func TestRunnerDefaultGuideDoesNotRequireTypedNilOperationRunners(t *testing.T) {
	var backfill *fakeRunner
	var setup *fakeRunner
	result := NewRunner(RunnerConfig{
		BackfillRunner: backfill,
		SetupRunner:    setup,
	}).Run(context.Background(), AWSBillingApplyPrereqsRequest(), workflow.DefaultExecutionOptions())

	if result.Code != "aws_billing_prereqs_operation_required" {
		t.Fatalf("Code = %q, want operation-required guide", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
}

func TestRunnerRoutesRequestBackfillOperationToBackfillFlow(t *testing.T) {
	backfill := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_backfill_support_case_created")}
	setup := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_cur2_create_export_plan_ready")}
	runner := NewRunner(RunnerConfig{
		BackfillRunner: backfill,
		SetupRunner:    setup,
	})
	options := workflow.DefaultExecutionOptions()
	options.AWSBillingOperation = workflow.AWSBillingOperationRequestBackfill

	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)

	if result.Code != "aws_backfill_support_case_created" {
		t.Fatalf("Code = %q, want backfill operation report", result.Code)
	}
	if backfill.calls != 1 {
		t.Fatalf("backfill calls = %d, want 1", backfill.calls)
	}
	if setup.calls != 0 {
		t.Fatalf("setup calls = %d, want 0", setup.calls)
	}
	if backfill.options.AWSBillingOperation != workflow.AWSBillingOperationRequestBackfill {
		t.Fatalf("backfill options operation = %q, want request_backfill", backfill.options.AWSBillingOperation)
	}
}

func TestRunnerRoutesCreateCUR2ExportOperationToSetupFlow(t *testing.T) {
	backfill := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_backfill_support_case_created")}
	setup := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_cur2_create_export_plan_ready")}
	runner := NewRunner(RunnerConfig{
		BackfillRunner: backfill,
		SetupRunner:    setup,
	})
	options := workflow.DefaultExecutionOptions()
	options.AWSBillingOperation = workflow.AWSBillingOperationCreateCUR2Export

	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)

	if result.Code != "aws_cur2_create_export_plan_ready" {
		t.Fatalf("Code = %q, want setup operation report", result.Code)
	}
	if backfill.calls != 0 {
		t.Fatalf("backfill calls = %d, want 0", backfill.calls)
	}
	if setup.calls != 1 {
		t.Fatalf("setup calls = %d, want 1", setup.calls)
	}
	if setup.options.AWSBillingOperation != workflow.AWSBillingOperationCreateCUR2Export {
		t.Fatalf("setup options operation = %q, want create_cur2_export", setup.options.AWSBillingOperation)
	}
}

func TestRunnerRoutesToValueTypedOperationRunner(t *testing.T) {
	runner := NewRunner(RunnerConfig{
		BackfillRunner: valueRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_value_runner_called")},
	})
	options := workflow.DefaultExecutionOptions()
	options.AWSBillingOperation = workflow.AWSBillingOperationRequestBackfill

	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)

	if result.Code != "aws_value_runner_called" {
		t.Fatalf("Code = %q, want value-typed operation runner report", result.Code)
	}
}

func TestRunnerBlocksConflictBeforeProviderCalls(t *testing.T) {
	backfill := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_backfill_support_case_created")}
	setup := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_cur2_create_export_plan_ready")}
	runner := NewRunner(RunnerConfig{
		BackfillRunner: backfill,
		SetupRunner:    setup,
	})
	options := workflow.DefaultExecutionOptions()
	options.AWSBillingOperation = workflow.AWSBillingOperationConflict

	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_billing_prereqs_operation_conflict" {
		t.Fatalf("Code = %q, want aws_billing_prereqs_operation_conflict", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if backfill.calls != 0 || setup.calls != 0 {
		t.Fatalf("provider calls = backfill %d setup %d, want none", backfill.calls, setup.calls)
	}
}

func TestRunnerFailsClosedWhenSelectedOperationRunnerIsMissing(t *testing.T) {
	tests := []struct {
		name      string
		config    RunnerConfig
		operation workflow.AWSBillingOperation
		wantCode  string
	}{
		{
			name:      "backfill runner missing for explicit backfill",
			config:    RunnerConfig{},
			operation: workflow.AWSBillingOperationRequestBackfill,
			wantCode:  "aws_billing_prereqs_backfill_runner_unavailable",
		},
		{
			name:      "setup runner missing",
			config:    RunnerConfig{},
			operation: workflow.AWSBillingOperationCreateCUR2Export,
			wantCode:  "aws_billing_prereqs_setup_runner_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := workflow.DefaultExecutionOptions()
			options.AWSBillingOperation = tt.operation

			result := NewRunner(tt.config).Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)

			if result.Status != workflow.RunStatusBlocked {
				t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
			}
			if result.SupportStatus != workflow.SupportBlocked {
				t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportBlocked)
			}
			if result.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", result.Code, tt.wantCode)
			}
			if result.Mutated {
				t.Fatal("Mutated = true, want false")
			}
			if result.PlanInput == nil || len(result.PlanInput.Steps) == 0 {
				t.Fatalf("PlanInput missing blocked step: %#v", result.PlanInput)
			}
		})
	}
}

func TestRunnerFailsClosedForTypedNilSelectedRunner(t *testing.T) {
	var backfill *fakeRunner
	var setup *fakeRunner
	tests := []struct {
		name      string
		config    RunnerConfig
		operation workflow.AWSBillingOperation
		wantCode  string
	}{
		{
			name: "typed nil backfill runner for explicit backfill",
			config: RunnerConfig{
				BackfillRunner: backfill,
				SetupRunner:    &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "unused")},
			},
			operation: workflow.AWSBillingOperationRequestBackfill,
			wantCode:  "aws_billing_prereqs_backfill_runner_unavailable",
		},
		{
			name: "typed nil setup runner",
			config: RunnerConfig{
				BackfillRunner: &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "unused")},
				SetupRunner:    setup,
			},
			operation: workflow.AWSBillingOperationCreateCUR2Export,
			wantCode:  "aws_billing_prereqs_setup_runner_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := workflow.DefaultExecutionOptions()
			options.AWSBillingOperation = tt.operation

			result := NewRunner(tt.config).Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)

			if result.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", result.Code, tt.wantCode)
			}
			if result.Mutated {
				t.Fatal("Mutated = true, want false")
			}
		})
	}
}

func TestRunnerBlocksUnsupportedOperation(t *testing.T) {
	backfill := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_backfill_support_case_created")}
	setup := &fakeRunner{report: prereqsReport(AWSBillingApplyPrereqsRequest(), "aws_cur2_create_export_plan_ready")}
	runner := NewRunner(RunnerConfig{
		BackfillRunner: backfill,
		SetupRunner:    setup,
	})
	options := workflow.DefaultExecutionOptions()
	options.AWSBillingOperation = workflow.AWSBillingOperation("future_operation")

	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)

	if result.Code != "aws_billing_prereqs_operation_unsupported" {
		t.Fatalf("Code = %q, want aws_billing_prereqs_operation_unsupported", result.Code)
	}
	if backfill.calls != 0 || setup.calls != 0 {
		t.Fatalf("provider calls = backfill %d setup %d, want none", backfill.calls, setup.calls)
	}
}

type fakeRunner struct {
	report  workflow.CapabilityReport
	calls   int
	options workflow.ExecutionOptions
}

func (runner *fakeRunner) Run(ctx context.Context, request workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
	runner.calls++
	runner.options = options
	return runner.report
}

type valueRunner struct {
	report workflow.CapabilityReport
}

func (runner valueRunner) Run(context.Context, workflow.Request, workflow.ExecutionOptions) workflow.CapabilityReport {
	return runner.report
}

func prereqsReport(request workflow.Request, code string) workflow.CapabilityReport {
	handles := []workflow.SourceHandle{{
		Label: "AWS CUR 2.0 Create-New Export",
		URI:   "docs/references/aws/aws-cur2-create-new-export.md",
	}}
	return workflow.CapabilityReport{
		Status:        workflow.RunStatusManualSteps,
		SupportStatus: workflow.SupportGuided,
		Code:          code,
		Message:       "AWS billing prerequisites operation report.",
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified before AWS billing apply-prereqs.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageUnverified,
				Summary:        "AWS billing coverage was not classified by this fake runner.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{{
				Intent:                    workflow.PlanStepGuide,
				Title:                     "AWS billing prerequisites fake step",
				Description:               "Fake step for AWS billing prerequisites orchestration tests.",
				Reason:                    "The orchestrator test only verifies routing.",
				ApprovalKind:              "not_required",
				CurrentState:              "Fake runner selected.",
				TargetState:               "Selected operation runner reports its result.",
				RequiredPermission:        "No provider permissions required in fake runner.",
				CredentialMaterialTouched: false,
				Validation:                "The fake runner was called once.",
				Rollback:                  "No cloud change is made by the fake runner.",
				SourceHandles:             handles,
			}},
			Checks: []workflow.PlanCheck{{
				ID:            code,
				Status:        workflow.CheckWarn,
				Title:         "AWS billing prerequisites fake check",
				Message:       "Fake check for AWS billing prerequisites orchestration tests.",
				Evidence:      []workflow.PlanEvidence{{Key: "code", Value: code}},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}

func TestAWSBillingApplyPrereqsRequest(t *testing.T) {
	request := AWSBillingApplyPrereqsRequest()

	if request.Goal != assessment.RapidAssessment ||
		request.CollectionPath != assessment.CollectionBilling ||
		request.Provider != assessment.ProviderAWS ||
		request.Action != assessment.ActionApplyPrereqs {
		t.Fatalf("request = %#v, want AWS rapid-assessment billing apply-prereqs", request)
	}
}
