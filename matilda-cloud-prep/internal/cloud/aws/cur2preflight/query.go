package cur2preflight

import (
	"fmt"
	"strings"
)

const cur2TableName = "COST_AND_USAGE_REPORT"

func requiredCUR2Columns() []string {
	return []string{
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
}

func validateCUR2Query(statement string, tableColumns []string) checkFinding {
	query := strings.TrimSpace(statement)
	if query == "" {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 query statement is empty.")
	}

	upper := strings.ToUpper(query)
	if strings.Count(upper, "SELECT ") != 1 || strings.Count(upper, " FROM ") != 1 {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 query must contain exactly one SELECT and one FROM.")
	}
	if containsKeyword(upper, " WHERE ") || strings.Contains(upper, "\nWHERE ") || strings.Contains(upper, "\tWHERE ") {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 preflight does not accept WHERE filters for this path.")
	}
	if containsKeyword(upper, " LIMIT ") || strings.Contains(upper, "\nLIMIT ") || strings.Contains(upper, "\tLIMIT ") {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 preflight does not accept LIMIT for this path.")
	}

	selectStart := strings.Index(upper, "SELECT ")
	fromStart := strings.Index(upper, " FROM ")
	if selectStart < 0 || fromStart < 0 || fromStart <= selectStart {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 query could not be parsed safely.")
	}

	selected := strings.TrimSpace(query[selectStart+len("SELECT ") : fromStart])
	table := strings.TrimSpace(query[fromStart+len(" FROM "):])
	tableParts := strings.Fields(table)
	if len(tableParts) != 1 {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 query has unsupported table clause content.")
	}
	if tableParts[0] != cur2TableName {
		return failFinding("aws_non_cur2_source_out_of_scope", "CUR 2.0 query source", "Query does not reference COST_AND_USAGE_REPORT exactly.")
	}
	if selected == "*" {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query fields", "SELECT * is not accepted for the first CUR 2.0 path.")
	}

	outputs := map[string]bool{}
	schemaColumns := tableColumnSet(tableColumns)
	for _, item := range strings.Split(selected, ",") {
		source, output, ok := parseSelectedColumn(item, schemaColumns)
		if !ok {
			return failFinding("aws_cur2_query_unverified", "CUR 2.0 query fields", "CUR 2.0 query contains unsupported selected field syntax.")
		}
		if source != output {
			return failFinding("aws_cur2_query_unverified", "CUR 2.0 query fields", "CUR 2.0 selected fields must keep the same output name for this preflight path.")
		}
		outputs[output] = true
	}

	missing := missingRequiredColumns(outputs)
	if len(missing) > 0 {
		return failFinding("aws_cur2_required_fields_missing", "CUR 2.0 required fields", "CUR 2.0 query is missing required Matilda billing fields.")
	}

	return passFinding("aws_cur2_query_valid", "CUR 2.0 query fields", "CUR 2.0 query selects the required billing fields from COST_AND_USAGE_REPORT.")
}

func parseSelectedColumn(item string, schemaColumns map[string]bool) (source string, output string, ok bool) {
	trimmed := strings.TrimSpace(item)
	if trimmed == "" || strings.ContainsAny(trimmed, "().*") {
		return "", "", false
	}

	fields := strings.Fields(trimmed)
	switch len(fields) {
	case 1:
		if !schemaColumns[fields[0]] {
			return "", "", false
		}
		return fields[0], fields[0], true
	case 3:
		if !strings.EqualFold(fields[1], "AS") {
			return "", "", false
		}
		if !schemaColumns[fields[0]] || !schemaColumns[fields[2]] {
			return "", "", false
		}
		return fields[0], fields[2], true
	default:
		return "", "", false
	}
}

func validateTableColumns(columns []string) checkFinding {
	available := map[string]bool{}
	for _, column := range columns {
		available[column] = true
	}
	if missing := missingRequiredColumns(available); len(missing) > 0 {
		return failFinding("aws_cur2_required_fields_missing", "CUR 2.0 table columns", "COST_AND_USAGE_REPORT metadata is missing required billing fields.")
	}
	return passFinding("aws_cur2_table_ready", "CUR 2.0 table columns", "COST_AND_USAGE_REPORT metadata includes required billing fields.")
}

func missingRequiredColumns(available map[string]bool) []string {
	missing := []string{}
	for _, required := range requiredCUR2Columns() {
		if !available[required] {
			missing = append(missing, required)
		}
	}
	return missing
}

func tableColumnSet(columns []string) map[string]bool {
	available := map[string]bool{}
	for _, column := range columns {
		available[column] = true
	}
	return available
}

func containsKeyword(value string, keyword string) bool {
	return strings.Contains(fmt.Sprintf(" %s ", value), keyword)
}
