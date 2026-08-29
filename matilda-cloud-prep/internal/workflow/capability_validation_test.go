package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/handoff"
)

func TestNewRegistryRejectsInvalidCapabilityRegistrations(t *testing.T) {
	tests := []struct {
		name       string
		capability Capability
		want       string
	}{
		{
			name: "unsupported action",
			capability: Capability{
				Request: Request{Goal: assessment.RapidAssessment, CollectionPath: assessment.CollectionBilling, Provider: assessment.ProviderAWS, Action: assessment.Action("destroy")},
				Runner:  RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport { return CapabilityReport{} }),
			},
			want: "not supported",
		},
		{
			name: "nil runner",
			capability: Capability{
				Request: awsBillingPreflightRequest(),
			},
			want: "runner is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRegistry(tt.capability)
			if err == nil {
				t.Fatal("NewRegistry succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestRegistryRejectsInvalidCapabilityReportFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CapabilityReport)
	}{
		{
			name: "unknown run status",
			mutate: func(report *CapabilityReport) {
				report.Status = RunStatus("ok")
			},
		},
		{
			name: "unknown support status",
			mutate: func(report *CapabilityReport) {
				report.SupportStatus = SupportStatus("ok")
			},
		},
		{
			name: "empty code",
			mutate: func(report *CapabilityReport) {
				report.Code = ""
			},
		},
		{
			name: "empty message",
			mutate: func(report *CapabilityReport) {
				report.Message = ""
			},
		},
		{
			name: "message with bare AWS account ID",
			mutate: func(report *CapabilityReport) {
				report.Message = "AWS account 123456789012 is ready."
			},
		},
		{
			name: "message with private path",
			mutate: func(report *CapabilityReport) {
				report.Message = "Review /private/tmp/matilda-output.json."
			},
		},
		{
			name: "unsafe missing source",
			mutate: func(report *CapabilityReport) {
				report.MissingSourceOfTruth = []string{"read /Users/lly/private.txt"}
			},
		},
		{
			name: "missing plan input",
			mutate: func(report *CapabilityReport) {
				report.PlanInput = nil
			},
		},
		{
			name: "invalid plan input",
			mutate: func(report *CapabilityReport) {
				report.PlanInput.SourceHandles = nil
			},
		},
		{
			name: "manifest without package schema",
			mutate: func(report *CapabilityReport) {
				report.Manifest = &handoff.Manifest{SchemaVersion: "provider_v0"}
			},
		},
		{
			name: "handoff from non package action",
			mutate: func(report *CapabilityReport) {
				report.Handoff = sampleStructuredHandoff()
			},
		},
		{
			name: "warning with empty code",
			mutate: func(report *CapabilityReport) {
				report.Warnings = []handoff.Warning{{Message: "warning message"}}
			},
		},
		{
			name: "warning with unsafe message",
			mutate: func(report *CapabilityReport) {
				report.Warnings = []handoff.Warning{{Code: "warning", Message: "token=plain-token"}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := awsBillingPreflightRequest()
			report := sampleCapabilityReport(request, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
			tt.mutate(&report)
			registry, err := NewRegistry(Capability{
				Request: request,
				Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
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
			if strings.Contains(result.Message, "plain-token") || strings.Contains(result.Message, "/Users/") {
				t.Fatalf("invalid result message leaked unsafe content: %q", result.Message)
			}
		})
	}
}

func TestRegistryAcceptsSafeHandoffFromPackageCapability(t *testing.T) {
	request := awsBillingPackageRequest()
	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
			report := sampleCapabilityReport(request, StatusReady, SupportSupported, "aws_cur2_package_handoff_ready")
			report.Handoff = sampleStructuredHandoff()
			return report
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.Execute(request)

	if result.Status != StatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, StatusReady)
	}
	if result.Handoff == nil {
		t.Fatal("package capability result did not include handoff payload")
	}
	if result.Manifest != nil {
		t.Fatalf("package capability returned manifest %#v, want nil", result.Manifest)
	}
}

func TestRegistrySanitizesPackageExecutionOptionsOnInvalidCapabilityResult(t *testing.T) {
	request := awsBillingPackageRequest()
	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
			report := sampleCapabilityReport(request, StatusReady, SupportSupported, "aws_cur2_package_handoff_ready")
			report.Handoff = sampleStructuredHandoff()
			report.Handoff.Fields[0].Value = "arn:aws:iam::123456789012:role/operator"
			return report
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.ExecuteContext(context.Background(), request, ExecutionOptions{
		InterfaceMode:  InterfaceModeDirect,
		TimeoutSeconds: 45,
		Selectors: &ExecutionSelectors{
			AWS: &AWSExecutionSelectors{
				Profile:       "default",
				Region:        "us-east-1",
				CUR2ExportRef: "cur2-abcdefghijklmnop",
			},
		},
	})

	if result.Status != RunStatusFailed {
		t.Fatalf("Status = %q, want failed", result.Status)
	}
	if result.ExecutionOptions.Selectors != nil {
		t.Fatalf("ExecutionOptions.Selectors = %#v, want nil for package failure output", result.ExecutionOptions.Selectors)
	}
}

func TestRegistryAcceptsSafeCapabilityWarnings(t *testing.T) {
	request := awsBillingPreflightRequest()
	registry, err := NewRegistry(Capability{
		Request: request,
		Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
			report := sampleCapabilityReport(request, StatusReady, SupportSupported, "aws_cur2_preflight_ready")
			report.Warnings = []handoff.Warning{{
				Code:    "aws_cur2_include_resources_not_required",
				Message: "INCLUDE_RESOURCES is surfaced for operator review.",
			}}
			return report
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	result := registry.Execute(request)

	if result.Status != StatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, StatusReady)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings length = %d, want 1", len(result.Warnings))
	}
}

func TestDeepDiscoveryDuplicateCapabilityUsesDeepDiscoveryKey(t *testing.T) {
	request := Request{Goal: assessment.DeepDiscovery, Provider: assessment.ProviderAWS, Action: assessment.ActionPreflight}
	_, err := NewRegistry(
		Capability{Request: request, Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
			return sampleCapabilityReport(request, StatusReady, SupportSupported, "aws_deep_discovery_preflight_ready")
		})},
		Capability{Request: request, Runner: RunnerFunc(func(context.Context, Request, ExecutionOptions) CapabilityReport {
			return sampleCapabilityReport(request, StatusReady, SupportSupported, "aws_deep_discovery_preflight_ready")
		})},
	)

	if err == nil {
		t.Fatal("NewRegistry accepted duplicate Deep Discovery capability")
	}
	if !strings.Contains(err.Error(), "deep-discovery/aws/preflight") {
		t.Fatalf("error = %q, want Deep Discovery capability key", err)
	}
}
