package workflow

import (
	"context"
	"time"

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
	ExecutionOptions              ExecutionOptions  `json:"execution_options"`
	SourceHandles                 []SourceHandle    `json:"source_handles,omitempty"`
	MissingSourceOfTruth          []string          `json:"missing_source_of_truth,omitempty"`
	Plan                          *ExecutionPlan    `json:"plan,omitempty"`
	Manifest                      *handoff.Manifest `json:"manifest,omitempty"`
	Handoff                       *handoff.Output   `json:"handoff,omitempty"`
	Warnings                      []handoff.Warning `json:"warnings,omitempty"`
}

type Registry struct {
	runners map[Request]CapabilityRunner
}

func DefaultRegistry() Registry {
	return Registry{}
}

func (registry Registry) Execute(request Request) Result {
	return registry.ExecuteContext(context.Background(), request)
}

func (registry Registry) ExecuteContext(ctx context.Context, request Request, requestedOptions ...ExecutionOptions) Result {
	options := DefaultExecutionOptions()
	if len(requestedOptions) > 0 {
		var err error
		options, err = NormalizeExecutionOptionsForRequest(request, requestedOptions[0])
		if err != nil {
			return executionOptionsInvalidResult(request, options, err)
		}
	}

	var cancel context.CancelFunc
	ctx, cancel = contextWithExecutionTimeout(ctx, options)
	defer cancel()

	if runner := registry.runners[request]; runner != nil {
		return buildCapabilityResult(request, options, runner.Run(ctx, request, options))
	}
	if request.Action == assessment.ActionPackage {
		return packageMinimalManifest(request, options)
	}

	contract := mustActionContract(request.Action)
	plan, planErr := providerNeutralBlockedPlan(request, options)
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
			ExecutionOptions:              options,
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
		ExecutionOptions:              options,
		SourceHandles:                 providerNeutralSourceHandles(),
		MissingSourceOfTruth:          providerCapabilityMissingSourceOfTruth(),
		Plan:                          &plan,
	}
}

func contextWithExecutionTimeout(ctx context.Context, options ExecutionOptions) (context.Context, context.CancelFunc) {
	if options.TimeoutSeconds <= 0 {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Duration(options.TimeoutSeconds)*time.Second)
}

func executionOptionsInvalidResult(request Request, options ExecutionOptions, optionsErr error) Result {
	contract := mustActionContract(request.Action)
	return Result{
		SchemaVersion:                 "matilda_cloud_prep.result_v0",
		Status:                        RunStatusFailed,
		SupportStatus:                 SupportBlocked,
		MutationLevel:                 contract.MutationLevel,
		ActionContract:                contract,
		Code:                          "execution_options_invalid",
		Message:                       "Execution options are invalid or unsafe.",
		Mutated:                       false,
		ProviderCapabilityImplemented: false,
		Request:                       request,
		ExecutionOptions:              options,
		SourceHandles:                 providerNeutralSourceHandles(),
		MissingSourceOfTruth: []string{
			optionsErr.Error(),
		},
	}
}

func packageMinimalManifest(request Request, options ExecutionOptions) Result {
	contract := mustActionContract(request.Action)
	resultOptions := resultExecutionOptions(request, options)
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
		ExecutionOptions:              resultOptions,
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

func providerNeutralBlockedPlan(request Request, options ExecutionOptions) (ExecutionPlan, error) {
	sourceHandles := providerNeutralSourceHandles()
	missingSourceOfTruth := providerCapabilityMissingSourceOfTruth()
	return BuildExecutionPlan(ExecutionPlanInput{
		Request:          request,
		ExecutionOptions: options,
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
