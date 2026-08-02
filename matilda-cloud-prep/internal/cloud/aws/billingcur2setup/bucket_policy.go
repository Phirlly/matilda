package billingcur2setup

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const dataExportsDeliveryStatementSid = "MatildaCloudPrepDataExportsDelivery"

type policyDocument struct {
	Version   string              `json:"Version,omitempty"`
	Statement rawPolicyStatements `json:"Statement"`
}

type rawPolicyStatements []json.RawMessage

type policyStatement struct {
	Sid       string                       `json:"Sid,omitempty"`
	Effect    string                       `json:"Effect"`
	Principal map[string][]string          `json:"Principal"`
	Action    []string                     `json:"Action"`
	Resource  string                       `json:"Resource"`
	Condition map[string]map[string]string `json:"Condition"`
}

type policyStatementIdentity struct {
	Sid string `json:"Sid,omitempty"`
}

type flexiblePolicyStatement struct {
	Sid       string                       `json:"Sid,omitempty"`
	Effect    string                       `json:"Effect"`
	Principal map[string]json.RawMessage   `json:"Principal"`
	Action    json.RawMessage              `json:"Action"`
	Resource  json.RawMessage              `json:"Resource"`
	Condition map[string]map[string]string `json:"Condition"`
}

type normalizedPolicyStatement struct {
	Sid       string                       `json:"Sid,omitempty"`
	Effect    string                       `json:"Effect"`
	Principal map[string][]string          `json:"Principal"`
	Action    []string                     `json:"Action"`
	Resource  []string                     `json:"Resource"`
	Condition map[string]map[string]string `json:"Condition"`
}

func (statements *rawPolicyStatements) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		var single json.RawMessage
		if err := json.Unmarshal(data, &single); err != nil {
			return err
		}
		*statements = []json.RawMessage{single}
		return nil
	}
	var many []json.RawMessage
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*statements = many
	return nil
}

func mergeDataExportsPolicy(existing string, plan setupPlan) (string, bool, error) {
	statement := dataExportsStatement(plan)
	statementJSON, err := json.Marshal(statement)
	if err != nil {
		return "", false, err
	}

	document := policyDocument{
		Version:   "2012-10-17",
		Statement: []json.RawMessage{},
	}
	if strings.TrimSpace(existing) != "" {
		if err := json.Unmarshal([]byte(existing), &document); err != nil {
			return "", false, fmt.Errorf("policy cannot be parsed safely")
		}
	}
	if document.Version == "" {
		document.Version = "2012-10-17"
	}

	changed := false
	readyStatementFound := false
	statements := make([]json.RawMessage, 0, len(document.Statement)+1)
	for _, raw := range document.Statement {
		var parsed policyStatementIdentity
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", false, fmt.Errorf("policy statement cannot be parsed safely")
		}
		if equivalentPolicyStatement(raw, statement) {
			readyStatementFound = true
			statements = append(statements, raw)
			continue
		}
		if parsed.Sid == dataExportsDeliveryStatementSid {
			if jsonEqual(raw, statementJSON) {
				readyStatementFound = true
				statements = append(statements, raw)
				continue
			}
			return "", false, fmt.Errorf("policy contains a non-equivalent %s statement and cannot be merged safely", dataExportsDeliveryStatementSid)
		}
		statements = append(statements, raw)
	}
	if !readyStatementFound {
		statements = append(statements, statementJSON)
		changed = true
	}
	document.Statement = statements

	merged, err := json.Marshal(document)
	if err != nil {
		return "", false, err
	}
	return string(merged), changed, nil
}

func dataExportsStatement(plan setupPlan) policyStatement {
	return policyStatement{
		Sid:    dataExportsDeliveryStatementSid,
		Effect: "Allow",
		Principal: map[string][]string{
			"Service": []string{"bcm-data-exports.amazonaws.com"},
		},
		Action:   []string{"s3:PutObject"},
		Resource: fmt.Sprintf("arn:%s:s3:::%s/%s/*", plan.Identity.Partition, plan.Facts.BucketName, strings.Trim(plan.Facts.Prefix, "/")),
		Condition: map[string]map[string]string{
			"ArnLike": map[string]string{
				"aws:SourceArn": fmt.Sprintf("arn:%s:bcm-data-exports:%s:%s:export/*", plan.Identity.Partition, dataExportsRegion, plan.Identity.AccountID),
			},
			"StringEquals": map[string]string{
				"aws:SourceAccount": plan.Identity.AccountID,
			},
		},
	}
}

func jsonEqual(left []byte, right []byte) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return fmt.Sprintf("%#v", leftValue) == fmt.Sprintf("%#v", rightValue)
}

func equivalentPolicyStatement(raw []byte, expected policyStatement) bool {
	existing, ok := normalizePolicyStatement(raw)
	if !ok {
		return false
	}
	existing.Sid = expected.Sid
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return false
	}
	expectedNormalized, ok := normalizePolicyStatement(expectedJSON)
	if !ok {
		return false
	}
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		return false
	}
	expectedNormalizedJSON, err := json.Marshal(expectedNormalized)
	if err != nil {
		return false
	}
	return jsonEqual(existingJSON, expectedNormalizedJSON)
}

func normalizePolicyStatement(raw []byte) (normalizedPolicyStatement, bool) {
	var parsed flexiblePolicyStatement
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return normalizedPolicyStatement{}, false
	}
	action, ok := normalizeStringList(parsed.Action)
	if !ok {
		return normalizedPolicyStatement{}, false
	}
	resource, ok := normalizeStringList(parsed.Resource)
	if !ok {
		return normalizedPolicyStatement{}, false
	}
	principal := map[string][]string{}
	for key, value := range parsed.Principal {
		values, ok := normalizeStringList(value)
		if !ok {
			return normalizedPolicyStatement{}, false
		}
		principal[key] = values
	}
	return normalizedPolicyStatement{
		Sid:       parsed.Sid,
		Effect:    parsed.Effect,
		Principal: principal,
		Action:    action,
		Resource:  resource,
		Condition: parsed.Condition,
	}, true
}

func normalizeStringList(raw json.RawMessage) ([]string, bool) {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, true
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err == nil {
		normalized := append([]string{}, multiple...)
		sort.Strings(normalized)
		return normalized, true
	}
	return nil, false
}
