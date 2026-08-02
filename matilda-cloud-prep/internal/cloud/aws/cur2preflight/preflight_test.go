package cur2preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
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
	assertPlanCheckID(t, result, "aws_s3_delivery_policy_ready")
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
		name        string
		err         error
		code        string
		wantMessage string
	}{
		{
			name:        "missing region",
			err:         NewProviderError("aws_config_missing_region", "AWS Region is not configured."),
			code:        "aws_config_missing_region",
			wantMessage: "AWS Region is not configured.",
		},
		{
			name:        "missing credentials",
			err:         NewProviderError("aws_config_missing_credentials", "AWS credentials are not available."),
			code:        "aws_config_missing_credentials",
			wantMessage: "AWS credentials are not available.",
		},
		{
			name:        "configuration timeout",
			err:         NewProviderError("aws_config_timeout", "raw request id from provider"),
			code:        "aws_config_timeout",
			wantMessage: "AWS SDK configuration timed out.",
		},
		{
			name:        "configuration cancelled",
			err:         NewProviderError("aws_config_cancelled", "raw arn:aws:iam::123456789012:role/operator"),
			code:        "aws_config_cancelled",
			wantMessage: "AWS SDK configuration was cancelled.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			client.configErr = tt.err

			result := runPreflight(t, client)

			assertBlockedCode(t, result, tt.code)
			assertPlanCheckMessage(t, result, "AWS SDK configuration", tt.wantMessage)
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
	secondExport.Destination.Output.Compression = "PARQUET"
	secondExport.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"] = "DAILY"
	secondExport.Destination.Region = "us-west-2"
	client.exportsByARN = map[string]Export{
		client.export.ExportARN: client.export,
		secondARN:               secondExport,
	}

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_cur2_export_ambiguous")
	assertGroupedCandidateEvidence(t, result, 1, cur2ExportRef(client.export.ExportARN), "HEALTHY", "TEXT_OR_CSV", "us-east-1")
	assertCheckEvidence(t, result, "candidate_1_compression", "GZIP")
	assertGroupedCandidateEvidence(t, result, 2, cur2ExportRef(secondARN), "WARNING", "PARQUET", "us-west-2")
	assertCheckEvidence(t, result, "candidate_2_compression", "PARQUET")
	assertNoCheckEvidenceKey(t, result, "candidate_export_ref")
	assertNoCheckEvidenceKey(t, result, "candidate_health")
	assertNoCheckEvidenceKey(t, result, "candidate_output_format")
	assertNoCheckEvidenceKey(t, result, "candidate_compression")
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

func TestPreflightOmitsIdentifierLikeCandidateMetadata(t *testing.T) {
	client := baselineClient()
	secondARN := "arn:aws:bcm-data-exports:us-east-1:123456789012:export/finance-cur2"
	client.exportPages = []ExportPage{{Exports: []ExportSummary{
		{Name: "matilda-cur2", ExportARN: client.export.ExportARN, TableName: "COST_AND_USAGE_REPORT"},
		{Name: "finance-cur2", ExportARN: secondARN, TableName: "COST_AND_USAGE_REPORT"},
	}}}
	client.export.HealthStatus = "123456789012"
	client.export.Destination.Output.Format = "AKIAIOSFODNN7EXAMPLE"
	client.export.Destination.Region = "ASIAIOSFODNN7EXAMPLE"
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

func TestSafeEvidenceValueRejectsSensitiveIdentifierShapesWithoutOverblocking(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "safe health value",
			value: "HEALTHY",
			want:  "HEALTHY",
		},
		{
			name:  "safe output format",
			value: "PARQUET",
			want:  "PARQUET",
		},
		{
			name:  "safe region",
			value: "us-west-2",
			want:  "us-west-2",
		},
		{
			name:  "raw account id",
			value: "123456789012",
		},
		{
			name:  "access key id shape",
			value: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "temporary access key id shape",
			value: "ASIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "non account id numeric-looking metadata",
			value: "12345678901A",
			want:  "12345678901A",
		},
		{
			name:  "non access key id metadata",
			value: "AKIAIOSFODNN7EXAMPL_",
			want:  "AKIAIOSFODNN7EXAMPL_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeEvidenceValue(tt.value); got != tt.want {
				t.Fatalf("safeEvidenceValue(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
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
		Keys: []string{
			"finance/cur2/finance-cur2/data/BILLING_PERIOD=2026-06/part-000.gz",
			"finance/cur2/finance-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
		},
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

func TestPreflightAcceptsAWSStandardProductMapForMatildaProductName(t *testing.T) {
	client := baselineClient()
	client.table.Columns = append(requiredCUR2ColumnsWithout("product_product_name"), "product")
	client.export.QueryStatement = "SELECT " + strings.Join(append(requiredCUR2ColumnsWithout("product_product_name"), "product"), ", ") + " FROM COST_AND_USAGE_REPORT"

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	assertCheckEvidence(t, result, "logical_field_source", "product_product_name<-product")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightAcceptsAWSStandardProductMapMetadataWithoutQueryMutation(t *testing.T) {
	client := baselineClient()
	client.table.Columns = append(requiredCUR2ColumnsWithout("product_product_name"), "product")
	client.export.QueryStatement = "SELECT " + strings.Join(requiredCUR2ColumnsWithout("product_product_name"), ", ") + " FROM COST_AND_USAGE_REPORT"

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	assertCheckEvidence(t, result, "logical_field_source", "product_product_name<-product")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightBlocksWhenProductNameLogicalFieldHasNoAWSStandardSource(t *testing.T) {
	client := baselineClient()
	client.table.Columns = requiredCUR2ColumnsWithout("product_product_name")
	client.export.QueryStatement = "SELECT " + strings.Join(requiredCUR2ColumnsWithout("product_product_name"), ", ") + " FROM COST_AND_USAGE_REPORT"

	result := runPreflight(t, client)

	assertBlockedCode(t, result, "aws_cur2_required_fields_missing")
	assertCheckEvidence(t, result, "missing_required_field", "product_product_name")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightAcceptsAWSStandardProductNameMapAlias(t *testing.T) {
	client := baselineClient()
	client.table.Columns = append(requiredCUR2ColumnsWithout("product_product_name"), "product")
	client.export.QueryStatement = "SELECT " + strings.Join(requiredCUR2ColumnsWithout("product_product_name"), ", ") + ", product.product_name AS product_product_name FROM COST_AND_USAGE_REPORT"

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	assertCheckEvidence(t, result, "logical_field_source", "product_product_name<-product.product_name")
	assertNoUnsafeAWSOutput(t, result)
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

func TestPreflightReportsMissingRequiredFieldEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeClient)
		field  string
	}{
		{
			name: "table column missing",
			mutate: func(client *fakeClient) {
				client.table.Columns = requiredCUR2ColumnsWithout("line_item_product_code")
			},
			field: "line_item_product_code",
		},
		{
			name: "query field missing",
			mutate: func(client *fakeClient) {
				client.export.QueryStatement = "SELECT " + strings.Join(requiredCUR2ColumnsWithout("line_item_usage_amount"), ", ") + " FROM COST_AND_USAGE_REPORT"
			},
			field: "line_item_usage_amount",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			tt.mutate(client)

			result := runPreflight(t, client)

			assertBlockedCode(t, result, "aws_cur2_required_fields_missing")
			assertCheckEvidence(t, result, "missing_required_field", tt.field)
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

func TestPreflightAcceptsCUR2ExportWithOmittedTableConfiguration(t *testing.T) {
	client := baselineClient()
	client.export.TableConfigurations = nil
	client.table.Properties = map[string]string{
		"TIME_GRANULARITY":  "MONTHLY",
		"INCLUDE_RESOURCES": "FALSE",
	}

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.Code != "aws_cur2_preflight_ready" {
		t.Fatalf("Code = %q, want aws_cur2_preflight_ready", result.Code)
	}
	if len(client.lastTableProperties) != 0 {
		t.Fatalf("GetTable table properties = %#v, want empty AWS-default request", client.lastTableProperties)
	}
	assertPlanCheckMessage(t, result, "CUR 2.0 table configuration", "AWS export omits COST_AND_USAGE_REPORT table configuration. AWS table-property defaults are used for read-only validation.")
	assertPlanCheckMessage(t, result, "CUR 2.0 time granularity", "CUR 2.0 export uses monthly time granularity.")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightWarnsWhenDefaultedCUR2GranularityCannotBeConfirmed(t *testing.T) {
	client := baselineClient()
	client.export.TableConfigurations = map[string]map[string]string{}
	client.table.Properties = nil

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.Code != "aws_cur2_time_granularity_unverified" {
		t.Fatalf("Code = %q, want aws_cur2_time_granularity_unverified", result.Code)
	}
	if len(client.lastTableProperties) != 0 {
		t.Fatalf("GetTable table properties = %#v, want empty AWS-default request", client.lastTableProperties)
	}
	assertPlanCheckMessage(t, result, "CUR 2.0 time granularity", "CUR 2.0 time granularity could not be confirmed from the export or returned table metadata. AWS table properties are optional and defaulted, so this is a warning, not an invalid CUR 2.0 export.")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightWarnsForValidNonPreferredCUR2Granularity(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		wantMessage string
	}{
		{
			name:        "daily granularity",
			granularity: "DAILY",
			wantMessage: "CUR 2.0 export uses daily time granularity. This is valid AWS CUR 2.0, but monthly is preferred for Matilda Rapid Assessment - Billing Based.",
		},
		{
			name:        "hourly granularity",
			granularity: "HOURLY",
			wantMessage: "CUR 2.0 export uses hourly time granularity. This is valid AWS CUR 2.0, but monthly is preferred and hourly exports can increase file volume.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := baselineClient()
			client.export.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"] = tt.granularity

			result := runPreflight(t, client)

			if result.Status != workflow.StatusReady {
				t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
			}
			if result.Code != "aws_cur2_time_granularity_not_preferred" {
				t.Fatalf("Code = %q, want aws_cur2_time_granularity_not_preferred", result.Code)
			}
			assertPlanCheckMessage(t, result, "CUR 2.0 time granularity", tt.wantMessage)
			assertNoUnsafeAWSOutput(t, result)
		})
	}
}

func TestPreflightAcceptsMatildaSupportedParquetCUR2Output(t *testing.T) {
	client := baselineClient()
	client.export.Destination.Output.Format = "PARQUET"
	client.export.Destination.Output.Compression = "PARQUET"
	client.objectPages = []ObjectPage{{
		Keys: []string{
			"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.snappy.parquet",
			"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
		},
	}}

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.Code != "aws_cur2_preflight_ready" {
		t.Fatalf("Code = %q, want aws_cur2_preflight_ready", result.Code)
	}
	assertPlanCheckMessage(t, result, "CUR 2.0 output format", "CUR 2.0 export uses PARQUET output. AWS supports PARQUET and Matilda can use it for this path.")
	assertPlanCheckMessage(t, result, "CUR 2.0 compression", "CUR 2.0 export uses PARQUET compression for the supported PARQUET output shape.")
	assertCheckEvidence(t, result, "output_format", "PARQUET")
	assertCheckEvidence(t, result, "compression", "PARQUET")
	assertCheckEvidence(t, result, "matilda_format_support", "supported")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightAcceptsAWSValidOverwriteCUR2Output(t *testing.T) {
	client := baselineClient()
	client.export.Destination.Output.Overwrite = "OVERWRITE_REPORT"

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.Code != "aws_cur2_preflight_ready" {
		t.Fatalf("Code = %q, want aws_cur2_preflight_ready", result.Code)
	}
	assertPlanCheckMessage(t, result, "CUR 2.0 overwrite setting", "CUR 2.0 export overwrites report files. AWS supports OVERWRITE_REPORT for CUR 2.0 exports; CREATE_NEW_REPORT remains preferred for tool-created exports.")
	assertCheckEvidence(t, result, "overwrite", "OVERWRITE_REPORT")
	assertCheckEvidence(t, result, "matilda_output_preference", "supported_not_preferred")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightAcceptsExistingDailyParquetOverwriteCUR2Output(t *testing.T) {
	client := baselineClient()
	client.export.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"] = "DAILY"
	client.export.Destination.Output.Format = "PARQUET"
	client.export.Destination.Output.Compression = "PARQUET"
	client.export.Destination.Output.Overwrite = "OVERWRITE_REPORT"
	client.objectPages = []ObjectPage{{
		Keys: []string{
			"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.snappy.parquet",
			"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
		},
	}}

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	if result.Code != "aws_cur2_time_granularity_not_preferred" {
		t.Fatalf("Code = %q, want aws_cur2_time_granularity_not_preferred", result.Code)
	}
	assertPlanCheckMessage(t, result, "CUR 2.0 time granularity", "CUR 2.0 export uses daily time granularity. This is valid AWS CUR 2.0, but monthly is preferred for Matilda Rapid Assessment - Billing Based.")
	assertPlanCheckMessage(t, result, "CUR 2.0 output format", "CUR 2.0 export uses PARQUET output. AWS supports PARQUET and Matilda can use it for this path.")
	assertPlanCheckMessage(t, result, "CUR 2.0 compression", "CUR 2.0 export uses PARQUET compression for the supported PARQUET output shape.")
	assertPlanCheckMessage(t, result, "CUR 2.0 overwrite setting", "CUR 2.0 export overwrites report files. AWS supports OVERWRITE_REPORT for CUR 2.0 exports; CREATE_NEW_REPORT remains preferred for tool-created exports.")
	assertCheckEvidence(t, result, "time_granularity", "DAILY")
	assertCheckEvidence(t, result, "output_format", "PARQUET")
	assertCheckEvidence(t, result, "compression", "PARQUET")
	assertCheckEvidence(t, result, "overwrite", "OVERWRITE_REPORT")
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightBlocksOutputSettingDeviations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Export)
		code   string
	}{
		{
			name: "unknown granularity",
			mutate: func(export *Export) {
				export.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"] = "WEEKLY"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "unknown format",
			mutate: func(export *Export) {
				export.Destination.Output.Format = "JSON"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "unknown compression",
			mutate: func(export *Export) {
				export.Destination.Output.Compression = "ZIP"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "mismatched parquet format with gzip compression",
			mutate: func(export *Export) {
				export.Destination.Output.Format = "PARQUET"
				export.Destination.Output.Compression = "GZIP"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "mismatched text format with parquet compression",
			mutate: func(export *Export) {
				export.Destination.Output.Format = "TEXT_OR_CSV"
				export.Destination.Output.Compression = "PARQUET"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "unknown overwrite setting",
			mutate: func(export *Export) {
				export.Destination.Output.Overwrite = "APPEND_REPORT"
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
			name: "athena output type not verified for this path",
			mutate: func(export *Export) {
				export.Destination.Output.OutputType = "ATHENA"
			},
			code: "aws_cur2_output_settings_blocked",
		},
		{
			name: "redshift output type not verified for this path",
			mutate: func(export *Export) {
				export.Destination.Output.OutputType = "REDSHIFT"
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
		evidence   string
		wantListed bool
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
			name: "missing data exports principal warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "service_principal_missing",
			wantListed: true,
		},
		{
			name: "padded data exports principal warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       " bcm-data-exports.amazonaws.com ",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "service_principal_missing",
			wantListed: true,
		},
		{
			name: "missing source account warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:   "bcm-data-exports.amazonaws.com",
					SourceARN: client.export.SourceARN,
					Action:    "s3:PutObject",
					Resource:  "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "missing source arn warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_arn_condition_missing",
			wantListed: true,
		},
		{
			name: "wrong source account warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "wrong source arn warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "source_arn_condition_missing",
			wantListed: true,
		},
		{
			name: "missing put object action warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_action_missing",
			wantListed: true,
		},
		{
			name: "padded put object action warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        " s3:PutObject ",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_action_missing",
			wantListed: true,
		},
		{
			name: "wildcard s3 action warns as missing put object action when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:*",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_action_missing",
			wantListed: true,
		},
		{
			name: "global wildcard action warns as missing put object action when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "*",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_action_missing",
			wantListed: true,
		},
		{
			name: "wrong policy resource warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_resource_not_covered",
			wantListed: true,
		},
		{
			name: "actual export name scoped resource covers destination",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/matilda-cur2/*",
				})
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
			wantListed: true,
		},
		{
			name: "different export name scoped resource warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/other-export/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_resource_not_covered",
			wantListed: true,
		},
		{
			name: "padded policy resource warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      " arn:aws:s3:::matilda-cur2-billing/matilda/cur2/* ",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_resource_not_covered",
			wantListed: true,
		},
		{
			name: "global bucket wildcard resource warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_resource_not_covered",
			wantListed: true,
		},
		{
			name: "bucket name wildcard resource warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-*/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_resource_not_covered",
			wantListed: true,
		},
		{
			name: "negated source account condition warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "negated source arn condition warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "source_arn_condition_missing",
			wantListed: true,
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
			wantListed: true,
		},
		{
			name: "source partition s3 resource covers destination",
			mutate: func(client *fakeClient) {
				client.export.SourceARN = "arn:aws-us-gov:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2"
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws-us-gov:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
			wantListed: true,
		},
		{
			name: "mismatched s3 resource partition warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.export.SourceARN = "arn:aws-us-gov:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2"
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "put_object_resource_not_covered",
			wantListed: true,
		},
		{
			name: "single statement bucket policy document shape",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = singleStatementBucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     "arn:aws:bcm-data-exports:us-east-1:123456789012:export/*",
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/*",
				})
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
			wantListed: true,
		},
		{
			name: "documented bucket policy with condition value lists",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionValueLists(client)
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
			wantListed: true,
		},
		{
			name: "exact stringlike source account condition is accepted",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringLike", "ArnLike")
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
			wantListed: true,
		},
		{
			name: "exact stringlike source account reaches backfill when previous month is missing",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringLike", "ArnLike")
				client.objectPages = []ObjectPage{{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-07/part-000.gz"}}}
			},
			code:       "aws_backfill_manual_step_required",
			wantStatus: workflow.RunStatusManualSteps,
			wantListed: true,
		},
		{
			name: "wildcard stringlike source account warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: "1234567890*",
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringLike", "ArnLike")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "question stringlike source account warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: "12345678901?",
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringLike", "ArnLike")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "wrong stringlike source account warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: "999999999999",
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringLike", "ArnLike")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "whitespace stringequals source account warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: " " + client.export.SourceAccount + " ",
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "whitespace stringlike source account warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithConditionOperators(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: " " + client.export.SourceAccount + " ",
					SourceARN:     client.export.SourceARN,
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				}, "StringLike", "ArnLike")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "stringlike source account condition value lists accept exact non wildcard match",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithStringLikeSourceAccountValueList(client)
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
			wantListed: true,
		},
		{
			name: "stringlike source account value list warns for mixed wildcard when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithStringLikeSourceAccountValues(client, client.export.SourceAccount, "1234567890*")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "stringlike source account value list warns for padded account when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithStringLikeSourceAccountValues(client, client.export.SourceAccount, " "+client.export.SourceAccount+" ")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "stringlike source account value list warns for mixed wrong account when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithStringLikeSourceAccountValues(client, client.export.SourceAccount, "999999999999")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "stringequals source account does not override wildcard stringlike and warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithStringEqualsAndStringLikeSourceAccounts(client, "1234567890*")
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "safe stringlike source account does not override wrong stringequals and warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicyWithWrongStringEqualsAndStringLikeSourceAccounts(client, client.export.SourceAccount)
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "global source arn wildcard warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "source_arn_condition_missing",
			wantListed: true,
		},
		{
			name: "padded source arn warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:       "bcm-data-exports.amazonaws.com",
					SourceAccount: client.export.SourceAccount,
					SourceARN:     " " + client.export.SourceARN + " ",
					Action:        "s3:PutObject",
					Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.StatusReady,
			evidence:   "source_arn_condition_missing",
			wantListed: true,
		},
		{
			name: "exact source arn operator warns when previous month is present",
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
			wantStatus: workflow.StatusReady,
			evidence:   "source_arn_condition_missing",
			wantListed: true,
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
			wantListed: true,
		},
		{
			name: "unrelated wildcard principal statement does not block later valid policy",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = multiStatementBucketPolicyWithRaw(
					map[string]any{
						"Effect":    "Allow",
						"Principal": "*",
						"Action":    "s3:GetObject",
						"Resource":  "arn:aws:s3:::matilda-cur2-billing/public/*",
					},
					statementFor(policySpec{
						Service:       "bcm-data-exports.amazonaws.com",
						SourceAccount: client.export.SourceAccount,
						SourceARN:     client.export.SourceARN,
						Action:        "s3:PutObject",
						Resource:      "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
					}, map[string]map[string]string{
						"StringEquals": {"aws:SourceAccount": client.export.SourceAccount},
						"ArnLike":      {"aws:SourceArn": client.export.SourceARN},
					}),
				)
			},
			code:       "aws_cur2_preflight_ready",
			wantStatus: workflow.StatusReady,
			wantListed: true,
		},
		{
			name: "policy inaccessible warns when previous month is present",
			mutate: func(client *fakeClient) {
				client.bucketPolicyErr = NewProviderError("aws_s3_bucket_policy_inaccessible", "bucket policy cannot be inspected.")
			},
			code:       "aws_s3_bucket_policy_inaccessible",
			wantStatus: workflow.StatusReady,
			wantListed: true,
		},
		{
			name: "previous month missing requires manual backfill",
			mutate: func(client *fakeClient) {
				client.objectPages = []ObjectPage{{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-07/part-000.gz"}}}
			},
			code:       "aws_backfill_manual_step_required",
			wantStatus: workflow.RunStatusManualSteps,
			wantListed: true,
		},
		{
			name: "previous month manifest missing requires manual backfill",
			mutate: func(client *fakeClient) {
				client.objectPages = []ObjectPage{{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz"}}}
			},
			code:       "aws_backfill_manual_step_required",
			wantStatus: workflow.RunStatusManualSteps,
			wantListed: true,
		},
		{
			name: "missing source account blocks when previous month is missing",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:   "bcm-data-exports.amazonaws.com",
					SourceARN: client.export.SourceARN,
					Action:    "s3:PutObject",
					Resource:  "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
				client.objectPages = []ObjectPage{{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-07/part-000.gz"}}}
			},
			code:       "aws_s3_delivery_policy_missing",
			wantStatus: workflow.RunStatusBlocked,
			evidence:   "source_account_condition_missing",
			wantListed: true,
		},
		{
			name: "policy inaccessible blocks when previous month is missing",
			mutate: func(client *fakeClient) {
				client.bucketPolicyErr = NewProviderError("aws_s3_bucket_policy_inaccessible", "bucket policy cannot be inspected.")
				client.objectPages = []ObjectPage{{Keys: []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-07/part-000.gz"}}}
			},
			code:       "aws_s3_bucket_policy_inaccessible",
			wantStatus: workflow.RunStatusBlocked,
			wantListed: true,
		},
		{
			name: "list objects failure blocks before policy severity",
			mutate: func(client *fakeClient) {
				client.bucketPolicy = bucketPolicy(policySpec{
					Service:   "bcm-data-exports.amazonaws.com",
					SourceARN: client.export.SourceARN,
					Action:    "s3:PutObject",
					Resource:  "arn:aws:s3:::matilda-cur2-billing/matilda/cur2/*",
				})
				client.objectErr = NewProviderError("aws_s3_bucket_inaccessible", "object list denied.")
			},
			code:       "aws_s3_bucket_inaccessible",
			wantStatus: workflow.RunStatusBlocked,
			wantListed: true,
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
			if tt.evidence != "" {
				assertCheckEvidence(t, result, "policy_gap", tt.evidence)
			}
			if tt.wantStatus == workflow.StatusReady && (tt.code == "aws_s3_delivery_policy_missing" || tt.code == "aws_s3_bucket_policy_inaccessible") {
				assertPlanCheckStatus(t, result, tt.code, workflow.CheckWarn)
			}
			if tt.wantListed && client.calls["ListObjects"] == 0 {
				t.Fatal("ListObjects should run before policy severity is finalized")
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
			Keys: []string{
				"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz",
				"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
			},
		},
	}

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q", result.Status, workflow.StatusReady)
	}
	if client.calls["ListExports"] != 2 {
		t.Fatalf("ListExports calls = %d, want 2", client.calls["ListExports"])
	}
	if client.calls["ListObjects"] != 4 {
		t.Fatalf("ListObjects calls = %d, want 4", client.calls["ListObjects"])
	}
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightListsExactPreviousMonthPrefixes(t *testing.T) {
	client := baselineClient()

	result := runPreflight(t, client)

	if result.Status != workflow.StatusReady {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.StatusReady, result.Code)
	}
	assertObjectListPrefix(t, client, 0, previousMonthDataPrefix(client.export, "2026-06"))
	assertObjectListPrefix(t, client, 1, previousMonthManifestPrefix(client.export, "2026-06"))
	assertNoUnsafeAWSOutput(t, result)
}

func TestPreflightDoesNotTreatPreviousMonthFolderMarkerAsData(t *testing.T) {
	client := baselineClient()
	dataPrefix := previousMonthDataPrefix(client.export, "2026-06")
	manifestPrefix := previousMonthManifestPrefix(client.export, "2026-06")
	client.objectPages = []ObjectPage{{Keys: []string{
		dataPrefix,
		manifestPrefix + "Manifest.json",
	}}}

	result := runPreflight(t, client)

	if result.Status != workflow.RunStatusManualSteps {
		t.Fatalf("Status = %q, want %q; code=%q", result.Status, workflow.RunStatusManualSteps, result.Code)
	}
	if result.Code != "aws_backfill_manual_step_required" {
		t.Fatalf("Code = %q, want aws_backfill_manual_step_required", result.Code)
	}
	assertCheckEvidence(t, result, "missing_previous_month_component", "data_partition")
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
			keys: []string{
				"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz",
				"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
			},
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
	if result.Message != "AWS CUR 2.0 billing preflight could not prove previous-month billing data availability." {
		t.Fatalf("Message = %q, want specific previous-month blocker message", result.Message)
	}
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

func assertPlanCheckMessage(t *testing.T, result workflow.Result, title string, message string) {
	t.Helper()

	if result.Plan == nil {
		t.Fatal("result.Plan is nil")
	}
	for _, check := range result.Plan.Checks {
		if check.Title == title {
			if check.Message != message {
				t.Fatalf("check %q message = %q, want %q", title, check.Message, message)
			}
			return
		}
	}
	t.Fatalf("did not find plan check titled %q", title)
}

func assertPlanCheckID(t *testing.T, result workflow.Result, id string) {
	t.Helper()

	if result.Plan == nil {
		t.Fatal("result.Plan is nil")
	}
	for _, check := range result.Plan.Checks {
		if check.ID == id {
			return
		}
	}
	t.Fatalf("did not find plan check id %q", id)
}

func assertPlanCheckStatus(t *testing.T, result workflow.Result, id string, status workflow.CheckStatus) {
	t.Helper()

	if result.Plan == nil {
		t.Fatal("result.Plan is nil")
	}
	for _, check := range result.Plan.Checks {
		if check.ID == id {
			if check.Status != status {
				t.Fatalf("check %q status = %q, want %q", id, check.Status, status)
			}
			return
		}
	}
	t.Fatalf("did not find plan check id %q", id)
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
		"matilda-cur2-billing",
		"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/",
		"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/",
		"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz",
		"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
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

func assertObjectListPrefix(t *testing.T, client *fakeClient, index int, want string) {
	t.Helper()

	if index >= len(client.objectRequests) {
		t.Fatalf("object list request %d missing from %#v", index, client.objectRequests)
	}
	if got := client.objectRequests[index].prefix; got != want {
		t.Fatalf("object list request %d prefix = %q, want %q", index, got, want)
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
	objectRequests       []objectListRequest
	objectErr            error
	lastTableProperties  map[string]string
	getTableAfterExport  bool
	calls                map[string]int
}

type objectListRequest struct {
	bucket  string
	prefix  string
	token   string
	maxKeys int32
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
	f.objectRequests = append(f.objectRequests, objectListRequest{bucket: bucket, prefix: prefix, token: token, maxKeys: maxKeys})
	index := pageIndex(token)
	if index >= len(f.objectPages) {
		return ObjectPage{}, nil
	}
	page := f.objectPages[index]
	page.Keys = filterKeysByPrefix(page.Keys, prefix)
	return page, f.objectErr
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
	lastDash := strings.LastIndex(token, "-")
	if lastDash >= 0 && lastDash+1 < len(token) {
		if index, err := strconv.Atoi(token[lastDash+1:]); err == nil && index > 0 {
			return index
		}
	}
	if strings.Contains(token, "next") {
		return 1
	}
	return 0
}

func filterKeysByPrefix(keys []string, prefix string) []string {
	filtered := []string{}
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			filtered = append(filtered, key)
		}
	}
	return filtered
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
			Keys: []string{
				"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz",
				"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
			},
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
		if conditions[accountOperator] == nil {
			conditions[accountOperator] = map[string]string{}
		}
		conditions[accountOperator]["aws:SourceAccount"] = spec.SourceAccount
	}
	if spec.SourceARN != "" {
		if conditions[arnOperator] == nil {
			conditions[arnOperator] = map[string]string{}
		}
		conditions[arnOperator]["aws:SourceArn"] = spec.SourceARN
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

func bucketPolicyWithStringLikeSourceAccountValueList(client *fakeClient) string {
	return bucketPolicyWithStringLikeSourceAccountValues(client, client.export.SourceAccount, client.export.SourceAccount)
}

func bucketPolicyWithStringLikeSourceAccountValues(client *fakeClient, sourceAccounts ...string) string {
	return bucketPolicyWithSourceAccountConditions(client, map[string]any{
		"StringLike": map[string]any{
			"aws:SourceAccount": sourceAccounts,
		},
	})
}

func bucketPolicyWithStringEqualsAndStringLikeSourceAccounts(client *fakeClient, sourceAccounts ...string) string {
	return bucketPolicyWithSourceAccountConditions(client, map[string]any{
		"StringEquals": map[string]any{
			"aws:SourceAccount": client.export.SourceAccount,
		},
		"StringLike": map[string]any{
			"aws:SourceAccount": sourceAccounts,
		},
	})
}

func bucketPolicyWithWrongStringEqualsAndStringLikeSourceAccounts(client *fakeClient, sourceAccounts ...string) string {
	return bucketPolicyWithSourceAccountConditions(client, map[string]any{
		"StringEquals": map[string]any{
			"aws:SourceAccount": "999999999999",
		},
		"StringLike": map[string]any{
			"aws:SourceAccount": sourceAccounts,
		},
	})
}

func bucketPolicyWithSourceAccountConditions(client *fakeClient, accountConditions map[string]any) string {
	conditions := map[string]any{}
	for key, value := range accountConditions {
		conditions[key] = value
	}
	conditions["ArnLike"] = map[string]any{
		"aws:SourceArn": []string{
			"arn:aws:bcm-data-exports:us-east-1:123456789012:export/other",
			"arn:aws:bcm-data-exports:us-east-1:123456789012:export/*",
		},
	}

	policy := policyDocument(map[string]any{
		"Effect": "Allow",
		"Principal": map[string]any{
			"Service": []string{"bcm-data-exports.amazonaws.com"},
		},
		"Action":    []string{"s3:PutObject"},
		"Resource":  []string{"arn:aws:s3:::matilda-cur2-billing/*"},
		"Condition": conditions,
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

func multiStatementBucketPolicyWithRaw(statements ...map[string]any) string {
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

func singleStatementBucketPolicy(spec policySpec) string {
	conditions := map[string]map[string]string{}
	if spec.SourceAccount != "" {
		conditions["StringEquals"] = map[string]string{"aws:SourceAccount": spec.SourceAccount}
	}
	if spec.SourceARN != "" {
		conditions["ArnLike"] = map[string]string{"aws:SourceArn": spec.SourceARN}
	}
	policy := map[string]any{
		"Version":   "2012-10-17",
		"Statement": statementFor(spec, conditions),
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		panic(err)
	}
	return string(encoded)
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
