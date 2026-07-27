package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/handoff"
)

const resultSchemaVersion = "matilda_cloud_prep.result_v0"

type CapabilityRunner interface {
	Run(context.Context, Request) CapabilityReport
}

type RunnerFunc func(context.Context, Request) CapabilityReport

func (f RunnerFunc) Run(ctx context.Context, request Request) CapabilityReport {
	return f(ctx, request)
}

type Capability struct {
	Request Request
	Runner  CapabilityRunner
}

type CapabilityReport struct {
	Status               RunStatus
	SupportStatus        SupportStatus
	Code                 string
	Message              string
	Mutated              bool
	SourceHandles        []SourceHandle
	MissingSourceOfTruth []string
	PlanInput            *ExecutionPlanInput
	Manifest             *handoff.Manifest
	Warnings             []handoff.Warning
}

func NewRegistry(capabilities ...Capability) (Registry, error) {
	runners := map[Request]CapabilityRunner{}
	for _, capability := range capabilities {
		if _, ok := ActionContractFor(capability.Request.Action); !ok {
			return Registry{}, fmt.Errorf("capability action %q is not supported", capability.Request.Action)
		}
		if capability.Request.Action == assessment.ActionPackage {
			return Registry{}, fmt.Errorf("provider package capabilities require an approved package schema first")
		}
		if capability.Runner == nil {
			return Registry{}, fmt.Errorf("capability runner is required")
		}
		if _, exists := runners[capability.Request]; exists {
			return Registry{}, fmt.Errorf("duplicate capability registration for %s", capabilityKey(capability.Request))
		}
		runners[capability.Request] = capability.Runner
	}
	return Registry{runners: runners}, nil
}

func buildCapabilityResult(request Request, report CapabilityReport) Result {
	contract := mustActionContract(request.Action)
	sourceHandles, err := safeSourceHandles("capability result source handles", report.SourceHandles)
	if err != nil {
		return invalidCapabilityResult(request, contract)
	}
	if err := validateCapabilityReport(contract, report); err != nil {
		return invalidCapabilityResult(request, contract)
	}
	missingSourceOfTruth, err := safeStringList("capability result missing_source_of_truth", report.MissingSourceOfTruth)
	if err != nil {
		return invalidCapabilityResult(request, contract)
	}
	warnings, err := safeWarnings(report.Warnings)
	if err != nil {
		return invalidCapabilityResult(request, contract)
	}
	if report.PlanInput == nil {
		return invalidCapabilityResult(request, contract)
	}

	planInput := *report.PlanInput
	planInput.Request = request
	plan, err := BuildExecutionPlan(planInput)
	if err != nil {
		return invalidCapabilityResult(request, contract)
	}

	return Result{
		SchemaVersion:                 resultSchemaVersion,
		Status:                        report.Status,
		SupportStatus:                 report.SupportStatus,
		MutationLevel:                 contract.MutationLevel,
		ActionContract:                contract,
		Code:                          strings.TrimSpace(report.Code),
		Message:                       strings.TrimSpace(report.Message),
		Mutated:                       report.Mutated,
		ProviderCapabilityImplemented: true,
		Request:                       request,
		SourceHandles:                 sourceHandles,
		MissingSourceOfTruth:          missingSourceOfTruth,
		Plan:                          &plan,
		Warnings:                      warnings,
	}
}

func validateCapabilityReport(contract ActionContract, report CapabilityReport) error {
	if err := validateRunStatus(report.Status); err != nil {
		return err
	}
	if err := validateSupportStatus(report.SupportStatus); err != nil {
		return err
	}
	if strings.TrimSpace(report.Code) == "" {
		return fmt.Errorf("capability result code is required")
	}
	if strings.TrimSpace(report.Message) == "" {
		return fmt.Errorf("capability result message is required")
	}
	if err := ensureSafeText("capability result", string(report.Status), string(report.SupportStatus), report.Code, report.Message); err != nil {
		return err
	}
	if contract.MutationLevel == MutationNone && report.Mutated {
		return fmt.Errorf("read-only capability result reported mutation")
	}
	if report.Manifest != nil {
		return fmt.Errorf("provider manifest requires an approved package schema")
	}
	return nil
}

func validateRunStatus(status RunStatus) error {
	switch status {
	case RunStatusReady, RunStatusManualSteps, RunStatusBlocked, RunStatusFailed, RunStatusNotImplemented:
		return nil
	default:
		return fmt.Errorf("unknown run status %q", status)
	}
}

func validateSupportStatus(status SupportStatus) error {
	switch status {
	case SupportSupported, SupportGuided, SupportBlocked, SupportNotImplemented:
		return nil
	default:
		return fmt.Errorf("unknown support status %q", status)
	}
}

func safeWarnings(warnings []handoff.Warning) ([]handoff.Warning, error) {
	if len(warnings) == 0 {
		return nil, nil
	}

	copied := make([]handoff.Warning, len(warnings))
	for i, warning := range warnings {
		if strings.TrimSpace(warning.Code) == "" {
			return nil, fmt.Errorf("warning %d code is required", i)
		}
		if strings.TrimSpace(warning.Message) == "" {
			return nil, fmt.Errorf("warning %d message is required", i)
		}
		if err := ensureSafeText("warning", warning.Code, warning.Message); err != nil {
			return nil, err
		}
		copied[i] = warning
	}
	return copied, nil
}

func invalidCapabilityResult(request Request, contract ActionContract) Result {
	return Result{
		SchemaVersion:                 resultSchemaVersion,
		Status:                        RunStatusFailed,
		SupportStatus:                 SupportBlocked,
		MutationLevel:                 contract.MutationLevel,
		ActionContract:                contract,
		Code:                          "provider_capability_result_invalid",
		Message:                       "Provider capability returned unsafe or invalid output.",
		Mutated:                       false,
		ProviderCapabilityImplemented: false,
		Request:                       request,
		SourceHandles:                 providerNeutralSourceHandles(),
		MissingSourceOfTruth: []string{
			"Provider capability output must pass cached-source-handle, mutation, and safe-text validation.",
		},
	}
}

func capabilityKey(request Request) string {
	if request.Goal == assessment.DeepDiscovery {
		return fmt.Sprintf("%s/%s/%s", request.Goal, request.Provider, request.Action)
	}
	return fmt.Sprintf("%s/%s/%s/%s", request.Goal, request.CollectionPath, request.Provider, request.Action)
}
