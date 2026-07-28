package workflow

import (
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestExecutionOptionsNormalizeDirectAWSSelectors(t *testing.T) {
	options, err := NormalizeExecutionOptions(ExecutionOptions{
		InterfaceMode:  InterfaceModeDirect,
		TimeoutSeconds: int((45 * time.Second).Seconds()),
		Selectors: &ExecutionSelectors{
			AWS: &AWSExecutionSelectors{
				Profile:       "default",
				Region:        "us-west-2",
				CUR2ExportRef: "cur2-1234abcd5678ef90",
			},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptions returned error: %v", err)
	}

	if options.SchemaVersion != ExecutionOptionsSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", options.SchemaVersion, ExecutionOptionsSchemaVersion)
	}
	if options.InterfaceMode != InterfaceModeDirect {
		t.Fatalf("InterfaceMode = %q, want %q", options.InterfaceMode, InterfaceModeDirect)
	}
	if options.TimeoutSeconds != 45 {
		t.Fatalf("TimeoutSeconds = %d, want 45", options.TimeoutSeconds)
	}
	if options.Selectors == nil || options.Selectors.AWS == nil {
		t.Fatalf("AWS selectors missing after normalization: %#v", options.Selectors)
	}
	if options.Selectors.AWS.Profile != "default" || options.Selectors.AWS.Region != "us-west-2" || options.Selectors.AWS.CUR2ExportRef != "cur2-1234abcd5678ef90" {
		t.Fatalf("AWS selectors = %#v, want profile, region, and export ref", options.Selectors.AWS)
	}
}

func TestExecutionOptionsRejectUnsafeSelectorValues(t *testing.T) {
	tests := []struct {
		name   string
		option ExecutionOptions
		want   string
	}{
		{
			name: "raw arn export ref",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				CUR2ExportRef: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/live",
			}}},
			want: "cur2_export_ref",
		},
		{
			name: "credential looking profile",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Profile: "secret_key=plain-secret-key",
			}}},
			want: "profile",
		},
		{
			name: "local path region",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Region: "/Users/lly/.aws/config",
			}}},
			want: "region",
		},
		{
			name: "account id looking profile",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Profile: "123456789012",
			}}},
			want: "profile",
		},
		{
			name: "access key looking profile",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Profile: "AKIAIOSFODNN7EXAMPLE",
			}}},
			want: "profile",
		},
		{
			name: "account id looking region",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Region: "123456789012",
			}}},
			want: "region",
		},
		{
			name: "access key looking region",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Region: "AKIAIOSFODNN7EXAMPLE",
			}}},
			want: "region",
		},
		{
			name: "generic absolute path profile",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Profile: "/private/tmp/aws-profile",
			}}},
			want: "profile",
		},
		{
			name: "relative path region",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Region: "./aws-region",
			}}},
			want: "region",
		},
		{
			name: "windows path profile",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				Profile: `C:\Users\customer\.aws\config`,
			}}},
			want: "profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeExecutionOptions(tt.option)
			if err == nil {
				t.Fatal("NormalizeExecutionOptions accepted unsafe selector")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestExecutionOptionsRejectInvalidContractValues(t *testing.T) {
	tests := []struct {
		name   string
		option ExecutionOptions
		want   string
	}{
		{
			name:   "schema version",
			option: ExecutionOptions{SchemaVersion: "wrong"},
			want:   "schema_version",
		},
		{
			name:   "interface mode",
			option: ExecutionOptions{InterfaceMode: InterfaceMode("wizard")},
			want:   "interface_mode",
		},
		{
			name:   "negative timeout",
			option: ExecutionOptions{TimeoutSeconds: -1},
			want:   "timeout_seconds",
		},
		{
			name: "invalid export ref length",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				CUR2ExportRef: "cur2-1234",
			}}},
			want: "cur2_export_ref",
		},
		{
			name: "uppercase export ref",
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
				CUR2ExportRef: "cur2-ABCDEF1234567890",
			}}},
			want: "cur2_export_ref",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeExecutionOptions(tt.option)
			if err == nil {
				t.Fatal("NormalizeExecutionOptions accepted invalid contract value")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestExecutionOptionsDropsEmptySelectors(t *testing.T) {
	options, err := NormalizeExecutionOptions(ExecutionOptions{
		Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{}},
	})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptions returned error: %v", err)
	}
	if options.Selectors != nil {
		t.Fatalf("Selectors = %#v, want nil for empty selectors", options.Selectors)
	}
}
