package assessment

import (
	"fmt"
	"strings"
)

type Goal string

const (
	RapidAssessment Goal = "rapid-assessment"
	DeepDiscovery   Goal = "deep-discovery"
)

type CollectionPath string

const (
	CollectionBilling CollectionPath = "billing"
	CollectionAPI     CollectionPath = "api"
)

type Provider string

const (
	ProviderAWS   Provider = "aws"
	ProviderAzure Provider = "azure"
	ProviderGCP   Provider = "gcp"
	ProviderOCI   Provider = "oci"
)

type Action string

const (
	ActionPreflight    Action = "preflight"
	ActionApplyPrereqs Action = "apply-prereqs"
	ActionValidate     Action = "validate"
	ActionPackage      Action = "package"
)

func ParseGoal(value string) (Goal, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch Goal(normalized) {
	case RapidAssessment, DeepDiscovery:
		return Goal(normalized), nil
	default:
		return "", fmt.Errorf("invalid goal %q: expected rapid-assessment or deep-discovery", value)
	}
}

func ParseCollectionPath(value string) (CollectionPath, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch CollectionPath(normalized) {
	case CollectionBilling, CollectionAPI:
		return CollectionPath(normalized), nil
	default:
		return "", fmt.Errorf("invalid collection path %q: expected billing or api", value)
	}
}

func ParseProvider(value string) (Provider, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch Provider(normalized) {
	case ProviderAWS, ProviderAzure, ProviderGCP, ProviderOCI:
		return Provider(normalized), nil
	default:
		return "", fmt.Errorf("invalid provider %q: expected aws, azure, gcp, or oci", value)
	}
}

func ParseAction(value string) (Action, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch Action(normalized) {
	case ActionPreflight, ActionApplyPrereqs, ActionValidate, ActionPackage:
		return Action(normalized), nil
	default:
		return "", fmt.Errorf("invalid action %q: expected preflight, apply-prereqs, validate, or package", value)
	}
}

func IsProvider(value string) bool {
	_, err := ParseProvider(value)
	return err == nil
}
