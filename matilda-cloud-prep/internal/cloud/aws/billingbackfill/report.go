package billingbackfill

import (
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func (runner Runner) report(request workflow.Request, status workflow.RunStatus, support workflow.SupportStatus, code string, message string, mutated bool, step workflow.PlanStep, checks ...workflow.PlanCheck) workflow.CapabilityReport {
	return runner.reportWithIdentity(request, unknownBackfillIdentitySummary, status, support, code, message, mutated, step, checks...)
}

func (runner Runner) verifiedReport(request workflow.Request, status workflow.RunStatus, support workflow.SupportStatus, code string, message string, mutated bool, step workflow.PlanStep, checks ...workflow.PlanCheck) workflow.CapabilityReport {
	return runner.reportWithIdentity(request, verifiedBackfillIdentitySummary, status, support, code, message, mutated, step, checks...)
}

func (runner Runner) preflightReport(request workflow.Request, preflight workflow.CapabilityReport, status workflow.RunStatus, support workflow.SupportStatus, code string, message string, mutated bool, step workflow.PlanStep, checks ...workflow.PlanCheck) workflow.CapabilityReport {
	return runner.reportWithIdentity(request, preflightBackfillIdentitySummary(preflight), status, support, code, message, mutated, step, checks...)
}

func (runner Runner) reportWithIdentity(request workflow.Request, identitySummary func([]workflow.SourceHandle) workflow.OperatorIdentitySummary, status workflow.RunStatus, support workflow.SupportStatus, code string, message string, mutated bool, step workflow.PlanStep, checks ...workflow.PlanCheck) workflow.CapabilityReport {
	handles := sourceHandles()
	step.SourceHandles = handles
	for index := range checks {
		checks[index].SourceHandles = handles
	}
	return workflow.CapabilityReport{
		Status:        status,
		SupportStatus: support,
		Code:          code,
		Message:       message,
		Mutated:       mutated,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request:                 request,
			OperatorIdentitySummary: identitySummary(handles),
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageUnknown,
				Summary:        "AWS billing coverage follows the selected CUR 2.0 export and previous-month billing period.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps:               []workflow.PlanStep{step},
			Checks:              checks,
			SourceHandles:       handles,
		},
	}
}

func unknownBackfillIdentitySummary(handles []workflow.SourceHandle) workflow.OperatorIdentitySummary {
	return workflow.OperatorIdentitySummary{
		IdentityStatus: "unknown",
		Summary:        "AWS caller identity and CUR 2.0 export state were not checked before this AWS billing apply-prereqs report.",
		SourceHandles:  handles,
	}
}

func verifiedBackfillIdentitySummary(handles []workflow.SourceHandle) workflow.OperatorIdentitySummary {
	return workflow.OperatorIdentitySummary{
		IdentityStatus: "verified",
		Summary:        "AWS caller identity and CUR 2.0 export state were checked before AWS billing apply-prereqs.",
		SourceHandles:  handles,
	}
}

func preflightBackfillIdentitySummary(preflight workflow.CapabilityReport) func([]workflow.SourceHandle) workflow.OperatorIdentitySummary {
	return func(handles []workflow.SourceHandle) workflow.OperatorIdentitySummary {
		if preflight.PlanInput == nil || strings.TrimSpace(preflight.PlanInput.OperatorIdentitySummary.IdentityStatus) == "" {
			return unknownBackfillIdentitySummary(handles)
		}
		summary := preflight.PlanInput.OperatorIdentitySummary
		summary.SourceHandles = handles
		return summary
	}
}

func withApprovedExecutionPlanID(report workflow.CapabilityReport, planID string) workflow.CapabilityReport {
	if report.PlanInput != nil {
		report.PlanInput.ApprovedExecutionPlanID = planID
	}
	return report
}

func approvalRequiredStep() workflow.PlanStep {
	return workflow.PlanStep{
		ID:                        workflow.AWSBackfillSupportCaseOperationID,
		Intent:                    workflow.PlanStepCreate,
		Title:                     "Request AWS CUR 2.0 previous-month backfill",
		Description:               "Create an AWS Support case for previous-month CUR 2.0 billing data backfill only after explicit approval.",
		Reason:                    "Matilda Rapid Assessment - Billing Based requires previous-month AWS billing data for this path.",
		ApprovalKind:              "cloud_mutation",
		CurrentState:              "Previous-month CUR 2.0 billing data is missing.",
		TargetState:               "AWS Support has a backfill request for the selected billing period.",
		RequiredPermission:        "support:DescribeServices, support:DescribeSeverityLevels, support:DescribeCreateCaseOptions, support:DescribeCases, support:CreateCase",
		CredentialMaterialTouched: false,
		Validation:                "The tool reruns read-only CUR 2.0 preflight checks and duplicate-case checks before creating a support case.",
		Rollback:                  "AWS Support cases are not deleted by this tool; users can resolve or close cases in AWS Support Center.",
	}
}

func supportCaseCreatedStep() workflow.PlanStep {
	return workflow.PlanStep{
		ID:                        workflow.AWSBackfillSupportCaseOperationID,
		Intent:                    workflow.PlanStepCreate,
		Title:                     "Requested AWS CUR 2.0 previous-month backfill",
		Description:               "Created an AWS Support case for previous-month CUR 2.0 billing data backfill after explicit approval.",
		Reason:                    "Matilda Rapid Assessment - Billing Based requires previous-month AWS billing data for this path.",
		ApprovalKind:              "cloud_mutation",
		CurrentState:              "AWS Support has a backfill request for the selected billing period.",
		TargetState:               "Previous-month CUR 2.0 billing data and manifest are visible to preflight after AWS completes the request.",
		RequiredPermission:        "support:DescribeServices, support:DescribeSeverityLevels, support:DescribeCreateCaseOptions, support:DescribeCases, support:CreateCase",
		CredentialMaterialTouched: false,
		Validation:                "Rerun preflight after AWS completes the Support request to confirm previous-month data partition and manifest availability.",
		Rollback:                  "AWS Support cases are not deleted by this tool; users can resolve or close cases in AWS Support Center.",
	}
}

func manualSupportCaseStep() workflow.PlanStep {
	return workflow.PlanStep{
		Intent:                    workflow.PlanStepGuide,
		Title:                     "Request AWS CUR 2.0 backfill manually",
		Description:               "Use AWS Support Center to request previous-month CUR 2.0 billing data backfill when automated case classification is unavailable.",
		Reason:                    "Matilda Rapid Assessment - Billing Based requires previous-month AWS billing data, but the tool must not guess AWS Support case classification values.",
		ApprovalKind:              "not_required",
		CurrentState:              "AWS Support case creation could not be automated safely.",
		TargetState:               "An AWS Support backfill request is submitted manually with the required billing details.",
		RequiredPermission:        "AWS Support Center access with permission to create a support case.",
		CredentialMaterialTouched: false,
		Validation:                "Rerun preflight after AWS completes the manual Support request to confirm previous-month data partition and manifest availability.",
		Rollback:                  "No cloud change was made by this tool; users manage manual Support cases in AWS Support Center.",
	}
}

func guideStep() workflow.PlanStep {
	return workflow.PlanStep{
		Intent:                    workflow.PlanStepGuide,
		Title:                     "Complete AWS CUR 2.0 backfill follow-up",
		Description:               "Wait for AWS to complete the previous-month CUR 2.0 backfill, then rerun preflight.",
		Reason:                    "AWS backfill completion is handled by AWS Support and cannot be guaranteed by this tool.",
		ApprovalKind:              "not_required",
		CurrentState:              "Previous-month CUR 2.0 billing data is not ready for Matilda yet.",
		TargetState:               "Previous-month CUR 2.0 billing data and manifest are visible to preflight.",
		RequiredPermission:        "Read-only CUR 2.0 and S3 validation permissions.",
		CredentialMaterialTouched: false,
		Validation:                "Rerun preflight to confirm data partition and manifest availability.",
		Rollback:                  "No rollback is performed by this tool for support-case follow-up.",
	}
}

func reuseStep() workflow.PlanStep {
	return workflow.PlanStep{
		Intent:                    workflow.PlanStepReuse,
		Title:                     "Reuse existing AWS billing state",
		Description:               "Reuse the current AWS CUR 2.0 or AWS Support state without creating duplicate support cases.",
		Reason:                    "Duplicate cloud-side resources and duplicate support cases create confusion for customers.",
		ApprovalKind:              "not_required",
		CurrentState:              "A reusable AWS billing prerequisite or support case already exists.",
		TargetState:               "Matilda preparation uses the existing AWS billing prerequisite state.",
		RequiredPermission:        "Read-only AWS billing, S3, and Support visibility.",
		CredentialMaterialTouched: false,
		Validation:                "The tool validates existing state before deciding no mutation is needed.",
		Rollback:                  "No cloud change is made.",
	}
}

func blockedStep() workflow.PlanStep {
	return workflow.PlanStep{
		Intent:                    workflow.PlanStepBlocked,
		Title:                     "Resolve AWS billing apply-prereqs blocker",
		Description:               "Stop before mutation because AWS billing backfill prerequisites could not be verified safely.",
		Reason:                    "The tool must not create AWS Support cases when readiness or duplicate-case checks are uncertain.",
		ApprovalKind:              "not_required",
		CurrentState:              "AWS billing apply-prereqs has a blocking readiness issue.",
		TargetState:               "Readiness and duplicate-case checks are verified before mutation.",
		RequiredPermission:        "AWS billing, S3, and Support permissions required by the selected action.",
		CredentialMaterialTouched: false,
		Validation:                "Rerun apply-prereqs after resolving the blocker.",
		Rollback:                  "No cloud change was made.",
	}
}

func passCheck(code string, title string, message string, evidence ...workflow.PlanEvidence) workflow.PlanCheck {
	return planCheck(workflow.CheckPass, code, title, message, evidence...)
}

func warnCheck(code string, title string, message string, evidence ...workflow.PlanEvidence) workflow.PlanCheck {
	return planCheck(workflow.CheckWarn, code, title, message, evidence...)
}

func failCheck(code string, title string, message string, evidence ...workflow.PlanEvidence) workflow.PlanCheck {
	return planCheck(workflow.CheckFail, code, title, message, evidence...)
}

func planCheck(status workflow.CheckStatus, code string, title string, message string, evidence ...workflow.PlanEvidence) workflow.PlanCheck {
	if len(evidence) == 0 {
		evidence = []workflow.PlanEvidence{{Key: "code", Value: code}}
	}
	return workflow.PlanCheck{
		ID:       code,
		Status:   status,
		Title:    title,
		Message:  message,
		Evidence: evidence,
	}
}

func supportCaseEvidence(supportCase SupportCase, period string) []workflow.PlanEvidence {
	evidence := []workflow.PlanEvidence{{Key: "previous_billing_period", Value: period}}
	if value := strings.TrimSpace(supportCase.CaseID); value != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "support_case_id", Value: value})
	}
	if value := strings.TrimSpace(supportCase.DisplayID); value != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "support_case_display_id", Value: value})
	}
	if value := strings.TrimSpace(supportCase.Status); value != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "support_case_status", Value: value})
	}
	return evidence
}

func manualSupportRequestEvidence(context backfillContext) []workflow.PlanEvidence {
	evidence := context.evidence()
	evidence = append(evidence, workflow.PlanEvidence{Key: "manual_support_request_needs", Value: "report name, billing period, S3 bucket details"})
	return evidence
}

func sourceHandles() []workflow.SourceHandle {
	return []workflow.SourceHandle{
		{
			Label: "AWS Rapid Assessment Billing CUR 2.0 Preflight Source Bundle",
			URI:   "docs/references/aws/aws-rapid-assessment-billing-cur2-preflight-source-bundle.md",
		},
		{
			Label: "AWS Support Backfill Request",
			URI:   "docs/references/aws/aws-support-backfill-request.md",
		},
		{
			Label: "AWS Official Implementation References",
			URI:   "docs/references/aws/official-implementation-references.md",
		},
	}
}
