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

func TestPreflightReturnsProviderNeutralExecutionPlan(t *testing.T) {
	request := Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionAPI,
		Provider:       assessment.ProviderGCP,
		Action:         assessment.ActionPreflight,
	}

	result := DefaultRegistry().Execute(request)

	if result.Plan == nil {
		t.Fatal("preflight result did not include execution plan")
	}
	if result.Plan.Request != request {
		t.Fatalf("plan request = %#v, want %#v", result.Plan.Request, request)
	}
	if result.Plan.CoverageRecommendation.CoverageStatus != CoverageUnknown {
		t.Fatalf("coverage status = %q, want %q", result.Plan.CoverageRecommendation.CoverageStatus, CoverageUnknown)
	}
	if result.Plan.PackageSchemaStatus != PackageSchemaProviderSchemaRequired {
		t.Fatalf("package schema status = %q, want %q", result.Plan.PackageSchemaStatus, PackageSchemaProviderSchemaRequired)
	}
	if len(result.Plan.Steps) != 1 {
		t.Fatalf("plan steps length = %d, want 1", len(result.Plan.Steps))
	}
	if result.Plan.Steps[0].Intent != PlanStepBlocked {
		t.Fatalf("plan step intent = %q, want %q", result.Plan.Steps[0].Intent, PlanStepBlocked)
	}
	if result.Plan.Steps[0].RequiresApproval {
		t.Fatal("provider-neutral blocked plan step unexpectedly requires approval")
	}
	if result.Plan.Approval.Approved {
		t.Fatal("provider-neutral plan approved itself")
	}
	if !result.Plan.Approval.Blocked {
		t.Fatal("provider-neutral blocked plan did not mark approval blocked")
	}
	if got := result.Plan.StatusCounts.StepIntents[PlanStepBlocked]; got != 1 {
		t.Fatalf("blocked step count = %d, want 1", got)
	}
	if got := result.Plan.StatusCounts.CheckStatuses[CheckFail]; got != 1 {
		t.Fatalf("failed check count = %d, want 1", got)
	}
	assertSafeSourceHandles(t, result.Plan.SourceHandles)
	if len(result.Plan.MissingSourceOfTruth) == 0 {
		t.Fatal("plan missing source-of-truth details")
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
			t.Fatalf("source handle URI must be a cached docs/ relative path, got %q", handle.URI)
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
