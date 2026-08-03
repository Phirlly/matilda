package cur2preflight

import (
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func validateExportShape(export Export, returnedTableProperties map[string]string) []checkFinding {
	findings := []checkFinding{}

	if strings.TrimSpace(export.QueryStatement) == "" {
		findings = append(findings, failFinding("aws_cur2_export_invalid_shape", "AWS export definition", "AWS export definition is missing its query statement."))
	}
	findings = append(findings, validateOutputSettings(export, returnedTableProperties)...)
	findings = append(findings, validateDestination(export.Destination)...)

	health := strings.ToUpper(strings.TrimSpace(export.HealthStatus))
	switch health {
	case "HEALTHY":
		findings = append(findings, withEvidence(passFinding("aws_cur2_export_health_ready", "AWS export health", "AWS export health is healthy."),
			exportHealthEvidence(health),
		))
	case "":
		findings = append(findings, withEvidence(failFinding("aws_cur2_export_invalid_shape", "AWS export health", "AWS export health could not be confirmed from AWS Data Exports status metadata."),
			exportHealthEvidence(health),
		))
	case "UNHEALTHY":
		findings = append(findings, withEvidence(failFinding("aws_cur2_export_invalid_shape", "AWS export health", "AWS reports the selected export as unhealthy."),
			exportHealthEvidence(health),
		))
	default:
		findings = append(findings, withEvidence(failFinding("aws_cur2_export_health_unverified", "AWS export health", "AWS export health status is not a verified AWS Data Exports value for this path."),
			exportHealthEvidence(health),
		))
	}

	return findings
}

func validateOutputSettings(export Export, returnedTableProperties map[string]string) []checkFinding {
	findings := []checkFinding{}
	tableConfig := cur2TableConfiguration(export)
	if len(tableConfig) == 0 {
		findings = append(findings, warnFinding("aws_cur2_table_configuration_defaulted", "CUR 2.0 table configuration", "AWS export omits COST_AND_USAGE_REPORT table configuration. AWS table-property defaults are used for read-only validation.", false))
	}
	tableProperties := effectiveTableProperties(tableConfig, returnedTableProperties)

	granularity := normalizedTableProperty(tableProperties, "TIME_GRANULARITY")
	switch granularity {
	case "MONTHLY":
		findings = append(findings, withEvidence(passFinding("aws_cur2_time_granularity_ready", "CUR 2.0 time granularity", "CUR 2.0 export uses monthly time granularity."),
			outputSettingEvidence("time_granularity", granularity),
		))
	case "DAILY":
		findings = append(findings, withEvidence(warnFinding("aws_cur2_time_granularity_not_preferred", "CUR 2.0 time granularity", "CUR 2.0 export uses daily time granularity. This is valid AWS CUR 2.0, but monthly is preferred for Matilda Rapid Assessment - Billing Based.", true),
			outputSettingEvidence("time_granularity", granularity),
		))
	case "HOURLY":
		findings = append(findings, withEvidence(warnFinding("aws_cur2_time_granularity_not_preferred", "CUR 2.0 time granularity", "CUR 2.0 export uses hourly time granularity. This is valid AWS CUR 2.0, but monthly is preferred and hourly exports can increase file volume.", true),
			outputSettingEvidence("time_granularity", granularity),
		))
	case "":
		findings = append(findings, withEvidence(warnFinding("aws_cur2_time_granularity_unverified", "CUR 2.0 time granularity", "CUR 2.0 time granularity could not be confirmed from the export or returned table metadata. AWS table properties are optional and defaulted, so this is a warning, not an invalid CUR 2.0 export.", true),
			workflow.PlanEvidence{Key: "time_granularity", Value: "unverified"},
		))
	default:
		findings = append(findings, withEvidence(failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 time granularity", "CUR 2.0 time granularity is not one of AWS's documented CUR 2.0 values: HOURLY, DAILY, or MONTHLY."),
			outputSettingEvidence("time_granularity", granularity),
		))
	}

	switch normalizedTableProperty(tableProperties, "INCLUDE_RESOURCES") {
	case "TRUE":
		findings = append(findings, warnFinding("aws_cur2_include_resources_enabled", "CUR 2.0 resource IDs", "INCLUDE_RESOURCES is enabled and may increase export size.", false))
	case "FALSE", "":
		findings = append(findings, warnFinding("aws_cur2_include_resources_not_required", "CUR 2.0 resource IDs", "INCLUDE_RESOURCES is not required by the current mandatory billing fields.", false))
	default:
		findings = append(findings, failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 resource IDs", "CUR 2.0 INCLUDE_RESOURCES setting is not recognized for this path."))
	}

	output := export.Destination.Output
	format := normalizedOutputSetting(output.Format)
	compression := normalizedOutputSetting(output.Compression)
	switch format {
	case "TEXT_OR_CSV":
		findings = append(findings, withEvidence(passFinding("aws_cur2_output_format_ready", "CUR 2.0 output format", "CUR 2.0 export uses TEXT_OR_CSV output, the currently documented preferred Matilda shape."),
			outputSettingEvidence("output_format", format),
			workflow.PlanEvidence{Key: "matilda_format_support", Value: "supported"},
			workflow.PlanEvidence{Key: "matilda_output_preference", Value: "preferred"},
		))
	case "PARQUET":
		findings = append(findings, withEvidence(passFinding("aws_cur2_output_format_supported", "CUR 2.0 output format", "CUR 2.0 export uses PARQUET output. AWS supports PARQUET and Matilda can use it for this path."),
			outputSettingEvidence("output_format", format),
			workflow.PlanEvidence{Key: "matilda_format_support", Value: "supported"},
			workflow.PlanEvidence{Key: "matilda_output_preference", Value: "supported_not_preferred"},
		))
	default:
		findings = append(findings, withEvidence(failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 output format", "CUR 2.0 output format is not one of AWS's documented Data Exports values supported for this Matilda path: TEXT_OR_CSV or PARQUET."),
			outputSettingEvidence("output_format", format),
			workflow.PlanEvidence{Key: "matilda_format_support", Value: "unsupported"},
		))
	}
	switch {
	case format == "TEXT_OR_CSV" && compression == "GZIP":
		findings = append(findings, withEvidence(passFinding("aws_cur2_compression_ready", "CUR 2.0 compression", "CUR 2.0 export uses GZIP compression for the preferred TEXT_OR_CSV output shape."),
			outputSettingEvidence("compression", compression),
		))
	case format == "PARQUET" && compression == "PARQUET":
		findings = append(findings, withEvidence(passFinding("aws_cur2_compression_supported", "CUR 2.0 compression", "CUR 2.0 export uses PARQUET compression for the supported PARQUET output shape."),
			outputSettingEvidence("compression", compression),
		))
	case compression != "GZIP" && compression != "PARQUET":
		findings = append(findings, withEvidence(failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 compression", "CUR 2.0 compression is not one of AWS's documented Data Exports values supported for this Matilda path: GZIP or PARQUET."),
			outputSettingEvidence("compression", compression),
		))
	default:
		findings = append(findings, withEvidence(failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 compression", "CUR 2.0 output format and compression combination is not supported for this Matilda path."),
			outputSettingEvidence("output_format", format),
			outputSettingEvidence("compression", compression),
		))
	}
	switch normalizedOutputSetting(output.Overwrite) {
	case "CREATE_NEW_REPORT":
		findings = append(findings, withEvidence(passFinding("aws_cur2_overwrite_ready", "CUR 2.0 overwrite setting", "CUR 2.0 export creates new report files."),
			outputSettingEvidence("overwrite", output.Overwrite),
			workflow.PlanEvidence{Key: "matilda_output_preference", Value: "preferred"},
		))
	case "OVERWRITE_REPORT":
		findings = append(findings, withEvidence(passFinding("aws_cur2_overwrite_supported", "CUR 2.0 overwrite setting", "CUR 2.0 export overwrites report files. AWS supports OVERWRITE_REPORT for CUR 2.0 exports; CREATE_NEW_REPORT remains preferred for tool-created exports."),
			outputSettingEvidence("overwrite", output.Overwrite),
			workflow.PlanEvidence{Key: "matilda_output_preference", Value: "supported_not_preferred"},
		))
	default:
		findings = append(findings, withEvidence(failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 overwrite setting", "CUR 2.0 overwrite setting is not one of AWS's documented Data Exports values supported for this path: CREATE_NEW_REPORT or OVERWRITE_REPORT."),
			outputSettingEvidence("overwrite", output.Overwrite),
			workflow.PlanEvidence{Key: "matilda_output_preference", Value: "unsupported"},
		))
	}
	if normalizedOutputSetting(output.OutputType) != "CUSTOM" {
		findings = append(findings, withEvidence(failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 output type", "CUR 2.0 preflight requires CUSTOM output type for this path."),
			outputSettingEvidence("output_type", output.OutputType),
		))
	} else {
		findings = append(findings, withEvidence(passFinding("aws_cur2_output_type_ready", "CUR 2.0 output type", "CUR 2.0 export uses CUSTOM output type."),
			outputSettingEvidence("output_type", output.OutputType),
		))
	}
	if normalizedOutputSetting(export.RefreshCadence) != "SYNCHRONOUS" {
		findings = append(findings, withEvidence(failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 refresh cadence", "CUR 2.0 preflight requires SYNCHRONOUS refresh cadence for this path."),
			outputSettingEvidence("refresh_cadence", export.RefreshCadence),
		))
	} else {
		findings = append(findings, withEvidence(passFinding("aws_cur2_refresh_ready", "CUR 2.0 refresh cadence", "CUR 2.0 export uses SYNCHRONOUS refresh cadence."),
			outputSettingEvidence("refresh_cadence", export.RefreshCadence),
		))
	}

	return findings
}

func cur2TableConfiguration(export Export) map[string]string {
	if export.TableConfigurations == nil {
		return nil
	}
	return export.TableConfigurations[cur2TableName]
}

func effectiveTableProperties(exportTableConfig map[string]string, returnedTableProperties map[string]string) map[string]string {
	if len(returnedTableProperties) > 0 {
		return returnedTableProperties
	}
	return exportTableConfig
}

func normalizedTableProperty(properties map[string]string, key string) string {
	return strings.ToUpper(strings.TrimSpace(properties[key]))
}

func normalizedOutputSetting(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func outputSettingEvidence(key string, value string) workflow.PlanEvidence {
	value = safeEvidenceValue(normalizedOutputSetting(value))
	if value == "" {
		value = "unavailable"
	}
	return workflow.PlanEvidence{Key: key, Value: value}
}

func exportHealthEvidence(health string) workflow.PlanEvidence {
	value := safeEvidenceValue(normalizedOutputSetting(health))
	if value == "" {
		value = "unverified"
	}
	return workflow.PlanEvidence{Key: "export_health", Value: value}
}

func validateDestination(destination S3Destination) []checkFinding {
	findings := []checkFinding{}
	missing := []string{}
	if strings.TrimSpace(destination.Bucket) == "" {
		missing = append(missing, "bucket")
	}
	if strings.TrimSpace(destination.Prefix) == "" {
		missing = append(missing, "prefix")
	}
	if strings.TrimSpace(destination.Region) == "" {
		missing = append(missing, "region")
	}

	if len(missing) > 0 {
		findings = append(findings, failFinding("aws_cur2_export_invalid_shape", "CUR 2.0 S3 destination", "CUR 2.0 export is missing required S3 destination fields."))
		return findings
	}

	findings = append(findings, passFinding("aws_cur2_s3_destination_ready", "CUR 2.0 S3 destination", "CUR 2.0 export has S3 bucket, prefix, and region configured."))
	return findings
}
