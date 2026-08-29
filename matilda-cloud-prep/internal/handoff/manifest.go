package handoff

import (
	"fmt"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/audit"
)

const (
	MinimalManifestSchemaVersion = "minimal_v0"
	StdoutHandoffSchemaVersion   = "handoff_stdout_v0"
	GeneratedBy                  = "matilda-prep"
)

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Request struct {
	Assessment       string
	CollectionPath   string
	Provider         string
	Action           string
	RequiredNextStep string
	Warnings         []Warning
}

type Manifest struct {
	SchemaVersion    string    `json:"schema_version"`
	GeneratedBy      string    `json:"generated_by"`
	Assessment       string    `json:"assessment"`
	CollectionPath   string    `json:"collection_path,omitempty"`
	Provider         string    `json:"provider"`
	Action           string    `json:"action"`
	RequiredNextStep string    `json:"required_next_step"`
	Warnings         []Warning `json:"warnings,omitempty"`
}

type Field struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type Output struct {
	SchemaVersion  string    `json:"schema_version"`
	GeneratedBy    string    `json:"generated_by"`
	HandoffType    string    `json:"handoff_type"`
	Assessment     string    `json:"assessment"`
	CollectionPath string    `json:"collection_path,omitempty"`
	Provider       string    `json:"provider"`
	Summary        string    `json:"summary"`
	Fields         []Field   `json:"fields,omitempty"`
	NextSteps      []string  `json:"next_steps,omitempty"`
	Warnings       []Warning `json:"warnings,omitempty"`
}

func BuildMinimalManifest(request Request) Manifest {
	return Manifest{
		SchemaVersion:    MinimalManifestSchemaVersion,
		GeneratedBy:      GeneratedBy,
		Assessment:       audit.RedactString(request.Assessment),
		CollectionPath:   audit.RedactString(request.CollectionPath),
		Provider:         audit.RedactString(request.Provider),
		Action:           audit.RedactString(request.Action),
		RequiredNextStep: audit.RedactString(request.RequiredNextStep),
		Warnings:         redactWarnings(request.Warnings),
	}
}

func BuildOutput(output Output) Output {
	output.SchemaVersion = StdoutHandoffSchemaVersion
	output.GeneratedBy = GeneratedBy
	output.HandoffType = strings.TrimSpace(output.HandoffType)
	output.Assessment = strings.TrimSpace(output.Assessment)
	output.CollectionPath = strings.TrimSpace(output.CollectionPath)
	output.Provider = strings.TrimSpace(output.Provider)
	output.Summary = strings.TrimSpace(output.Summary)
	output.Fields = compactFields(output.Fields)
	output.NextSteps = compactStringList(output.NextSteps)
	output.Warnings = compactWarnings(output.Warnings)
	return output
}

func ValidateOutput(output *Output) error {
	if output == nil {
		return fmt.Errorf("handoff output is required")
	}
	if output.SchemaVersion != StdoutHandoffSchemaVersion {
		return fmt.Errorf("handoff output schema_version is unsupported")
	}
	if output.GeneratedBy != GeneratedBy {
		return fmt.Errorf("handoff output generated_by is unsupported")
	}
	for name, value := range map[string]string{
		"handoff_type": output.HandoffType,
		"assessment":   output.Assessment,
		"provider":     output.Provider,
		"summary":      output.Summary,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("handoff output %s is required", name)
		}
		if unsafeOutputText(value) {
			return fmt.Errorf("handoff output %s contains unsafe content", name)
		}
	}
	if output.CollectionPath != "" && unsafeOutputText(output.CollectionPath) {
		return fmt.Errorf("handoff output collection_path contains unsafe content")
	}
	if len(output.Fields) == 0 {
		return fmt.Errorf("handoff output fields are required")
	}
	for index, field := range output.Fields {
		if strings.TrimSpace(field.Key) == "" {
			return fmt.Errorf("handoff output field %d key is required", index)
		}
		if strings.TrimSpace(field.Label) == "" {
			return fmt.Errorf("handoff output field %d label is required", index)
		}
		if strings.TrimSpace(field.Value) == "" {
			return fmt.Errorf("handoff output field %d value is required", index)
		}
		if unsafeOutputText(field.Key) || unsafeOutputText(field.Label) || unsafeOutputText(field.Value) {
			return fmt.Errorf("handoff output field %d contains unsafe content", index)
		}
	}
	for index, nextStep := range output.NextSteps {
		if strings.TrimSpace(nextStep) == "" {
			return fmt.Errorf("handoff output next_step %d is required", index)
		}
		if unsafeOutputText(nextStep) {
			return fmt.Errorf("handoff output next_step %d contains unsafe content", index)
		}
	}
	for index, warning := range output.Warnings {
		if strings.TrimSpace(warning.Code) == "" {
			return fmt.Errorf("handoff output warning %d code is required", index)
		}
		if strings.TrimSpace(warning.Message) == "" {
			return fmt.Errorf("handoff output warning %d message is required", index)
		}
		if unsafeOutputText(warning.Code) || unsafeOutputText(warning.Message) {
			return fmt.Errorf("handoff output warning %d contains unsafe content", index)
		}
	}
	return nil
}

func redactWarnings(warnings []Warning) []Warning {
	if len(warnings) == 0 {
		return nil
	}

	redacted := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		redacted = append(redacted, Warning{
			Code:    audit.RedactString(warning.Code),
			Message: audit.RedactString(warning.Message),
		})
	}
	return redacted
}

func compactFields(fields []Field) []Field {
	if len(fields) == 0 {
		return nil
	}
	copied := make([]Field, 0, len(fields))
	for _, field := range fields {
		field.Key = strings.TrimSpace(field.Key)
		field.Label = strings.TrimSpace(field.Label)
		field.Value = strings.TrimSpace(field.Value)
		if field.Key == "" && field.Label == "" && field.Value == "" {
			continue
		}
		copied = append(copied, field)
	}
	return copied
}

func compactStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	copied := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		copied = append(copied, value)
	}
	return copied
}

func compactWarnings(warnings []Warning) []Warning {
	if len(warnings) == 0 {
		return nil
	}
	copied := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		warning.Code = strings.TrimSpace(warning.Code)
		warning.Message = strings.TrimSpace(warning.Message)
		if warning.Code == "" && warning.Message == "" {
			continue
		}
		copied = append(copied, warning)
	}
	return copied
}

func unsafeOutputText(value string) bool {
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"/users/",
		"/private/",
		"/tmp/",
		"/home/",
		"\\",
		".pem",
		"access_key",
		"apikey",
		"api_key",
		"arn:",
		"bearer ",
		"client-secret",
		"client_secret",
		"customer_name",
		"org_name",
		"ocid1.",
		"passphrase",
		"password",
		"plain-secret",
		"plain-token",
		"private-key",
		"private_key",
		"raw_billing",
		"refresh_token",
		"secret_key",
		"service_account_json",
		"session_token",
		"token=",
		"x-amz-",
	} {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	return containsTwelveConsecutiveDigits(value) || containsAWSAccessKeyLike(value)
}

func containsTwelveConsecutiveDigits(value string) bool {
	run := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			run++
			if run >= 12 {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func containsAWSAccessKeyLike(value string) bool {
	upper := strings.ToUpper(value)
	for index := 0; index+20 <= len(upper); index++ {
		candidate := upper[index : index+20]
		if (strings.HasPrefix(candidate, "AKIA") || strings.HasPrefix(candidate, "ASIA")) && allUpperAlphaNumeric(candidate) {
			return true
		}
	}
	return false
}

func allUpperAlphaNumeric(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return value != ""
}
