package workflow

import "github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"

type RunStatus string

const (
	RunStatusReady          RunStatus = "ready"
	RunStatusManualSteps    RunStatus = "ready_with_manual_steps"
	RunStatusBlocked        RunStatus = "blocked"
	RunStatusFailed         RunStatus = "failed"
	RunStatusNotImplemented RunStatus = "not_implemented"
)

type Status = RunStatus

const (
	StatusReady          = RunStatusReady
	StatusNotImplemented = RunStatusNotImplemented
)

type SupportStatus string

const (
	SupportSupported      SupportStatus = "supported"
	SupportGuided         SupportStatus = "guided"
	SupportBlocked        SupportStatus = "blocked"
	SupportNotImplemented SupportStatus = "not_implemented"
)

type MutationLevel string

const (
	MutationNone      MutationLevel = "none"
	MutationLocalOnly MutationLevel = "local_only"
	MutationCloud     MutationLevel = "cloud"
)

type SourceHandle struct {
	Label string `json:"label"`
	URI   string `json:"uri"`
}

type ActionContract struct {
	Action         assessment.Action `json:"action"`
	MutationLevel  MutationLevel     `json:"mutation_level"`
	Purpose        string            `json:"purpose"`
	RequiredResult string            `json:"required_result"`
	MustNotDo      []string          `json:"must_not_do,omitempty"`
}

func ActionContractFor(action assessment.Action) (ActionContract, bool) {
	contract, ok := actionContracts[action]
	if !ok {
		return ActionContract{}, false
	}
	contract.MustNotDo = append([]string(nil), contract.MustNotDo...)
	return contract, true
}

var actionContracts = map[assessment.Action]ActionContract{
	assessment.ActionPreflight: {
		Action:         assessment.ActionPreflight,
		MutationLevel:  MutationNone,
		Purpose:        "Checks readiness before setup.",
		RequiredResult: "Structured readiness report with checks, blockers, and planned prerequisites.",
		MustNotDo: []string{
			"create cloud resources",
			"update cloud resources",
			"delete cloud resources",
			"package handoff artifacts",
		},
	},
	assessment.ActionApplyPrereqs: {
		Action:         assessment.ActionApplyPrereqs,
		MutationLevel:  MutationCloud,
		Purpose:        "Creates or updates verified cloud-platform-side prerequisites after approval.",
		RequiredResult: "Structured change report with created, updated, skipped, guided, blocked, and failed items.",
		MustNotDo: []string{
			"automate Matilda SaaS portal steps",
			"store secrets",
			"run unsupported provider actions",
			"perform destructive rollback",
		},
	},
	assessment.ActionValidate: {
		Action:         assessment.ActionValidate,
		MutationLevel:  MutationNone,
		Purpose:        "Verifies configured prerequisites after setup.",
		RequiredResult: "Structured validation report proving whether the selected path is satisfied.",
		MustNotDo: []string{
			"mutate cloud resources",
			"infer success from planned state alone",
		},
	},
	assessment.ActionPackage: {
		Action:         assessment.ActionPackage,
		MutationLevel:  MutationLocalOnly,
		Purpose:        "Builds a whitelisted handoff artifact.",
		RequiredResult: "Provider-neutral minimal manifest until a provider-specific package schema is approved.",
		MustNotDo: []string{
			"include credentials",
			"include private keys",
			"include raw logs",
			"include live inventory",
			"include cloud state",
		},
	},
}

type PlanStepIntent string

const (
	PlanStepReuse   PlanStepIntent = "reuse"
	PlanStepRepair  PlanStepIntent = "repair"
	PlanStepCreate  PlanStepIntent = "create"
	PlanStepGuide   PlanStepIntent = "guide"
	PlanStepBlocked PlanStepIntent = "blocked"
	PlanStepSkip    PlanStepIntent = "skip"
)

func PlanStepIntents() []PlanStepIntent {
	return []PlanStepIntent{
		PlanStepReuse,
		PlanStepRepair,
		PlanStepCreate,
		PlanStepGuide,
		PlanStepBlocked,
		PlanStepSkip,
	}
}

type CheckStatus string

const (
	CheckPass    CheckStatus = "pass"
	CheckWarn    CheckStatus = "warn"
	CheckFail    CheckStatus = "fail"
	CheckUnknown CheckStatus = "unknown"
	CheckSkipped CheckStatus = "skipped"
)

func CheckStatuses() []CheckStatus {
	return []CheckStatus{
		CheckPass,
		CheckWarn,
		CheckFail,
		CheckUnknown,
		CheckSkipped,
	}
}

type ApplyOutcome string

const (
	ApplyCreated   ApplyOutcome = "created"
	ApplyUpdated   ApplyOutcome = "updated"
	ApplyUnchanged ApplyOutcome = "unchanged"
	ApplyGuided    ApplyOutcome = "guided"
	ApplyBlocked   ApplyOutcome = "blocked"
	ApplySkipped   ApplyOutcome = "skipped"
	ApplyFailed    ApplyOutcome = "failed"
)

func ApplyOutcomes() []ApplyOutcome {
	return []ApplyOutcome{
		ApplyCreated,
		ApplyUpdated,
		ApplyUnchanged,
		ApplyGuided,
		ApplyBlocked,
		ApplySkipped,
		ApplyFailed,
	}
}

func providerNeutralSourceHandles() []SourceHandle {
	return []SourceHandle{
		{
			Label: "Architecture Workflow",
			URI:   "docs/workflows/ARCHITECTURE.md",
		},
		{
			Label: "Orchestrator Guided Workflow Design",
			URI:   "docs/references/cross-cloud/orchestrator-guided-workflow-design.md",
		},
	}
}
