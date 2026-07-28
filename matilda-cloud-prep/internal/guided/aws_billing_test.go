package guided

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestRunAWSBillingAutoSelectsSingleVerifiedSourceAndRunsPreflight(t *testing.T) {
	request := awsBillingRequest()
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		if got != request {
			t.Fatalf("request = %#v, want %#v", got, request)
		}
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {
				Source:       billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"},
				AccountLabel: "account-ending-9012",
				CallerRef:    "sha256:abcdef123456",
				Region:       "us-east-1",
			},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"Connect AWS account",
		"account-ending-9012",
		"Continue with this AWS account? [y/N]",
		"Inspect AWS CUR 2.0 billing exports",
		"AWS CUR 2.0 billing preflight is ready.",
		"matilda-prep rapid-assessment billing aws preflight --profile default --region us-east-1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if gotOptions.InterfaceMode != workflow.InterfaceModeGuided {
		t.Fatalf("InterfaceMode = %q, want guided", gotOptions.InterfaceMode)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil {
		t.Fatalf("AWS selectors missing: %#v", gotOptions)
	}
	if gotOptions.Selectors.AWS.Profile != "default" || gotOptions.Selectors.AWS.Region != "us-east-1" {
		t.Fatalf("AWS selectors = %#v, want profile default and region us-east-1", gotOptions.Selectors.AWS)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingUsesEnvironmentCredentialSourceSafely(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceEnvironment, Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"environment": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceEnvironment, Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"source environment credentials in us-east-1",
		"matilda-prep rapid-assessment billing aws preflight --region us-east-1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil || gotOptions.Selectors.AWS.Profile != "" || gotOptions.Selectors.AWS.Region != "us-east-1" {
		t.Fatalf("AWS selectors = %#v, want environment source region only", gotOptions)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingUsesEnvironmentCredentialSourceWithoutRegion(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceEnvironment}},
		verified: map[string]billingguide.VerifiedIdentity{
			"environment": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceEnvironment}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if !strings.Contains(output, "source environment credentials") {
		t.Fatalf("output = %q, want environment credential source", output)
	}
	if strings.Contains(output, "--profile") || strings.Contains(output, "--region") {
		t.Fatalf("environment source without selectors should not print selector flags: %s", output)
	}
	if gotOptions.Selectors != nil {
		t.Fatalf("AWS selectors = %#v, want none for environment source without region", gotOptions.Selectors)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingPromptsForMultipleVerifiedSources(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{
			{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"},
			{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"},
		},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-1111", CallerRef: "sha256:111111111111", Region: "us-east-1"},
			"profile:finance": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n2\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"Select AWS account [1-2]",
		"profile default",
		"profile finance",
		"account-ending-2222",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil || gotOptions.Selectors.AWS.Profile != "finance" {
		t.Fatalf("AWS selectors = %#v, want selected finance profile", gotOptions)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingDoesNotVerifyNonSelectedCredentialProcessProfile(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{
			{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"},
			{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true},
		},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-1111", CallerRef: "sha256:111111111111", Region: "us-east-1"},
			"profile:process": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:default" {
		t.Fatalf("VerifyIdentity calls = %q, want only selected safe source preverified", got)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil || gotOptions.Selectors.AWS.Profile != "default" {
		t.Fatalf("AWS selectors = %#v, want selected default profile", gotOptions)
	}
	if !strings.Contains(output, "profile process in us-west-2 with credential process") {
		t.Fatalf("output = %q, want credential-process profile listed without verification", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingCredentialProcessProfileRequiresConfirmationBeforeVerification(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{
			{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"},
			{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true},
		},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-1111", CallerRef: "sha256:111111111111", Region: "us-east-1"},
			"profile:process": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n2\nn\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when user declines credential-process verification")
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:default" {
		t.Fatalf("VerifyIdentity calls = %q, want credential-process source not verified after decline", got)
	}
	if !strings.Contains(output, "Verify selected AWS credential source now? [y/N]") {
		t.Fatalf("output = %q, want explicit verification confirmation", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingSelectedCredentialProcessVerifiesThenRequiresIdentityConfirmation(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:process": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\nn\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when user declines verified credential-process identity")
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:process" {
		t.Fatalf("VerifyIdentity calls = %q, want selected credential-process source verified once", got)
	}
	for _, want := range []string{
		"AWS credential source requires verification: profile process in us-west-2 with credential process",
		"Verify this AWS credential source now? [y/N]",
		"AWS account verified: account-ending-2222",
		"Continue with this AWS account? [y/N]",
		"AWS billing preflight was not run.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingSelectedCredentialProcessRunsPreflightAfterIdentityConfirmation(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:process": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:process" {
		t.Fatalf("VerifyIdentity calls = %q, want selected credential-process source verified once", got)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil || gotOptions.Selectors.AWS.Profile != "process" {
		t.Fatalf("AWS selectors = %#v, want selected credential-process profile", gotOptions)
	}
	if !strings.Contains(output, "Inspect AWS CUR 2.0 billing exports") {
		t.Fatalf("output = %q, want CUR inspection after confirmation", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingSelectedCredentialProcessFailureUsesCodeSpecificRemediation(t *testing.T) {
	registryCalled := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		registryCalled = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "process", Region: "us-west-2", HasCredentialProcess: true, HasLoginSession: true}},
		verifyErrs: map[string]error{
			"profile:process": billingguide.VerificationError{Code: "aws_config_missing_credentials", Message: "AWS credentials are not available."},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if registryCalled {
		t.Fatal("preflight registry should not run when selected credential-process verification fails")
	}
	for _, want := range []string{
		"No verified AWS credential source is available.",
		"profile process in us-west-2 with login session with credential process blocked: aws_config_missing_credentials",
		"aws login --profile process",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingBlocksUnsafeVerifiedSelectorBeforePreflight(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "safe", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:safe": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "/Users/example/.aws/private", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when verified source metadata is unsafe")
	}
	for _, want := range []string{
		"No verified AWS credential source is available.",
		"profile safe in us-east-1 blocked: aws_config_invalid_selector",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingBlocksUnsafeFailedSourceBeforeDisplay(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "/Users/example/.aws/private", Region: "us-east-1", HasLoginSession: true}},
		verifyErrs: map[string]error{
			"profile:/Users/example/.aws/private": billingguide.VerificationError{Code: "aws_auth_failed", Message: "AWS caller identity could not be verified."},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when credential source metadata is unsafe")
	}
	if !strings.Contains(output, "AWS credential source blocked: aws_config_invalid_selector") {
		t.Fatalf("output = %q, want generic unsafe blocked source message", output)
	}
	if strings.Contains(output, "aws login --profile") {
		t.Fatalf("unsafe profile should not be shown in login remediation: %s", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingStopsBeforePreflightWhenIdentityCannotBeVerified(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1", HasLoginSession: true}},
		verifyErrs: map[string]error{
			"profile:default": billingguide.VerificationError{Code: "aws_config_missing_credentials", Message: "AWS credentials are not available."},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when identity is unavailable")
	}
	for _, want := range []string{
		"No verified AWS credential source is available.",
		"aws login --profile default",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingBlockedEnvironmentCredentialHasNoLoginRemediation(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceEnvironment}},
		verifyErrs: map[string]error{
			"environment": billingguide.VerificationError{Code: "aws_auth_failed", Message: "AWS caller identity could not be verified."},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when environment credentials cannot be verified")
	}
	if !strings.Contains(output, "environment credentials blocked: aws_auth_failed") {
		t.Fatalf("output = %q, want blocked environment credential", output)
	}
	if strings.Contains(output, "aws login --profile") {
		t.Fatalf("environment credentials should not show profile login remediation: %s", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingLoginRemediationOnlyForMissingCredentials(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "missing region", err: billingguide.VerificationError{Code: "aws_config_missing_region", Message: "AWS Region is not configured."}, code: "aws_config_missing_region"},
		{name: "profile shadowed", err: billingguide.VerificationError{Code: "aws_config_profile_shadowed", Message: "AWS profile selection is blocked."}, code: "aws_config_profile_shadowed"},
		{name: "configuration timeout", err: billingguide.VerificationError{Code: "aws_config_timeout", Message: "AWS SDK configuration timed out."}, code: "aws_config_timeout"},
		{name: "configuration cancelled", err: billingguide.VerificationError{Code: "aws_config_cancelled", Message: "AWS SDK configuration was cancelled."}, code: "aws_config_cancelled"},
		{name: "auth failed", err: billingguide.VerificationError{Code: "aws_auth_failed", Message: "AWS caller identity could not be verified."}, code: "aws_auth_failed"},
		{name: "transient", err: billingguide.VerificationError{Code: "aws_data_exports_transient", Message: "AWS did not respond in time."}, code: "aws_data_exports_transient"},
		{name: "generic error", err: errors.New("generic provider error"), code: "aws_auth_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
				called = true
				return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
			}))
			guide := &fakeAWSBillingGuide{
				sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1", HasLoginSession: true}},
				verifyErrs: map[string]error{
					"profile:default": tt.err,
				},
			}

			output, err := runGuidedWithConfig("1\n1\n", Config{Registry: registry, AWSBilling: guide})

			if err != nil {
				t.Fatalf("RunWithConfig returned error: %v", err)
			}
			if called {
				t.Fatal("preflight registry should not run when identity is unavailable")
			}
			if !strings.Contains(output, tt.code) {
				t.Fatalf("output = %q, want blocked code %q", output, tt.code)
			}
			if strings.Contains(output, "aws login --profile") {
				t.Fatalf("output should not show login remediation for %s: %s", tt.code, output)
			}
			assertGuidedOutputSafe(t, output)
		})
	}
}

func TestRunAWSBillingStopsBeforePreflightWhenNoCredentialSourcesExist(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{}

	output, err := runGuidedWithConfig("1\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when no AWS credential sources exist")
	}
	for _, want := range []string{
		"No AWS credential sources were found.",
		"Sign in or configure an AWS profile",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingDiscoveryFailureDoesNotLeakRawProviderError(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{discoverErr: errors.New("raw arn:aws:iam::123456789012:role/operator from /Users/example/.aws/config")}

	output, err := runGuidedWithConfig("1\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when AWS credential discovery fails")
	}
	if !strings.Contains(output, "AWS credential discovery could not complete.") {
		t.Fatalf("output = %q, want generic discovery failure", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingDiscoveryTimeoutShowsRetryableMessage(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{discoverErr: context.DeadlineExceeded}

	output, err := runGuidedWithConfig("1\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when AWS credential discovery times out")
	}
	if !strings.Contains(output, "AWS credential discovery timed out.") {
		t.Fatalf("output = %q, want retryable timeout message", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingDeclineStopsBeforePreflight(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\nn\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if called {
		t.Fatal("preflight registry should not run when user declines the verified account")
	}
	if !strings.Contains(output, "AWS billing preflight was not run.") {
		t.Fatalf("output = %q, want declined preflight message", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingInvalidConfirmationReturnsSelectionError(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\nmaybe\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("RunWithConfig error = %v, want ErrInvalidSelection", err)
	}
	if called {
		t.Fatal("preflight registry should not run after invalid confirmation")
	}
	if !strings.Contains(err.Error(), "expected y or n") {
		t.Fatalf("error = %q, want confirmation guidance", err.Error())
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingInvalidConfirmationDoesNotEchoRawInput(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}
	raw := "arn:aws:iam::123456789012:role/operator"

	output, err := runGuidedWithConfig("1\n1\n"+raw+"\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("RunWithConfig error = %v, want ErrInvalidSelection", err)
	}
	if called {
		t.Fatal("preflight registry should not run after invalid confirmation")
	}
	for _, forbidden := range []string{raw, "arn:aws", "123456789012", "operator"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("error leaked raw confirmation value %q: %v", forbidden, err)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingCancelledConfirmationReturnsCancelled(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run after cancelled confirmation")
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingEOFConfirmationReturnsCancelled(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run after EOF at confirmation")
	}
	if !strings.Contains(err.Error(), "guided setup cancelled before confirmation") {
		t.Fatalf("error = %q, want confirmation cancellation guidance", err.Error())
	}
	assertGuidedOutputSafe(t, output)
}

func TestDirectAWSBillingCommandShellQuotesUnsafeSelectorCharacters(t *testing.T) {
	command := directAWSBillingCommand(
		billingguide.CredentialSource{
			Kind:    billingguide.CredentialSourceProfile,
			Profile: "prod;date",
			Region:  "us-east-1",
		},
		"cur2-1234abcd5678ef90",
	)

	if !strings.Contains(command, "--profile 'prod;date'") {
		t.Fatalf("command = %q, want shell-quoted profile selector", command)
	}
	if strings.Contains(command, "--profile prod;date") {
		t.Fatalf("command left shell metacharacter unquoted: %q", command)
	}
}

func runGuidedWithConfig(input string, config Config) (string, error) {
	var output bytes.Buffer
	err := RunWithConfig(strings.NewReader(input), &output, config)
	return output.String(), err
}

func testRegistry(t *testing.T, runner workflow.CapabilityRunner) workflow.Registry {
	t.Helper()
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: awsBillingRequest(),
		Runner:  runner,
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	return registry
}

func guidedCapabilityReport(request workflow.Request, status workflow.RunStatus, code string, evidence []workflow.PlanEvidence) workflow.CapabilityReport {
	checkStatus := workflow.CheckPass
	support := workflow.SupportSupported
	if status == workflow.RunStatusBlocked || status == workflow.RunStatusFailed {
		checkStatus = workflow.CheckFail
		support = workflow.SupportBlocked
	}
	check := workflow.PlanCheck{
		Status:  checkStatus,
		Title:   "AWS CUR 2.0 preflight",
		Message: "AWS CUR 2.0 preflight test result.",
		Evidence: append([]workflow.PlanEvidence{
			{Key: "code", Value: code},
		}, evidence...),
		SourceHandles: guidedTestSourceHandles(),
	}
	return workflow.CapabilityReport{
		Status:        status,
		SupportStatus: support,
		Code:          code,
		Message:       guidedTestMessage(code),
		Mutated:       false,
		SourceHandles: guidedTestSourceHandles(),
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified with account-ending-9012 and caller hash sha256:abcdef123456.",
				SourceHandles:  guidedTestSourceHandles(),
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageUnknown,
				Summary:        "AWS billing coverage is evaluated from the selected CUR 2.0 export and account context.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Checks:              []workflow.PlanCheck{check},
			Steps: []workflow.PlanStep{{
				Intent:                    workflow.PlanStepReuse,
				Title:                     "Review existing AWS CUR 2.0 export",
				Description:               "Use read-only AWS checks.",
				Reason:                    "Matilda requires billing data.",
				ApprovalKind:              "not_required",
				CurrentState:              "Existing export is visible.",
				TargetState:               "Export satisfies preflight.",
				RequiredPermission:        "read-only",
				CredentialMaterialTouched: false,
				Validation:                "Read-only validation.",
				Rollback:                  "No cloud change is made.",
				SourceHandles:             guidedTestSourceHandles(),
			}},
			SourceHandles: guidedTestSourceHandles(),
		},
	}
}

func guidedTestMessage(code string) string {
	if code == "aws_cur2_preflight_ready" {
		return "AWS CUR 2.0 billing preflight is ready."
	}
	return "AWS CUR 2.0 billing preflight test result."
}

func guidedTestSourceHandles() []workflow.SourceHandle {
	return []workflow.SourceHandle{{Label: "AWS test reference", URI: "docs/references/aws/aws-cur2-export-selection-guided-ux.md"}}
}

type fakeAWSBillingGuide struct {
	sources     []billingguide.CredentialSource
	discoverErr error
	verified    map[string]billingguide.VerifiedIdentity
	verifyErrs  map[string]error
	verifyCalls []string
}

func (f *fakeAWSBillingGuide) DiscoverCredentialSources(context.Context) ([]billingguide.CredentialSource, error) {
	return f.sources, f.discoverErr
}

func (f *fakeAWSBillingGuide) VerifyIdentity(_ context.Context, source billingguide.CredentialSource) (billingguide.VerifiedIdentity, error) {
	key := sourceKey(source)
	f.verifyCalls = append(f.verifyCalls, key)
	if err := f.verifyErrs[key]; err != nil {
		return billingguide.VerifiedIdentity{}, err
	}
	identity, ok := f.verified[key]
	if !ok {
		return billingguide.VerifiedIdentity{}, billingguide.VerificationError{Code: "aws_auth_failed", Message: "AWS caller identity could not be verified."}
	}
	return identity, nil
}

func sourceKey(source billingguide.CredentialSource) string {
	if source.Kind == billingguide.CredentialSourceProfile {
		return "profile:" + source.Profile
	}
	return "environment"
}

func assertGuidedOutputSafe(t *testing.T, output string) {
	t.Helper()
	for _, forbidden := range []string{
		"arn:aws",
		"123456789012",
		"AKIA",
		"access_key",
		"secret_key",
		"session_token",
		"/Users/",
		"/private/",
		"request id",
		"host id",
	} {
		if strings.Contains(strings.ToLower(output), strings.ToLower(forbidden)) {
			t.Fatalf("guided output leaked forbidden value %q in %s", forbidden, output)
		}
	}
}
