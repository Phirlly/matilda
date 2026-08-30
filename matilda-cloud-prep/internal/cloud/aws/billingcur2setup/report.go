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
			Label: "AWS CUR 2.0 Existing Bucket Selection",
			URI:   "docs/references/aws/aws-cur2-existing-bucket-selection.md",
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
	if existingBucketSelectionRequired(plan) {
		return existingBucketSelectionCheck(plan)
	}
	evidence := []workflow.PlanEvidence{
		{Key: "destination_mode", Value: string(plan.Facts.DestinationMode)},
		{Key: "candidate_index", Value: plan.Facts.CandidateIndex},
		{Key: "setup_binding_ref", Value: setupBindingRef(plan)},
		{Key: "selected_export_ref", Value: safePlannedExportRef(plan), PlanIDExcluded: true},
		{Key: "data_exports_region", Value: dataExportsRegion},
		{Key: "s3_region", Value: plan.Region},
		{Key: "coverage_status", Value: string(plan.Coverage.Status)},
	}
	if plan.Facts.BucketRef != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "selected_s3_bucket_ref", Value: plan.Facts.BucketRef})
	}
	return workflow.PlanCheck{
		ID:       "aws_cur2_create_export_plan_facts",
		Status:   workflow.CheckWarn,
		Title:    "AWS CUR 2.0 setup plan",
		Message:  "A Matilda-specific AWS CUR 2.0 export setup plan was generated without exposing generated resource names or account identifiers.",
		Evidence: evidence,
	}
}

func existingBucketSelectionCheck(plan setupPlan) workflow.PlanCheck {
	evidence := []workflow.PlanEvidence{
		{Key: "destination_mode", Value: string(workflow.AWSCUR2DestinationExistingSameAccount)},
		{Key: "s3_region", Value: plan.Region},
		{Key: "candidate_count", Value: fmt.Sprintf("%d", len(plan.BucketCandidates))},
	}
	for index, candidate := range plan.BucketCandidates {
		prefix := fmt.Sprintf("candidate_%d", index+1)
		evidence = append(evidence,
			workflow.PlanEvidence{Key: prefix + "_bucket_ref", Value: candidate.BucketRef},
			workflow.PlanEvidence{Key: prefix + "_bucket_label", Value: safeBucketDisplayLabel(candidate.BucketName, candidate.BucketRef), PlanIDExcluded: true},
			workflow.PlanEvidence{Key: prefix + "_bucket_region", Value: plan.Region},
		)
	}
	return workflow.PlanCheck{
		ID:       "aws_cur2_existing_bucket_selection_required",
		Status:   workflow.CheckWarn,
		Title:    "AWS existing S3 bucket selection",
		Message:  "Select a verified same-account S3 bucket by safe reference before creating the AWS CUR 2.0 setup plan.",
		Evidence: evidence,
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

func existingBucketSelectionRequired(plan setupPlan) bool {
	return guidePlanCode(plan) == "aws_cur2_existing_bucket_selection_required"
}

func guidePlanCode(plan setupPlan) string {
	for _, step := range plan.Steps {
		if step.Intent == "guide" {
			return step.ID
		}
	}
	return ""
}

func existingBucketSelectionCurrentState(plan setupPlan) string {
	if len(plan.BucketCandidates) == 0 {
		return "No existing same-account S3 buckets were discovered in the selected region."
	}
	return "Existing same-account S3 buckets were discovered."
}

func planSteps(plan setupPlan) []workflow.PlanStep {
	if existingBucketSelectionRequired(plan) {
		return []workflow.PlanStep{{
			Intent:                    workflow.PlanStepGuide,
			Title:                     "Select existing same-account S3 bucket",
			Description:               "Choose one S3 bucket discovered from the verified AWS account before a CUR 2.0 setup plan is generated.",
			Reason:                    "Matilda Cloud Prep must bind the exact destination before any bucket policy or Data Exports change can be approved.",
			ApprovalKind:              "not_required",
			CurrentState:              existingBucketSelectionCurrentState(plan),
			TargetState:               "One bucket is selected by safe reference for the setup plan.",
			RequiredPermission:        "s3:ListAllMyBuckets",
			CredentialMaterialTouched: false,
			Validation:                "The selected safe bucket ref is resolved again before planning.",
			Rollback:                  "No cloud change was made.",
		}}
	}
	if blockedCode := blockedPlanCode(plan); blockedCode != "" {
		return []workflow.PlanStep{blockedSetupStep(blockedCode, plan.identityVerified())}
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
		policyDescription := "Merge the scoped AWS Data Exports delivery statement into the generated bucket policy."
		policyCurrentState := "The generated bucket policy does not yet contain the scoped Data Exports delivery statement."
		policyTargetState := "The generated bucket policy allows Data Exports delivery for the selected account and export scope."
		if plan.Facts.DestinationMode == workflow.AWSCUR2DestinationExistingSameAccount {
			policyDescription = "Merge the scoped AWS Data Exports delivery statement into the selected same-account bucket policy."
			policyCurrentState = "The selected same-account S3 bucket policy does not yet contain the scoped Data Exports delivery statement."
			policyTargetState = "The selected bucket policy allows Data Exports delivery for the selected account and export scope."
		}
		steps = append(steps, workflow.PlanStep{
			ID:                        workflow.AWSCUR2MergeBucketPolicyOperationID,
			Intent:                    workflow.PlanStepRepair,
			Title:                     "Allow AWS Data Exports delivery to the bucket",
			Description:               policyDescription,
			Reason:                    "AWS requires the bucket policy to allow Data Exports to write report objects with source conditions.",
			ApprovalKind:              "cloud_mutation",
			CurrentState:              policyCurrentState,
			TargetState:               policyTargetState,
			RequiredPermission:        "s3:GetBucketPolicy, s3:PutBucketPolicy",
			CredentialMaterialTouched: false,
			Validation:                "The policy is read, parsed, merged, and written with the expected bucket owner.",
			Rollback:                  "The tool does not remove bucket policy statements automatically.",
		})
	}
	exportTargetState := "A Matilda-generated CUR 2.0 export writes monthly billing data to the generated S3 destination."
	if plan.Facts.DestinationMode == workflow.AWSCUR2DestinationExistingSameAccount {
		exportTargetState = "A Matilda-generated CUR 2.0 export writes monthly billing data to the selected S3 destination."
	}
	steps = append(steps, workflow.PlanStep{
		ID:                        workflow.AWSCUR2CreateExportOperationID,
		Intent:                    workflow.PlanStepCreate,
		Title:                     "Create Matilda AWS CUR 2.0 export",
		Description:               "Create a Matilda-specific AWS CUR 2.0 export using the verified Rapid Assessment - Billing Based setup defaults.",
		Reason:                    "Matilda Rapid Assessment - Billing Based needs a CUR 2.0 billing export when no reusable export is selected.",
		ApprovalKind:              "cloud_mutation",
		CurrentState:              "No matching Matilda-generated CUR 2.0 export exists for the selected account and region.",
		TargetState:               exportTargetState,
		RequiredPermission:        "bcm-data-exports:CreateExport, cur:PutReportDefinition",
		CredentialMaterialTouched: false,
		Validation:                "The created export request uses COST_AND_USAGE_REPORT, monthly granularity, text CSV gzip output, create-new report files, and synchronous refresh.",
		Rollback:                  "The tool does not delete Data Exports resources automatically.",
	})
	return steps
}

func blockedSetupStep(code string, identityVerified bool) workflow.PlanStep {
	if code == "aws_s3_bucket_inaccessible" {
		return workflow.PlanStep{
			Intent:                    workflow.PlanStepBlocked,
			Title:                     "Resolve AWS S3 bucket candidate access",
			Description:               "Stop before AWS CUR 2.0 setup because AWS did not return enough S3 evidence for the generated destination bucket candidate.",
			Reason:                    "Matilda Cloud Prep creates or reuses only a generated same-account S3 bucket for this setup path, and must prove the bucket candidate is safe before creating a CUR 2.0 export.",
			ApprovalKind:              "not_required",
			CurrentState:              "The generated same-account S3 bucket candidate could not be verified as available to create or safely owned by this account.",
			TargetState:               "Matilda Cloud Prep can show an approval-required plan to create or reuse the generated bucket, update its Data Exports delivery policy, and create the CUR 2.0 export.",
			RequiredPermission:        "s3:ListBucket for existing bucket checks, plus s3:CreateBucket, s3:GetBucketPolicy, s3:PutBucketPolicy, bcm-data-exports:CreateExport, and cur:PutReportDefinition for approved setup.",
			CredentialMaterialTouched: false,
			Validation:                "Do not manually create or select arbitrary buckets for the normal guided path. Resolve S3 access ambiguity, then rerun apply-prereqs to get a new approval-required setup plan.",
			Rollback:                  "No cloud change was made.",
		}
	}

	currentState := "AWS CUR 2.0 setup is blocked."
	if identityVerified {
		currentState = "AWS CUR 2.0 setup is blocked after caller identity verification."
	}
	return workflow.PlanStep{
		Intent:                    workflow.PlanStepBlocked,
		Title:                     "Resolve AWS CUR 2.0 setup blocker",
		Description:               "Stop before AWS CUR 2.0 setup because a required prerequisite could not be verified safely.",
		Reason:                    "Cloud-side setup must fail closed when AWS identity, configuration, or setup evidence is unavailable.",
		ApprovalKind:              "not_required",
		CurrentState:              currentState,
		TargetState:               "Required AWS setup evidence is available before mutation.",
		RequiredPermission:        "AWS setup permissions required by the selected operation.",
		CredentialMaterialTouched: false,
		Validation:                "Rerun apply-prereqs after resolving the blocker.",
		Rollback:                  "No cloud change was made.",
	}
}
