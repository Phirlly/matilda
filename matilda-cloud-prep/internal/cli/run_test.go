package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/bootstrap"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/guided"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func runCLI(args ...string) (int, string, string) {
	return runCLIWithInput("", args...)
}

func runCLIWithInput(input string, args ...string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithInput(args, strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func runCLIWithRegistry(registryInput string, args ...string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithRegistry(args, strings.NewReader(registryInput), &stdout, &stderr, bootstrap.Registry(bootstrap.RegistryConfig{}))
	return code, stdout.String(), stderr.String()
}

func TestStartGuidesToSelectedPreflightCommand(t *testing.T) {
	code, stdout, stderr := runCLIWithInput("1\n3\n", "start")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{
		"Matilda Cloud Prep",
		"Rapid Assessment - Billing Based",
		"GCP",
		"matilda-prep rapid-assessment billing gcp preflight",
		"Implemented provider paths run verified read-only checks",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout, want)
		}
	}
}

func TestStartAWSBillingUsesInjectedGuidedRuntime(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}
	var gotOptions workflow.ExecutionOptions
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: request,
		Runner: workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			gotOptions = options
			return cliCapabilityReport(got, workflow.StatusReady, workflow.SupportSupported, "aws_cur2_preflight_ready")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	guide := fakeCLIAWSBillingGuide{
		source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"},
		identity: billingguide.VerifiedIdentity{
			Source:       billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"},
			AccountLabel: "account-ending-9012",
			CallerRef:    "sha256:abcdef123456",
			Region:       "us-east-1",
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithRuntime([]string{"start"}, strings.NewReader("1\n1\ny\n"), &stdout, &stderr, registry, guided.Config{Registry: registry, AWSBilling: guide})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if gotOptions.InterfaceMode != workflow.InterfaceModeGuided {
		t.Fatalf("runner interface mode = %q, want guided", gotOptions.InterfaceMode)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil || gotOptions.Selectors.AWS.Profile != "default" {
		t.Fatalf("runner AWS selectors = %#v, want selected profile", gotOptions)
	}
	for _, want := range []string{
		"Connect AWS account",
		"Inspect AWS CUR 2.0 billing exports",
		"matilda-prep rapid-assessment billing aws preflight --profile default --region us-east-1",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout.String(), want)
		}
	}
	for _, forbidden := range []string{"arn:aws", "123456789012", "access_key", "secret_key", "session_token", "/Users/"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("stdout leaked forbidden value %q in %s", forbidden, stdout.String())
		}
	}
}

func TestStartInvalidSelectionReturnsUsageError(t *testing.T) {
	code, stdout, stderr := runCLIWithInput("nope\n", "start")

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stdout, "What do you want to prepare?") {
		t.Fatalf("stdout = %q, want guided prompt before error", stdout)
	}
	if !strings.Contains(stderr, "invalid selection") {
		t.Fatalf("stderr = %q, want invalid selection message", stderr)
	}
}

func TestStartEOFReturnsUsageErrorWithoutProviderFailure(t *testing.T) {
	code, stdout, stderr := runCLIWithInput("", "start")

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stdout, "Matilda Cloud Prep") {
		t.Fatalf("stdout = %q, want guided intro before cancellation", stdout)
	}
	if !strings.Contains(stderr, "guided setup cancelled") {
		t.Fatalf("stderr = %q, want cancellation message", stderr)
	}
	if strings.Contains(strings.ToLower(stderr), "provider") {
		t.Fatalf("stderr = %q, want cancellation distinct from provider failure", stderr)
	}
}

func TestNoArgumentsReturnUsageError(t *testing.T) {
	code, stdout, stderr := runCLI()

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "usage: expected") {
		t.Fatalf("stderr = %q, want usage guidance", stderr)
	}
}

func TestDirectCommandParserRejectsEmptyInput(t *testing.T) {
	_, _, _, err := parseDirectCommand(nil)
	if err == nil {
		t.Fatal("parseDirectCommand accepted empty input")
	}
	if !strings.Contains(err.Error(), "usage: expected matilda-prep rapid-assessment or deep-discovery command") {
		t.Fatalf("error = %q, want direct command usage", err)
	}
}

func TestStartRejectsUnexpectedArguments(t *testing.T) {
	code, stdout, stderr := runCLI("start", "gcp")

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "usage: matilda-prep start") {
		t.Fatalf("stderr = %q, want start usage", stderr)
	}
}

func TestRapidAssessmentObjectiveFirstAcceptedButFailsClosed(t *testing.T) {
	for _, collectionPath := range []string{"billing", "api"} {
		t.Run(collectionPath, func(t *testing.T) {
			code, stdout, stderr := runCLI("rapid-assessment", collectionPath, "gcp", "preflight")

			if code != 3 {
				t.Fatalf("exit code = %d, want 3; stderr: %s", code, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty for structured fail-closed output", stderr)
			}

			doc := decodeJSON(t, stdout)
			if doc["status"] != "not_implemented" {
				t.Fatalf("status = %v, want not_implemented in %s", doc["status"], stdout)
			}
			assertWorkflowContractFields(t, doc, "not_implemented", "none", "preflight")
			if doc["code"] != "provider_capability_not_implemented" {
				t.Fatalf("code = %v, want provider_capability_not_implemented", doc["code"])
			}
			if doc["mutated"] != false {
				t.Fatalf("mutated = %v, want false", doc["mutated"])
			}
		})
	}
}

func TestAWSBillingPreflightUsesDependencyBlockedRuntimePath(t *testing.T) {
	code, stdout, stderr := runCLIWithRegistry("", "rapid-assessment", "billing", "aws", "preflight")

	if code != 4 {
		t.Fatalf("exit code = %d, want 4; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty for structured blocked output", stderr)
	}

	doc := decodeJSON(t, stdout)
	if doc["status"] != "blocked" {
		t.Fatalf("status = %v, want blocked in %s", doc["status"], stdout)
	}
	if doc["support_status"] != "blocked" {
		t.Fatalf("support_status = %v, want blocked", doc["support_status"])
	}
	if doc["mutation_level"] != "none" {
		t.Fatalf("mutation_level = %v, want none", doc["mutation_level"])
	}
	actionContract, ok := doc["action_contract"].(map[string]any)
	if !ok {
		t.Fatalf("action_contract missing or wrong type: %#v", doc["action_contract"])
	}
	if actionContract["action"] != "preflight" {
		t.Fatalf("action_contract.action = %v, want preflight", actionContract["action"])
	}
	sourceHandles, ok := doc["source_handles"].([]any)
	if !ok || len(sourceHandles) == 0 {
		t.Fatalf("source_handles missing or empty: %#v", doc["source_handles"])
	}
	if doc["code"] != "aws_provider_capability_blocked" {
		t.Fatalf("code = %v, want aws_provider_capability_blocked", doc["code"])
	}
	if doc["mutated"] != false {
		t.Fatalf("mutated = %v, want false", doc["mutated"])
	}
	if doc["provider_capability_implemented"] != true {
		t.Fatalf("provider_capability_implemented = %v, want true", doc["provider_capability_implemented"])
	}
	for _, forbidden := range []string{"/Users/", "arn:aws", "access_key", "secret_key", "session_token", "raw_billing"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("AWS preflight output contains forbidden term %q in %s", forbidden, stdout)
		}
	}
}

func TestAWSBillingPreflightFlagsReachRunnerAndJSON(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}
	var gotOptions workflow.ExecutionOptions
	var sawDeadline bool
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: request,
		Runner: workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			gotOptions = options
			_, sawDeadline = ctx.Deadline()
			return cliCapabilityReport(got, workflow.StatusReady, workflow.SupportSupported, "aws_cur2_preflight_ready")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithRegistry([]string{
		"rapid-assessment", "billing", "aws", "preflight",
		"--profile", "default",
		"--region", "us-west-2",
		"--export-ref", "cur2-abcdefghijklmnop",
		"--timeout", "45s",
	}, strings.NewReader(""), &stdout, &stderr, registry)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !sawDeadline {
		t.Fatal("runner context did not include a deadline from --timeout")
	}
	if gotOptions.TimeoutSeconds != int((45 * time.Second).Seconds()) {
		t.Fatalf("runner timeout = %d, want 45", gotOptions.TimeoutSeconds)
	}
	if gotOptions.InterfaceMode != workflow.InterfaceModeDirect {
		t.Fatalf("runner interface mode = %q, want direct", gotOptions.InterfaceMode)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil {
		t.Fatalf("runner AWS selectors missing: %#v", gotOptions)
	}
	if gotOptions.Selectors.AWS.Profile != "default" ||
		gotOptions.Selectors.AWS.Region != "us-west-2" ||
		gotOptions.Selectors.AWS.CUR2ExportRef != "cur2-abcdefghijklmnop" {
		t.Fatalf("runner AWS selectors = %#v, want supplied selector values", gotOptions.Selectors.AWS)
	}

	doc := decodeJSON(t, stdout.String())
	executionOptions, ok := doc["execution_options"].(map[string]any)
	if !ok {
		t.Fatalf("execution_options missing or wrong type in %s", stdout.String())
	}
	if executionOptions["schema_version"] != "matilda_cloud_prep.execution_options_v0" {
		t.Fatalf("execution_options.schema_version = %v, want v0", executionOptions["schema_version"])
	}
	if executionOptions["interface_mode"] != "direct" {
		t.Fatalf("execution_options.interface_mode = %v, want direct", executionOptions["interface_mode"])
	}
	if executionOptions["timeout_seconds"] != float64(45) {
		t.Fatalf("execution_options.timeout_seconds = %v, want 45", executionOptions["timeout_seconds"])
	}
	selectors, ok := executionOptions["selectors"].(map[string]any)
	if !ok {
		t.Fatalf("execution_options.selectors missing or wrong type: %#v", executionOptions["selectors"])
	}
	awsSelectors, ok := selectors["aws"].(map[string]any)
	if !ok {
		t.Fatalf("execution_options.selectors.aws missing or wrong type: %#v", selectors["aws"])
	}
	if awsSelectors["profile"] != "default" || awsSelectors["region"] != "us-west-2" || awsSelectors["cur2_export_ref"] != "cur2-abcdefghijklmnop" {
		t.Fatalf("execution_options.selectors.aws = %#v, want supplied selector values", awsSelectors)
	}
	for _, forbidden := range []string{"arn:aws", "/Users/", "access_key", "secret_key", "session_token"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("stdout contains forbidden term %q in %s", forbidden, stdout.String())
		}
	}
}

func TestAWSBillingBackfillPlanFlagsReachRunnerAndJSON(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}
	var gotOptions workflow.ExecutionOptions
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: request,
		Runner: workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			gotOptions = options
			return cliCapabilityReport(got, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_backfill_support_case_approval_required")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithRegistry([]string{
		"rapid-assessment", "billing", "aws", "apply-prereqs",
		"--profile", "default",
		"--region", "us-west-2",
		"--export-ref", "cur2-abcdefghijklmnop",
		"--request-backfill",
	}, strings.NewReader(""), &stdout, &stderr, registry)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil {
		t.Fatalf("runner AWS selectors missing: %#v", gotOptions)
	}
	if gotOptions.Selectors.AWS.Profile != "default" ||
		gotOptions.Selectors.AWS.Region != "us-west-2" ||
		gotOptions.Selectors.AWS.CUR2ExportRef != "cur2-abcdefghijklmnop" {
		t.Fatalf("runner AWS selectors = %#v, want supplied selector values", gotOptions.Selectors.AWS)
	}
	if gotOptions.AWSBillingOperation != workflow.AWSBillingOperationRequestBackfill {
		t.Fatalf("AWSBillingOperation = %q, want %q", gotOptions.AWSBillingOperation, workflow.AWSBillingOperationRequestBackfill)
	}
	if len(gotOptions.Approvals) != 0 {
		t.Fatalf("runner approvals length = %d, want none for plan-only backfill", len(gotOptions.Approvals))
	}

	doc := decodeJSON(t, stdout.String())
	executionOptions, ok := doc["execution_options"].(map[string]any)
	if !ok {
		t.Fatalf("execution_options missing or wrong type in %s", stdout.String())
	}
	if executionOptions["aws_billing_operation"] != string(workflow.AWSBillingOperationRequestBackfill) {
		t.Fatalf("execution_options.aws_billing_operation = %v, want request backfill", executionOptions["aws_billing_operation"])
	}
	if _, ok := executionOptions["approvals"]; ok {
		t.Fatalf("execution_options unexpectedly includes approvals: %#v", executionOptions["approvals"])
	}
}

func TestAWSBillingBackfillApprovalFlagsReachRunnerAndJSON(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}
	var gotOptions workflow.ExecutionOptions
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: request,
		Runner: workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			gotOptions = options
			return cliCapabilityReport(got, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_backfill_support_case_approval_received")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithRegistry([]string{
		"rapid-assessment", "billing", "aws", "apply-prereqs",
		"--profile", "default",
		"--region", "us-west-2",
		"--export-ref", "cur2-abcdefghijklmnop",
		"--request-backfill",
		"--confirm-create-support-case",
		"--approve-plan", "plan_abcdefghijklmnop",
		"--approve-step", workflow.AWSBackfillSupportCaseOperationID,
	}, strings.NewReader(""), &stdout, &stderr, registry)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if gotOptions.AWSBillingOperation != workflow.AWSBillingOperationRequestBackfill {
		t.Fatalf("AWSBillingOperation = %q, want %q", gotOptions.AWSBillingOperation, workflow.AWSBillingOperationRequestBackfill)
	}
	if !workflow.HasApprovedPlanStep(gotOptions, "plan_abcdefghijklmnop", workflow.AWSBackfillSupportCaseOperationID) {
		t.Fatalf("runner options missing plan-bound backfill approval: %#v", gotOptions.Approvals)
	}
	if gotOptions.Approvals[0].Intent != workflow.ApprovalIntentRequestBackfillSupportCase {
		t.Fatalf("runner approval intent = %q, want backfill support case intent", gotOptions.Approvals[0].Intent)
	}

	doc := decodeJSON(t, stdout.String())
	executionOptions, ok := doc["execution_options"].(map[string]any)
	if !ok {
		t.Fatalf("execution_options missing or wrong type in %s", stdout.String())
	}
	approvals, ok := executionOptions["approvals"].([]any)
	if !ok || len(approvals) != 1 {
		t.Fatalf("execution_options.approvals = %#v, want one approval", executionOptions["approvals"])
	}
	approval, ok := approvals[0].(map[string]any)
	if !ok {
		t.Fatalf("approval has wrong type: %#v", approvals[0])
	}
	if approval["operation_id"] != workflow.AWSBackfillSupportCaseOperationID ||
		approval["intent"] != workflow.ApprovalIntentRequestBackfillSupportCase ||
		approval["plan_id"] != "plan_abcdefghijklmnop" ||
		approval["confirmed"] != true {
		t.Fatalf("approval = %#v, want plan-bound AWS backfill support case approval", approval)
	}
}

func TestAWSBillingCreateCUR2ExportPlanFlagsReachRunnerAndJSON(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}
	var gotOptions workflow.ExecutionOptions
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: request,
		Runner: workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			gotOptions = options
			return cliCapabilityReport(got, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_cur2_create_export_plan_ready")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithRegistry([]string{
		"rapid-assessment", "billing", "aws", "apply-prereqs",
		"--profile", "default",
		"--region", "us-west-2",
		"--create-cur2-export",
	}, strings.NewReader(""), &stdout, &stderr, registry)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if gotOptions.AWSBillingOperation != workflow.AWSBillingOperationCreateCUR2Export {
		t.Fatalf("AWSBillingOperation = %q, want %q", gotOptions.AWSBillingOperation, workflow.AWSBillingOperationCreateCUR2Export)
	}
	if len(gotOptions.Approvals) != 0 {
		t.Fatalf("approvals length = %d, want none for plan-only create-new", len(gotOptions.Approvals))
	}

	doc := decodeJSON(t, stdout.String())
	executionOptions, ok := doc["execution_options"].(map[string]any)
	if !ok {
		t.Fatalf("execution_options missing or wrong type in %s", stdout.String())
	}
	if executionOptions["aws_billing_operation"] != string(workflow.AWSBillingOperationCreateCUR2Export) {
		t.Fatalf("execution_options.aws_billing_operation = %v, want create CUR 2.0 export", executionOptions["aws_billing_operation"])
	}
	if _, ok := executionOptions["approvals"]; ok {
		t.Fatalf("execution_options unexpectedly includes approvals: %#v", executionOptions["approvals"])
	}
}

func TestAWSBillingCreateCUR2ExportApprovalFlagsReachRunnerAndJSON(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}
	var gotOptions workflow.ExecutionOptions
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: request,
		Runner: workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			gotOptions = options
			return cliCapabilityReport(got, workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_cur2_create_export_approval_received")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithRegistry([]string{
		"rapid-assessment", "billing", "aws", "apply-prereqs",
		"--profile", "default",
		"--region", "us-west-2",
		"--create-cur2-export",
		"--approve-plan", "plan_abcdefghijklmnop",
		"--approve-step", workflow.AWSCUR2CreateBucketOperationID,
		"--approve-step", workflow.AWSCUR2MergeBucketPolicyOperationID,
		"--approve-step", workflow.AWSCUR2CreateExportOperationID,
	}, strings.NewReader(""), &stdout, &stderr, registry)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if gotOptions.AWSBillingOperation != workflow.AWSBillingOperationCreateCUR2Export {
		t.Fatalf("AWSBillingOperation = %q, want %q", gotOptions.AWSBillingOperation, workflow.AWSBillingOperationCreateCUR2Export)
	}
	if len(gotOptions.Approvals) != 3 {
		t.Fatalf("approvals length = %d, want 3", len(gotOptions.Approvals))
	}
	for _, step := range []string{
		workflow.AWSCUR2CreateBucketOperationID,
		workflow.AWSCUR2MergeBucketPolicyOperationID,
		workflow.AWSCUR2CreateExportOperationID,
	} {
		if !workflow.HasApprovedPlanStep(gotOptions, "plan_abcdefghijklmnop", step) {
			t.Fatalf("missing approved step %q in %#v", step, gotOptions.Approvals)
		}
	}

	doc := decodeJSON(t, stdout.String())
	executionOptions, ok := doc["execution_options"].(map[string]any)
	if !ok {
		t.Fatalf("execution_options missing or wrong type in %s", stdout.String())
	}
	approvals, ok := executionOptions["approvals"].([]any)
	if !ok || len(approvals) != 3 {
		t.Fatalf("execution_options.approvals = %#v, want 3 approvals", executionOptions["approvals"])
	}
	for _, approvalAny := range approvals {
		approval, ok := approvalAny.(map[string]any)
		if !ok {
			t.Fatalf("approval wrong type: %#v", approvalAny)
		}
		if approval["plan_id"] != "plan_abcdefghijklmnop" || approval["confirmed"] != true {
			t.Fatalf("approval = %#v, want approved plan step", approval)
		}
	}
}

func TestAWSSelectorFlagsAreScopedToAWSBillingPreflightAndApplyPrereqs(t *testing.T) {
	tests := [][]string{
		{"rapid-assessment", "api", "aws", "preflight", "--profile", "default"},
		{"rapid-assessment", "billing", "gcp", "preflight", "--region", "us-west-2"},
		{"deep-discovery", "aws", "preflight", "--export-ref", "cur2-abcdefghijklmnop"},
		{"rapid-assessment", "billing", "aws", "validate", "--profile", "default"},
		{"rapid-assessment", "billing", "aws", "package", "--profile", "default"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, stdout, stderr := runCLI(args...)

			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "AWS selector flags are supported only for matilda-prep rapid-assessment billing aws preflight or apply-prereqs") {
				t.Fatalf("stderr = %q, want AWS selector scope message", stderr)
			}
		})
	}
}

func TestAWSBackfillOperationFlagsAreScopedToAWSBillingApplyPrereqs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "operation rejected on preflight",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--request-backfill", "--confirm-create-support-case"},
			want: "AWS backfill operation flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs",
		},
		{
			name: "operation rejected on non aws provider",
			args: []string{"rapid-assessment", "billing", "gcp", "apply-prereqs", "--request-backfill", "--confirm-create-support-case"},
			want: "AWS backfill operation flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs",
		},
		{
			name: "confirm without request flag",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--confirm-create-support-case"},
			want: "AWS backfill support case confirmation requires --request-backfill",
		},
		{
			name: "confirm without plan-bound approval",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--request-backfill", "--confirm-create-support-case"},
			want: "AWS backfill support case approval requires --confirm-create-support-case, --approve-plan, and at least one --approve-step",
		},
		{
			name: "approve without confirm",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--request-backfill", "--approve-plan", "plan_abcdefghijklmnop", "--approve-step", workflow.AWSBackfillSupportCaseOperationID},
			want: "AWS backfill support case approval requires --confirm-create-support-case, --approve-plan, and at least one --approve-step",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(tt.args...)

			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}
}

func TestAWSBillingCreateCUR2ExportFlagsAreScopedAndConflictChecked(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "create cur2 rejected on preflight",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--create-cur2-export"},
			want: "AWS billing operation flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs",
		},
		{
			name: "create cur2 rejected on non aws provider",
			args: []string{"rapid-assessment", "billing", "gcp", "apply-prereqs", "--create-cur2-export"},
			want: "AWS billing operation flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs",
		},
		{
			name: "approve plan without create intent",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--approve-plan", "plan_abcdefghijklmnop"},
			want: "approval flags require a matching AWS billing operation",
		},
		{
			name: "approve step without approve plan",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--create-cur2-export", "--approve-step", workflow.AWSCUR2CreateExportOperationID},
			want: "plan-bound approval requires both --approve-plan and at least one --approve-step",
		},
		{
			name: "approve plan without approve step",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--create-cur2-export", "--approve-plan", "plan_abcdefghijklmnop"},
			want: "plan-bound approval requires both --approve-plan and at least one --approve-step",
		},
		{
			name: "create conflicts with backfill",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--create-cur2-export", "--request-backfill", "--confirm-create-support-case"},
			want: "aws_billing_prereqs_operation_conflict",
		},
		{
			name: "create cur2 does not accept selected export ref",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--create-cur2-export", "--export-ref", "cur2-abcdefghijklmnop"},
			want: "export-ref applies only to AWS CUR 2.0 preflight and request-backfill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(tt.args...)

			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}
}

func TestDirectFlagValidationFailsBeforeExecution(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "unknown flag",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--export-name", "report"},
			want: "flag provided but not defined",
		},
		{
			name: "flag needs argument",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--profile"},
			want: "flag needs an argument",
		},
		{
			name: "unsafe trailing argument",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "arn:aws:bcm-data-exports:us-east-1:123456789012:export/live"},
			want: "unexpected argument after command",
		},
		{
			name: "unsafe timeout value",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--timeout", "/private/tmp/timeout"},
			want: "invalid timeout value",
		},
		{
			name: "invalid boolean flag",
			args: []string{"rapid-assessment", "billing", "aws", "apply-prereqs", "--request-backfill=maybe", "--confirm-create-support-case"},
			want: "invalid command flags",
		},
		{
			name: "short timeout",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--timeout", "5s"},
			want: "timeout must be between 10s and 30m",
		},
		{
			name: "long timeout",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--timeout", "31m"},
			want: "timeout must be between 10s and 30m",
		},
		{
			name: "fractional timeout",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--timeout", "10.5s"},
			want: "timeout must use whole seconds",
		},
		{
			name: "empty profile",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--profile", ""},
			want: "profile cannot be empty",
		},
		{
			name: "unsafe export ref",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--export-ref", "arn:aws:bcm-data-exports:us-east-1:123456789012:export/live"},
			want: "cur2_export_ref",
		},
		{
			name: "path-like profile",
			args: []string{"rapid-assessment", "billing", "aws", "preflight", "--profile", "/private/tmp/aws-profile"},
			want: "profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(tt.args...)

			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr, tt.want)
			}
			if strings.Contains(stderr, "arn:aws") {
				t.Fatalf("stderr leaked raw ARN: %s", stderr)
			}
			if strings.Contains(stderr, "/private/tmp") {
				t.Fatalf("stderr leaked local path: %s", stderr)
			}
		})
	}
}

func TestDeepDiscoveryObjectiveFirstAcceptedButFailsClosed(t *testing.T) {
	code, stdout, stderr := runCLI("deep-discovery", "gcp", "preflight")

	if code != 3 {
		t.Fatalf("exit code = %d, want 3; stderr: %s", code, stderr)
	}
	doc := decodeJSON(t, stdout)
	if doc["status"] != "not_implemented" {
		t.Fatalf("status = %v, want not_implemented", doc["status"])
	}
	assertWorkflowContractFields(t, doc, "not_implemented", "none", "preflight")
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestPreflightJSONIncludesExecutionPlan(t *testing.T) {
	code, stdout, stderr := runCLI("rapid-assessment", "api", "gcp", "preflight")

	if code != 3 {
		t.Fatalf("exit code = %d, want 3; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	doc := decodeJSON(t, stdout)
	plan, ok := doc["plan"].(map[string]any)
	if !ok {
		t.Fatalf("plan missing or wrong type in %s", stdout)
	}
	if plan["schema_version"] != "matilda_cloud_prep.execution_plan_v0" {
		t.Fatalf("plan schema_version = %v, want execution plan v0", plan["schema_version"])
	}
	if plan["plan_id"] == "" {
		t.Fatalf("plan_id is empty in %s", stdout)
	}
	if plan["package_schema_status"] != "provider_schema_required" {
		t.Fatalf("package_schema_status = %v, want provider_schema_required", plan["package_schema_status"])
	}

	coverage, ok := plan["coverage_recommendation"].(map[string]any)
	if !ok {
		t.Fatalf("coverage_recommendation missing or wrong type in %s", stdout)
	}
	if coverage["coverage_status"] != "unknown" {
		t.Fatalf("coverage_status = %v, want unknown", coverage["coverage_status"])
	}

	approval, ok := plan["approval"].(map[string]any)
	if !ok {
		t.Fatalf("approval missing or wrong type in %s", stdout)
	}
	if approval["approved"] != false {
		t.Fatalf("approval.approved = %v, want false", approval["approved"])
	}
	if approval["blocked"] != true {
		t.Fatalf("approval.blocked = %v, want true", approval["blocked"])
	}

	steps, ok := plan["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("steps missing or wrong length in %s", stdout)
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("step missing or wrong type in %s", stdout)
	}
	if step["intent"] != "blocked" {
		t.Fatalf("step intent = %v, want blocked", step["intent"])
	}
	if step["requires_approval"] != false {
		t.Fatalf("step requires_approval = %v, want false", step["requires_approval"])
	}
	if step["credential_material_touched"] != false {
		t.Fatalf("step credential_material_touched = %v, want false", step["credential_material_touched"])
	}
	for _, required := range []string{"current_state", "target_state", "required_permission", "validation", "rollback"} {
		if step[required] == "" {
			t.Fatalf("step field %s is empty in %s", required, stdout)
		}
	}

	statusCounts, ok := plan["status_counts"].(map[string]any)
	if !ok {
		t.Fatalf("status_counts missing or wrong type in %s", stdout)
	}
	stepCounts, ok := statusCounts["step_intents"].(map[string]any)
	if !ok || stepCounts["blocked"] != float64(1) {
		t.Fatalf("step_intents counts = %#v, want blocked=1", statusCounts["step_intents"])
	}
	checkCounts, ok := statusCounts["check_statuses"].(map[string]any)
	if !ok || checkCounts["fail"] != float64(1) {
		t.Fatalf("check_statuses counts = %#v, want fail=1", statusCounts["check_statuses"])
	}

	for _, forbidden := range []string{"/Users/", "private_key", "client_secret", "plain-token", "Bearer"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("preflight output contains forbidden term %q in %s", forbidden, stdout)
		}
	}
}

func TestPackageProducesMinimalManifest(t *testing.T) {
	code, stdout, stderr := runCLI("rapid-assessment", "billing", "aws", "package")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	doc := decodeJSON(t, stdout)
	if doc["status"] != "ready" {
		t.Fatalf("status = %v, want ready", doc["status"])
	}
	assertWorkflowContractFields(t, doc, "guided", "local_only", "package")

	manifest, ok := doc["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("manifest missing or wrong type in %s", stdout)
	}
	if manifest["schema_version"] != "minimal_v0" {
		t.Fatalf("manifest schema_version = %v, want minimal_v0", manifest["schema_version"])
	}
	if strings.Contains(strings.ToLower(stdout), "private_key") ||
		strings.Contains(strings.ToLower(stdout), "token") ||
		strings.Contains(strings.ToLower(stdout), "raw_billing") {
		t.Fatalf("package output contains forbidden sensitive/raw fields: %s", stdout)
	}
}

func TestPackageReportsStructuredOutputWriteFailure(t *testing.T) {
	var stderr bytes.Buffer
	code := Run([]string{"rapid-assessment", "billing", "aws", "package"}, failingWriter{}, &stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "write failed") {
		t.Fatalf("stderr = %q, want write failure", stderr.String())
	}
}

func TestDeepDiscoveryPackageOmitsRapidCollectionPath(t *testing.T) {
	code, stdout, stderr := runCLI("deep-discovery", "oci", "package")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if strings.Contains(stdout, "collection_path") {
		t.Fatalf("Deep Discovery package output contains Rapid-only collection_path: %s", stdout)
	}

	doc := decodeJSON(t, stdout)
	request, ok := doc["request"].(map[string]any)
	if !ok {
		t.Fatalf("request missing or wrong type in %s", stdout)
	}
	if request["goal"] != "deep-discovery" {
		t.Fatalf("request goal = %v, want deep-discovery", request["goal"])
	}
	if request["provider"] != "oci" {
		t.Fatalf("request provider = %v, want oci", request["provider"])
	}
}

func TestProviderFirstCommandRejectedWithCorrection(t *testing.T) {
	code, stdout, stderr := runCLI("gcp", "rapid-assessment", "billing", "preflight")

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	for _, want := range []string{
		"Provider-first command order is not supported",
		"matilda-prep start",
		"matilda-prep rapid-assessment billing gcp preflight",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, want to contain %q", stderr, want)
		}
	}
}

func TestInvalidInputsReturnUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "goal", args: []string{"migration"}, want: "invalid goal"},
		{name: "unsafe goal", args: []string{"arn:aws:iam::123456789012:role/operator"}, want: "invalid goal"},
		{name: "collection path", args: []string{"rapid-assessment", "full", "gcp", "preflight"}, want: "invalid collection path"},
		{name: "unsafe collection path", args: []string{"rapid-assessment", "/private/tmp/full", "gcp", "preflight"}, want: "invalid collection path"},
		{name: "provider", args: []string{"rapid-assessment", "billing", "digitalocean", "preflight"}, want: "invalid provider"},
		{name: "unsafe provider", args: []string{"rapid-assessment", "billing", "arn:aws:iam::123456789012:role/operator", "preflight"}, want: "invalid provider"},
		{name: "action", args: []string{"rapid-assessment", "billing", "gcp", "destroy"}, want: "invalid action"},
		{name: "unsafe action", args: []string{"rapid-assessment", "billing", "gcp", "/private/tmp/destroy"}, want: "invalid action"},
		{name: "rapid assessment arity", args: []string{"rapid-assessment", "billing", "gcp"}, want: "usage"},
		{name: "deep discovery provider", args: []string{"deep-discovery", "digitalocean", "preflight"}, want: "invalid provider"},
		{name: "deep discovery action", args: []string{"deep-discovery", "gcp", "destroy"}, want: "invalid action"},
		{name: "deep discovery arity", args: []string{"deep-discovery", "gcp"}, want: "usage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(tt.args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Fatalf("stderr = %q, want to contain %q", stderr, tt.want)
			}
			for _, forbidden := range []string{"arn:aws", "/private/tmp", "/Users/"} {
				if strings.Contains(stderr, forbidden) {
					t.Fatalf("stderr leaked unsafe value %q: %s", forbidden, stderr)
				}
			}
		})
	}
}

func TestTrailingActionHelpRequiresValidCommandContext(t *testing.T) {
	code, stdout, stderr := runCLI("rapid-assessment", "billing", "digitalocean", "preflight", "--help")

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "invalid provider") {
		t.Fatalf("stderr = %q, want invalid provider", stderr)
	}
	if strings.Contains(stderr, "Cloud mutation") {
		t.Fatalf("invalid command rendered action help instead of usage error: %s", stderr)
	}
}

func TestHelpAndVersionAreDeterministicAndPublicSafe(t *testing.T) {
	code, stdout, stderr := runCLI("--help")
	if code != 0 {
		t.Fatalf("help exit code = %d, want 0; stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"matilda-prep start",
		"matilda-prep rapid-assessment <billing|api> <provider> <action>",
		"matilda-prep deep-discovery <provider> <action>",
		"AWS Rapid Assessment - Billing Based preflight/backfill selection options:",
		"--export-ref <cur2-ref-from-previous-output>",
		"--request-backfill       plan an AWS Support request for previous-month CUR 2.0 backfill",
		"--request-backfill --confirm-create-support-case --approve-plan <plan-id> --approve-step aws.billing.cur2.previous_month_backfill_support_case",
		"--create-cur2-export --approve-plan <plan-id> --approve-step <plan-step-id>",
		"repeat --approve-step for each mutating step ID returned by the current plan",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help stdout = %q, want to contain %q", stdout, want)
		}
	}
	for _, forbidden := range []string{"/Users/", "docs/references", "client_secret", "token"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("help output contains forbidden term %q in %q", forbidden, stdout)
		}
	}
	if strings.Contains(stdout, "AWS Rapid Assessment - Billing Based preflight/apply-prereqs options:") {
		t.Fatalf("help output describes export-ref too broadly: %s", stdout)
	}

	code, stdout, stderr = runCLI("--version")
	if code != 0 {
		t.Fatalf("version exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stdout != "matilda-prep dev\n" {
		t.Fatalf("version stdout = %q, want matilda-prep dev newline", stdout)
	}
}

func TestHelpAliasesAreAccepted(t *testing.T) {
	for _, arg := range []string{"-h", "help"} {
		t.Run(arg, func(t *testing.T) {
			code, stdout, stderr := runCLI(arg)

			if code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			if !strings.Contains(stdout, "Usage:") || !strings.Contains(stdout, "matilda-prep start") {
				t.Fatalf("stdout = %q, want help text", stdout)
			}
		})
	}
}

func TestActionHelpDoesNotExecuteWorkflow(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantCommand string
		wantPurpose string
	}{
		{
			name:        "rapid assessment preflight help",
			args:        []string{"rapid-assessment", "billing", "gcp", "preflight", "--help"},
			wantCommand: "matilda-prep rapid-assessment billing gcp preflight",
			wantPurpose: "Checks readiness before setup.",
		},
		{
			name:        "rapid assessment apply help",
			args:        []string{"rapid-assessment", "billing", "gcp", "apply-prereqs", "--help"},
			wantCommand: "matilda-prep rapid-assessment billing gcp apply-prereqs",
			wantPurpose: "only after explicit approval",
		},
		{
			name:        "rapid assessment validate help",
			args:        []string{"rapid-assessment", "billing", "gcp", "validate", "--help"},
			wantCommand: "matilda-prep rapid-assessment billing gcp validate",
			wantPurpose: "Verifies configured prerequisites after setup.",
		},
		{
			name:        "rapid assessment package help",
			args:        []string{"rapid-assessment", "billing", "gcp", "package", "--help"},
			wantCommand: "matilda-prep rapid-assessment billing gcp package",
			wantPurpose: "Builds a whitelisted handoff artifact.",
		},
		{
			name:        "deep discovery action help",
			args:        []string{"deep-discovery", "gcp", "preflight", "--help"},
			wantCommand: "matilda-prep deep-discovery gcp preflight",
			wantPurpose: "Checks readiness before setup.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			for _, want := range []string{tt.wantCommand, tt.wantPurpose, "Cloud mutation", "matilda-prep start"} {
				if !strings.Contains(stdout, want) {
					t.Fatalf("stdout = %q, want to contain %q", stdout, want)
				}
			}
			if strings.Contains(stdout, "not_implemented") || strings.Contains(stdout, "provider_capability_not_implemented") {
				t.Fatalf("action help executed workflow instead of rendering help: %s", stdout)
			}
		})
	}
}

func TestAWSBillingApplyPrereqsActionHelpIncludesOperationAndApprovalFlags(t *testing.T) {
	code, stdout, stderr := runCLI("rapid-assessment", "billing", "aws", "apply-prereqs", "--help")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{
		"matilda-prep rapid-assessment billing aws apply-prereqs",
		"--request-backfill",
		"--create-cur2-export",
		"--approve-plan <plan-id>",
		"--approve-step <plan-step-id>",
		"aws.billing.cur2.previous_month_backfill_support_case",
		"repeat --approve-step for each mutating step ID returned by the current plan",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout, want)
		}
	}
}

func TestAWSBillingPreflightActionHelpIncludesSelectorFlags(t *testing.T) {
	code, stdout, stderr := runCLI("rapid-assessment", "billing", "aws", "preflight", "--help")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{
		"matilda-prep rapid-assessment billing aws preflight",
		"AWS Rapid Assessment - Billing Based preflight/backfill selection options:",
		"--profile <aws-profile-name>",
		"--region <aws-region>",
		"--export-ref <cur2-ref-from-previous-output>",
		"--timeout <duration>",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "not_implemented") || strings.Contains(stdout, "provider_capability_not_implemented") {
		t.Fatalf("action help executed workflow instead of rendering help: %s", stdout)
	}
}

func TestGenericActionHelpOmitsAWSBillingSelectorFlags(t *testing.T) {
	code, stdout, stderr := runCLI("rapid-assessment", "billing", "gcp", "preflight", "--help")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, forbidden := range []string{
		"AWS Rapid Assessment - Billing Based preflight/backfill selection options:",
		"--profile <aws-profile-name>",
		"--region <aws-region>",
		"--export-ref <cur2-ref-from-previous-output>",
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("generic preflight help contains AWS-only text %q in %q", forbidden, stdout)
		}
	}
}

func TestActionHelpFailsClosedForUnknownContracts(t *testing.T) {
	if got := actionPurpose(assessment.Action("unknown-action")); !strings.Contains(got, "Cloud mutation: unknown") || !strings.Contains(got, "Unsupported actions fail closed") {
		t.Fatalf("actionPurpose unknown = %q, want fail-closed message", got)
	}
	if got := mutationHelp(workflow.MutationLevel("future-mutation")); got != "unknown" {
		t.Fatalf("mutationHelp unknown = %q, want unknown", got)
	}
}

func TestUsageErrorsDoNotEchoSecretLikeInput(t *testing.T) {
	secretArgs := []string{
		"client_secret=plain-secret",
		"private_key_id=plain-private-key-id",
		"service_account_key=plain-service-account-key",
		"service-account-key=plain-service-account-key-dashed",
		"serviceaccount_key=plain-serviceaccount-key",
		"sa-key=plain-sa-key",
		"secret_key=plain-secret-key",
		"key_content=plain-key-content",
		"key_phrase=plain-key-phrase",
		"session_token=plain-session-token",
		"cookie=session=plain-cookie",
		"authorization=Bearer plain-authorization",
		"header=Bearer plain-header-secret",
	}

	for _, arg := range secretArgs {
		t.Run(arg, func(t *testing.T) {
			code, stdout, stderr := runCLI(arg)

			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if strings.Contains(stderr, "plain-") {
				t.Fatalf("stderr leaked secret-like value: %q", stderr)
			}
			if strings.Contains(stderr, arg) {
				t.Fatalf("stderr echoed invalid secret-like input: %q", stderr)
			}
			if !strings.Contains(stderr, "expected rapid-assessment or deep-discovery") {
				t.Fatalf("stderr = %q, want surrounding error text preserved", stderr)
			}
		})
	}
}

func decodeJSON(t *testing.T, input string) map[string]any {
	t.Helper()

	var doc map[string]any
	if err := json.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, input)
	}
	return doc
}

func assertWorkflowContractFields(t *testing.T, doc map[string]any, supportStatus, mutationLevel, action string) {
	t.Helper()

	if doc["support_status"] != supportStatus {
		t.Fatalf("support_status = %v, want %s", doc["support_status"], supportStatus)
	}
	if doc["mutation_level"] != mutationLevel {
		t.Fatalf("mutation_level = %v, want %s", doc["mutation_level"], mutationLevel)
	}

	actionContract, ok := doc["action_contract"].(map[string]any)
	if !ok {
		t.Fatalf("action_contract missing or wrong type: %#v", doc["action_contract"])
	}
	if actionContract["action"] != action {
		t.Fatalf("action_contract.action = %v, want %s", actionContract["action"], action)
	}
	if actionContract["mutation_level"] != mutationLevel {
		t.Fatalf("action_contract.mutation_level = %v, want %s", actionContract["mutation_level"], mutationLevel)
	}
	if actionContract["purpose"] == "" {
		t.Fatal("action_contract.purpose is empty")
	}
	if actionContract["required_result"] == "" {
		t.Fatal("action_contract.required_result is empty")
	}

	sourceHandles, ok := doc["source_handles"].([]any)
	if !ok || len(sourceHandles) == 0 {
		t.Fatalf("source_handles missing or empty: %#v", doc["source_handles"])
	}
	missingSource, ok := doc["missing_source_of_truth"].([]any)
	if !ok || len(missingSource) == 0 {
		t.Fatalf("missing_source_of_truth missing or empty: %#v", doc["missing_source_of_truth"])
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type fakeCLIAWSBillingGuide struct {
	source   billingguide.CredentialSource
	identity billingguide.VerifiedIdentity
}

func (fake fakeCLIAWSBillingGuide) DiscoverCredentialSources(context.Context) ([]billingguide.CredentialSource, error) {
	return []billingguide.CredentialSource{fake.source}, nil
}

func (fake fakeCLIAWSBillingGuide) VerifyIdentity(context.Context, billingguide.CredentialSource) (billingguide.VerifiedIdentity, error) {
	return fake.identity, nil
}

func cliCapabilityReport(request workflow.Request, status workflow.RunStatus, support workflow.SupportStatus, code string) workflow.CapabilityReport {
	handles := []workflow.SourceHandle{{
		Label: "AWS CUR 2.0 Preflight Source Bundle",
		URI:   "docs/references/aws/aws-rapid-assessment-billing-cur2-preflight-source-bundle.md",
	}}
	return workflow.CapabilityReport{
		Status:        status,
		SupportStatus: support,
		Code:          code,
		Message:       "AWS CUR 2.0 billing preflight completed.",
		Mutated:       false,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified with account ending 9012 and caller hash sha256:123456789abc.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageUnknown,
				Summary:        "AWS billing coverage is evaluated from the selected CUR 2.0 export.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{{
				Intent:                    workflow.PlanStepReuse,
				Title:                     "Review existing AWS CUR 2.0 export",
				Description:               "Use read-only checks to evaluate an existing AWS Data Exports CUR 2.0 export.",
				Reason:                    "Rapid Assessment - Billing Based requires exported AWS billing data.",
				ApprovalKind:              "not_required",
				CurrentState:              "Existing AWS export metadata is visible through read-only checks.",
				TargetState:               "Existing AWS export satisfies the CUR 2.0 billing readiness rules.",
				RequiredPermission:        "bcm-data-exports:ListExports",
				CredentialMaterialTouched: false,
				Validation:                "Read-only preflight checks produced pass signals for the selected export.",
				Rollback:                  "No cloud change is made by preflight.",
				SourceHandles:             handles,
			}},
			Checks: []workflow.PlanCheck{{
				Status:  workflow.CheckPass,
				Title:   "AWS CUR 2.0 preflight capability",
				Message: "Injected AWS CUR 2.0 preflight capability returned a safe result.",
				Evidence: []workflow.PlanEvidence{
					{Key: "mutated", Value: "false"},
					{Key: "code", Value: code},
				},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}
