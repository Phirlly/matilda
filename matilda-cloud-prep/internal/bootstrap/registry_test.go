package bootstrap

import (
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestDefaultRegistryWiresAWSBillingPreflightAsDependencyBlocked(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}

	result := DefaultRegistry().Execute(request)

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.SupportStatus != workflow.SupportBlocked {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportBlocked)
	}
	if result.Code != "aws_provider_capability_blocked" {
		t.Fatalf("Code = %q, want aws_provider_capability_blocked", result.Code)
	}
	if !result.ProviderCapabilityImplemented {
		t.Fatal("AWS runtime capability should be registered even when live client wiring is blocked")
	}
	if result.Mutated {
		t.Fatal("AWS preflight runtime path must remain read-only")
	}
}

func TestDefaultRegistryKeepsUnregisteredProviderPathsFailClosed(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderGCP,
		Action:         assessment.ActionPreflight,
	}

	result := DefaultRegistry().Execute(request)

	if result.Status != workflow.StatusNotImplemented {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.StatusNotImplemented)
	}
	if result.Code != "provider_capability_not_implemented" {
		t.Fatalf("Code = %q, want provider_capability_not_implemented", result.Code)
	}
	if result.ProviderCapabilityImplemented {
		t.Fatal("unregistered provider path should not be marked implemented")
	}
	if result.Mutated {
		t.Fatal("unregistered provider path must not report mutation")
	}
}
