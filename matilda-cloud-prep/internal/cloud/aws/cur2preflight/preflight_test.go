package cur2preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestPreflightReadyUsesCUR2AndSafeEvidence(t *testing.T) {
	client := baselineClient()
	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.SupportStatus != workflow.SupportSupported {
		t.Fatalf("SupportStatus = %q, want %q", result.SupportStatus, workflow.SupportSupported)
	}
	if result.Mutated {
		t.Fatal("preflight result reported mutation")
	}
	if !result.ProviderCapabilityImplemented {
		t.Fatal("provider capability should be marked implemented")
	}
	if result.Code != "aws_cur2_preflight_ready" {
		t.Fatalf("Code = %q, want aws_cur2_preflight_ready", result.Code)
	}
	if result.Plan == nil {
		t.Fatal("preflight result did not include an execution plan")
	}
	if got := result.Plan.StatusCounts.CheckStatuses[workflow.CheckWarn]; got == 0 {
		t.Fatal("expected INCLUDE_RESOURCES warning to be surfaced")
	}
	assertNoUnsafeAWSOutput(t, result)
	assertCheckEvidence(t, result, "caller_account", "account-ending-9012")
	assertCheckEvidence(t, result, "caller_ref", "sha256:")
	assertCheckEvidence(t, result, "cur_version", "CUR2.0")
	assertCheckEvidence(t, result, "previous_billing_period", "2026-06")
	assertSourceHandle(t, result, "docs/references/aws/aws-sdk-go-v2-readonly-adapter.md")

	for _, call := range []string{
		"CheckConfiguration",
		"GetCallerIdentity",
		"ListTables",
		"GetTable",
		"ListExports",
		"GetExport",
		"HeadBucket",
		"GetBucketPolicy",
		"ListExecutions",
		"GetExecution",
		"ListObjects",
	} {
		if client.calls[call] == 0 {
			t.Fatalf("%s was not called", call)
		}
	}
}

func TestPreflightDiscoversCUR2ByInspectingListedExports(t *testing.T) {
	client := baselineClient()
	client.exportPages = []ExportPage{{Exports: []ExportSummary{
		{Name: "focus", ExportARN: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus"},
		{Name: "matilda-cur2", ExportARN: client.export.ExportARN},
	}}}
	client.exportsByARN = map[string]Export{
		"arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus": {
			Name:           "focus",
			ExportARN:      "arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus",
			QueryStatement: "SELECT charge_category FROM FOCUS_1_2_AWS",
			TableConfigurations: map[string]map[string]string{
				"FOCUS_1_2_AWS": {},
			},
		},
		client.export.ExportARN: client.export,
	}

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.Code != "aws_cur2_preflight_ready" {
		t.Fatalf("Code = %q, want aws_cur2_preflight_ready", result.Code)
	}
	if client.calls["GetExport"] != 2 {
		t.Fatalf("GetExport calls = %d, want 2", client.calls["GetExport"])
	}
	if got := client.getExportARNs; !reflect.DeepEqual(got, []string{
		"arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus",
		client.export.ExportARN,
	}) {
		t.Fatalf("GetExport ARNs = %#v, want listed export ARNs in order", got)
	}
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightClassifiesConfigurationFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{
			name: "missing region",
			err:  NewProviderError("aws_config_missing_region", "AWS Region is not configured."),
			code: "aws_config_missing_region",
		},
		{
			name: "missing credentials",
			err:  NewProviderError("aws_config_missing_credentials", "AWS credentials are not available."),
			code: "aws_config_missing_credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			client.configErr = tt.err

			result := runPreflight(t, client)

			assertBlockedCode(t, result, tt.code)
			assertNoUnsafeAWSOutput(t, result)
			if client.calls["GetCallerIdentity"] != 0 {
				t.Fatal("identity check should not run after configuration failure")
			}
		})
	}
}

func TestPreflightClassifiesTableAndExportDiscoveryFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeClient)
		code   string
	}{
		{
			name: "missing CUR 2.0 table",
			mutate: func(client *fakeClient) {
				client.tablePages = []TablePage{{Tables: []TableSummary{{Name: "FOCUS_1_2_AWS"}}}}
			},
			code: "aws_cur2_table_unavailable",
		},
		{
			name: "required table column absent",
			mutate: func(client *fakeClient) {
				client.table.Columns = requiredCUR2Columns()[1:]
			},
			code: "aws_cur2_required_fields_missing",
		},
		{
			name: "no exports",
			mutate: func(client *fakeClient) {
				client.exportPages = []ExportPage{{}}
			},
			code: "aws_cur2_export_not_found",
		},
		{
			name: "multiple CUR 2.0 exports",
			mutate: func(client *fakeClient) {
				exportA := client.export
				exportA.Name = "cur2-a"
				exportA.ExportARN = "arn:aws:bcm-data-exports:us-east-1:123456789012:export/a"
				exportA.SourceARN = exportA.ExportARN
				exportB := client.export
				exportB.Name = "cur2-b"
				exportB.ExportARN = "arn:aws:bcm-data-exports:us-east-1:123456789012:export/b"
				exportB.SourceARN = exportB.ExportARN
				client.exportPages = []ExportPage{{Exports: []ExportSummary{
					{Name: exportA.Name, ExportARN: exportA.ExportARN, TableName: "COST_AND_USAGE_REPORT"},
					{Name: exportB.Name, ExportARN: exportB.ExportARN, TableName: "COST_AND_USAGE_REPORT"},
				}}}
				client.exportsByARN = map[string]Export{
					exportA.ExportARN: exportA,
					exportB.ExportARN: exportB,
				}
			},
			code: "aws_cur2_export_ambiguous",
		},
		{
			name: "FOCUS export only",
			mutate: func(client *fakeClient) {
				client.exportPages = []ExportPage{{Exports: []ExportSummary{
					{Name: "focus", ExportARN: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus"},
				}}}
				client.exportsByARN = map[string]Export{
					"arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus": {
						Name:           "focus",
						ExportARN:      "arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus",
						QueryStatement: "SELECT charge_category FROM FOCUS_1_2_AWS",
						TableConfigurations: map[string]map[string]string{
							"FOCUS_1_2_AWS": {},
						},
					},
				}
			},
			code: "aws_non_cur2_source_out_of_scope",
		},
		{
			name: "legacy CUR export only",
			mutate: func(client *fakeClient) {
				client.exportPages = []ExportPage{{Exports: []ExportSummary{
					{Name: "legacy", ExportARN: "arn:aws:cur:us-east-1:123456789012:definition/legacy"},
				}}}
				client.exportsByARN = map[string]Export{
					"arn:aws:cur:us-east-1:123456789012:definition/legacy": {
						Name:           "legacy",
						ExportARN:      "arn:aws:cur:us-east-1:123456789012:definition/legacy",
						QueryStatement: "SELECT identity_line_item_id FROM LEGACY_CUR",
					},
				}
			},
			code: "aws_non_cur2_source_out_of_scope",
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

func TestPreflightAmbiguousCUR2ExportsReturnsSafeCandidateRefs(t *testing.T) {
	client := baselineClient()
	secondARN := "arn:aws:bcm-data-exports:us-east-1:123456789012:export/finance-cur2"
	client.exportPages = []ExportPage{{Exports: []ExportSummary{
		{Name: "matilda-cur2", ExportARN: client.export.ExportARN, TableName: "COST_AND_USAGE_REPORT"},
		{Name: "finance-cur2", ExportARN: secondARN, TableName: "COST_AND_USAGE_REPORT"},
	}}}
	secondExport := client.export
	secondExport.Name = "finance-cur2"
	secondExport.ExportARN = secondARN
	secondExport.SourceARN = secondARN
	secondExport.HealthStatus = "WARNING"
	secondExport.Destination.Output.Format = "PARQUET"
	secondExport.Destination.Region = "us-west-2"
	client.exportsByARN = map[string]Export{
		client.export.ExportARN: client.export,
		secondARN:               secondExport,
	}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_cur2_export_ambiguous")
	assertGroupedCandidateEvidence(t, result, 1, cur2ExportRef(client.export.ExportARN), "HEALTHY", "TEXT_OR_CSV", "us-east-1")
	assertGroupedCandidateEvidence(t, result, 2, cur2ExportRef(secondARN), "WARNING", "PARQUET", "us-west-2")
	assertNoCheckEvidenceKey(t, result, "candidate_export_ref")
	assertNoCheckEvidenceKey(t, result, "candidate_health")
	assertNoCheckEvidenceKey(t, result, "candidate_output_format")
	assertNoCheckEvidenceKey(t, result, "candidate_destination_region")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightOmitsUnsafeCandidateMetadata(t *testing.T) {
	client := baselineClient()
	secondARN := "arn:aws:bcm-data-exports:us-east-1:123456789012:export/finance-cur2"
	client.exportPages = []ExportPage{{Exports: []ExportSummary{
		{Name: "matilda-cur2", ExportARN: client.export.ExportARN, TableName: "COST_AND_USAGE_REPORT"},
		{Name: "finance-cur2", ExportARN: secondARN, TableName: "COST_AND_USAGE_REPORT"},
	}}}
	client.export.HealthStatus = "/private/tmp/health"
	client.export.Destination.Output.Format = "secret_key=plain-secret-key"
	client.export.Destination.Region = "arn:aws:ec2:us-east-1:123456789012:region/us-east-1"
	secondExport := client.export
	secondExport.Name = "finance-cur2"
	secondExport.ExportARN = secondARN
	secondExport.SourceARN = secondARN
	secondExport.HealthStatus = "HEALTHY"
	secondExport.Destination.Output.Format = "PARQUET"
	secondExport.Destination.Region = "us-west-2"
	client.exportsByARN = map[string]Export{
		client.export.ExportARN: client.export,
		secondARN:               secondExport,
	}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_cur2_export_ambiguous")
	assertCheckEvidence(t, result, "candidate_1_export_ref", cur2ExportRef(client.export.ExportARN))
	assertNoCheckEvidenceKey(t, result, "candidate_1_health")
	assertNoCheckEvidenceKey(t, result, "candidate_1_output_format")
	assertNoCheckEvidenceKey(t, result, "candidate_1_destination_region")
	assertGroupedCandidateEvidence(t, result, 2, cur2ExportRef(secondARN), "HEALTHY", "PARQUET", "us-west-2")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightSelectsCUR2ExportByRef(t *testing.T) {
	client := baselineClient()
	selectedARN := "arn:aws:bcm-data-exports:us-east-1:123456789012:export/finance-cur2"
	client.exportPages = []ExportPage{{Exports: []ExportSummary{
		{Name: "matilda-cur2", ExportARN: client.export.ExportARN, TableName: "COST_AND_USAGE_REPORT"},
		{Name: "finance-cur2", ExportARN: selectedARN, TableName: "COST_AND_USAGE_REPORT"},
	}}}
	selectedExport := client.export
	selectedExport.Name = "finance-cur2"
	selectedExport.ExportARN = selectedARN
	selectedExport.SourceARN = selectedARN
	selectedExport.Destination.Prefix = "finance/cur2"
	client.exportsByARN = map[string]Export{
		client.export.ExportARN: client.export,
		selectedARN:             selectedExport,
	}
	client.bucketPolicy = bucketPolicy(policySpec{
		Service:       "bcm-data-exports.amazonaws.com",
		SourceAccount: selectedExport.SourceAccount,
		SourceARN:     selectedExport.SourceARN,
		Action:        "s3:PutObject",
		Resource:      "arn:aws:s3:::matilda-cur2-billing/finance/cur2/*",
	})
	client.objectPages = []ObjectPage{{
		Keys: []string{"finance/cur2/finance-cur2/data/BILLING_PERIOD=2026-06/part-000.gz"},
	}}

	result := runPreflightWithOptions(t, client, workflow.ExecutionOptions{
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				CUR2ExportRef: cur2ExportRef(selectedARN),
			},
		},
	})

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.Code != "aws_cur2_preflight_ready" {
		t.Fatalf("Code = %q, want aws_cur2_preflight_ready", result.Code)
	}
	assertCheckEvidence(t, result, "selected_export_ref", cur2ExportRef(selectedARN))
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightRejectsUnknownCUR2ExportRef(t *testing.T) {
	client := baselineClient()
	secondARN := "arn:aws:bcm-data-exports:us-east-1:123456789012:export/finance-cur2"
	client.exportPages = []ExportPage{{Exports: []ExportSummary{
		{Name: "matilda-cur2", ExportARN: client.export.ExportARN, TableName: "COST_AND_USAGE_REPORT"},
		{Name: "finance-cur2", ExportARN: secondARN, TableName: "COST_AND_USAGE_REPORT"},
	}}}
	secondExport := client.export
	secondExport.Name = "finance-cur2"
	secondExport.ExportARN = secondARN
	secondExport.SourceARN = secondARN
	client.exportsByARN = map[string]Export{
		client.export.ExportARN: client.export,
		secondARN:               secondExport,
	}

	result := runPreflightWithOptions(t, client, workflow.ExecutionOptions{
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				CUR2ExportRef: "cur2-ffffffffffffffff",
			},
		},
	})

	assertBlockedCode(t, result, "aws_cur2_export_ref_not_found")
	assertGroupedCandidateEvidence(t, result, 1, cur2ExportRef(client.export.ExportARN), "HEALTHY", "TEXT_OR_CSV", "us-east-1")
	assertGroupedCandidateEvidence(t, result, 2, cur2ExportRef(secondARN), "HEALTHY", "TEXT_OR_CSV", "us-east-1")
	assertNoCheckEvidenceKey(t, result, "candidate_export_ref")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightFailsClosedWhenExportRefsCannotBeUnique(t *testing.T) {
	client := baselineClient()
	duplicateARN := client.export.ExportARN
	client.exportPages = []ExportPage{{Exports: []ExportSummary{
		{Name: "matilda-cur2", ExportARN: duplicateARN, TableName: "COST_AND_USAGE_REPORT"},
		{Name: "duplicate-cur2", ExportARN: duplicateARN, TableName: "COST_AND_USAGE_REPORT"},
	}}}
	duplicateExport := client.export
	duplicateExport.Name = "duplicate-cur2"
	client.exportsByARN = map[string]Export{
		duplicateARN: duplicateExport,
	}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_cur2_export_ref_collision")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightRejectsUnverifiedQueries(t *testing.T) {
	validSelect := requiredCUR2Select()
	tests := []struct {
		name  string
		query string
		code  string
	}{
		{
			name:  "select star",
			query: "SELECT * FROM COST_AND_USAGE_REPORT",
			code:  "aws_cur2_query_unverified",
		},
		{
			name:  "select without fields",
			query: "SELECT FROM COST_AND_USAGE_REPORT",
			code:  "aws_cur2_query_unverified",
		},
		{
			name:  "wrong table case",
			query: "SELECT " + validSelect + " FROM cost_and_usage_report",
			code:  "aws_non_cur2_source_out_of_scope",
		},
		{
			name:  "missing required field",
			query: "SELECT " + strings.Join(requiredCUR2Columns()[1:], ", ") + " FROM COST_AND_USAGE_REPORT",
			code:  "aws_cur2_required_fields_missing",
		},
		{
			name:  "where clause",
			query: "SELECT " + validSelect + " FROM COST_AND_USAGE_REPORT WHERE line_item_usage_amount > 0",
			code:  "aws_cur2_query_unverified",
		},
		{
			name:  "limit clause",
			query: "SELECT " + validSelect + " FROM COST_AND_USAGE_REPORT LIMIT 10",
			code:  "aws_cur2_query_unverified",
		},
		{
			name:  "different source aliased as required field",
			query: "SELECT cost AS line_item_usage_amount, " + strings.Join(requiredCUR2ColumnsWithout("line_item_usage_amount"), ", ") + " FROM COST_AND_USAGE_REPORT",
			code:  "aws_cur2_query_unverified",
		},
		{
			name:  "required field renamed",
			query: "SELECT line_item_usage_amount AS usage_amount, " + strings.Join(requiredCUR2ColumnsWithout("line_item_usage_amount"), ", ") + " FROM COST_AND_USAGE_REPORT",
			code:  "aws_cur2_query_unverified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			client.export.QueryStatement = tt.query

			result := runPreflight(t, client)

			assertBlockedCode(t, result, tt.code)
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightAcceptsAWSStandardMultilineCUR2Query(t *testing.T) {
	client := baselineClient()
	client.table.Columns = append(client.table.Columns, "line_item_resource_id")
	client.export.QueryStatement = "select\n" +
		"  " + strings.Join(requiredCUR2Columns(), ",\n  ") + ",\n" +
		"  line_item_resource_id\n" +
		"from\n" +
		"  COST_AND_USAGE_REPORT"

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q: %#v", result.Status, workflow.StatusReady, result.Plan.Checks)
	}
	assertNoUnsafeAWSOutput(t, result)
}

func TestReferencesCUR2QuerySourceAcceptsAWSStandardWhitespace(t *testing.T) {
	query := "select\n" +
		"  " + requiredCUR2Select() + "\n" +
		"from\n" +
		"  COST_AND_USAGE_REPORT"

	if !referencesCUR2QuerySource(query) {
		t.Fatalf("referencesCUR2QuerySource rejected AWS-standard multiline query")
	}
	if referencesCUR2QuerySource("SELECT " + requiredCUR2Select() + " FROM COST_AND_USAGE_REPORT JOIN other_table") {
		t.Fatalf("referencesCUR2QuerySource accepted unsupported table clause")
	}
}

func TestPreflightAllowsVerifiedExtraTopLevelCUR2Columns(t *testing.T) {
	client := baselineClient()
	client.table.Columns = append(client.table.Columns,
		"bill_billing_period_start_date",
		"line_item_resource_id",
	)
	client.export.QueryStatement = "SELECT " + requiredCUR2Select() + ", bill_billing_period_start_date, line_item_resource_id FROM COST_AND_USAGE_REPORT"

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.Code != "aws_cur2_preflight_ready" {
		t.Fatalf("Code = %q, want aws_cur2_preflight_ready", result.Code)
	}
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightValidatesExtraCUR2ColumnsAgainstTableSchema(t *testing.T) {
	tests := []struct {
		name  string
		query string
		code  string
	}{
		{
			name:  "unknown extra column",
			query: "SELECT " + requiredCUR2Select() + ", not_a_cur2_column FROM COST_AND_USAGE_REPORT",
			code:  "aws_cur2_query_unverified",
		},
		{
			name:  "renamed extra column",
			query: "SELECT " + requiredCUR2Select() + ", bill_billing_period_start_date AS billing_start FROM COST_AND_USAGE_REPORT",
			code:  "aws_cur2_query_unverified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			client.table.Columns = append(client.table.Columns, "bill_billing_period_start_date")
			client.export.QueryStatement = tt.query

			result := runPreflight(t, client)

			assertBlockedCode(t, result, tt.code)
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightRequestsTableSchemaAfterSelectedExportConfiguration(t *testing.T) {
	client := baselineClient()

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if !client.getTableAfterExport {
		t.Fatal("GetTable should use the selected export context, after GetExport")
	}
	if !reflect.DeepEqual(client.lastTableProperties, client.export.TableConfigurations["COST_AND_USAGE_REPORT"]) {
		t.Fatalf("GetTable table properties = %#v, want %#v", client.lastTableProperties, client.export.TableConfigurations["COST_AND_USAGE_REPORT"])
	}
}

func TestPreflightBlocksOutputSettingDeviations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Export)
		code   string
	}{
		{
			name: "daily granularity",
			mutate: func(export *Export) {
				export.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"] = "DAILY"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "parquet format",
			mutate: func(export *Export) {
				export.Destination.Output.Format = "PARQUET"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "parquet compression",
			mutate: func(export *Export) {
				export.Destination.Output.Compression = "PARQUET"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "overwrite report",
			mutate: func(export *Export) {
				export.Destination.Output.Overwrite = "OVERWRITE_REPORT"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "non custom output type",
			mutate: func(export *Export) {
				export.Destination.Output.OutputType = "LEGACY"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "non synchronous cadence",
			mutate: func(export *Export) {
				export.RefreshCadence = "ASYNCHRONOUS"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "missing bucket",
			mutate: func(export *Export) {
				export.Destination.Bucket = ""
			},
			code: "aws_cur2_export_invalid_shape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			tt.mutate(&client.export)

			result := runPreflight(t, client)

			assertBlockedCode(t, result, tt.code)
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightClassifiesS3PolicyAndPreviousMonthFailures(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*fakeClient)
		code       string
		wantStatus workflow.RunStatus
	}{
		{
			name: "head bucket forbidden is ambiguous",
			mutate: func(client *fakeClient) {
				client.bucketAccess = BucketAccess{Accessible: false, StatusCode: 403}
			},
			code:       "aws_s3_bucket_inaccessible",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "head bucket not found is ambiguous",
			mutate: func(client *fakeClient) {
				client.bucketAccess = BucketAccess{Accessible: false, StatusCode: 404}
			},
			code:       "aws_s3_bucket_inaccessible",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "missing data exports principal",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "s3.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "missing source account",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:   "bcm-data-exports.amazonaws.com",
					SourceARN: client.export.SourceARN,
					Action:    "s3:PutObject",
					Resource:  "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "missing source arn",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "wrong source account",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: "999999999999",
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "wrong source arn",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     "arn:aws:bcm-data-exports:us-east-1:123456789012:export/wrong",
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "missing put object action",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:GetObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "wrong policy resource",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::other-bucket/other-prefix/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "negated source account condition does not prove allow",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringNotEquals", "ArnLike")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "negated source arn condition does not prove allow",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringEquals", "ArnNotLike")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "documented bucket policy shape",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       []string{"bcm-data-exports.amazonaws.com"},
					SourceAccount: client.export.SourceAccount,
					SourceARN:     "arn:aws:bcm-data-exports:us-east-1:123456789012:export/*",
					Action:        []string{"s3:PutObject"},
					Resource:      []string{"arn:aws:s3:::matilda-cur2-billing/*"},
				})
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "documented bucket policy with condition value lists",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionValueLists(client)
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "global source arn wildcard does not prove allow",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       []string{"bcm-data-exports.amazonaws.com"},
					SourceAccount: client.export.SourceAccount,
					SourceARN:     "*",
					Action:        []string{"s3:PutObject"},
					Resource:      []string{"arn:aws:s3:::matilda-cur2-billing/*"},
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "exact source arn operator does not accept wildcard",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     "arn:aws:bcm-data-exports:us-east-1:123456789012:export/*",
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringEquals", "StringEquals")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "later valid statement can satisfy policy",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = multiStatementBucketPolicy(
					policySpec{
						Service:       "bcm-data-exports.amazonaws.com",
						SourceAccount: client.export.SourceAccount,
						SourceARN:     client.export.SourceARN,
						Action:        "s3:GetObject",
						Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
					},
					policySpec{
						Service:       "bcm-data-exports.amazonaws.com",
						SourceAccount: client.export.SourceAccount,
						SourceARN:     client.export.SourceARN,
						Action:        "s3:PutObject",
						Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
					},
				)
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "policy inaccessible",
			mutate: func(client *fakeClient) {
				client.bucketPolicyErr = NewProviderError("aws_s3_bucket_policy_inaccessible", "bucket policy cannot be inspected.")
			},
			code:       "aws_s3_bucket_policy_inaccessible",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "previous month missing requires manual backfill",
			mutate: func(client *fakeClient) {
				client.objectPages = []ObjectPage{{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-07/part-000.gz"}}}
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
				t.Fatalf("Status = %q, want %q", result.Status, tt.wantStatus)
			}
			if result.Code != tt.code {
				t.Fatalf("Code = %q, want %q", result.Code, tt.code)
			}
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightClassifiesDeliveryStatus(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*fakeClient)
		code       string
		wantStatus workflow.RunStatus
	}{
		{
			name: "new export with no executions warns",
			mutate: func(client *fakeClient) {
				client.export.CreatedAt = fixedNow().Add(-2 * time.Hour)
				client.executionPages = []ExecutionPage{{}}
			},
			code:       "aws_cur2_delivery_not_started",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "AWS delivery success passes",
			mutate: func(client *fakeClient) {
				client.executionPages = []ExecutionPage{{Executions: []Execution{{ID: "execution-1"}}}}
				client.execution = Execution{ID: "execution-1", Status: "DELIVERY_SUCCESS", StatusObservedAt: fixedNow().Add(-1 * time.Hour)}
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "AWS delivery in progress warns",
			mutate: func(client *fakeClient) {
				client.executionPages = []ExecutionPage{{Executions: []Execution{{ID: "execution-1"}}}}
				client.execution = Execution{ID: "execution-1", Status: "DELIVERY_IN_PROCESS"}
			},
			code:       "aws_cur2_delivery_not_started",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "AWS delivery failure blocks",
			mutate: func(client *fakeClient) {
				client.executionPages = []ExecutionPage{{Executions: []Execution{{ID: "execution-1"}}}}
				client.execution = Execution{ID: "execution-1", Status: "DELIVERY_FAILURE"}
			},
			code:       "aws_cur2_export_invalid_shape",
			wantStatus: workflow.RunStatusBlocked,
		},
		{
			name: "unverified success-like status warns",
			mutate: func(client *fakeClient) {
				client.executionPages = []ExecutionPage{{Executions: []Execution{{ID: "execution-1"}}}}
				client.execution = Execution{ID: "execution-1", Status: "SUCCEEDED"}
			},
			code:       "aws_cur2_delivery_not_started",
			wantStatus: workflow.StatusReady,
		},
		{
			name: "latest failed execution blocks despite older success",
			mutate: func(client *fakeClient) {
				client.executionPages = []ExecutionPage{{Executions: []Execution{
					{ID: "older", StatusObservedAt: fixedNow().Add(-3 * time.Hour)},
					{ID: "newer", StatusObservedAt: fixedNow().Add(-1 * time.Hour)},
				}}}
				client.executionsByID = map[string]Execution{
					"older": {ID: "older", Status: "DELIVERY_SUCCESS", StatusObservedAt: fixedNow().Add(-3 * time.Hour)},
					"newer": {ID: "newer", Status: "DELIVERY_FAILURE", StatusObservedAt: fixedNow().Add(-1 * time.Hour)},
				}
			},
			code:       "aws_cur2_export_invalid_shape",
			wantStatus: workflow.RunStatusBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			tt.mutate(client)

			result := runPreflight(t, client)

			if result.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", result.Status, tt.wantStatus)
			}
			if result.Code != tt.code {
				t.Fatalf("Code = %q, want %q", result.Code, tt.code)
			}
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightChoosesLatestExecutionByStatusObservationTime(t *testing.T) {
	client := baselineClient()
	client.executionPages = []ExecutionPage{{Executions: []Execution{
		{ID: "older-created", StatusObservedAt: fixedNow().Add(-30 * time.Minute)},
		{ID: "newer-updated", StatusObservedAt: fixedNow().Add(-5 * time.Minute)},
	}}}
	client.executionsByID = map[string]Execution{
		"older-created": {ID: "older-created", Status: "DELIVERY_SUCCESS", StatusObservedAt: fixedNow().Add(-30 * time.Minute)},
		"newer-updated": {ID: "newer-updated", Status: "DELIVERY_FAILURE", StatusObservedAt: fixedNow().Add(-5 * time.Minute)},
	}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_cur2_export_invalid_shape")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightHandlesListPagination(t *testing.T) {
	client := baselineClient()
	client.exportPages = []ExportPage{
		{
			Exports:   []ExportSummary{{Name: "focus", ExportARN: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus"}},
			NextToken: "next-export-page",
		},
		{
			Exports: []ExportSummary{{Name: "matilda-cur2", ExportARN: client.export.ExportARN}},
		},
	}
	client.exportsByARN = map[string]Export{
		"arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus": {
			Name:           "focus",
			ExportARN:      "arn:aws:bcm-data-exports:us-east-1:123456789012:export/focus",
			QueryStatement: "SELECT charge_category FROM FOCUS_1_2_AWS",
			TableConfigurations: map[string]map[string]string{
				"FOCUS_1_2_AWS": {},
			},
		},
		client.export.ExportARN: client.export,
	}
	client.objectPages = []ObjectPage{
		{
			Keys:      []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-05/part-000.gz"},
			NextToken: "next-object-page",
		},
		{
			Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz"},
		},
	}

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.StatusReady)
	}
	if client.calls["ListExports"] != 2 {
		t.Fatalf("ListExports calls = %d, want 2", client.calls["ListExports"])
	}
	if client.calls["ListObjects"] != 2 {
		t.Fatalf("ListObjects calls = %d, want 2", client.calls["ListObjects"])
	}
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightBlocksBucketRegionMismatch(t *testing.T) {
	client := baselineClient()
	client.bucketAccess.Region = "us-west-2"

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_s3_bucket_inaccessible")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightRequiresSpecificPreviousMonthExportPath(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		code string
	}{
		{
			name: "billing period under wrong export name",
			keys: []string{"matilda/cur2/wrong-export/data/BILLING_PERIOD=2026-06/part-000.gz"},
			code: "aws_backfill_manual_step_required",
		},
		{
			name: "billing period under metadata path",
			keys: []string{"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json"},
			code: "aws_backfill_manual_step_required",
		},
		{
			name: "billing period under export data path",
			keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz"},
			code: "aws_cur2_preflight_ready",
		},
		{
			name: "old pre-correction data path order",
			keys: []string{"matilda/cur2/data/matilda-cur2/BILLING_PERIOD=2026-06/part-000.gz"},
			code: "aws_backfill_manual_step_required",
		},
		{
			name: "billing period embedded in backup sibling path",
			keys: []string{"backup/matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz"},
			code: "aws_backfill_manual_step_required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			client.objectPages = []ObjectPage{{Keys: tt.keys}}

			result := runPreflight(t, client)

			if result.Code != tt.code {
				t.Fatalf("Code = %q, want %q", result.Code, tt.code)
			}
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightClassifiesBoundedPaginationIncomplete(t *testing.T) {
	client := baselineClient()
	client.objectPages = []ObjectPage{
		{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-01/part.gz"}, NextToken: "next-object-page"},
		{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-02/part.gz"}, NextToken: "next-object-page"},
		{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-03/part.gz"}, NextToken: "next-object-page"},
		{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-04/part.gz"}, NextToken: "next-object-page"},
		{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-05/part.gz"}, NextToken: "next-object-page"},
		{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part.gz"}},
	}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_cur2_previous_month_missing")
	assertNoUnsafeAWSOutput(t, result)
}

func runPreflight(t *testing.T, client *fakeClient) workflow.Result {
	t.Helper()
	return runPreflightWithOptions(t, client, workflow.ExecutionOptions{})
}

func runPreflightWithOptions(t *testing.T, client *fakeClient, options workflow.ExecutionOptions) workflow.Result {
	t.Helper()

	request := workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: request,
		Runner: NewRunner(RunnerConfig{
			Client: client,
			Now:    fixedNow(),
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	return registry.ExecuteContext(context.Background(), request, options)
}

func assertBlockedCode(t *testing.T, result workflow.Result, code string) {
	t.Helper()

	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.RunStatusBlocked)
	}
	if result.Code != code {
		t.Fatalf("Code = %q, want %q", result.Code, code)
	}
	if result.Mutated {
		t.Fatal("blocked preflight must not report mutation")
	}
}

func assertNoUnsafeAWSOutput(t *testing.T, result workflow.Result) {
	t.Helper()

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal result returned error: %v", err)
	}
	output := string(encoded)
	for _, forbidden := range []string{
		"arn:aws",
		"/Users/",
		"AKIA",
		"access_key",
		"secret_key",
		"session_token",
		"raw_billing",
		"line item row",
		"part-000.gz content",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("unsafe AWS output contains %q in %s", forbidden, output)
		}
	}
}

func assertCheckEvidence(t *testing.T, result workflow.Result, key string, wantContains string) {
	t.Helper()

	values := checkEvidenceValues(result, key)
	for _, value := range values {
		if strings.Contains(value, wantContains) {
			return
		}
	}
	t.Fatalf("did not find evidence key %q containing %q", key, wantContains)
}

func assertNoCheckEvidenceKey(t *testing.T, result workflow.Result, key string) {
	t.Helper()

	if values := checkEvidenceValues(result, key); len(values) > 0 {
		t.Fatalf("evidence key %q = %#v, want absent", key, values)
	}
}

func assertGroupedCandidateEvidence(t *testing.T, result workflow.Result, index int, exportRef string, health string, outputFormat string, region string) {
	t.Helper()

	prefix := fmt.Sprintf("candidate_%d", index)
	assertCheckEvidence(t, result, prefix+"_export_ref", exportRef)
	assertCheckEvidence(t, result, prefix+"_health", health)
	assertCheckEvidence(t, result, prefix+"_output_format", outputFormat)
	assertCheckEvidence(t, result, prefix+"_destination_region", region)
	if !strings.HasPrefix(exportRef, "cur2-") {
		t.Fatalf("candidate %d export ref = %q, want cur2- prefix", index, exportRef)
	}
}

func checkEvidenceValues(result workflow.Result, key string) []string {
	if result.Plan == nil {
		return nil
	}
	values := []string{}
	for _, check := range result.Plan.Checks {
		for _, evidence := range check.Evidence {
			if evidence.Key == key {
				values = append(values, evidence.Value)
			}
		}
	}
	return values
}

func assertSourceHandle(t *testing.T, result workflow.Result, uri string) {
	t.Helper()

	for _, handle := range result.SourceHandles {
		if handle.URI == uri {
			return
		}
	}
	t.Fatalf("did not find source handle URI %q in %#v", uri, result.SourceHandles)
}

type fakeClient struct {
	config               Configuration
	configErr            error
	identity             Identity
	identityErr          error
	tablePages           []TablePage
	tablePagesByCall     bool
	listTablesErr        error
	table                Table
	tableErr             error
	exportPages          []ExportPage
	exportPagesByCall    bool
	listExportsErr       error
	export               Export
	exportsByARN         map[string]Export
	getExportARNs        []string
	exportErr            error
	bucketAccess         BucketAccess
	bucketErr            error
	bucketPolicy         string
	bucketPolicyErr      error
	executionPages       []ExecutionPage
	executionPagesByCall bool
	execution            Execution
	executionsByID       map[string]Execution
	executionErr         error
	getExecutionErr      error
	objectPages          []ObjectPage
	objectErr            error
	lastTableProperties  map[string]string
	getTableAfterExport  bool
	calls                map[string]int
}

func (f *fakeClient) CheckConfiguration(context.Context) (Configuration, error) {
	f.record("CheckConfiguration")
	return f.config, f.configErr
}

func (f *fakeClient) GetCallerIdentity(context.Context) (Identity, error) {
	f.record("GetCallerIdentity")
	return f.identity, f.identityErr
}

func (f *fakeClient) ListTables(_ context.Context, token string) (TablePage, error) {
	f.record("ListTables")
	if f.listTablesErr != nil {
		return TablePage{}, f.listTablesErr
	}
	index := pageIndex(token)
	if f.tablePagesByCall {
		index = f.calls["ListTables"] - 1
	}
	if index >= len(f.tablePages) {
		return TablePage{}, nil
	}
	return f.tablePages[index], nil
}

func (f *fakeClient) GetTable(_ context.Context, name string, properties map[string]string) (Table, error) {
	f.record("GetTable")
	f.getTableAfterExport = f.calls["GetExport"] > 0
	f.lastTableProperties = copyStringMap(properties)
	return f.table, f.tableErr
}

func (f *fakeClient) ListExports(_ context.Context, token string) (ExportPage, error) {
	f.record("ListExports")
	if f.listExportsErr != nil {
		return ExportPage{}, f.listExportsErr
	}
	index := pageIndex(token)
	if f.exportPagesByCall {
		index = f.calls["ListExports"] - 1
	}
	if index >= len(f.exportPages) {
		return ExportPage{}, nil
	}
	return f.exportPages[index], nil
}

func (f *fakeClient) GetExport(_ context.Context, exportARN string) (Export, error) {
	f.record("GetExport")
	f.getExportARNs = append(f.getExportARNs, exportARN)
	if f.exportsByARN != nil {
		return f.exportsByARN[exportARN], f.exportErr
	}
	return f.export, f.exportErr
}

func (f *fakeClient) HeadBucket(_ context.Context, bucket string) (BucketAccess, error) {
	f.record("HeadBucket")
	return f.bucketAccess, f.bucketErr
}

func (f *fakeClient) GetBucketPolicy(_ context.Context, bucket string) (string, error) {
	f.record("GetBucketPolicy")
	return f.bucketPolicy, f.bucketPolicyErr
}

func (f *fakeClient) ListExecutions(_ context.Context, exportARN string, token string) (ExecutionPage, error) {
	f.record("ListExecutions")
	index := pageIndex(token)
	if f.executionPagesByCall {
		index = f.calls["ListExecutions"] - 1
	}
	if index >= len(f.executionPages) {
		return ExecutionPage{}, nil
	}
	return f.executionPages[index], f.executionErr
}

func (f *fakeClient) GetExecution(_ context.Context, exportARN string, executionID string) (Execution, error) {
	f.record("GetExecution")
	if f.getExecutionErr != nil {
		return Execution{}, f.getExecutionErr
	}
	if f.executionsByID != nil {
		return f.executionsByID[executionID], f.executionErr
	}
	return f.execution, f.executionErr
}

func (f *fakeClient) ListObjects(_ context.Context, bucket string, prefix string, token string, maxKeys int32) (ObjectPage, error) {
	f.record("ListObjects")
	index := f.calls["ListObjects"] - 1
	if index >= len(f.objectPages) {
		return ObjectPage{}, nil
	}
	return f.objectPages[index], f.objectErr
}

func (f *fakeClient) record(call string) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[call]++
}

func pageIndex(token string) int {
	if token == "" {
		return 0
	}
	if strings.Contains(token, "next") {
		return 1
	}
	return 0
}

func baselineClient() *fakeClient {
	sourceARN := "arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2"
	export := Export{
		Name:           "matilda-cur2",
		ExportARN:      sourceARN,
		SourceARN:      sourceARN,
		SourceAccount:  "123456789012",
		QueryStatement: "SELECT " + requiredCUR2Select() + " FROM COST_AND_USAGE_REPORT",
		TableConfigurations: map[string]map[string]string{
			"COST_AND_USAGE_REPORT": {
				"TIME_GRANULARITY":  "MONTHLY",
				"INCLUDE_RESOURCES": "FALSE",
			},
		},
		Destination: S3Destination{
			Bucket: "matilda-cur2-billing",
			Prefix: "matilda/cur2",
			Region: "us-east-1",
			Output: S3Output{
				Format:      "TEXT_OR_CSV",
				Compression: "GZIP",
				Overwrite:   "CREATE_NEW_REPORT",
				OutputType:  "CUSTOM",
			},
		},
		RefreshCadence: "SYNCHRONOUS",
		CreatedAt:      fixedNow().Add(-48 * time.Hour),
		HealthStatus:   "HEALTHY",
	}

	return &fakeClient{
		config:   Configuration{Region: "us-east-1"},
		identity: Identity{AccountID: "123456789012", CallerARN: "arn:aws:iam::123456789012:role/operator"},
		tablePages: []TablePage{{
			Tables: []TableSummary{{Name: "COST_AND_USAGE_REPORT"}},
		}},
		table: Table{Name: "COST_AND_USAGE_REPORT", Columns: requiredCUR2Columns()},
		exportPages: []ExportPage{{
			Exports: []ExportSummary{{Name: "matilda-cur2", ExportARN: sourceARN, TableName: "COST_AND_USAGE_REPORT"}},
		}},
		export:       export,
		bucketAccess: BucketAccess{Accessible: true, StatusCode: 200, Region: "us-east-1"},
		bucketPolicy: bucketPolicy(policySpec{
			Service:       "bcm-data-exports.amazonaws.com",
			SourceAccount: export.SourceAccount,
			SourceARN:     export.SourceARN,
			Action:        "s3:PutObject",
			Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
		}),
		executionPages: []ExecutionPage{{
			Executions: []Execution{{ID: "execution-1", StatusObservedAt: fixedNow().Add(-2 * time.Hour)}},
		}},
		execution: Execution{ID: "execution-1", Status: "DELIVERY_SUCCESS", StatusObservedAt: fixedNow().Add(-2 * time.Hour)},
		objectPages: []ObjectPage{{
			Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz"},
		}},
		calls: map[string]int{},
	}
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
}

func requiredCUR2Select() string {
	return strings.Join(requiredCUR2Columns(), ", ")
}

func requiredCUR2ColumnsWithout(excluded string) []string {
	columns := []string{}
	for _, column := range requiredCUR2Columns() {
		if column != excluded {
			columns = append(columns, column)
		}
	}
	return columns
}

type policySpec struct {
	Service       any
	SourceAccount string
	SourceARN     string
	Action        any
	Resource      any
}

func bucketPolicy(spec policySpec) string {
	return bucketPolicyWithConditionOperators(spec, "StringEquals", "ArnLike")
}

func bucketPolicyWithConditionOperators(spec policySpec, accountOperator string, arnOperator string) string {
	conditions := map[string]map[string]string{}
	if spec.SourceAccount != "" {
		conditions[accountOperator] = map[string]string{"aws:SourceAccount": spec.SourceAccount}
	}
	if spec.SourceARN != "" {
		conditions[arnOperator] = map[string]string{"aws:SourceArn": spec.SourceARN}
	}

	policy := policyDocument(statementFor(spec, conditions))

	encoded, err := json.Marshal(policy)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func bucketPolicyWithConditionValueLists(client *fakeClient) string {
	policy := policyDocument(map[string]any{
		"Effect": "Allow",
		"Principal": map[string]any{
			"Service": []string{"bcm-data-exports.amazonaws.com"},
		},
		"Action":   []string{"s3:PutObject"},
		"Resource": []string{"arn:aws:s3:::matilda-cur2-billing/*"},
		"Condition": map[string]any{
			"StringEquals": map[string]any{
				"aws:SourceAccount": []string{"000000000000", client.export.SourceAccount},
			},
			"ArnLike": map[string]any{
				"aws:SourceArn": []string{
					"arn:aws:bcm-data-exports:us-east-1:123456789012:export/other",
					"arn:aws:bcm-data-exports:us-east-1:123456789012:export/*",
				},
			},
		},
	})

	encoded, err := json.Marshal(policy)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func multiStatementBucketPolicy(specs ...policySpec) string {
	statements := []any{}
	for _, spec := range specs {
		conditions := map[string]map[string]string{}
		if spec.SourceAccount != "" {
			conditions["StringEquals"] = map[string]string{"aws:SourceAccount": spec.SourceAccount}
		}
		if spec.SourceARN != "" {
			conditions["ArnLike"] = map[string]string{"aws:SourceArn": spec.SourceARN}
		}
		statements = append(statements, statementFor(spec, conditions))
	}

	policy := map[string]any{
		"Version":   "2012-10-17",
		"Statement": statements,
	}

	encoded, err := json.Marshal(policy)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func policyDocument(statement map[string]any) map[string]any {
	return map[string]any{
		"Version":   "2012-10-17",
		"Statement": []any{statement},
	}
}

func statementFor(spec policySpec, conditions map[string]map[string]string) map[string]any {
	return map[string]any{
		"Effect": "Allow",
		"Principal": map[string]any{
			"Service": spec.Service,
		},
		"Action":    spec.Action,
		"Resource":  spec.Resource,
		"Condition": conditions,
	}
}
