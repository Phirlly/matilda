package handoff

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMinimalManifestAllowlist(t *testing.T) {
	manifest := BuildMinimalManifest(Request{
		Assessment:     "rapid-assessment",
		CollectionPath: "billing",
		Provider:       "aws",
		Action:         "package",
		RequiredNextStep: "Provider-specific handoff schemas are not implemented in " +
			"this scaffold.",
		Warnings: []Warning{
			{
				Code:    "provider_schema_required",
				Message: "A provider-specific handoff schema must be verified before archive generation.",
			},
		},
	})

	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) returned error: %v", err)
	}

	var keys map[string]any
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatalf("Unmarshal(manifest) returned error: %v", err)
	}

	allowed := map[string]bool{
		"schema_version":     true,
		"generated_by":       true,
		"assessment":         true,
		"collection_path":    true,
		"provider":           true,
		"action":             true,
		"required_next_step": true,
		"warnings":           true,
	}

	for key := range keys {
		if !allowed[key] {
			t.Fatalf("manifest contains unapproved key %q in %s", key, encoded)
		}
	}

	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"credential",
		"private_key",
		"token",
		"raw_billing",
		"inventory",
		"logs",
		"cloud_state",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("manifest contains forbidden term %q in %s", forbidden, encoded)
		}
	}
}

func TestMinimalManifestOmitsEmptyOptionalFields(t *testing.T) {
	manifest := BuildMinimalManifest(Request{
		Assessment:       "deep-discovery",
		Provider:         "oci",
		Action:           "package",
		RequiredNextStep: "Provider-specific handoff schemas are not implemented in this scaffold.",
	})

	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) returned error: %v", err)
	}

	if strings.Contains(string(encoded), "collection_path") {
		t.Fatalf("manifest includes empty collection_path: %s", encoded)
	}
	if strings.Contains(string(encoded), "warnings") {
		t.Fatalf("manifest includes empty warnings: %s", encoded)
	}
}

func TestMinimalManifestRedactsAllowlistedFields(t *testing.T) {
	manifest := BuildMinimalManifest(Request{
		Assessment:       "rapid-assessment client_secret=plain-client-secret",
		CollectionPath:   "billing",
		Provider:         "aws api_key=plain-api-key",
		Action:           "package",
		RequiredNextStep: "do not expose token=plain-token",
		Warnings: []Warning{
			{
				Code:    "secret_key=plain-secret-key",
				Message: "contains -----BEGIN PRIVATE KEY-----plain-private-key-----END PRIVATE KEY-----",
			},
		},
	})

	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) returned error: %v", err)
	}

	lower := strings.ToLower(string(encoded))
	for _, leaked := range []string{
		"plain-client-secret",
		"plain-api-key",
		"plain-token",
		"plain-secret-key",
		"plain-private-key",
		"begin private key",
	} {
		if strings.Contains(lower, leaked) {
			t.Fatalf("manifest leaked %q in %s", leaked, encoded)
		}
	}
	if !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("manifest = %s, want redaction marker", encoded)
	}
}

func TestStructuredHandoffOutputAllowlist(t *testing.T) {
	output := BuildOutput(Output{
		HandoffType:    "aws_rapid_assessment_billing_cur2",
		Assessment:     "rapid-assessment",
		CollectionPath: "billing",
		Provider:       "aws",
		Summary:        "AWS CUR 2.0 billing handoff is ready.",
		Fields: []Field{
			{Key: "selected_export_ref", Label: "Selected CUR 2.0 export", Value: "cur2-abcdefghijklmnop"},
			{Key: "billing_source", Label: "Billing source", Value: "CUR2.0"},
		},
		NextSteps: []string{
			"Use an AWS cloud account with Skip Configuration in Matilda SaaS.",
			"This tool does not upload billing files to Matilda SaaS.",
		},
	})

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal(output) returned error: %v", err)
	}

	var keys map[string]any
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatalf("Unmarshal(output) returned error: %v", err)
	}

	allowed := map[string]bool{
		"schema_version":  true,
		"generated_by":    true,
		"handoff_type":    true,
		"assessment":      true,
		"collection_path": true,
		"provider":        true,
		"summary":         true,
		"fields":          true,
		"next_steps":      true,
		"warnings":        true,
	}

	for key := range keys {
		if !allowed[key] {
			t.Fatalf("structured handoff output contains unapproved key %q in %s", key, encoded)
		}
	}
	if output.SchemaVersion != "handoff_stdout_v0" {
		t.Fatalf("SchemaVersion = %q, want handoff_stdout_v0", output.SchemaVersion)
	}
	if err := ValidateOutput(&output); err != nil {
		t.Fatalf("ValidateOutput returned error: %v", err)
	}
}

func TestStructuredHandoffOutputRejectsUnsafeValues(t *testing.T) {
	output := BuildOutput(Output{
		HandoffType: "aws_rapid_assessment_billing_cur2",
		Assessment:  "rapid-assessment",
		Provider:    "aws",
		Summary:     "ready",
		Fields: []Field{{
			Key:   "unsafe",
			Label: "Unsafe",
			Value: "arn:aws:iam::123456789012:role/operator",
		}},
	})

	if err := ValidateOutput(&output); err == nil {
		t.Fatal("ValidateOutput accepted unsafe field value")
	}
}

func TestStructuredHandoffOutputRejectsInvalidContractFields(t *testing.T) {
	valid := BuildOutput(Output{
		HandoffType: "aws_rapid_assessment_billing_cur2",
		Assessment:  "rapid-assessment",
		Provider:    "aws",
		Summary:     "ready",
		Fields: []Field{{
			Key:   "selected_export_ref",
			Label: "Selected CUR 2.0 export",
			Value: "cur2-abcdefghijklmnop",
		}},
	})

	tests := []struct {
		name   string
		mutate func(*Output)
	}{
		{name: "unsupported schema", mutate: func(output *Output) { output.SchemaVersion = "provider_v0" }},
		{name: "unsupported generated by", mutate: func(output *Output) { output.GeneratedBy = "other-tool" }},
		{name: "missing handoff type", mutate: func(output *Output) { output.HandoffType = "" }},
		{name: "missing assessment", mutate: func(output *Output) { output.Assessment = "" }},
		{name: "missing provider", mutate: func(output *Output) { output.Provider = "" }},
		{name: "missing summary", mutate: func(output *Output) { output.Summary = "" }},
		{name: "unsafe collection path", mutate: func(output *Output) { output.CollectionPath = "/Users/lly/private" }},
		{name: "missing fields", mutate: func(output *Output) { output.Fields = nil }},
		{name: "empty field key", mutate: func(output *Output) { output.Fields[0].Key = "" }},
		{name: "empty field label", mutate: func(output *Output) { output.Fields[0].Label = "" }},
		{name: "empty field value", mutate: func(output *Output) { output.Fields[0].Value = "" }},
		{name: "unsafe field key", mutate: func(output *Output) { output.Fields[0].Key = "token=plain-token" }},
		{name: "unsafe field label", mutate: func(output *Output) { output.Fields[0].Label = "access_key" }},
		{name: "unsafe next step", mutate: func(output *Output) { output.NextSteps = []string{"Use AKIA1234567890ABCDEF"} }},
		{name: "empty warning code", mutate: func(output *Output) { output.Warnings = []Warning{{Message: "warning"}} }},
		{name: "unsafe warning message", mutate: func(output *Output) {
			output.Warnings = []Warning{{Code: "warning", Message: "arn:aws:iam::123456789012:role/operator"}}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := valid
			output.Fields = append([]Field(nil), valid.Fields...)
			output.NextSteps = append([]string(nil), valid.NextSteps...)
			output.Warnings = append([]Warning(nil), valid.Warnings...)
			tt.mutate(&output)
			if err := ValidateOutput(&output); err == nil {
				t.Fatal("ValidateOutput accepted invalid structured handoff output")
			}
		})
	}
}

func TestStructuredHandoffOutputCompactsEmptyOptionalItems(t *testing.T) {
	output := BuildOutput(Output{
		HandoffType: "aws_rapid_assessment_billing_cur2",
		Assessment:  "rapid-assessment",
		Provider:    "aws",
		Summary:     "ready",
		Fields: []Field{
			{},
			{Key: "selected_export_ref", Label: "Selected CUR 2.0 export", Value: "cur2-abcdefghijklmnop"},
		},
		NextSteps: []string{"", "Use Skip Configuration in Matilda SaaS."},
		Warnings:  []Warning{{}, {Code: "aws_cur2_time_granularity_not_preferred", Message: "Monthly granularity is preferred."}},
	})

	if len(output.Fields) != 1 {
		t.Fatalf("Fields len = %d, want 1", len(output.Fields))
	}
	if len(output.NextSteps) != 1 {
		t.Fatalf("NextSteps len = %d, want 1", len(output.NextSteps))
	}
	if len(output.Warnings) != 1 {
		t.Fatalf("Warnings len = %d, want 1", len(output.Warnings))
	}
	if err := ValidateOutput(&output); err != nil {
		t.Fatalf("ValidateOutput returned error: %v", err)
	}
}

func TestStructuredHandoffOutputRejectsNil(t *testing.T) {
	if err := ValidateOutput(nil); err == nil {
		t.Fatal("ValidateOutput accepted nil output")
	}
}
