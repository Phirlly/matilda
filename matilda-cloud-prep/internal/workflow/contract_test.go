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
	if got, want := CoverageStatuses(), []CoverageStatus{
		CoverageUnknown,
		CoverageOrganizationWide,
		CoverageAccountOnly,
		CoverageSingleAccount,
		CoverageUnverified,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CoverageStatuses() = %#v, want %#v", got, want)
	}

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
				CUR2ExportRef: "cur2-abcdefghijklmnop",
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
	if options.Selectors.AWS.Profile != "default" || options.Selectors.AWS.Region != "us-west-2" || options.Selectors.AWS.CUR2ExportRef != "cur2-abcdefghijklmnop" {
		t.Fatalf("AWS selectors = %#v, want profile, region, and export ref", options.Selectors.AWS)
	}
}

func TestExecutionOptionsNormalizeGeneratedCUR2ExportRef(t *testing.T) {
	options, err := NormalizeExecutionOptions(ExecutionOptions{
		Selectors: &ExecutionSelectors{
			AWS: &AWSExecutionSelectors{
				CUR2ExportRef: "cur2-abcdefghijklmnop",
			},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptions returned error: %v", err)
	}
	if options.Selectors == nil || options.Selectors.AWS == nil || options.Selectors.AWS.CUR2ExportRef != "cur2-abcdefghijklmnop" {
		t.Fatalf("AWS CUR2ExportRef = %#v, want generated ref", options.Selectors)
	}
}

func TestExecutionOptionsNormalizeCUR2DestinationSelectors(t *testing.T) {
	options, err := NormalizeExecutionOptionsForRequest(Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}, ExecutionOptions{
		AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
		Selectors: &ExecutionSelectors{
			AWS: &AWSExecutionSelectors{
				Profile:             "default",
				Region:              "us-west-2",
				CUR2DestinationMode: AWSCUR2DestinationExistingSameAccount,
				CUR2S3BucketRef:     "s3b-abcdefghijklmnop",
			},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptionsForRequest returned error: %v", err)
	}
	if options.Selectors == nil || options.Selectors.AWS == nil {
		t.Fatalf("AWS selectors missing after normalization: %#v", options.Selectors)
	}
	aws := options.Selectors.AWS
	if aws.CUR2DestinationMode != AWSCUR2DestinationExistingSameAccount || aws.CUR2S3BucketRef != "s3b-abcdefghijklmnop" {
		t.Fatalf("AWS CUR2 destination selectors = %#v, want existing bucket selection", aws)
	}
}

func TestExecutionOptionsRejectsUnsafeOrMisScopedCUR2DestinationSelectors(t *testing.T) {
	applyRequest := Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}
	preflightRequest := applyRequest
	preflightRequest.Action = assessment.ActionPreflight

	tests := []struct {
		name    string
		request Request
		input   ExecutionOptions
		want    string
	}{
		{
			name:    "destination selector rejected outside apply prereqs create operation",
			request: preflightRequest,
			input: ExecutionOptions{
				Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{CUR2DestinationMode: AWSCUR2DestinationGenerated}},
			},
			want: "AWS CUR 2.0 destination selector flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs --create-cur2-export",
		},
		{
			name:    "destination selector requires create operation",
			request: applyRequest,
			input: ExecutionOptions{
				Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{CUR2DestinationMode: AWSCUR2DestinationGenerated}},
			},
			want: "AWS CUR 2.0 destination selector flags require create_cur2_export",
		},
		{
			name:    "bucket ref must be generated safe ref",
			request: applyRequest,
			input: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
				Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
					CUR2DestinationMode: AWSCUR2DestinationExistingSameAccount,
					CUR2S3BucketRef:     "customer-cur-bucket",
				}},
			},
			want: "cur2_s3_bucket_ref must use format s3b- plus 16, 24, or 32 lowercase generated reference characters",
		},
		{
			name:    "generated destination cannot include existing bucket ref",
			request: applyRequest,
			input: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
				Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{
					CUR2DestinationMode: AWSCUR2DestinationGenerated,
					CUR2S3BucketRef:     "s3b-abcdefghijklmnop",
				}},
			},
			want: "cur2_s3_bucket_ref requires existing_same_account destination mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeExecutionOptionsForRequest(tt.request, tt.input)
			if err == nil {
				t.Fatal("NormalizeExecutionOptionsForRequest returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestDefaultExecutionOptionsAreSafeForDirectReadOnlyExecution(t *testing.T) {
	options := DefaultExecutionOptions()

	if options.SchemaVersion != ExecutionOptionsSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", options.SchemaVersion, ExecutionOptionsSchemaVersion)
	}
	if options.InterfaceMode != InterfaceModeDirect {
		t.Fatalf("InterfaceMode = %q, want %q", options.InterfaceMode, InterfaceModeDirect)
	}
	if options.TimeoutSeconds != DefaultExecutionTimeoutSeconds {
		t.Fatalf("TimeoutSeconds = %d, want %d", options.TimeoutSeconds, DefaultExecutionTimeoutSeconds)
	}
	if options.AWSBillingOperation != "" {
		t.Fatalf("AWSBillingOperation = %q, want empty default", options.AWSBillingOperation)
	}
	if options.Selectors != nil {
		t.Fatalf("Selectors = %#v, want nil default", options.Selectors)
	}
	if len(options.Approvals) != 0 {
		t.Fatalf("Approvals = %#v, want no default approvals", options.Approvals)
	}
}

func TestExecutionOptionsNormalizeScopedBackfillApprovalForAWSBillingApplyPrereqs(t *testing.T) {
	request := Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}

	options, err := NormalizeExecutionOptionsForRequest(request, ExecutionOptions{
		InterfaceMode:       InterfaceModeDirect,
		AWSBillingOperation: AWSBillingOperationRequestBackfill,
		Approvals: []ExecutionApproval{{
			OperationID: AWSBackfillSupportCaseOperationID,
			Intent:      ApprovalIntentRequestBackfillSupportCase,
			PlanID:      "plan_abcdefghijklmnop",
			Confirmed:   true,
		}},
		Selectors: &ExecutionSelectors{
			AWS: &AWSExecutionSelectors{
				Profile:       "default",
				Region:        "us-west-2",
				CUR2ExportRef: "cur2-abcdefghijklmnop",
			},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptionsForRequest returned error: %v", err)
	}
	if len(options.Approvals) != 1 {
		t.Fatalf("Approvals length = %d, want 1", len(options.Approvals))
	}
	approval := options.Approvals[0]
	if approval.OperationID != AWSBackfillSupportCaseOperationID ||
		approval.Intent != ApprovalIntentRequestBackfillSupportCase ||
		approval.PlanID != "plan_abcdefghijklmnop" ||
		!approval.Confirmed {
		t.Fatalf("approval = %#v, want scoped AWS backfill support case approval", approval)
	}
	if !HasApprovedPlanStep(options, "plan_abcdefghijklmnop", AWSBackfillSupportCaseOperationID) {
		t.Fatalf("HasApprovedPlanStep returned false for approved AWS backfill support case step: %#v", options.Approvals)
	}
}

func TestExecutionOptionsNormalizeScopedCUR2CreateExportApprovalForAWSBillingApplyPrereqs(t *testing.T) {
	request := Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}

	options, err := NormalizeExecutionOptionsForRequest(request, ExecutionOptions{
		InterfaceMode:       InterfaceModeDirect,
		AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
		Approvals: []ExecutionApproval{
			{
				OperationID: AWSCUR2CreateBucketOperationID,
				PlanID:      "plan_abcdefghijklmnop",
				Confirmed:   true,
			},
			{
				OperationID: AWSCUR2MergeBucketPolicyOperationID,
				PlanID:      "plan_abcdefghijklmnop",
				Confirmed:   true,
			},
			{
				OperationID: AWSCUR2CreateExportOperationID,
				PlanID:      "plan_abcdefghijklmnop",
				Confirmed:   true,
			},
		},
		Selectors: &ExecutionSelectors{
			AWS: &AWSExecutionSelectors{
				Profile: "default",
				Region:  "us-west-2",
			},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptionsForRequest returned error: %v", err)
	}
	if options.AWSBillingOperation != AWSBillingOperationCreateCUR2Export {
		t.Fatalf("AWSBillingOperation = %q, want %q", options.AWSBillingOperation, AWSBillingOperationCreateCUR2Export)
	}
	if len(options.Approvals) != 3 {
		t.Fatalf("Approvals length = %d, want 3", len(options.Approvals))
	}
	if !HasApprovedPlanStep(options, "plan_abcdefghijklmnop", AWSCUR2MergeBucketPolicyOperationID) {
		t.Fatalf("HasApprovedPlanStep returned false for approved bucket policy merge step: %#v", options.Approvals)
	}
}

func TestExecutionOptionsNormalizeTrimmedAWSBillingOperation(t *testing.T) {
	options, err := NormalizeExecutionOptions(ExecutionOptions{
		AWSBillingOperation: AWSBillingOperation(" create_cur2_export "),
	})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptions returned error: %v", err)
	}
	if options.AWSBillingOperation != AWSBillingOperationCreateCUR2Export {
		t.Fatalf("AWSBillingOperation = %q, want %q", options.AWSBillingOperation, AWSBillingOperationCreateCUR2Export)
	}
}

func TestExecutionOptionsBackfillApprovalUsesPlanStepBinding(t *testing.T) {
	options := ExecutionOptions{
		AWSBillingOperation: AWSBillingOperationRequestBackfill,
		Approvals: []ExecutionApproval{{
			OperationID: AWSBackfillSupportCaseOperationID,
			Intent:      ApprovalIntentRequestBackfillSupportCase,
			PlanID:      "plan_abcdefghijklmnop",
			Confirmed:   true,
		}},
	}

	if !HasApprovedPlanStep(options, "plan_abcdefghijklmnop", AWSBackfillSupportCaseOperationID) {
		t.Fatal("HasApprovedPlanStep returned false for confirmed backfill plan step")
	}
	if HasApprovedPlanStep(options, "plan_ponmlkjihgfedcba", AWSBackfillSupportCaseOperationID) {
		t.Fatal("HasApprovedPlanStep returned true for wrong backfill plan")
	}
	if HasApprovedPlanStep(options, "plan_abcdefghijklmnop", AWSCUR2CreateExportOperationID) {
		t.Fatal("HasApprovedPlanStep returned true for wrong backfill operation")
	}
}

func TestExecutionOptionsCUR2CreateApprovalHelpers(t *testing.T) {
	options := ExecutionOptions{
		AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
		Approvals: []ExecutionApproval{{
			OperationID: AWSCUR2CreateExportOperationID,
			PlanID:      "plan_abcdefghijklmnop",
			Confirmed:   true,
		}},
	}

	if !HasAWSBillingOperation(options, AWSBillingOperationCreateCUR2Export) {
		t.Fatal("HasAWSBillingOperation returned false for create CUR 2.0 export")
	}
	if !HasApprovedPlanStep(options, "plan_abcdefghijklmnop", AWSCUR2CreateExportOperationID) {
		t.Fatal("HasApprovedPlanStep returned false for confirmed plan step")
	}
	if HasApprovedPlanStep(options, "plan_ponmlkjihgfedcba", AWSCUR2CreateExportOperationID) {
		t.Fatal("HasApprovedPlanStep returned true for wrong plan")
	}
	if HasApprovedPlanStep(options, "plan_abcdefghijklmnop", AWSCUR2CreateBucketOperationID) {
		t.Fatal("HasApprovedPlanStep returned true for wrong operation")
	}
}

func TestExecutionOptionsRejectInvalidApprovals(t *testing.T) {
	tests := []struct {
		name   string
		option ExecutionOptions
		want   string
	}{
		{
			name: "missing operation id",
			option: ExecutionOptions{Approvals: []ExecutionApproval{{
				Intent:    ApprovalIntentRequestBackfillSupportCase,
				Confirmed: true,
			}}},
			want: "operation_id",
		},
		{
			name: "missing intent for backfill approval",
			option: ExecutionOptions{Approvals: []ExecutionApproval{{
				OperationID: AWSBackfillSupportCaseOperationID,
				PlanID:      "plan_abcdefghijklmnop",
				Confirmed:   true,
			}}},
			want: "intent",
		},
		{
			name: "missing plan id for backfill approval",
			option: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationRequestBackfill,
				Approvals: []ExecutionApproval{{
					OperationID: AWSBackfillSupportCaseOperationID,
					Intent:      ApprovalIntentRequestBackfillSupportCase,
					Confirmed:   true,
				}},
			},
			want: "plan_id",
		},
		{
			name: "invalid plan id format for backfill approval",
			option: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationRequestBackfill,
				Approvals: []ExecutionApproval{{
					OperationID: AWSBackfillSupportCaseOperationID,
					Intent:      ApprovalIntentRequestBackfillSupportCase,
					PlanID:      "plan_1234",
					Confirmed:   true,
				}},
			},
			want: "plan_id",
		},
		{
			name: "missing plan id for plan-bound approval",
			option: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
				Approvals: []ExecutionApproval{{
					OperationID: AWSCUR2CreateExportOperationID,
					Confirmed:   true,
				}},
			},
			want: "plan_id",
		},
		{
			name: "unsupported operation",
			option: ExecutionOptions{Approvals: []ExecutionApproval{{
				OperationID: "aws.billing.unsupported",
				Intent:      ApprovalIntentRequestBackfillSupportCase,
				Confirmed:   true,
			}}},
			want: "unsupported",
		},
		{
			name: "unsafe operation",
			option: ExecutionOptions{Approvals: []ExecutionApproval{{
				OperationID: "arn:aws:support:::case/example",
				Intent:      ApprovalIntentRequestBackfillSupportCase,
				Confirmed:   true,
			}}},
			want: "unsafe",
		},
		{
			name: "raw arn plan id",
			option: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
				Approvals: []ExecutionApproval{{
					OperationID: AWSCUR2CreateExportOperationID,
					PlanID:      "arn:aws:support:::case/example",
					Confirmed:   true,
				}},
			},
			want: "unsafe",
		},
		{
			name: "invalid plan id format",
			option: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
				Approvals: []ExecutionApproval{{
					OperationID: AWSCUR2CreateExportOperationID,
					PlanID:      "plan_1234",
					Confirmed:   true,
				}},
			},
			want: "plan_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeExecutionOptions(tt.option)
			if err == nil {
				t.Fatal("NormalizeExecutionOptions accepted invalid approval")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestExecutionOptionsRejectAWSBillingOperationConflicts(t *testing.T) {
	request := Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}

	tests := []struct {
		name    string
		option  ExecutionOptions
		wantErr string
	}{
		{
			name: "create cur2 rejected outside aws billing apply prereqs",
			option: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
			},
		},
		{
			name: "operation conflict",
			option: ExecutionOptions{
				AWSBillingOperation: AWSBillingOperationConflict,
			},
			wantErr: "aws_billing_prereqs_operation_conflict",
		},
		{
			name: "approval without matching operation intent",
			option: ExecutionOptions{
				Approvals: []ExecutionApproval{{
					OperationID: AWSCUR2CreateExportOperationID,
					PlanID:      "plan_abcdefghijklmnop",
					Confirmed:   true,
				}},
			},
			wantErr: "matching AWS billing operation",
		},
		{
			name: "backfill approval without matching operation intent",
			option: ExecutionOptions{
				Approvals: []ExecutionApproval{{
					OperationID: AWSBackfillSupportCaseOperationID,
					Intent:      ApprovalIntentRequestBackfillSupportCase,
					PlanID:      "plan_abcdefghijklmnop",
					Confirmed:   true,
				}},
			},
			wantErr: "matching AWS billing operation",
		},
		{
			name: "unconfirmed scoped approval",
			option: ExecutionOptions{
				Approvals: []ExecutionApproval{{
					OperationID: AWSBackfillSupportCaseOperationID,
					Intent:      ApprovalIntentRequestBackfillSupportCase,
					PlanID:      "plan_abcdefghijklmnop",
				}},
			},
			wantErr: "confirmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testRequest := request
			if tt.name == "create cur2 rejected outside aws billing apply prereqs" {
				testRequest.Action = assessment.ActionPreflight
				tt.wantErr = "AWS billing operation"
			}
			_, err := NormalizeExecutionOptionsForRequest(testRequest, tt.option)
			if err == nil {
				t.Fatal("NormalizeExecutionOptionsForRequest accepted invalid AWS billing operation")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestExecutionOptionsRequestAwareValidationScopesAWSSelectorsAndApprovals(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		option  ExecutionOptions
		wantErr string
	}{
		{
			name: "selector allowed on aws billing preflight",
			request: Request{
				Goal:           assessment.RapidAssessment,
				CollectionPath: assessment.CollectionBilling,
				Provider:       assessment.ProviderAWS,
				Action:         assessment.ActionPreflight,
			},
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{Profile: "default"}}},
		},
		{
			name: "selector allowed on aws billing apply prereqs",
			request: Request{
				Goal:           assessment.RapidAssessment,
				CollectionPath: assessment.CollectionBilling,
				Provider:       assessment.ProviderAWS,
				Action:         assessment.ActionApplyPrereqs,
			},
			option: ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{Profile: "default"}}},
		},
		{
			name: "selector rejected on aws billing validate",
			request: Request{
				Goal:           assessment.RapidAssessment,
				CollectionPath: assessment.CollectionBilling,
				Provider:       assessment.ProviderAWS,
				Action:         assessment.ActionValidate,
			},
			option:  ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{Profile: "default"}}},
			wantErr: "AWS selector",
		},
		{
			name: "selector rejected on aws api preflight",
			request: Request{
				Goal:           assessment.RapidAssessment,
				CollectionPath: assessment.CollectionAPI,
				Provider:       assessment.ProviderAWS,
				Action:         assessment.ActionPreflight,
			},
			option:  ExecutionOptions{Selectors: &ExecutionSelectors{AWS: &AWSExecutionSelectors{Profile: "default"}}},
			wantErr: "AWS selector",
		},
		{
			name: "approval rejected on aws billing preflight",
			request: Request{
				Goal:           assessment.RapidAssessment,
				CollectionPath: assessment.CollectionBilling,
				Provider:       assessment.ProviderAWS,
				Action:         assessment.ActionPreflight,
			},
			option: ExecutionOptions{Approvals: []ExecutionApproval{{
				OperationID: AWSBackfillSupportCaseOperationID,
				Intent:      ApprovalIntentRequestBackfillSupportCase,
				PlanID:      "plan_abcdefghijklmnop",
				Confirmed:   true,
			}}},
			wantErr: "approval",
		},
		{
			name: "approval rejected on non aws provider",
			request: Request{
				Goal:           assessment.RapidAssessment,
				CollectionPath: assessment.CollectionBilling,
				Provider:       assessment.ProviderGCP,
				Action:         assessment.ActionApplyPrereqs,
			},
			option: ExecutionOptions{Approvals: []ExecutionApproval{{
				OperationID: AWSBackfillSupportCaseOperationID,
				Intent:      ApprovalIntentRequestBackfillSupportCase,
				PlanID:      "plan_abcdefghijklmnop",
				Confirmed:   true,
			}}},
			wantErr: "approval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeExecutionOptionsForRequest(tt.request, tt.option)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("NormalizeExecutionOptionsForRequest returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("NormalizeExecutionOptionsForRequest accepted invalid scoped options")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want %q", err, tt.wantErr)
			}
		})
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
