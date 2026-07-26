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
