package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const executionPlanSchemaVersion = "matilda_cloud_prep.execution_plan_v0"

type CoverageStatus string

const (
	CoverageUnknown CoverageStatus = "unknown"
)

type PackageSchemaStatus string

const (
	PackageSchemaNone                   PackageSchemaStatus = "none"
	PackageSchemaMinimalV0              PackageSchemaStatus = "minimal_v0"
	PackageSchemaProviderSchemaRequired PackageSchemaStatus = "provider_schema_required"
)

const (
	approvalKindCloudMutation = "cloud_mutation"
	approvalKindNotRequired   = "not_required"
)

type ExecutionPlanInput struct {
	PlanGeneratedAt         time.Time
	Request                 Request
	OperatorIdentitySummary OperatorIdentitySummary
	CoverageRecommendation  CoverageRecommendation
	PackageSchemaStatus     PackageSchemaStatus
	Steps                   []PlanStep
	Checks                  []PlanCheck
	SourceHandles           []SourceHandle
	MissingSourceOfTruth    []string
}

type ExecutionPlan struct {
	SchemaVersion           string                  `json:"schema_version"`
	PlanID                  string                  `json:"plan_id"`
	PlanGeneratedAt         time.Time               `json:"plan_generated_at"`
	Request                 Request                 `json:"request"`
	OperatorIdentitySummary OperatorIdentitySummary `json:"operator_identity_summary"`
	CoverageRecommendation  CoverageRecommendation  `json:"coverage_recommendation"`
	PackageSchemaStatus     PackageSchemaStatus     `json:"package_schema_status"`
	Steps                   []PlanStep              `json:"steps"`
	Checks                  []PlanCheck             `json:"checks"`
	StatusCounts            PlanStatusCounts        `json:"status_counts"`
	SourceHandles           []SourceHandle          `json:"source_handles"`
	MissingSourceOfTruth    []string                `json:"missing_source_of_truth"`
	Approval                ApprovalSummary         `json:"approval"`
}

type OperatorIdentitySummary struct {
	IdentityStatus       string         `json:"identity_status"`
	Summary              string         `json:"summary"`
	SourceHandles        []SourceHandle `json:"source_handles,omitempty"`
	MissingSourceOfTruth []string       `json:"missing_source_of_truth,omitempty"`
}

type CoverageRecommendation struct {
	CoverageStatus CoverageStatus `json:"coverage_status"`
	Summary        string         `json:"summary"`
}

type PlanStep struct {
	ID                        string         `json:"id"`
	Intent                    PlanStepIntent `json:"intent"`
	Title                     string         `json:"title"`
	Description               string         `json:"description"`
	Reason                    string         `json:"reason"`
	RequiresApproval          bool           `json:"requires_approval"`
	ApprovalKind              string         `json:"approval_kind"`
	CurrentState              string         `json:"current_state"`
	TargetState               string         `json:"target_state"`
	RequiredPermission        string         `json:"required_permission"`
	CredentialMaterialTouched bool           `json:"credential_material_touched"`
	Validation                string         `json:"validation"`
	Rollback                  string         `json:"rollback"`
	SourceHandles             []SourceHandle `json:"source_handles"`
	MissingSourceOfTruth      []string       `json:"missing_source_of_truth,omitempty"`
}

type PlanCheck struct {
	ID            string         `json:"id"`
	Status        CheckStatus    `json:"status"`
	Title         string         `json:"title"`
	Message       string         `json:"message"`
	Evidence      []PlanEvidence `json:"evidence,omitempty"`
	SourceHandles []SourceHandle `json:"source_handles"`
}

type PlanEvidence struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PlanStatusCounts struct {
	StepIntents   map[PlanStepIntent]int `json:"step_intents"`
	CheckStatuses map[CheckStatus]int    `json:"check_statuses"`
}

type ApprovalSummary struct {
	Required bool   `json:"required"`
	Approved bool   `json:"approved"`
	Blocked  bool   `json:"blocked"`
	Reason   string `json:"reason"`
}

func BuildExecutionPlan(input ExecutionPlanInput) (ExecutionPlan, error) {
	if err := validatePackageSchemaStatus(input.PackageSchemaStatus); err != nil {
		return ExecutionPlan{}, err
	}
	if err := validateCoverageRecommendation(input.CoverageRecommendation); err != nil {
		return ExecutionPlan{}, err
	}

	sourceHandles, err := safeSourceHandles("source_handles", input.SourceHandles)
	if err != nil {
		return ExecutionPlan{}, err
	}
	operatorSummary, err := safeOperatorIdentitySummary(input.OperatorIdentitySummary)
	if err != nil {
		return ExecutionPlan{}, err
	}
	steps, err := safePlanSteps(input.Steps)
	if err != nil {
		return ExecutionPlan{}, err
	}
	checks, err := safePlanChecks(input.Checks)
	if err != nil {
		return ExecutionPlan{}, err
	}
	missingSourceOfTruth, err := safeStringList("missing_source_of_truth", input.MissingSourceOfTruth)
	if err != nil {
		return ExecutionPlan{}, err
	}

	generatedAt := input.PlanGeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}

	plan := ExecutionPlan{
		SchemaVersion:           executionPlanSchemaVersion,
		PlanGeneratedAt:         generatedAt.UTC(),
		Request:                 input.Request,
		OperatorIdentitySummary: operatorSummary,
		CoverageRecommendation:  input.CoverageRecommendation,
		PackageSchemaStatus:     input.PackageSchemaStatus,
		Steps:                   steps,
		Checks:                  checks,
		StatusCounts:            countPlanStatuses(steps, checks),
		SourceHandles:           sourceHandles,
		MissingSourceOfTruth:    missingSourceOfTruth,
		Approval:                approvalSummaryFor(steps),
	}
	plan.PlanID = stableID("plan", planIDMaterial(plan))
	return plan, nil
}

func safeOperatorIdentitySummary(summary OperatorIdentitySummary) (OperatorIdentitySummary, error) {
	if strings.TrimSpace(summary.IdentityStatus) == "" {
		return OperatorIdentitySummary{}, fmt.Errorf("operator_identity_summary identity_status is required")
	}
	if strings.TrimSpace(summary.Summary) == "" {
		return OperatorIdentitySummary{}, fmt.Errorf("operator_identity_summary summary is required")
	}
	if err := ensureSafeText("operator_identity_summary", summary.IdentityStatus, summary.Summary); err != nil {
		return OperatorIdentitySummary{}, err
	}
	handles, err := safeSourceHandles("operator_identity_summary source handles", summary.SourceHandles)
	if err != nil {
		return OperatorIdentitySummary{}, err
	}
	missing, err := safeStringList("operator_identity_summary missing_source_of_truth", summary.MissingSourceOfTruth)
	if err != nil {
		return OperatorIdentitySummary{}, err
	}
	summary.SourceHandles = handles
	summary.MissingSourceOfTruth = missing
	return summary, nil
}

func validateCoverageRecommendation(recommendation CoverageRecommendation) error {
	if recommendation.CoverageStatus != CoverageUnknown {
		return fmt.Errorf("coverage_recommendation coverage_status = %q, want %q", recommendation.CoverageStatus, CoverageUnknown)
	}
	if strings.TrimSpace(recommendation.Summary) == "" {
		return fmt.Errorf("coverage_recommendation summary is required")
	}
	return ensureSafeText("coverage_recommendation", string(recommendation.CoverageStatus), recommendation.Summary)
}

func validatePackageSchemaStatus(status PackageSchemaStatus) error {
	switch status {
	case PackageSchemaNone, PackageSchemaMinimalV0, PackageSchemaProviderSchemaRequired:
		return nil
	default:
		return fmt.Errorf("package_schema_status = %q, want one of none, minimal_v0, provider_schema_required", status)
	}
}

func safePlanSteps(steps []PlanStep) ([]PlanStep, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("steps are required")
	}

	copied := make([]PlanStep, len(steps))
	for i, step := range steps {
		if err := validatePlanStepIntent(step.Intent); err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}
		step.ApprovalKind = strings.TrimSpace(step.ApprovalKind)
		step.RequiresApproval = stepRequiresApproval(step.Intent)
		if err := requirePlanStepExplanations(step); err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}
		if err := validateApprovalKindForIntent(step); err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}
		if err := ensureSafeText("plan step", string(step.Intent), step.Title, step.Description, step.Reason, step.ApprovalKind, step.CurrentState, step.TargetState, step.RequiredPermission, step.Validation, step.Rollback); err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}
		handles, err := safeSourceHandles("plan step source handles", step.SourceHandles)
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}
		missing, err := safeStringList("plan step missing_source_of_truth", step.MissingSourceOfTruth)
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}

		step.SourceHandles = handles
		step.MissingSourceOfTruth = missing
		step.ID = stableID("step", stepIDMaterial(step))
		copied[i] = step
	}
	return copied, nil
}

func safePlanChecks(checks []PlanCheck) ([]PlanCheck, error) {
	copied := make([]PlanCheck, len(checks))
	for i, check := range checks {
		if err := validateCheckStatus(check.Status); err != nil {
			return nil, fmt.Errorf("check %d: %w", i, err)
		}
		if strings.TrimSpace(check.Title) == "" {
			return nil, fmt.Errorf("check %d: plan check title is required", i)
		}
		if strings.TrimSpace(check.Message) == "" {
			return nil, fmt.Errorf("check %d: plan check message is required", i)
		}
		if err := ensureSafeText("plan check", check.ID, string(check.Status), check.Title, check.Message); err != nil {
			return nil, fmt.Errorf("check %d: %w", i, err)
		}
		if len(check.Evidence) == 0 {
			return nil, fmt.Errorf("check %d: plan check evidence is required", i)
		}
		for _, evidence := range check.Evidence {
			if strings.TrimSpace(evidence.Key) == "" {
				return nil, fmt.Errorf("check %d: plan evidence key is required", i)
			}
			if strings.TrimSpace(evidence.Value) == "" {
				return nil, fmt.Errorf("check %d: plan evidence value is required", i)
			}
			if err := ensureSafeText("plan evidence", evidence.Key, evidence.Value); err != nil {
				return nil, fmt.Errorf("check %d: %w", i, err)
			}
		}
		handles, err := safeSourceHandles("plan check source handles", check.SourceHandles)
		if err != nil {
			return nil, fmt.Errorf("check %d: %w", i, err)
		}
		check.SourceHandles = handles
		if strings.TrimSpace(check.ID) == "" {
			check.ID = stableID("check", checkIDMaterial(check))
		}
		copied[i] = check
	}
	return copied, nil
}

func safeSourceHandles(context string, handles []SourceHandle) ([]SourceHandle, error) {
	if len(handles) == 0 {
		return nil, fmt.Errorf("%s: source handle is required", context)
	}

	copied := make([]SourceHandle, len(handles))
	for i, handle := range handles {
		if strings.TrimSpace(handle.Label) == "" {
			return nil, fmt.Errorf("%s: source handle %d has empty label", context, i)
		}
		if strings.TrimSpace(handle.URI) == "" {
			return nil, fmt.Errorf("%s: source handle %d has empty URI", context, i)
		}
		if strings.HasPrefix(handle.URI, "/") || strings.Contains(handle.URI, "/Users/") {
			return nil, fmt.Errorf("%s: source handle %d must use a cached docs/ relative path", context, i)
		}
		if err := validateSourceHandleURI(handle.URI); err != nil {
			return nil, fmt.Errorf("%s: source handle %d: %w", context, i, err)
		}
		if err := ensureSafeText("source handle", handle.Label, handle.URI); err != nil {
			return nil, fmt.Errorf("%s: source handle %d: %w", context, i, err)
		}
		copied[i] = handle
	}

	sort.SliceStable(copied, func(i, j int) bool {
		if copied[i].Label == copied[j].Label {
			return copied[i].URI < copied[j].URI
		}
		return copied[i].Label < copied[j].Label
	})
	return copied, nil
}

func requirePlanStepExplanations(step PlanStep) error {
	required := []struct {
		name  string
		value string
	}{
		{name: "title", value: step.Title},
		{name: "description", value: step.Description},
		{name: "reason", value: step.Reason},
		{name: "approval_kind", value: step.ApprovalKind},
		{name: "current_state", value: step.CurrentState},
		{name: "target_state", value: step.TargetState},
		{name: "required_permission", value: step.RequiredPermission},
		{name: "validation", value: step.Validation},
		{name: "rollback", value: step.Rollback},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("plan step %s is required", field.name)
		}
	}
	return nil
}

func validateSourceHandleURI(uri string) error {
	if strings.Contains(uri, "..") {
		return fmt.Errorf("source handle URI must not contain relative traversal")
	}
	if strings.Contains(uri, "://") {
		return fmt.Errorf("external source handle URLs must be cached under docs/references before use")
	}
	if strings.HasPrefix(uri, "docs/") {
		return nil
	}
	return fmt.Errorf("source handle URI must be a docs/ relative path")
}

func safeStringList(context string, values []string) ([]string, error) {
	copied := append([]string(nil), values...)
	for _, value := range copied {
		if err := ensureSafeText(context, value); err != nil {
			return nil, err
		}
	}
	sort.Strings(copied)
	return copied, nil
}

func validatePlanStepIntent(intent PlanStepIntent) error {
	for _, allowed := range PlanStepIntents() {
		if intent == allowed {
			return nil
		}
	}
	return fmt.Errorf("unknown plan step intent %q", intent)
}

func validateCheckStatus(status CheckStatus) error {
	for _, allowed := range CheckStatuses() {
		if status == allowed {
			return nil
		}
	}
	return fmt.Errorf("unknown check status %q", status)
}

func countPlanStatuses(steps []PlanStep, checks []PlanCheck) PlanStatusCounts {
	counts := PlanStatusCounts{
		StepIntents:   map[PlanStepIntent]int{},
		CheckStatuses: map[CheckStatus]int{},
	}
	for _, step := range steps {
		counts.StepIntents[step.Intent]++
	}
	for _, check := range checks {
		counts.CheckStatuses[check.Status]++
	}
	return counts
}

func approvalSummaryFor(steps []PlanStep) ApprovalSummary {
	var required bool
	var blocked bool
	for _, step := range steps {
		if step.RequiresApproval {
			required = true
		}
		if step.Intent == PlanStepBlocked {
			blocked = true
		}
	}

	switch {
	case blocked:
		return ApprovalSummary{
			Required: required,
			Approved: false,
			Blocked:  true,
			Reason:   "Plan contains blocked steps and cannot be approved for execution.",
		}
	case required:
		return ApprovalSummary{
			Required: true,
			Approved: false,
			Blocked:  false,
			Reason:   "Mutating steps require explicit approval bound to this plan.",
		}
	default:
		return ApprovalSummary{
			Required: false,
			Approved: false,
			Blocked:  false,
			Reason:   "No mutation approval is required for this provider-neutral plan.",
		}
	}
}

func stepRequiresApproval(intent PlanStepIntent) bool {
	return intent == PlanStepRepair || intent == PlanStepCreate
}

func validateApprovalKindForIntent(step PlanStep) error {
	if step.RequiresApproval {
		if step.ApprovalKind != approvalKindCloudMutation {
			return fmt.Errorf("plan step approval_kind must be %q for mutating intent %q", approvalKindCloudMutation, step.Intent)
		}
		return nil
	}
	if step.ApprovalKind != approvalKindNotRequired {
		return fmt.Errorf("plan step approval_kind must be %q for non-mutating intent %q", approvalKindNotRequired, step.Intent)
	}
	return nil
}

func stableID(prefix string, material any) string {
	encoded, err := json.Marshal(material)
	if err != nil {
		panic(fmt.Sprintf("workflow stable ID material is not JSON-serializable: %v", err))
	}
	sum := sha256.Sum256(encoded)
	return prefix + "_" + hex.EncodeToString(sum[:])[:16]
}

func planIDMaterial(plan ExecutionPlan) any {
	return struct {
		Provider               string                 `json:"provider"`
		Goal                   string                 `json:"goal"`
		CollectionPath         string                 `json:"collection_path,omitempty"`
		Action                 string                 `json:"action"`
		CoverageRecommendation CoverageRecommendation `json:"coverage_recommendation"`
		Steps                  []PlanStep             `json:"steps"`
		SourceHandles          []SourceHandle         `json:"source_handles"`
		PackageSchemaStatus    PackageSchemaStatus    `json:"package_schema_status"`
	}{
		Provider:               string(plan.Request.Provider),
		Goal:                   string(plan.Request.Goal),
		CollectionPath:         string(plan.Request.CollectionPath),
		Action:                 string(plan.Request.Action),
		CoverageRecommendation: plan.CoverageRecommendation,
		Steps:                  plan.Steps,
		SourceHandles:          plan.SourceHandles,
		PackageSchemaStatus:    plan.PackageSchemaStatus,
	}
}

func stepIDMaterial(step PlanStep) any {
	step.ID = ""
	return step
}

func checkIDMaterial(check PlanCheck) any {
	check.ID = ""
	return check
}

func ensureSafeText(context string, values ...string) error {
	for _, value := range values {
		lower := strings.ToLower(value)
		for _, forbidden := range forbiddenPlanTextFragments {
			if strings.Contains(lower, forbidden) {
				return fmt.Errorf("%s contains unsafe content matching %q", context, forbidden)
			}
		}
	}
	return nil
}

var forbiddenPlanTextFragments = []string{
	"/users/",
	"access_key",
	"apikey",
	"api_key",
	"arn:",
	"bearer ",
	"client-secret",
	"client_secret",
	"c:\\users\\",
	".pem",
	"/home/",
	"ocid1.",
	"passphrase",
	"password",
	"plain-secret",
	"plain-token",
	"private-key",
	"private_key",
	"projects/",
	"refresh_token",
	"secret_key",
	"service_account_json",
	"session_token",
	"token=",
}
