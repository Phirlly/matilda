package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/handoff"
)

func TestRegistryDispatchesInjectedCapability(t *testing.T) {
	request := awsBillingPreflightRequest()
	called := false
	sawDeadline := false

	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(ctx context.Context, got Request, options ExecutionOptions) CapabilityReport {
			called = true
			_, sawDeadline = ctx.Deadline()
			if got != request {
				t.Fatalf("runner request = %#v, want %#v", got, request)
			}
			if options.TimeoutSeconds != DefaultExecutionTimeoutSeconds {
				t.Fatalf("runner timeout = %d, want default %d", options.TimeoutSeconds, DefaultExecutionTimeoutSeconds)
			}
			return sampleCapabilityReport(request, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.Execute(request)

	if !called {
		t.Fatal("injected capability was not called")
	}
	if !sawDeadline {
		t.Fatal("registry Execute did not apply default execution deadline")
	}
	if result.Status != StatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, StatusReady)
	}
	if result.SupportStatus != SupportSupported {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, SupportSupported)
	}
	if result.MutationLevel != MutationNone {
		t.Fatalf("MutationLevel = %q, want %q", result.MutationLevel, MutationNone)
	}
	if result.Code != "aws_cur2_preflight_ready" {
		t.Fatalf("Code = %q, want aws_cur2_preflight_ready", result.Code)
	}
	if result.Mutated {
		t.Fatal("preflight capability reported mutation")
	}
	if !result.ProviderCapabilityImplemented {
		t.Fatal("provider capability should be marked implemented")
	}
	if result.Plan == nil {
		t.Fatal("provider capability result did not include validated execution plan")
	}
	assertSafeSourceHandles(t, result.SourceHandles)
}

func TestRegistryDispatchesPackageCapabilityBeforeMinimalFallback(t *testing.T) {
	request := awsBillingPackageRequest()
	called := false

	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(ctx context.Context, got Request, options ExecutionOptions) CapabilityReport {
			called = true
			report := sampleCapabilityReport(got, StatusReady, SupportSupported, "aws_cur2_package_handoff_ready")
			report.Handoff = sampleStructuredHandoff()
			return report
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.Execute(request)

	if !called {
		t.Fatal("package capability was not called")
	}
	if result.Code != "aws_cur2_package_handoff_ready" {
		t.Fatalf("Code = %q, want aws_cur2_package_handoff_ready", result.Code)
	}
	if result.Handoff == nil {
		t.Fatal("package capability did not return handoff payload")
	}
	if result.Manifest != nil {
		t.Fatalf("provider-specific package result Manifest = %#v, want nil", result.Manifest)
	}
}

func TestRegistryKeepsUnregisteredPackageOnMinimalFallback(t *testing.T) {
	registry, err := NewRegistry(Capability{
		Request: awsBillingPreflightRequest(),
		Runner: RunnerFunc(func(ctx context.Context, got Request, options ExecutionOptions) CapabilityReport {
			return sampleCapabilityReport(got, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.Execute(Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderGCP,
		Action:         assessment.ActionPackage,
	})

	if result.Code != "minimal_manifest_ready" {
		t.Fatalf("Code = %q, want minimal_manifest_ready", result.Code)
	}
	if result.Manifest == nil {
		t.Fatal("unregistered package fallback did not return minimal manifest")
	}
	if result.Handoff != nil {
		t.Fatalf("unregistered package fallback returned handoff %#v, want nil", result.Handoff)
	}
}

func TestRegistryPreservesCallerSuppliedDeadline(t *testing.T) {
	request := awsBillingPreflightRequest()
	callerDeadline := time.Now().Add(30 * time.Second).Round(time.Second)
	parent, cancel := context.WithDeadline(context.Background(), callerDeadline)
	defer cancel()
	var gotDeadline time.Time

	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(ctx context.Context, got Request, options ExecutionOptions) CapabilityReport {
			var ok bool
			gotDeadline, ok = ctx.Deadline()
			if !ok {
				t.Fatal("runner context did not preserve caller deadline")
			}
			return sampleCapabilityReport(got, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.ExecuteContext(parent, request, ExecutionOptions{TimeoutSeconds: 120})

	if result.Status != StatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, StatusReady)
	}
	if !gotDeadline.Equal(callerDeadline) {
		t.Fatalf("runner deadline = %s, want caller deadline %s", gotDeadline, callerDeadline)
	}
}

func TestRegistryPassesExecutionOptionsToCapabilityRunner(t *testing.T) {
	request := awsBillingPreflightRequest()
	options := ExecutionOptions{
		InterfaceMode:  InterfaceModeDirect,
		TimeoutSeconds: 45,
		Selectors: &ExecutionSelectors{
			AWS: &AWSExecutionSelectors{
				Profile:       "default",
				Region:        "us-west-2",
				CUR2ExportRef: "cur2-abcdefghijklmnop",
			},
		},
	}
	var gotOptions ExecutionOptions

	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(ctx context.Context, got Request, options ExecutionOptions) CapabilityReport {
			gotOptions = options
			return sampleCapabilityReport(got, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.ExecuteContext(context.Background(), request, options)

	if result.Status != StatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, StatusReady)
	}
	if gotOptions.SchemaVersion != ExecutionOptionsSchemaVersion {
		t.Fatalf("runner options schema = %q, want %q", gotOptions.SchemaVersion, ExecutionOptionsSchemaVersion)
	}
	if gotOptions.InterfaceMode != InterfaceModeDirect || gotOptions.TimeoutSeconds != 45 {
		t.Fatalf("runner options = %#v, want direct 45s", gotOptions)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil {
		t.Fatalf("runner AWS selectors missing: %#v", gotOptions)
	}
	if gotOptions.Selectors.AWS.Profile != "default" || gotOptions.Selectors.AWS.Region != "us-west-2" || gotOptions.Selectors.AWS.CUR2ExportRef != "cur2-abcdefghijklmnop" {
		t.Fatalf("runner AWS selectors = %#v, want supplied selectors", gotOptions.Selectors.AWS)
	}
	if result.ExecutionOptions.Selectors == nil || result.ExecutionOptions.Selectors.AWS == nil {
		t.Fatalf("result execution_options missing AWS selectors: %#v", result.ExecutionOptions)
	}
	if result.ExecutionOptions.TimeoutSeconds != 45 {
		t.Fatalf("result execution_options timeout = %d, want 45", result.ExecutionOptions.TimeoutSeconds)
	}
	if result.Plan == nil || result.Plan.ExecutionOptions.TimeoutSeconds != 45 {
		t.Fatalf("plan execution_options = %#v, want timeout 45", result.Plan)
	}
}

func TestRegistryRejectsInvalidExecutionOptionsBeforeRunner(t *testing.T) {
	tests := []struct {
		name      string
		options   ExecutionOptions
		forbidden []string
	}{
		{
			name: "raw ARN export ref",
			options: ExecutionOptions{
				Selectors: &ExecutionSelectors{
					AWS: &AWSExecutionSelectors{
						CUR2ExportRef: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/live",
					},
				},
			},
			forbidden: []string{"arn:aws"},
		},
		{
			name:      "unsafe schema version",
			options:   ExecutionOptions{SchemaVersion: "/private/tmp/schema"},
			forbidden: []string{"/private/tmp"},
		},
		{
			name:      "unsafe interface mode",
			options:   ExecutionOptions{InterfaceMode: InterfaceMode("arn:aws:iam::123456789012:role/operator")},
			forbidden: []string{"arn:aws"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := awsBillingPreflightRequest()
			called := false
			registry, err := NewRegistry(Capability{
				Request: request,
				Runner: RunnerFunc(func(ctx context.Context, got Request, options ExecutionOptions) CapabilityReport {
					called = true
					return sampleCapabilityReport(got, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
				}),
			})
			if err != nil {
				t.Fatalf("NewRegistry returned error: %v", err)
			}

			result := registry.ExecuteContext(context.Background(), request, tt.options)
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("Marshal result returned error: %v", err)
			}
			output := string(encoded)

			if called {
				t.Fatal("runner was called after invalid execution options")
			}
			if result.Status != RunStatusFailed {
				t.Fatalf("Status = %q, want %q", result.Status, RunStatusFailed)
			}
			if result.Code != "execution_options_invalid" {
				t.Fatalf("Code = %q, want execution_options_invalid", result.Code)
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(output, forbidden) {
					t.Fatalf("invalid execution-options result leaked %q: %s", forbidden, output)
				}
			}
		})
	}
}

func TestRegistryKeepsUnregisteredPathsFailClosed(t *testing.T) {
	request := awsBillingPreflightRequest()
	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(ctx context.Context, got Request, options ExecutionOptions) CapabilityReport {
			return sampleCapabilityReport(got, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	unregistered := Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionAPI,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}

	result := registry.Execute(unregistered)

	if result.Status != StatusNotImplemented {
		t.Fatalf("Status = %q, want %q", result.Status, StatusNotImplemented)
	}
	if result.ProviderCapabilityImplemented {
		t.Fatal("unregistered provider path should not be marked implemented")
	}
	if result.Mutated {
		t.Fatal("unregistered provider path must not report mutation")
	}
}

func TestRegistryRejectsDuplicateCapabilityRegistration(t *testing.T) {
	request := awsBillingPreflightRequest()
	_, err := NewRegistry(
		Capability{Request: request, Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
			return sampleCapabilityReport(request, StatusReady, SupportSupported, "first")
		})},
		Capability{Request: request, Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
			return sampleCapabilityReport(request, StatusReady, SupportSupported, "second")
		})},
	)

	if err == nil {
		t.Fatal("NewRegistry accepted duplicate capability registration")
	}
	if !strings.Contains(err.Error(), "duplicate capability") {
		t.Fatalf("error = %q, want duplicate capability context", err)
	}
}

func TestRegistryRejectsUnsafeCapabilityResult(t *testing.T) {
	tests := []struct {
		name   string
		report CapabilityReport
	}{
		{
			name: "unsafe source handle",
			report: CapabilityReport{
				Status:        StatusReady,
				SupportStatus: SupportSupported,
				Code:          "aws_cur2_preflight_ready",
				Message:       "AWS CUR 2.0 preflight completed.",
				SourceHandles: []SourceHandle{{
					Label: "Private",
					URI:   "/Users/lly/Documents/Development/docs/matildaDocs/private.md",
				}},
				PlanInput: sampleCapabilityPlanInput(awsBillingPreflightRequest()),
			},
		},
		{
			name: "unsafe message",
			report: CapabilityReport{
				Status:        StatusReady,
				SupportStatus: SupportSupported,
				Code:          "aws_cur2_preflight_ready",
				Message:       "caller arn:aws:iam::123456789012:user/operator",
				SourceHandles: awsCapabilitySourceHandles(),
				PlanInput:     sampleCapabilityPlanInput(awsBillingPreflightRequest()),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := awsBillingPreflightRequest()
			registry, err := NewRegistry(Capability{
				Request: request,
				Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
					return tt.report
				}),
			})
			if err != nil {
				t.Fatalf("NewRegistry returned error: %v", err)
			}

			result := registry.Execute(request)
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("Marshal result returned error: %v", err)
			}
			output := string(encoded)

			if result.Status != RunStatusFailed {
				t.Fatalf("Status = %q, want %q", result.Status, RunStatusFailed)
			}
			if result.Code != "provider_capability_result_invalid" {
				t.Fatalf("Code = %q, want provider_capability_result_invalid", result.Code)
			}
			for _, forbidden := range []string{"/Users/", "arn:aws", "access_key", "secret_key", "session_token"} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("unsafe provider result leaked %q in %s", forbidden, output)
				}
			}
		})
	}
}

func TestRegistryRejectsMutationFromReadOnlyCapability(t *testing.T) {
	request := awsBillingPreflightRequest()
	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
			report := sampleCapabilityReport(request, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
			report.Mutated = true
			return report
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.Execute(request)

	if result.Status != RunStatusFailed {
		t.Fatalf("Status = %q, want %q", result.Status, RunStatusFailed)
	}
	if result.Code != "provider_capability_result_invalid" {
		t.Fatalf("Code = %q, want provider_capability_result_invalid", result.Code)
	}
	if result.Mutated {
		t.Fatal("invalid read-only mutation must not leak as mutated=true")
	}
}

func awsBillingPreflightRequest() Request {
	return Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}
}

func awsBillingPackageRequest() Request {
	return Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPackage,
	}
}

func sampleStructuredHandoff() *handoff.Output {
	output := handoff.BuildOutput(handoff.Output{
		HandoffType:    "aws_rapid_assessment_billing_cur2",
		Assessment:     "rapid-assessment",
		CollectionPath: "billing",
		Provider:       "aws",
		Summary:        "AWS CUR 2.0 billing handoff is ready.",
		Fields: []handoff.Field{{
			Key:   "selected_export_ref",
			Label: "Selected CUR 2.0 export",
			Value: "cur2-abcdefghijklmnop",
		}},
		NextSteps: []string{"Use Skip Configuration in Matilda SaaS."},
	})
	return &output
}

func sampleCapabilityReport(request Request, status RunStatus, support SupportStatus, code string) CapabilityReport {
	return CapabilityReport{
		Status:        status,
		SupportStatus: support,
		Code:          code,
		Message:       "AWS CUR 2.0 billing preflight completed.",
		Mutated:       false,
		SourceHandles: awsCapabilitySourceHandles(),
		PlanInput:     sampleCapabilityPlanInput(request),
	}
}

func sampleCapabilityPlanInput(request Request) *ExecutionPlanInput {
	handles := awsCapabilitySourceHandles()
	return &ExecutionPlanInput{
		Request: request,
		OperatorIdentitySummary: OperatorIdentitySummary{
			IdentityStatus: "verified",
			Summary:        "AWS caller identity was verified with account ending 9012 and caller hash sha256:123456789abc.",
			SourceHandles:  handles,
		},
		CoverageRecommendation: CoverageRecommendation{
			CoverageStatus: CoverageUnknown,
			Summary:        "AWS billing coverage is evaluated from the selected CUR 2.0 export.",
		},
		PackageSchemaStatus: PackageSchemaProviderSchemaRequired,
		Steps: []PlanStep{{
			Intent:                    PlanStepReuse,
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
		Checks: []PlanCheck{{
			Status:  CheckPass,
			Title:   "AWS CUR 2.0 preflight capability",
			Message: "Injected AWS CUR 2.0 preflight capability returned a safe result.",
			Evidence: []PlanEvidence{
				{Key: "mutated", Value: "false"},
				{Key: "code", Value: "aws_cur2_preflight_ready"},
			},
			SourceHandles: handles,
		}},
		SourceHandles: handles,
	}
}

func awsCapabilitySourceHandles() []SourceHandle {
	return []SourceHandle{
		{
			Label: "AWS CUR 2.0 Preflight Source Bundle",
			URI:   "docs/references/aws/aws-rapid-assessment-billing-cur2-preflight-source-bundle.md",
		},
	}
}
