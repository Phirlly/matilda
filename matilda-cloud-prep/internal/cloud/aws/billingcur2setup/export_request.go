package billingcur2setup

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
)

const maxQueryStatementLength = 36000

var cur2ColumnNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func buildCreateExportRequest(plan setupPlan) CreateExportRequest {
	return CreateExportRequest{
		Name:                plan.Facts.ExportName,
		QueryStatement:      plan.QueryStatement,
		TableConfigurations: matildaCUR2TableConfigurations(plan.Identity),
		Destination: cur2preflight.S3Destination{
			Bucket:      plan.Facts.BucketName,
			BucketOwner: plan.Facts.BucketOwner,
			Prefix:      plan.Facts.Prefix,
			Region:      plan.Region,
			Output: cur2preflight.S3Output{
				Format:      "TEXT_OR_CSV",
				Compression: "GZIP",
				Overwrite:   "CREATE_NEW_REPORT",
				OutputType:  "CUSTOM",
			},
		},
		RefreshCadence:    "SYNCHRONOUS",
		DataExportsRegion: dataExportsRegion,
	}
}

func matildaCUR2QueryStatement(columns []string) (string, error) {
	selected := make([]string, 0, len(columns)+1)
	seen := map[string]bool{}
	for _, column := range columns {
		trimmed := strings.TrimSpace(column)
		if trimmed == "" || trimmed != column || !cur2ColumnNamePattern.MatchString(column) {
			return "", NewProviderError("aws_cur2_table_invalid_shape", "AWS CUR 2.0 table schema contains an unsafe column name.")
		}
		if seen[trimmed] {
			return "", NewProviderError("aws_cur2_table_invalid_shape", "AWS CUR 2.0 table schema contains duplicate column names.")
		}
		seen[trimmed] = true
		selected = append(selected, trimmed)
	}
	if len(selected) == 0 {
		return "", NewProviderError("aws_cur2_table_invalid_shape", "AWS CUR 2.0 table schema is empty.")
	}
	if !seen["product_product_name"] && seen["product"] {
		selected = append(selected, "product.product_name AS product_product_name")
	}
	if missing := missingMatildaLogicalColumns(seen); len(missing) > 0 {
		return "", NewProviderError("aws_cur2_table_invalid_shape", "AWS CUR 2.0 table schema is missing required Matilda billing fields.")
	}
	query := "SELECT " + strings.Join(selected, ", ") + " FROM " + cur2TableName
	if len(query) > maxQueryStatementLength {
		return "", NewProviderError("aws_cur2_table_invalid_shape", "AWS CUR 2.0 complete-schema query exceeds the AWS QueryStatement length limit.")
	}
	return query, nil
}

func matildaCUR2TableConfigurations(identity identityContext) map[string]map[string]string {
	return map[string]map[string]string{cur2TableName: matildaCUR2TableConfiguration(identity)}
}

func matildaCUR2TableConfiguration(identity identityContext) map[string]string {
	return map[string]string{
		"BILLING_VIEW_ARN":                      primaryBillingViewARN(identity),
		"TIME_GRANULARITY":                      "MONTHLY",
		"INCLUDE_RESOURCES":                     "FALSE",
		"INCLUDE_SPLIT_COST_ALLOCATION_DATA":    "FALSE",
		"INCLUDE_CAPACITY_RESERVATION_DATA":     "FALSE",
		"INCLUDE_IAM_PRINCIPAL_DATA":            "FALSE",
		"INCLUDE_MANUAL_DISCOUNT_COMPATIBILITY": "FALSE",
	}
}

func missingMatildaLogicalColumns(columns map[string]bool) []string {
	required := []string{
		"line_item_product_code",
		"product_product_name",
		"line_item_operation",
		"line_item_line_item_description",
		"line_item_line_item_type",
		"line_item_currency_code",
		"pricing_unit",
		"line_item_usage_amount",
		"line_item_unblended_cost",
		"line_item_usage_type",
	}
	missing := []string{}
	for _, column := range required {
		if column == "product_product_name" && columns["product"] {
			continue
		}
		if !columns[column] {
			missing = append(missing, column)
		}
	}
	return missing
}

func primaryBillingViewARN(identity identityContext) string {
	return fmt.Sprintf("arn:%s:billing::%s:billingview/primary", strings.TrimSpace(identity.Partition), strings.TrimSpace(identity.AccountID))
}

func exportFromRequest(request CreateExportRequest, exportARN string) cur2preflight.Export {
	return cur2preflight.Export{
		Name:                request.Name,
		ExportARN:           exportARN,
		QueryStatement:      request.QueryStatement,
		TableConfigurations: request.TableConfigurations,
		Destination:         request.Destination,
		RefreshCadence:      request.RefreshCadence,
		HealthStatus:        "HEALTHY",
	}
}

func plannedExportARN(plan setupPlan) string {
	return fmt.Sprintf("arn:%s:bcm-data-exports:%s:%s:export/%s", plan.Identity.Partition, dataExportsRegion, plan.Identity.AccountID, plan.Facts.ExportName)
}

func safePlannedExportRef(plan setupPlan) string {
	return cur2preflight.SafeCUR2ExportRef(plannedExportARN(plan))
}

func validateReturnedExportARN(exportARN string, plan setupPlan) error {
	return validateExportARN(exportARN, plan, "aws_cur2_create_export_validation_failed", "AWS Data Exports returned")
}

func validateManagedExportARN(exportARN string, plan setupPlan) error {
	return validateExportARN(exportARN, plan, "aws_cur2_managed_export_validation_failed", "Managed AWS CUR 2.0 export has")
}

func validateExportARN(exportARN string, plan setupPlan, code string, subject string) error {
	trimmed := strings.TrimSpace(exportARN)
	if trimmed == "" || trimmed != exportARN {
		return NewProviderError(code, subject+" an invalid export ARN.")
	}
	parts := strings.SplitN(trimmed, ":", 6)
	if len(parts) != 6 ||
		parts[0] != "arn" ||
		parts[1] != plan.Identity.Partition ||
		parts[2] != "bcm-data-exports" ||
		parts[3] != dataExportsRegion ||
		parts[4] != plan.Identity.AccountID {
		return NewProviderError(code, subject+" an export ARN outside the approved account, partition, or service region.")
	}
	resource := parts[5]
	if !strings.HasPrefix(resource, "export/") || strings.TrimPrefix(resource, "export/") == "" {
		return NewProviderError(code, subject+" a non-export ARN.")
	}
	return nil
}

func isManagedExport(export cur2preflight.Export, plan setupPlan) bool {
	request := buildCreateExportRequest(plan)
	if export.Name != request.Name {
		return false
	}
	if export.QueryStatement != request.QueryStatement {
		return false
	}
	if export.RefreshCadence != request.RefreshCadence {
		return false
	}
	if export.Destination.Bucket != request.Destination.Bucket ||
		export.Destination.BucketOwner != request.Destination.BucketOwner ||
		export.Destination.Prefix != request.Destination.Prefix ||
		export.Destination.Region != request.Destination.Region {
		return false
	}
	if export.Destination.Output != request.Destination.Output {
		return false
	}
	return tableConfigurationsEqual(export.TableConfigurations, request.TableConfigurations)
}

func tableConfigurationsEqual(left map[string]map[string]string, right map[string]map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for table, leftValues := range left {
		rightValues, ok := right[table]
		if !ok || len(leftValues) != len(rightValues) {
			return false
		}
		for key, value := range leftValues {
			if rightValues[key] != value {
				return false
			}
		}
	}
	return true
}
