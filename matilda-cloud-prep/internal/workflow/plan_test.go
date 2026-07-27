package workflow

import (
	"strings"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
)

func TestBuildExecutionPlanCreatesProviderNeutralPlan(t *testing.T) {
	request := samplePlanRequest()
	plan, err := BuildExecutionPlan(sampleExecutionPlanInput(request))
	if err != nil {
		t.Fatalf("BuildExecutionPlan returned error: %v", err)
	}

	if plan.SchemaVersion != "matilda_cloud_prep.execution_plan_v0" {
		t.Fatalf("SchemaVersion = %q, want execution plan v0", plan.SchemaVersion)
	}
	if plan.PlanID == "" {
		t.Fatal("PlanID is empty")
	}
	if plan.PlanGeneratedAt.IsZero() {
		t.Fatal("PlanGeneratedAt is zero")
	}
	if plan.Request != request {
		t.Fatalf("Request = %#v, want %#v", plan.Request, request)
	}
	if plan.OperatorIdentitySummary.IdentityStatus != "unknown" {
		t.Fatalf("OperatorIdentitySummary.IdentityStatus = %q, want unknown", plan.OperatorIdentitySummary.IdentityStatus)
	}
	if plan.CoverageRecommendation.CoverageStatus != CoverageUnknown {
		t.Fatalf("CoverageStatus = %q, want %q", plan.CoverageRecommendation.CoverageStatus, CoverageUnknown)
	}
	if plan.PackageSchemaStatus != PackageSchemaProviderSchemaRequired {
		t.Fatalf("PackageSchemaStatus = %q, want %q", plan.PackageSchemaStatus, PackageSchemaProviderSchemaRequired)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("Steps length = %d, want 1", len(plan.Steps))
	}
	if plan.Steps[0].ID == "" {
		t.Fatal("Plan step ID is empty")
	}
	if plan.Steps[0].Intent != PlanStepBlocked {
		t.Fatalf("Plan step intent = %q, want %q", plan.Steps[0].Intent, PlanStepBlocked)
	}
	if plan.Steps[0].RequiresApproval {
		t.Fatal("blocked step unexpectedly requires approval")
	}
	if plan.Steps[0].CredentialMaterialTouched {
		t.Fatal("blocked step unexpectedly touches credential material")
	}
	if plan.Approval.Approved {
		t.Fatal("execution plan approved itself")
	}
	if !plan.Approval.Blocked {
		t.Fatal("execution plan with blocked step did not mark approval blocked")
	}
	assertSafeSourceHandles(t, plan.SourceHandles)
}

func TestPlanIDIsDeterministicAndUsesReviewedMaterial(t *testing.T) {
	input := sampleExecutionPlanInput(samplePlanRequest())

	first, err := BuildExecutionPlan(input)
	if err != nil {
		t.Fatalf("BuildExecutionPlan(first) returned error: %v", err)
	}
	input.PlanGeneratedAt = input.PlanGeneratedAt.Add(24 * time.Hour)
	second, err := BuildExecutionPlan(input)
	if err != nil {
		t.Fatalf("BuildExecutionPlan(second) returned error: %v", err)
	}
	if first.PlanID != second.PlanID {
		t.Fatalf("PlanID changed when only timestamp changed: %q != %q", first.PlanID, second.PlanID)
	}

	input.Steps[0].Title = "Different reviewed step"
	stepChanged, err := BuildExecutionPlan(input)
	if err != nil {
		t.Fatalf("BuildExecutionPlan(stepChanged) returned error: %v", err)
	}
	if stepChanged.PlanID == first.PlanID {
		t.Fatal("PlanID did not change after material step content changed")
	}

	input = sampleExecutionPlanInput(samplePlanRequest())
	input.PackageSchemaStatus = PackageSchemaMinimalV0
	packageChanged, err := BuildExecutionPlan(input)
	if err != nil {
		t.Fatalf("BuildExecutionPlan(packageChanged) returned error: %v", err)
	}
	if packageChanged.PlanID == first.PlanID {
		t.Fatal("PlanID did not change after package schema status changed")
	}
}

func TestOperatorIdentitySummaryRejectsCredentialMaterial(t *testing.T) {
	for _, unsafeSummary := range []string{
		"token=plain-token",
		"Bearer plain-authorization",
		"client_secret=plain-secret",
		"private_key=/Users/lly/.ssh/key.pem",
		"private key at /home/operator/.ssh/id_rsa",
		"private key at C:\\Users\\operator\\.ssh\\id_rsa",
		"certificate path discovery.pem",
		"arn:aws:iam::123456789012:user/operator",
		"ocid1.tenancy.oc1..example",
		"projects/live-project/serviceAccounts/discovery@example.iam.gserviceaccount.com",
	} {
		t.Run(unsafeSummary, func(t *testing.T) {
			input := sampleExecutionPlanInput(samplePlanRequest())
			input.OperatorIdentitySummary.Summary = unsafeSummary

			_, err := BuildExecutionPlan(input)
			if err == nil {
				t.Fatal("BuildExecutionPlan accepted unsafe operator identity summary")
			}
			if !strings.Contains(err.Error(), "operator_identity_summary") {
				t.Fatalf("error = %q, want operator_identity_summary context", err)
			}
		})
	}
}

func TestBuildExecutionPlanCalculatesStatusCounts(t *testing.T) {
	input := sampleExecutionPlanInput(samplePlanRequest())
	input.Steps = []PlanStep{
		sampleStep(PlanStepBlocked),
		sampleStep(PlanStepCreate),
		sampleStep(PlanStepCreate),
	}
	input.Checks = []PlanCheck{
		sampleCheck(CheckPass),
		sampleCheck(CheckFail),
		sampleCheck(CheckFail),
		sampleCheck(CheckUnknown),
	}

	plan, err := BuildExecutionPlan(input)
	if err != nil {
		t.Fatalf("BuildExecutionPlan returned error: %v", err)
	}

	if got := plan.StatusCounts.StepIntents[PlanStepBlocked]; got != 1 {
		t.Fatalf("blocked step count = %d, want 1", got)
	}
	if got := plan.StatusCounts.StepIntents[PlanStepCreate]; got != 2 {
		t.Fatalf("create step count = %d, want 2", got)
	}
	if got := plan.StatusCounts.CheckStatuses[CheckPass]; got != 1 {
		t.Fatalf("pass check count = %d, want 1", got)
	}
	if got := plan.StatusCounts.CheckStatuses[CheckFail]; got != 2 {
		t.Fatalf("fail check count = %d, want 2", got)
	}
	if got := plan.StatusCounts.CheckStatuses[CheckUnknown]; got != 1 {
		t.Fatalf("unknown check count = %d, want 1", got)
	}
}

func TestPlanStepApprovalRequirementsFollowIntent(t *testing.T) {
	for _, intent := range []PlanStepIntent{PlanStepRepair, PlanStepCreate} {
		t.Run(string(intent), func(t *testing.T) {
			input := sampleExecutionPlanInput(samplePlanRequest())
			input.Steps = []PlanStep{sampleStep(intent)}

			plan, err := BuildExecutionPlan(input)
			if err != nil {
				t.Fatalf("BuildExecutionPlan returned error: %v", err)
			}
			if !plan.Steps[0].RequiresApproval {
				t.Fatalf("%s step did not require approval", intent)
			}
			if !plan.Approval.Required {
				t.Fatalf("%s step did not mark plan approval required", intent)
			}
			if plan.Approval.Approved {
				t.Fatalf("%s step plan approved itself", intent)
			}
			if plan.Steps[0].RequiredPermission == "" {
				t.Fatalf("%s step missing required permission", intent)
			}
			if plan.Steps[0].CurrentState == "" || plan.Steps[0].TargetState == "" {
				t.Fatalf("%s step missing current or target state", intent)
			}
			if plan.Steps[0].Validation == "" || plan.Steps[0].Rollback == "" {
				t.Fatalf("%s step missing validation or rollback metadata", intent)
			}
		})
	}

	for _, intent := range []PlanStepIntent{PlanStepReuse, PlanStepGuide, PlanStepBlocked, PlanStepSkip} {
		t.Run(string(intent), func(t *testing.T) {
			input := sampleExecutionPlanInput(samplePlanRequest())
			input.Steps = []PlanStep{sampleStep(intent)}

			plan, err := BuildExecutionPlan(input)
			if err != nil {
				t.Fatalf("BuildExecutionPlan returned error: %v", err)
			}
			if plan.Steps[0].RequiresApproval {
				t.Fatalf("%s step unexpectedly requires approval", intent)
			}
			if plan.Approval.Required {
				t.Fatalf("%s step unexpectedly marked plan approval required", intent)
			}
		})
	}
}

func TestPlanStepApprovalKindMustMatchIntent(t *testing.T) {
	tests := []struct {
		name         string
		intent       PlanStepIntent
		approvalKind string
	}{
		{name: "blocked cannot be cloud mutation", intent: PlanStepBlocked, approvalKind: "cloud_mutation"},
		{name: "reuse cannot be cloud mutation", intent: PlanStepReuse, approvalKind: "cloud_mutation"},
		{name: "guide cannot be cloud mutation", intent: PlanStepGuide, approvalKind: "cloud_mutation"},
		{name: "skip cannot be cloud mutation", intent: PlanStepSkip, approvalKind: "cloud_mutation"},
		{name: "create cannot be not required", intent: PlanStepCreate, approvalKind: "not_required"},
		{name: "repair cannot be not required", intent: PlanStepRepair, approvalKind: "not_required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := sampleExecutionPlanInput(samplePlanRequest())
			input.Steps = []PlanStep{sampleStep(tt.intent)}
			input.Steps[0].ApprovalKind = tt.approvalKind

			_, err := BuildExecutionPlan(input)
			if err == nil {
				t.Fatal("BuildExecutionPlan accepted mismatched approval kind")
			}
			if !strings.Contains(err.Error(), "approval_kind") {
				t.Fatalf("error = %q, want approval_kind context", err)
			}
		})
	}
}

func TestBuildExecutionPlanDerivesStepIDsFromContent(t *testing.T) {
	withCallerID := sampleExecutionPlanInput(samplePlanRequest())
	withCallerID.Steps[0].ID = "caller-controlled-id"
	withCallerIDPlan, err := BuildExecutionPlan(withCallerID)
	if err != nil {
		t.Fatalf("BuildExecutionPlan(withCallerID) returned error: %v", err)
	}

	withoutCallerID := sampleExecutionPlanInput(samplePlanRequest())
	withoutCallerIDPlan, err := BuildExecutionPlan(withoutCallerID)
	if err != nil {
		t.Fatalf("BuildExecutionPlan(withoutCallerID) returned error: %v", err)
	}

	if withCallerIDPlan.Steps[0].ID == "caller-controlled-id" {
		t.Fatal("caller-provided step ID was trusted")
	}
	if withCallerIDPlan.Steps[0].ID != withoutCallerIDPlan.Steps[0].ID {
		t.Fatalf("derived step ID = %q, want %q", withCallerIDPlan.Steps[0].ID, withoutCallerIDPlan.Steps[0].ID)
	}
	if withCallerIDPlan.PlanID != withoutCallerIDPlan.PlanID {
		t.Fatalf("plan ID changed when only caller-provided step ID changed: %q != %q", withCallerIDPlan.PlanID, withoutCallerIDPlan.PlanID)
	}
}

func TestBuildExecutionPlanRejectsUnsafeSourceHandles(t *testing.T) {
	for _, uri := range []string{
		"/Users/lly/Documents/Development/docs/matildaDocs/private.md",
		"../docs/references/private.md",
		"docs/../private.md",
		"docs/references/private_key.md",
		"https://example.com/access_key",
		"https://example.com/reference.md",
		"https://docs.aws.amazon.com/sdk-for-go/",
		"https://learn.microsoft.com/en-us/azure/developer/go/",
		"https://cloud.google.com/go/docs/reference",
		"https://docs.oracle.com/en-us/iaas/Content/API/Concepts/sdkconfig.htm",
		"https://pkg.go.dev/cloud.google.com/go",
		"https://go.dev/doc/",
		"https://github.com/aws/aws-sdk-go-v2",
		"ftp://docs.example.com/reference.md",
	} {
		t.Run(uri, func(t *testing.T) {
			input := sampleExecutionPlanInput(samplePlanRequest())
			input.SourceHandles = []SourceHandle{{Label: "Unsafe", URI: uri}}

			_, err := BuildExecutionPlan(input)
			if err == nil {
				t.Fatal("BuildExecutionPlan accepted unsafe source handle")
			}
			if !strings.Contains(err.Error(), "source handle") {
				t.Fatalf("error = %q, want source handle context", err)
			}
		})
	}
}

func TestBuildExecutionPlanAcceptsCachedSourceHandleReferences(t *testing.T) {
	for _, uri := range []string{
		"docs/workflows/ARCHITECTURE.md",
		"docs/references/cross-cloud/orchestrator-guided-workflow-design.md",
		"docs/references/aws/official-implementation-references.md",
	} {
		t.Run(uri, func(t *testing.T) {
			input := sampleExecutionPlanInput(samplePlanRequest())
			input.SourceHandles = []SourceHandle{{Label: "Cached", URI: uri}}

			plan, err := BuildExecutionPlan(input)
			if err != nil {
				t.Fatalf("BuildExecutionPlan returned error: %v", err)
			}
			if plan.SourceHandles[0].URI != uri {
				t.Fatalf("source handle URI = %q, want %q", plan.SourceHandles[0].URI, uri)
			}
		})
	}
}

func TestBuildExecutionPlanRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ExecutionPlanInput)
		wantErr string
	}{
		{
			name: "invalid package schema status",
			mutate: func(input *ExecutionPlanInput) {
				input.PackageSchemaStatus = PackageSchemaStatus("provider_ready")
			},
			wantErr: "package_schema_status",
		},
		{
			name: "invalid coverage status",
			mutate: func(input *ExecutionPlanInput) {
				input.CoverageRecommendation.CoverageStatus = CoverageStatus("complete")
			},
			wantErr: "coverage_recommendation",
		},
		{
			name: "empty coverage summary",
			mutate: func(input *ExecutionPlanInput) {
				input.CoverageRecommendation.Summary = ""
			},
			wantErr: "coverage_recommendation",
		},
		{
			name: "empty operator identity status",
			mutate: func(input *ExecutionPlanInput) {
				input.OperatorIdentitySummary.IdentityStatus = ""
			},
			wantErr: "operator_identity_summary",
		},
		{
			name: "empty operator identity summary",
			mutate: func(input *ExecutionPlanInput) {
				input.OperatorIdentitySummary.Summary = ""
			},
			wantErr: "operator_identity_summary",
		},
		{
			name: "empty operator source handles",
			mutate: func(input *ExecutionPlanInput) {
				input.OperatorIdentitySummary.SourceHandles = nil
			},
			wantErr: "source handle",
		},
		{
			name: "empty plan source handles",
			mutate: func(input *ExecutionPlanInput) {
				input.SourceHandles = nil
			},
			wantErr: "source handle",
		},
		{
			name: "empty source handle label",
			mutate: func(input *ExecutionPlanInput) {
				input.SourceHandles = []SourceHandle{{URI: "docs/workflows/ARCHITECTURE.md"}}
			},
			wantErr: "empty label",
		},
		{
			name: "empty source handle uri",
			mutate: func(input *ExecutionPlanInput) {
				input.SourceHandles = []SourceHandle{{Label: "Architecture"}}
			},
			wantErr: "empty URI",
		},
		{
			name: "no plan steps",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps = nil
			},
			wantErr: "steps are required",
		},
		{
			name: "empty step title",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].Title = ""
			},
			wantErr: "plan step title",
		},
		{
			name: "empty step description",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].Description = ""
			},
			wantErr: "plan step description",
		},
		{
			name: "empty step reason",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].Reason = ""
			},
			wantErr: "plan step reason",
		},
		{
			name: "empty step approval kind",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].ApprovalKind = ""
			},
			wantErr: "plan step approval_kind",
		},
		{
			name: "empty step current state",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].CurrentState = ""
			},
			wantErr: "plan step current_state",
		},
		{
			name: "empty step target state",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].TargetState = ""
			},
			wantErr: "plan step target_state",
		},
		{
			name: "empty step required permission",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].RequiredPermission = ""
			},
			wantErr: "plan step required_permission",
		},
		{
			name: "empty step validation",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].Validation = ""
			},
			wantErr: "plan step validation",
		},
		{
			name: "empty step rollback",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].Rollback = ""
			},
			wantErr: "plan step rollback",
		},
		{
			name: "unknown step intent",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps[0].Intent = PlanStepIntent("discover")
			},
			wantErr: "unknown plan step intent",
		},
		{
			name: "mutating step missing validation metadata",
			mutate: func(input *ExecutionPlanInput) {
				input.Steps = []PlanStep{sampleStep(PlanStepCreate)}
				input.Steps[0].Validation = ""
			},
			wantErr: "plan step validation",
		},
		{
			name: "empty check title",
			mutate: func(input *ExecutionPlanInput) {
				input.Checks[0].Title = ""
			},
			wantErr: "plan check title",
		},
		{
			name: "empty check message",
			mutate: func(input *ExecutionPlanInput) {
				input.Checks[0].Message = ""
			},
			wantErr: "plan check message",
		},
		{
			name: "empty check evidence",
			mutate: func(input *ExecutionPlanInput) {
				input.Checks[0].Evidence = nil
			},
			wantErr: "plan check evidence",
		},
		{
			name: "empty check evidence key",
			mutate: func(input *ExecutionPlanInput) {
				input.Checks[0].Evidence = []PlanEvidence{{Value: "false"}}
			},
			wantErr: "plan evidence key",
		},
		{
			name: "empty check evidence value",
			mutate: func(input *ExecutionPlanInput) {
				input.Checks[0].Evidence = []PlanEvidence{{Key: "mutated"}}
			},
			wantErr: "plan evidence value",
		},
		{
			name: "unknown check status",
			mutate: func(input *ExecutionPlanInput) {
				input.Checks[0].Status = CheckStatus("ok")
			},
			wantErr: "unknown check status",
		},
		{
			name: "unsafe check evidence",
			mutate: func(input *ExecutionPlanInput) {
				input.Checks[0].Evidence = []PlanEvidence{{Key: "client_secret", Value: "plain-secret"}}
			},
			wantErr: "plan evidence",
		},
		{
			name: "unsafe missing source of truth",
			mutate: func(input *ExecutionPlanInput) {
				input.MissingSourceOfTruth = []string{"read /Users/lly/private.txt"}
			},
			wantErr: "missing_source_of_truth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := sampleExecutionPlanInput(samplePlanRequest())
			tt.mutate(&input)

			_, err := BuildExecutionPlan(input)
			if err == nil {
				t.Fatal("BuildExecutionPlan succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err, tt.wantErr)
			}
		})
	}
}

func samplePlanRequest() Request {
	return Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionAPI,
		Provider:       assessment.ProviderGCP,
		Action:         assessment.ActionPreflight,
	}
}

func sampleExecutionPlanInput(request Request) ExecutionPlanInput {
	handles := providerNeutralSourceHandles()
	return ExecutionPlanInput{
		PlanGeneratedAt: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
		Request:         request,
		OperatorIdentitySummary: OperatorIdentitySummary{
			IdentityStatus:       "unknown",
			Summary:              "Provider-specific operator identity discovery is not implemented.",
			SourceHandles:        handles,
			MissingSourceOfTruth: []string{"Provider-specific operator identity evidence is required before discovery."},
		},
		CoverageRecommendation: CoverageRecommendation{
			CoverageStatus: CoverageUnknown,
			Summary:        "Provider-specific discovery is not implemented in this scaffold.",
		},
		PackageSchemaStatus:  PackageSchemaProviderSchemaRequired,
		Steps:                []PlanStep{sampleStep(PlanStepBlocked)},
		Checks:               []PlanCheck{sampleCheck(CheckFail)},
		SourceHandles:        handles,
		MissingSourceOfTruth: []string{"Provider-specific Matilda requirements and official provider API evidence are required before this capability can be implemented."},
	}
}

func sampleStep(intent PlanStepIntent) PlanStep {
	return PlanStep{
		Intent:                    intent,
		Title:                     "Provider capability not implemented",
		Description:               "Provider-specific cloud preparation is not implemented in this scaffold.",
		Reason:                    "Matilda requirements and official provider API evidence must be verified first.",
		ApprovalKind:              sampleApprovalKind(intent),
		CurrentState:              "Provider-specific implementation is unavailable.",
		TargetState:               "Verified provider-specific implementation exists.",
		RequiredPermission:        "Provider-specific permission evidence is required before implementation.",
		CredentialMaterialTouched: false,
		Validation:                "Provider-specific tests and source handles prove the action is supported.",
		Rollback:                  "No cloud change is made by this provider-neutral plan.",
		SourceHandles:             providerNeutralSourceHandles(),
		MissingSourceOfTruth:      []string{"Provider-specific source of truth is required before implementation."},
	}
}

func sampleApprovalKind(intent PlanStepIntent) string {
	switch intent {
	case PlanStepCreate, PlanStepRepair:
		return "cloud_mutation"
	default:
		return "not_required"
	}
}

func sampleCheck(status CheckStatus) PlanCheck {
	return PlanCheck{
		Status:  status,
		Title:   "Provider capability check",
		Message: "Provider-specific capability is not implemented.",
		Evidence: []PlanEvidence{
			{Key: "mutated", Value: "false"},
		},
		SourceHandles: providerNeutralSourceHandles(),
	}
}
