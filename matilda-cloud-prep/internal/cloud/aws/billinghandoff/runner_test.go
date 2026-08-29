package billinghandoff

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestRunnerBuildsStructuredHandoffFromReadyPreflightEvidence(t *testing.T) {
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{
		report: readyPreflightReport("aws_cur2_preflight_ready"),
	}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want ready", report.Status)
	}
	if report.Code != "aws_cur2_package_handoff_ready" {
		t.Fatalf("Code = %q, want aws_cur2_package_handoff_ready", report.Code)
	}
	if report.Mutated {
		t.Fatal("package handoff must not mutate AWS resources")
	}
	if report.Manifest != nil {
		t.Fatalf("Manifest = %#v, want nil for stdout handoff", report.Manifest)
	}
	if report.Handoff == nil {
		t.Fatal("Handoff is nil")
	}
	if report.Handoff.HandoffType != "aws_rapid_assessment_billing_cur2" {
		t.Fatalf("HandoffType = %q, want aws_rapid_assessment_billing_cur2", report.Handoff.HandoffType)
	}
	assertHandoffField(t, report, "selected_export_ref", "cur2-abcdefghijklmnop")
	assertHandoffField(t, report, "billing_source", "CUR2.0")
	assertHandoffField(t, report, "readiness_status", "ready")
	assertHandoffField(t, report, "aws_delivery_status", "ready")
	assertHandoffField(t, report, "s3_delivery_policy_readiness", "ready")
	assertHandoffField(t, report, "s3_bucket", "matilda-cur2-billing")
	assertHandoffField(t, report, "s3_prefix", "matilda/cur2")
	assertHandoffField(t, report, "s3_region", "us-east-1")
	assertHandoffField(t, report, "cur2_data_prefix", "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/")
	assertHandoffField(t, report, "cur2_manifest_prefix", "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/")
	assertNextStepContains(t, report, "Skip Configuration")
	assertNextStepContains(t, report, "Billing Based assessment")
	assertNextStepContains(t, report, "alternate large-file utility")

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal(report) returned error: %v", err)
	}
	for _, forbidden := range []string{
		"arn:aws",
		"123456789012",
		"AKIA",
		"secret_key",
		"session_token",
		"/Users/",
		"part-000",
		"Manifest.json",
		"raw_billing",
		"customer_name",
		"org_name",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("handoff report leaked %q in %s", forbidden, encoded)
		}
	}
}

func TestRunnerRequiresSelectedExportRef(t *testing.T) {
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{
		report: readyPreflightReport("aws_cur2_preflight_ready"),
	}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), workflow.ExecutionOptions{})

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_cur2_package_export_ref_required" {
		t.Fatalf("Code = %q, want aws_cur2_package_export_ref_required", report.Code)
	}
	if report.Handoff != nil {
		t.Fatalf("Handoff = %#v, want nil", report.Handoff)
	}
}

func TestRunnerFailsClosedWhenPreflightRunnerUnavailable(t *testing.T) {
	runner := NewRunner(RunnerConfig{})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_provider_capability_blocked" {
		t.Fatalf("Code = %q, want aws_provider_capability_blocked", report.Code)
	}
	if report.Handoff != nil {
		t.Fatalf("Handoff = %#v, want nil", report.Handoff)
	}
}

func TestRunnerFailsClosedWhenPreflightPlanIsMissing(t *testing.T) {
	preflight := readyPreflightReport("aws_cur2_preflight_ready")
	preflight.PlanInput = nil
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_cur2_package_preflight_not_ready" {
		t.Fatalf("Code = %q, want aws_cur2_package_preflight_not_ready", report.Code)
	}
	if report.Handoff != nil {
		t.Fatalf("Handoff = %#v, want nil", report.Handoff)
	}
}

func TestRunnerFailsClosedWhenPreflightChecksAreMissing(t *testing.T) {
	preflight := readyPreflightReport("aws_cur2_preflight_ready")
	preflight.PlanInput.Checks = nil
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_cur2_package_preflight_not_ready" {
		t.Fatalf("Code = %q, want aws_cur2_package_preflight_not_ready", report.Code)
	}
	if report.Handoff != nil {
		t.Fatalf("Handoff = %#v, want nil", report.Handoff)
	}
}

func TestRunnerFailsClosedForDisallowedPreflightCheckEvenWhenTopLevelCodeIsAllowed(t *testing.T) {
	preflight := readyPreflightReport("aws_cur2_time_granularity_not_preferred")
	preflight.PlanInput.Checks = append(preflight.PlanInput.Checks,
		planCheck(workflow.CheckWarn, "aws_cur2_delivery_not_started"),
	)
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_cur2_package_preflight_not_ready" {
		t.Fatalf("Code = %q, want aws_cur2_package_preflight_not_ready", report.Code)
	}
	if report.Handoff != nil {
		t.Fatalf("Handoff = %#v, want nil", report.Handoff)
	}
}

func TestRunnerFailsClosedForUnknownOrSkippedPreflightChecks(t *testing.T) {
	for _, status := range []workflow.CheckStatus{workflow.CheckUnknown, workflow.CheckSkipped} {
		t.Run(string(status), func(t *testing.T) {
			preflight := readyPreflightReport("aws_cur2_preflight_ready")
			preflight.PlanInput.Checks = append(preflight.PlanInput.Checks,
				planCheck(status, "aws_cur2_handoff_unknown"),
			)
			runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

			report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

			if report.Status != workflow.RunStatusBlocked {
				t.Fatalf("Status = %q, want blocked", report.Status)
			}
			if report.Code != "aws_cur2_package_preflight_not_ready" {
				t.Fatalf("Code = %q, want aws_cur2_package_preflight_not_ready", report.Code)
			}
		})
	}
}

func TestRunnerFailsClosedForUnclassifiedPreflightWarning(t *testing.T) {
	for _, tt := range []struct {
		name  string
		check workflow.PlanCheck
		want  string
	}{
		{
			name:  "unknown warning",
			check: planCheck(workflow.CheckWarn, "aws_cur2_future_warning"),
			want:  "aws_cur2_package_warning_unclassified",
		},
		{
			name:  "blank warning",
			check: planCheck(workflow.CheckWarn, ""),
			want:  "aws_cur2_package_warning_unclassified",
		},
		{
			name: "missing warning code evidence",
			check: workflow.PlanCheck{
				Status:        workflow.CheckWarn,
				Title:         "future warning",
				Message:       "future warning message",
				SourceHandles: sourceHandles(),
			},
			want: "aws_cur2_package_warning_unclassified",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			preflight := readyPreflightReport("aws_cur2_preflight_ready")
			preflight.PlanInput.Checks = append(preflight.PlanInput.Checks,
				tt.check,
			)
			runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

			report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

			if report.Status != workflow.RunStatusBlocked {
				t.Fatalf("Status = %q, want blocked", report.Status)
			}
			if report.Code != tt.want {
				t.Fatalf("Code = %q, want %q", report.Code, tt.want)
			}
			if report.Handoff != nil {
				t.Fatalf("Handoff = %#v, want nil", report.Handoff)
			}
		})
	}
}

func TestRunnerFailsClosedForManualOrBlockedPreflight(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status workflow.RunStatus
		code   string
	}{
		{name: "manual", status: workflow.RunStatusManualSteps, code: "aws_backfill_manual_step_required"},
		{name: "blocked", status: workflow.RunStatusBlocked, code: "aws_s3_bucket_inaccessible"},
		{name: "failed", status: workflow.RunStatusFailed, code: "aws_data_exports_transient"},
		{name: "not implemented", status: workflow.RunStatusNotImplemented, code: "provider_capability_not_implemented"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			preflight := readyPreflightReport("aws_cur2_preflight_ready")
			preflight.Status = tt.status
			preflight.Code = tt.code
			runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

			report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

			if report.Status != workflow.RunStatusBlocked {
				t.Fatalf("Status = %q, want blocked", report.Status)
			}
			if report.Handoff != nil {
				t.Fatalf("Handoff = %#v, want nil", report.Handoff)
			}
		})
	}
}

func TestRunnerPreservesProviderCapabilityBlockedFromPreflight(t *testing.T) {
	preflight := readyPreflightReport("aws_cur2_preflight_ready")
	preflight.Status = workflow.RunStatusBlocked
	preflight.Code = "aws_provider_capability_blocked"
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_provider_capability_blocked" {
		t.Fatalf("Code = %q, want aws_provider_capability_blocked", report.Code)
	}
	if report.Handoff != nil {
		t.Fatalf("Handoff = %#v, want nil", report.Handoff)
	}
}

func TestRunnerFailsClosedWhenSafeHandoffEvidenceIsMissing(t *testing.T) {
	preflight := readyPreflightReport("aws_cur2_preflight_ready")
	for checkIndex := range preflight.PlanInput.Checks {
		for evidenceIndex := range preflight.PlanInput.Checks[checkIndex].Evidence {
			if preflight.PlanInput.Checks[checkIndex].Evidence[evidenceIndex].Key == "s3_bucket" {
				preflight.PlanInput.Checks[checkIndex].Evidence[evidenceIndex].Value = "arn:aws:s3:::unsafe"
			}
		}
	}
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_cur2_package_handoff_evidence_incomplete" {
		t.Fatalf("Code = %q, want aws_cur2_package_handoff_evidence_incomplete", report.Code)
	}
	if report.Handoff != nil {
		t.Fatalf("Handoff = %#v, want nil", report.Handoff)
	}
}

func TestRunnerFailsClosedWhenSelectedExportEvidenceDoesNotMatch(t *testing.T) {
	preflight := readyPreflightReport("aws_cur2_preflight_ready")
	for checkIndex := range preflight.PlanInput.Checks {
		for evidenceIndex := range preflight.PlanInput.Checks[checkIndex].Evidence {
			if preflight.PlanInput.Checks[checkIndex].Evidence[evidenceIndex].Key == "selected_export_ref" {
				preflight.PlanInput.Checks[checkIndex].Evidence[evidenceIndex].Value = "cur2-ponmlkjihgfedcba"
			}
		}
	}
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_cur2_package_handoff_evidence_incomplete" {
		t.Fatalf("Code = %q, want aws_cur2_package_handoff_evidence_incomplete", report.Code)
	}
}

func TestRunnerFailsClosedForUnapprovedReadyPreflightCode(t *testing.T) {
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{
		report: readyPreflightReport("aws_cur2_include_resources_not_required"),
	}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", report.Status)
	}
	if report.Code != "aws_cur2_package_preflight_not_ready" {
		t.Fatalf("Code = %q, want aws_cur2_package_preflight_not_ready", report.Code)
	}
	if report.Handoff != nil {
		t.Fatalf("Handoff = %#v, want nil", report.Handoff)
	}
}

func TestRunnerFailsClosedWhenSimpleHandoffEvidenceIsInvalid(t *testing.T) {
	for _, tt := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "output format", key: "output_format", value: "TEXT-OR-CSV"},
		{name: "previous period", key: "previous_billing_period", value: "2026/06"},
		{name: "manifest object", key: "cur2_manifest_prefix", value: "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			preflight := readyPreflightReport("aws_cur2_preflight_ready")
			for checkIndex := range preflight.PlanInput.Checks {
				for evidenceIndex := range preflight.PlanInput.Checks[checkIndex].Evidence {
					if preflight.PlanInput.Checks[checkIndex].Evidence[evidenceIndex].Key == tt.key {
						preflight.PlanInput.Checks[checkIndex].Evidence[evidenceIndex].Value = tt.value
					}
				}
			}
			runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

			report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

			if report.Status != workflow.RunStatusBlocked {
				t.Fatalf("Status = %q, want blocked", report.Status)
			}
			if report.Code != "aws_cur2_package_handoff_evidence_incomplete" {
				t.Fatalf("Code = %q, want aws_cur2_package_handoff_evidence_incomplete", report.Code)
			}
		})
	}
}

func TestRunnerIgnoresKnownPreflightOnlyWarningsInHandoff(t *testing.T) {
	for _, code := range []string{
		"aws_cur2_table_configuration_defaulted",
		"aws_cur2_include_resources_enabled",
		"aws_cur2_include_resources_not_required",
	} {
		t.Run(code, func(t *testing.T) {
			preflight := readyPreflightReport("aws_cur2_preflight_ready")
			preflight.PlanInput.Checks = append(preflight.PlanInput.Checks,
				planCheck(workflow.CheckWarn, code),
			)
			runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

			report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

			if report.Status != workflow.StatusReady {
				t.Fatalf("Status = %q, want ready", report.Status)
			}
			if report.Handoff == nil {
				t.Fatal("Handoff is nil")
			}
			assertHandoffWarningAbsent(t, report, code)
		})
	}
}

func TestRunnerBuildsHandoffForAcceptedReadyWarnings(t *testing.T) {
	for _, tt := range []struct {
		code        string
		wantWarning string
	}{
		{code: "aws_cur2_time_granularity_not_preferred", wantWarning: "aws_cur2_time_granularity_not_preferred"},
		{code: "aws_cur2_time_granularity_unverified", wantWarning: "aws_cur2_time_granularity_unverified"},
		{code: "aws_s3_delivery_policy_missing", wantWarning: "aws_s3_delivery_policy_missing"},
		{code: "aws_s3_bucket_policy_inaccessible", wantWarning: "aws_s3_bucket_policy_inaccessible"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{
				report: readyPreflightReport(tt.code),
			}})

			report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

			if report.Status != workflow.StatusReady {
				t.Fatalf("Status = %q, want ready", report.Status)
			}
			if report.Handoff == nil {
				t.Fatal("Handoff is nil")
			}
			if len(report.Handoff.Warnings) != 1 {
				t.Fatalf("Warnings len = %d, want 1", len(report.Handoff.Warnings))
			}
			if report.Handoff.Warnings[0].Code != tt.wantWarning {
				t.Fatalf("warning code = %q, want %q", report.Handoff.Warnings[0].Code, tt.wantWarning)
			}
			if tt.code == "aws_s3_delivery_policy_missing" || tt.code == "aws_s3_bucket_policy_inaccessible" {
				assertHandoffField(t, report, "s3_delivery_policy_readiness", "future_delivery_not_proven")
			}
		})
	}
}

func TestRunnerBuildsHandoffWarningsFromFullPreflightPlan(t *testing.T) {
	preflight := readyPreflightReport("aws_cur2_time_granularity_not_preferred")
	preflight.PlanInput.Checks = append(preflight.PlanInput.Checks,
		planCheck(workflow.CheckWarn, "aws_s3_delivery_policy_missing"),
	)
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want ready", report.Status)
	}
	if report.Handoff == nil {
		t.Fatal("Handoff is nil")
	}
	assertHandoffField(t, report, "s3_delivery_policy_readiness", "future_delivery_not_proven")
	assertHandoffWarning(t, report, "aws_cur2_time_granularity_not_preferred")
	assertHandoffWarning(t, report, "aws_s3_delivery_policy_missing")
	if len(report.Handoff.Warnings) != 2 {
		t.Fatalf("Warnings len = %d, want 2: %#v", len(report.Handoff.Warnings), report.Handoff.Warnings)
	}
}

func TestRunnerBuildsHandoffForUnverifiedTimeGranularityEvidence(t *testing.T) {
	preflight := readyPreflightReport("aws_cur2_time_granularity_unverified")
	for checkIndex := range preflight.PlanInput.Checks {
		for evidenceIndex := range preflight.PlanInput.Checks[checkIndex].Evidence {
			if preflight.PlanInput.Checks[checkIndex].Evidence[evidenceIndex].Key == "time_granularity" {
				preflight.PlanInput.Checks[checkIndex].Evidence[evidenceIndex].Value = "unverified"
			}
		}
	}
	runner := NewRunner(RunnerConfig{PreflightRunner: fakePreflightRunner{report: preflight}})

	report := runner.Run(context.Background(), AWSBillingPackageRequest(), packageOptions("cur2-abcdefghijklmnop"))

	if report.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want ready", report.Status)
	}
	assertHandoffField(t, report, "time_granularity", "unverified")
	if report.Handoff == nil || len(report.Handoff.Warnings) != 1 {
		t.Fatalf("Warnings = %#v, want one unverified granularity warning", report.Handoff)
	}
	if report.Handoff.Warnings[0].Code != "aws_cur2_time_granularity_unverified" {
		t.Fatalf("warning code = %q, want aws_cur2_time_granularity_unverified", report.Handoff.Warnings[0].Code)
	}
}

type fakePreflightRunner struct {
	report workflow.CapabilityReport
	got    workflow.ExecutionOptions
}

func (runner fakePreflightRunner) Run(ctx context.Context, request workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
	runner.got = options
	return runner.report
}

func packageOptions(exportRef string) workflow.ExecutionOptions {
	return workflow.ExecutionOptions{
		InterfaceMode: workflow.InterfaceModeDirect,
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				Profile:       "default",
				Region:        "us-east-1",
				CUR2ExportRef: exportRef,
			},
		},
	}
}

func readyPreflightReport(code string) workflow.CapabilityReport {
	return workflow.CapabilityReport{
		Status:        workflow.StatusReady,
		SupportStatus: workflow.SupportSupported,
		Code:          code,
		Message:       "AWS CUR 2.0 billing preflight is ready.",
		Mutated:       false,
		SourceHandles: sourceHandles(),
		PlanInput: &workflow.ExecutionPlanInput{
			Request: workflow.Request{
				Goal:           assessment.RapidAssessment,
				CollectionPath: assessment.CollectionBilling,
				Provider:       assessment.ProviderAWS,
				Action:         assessment.ActionPreflight,
			},
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified with account ending 9012 and caller hash sha256:123456789abc.",
				SourceHandles:  sourceHandles(),
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageUnknown,
				Summary:        "AWS billing coverage is evaluated from the selected CUR 2.0 export.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Checks: []workflow.PlanCheck{
				planCheck(workflow.CheckPass, "aws_cur2_export_selected",
					workflow.PlanEvidence{Key: "cur_version", Value: "CUR2.0"},
					workflow.PlanEvidence{Key: "selected_export_ref", Value: "cur2-abcdefghijklmnop"},
				),
				planCheck(workflow.CheckPass, "aws_cur2_output_format_ready",
					workflow.PlanEvidence{Key: "output_format", Value: "TEXT_OR_CSV"},
				),
				planCheck(workflow.CheckPass, "aws_cur2_compression_ready",
					workflow.PlanEvidence{Key: "compression", Value: "GZIP"},
				),
				planCheck(workflow.CheckPass, "aws_cur2_time_granularity_ready",
					workflow.PlanEvidence{Key: "time_granularity", Value: "MONTHLY"},
				),
				planCheck(workflow.CheckPass, "aws_cur2_overwrite_ready",
					workflow.PlanEvidence{Key: "overwrite", Value: "CREATE_NEW_REPORT"},
				),
				planCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_bucket", Value: "matilda-cur2-billing"},
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda/cur2"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us-east-1"},
				),
				planCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"},
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/"},
				),
			},
			SourceHandles: sourceHandles(),
		},
	}
}

func planCheck(status workflow.CheckStatus, code string, evidence ...workflow.PlanEvidence) workflow.PlanCheck {
	return workflow.PlanCheck{
		Status:  status,
		Title:   code,
		Message: code + " message",
		Evidence: append([]workflow.PlanEvidence{
			{Key: "code", Value: code},
		}, evidence...),
		SourceHandles: sourceHandles(),
	}
}

func sourceHandles() []workflow.SourceHandle {
	return []workflow.SourceHandle{
		{Label: "AWS Rapid Assessment Billing Handoff Schema", URI: "docs/references/aws/aws-rapid-assessment-billing-handoff-schema.md"},
	}
}

func assertHandoffField(t *testing.T, report workflow.CapabilityReport, key string, want string) {
	t.Helper()
	for _, field := range report.Handoff.Fields {
		if field.Key == key {
			if field.Value != want {
				t.Fatalf("handoff field %s = %q, want %q", key, field.Value, want)
			}
			return
		}
	}
	t.Fatalf("handoff field %s missing in %#v", key, report.Handoff.Fields)
}

func assertNextStepContains(t *testing.T, report workflow.CapabilityReport, want string) {
	t.Helper()
	if report.Handoff == nil {
		t.Fatal("Handoff is nil")
	}
	for _, nextStep := range report.Handoff.NextSteps {
		if strings.Contains(nextStep, want) {
			return
		}
	}
	t.Fatalf("next step containing %q missing in %#v", want, report.Handoff.NextSteps)
}

func assertHandoffWarning(t *testing.T, report workflow.CapabilityReport, code string) {
	t.Helper()
	if report.Handoff == nil {
		t.Fatal("Handoff is nil")
	}
	for _, warning := range report.Handoff.Warnings {
		if warning.Code == code {
			return
		}
	}
	t.Fatalf("handoff warning %q missing in %#v", code, report.Handoff.Warnings)
}

func assertHandoffWarningAbsent(t *testing.T, report workflow.CapabilityReport, code string) {
	t.Helper()
	if report.Handoff == nil {
		t.Fatal("Handoff is nil")
	}
	for _, warning := range report.Handoff.Warnings {
		if warning.Code == code {
			t.Fatalf("handoff warning %q present in %#v", code, report.Handoff.Warnings)
		}
	}
}
