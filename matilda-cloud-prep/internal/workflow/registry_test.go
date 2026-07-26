package workflow

import (
	"strings"
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
			if result.SupportStatus != SupportNotImplemented {
				t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, SupportNotImplemented)
			}
			contract, ok := ActionContractFor(request.Action)
			if !ok {
				t.Fatalf("ActionContractFor(%q) returned ok=false", request.Action)
			}
			if result.MutationLevel != contract.MutationLevel {
				t.Fatalf("MutationLevel = %q, want %q", result.MutationLevel, contract.MutationLevel)
			}
			if result.ActionContract.Action != request.Action {
				t.Fatalf("ActionContract.Action = %q, want %q", result.ActionContract.Action, request.Action)
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
			if len(result.SourceHandles) == 0 {
				t.Fatal("unimplemented capability did not include source handles")
			}
			assertSafeSourceHandles(t, result.SourceHandles)
			if len(result.MissingSourceOfTruth) == 0 {
				t.Fatal("unimplemented capability did not include missing source of truth")
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
	if result.SupportStatus != SupportGuided {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, SupportGuided)
	}
	if result.MutationLevel != MutationLocalOnly {
		t.Fatalf("MutationLevel = %q, want %q", result.MutationLevel, MutationLocalOnly)
	}
	if result.ActionContract.Action != assessment.ActionPackage {
		t.Fatalf("ActionContract.Action = %q, want %q", result.ActionContract.Action, assessment.ActionPackage)
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
	if len(result.SourceHandles) == 0 {
		t.Fatal("package action did not include source handles")
	}
	assertSafeSourceHandles(t, result.SourceHandles)
	if len(result.MissingSourceOfTruth) == 0 {
		t.Fatal("package action did not include missing source of truth")
	}
}

func assertSafeSourceHandles(t *testing.T, handles []SourceHandle) {
	t.Helper()

	for _, handle := range handles {
		if handle.Label == "" {
			t.Fatalf("source handle has empty label: %#v", handle)
		}
		if handle.URI == "" {
			t.Fatalf("source handle has empty URI: %#v", handle)
		}
		if strings.HasPrefix(handle.URI, "/") || strings.Contains(handle.URI, "/Users/") {
			t.Fatalf("source handle URI must be relative or official URL, got %q", handle.URI)
		}

		combined := strings.ToLower(handle.Label + " " + handle.URI)
		for _, forbidden := range []string{
			"matildadocs",
			"private_key",
			"plain-secret",
			"secret_key",
			"session_token",
			"access_key",
			"customer-data",
		} {
			if strings.Contains(combined, forbidden) {
				t.Fatalf("source handle contains forbidden term %q: %#v", forbidden, handle)
			}
		}
	}
}
