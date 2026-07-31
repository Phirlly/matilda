package cur2preflight

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestPreflightClassifiesReadOnlyProviderErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeClient)
		code   string
	}{
		{
			name: "generic configuration failure",
			mutate: func(client *fakeClient) {
				client.configErr = errors.New("configuration load failed")
			},
			code: "aws_config_missing_credentials",
		},
		{
			name: "identity auth failed",
			mutate: func(client *fakeClient) {
				client.identityErr = NewProviderError("aws_auth_failed", "signed identity request failed.")
			},
			code: "aws_auth_failed",
		},
		{
			name: "identity unavailable",
			mutate: func(client *fakeClient) {
				client.identity = Identity{}
			},
			code: "aws_identity_unavailable",
		},
		{
			name: "tables access denied",
			mutate: func(client *fakeClient) {
				client.tablePages = nil
				client.tableErr = nil
				client.listTablesErr = NewProviderError("aws_data_exports_access_denied", "table list denied.")
			},
			code: "aws_data_exports_access_denied",
		},
		{
			name: "get table unavailable",
			mutate: func(client *fakeClient) {
				client.tableErr = NewProviderError("aws_cur2_table_unavailable", "table unavailable.")
			},
			code: "aws_cur2_table_unavailable",
		},
		{
			name: "exports access denied",
			mutate: func(client *fakeClient) {
				client.listExportsErr = NewProviderError("aws_data_exports_access_denied", "export list denied.")
			},
			code: "aws_data_exports_access_denied",
		},
		{
			name: "get export unavailable",
			mutate: func(client *fakeClient) {
				client.exportErr = NewProviderError("aws_cur2_export_invalid_shape", "export unavailable.")
			},
			code: "aws_cur2_export_invalid_shape",
		},
		{
			name: "head bucket error",
			mutate: func(client *fakeClient) {
				client.bucketErr = NewProviderError("aws_s3_bucket_inaccessible", "bucket head failed.")
			},
			code: "aws_s3_bucket_inaccessible",
		},
		{
			name: "list executions denied",
			mutate: func(client *fakeClient) {
				client.executionErr = NewProviderError("aws_data_exports_access_denied", "execution list denied.")
			},
			code: "aws_data_exports_access_denied",
		},
		{
			name: "get execution denied",
			mutate: func(client *fakeClient) {
				client.getExecutionErr = NewProviderError("aws_data_exports_access_denied", "execution get denied.")
			},
			code: "aws_data_exports_access_denied",
		},
		{
			name: "list objects denied",
			mutate: func(client *fakeClient) {
				client.objectErr = NewProviderError("aws_s3_bucket_inaccessible", "object list denied.")
			},
			code: "aws_s3_bucket_inaccessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			tt.mutate(client)

			result := runPreflight(t, client)

			assertBlockedCode(t, result, tt.code)
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightClassifiesAdditionalQueryAndShapeFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeClient)
		code   string
	}{
		{
			name: "empty query",
			mutate: func(client *fakeClient) {
				client.export.QueryStatement = ""
			},
			code: "aws_cur2_query_unverified",
		},
		{
			name: "multiple select keywords",
			mutate: func(client *fakeClient) {
				client.export.QueryStatement = "SELECT SELECT " + requiredCUR2Select() + " FROM COST_AND_USAGE_REPORT"
			},
			code: "aws_cur2_query_unverified",
		},
		{
			name: "unsupported table clause",
			mutate: func(client *fakeClient) {
				client.export.QueryStatement = "SELECT " + requiredCUR2Select() + " FROM COST_AND_USAGE_REPORT alias"
			},
			code: "aws_cur2_query_unverified",
		},
		{
			name: "calculated expression",
			mutate: func(client *fakeClient) {
				client.export.QueryStatement = "SELECT sum(line_item_usage_amount) AS line_item_usage_amount, " + strings.Join(requiredCUR2ColumnsWithout("line_item_usage_amount"), ", ") + " FROM COST_AND_USAGE_REPORT"
			},
			code: "aws_cur2_query_unverified",
		},
		{
			name: "dot operator selection",
			mutate: func(client *fakeClient) {
				client.export.QueryStatement = "SELECT line_item.usage_amount AS line_item_usage_amount, " + strings.Join(requiredCUR2ColumnsWithout("line_item_usage_amount"), ", ") + " FROM COST_AND_USAGE_REPORT"
			},
			code: "aws_cur2_query_unverified",
		},
		{
			name: "unrecognized include resources",
			mutate: func(client *fakeClient) {
				client.export.TableConfigurations["COST_AND_USAGE_REPORT"]["INCLUDE_RESOURCES"] = "MAYBE"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "missing prefix",
			mutate: func(client *fakeClient) {
				client.export.Destination.Prefix = ""
			},
			code: "aws_cur2_export_invalid_shape",
		},
		{
			name: "missing region",
			mutate: func(client *fakeClient) {
				client.export.Destination.Region = ""
			},
			code: "aws_cur2_export_invalid_shape",
		},
		{
			name: "unhealthy export",
			mutate: func(client *fakeClient) {
				client.export.HealthStatus = "UNHEALTHY"
			},
			code: "aws_cur2_export_invalid_shape",
		},
		{
			name: "missing export health fails closed",
			mutate: func(client *fakeClient) {
				client.export.HealthStatus = ""
			},
			code: "aws_cur2_export_invalid_shape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			tt.mutate(client)

			result := runPreflight(t, client)

			assertBlockedCode(t, result, tt.code)
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightClassifiesAdditionalS3AndDeliveryEdges(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*fakeClient)
		code       string
		wantStatus workflow.RunStatus
	}{
		{
			name: "head bucket bad request is ambiguous",
			mutate: func(client *fakeClient) {
				client.bucketAccess = BucketAccess{Accessible: false, StatusCode: 400}
			},
			code:       "aws_s3_bucket_inaccessible",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "head bucket no status is unknown",
			mutate: func(client *fakeClient) {
				client.bucketAccess = BucketAccess{Accessible: false}
			},
			code:       "aws_s3_bucket_inaccessible",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "invalid bucket policy json warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = "{not-json"
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "old export with no executions blocks",
			mutate: func(client *fakeClient) {
				client.export.CreatedAt = fixedNow().Add(-25 * time.Hour)
				client.executionPages = []ExecutionPage{{}}
			},
			code:       "aws_cur2_delivery_not_started",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "unknown execution status warns",
			mutate: func(client *fakeClient) {
				client.execution = Execution{ID: "execution-1", Status: "QUEUED"}
			},
			code:       "aws_cur2_delivery_not_started",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "previous month missing after complete pages",
			mutate: func(client *fakeClient) {
				client.objectPages = []ObjectPage{
					{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-01/part.gz"}, NextToken: "next-object-page"},
					{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-02/part.gz"}},
				}
			},
			code:       "aws_backfill_manual_step_required",
			wantStatus: workflow.RunStatusManualSteps,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			tt.mutate(client)

			result := runPreflight(t, client)

			if result.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q; code=%q", result.Status, tt.wantStatus, result.Code)
			}
			if result.Code != tt.code {
				t.Fatalf("Code = %q, want %q", result.Code, tt.code)
			}
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightNilClientAndShortAccountAreSafe(t *testing.T) {
	nilResult := runPreflight(t, nil)
	assertBlockedCode(t, nilResult, "aws_provider_capability_blocked")
	assertNoUnsafeAWSOutput(t, nilResult)

	client := baselineClient()
	client.identity = Identity{AccountID: "123", CallerARN: "arn:aws:iam::123:role/operator"}
	shortAccountResult := runPreflight(t, client)

	if shortAccountResult.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q", shortAccountResult.Status, workflow.StatusReady)
	}
	assertCheckEvidence(t, shortAccountResult, "caller_account", "account-ending-unknown")
	assertNoUnsafeAWSOutput(t, shortAccountResult)
}

func TestPreflightBoundsDataExportsPagination(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeClient)
	}{
		{
			name: "table listing repeats continuation token",
			mutate: func(client *fakeClient) {
				client.tablePages = []TablePage{
					{
						Tables:    []TableSummary{{Name: "COST_AND_USAGE_REPORT"}},
						NextToken: "next-table-page",
					},
					{
						Tables:    []TableSummary{{Name: "COST_AND_USAGE_REPORT"}},
						NextToken: "next-table-page",
					},
				}
			},
		},
		{
			name: "table listing exceeds bounded inspection",
			mutate: func(client *fakeClient) {
				client.tablePagesByCall = true
				client.tablePages = make([]TablePage, maxDataExportsListPages+1)
				for index := range client.tablePages {
					client.tablePages[index] = TablePage{
						Tables:    []TableSummary{{Name: "COST_AND_USAGE_REPORT"}},
						NextToken: fmt.Sprintf("next-table-page-%03d", index+1),
					}
				}
				client.tablePages[len(client.tablePages)-1].NextToken = ""
			},
		},
		{
			name: "export listing exceeds bounded inspection",
			mutate: func(client *fakeClient) {
				client.exportPagesByCall = true
				client.exportPages = make([]ExportPage, maxDataExportsListPages+1)
				for index := range client.exportPages {
					client.exportPages[index] = ExportPage{
						Exports:   []ExportSummary{{Name: "focus", ExportARN: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus", TableName: "FOCUS_1_2_AWS"}},
						NextToken: fmt.Sprintf("next-export-page-%03d", index+1),
					}
				}
				client.exportPages[len(client.exportPages)-1] = ExportPage{
					Exports: []ExportSummary{{Name: "matilda-cur2", ExportARN: client.export.ExportARN, TableName: "COST_AND_USAGE_REPORT"}},
				}
			},
		},
		{
			name: "execution listing exceeds bounded inspection",
			mutate: func(client *fakeClient) {
				client.executionPagesByCall = true
				client.executionPages = make([]ExecutionPage, maxDataExportsListPages+1)
				for index := range client.executionPages {
					client.executionPages[index] = ExecutionPage{
						Executions: []Execution{{Status: "SUCCEEDED", StatusObservedAt: fixedNow().Add(time.Duration(index) * time.Minute)}},
						NextToken:  fmt.Sprintf("next-execution-page-%03d", index+1),
					}
				}
				client.executionPages[len(client.executionPages)-1].NextToken = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			tt.mutate(client)

			result := runPreflight(t, client)

			assertBlockedCode(t, result, "aws_data_exports_pagination_unbounded")
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightBoundsExportDetailInspection(t *testing.T) {
	client := baselineClient()
	exports := make([]ExportSummary, maxExportDetailChecks+1)
	for index := range exports {
		exports[index] = ExportSummary{
			Name:      fmt.Sprintf("export-%03d", index),
			ExportARN: fmt.Sprintf("arn:aws:bcm-data-exports:us-east-1:123456789012:export/export-%03d", index),
		}
	}
	client.exportPages = []ExportPage{{Exports: exports}}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_data_exports_pagination_unbounded")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightHandlesListedExportWithoutARN(t *testing.T) {
	client := baselineClient()
	client.exportPages = []ExportPage{{Exports: []ExportSummary{{Name: "incomplete-export-reference"}}}}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_cur2_export_not_found")
	if client.calls["GetExport"] != 0 {
		t.Fatalf("GetExport calls = %d, want 0 for missing export ARN", client.calls["GetExport"])
	}
	assertNoUnsafeAWSOutput(t, result)
}

func TestProviderErrorAndRequestHelpersAreStable(t *testing.T) {
	err := NewProviderError("aws_data_exports_transient", "temporary failure")
	if !strings.Contains(err.Error(), "aws_data_exports_transient") {
		t.Fatalf("ProviderError string = %q, want code", err.Error())
	}

	request := AWSBillingPreflightRequest()
	if request.Provider != "aws" || request.CollectionPath != "billing" || request.Action != "preflight" {
		t.Fatalf("AWSBillingPreflightRequest = %#v, want AWS billing preflight", request)
	}
}

func TestRunnerAndRunStateDefaultsFailClosedSafely(t *testing.T) {
	runner := NewRunner(RunnerConfig{Client: baselineClient()})
	if runner.now.IsZero() {
		t.Fatal("NewRunner did not default an unset clock")
	}

	state := newRunState(AWSBillingPreflightRequest())
	state.add(checkFinding{})
	report := state.report("", "")

	if report.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", report.Status, workflow.RunStatusBlocked)
	}
	if report.Code != "aws_provider_capability_blocked" {
		t.Fatalf("Code = %q, want aws_provider_capability_blocked", report.Code)
	}
	if report.PlanInput == nil || len(report.PlanInput.Checks) != 1 {
		t.Fatalf("PlanInput checks = %#v, want fallback blocked check", report.PlanInput)
	}
	if report.PlanInput.OperatorIdentitySummary.IdentityStatus != "unknown" {
		t.Fatalf("identity status = %q, want unknown", report.PlanInput.OperatorIdentitySummary.IdentityStatus)
	}
}

func TestPolicyAcceptsArrayActionAndBucketWideResource(t *testing.T) {
	client := baselineClient()
	client.bucketPolicy = bucketPolicy(policySpec{
		Service:       "bcm-data-exports.amazonaws.com",
		SourceAccount: client.export.SourceAccount,
		SourceARN:     client.export.SourceARN,
		Action:        []string{"s3:GetObject", "s3:PutObject"},
		Resource:      []string{"arn:aws:s3:::matilda-cur2-billing/*"},
	})

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	assertNoUnsafeAWSOutput(t, result)
}

func TestPolicyAcceptsParentPrefixWildcardResource(t *testing.T) {
	client := baselineClient()
	client.bucketPolicy = bucketPolicy(policySpec{
		Service:       "bcm-data-exports.amazonaws.com",
		SourceAccount: client.export.SourceAccount,
		SourceARN:     client.export.SourceARN,
		Action:        "s3:PutObject",
		Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/*",
	})
	client.objectPages = []ObjectPage{{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-07/part-000.gz"}}}

	result := runPreflight(t, client)

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.RunStatusManualSteps, result.Code)
	}
	if result.Code != "aws_backfill_manual_step_required" {
		t.Fatalf("Code = %q, want aws_backfill_manual_step_required", result.Code)
	}
	assertNoCheckEvidenceKey(t, result, "policy_gap")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPolicyRejectsWildcardThatOnlyMatchesSyntheticProbeObject(t *testing.T) {
	client := baselineClient()
	client.bucketPolicy = bucketPolicy(policySpec{
		Service:       "bcm-data-exports.amazonaws.com",
		SourceAccount: client.export.SourceAccount,
		SourceARN:     client.export.SourceARN,
		Action:        "s3:PutObject",
		Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/__matilda-preflight-*",
	})
	client.objectPages = []ObjectPage{{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-07/part-000.gz"}}}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_s3_delivery_policy_missing")
	assertCheckEvidence(t, result, "policy_gap", "put_object_resource_not_covered")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreviousMonthMatcherRejectsIncompleteDestination(t *testing.T) {
	export := baselineClient().export
	export.Destination.Prefix = ""

	if matchesPreviousMonthDataKey("matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part.gz", export, "2026-06") {
		t.Fatal("previous-month matcher accepted key with incomplete destination prefix")
	}
	if matchesPreviousMonthManifestKey("matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json", export, "2026-06") {
		t.Fatal("previous-month manifest matcher accepted key with incomplete destination prefix")
	}
}

func TestPreviousMonthDataMatcherRejectsFolderMarkers(t *testing.T) {
	export := baselineClient().export
	prefix := previousMonthDataPrefix(export, "2026-06")

	if matchesPreviousMonthDataKey(prefix, export, "2026-06") {
		t.Fatal("previous-month data matcher accepted partition folder marker")
	}
	if matchesPreviousMonthDataKey(prefix+"execution/", export, "2026-06") {
		t.Fatal("previous-month data matcher accepted execution folder marker")
	}
	if !matchesPreviousMonthDataKey(prefix+"execution/part.gz", export, "2026-06") {
		t.Fatal("previous-month data matcher rejected file below partition")
	}
}

func TestPreviousMonthManifestMatcherAcceptsAWSManifestPaths(t *testing.T) {
	export := baselineClient().export

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "latest manifest",
			key:  "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
			want: true,
		},
		{
			name: "export-named manifest",
			key:  "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/matilda-cur2-Manifest.json",
			want: true,
		},
		{
			name: "execution manifest",
			key:  "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/2026-06-15T00-00Z-execution/Manifest.json",
			want: true,
		},
		{
			name: "data file is not manifest",
			key:  "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part.gz",
		},
		{
			name: "wrong period manifest",
			key:  "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-05/Manifest.json",
		},
		{
			name: "wrong export manifest",
			key:  "matilda/cur2/other-export/metadata/BILLING_PERIOD=2026-06/Manifest.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesPreviousMonthManifestKey(tt.key, export, "2026-06"); got != tt.want {
				t.Fatalf("matchesPreviousMonthManifestKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
