package workflow

import (
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
)

func TestDefaultRegistryFailsClosedWithoutMutation(t *testing.T) {
	requests := []Request{}
	for _, provider := range []assessment.Provider{
		assessment.ProviderAWS,
		assessment.ProviderAzure,
		assessment.ProviderGCP,
		assessment.ProviderOCI,
	} {
		for _, collectionPath := range []assessment.CollectionPath{
			assessment.CollectionBilling,
			assessment.CollectionAPI,
		} {
			for _, action := range []assessment.Action{
				assessment.ActionPreflight,
				assessment.ActionApplyPrereqs,
				assessment.ActionValidate,
			} {
				requests = append(requests, Request{
					Goal:           assessment.RapidAssessment,
					CollectionPath: collectionPath,
					Provider:       provider,
					Action:         action,
				})
			}
		}

		for _, action := range []assessment.Action{
			assessment.ActionPreflight,
			assessment.ActionApplyPrereqs,
			assessment.ActionValidate,
		} {
			requests = append(requests, Request{
				Goal:     assessment.DeepDiscovery,
				Provider: provider,
				Action:   action,
			})
		}
	}

	for _, request := range requests {
		t.Run(string(request.Goal)+"/"+string(request.CollectionPath)+"/"+string(request.Provider)+"/"+string(request.Action), func(t *testing.T) {
			result := DefaultRegistry().Execute(request)

			if result.Status != StatusNotImplemented {
				t.Fatalf("Status = %q, want %q", result.Status, StatusNotImplemented)
			}
			if result.Code != "provider_capability_not_implemented" {
				t.Fatalf("Code = %q, want provider_capability_not_implemented", result.Code)
			}
			if result.Mutated {
				t.Fatal("unimplemented capability reported mutation")
			}
			if result.ProviderCapabilityImplemented {
				t.Fatal("default registry should fail closed for provider-specific capability")
			}
		})
	}
}

func TestPackageActionReturnsMinimalManifestWithProviderSchemaWarning(t *testing.T) {
	result := DefaultRegistry().Execute(Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderGCP,
		Action:         assessment.ActionPackage,
	})

	if result.Status != StatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, StatusReady)
	}
	if result.Mutated {
		t.Fatal("package action must not report cloud mutation")
	}
	if result.Manifest == nil {
		t.Fatal("package action did not return a manifest")
	}
	if result.Manifest.SchemaVersion != "minimal_v0" {
		t.Fatalf("Manifest.SchemaVersion = %q, want minimal_v0", result.Manifest.SchemaVersion)
	}
	if len(result.Warnings) == 0 || result.Warnings[0].Code != "provider_schema_required" {
		t.Fatalf("Warnings = %#v, want provider_schema_required warning", result.Warnings)
	}
}
