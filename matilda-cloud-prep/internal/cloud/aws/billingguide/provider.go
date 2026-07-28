package billingguide

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/authdiscovery"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

type CredentialSourceKind string

const (
	CredentialSourceEnvironment CredentialSourceKind = "environment"
	CredentialSourceProfile     CredentialSourceKind = "profile"
)

type CredentialSource struct {
	Kind                 CredentialSourceKind
	Profile              string
	Region               string
	HasLoginSession      bool
	HasSSOSession        bool
	HasCredentialProcess bool
	HasSourceProfile     bool
	HasRoleARN           bool
}

type VerifiedIdentity struct {
	Source       CredentialSource
	AccountLabel string
	CallerRef    string
	Region       string
}

type VerificationError struct {
	Code    string
	Message string
}

func (err VerificationError) Error() string {
	if strings.TrimSpace(err.Message) != "" {
		return fmt.Sprintf("%s: %s", err.Code, err.Message)
	}
	return err.Code
}

type Discoverer interface {
	Discover(context.Context) ([]authdiscovery.Profile, error)
}

type ClientFactory func(workflow.ExecutionOptions) cur2preflight.Client

type Config struct {
	EnvLookup     func(string) (string, bool)
	Discoverer    Discoverer
	ClientFactory ClientFactory
}

type Provider struct {
	envLookup     func(string) (string, bool)
	discoverer    Discoverer
	clientFactory ClientFactory
}

func New(config Config) Provider {
	envLookup := config.EnvLookup
	if envLookup == nil {
		envLookup = os.LookupEnv
	}
	discoverer := config.Discoverer
	if discoverer == nil {
		discoverer = authdiscovery.Discoverer{EnvLookup: envLookup}
	}
	return Provider{
		envLookup:     envLookup,
		discoverer:    discoverer,
		clientFactory: config.ClientFactory,
	}
}

func (provider Provider) DiscoverCredentialSources(ctx context.Context) ([]CredentialSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sources := []CredentialSource{}
	if provider.hasCredentialEnvironment() {
		sources = append(sources, CredentialSource{Kind: CredentialSourceEnvironment})
	}

	profiles, err := provider.discoverer.Discover(ctx)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if !safeSelectorValue(profile.Name) {
			continue
		}
		source := CredentialSource{
			Kind:                 CredentialSourceProfile,
			Profile:              profile.Name,
			Region:               safeRegion(profile.Region),
			HasLoginSession:      profile.HasLoginSession,
			HasSSOSession:        profile.HasSSOSession,
			HasCredentialProcess: profile.HasCredentialProcess,
			HasSourceProfile:     profile.HasSourceProfile,
			HasRoleARN:           profile.HasRoleARN,
		}
		sources = append(sources, source)
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return sources[i].Kind == CredentialSourceEnvironment
		}
		return sources[i].Profile < sources[j].Profile
	})
	return sources, nil
}

func (provider Provider) VerifyIdentity(ctx context.Context, source CredentialSource) (VerifiedIdentity, error) {
	if provider.clientFactory == nil {
		return VerifiedIdentity{}, VerificationError{Code: "aws_provider_capability_blocked", Message: "AWS identity verification is not configured."}
	}
	options := workflow.ExecutionOptions{
		InterfaceMode: workflow.InterfaceModeGuided,
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				Profile: source.Profile,
				Region:  source.Region,
			},
		},
	}
	normalized, err := workflow.NormalizeExecutionOptions(options)
	if err != nil {
		return VerifiedIdentity{}, VerificationError{Code: "aws_config_invalid_selector", Message: "AWS credential source contains unsafe selector metadata."}
	}

	client := provider.clientFactory(normalized)
	if client == nil {
		return VerifiedIdentity{}, VerificationError{Code: "aws_provider_capability_blocked", Message: "AWS identity verification client is not configured."}
	}
	config, err := client.CheckConfiguration(ctx)
	if err != nil {
		return VerifiedIdentity{}, verificationErrorFromProvider(err, "aws_config_missing_credentials", "AWS credentials are not available.")
	}
	identity, err := client.GetCallerIdentity(ctx)
	if err != nil {
		return VerifiedIdentity{}, verificationErrorFromProvider(err, "aws_auth_failed", "AWS caller identity could not be verified.")
	}
	if strings.TrimSpace(identity.AccountID) == "" || strings.TrimSpace(identity.CallerARN) == "" {
		return VerifiedIdentity{}, VerificationError{Code: "aws_identity_unavailable", Message: "AWS caller identity response was incomplete."}
	}
	region := safeRegion(config.Region)
	if region == "" {
		region = source.Region
	}
	verifiedSource := source
	verifiedSource.Region = region
	return VerifiedIdentity{
		Source:       verifiedSource,
		AccountLabel: maskedAccount(identity.AccountID),
		CallerRef:    hashedRef(identity.CallerARN),
		Region:       region,
	}, nil
}

func (provider Provider) hasCredentialEnvironment() bool {
	for _, name := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_ACCESS_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_WEB_IDENTITY_TOKEN_FILE",
	} {
		if _, ok := provider.envLookup(name); ok {
			return true
		}
	}
	return false
}

func verificationErrorFromProvider(err error, fallback string, message string) VerificationError {
	var providerErr cur2preflight.ProviderError
	if errors.As(err, &providerErr) && providerErr.Code != "" {
		return VerificationError{Code: providerErr.Code, Message: safeErrorMessage(providerErr.Code, message)}
	}
	return VerificationError{Code: fallback, Message: message}
}

func safeErrorMessage(code string, fallback string) string {
	switch code {
	case "aws_config_missing_region":
		return "AWS Region is not configured."
	case "aws_config_missing_credentials":
		return "AWS credentials are not available."
	case "aws_config_timeout":
		return "AWS SDK configuration timed out."
	case "aws_config_cancelled":
		return "AWS SDK configuration was cancelled."
	case "aws_config_profile_shadowed":
		return "AWS profile selection is blocked because credential environment variables would take precedence."
	default:
		return fallback
	}
}

func maskedAccount(accountID string) string {
	trimmed := strings.TrimSpace(accountID)
	if len(trimmed) < 4 {
		return "account-ending-unknown"
	}
	return "account-ending-" + trimmed[len(trimmed)-4:]
}

func hashedRef(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}

func safeRegion(region string) string {
	region = strings.TrimSpace(region)
	if safeSelectorValue(region) {
		return region
	}
	return ""
}

func safeSelectorValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, `/\`) {
		return false
	}
	if sensitiveIdentifierLikeValue(value) {
		return false
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"access_key",
		"apikey",
		"api_key",
		"arn:",
		"bearer ",
		"client-secret",
		"client_secret",
		".pem",
		"ocid1.",
		"passphrase",
		"password",
		"plain-secret",
		"plain-token",
		"private-key",
		"private_key",
		"refresh_token",
		"secret_key",
		"service_account_json",
		"session_token",
		"token=",
	} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return true
}

func sensitiveIdentifierLikeValue(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 12 && allDigits(value) {
		return true
	}
	upper := strings.ToUpper(value)
	return len(upper) == 20 &&
		(strings.HasPrefix(upper, "AKIA") || strings.HasPrefix(upper, "ASIA")) &&
		allUpperAlphaNumeric(upper)
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func allUpperAlphaNumeric(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return value != ""
}
