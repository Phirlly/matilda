package workflow

import (
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
)

func TestNormalizeExecutionOptionsRejectsUnexpectedCUR2CreateApprovalIntent(t *testing.T) {
	request := Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}

	_, err := NormalizeExecutionOptionsForRequest(request, ExecutionOptions{
		InterfaceMode:       InterfaceModeDirect,
		TimeoutSeconds:      DefaultExecutionTimeoutSeconds,
		AWSBillingOperation: AWSBillingOperationCreateCUR2Export,
		Approvals: []ExecutionApproval{{
			OperationID: AWSCUR2CreateExportOperationID,
			Intent:      ApprovalIntentRequestBackfillSupportCase,
			PlanID:      "plan_abcdefghijklmnop",
			Confirmed:   true,
		}},
	})

	if err == nil {
		t.Fatal("NormalizeExecutionOptionsForRequest accepted unexpected CUR2 create approval intent")
	}
	if !strings.Contains(err.Error(), "approval intent is unsupported for create-cur2-export") {
		t.Fatalf("error = %q, want create approval intent rejection", err)
	}
}
