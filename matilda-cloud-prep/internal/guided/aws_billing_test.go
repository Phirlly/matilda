package guided

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

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

func TestRunAWSBillingSummarySeparatesReadinessFromSupportCode(t *testing.T) {
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_delivery_not_started", nil)
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
		"Result: ready",
		"Support code: aws_cur2_delivery_not_started",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if strings.Contains(output, "Result: ready (aws_cur2_delivery_not_started)") {
		t.Fatalf("output combines readiness and support code: %s", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingSummaryRendersPlanFactsAndDynamicNextAction(t *testing.T) {
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		return guidedCapabilityReport(got, workflow.RunStatusManualSteps, "aws_backfill_manual_step_required", []workflow.PlanEvidence{
			{Key: "output_format", Value: "TEXT_OR_CSV"},
			{Key: "compression", Value: "GZIP"},
			{Key: "time_granularity", Value: "DAILY"},
			{Key: "overwrite", Value: "CREATE_NEW_REPORT"},
			{Key: "previous_billing_period", Value: "2026-06"},
			{Key: "missing_previous_month_component", Value: "data_partition"},
			{Key: "missing_previous_month_component", Value: "manifest"},
		})
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
		"Result: ready_with_manual_steps",
		"Support code: aws_backfill_manual_step_required",
		"Readiness: manual step required",
		"Export: TEXT_OR_CSV / GZIP, DAILY, CREATE_NEW_REPORT",
		"Previous month: 2026-06 missing data partition, manifest",
		"Next action: request or complete previous-month billing data backfill, then rerun preflight.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingSummaryRendersBlockedPolicyAccessAsNonReady(t *testing.T) {
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_s3_bucket_policy_inaccessible", []workflow.PlanEvidence{
			{Key: "output_format", Value: "TEXT_OR_CSV"},
			{Key: "compression", Value: "GZIP"},
			{Key: "time_granularity", Value: "MONTHLY"},
			{Key: "previous_billing_period", Value: "2026-06"},
			{Key: "missing_previous_month_component", Value: "manifest"},
		})
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
		"Result: blocked",
		"Support code: aws_s3_bucket_policy_inaccessible",
		"Readiness: not ready",
		"S3 delivery policy: not inspected",
		"Previous month: 2026-06 missing manifest",
		"Next action: grant read access to inspect the S3 bucket policy, then rerun preflight.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if strings.Contains(output, "Readiness: repair required") {
		t.Fatalf("summary used stale repairable wording for inaccessible policy: %s", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestWriteAWSBillingSummaryShowsBackfillApplyPrereqsCommand(t *testing.T) {
	var output bytes.Buffer
	result := workflow.Result{
		Status:  workflow.RunStatusManualSteps,
		Code:    "aws_backfill_manual_step_required",
		Message: "AWS CUR 2.0 billing preflight requires previous-month billing backfill or manual remediation.",
		ExecutionOptions: workflow.ExecutionOptions{
			Selectors: &workflow.ExecutionSelectors{
				AWS: &workflow.AWSExecutionSelectors{
					CUR2ExportRef: "cur2-abcdefghijklmnop",
				},
			},
		},
	}

	writeAWSBillingSummary(&output, billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "default",
		Region:  "us-east-1",
	}, result)

	got := output.String()
	for _, want := range []string{
		"Next command:",
		"matilda-prep rapid-assessment billing aws apply-prereqs --profile default --region us-east-1 --export-ref cur2-abcdefghijklmnop --request-backfill",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want to contain %q", got, want)
		}
	}
	if strings.Contains(got, "Reproduce with:") || strings.Contains(got, "rapid-assessment billing aws preflight") {
		t.Fatalf("backfill summary printed preflight reproduction instead of next command: %s", got)
	}
	assertGuidedOutputSafe(t, got)
}

func TestWriteAWSBillingSummaryShowsCreateCUR2ExportCommandWhenNoCURExists(t *testing.T) {
	var output bytes.Buffer
	result := workflow.Result{
		Status:  workflow.RunStatusBlocked,
		Code:    "aws_cur2_export_not_found",
		Message: "No AWS CUR 2.0 export was found.",
	}

	writeAWSBillingSummary(&output, billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "default",
		Region:  "us-east-1",
	}, result)

	got := output.String()
	for _, want := range []string{
		"Next command:",
		"matilda-prep rapid-assessment billing aws apply-prereqs --profile default --region us-east-1 --create-cur2-export",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want to contain %q", got, want)
		}
	}
	if strings.Contains(got, "--export-ref") || strings.Contains(got, "rapid-assessment billing aws preflight") {
		t.Fatalf("no-CUR summary printed wrong follow-up command: %s", got)
	}
	assertGuidedOutputSafe(t, got)
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

	output, err := runGuidedWithConfig("1\n1\n2\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"Select AWS account [1-4]",
		"profile default",
		"profile finance",
		"account-ending-2222",
		"Continue with this AWS account? [y/N]",
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

func TestRunAWSBillingDecliningSingleAccountCanRescanAfterExternalSignIn(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sourceSequences: [][]billingguide.CredentialSource{
			{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
			{{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}},
		},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-1111", CallerRef: "sha256:111111111111", Region: "us-east-1"},
			"profile:finance": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\nn\n2\n\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"Choose how to connect another AWS account.",
		"Sign in or configure another AWS profile, then re-scan",
		"Use an existing AWS profile name manually (advanced)",
		"Sign in or configure the AWS profile for the account you want outside this tool.",
		"Press Enter after the AWS profile is ready to re-scan",
		"Re-scanning safe local AWS credential sources.",
		"account-ending-2222",
		"Inspect AWS CUR 2.0 billing exports",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil ||
		gotOptions.Selectors.AWS.Profile != "finance" ||
		gotOptions.Selectors.AWS.Region != "us-west-2" {
		t.Fatalf("AWS selectors = %#v, want re-scanned finance/us-west-2", gotOptions)
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:default,profile:finance" {
		t.Fatalf("VerifyIdentity calls = %q, want default then re-scanned finance", got)
	}
	if guide.discoverCalls != 2 {
		t.Fatalf("DiscoverCredentialSources calls = %d, want 2", guide.discoverCalls)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingDecliningMultipleAccountLoopsToAnotherProfile(t *testing.T) {
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

	output, err := runGuidedWithConfig("1\n1\n1\nn\n2\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"Select AWS account [1-4]",
		"Continue with this AWS account? [y/N]",
		"Choose how to connect another AWS account.",
		"account-ending-2222",
		"Inspect AWS CUR 2.0 billing exports",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil || gotOptions.Selectors.AWS.Profile != "finance" {
		t.Fatalf("AWS selectors = %#v, want selected finance profile after loop", gotOptions)
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

	output, err := runGuidedWithConfig("1\n1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

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

	output, err := runGuidedWithConfig("1\n1\n2\nn\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
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
	if !strings.Contains(output, "Choose how to connect another AWS account.") {
		t.Fatalf("output = %q, want source reselection guidance", output)
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

	output, err := runGuidedWithConfig("1\n1\ny\nn\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
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
		"Choose how to connect another AWS account.",
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

	output, err := runGuidedWithConfig("1\n1\ny\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if registryCalled {
		t.Fatal("preflight registry should not run when selected credential-process verification fails")
	}
	for _, want := range []string{
		"No verified AWS credential source is available.",
		"profile process in us-west-2 with login session with credential process blocked: aws_config_missing_credentials",
		"aws login --profile process",
		"Choose how to connect another AWS account.",
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

	output, err := runGuidedWithConfig("1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when verified source metadata is unsafe")
	}
	for _, want := range []string{
		"No verified AWS credential source is available.",
		"profile safe in us-east-1 blocked: aws_config_invalid_selector",
		"Use an existing AWS profile name manually (advanced)",
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

	output, err := runGuidedWithConfig("1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
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
	if !strings.Contains(output, "Use an existing AWS profile name manually (advanced)") {
		t.Fatalf("output = %q, want manual profile option", output)
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

	output, err := runGuidedWithConfig("1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when identity is unavailable")
	}
	for _, want := range []string{
		"No verified AWS credential source is available.",
		"aws login --profile default",
		"Use an existing AWS profile name manually (advanced)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingCanRunAWSLoginForMissingLoginProfileAndRescan(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sourceSequences: [][]billingguide.CredentialSource{
			{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1", HasLoginSession: true}},
			{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1", HasLoginSession: true}},
		},
		verifyErrSequences: map[string][]error{
			"profile:default": {
				billingguide.VerificationError{Code: "aws_config_missing_credentials", Message: "AWS credentials are not available."},
				nil,
			},
		},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1", HasLoginSession: true}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-east-1"},
		},
	}
	login := &fakeAWSLoginRunner{support: AWSLoginSupport{Available: true, Version: "2.36.34"}}

	output, err := runGuidedWithConfig("1\n1\n1\ny\ny\n", Config{Registry: registry, AWSBilling: guide, AWSLogin: login})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(login.loginCalls) != 1 {
		t.Fatalf("login calls = %#v, want one AWS login call", login.loginCalls)
	}
	if login.loginCalls[0].Profile != "default" || login.loginCalls[0].Region != "us-east-1" {
		t.Fatalf("login source = %#v, want default/us-east-1", login.loginCalls[0])
	}
	if guide.discoverCalls != 2 {
		t.Fatalf("DiscoverCredentialSources calls = %d, want initial discovery plus re-scan", guide.discoverCalls)
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:default,profile:default" {
		t.Fatalf("VerifyIdentity calls = %q, want failed verification then re-scan verification", got)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil ||
		gotOptions.Selectors.AWS.Profile != "default" ||
		gotOptions.Selectors.AWS.Region != "us-east-1" {
		t.Fatalf("AWS selectors = %#v, want re-scanned default/us-east-1", gotOptions)
	}
	for _, want := range []string{
		"No verified AWS credential source is available.",
		"Sign in to profile default, then re-scan",
		"Run AWS login for profile default now? [y/N]",
		"AWS login completed. Re-scanning safe local AWS credential sources.",
		"Re-scanning safe local AWS credential sources.",
		"AWS account verified: account-ending-2222",
		"Inspect AWS CUR 2.0 billing exports",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingDecliningInFlowAWSLoginDoesNotRunLoginOrPreflight(t *testing.T) {
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
	login := &fakeAWSLoginRunner{support: AWSLoginSupport{Available: true, Version: "2.36.34"}}

	output, err := runGuidedWithConfig("1\n1\n1\n\ncancel\n", Config{Registry: registry, AWSBilling: guide, AWSLogin: login})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when AWS login is declined")
	}
	if len(login.loginCalls) != 0 {
		t.Fatalf("login calls = %#v, want none after default-no confirmation", login.loginCalls)
	}
	if guide.discoverCalls != 1 {
		t.Fatalf("DiscoverCredentialSources calls = %d, want no re-scan after declined login", guide.discoverCalls)
	}
	for _, want := range []string{
		"Sign in to profile default, then re-scan",
		"Run AWS login for profile default now? [y/N]",
		"Choose how to connect another AWS account.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingInFlowAWSLoginFailureReturnsToSelectionWithoutPreflight(t *testing.T) {
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
	login := &fakeAWSLoginRunner{
		support:  AWSLoginSupport{Available: true, Version: "2.36.34"},
		loginErr: errors.New("raw arn:aws:iam::123456789012:user/example login failed"),
	}

	output, err := runGuidedWithConfig("1\n1\n1\ny\ncancel\n", Config{Registry: registry, AWSBilling: guide, AWSLogin: login})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when AWS login fails")
	}
	if len(login.loginCalls) != 1 {
		t.Fatalf("login calls = %#v, want one failed login call", login.loginCalls)
	}
	if guide.discoverCalls != 1 {
		t.Fatalf("DiscoverCredentialSources calls = %d, want no re-scan after failed login", guide.discoverCalls)
	}
	for _, want := range []string{
		"Run AWS login for profile default now? [y/N]",
		"AWS login did not complete. No AWS billing inspection was run.",
		"Choose how to connect another AWS account.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{"arn:aws", "123456789012", "example login failed"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output leaked login error detail %q: %s", forbidden, output)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingDoesNotOfferInFlowLoginWhenAWSLoginUnsupported(t *testing.T) {
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
	login := &fakeAWSLoginRunner{support: AWSLoginSupport{Available: false, Version: "2.31.0", Reason: "aws_cli_login_unsupported_version"}}

	output, err := runGuidedWithConfig("1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide, AWSLogin: login})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when identity is unavailable")
	}
	if login.supportCalls == 0 {
		t.Fatal("AWS login support should be checked before offering in-flow login")
	}
	if strings.Contains(output, "Sign in to profile default, then re-scan") {
		t.Fatalf("output offered in-flow login despite unsupported AWS login: %s", output)
	}
	if !strings.Contains(output, "Remediation: aws login --profile default") {
		t.Fatalf("output = %q, want manual login remediation", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingInFlowLoginOptionOnlyForSafeMissingLoginProfiles(t *testing.T) {
	tests := []struct {
		name   string
		source billingguide.CredentialSource
		err    error
	}{
		{
			name:   "environment credentials",
			source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceEnvironment},
			err:    billingguide.VerificationError{Code: "aws_config_missing_credentials", Message: "AWS credentials are not available."},
		},
		{
			name:   "profile without login session",
			source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"},
			err:    billingguide.VerificationError{Code: "aws_config_missing_credentials", Message: "AWS credentials are not available."},
		},
		{
			name:   "missing region",
			source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", HasLoginSession: true},
			err:    billingguide.VerificationError{Code: "aws_config_missing_region", Message: "AWS Region is not configured."},
		},
		{
			name:   "profile shadowed",
			source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1", HasLoginSession: true},
			err:    billingguide.VerificationError{Code: "aws_config_profile_shadowed", Message: "AWS profile selection is blocked."},
		},
		{
			name:   "generic auth failure",
			source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1", HasLoginSession: true},
			err:    billingguide.VerificationError{Code: "aws_auth_failed", Message: "AWS caller identity could not be verified."},
		},
		{
			name:   "unsafe profile",
			source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "/private/tmp/profile", Region: "us-east-1", HasLoginSession: true},
			err:    billingguide.VerificationError{Code: "aws_config_missing_credentials", Message: "AWS credentials are not available."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
				called = true
				return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
			}))
			guide := &fakeAWSBillingGuide{
				sources: []billingguide.CredentialSource{tt.source},
				verifyErrs: map[string]error{
					sourceKey(tt.source): tt.err,
				},
			}
			login := &fakeAWSLoginRunner{support: AWSLoginSupport{Available: true, Version: "2.36.34"}}

			output, err := runGuidedWithConfig("1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide, AWSLogin: login})

			if !errors.Is(err, ErrInputCancelled) {
				t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
			}
			if called {
				t.Fatal("preflight registry should not run when identity is unavailable")
			}
			if len(login.loginCalls) != 0 {
				t.Fatalf("login calls = %#v, want none for %s", login.loginCalls, tt.name)
			}
			if strings.Contains(output, "Sign in to profile default, then re-scan") ||
				strings.Contains(output, "Run AWS login for profile default now? [y/N]") {
				t.Fatalf("output offered in-flow login for %s: %s", tt.name, output)
			}
			assertGuidedOutputSafe(t, output)
		})
	}
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

	output, err := runGuidedWithConfig("1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
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
	if !strings.Contains(output, "Use an existing AWS profile name manually (advanced)") {
		t.Fatalf("output = %q, want manual profile option", output)
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

			output, err := runGuidedWithConfig("1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide})

			if !errors.Is(err, ErrInputCancelled) {
				t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
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
			if !strings.Contains(output, "Use an existing AWS profile name manually (advanced)") {
				t.Fatalf("output = %q, want manual profile option", output)
			}
			assertGuidedOutputSafe(t, output)
		})
	}
}

func TestRunAWSBillingNoCredentialSourcesCanRescanAfterExternalSignIn(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sourceSequences: [][]billingguide.CredentialSource{
			nil,
			{{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}},
		},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:finance": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n1\n\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"No AWS credential sources were found.",
		"Sign in or configure another AWS profile, then re-scan",
		"Use an existing AWS profile name manually (advanced)",
		"Press Enter after the AWS profile is ready to re-scan",
		"Re-scanning safe local AWS credential sources.",
		"account-ending-2222",
		"Inspect AWS CUR 2.0 billing exports",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil ||
		gotOptions.Selectors.AWS.Profile != "finance" ||
		gotOptions.Selectors.AWS.Region != "us-west-2" {
		t.Fatalf("AWS selectors = %#v, want re-scanned finance/us-west-2", gotOptions)
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:finance" {
		t.Fatalf("VerifyIdentity calls = %q, want re-scanned finance", got)
	}
	if guide.discoverCalls != 2 {
		t.Fatalf("DiscoverCredentialSources calls = %d, want 2", guide.discoverCalls)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingNoVerifiedSourcesCanRescanAfterExternalSignIn(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sourceSequences: [][]billingguide.CredentialSource{
			{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1", HasLoginSession: true}},
			{{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}},
		},
		verifyErrs: map[string]error{
			"profile:default": billingguide.VerificationError{Code: "aws_config_missing_credentials", Message: "AWS credentials are not available."},
		},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:finance": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n1\n\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"No verified AWS credential source is available.",
		"aws login --profile default",
		"Sign in or configure another AWS profile, then re-scan",
		"Use an existing AWS profile name manually (advanced)",
		"Re-scanning safe local AWS credential sources.",
		"account-ending-2222",
		"Inspect AWS CUR 2.0 billing exports",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil ||
		gotOptions.Selectors.AWS.Profile != "finance" ||
		gotOptions.Selectors.AWS.Region != "us-west-2" {
		t.Fatalf("AWS selectors = %#v, want re-scanned finance/us-west-2", gotOptions)
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:default,profile:finance" {
		t.Fatalf("VerifyIdentity calls = %q, want default then re-scanned finance", got)
	}
	if guide.discoverCalls != 2 {
		t.Fatalf("DiscoverCredentialSources calls = %d, want 2", guide.discoverCalls)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingRescanWaitCanBeCancelledBeforeRediscovery(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{}

	output, err := runGuidedWithConfig("1\n1\n1\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when re-scan wait is cancelled")
	}
	if guide.discoverCalls != 1 {
		t.Fatalf("DiscoverCredentialSources calls = %d, want only initial discovery", guide.discoverCalls)
	}
	for _, want := range []string{
		"Press Enter after the AWS profile is ready to re-scan, or type cancel:",
		"guided setup cancelled by user",
	} {
		if !strings.Contains(output+err.Error(), want) {
			t.Fatalf("combined output/error = %q, want to contain %q", output+err.Error(), want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingRescanUsesFreshOperationContextsAfterExternalWait(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		if err := ctx.Err(); err != nil {
			t.Fatalf("preflight context error = %v, want fresh active context", err)
		}
		gotOptions = options
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		sourceSequences: [][]billingguide.CredentialSource{
			nil,
			{{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}},
		},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:finance": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfigReader(&delayedInput{
		chunks: []delayedInputChunk{
			{text: "1\n1\n1\n"},
			{text: "\n", delay: 1100 * time.Millisecond},
			{text: "y\n"},
		},
	}, Config{Registry: registry, AWSBilling: guide, TimeoutSeconds: 1})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil ||
		gotOptions.Selectors.AWS.Profile != "finance" ||
		gotOptions.Selectors.AWS.Region != "us-west-2" {
		t.Fatalf("AWS selectors = %#v, want re-scanned finance/us-west-2", gotOptions)
	}
	if guide.discoverCalls != 2 {
		t.Fatalf("DiscoverCredentialSources calls = %d, want 2", guide.discoverCalls)
	}
	if !strings.Contains(output, "Re-scanning safe local AWS credential sources.") {
		t.Fatalf("output = %q, want re-scan message", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingManualProfileRejectsUnsafeInputWithoutEcho(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{}

	output, err := runGuidedWithConfig("1\n1\n2\n/private/tmp/aws-profile\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run for unsafe manual profile input")
	}
	if len(guide.verifyCalls) != 0 {
		t.Fatalf("VerifyIdentity calls = %#v, want none for unsafe manual profile input", guide.verifyCalls)
	}
	if !strings.Contains(output, "AWS profile name is not safe to use.") {
		t.Fatalf("output = %q, want generic unsafe profile message", output)
	}
	for _, forbidden := range []string{"/private/tmp/aws-profile", "/private/", "aws-profile"} {
		if strings.Contains(output, forbidden) || (err != nil && strings.Contains(err.Error(), forbidden)) {
			t.Fatalf("unsafe manual profile value leaked %q; output=%q err=%v", forbidden, output, err)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingManualProfileRequiresVerificationConfirmation(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:finance": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "finance", Region: "us-west-2"}, AccountLabel: "account-ending-2222", CallerRef: "sha256:222222222222", Region: "us-west-2"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n2\nfinance\nus-west-2\nn\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when manual profile verification is declined")
	}
	if len(guide.verifyCalls) != 0 {
		t.Fatalf("VerifyIdentity calls = %#v, want none before manual verification confirmation", guide.verifyCalls)
	}
	for _, want := range []string{
		"AWS profile verification may run normal AWS SDK credential resolution",
		"including a configured credential process",
		"Verify this AWS profile now? [y/N]",
		"Choose how to connect another AWS account.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingManualProfileMissingCredentialsShowsSafeRemediation(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		verifyErrs: map[string]error{
			"profile:finance": billingguide.VerificationError{Code: "aws_config_missing_credentials", Message: "AWS credentials are not available."},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n2\nfinance\nus-west-2\ny\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when manual profile credentials are missing")
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:finance" {
		t.Fatalf("VerifyIdentity calls = %q, want manual finance verification once", got)
	}
	for _, want := range []string{
		"profile finance in us-west-2 blocked: aws_config_missing_credentials",
		"Run aws login --profile finance if this is an AWS login profile, or configure credentials for profile finance.",
		"Then choose this profile again after login or configuration is complete.",
		"Choose how to connect another AWS account.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingManualProfileMissingRegionShowsSafeRemediation(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		verifyErrs: map[string]error{
			"profile:finance": billingguide.VerificationError{Code: "aws_config_missing_region", Message: "AWS Region is not configured."},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n2\nfinance\n\ny\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when manual profile Region is missing")
	}
	if got := strings.Join(guide.verifyCalls, ","); got != "profile:finance" {
		t.Fatalf("VerifyIdentity calls = %q, want manual finance verification once", got)
	}
	for _, want := range []string{
		"profile finance blocked: aws_config_missing_region",
		"Enter an AWS Region when choosing this profile again, or configure a Region for profile finance outside this tool.",
		"Choose how to connect another AWS account.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingManualProfileShadowedByEnvironmentExplainsRestartBoundary(t *testing.T) {
	called := false
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		called = true
		return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
	}))
	guide := &fakeAWSBillingGuide{
		verifyErrs: map[string]error{
			"profile:finance": billingguide.VerificationError{Code: "aws_config_profile_shadowed", Message: "AWS profile selection is blocked because credential environment variables would take precedence."},
		},
	}

	output, err := runGuidedWithConfig("1\n1\n2\nfinance\nus-west-2\ny\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when environment credentials shadow a manual profile")
	}
	for _, want := range []string{
		"profile finance in us-west-2 blocked: aws_config_profile_shadowed",
		"AWS credential environment variables would take precedence over the selected profile.",
		"Unset AWS credential environment variables and start a new shell before retrying this profile.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if strings.Contains(output, "Then choose this profile again") {
		t.Fatalf("profile-shadowed output implied same-process retry: %s", output)
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

func TestRunAWSBillingDeclineThenCancelStopsBeforePreflight(t *testing.T) {
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

	output, err := runGuidedWithConfig("1\n1\nn\ncancel\n", Config{Registry: registry, AWSBilling: guide})

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
	}
	if called {
		t.Fatal("preflight registry should not run when user declines the verified account")
	}
	if !strings.Contains(output, "Choose how to connect another AWS account.") {
		t.Fatalf("output = %q, want source reselection guidance", output)
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
		"cur2-abcdefghijklmnop",
	)

	if !strings.Contains(command, "--profile 'prod;date'") {
		t.Fatalf("command = %q, want shell-quoted profile selector", command)
	}
	if strings.Contains(command, "--profile prod;date") {
		t.Fatalf("command left shell metacharacter unquoted: %q", command)
	}
}

func TestReadConfirmationDefaultYesSemantics(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr error
	}{
		{name: "blank defaults yes", input: "\n", want: true},
		{name: "yes", input: "yes\n", want: true},
		{name: "no", input: "no\n", want: false},
		{name: "cancel", input: "cancel\n", wantErr: ErrInputCancelled},
		{name: "invalid", input: "maybe\n", wantErr: ErrInvalidSelection},
		{name: "eof", wantErr: ErrInputCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewScanner(strings.NewReader(tt.input))
			var output bytes.Buffer

			got, err := readConfirmationDefaultYes(reader, &output, "Continue? [Y/n] ")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("readConfirmationDefaultYes returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("confirmation = %v, want %v", got, tt.want)
			}
			if output.String() != "Continue? [Y/n] " {
				t.Fatalf("prompt output = %q, want prompt", output.String())
			}
		})
	}
}

func TestWriteCreateCUR2SetupPlanSummaryShowsNonMutatingReuse(t *testing.T) {
	source := billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}
	result := workflow.Result{
		Plan: &workflow.ExecutionPlan{
			Steps: []workflow.PlanStep{{
				Title:            "Reuse existing Matilda AWS CUR 2.0 export",
				RequiresApproval: false,
			}},
			Approval: workflow.ApprovalSummary{Required: false},
		},
	}
	var output bytes.Buffer

	writeCreateCUR2SetupPlanSummary(&output, source, result)

	for _, want := range []string{
		"Setup plan:",
		"No approval required: Reuse existing Matilda AWS CUR 2.0 export",
		"No cloud changes were made.",
		"No mutation approval is required for this result.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want to contain %q", output.String(), want)
		}
	}
	for _, forbidden := range []string{
		"Approve with:",
		"--approve-step",
	} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("output = %q, want no %q", output.String(), forbidden)
		}
	}
}

func TestWriteCreateCUR2SetupPlanSummaryShowsBlockedApproval(t *testing.T) {
	source := billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}
	result := workflow.Result{
		Plan: &workflow.ExecutionPlan{
			Steps: []workflow.PlanStep{{
				Title:              "Resolve AWS S3 bucket candidate access",
				RequiresApproval:   false,
				CurrentState:       "The generated same-account S3 bucket candidate could not be verified as available to create or safely owned by this account.",
				TargetState:        "Matilda Cloud Prep can show an approval-required plan to create or reuse the generated bucket, update its Data Exports delivery policy, and create the CUR 2.0 export.",
				RequiredPermission: "s3:ListBucket for existing bucket checks, plus s3:CreateBucket, s3:GetBucketPolicy, s3:PutBucketPolicy, bcm-data-exports:CreateExport, and cur:PutReportDefinition for approved setup.",
				Validation:         "Do not manually create or select arbitrary buckets for the normal guided path. Resolve S3 access ambiguity, then rerun apply-prereqs to get a new approval-required setup plan.",
			}},
			Approval: workflow.ApprovalSummary{
				Required: true,
				Blocked:  true,
			},
		},
	}
	var output bytes.Buffer

	writeCreateCUR2SetupPlanSummary(&output, source, result)

	for _, want := range []string{
		"Setup plan:",
		"No approval required: Resolve AWS S3 bucket candidate access",
		"Current state: The generated same-account S3 bucket candidate could not be verified as available to create or safely owned by this account.",
		"Target state: Matilda Cloud Prep can show an approval-required plan to create or reuse the generated bucket, update its Data Exports delivery policy, and create the CUR 2.0 export.",
		"Required permission: s3:ListBucket for existing bucket checks",
		"Validation: Do not manually create or select arbitrary buckets for the normal guided path.",
		"No cloud changes were made.",
		"This setup plan is blocked and cannot be approved until the blocker is resolved.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want to contain %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "Approve with:") {
		t.Fatalf("output = %q, want no approval command for blocked plan", output.String())
	}
}

func TestWriteCreateCUR2SetupPlanSummaryShowsAppliedMutation(t *testing.T) {
	source := billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}
	result := workflow.Result{
		Mutated: true,
		Plan: &workflow.ExecutionPlan{
			Steps: []workflow.PlanStep{{
				Title:            "Create Matilda AWS CUR 2.0 export",
				RequiresApproval: true,
				ID:               workflow.AWSCUR2CreateExportOperationID,
			}},
			Approval: workflow.ApprovalSummary{
				Required:       true,
				Approved:       true,
				ApprovalPlanID: "plan_abcdefghijklmnop",
			},
		},
	}
	var output bytes.Buffer

	writeCreateCUR2SetupPlanSummary(&output, source, result)

	for _, want := range []string{
		"Setup plan:",
		"Approval required: Create Matilda AWS CUR 2.0 export",
		"Cloud changes were made for the approved setup plan.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want to contain %q", output.String(), want)
		}
	}
	for _, forbidden := range []string{
		"No cloud changes were made.",
		"Cloud changes require plan-bound approval",
		"Approve with:",
	} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("output = %q, want no %q", output.String(), forbidden)
		}
	}
}

func TestDirectAWSBillingCreateCUR2ApprovalCommandRequiresApprovablePlan(t *testing.T) {
	source := billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}
	tests := []struct {
		name   string
		result workflow.Result
	}{
		{name: "missing plan"},
		{
			name: "no approval required",
			result: workflow.Result{Plan: &workflow.ExecutionPlan{
				Approval: workflow.ApprovalSummary{Required: false},
			}},
		},
		{
			name: "blocked",
			result: workflow.Result{Plan: &workflow.ExecutionPlan{
				Approval: workflow.ApprovalSummary{Required: true, Blocked: true, ApprovalPlanID: "plan_abcdefghijklmnop"},
			}},
		},
		{
			name: "missing plan id",
			result: workflow.Result{Plan: &workflow.ExecutionPlan{
				Approval: workflow.ApprovalSummary{Required: true},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := directAWSBillingCreateCUR2ApprovalCommand(source, tt.result); got != "" {
				t.Fatalf("approval command = %q, want empty", got)
			}
		})
	}
}

func runGuidedWithConfig(input string, config Config) (string, error) {
	return runGuidedWithConfigReader(strings.NewReader(input), config)
}

func runGuidedWithConfigReader(input io.Reader, config Config) (string, error) {
	var output bytes.Buffer
	err := RunWithConfig(input, &output, config)
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
		ID:      code,
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
	sources            []billingguide.CredentialSource
	sourceSequences    [][]billingguide.CredentialSource
	discoverErr        error
	discoverCalls      int
	verified           map[string]billingguide.VerifiedIdentity
	verifyErrs         map[string]error
	verifyErrSequences map[string][]error
	verifyCalls        []string
}

func (f *fakeAWSBillingGuide) DiscoverCredentialSources(ctx context.Context) ([]billingguide.CredentialSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.discoverCalls++
	if f.discoverErr != nil {
		return nil, f.discoverErr
	}
	if len(f.sourceSequences) > 0 {
		index := f.discoverCalls - 1
		if index >= len(f.sourceSequences) {
			index = len(f.sourceSequences) - 1
		}
		return append([]billingguide.CredentialSource{}, f.sourceSequences[index]...), nil
	}
	return append([]billingguide.CredentialSource{}, f.sources...), nil
}

func (f *fakeAWSBillingGuide) VerifyIdentity(_ context.Context, source billingguide.CredentialSource) (billingguide.VerifiedIdentity, error) {
	key := sourceKey(source)
	f.verifyCalls = append(f.verifyCalls, key)
	if sequence, ok := f.verifyErrSequences[key]; ok && len(sequence) > 0 {
		err := sequence[0]
		f.verifyErrSequences[key] = sequence[1:]
		if err != nil {
			return billingguide.VerifiedIdentity{}, err
		}
	}
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

type fakeAWSLoginRunner struct {
	support      AWSLoginSupport
	supportCalls int
	loginCalls   []billingguide.CredentialSource
	loginErr     error
}

func (f *fakeAWSLoginRunner) SupportsLogin(ctx context.Context) AWSLoginSupport {
	f.supportCalls++
	if err := ctx.Err(); err != nil {
		return AWSLoginSupport{Available: false, Reason: "aws_cli_login_check_cancelled", Message: err.Error()}
	}
	return f.support
}

func (f *fakeAWSLoginRunner) Login(ctx context.Context, source billingguide.CredentialSource) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.loginCalls = append(f.loginCalls, source)
	return f.loginErr
}

type delayedInputChunk struct {
	text  string
	delay time.Duration
}

type delayedInput struct {
	chunks []delayedInputChunk
	index  int
}

func (input *delayedInput) Read(p []byte) (int, error) {
	if input.index >= len(input.chunks) {
		return 0, io.EOF
	}
	chunk := input.chunks[input.index]
	input.index++
	if chunk.delay > 0 {
		time.Sleep(chunk.delay)
	}
	return copy(p, chunk.text), nil
}

func assertGuidedOutputSafe(t *testing.T, output string) {
	t.Helper()
	for _, forbidden := range []string{
		"arn:aws",
		"123456789012",
		"matilda/cur2/private-prefix",
		"BILLING_PERIOD=2026-06/part-000.gz",
		"Manifest.json",
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
