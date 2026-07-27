package workflow

import (
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/handoff"
)

type Request struct {
	Goal           assessment.Goal           `json:"goal"`
	CollectionPath assessment.CollectionPath `json:"collection_path,omitempty"`
	Provider       assessment.Provider       `json:"provider"`
	Action         assessment.Action         `json:"action"`
}

type Result struct {
	SchemaVersion                 string            `json:"schema_version"`
	Status                        RunStatus         `json:"status"`
	SupportStatus                 SupportStatus     `json:"support_status"`
	MutationLevel                 MutationLevel     `json:"mutation_level"`
	ActionContract                ActionContract    `json:"action_contract"`
	Code                          string            `json:"code"`
	Message                       string            `json:"message"`
	Mutated                       bool              `json:"mutated"`
	ProviderCapabilityImplemented bool              `json:"provider_capability_implemented"`
	Request                       Request           `json:"request"`
	SourceHandles                 []SourceHandle    `json:"source_handles,omitempty"`
	MissingSourceOfTruth          []string          `json:"missing_source_of_truth,omitempty"`
	Plan                          *ExecutionPlan    `json:"plan,omitempty"`
	Manifest                      *handoff.Manifest `json:"manifest,omitempty"`
	Warnings                      []handoff.Warning `json:"warnings,omitempty"`
}

type Registry struct{}

func DefaultRegistry() Registry {
	return Registry{}
}

func (Registry) Execute(request Request) Result {
	if request.Action == assessment.ActionPackage {
		return packageMinimalManifest(request)
	}

	contract := mustActionContract(request.Action)
	plan, planErr := providerNeutralBlockedPlan(request)
	if planErr != nil {
		return Result{
			SchemaVersion:                 "matilda_cloud_prep.result_v0",
			Status:                        RunStatusFailed,
			SupportStatus:                 SupportBlocked,
			MutationLevel:                 contract.MutationLevel,
			ActionContract:                contract,
			Code:                          "execution_plan_build_failed",
			Message:                       "Provider-neutral execution plan could not be built.",
			Mutated:                       false,
			ProviderCapabilityImplemented: false,
			Request:                       request,
			SourceHandles:                 providerNeutralSourceHandles(),
			MissingSourceOfTruth: append(providerCapabilityMissingSourceOfTruth(),
				planErr.Error(),
			),
		}
	}

	return Result{
		SchemaVersion:                 "matilda_cloud_prep.result_v0",
		Status:                        RunStatusNotImplemented,
		SupportStatus:                 SupportNotImplemented,
		MutationLevel:                 contract.MutationLevel,
		ActionContract:                contract,
		Code:                          "provider_capability_not_implemented",
		Message:                       "Provider-specific cloud preparation is not implemented in this scaffold.",
		Mutated:                       false,
		ProviderCapabilityImplemented: false,
		Request:                       request,
		SourceHandles:                 providerNeutralSourceHandles(),
		MissingSourceOfTruth:          providerCapabilityMissingSourceOfTruth(),
		Plan:                          &plan,
	}
}

func packageMinimalManifest(request Request) Result {
	contract := mustActionContract(request.Action)
	warnings := []handoff.Warning{
		{
			Code:    "provider_schema_required",
			Message: "A provider-specific handoff schema must be verified before archive generation.",
		},
	}

	manifest := handoff.BuildMinimalManifest(handoff.Request{
		Assessment:       string(request.Goal),
		CollectionPath:   string(request.CollectionPath),
		Provider:         string(request.Provider),
		Action:           string(request.Action),
		RequiredNextStep: "Provider-specific handoff schemas are not implemented in this scaffold.",
		Warnings:         warnings,
	})

	return Result{
		SchemaVersion:                 "matilda_cloud_prep.result_v0",
		Status:                        RunStatusReady,
		SupportStatus:                 SupportGuided,
		MutationLevel:                 contract.MutationLevel,
		ActionContract:                contract,
		Code:                          "minimal_manifest_ready",
		Message:                       "Provider-neutral minimal manifest is ready.",
		Mutated:                       false,
		ProviderCapabilityImplemented: false,
		Request:                       request,
		SourceHandles:                 providerNeutralSourceHandles(),
		MissingSourceOfTruth:          packageSchemaMissingSourceOfTruth(),
		Manifest:                      &manifest,
		Warnings:                      warnings,
	}
}

func mustActionContract(action assessment.Action) ActionContract {
	contract, ok := ActionContractFor(action)
	if !ok {
		return ActionContract{
			Action:         action,
			MutationLevel:  MutationNone,
			Purpose:        "Unsupported action.",
			RequiredResult: "Unsupported actions fail closed.",
			MustNotDo:      []string{"mutate cloud resources"},
		}
	}
	return contract
}

func providerCapabilityMissingSourceOfTruth() []string {
	return []string{
		"Provider-specific Matilda requirements and official provider API evidence are required before this capability can be implemented.",
	}
}

func providerNeutralBlockedPlan(request Request) (ExecutionPlan, error) {
	sourceHandles := providerNeutralSourceHandles()
	missingSourceOfTruth := providerCapabilityMissingSourceOfTruth()
	return BuildExecutionPlan(ExecutionPlanInput{
		Request: request,
		OperatorIdentitySummary: OperatorIdentitySummary{
			IdentityStatus:       "unknown",
			Summary:              "Provider-specific operator identity discovery is not implemented in this scaffold.",
			SourceHandles:        sourceHandles,
			MissingSourceOfTruth: missingSourceOfTruth,
		},
		CoverageRecommendation: CoverageRecommendation{
			CoverageStatus: CoverageUnknown,
			Summary:        "Provider-specific discovery is not implemented in this scaffold.",
		},
		PackageSchemaStatus: PackageSchemaProviderSchemaRequired,
		Steps: []PlanStep{
			{
				Intent:                    PlanStepBlocked,
				Title:                     "Provider capability not implemented",
				Description:               "Provider-specific cloud preparation is not implemented in this scaffold.",
				Reason:                    "Matilda requirements and official provider API evidence must be verified before this capability can run.",
				ApprovalKind:              "not_required",
				CurrentState:              "Provider-specific implementation is unavailable.",
				TargetState:               "Verified provider-specific implementation exists.",
				RequiredPermission:        "Provider-specific permission evidence is required before implementation.",
				CredentialMaterialTouched: false,
				Validation:                "Provider-specific tests and source handles prove the action is supported before it is enabled.",
				Rollback:                  "No cloud change is made by this provider-neutral plan.",
				SourceHandles:             sourceHandles,
				MissingSourceOfTruth:      missingSourceOfTruth,
			},
		},
		Checks: []PlanCheck{
			{
				Status:  CheckFail,
				Title:   "Provider capability check",
				Message: "Provider-specific capability is not implemented.",
				Evidence: []PlanEvidence{
					{Key: "mutated", Value: "false"},
					{Key: "provider_capability_implemented", Value: "false"},
				},
				SourceHandles: sourceHandles,
			},
		},
		SourceHandles:        sourceHandles,
		MissingSourceOfTruth: missingSourceOfTruth,
	})
}

func packageSchemaMissingSourceOfTruth() []string {
	return []string{
		"Provider-specific handoff package schema is required before archive generation.",
	}
}
