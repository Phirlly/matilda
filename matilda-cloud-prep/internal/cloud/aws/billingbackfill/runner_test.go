package billingbackfill

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestRunnerRequiresApprovalBeforeCreatingSupportCase(t *testing.T) {
	client := baselineBackfillClient()
	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.Code != "aws_backfill_support_case_approval_required" {
		t.Fatalf("Code = %q, want aws_backfill_support_case_approval_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false without approval")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
	assertBackfillIdentityStatus(t, result, "verified", "AWS caller identity and CUR 2.0 export state were checked")
}

func TestRunnerBackfillPreviewExposesPlanBoundSupportCaseStep(t *testing.T) {
	client := baselineBackfillClient()
	result := runBackfill(t, client, requestBackfillOptions(client.export))
	plan := planFromBackfillReport(t, result, requestBackfillOptions(client.export))

	if len(plan.Steps) != 1 {
		t.Fatalf("Steps length = %d, want 1", len(plan.Steps))
	}
	step := plan.Steps[0]
	if step.ID != workflow.AWSBackfillSupportCaseOperationID {
		t.Fatalf("support-case step ID = %q, want %q", step.ID, workflow.AWSBackfillSupportCaseOperationID)
	}
	if !step.RequiresApproval || step.ApprovalKind != "cloud_mutation" {
		t.Fatalf("support-case step approval = requires %t kind %q, want cloud mutation approval", step.RequiresApproval, step.ApprovalKind)
	}
	if !plan.Approval.Required || plan.Approval.Blocked {
		t.Fatalf("plan approval summary = %#v, want required and not blocked", plan.Approval)
	}
	check, ok := reportCheck(result, "aws_backfill_support_case_approval_required")
	if !ok {
		t.Fatalf("approval check not found in %#v", result.PlanInput.Checks)
	}
	var binding string
	for _, evidence := range check.Evidence {
		if evidence.Key == "support_case_binding_ref" {
			binding = evidence.Value
		}
		for _, forbidden := range []string{
			client.export.Name,
			client.export.ExportARN,
			client.export.SourceAccount,
			client.export.Destination.Bucket,
			client.export.Destination.Prefix,
			supportCaseBody(backfillContext{Export: client.export, Period: "2026-06", MissingDataPartition: true, MissingManifest: true}, backfillRequestReference(client.export.ExportARN, "2026-06")),
		} {
			if strings.Contains(evidence.Value, forbidden) {
				t.Fatalf("approval evidence %s leaked raw support-case detail %q in %q", evidence.Key, forbidden, evidence.Value)
			}
		}
	}
	if binding == "" {
		t.Fatalf("approval evidence missing support_case_binding_ref: %#v", check.Evidence)
	}
}

func TestRunnerBackfillWorkflowResultsDoNotSerializeSupportCaseBodyFacts(t *testing.T) {
	tests := []struct {
		name   string
		result func(*testing.T, *fakeBackfillClient) workflow.Result
	}{
		{
			name: "preview",
			result: func(t *testing.T, client *fakeBackfillClient) workflow.Result {
				t.Helper()
				return runBackfillThroughRegistry(t, client, requestBackfillOptions(client.export))
			},
		},
		{
			name: "manual fallback",
			result: func(t *testing.T, client *fakeBackfillClient) workflow.Result {
				t.Helper()
				client.describeServicesErr = NewProviderError("aws_support_subscription_required", "support plan does not allow Support API case creation")
				return runBackfillThroughRegistry(t, client, requestBackfillOptions(client.export))
			},
		},
		{
			name: "approved create",
			result: func(t *testing.T, client *fakeBackfillClient) workflow.Result {
				t.Helper()
				return runBackfillThroughRegistry(t, client, approvedBackfillOptions(t, client))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineBackfillClient()
			result := tt.result(t, client)
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("json.Marshal returned error: %v", err)
			}
			body := supportCaseBody(backfillContext{
				Export:               client.export,
				ExportRef:            cur2preflight.SafeCUR2ExportRef(client.export.ExportARN),
				Period:               "2026-06",
				MissingDataPartition: true,
				MissingManifest:      true,
			}, backfillRequestReference(client.export.ExportARN, "2026-06"))
			text := string(encoded)
			for _, forbidden := range []string{
				client.export.Name,
				client.export.ExportARN,
				client.export.SourceAccount,
				client.export.Destination.Bucket,
				client.export.Destination.Prefix,
				body,
				"Please backfill AWS CUR 2.0 cost data",
				"Report name:",
				"S3 bucket:",
			} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s result serialized forbidden support-case detail %q in %s", tt.name, forbidden, text)
				}
			}
		})
	}
}

func TestRunnerBlocksStaleBackfillApprovalBeforeCreatingSupportCase(t *testing.T) {
	client := baselineBackfillClient()
	options := requestBackfillOptions(client.export)
	options.Approvals = []workflow.ExecutionApproval{{
		OperationID: workflow.AWSBackfillSupportCaseOperationID,
		Intent:      workflow.ApprovalIntentRequestBackfillSupportCase,
		PlanID:      "plan_ponmlkjihgfedcba",
		Confirmed:   true,
	}}

	result := runBackfill(t, client, options)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_plan_stale" {
		t.Fatalf("Code = %q, want aws_plan_stale", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for stale approval")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerBlocksBackfillApprovalWhenSupportCaseBodyFactsChange(t *testing.T) {
	client := baselineBackfillClient()
	options := approvedBackfillOptions(t, client)
	client.exports[0].Name = "matilda-cur2-renamed"
	client.export = client.exports[0]

	result := runBackfill(t, client, options)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_plan_stale" {
		t.Fatalf("Code = %q, want aws_plan_stale", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for stale support-case request approval")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerBlocksBackfillApprovalWhenSupportClassificationChanges(t *testing.T) {
	client := baselineBackfillClient()
	options := approvedBackfillOptions(t, client)
	client.services = []SupportService{{
		Code: "billing",
		Name: "Billing",
		Categories: []SupportCategory{{
			Code: "cur-backfill",
			Name: "CUR backfill",
		}},
	}}

	result := runBackfill(t, client, options)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_plan_stale" {
		t.Fatalf("Code = %q, want aws_plan_stale", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for stale support-case classification approval")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerBlocksMismatchedBackfillApprovalBeforeCreatingSupportCase(t *testing.T) {
	client := baselineBackfillClient()
	options := approvedBackfillOptions(t, client)
	options.Approvals = append(options.Approvals, options.Approvals[0])

	result := runBackfill(t, client, options)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_plan_approval_mismatch" {
		t.Fatalf("Code = %q, want aws_plan_approval_mismatch", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for mismatched approval")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerBlocksWhenClientIsUnavailable(t *testing.T) {
	runner := NewRunner(RunnerConfig{})
	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), requestBackfillOptions(baselineBackfillClient().export))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_provider_capability_blocked" {
		t.Fatalf("Code = %q, want aws_provider_capability_blocked", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false without client")
	}
	assertBackfillIdentityStatus(t, result, "unknown", "AWS caller identity and CUR 2.0 export state were not checked")
}

func TestRunnerRequiresBackfillOperationBeforeResolvingClient(t *testing.T) {
	client := baselineBackfillClient()
	options := approvedBackfillOptions(t, client)
	options.AWSBillingOperation = ""
	factoryCalls := 0
	runner := NewRunner(RunnerConfig{
		ClientFactory: func(workflow.ExecutionOptions) Client {
			factoryCalls++
			return client
		},
		Now: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
	})

	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_backfill_operation_required" {
		t.Fatalf("Code = %q, want aws_backfill_operation_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if factoryCalls != 0 || client.createCaseCalls != 0 {
		t.Fatalf("provider calls = factory %d create case %d, want none", factoryCalls, client.createCaseCalls)
	}
	assertBackfillIdentityStatus(t, result, "unknown", "AWS caller identity and CUR 2.0 export state were not checked")
}

func TestRunnerRequiresAWSBillingApplyPrereqsRequestBeforeResolvingClient(t *testing.T) {
	client := baselineBackfillClient()
	factoryCalls := 0
	runner := NewRunner(RunnerConfig{
		ClientFactory: func(workflow.ExecutionOptions) Client {
			factoryCalls++
			return client
		},
		Now: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
	})
	request := AWSBillingApplyPrereqsRequest()
	request.Action = assessment.ActionPreflight

	result := runner.Run(context.Background(), request, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_billing_apply_prereqs_request_required" {
		t.Fatalf("Code = %q, want aws_billing_apply_prereqs_request_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if factoryCalls != 0 || client.createCaseCalls != 0 {
		t.Fatalf("provider calls = factory %d create case %d, want none", factoryCalls, client.createCaseCalls)
	}
	assertBackfillIdentityStatus(t, result, "unknown", "AWS caller identity and CUR 2.0 export state were not checked")
}

func TestRunnerBlocksUnconfirmedAndWrongIntentBackfillApprovalsBeforeCreatingSupportCase(t *testing.T) {
	tests := []struct {
		name      string
		configure func(workflow.ExecutionOptions) workflow.ExecutionOptions
	}{
		{
			name: "unconfirmed approval",
			configure: func(options workflow.ExecutionOptions) workflow.ExecutionOptions {
				options.Approvals[0].Confirmed = false
				return options
			},
		},
		{
			name: "wrong intent",
			configure: func(options workflow.ExecutionOptions) workflow.ExecutionOptions {
				options.Approvals[0].Intent = "create_cur2_export"
				return options
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineBackfillClient()

			result := runBackfill(t, client, tt.configure(approvedBackfillOptions(t, client)))

			if result.Status != workflow.RunStatusBlocked {
				t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
			}
			if result.Code != "aws_plan_approval_mismatch" {
				t.Fatalf("Code = %q, want aws_plan_approval_mismatch", result.Code)
			}
			if result.Mutated {
				t.Fatal("Mutated = true, want false")
			}
			if client.createCaseCalls != 0 {
				t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
			}
		})
	}
}

func TestRunnerUsesClientFactory(t *testing.T) {
	client := baselineBackfillClient()
	runner := NewRunner(RunnerConfig{
		ClientFactory: func(options workflow.ExecutionOptions) Client {
			if options.AWSBillingOperation != workflow.AWSBillingOperationRequestBackfill {
				t.Fatalf("factory options operation = %q, want request_backfill", options.AWSBillingOperation)
			}
			return client
		},
		Now: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
	})

	result := runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), approvedBackfillOptions(t, client))

	if result.Code != "aws_backfill_support_case_created" {
		t.Fatalf("Code = %q, want aws_backfill_support_case_created", result.Code)
	}
	if client.createCaseCalls != 1 {
		t.Fatalf("CreateCase calls = %d, want 1", client.createCaseCalls)
	}
}

func TestRunnerCreatesSupportCaseWhenExplicitlyApproved(t *testing.T) {
	client := baselineBackfillClient()
	options := approvedBackfillOptions(t, client)
	result := runBackfill(t, client, options)

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.Code != "aws_backfill_support_case_created" {
		t.Fatalf("Code = %q, want aws_backfill_support_case_created", result.Code)
	}
	if !result.Mutated {
		t.Fatal("Mutated = false, want true after support case creation")
	}
	if result.PlanInput == nil || len(result.PlanInput.Steps) != 1 {
		t.Fatalf("PlanInput.Steps = %#v, want one support case creation step", result.PlanInput)
	}
	plan := planFromBackfillReport(t, result, options)
	if !plan.Approval.Required || !plan.Approval.Approved || plan.Approval.Blocked {
		t.Fatalf("approval summary = %#v, want required and approved after approved support case creation", plan.Approval)
	}
	if result.PlanInput.Steps[0].Intent != workflow.PlanStepCreate {
		t.Fatalf("step intent = %q, want %q", result.PlanInput.Steps[0].Intent, workflow.PlanStepCreate)
	}
	if result.PlanInput.Steps[0].ApprovalKind != "cloud_mutation" {
		t.Fatalf("step approval kind = %q, want cloud_mutation", result.PlanInput.Steps[0].ApprovalKind)
	}
	if client.createCaseCalls != 1 {
		t.Fatalf("CreateCase calls = %d, want 1", client.createCaseCalls)
	}
	if !client.openCasesRequest.IncludeCommunicationsSet || client.openCasesRequest.IncludeCommunications {
		t.Fatalf("open case duplicate check did not explicitly exclude communications: %#v", client.openCasesRequest)
	}
	if !strings.Contains(client.createdCase.Subject, backfillRequestReference(client.export.ExportARN, "2026-06")) {
		t.Fatalf("case subject = %q, want deterministic request reference", client.createdCase.Subject)
	}
	for _, want := range []string{client.export.Name, client.export.Destination.Bucket, client.export.Destination.Prefix, "2026-06"} {
		if !strings.Contains(client.createdCase.Body, want) {
			t.Fatalf("created case body = %q, want to include %q for AWS Support request", client.createdCase.Body, want)
		}
	}
	for _, forbidden := range []string{client.export.Destination.Bucket, client.export.Destination.Prefix, client.createdCase.Body} {
		if strings.Contains(result.Message, forbidden) {
			t.Fatalf("result message leaked support-case detail %q in %q", forbidden, result.Message)
		}
	}
}

func TestRunnerReportsWarningWhenCreatedCaseStatusLookupFails(t *testing.T) {
	client := baselineBackfillClient()
	client.describeCreatedCaseErr = NewProviderError("aws_support_describe_cases_failed", "created case lookup failed")

	result := runBackfill(t, client, approvedBackfillOptions(t, client))

	if result.Code != "aws_backfill_support_case_created" {
		t.Fatalf("Code = %q, want aws_backfill_support_case_created", result.Code)
	}
	if !result.Mutated {
		t.Fatal("Mutated = false, want true after support case creation")
	}
	check, ok := reportCheck(result, "aws_support_case_status_lookup_unavailable")
	if !ok {
		t.Fatalf("warning check not found in %#v", result.PlanInput.Checks)
	}
	if check.Status != workflow.CheckWarn {
		t.Fatalf("status lookup check status = %q, want %q", check.Status, workflow.CheckWarn)
	}
	assertCheckEvidence(t, check, "support_case_id", "case-created")
}

func TestRunnerReusesExistingMatchingOpenSupportCase(t *testing.T) {
	client := baselineBackfillClient()
	ref := backfillRequestReference(client.export.ExportARN, "2026-06")
	client.openCases = []SupportCase{{
		CaseID:    "case-123",
		DisplayID: "1234567890",
		Subject:   "Existing backfill request [" + ref + "]",
		Status:    "opened",
	}}

	result := runBackfill(t, client, approvedBackfillOptions(t, client))

	if result.Code != "aws_backfill_support_case_already_open" {
		t.Fatalf("Code = %q, want aws_backfill_support_case_already_open", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when reusing existing case")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerFailsClosedWhenMatchingOpenSupportCaseHasNoSafeReference(t *testing.T) {
	client := baselineBackfillClient()
	ref := backfillRequestReference(client.export.ExportARN, "2026-06")
	client.openCases = []SupportCase{{
		Subject: "Existing backfill request [" + ref + "]",
		Status:  "opened",
	}}

	result := runBackfill(t, client, approvedBackfillOptions(t, client))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_backfill_duplicate_check_failed" {
		t.Fatalf("Code = %q, want aws_backfill_duplicate_check_failed", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when matching case has no safe reference")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
	if _, ok := reportCheck(result, "aws_backfill_support_case_already_open"); ok {
		t.Fatal("existing-case reuse check present, want fail-closed duplicate check")
	}
}

func TestRunnerFailsClosedWhenDuplicateCheckFails(t *testing.T) {
	client := baselineBackfillClient()
	client.describeOpenCasesErr = NewProviderError("aws_support_describe_cases_failed", "duplicate check unavailable")

	result := runBackfill(t, client, approvedBackfillOptions(t, client))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_backfill_duplicate_check_failed" {
		t.Fatalf("Code = %q, want aws_backfill_duplicate_check_failed", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when duplicate check fails")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerBlocksWhenCreateCaseFails(t *testing.T) {
	client := baselineBackfillClient()
	client.createCaseErr = NewProviderError("aws_support_create_case_failed", "create failed")

	result := runBackfill(t, client, approvedBackfillOptions(t, client))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_support_create_case_failed" {
		t.Fatalf("Code = %q, want aws_support_create_case_failed", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when CreateCase fails")
	}
}

func TestRunnerBlocksWhenCreatedSupportCaseIDIsEmpty(t *testing.T) {
	client := baselineBackfillClient()
	client.createdCaseID = ""

	result := runBackfill(t, client, approvedBackfillOptions(t, client))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_support_create_case_response_incomplete" {
		t.Fatalf("Code = %q, want aws_support_create_case_response_incomplete", result.Code)
	}
	if !result.Mutated {
		t.Fatal("Mutated = false, want true because AWS Support create was attempted and returned an incomplete response")
	}
	if client.createCaseCalls != 1 {
		t.Fatalf("CreateCase calls = %d, want 1", client.createCaseCalls)
	}
	if _, ok := reportCheck(result, "aws_backfill_support_case_created"); ok {
		t.Fatal("created support case check present, want fail-closed report")
	}
}

func TestRunnerTreatsIncompleteCreateCaseProviderResponseAsMutationAmbiguous(t *testing.T) {
	client := baselineBackfillClient()
	client.createCaseErr = NewProviderError("aws_support_create_case_response_incomplete", "AWS Support CreateCase response did not include a case ID.")

	result := runBackfill(t, client, approvedBackfillOptions(t, client))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_support_create_case_response_incomplete" {
		t.Fatalf("Code = %q, want aws_support_create_case_response_incomplete", result.Code)
	}
	if !result.Mutated {
		t.Fatal("Mutated = false, want true for incomplete CreateCase response after request")
	}
	if _, ok := reportCheck(result, "aws_backfill_support_case_created"); ok {
		t.Fatal("created support case check present, want fail-closed incomplete-response report")
	}
}

func TestRunnerGuidesWhenSupportAPIIsUnavailable(t *testing.T) {
	client := baselineBackfillClient()
	client.describeServicesErr = NewProviderError("aws_support_subscription_required", "support plan does not allow Support API case creation")

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.Code != "aws_support_case_manual_fallback_required" {
		t.Fatalf("Code = %q, want aws_support_case_manual_fallback_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when Support API is unavailable")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerDoesNotCreateCaseWhenPreviousMonthIsAlreadyPresent(t *testing.T) {
	client := baselineBackfillClient()
	client.previousMonthDataReady = true
	client.previousMonthManifestReady = true

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusReady)
	}
	if result.Code != "aws_backfill_not_required" {
		t.Fatalf("Code = %q, want aws_backfill_not_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when previous-month billing data is already present")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerGuidesWhenNoCUR2ExportExists(t *testing.T) {
	client := baselineBackfillClient()
	client.exports = nil

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.Code != "aws_cur2_creation_required" {
		t.Fatalf("Code = %q, want aws_cur2_creation_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when no CUR 2.0 export exists")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
	assertBackfillIdentityStatus(t, result, "verified", "AWS caller identity was verified")
}

func TestRunnerBlocksWhenPreflightSeesIncompleteExportSummary(t *testing.T) {
	client := baselineBackfillClient()
	client.exportPages = map[string]cur2preflight.ExportPage{
		"": {
			Exports: []cur2preflight.ExportSummary{{
				Name:       "matilda-cur2",
				TableName:  "COST_AND_USAGE_REPORT",
				SourceType: "COST_AND_USAGE_REPORT",
			}},
		},
	}

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_data_exports_incomplete_export_summary" {
		t.Fatalf("Code = %q, want aws_data_exports_incomplete_export_summary", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when AWS Data Exports response is incomplete")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerBlocksWhenCUR2PreflightIsNotReadyForBackfill(t *testing.T) {
	client := baselineBackfillClient()
	client.table.Columns = client.table.Columns[1:]

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_backfill_preflight_not_ready" {
		t.Fatalf("Code = %q, want aws_backfill_preflight_not_ready", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when preflight blocks")
	}
	assertBackfillIdentityStatus(t, result, "verified", "AWS caller identity was verified")
}

func TestRunnerBackfillWithoutExportRefBlocksBeforeExportDiscovery(t *testing.T) {
	client := baselineBackfillClient()
	second := client.export
	second.Name = "matilda-cur2-second"
	second.ExportARN = "arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2-second"
	client.exports = append(client.exports, second)
	options := requestBackfillOptions(client.export)
	options.Selectors.AWS.CUR2ExportRef = ""
	options, err := workflow.NormalizeExecutionOptionsForRequest(AWSBillingApplyPrereqsRequest(), options)
	if err != nil {
		t.Fatalf("NormalizeExecutionOptionsForRequest returned error: %v", err)
	}

	result := runBackfill(t, client, options)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_cur2_export_selection_required" {
		t.Fatalf("Code = %q, want aws_cur2_export_selection_required", result.Code)
	}
	if !strings.Contains(result.Message, "--export-ref") {
		t.Fatalf("Message = %q, want actionable --export-ref guidance", result.Message)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when export ref is missing")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
	if client.listObjectsCalls != 0 {
		t.Fatalf("ListObjects calls = %d, want 0 before explicit export selection", client.listObjectsCalls)
	}
	check, ok := reportCheck(result, "aws_cur2_export_selection_required")
	if !ok {
		t.Fatalf("selection-required check not found in %#v", result.PlanInput.Checks)
	}
	if !strings.Contains(check.Message, "--export-ref") {
		t.Fatalf("selection-required check message = %q, want --export-ref guidance", check.Message)
	}
}

func TestRunnerBackfillWithoutExportRefBlocksSingleExportAutoSelection(t *testing.T) {
	client := baselineBackfillClient()
	options := requestBackfillOptions(client.export)
	options.Selectors.AWS.CUR2ExportRef = ""
	options, err := workflow.NormalizeExecutionOptionsForRequest(AWSBillingApplyPrereqsRequest(), options)
	if err != nil {
		t.Fatalf("NormalizeExecutionOptionsForRequest returned error: %v", err)
	}

	result := runBackfill(t, client, options)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_cur2_export_selection_required" {
		t.Fatalf("Code = %q, want aws_cur2_export_selection_required", result.Code)
	}
	if !strings.Contains(result.Message, "--export-ref") {
		t.Fatalf("Message = %q, want actionable --export-ref guidance", result.Message)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when export ref is missing")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestPreflightNotBackfillReportPreservesSelectionEvidence(t *testing.T) {
	preflight := workflow.CapabilityReport{
		Status:  workflow.RunStatusBlocked,
		Code:    "aws_cur2_export_ambiguous",
		Message: "Multiple AWS CUR 2.0 exports were found.",
		PlanInput: &workflow.ExecutionPlanInput{Checks: []workflow.PlanCheck{{
			ID:     "aws_cur2_export_ambiguous",
			Status: workflow.CheckFail,
			Evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-abcdefghijklmnop"},
				{Key: "candidate_1_matilda_support", Value: "preferred"},
				{Key: "unrelated_detail", Value: "ignored"},
			},
		}}},
	}

	result := Runner{}.preflightNotBackfillReport(AWSBillingApplyPrereqsRequest(), preflight)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_cur2_export_ambiguous" {
		t.Fatalf("Code = %q, want aws_cur2_export_ambiguous", result.Code)
	}
	check, ok := reportCheck(result, "aws_cur2_export_ambiguous")
	if !ok {
		t.Fatalf("ambiguous selection check not found in %#v", result.PlanInput.Checks)
	}
	assertCheckEvidence(t, check, "candidate_1_export_ref", "cur2-abcdefghijklmnop")
	assertCheckEvidence(t, check, "candidate_1_matilda_support", "preferred")
	for _, evidence := range check.Evidence {
		if evidence.Key == "unrelated_detail" {
			t.Fatalf("selection evidence included unrelated item: %#v", check.Evidence)
		}
	}
}

func TestRunnerBlocksWhenPreviousMonthInspectionFails(t *testing.T) {
	client := baselineBackfillClient()
	client.listObjectsErr = cur2preflight.NewProviderError("aws_s3_bucket_inaccessible", "list failed")
	client.listObjectsErrAfterCalls = 2

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_s3_bucket_inaccessible" {
		t.Fatalf("Code = %q, want aws_s3_bucket_inaccessible", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when previous-month inspection fails")
	}
}

func TestRunnerBlocksWhenPreviousMonthManifestRecheckFails(t *testing.T) {
	client := baselineBackfillClient()
	client.previousMonthDataReady = true
	client.listObjectsErr = cur2preflight.NewProviderError("aws_s3_bucket_inaccessible", "manifest list failed")
	client.listObjectsErrAfterCalls = 3

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_s3_bucket_inaccessible" {
		t.Fatalf("Code = %q, want aws_s3_bucket_inaccessible", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when previous-month manifest recheck fails")
	}
}

func TestRunnerGuidesWhenSupportClassificationIsAmbiguous(t *testing.T) {
	client := baselineBackfillClient()
	client.services = append(client.services, SupportService{
		Code: "billing-2",
		Name: "Billing",
		Categories: []SupportCategory{{
			Code: "cur-backfill",
			Name: "CUR backfill",
		}},
	})

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.SupportStatus != workflow.SupportGuided {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportGuided)
	}
	if result.Code != "aws_support_case_classification_ambiguous" {
		t.Fatalf("Code = %q, want aws_support_case_classification_ambiguous", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for ambiguous support classification")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
	check, ok := reportCheck(result, "aws_support_case_classification_ambiguous")
	if !ok {
		t.Fatalf("manual fallback check not found in %#v", result.PlanInput.Checks)
	}
	assertCheckEvidence(t, check, "manual_support_request_needs", "report name, billing period, S3 bucket details")
}

func TestRunnerGuidesForGenericBillingCategoryWithoutMutation(t *testing.T) {
	client := baselineBackfillClient()
	client.services = []SupportService{{
		Code: "billing",
		Name: "Billing",
		Categories: []SupportCategory{{
			Code: "billing",
			Name: "Billing",
		}},
	}}

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.SupportStatus != workflow.SupportGuided {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportGuided)
	}
	if result.Code != "aws_support_case_classification_unavailable" {
		t.Fatalf("Code = %q, want aws_support_case_classification_unavailable", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for generic billing category")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerGuidesWhenLowSeverityIsUnavailable(t *testing.T) {
	client := baselineBackfillClient()
	client.severities = []SupportSeverity{{Code: "normal", Name: "System impaired"}}

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.SupportStatus != workflow.SupportGuided {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportGuided)
	}
	if result.Code != "aws_support_low_severity_unavailable" {
		t.Fatalf("Code = %q, want aws_support_low_severity_unavailable", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when low severity is unavailable")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestRunnerGuidesWhenSupportClassificationHasIncompleteCodes(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeBackfillClient)
		wantCode  string
	}{
		{
			name: "empty service code",
			configure: func(client *fakeBackfillClient) {
				client.services = []SupportService{{
					Name: "Billing",
					Categories: []SupportCategory{{
						Code: "cost-and-usage-reports",
						Name: "Cost and Usage Reports",
					}},
				}}
			},
			wantCode: "aws_support_case_classification_unavailable",
		},
		{
			name: "empty category code",
			configure: func(client *fakeBackfillClient) {
				client.services = []SupportService{{
					Code: "billing",
					Name: "Billing",
					Categories: []SupportCategory{{
						Name: "Cost and Usage Reports",
					}},
				}}
			},
			wantCode: "aws_support_case_classification_unavailable",
		},
		{
			name: "empty severity code",
			configure: func(client *fakeBackfillClient) {
				client.severities = []SupportSeverity{{Name: "Low"}}
			},
			wantCode: "aws_support_low_severity_unavailable",
		},
		{
			name: "non-low severity code with low display name",
			configure: func(client *fakeBackfillClient) {
				client.severities = []SupportSeverity{{Code: "normal", Name: "Low"}}
			},
			wantCode: "aws_support_low_severity_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineBackfillClient()
			tt.configure(client)

			result := runBackfill(t, client, requestBackfillOptions(client.export))

			if result.Status != workflow.RunStatusManualSteps {
				t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
			}
			if result.SupportStatus != workflow.SupportGuided {
				t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportGuided)
			}
			if result.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", result.Code, tt.wantCode)
			}
			if result.Mutated {
				t.Fatal("Mutated = true, want false when support classification is incomplete")
			}
			if client.createCaseCalls != 0 {
				t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
			}
		})
	}
}

func TestRunnerGuidesWhenNoSupportCategoryMatchesBackfillIntent(t *testing.T) {
	client := baselineBackfillClient()
	client.services = []SupportService{{
		Code: "compute",
		Name: "Compute",
		Categories: []SupportCategory{{
			Code: "general",
			Name: "General guidance",
		}},
	}}

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.SupportStatus != workflow.SupportGuided {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportGuided)
	}
	if result.Code != "aws_support_case_classification_unavailable" {
		t.Fatalf("Code = %q, want aws_support_case_classification_unavailable", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when no safe support category exists")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestSupportTextMatchesBackfillIntent(t *testing.T) {
	tests := []struct {
		name string
		text []string
		want bool
	}{
		{name: "cost and usage reports", text: []string{"Billing", "Cost and Usage Reports"}, want: true},
		{name: "cost ampersand usage reports", text: []string{"Billing", "Cost & Usage Reports"}, want: true},
		{name: "cur backfill", text: []string{"Billing", "CUR backfill"}, want: true},
		{name: "generic billing", text: []string{"Billing", "Billing"}, want: false},
		{name: "generic cost", text: []string{"Billing", "Cost"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportTextMatchesBackfillIntent(tt.text...); got != tt.want {
				t.Fatalf("supportTextMatchesBackfillIntent(%q) = %t, want %t", tt.text, got, tt.want)
			}
		})
	}
}

func TestRunnerGuidesWhenCreateCaseOptionsArePartiallyUnverified(t *testing.T) {
	client := baselineBackfillClient()
	client.services = []SupportService{{
		Code: "billing",
		Name: "Billing",
		Categories: []SupportCategory{
			{Code: "cost-and-usage-reports", Name: "Cost and Usage Reports"},
			{Code: "cur-backfill", Name: "CUR backfill"},
		},
	}}
	client.createCaseOptionsErrByCategory = map[string]error{
		"billing/cost-and-usage-reports": NewProviderError("aws_support_api_unavailable", "options lookup unavailable"),
	}
	client.createCaseOptionsByCategory = map[string]SupportCreateCaseOptions{
		"billing/cur-backfill": {Available: true},
	}

	result := runBackfill(t, client, requestBackfillOptions(client.export))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusManualSteps)
	}
	if result.SupportStatus != workflow.SupportGuided {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportGuided)
	}
	if result.Code != "aws_support_create_case_options_unverified" {
		t.Fatalf("Code = %q, want aws_support_create_case_options_unverified", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when any create-case option lookup is unverified")
	}
	if client.createCaseCalls != 0 {
		t.Fatalf("CreateCase calls = %d, want 0", client.createCaseCalls)
	}
}

func TestProviderErrorStringAndManualFallbackClassifier(t *testing.T) {
	err := NewProviderError("aws_support_access_denied", "denied")
	if !strings.Contains(err.Error(), "aws_support_access_denied") {
		t.Fatalf("ProviderError.Error() = %q", err.Error())
	}
	if !manualFallbackProviderError("aws_support_access_denied") {
		t.Fatal("manualFallbackProviderError rejected access denied fallback")
	}
	if manualFallbackProviderError("aws_support_case_classification_ambiguous") {
		t.Fatal("manualFallbackProviderError accepted classification ambiguity")
	}
}

func TestSelectCUR2ExportFailsClosedForMissingAmbiguousAndProviderErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("requested ref not found", func(t *testing.T) {
		client := baselineBackfillClient()
		_, _, err := selectCUR2Export(ctx, client, "cur2-ffffffffffffffff")
		assertProviderErrorCode(t, err, "aws_cur2_export_ref_not_found")
	})

	t.Run("ambiguous without selector", func(t *testing.T) {
		client := baselineBackfillClient()
		second := client.export
		second.Name = "matilda-cur2-second"
		second.ExportARN = "arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2-second"
		client.exports = append(client.exports, second)
		_, _, err := selectCUR2Export(ctx, client, "")
		assertProviderErrorCode(t, err, "aws_cur2_export_ambiguous")
	})

	t.Run("list exports error", func(t *testing.T) {
		client := baselineBackfillClient()
		client.listExportsErr = NewProviderError("aws_data_exports_access_denied", "denied")
		_, _, err := selectCUR2Export(ctx, client, "")
		assertProviderErrorCode(t, err, "aws_data_exports_access_denied")
	})

	t.Run("incomplete export summary", func(t *testing.T) {
		client := baselineBackfillClient()
		client.exportPages = map[string]cur2preflight.ExportPage{
			"": {
				Exports: []cur2preflight.ExportSummary{{
					Name:       "matilda-cur2",
					TableName:  "COST_AND_USAGE_REPORT",
					SourceType: "COST_AND_USAGE_REPORT",
				}},
			},
		}
		_, _, err := selectCUR2Export(ctx, client, "")
		assertProviderErrorCode(t, err, "aws_data_exports_incomplete_export_summary")
	})

	t.Run("get export error", func(t *testing.T) {
		client := baselineBackfillClient()
		client.getExportErr = NewProviderError("aws_cur2_export_unavailable", "unavailable")
		_, _, err := selectCUR2Export(ctx, client, "")
		assertProviderErrorCode(t, err, "aws_cur2_export_unavailable")
	})

	t.Run("paginates list exports", func(t *testing.T) {
		client := baselineBackfillClient()
		client.exportPages = map[string]cur2preflight.ExportPage{
			"": {
				NextToken: "page-2",
			},
			"page-2": {
				Exports: []cur2preflight.ExportSummary{{
					Name:       client.export.Name,
					ExportARN:  client.export.ExportARN,
					TableName:  "COST_AND_USAGE_REPORT",
					SourceType: "COST_AND_USAGE_REPORT",
				}},
			},
		}

		export, ref, err := selectCUR2Export(ctx, client, cur2preflight.SafeCUR2ExportRef(client.export.ExportARN))
		if err != nil {
			t.Fatalf("selectCUR2Export returned error: %v", err)
		}
		if export.ExportARN != client.export.ExportARN {
			t.Fatalf("selected export ARN = %q, want baseline export", export.ExportARN)
		}
		if ref != cur2preflight.SafeCUR2ExportRef(client.export.ExportARN) {
			t.Fatalf("selected ref = %q, want requested ref", ref)
		}
	})

	t.Run("fails closed on repeated pagination token", func(t *testing.T) {
		client := baselineBackfillClient()
		client.exportPages = map[string]cur2preflight.ExportPage{
			"":       {NextToken: "page-2"},
			"page-2": {NextToken: "page-2"},
		}

		_, _, err := selectCUR2Export(ctx, client, "")
		assertProviderErrorCode(t, err, "aws_data_exports_pagination_unbounded")
	})
}

func TestPrefixHasMatchingObjectFailsClosedForEmptyOrUnboundedPrefixes(t *testing.T) {
	ctx := context.Background()
	client := baselineBackfillClient()
	if _, err := prefixHasMatchingObject(ctx, client, client.export, "", func(string) bool { return true }); err == nil {
		t.Fatal("prefixHasMatchingObject accepted empty prefix")
	}

	client = baselineBackfillClient()
	client.previousMonthDataReady = true
	found, err := prefixHasMatchingObject(ctx, client, client.export, cur2preflight.PreviousMonthDataPrefix(client.export, "2026-06"), func(key string) bool {
		return cur2preflight.MatchesPreviousMonthDataKey(key, client.export, "2026-06")
	})
	if err != nil {
		t.Fatalf("prefixHasMatchingObject returned error: %v", err)
	}
	if !found {
		t.Fatal("prefixHasMatchingObject did not find expected data object")
	}

	client = baselineBackfillClient()
	client.listObjectsNextToken = "next"
	_, err = prefixHasMatchingObject(ctx, client, client.export, cur2preflight.PreviousMonthDataPrefix(client.export, "2026-06"), func(string) bool { return false })
	assertProviderErrorCode(t, err, "aws_cur2_previous_month_missing")
}

func TestSupportCaseBodyUsesFallbackMissingComponentText(t *testing.T) {
	client := baselineBackfillClient()
	body := supportCaseBody(backfillContext{
		Export: client.export,
		Period: "2026-06",
	}, "backfill-test")
	if !strings.Contains(body, "previous-month billing export") {
		t.Fatalf("support case body = %q, want fallback missing component text", body)
	}
}

func TestSupportCaseBindingRefIsOpaqueStableAndSensitive(t *testing.T) {
	client := baselineBackfillClient()
	context := backfillContext{
		Export:               client.export,
		ExportRef:            cur2preflight.SafeCUR2ExportRef(client.export.ExportARN),
		Period:               "2026-06",
		MissingDataPartition: true,
		MissingManifest:      true,
	}
	classification := supportClassification{
		Language:     "en",
		IssueType:    "technical",
		ServiceCode:  "billing",
		CategoryCode: "cost-and-usage-reports",
		SeverityCode: "low",
	}
	reference := backfillRequestReference(client.export.ExportARN, context.Period)

	binding := supportCaseBindingRef(classification, context, reference)

	if !strings.HasPrefix(binding, "support_case_") {
		t.Fatalf("supportCaseBindingRef = %q, want support_case_ prefix", binding)
	}
	for _, r := range strings.TrimPrefix(binding, "support_case_") {
		if r < 'a' || r > 'p' {
			t.Fatalf("supportCaseBindingRef = %q, want account-id-safe lowercase a-p suffix", binding)
		}
	}
	if binding != supportCaseBindingRef(classification, context, reference) {
		t.Fatal("supportCaseBindingRef is not stable for identical support-case facts")
	}
	body := supportCaseBody(context, reference)
	for _, forbidden := range []string{
		client.export.Name,
		client.export.ExportARN,
		client.export.SourceAccount,
		client.export.Destination.Bucket,
		client.export.Destination.Prefix,
		body,
	} {
		if strings.Contains(binding, forbidden) {
			t.Fatalf("supportCaseBindingRef leaked raw support-case detail %q in %q", forbidden, binding)
		}
	}
	changedContext := context
	changedContext.Export.Destination.Prefix = "matilda/cur2-renamed"
	if binding == supportCaseBindingRef(classification, changedContext, reference) {
		t.Fatal("supportCaseBindingRef did not change when support-case body facts changed")
	}
	changedClassification := classification
	changedClassification.CategoryCode = "cur-backfill"
	if binding == supportCaseBindingRef(changedClassification, context, reference) {
		t.Fatal("supportCaseBindingRef did not change when support classification changed")
	}
	if encoded := letterEncodeHash([]byte{0x1f}, 1); encoded != "b" {
		t.Fatalf("letterEncodeHash odd length = %q, want first high-nibble letter", encoded)
	}
}

func runBackfill(t *testing.T, client *fakeBackfillClient, options workflow.ExecutionOptions) workflow.CapabilityReport {
	t.Helper()
	runner := NewRunner(RunnerConfig{
		Client: client,
		Now:    time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
	})
	return runner.Run(context.Background(), AWSBillingApplyPrereqsRequest(), options)
}

func runBackfillThroughRegistry(t *testing.T, client *fakeBackfillClient, options workflow.ExecutionOptions) workflow.Result {
	t.Helper()
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: AWSBillingApplyPrereqsRequest(),
		Runner: NewRunner(RunnerConfig{
			Client: client,
			Now:    time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	return registry.ExecuteContext(context.Background(), AWSBillingApplyPrereqsRequest(), options)
}

func requestBackfillOptions(export cur2preflight.Export) workflow.ExecutionOptions {
	ref := cur2preflight.SafeCUR2ExportRef(export.ExportARN)
	options, err := workflow.NormalizeExecutionOptionsForRequest(AWSBillingApplyPrereqsRequest(), workflow.ExecutionOptions{
		AWSBillingOperation: workflow.AWSBillingOperationRequestBackfill,
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				Profile:       "default",
				Region:        "us-east-1",
				CUR2ExportRef: ref,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return options
}

func approvedBackfillOptions(t *testing.T, client *fakeBackfillClient) workflow.ExecutionOptions {
	t.Helper()
	options := requestBackfillOptions(client.export)
	plan := backfillPreviewPlan(t, client, options)
	options.Approvals = []workflow.ExecutionApproval{{
		OperationID: workflow.AWSBackfillSupportCaseOperationID,
		Intent:      workflow.ApprovalIntentRequestBackfillSupportCase,
		PlanID:      plan.PlanID,
		Confirmed:   true,
	}}
	normalized, err := workflow.NormalizeExecutionOptionsForRequest(AWSBillingApplyPrereqsRequest(), options)
	if err != nil {
		t.Fatalf("NormalizeExecutionOptionsForRequest returned error: %v", err)
	}
	return normalized
}

func backfillPreviewPlan(t *testing.T, client *fakeBackfillClient, options workflow.ExecutionOptions) workflow.ExecutionPlan {
	t.Helper()
	probe := *client
	result := runBackfill(t, &probe, options)
	if result.Code != "aws_backfill_support_case_approval_required" {
		t.Fatalf("preview code = %q, want aws_backfill_support_case_approval_required", result.Code)
	}
	return planFromBackfillReport(t, result, options)
}

func planFromBackfillReport(t *testing.T, result workflow.CapabilityReport, options workflow.ExecutionOptions) workflow.ExecutionPlan {
	t.Helper()
	if result.PlanInput == nil {
		t.Fatal("PlanInput is nil")
	}
	input := *result.PlanInput
	input.ExecutionOptions = options
	plan, err := workflow.BuildExecutionPlan(input)
	if err != nil {
		t.Fatalf("BuildExecutionPlan returned error: %v", err)
	}
	return plan
}

func assertBackfillIdentityStatus(t *testing.T, result workflow.CapabilityReport, wantStatus string, wantSummary string) {
	t.Helper()
	if result.PlanInput == nil {
		t.Fatal("PlanInput is nil")
	}
	got := result.PlanInput.OperatorIdentitySummary
	if got.IdentityStatus != wantStatus {
		t.Fatalf("IdentityStatus = %q, want %q", got.IdentityStatus, wantStatus)
	}
	if !strings.Contains(got.Summary, wantSummary) {
		t.Fatalf("identity summary = %q, want to contain %q", got.Summary, wantSummary)
	}
}

func baselineBackfillClient() *fakeBackfillClient {
	requiredColumns := requiredCUR2ColumnsForTest()
	export := cur2preflight.Export{
		Name:           "matilda-cur2",
		ExportARN:      "arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2",
		SourceARN:      "arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2",
		SourceAccount:  "123456789012",
		QueryStatement: "SELECT " + strings.Join(requiredColumns, ", ") + " FROM COST_AND_USAGE_REPORT",
		TableConfigurations: map[string]map[string]string{
			"COST_AND_USAGE_REPORT": {
				"TIME_GRANULARITY":  "MONTHLY",
				"INCLUDE_RESOURCES": "FALSE",
			},
		},
		Destination: cur2preflight.S3Destination{
			Bucket: "matilda-cur2-billing",
			Prefix: "matilda/cur2",
			Region: "us-east-1",
			Output: cur2preflight.S3Output{
				Format:      "TEXT_OR_CSV",
				Compression: "GZIP",
				Overwrite:   "CREATE_NEW_REPORT",
				OutputType:  "CUSTOM",
			},
		},
		RefreshCadence: "SYNCHRONOUS",
		HealthStatus:   "HEALTHY",
	}
	return &fakeBackfillClient{
		config: cur2preflight.Configuration{Region: "us-east-1"},
		identity: cur2preflight.Identity{
			AccountID: "123456789012",
			CallerARN: "arn:aws:sts::123456789012:assumed-role/Admin/example",
		},
		table: cur2preflight.Table{
			Name:       "COST_AND_USAGE_REPORT",
			Columns:    requiredColumns,
			Properties: map[string]string{"TIME_GRANULARITY": "MONTHLY", "INCLUDE_RESOURCES": "FALSE"},
		},
		exports: []cur2preflight.Export{export},
		export:  export,
		services: []SupportService{{
			Code: "billing",
			Name: "Billing",
			Categories: []SupportCategory{{
				Code: "cost-and-usage-reports",
				Name: "Cost and Usage Reports",
			}},
		}},
		severities: []SupportSeverity{{Code: "low", Name: "Low"}},
		createCaseOptions: SupportCreateCaseOptions{
			Available: true,
		},
		createdCaseID: "case-created",
		createdCaseDetails: SupportCase{
			CaseID:    "case-created",
			DisplayID: "1234567890",
			Status:    "opened",
		},
	}
}

type fakeBackfillClient struct {
	config   cur2preflight.Configuration
	identity cur2preflight.Identity
	table    cur2preflight.Table
	exports  []cur2preflight.Export
	export   cur2preflight.Export

	previousMonthDataReady     bool
	previousMonthManifestReady bool

	services                       []SupportService
	severities                     []SupportSeverity
	createCaseOptions              SupportCreateCaseOptions
	createCaseOptionsByCategory    map[string]SupportCreateCaseOptions
	createCaseOptionsErrByCategory map[string]error

	openCases            []SupportCase
	openCasesRequest     DescribeCasesRequest
	describeOpenCasesErr error

	describeServicesErr error

	createdCase              CreateCaseRequest
	createdCaseID            string
	createdCaseDetails       SupportCase
	describeCreatedCaseErr   error
	createCaseErr            error
	createCaseCalls          int
	exportPages              map[string]cur2preflight.ExportPage
	listExportsErr           error
	getExportErr             error
	listObjectsErr           error
	listObjectsErrAfterCalls int
	listObjectsCalls         int
	listObjectsNextToken     string
}

func (f *fakeBackfillClient) CheckConfiguration(context.Context) (cur2preflight.Configuration, error) {
	return f.config, nil
}

func (f *fakeBackfillClient) GetCallerIdentity(context.Context) (cur2preflight.Identity, error) {
	return f.identity, nil
}

func (f *fakeBackfillClient) ListTables(context.Context, string) (cur2preflight.TablePage, error) {
	return cur2preflight.TablePage{Tables: []cur2preflight.TableSummary{{Name: "COST_AND_USAGE_REPORT"}}}, nil
}

func (f *fakeBackfillClient) GetTable(context.Context, string, map[string]string) (cur2preflight.Table, error) {
	return f.table, nil
}

func (f *fakeBackfillClient) ListExports(_ context.Context, token string) (cur2preflight.ExportPage, error) {
	if f.listExportsErr != nil {
		return cur2preflight.ExportPage{}, f.listExportsErr
	}
	if f.exportPages != nil {
		return f.exportPages[token], nil
	}
	page := cur2preflight.ExportPage{}
	for _, export := range f.exports {
		page.Exports = append(page.Exports, cur2preflight.ExportSummary{
			Name:       export.Name,
			ExportARN:  export.ExportARN,
			TableName:  "COST_AND_USAGE_REPORT",
			SourceType: "COST_AND_USAGE_REPORT",
		})
	}
	return page, nil
}

func (f *fakeBackfillClient) GetExport(_ context.Context, exportARN string) (cur2preflight.Export, error) {
	if f.getExportErr != nil {
		return cur2preflight.Export{}, f.getExportErr
	}
	for _, export := range f.exports {
		if export.ExportARN == exportARN {
			return export, nil
		}
	}
	return cur2preflight.Export{}, errors.New("export not found")
}

func (f *fakeBackfillClient) HeadBucket(context.Context, string) (cur2preflight.BucketAccess, error) {
	return cur2preflight.BucketAccess{Accessible: true, Region: "us-east-1"}, nil
}

func (f *fakeBackfillClient) GetBucketPolicy(context.Context, string) (string, error) {
	return `{"Statement":[{"Effect":"Allow","Principal":{"Service":"bcm-data-exports.amazonaws.com"},"Action":"s3:PutObject","Resource":"arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*","Condition":{"StringEquals":{"aws:SourceAccount":"123456789012","aws:SourceArn":"arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2"}}}]}`, nil
}

func (f *fakeBackfillClient) ListExecutions(context.Context, string, string) (cur2preflight.ExecutionPage, error) {
	return cur2preflight.ExecutionPage{Executions: []cur2preflight.Execution{{ID: "execution-1", Status: "DELIVERY_SUCCESS"}}}, nil
}

func (f *fakeBackfillClient) GetExecution(context.Context, string, string) (cur2preflight.Execution, error) {
	return cur2preflight.Execution{ID: "execution-1", Status: "DELIVERY_SUCCESS"}, nil
}

func (f *fakeBackfillClient) ListObjects(_ context.Context, _ string, prefix string, _ string, _ int32) (cur2preflight.ObjectPage, error) {
	f.listObjectsCalls++
	if f.listObjectsErr != nil && f.listObjectsCalls > f.listObjectsErrAfterCalls {
		return cur2preflight.ObjectPage{}, f.listObjectsErr
	}
	keys := []string{}
	if f.previousMonthDataReady && strings.Contains(prefix, "/data/BILLING_PERIOD=2026-06/") {
		keys = append(keys, prefix+"00001.csv.gz")
	}
	if f.previousMonthManifestReady && strings.Contains(prefix, "/metadata/BILLING_PERIOD=2026-06/") {
		keys = append(keys, prefix+"Manifest.json")
	}
	return cur2preflight.ObjectPage{Keys: keys, NextToken: f.listObjectsNextToken}, nil
}

func (f *fakeBackfillClient) DescribeServices(context.Context, DescribeServicesRequest) ([]SupportService, error) {
	if f.describeServicesErr != nil {
		return nil, f.describeServicesErr
	}
	return f.services, nil
}

func (f *fakeBackfillClient) DescribeSeverityLevels(context.Context, DescribeSeverityLevelsRequest) ([]SupportSeverity, error) {
	return f.severities, nil
}

func (f *fakeBackfillClient) DescribeCreateCaseOptions(_ context.Context, request DescribeCreateCaseOptionsRequest) (SupportCreateCaseOptions, error) {
	if request.IssueType != "technical" {
		return SupportCreateCaseOptions{}, nil
	}
	key := request.ServiceCode + "/" + request.CategoryCode
	if err, ok := f.createCaseOptionsErrByCategory[key]; ok {
		return SupportCreateCaseOptions{}, err
	}
	if options, ok := f.createCaseOptionsByCategory[key]; ok {
		return options, nil
	}
	return f.createCaseOptions, nil
}

func (f *fakeBackfillClient) DescribeCases(_ context.Context, request DescribeCasesRequest) ([]SupportCase, error) {
	if len(request.CaseIDs) > 0 {
		if f.describeCreatedCaseErr != nil {
			return nil, f.describeCreatedCaseErr
		}
		return []SupportCase{f.createdCaseDetails}, nil
	}
	f.openCasesRequest = request
	if f.describeOpenCasesErr != nil {
		return nil, f.describeOpenCasesErr
	}
	return f.openCases, nil
}

func (f *fakeBackfillClient) CreateCase(_ context.Context, request CreateCaseRequest) (CreateCaseResult, error) {
	f.createCaseCalls++
	f.createdCase = request
	if f.createCaseErr != nil {
		return CreateCaseResult{}, f.createCaseErr
	}
	return CreateCaseResult{CaseID: f.createdCaseID}, nil
}

func requiredCUR2ColumnsForTest() []string {
	return []string{
		"line_item_product_code",
		"product_product_name",
		"line_item_operation",
		"line_item_line_item_description",
		"line_item_line_item_type",
		"line_item_currency_code",
		"pricing_unit",
		"line_item_usage_amount",
		"line_item_unblended_cost",
		"line_item_usage_type",
	}
}

func assertProviderErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	var providerErr ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %#v, want ProviderError %s", err, want)
	}
	if providerErr.Code != want {
		t.Fatalf("ProviderError.Code = %q, want %s", providerErr.Code, want)
	}
}

func reportCheck(result workflow.CapabilityReport, id string) (workflow.PlanCheck, bool) {
	if result.PlanInput == nil {
		return workflow.PlanCheck{}, false
	}
	for _, check := range result.PlanInput.Checks {
		if check.ID == id {
			return check, true
		}
	}
	return workflow.PlanCheck{}, false
}

func assertCheckEvidence(t *testing.T, check workflow.PlanCheck, key string, want string) {
	t.Helper()
	for _, evidence := range check.Evidence {
		if evidence.Key == key && evidence.Value == want {
			return
		}
	}
	t.Fatalf("check %s evidence missing %s=%s: %#v", check.ID, key, want, check.Evidence)
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
