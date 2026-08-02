package billingcur2setup

import (
	"fmt"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
)

func buildCreateExportRequest(plan setupPlan) CreateExportRequest {
	return CreateExportRequest{
		Name:                plan.Facts.ExportName,
		QueryStatement:      matildaCUR2QueryStatement(),
		TableConfigurations: matildaCUR2TableConfigurations(),
		Destination: cur2preflight.S3Destination{
			Bucket: plan.Facts.BucketName,
			Prefix: plan.Facts.Prefix,
			Region: plan.Region,
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

func matildaCUR2QueryStatement() string {
	return "SELECT line_item_product_code, product.product_name AS product_product_name, line_item_operation, line_item_line_item_description, line_item_line_item_type, line_item_currency_code, pricing_unit, line_item_usage_amount, line_item_unblended_cost, line_item_usage_type FROM COST_AND_USAGE_REPORT"
}

func matildaCUR2TableConfigurations() map[string]map[string]string {
	return map[string]map[string]string{
		cur2TableName: {
			"TIME_GRANULARITY":                      "MONTHLY",
			"INCLUDE_RESOURCES":                     "FALSE",
			"INCLUDE_SPLIT_COST_ALLOCATION_DATA":    "FALSE",
			"INCLUDE_CAPACITY_RESERVATION_DATA":     "FALSE",
			"INCLUDE_IAM_PRINCIPAL_DATA":            "FALSE",
			"INCLUDE_MANUAL_DISCOUNT_COMPATIBILITY": "FALSE",
		},
	}
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
