package cur2preflight

import (
	"encoding/json"
	"strings"
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
		return failFinding("aws_s3_delivery_policy_missing", "AWS Data Exports S3 delivery policy", "S3 bucket policy could not be parsed safely.")
	}

	for _, statement := range document.Statement {
		if statement.Effect != "Allow" || !containsString(statement.Principal.Service, dataExportsServicePrincipal) {
			continue
		}
		if !includesS3Write(statement.Action) {
			continue
		}
		if !resourceCoversDestination(statement.Resource, destination) {
			continue
		}
		if !sourceAccountConditionMatches(statement.Condition, sourceAccount) ||
			!sourceARNConditionMatches(statement.Condition, sourceARN) {
			continue
		}
		return passFinding("aws_s3_delivery_policy_ready", "AWS Data Exports S3 delivery policy", "S3 bucket policy allows AWS Data Exports delivery with expected source conditions.")
	}

	return failFinding("aws_s3_delivery_policy_missing", "AWS Data Exports S3 delivery policy", "S3 bucket policy does not allow the AWS Data Exports service principal.")
}

func includesS3Write(actions []string) bool {
	for _, action := range actions {
		switch strings.TrimSpace(action) {
		case "s3:PutObject", "s3:*", "*":
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

	for _, resource := range resources {
		switch strings.TrimSpace(resource) {
		case expectedPrefixWildcard, expectedBucketWildcard:
			return true
		}
	}
	return false
}

func sourceAccountConditionMatches(conditions conditionMap, expected string) bool {
	return conditionMatches(conditions, []string{"StringEquals"}, "aws:SourceAccount", expected)
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
			if strings.TrimSpace(value) == expected {
				return true
			}
		}
	}
	return false
}

func sourceARNValueMatches(operator string, value string, expected string) bool {
	value = strings.TrimSpace(value)
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
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}
