package handoff

import "github.com/Phirlly/matilda/matilda-cloud-prep/internal/audit"

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

func BuildMinimalManifest(request Request) Manifest {
	return Manifest{
		SchemaVersion:    "minimal_v0",
		GeneratedBy:      "matilda-prep",
		Assessment:       audit.RedactString(request.Assessment),
		CollectionPath:   audit.RedactString(request.CollectionPath),
		Provider:         audit.RedactString(request.Provider),
		Action:           audit.RedactString(request.Action),
		RequiredNextStep: audit.RedactString(request.RequiredNextStep),
		Warnings:         redactWarnings(request.Warnings),
	}
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
