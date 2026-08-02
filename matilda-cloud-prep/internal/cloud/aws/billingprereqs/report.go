package billingprereqs

import "github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"

func operationRequiredReport(request workflow.Request) workflow.CapabilityReport {
	handles := sourceHandles()
	return workflow.CapabilityReport{
		Status:        workflow.RunStatusManualSteps,
		SupportStatus: workflow.SupportGuided,
		Code:          "aws_billing_prereqs_operation_required",
		Message:       "Choose the AWS billing prerequisite operation to plan before cloud changes are made.",
		Mutated:       false,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "unknown",
				Summary:        "AWS caller identity is not inspected until a specific AWS billing prerequisites operation is selected.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageUnknown,
				Summary:        "AWS billing coverage is classified by the selected operation before any approval plan is shown.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{{
				Intent:                    workflow.PlanStepGuide,
				Title:                     "Select AWS billing prerequisites operation",
				Description:               "Run AWS billing apply-prereqs with an explicit operation so the tool can produce the matching reviewed plan.",
				Reason:                    "Cloud mutations must be approved against the exact operation, plan, and step set that will be applied.",
				ApprovalKind:              "not_required",
				CurrentState:              "No AWS billing prerequisites operation was selected.",
				TargetState:               "A specific AWS billing operation is selected before any cloud-side setup plan is produced.",
				RequiredPermission:        "No AWS permission is evaluated before an operation is selected.",
				CredentialMaterialTouched: false,
				Validation:                "Use --create-cur2-export to plan CUR 2.0 setup, or use --request-backfill to plan a support request when previous-month data is missing.",
				Rollback:                  "No cloud change was made.",
				SourceHandles:             handles,
			}},
			Checks: []workflow.PlanCheck{{
				ID:      "aws_billing_prereqs_operation_required",
				Status:  workflow.CheckWarn,
				Title:   "AWS billing operation selection",
				Message: "AWS billing apply-prereqs requires an explicit operation before the tool can plan cloud-side mutations.",
				Evidence: []workflow.PlanEvidence{
					{Key: "create_cur2_export_flag", Value: "--create-cur2-export"},
					{Key: "request_backfill_flag", Value: "--request-backfill"},
				},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}

func blockedReport(request workflow.Request, code string, message string) workflow.CapabilityReport {
	handles := sourceHandles()
	return workflow.CapabilityReport{
		Status:        workflow.RunStatusBlocked,
		SupportStatus: workflow.SupportBlocked,
		Code:          code,
		Message:       message,
		Mutated:       false,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "unknown",
				Summary:        "AWS billing apply-prereqs operation could not run because the selected operation handler is unavailable.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageUnknown,
				Summary:        "AWS billing coverage was not classified because the selected operation handler is unavailable.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{{
				Intent:                    workflow.PlanStepBlocked,
				Title:                     "Resolve AWS billing apply-prereqs operation routing",
				Description:               "Stop before cloud mutation because the selected AWS billing prerequisites operation cannot run.",
				Reason:                    "Cloud-side setup must use a verified operation handler before any AWS mutation is attempted.",
				ApprovalKind:              "not_required",
				CurrentState:              "The selected AWS billing prerequisites operation handler is unavailable.",
				TargetState:               "A verified AWS billing prerequisites operation handler is configured.",
				RequiredPermission:        "No AWS permission is evaluated before operation routing is resolved.",
				CredentialMaterialTouched: false,
				Validation:                "Rerun apply-prereqs after the operation handler is configured.",
				Rollback:                  "No cloud change was made.",
				SourceHandles:             handles,
			}},
			Checks: []workflow.PlanCheck{{
				ID:            code,
				Status:        workflow.CheckFail,
				Title:         "AWS billing apply-prereqs operation routing",
				Message:       message,
				Evidence:      []workflow.PlanEvidence{{Key: "code", Value: code}},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}

func sourceHandles() []workflow.SourceHandle {
	return []workflow.SourceHandle{
		{
			Label: "AWS CUR 2.0 Create-New Export",
			URI:   "docs/references/aws/aws-cur2-create-new-export.md",
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
