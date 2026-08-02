package billingcur2setup

import (
	"fmt"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func sourceHandles() []workflow.SourceHandle {
	return []workflow.SourceHandle{
		{
			Label: "AWS CUR 2.0 Create-New Export",
			URI:   "docs/references/aws/aws-cur2-create-new-export.md",
		},
		{
			Label: "AWS Rapid Assessment Billing CUR 2.0 Preflight Source Bundle",
			URI:   "docs/references/aws/aws-rapid-assessment-billing-cur2-preflight-source-bundle.md",
		},
		{
			Label: "AWS Official Implementation References",
			URI:   "docs/references/aws/official-implementation-references.md",
		},
	}
}

func reportWithPlan(request workflow.Request, status workflow.RunStatus, support workflow.SupportStatus, code string, message string, mutated bool, input workflow.ExecutionPlanInput) workflow.CapabilityReport {
	handles := sourceHandles()
	input.Request = request
	input.SourceHandles = handles
	for index := range input.Steps {
		input.Steps[index].SourceHandles = handles
	}
	for index := range input.Checks {
		input.Checks[index].SourceHandles = handles
	}
	if input.OperatorIdentitySummary.SourceHandles == nil {
		input.OperatorIdentitySummary.SourceHandles = handles
	}
	return workflow.CapabilityReport{
		Status:        status,
		SupportStatus: support,
		Code:          code,
		Message:       message,
		Mutated:       mutated,
		SourceHandles: handles,
		PlanInput:     &input,
	}
}

func setupPlanInput(request workflow.Request, plan setupPlan, steps []workflow.PlanStep, checks []workflow.PlanCheck) workflow.ExecutionPlanInput {
	handles := sourceHandles()
	for index := range steps {
		steps[index].SourceHandles = handles
	}
	for index := range checks {
		checks[index].SourceHandles = handles
	}
	return workflow.ExecutionPlanInput{
		Request: request,
		OperatorIdentitySummary: workflow.OperatorIdentitySummary{
			IdentityStatus: "verified",
			Summary:        fmt.Sprintf("AWS caller identity was verified for account-ending-%s before CUR 2.0 setup.", last4(plan.Identity.AccountID)),
			SourceHandles:  handles,
		},
		CoverageRecommendation: workflow.CoverageRecommendation{
			CoverageStatus: plan.Coverage.Status,
			Summary:        plan.Coverage.Summary,
		},
		PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
		Steps:               steps,
		Checks:              checks,
		SourceHandles:       handles,
	}
}

func last4(value string) string {
	if len(value) <= 4 {
		return "unknown"
	}
	return value[len(value)-4:]
}

func planFactsCheck(plan setupPlan) workflow.PlanCheck {
	return workflow.PlanCheck{
		ID:      "aws_cur2_create_export_plan_facts",
		Status:  workflow.CheckWarn,
		Title:   "AWS CUR 2.0 setup plan",
		Message: "A Matilda-specific AWS CUR 2.0 export setup plan was generated without exposing generated resource names or account identifiers.",
		Evidence: []workflow.PlanEvidence{
			{Key: "candidate_index", Value: plan.Facts.CandidateIndex},
			{Key: "setup_binding_ref", Value: setupBindingRef(plan)},
			{Key: "selected_export_ref", Value: safePlannedExportRef(plan), PlanIDExcluded: true},
			{Key: "data_exports_region", Value: dataExportsRegion},
			{Key: "s3_region", Value: plan.Region},
			{Key: "coverage_status", Value: string(plan.Coverage.Status)},
		},
	}
}

func setPlanEvidenceValue(input *workflow.ExecutionPlanInput, key string, value string) {
	if input == nil {
		return
	}
	for checkIndex := range input.Checks {
		for evidenceIndex := range input.Checks[checkIndex].Evidence {
			if input.Checks[checkIndex].Evidence[evidenceIndex].Key == key {
				input.Checks[checkIndex].Evidence[evidenceIndex].Value = value
				return
			}
		}
	}
}

func planSteps(plan setupPlan) []workflow.PlanStep {
	if blockedCode := blockedPlanCode(plan); blockedCode != "" {
		return []workflow.PlanStep{{
			Intent:                    workflow.PlanStepBlocked,
			Title:                     "Resolve AWS CUR 2.0 setup blocker",
			Description:               "Stop before AWS CUR 2.0 setup because the current setup plan is blocked.",
			Reason:                    "Cloud-side setup must fail closed when AWS cannot safely create or reuse the required CUR 2.0 export.",
			ApprovalKind:              "not_required",
			CurrentState:              "AWS CUR 2.0 setup is blocked.",
			TargetState:               "The blocker is resolved before any cloud mutation is approved.",
			RequiredPermission:        "AWS billing and S3 permissions required by the selected setup operation.",
			CredentialMaterialTouched: false,
			Validation:                "Resolve the reported blocker and rerun apply-prereqs before approving setup.",
			Rollback:                  "No cloud change was made.",
		}}
	}
	if plan.ManagedExport != nil && !plan.PolicyNeedsMerge {
		return []workflow.PlanStep{{
			Intent:                    workflow.PlanStepReuse,
			Title:                     "Reuse existing Matilda AWS CUR 2.0 export",
			Description:               "Reuse an existing Matilda-generated CUR 2.0 export that matches the current setup contract.",
			Reason:                    "Creating duplicate billing exports adds cost and confusion when an existing generated export already matches the verified settings.",
			ApprovalKind:              "not_required",
			CurrentState:              "A matching Matilda CUR 2.0 export already exists.",
			TargetState:               "Matilda preparation reuses the existing generated CUR 2.0 export.",
			RequiredPermission:        "Read-only AWS Data Exports and S3 validation permissions.",
			CredentialMaterialTouched: false,
			Validation:                "The export name, query, table configuration, destination, output settings, and refresh cadence match the setup contract.",
			Rollback:                  "No cloud change is made.",
		}}
	}
	if plan.ManagedExport != nil && plan.PolicyNeedsMerge {
		return []workflow.PlanStep{{
			ID:                        workflow.AWSCUR2MergeBucketPolicyOperationID,
			Intent:                    workflow.PlanStepRepair,
			Title:                     "Repair Matilda AWS CUR 2.0 export delivery policy",
			Description:               "Merge the scoped AWS Data Exports delivery statement into the generated bucket policy for the existing Matilda CUR 2.0 export.",
			Reason:                    "The existing Matilda-generated CUR 2.0 export matches the setup contract, but AWS delivery requires the generated bucket policy to allow Data Exports writes.",
			ApprovalKind:              "cloud_mutation",
			CurrentState:              "The existing Matilda-generated bucket policy is missing the scoped Data Exports delivery statement.",
			TargetState:               "The bucket policy allows Data Exports delivery for the selected account and export scope without creating a duplicate export.",
			RequiredPermission:        "s3:GetBucketPolicy, s3:PutBucketPolicy",
			CredentialMaterialTouched: false,
			Validation:                "The policy is read, parsed, merged, written with the expected bucket owner, and revalidated before completion.",
			Rollback:                  "The tool does not remove bucket policy statements automatically.",
		}}
	}

	steps := []workflow.PlanStep{}
	if !plan.BucketExists {
		steps = append(steps, workflow.PlanStep{
			ID:                        workflow.AWSCUR2CreateBucketOperationID,
			Intent:                    workflow.PlanStepCreate,
			Title:                     "Create Matilda AWS billing export bucket",
			Description:               "Create a generated same-account S3 bucket for AWS CUR 2.0 delivery.",
			Reason:                    "AWS Data Exports requires an S3 destination before a CUR 2.0 export can be created.",
			ApprovalKind:              "cloud_mutation",
			CurrentState:              "No matching generated same-account S3 bucket is available.",
			TargetState:               "A generated same-account S3 bucket exists in the selected region.",
			RequiredPermission:        "s3:CreateBucket",
			CredentialMaterialTouched: false,
			Validation:                "The bucket is checked with the expected bucket owner before policy or export creation continues.",
			Rollback:                  "The tool does not delete S3 buckets automatically.",
		})
	}
	if plan.PolicyNeedsMerge {
		steps = append(steps, workflow.PlanStep{
			ID:                        workflow.AWSCUR2MergeBucketPolicyOperationID,
			Intent:                    workflow.PlanStepRepair,
			Title:                     "Allow AWS Data Exports delivery to the bucket",
			Description:               "Merge the scoped AWS Data Exports delivery statement into the generated bucket policy.",
			Reason:                    "AWS requires the bucket policy to allow Data Exports to write report objects with source conditions.",
			ApprovalKind:              "cloud_mutation",
			CurrentState:              "The generated bucket policy does not yet contain the scoped Data Exports delivery statement.",
			TargetState:               "The generated bucket policy allows Data Exports delivery for the selected account and export scope.",
			RequiredPermission:        "s3:GetBucketPolicy, s3:PutBucketPolicy",
			CredentialMaterialTouched: false,
			Validation:                "The policy is read, parsed, merged, and written with the expected bucket owner.",
			Rollback:                  "The tool does not remove bucket policy statements automatically.",
		})
	}
	steps = append(steps, workflow.PlanStep{
		ID:                        workflow.AWSCUR2CreateExportOperationID,
		Intent:                    workflow.PlanStepCreate,
		Title:                     "Create Matilda AWS CUR 2.0 export",
		Description:               "Create a Matilda-specific AWS CUR 2.0 export using the verified Rapid Assessment - Billing Based setup defaults.",
		Reason:                    "Matilda Rapid Assessment - Billing Based needs a CUR 2.0 billing export when no reusable export is selected.",
		ApprovalKind:              "cloud_mutation",
		CurrentState:              "No matching Matilda-generated CUR 2.0 export exists for the selected account and region.",
		TargetState:               "A Matilda-generated CUR 2.0 export writes monthly billing data to the generated S3 destination.",
		RequiredPermission:        "bcm-data-exports:CreateExport, cur:PutReportDefinition",
		CredentialMaterialTouched: false,
		Validation:                "The created export request uses COST_AND_USAGE_REPORT, monthly granularity, text CSV gzip output, create-new report files, and synchronous refresh.",
		Rollback:                  "The tool does not delete Data Exports resources automatically.",
	})
	return steps
}
