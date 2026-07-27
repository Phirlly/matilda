package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
		"Provider-specific cloud automation is not implemented yet",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout, want)
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
		{name: "collection path", args: []string{"rapid-assessment", "full", "gcp", "preflight"}, want: "invalid collection path"},
		{name: "provider", args: []string{"rapid-assessment", "billing", "digitalocean", "preflight"}, want: "invalid provider"},
		{name: "action", args: []string{"rapid-assessment", "billing", "gcp", "destroy"}, want: "invalid action"},
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

func TestUsageErrorsAreRedacted(t *testing.T) {
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
			if !strings.Contains(stderr, "[REDACTED]") {
				t.Fatalf("stderr = %q, want redaction marker", stderr)
			}
			if !strings.Contains(stderr, "expected rapid-assessment or deep-discovery") {
				t.Fatalf("stderr = %q, want surrounding error text preserved", stderr)
			}
			if !strings.Contains(stderr, "\": expected rapid-assessment or deep-discovery") {
				t.Fatalf("stderr = %q, want redaction to preserve closing quote and colon", stderr)
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
