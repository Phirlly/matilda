package awsclient

import (
	"context"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsbcm "github.com/aws/aws-sdk-go-v2/service/bcmdataexports"
	bcmtypes "github.com/aws/aws-sdk-go-v2/service/bcmdataexports/types"
)

func (client *Client) ListTables(ctx context.Context, token string) (cur2preflight.TablePage, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.TablePage{}, err
	}
	input := &awsbcm.ListTablesInput{}
	if token != "" {
		input.NextToken = aws.String(token)
	}
	output, err := client.data.ListTables(ctx, input)
	if err != nil {
		return cur2preflight.TablePage{}, classifyDataExportsError(err, "aws_data_exports_access_denied")
	}
	if output == nil {
		return cur2preflight.TablePage{}, nil
	}
	page := cur2preflight.TablePage{NextToken: aws.ToString(output.NextToken)}
	for _, table := range output.Tables {
		if name := aws.ToString(table.TableName); name != "" {
			page.Tables = append(page.Tables, cur2preflight.TableSummary{Name: name})
		}
	}
	return page, nil
}

func (client *Client) GetTable(ctx context.Context, name string, properties map[string]string) (cur2preflight.Table, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.Table{}, err
	}
	input := &awsbcm.GetTableInput{
		TableName:       aws.String(name),
		TableProperties: copyStringMap(properties),
	}
	output, err := client.data.GetTable(ctx, input)
	if err != nil {
		return cur2preflight.Table{}, classifyDataExportsError(err, "aws_cur2_table_unavailable")
	}
	if output == nil {
		return cur2preflight.Table{}, providerError("aws_cur2_table_unavailable")
	}
	table := cur2preflight.Table{Name: aws.ToString(output.TableName)}
	for _, column := range output.Schema {
		if name := aws.ToString(column.Name); name != "" {
			table.Columns = append(table.Columns, name)
		}
	}
	return table, nil
}

func (client *Client) ListExports(ctx context.Context, token string) (cur2preflight.ExportPage, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.ExportPage{}, err
	}
	input := &awsbcm.ListExportsInput{}
	if token != "" {
		input.NextToken = aws.String(token)
	}
	output, err := client.data.ListExports(ctx, input)
	if err != nil {
		return cur2preflight.ExportPage{}, classifyDataExportsError(err, "aws_data_exports_access_denied")
	}
	if output == nil {
		return cur2preflight.ExportPage{}, nil
	}
	page := cur2preflight.ExportPage{NextToken: aws.ToString(output.NextToken)}
	for _, export := range output.Exports {
		page.Exports = append(page.Exports, cur2preflight.ExportSummary{
			Name:      aws.ToString(export.ExportName),
			ExportARN: aws.ToString(export.ExportArn),
		})
	}
	return page, nil
}

func (client *Client) GetExport(ctx context.Context, exportARN string) (cur2preflight.Export, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.Export{}, err
	}
	output, err := client.data.GetExport(ctx, &awsbcm.GetExportInput{ExportArn: aws.String(exportARN)})
	if err != nil {
		return cur2preflight.Export{}, classifyDataExportsError(err, "aws_cur2_export_invalid_shape")
	}
	if output == nil || output.Export == nil {
		return cur2preflight.Export{}, providerError("aws_cur2_export_invalid_shape")
	}
	return mapExport(output), nil
}

func (client *Client) ListExecutions(ctx context.Context, exportARN string, token string) (cur2preflight.ExecutionPage, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.ExecutionPage{}, err
	}
	input := &awsbcm.ListExecutionsInput{ExportArn: aws.String(exportARN)}
	if token != "" {
		input.NextToken = aws.String(token)
	}
	output, err := client.data.ListExecutions(ctx, input)
	if err != nil {
		return cur2preflight.ExecutionPage{}, classifyDataExportsError(err, "aws_data_exports_access_denied")
	}
	if output == nil {
		return cur2preflight.ExecutionPage{}, nil
	}
	page := cur2preflight.ExecutionPage{NextToken: aws.ToString(output.NextToken)}
	for _, execution := range output.Executions {
		page.Executions = append(page.Executions, mapExecutionReference(execution))
	}
	return page, nil
}

func (client *Client) GetExecution(ctx context.Context, exportARN string, executionID string) (cur2preflight.Execution, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.Execution{}, err
	}
	output, err := client.data.GetExecution(ctx, &awsbcm.GetExecutionInput{
		ExportArn:   aws.String(exportARN),
		ExecutionId: aws.String(executionID),
	})
	if err != nil {
		return cur2preflight.Execution{}, classifyExecutionDetailError(err)
	}
	if output == nil {
		return cur2preflight.Execution{}, providerError("aws_data_exports_access_denied")
	}
	return cur2preflight.Execution{
		ID:               aws.ToString(output.ExecutionId),
		Status:           statusCodeString(output.ExecutionStatus),
		StatusObservedAt: statusObservationTime(output.ExecutionStatus),
	}, nil
}

func mapExport(output *awsbcm.GetExportOutput) cur2preflight.Export {
	export := output.Export
	mapped := cur2preflight.Export{
		Name:      aws.ToString(export.Name),
		ExportARN: aws.ToString(export.ExportArn),
	}
	if output.ExportStatus != nil {
		mapped.CreatedAt = timeValue(output.ExportStatus.CreatedAt)
		mapped.HealthStatus = string(output.ExportStatus.StatusCode)
	}
	if export.DataQuery != nil {
		mapped.QueryStatement = aws.ToString(export.DataQuery.QueryStatement)
		mapped.TableConfigurations = copyNestedStringMap(export.DataQuery.TableConfigurations)
	}
	if export.RefreshCadence != nil {
		mapped.RefreshCadence = string(export.RefreshCadence.Frequency)
	}
	if export.DestinationConfigurations != nil && export.DestinationConfigurations.S3Destination != nil {
		mapped.Destination = mapS3Destination(export.DestinationConfigurations.S3Destination)
	}
	return mapped
}

func mapS3Destination(destination *bcmtypes.S3Destination) cur2preflight.S3Destination {
	mapped := cur2preflight.S3Destination{
		Bucket: aws.ToString(destination.S3Bucket),
		Prefix: aws.ToString(destination.S3Prefix),
		Region: aws.ToString(destination.S3Region),
	}
	if destination.S3OutputConfigurations != nil {
		mapped.Output = cur2preflight.S3Output{
			Format:      string(destination.S3OutputConfigurations.Format),
			Compression: string(destination.S3OutputConfigurations.Compression),
			Overwrite:   string(destination.S3OutputConfigurations.Overwrite),
			OutputType:  string(destination.S3OutputConfigurations.OutputType),
		}
	}
	return mapped
}

func mapExecutionReference(reference bcmtypes.ExecutionReference) cur2preflight.Execution {
	return cur2preflight.Execution{
		ID:               aws.ToString(reference.ExecutionId),
		Status:           statusCodeString(reference.ExecutionStatus),
		StatusObservedAt: statusObservationTime(reference.ExecutionStatus),
	}
}

func statusCodeString(status *bcmtypes.ExecutionStatus) string {
	if status == nil {
		return ""
	}
	return string(status.StatusCode)
}

func statusObservationTime(status *bcmtypes.ExecutionStatus) time.Time {
	if status == nil {
		return time.Time{}
	}
	if status.LastUpdatedAt != nil {
		return *status.LastUpdatedAt
	}
	if status.CompletedAt != nil {
		return *status.CompletedAt
	}
	if status.CreatedAt != nil {
		return *status.CreatedAt
	}
	return time.Time{}
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
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

func copyNestedStringMap(values map[string]map[string]string) map[string]map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]map[string]string, len(values))
	for key, value := range values {
		copied[key] = copyStringMap(value)
	}
	return copied
}
