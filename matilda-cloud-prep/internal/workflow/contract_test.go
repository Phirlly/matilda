package workflow

import (
	"reflect"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
)

func TestActionContractsHaveExpectedMutationLevels(t *testing.T) {
	tests := []struct {
		action   assessment.Action
		mutation MutationLevel
	}{
		{action: assessment.ActionPreflight, mutation: MutationNone},
		{action: assessment.ActionApplyPrereqs, mutation: MutationCloud},
		{action: assessment.ActionValidate, mutation: MutationNone},
		{action: assessment.ActionPackage, mutation: MutationLocalOnly},
	}

	for _, test := range tests {
		t.Run(string(test.action), func(t *testing.T) {
			contract, ok := ActionContractFor(test.action)
			if !ok {
				t.Fatalf("ActionContractFor(%q) returned ok=false", test.action)
			}
			if contract.Action != test.action {
				t.Fatalf("Action = %q, want %q", contract.Action, test.action)
			}
			if contract.MutationLevel != test.mutation {
				t.Fatalf("MutationLevel = %q, want %q", contract.MutationLevel, test.mutation)
			}
			if contract.Purpose == "" {
				t.Fatal("Purpose is empty")
			}
			if contract.RequiredResult == "" {
				t.Fatal("RequiredResult is empty")
			}
			if len(contract.MustNotDo) == 0 {
				t.Fatal("MustNotDo is empty")
			}
		})
	}
}

func TestNormalizedWorkflowTermsAreStable(t *testing.T) {
	if got, want := PlanStepIntents(), []PlanStepIntent{
		PlanStepReuse,
		PlanStepRepair,
		PlanStepCreate,
		PlanStepGuide,
		PlanStepBlocked,
		PlanStepSkip,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanStepIntents() = %#v, want %#v", got, want)
	}

	if got, want := CheckStatuses(), []CheckStatus{
		CheckPass,
		CheckWarn,
		CheckFail,
		CheckUnknown,
		CheckSkipped,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CheckStatuses() = %#v, want %#v", got, want)
	}

	if got, want := ApplyOutcomes(), []ApplyOutcome{
		ApplyCreated,
		ApplyUpdated,
		ApplyUnchanged,
		ApplyGuided,
		ApplyBlocked,
		ApplySkipped,
		ApplyFailed,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplyOutcomes() = %#v, want %#v", got, want)
	}
}
