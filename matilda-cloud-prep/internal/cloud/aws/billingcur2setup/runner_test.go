package billingcur2setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestRunnerPlansCreateNewCUR2ExportWithoutMutation(t *testing.T) {
	client := baselineSetupClient()

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, Code = %q, Message = %q, want %q", result.Status, result.Code, result.Message, workflow.RunStatusManualSteps)
	}
	if result.Code != "aws_cur2_create_export_approval_required" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_approval_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false before approval")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls before approval = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
	if result.Plan == nil {
		t.Fatal("Plan is nil")
	}
	if result.Plan.CoverageRecommendation.CoverageStatus != workflow.CoverageOrganizationWide {
		t.Fatalf("CoverageStatus = %q, want organization_wide", result.Plan.CoverageRecommendation.CoverageStatus)
	}
	if len(client.getTableRequests) != 1 {
		t.Fatalf("GetTable calls = %d, want 1 before planning create-new export", len(client.getTableRequests))
	}
	if client.getTableRequests[0].Name != cur2TableName {
		t.Fatalf("GetTable table = %q, want %s", client.getTableRequests[0].Name, cur2TableName)
	}
	for _, key := range optionalCUR2ContentSettingKeysForTest() {
		if client.getTableRequests[0].Properties[key] != "FALSE" {
			t.Fatalf("GetTable %s = %q, want FALSE for authorized CUR2 table settings", key, client.getTableRequests[0].Properties[key])
		}
	}
	wantSteps := []string{
		workflow.AWSCUR2CreateBucketOperationID,
		workflow.AWSCUR2MergeBucketPolicyOperationID,
		workflow.AWSCUR2CreateExportOperationID,
	}
	if got := mutatingStepIDs(result.Plan.Steps); !equalStrings(got, wantSteps) {
		t.Fatalf("mutating step IDs = %#v, want %#v", got, wantSteps)
	}
	for _, step := range result.Plan.Steps {
		if strings.Contains(strings.ToLower(step.Description), "billing-only") ||
			strings.Contains(strings.ToLower(step.Reason), "billing-only") {
			t.Fatalf("plan step %q used banned invented terminology: description %q reason %q", step.ID, step.Description, step.Reason)
		}
	}
	assertResultDoesNotLeakAWSSecrets(t, result)
}

func TestRunnerBlocksBeforeMutationWhenCompleteCUR2SchemaUnavailable(t *testing.T) {
	client := baselineSetupClient()
	client.getTableErr = NewProviderError("aws_cur2_table_unavailable", "table unavailable")

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_table_unavailable" {
		t.Fatalf("Code = %q, want aws_cur2_table_unavailable", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when schema lookup fails")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerBlocksBeforeMutationWhenCompleteCUR2SchemaIsUnsafe(t *testing.T) {
	client := baselineSetupClient()
	client.table.Columns = []string{"identity_line_item_id", "line_item_usage_amount)", "product"}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_table_invalid_shape" {
		t.Fatalf("Code = %q, want aws_cur2_table_invalid_shape", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when schema is unsafe")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerListsExistingSameAccountBucketsForSelectionWithoutMutation(t *testing.T) {
	client := baselineSetupClient()
	client.buckets = []BucketSummary{
		{Name: "matilda-existing-cur2", Region: "us-west-2"},
		{Name: "matilda-existing-cur2-logs", Region: "us-west-2"},
	}

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", ""))

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, Code = %q, want manual steps", result.Status, result.Code)
	}
	if result.Code != "aws_cur2_existing_bucket_selection_required" {
		t.Fatalf("Code = %q, want aws_cur2_existing_bucket_selection_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false while selecting bucket")
	}
	if len(client.listBucketsRequests) != 1 {
		t.Fatalf("ListBuckets calls = %d, want 1", len(client.listBucketsRequests))
	}
	if client.listBucketsRequests[0].Region != "us-west-2" {
		t.Fatalf("ListBuckets region = %q, want selected region", client.listBucketsRequests[0].Region)
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls before bucket selection = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
	ref := checkEvidenceValue(result, "candidate_1_bucket_ref")
	if ref == "" || !strings.HasPrefix(ref, "s3b-") {
		t.Fatalf("candidate_1_bucket_ref = %q, want safe s3b ref", ref)
	}
	if got := checkEvidenceValue(result, "candidate_1_bucket_label"); got != "matilda-existing-cur2" {
		t.Fatalf("candidate_1_bucket_label = %q, want audited bucket name", got)
	}
	if got := checkEvidenceValue(result, "candidate_1_bucket_region"); got != "us-west-2" {
		t.Fatalf("candidate_1_bucket_region = %q, want us-west-2", got)
	}
	assertResultDoesNotLeakAWSSecrets(t, result)
}

func TestRunnerMasksUnsafeExistingBucketSelectionLabels(t *testing.T) {
	client := baselineSetupClient()
	client.buckets = []BucketSummary{{Name: "customer-123456789012-plain-token", Region: "us-west-2"}}

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", ""))

	ref := checkEvidenceValue(result, "candidate_1_bucket_ref")
	label := checkEvidenceValue(result, "candidate_1_bucket_label")
	if ref == "" || !strings.HasPrefix(ref, "s3b-") {
		t.Fatalf("candidate_1_bucket_ref = %q, want safe ref", ref)
	}
	if label != "bucket-"+ref {
		t.Fatalf("candidate_1_bucket_label = %q, want masked label bucket-%s", label, ref)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	for _, forbidden := range []string{"customer-123456789012-plain-token", "plain-token", "123456789012"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("result leaked unsafe bucket label %q in %s", forbidden, string(encoded))
		}
	}
}

func TestRunnerExistingBucketSelectionHandlesNoDiscoverableBuckets(t *testing.T) {
	client := baselineSetupClient()

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", ""))

	if result.Code != "aws_cur2_existing_bucket_selection_required" {
		t.Fatalf("Code = %q, want selection required", result.Code)
	}
	if got := checkEvidenceValue(result, "candidate_count"); got != "0" {
		t.Fatalf("candidate_count = %q, want 0", got)
	}
	if result.Plan == nil || len(result.Plan.Steps) != 1 {
		t.Fatalf("Plan steps = %#v, want one guide step", result.Plan)
	}
	if !strings.Contains(result.Plan.Steps[0].CurrentState, "No existing same-account S3 buckets") {
		t.Fatalf("CurrentState = %q, want no-buckets guidance", result.Plan.Steps[0].CurrentState)
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerExistingBucketSelectionListFailureBlocksBeforeMutation(t *testing.T) {
	client := baselineSetupClient()
	client.listBucketsErr = NewProviderError("aws_s3_list_buckets_failed", "bucket listing denied")

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", ""))

	if result.Code != "aws_s3_list_buckets_failed" {
		t.Fatalf("Code = %q, want aws_s3_list_buckets_failed", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", result.Status)
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerExistingBucketSelectionRejectsUnknownBucketRef(t *testing.T) {
	client := baselineSetupClient()
	client.buckets = []BucketSummary{{Name: "matilda-existing-cur2", Region: "us-west-2"}}

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", "s3b-abcdefghijklmnop"))

	if result.Code != "aws_cur2_existing_bucket_ref_not_found" {
		t.Fatalf("Code = %q, want aws_cur2_existing_bucket_ref_not_found", result.Code)
	}
	if len(client.headBucketRequests) != 0 {
		t.Fatalf("HeadBucket calls = %d, want none for unknown selected ref", len(client.headBucketRequests))
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerBlocksSameNameMismatchedGeneratedExportBeforeMutation(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	facts := setupFactsFromPlan(t, client, preview.Plan)
	client.headBucketRequests = nil
	client.getPolicyRequests = nil
	client.exports = []cur2preflight.Export{{
		Name:                facts.ExportName,
		ExportARN:           "arn:aws:bcm-data-exports:us-east-1:123456789012:export/" + facts.ExportName,
		QueryStatement:      "SELECT " + requiredCUR2SelectForSetupTest() + " FROM COST_AND_USAGE_REPORT",
		TableConfigurations: matildaCUR2TableConfigurations(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}),
		Destination: cur2preflight.S3Destination{
			Bucket:      facts.BucketName,
			BucketOwner: client.identity.AccountID,
			Prefix:      facts.Prefix,
			Region:      "us-west-2",
			Output: cur2preflight.S3Output{
				Format:      "TEXT_OR_CSV",
				Compression: "GZIP",
				Overwrite:   "CREATE_NEW_REPORT",
				OutputType:  "CUSTOM",
			},
		},
		RefreshCadence: "SYNCHRONOUS",
	}}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_generated_export_name_conflict" {
		t.Fatalf("Code = %q, want aws_cur2_generated_export_name_conflict", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for generated export name conflict")
	}
	if len(client.headBucketRequests) != 0 || len(client.getPolicyRequests) != 0 {
		t.Fatalf("S3 calls = head %d policy %d, want none before name conflict is resolved", len(client.headBucketRequests), len(client.getPolicyRequests))
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerDoesNotBlockGeneratedPlanForUnusedLaterNameConflict(t *testing.T) {
	client := baselineSetupClient()
	identity := identityContext{AccountID: client.identity.AccountID, Partition: "aws"}
	candidates := generatedNameCandidates(identity, "us-west-2")
	if len(candidates) < 2 {
		t.Fatalf("generated candidates = %#v, want at least two", candidates)
	}
	unusedFacts := candidates[1]
	client.exports = []cur2preflight.Export{{
		Name:                unusedFacts.ExportName,
		ExportARN:           "arn:aws:bcm-data-exports:us-east-1:123456789012:export/" + unusedFacts.ExportName,
		QueryStatement:      "SELECT " + requiredCUR2SelectForSetupTest() + " FROM COST_AND_USAGE_REPORT",
		TableConfigurations: matildaCUR2TableConfigurations(identity),
		Destination: cur2preflight.S3Destination{
			Bucket:      unusedFacts.BucketName,
			BucketOwner: client.identity.AccountID,
			Prefix:      unusedFacts.Prefix,
			Region:      "us-west-2",
			Output: cur2preflight.S3Output{
				Format:      "TEXT_OR_CSV",
				Compression: "GZIP",
				Overwrite:   "CREATE_NEW_REPORT",
				OutputType:  "CUSTOM",
			},
		},
		RefreshCadence: "SYNCHRONOUS",
	}}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_create_export_approval_required" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_approval_required for first available generated candidate", result.Code)
	}
	if got := checkEvidenceValue(result, "candidate_index"); got != "00" {
		t.Fatalf("candidate_index = %q, want first generated candidate 00", got)
	}
	if len(client.headBucketRequests) != 1 {
		t.Fatalf("HeadBucket calls = %d, want first generated candidate checked", len(client.headBucketRequests))
	}
	if client.headBucketRequests[0].Bucket != candidates[0].BucketName {
		t.Fatalf("HeadBucket bucket = %q, want first candidate %q", client.headBucketRequests[0].Bucket, candidates[0].BucketName)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for plan-only create-new")
	}
}

func TestRunnerBlocksSameNameMismatchedExistingBucketExportBeforeMutation(t *testing.T) {
	client := baselineSetupClient()
	client.bucketExists = true
	client.buckets = []BucketSummary{{Name: "matilda-existing-cur2", Region: "us-west-2"}}
	identity := identityContext{AccountID: client.identity.AccountID, Partition: "aws"}
	candidates := existingBucketCandidates(identity, "us-west-2", client.buckets)
	if len(candidates) != 1 {
		t.Fatalf("existing bucket candidates = %#v, want one", candidates)
	}
	facts := candidates[0]
	client.exports = []cur2preflight.Export{{
		Name:                facts.ExportName,
		ExportARN:           "arn:aws:bcm-data-exports:us-east-1:123456789012:export/" + facts.ExportName,
		QueryStatement:      "SELECT " + requiredCUR2SelectForSetupTest() + " FROM COST_AND_USAGE_REPORT",
		TableConfigurations: matildaCUR2TableConfigurations(identity),
		Destination: cur2preflight.S3Destination{
			Bucket:      facts.BucketName,
			BucketOwner: client.identity.AccountID,
			Prefix:      facts.Prefix,
			Region:      "us-west-2",
			Output: cur2preflight.S3Output{
				Format:      "TEXT_OR_CSV",
				Compression: "GZIP",
				Overwrite:   "CREATE_NEW_REPORT",
				OutputType:  "CUSTOM",
			},
		},
		RefreshCadence: "SYNCHRONOUS",
	}}

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", facts.BucketRef))

	if result.Code != "aws_cur2_generated_export_name_conflict" {
		t.Fatalf("Code = %q, want aws_cur2_generated_export_name_conflict", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for generated export name conflict")
	}
	if len(client.headBucketRequests) != 0 || len(client.getPolicyRequests) != 0 {
		t.Fatalf("S3 calls = head %d policy %d, want none before name conflict is resolved", len(client.headBucketRequests), len(client.getPolicyRequests))
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerExistingBucketSelectionBlocksInaccessibleSelectedBucketBeforeMutation(t *testing.T) {
	client := baselineSetupClient()
	client.buckets = []BucketSummary{{Name: "matilda-existing-cur2", Region: "us-west-2"}}
	bucketRef := safeS3BucketRef(client.identity.AccountID, "matilda-existing-cur2", "us-west-2")

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", bucketRef))

	if result.Code != "aws_s3_bucket_not_found" {
		t.Fatalf("Code = %q, want aws_s3_bucket_not_found", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", result.Status)
	}
	if len(client.headBucketRequests) != 1 {
		t.Fatalf("HeadBucket calls = %d, want selected bucket validation", len(client.headBucketRequests))
	}
	head := client.headBucketRequests[0]
	if head.Bucket != "matilda-existing-cur2" || head.ExpectedOwner != client.identity.AccountID || head.Region != "us-west-2" {
		t.Fatalf("HeadBucket request = %#v, want selected same-account bucket proof", head)
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerExistingBucketSelectionHonorsPaginationAndQuotaBlock(t *testing.T) {
	client := baselineSetupClient()
	client.buckets = []BucketSummary{{Name: "matilda-existing-cur2", Region: "us-west-2"}}
	client.listBucketsNextToken = "next-token"
	for index := 0; index < 5; index++ {
		client.exports = append(client.exports, cur2preflight.Export{
			Name:           fmt.Sprintf("external-cur2-%d", index),
			ExportARN:      fmt.Sprintf("arn:aws:bcm-data-exports:us-east-1:123456789012:export/external-cur2-%d", index),
			QueryStatement: "SELECT line_item_usage_amount FROM COST_AND_USAGE_REPORT",
		})
	}
	bucketRef := safeS3BucketRef(client.identity.AccountID, "matilda-existing-cur2", "us-west-2")

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", bucketRef))

	if result.Code != "aws_cur2_export_quota_full" {
		t.Fatalf("Code = %q, want aws_cur2_export_quota_full", result.Code)
	}
	if len(client.listBucketsRequests) != 2 {
		t.Fatalf("ListBuckets calls = %d, want two paginated calls", len(client.listBucketsRequests))
	}
	if client.listBucketsRequests[1].Token != "next-token" {
		t.Fatalf("second ListBuckets token = %q, want next-token", client.listBucketsRequests[1].Token)
	}
	if len(client.headBucketRequests) != 0 {
		t.Fatalf("HeadBucket calls = %d, want none after quota block", len(client.headBucketRequests))
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerExistingBucketSelectionBlocksUnboundedBucketPaginationBeforeMutation(t *testing.T) {
	client := baselineSetupClient()
	client.buckets = []BucketSummary{{Name: "matilda-existing-cur2", Region: "us-west-2"}}
	client.listBucketsNextToken = "next-token"
	client.listBucketsAlwaysNextToken = true

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", ""))

	if result.Code != "aws_s3_list_buckets_pagination_unbounded" {
		t.Fatalf("Code = %q, want aws_s3_list_buckets_pagination_unbounded", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", result.Status)
	}
	if len(client.listBucketsRequests) != 10 {
		t.Fatalf("ListBuckets calls = %d, want bounded retry limit of 10", len(client.listBucketsRequests))
	}
	if len(client.headBucketRequests) != 0 {
		t.Fatalf("HeadBucket calls = %d, want none after unbounded bucket listing", len(client.headBucketRequests))
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerUsesSelectedExistingSameAccountBucketWithoutCreatingBucket(t *testing.T) {
	client := baselineSetupClient()
	client.bucketExists = true
	client.buckets = []BucketSummary{{Name: "matilda-existing-cur2", Region: "us-west-2"}}
	bucketRef := safeS3BucketRef(client.identity.AccountID, "matilda-existing-cur2", "us-west-2")
	preview := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", bucketRef))

	if preview.Code != "aws_cur2_create_export_approval_required" {
		t.Fatalf("preview Code = %q, want approval required", preview.Code)
	}
	if got := mutatingStepIDs(preview.Plan.Steps); !equalStrings(got, []string{workflow.AWSCUR2MergeBucketPolicyOperationID, workflow.AWSCUR2CreateExportOperationID}) {
		t.Fatalf("mutating steps = %#v, want policy merge and export create only", got)
	}
	if got := checkEvidenceValue(preview, "destination_mode"); got != string(workflow.AWSCUR2DestinationExistingSameAccount) {
		t.Fatalf("destination_mode = %q, want existing_same_account", got)
	}
	if got := checkEvidenceValue(preview, "selected_s3_bucket_ref"); got != bucketRef {
		t.Fatalf("selected_s3_bucket_ref = %q, want %q", got, bucketRef)
	}

	result := runSetup(t, client, approvedExistingBucketCUR2Options("default", "us-west-2", bucketRef, preview.Plan))

	if result.Code != "aws_cur2_create_export_created" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_created", result.Code)
	}
	if client.createBucketCalls() != 0 {
		t.Fatalf("CreateBucket calls = %d, want 0 for selected existing bucket", client.createBucketCalls())
	}
	if client.putBucketPolicyCalls() != 1 {
		t.Fatalf("PutBucketPolicy calls = %d, want 1", client.putBucketPolicyCalls())
	}
	if client.createExportCalls() != 1 {
		t.Fatalf("CreateExport calls = %d, want 1", client.createExportCalls())
	}
	exportRequest := client.createExportRequests[0]
	if exportRequest.Destination.Bucket != "matilda-existing-cur2" {
		t.Fatalf("CreateExport bucket = %q, want selected existing bucket", exportRequest.Destination.Bucket)
	}
	if exportRequest.Destination.BucketOwner != client.identity.AccountID {
		t.Fatalf("CreateExport bucket owner = %q, want caller account", exportRequest.Destination.BucketOwner)
	}
	if exportRequest.Destination.Prefix != matildaBillingPrefix {
		t.Fatalf("CreateExport prefix = %q, want Matilda billing prefix", exportRequest.Destination.Prefix)
	}
	assertResultDoesNotLeakAWSSecrets(t, result)
}

func TestBuildCreateExportRequestIncludesPrimaryBillingViewARN(t *testing.T) {
	identity := identityContext{AccountID: "123456789012", Partition: "aws"}
	existingCandidates := existingBucketCandidates(identity, "us-west-2", []BucketSummary{{Name: "matilda-existing-cur2", Region: "us-west-2"}})
	if len(existingCandidates) != 1 {
		t.Fatalf("existing bucket candidates = %#v, want one", existingCandidates)
	}

	tests := []struct {
		name string
		plan setupPlan
	}{
		{
			name: "generated destination",
			plan: setupPlan{
				Facts:    generatedNameCandidates(identity, "us-west-2")[0],
				Identity: identity,
				Region:   "us-west-2",
			},
		},
		{
			name: "existing same-account destination",
			plan: setupPlan{
				Facts:    existingCandidates[0],
				Identity: identity,
				Region:   "us-west-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.plan.QueryStatement = "SELECT identity_line_item_id, line_item_product_code, product.product_name AS product_product_name FROM COST_AND_USAGE_REPORT"
			request := buildCreateExportRequest(tt.plan)
			if got := request.TableConfigurations[cur2TableName]["BILLING_VIEW_ARN"]; got != "arn:aws:billing::123456789012:billingview/primary" {
				t.Fatalf("BILLING_VIEW_ARN = %q, want primary billing view ARN", got)
			}
		})
	}
}

func TestBuildCreateExportRequestUsesCompleteTableSchemaQuery(t *testing.T) {
	identity := identityContext{AccountID: "123456789012", Partition: "aws"}
	query, err := matildaCUR2QueryStatement(completeCUR2TableColumnsForTest())
	if err != nil {
		t.Fatalf("matildaCUR2QueryStatement returned error: %v", err)
	}
	plan := setupPlan{
		Facts:          generatedNameCandidates(identity, "us-west-2")[0],
		Identity:       identity,
		Region:         "us-west-2",
		QueryStatement: query,
	}

	request := buildCreateExportRequest(plan)

	for _, want := range completeCUR2TableColumnsForTest() {
		if !strings.Contains(request.QueryStatement, want) {
			t.Fatalf("QueryStatement = %q, want complete schema column %q", request.QueryStatement, want)
		}
	}
	for _, forbidden := range []string{"SELECT *", " WHERE ", " LIMIT "} {
		if strings.Contains(strings.ToUpper(request.QueryStatement), forbidden) {
			t.Fatalf("QueryStatement = %q, want no %s", request.QueryStatement, forbidden)
		}
	}
	if !strings.Contains(request.QueryStatement, "product.product_name AS product_product_name") {
		t.Fatalf("QueryStatement = %q, want Matilda product name logical alias", request.QueryStatement)
	}
	if strings.Contains(request.QueryStatement, "line_item_product_code, product.product_name AS product_product_name, line_item_operation") {
		t.Fatalf("QueryStatement = %q, want complete schema query instead of reduced mandatory-field query", request.QueryStatement)
	}
}

func TestMatildaCUR2QueryStatementRejectsUnsafeSchema(t *testing.T) {
	tests := []struct {
		name    string
		columns []string
	}{
		{name: "empty schema", columns: nil},
		{name: "unsafe syntax", columns: []string{"identity_line_item_id", "line_item_usage_amount)", "product"}},
		{name: "whitespace padded schema name", columns: func() []string {
			columns := completeCUR2TableColumnsForTest()
			columns[1] = " line_item_product_code"
			return columns
		}()},
		{name: "duplicate", columns: []string{"identity_line_item_id", "identity_line_item_id", "product"}},
		{name: "missing product logical source", columns: requiredSetupColumnsWithoutProductName()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if query, err := matildaCUR2QueryStatement(tt.columns); err == nil {
				t.Fatalf("matildaCUR2QueryStatement = %q nil error, want failure", query)
			}
		})
	}
}

func TestMatildaCUR2QueryStatementRejectsAWSLengthOverflow(t *testing.T) {
	columns := completeCUR2TableColumnsForTest()
	for index := 0; index < 4000; index++ {
		columns = append(columns, fmt.Sprintf("synthetic_column_%04d", index))
	}

	if query, err := matildaCUR2QueryStatement(columns); err == nil {
		t.Fatalf("matildaCUR2QueryStatement length %d nil error, want failure", len(query))
	}
}

func TestMatildaCUR2QueryStatementDoesNotDuplicatePhysicalProductName(t *testing.T) {
	columns := append(requiredSetupColumnsWithoutProductName(), "product_product_name")

	query, err := matildaCUR2QueryStatement(columns)

	if err != nil {
		t.Fatalf("matildaCUR2QueryStatement returned error: %v", err)
	}
	if strings.Contains(query, "product.product_name AS product_product_name") {
		t.Fatalf("QueryStatement = %q, want no product alias when physical product_product_name exists", query)
	}
	if !strings.Contains(query, "product_product_name") {
		t.Fatalf("QueryStatement = %q, want physical product_product_name", query)
	}
}

func TestMatildaCUR2TableConfigurationsPreserveAuthorizedSettings(t *testing.T) {
	configs := matildaCUR2TableConfigurations(identityContext{AccountID: "123456789012", Partition: "aws"})[cur2TableName]

	for _, key := range optionalCUR2ContentSettingKeysForTest() {
		if configs[key] != "FALSE" {
			t.Fatalf("%s = %q, want FALSE for authorized CUR2 table settings", key, configs[key])
		}
	}
	if configs["TIME_GRANULARITY"] != "MONTHLY" {
		t.Fatalf("TIME_GRANULARITY = %q, want MONTHLY", configs["TIME_GRANULARITY"])
	}
	if configs["BILLING_VIEW_ARN"] != "arn:aws:billing::123456789012:billingview/primary" {
		t.Fatalf("BILLING_VIEW_ARN = %q, want primary billing view ARN", configs["BILLING_VIEW_ARN"])
	}
}

func optionalCUR2ContentSettingKeysForTest() []string {
	return []string{
		"INCLUDE_RESOURCES",
		"INCLUDE_SPLIT_COST_ALLOCATION_DATA",
		"INCLUDE_CAPACITY_RESERVATION_DATA",
		"INCLUDE_IAM_PRINCIPAL_DATA",
		"INCLUDE_MANUAL_DISCOUNT_COMPATIBILITY",
	}
}

func completeCUR2TableColumnsForTest() []string {
	return []string{
		"identity_line_item_id",
		"identity_time_interval",
		"bill_invoice_id",
		"bill_billing_entity",
		"bill_billing_period_start_date",
		"line_item_product_code",
		"line_item_operation",
		"line_item_line_item_description",
		"line_item_line_item_type",
		"line_item_currency_code",
		"pricing_unit",
		"line_item_usage_amount",
		"line_item_unblended_cost",
		"line_item_usage_type",
		"line_item_legal_entity",
		"pricing_term",
		"product",
		"discount",
		"cost_category",
		"resource_tags",
	}
}

func completeCUR2QueryStatementForTest() string {
	query, err := matildaCUR2QueryStatement(completeCUR2TableColumnsForTest())
	if err != nil {
		panic(err)
	}
	return query
}

func requiredSetupColumnsWithoutProductName() []string {
	return []string{
		"identity_line_item_id",
		"line_item_product_code",
		"line_item_operation",
		"line_item_line_item_description",
		"line_item_line_item_type",
		"line_item_currency_code",
		"pricing_unit",
		"line_item_usage_amount",
		"line_item_unblended_cost",
		"line_item_usage_type",
	}
}

func requiredCUR2SelectForSetupTest() string {
	return strings.Join(append(requiredSetupColumnsWithoutProductName(), "product.product_name AS product_product_name"), ", ")
}

func TestRunnerReusesManagedExportForSelectedExistingSameAccountBucket(t *testing.T) {
	client := baselineSetupClient()
	client.buckets = []BucketSummary{{Name: "matilda-existing-cur2", Region: "us-west-2"}}
	identity := identityContext{
		AccountID: client.identity.AccountID,
		Partition: partitionFromARN(client.identity.CallerARN),
	}
	candidates := existingBucketCandidates(identity, "us-west-2", client.buckets)
	if len(candidates) != 1 {
		t.Fatalf("existing bucket candidates = %#v, want one", candidates)
	}
	configureReusableManagedExport(t, client, candidates[0])

	result := runSetup(t, client, createCUR2ExistingBucketSelectionOptions("default", "us-west-2", candidates[0].BucketRef))

	if result.Code != "aws_cur2_create_export_reused" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_reused", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when reusing existing managed export")
	}
	if got := checkEvidenceValue(result, "selected_s3_bucket_ref"); got != candidates[0].BucketRef {
		t.Fatalf("selected_s3_bucket_ref = %q, want selected existing bucket ref", got)
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
	if len(client.headBucketRequests) != 1 || len(client.getPolicyRequests) != 1 {
		t.Fatalf("reuse validation calls = head %d policy %d, want one each", len(client.headBucketRequests), len(client.getPolicyRequests))
	}
	if client.headBucketRequests[0].Bucket != "matilda-existing-cur2" ||
		client.headBucketRequests[0].ExpectedOwner != client.identity.AccountID {
		t.Fatalf("HeadBucket request = %#v, want selected bucket and caller owner", client.headBucketRequests[0])
	}
	assertResultDoesNotLeakAWSSecrets(t, result)
}

func TestSelectedCUR2S3BucketRefHandlesMissingSelectorsAndTrimsValue(t *testing.T) {
	if got := selectedCUR2S3BucketRef(workflow.ExecutionOptions{Selectors: &workflow.ExecutionSelectors{}}); got != "" {
		t.Fatalf("selectedCUR2S3BucketRef without AWS selectors = %q, want empty", got)
	}

	options := workflow.ExecutionOptions{
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{CUR2S3BucketRef: " s3b-abcdefghijklmnop "},
		},
	}
	if got := selectedCUR2S3BucketRef(options); got != "s3b-abcdefghijklmnop" {
		t.Fatalf("selectedCUR2S3BucketRef = %q, want trimmed ref", got)
	}
}

func TestRunnerRequiresCreateOperationBeforeResolvingClient(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	options := approvedCreateCUR2Options("default", "us-west-2", preview.Plan)
	options.AWSBillingOperation = ""
	factoryCalls := 0
	runner := NewRunner(RunnerConfig{
		ClientFactory: func(workflow.ExecutionOptions) Client {
			factoryCalls++
			return client
		},
		Now: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	})

	result := runner.Run(context.Background(), awsBillingApplyPrereqsRequest(), options)

	if result.Code != "aws_cur2_create_export_operation_required" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_operation_required", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if factoryCalls != 0 || client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("provider calls = factory %d bucket %d policy %d export %d, want none", factoryCalls, client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerRequiresAWSBillingApplyPrereqsRequestBeforeResolvingClient(t *testing.T) {
	client := baselineSetupClient()
	factoryCalls := 0
	runner := NewRunner(RunnerConfig{
		ClientFactory: func(workflow.ExecutionOptions) Client {
			factoryCalls++
			return client
		},
		Now: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	})
	request := awsBillingApplyPrereqsRequest()
	request.Action = assessment.ActionPreflight

	result := runner.Run(context.Background(), request, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_billing_apply_prereqs_request_required" {
		t.Fatalf("Code = %q, want aws_billing_apply_prereqs_request_required", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if factoryCalls != 0 || client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("provider calls = factory %d bucket %d policy %d export %d, want none", factoryCalls, client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerRejectsUnparseableCallerPartitionBeforePlanning(t *testing.T) {
	client := baselineSetupClient()
	client.identity.CallerARN = "not-an-arn"

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_auth_partition_unresolved" {
		t.Fatalf("Code = %q, want aws_auth_partition_unresolved", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Plan == nil {
		t.Fatal("Plan is nil, want safe blocked plan")
	}
	if !result.Plan.Approval.Blocked || len(result.Plan.Steps) != 1 || result.Plan.Steps[0].Intent != workflow.PlanStepBlocked {
		t.Fatalf("Plan = %#v, want safe blocked plan", result.Plan)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if len(client.headBucketRequests) != 0 || len(client.getPolicyRequests) != 0 {
		t.Fatalf("bucket checks = head %d policy %d, want none when caller partition cannot be verified", len(client.headBucketRequests), len(client.getPolicyRequests))
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerReportsVerifiedIdentityForPostIdentityPlanFailure(t *testing.T) {
	client := baselineSetupClient()
	client.listExportsErr = NewProviderError("aws_data_exports_access_denied", "data exports denied")
	runner := NewRunner(RunnerConfig{
		Client: client,
		Now:    time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	})

	result := runner.Run(context.Background(), awsBillingApplyPrereqsRequest(), createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_data_exports_access_denied" {
		t.Fatalf("Code = %q, want aws_data_exports_access_denied", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.PlanInput == nil {
		t.Fatal("PlanInput is nil")
	}
	identity := result.PlanInput.OperatorIdentitySummary
	if identity.IdentityStatus != "verified" {
		t.Fatalf("IdentityStatus = %q, want verified", identity.IdentityStatus)
	}
	if !strings.Contains(identity.Summary, "account-ending-9012") {
		t.Fatalf("identity summary = %q, want redacted verified account", identity.Summary)
	}
	if strings.Contains(identity.Summary, "could not verify caller identity") {
		t.Fatalf("identity summary used pre-identity wording after identity verification: %q", identity.Summary)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if len(client.headBucketRequests) != 0 || len(client.getPolicyRequests) != 0 {
		t.Fatalf("bucket checks = head %d policy %d, want none after list exports failure", len(client.headBucketRequests), len(client.getPolicyRequests))
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerPlanOutputDoesNotExposeGeneratedResourceNames(t *testing.T) {
	client := baselineSetupClient()

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Plan == nil {
		t.Fatal("Plan is nil")
	}
	for _, key := range []string{"bucket_name", "export_name", "s3_prefix"} {
		if got := checkEvidenceValue(result, key); got != "" {
			t.Fatalf("evidence %s = %q, want omitted from terminal/JSON output", key, got)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	for _, forbidden := range []string{
		"matilda-ra-billing-aws",
		"matilda-cur2-ra-billing",
		matildaBillingPrefix,
		client.identity.AccountID,
		client.identity.CallerARN,
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("result leaked forbidden value %q in %s", forbidden, string(encoded))
		}
	}
}

func TestSetupPlanInputBuildsWorkflowPlan(t *testing.T) {
	client := baselineSetupClient()
	runner := NewRunner(RunnerConfig{Client: client})
	options := createCUR2Options("default", "us-west-2")
	plan, err := runner.buildPlan(context.Background(), client, options)
	if err != nil {
		t.Fatalf("buildPlan returned error: %v", err)
	}
	input := setupPlanInput(awsBillingApplyPrereqsRequest(), plan, planSteps(plan), []workflow.PlanCheck{planFactsCheck(plan)})

	if _, err := workflow.BuildExecutionPlan(input); err != nil {
		t.Fatalf("BuildExecutionPlan returned error: %v", err)
	}
}

func TestRunnerCreatesBucketPolicyAndExportAfterPlanBoundApproval(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	if preview.Plan == nil {
		t.Fatal("preview Plan is nil")
	}
	options := approvedCreateCUR2Options("default", "us-west-2", preview.Plan)
	currentPlan, currentSteps := currentSetupPlanForTest(t, client, options)
	if currentPlan.PlanID != preview.Plan.PlanID {
		t.Fatalf("current plan ID = %q, preview plan ID = %q, current steps = %#v, approvals = %#v", currentPlan.PlanID, preview.Plan.PlanID, mutatingStepIDs(currentSteps), options.Approvals)
	}
	if state := createApprovalState(options, currentPlan.PlanID, currentSteps); state != approvalReady {
		t.Fatalf("approval state = %q, current steps = %#v, approvals = %#v", state, mutatingStepIDs(currentSteps), options.Approvals)
	}

	result := runSetup(t, client, options)

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, Code = %q, Message = %q, want %q", result.Status, result.Code, result.Message, workflow.RunStatusManualSteps)
	}
	if result.Code != "aws_cur2_create_export_created" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_created", result.Code)
	}
	if !result.Mutated {
		t.Fatal("Mutated = false, want true after approved setup")
	}
	if result.Plan == nil {
		t.Fatal("Plan is nil")
	}
	if !result.Plan.Approval.Required || !result.Plan.Approval.Approved || result.Plan.Approval.Blocked {
		t.Fatalf("approval summary = %#v, want required and approved after approved setup", result.Plan.Approval)
	}
	if client.createBucketCalls() != 1 {
		t.Fatalf("CreateBucket calls = %d, want 1", client.createBucketCalls())
	}
	if client.putBucketPolicyCalls() != 1 {
		t.Fatalf("PutBucketPolicy calls = %d, want 1", client.putBucketPolicyCalls())
	}
	if client.createExportCalls() != 1 {
		t.Fatalf("CreateExport calls = %d, want 1", client.createExportCalls())
	}
	if len(client.getExportRequests) == 0 || client.getExportRequests[len(client.getExportRequests)-1] != client.createdExports[0].ExportARN {
		t.Fatalf("post-create GetExport requests = %#v, want created export ARN %q", client.getExportRequests, client.createdExports[0].ExportARN)
	}
	for _, request := range client.headBucketRequests {
		if request.ExpectedOwner != client.identity.AccountID {
			t.Fatalf("HeadBucket ExpectedOwner = %q, want caller account", request.ExpectedOwner)
		}
	}
	if client.getPolicyRequests[0].ExpectedOwner != client.identity.AccountID {
		t.Fatalf("GetBucketPolicy ExpectedOwner = %q, want caller account", client.getPolicyRequests[0].ExpectedOwner)
	}
	if client.putPolicyRequests[0].ExpectedOwner != client.identity.AccountID {
		t.Fatalf("PutBucketPolicy ExpectedOwner = %q, want caller account", client.putPolicyRequests[0].ExpectedOwner)
	}

	exportRequest := client.createExportRequests[0]
	if exportRequest.DataExportsRegion != "us-east-1" {
		t.Fatalf("DataExportsRegion = %q, want us-east-1", exportRequest.DataExportsRegion)
	}
	if exportRequest.Destination.Region != "us-west-2" {
		t.Fatalf("S3Destination region = %q, want selected bucket region", exportRequest.Destination.Region)
	}
	if exportRequest.Destination.Output.Format != "TEXT_OR_CSV" ||
		exportRequest.Destination.Output.Compression != "GZIP" ||
		exportRequest.Destination.Output.Overwrite != "CREATE_NEW_REPORT" ||
		exportRequest.Destination.Output.OutputType != "CUSTOM" {
		t.Fatalf("output settings = %#v, want preferred Matilda CUR2 settings", exportRequest.Destination.Output)
	}
	if exportRequest.RefreshCadence != "SYNCHRONOUS" {
		t.Fatalf("RefreshCadence = %q, want SYNCHRONOUS", exportRequest.RefreshCadence)
	}
	if exportRequest.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"] != "MONTHLY" {
		t.Fatalf("TIME_GRANULARITY = %q, want MONTHLY", exportRequest.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"])
	}
	for _, key := range optionalCUR2ContentSettingKeysForTest() {
		if exportRequest.TableConfigurations[cur2TableName][key] != "FALSE" {
			t.Fatalf("%s = %q, want FALSE for authorized CUR2 table settings", key, exportRequest.TableConfigurations[cur2TableName][key])
		}
	}
	for _, want := range completeCUR2TableColumnsForTest() {
		if !strings.Contains(exportRequest.QueryStatement, want) {
			t.Fatalf("CreateExport QueryStatement = %q, want schema column %q", exportRequest.QueryStatement, want)
		}
	}
	if !strings.Contains(client.putPolicyRequests[0].Policy, `"aws:SourceAccount":"123456789012"`) {
		t.Fatalf("bucket policy does not include source account condition: %s", client.putPolicyRequests[0].Policy)
	}
	if !strings.Contains(client.putPolicyRequests[0].Policy, `"aws:SourceArn":"arn:aws:bcm-data-exports:us-east-1:123456789012:export/*"`) {
		t.Fatalf("bucket policy does not include Data Exports source ARN: %s", client.putPolicyRequests[0].Policy)
	}
	assertDataExportsPolicyShape(t, client.putPolicyRequests[0].Policy)
	selectedRef := checkEvidenceValue(result, "selected_export_ref")
	if selectedRef == "" || !strings.HasPrefix(selectedRef, "cur2-") {
		t.Fatalf("selected_export_ref = %q, want safe cur2 ref", selectedRef)
	}
	if strings.Contains(selectedRef, "arn:aws") || strings.Contains(selectedRef, client.identity.AccountID) {
		t.Fatalf("selected_export_ref leaked unsafe identifier: %q", selectedRef)
	}
	assertResultDoesNotLeakAWSSecrets(t, result)
}

func TestRunnerRejectsCreateApprovalWhenCallerAccountChanges(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	if preview.Plan == nil {
		t.Fatal("preview Plan is nil")
	}
	options := approvedCreateCUR2Options("default", "us-west-2", preview.Plan)
	client.identity.AccountID = "210987654321"
	client.identity.CallerARN = "arn:aws:iam::210987654321:role/MatildaPrepOperator"
	client.organization.ManagementAccountID = "210987654321"

	result := runSetup(t, client, options)

	if result.Code != "aws_plan_stale" {
		t.Fatalf("Code = %q, want aws_plan_stale", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when caller account changed after approval")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestSetupBindingRefIsOpaqueAndSensitiveToSetupFacts(t *testing.T) {
	client := baselineSetupClient()
	plan, err := NewRunner(RunnerConfig{Client: client}).buildPlan(context.Background(), client, createCUR2Options("default", "us-west-2"))
	if err != nil {
		t.Fatalf("buildPlan returned error: %v", err)
	}

	base := setupBindingRef(plan)
	if !isSetupBindingRef(base) {
		t.Fatalf("setupBindingRef = %q, want setup_ plus account-id-safe letters", base)
	}
	for _, forbidden := range []string{
		client.identity.AccountID,
		client.identity.CallerARN,
		plan.Facts.BucketName,
		plan.Facts.ExportName,
		plan.Facts.Prefix,
	} {
		if strings.Contains(base, forbidden) {
			t.Fatalf("setupBindingRef leaked forbidden value %q in %q", forbidden, base)
		}
	}

	changedAccount := plan
	changedAccount.Identity.AccountID = "210987654321"
	assertDifferentSetupBindingRef(t, base, changedAccount, "account ID")

	changedPartition := plan
	changedPartition.Identity.Partition = "aws-us-gov"
	assertDifferentSetupBindingRef(t, base, changedPartition, "partition")

	changedRegion := plan
	changedRegion.Region = "us-east-2"
	assertDifferentSetupBindingRef(t, base, changedRegion, "S3 region")

	changedFacts := plan
	changedFacts.Facts = generatedNameCandidates(plan.Identity, plan.Region)[1]
	assertDifferentSetupBindingRef(t, base, changedFacts, "generated setup facts")

	changedQuery := plan
	changedQuery.QueryStatement = strings.Replace(plan.QueryStatement, "identity_time_interval", "identity_time_interval AS identity_time_interval", 1)
	assertDifferentSetupBindingRef(t, base, changedQuery, "query statement")

	managedExport := plan
	export := exportFromRequest(buildCreateExportRequest(plan), plannedExportARN(plan))
	managedExport.ManagedExport = &export
	assertDifferentSetupBindingRef(t, base, managedExport, "managed export ARN")
}

func TestLetterEncodeHashHonorsOddSafeLength(t *testing.T) {
	encoded := letterEncodeHash([]byte{0x1f}, 1)

	if encoded != "b" {
		t.Fatalf("letterEncodeHash odd length = %q, want first high-nibble letter", encoded)
	}
}

func TestExistingBucketCandidatesFiltersSortsAndBindsSafeRefs(t *testing.T) {
	identity := identityContext{AccountID: "123456789012", Partition: "aws"}

	candidates := existingBucketCandidates(identity, "us-west-2", []BucketSummary{
		{Name: " zeta-cur2 ", Region: "us-west-2 "},
		{Name: "east-only", Region: "us-east-1"},
		{Name: " ", Region: "us-west-2"},
		{Name: "alpha-cur2", Region: "us-west-2"},
	})

	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want two matching-region buckets", candidates)
	}
	if candidates[0].BucketName != "alpha-cur2" || candidates[1].BucketName != "zeta-cur2" {
		t.Fatalf("candidate order = %#v, want trimmed bucket names sorted by name", []string{candidates[0].BucketName, candidates[1].BucketName})
	}
	for index, candidate := range candidates {
		if candidate.BucketOwner != identity.AccountID {
			t.Fatalf("candidate %d bucket owner = %q, want caller account", index, candidate.BucketOwner)
		}
		if candidate.DestinationMode != workflow.AWSCUR2DestinationExistingSameAccount {
			t.Fatalf("candidate %d destination mode = %q, want existing_same_account", index, candidate.DestinationMode)
		}
		if !strings.HasPrefix(candidate.BucketRef, "s3b-") || strings.Contains(candidate.BucketRef, candidate.BucketName) || strings.Contains(candidate.BucketRef, identity.AccountID) {
			t.Fatalf("candidate %d bucket ref = %q, want opaque safe ref", index, candidate.BucketRef)
		}
		if !strings.HasPrefix(candidate.ExportName, "matilda-cur2-ra-billing-") ||
			strings.Contains(candidate.ExportName, candidate.BucketName) ||
			strings.Contains(candidate.ExportName, identity.AccountID) {
			t.Fatalf("candidate %d export name = %q, want generated safe export name", index, candidate.ExportName)
		}
		if candidate.Prefix != matildaBillingPrefix {
			t.Fatalf("candidate %d prefix = %q, want Matilda billing prefix", index, candidate.Prefix)
		}
	}
	if candidates[0].CandidateIndex != "existing_00" || candidates[1].CandidateIndex != "existing_01" {
		t.Fatalf("candidate indexes = %#v, want stable existing indexes", []string{candidates[0].CandidateIndex, candidates[1].CandidateIndex})
	}
}

func TestExistingBucketCandidatesReturnsNilWhenSafeRefsCannotBeUnique(t *testing.T) {
	identity := identityContext{AccountID: "123456789012", Partition: "aws"}

	candidates := existingBucketCandidates(identity, "us-west-2", []BucketSummary{
		{Name: "duplicate-cur2", Region: "us-west-2"},
		{Name: "duplicate-cur2", Region: "us-west-2"},
	})

	if candidates != nil {
		t.Fatalf("candidates = %#v, want nil when safe refs cannot distinguish duplicate buckets", candidates)
	}
}

func TestSafeBucketDisplayLabelMasksUnsafeValues(t *testing.T) {
	bucketRef := "s3b-abcdefghijklmnop"
	tests := []struct {
		name       string
		bucketName string
		want       string
	}{
		{name: "empty", bucketName: " ", want: "bucket-" + bucketRef},
		{name: "too long", bucketName: strings.Repeat("a", 64), want: "bucket-" + bucketRef},
		{name: "forbidden token", bucketName: "customer-plain-token", want: "bucket-" + bucketRef},
		{name: "safe", bucketName: "matilda-existing-cur2", want: "matilda-existing-cur2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeBucketDisplayLabel(tt.bucketName, bucketRef); got != tt.want {
				t.Fatalf("safeBucketDisplayLabel(%q) = %q, want %q", tt.bucketName, got, tt.want)
			}
		})
	}
}

func TestRunnerReportsSafeExportRefFromReturnedExportARN(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	client.createdExportIdentifierSuffix = "aws-generated-id-1234"

	result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

	if result.Code != "aws_cur2_create_export_created" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_created", result.Code)
	}
	if len(client.createdExports) != 1 {
		t.Fatalf("created exports = %#v, want one", client.createdExports)
	}
	wantRef := cur2preflight.SafeCUR2ExportRef(client.createdExports[0].ExportARN)
	if got := checkEvidenceValue(result, "selected_export_ref"); got != wantRef {
		t.Fatalf("selected_export_ref = %q, want safe ref from returned ARN %q", got, wantRef)
	}
	if got := checkEvidenceValue(result, "selected_export_ref"); got == safePlannedExportRefFromFacts(t, client, preview.Plan) {
		t.Fatalf("selected_export_ref = %q, still using predicted planned ref", got)
	}
	if result.Plan == nil {
		t.Fatal("result Plan is nil")
	}
	if result.Plan.PlanID != preview.Plan.PlanID {
		t.Fatalf("PlanID changed after returned export ref was recorded: %q != %q", result.Plan.PlanID, preview.Plan.PlanID)
	}
}

func TestRunnerBlocksWhenPostCreateBucketAccessIsNotVerified(t *testing.T) {
	tests := []struct {
		name          string
		postCreate    cur2preflight.BucketAccess
		wantCode      string
		createFailure error
	}{
		{
			name:       "normal create then inaccessible",
			postCreate: cur2preflight.BucketAccess{Accessible: false, StatusCode: 404, Region: "us-west-2"},
			wantCode:   "aws_s3_bucket_validation_failed",
		},
		{
			name:       "normal create then wrong region",
			postCreate: cur2preflight.BucketAccess{Accessible: true, StatusCode: 200, Region: "eu-west-1"},
			wantCode:   "aws_s3_bucket_validation_failed",
		},
		{
			name:       "normal create then missing region proof",
			postCreate: cur2preflight.BucketAccess{Accessible: true, StatusCode: 200},
			wantCode:   "aws_s3_bucket_validation_failed",
		},
		{
			name:          "already owned race then inaccessible",
			postCreate:    cur2preflight.BucketAccess{Accessible: false, StatusCode: 404, Region: "us-west-2"},
			wantCode:      "aws_s3_bucket_validation_failed",
			createFailure: NewProviderError("aws_s3_bucket_already_owned_by_caller", "bucket appeared between plan and apply"),
		},
		{
			name:          "already owned race then missing region proof",
			postCreate:    cur2preflight.BucketAccess{Accessible: true, StatusCode: 200},
			wantCode:      "aws_s3_bucket_validation_failed",
			createFailure: NewProviderError("aws_s3_bucket_already_owned_by_caller", "bucket appeared between plan and apply"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			client.headBucketErrs = []error{
				NewProviderError("aws_s3_bucket_not_found", "bucket is available during preview"),
				NewProviderError("aws_s3_bucket_not_found", "bucket is available during apply"),
				nil,
			}
			client.headBucketAccesses = []cur2preflight.BucketAccess{
				{},
				{},
				tt.postCreate,
			}
			preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
			client.createBucketErr = tt.createFailure

			result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

			if result.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", result.Code, tt.wantCode)
			}
			if !result.Mutated && tt.createFailure == nil {
				t.Fatal("Mutated = false, want true after create bucket completed before validation failed")
			}
			if client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
				t.Fatalf("policy/export calls = %d/%d, want none after failed post-create bucket validation", client.putBucketPolicyCalls(), client.createExportCalls())
			}
			if len(client.getPolicyRequests) != 0 {
				t.Fatalf("GetBucketPolicy calls = %d, want none after failed post-create bucket validation", len(client.getPolicyRequests))
			}
		})
	}
}

func TestRunnerRejectsMissingOrExtraPlanStepApprovalBeforeMutation(t *testing.T) {
	tests := []struct {
		name     string
		approver func(*workflow.ExecutionPlan) workflow.ExecutionOptions
	}{
		{
			name: "missing step approval",
			approver: func(plan *workflow.ExecutionPlan) workflow.ExecutionOptions {
				options := approvedCreateCUR2Options("default", "us-west-2", plan)
				options.Approvals = options.Approvals[:len(options.Approvals)-1]
				return options
			},
		},
		{
			name: "extra step approval",
			approver: func(plan *workflow.ExecutionPlan) workflow.ExecutionOptions {
				options := approvedCreateCUR2Options("default", "us-west-2", plan)
				options.Approvals = append(options.Approvals, workflow.ExecutionApproval{
					OperationID: workflow.AWSCUR2CreateBucketOperationID,
					PlanID:      plan.PlanID,
					Confirmed:   true,
				})
				return options
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))

			result := runSetup(t, client, tt.approver(preview.Plan))

			if result.Code != "aws_plan_approval_mismatch" {
				t.Fatalf("Code = %q, want aws_plan_approval_mismatch", result.Code)
			}
			if result.Mutated {
				t.Fatal("Mutated = true, want false")
			}
			if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
				t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
			}
		})
	}
}

func TestRunnerRejectsUnconfirmedPlanStepApprovalBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]workflow.ExecutionApproval)
	}{
		{
			name: "unconfirmed approvals",
			mutate: func(approvals []workflow.ExecutionApproval) {
				for index := range approvals {
					approvals[index].Confirmed = false
				}
			},
		},
		{
			name: "unexpected approval intent",
			mutate: func(approvals []workflow.ExecutionApproval) {
				for index := range approvals {
					approvals[index].Intent = workflow.ApprovalIntentRequestBackfillSupportCase
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
			options := approvedCreateCUR2Options("default", "us-west-2", preview.Plan)
			tt.mutate(options.Approvals)
			runner := NewRunner(RunnerConfig{
				Client: client,
				Now:    time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
			})

			result := runner.Run(context.Background(), awsBillingApplyPrereqsRequest(), options)

			if result.Code != "aws_plan_approval_mismatch" {
				t.Fatalf("Code = %q, want aws_plan_approval_mismatch", result.Code)
			}
			if result.Mutated {
				t.Fatal("Mutated = true, want false")
			}
			if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
				t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
			}
		})
	}
}

func TestRunnerRejectsStalePlanBeforeMutation(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	options := approvedCreateCUR2Options("default", "us-east-1", preview.Plan)

	result := runSetup(t, client, options)

	if result.Code != "aws_plan_stale" {
		t.Fatalf("Code = %q, want aws_plan_stale", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerMergesBucketPolicyWithoutDroppingUnrelatedStatements(t *testing.T) {
	client := baselineSetupClient()
	client.bucketExists = true
	client.bucketPolicy = `{"Version":"2012-10-17","Statement":[{"Sid":"KeepExisting","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::999999999999:root"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::example/*"}]}`
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

	if result.Code != "aws_cur2_create_export_created" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_created", result.Code)
	}
	if client.createBucketCalls() != 0 {
		t.Fatalf("CreateBucket calls = %d, want 0 for reusable generated bucket", client.createBucketCalls())
	}
	if client.putBucketPolicyCalls() != 1 {
		t.Fatalf("PutBucketPolicy calls = %d, want 1", client.putBucketPolicyCalls())
	}
	policy := client.putPolicyRequests[0].Policy
	if !strings.Contains(policy, `"Sid":"KeepExisting"`) {
		t.Fatalf("merged policy dropped existing statement: %s", policy)
	}
	if !strings.Contains(policy, `"Sid":"MatildaCloudPrepDataExportsDelivery"`) {
		t.Fatalf("merged policy missing Matilda Data Exports statement: %s", policy)
	}
}

func TestRunnerBlocksUnsafeExistingBucketPolicyBeforeMutation(t *testing.T) {
	client := baselineSetupClient()
	client.bucketExists = true
	client.bucketPolicy = `{not-json`

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_s3_bucket_policy_unmergeable" {
		t.Fatalf("Code = %q, want aws_s3_bucket_policy_unmergeable", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerReusesExistingMatildaManagedExport(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	facts := setupFactsFromPlan(t, client, preview.Plan)
	configureReusableManagedExport(t, client, facts)
	client.exports[0].ExportARN = "arn:aws:bcm-data-exports:us-east-1:123456789012:export/" + facts.ExportName + "-aws-generated-id-1234"

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_create_export_reused" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_reused", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false when reusing managed export")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
	if len(client.headBucketRequests) != 1 {
		t.Fatalf("HeadBucket calls = %d, want 1 to prove managed export bucket", len(client.headBucketRequests))
	}
	if len(client.getPolicyRequests) != 1 {
		t.Fatalf("GetBucketPolicy calls = %d, want 1 to prove managed export policy", len(client.getPolicyRequests))
	}
	wantRef := cur2preflight.SafeCUR2ExportRef(client.exports[0].ExportARN)
	if got := checkEvidenceValue(result, "selected_export_ref"); got != wantRef {
		t.Fatalf("selected_export_ref = %q, want safe ref from discovered export ARN %q", got, wantRef)
	}
	if got := checkEvidenceValue(result, "selected_export_ref"); got == safePlannedExportRefFromFacts(t, client, preview.Plan) {
		t.Fatalf("selected_export_ref = %q, still using predicted planned ref", got)
	}
}

func TestRunnerRejectsStaleCreateApprovalsWhenCurrentPlanReusesExport(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	options := approvedCreateCUR2Options("default", "us-west-2", preview.Plan)
	facts := setupFactsFromPlan(t, client, preview.Plan)
	configureReusableManagedExport(t, client, facts)
	client.exports[0].ExportARN = "arn:aws:bcm-data-exports:us-east-1:123456789012:export/" + facts.ExportName + "-aws-generated-id-1234"

	result := runSetup(t, client, options)

	if result.Code != "aws_plan_stale" {
		t.Fatalf("Code = %q, want aws_plan_stale", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false for stale approval/no-op mismatch")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerRejectsManagedExportWithMismatchedARNBeforeReuse(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	facts := setupFactsFromPlan(t, client, preview.Plan)
	configureReusableManagedExport(t, client, facts)
	client.exports[0].ExportARN = "arn:aws:bcm-data-exports:us-east-1:999999999999:export/" + facts.ExportName

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_managed_export_validation_failed" {
		t.Fatalf("Code = %q, want aws_cur2_managed_export_validation_failed", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if len(client.headBucketRequests) != 0 || len(client.getPolicyRequests) != 0 {
		t.Fatalf("bucket checks = head %d policy %d, want none before managed export ARN proof", len(client.headBucketRequests), len(client.getPolicyRequests))
	}
}

func TestRunnerBlocksManagedExportReuseWithoutBucketProof(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*fakeSetupClient)
		wantCode   string
		wantPolicy bool
	}{
		{
			name: "inaccessible bucket",
			configure: func(client *fakeSetupClient) {
				client.bucketExists = false
			},
			wantCode: "aws_s3_bucket_not_found",
		},
		{
			name: "missing region proof",
			configure: func(client *fakeSetupClient) {
				client.headBucketAccess = cur2preflight.BucketAccess{Accessible: true, StatusCode: 200}
			},
			wantCode: "aws_s3_bucket_validation_failed",
		},
		{
			name: "wrong region",
			configure: func(client *fakeSetupClient) {
				client.headBucketAccess = cur2preflight.BucketAccess{Accessible: true, StatusCode: 200, Region: "eu-west-1"}
			},
			wantCode: "aws_s3_bucket_validation_failed",
		},
		{
			name: "owner mismatch",
			configure: func(client *fakeSetupClient) {
				client.headBucketErr = NewProviderError("aws_s3_bucket_owner_mismatch", "expected owner mismatch")
			},
			wantCode: "aws_s3_bucket_owner_mismatch",
		},
		{
			name: "unreadable policy",
			configure: func(client *fakeSetupClient) {
				client.getBucketPolicyErr = NewProviderError("aws_s3_bucket_policy_inaccessible", "policy denied")
			},
			wantCode:   "aws_s3_bucket_policy_inaccessible",
			wantPolicy: true,
		},
		{
			name: "unmergeable policy",
			configure: func(client *fakeSetupClient) {
				client.bucketPolicy = "{not-json"
			},
			wantCode:   "aws_s3_bucket_policy_unmergeable",
			wantPolicy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
			facts := setupFactsFromPlan(t, client, preview.Plan)
			configureReusableManagedExport(t, client, facts)
			tt.configure(client)

			result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

			if result.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", result.Code, tt.wantCode)
			}
			if result.Mutated {
				t.Fatal("Mutated = true, want false")
			}
			if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
				t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
			}
			if !tt.wantPolicy && len(client.getPolicyRequests) != 0 {
				t.Fatalf("GetBucketPolicy calls = %d, want 0 before bucket proof succeeds", len(client.getPolicyRequests))
			}
		})
	}
}

func TestRunnerPlansManagedExportPolicyRepairWithoutCreatingDuplicateExport(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	facts := setupFactsFromPlan(t, client, preview.Plan)
	configureReusableManagedExport(t, client, facts)
	client.bucketPolicy = "{}"

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_create_export_approval_required" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_approval_required", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false before approval")
	}
	if result.Plan == nil {
		t.Fatal("Plan is nil")
	}
	if got := mutatingStepIDs(result.Plan.Steps); !equalStrings(got, []string{workflow.AWSCUR2MergeBucketPolicyOperationID}) {
		t.Fatalf("mutating step IDs = %#v, want bucket policy repair only", got)
	}
	if got := checkEvidenceValue(result, "selected_export_ref"); got != cur2preflight.SafeCUR2ExportRef(client.exports[0].ExportARN) {
		t.Fatalf("selected_export_ref = %q, want existing export safe ref", got)
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none before approval", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerRepairsManagedExportPolicyAfterPlanBoundApproval(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	facts := setupFactsFromPlan(t, client, preview.Plan)
	configureReusableManagedExport(t, client, facts)
	client.bucketPolicy = "{}"
	repairPreview := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", repairPreview.Plan))

	if result.Code != "aws_cur2_create_export_repaired" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_repaired", result.Code)
	}
	if !result.Mutated {
		t.Fatal("Mutated = false, want true after approved policy repair")
	}
	if client.createBucketCalls() != 0 {
		t.Fatalf("CreateBucket calls = %d, want 0 for managed export repair", client.createBucketCalls())
	}
	if client.putBucketPolicyCalls() != 1 {
		t.Fatalf("PutBucketPolicy calls = %d, want 1", client.putBucketPolicyCalls())
	}
	if client.createExportCalls() != 0 {
		t.Fatalf("CreateExport calls = %d, want 0 for managed export repair", client.createExportCalls())
	}
	if got := checkEvidenceValue(result, "selected_export_ref"); got != cur2preflight.SafeCUR2ExportRef(client.exports[0].ExportARN) {
		t.Fatalf("selected_export_ref = %q, want existing export safe ref", got)
	}
}

func TestValidateManagedExportRejectsMissingExportARNBeforeRead(t *testing.T) {
	client, runner, plan := managedExportValidationPlan(t)
	plan.ManagedExport.ExportARN = ""

	err := runner.validateManagedExport(context.Background(), client, plan)

	assertProviderErrorCode(t, err, "aws_cur2_managed_export_repair_validation_failed")
	if len(client.getExportRequests) != 0 {
		t.Fatalf("GetExport calls = %d, want 0 when managed export ARN is missing", len(client.getExportRequests))
	}
}

func TestValidateManagedExportPropagatesExportReadFailure(t *testing.T) {
	client, runner, plan := managedExportValidationPlan(t)
	client.getExportErr = NewProviderError("aws_cur2_export_invalid_shape", "managed export could not be read")

	err := runner.validateManagedExport(context.Background(), client, plan)

	assertProviderErrorCode(t, err, "aws_cur2_export_invalid_shape")
	if len(client.getExportRequests) == 0 || client.getExportRequests[len(client.getExportRequests)-1] != plan.ManagedExport.ExportARN {
		t.Fatalf("GetExport requests = %#v, want managed export ARN", client.getExportRequests)
	}
}

func TestValidateManagedExportRejectsExportSettingsDrift(t *testing.T) {
	client, runner, plan := managedExportValidationPlan(t)
	client.exports[0].Destination.Output.Format = "PARQUET"

	err := runner.validateManagedExport(context.Background(), client, plan)

	assertProviderErrorCode(t, err, "aws_cur2_managed_export_repair_validation_failed")
	if len(client.headBucketRequests) != 0 {
		t.Fatalf("HeadBucket calls = %d, want 0 before export settings validate", len(client.headBucketRequests))
	}
}

func TestValidateManagedExportRejectsPostRepairBucketAccessFailure(t *testing.T) {
	client, runner, plan := managedExportValidationPlan(t)
	client.headBucketErr = NewProviderError("aws_s3_bucket_owner_mismatch", "bucket owner mismatch")

	err := runner.validateManagedExport(context.Background(), client, plan)

	assertProviderErrorCode(t, err, "aws_s3_bucket_owner_mismatch")
	if len(client.getPolicyRequests) != 0 {
		t.Fatalf("GetBucketPolicy calls = %d, want 0 before bucket proof succeeds", len(client.getPolicyRequests))
	}
}

func TestValidateManagedExportRejectsPostRepairBucketAccessDrift(t *testing.T) {
	client, runner, plan := managedExportValidationPlan(t)
	client.headBucketAccess = cur2preflight.BucketAccess{Accessible: true, StatusCode: 200, Region: "eu-west-1"}

	err := runner.validateManagedExport(context.Background(), client, plan)

	assertProviderErrorCode(t, err, "aws_s3_bucket_validation_failed")
	if len(client.getPolicyRequests) != 0 {
		t.Fatalf("GetBucketPolicy calls = %d, want 0 before bucket region proof succeeds", len(client.getPolicyRequests))
	}
}

func TestValidateManagedExportRejectsPostRepairPolicyFailure(t *testing.T) {
	client, runner, plan := managedExportValidationPlan(t)
	client.bucketPolicy = "{not-json"

	err := runner.validateManagedExport(context.Background(), client, plan)

	assertProviderErrorCode(t, err, "aws_s3_bucket_policy_unmergeable")
	if len(client.getPolicyRequests) == 0 {
		t.Fatal("GetBucketPolicy was not called")
	}
}

func TestValidateManagedExportRejectsMissingPolicyStatementAfterRepair(t *testing.T) {
	client, runner, plan := managedExportValidationPlan(t)
	client.bucketPolicy = "{}"

	err := runner.validateManagedExport(context.Background(), client, plan)

	assertProviderErrorCode(t, err, "aws_s3_delivery_policy_validation_failed")
	if len(client.getPolicyRequests) == 0 {
		t.Fatal("GetBucketPolicy was not called")
	}
}

func TestValidateManagedExportAcceptsReadyManagedExport(t *testing.T) {
	client, runner, plan := managedExportValidationPlan(t)

	err := runner.validateManagedExport(context.Background(), client, plan)

	if err != nil {
		t.Fatalf("validateManagedExport returned error: %v", err)
	}
	if len(client.getExportRequests) == 0 || client.getExportRequests[len(client.getExportRequests)-1] != plan.ManagedExport.ExportARN {
		t.Fatalf("GetExport requests = %#v, want managed export ARN %q", client.getExportRequests, plan.ManagedExport.ExportARN)
	}
	if len(client.headBucketRequests) == 0 {
		t.Fatal("HeadBucket was not called")
	}
	if len(client.getPolicyRequests) == 0 {
		t.Fatal("GetBucketPolicy was not called")
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none for validation",
			client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerBlocksWhenCUR2QuotaIsFull(t *testing.T) {
	client := baselineSetupClient()
	for i := 0; i < 5; i++ {
		client.exports = append(client.exports, cur2preflight.Export{
			Name:           "customer-cur2-export",
			ExportARN:      "arn:aws:bcm-data-exports:us-east-1:123456789012:export/customer-" + string(rune('a'+i)),
			QueryStatement: "SELECT line_item_usage_amount FROM COST_AND_USAGE_REPORT",
		})
	}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_export_quota_full" {
		t.Fatalf("Code = %q, want aws_cur2_export_quota_full", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if result.Plan == nil {
		t.Fatal("Plan is nil")
	}
	if len(result.Plan.Steps) != 1 {
		t.Fatalf("Steps length = %d, want one blocked step", len(result.Plan.Steps))
	}
	step := result.Plan.Steps[0]
	if step.Intent != workflow.PlanStepBlocked || step.RequiresApproval {
		t.Fatalf("quota step = %#v, want blocked non-approval step", step)
	}
	for _, mutatingID := range []string{
		workflow.AWSCUR2CreateBucketOperationID,
		workflow.AWSCUR2MergeBucketPolicyOperationID,
		workflow.AWSCUR2CreateExportOperationID,
	} {
		if step.ID == mutatingID {
			t.Fatalf("quota step used mutating step ID %q", mutatingID)
		}
	}
	if result.Plan.Approval.Required || !result.Plan.Approval.Blocked {
		t.Fatalf("approval summary = %#v, want blocked without required mutation approval", result.Plan.Approval)
	}
	if len(client.headBucketRequests) != 0 || len(client.getPolicyRequests) != 0 ||
		client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("provider calls after quota = head %d policy %d bucket %d put %d export %d, want no S3 or mutation calls",
			len(client.headBucketRequests), len(client.getPolicyRequests), client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerBlocksAmbiguousGeneratedBucketHeadBucketStatusBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		access cur2preflight.BucketAccess
	}{
		{name: "bad request", access: cur2preflight.BucketAccess{Accessible: false, StatusCode: 400}},
		{name: "forbidden", access: cur2preflight.BucketAccess{Accessible: false, StatusCode: 403}},
		{name: "not found without provider proof", access: cur2preflight.BucketAccess{Accessible: false, StatusCode: 404}},
		{name: "no status", access: cur2preflight.BucketAccess{Accessible: false, Region: "us-west-2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			client.headBucketAccess = tt.access

			result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

			if result.Code != "aws_s3_bucket_inaccessible" {
				t.Fatalf("Code = %q, want aws_s3_bucket_inaccessible", result.Code)
			}
			if len(client.headBucketRequests) != 1 {
				t.Fatalf("HeadBucket calls = %d, want fail-closed after first ambiguous response", len(client.headBucketRequests))
			}
			if result.Plan == nil || len(result.Plan.Steps) != 1 {
				t.Fatalf("Plan = %#v, want one blocked guidance step", result.Plan)
			}
			step := result.Plan.Steps[0]
			for _, want := range []string{
				"generated same-account S3 bucket candidate",
				"Matilda Cloud Prep can show an approval-required plan",
				"Do not manually create or select arbitrary buckets",
			} {
				if !strings.Contains(step.CurrentState+step.TargetState+step.Validation, want) {
					t.Fatalf("blocked step = %#v, want guidance containing %q", step, want)
				}
			}
			if result.Mutated || client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
				t.Fatalf("mutation state = result %t bucket %d policy %d export %d, want no mutation",
					result.Mutated, client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
			}
		})
	}
}

func TestRunnerBlocksGeneratedBucketProviderHeadBucketErrorBeforeCandidateExhaustion(t *testing.T) {
	client := baselineSetupClient()
	client.headBucketErr = NewProviderError("aws_s3_bucket_throttled", "head bucket throttled")

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_s3_bucket_throttled" {
		t.Fatalf("Code = %q, want aws_s3_bucket_throttled", result.Code)
	}
	if len(client.headBucketRequests) != 1 {
		t.Fatalf("HeadBucket calls = %d, want fail-closed after provider error", len(client.headBucketRequests))
	}
	if result.Mutated || client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation state = result %t bucket %d policy %d export %d, want no mutation",
			result.Mutated, client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerExhaustsGeneratedBucketCandidatesOnOwnerMismatchBeforeMutation(t *testing.T) {
	client := baselineSetupClient()
	client.bucketExists = true
	client.headBucketErr = NewProviderError("aws_s3_bucket_owner_mismatch", "expected owner mismatch")

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_bucket_name_candidates_exhausted" {
		t.Fatalf("Code = %q, want aws_cur2_bucket_name_candidates_exhausted", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if len(client.headBucketRequests) != maxBucketNameCandidates {
		t.Fatalf("HeadBucket calls = %d, want %d", len(client.headBucketRequests), maxBucketNameCandidates)
	}
	if len(client.getPolicyRequests) != 0 || client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("calls after owner mismatch = getPolicy %d bucket %d putPolicy %d export %d, want none", len(client.getPolicyRequests), client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerUsesClientFactoryAndConfiguredRegionFallback(t *testing.T) {
	client := baselineSetupClient()
	client.config.Region = "us-west-1"
	var factoryOptions workflow.ExecutionOptions
	runner := NewRunner(RunnerConfig{
		ClientFactory: func(options workflow.ExecutionOptions) Client {
			factoryOptions = options
			return client
		},
	})
	options := createCUR2Options("default", "")
	report := runner.Run(context.Background(), awsBillingApplyPrereqsRequest(), options)

	if report.Code != "aws_cur2_create_export_approval_required" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_approval_required", report.Code)
	}
	if factoryOptions.AWSBillingOperation != workflow.AWSBillingOperationCreateCUR2Export {
		t.Fatalf("factory operation = %q, want create_cur2_export", factoryOptions.AWSBillingOperation)
	}
	if len(client.headBucketRequests) == 0 || client.headBucketRequests[0].Region != "us-west-1" {
		t.Fatalf("head bucket requests = %#v, want configured region fallback", client.headBucketRequests)
	}
}

func TestRunnerBlocksWithoutSetupClient(t *testing.T) {
	runner := NewRunner(RunnerConfig{})

	result := runner.Run(context.Background(), awsBillingApplyPrereqsRequest(), createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_provider_capability_blocked" {
		t.Fatalf("Code = %q, want aws_provider_capability_blocked", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if result.PlanInput == nil {
		t.Fatal("PlanInput is nil")
	}
}

func TestRunnerBlocksTypedNilSetupClientFromFactory(t *testing.T) {
	runner := NewRunner(RunnerConfig{
		ClientFactory: func(workflow.ExecutionOptions) Client {
			var client *fakeSetupClient
			return client
		},
	})

	result := runner.Run(context.Background(), awsBillingApplyPrereqsRequest(), createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_provider_capability_blocked" {
		t.Fatalf("Code = %q, want aws_provider_capability_blocked", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
}

func TestRunnerReportsAWSBillingCoverageClassifications(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*fakeSetupClient)
		wantStatus workflow.CoverageStatus
	}{
		{
			name: "member account is account-only",
			configure: func(client *fakeSetupClient) {
				client.organization.ManagementAccountID = "999999999999"
			},
			wantStatus: workflow.CoverageAccountOnly,
		},
		{
			name: "not in organization is single-account",
			configure: func(client *fakeSetupClient) {
				client.describeOrganizationErr = NewProviderError("aws_organizations_not_in_use", "not in organization")
			},
			wantStatus: workflow.CoverageSingleAccount,
		},
		{
			name: "access denied is unverified",
			configure: func(client *fakeSetupClient) {
				client.describeOrganizationErr = NewProviderError("aws_organizations_access_denied", "denied")
			},
			wantStatus: workflow.CoverageUnverified,
		},
		{
			name: "incomplete organization is unverified",
			configure: func(client *fakeSetupClient) {
				client.organization = Organization{Available: true}
			},
			wantStatus: workflow.CoverageUnverified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			tt.configure(client)

			result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

			if result.Plan == nil {
				t.Fatal("Plan is nil")
			}
			if result.Plan.CoverageRecommendation.CoverageStatus != tt.wantStatus {
				t.Fatalf("CoverageStatus = %q, want %q", result.Plan.CoverageRecommendation.CoverageStatus, tt.wantStatus)
			}
		})
	}
}

func TestRunnerBlocksWhenDataExportsPaginationDoesNotConverge(t *testing.T) {
	client := baselineSetupClient()
	client.listExportsNextToken = "same-token"

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_data_exports_pagination_unbounded" {
		t.Fatalf("Code = %q, want aws_data_exports_pagination_unbounded", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
}

func TestListFullExportsFailsClosedForSummariesWithoutARN(t *testing.T) {
	client := baselineSetupClient()
	client.exports = []cur2preflight.Export{
		{Name: "missing-arn"},
		{
			Name:           "valid",
			ExportARN:      "arn:aws:bcm-data-exports:us-east-1:123456789012:export/valid",
			QueryStatement: "SELECT line_item_usage_amount FROM COST_AND_USAGE_REPORT",
		},
	}

	exports, err := listFullExports(context.Background(), client)

	if err == nil {
		t.Fatalf("listFullExports returned exports %#v, want error for incomplete summary", exports)
	}
	assertProviderErrorCode(t, err, "aws_data_exports_incomplete_export_summary")
}

func TestRunnerBlocksWhenBucketNameCandidatesAreExhausted(t *testing.T) {
	client := baselineSetupClient()
	client.headBucketAccess = cur2preflight.BucketAccess{Accessible: false, StatusCode: 409}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_bucket_name_candidates_exhausted" {
		t.Fatalf("Code = %q, want aws_cur2_bucket_name_candidates_exhausted", result.Code)
	}
	if len(client.headBucketRequests) != maxBucketNameCandidates {
		t.Fatalf("HeadBucket calls = %d, want %d", len(client.headBucketRequests), maxBucketNameCandidates)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
}

func TestRunnerSkipsGeneratedBucketCandidatesInWrongRegion(t *testing.T) {
	client := baselineSetupClient()
	client.headBucketAccess = cur2preflight.BucketAccess{Accessible: true, StatusCode: 200, Region: "eu-west-1"}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_bucket_name_candidates_exhausted" {
		t.Fatalf("Code = %q, want aws_cur2_bucket_name_candidates_exhausted", result.Code)
	}
	if len(client.headBucketRequests) != maxBucketNameCandidates {
		t.Fatalf("HeadBucket calls = %d, want %d", len(client.headBucketRequests), maxBucketNameCandidates)
	}
	if len(client.getPolicyRequests) != 0 {
		t.Fatalf("GetBucketPolicy calls = %d, want 0 for wrong-region buckets", len(client.getPolicyRequests))
	}
}

func TestRunnerSkipsGeneratedBucketCandidatesWithoutRegionProof(t *testing.T) {
	client := baselineSetupClient()
	client.headBucketAccess = cur2preflight.BucketAccess{Accessible: true, StatusCode: 200}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_bucket_name_candidates_exhausted" {
		t.Fatalf("Code = %q, want aws_cur2_bucket_name_candidates_exhausted", result.Code)
	}
	if len(client.headBucketRequests) != maxBucketNameCandidates {
		t.Fatalf("HeadBucket calls = %d, want %d", len(client.headBucketRequests), maxBucketNameCandidates)
	}
	if len(client.getPolicyRequests) != 0 {
		t.Fatalf("GetBucketPolicy calls = %d, want 0 when bucket region is unproved", len(client.getPolicyRequests))
	}
	if client.createBucketCalls() != 0 || client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("mutation calls = bucket %d policy %d export %d, want none", client.createBucketCalls(), client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerSkipsOtherAccountGeneratedBucketCandidate(t *testing.T) {
	client := baselineSetupClient()
	client.headBucketErrs = []error{
		NewProviderError("aws_s3_bucket_owner_mismatch", "first candidate belongs to another account"),
	}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_create_export_approval_required" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_approval_required", result.Code)
	}
	facts := setupFactsFromPlan(t, client, result.Plan)
	if facts.CandidateIndex != "01" {
		t.Fatalf("CandidateIndex = %q, want 01 after first candidate owner mismatch", facts.CandidateIndex)
	}
	if len(client.headBucketRequests) < 2 {
		t.Fatalf("HeadBucket calls = %d, want at least 2", len(client.headBucketRequests))
	}
}

func TestRunnerHandlesCreateBucketAlreadyOwnedRaceAfterApproval(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	client.createBucketErr = NewProviderError("aws_s3_bucket_already_owned_by_caller", "bucket appeared between plan and apply")

	result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

	if result.Code != "aws_cur2_create_export_created" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_created", result.Code)
	}
	if !result.Mutated {
		t.Fatal("Mutated = false, want true because policy/export setup completed")
	}
	if client.createBucketCalls() != 1 {
		t.Fatalf("CreateBucket calls = %d, want 1", client.createBucketCalls())
	}
	if len(client.headBucketRequests) < 2 {
		t.Fatalf("HeadBucket calls = %d, want post-create owner verification", len(client.headBucketRequests))
	}
	if client.putBucketPolicyCalls() != 1 || client.createExportCalls() != 1 {
		t.Fatalf("policy/export calls = %d/%d, want 1/1 after safe already-owned reuse", client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerReturnsStalePlanWhenApprovedBucketCandidateBecomesUnavailable(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	client.createBucketErr = NewProviderError("aws_s3_bucket_already_exists", "bucket name unavailable")

	result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

	if result.Code != "aws_plan_stale" {
		t.Fatalf("Code = %q, want aws_plan_stale", result.Code)
	}
	if result.Mutated {
		t.Fatal("Mutated = true, want false")
	}
	if client.putBucketPolicyCalls() != 0 || client.createExportCalls() != 0 {
		t.Fatalf("policy/export calls = %d/%d, want none after stale bucket candidate", client.putBucketPolicyCalls(), client.createExportCalls())
	}
}

func TestRunnerAdvancesUnavailableBucketCandidateDuringPlanning(t *testing.T) {
	client := baselineSetupClient()
	client.headBucketAccesses = []cur2preflight.BucketAccess{{Accessible: false, StatusCode: 409, Region: "us-west-2"}}
	client.headBucketErrs = []error{nil, NewProviderError("aws_s3_bucket_not_found", "second candidate is available")}

	result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	if result.Code != "aws_cur2_create_export_approval_required" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_approval_required", result.Code)
	}
	facts := setupFactsFromPlan(t, client, result.Plan)
	if facts.CandidateIndex != "01" {
		t.Fatalf("CandidateIndex = %q, want 01 after first candidate is unavailable", facts.CandidateIndex)
	}
}

func TestRunnerSkipsPolicyWriteWhenGeneratedBucketPolicyAlreadyAllowsDelivery(t *testing.T) {
	client := baselineSetupClient()
	client.bucketExists = true
	candidates := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")
	policy, _, err := mergeDataExportsPolicy("", setupPlan{
		Facts:    candidates[0],
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	})
	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	client.bucketPolicy = policy
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))

	result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

	if result.Code != "aws_cur2_create_export_created" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_created", result.Code)
	}
	if client.putBucketPolicyCalls() != 0 {
		t.Fatalf("PutBucketPolicy calls = %d, want 0", client.putBucketPolicyCalls())
	}
	if client.createExportCalls() != 1 {
		t.Fatalf("CreateExport calls = %d, want 1", client.createExportCalls())
	}
}

func TestPolicyMergeUsesAWSDocumentedDataExportsStatementShape(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:          facts,
		Identity:       identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:         "us-west-2",
		QueryStatement: completeCUR2QueryStatementForTest(),
	}

	merged, changed, err := mergeDataExportsPolicy("", plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true for empty policy")
	}
	assertDataExportsPolicyShape(t, merged)
	again, changed, err := mergeDataExportsPolicy(merged, plan)
	if err != nil {
		t.Fatalf("mergeDataExportsPolicy(existing matching) returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false for matching policy: %s", again)
	}
}

func TestPolicyMergeTreatsEquivalentDifferentSidStatementAsReady(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:          facts,
		Identity:       identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:         "us-west-2",
		QueryStatement: completeCUR2QueryStatementForTest(),
	}
	generated, _, err := mergeDataExportsPolicy("", plan)
	if err != nil {
		t.Fatalf("mergeDataExportsPolicy(empty) returned error: %v", err)
	}
	existing := strings.Replace(generated, dataExportsDeliveryStatementSid, "ExistingEquivalentDataExportsDelivery", 1)

	merged, changed, err := mergeDataExportsPolicy(existing, plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy(equivalent) returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false for equivalent statement under different Sid: %s", merged)
	}
	if strings.Contains(merged, dataExportsDeliveryStatementSid) {
		t.Fatalf("merge added duplicate generated Sid despite equivalent statement: %s", merged)
	}
	if got := strings.Count(merged, "bcm-data-exports.amazonaws.com"); got != 1 {
		t.Fatalf("Data Exports principal count = %d, want 1 in %s", got, merged)
	}
}

func TestPolicyMergeTreatsEquivalentSingleValueStatementAsReady(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:          facts,
		Identity:       identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:         "us-west-2",
		QueryStatement: completeCUR2QueryStatementForTest(),
	}
	existingDocument := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{map[string]any{
			"Sid":       "ExistingEquivalentSingleValueDataExportsDelivery",
			"Effect":    "Allow",
			"Principal": map[string]any{"Service": "bcm-data-exports.amazonaws.com"},
			"Action":    "s3:PutObject",
			"Resource":  fmt.Sprintf("arn:aws:s3:::%s/*", facts.BucketName),
			"Condition": map[string]any{
				"ArnLike":      map[string]any{"aws:SourceArn": fmt.Sprintf("arn:aws:bcm-data-exports:%s:%s:export/*", dataExportsRegion, client.identity.AccountID)},
				"StringEquals": map[string]any{"aws:SourceAccount": client.identity.AccountID},
			},
		}},
	}
	existingBytes, err := json.Marshal(existingDocument)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	merged, changed, err := mergeDataExportsPolicy(string(existingBytes), plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false for equivalent single-value statement: %s", merged)
	}
	if strings.Contains(merged, dataExportsDeliveryStatementSid) {
		t.Fatalf("merge added duplicate generated Sid despite equivalent single-value statement: %s", merged)
	}
	if got := strings.Count(merged, "bcm-data-exports.amazonaws.com"); got != 1 {
		t.Fatalf("Data Exports principal count = %d, want 1 in %s", got, merged)
	}
}

func TestPolicyMergeTreatsEquivalentSameSidSingleValueStatementAsReady(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:    facts,
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}
	existingDocument := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{map[string]any{
			"Sid":       dataExportsDeliveryStatementSid,
			"Effect":    "Allow",
			"Principal": map[string]any{"Service": "bcm-data-exports.amazonaws.com"},
			"Action":    "s3:PutObject",
			"Resource":  fmt.Sprintf("arn:aws:s3:::%s/*", facts.BucketName),
			"Condition": map[string]any{
				"ArnLike":      map[string]any{"aws:SourceArn": fmt.Sprintf("arn:aws:bcm-data-exports:%s:%s:export/*", dataExportsRegion, client.identity.AccountID)},
				"StringEquals": map[string]any{"aws:SourceAccount": client.identity.AccountID},
			},
		}},
	}
	existingBytes, err := json.Marshal(existingDocument)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	merged, changed, err := mergeDataExportsPolicy(string(existingBytes), plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false for same-Sid equivalent single-value statement: %s", merged)
	}
	if got := strings.Count(merged, dataExportsDeliveryStatementSid); got != 1 {
		t.Fatalf("generated Sid count = %d, want 1 in %s", got, merged)
	}
	if got := strings.Count(merged, "bcm-data-exports.amazonaws.com"); got != 1 {
		t.Fatalf("Data Exports principal count = %d, want 1 in %s", got, merged)
	}
}

func TestPolicyMergeAcceptsSingleStatementDocument(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:    facts,
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}
	existingDocument := map[string]any{
		"Version": "2012-10-17",
		"Statement": map[string]any{
			"Sid":       "ExistingEquivalentSingleStatementDataExportsDelivery",
			"Effect":    "Allow",
			"Principal": map[string]any{"Service": "bcm-data-exports.amazonaws.com"},
			"Action":    "s3:PutObject",
			"Resource":  fmt.Sprintf("arn:aws:s3:::%s/*", facts.BucketName),
			"Condition": map[string]any{
				"ArnLike":      map[string]any{"aws:SourceArn": fmt.Sprintf("arn:aws:bcm-data-exports:%s:%s:export/*", dataExportsRegion, client.identity.AccountID)},
				"StringEquals": map[string]any{"aws:SourceAccount": client.identity.AccountID},
			},
		},
	}
	existingBytes, err := json.Marshal(existingDocument)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	merged, changed, err := mergeDataExportsPolicy(string(existingBytes), plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false for equivalent single-statement policy document: %s", merged)
	}
	if got := strings.Count(merged, "bcm-data-exports.amazonaws.com"); got != 1 {
		t.Fatalf("Data Exports principal count = %d, want 1 in %s", got, merged)
	}
}

func TestPolicyStatementNormalizationAcceptsAWSStringAndArrayForms(t *testing.T) {
	normalized, ok := normalizePolicyStatement([]byte(`{
		"Sid":"Normalize",
		"Effect":"Allow",
		"Principal":{"Service":"bcm-data-exports.amazonaws.com"},
		"Action":["s3:PutObject","s3:AbortMultipartUpload"],
		"Resource":"arn:aws:s3:::example/*",
		"Condition":{"StringEquals":{"aws:SourceAccount":"123456789012"}}
	}`))

	if !ok {
		t.Fatal("normalizePolicyStatement returned ok=false for valid AWS string/array policy forms")
	}
	if normalized.Principal["Service"][0] != "bcm-data-exports.amazonaws.com" {
		t.Fatalf("Principal.Service = %#v, want service principal", normalized.Principal["Service"])
	}
	if !equalStrings(normalized.Action, []string{"s3:AbortMultipartUpload", "s3:PutObject"}) {
		t.Fatalf("Action = %#v, want sorted action list", normalized.Action)
	}
	if !equalStrings(normalized.Resource, []string{"arn:aws:s3:::example/*"}) {
		t.Fatalf("Resource = %#v, want one resource", normalized.Resource)
	}

	if _, ok := normalizeStringList(json.RawMessage(`123`)); ok {
		t.Fatal("normalizeStringList accepted non-string policy value")
	}
	if _, ok := normalizePolicyStatement([]byte(`{"Effect":"Allow","Principal":{"Service":123},"Action":"s3:PutObject","Resource":"arn:aws:s3:::example/*"}`)); ok {
		t.Fatal("normalizePolicyStatement accepted non-string principal value")
	}
}

func TestPolicyMergePreservesUnrelatedListFormStatements(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:    facts,
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}
	existing := `{"Version":"2012-10-17","Statement":[{"Sid":"KeepListForm","Effect":"Allow","Principal":{"AWS":["arn:aws:iam::999999999999:root"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::example/*"]}]}`

	merged, changed, err := mergeDataExportsPolicy(existing, plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true when adding Data Exports statement")
	}
	if !strings.Contains(merged, `"Sid":"KeepListForm"`) {
		t.Fatalf("merged policy dropped unrelated list-form statement: %s", merged)
	}
	assertDataExportsPolicyShape(t, merged)
}

func TestPolicyMergeRepairsPreviousMatildaPrefixScopedDeliveryStatement(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:    facts,
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}
	existingDocument := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{map[string]any{
			"Sid":       dataExportsDeliveryStatementSid,
			"Effect":    "Allow",
			"Principal": map[string]any{"Service": "bcm-data-exports.amazonaws.com"},
			"Action":    "s3:PutObject",
			"Resource":  fmt.Sprintf("arn:aws:s3:::%s/%s/*", facts.BucketName, facts.Prefix),
			"Condition": map[string]any{
				"ArnLike":      map[string]any{"aws:SourceArn": fmt.Sprintf("arn:aws:bcm-data-exports:%s:%s:export/*", dataExportsRegion, client.identity.AccountID)},
				"StringEquals": map[string]any{"aws:SourceAccount": client.identity.AccountID},
			},
		}, map[string]any{
			"Sid":       "Keep",
			"Effect":    "Allow",
			"Principal": map[string]any{"AWS": "arn:aws:iam::999999999999:root"},
			"Action":    "s3:GetObject",
			"Resource":  "arn:aws:s3:::example/*",
		}},
	}
	existingBytes, err := json.Marshal(existingDocument)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	merged, changed, err := mergeDataExportsPolicy(string(existingBytes), plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want repair of previous prefix-scoped Matilda delivery statement")
	}
	if strings.Contains(merged, fmt.Sprintf("arn:aws:s3:::%s/%s/*", facts.BucketName, facts.Prefix)) {
		t.Fatalf("merged policy retained prefix-scoped Matilda delivery resource: %s", merged)
	}
	if got := strings.Count(merged, dataExportsDeliveryStatementSid); got != 1 {
		t.Fatalf("Matilda delivery statement count = %d, want 1 in %s", got, merged)
	}
	if !strings.Contains(merged, `"Sid":"Keep"`) {
		t.Fatalf("merged policy dropped unrelated statement: %s", merged)
	}
	assertDataExportsPolicyShape(t, merged)
}

func TestRunnerBlocksProviderFailuresDuringApply(t *testing.T) {
	tests := []struct {
		name        string
		configure   func(*fakeSetupClient)
		wantCode    string
		wantMutated bool
	}{
		{
			name: "create bucket failure",
			configure: func(client *fakeSetupClient) {
				client.createBucketErr = NewProviderError("aws_s3_create_bucket_failed", "create failed")
			},
			wantCode: "aws_s3_create_bucket_failed",
		},
		{
			name: "get policy failure",
			configure: func(client *fakeSetupClient) {
				client.getBucketPolicyErr = NewProviderError("aws_s3_bucket_policy_inaccessible", "policy denied")
			},
			wantCode:    "aws_s3_bucket_policy_inaccessible",
			wantMutated: true,
		},
		{
			name: "put policy failure",
			configure: func(client *fakeSetupClient) {
				client.putBucketPolicyErr = NewProviderError("aws_s3_put_bucket_policy_failed", "put failed")
			},
			wantCode:    "aws_s3_put_bucket_policy_failed",
			wantMutated: true,
		},
		{
			name: "create export failure",
			configure: func(client *fakeSetupClient) {
				client.createExportErr = NewProviderError("aws_cur2_create_export_failed", "create export failed")
			},
			wantCode:    "aws_cur2_create_export_failed",
			wantMutated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
			tt.configure(client)

			result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

			if result.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", result.Code, tt.wantCode)
			}
			if result.Mutated != tt.wantMutated {
				t.Fatalf("Mutated = %v, want %v", result.Mutated, tt.wantMutated)
			}
		})
	}
}

func TestRunnerReportsNoMutationWhenExistingBucketUpdateFailsBeforeChange(t *testing.T) {
	t.Run("policy write fails before any successful mutation", func(t *testing.T) {
		client := baselineSetupClient()
		client.bucketExists = true
		client.bucketPolicy = "{}"
		preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
		client.putBucketPolicyErr = NewProviderError("aws_s3_put_bucket_policy_failed", "put failed")

		result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

		if result.Code != "aws_s3_put_bucket_policy_failed" {
			t.Fatalf("Code = %q, want aws_s3_put_bucket_policy_failed", result.Code)
		}
		if result.Mutated {
			t.Fatal("Mutated = true, want false when the existing bucket was not changed")
		}
	})
	t.Run("export creation fails after existing bucket and policy are already ready", func(t *testing.T) {
		client := baselineSetupClient()
		client.bucketExists = true
		candidates := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")
		policy, _, err := mergeDataExportsPolicy("", setupPlan{
			Facts:    candidates[0],
			Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
			Region:   "us-west-2",
		})
		if err != nil {
			t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
		}
		client.bucketPolicy = policy
		preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
		client.createExportErr = NewProviderError("aws_cur2_create_export_failed", "create export failed")

		result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

		if result.Code != "aws_cur2_create_export_failed" {
			t.Fatalf("Code = %q, want aws_cur2_create_export_failed", result.Code)
		}
		if result.Mutated {
			t.Fatal("Mutated = true, want false when no cloud mutation completed")
		}
	})
}

func TestRunnerBlocksWhenCreatedExportCannotBeValidated(t *testing.T) {
	client := baselineSetupClient()
	preview := runSetup(t, client, createCUR2Options("default", "us-west-2"))
	client.postCreateExportTamper = func(export cur2preflight.Export) cur2preflight.Export {
		export.Destination.Output.Overwrite = "OVERWRITE_REPORT"
		return export
	}

	result := runSetup(t, client, approvedCreateCUR2Options("default", "us-west-2", preview.Plan))

	if result.Code != "aws_cur2_create_export_validation_failed" {
		t.Fatalf("Code = %q, want aws_cur2_create_export_validation_failed", result.Code)
	}
	if !result.Mutated {
		t.Fatal("Mutated = false, want true because export creation completed before validation failed")
	}
}

func TestValidateCreatedExportRejectsMissingCreatedExportARN(t *testing.T) {
	client := baselineSetupClient()
	runner := NewRunner(RunnerConfig{Client: client})
	plan, err := runner.buildPlan(context.Background(), client, createCUR2Options("default", "us-west-2"))
	if err != nil {
		t.Fatalf("buildPlan returned error: %v", err)
	}

	err = runner.validateCreatedExport(context.Background(), client, plan, CreateExportResult{})

	assertProviderErrorCode(t, err, "aws_cur2_create_export_validation_failed")
	if len(client.getExportRequests) != 0 {
		t.Fatalf("GetExport calls = %d, want 0 when created export ARN is missing", len(client.getExportRequests))
	}
}

func TestValidateCreatedExportPropagatesCreatedExportReadFailure(t *testing.T) {
	client := baselineSetupClient()
	runner := NewRunner(RunnerConfig{Client: client})
	plan, err := runner.buildPlan(context.Background(), client, createCUR2Options("default", "us-west-2"))
	if err != nil {
		t.Fatalf("buildPlan returned error: %v", err)
	}
	client.getExportErr = NewProviderError("aws_cur2_export_invalid_shape", "created export could not be read")

	err = runner.validateCreatedExport(context.Background(), client, plan, CreateExportResult{ExportARN: validReturnedExportARN(plan, "aws-generated-id")})

	assertProviderErrorCode(t, err, "aws_cur2_export_invalid_shape")
	if len(client.getExportRequests) == 0 || client.getExportRequests[len(client.getExportRequests)-1] != validReturnedExportARN(plan, "aws-generated-id") {
		t.Fatalf("GetExport requests = %#v, want returned export ARN", client.getExportRequests)
	}
}

func TestValidateCreatedExportRejectsInvalidReturnedExportARNsBeforeRead(t *testing.T) {
	tests := []struct {
		name string
		arn  string
	}{
		{name: "malformed", arn: "not-an-arn"},
		{name: "wrong service", arn: "arn:aws:cur:us-east-1:123456789012:definition/example"},
		{name: "wrong partition", arn: "arn:aws-us-gov:bcm-data-exports:us-east-1:123456789012:export/example"},
		{name: "wrong region", arn: "arn:aws:bcm-data-exports:us-west-2:123456789012:export/example"},
		{name: "wrong account", arn: "arn:aws:bcm-data-exports:us-east-1:999999999999:export/example"},
		{name: "non export resource", arn: "arn:aws:bcm-data-exports:us-east-1:123456789012:table/example"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			runner := NewRunner(RunnerConfig{Client: client})
			plan, err := runner.buildPlan(context.Background(), client, createCUR2Options("default", "us-west-2"))
			if err != nil {
				t.Fatalf("buildPlan returned error: %v", err)
			}

			err = runner.validateCreatedExport(context.Background(), client, plan, CreateExportResult{ExportARN: tt.arn})

			assertProviderErrorCode(t, err, "aws_cur2_create_export_validation_failed")
			if len(client.getExportRequests) != 0 {
				t.Fatalf("GetExport calls = %d, want 0 for invalid returned ARN", len(client.getExportRequests))
			}
		})
	}
}

func TestValidateReturnedExportARNRejectsPaddedARN(t *testing.T) {
	client := baselineSetupClient()
	runner := NewRunner(RunnerConfig{Client: client})
	plan, err := runner.buildPlan(context.Background(), client, createCUR2Options("default", "us-west-2"))
	if err != nil {
		t.Fatalf("buildPlan returned error: %v", err)
	}

	err = validateReturnedExportARN(validReturnedExportARN(plan, "aws-generated-id")+" ", plan)

	assertProviderErrorCode(t, err, "aws_cur2_create_export_validation_failed")
}

func TestValidateCreatedExportAcceptsReadyCreatedExport(t *testing.T) {
	client, runner, plan, createResult := createdExportValidationPlan(t)

	err := runner.validateCreatedExport(context.Background(), client, plan, createResult)

	if err != nil {
		t.Fatalf("validateCreatedExport returned error: %v", err)
	}
	if len(client.getExportRequests) == 0 || client.getExportRequests[len(client.getExportRequests)-1] != createResult.ExportARN {
		t.Fatalf("GetExport requests = %#v, want created export ARN %q", client.getExportRequests, createResult.ExportARN)
	}
	if len(client.headBucketRequests) == 0 {
		t.Fatal("HeadBucket was not called")
	}
	if len(client.getPolicyRequests) == 0 {
		t.Fatal("GetBucketPolicy was not called")
	}
}

func TestValidateCreatedExportRejectsPostCreateBucketAccessFailure(t *testing.T) {
	client, runner, plan, createResult := createdExportValidationPlan(t)
	client.headBucketErr = NewProviderError("aws_s3_bucket_owner_mismatch", "bucket owner mismatch")

	err := runner.validateCreatedExport(context.Background(), client, plan, createResult)

	assertProviderErrorCode(t, err, "aws_s3_bucket_owner_mismatch")
	if len(client.getPolicyRequests) != 0 {
		t.Fatalf("GetBucketPolicy calls = %d, want 0 before bucket proof succeeds", len(client.getPolicyRequests))
	}
}

func TestValidateCreatedExportRejectsPostCreateBucketAccessDrift(t *testing.T) {
	client, runner, plan, createResult := createdExportValidationPlan(t)
	client.headBucketAccess = cur2preflight.BucketAccess{Accessible: true, StatusCode: 200, Region: "eu-west-1"}

	err := runner.validateCreatedExport(context.Background(), client, plan, createResult)

	assertProviderErrorCode(t, err, "aws_s3_bucket_validation_failed")
	if len(client.getPolicyRequests) != 0 {
		t.Fatalf("GetBucketPolicy calls = %d, want 0 before bucket region proof succeeds", len(client.getPolicyRequests))
	}
}

func TestValidateCreatedExportRejectsPostCreatePolicyFailure(t *testing.T) {
	client, runner, plan, createResult := createdExportValidationPlan(t)
	client.getBucketPolicyErr = NewProviderError("aws_s3_bucket_policy_inaccessible", "policy denied")

	err := runner.validateCreatedExport(context.Background(), client, plan, createResult)

	assertProviderErrorCode(t, err, "aws_s3_bucket_policy_inaccessible")
}

func TestValidateCreatedExportRejectsMissingPolicyStatementAfterCreate(t *testing.T) {
	client, runner, plan, createResult := createdExportValidationPlan(t)
	client.bucketPolicy = "{}"

	err := runner.validateCreatedExport(context.Background(), client, plan, createResult)

	assertProviderErrorCode(t, err, "aws_s3_delivery_policy_validation_failed")
	if len(client.getPolicyRequests) == 0 {
		t.Fatal("GetBucketPolicy was not called")
	}
}

func TestRunnerBlocksProviderDiscoveryFailures(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeSetupClient)
		wantCode  string
	}{
		{
			name: "configuration failure",
			configure: func(client *fakeSetupClient) {
				client.configErr = NewProviderError("aws_config_missing_region", "region missing")
			},
			wantCode: "aws_config_missing_region",
		},
		{
			name: "identity failure",
			configure: func(client *fakeSetupClient) {
				client.identityErr = NewProviderError("aws_auth_failed", "auth failed")
			},
			wantCode: "aws_auth_failed",
		},
		{
			name: "list exports failure",
			configure: func(client *fakeSetupClient) {
				client.listExportsErr = NewProviderError("aws_data_exports_access_denied", "denied")
			},
			wantCode: "aws_data_exports_access_denied",
		},
		{
			name: "get export failure",
			configure: func(client *fakeSetupClient) {
				client.exports = []cur2preflight.Export{{ExportARN: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/customer"}}
				client.getExportErr = NewProviderError("aws_cur2_export_invalid_shape", "invalid export")
			},
			wantCode: "aws_cur2_export_invalid_shape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineSetupClient()
			tt.configure(client)

			result := runSetup(t, client, createCUR2Options("default", "us-west-2"))

			if result.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", result.Code, tt.wantCode)
			}
			if result.Mutated {
				t.Fatal("Mutated = true, want false")
			}
		})
	}
}

func TestVerifyManagedExportReuseRejectsMissingManagedExportBeforeProviderCalls(t *testing.T) {
	client := baselineSetupClient()
	plan := setupPlan{
		Facts: generatedNameCandidates(identityContext{
			AccountID: client.identity.AccountID,
			Partition: partitionFromARN(client.identity.CallerARN),
		}, "us-west-2")[0],
		Identity: identityContext{
			AccountID: client.identity.AccountID,
			Partition: partitionFromARN(client.identity.CallerARN),
		},
		Region: "us-west-2",
	}

	_, err := verifyManagedExportReuse(context.Background(), client, plan)

	assertProviderErrorCode(t, err, "aws_cur2_managed_export_validation_failed")
	if len(client.headBucketRequests) != 0 || len(client.getPolicyRequests) != 0 {
		t.Fatalf("provider calls = head %d policy %d, want none before managed export exists", len(client.headBucketRequests), len(client.getPolicyRequests))
	}
}

func TestSetupUtilityFallbacksStaySafe(t *testing.T) {
	if code := providerErrorCode(errors.New("plain failure"), "fallback_code"); code != "fallback_code" {
		t.Fatalf("providerErrorCode = %q, want fallback_code", code)
	}
	if code := providerErrorCode(cur2preflight.NewProviderError("aws_preflight_failed", "preflight failed"), "fallback_code"); code != "aws_preflight_failed" {
		t.Fatalf("providerErrorCode = %q, want aws_preflight_failed", code)
	}
	if text := NewProviderError("aws_test", "message").Error(); !strings.Contains(text, "aws_test") || !strings.Contains(text, "message") {
		t.Fatalf("ProviderError.Error() = %q, want code and message", text)
	}
	if got := last4("123"); got != "unknown" {
		t.Fatalf("last4 short value = %q, want unknown", got)
	}
	if got := partitionFromARN("not-an-arn"); got != "" {
		t.Fatalf("partitionFromARN invalid = %q, want empty", got)
	}
	if got := sanitizedRegion(" US-WEST-2 "); got != "us-west-2" {
		t.Fatalf("sanitizedRegion = %q, want us-west-2", got)
	}
	if got := sanitizedRegion(" "); got != "us-east-1" {
		t.Fatalf("sanitizedRegion empty = %q, want us-east-1", got)
	}
	if got := namingHash(identityContext{AccountID: "", Partition: ""}, ""); got == "" {
		t.Fatal("namingHash returned empty value")
	} else if !isLetterEncodedHash(got, 12) || hasTwelveDigitRun(got) {
		t.Fatalf("namingHash = %q, want 12 account-ID-safe letters", got)
	}
}

func isLetterEncodedHash(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, r := range value {
		if r < 'a' || r > 'p' {
			return false
		}
	}
	return true
}

func hasTwelveDigitRun(value string) bool {
	run := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			run++
			if run >= 12 {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func TestPolicyMergeFailsClosedOnSameSidNonEquivalentStatement(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:    facts,
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}
	existing := `{"Version":"2012-10-17","Statement":[{"Sid":"MatildaCloudPrepDataExportsDelivery","Effect":"Allow","Principal":{"Service":"bcm-data-exports.amazonaws.com"},"Action":"s3:PutObject","Resource":"arn:aws:s3:::old-bucket/old-prefix/*","Condition":{"StringEquals":{"aws:SourceAccount":"123456789012","aws:SourceArn":"arn:aws:bcm-data-exports:us-east-1:123456789012:export/*"}}},{"Sid":"Keep","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::999999999999:root"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::example/*"}]}`

	merged, changed, err := mergeDataExportsPolicy(existing, plan)

	if err == nil {
		t.Fatalf("mergeDataExportsPolicy returned nil error for same-Sid non-equivalent statement; merged=%s changed=%t", merged, changed)
	}
	if changed {
		t.Fatal("changed = true, want false when same-Sid statement cannot be merged safely")
	}
	if merged != "" {
		t.Fatalf("merged policy = %q, want empty on fail-closed conflict", merged)
	}
}

func TestComparisonHelpersRejectInvalidOrMissingInputs(t *testing.T) {
	if jsonEqual([]byte("{"), []byte("{}")) {
		t.Fatal("jsonEqual returned true for invalid JSON")
	}
	if tableConfigurationsEqual(map[string]map[string]string{"COST_AND_USAGE_REPORT": {"TIME_GRANULARITY": "MONTHLY"}}, nil) {
		t.Fatal("tableConfigurationsEqual returned true for missing configuration")
	}
}

func TestPolicyMergeRejectsUnsafeExistingPolicyDocuments(t *testing.T) {
	client := baselineSetupClient()
	plan := setupPlan{
		Facts:    generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0],
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}

	if _, _, err := mergeDataExportsPolicy(`{"Version":"2012-10-17","Statement":`, plan); err == nil {
		t.Fatal("mergeDataExportsPolicy accepted malformed policy JSON")
	}
	if _, _, err := mergeDataExportsPolicy(`{"Version":"2012-10-17","Statement":["not-object"]}`, plan); err == nil {
		t.Fatal("mergeDataExportsPolicy accepted a non-object policy statement")
	}
}

func TestPolicyMergeDefaultsMissingVersionForValidDocument(t *testing.T) {
	client := baselineSetupClient()
	plan := setupPlan{
		Facts:    generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0],
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}

	merged, changed, err := mergeDataExportsPolicy(`{"Statement":[]}`, plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want data exports delivery statement added")
	}
	if !strings.Contains(merged, `"Version":"2012-10-17"`) {
		t.Fatalf("merged policy did not set default policy version: %s", merged)
	}
}

func TestPolicyMergePreservesNonEquivalentStatementAndAddsDelivery(t *testing.T) {
	client := baselineSetupClient()
	plan := setupPlan{
		Facts:    generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0],
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}
	existing := `{"Version":"2012-10-17","Statement":[{"Sid":"KeepUnsupportedShape","Effect":"Allow","Principal":"not-a-principal-map","Action":"s3:GetObject","Resource":"arn:aws:s3:::example/*"}]}`

	merged, changed, err := mergeDataExportsPolicy(existing, plan)

	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want data exports delivery statement added")
	}
	if !strings.Contains(merged, `"Sid":"KeepUnsupportedShape"`) {
		t.Fatalf("merged policy dropped unrelated non-equivalent statement: %s", merged)
	}
	if got := strings.Count(merged, dataExportsDeliveryStatementSid); got != 1 {
		t.Fatalf("Matilda delivery statement count = %d, want 1 in %s", got, merged)
	}
}

func TestReportWithPlanAddsDefaultSourceHandlesToIdentitySummary(t *testing.T) {
	report := reportWithPlan(awsBillingApplyPrereqsRequest(), workflow.RunStatusManualSteps, workflow.SupportGuided, "aws_test", "message", false, workflow.ExecutionPlanInput{
		OperatorIdentitySummary: workflow.OperatorIdentitySummary{
			IdentityStatus: "verified",
			Summary:        "AWS caller identity was verified for test setup.",
		},
		CoverageRecommendation: workflow.CoverageRecommendation{
			CoverageStatus: workflow.CoverageUnverified,
			Summary:        "AWS billing coverage was not verified in this test.",
		},
		PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
		Steps: []workflow.PlanStep{{
			Intent:                    workflow.PlanStepGuide,
			Title:                     "Test step",
			Description:               "Test step description.",
			Reason:                    "Test reason.",
			ApprovalKind:              "not_required",
			CurrentState:              "Test current state.",
			TargetState:               "Test target state.",
			RequiredPermission:        "No permission required.",
			CredentialMaterialTouched: false,
			Validation:                "Test validation.",
			Rollback:                  "No rollback required.",
		}},
	})

	if report.PlanInput == nil || len(report.PlanInput.OperatorIdentitySummary.SourceHandles) == 0 {
		t.Fatalf("identity source handles missing: %#v", report.PlanInput)
	}
	if len(report.PlanInput.Steps[0].SourceHandles) == 0 {
		t.Fatalf("step source handles missing: %#v", report.PlanInput.Steps[0])
	}
}

func TestExportAndPolicyPredicatesRejectMismatches(t *testing.T) {
	client := baselineSetupClient()
	facts := generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: "aws"}, "us-west-2")[0]
	plan := setupPlan{
		Facts:    facts,
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	}
	request := buildCreateExportRequest(plan)
	matching := exportFromRequest(request, "arn:aws:bcm-data-exports:us-east-1:123456789012:export/"+request.Name)

	mismatches := []cur2preflight.Export{
		func() cur2preflight.Export { export := cloneExport(matching); export.Name = "other"; return export }(),
		func() cur2preflight.Export {
			export := cloneExport(matching)
			export.QueryStatement = "SELECT line_item_usage_amount FROM COST_AND_USAGE_REPORT"
			return export
		}(),
		func() cur2preflight.Export {
			export := cloneExport(matching)
			export.RefreshCadence = "MANUAL"
			return export
		}(),
		func() cur2preflight.Export {
			export := cloneExport(matching)
			export.Destination.Bucket = "other"
			return export
		}(),
		func() cur2preflight.Export {
			export := cloneExport(matching)
			export.Destination.Output.Format = "PARQUET"
			return export
		}(),
		func() cur2preflight.Export {
			export := cloneExport(matching)
			export.TableConfigurations[cur2TableName]["TIME_GRANULARITY"] = "DAILY"
			return export
		}(),
		func() cur2preflight.Export {
			export := cloneExport(matching)
			delete(export.TableConfigurations[cur2TableName], "BILLING_VIEW_ARN")
			return export
		}(),
		func() cur2preflight.Export {
			export := cloneExport(matching)
			export.TableConfigurations[cur2TableName]["BILLING_VIEW_ARN"] = "arn:aws:billing::123456789012:billingview/other"
			return export
		}(),
		func() cur2preflight.Export {
			export := cloneExport(matching)
			export.TableConfigurations[cur2TableName]["INCLUDE_RESOURCES"] = "TRUE"
			return export
		}(),
	}
	for index, export := range mismatches {
		if isManagedExport(export, plan) {
			t.Fatalf("mismatch %d was treated as managed: %#v", index, export)
		}
	}
	if !isManagedExport(matching, plan) {
		t.Fatal("matching export was not treated as managed")
	}
	if _, _, err := mergeDataExportsPolicy(`{"Version":"2012-10-17","Statement":[{not-json}]}`, plan); err == nil {
		t.Fatal("mergeDataExportsPolicy accepted unparseable statement")
	}
	if _, changed, err := mergeDataExportsPolicy("", plan); err != nil || !changed {
		t.Fatalf("mergeDataExportsPolicy(empty) changed=%v err=%v, want changed", changed, err)
	}
}

func cloneExport(export cur2preflight.Export) cur2preflight.Export {
	cloned := export
	cloned.TableConfigurations = map[string]map[string]string{}
	for table, values := range export.TableConfigurations {
		cloned.TableConfigurations[table] = map[string]string{}
		for key, value := range values {
			cloned.TableConfigurations[table][key] = value
		}
	}
	return cloned
}

func copyTestStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func runSetup(t *testing.T, client *fakeSetupClient, options workflow.ExecutionOptions) workflow.Result {
	t.Helper()
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: awsBillingApplyPrereqsRequest(),
		Runner: NewRunner(RunnerConfig{
			Client: client,
			Now:    time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	return registry.ExecuteContext(context.Background(), awsBillingApplyPrereqsRequest(), options)
}

func currentSetupPlanForTest(t *testing.T, client *fakeSetupClient, options workflow.ExecutionOptions) (*workflow.ExecutionPlan, []workflow.PlanStep) {
	t.Helper()
	runner := NewRunner(RunnerConfig{Client: client})
	plan, err := runner.buildPlan(context.Background(), client, options)
	if err != nil {
		t.Fatalf("buildPlan returned error: %v", err)
	}
	steps := planSteps(plan)
	input := setupPlanInput(awsBillingApplyPrereqsRequest(), plan, steps, []workflow.PlanCheck{planFactsCheck(plan)})
	input.ExecutionOptions = options
	current, err := workflow.BuildExecutionPlan(input)
	if err != nil {
		t.Fatalf("BuildExecutionPlan returned error: %v", err)
	}
	return &current, current.Steps
}

func managedExportValidationPlan(t *testing.T) (*fakeSetupClient, Runner, setupPlan) {
	t.Helper()
	client := baselineSetupClient()
	runner := NewRunner(RunnerConfig{Client: client})
	options := createCUR2Options("default", "us-west-2")
	initialPlan, err := runner.buildPlan(context.Background(), client, options)
	if err != nil {
		t.Fatalf("initial buildPlan returned error: %v", err)
	}
	configureReusableManagedExport(t, client, initialPlan.Facts)
	policy, _, err := mergeDataExportsPolicy("", setupPlan{
		Facts:    initialPlan.Facts,
		Identity: identityContext{AccountID: client.identity.AccountID, Partition: "aws"},
		Region:   "us-west-2",
	})
	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	client.bucketPolicy = policy
	plan, err := runner.buildPlan(context.Background(), client, options)
	if err != nil {
		t.Fatalf("managed buildPlan returned error: %v", err)
	}
	if plan.ManagedExport == nil {
		t.Fatal("managed buildPlan did not select managed export")
	}
	client.getExportRequests = nil
	client.getPolicyRequests = nil
	client.headBucketRequests = nil
	return client, runner, plan
}

func createdExportValidationPlan(t *testing.T) (*fakeSetupClient, Runner, setupPlan, CreateExportResult) {
	t.Helper()
	client := baselineSetupClient()
	runner := NewRunner(RunnerConfig{Client: client})
	plan, err := runner.buildPlan(context.Background(), client, createCUR2Options("default", "us-west-2"))
	if err != nil {
		t.Fatalf("buildPlan returned error: %v", err)
	}
	policy, _, err := mergeDataExportsPolicy("", plan)
	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	createResult := CreateExportResult{ExportARN: validReturnedExportARN(plan, "aws-generated-id")}
	client.bucketExists = true
	client.bucketPolicy = policy
	client.exports = []cur2preflight.Export{exportFromRequest(buildCreateExportRequest(plan), createResult.ExportARN)}
	client.getExportRequests = nil
	client.getPolicyRequests = nil
	client.headBucketRequests = nil
	return client, runner, plan, createResult
}

func awsBillingApplyPrereqsRequest() workflow.Request {
	return workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}
}

func createCUR2Options(profile string, region string) workflow.ExecutionOptions {
	options, err := workflow.NormalizeExecutionOptionsForRequest(awsBillingApplyPrereqsRequest(), workflow.ExecutionOptions{
		InterfaceMode:       workflow.InterfaceModeDirect,
		AWSBillingOperation: workflow.AWSBillingOperationCreateCUR2Export,
		TimeoutSeconds:      workflow.DefaultExecutionTimeoutSeconds,
		Selectors:           &workflow.ExecutionSelectors{AWS: &workflow.AWSExecutionSelectors{Profile: profile, Region: region}},
	})
	if err != nil {
		panic(err)
	}
	return options
}

func createCUR2ExistingBucketSelectionOptions(profile string, region string, bucketRef string) workflow.ExecutionOptions {
	options := createCUR2Options(profile, region)
	options.Selectors.AWS.CUR2DestinationMode = workflow.AWSCUR2DestinationExistingSameAccount
	options.Selectors.AWS.CUR2S3BucketRef = bucketRef
	normalized, err := workflow.NormalizeExecutionOptionsForRequest(awsBillingApplyPrereqsRequest(), options)
	if err != nil {
		panic(err)
	}
	return normalized
}

func approvedCreateCUR2Options(profile string, region string, plan *workflow.ExecutionPlan) workflow.ExecutionOptions {
	options := createCUR2Options(profile, region)
	for _, step := range plan.Steps {
		if !step.RequiresApproval {
			continue
		}
		options.Approvals = append(options.Approvals, workflow.ExecutionApproval{
			OperationID: step.ID,
			PlanID:      plan.PlanID,
			Confirmed:   true,
		})
	}
	normalized, err := workflow.NormalizeExecutionOptionsForRequest(awsBillingApplyPrereqsRequest(), options)
	if err != nil {
		panic(err)
	}
	return normalized
}

func approvedExistingBucketCUR2Options(profile string, region string, bucketRef string, plan *workflow.ExecutionPlan) workflow.ExecutionOptions {
	options := approvedCreateCUR2Options(profile, region, plan)
	options.Selectors.AWS.CUR2DestinationMode = workflow.AWSCUR2DestinationExistingSameAccount
	options.Selectors.AWS.CUR2S3BucketRef = bucketRef
	normalized, err := workflow.NormalizeExecutionOptionsForRequest(awsBillingApplyPrereqsRequest(), options)
	if err != nil {
		panic(err)
	}
	return normalized
}

func mutatingStepIDs(steps []workflow.PlanStep) []string {
	ids := []string{}
	for _, step := range steps {
		if step.RequiresApproval {
			ids = append(ids, step.ID)
		}
	}
	return ids
}

func setupFactsFromPlan(t *testing.T, client *fakeSetupClient, plan *workflow.ExecutionPlan) setupFacts {
	t.Helper()
	if plan == nil {
		t.Fatal("plan is nil")
	}
	candidateIndex := ""
	region := ""
	for _, check := range plan.Checks {
		for _, evidence := range check.Evidence {
			switch evidence.Key {
			case "candidate_index":
				candidateIndex = evidence.Value
			case "s3_region":
				region = evidence.Value
			}
		}
	}
	if candidateIndex == "" || region == "" {
		t.Fatalf("plan facts incomplete: candidate_index=%q s3_region=%q", candidateIndex, region)
	}
	partition := partitionFromARN(client.identity.CallerARN)
	if partition == "" {
		t.Fatal("test client caller ARN does not contain a parseable partition")
	}
	for _, facts := range generatedNameCandidates(identityContext{AccountID: client.identity.AccountID, Partition: partition}, region) {
		if facts.CandidateIndex == candidateIndex {
			return facts
		}
	}
	t.Fatalf("candidate index %q not found for region %q", candidateIndex, region)
	return setupFacts{}
}

func checkEvidenceValue(result workflow.Result, key string) string {
	if result.Plan == nil {
		return ""
	}
	for _, check := range result.Plan.Checks {
		for _, evidence := range check.Evidence {
			if evidence.Key == key {
				return evidence.Value
			}
		}
	}
	return ""
}

func isSetupBindingRef(value string) bool {
	if !strings.HasPrefix(value, "setup_") || len(value) != len("setup_")+16 {
		return false
	}
	for _, char := range strings.TrimPrefix(value, "setup_") {
		if char < 'a' || char > 'p' {
			return false
		}
	}
	return true
}

func assertDifferentSetupBindingRef(t *testing.T, base string, plan setupPlan, changedField string) {
	t.Helper()
	next := setupBindingRef(plan)
	if !isSetupBindingRef(next) {
		t.Fatalf("setupBindingRef after %s change = %q, want safe binding ref", changedField, next)
	}
	if next == base {
		t.Fatalf("setupBindingRef did not change after %s changed: %q", changedField, next)
	}
}

func safePlannedExportRefFromFacts(t *testing.T, client *fakeSetupClient, plan *workflow.ExecutionPlan) string {
	t.Helper()
	facts := setupFactsFromPlan(t, client, plan)
	return cur2preflight.SafeCUR2ExportRef(fmt.Sprintf("arn:aws:bcm-data-exports:us-east-1:%s:export/%s", client.identity.AccountID, facts.ExportName))
}

func validReturnedExportARN(plan setupPlan, identifierSuffix string) string {
	return fmt.Sprintf("arn:%s:bcm-data-exports:%s:%s:export/%s-%s", plan.Identity.Partition, dataExportsRegion, plan.Identity.AccountID, plan.Facts.ExportName, identifierSuffix)
}

func assertDataExportsPolicyShape(t *testing.T, policy string) {
	t.Helper()
	var document struct {
		Statement []struct {
			Sid       string                    `json:"Sid"`
			Principal map[string]any            `json:"Principal"`
			Action    any                       `json:"Action"`
			Resource  any                       `json:"Resource"`
			Condition map[string]map[string]any `json:"Condition"`
		} `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(policy), &document); err != nil {
		t.Fatalf("policy did not parse as JSON: %v\n%s", err, policy)
	}
	for _, statement := range document.Statement {
		if statement.Sid != dataExportsDeliveryStatementSid {
			continue
		}
		service, ok := statement.Principal["Service"].([]any)
		if !ok || len(service) != 1 || service[0] != "bcm-data-exports.amazonaws.com" {
			t.Fatalf("Principal.Service = %#v, want single-item array", statement.Principal["Service"])
		}
		action, ok := statement.Action.([]any)
		if !ok || len(action) != 1 || action[0] != "s3:PutObject" {
			t.Fatalf("Action = %#v, want single-item array", statement.Action)
		}
		resource, ok := statement.Resource.(string)
		if !ok || !strings.HasPrefix(resource, "arn:aws:s3:::") || !strings.HasSuffix(resource, "/*") {
			t.Fatalf("Resource = %#v, want bucket object wildcard", statement.Resource)
		}
		if strings.Contains(resource, matildaBillingPrefix) {
			t.Fatalf("Resource = %q, want bucket object wildcard without Matilda prefix", resource)
		}
		if _, ok := statement.Condition["ArnLike"]["aws:SourceArn"]; !ok {
			t.Fatalf("missing ArnLike.aws:SourceArn in %#v", statement.Condition)
		}
		if _, ok := statement.Condition["StringEquals"]["aws:SourceAccount"]; !ok {
			t.Fatalf("missing StringEquals.aws:SourceAccount in %#v", statement.Condition)
		}
		if _, ok := statement.Condition["StringEquals"]["aws:SourceArn"]; ok {
			t.Fatalf("aws:SourceArn used StringEquals instead of ArnLike: %#v", statement.Condition)
		}
		return
	}
	t.Fatalf("missing %s statement in policy: %s", dataExportsDeliveryStatementSid, policy)
}

func assertProviderErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	var providerErr ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != want {
		t.Fatalf("error = %#v, want provider code %s", err, want)
	}
}

func assertResultDoesNotLeakAWSSecrets(t *testing.T, result workflow.Result) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	for _, forbidden := range []string{
		"123456789012",
		"arn:",
		"billingview/primary",
		"access_key",
		"secret_key",
		"session_token",
		"/Users/",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("result leaked forbidden value %q in %s", forbidden, string(encoded))
		}
	}
}

func equalStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

type fakeSetupClient struct {
	config       cur2preflight.Configuration
	configErr    error
	identity     cur2preflight.Identity
	identityErr  error
	organization Organization
	table        cur2preflight.Table
	exports      []cur2preflight.Export
	buckets      []BucketSummary

	bucketExists               bool
	bucketPolicy               string
	headBucketAccess           cur2preflight.BucketAccess
	headBucketAccesses         []cur2preflight.BucketAccess
	listExportsNextToken       string
	listBucketsNextToken       string
	listBucketsAlwaysNextToken bool
	headBucketErrs             []error

	describeOrganizationErr error
	getTableErr             error
	listExportsErr          error
	listBucketsErr          error
	getExportErr            error
	headBucketErr           error
	getBucketPolicyErr      error
	putBucketPolicyErr      error
	createBucketErr         error
	createExportErr         error

	headBucketRequests   []HeadBucketRequest
	getTableRequests     []getTableRequest
	listBucketsRequests  []ListBucketsRequest
	createBucketRequests []CreateBucketRequest
	getPolicyRequests    []BucketPolicyRequest
	putPolicyRequests    []PutBucketPolicyRequest
	getExportRequests    []string
	createExportRequests []CreateExportRequest
	createdExports       []CreateExportResult

	postCreateExportTamper        func(cur2preflight.Export) cur2preflight.Export
	createdExportIdentifierSuffix string
}

type getTableRequest struct {
	Name       string
	Properties map[string]string
}

func baselineSetupClient() *fakeSetupClient {
	return &fakeSetupClient{
		config: cur2preflight.Configuration{Region: "us-west-2"},
		identity: cur2preflight.Identity{
			AccountID: "123456789012",
			CallerARN: "arn:aws:iam::123456789012:role/MatildaPrepOperator",
		},
		organization: Organization{
			ManagementAccountID: "123456789012",
			Available:           true,
		},
		table: cur2preflight.Table{
			Name:    cur2TableName,
			Columns: completeCUR2TableColumnsForTest(),
		},
	}
}

func (client *fakeSetupClient) CheckConfiguration(context.Context) (cur2preflight.Configuration, error) {
	if client.configErr != nil {
		return cur2preflight.Configuration{}, client.configErr
	}
	return client.config, nil
}

func (client *fakeSetupClient) GetCallerIdentity(context.Context) (cur2preflight.Identity, error) {
	if client.identityErr != nil {
		return cur2preflight.Identity{}, client.identityErr
	}
	return client.identity, nil
}

func (client *fakeSetupClient) DescribeOrganization(context.Context) (Organization, error) {
	if client.describeOrganizationErr != nil {
		return Organization{}, client.describeOrganizationErr
	}
	return client.organization, nil
}

func (client *fakeSetupClient) GetTable(_ context.Context, name string, properties map[string]string) (cur2preflight.Table, error) {
	client.getTableRequests = append(client.getTableRequests, getTableRequest{
		Name:       name,
		Properties: copyTestStringMap(properties),
	})
	if client.getTableErr != nil {
		return cur2preflight.Table{}, client.getTableErr
	}
	return client.table, nil
}

func (client *fakeSetupClient) ListExports(context.Context, string) (cur2preflight.ExportPage, error) {
	if client.listExportsErr != nil {
		return cur2preflight.ExportPage{}, client.listExportsErr
	}
	summaries := make([]cur2preflight.ExportSummary, 0, len(client.exports))
	for _, export := range client.exports {
		summaries = append(summaries, cur2preflight.ExportSummary{
			Name:      export.Name,
			ExportARN: export.ExportARN,
		})
	}
	return cur2preflight.ExportPage{Exports: summaries, NextToken: client.listExportsNextToken}, nil
}

func (client *fakeSetupClient) ListBuckets(_ context.Context, request ListBucketsRequest) (BucketPage, error) {
	client.listBucketsRequests = append(client.listBucketsRequests, request)
	if client.listBucketsErr != nil {
		return BucketPage{}, client.listBucketsErr
	}
	buckets := []BucketSummary{}
	if request.Token == "" {
		for _, bucket := range client.buckets {
			if request.Region != "" && bucket.Region != request.Region {
				continue
			}
			buckets = append(buckets, bucket)
		}
	}
	nextToken := ""
	if request.Token == "" {
		nextToken = client.listBucketsNextToken
	}
	if client.listBucketsAlwaysNextToken {
		nextToken = client.listBucketsNextToken
	}
	return BucketPage{Buckets: buckets, NextToken: nextToken}, nil
}

func (client *fakeSetupClient) GetExport(_ context.Context, exportARN string) (cur2preflight.Export, error) {
	client.getExportRequests = append(client.getExportRequests, exportARN)
	if client.getExportErr != nil {
		return cur2preflight.Export{}, client.getExportErr
	}
	for _, export := range client.exports {
		if export.ExportARN == exportARN {
			return export, nil
		}
	}
	return cur2preflight.Export{}, NewProviderError("aws_cur2_export_invalid_shape", "export not found")
}

func (client *fakeSetupClient) HeadBucket(_ context.Context, request HeadBucketRequest) (cur2preflight.BucketAccess, error) {
	client.headBucketRequests = append(client.headBucketRequests, request)
	call := len(client.headBucketRequests) - 1
	if call < len(client.headBucketErrs) && client.headBucketErrs[call] != nil {
		return cur2preflight.BucketAccess{}, client.headBucketErrs[call]
	}
	if client.headBucketErr != nil {
		return cur2preflight.BucketAccess{}, client.headBucketErr
	}
	if call < len(client.headBucketAccesses) {
		return client.headBucketAccesses[call], nil
	}
	if client.headBucketAccess.StatusCode != 0 || client.headBucketAccess.Region != "" {
		return client.headBucketAccess, nil
	}
	if client.bucketExists {
		return cur2preflight.BucketAccess{Accessible: true, StatusCode: 200, Region: request.Region}, nil
	}
	return cur2preflight.BucketAccess{}, NewProviderError("aws_s3_bucket_not_found", "generated bucket name is available")
}

func (client *fakeSetupClient) CreateBucket(_ context.Context, request CreateBucketRequest) error {
	client.createBucketRequests = append(client.createBucketRequests, request)
	if client.createBucketErr != nil {
		var providerErr ProviderError
		if errors.As(client.createBucketErr, &providerErr) && providerErr.Code == "aws_s3_bucket_already_owned_by_caller" {
			client.bucketExists = true
		}
		return client.createBucketErr
	}
	client.bucketExists = true
	return nil
}

func (client *fakeSetupClient) GetBucketPolicy(_ context.Context, request BucketPolicyRequest) (string, error) {
	client.getPolicyRequests = append(client.getPolicyRequests, request)
	if client.getBucketPolicyErr != nil {
		return "", client.getBucketPolicyErr
	}
	return client.bucketPolicy, nil
}

func (client *fakeSetupClient) PutBucketPolicy(_ context.Context, request PutBucketPolicyRequest) error {
	client.putPolicyRequests = append(client.putPolicyRequests, request)
	if client.putBucketPolicyErr != nil {
		return client.putBucketPolicyErr
	}
	client.bucketPolicy = request.Policy
	return nil
}

func (client *fakeSetupClient) CreateExport(_ context.Context, request CreateExportRequest) (CreateExportResult, error) {
	client.createExportRequests = append(client.createExportRequests, request)
	if client.createExportErr != nil {
		return CreateExportResult{}, client.createExportErr
	}
	identifier := request.Name
	if client.createdExportIdentifierSuffix != "" {
		identifier = request.Name + "-" + client.createdExportIdentifierSuffix
	}
	result := CreateExportResult{ExportARN: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/" + identifier}
	client.createdExports = append(client.createdExports, result)
	export := exportFromRequest(request, result.ExportARN)
	if client.postCreateExportTamper != nil {
		export = client.postCreateExportTamper(export)
	}
	client.exports = append(client.exports, export)
	return result, nil
}

func (client *fakeSetupClient) createBucketCalls() int {
	return len(client.createBucketRequests)
}

func (client *fakeSetupClient) putBucketPolicyCalls() int {
	return len(client.putPolicyRequests)
}

func (client *fakeSetupClient) createExportCalls() int {
	return len(client.createExportRequests)
}

func managedExportFromFacts(client *fakeSetupClient, facts setupFacts) cur2preflight.Export {
	request := buildCreateExportRequest(setupPlan{
		Facts: facts,
		Identity: identityContext{
			AccountID: client.identity.AccountID,
			Partition: partitionFromARN(client.identity.CallerARN),
		},
		Region:         client.config.Region,
		QueryStatement: completeCUR2QueryStatementForTest(),
	})
	return exportFromRequest(request, "arn:aws:bcm-data-exports:us-east-1:123456789012:export/"+request.Name)
}

func configureReusableManagedExport(t *testing.T, client *fakeSetupClient, facts setupFacts) {
	t.Helper()
	plan := setupPlan{
		Facts: facts,
		Identity: identityContext{
			AccountID: client.identity.AccountID,
			Partition: partitionFromARN(client.identity.CallerARN),
		},
		Region: client.config.Region,
	}
	policy, changed, err := mergeDataExportsPolicy("", plan)
	if err != nil {
		t.Fatalf("mergeDataExportsPolicy returned error: %v", err)
	}
	if !changed {
		t.Fatal("mergeDataExportsPolicy changed = false, want generated policy")
	}
	client.exports = []cur2preflight.Export{managedExportFromFacts(client, facts)}
	client.bucketExists = true
	client.bucketPolicy = policy
	client.createdExports = nil
	client.createBucketRequests = nil
	client.putPolicyRequests = nil
	client.createExportRequests = nil
	client.headBucketRequests = nil
	client.getPolicyRequests = nil
}
