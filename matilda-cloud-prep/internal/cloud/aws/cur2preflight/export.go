package cur2preflight

import "strings"

func validateExportShape(export Export) []checkFinding {
	findings := []checkFinding{}

	if strings.TrimSpace(export.QueryStatement) == "" {
		findings = append(findings, failFinding("aws_cur2_export_invalid_shape", "AWS export definition", "AWS export definition is missing its query statement."))
	}
	findings = append(findings, validateOutputSettings(export)...)
	findings = append(findings, validateDestination(export.Destination)...)

	health := strings.ToUpper(strings.TrimSpace(export.HealthStatus))
	if health != "" && health != "HEALTHY" {
		findings = append(findings, failFinding("aws_cur2_export_invalid_shape", "AWS export health", "AWS reports the selected export as unhealthy."))
	} else {
		findings = append(findings, passFinding("aws_cur2_export_health_ready", "AWS export health", "AWS export health does not report a blocking failure."))
	}

	return findings
}

func validateOutputSettings(export Export) []checkFinding {
	findings := []checkFinding{}
	tableConfig := export.TableConfigurations[cur2TableName]
	if tableConfig == nil {
		findings = append(findings, failFinding("aws_cur2_export_invalid_shape", "CUR 2.0 table configuration", "AWS export is missing COST_AND_USAGE_REPORT table configuration."))
		return findings
	}

	if tableConfig["TIME_GRANULARITY"] != "MONTHLY" {
		findings = append(findings, failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 time granularity", "CUR 2.0 preflight requires monthly time granularity for this path."))
	} else {
		findings = append(findings, passFinding("aws_cur2_time_granularity_ready", "CUR 2.0 time granularity", "CUR 2.0 export uses monthly time granularity."))
	}

	switch tableConfig["INCLUDE_RESOURCES"] {
	case "TRUE":
		findings = append(findings, warnFinding("aws_cur2_include_resources_enabled", "CUR 2.0 resource IDs", "INCLUDE_RESOURCES is enabled and may increase export size.", false))
	case "FALSE", "":
		findings = append(findings, warnFinding("aws_cur2_include_resources_not_required", "CUR 2.0 resource IDs", "INCLUDE_RESOURCES is not required by the current mandatory billing fields.", false))
	default:
		findings = append(findings, failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 resource IDs", "CUR 2.0 INCLUDE_RESOURCES setting is not recognized for this path."))
	}

	output := export.Destination.Output
	if output.Format != "TEXT_OR_CSV" {
		findings = append(findings, failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 output format", "CUR 2.0 preflight requires TEXT_OR_CSV output for this path."))
	} else {
		findings = append(findings, passFinding("aws_cur2_output_format_ready", "CUR 2.0 output format", "CUR 2.0 export uses TEXT_OR_CSV output."))
	}
	if output.Compression != "GZIP" {
		findings = append(findings, failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 compression", "CUR 2.0 preflight requires GZIP compression for this path."))
	} else {
		findings = append(findings, passFinding("aws_cur2_compression_ready", "CUR 2.0 compression", "CUR 2.0 export uses GZIP compression."))
	}
	if output.Overwrite != "CREATE_NEW_REPORT" {
		findings = append(findings, failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 overwrite setting", "CUR 2.0 preflight requires CREATE_NEW_REPORT for this path."))
	} else {
		findings = append(findings, passFinding("aws_cur2_overwrite_ready", "CUR 2.0 overwrite setting", "CUR 2.0 export creates new report files."))
	}
	if output.OutputType != "CUSTOM" {
		findings = append(findings, failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 output type", "CUR 2.0 preflight requires CUSTOM output type for this path."))
	} else {
		findings = append(findings, passFinding("aws_cur2_output_type_ready", "CUR 2.0 output type", "CUR 2.0 export uses CUSTOM output type."))
	}
	if export.RefreshCadence != "SYNCHRONOUS" {
		findings = append(findings, failFinding("aws_cur2_output_settings_blocked", "CUR 2.0 refresh cadence", "CUR 2.0 preflight requires SYNCHRONOUS refresh cadence for this path."))
	} else {
		findings = append(findings, passFinding("aws_cur2_refresh_ready", "CUR 2.0 refresh cadence", "CUR 2.0 export uses SYNCHRONOUS refresh cadence."))
	}

	return findings
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
