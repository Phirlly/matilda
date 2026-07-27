package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
)

func TestRegistryDispatchesInjectedCapability(t *testing.T) {
	request := awsBillingPreflightRequest()
	called := false

	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(ctx context.Context, got Request) CapabilityReport {
			called = true
			if got != request {
				t.Fatalf("runner request = %#v, want %#v", got, request)
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

func TestRegistryKeepsUnregisteredPathsFailClosed(t *testing.T) {
	request := awsBillingPreflightRequest()
	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(ctx context.Context, got Request) CapabilityReport {
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
		Capability{Request: request, Runner: RunnerFunc(func(context.Context, Request) CapabilityReport {
			return sampleCapabilityReport(request, StatusReady, SupportSupported, "first")
		})},
		Capability{Request: request, Runner: RunnerFunc(func(context.Context, Request) CapabilityReport {
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
				Runner: RunnerFunc(func(context.Context, Request) CapabilityReport {
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
		Runner: RunnerFunc(func(context.Context, Request) CapabilityReport {
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
