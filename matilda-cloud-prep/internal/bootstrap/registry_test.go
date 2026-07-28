package bootstrap

import (
	"context"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestRegistryCanKeepAWSBillingPreflightDependencyBlocked(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}

	result := Registry(RegistryConfig{}).Execute(request)

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

func TestRegistryFactoryReceivesAWSExecutionOptions(t *testing.T) {
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}
	var gotOptions workflow.ExecutionOptions
	registry := Registry(RegistryConfig{
		AWSBillingPreflightClientFactory: func(options workflow.ExecutionOptions) cur2preflight.Client {
			gotOptions = options
			return nil
		},
	})

	result := registry.ExecuteContext(context.Background(), request, workflow.ExecutionOptions{
		InterfaceMode:  workflow.InterfaceModeDirect,
		TimeoutSeconds: 45,
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				Profile:       "default",
				Region:        "us-west-2",
				CUR2ExportRef: "cur2-1234abcd5678ef90",
			},
		},
	})

	if result.Code != "aws_provider_capability_blocked" {
		t.Fatalf("Code = %q, want aws_provider_capability_blocked", result.Code)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil {
		t.Fatalf("factory options missing AWS selectors: %#v", gotOptions)
	}
	if gotOptions.Selectors.AWS.Profile != "default" || gotOptions.Selectors.AWS.Region != "us-west-2" || gotOptions.Selectors.AWS.CUR2ExportRef != "cur2-1234abcd5678ef90" {
		t.Fatalf("factory AWS selectors = %#v, want supplied selectors", gotOptions.Selectors.AWS)
	}
}

func TestDefaultRegistryFailsClosedWhenAWSProfileIsShadowedByEnvironment(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}

	result := DefaultRegistry().ExecuteContext(context.Background(), request, workflow.ExecutionOptions{
		InterfaceMode:  workflow.InterfaceModeDirect,
		TimeoutSeconds: 45,
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				Profile: "default",
				Region:  "us-west-2",
			},
		},
	})

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != "aws_config_profile_shadowed" {
		t.Fatalf("Code = %q, want aws_config_profile_shadowed", result.Code)
	}
	if result.Mutated {
		t.Fatal("profile-shadowed preflight must not mutate cloud resources")
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

func TestDefaultRegistryConstructionDoesNotRequireAWSConfiguration(t *testing.T) {
	registry := DefaultRegistry()

	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderGCP,
		Action:         assessment.ActionPreflight,
	}
	result := registry.Execute(request)

	if result.Code != "provider_capability_not_implemented" {
		t.Fatalf("Code = %q, want provider_capability_not_implemented", result.Code)
	}
}
