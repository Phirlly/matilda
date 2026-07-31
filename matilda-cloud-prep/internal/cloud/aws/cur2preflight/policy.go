package cur2preflight

import (
	"encoding/json"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const dataExportsServicePrincipal = "bcm-data-exports.amazonaws.com"

type bucketPolicyDocument struct {
	Statement []bucketPolicyStatement `json:"Statement"`
}

type bucketPolicyStatement struct {
	Effect    string                `json:"Effect"`
	Principal bucketPolicyPrincipal `json:"Principal"`
	Action    stringList            `json:"Action"`
	Resource  stringList            `json:"Resource"`
	Condition conditionMap          `json:"Condition"`
}

type bucketPolicyPrincipal struct {
	Service stringList `json:"Service"`
}

type stringList []string
type conditionMap map[string]map[string]stringList

func (values *stringList) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*values = []string{single}
		return nil
	}

	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*values = many
	return nil
}

func validateBucketPolicy(policy string, sourceAccount string, sourceARN string, destination S3Destination) checkFinding {
	var document bucketPolicyDocument
	if err := json.Unmarshal([]byte(policy), &document); err != nil {
		return withEvidence(failFinding("aws_s3_delivery_policy_missing", "AWS Data Exports S3 delivery policy", "S3 bucket policy could not be parsed safely."), policyGapEvidence("policy_unparseable"))
	}

	gaps := newPolicyGaps()
	for _, statement := range document.Statement {
		if statement.Effect != "Allow" {
			continue
		}
		hasPrincipal := containsString(statement.Principal.Service, dataExportsServicePrincipal)
		gaps.observe("service_principal_missing", hasPrincipal)
		if !hasPrincipal {
			continue
		}

		hasWriteAction := includesS3Write(statement.Action)
		gaps.observe("put_object_action_missing", hasWriteAction)
		hasCoveredResource := resourceCoversDestination(statement.Resource, destination)
		gaps.observe("put_object_resource_not_covered", hasCoveredResource)
		hasSourceAccount := sourceAccountConditionMatches(statement.Condition, sourceAccount)
		gaps.observe("source_account_condition_missing", hasSourceAccount)
		hasSourceARN := sourceARNConditionMatches(statement.Condition, sourceARN)
		gaps.observe("source_arn_condition_missing", hasSourceARN)

		if !hasWriteAction {
			continue
		}
		if !hasCoveredResource {
			continue
		}
		if !hasSourceAccount || !hasSourceARN {
			continue
		}
		return passFinding("aws_s3_delivery_policy_ready", "AWS Data Exports S3 delivery policy", "S3 bucket policy allows AWS Data Exports delivery with expected source conditions.")
	}

	return withEvidence(failFinding("aws_s3_delivery_policy_missing", "AWS Data Exports S3 delivery policy", policyGapMessage(gaps.first())), policyGapEvidence(gaps.first()))
}

type policyGaps struct {
	satisfied map[string]bool
	order     []string
}

func newPolicyGaps() *policyGaps {
	return &policyGaps{
		satisfied: map[string]bool{},
		order: []string{
			"service_principal_missing",
			"put_object_action_missing",
			"put_object_resource_not_covered",
			"source_account_condition_missing",
			"source_arn_condition_missing",
		},
	}
}

func (gaps *policyGaps) observe(gap string, satisfied bool) {
	if satisfied {
		gaps.satisfied[gap] = true
	}
}

func (gaps *policyGaps) first() string {
	for _, gap := range gaps.order {
		if !gaps.satisfied[gap] {
			return gap
		}
	}
	return "matching_allow_statement_missing"
}

func policyGapEvidence(gap string) workflow.PlanEvidence {
	return workflow.PlanEvidence{Key: "policy_gap", Value: gap}
}

func policyGapMessage(gap string) string {
	switch gap {
	case "service_principal_missing":
		return "S3 bucket policy does not include an Allow statement for the AWS Data Exports service principal."
	case "put_object_action_missing":
		return "S3 bucket policy does not allow AWS Data Exports to write objects."
	case "put_object_resource_not_covered":
		return "S3 bucket policy does not cover the selected CUR 2.0 export destination prefix."
	case "source_account_condition_missing":
		return "S3 bucket policy does not include the expected AWS source account condition for Data Exports delivery."
	case "source_arn_condition_missing":
		return "S3 bucket policy does not include the expected AWS source ARN condition for Data Exports delivery."
	case "policy_unparseable":
		return "S3 bucket policy could not be parsed safely."
	default:
		return "S3 bucket policy does not include a matching AWS Data Exports delivery statement."
	}
}

func includesS3Write(actions []string) bool {
	for _, action := range actions {
		switch action {
		case "s3:PutObject":
			return true
		}
	}
	return false
}

func resourceCoversDestination(resources []string, destination S3Destination) bool {
	bucket := strings.TrimSpace(destination.Bucket)
	prefix := strings.Trim(strings.TrimSpace(destination.Prefix), "/")
	if bucket == "" || prefix == "" {
		return false
	}

	expectedPrefix := "arn:aws:s3:::" + bucket + "/" + prefix + "/"
	expectedPrefixWildcard := expectedPrefix + "*"
	expectedBucketWildcard := "arn:aws:s3:::" + bucket + "/*"
	destinationProbes := []string{
		expectedPrefix + "*",
		expectedPrefix + "export-name/data/BILLING_PERIOD=2000-01/part-00001.csv.gz",
		expectedPrefix + "export-name/metadata/BILLING_PERIOD=2000-01/Manifest.json",
	}

	for _, resource := range resources {
		switch resource {
		case expectedPrefixWildcard, expectedBucketWildcard:
			return true
		}
		if s3ResourceWildcardCovers(resource, bucket, destinationProbes) {
			return true
		}
	}
	return false
}

func s3ResourceWildcardCovers(pattern string, bucket string, objectARNs []string) bool {
	if !strings.HasPrefix(pattern, "arn:aws:s3:::") || !strings.Contains(pattern, "*") {
		return false
	}
	if !strings.HasPrefix(pattern, "arn:aws:s3:::"+bucket+"/") {
		return false
	}
	for _, objectARN := range objectARNs {
		if !wildcardMatches(pattern, objectARN) {
			return false
		}
	}
	return true
}

func wildcardMatches(pattern string, value string) bool {
	patternIndex := 0
	valueIndex := 0
	lastStar := -1
	valueAfterLastStar := 0

	for valueIndex < len(value) {
		switch {
		case patternIndex < len(pattern) && (pattern[patternIndex] == value[valueIndex] || pattern[patternIndex] == '?'):
			patternIndex++
			valueIndex++
		case patternIndex < len(pattern) && pattern[patternIndex] == '*':
			lastStar = patternIndex
			valueAfterLastStar = valueIndex
			patternIndex++
		case lastStar != -1:
			patternIndex = lastStar + 1
			valueAfterLastStar++
			valueIndex = valueAfterLastStar
		default:
			return false
		}
	}

	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}

func sourceAccountConditionMatches(conditions conditionMap, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}
	stringEqualsValues := conditions["StringEquals"]["aws:SourceAccount"]
	stringLikeValues := conditions["StringLike"]["aws:SourceAccount"]
	if len(stringLikeValues) > 0 {
		if !exactStringLikeSourceAccountValuesMatch(stringLikeValues, expected) {
			return false
		}
		if len(stringEqualsValues) > 0 {
			return conditionMatches(conditions, []string{"StringEquals"}, "aws:SourceAccount", expected)
		}
		return true
	}
	return conditionMatches(conditions, []string{"StringEquals"}, "aws:SourceAccount", expected)
}

func exactStringLikeSourceAccountValuesMatch(values []string, expected string) bool {
	for _, value := range values {
		if value != expected || strings.ContainsAny(value, "*?") {
			return false
		}
	}
	return true
}

func sourceARNConditionMatches(conditions conditionMap, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}
	for _, operator := range []string{"ArnLike", "StringLike", "ArnEquals", "StringEquals"} {
		valuesByKey := conditions[operator]
		for _, value := range valuesByKey["aws:SourceArn"] {
			if sourceARNValueMatches(operator, value, expected) {
				return true
			}
		}
	}
	return false
}

func conditionMatches(conditions conditionMap, allowedOperators []string, key string, expected string) bool {
	if expected == "" {
		return false
	}
	for _, operator := range allowedOperators {
		valuesByKey := conditions[operator]
		for _, value := range valuesByKey[key] {
			if value == expected {
				return true
			}
		}
	}
	return false
}

func sourceARNValueMatches(operator string, value string, expected string) bool {
	if value == expected {
		return true
	}
	switch operator {
	case "ArnLike", "StringLike":
		return scopedTrailingWildcardMatches(value, expected)
	default:
		return false
	}
}

func scopedTrailingWildcardMatches(pattern string, value string) bool {
	if strings.Count(pattern, "*") != 1 || !strings.HasSuffix(pattern, "*") {
		return false
	}
	prefix := strings.TrimSuffix(pattern, "*")
	if prefix == "" || !strings.Contains(prefix, ":export/") {
		return false
	}
	return strings.HasPrefix(value, prefix)
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
