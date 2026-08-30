package awsclient

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsbcm "github.com/aws/aws-sdk-go-v2/service/bcmdataexports"
	bcmtypes "github.com/aws/aws-sdk-go-v2/service/bcmdataexports/types"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestIdentityAndDataExportsMapping(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	stsClient := &fakeSTS{
		output: &awssts.GetCallerIdentityOutput{
			Account: aws.String("123456789012"),
			Arn:     aws.String("arn:aws:iam::123456789012:role/operator"),
		},
	}
	data := &fakeDataExports{
		listTablesOutput: &awsbcm.ListTablesOutput{
			Tables:    []bcmtypes.Table{{TableName: aws.String("COST_AND_USAGE_REPORT")}},
			NextToken: aws.String("table-token"),
		},
		getTableOutput: &awsbcm.GetTableOutput{
			TableName: aws.String("COST_AND_USAGE_REPORT"),
			TableProperties: map[string]string{
				"TIME_GRANULARITY":  "MONTHLY",
				"INCLUDE_RESOURCES": "FALSE",
			},
			Schema: []bcmtypes.Column{
				{Name: aws.String("line_item_usage_amount")},
				{Name: aws.String("line_item_unblended_cost")},
			},
		},
		listExportsOutput: &awsbcm.ListExportsOutput{
			Exports: []bcmtypes.ExportReference{{
				ExportName: aws.String("matilda-cur2"),
				ExportArn:  aws.String("arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2"),
				ExportStatus: &bcmtypes.ExportStatus{
					StatusCode: bcmtypes.ExportStatusCodeHealthy,
					CreatedAt:  aws.Time(now.Add(-48 * time.Hour)),
				},
			}},
			NextToken: aws.String("export-token"),
		},
		getExportOutput: &awsbcm.GetExportOutput{
			Export: &bcmtypes.Export{
				Name:      aws.String("matilda-cur2"),
				ExportArn: aws.String("arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2"),
				DataQuery: &bcmtypes.DataQuery{
					QueryStatement: aws.String("SELECT line_item_usage_amount FROM COST_AND_USAGE_REPORT"),
					TableConfigurations: map[string]map[string]string{
						"COST_AND_USAGE_REPORT": {"TIME_GRANULARITY": "MONTHLY"},
					},
				},
				DestinationConfigurations: &bcmtypes.DestinationConfigurations{
					S3Destination: &bcmtypes.S3Destination{
						S3Bucket:      aws.String("matilda-cur2-billing"),
						S3BucketOwner: aws.String("123456789012"),
						S3Prefix:      aws.String("matilda/cur2"),
						S3Region:      aws.String("us-east-1"),
						S3OutputConfigurations: &bcmtypes.S3OutputConfigurations{
							Format:      bcmtypes.FormatOptionTextOrCsv,
							Compression: bcmtypes.CompressionOptionGzip,
							Overwrite:   bcmtypes.OverwriteOptionCreateNewReport,
							OutputType:  bcmtypes.S3OutputTypeCustom,
						},
					},
				},
				RefreshCadence: &bcmtypes.RefreshCadence{Frequency: bcmtypes.FrequencyOptionSynchronous},
			},
			ExportStatus: &bcmtypes.ExportStatus{
				CreatedAt:  aws.Time(now.Add(-48 * time.Hour)),
				StatusCode: bcmtypes.ExportStatusCodeHealthy,
			},
		},
		listExecutionsOutput: &awsbcm.ListExecutionsOutput{
			Executions: []bcmtypes.ExecutionReference{{
				ExecutionId: aws.String("execution-1"),
				ExecutionStatus: &bcmtypes.ExecutionStatus{
					CreatedAt:     aws.Time(now.Add(-2 * time.Hour)),
					CompletedAt:   aws.Time(now.Add(-90 * time.Minute)),
					LastUpdatedAt: aws.Time(now.Add(-80 * time.Minute)),
					StatusCode:    bcmtypes.ExecutionStatusCodeDeliverySuccess,
				},
			}},
			NextToken: aws.String("execution-token"),
		},
		getExecutionOutput: &awsbcm.GetExecutionOutput{
			ExecutionId: aws.String("execution-1"),
			ExecutionStatus: &bcmtypes.ExecutionStatus{
				CreatedAt:     aws.Time(now.Add(-2 * time.Hour)),
				CompletedAt:   aws.Time(now.Add(-90 * time.Minute)),
				LastUpdatedAt: aws.Time(now.Add(-80 * time.Minute)),
				StatusCode:    bcmtypes.ExecutionStatusCodeDeliverySuccess,
			},
		},
	}
	client := readyClient(t, stsClient, data, &fakeS3{})

	identity, err := client.GetCallerIdentity(context.Background())
	if err != nil {
		t.Fatalf("GetCallerIdentity returned error: %v", err)
	}
	if identity != (cur2preflight.Identity{AccountID: "123456789012", CallerARN: "arn:aws:iam::123456789012:role/operator"}) {
		t.Fatalf("identity = %#v, want mapped account and ARN", identity)
	}

	tables, err := client.ListTables(context.Background(), "table-input-token")
	if err != nil {
		t.Fatalf("ListTables returned error: %v", err)
	}
	if !reflect.DeepEqual(data.listTablesInputs[0], &awsbcm.ListTablesInput{NextToken: aws.String("table-input-token")}) {
		t.Fatalf("ListTables input = %#v", data.listTablesInputs[0])
	}
	if tables.NextToken != "table-token" || len(tables.Tables) != 1 || tables.Tables[0].Name != "COST_AND_USAGE_REPORT" {
		t.Fatalf("tables = %#v, want mapped table page", tables)
	}

	table, err := client.GetTable(context.Background(), "COST_AND_USAGE_REPORT", map[string]string{"TIME_GRANULARITY": "MONTHLY"})
	if err != nil {
		t.Fatalf("GetTable returned error: %v", err)
	}
	if data.getTableInputs[0].TableName == nil || *data.getTableInputs[0].TableName != "COST_AND_USAGE_REPORT" {
		t.Fatalf("GetTable table name input = %#v", data.getTableInputs[0].TableName)
	}
	if data.getTableInputs[0].TableProperties["TIME_GRANULARITY"] != "MONTHLY" {
		t.Fatalf("GetTable properties = %#v", data.getTableInputs[0].TableProperties)
	}
	if !reflect.DeepEqual(table.Columns, []string{"line_item_usage_amount", "line_item_unblended_cost"}) {
		t.Fatalf("table columns = %#v", table.Columns)
	}
	if !reflect.DeepEqual(table.Properties, map[string]string{"TIME_GRANULARITY": "MONTHLY", "INCLUDE_RESOURCES": "FALSE"}) {
		t.Fatalf("table properties = %#v", table.Properties)
	}

	exports, err := client.ListExports(context.Background(), "export-input-token")
	if err != nil {
		t.Fatalf("ListExports returned error: %v", err)
	}
	if exports.NextToken != "export-token" || len(exports.Exports) != 1 {
		t.Fatalf("exports = %#v, want mapped summary page", exports)
	}
	if exports.Exports[0].TableName != "" || exports.Exports[0].SourceType != "" {
		t.Fatalf("ListExports summary should not invent table metadata: %#v", exports.Exports[0])
	}

	export, err := client.GetExport(context.Background(), "arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2")
	if err != nil {
		t.Fatalf("GetExport returned error: %v", err)
	}
	if export.Name != "matilda-cur2" || export.Destination.Bucket != "matilda-cur2-billing" || export.Destination.BucketOwner != "123456789012" {
		t.Fatalf("export = %#v, want mapped export detail", export)
	}
	if export.Destination.Output.Format != "TEXT_OR_CSV" || export.RefreshCadence != "SYNCHRONOUS" || export.HealthStatus != "HEALTHY" {
		t.Fatalf("export output/status = %#v", export)
	}

	executions, err := client.ListExecutions(context.Background(), export.ExportARN, "execution-input-token")
	if err != nil {
		t.Fatalf("ListExecutions returned error: %v", err)
	}
	if executions.NextToken != "execution-token" || len(executions.Executions) != 1 {
		t.Fatalf("executions = %#v, want mapped execution page", executions)
	}
	if executions.Executions[0].Status != "DELIVERY_SUCCESS" || !executions.Executions[0].StatusObservedAt.Equal(now.Add(-80*time.Minute)) {
		t.Fatalf("execution = %#v, want status and LastUpdatedAt observation", executions.Executions[0])
	}

	execution, err := client.GetExecution(context.Background(), export.ExportARN, "execution-1")
	if err != nil {
		t.Fatalf("GetExecution returned error: %v", err)
	}
	if execution.ID != "execution-1" || execution.Status != "DELIVERY_SUCCESS" || !execution.StatusObservedAt.Equal(now.Add(-80*time.Minute)) {
		t.Fatalf("execution detail = %#v, want mapped detail", execution)
	}
}

func TestGetTableFailsClosedOnMalformedTableMetadata(t *testing.T) {
	tests := []struct {
		name      string
		tableName *string
		schema    []bcmtypes.Column
	}{
		{
			name:      "nil returned table name",
			tableName: nil,
			schema:    []bcmtypes.Column{{Name: aws.String("line_item_usage_amount")}},
		},
		{
			name:      "unexpected returned table name",
			tableName: aws.String("OTHER_TABLE"),
			schema:    []bcmtypes.Column{{Name: aws.String("line_item_usage_amount")}},
		},
		{
			name:      "nil column name",
			tableName: aws.String("COST_AND_USAGE_REPORT"),
			schema:    []bcmtypes.Column{{Name: nil}},
		},
		{
			name:      "empty column name",
			tableName: aws.String("COST_AND_USAGE_REPORT"),
			schema:    []bcmtypes.Column{{Name: aws.String("")}},
		},
		{
			name:      "whitespace padded column name",
			tableName: aws.String("COST_AND_USAGE_REPORT"),
			schema:    []bcmtypes.Column{{Name: aws.String(" line_item_usage_amount")}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := readyClient(t, &fakeSTS{}, &fakeDataExports{
				getTableOutput: &awsbcm.GetTableOutput{
					TableName: tt.tableName,
					Schema:    tt.schema,
				},
			}, &fakeS3{})

			table, err := client.GetTable(context.Background(), "COST_AND_USAGE_REPORT", nil)

			assertProviderCode(t, err, "aws_cur2_table_invalid_shape")
			if len(table.Columns) != 0 {
				t.Fatalf("table columns = %#v, want no mapped columns on malformed schema", table.Columns)
			}
		})
	}
}

func TestIdentityNilOutputMapsToEmptyIdentity(t *testing.T) {
	client := readyClient(t, &fakeSTS{output: nil}, &fakeDataExports{}, &fakeS3{})

	identity, err := client.GetCallerIdentity(context.Background())

	if err != nil {
		t.Fatalf("GetCallerIdentity returned error: %v", err)
	}
	if identity.AccountID != "" || identity.CallerARN != "" {
		t.Fatalf("identity = %#v, want empty identity", identity)
	}
}

func TestDataExportsEmptyPagesAndTimestampFallbacks(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	data := &fakeDataExports{
		listTablesOutput:     &awsbcm.ListTablesOutput{},
		listExportsOutput:    &awsbcm.ListExportsOutput{},
		listExecutionsOutput: &awsbcm.ListExecutionsOutput{},
		getExecutionOutput: &awsbcm.GetExecutionOutput{
			ExecutionId: aws.String("execution-created"),
			ExecutionStatus: &bcmtypes.ExecutionStatus{
				CreatedAt:  aws.Time(now.Add(-4 * time.Hour)),
				StatusCode: bcmtypes.ExecutionStatusCodeDeliveryInProcess,
			},
		},
	}
	client := readyClient(t, &fakeSTS{}, data, &fakeS3{})

	tables, err := client.ListTables(context.Background(), "")
	if err != nil {
		t.Fatalf("ListTables returned error: %v", err)
	}
	if len(tables.Tables) != 0 || tables.NextToken != "" || data.listTablesInputs[0].NextToken != nil {
		t.Fatalf("tables or input token = %#v / %#v", tables, data.listTablesInputs[0])
	}

	exports, err := client.ListExports(context.Background(), "")
	if err != nil {
		t.Fatalf("ListExports returned error: %v", err)
	}
	if len(exports.Exports) != 0 || exports.NextToken != "" || data.listExportsInputs[0].NextToken != nil {
		t.Fatalf("exports or input token = %#v / %#v", exports, data.listExportsInputs[0])
	}

	executions, err := client.ListExecutions(context.Background(), "export-arn", "")
	if err != nil {
		t.Fatalf("ListExecutions returned error: %v", err)
	}
	if len(executions.Executions) != 0 || executions.NextToken != "" || data.listExecutionsInputs[0].NextToken != nil {
		t.Fatalf("executions or input token = %#v / %#v", executions, data.listExecutionsInputs[0])
	}

	execution, err := client.GetExecution(context.Background(), "export-arn", "execution-created")
	if err != nil {
		t.Fatalf("GetExecution returned error: %v", err)
	}
	if execution.Status != "DELIVERY_IN_PROCESS" || !execution.StatusObservedAt.Equal(now.Add(-4*time.Hour)) {
		t.Fatalf("execution = %#v, want CreatedAt fallback", execution)
	}

	completedOnly := &bcmtypes.ExecutionStatus{
		CompletedAt: aws.Time(now.Add(-2 * time.Hour)),
		StatusCode:  bcmtypes.ExecutionStatusCodeDeliverySuccess,
	}
	if !statusObservationTime(completedOnly).Equal(now.Add(-2 * time.Hour)) {
		t.Fatalf("CompletedAt fallback failed")
	}
	if statusCodeString(nil) != "" || !statusObservationTime(nil).IsZero() {
		t.Fatal("nil execution status should map to empty status and zero time")
	}
	if !statusObservationTime(&bcmtypes.ExecutionStatus{}).IsZero() {
		t.Fatal("execution status without timestamps should map to zero time")
	}
}

func TestDataExportsNilPagesAndPartialExportDetails(t *testing.T) {
	data := &fakeDataExports{
		listTablesOutput:     nil,
		listExportsOutput:    nil,
		listExecutionsOutput: nil,
		getExportOutput: &awsbcm.GetExportOutput{
			Export: &bcmtypes.Export{
				Name:      aws.String("partial"),
				ExportArn: aws.String("arn:aws:bcm-data-exports:us-east-1:123456789012:export/partial"),
			},
		},
	}
	client := readyClient(t, &fakeSTS{}, data, &fakeS3{})

	tables, err := client.ListTables(context.Background(), "")
	if err != nil {
		t.Fatalf("ListTables returned error: %v", err)
	}
	if len(tables.Tables) != 0 || tables.NextToken != "" {
		t.Fatalf("tables = %#v, want empty nil-output page", tables)
	}

	exports, err := client.ListExports(context.Background(), "")
	if err != nil {
		t.Fatalf("ListExports returned error: %v", err)
	}
	if len(exports.Exports) != 0 || exports.NextToken != "" {
		t.Fatalf("exports = %#v, want empty nil-output page", exports)
	}

	executions, err := client.ListExecutions(context.Background(), "export-arn", "")
	if err != nil {
		t.Fatalf("ListExecutions returned error: %v", err)
	}
	if len(executions.Executions) != 0 || executions.NextToken != "" {
		t.Fatalf("executions = %#v, want empty nil-output page", executions)
	}

	export, err := client.GetExport(context.Background(), "arn:aws:bcm-data-exports:us-east-1:123456789012:export/partial")
	if err != nil {
		t.Fatalf("GetExport returned error: %v", err)
	}
	if export.Name != "partial" || !export.CreatedAt.IsZero() || export.QueryStatement != "" || export.RefreshCadence != "" || export.Destination.Bucket != "" {
		t.Fatalf("partial export = %#v, want safe zero values for missing nested SDK fields", export)
	}
}
