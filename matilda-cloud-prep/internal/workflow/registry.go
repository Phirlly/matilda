package workflow

import (
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/handoff"
)

type Status string

const (
	StatusGuided         Status = "guided"
	StatusReady          Status = "ready"
	StatusNotImplemented Status = "not_implemented"
)

type Request struct {
	Goal           assessment.Goal           `json:"goal"`
	CollectionPath assessment.CollectionPath `json:"collection_path,omitempty"`
	Provider       assessment.Provider       `json:"provider"`
	Action         assessment.Action         `json:"action"`
}

type Result struct {
	SchemaVersion                 string            `json:"schema_version"`
	Status                        Status            `json:"status"`
	Code                          string            `json:"code"`
	Message                       string            `json:"message"`
	Mutated                       bool              `json:"mutated"`
	ProviderCapabilityImplemented bool              `json:"provider_capability_implemented"`
	Request                       Request           `json:"request"`
	Manifest                      *handoff.Manifest `json:"manifest,omitempty"`
	Warnings                      []handoff.Warning `json:"warnings,omitempty"`
}

type Registry struct{}

func DefaultRegistry() Registry {
	return Registry{}
}

func (Registry) Execute(request Request) Result {
	if request.Action == assessment.ActionPackage {
		return packageMinimalManifest(request)
	}

	return Result{
		SchemaVersion:                 "matilda_cloud_prep.result_v0",
		Status:                        StatusNotImplemented,
		Code:                          "provider_capability_not_implemented",
		Message:                       "Provider-specific cloud preparation is not implemented in this scaffold.",
		Mutated:                       false,
		ProviderCapabilityImplemented: false,
		Request:                       request,
	}
}

func packageMinimalManifest(request Request) Result {
	warnings := []handoff.Warning{
		{
			Code:    "provider_schema_required",
			Message: "A provider-specific handoff schema must be verified before archive generation.",
		},
	}

	manifest := handoff.BuildMinimalManifest(handoff.Request{
		Assessment:       string(request.Goal),
		CollectionPath:   string(request.CollectionPath),
		Provider:         string(request.Provider),
		Action:           string(request.Action),
		RequiredNextStep: "Provider-specific handoff schemas are not implemented in this scaffold.",
		Warnings:         warnings,
	})

	return Result{
		SchemaVersion:                 "matilda_cloud_prep.result_v0",
		Status:                        StatusReady,
		Code:                          "minimal_manifest_ready",
		Message:                       "Provider-neutral minimal manifest is ready.",
		Mutated:                       false,
		ProviderCapabilityImplemented: false,
		Request:                       request,
		Manifest:                      &manifest,
		Warnings:                      warnings,
	}
}
