package cur2preflight

import (
	"fmt"
	"regexp"
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

	if keywordCount(query, "SELECT") != 1 || keywordCount(query, "FROM") != 1 {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 query must contain exactly one SELECT and one FROM.")
	}
	if containsKeyword(query, "WHERE") {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 preflight does not accept WHERE filters for this path.")
	}
	if containsKeyword(query, "LIMIT") {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 preflight does not accept LIMIT for this path.")
	}

	selected, table, ok := parseSelectFrom(query)
	if !ok {
		return failFinding("aws_cur2_query_unverified", "CUR 2.0 query shape", "CUR 2.0 query could not be parsed safely.")
	}

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

func referencesCUR2QuerySource(statement string) bool {
	query := strings.TrimSpace(statement)
	if keywordCount(query, "SELECT") != 1 || keywordCount(query, "FROM") != 1 {
		return false
	}
	_, table, ok := parseSelectFrom(query)
	if !ok {
		return false
	}
	tableParts := strings.Fields(table)
	return len(tableParts) == 1 && tableParts[0] == cur2TableName
}

func parseSelectFrom(query string) (string, string, bool) {
	matches := selectFromPattern.FindStringSubmatch(query)
	if len(matches) != 3 {
		return "", "", false
	}
	return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2]), true
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

var selectFromPattern = regexp.MustCompile(`(?is)^\s*select\s+(.+?)\s+from\s+(.+?)\s*$`)

func containsKeyword(value string, keyword string) bool {
	return keywordCount(value, keyword) > 0
}

func keywordCount(value string, keyword string) int {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(keyword)))
	return len(pattern.FindAllStringIndex(value, -1))
}
