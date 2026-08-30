package guided

import (
	"bufio"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestRunAWSBillingPromptsBeforeSelectedExportPreflight(t *testing.T) {
	calls := []workflow.ExecutionOptions{}
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, options)
		exportRef := ""
		if options.Selectors != nil && options.Selectors.AWS != nil {
			exportRef = options.Selectors.AWS.CUR2ExportRef
		}
		switch exportRef {
		case "":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_ambiguous", []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-acacacacacacacac"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_2_export_ref", Value: "cur2-bdbdbdbdbdbdbdbd"},
				{Key: "candidate_2_health", Value: "WARNING"},
				{Key: "candidate_2_output_format", Value: "PARQUET"},
				{Key: "candidate_2_compression", Value: "PARQUET"},
				{Key: "candidate_2_time_granularity", Value: "DAILY"},
				{Key: "candidate_2_overwrite", Value: "OVERWRITE_REPORT"},
				{Key: "candidate_2_output_type", Value: "CUSTOM"},
				{Key: "candidate_2_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_2_destination_region", Value: "us-west-2"},
			})
		case "cur2-acacacacacacacac":
			return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", []workflow.PlanEvidence{
				{Key: "output_format", Value: "TEXT_OR_CSV"},
				{Key: "compression", Value: "GZIP"},
				{Key: "time_granularity", Value: "MONTHLY"},
				{Key: "overwrite", Value: "CREATE_NEW_REPORT"},
			})
		case "cur2-bdbdbdbdbdbdbdbd":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_output_settings_blocked", nil)
		default:
			t.Fatalf("unexpected export ref %q", exportRef)
			return workflow.CapabilityReport{}
		}
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n2\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("preflight calls = %d, want initial discovery plus selected export preflight", len(calls))
	}
	if calls[0].Selectors.AWS.CUR2ExportRef != "" {
		t.Fatalf("initial preflight ref = %q, want empty discovery selector", calls[0].Selectors.AWS.CUR2ExportRef)
	}
	if calls[1].Selectors.AWS.CUR2ExportRef != "cur2-bdbdbdbdbdbdbdbd" {
		t.Fatalf("selected preflight ref = %q, want selected candidate ref", calls[1].Selectors.AWS.CUR2ExportRef)
	}
	for _, want := range []string{
		"Select AWS CUR 2.0 export",
		"Full readiness checks run after selection.",
		"Blocker: pre-selection metadata has unsupported settings: health status WARNING.",
		"Running readiness preflight for selected CUR 2.0 export cur2-bdbdbdbdbdbdbdbd",
		"Readiness: not ready",
		"aws_cur2_output_settings_blocked",
		"--export-ref cur2-bdbdbdbdbdbdbdbd",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingCanPrepareNewCUR2PlanInsteadOfDiscoveredExports(t *testing.T) {
	calls := []guidedAWSBillingCall{}
	preflightRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		if options.Selectors != nil && options.Selectors.AWS != nil && options.Selectors.AWS.CUR2ExportRef != "" {
			t.Fatalf("selected preflight should not run when user declines discovered exports: %#v", options.Selectors.AWS)
		}
		return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_ambiguous", []workflow.PlanEvidence{
			{Key: "candidate_1_export_ref", Value: "cur2-acacacacacacacac"},
			{Key: "candidate_1_health", Value: "HEALTHY"},
			{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
			{Key: "candidate_1_compression", Value: "GZIP"},
			{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
			{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
			{Key: "candidate_1_output_type", Value: "CUSTOM"},
			{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
			{Key: "candidate_1_destination_region", Value: "us-east-1"},
			{Key: "candidate_2_export_ref", Value: "cur2-bdbdbdbdbdbdbdbd"},
			{Key: "candidate_2_health", Value: "HEALTHY"},
			{Key: "candidate_2_output_format", Value: "PARQUET"},
			{Key: "candidate_2_compression", Value: "PARQUET"},
			{Key: "candidate_2_time_granularity", Value: "DAILY"},
			{Key: "candidate_2_overwrite", Value: "OVERWRITE_REPORT"},
			{Key: "candidate_2_output_type", Value: "CUSTOM"},
			{Key: "candidate_2_refresh_cadence", Value: "SYNCHRONOUS"},
			{Key: "candidate_2_destination_region", Value: "us-west-2"},
		})
	})
	applyRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		return guidedCreateCUR2PlanReport(got)
	})
	registry := testAWSBillingRegistry(t, preflightRunner, applyRunner)
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n3\n1\n\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	assertGuidedCreateCUR2PlanCalls(t, calls)
	for _, want := range []string{
		"Select AWS CUR 2.0 export",
		"Prepare a new Matilda CUR 2.0 setup plan",
		"Select AWS CUR 2.0 destination",
		"Use a generated same-account Matilda S3 bucket",
		"Preparing a new Matilda AWS CUR 2.0 setup plan.",
		"Result: ready_with_manual_steps",
		"Support code: aws_cur2_create_export_approval_required",
		"Approval plan:",
		"Step ID: aws.billing.cur2.bucket.create",
		"Current state: No matching generated same-account S3 bucket is available.",
		"Target state: A generated same-account S3 bucket exists in the selected region.",
		"Required permission: s3:CreateBucket",
		"Validation: The bucket is checked with the expected bucket owner before policy or export creation continues.",
		"Rollback: The tool does not delete S3 buckets automatically.",
		"Cloud changes require plan-bound approval before they are made.",
		"Apply this AWS CUR 2.0 setup plan now? [y/N]",
		"Next command:",
		"matilda-prep rapid-assessment billing aws apply-prereqs --profile default --region us-east-1 --create-cur2-export",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"Running readiness preflight for selected CUR 2.0 export",
		"Applying approved Matilda AWS CUR 2.0 setup plan.",
		"Reproduce with:",
		"--export-ref",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingCanSelectExistingBucketForNewCUR2Plan(t *testing.T) {
	calls := []guidedAWSBillingCall{}
	preflightRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		if options.Selectors != nil && options.Selectors.AWS != nil && options.Selectors.AWS.CUR2ExportRef != "" {
			t.Fatalf("selected preflight should not run when user prepares a new setup plan: %#v", options.Selectors.AWS)
		}
		return guidedAmbiguousCUR2SelectionReport(got)
	})
	applyRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		if options.Selectors == nil || options.Selectors.AWS == nil {
			t.Fatalf("apply-prereqs selectors missing: %#v", options.Selectors)
		}
		if options.Selectors.AWS.CUR2DestinationMode != workflow.AWSCUR2DestinationExistingSameAccount {
			t.Fatalf("destination mode = %q, want existing_same_account", options.Selectors.AWS.CUR2DestinationMode)
		}
		switch options.Selectors.AWS.CUR2S3BucketRef {
		case "":
			return guidedExistingBucketSelectionReport(got)
		case "s3b-abcdefghijklmnop":
			return guidedCreateCUR2ExistingBucketPlanReport(got)
		default:
			t.Fatalf("unexpected bucket ref %q", options.Selectors.AWS.CUR2S3BucketRef)
			return workflow.CapabilityReport{}
		}
	})
	registry := testAWSBillingRegistry(t, preflightRunner, applyRunner)
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n3\n2\n1\n\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("registry calls = %d, want preflight, bucket selection, selected bucket plan", len(calls))
	}
	if calls[1].Options.Selectors.AWS.CUR2DestinationMode != workflow.AWSCUR2DestinationExistingSameAccount ||
		calls[1].Options.Selectors.AWS.CUR2S3BucketRef != "" {
		t.Fatalf("bucket discovery selectors = %#v, want existing destination without selected ref", calls[1].Options.Selectors.AWS)
	}
	if calls[2].Options.Selectors.AWS.CUR2DestinationMode != workflow.AWSCUR2DestinationExistingSameAccount ||
		calls[2].Options.Selectors.AWS.CUR2S3BucketRef != "s3b-abcdefghijklmnop" {
		t.Fatalf("selected bucket plan selectors = %#v, want selected existing bucket ref", calls[2].Options.Selectors.AWS)
	}
	for _, want := range []string{
		"Select AWS CUR 2.0 destination",
		"Use an existing S3 bucket owned by this AWS account",
		"Discovering existing S3 buckets in the selected AWS account.",
		"Select S3 bucket for new Matilda CUR 2.0 export",
		"matilda-existing-cur2",
		"s3b-abcdefghijklmnop",
		"Support code: aws_cur2_create_export_approval_required",
		"Step ID: aws.billing.cur2.bucket_policy.merge_data_exports_delivery",
		"Step ID: aws.billing.cur2.export.create",
		"--cur2-destination existing-same-account --cur2-s3-bucket-ref s3b-abcdefghijklmnop",
		"Apply this AWS CUR 2.0 setup plan now? [y/N]",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"Step ID: aws.billing.cur2.bucket.create",
		"--export-ref",
		"Running readiness preflight for selected CUR 2.0 export",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingStopsExistingBucketSetupWhenNoBucketsDiscovered(t *testing.T) {
	calls := []guidedAWSBillingCall{}
	preflightRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		return guidedAmbiguousCUR2SelectionReport(got)
	})
	applyRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		if options.Selectors == nil || options.Selectors.AWS == nil {
			t.Fatalf("apply-prereqs selectors missing: %#v", options.Selectors)
		}
		if options.Selectors.AWS.CUR2DestinationMode != workflow.AWSCUR2DestinationExistingSameAccount ||
			options.Selectors.AWS.CUR2S3BucketRef != "" {
			t.Fatalf("bucket discovery selectors = %#v, want existing destination without selected ref", options.Selectors.AWS)
		}
		return guidedExistingBucketSelectionNoCandidatesReport(got)
	})
	registry := testAWSBillingRegistry(t, preflightRunner, applyRunner)
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n3\n2\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("registry calls = %d, want preflight and bucket discovery only", len(calls))
	}
	for _, want := range []string{
		"Select AWS CUR 2.0 destination",
		"Use an existing S3 bucket owned by this AWS account",
		"Discovering existing S3 buckets in the selected AWS account.",
		"Support code: aws_cur2_existing_bucket_selection_required",
		"No existing same-account S3 buckets were discovered.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"Select S3 bucket for new Matilda CUR 2.0 export",
		"Preparing a new Matilda AWS CUR 2.0 setup plan.",
		"Apply this AWS CUR 2.0 setup plan now? [y/N]",
		"--cur2-s3-bucket-ref",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestSelectCreateCUR2DestinationRejectsInvalidChoice(t *testing.T) {
	reader := bufio.NewScanner(strings.NewReader("3\n"))
	var output strings.Builder

	mode, err := selectCreateCUR2Destination(reader, &output)

	if mode != "" {
		t.Fatalf("destination mode = %q, want empty after invalid selection", mode)
	}
	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("error = %v, want ErrInvalidSelection", err)
	}
	for _, want := range []string{
		"Select AWS CUR 2.0 destination",
		"Select AWS CUR 2.0 destination [1-2]:",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want to contain %q", output.String(), want)
		}
	}
}

func TestSelectExistingS3BucketRejectsInvalidChoice(t *testing.T) {
	reader := bufio.NewScanner(strings.NewReader("2\n"))
	var output strings.Builder

	ref, err := selectExistingS3Bucket(reader, &output, []existingS3BucketCandidate{{
		Ref:    "s3b-abcdefghijklmnop",
		Label:  "matilda-existing-cur2",
		Region: "us-east-1",
	}})

	if ref != "" {
		t.Fatalf("bucket ref = %q, want empty after invalid selection", ref)
	}
	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("error = %v, want ErrInvalidSelection", err)
	}
	for _, want := range []string{
		"Select S3 bucket for new Matilda CUR 2.0 export",
		"matilda-existing-cur2 in us-east-1 (s3b-abcdefghijklmnop)",
		"Select S3 bucket [1-1]:",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want to contain %q", output.String(), want)
		}
	}
}

func TestExistingS3BucketCandidatesSkipsUnsafeRefsAndUsesSafeRefFallbackLabel(t *testing.T) {
	if got := existingS3BucketCandidates(workflow.Result{}); got != nil {
		t.Fatalf("candidates without plan = %#v, want nil", got)
	}

	result := workflow.Result{Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
		Evidence: []workflow.PlanEvidence{
			{Key: "ignored_bucket_ref", Value: "s3b-aaaaaaaaaaaaaaaa"},
			{Key: "candidate_1_bucket_ref", Value: "raw-bucket-name"},
			{Key: "candidate_1_bucket_label", Value: "raw-bucket-name"},
			{Key: "candidate_2_bucket_ref", Value: "s3b-abcdefghijklmnop"},
			{Key: "candidate_2_bucket_label", Value: "arn:aws:s3:::customer-123456789012-bucket"},
			{Key: "candidate_2_bucket_region", Value: "us-east-1"},
		},
	}}}}

	candidates := existingS3BucketCandidates(result)

	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one safe candidate", candidates)
	}
	candidate := candidates[0]
	if candidate.Index != 2 || candidate.Ref != "s3b-abcdefghijklmnop" ||
		candidate.Label != "s3b-abcdefghijklmnop" || candidate.Region != "us-east-1" {
		t.Fatalf("candidate = %#v, want unsafe label replaced with safe ref fallback", candidate)
	}
}

func TestRunCreateCUR2SetupPlanWithDestinationRejectsUnsafeBucketRefBeforeRegistryCall(t *testing.T) {
	registryCalls := 0
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		registryCalls++
		return guidedCreateCUR2PlanReport(got)
	}))
	source := billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}

	result := runCreateCUR2SetupPlanWithDestination(context.Background(), registry, source, workflow.AWSCUR2DestinationExistingSameAccount, "arn:aws:s3:::unsafe")

	if result.Code != "execution_options_invalid" {
		t.Fatalf("Code = %q, want execution_options_invalid", result.Code)
	}
	if result.Status != workflow.RunStatusBlocked {
		t.Fatalf("Status = %q, want blocked", result.Status)
	}
	if registryCalls != 0 {
		t.Fatalf("registry calls = %d, want none before invalid bucket ref is rejected", registryCalls)
	}
}

func TestPreserveCreateCUR2DestinationSelectorsCreatesMissingSelectorContainers(t *testing.T) {
	options := workflow.ExecutionOptions{}
	preview := workflow.Result{
		ExecutionOptions: workflow.ExecutionOptions{
			Selectors: &workflow.ExecutionSelectors{
				AWS: &workflow.AWSExecutionSelectors{
					CUR2DestinationMode: workflow.AWSCUR2DestinationExistingSameAccount,
					CUR2S3BucketRef:     "s3b-abcdefghijklmnop",
				},
			},
		},
	}

	preserveCreateCUR2DestinationSelectors(&options, preview)

	if options.Selectors == nil || options.Selectors.AWS == nil {
		t.Fatalf("selectors = %#v, want AWS selector container created", options.Selectors)
	}
	if options.Selectors.AWS.CUR2DestinationMode != workflow.AWSCUR2DestinationExistingSameAccount ||
		options.Selectors.AWS.CUR2S3BucketRef != "s3b-abcdefghijklmnop" {
		t.Fatalf("AWS selectors = %#v, want preserved existing bucket destination", options.Selectors.AWS)
	}

	preserveCreateCUR2DestinationSelectors(nil, preview)
	preserveCreateCUR2DestinationSelectors(&options, workflow.Result{})
	if options.Selectors.AWS.CUR2S3BucketRef != "s3b-abcdefghijklmnop" {
		t.Fatalf("AWS selectors changed after missing-input preservation: %#v", options.Selectors.AWS)
	}
}

func TestRunAWSBillingSelectedExportPreflightUsesFreshContextAfterUserWait(t *testing.T) {
	calls := []workflow.ExecutionOptions{}
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, options)
		exportRef := ""
		if options.Selectors != nil && options.Selectors.AWS != nil {
			exportRef = options.Selectors.AWS.CUR2ExportRef
		}
		switch exportRef {
		case "":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_ambiguous", []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-aaaaaaaaaaaaaaaa"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_2_export_ref", Value: "cur2-bbbbbbbbbbbbbbbb"},
				{Key: "candidate_2_health", Value: "HEALTHY"},
				{Key: "candidate_2_output_format", Value: "PARQUET"},
				{Key: "candidate_2_compression", Value: "PARQUET"},
				{Key: "candidate_2_time_granularity", Value: "DAILY"},
				{Key: "candidate_2_overwrite", Value: "OVERWRITE_REPORT"},
				{Key: "candidate_2_output_type", Value: "CUSTOM"},
				{Key: "candidate_2_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_2_destination_region", Value: "us-east-1"},
			})
		case "cur2-aaaaaaaaaaaaaaaa":
			if err := ctx.Err(); err != nil {
				t.Fatalf("selected-export preflight context error = %v, want fresh active context", err)
			}
			return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", []workflow.PlanEvidence{
				{Key: "output_format", Value: "TEXT_OR_CSV"},
				{Key: "compression", Value: "GZIP"},
				{Key: "time_granularity", Value: "MONTHLY"},
				{Key: "overwrite", Value: "CREATE_NEW_REPORT"},
			})
		default:
			t.Fatalf("unexpected export ref %q", exportRef)
			return workflow.CapabilityReport{}
		}
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfigReader(&delayedInput{
		chunks: []delayedInputChunk{
			{text: "1\n1\ny\n"},
			{text: "1\n", delay: 1100 * time.Millisecond},
		},
	}, Config{Registry: registry, AWSBilling: guide, TimeoutSeconds: 1})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("preflight calls = %d, want initial discovery plus selected export preflight", len(calls))
	}
	if calls[1].Selectors.AWS.CUR2ExportRef != "cur2-aaaaaaaaaaaaaaaa" {
		t.Fatalf("selected preflight ref = %q, want chosen candidate ref", calls[1].Selectors.AWS.CUR2ExportRef)
	}
	for _, want := range []string{
		"Select AWS CUR 2.0 export",
		"Running readiness preflight for selected CUR 2.0 export cur2-aaaaaaaaaaaaaaaa",
		"Support code: aws_cur2_preflight_ready",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingRanksPreferredShapeFirstWithoutFallbackRecommendationLabel(t *testing.T) {
	calls := []workflow.ExecutionOptions{}
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, options)
		exportRef := ""
		if options.Selectors != nil && options.Selectors.AWS != nil {
			exportRef = options.Selectors.AWS.CUR2ExportRef
		}
		switch exportRef {
		case "":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_ambiguous", []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-aaaaaaaaaaaaaaaa"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "PARQUET"},
				{Key: "candidate_1_compression", Value: "PARQUET"},
				{Key: "candidate_1_time_granularity", Value: "DAILY"},
				{Key: "candidate_1_overwrite", Value: "OVERWRITE_REPORT"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_2_export_ref", Value: "cur2-bbbbbbbbbbbbbbbb"},
				{Key: "candidate_2_health", Value: "HEALTHY"},
				{Key: "candidate_2_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_2_compression", Value: "GZIP"},
				{Key: "candidate_2_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_2_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_2_destination_region", Value: "us-west-2"},
			})
		case "cur2-aaaaaaaaaaaaaaaa":
			return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_time_granularity_not_preferred", []workflow.PlanEvidence{
				{Key: "output_format", Value: "PARQUET"},
				{Key: "compression", Value: "PARQUET"},
				{Key: "time_granularity", Value: "DAILY"},
				{Key: "overwrite", Value: "OVERWRITE_REPORT"},
			})
		case "cur2-bbbbbbbbbbbbbbbb":
			return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", []workflow.PlanEvidence{
				{Key: "output_format", Value: "TEXT_OR_CSV"},
				{Key: "compression", Value: "GZIP"},
				{Key: "time_granularity", Value: "MONTHLY"},
				{Key: "overwrite", Value: "CREATE_NEW_REPORT"},
			})
		default:
			t.Fatalf("unexpected export ref %q", exportRef)
			return workflow.CapabilityReport{}
		}
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("preflight calls = %d, want initial discovery plus selected export preflight", len(calls))
	}
	if calls[1].Selectors.AWS.CUR2ExportRef != "cur2-bbbbbbbbbbbbbbbb" {
		t.Fatalf("selected preflight ref = %q, want preferred-shape candidate selected first", calls[1].Selectors.AWS.CUR2ExportRef)
	}
	firstChoice := "1. cur2-bbbbbbbbbbbbbbbb"
	nonPreferred := "2. cur2-aaaaaaaaaaaaaaaa"
	if !strings.Contains(output, firstChoice) || !strings.Contains(output, nonPreferred) {
		t.Fatalf("output = %q, want preferred-shape candidate first and non-preferred second", output)
	}
	if strings.Index(output, firstChoice) > strings.Index(output, nonPreferred) {
		t.Fatalf("output = %q, want preferred-shape candidate before non-preferred candidate", output)
	}
	for _, want := range []string{
		"Select AWS CUR 2.0 export",
		"cur2-bbbbbbbbbbbbbbbb, health HEALTHY, output TEXT_OR_CSV, compression GZIP, granularity MONTHLY, versioning CREATE_NEW_REPORT, region us-west-2",
		"cur2-aaaaaaaaaaaaaaaa, health HEALTHY, output PARQUET, compression PARQUET, granularity DAILY, versioning OVERWRITE_REPORT, region us-east-1",
		"Full readiness checks run after selection.",
		"Running readiness preflight for selected CUR 2.0 export cur2-bbbbbbbbbbbbbbbb",
		"Readiness: ready",
		"Support code: aws_cur2_preflight_ready",
		"Export: TEXT_OR_CSV / GZIP, MONTHLY, CREATE_NEW_REPORT",
		"--export-ref cur2-bbbbbbbbbbbbbbbb",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if strings.Contains(output, "recommended") {
		t.Fatalf("output = %q, want no recommended label without canonical provider preferred facts", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRankedCUR2CandidatesPutsCompleteValidShapeBeforeIncompleteMetadata(t *testing.T) {
	ranked := rankedCUR2Candidates([]cur2Candidate{
		{
			Ref:    "cur2-incomplete-health",
			Health: "HEALTHY",
		},
		{
			Ref:         "cur2-incomplete-health-status",
			Output:      "TEXT_OR_CSV",
			Compression: "GZIP",
			Granularity: "MONTHLY",
			Overwrite:   "CREATE_NEW_REPORT",
			Destination: "us-east-1",
		},
		{
			Ref:         "cur2-complete-nonpreferred",
			Health:      "HEALTHY",
			Output:      "PARQUET",
			Compression: "PARQUET",
			Granularity: "DAILY",
			Overwrite:   "OVERWRITE_REPORT",
			Destination: "us-east-1",
		},
		{
			Ref:         "cur2-unhealthy-complete",
			Health:      "UNHEALTHY",
			Output:      "TEXT_OR_CSV",
			Compression: "GZIP",
			Granularity: "MONTHLY",
			Overwrite:   "CREATE_NEW_REPORT",
			Destination: "us-east-1",
		},
	})

	if ranked[0].Ref != "cur2-complete-nonpreferred" {
		t.Fatalf("first ranked ref = %q, want complete valid non-preferred candidate", ranked[0].Ref)
	}
	if ranked[len(ranked)-1].Ref != "cur2-unhealthy-complete" {
		t.Fatalf("last ranked ref = %q, want unhealthy candidate last", ranked[len(ranked)-1].Ref)
	}
	incompleteRefs := map[string]bool{
		ranked[1].Ref: true,
		ranked[2].Ref: true,
	}
	if !incompleteRefs["cur2-incomplete-health"] || !incompleteRefs["cur2-incomplete-health-status"] {
		t.Fatalf("middle ranked refs = %#v, want both incomplete candidates", []string{ranked[1].Ref, ranked[2].Ref})
	}
}

func TestHasSupportedCUR2SettingsRequiresMatildaSupportedShape(t *testing.T) {
	supportedCSV := cur2Candidate{
		Output:          "TEXT_OR_CSV",
		Compression:     "GZIP",
		Granularity:     "MONTHLY",
		Overwrite:       "CREATE_NEW_REPORT",
		OutputType:      "CUSTOM",
		RefreshCadence:  "SYNCHRONOUS",
		IncludeResource: "TRUE",
	}
	if !hasSupportedCUR2Settings(supportedCSV) {
		t.Fatalf("hasSupportedCUR2Settings(%#v) = false, want true", supportedCSV)
	}

	supportedParquet := supportedCSV
	supportedParquet.Output = "PARQUET"
	supportedParquet.Compression = "PARQUET"
	supportedParquet.Granularity = "HOURLY"
	supportedParquet.Overwrite = "OVERWRITE_REPORT"
	supportedParquet.IncludeResource = ""
	if !hasSupportedCUR2Settings(supportedParquet) {
		t.Fatalf("hasSupportedCUR2Settings(%#v) = false, want true", supportedParquet)
	}

	unsupported := supportedCSV
	unsupported.OutputType = "CUSTOM_AND_STANDARD"
	if hasSupportedCUR2Settings(unsupported) {
		t.Fatalf("hasSupportedCUR2Settings(%#v) = true, want false", unsupported)
	}
}

func TestHasSupportedCUR2GranularityAndOverwriteRejectUnsupportedValues(t *testing.T) {
	for _, granularity := range []string{"", "WEEKLY"} {
		if hasSupportedCUR2Granularity(cur2Candidate{Granularity: granularity}) {
			t.Fatalf("hasSupportedCUR2Granularity(%q) = true, want false", granularity)
		}
	}

	for _, overwrite := range []string{"", "APPEND_REPORT"} {
		if hasSupportedCUR2Overwrite(cur2Candidate{Overwrite: overwrite}) {
			t.Fatalf("hasSupportedCUR2Overwrite(%q) = true, want false", overwrite)
		}
	}
}

func TestRankedCUR2CandidatesPutsIncompleteMetadataBeforeUnsupportedSettings(t *testing.T) {
	ranked := rankedCUR2Candidates([]cur2Candidate{
		{
			Ref:            "cur2-unsupported-settings",
			Health:         "HEALTHY",
			Output:         "TEXT_OR_CSV",
			Compression:    "ZIP",
			Granularity:    "MONTHLY",
			Overwrite:      "CREATE_NEW_REPORT",
			OutputType:     "CUSTOM",
			RefreshCadence: "SYNCHRONOUS",
			Destination:    "us-east-1",
		},
		{
			Ref:    "cur2-incomplete-health",
			Health: "HEALTHY",
		},
	})

	got := []string{ranked[0].Ref, ranked[1].Ref}
	want := []string{"cur2-incomplete-health", "cur2-unsupported-settings"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranked refs = %#v, want %#v", got, want)
	}
}

func TestRankedCUR2CandidatesRequiresDestinationForSupportedRecommendation(t *testing.T) {
	ranked := rankedCUR2Candidates([]cur2Candidate{
		{
			Ref:            "cur2-missing-destination",
			Health:         "HEALTHY",
			Output:         "TEXT_OR_CSV",
			Compression:    "GZIP",
			Granularity:    "MONTHLY",
			Overwrite:      "CREATE_NEW_REPORT",
			OutputType:     "CUSTOM",
			RefreshCadence: "SYNCHRONOUS",
		},
		{
			Ref:            "cur2-complete-nonpreferred",
			Health:         "HEALTHY",
			Output:         "PARQUET",
			Compression:    "PARQUET",
			Granularity:    "DAILY",
			Overwrite:      "OVERWRITE_REPORT",
			OutputType:     "CUSTOM",
			RefreshCadence: "SYNCHRONOUS",
			Destination:    "us-east-1",
		},
	})

	if ranked[0].Ref != "cur2-complete-nonpreferred" {
		t.Fatalf("first ranked ref = %q, want complete non-preferred candidate ahead of missing destination", ranked[0].Ref)
	}
	if isRecommendedCUR2Candidate(ranked[1]) {
		t.Fatalf("candidate with missing destination was recommended: %#v", ranked[1])
	}
}

func TestRunAWSBillingUsesSingleCandidateOnDefaultConfirmation(t *testing.T) {
	calls := []workflow.ExecutionOptions{}
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, options)
		exportRef := ""
		if options.Selectors != nil && options.Selectors.AWS != nil {
			exportRef = options.Selectors.AWS.CUR2ExportRef
		}
		switch exportRef {
		case "":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_selection_required", []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-cacacacacacacaca"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_include_resources", Value: "FALSE"},
				{Key: "candidate_1_pre_selection_metadata_status", Value: "preferred"},
				{Key: "candidate_1_matilda_support", Value: "preferred"},
				{Key: "candidate_1_primary_issue", Value: "none"},
				{Key: "candidate_1_required_next_action", Value: "run full readiness preflight after selection."},
			})
		case "cur2-cacacacacacacaca":
			return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", []workflow.PlanEvidence{
				{Key: "output_format", Value: "TEXT_OR_CSV"},
				{Key: "compression", Value: "GZIP"},
				{Key: "time_granularity", Value: "MONTHLY"},
				{Key: "overwrite", Value: "CREATE_NEW_REPORT"},
			})
		default:
			t.Fatalf("unexpected export ref %q", exportRef)
			return workflow.CapabilityReport{}
		}
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("preflight calls = %d, want initial discovery plus selected export preflight", len(calls))
	}
	if calls[1].Selectors.AWS.CUR2ExportRef != "cur2-cacacacacacacaca" {
		t.Fatalf("selected preflight ref = %q, want auto-selected candidate ref", calls[1].Selectors.AWS.CUR2ExportRef)
	}
	for _, want := range []string{
		"Detected one usable CUR 2.0 export cur2-cacacacacacacaca",
		"Recommendation: preferred Rapid Assessment billing export shape.",
		"Full readiness checks run after selection.",
		"Use this detected CUR 2.0 export? [Y/n]",
		"Running readiness preflight for selected CUR 2.0 export cur2-cacacacacacacaca",
		"Result: ready",
		"Support code: aws_cur2_preflight_ready",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if strings.Contains(output, "Select AWS CUR 2.0 export") {
		t.Fatalf("output prompted for a single candidate: %s", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingSingleCandidateDeclinePreparesNewCUR2Plan(t *testing.T) {
	calls := []guidedAWSBillingCall{}
	preflightRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		if options.Selectors != nil && options.Selectors.AWS != nil && options.Selectors.AWS.CUR2ExportRef != "" {
			t.Fatalf("selected preflight should not run when user declines the detected export: %#v", options.Selectors.AWS)
		}
		return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_selection_required", []workflow.PlanEvidence{
			{Key: "candidate_1_export_ref", Value: "cur2-cacacacacacacaca"},
			{Key: "candidate_1_health", Value: "HEALTHY"},
			{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
			{Key: "candidate_1_compression", Value: "GZIP"},
			{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
			{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
			{Key: "candidate_1_destination_region", Value: "us-east-1"},
			{Key: "candidate_1_output_type", Value: "CUSTOM"},
			{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
			{Key: "candidate_1_include_resources", Value: "FALSE"},
			{Key: "candidate_1_pre_selection_metadata_status", Value: "preferred"},
			{Key: "candidate_1_matilda_support", Value: "preferred"},
			{Key: "candidate_1_primary_issue", Value: "none"},
			{Key: "candidate_1_required_next_action", Value: "run full readiness preflight after selection."},
		})
	})
	applyRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		return guidedCreateCUR2PlanReport(got)
	})
	registry := testAWSBillingRegistry(t, preflightRunner, applyRunner)
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\nn\n1\n\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	assertGuidedCreateCUR2PlanCalls(t, calls)
	for _, want := range []string{
		"Detected one usable CUR 2.0 export cur2-cacacacacacacaca",
		"Use this detected CUR 2.0 export? [Y/n]",
		"Select AWS CUR 2.0 destination",
		"Preparing a new Matilda AWS CUR 2.0 setup plan.",
		"Result: ready_with_manual_steps",
		"Support code: aws_cur2_create_export_approval_required",
		"Apply this AWS CUR 2.0 setup plan now? [y/N]",
		"matilda-prep rapid-assessment billing aws apply-prereqs --profile default --region us-east-1 --create-cur2-export",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"Running readiness preflight for selected CUR 2.0 export",
		"Reproduce with:",
		"--export-ref",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingSingleNonSelectableCandidateCanPrepareNewCUR2Plan(t *testing.T) {
	calls := []guidedAWSBillingCall{}
	preflightRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		if options.Selectors != nil && options.Selectors.AWS != nil && options.Selectors.AWS.CUR2ExportRef != "" {
			t.Fatalf("selected preflight should not run when user chooses create-new setup: %#v", options.Selectors.AWS)
		}
		return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_selection_required", []workflow.PlanEvidence{
			{Key: "candidate_1_export_ref", Value: "cur2-eccececcececcece"},
			{Key: "candidate_1_health", Value: "WARNING"},
			{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
			{Key: "candidate_1_compression", Value: "GZIP"},
			{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
			{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
			{Key: "candidate_1_output_type", Value: "CUSTOM"},
			{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
			{Key: "candidate_1_destination_region", Value: "us-east-1"},
		})
	})
	applyRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		return guidedCreateCUR2PlanReport(got)
	})
	registry := testAWSBillingRegistry(t, preflightRunner, applyRunner)
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n2\n1\n\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	assertGuidedCreateCUR2PlanCalls(t, calls)
	for _, want := range []string{
		"One AWS CUR 2.0 export candidate needs review.",
		"Blocker: pre-selection metadata has unsupported settings: health status WARNING.",
		"Select AWS CUR 2.0 action",
		"Review this CUR 2.0 export with full readiness preflight",
		"Prepare a new Matilda CUR 2.0 setup plan",
		"Select AWS CUR 2.0 destination",
		"Preparing a new Matilda AWS CUR 2.0 setup plan.",
		"Apply this AWS CUR 2.0 setup plan now? [y/N]",
		"matilda-prep rapid-assessment billing aws apply-prereqs --profile default --region us-east-1 --create-cur2-export",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"Review with:",
		"Running readiness preflight for selected CUR 2.0 export",
		"--export-ref",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingSingleNonSelectableCandidateCanRunSelectedPreflight(t *testing.T) {
	calls := []workflow.ExecutionOptions{}
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, options)
		exportRef := ""
		if options.Selectors != nil && options.Selectors.AWS != nil {
			exportRef = options.Selectors.AWS.CUR2ExportRef
		}
		switch exportRef {
		case "":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_selection_required", []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-eccececcececcece"},
				{Key: "candidate_1_health", Value: "WARNING"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
			})
		case "cur2-eccececcececcece":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_output_settings_blocked", []workflow.PlanEvidence{
				{Key: "output_format", Value: "TEXT_OR_CSV"},
				{Key: "compression", Value: "GZIP"},
				{Key: "time_granularity", Value: "MONTHLY"},
				{Key: "overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "blocker", Value: "AWS CUR 2.0 export needs full readiness review."},
			})
		default:
			t.Fatalf("unexpected export ref %q", exportRef)
			return workflow.CapabilityReport{}
		}
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("preflight calls = %d, want initial discovery plus selected export preflight", len(calls))
	}
	if calls[1].Selectors.AWS.CUR2ExportRef != "cur2-eccececcececcece" {
		t.Fatalf("selected preflight ref = %q, want single candidate ref", calls[1].Selectors.AWS.CUR2ExportRef)
	}
	for _, want := range []string{
		"One AWS CUR 2.0 export candidate needs review.",
		"Select AWS CUR 2.0 action",
		"Running readiness preflight for selected CUR 2.0 export cur2-eccececcececcece",
		"Support code: aws_cur2_output_settings_blocked",
		"--export-ref cur2-eccececcececcece",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if strings.Contains(output, "Preparing a new Matilda AWS CUR 2.0 setup plan.") {
		t.Fatalf("output = %q, want no create-new setup when selected export is reviewed", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingCreateCUR2PlanApprovedInGuidedModeUsesPlanBoundApprovals(t *testing.T) {
	calls := []guidedAWSBillingCall{}
	preflightRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		if options.Selectors != nil && options.Selectors.AWS != nil && options.Selectors.AWS.CUR2ExportRef != "" {
			t.Fatalf("selected preflight should not run when user chooses create-new setup: %#v", options.Selectors.AWS)
		}
		return guidedAmbiguousCUR2SelectionReport(got)
	})
	applyRunner := workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
		if len(options.Approvals) == 0 {
			return guidedCreateCUR2PlanReport(got)
		}
		return guidedCreateCUR2AppliedReport(got)
	})
	registry := testAWSBillingRegistry(t, preflightRunner, applyRunner)
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n3\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("registry calls = %d, want preflight, plan preview, approved apply", len(calls))
	}
	assertGuidedCreateCUR2PlanCallAt(t, calls[1])
	previewReport := guidedCreateCUR2PlanReport(calls[1].Request)
	previewPlanID := testPlanIDForReport(t, calls[1].Request, calls[1].Options, previewReport)
	apply := calls[2]
	if apply.Request != testAWSBillingApplyPrereqsRequest() {
		t.Fatalf("apply request = %#v, want AWS billing apply-prereqs", apply.Request)
	}
	if apply.Options.AWSBillingOperation != workflow.AWSBillingOperationCreateCUR2Export {
		t.Fatalf("apply operation = %q, want create_cur2_export", apply.Options.AWSBillingOperation)
	}
	if apply.Options.Selectors == nil || apply.Options.Selectors.AWS == nil {
		t.Fatalf("apply selectors missing: %#v", apply.Options.Selectors)
	}
	if apply.Options.Selectors.AWS.Profile != "default" || apply.Options.Selectors.AWS.Region != "us-east-1" {
		t.Fatalf("apply selectors = %#v, want selected profile and region", apply.Options.Selectors.AWS)
	}
	if apply.Options.Selectors.AWS.CUR2ExportRef != "" {
		t.Fatalf("apply export ref = %q, want empty create-new selector", apply.Options.Selectors.AWS.CUR2ExportRef)
	}
	if len(apply.Options.Approvals) != 2 {
		t.Fatalf("apply approvals = %#v, want two mutating step approvals", apply.Options.Approvals)
	}
	for _, stepID := range []string{
		workflow.AWSCUR2CreateBucketOperationID,
		workflow.AWSCUR2CreateExportOperationID,
	} {
		if !workflow.HasApprovedPlanStep(apply.Options, previewPlanID, stepID) {
			t.Fatalf("apply approvals = %#v, want approved step %s bound to %s", apply.Options.Approvals, stepID, previewPlanID)
		}
	}
	for _, want := range []string{
		"Apply this AWS CUR 2.0 setup plan now? [y/N]",
		"Applying approved Matilda AWS CUR 2.0 setup plan.",
		"Support code: aws_cur2_create_export_created",
		"Cloud changes were made for the approved setup plan.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"Running readiness preflight for selected CUR 2.0 export",
		"--export-ref",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingCreateCUR2PlanReuseDoesNotPromptForApply(t *testing.T) {
	calls := []guidedAWSBillingCall{}
	registry := testAWSBillingRegistry(t,
		workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
			return guidedAmbiguousCUR2SelectionReport(got)
		}),
		workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
			return guidedCreateCUR2ReuseReport(got)
		}),
	)
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n3\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("registry calls = %d, want preflight plus non-mutating reuse result", len(calls))
	}
	assertGuidedCreateCUR2PlanCallAt(t, calls[1])
	for _, want := range []string{
		"Result: ready",
		"Support code: aws_cur2_create_export_reused",
		"No approval required: Reuse existing Matilda AWS CUR 2.0 export",
		"No cloud changes were made.",
		"No mutation approval is required for this result.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"Apply this AWS CUR 2.0 setup plan now? [y/N]",
		"Applying approved Matilda AWS CUR 2.0 setup plan.",
		"--approve-step",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingCreateCUR2PlanBlockedDoesNotPromptForApply(t *testing.T) {
	calls := []guidedAWSBillingCall{}
	registry := testAWSBillingRegistry(t,
		workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
			return guidedAmbiguousCUR2SelectionReport(got)
		}),
		workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			calls = append(calls, guidedAWSBillingCall{Request: got, Options: options})
			return guidedCreateCUR2BlockedReport(got)
		}),
	)
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n3\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("registry calls = %d, want preflight plus blocked setup plan", len(calls))
	}
	assertGuidedCreateCUR2PlanCallAt(t, calls[1])
	for _, want := range []string{
		"Result: blocked",
		"Support code: aws_s3_bucket_inaccessible",
		"Setup plan:",
		"No approval required: Resolve AWS S3 bucket candidate access",
		"Current state: The generated same-account S3 bucket candidate could not be verified as available to create or safely owned by this account.",
		"Target state: Matilda Cloud Prep can show an approval-required plan to create or reuse the generated bucket, update its Data Exports delivery policy, and create the CUR 2.0 export.",
		"Required permission: s3:ListBucket for existing bucket checks",
		"Validation: Do not manually create or select arbitrary buckets for the normal guided path.",
		"No cloud changes were made.",
		"This setup plan is blocked and cannot be approved until the blocker is resolved.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"Apply this AWS CUR 2.0 setup plan now? [y/N]",
		"Applying approved Matilda AWS CUR 2.0 setup plan.",
		"--approve-step",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

type guidedAWSBillingCall struct {
	Request workflow.Request
	Options workflow.ExecutionOptions
}

func testAWSBillingRegistry(t *testing.T, preflightRunner workflow.CapabilityRunner, applyPrereqsRunner workflow.CapabilityRunner) workflow.Registry {
	t.Helper()
	registry, err := workflow.NewRegistry(
		workflow.Capability{
			Request: awsBillingRequest(),
			Runner:  preflightRunner,
		},
		workflow.Capability{
			Request: testAWSBillingApplyPrereqsRequest(),
			Runner:  applyPrereqsRunner,
		},
	)
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	return registry
}

func testAWSBillingApplyPrereqsRequest() workflow.Request {
	return workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionApplyPrereqs,
	}
}

func assertGuidedCreateCUR2PlanCalls(t *testing.T, calls []guidedAWSBillingCall) {
	t.Helper()
	if len(calls) != 2 {
		t.Fatalf("registry calls = %d, want initial preflight plus create-new plan", len(calls))
	}
	if calls[0].Request != awsBillingRequest() {
		t.Fatalf("first request = %#v, want AWS billing preflight", calls[0].Request)
	}
	if calls[0].Options.AWSBillingOperation != "" {
		t.Fatalf("first operation = %q, want empty preflight operation", calls[0].Options.AWSBillingOperation)
	}
	if calls[0].Options.Selectors == nil || calls[0].Options.Selectors.AWS == nil {
		t.Fatalf("first call selectors missing: %#v", calls[0].Options.Selectors)
	}
	if calls[0].Options.Selectors.AWS.CUR2ExportRef != "" {
		t.Fatalf("first export ref = %q, want empty discovery selector", calls[0].Options.Selectors.AWS.CUR2ExportRef)
	}

	assertGuidedCreateCUR2PlanCallAt(t, calls[1])
}

func assertGuidedCreateCUR2PlanCallAt(t *testing.T, call guidedAWSBillingCall) {
	t.Helper()
	if call.Request != testAWSBillingApplyPrereqsRequest() {
		t.Fatalf("request = %#v, want AWS billing apply-prereqs", call.Request)
	}
	if call.Options.InterfaceMode != workflow.InterfaceModeGuided {
		t.Fatalf("interface mode = %q, want guided", call.Options.InterfaceMode)
	}
	if call.Options.AWSBillingOperation != workflow.AWSBillingOperationCreateCUR2Export {
		t.Fatalf("operation = %q, want create_cur2_export", call.Options.AWSBillingOperation)
	}
	if call.Options.Selectors == nil || call.Options.Selectors.AWS == nil {
		t.Fatalf("call selectors missing: %#v", call.Options.Selectors)
	}
	if call.Options.Selectors.AWS.Profile != "default" || call.Options.Selectors.AWS.Region != "us-east-1" {
		t.Fatalf("AWS selectors = %#v, want selected profile and region", call.Options.Selectors.AWS)
	}
	if call.Options.Selectors.AWS.CUR2ExportRef != "" {
		t.Fatalf("export ref = %q, want empty create-new selector", call.Options.Selectors.AWS.CUR2ExportRef)
	}
	if len(call.Options.Approvals) != 0 {
		t.Fatalf("approvals = %#v, want none for plan-only create-new", call.Options.Approvals)
	}
}

func testPlanIDForReport(t *testing.T, request workflow.Request, options workflow.ExecutionOptions, report workflow.CapabilityReport) string {
	t.Helper()
	if report.PlanInput == nil {
		t.Fatal("report PlanInput missing")
	}
	input := *report.PlanInput
	input.Request = request
	input.ExecutionOptions = options
	plan, err := workflow.BuildExecutionPlan(input)
	if err != nil {
		t.Fatalf("BuildExecutionPlan returned error: %v", err)
	}
	return plan.PlanID
}

func guidedCreateCUR2PlanResult(t *testing.T) workflow.Result {
	t.Helper()
	request := testAWSBillingApplyPrereqsRequest()
	source := billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}
	options, err := awsBillingOptions(source)
	if err != nil {
		t.Fatalf("awsBillingOptions returned error: %v", err)
	}
	options.AWSBillingOperation = workflow.AWSBillingOperationCreateCUR2Export
	options, err = workflow.NormalizeExecutionOptionsForRequest(request, options)
	if err != nil {
		t.Fatalf("NormalizeExecutionOptionsForRequest returned error: %v", err)
	}
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: request,
		Runner: workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			return guidedCreateCUR2PlanReport(got)
		}),
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	result := registry.ExecuteContext(context.Background(), request, options)
	if result.Plan == nil {
		t.Fatalf("result plan missing: %#v", result)
	}
	return result
}

func guidedCreateCUR2PlanReport(request workflow.Request) workflow.CapabilityReport {
	handles := []workflow.SourceHandle{{
		Label: "AWS CUR 2.0 Create-New Export",
		URI:   "docs/references/aws/aws-cur2-create-new-export.md",
	}}
	return workflow.CapabilityReport{
		Status:        workflow.RunStatusManualSteps,
		SupportStatus: workflow.SupportGuided,
		Code:          "aws_cur2_create_export_approval_required",
		Message:       "Review the AWS CUR 2.0 setup plan and approve each mutating step before cloud changes are made.",
		Mutated:       false,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified for account-ending-9012 before CUR 2.0 setup.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageSingleAccount,
				Summary:        "AWS billing coverage is single-account for this guided setup test.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{
				{
					ID:                        workflow.AWSCUR2CreateBucketOperationID,
					Intent:                    workflow.PlanStepCreate,
					Title:                     "Create Matilda AWS billing export bucket",
					Description:               "Create a generated same-account S3 bucket for AWS CUR 2.0 delivery.",
					Reason:                    "AWS Data Exports requires an S3 destination before a CUR 2.0 export can be created.",
					ApprovalKind:              "cloud_mutation",
					CurrentState:              "No matching generated same-account S3 bucket is available.",
					TargetState:               "A generated same-account S3 bucket exists in the selected region.",
					RequiredPermission:        "s3:CreateBucket",
					CredentialMaterialTouched: false,
					Validation:                "The bucket is checked with the expected bucket owner before policy or export creation continues.",
					Rollback:                  "The tool does not delete S3 buckets automatically.",
					SourceHandles:             handles,
				},
				{
					ID:                        workflow.AWSCUR2CreateExportOperationID,
					Intent:                    workflow.PlanStepCreate,
					Title:                     "Create Matilda AWS CUR 2.0 export",
					Description:               "Create a Matilda-specific AWS CUR 2.0 export using the verified Rapid Assessment - Billing Based setup defaults.",
					Reason:                    "Matilda Rapid Assessment - Billing Based needs a CUR 2.0 billing export when no reusable export is selected.",
					ApprovalKind:              "cloud_mutation",
					CurrentState:              "No matching Matilda-generated CUR 2.0 export exists for the selected account and region.",
					TargetState:               "A Matilda-generated CUR 2.0 export writes monthly billing data to the generated S3 destination.",
					RequiredPermission:        "bcm-data-exports:CreateExport",
					CredentialMaterialTouched: false,
					Validation:                "The created export request uses Matilda-supported CUR 2.0 settings.",
					Rollback:                  "The tool does not delete Data Exports resources automatically.",
					SourceHandles:             handles,
				},
			},
			Checks: []workflow.PlanCheck{{
				ID:      "aws_cur2_create_export_plan_facts",
				Status:  workflow.CheckWarn,
				Title:   "AWS CUR 2.0 setup plan",
				Message: "A Matilda-specific AWS CUR 2.0 setup plan was generated.",
				Evidence: []workflow.PlanEvidence{
					{Key: "data_exports_region", Value: "us-east-1"},
					{Key: "s3_region", Value: "us-east-1"},
					{Key: "coverage_status", Value: string(workflow.CoverageSingleAccount)},
				},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}

func guidedExistingBucketSelectionReport(request workflow.Request) workflow.CapabilityReport {
	handles := guidedCreateCUR2SourceHandles()
	return workflow.CapabilityReport{
		Status:        workflow.RunStatusManualSteps,
		SupportStatus: workflow.SupportGuided,
		Code:          "aws_cur2_existing_bucket_selection_required",
		Message:       "Select one verified same-account S3 bucket before generating the AWS CUR 2.0 setup plan.",
		Mutated:       false,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified for account-ending-9012 before existing bucket discovery.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageSingleAccount,
				Summary:        "AWS billing coverage is single-account for this guided setup test.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{{
				Intent:                    workflow.PlanStepGuide,
				Title:                     "Select existing same-account S3 bucket",
				Description:               "Choose one S3 bucket discovered from the verified AWS account before a CUR 2.0 setup plan is generated.",
				Reason:                    "Matilda Cloud Prep must bind the exact destination before any bucket policy or Data Exports change can be approved.",
				ApprovalKind:              "not_required",
				CurrentState:              "Existing same-account S3 buckets were discovered.",
				TargetState:               "One bucket is selected by safe reference for the setup plan.",
				RequiredPermission:        "s3:ListAllMyBuckets",
				CredentialMaterialTouched: false,
				Validation:                "The selected safe bucket ref is resolved again before planning.",
				Rollback:                  "No cloud change was made.",
				SourceHandles:             handles,
			}},
			Checks: []workflow.PlanCheck{{
				ID:      "aws_cur2_existing_bucket_selection_required",
				Status:  workflow.CheckWarn,
				Title:   "AWS existing S3 bucket selection",
				Message: "Existing same-account S3 buckets are available for selection by safe reference.",
				Evidence: []workflow.PlanEvidence{
					{Key: "candidate_1_bucket_ref", Value: "s3b-abcdefghijklmnop"},
					{Key: "candidate_1_bucket_label", Value: "matilda-existing-cur2", PlanIDExcluded: true},
					{Key: "candidate_1_bucket_region", Value: "us-east-1"},
				},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}

func guidedExistingBucketSelectionNoCandidatesReport(request workflow.Request) workflow.CapabilityReport {
	report := guidedExistingBucketSelectionReport(request)
	report.Message = "No existing same-account S3 buckets were discovered for the selected account and Region."
	if report.PlanInput == nil {
		return report
	}
	if len(report.PlanInput.Steps) > 0 {
		report.PlanInput.Steps[0].CurrentState = "No existing same-account S3 buckets were discovered."
		report.PlanInput.Steps[0].TargetState = "Use the generated same-account Matilda S3 bucket destination or add a suitable owned bucket before retrying existing-bucket selection."
	}
	if len(report.PlanInput.Checks) > 0 {
		report.PlanInput.Checks[0].Message = "No existing same-account S3 buckets were returned for selection."
		report.PlanInput.Checks[0].Evidence = []workflow.PlanEvidence{{Key: "candidate_count", Value: "0"}}
	}
	return report
}

func guidedCreateCUR2ExistingBucketPlanReport(request workflow.Request) workflow.CapabilityReport {
	handles := guidedCreateCUR2SourceHandles()
	return workflow.CapabilityReport{
		Status:        workflow.RunStatusManualSteps,
		SupportStatus: workflow.SupportGuided,
		Code:          "aws_cur2_create_export_approval_required",
		Message:       "Review the AWS CUR 2.0 setup plan and approve each mutating step before cloud changes are made.",
		Mutated:       false,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified for account-ending-9012 before CUR 2.0 setup.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageSingleAccount,
				Summary:        "AWS billing coverage is single-account for this guided setup test.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{
				{
					ID:                        workflow.AWSCUR2MergeBucketPolicyOperationID,
					Intent:                    workflow.PlanStepRepair,
					Title:                     "Allow AWS Data Exports delivery to the selected bucket",
					Description:               "Merge the scoped AWS Data Exports delivery statement into the selected same-account bucket policy.",
					Reason:                    "AWS requires the bucket policy to allow Data Exports to write report objects with source conditions.",
					ApprovalKind:              "cloud_mutation",
					CurrentState:              "The selected same-account S3 bucket exists but does not yet contain the scoped Data Exports delivery statement.",
					TargetState:               "The selected bucket policy allows Data Exports delivery for the selected account and export scope.",
					RequiredPermission:        "s3:GetBucketPolicy, s3:PutBucketPolicy",
					CredentialMaterialTouched: false,
					Validation:                "The policy is read, parsed, merged, and written with the expected bucket owner.",
					Rollback:                  "The tool does not remove bucket policy statements automatically.",
					SourceHandles:             handles,
				},
				{
					ID:                        workflow.AWSCUR2CreateExportOperationID,
					Intent:                    workflow.PlanStepCreate,
					Title:                     "Create Matilda AWS CUR 2.0 export",
					Description:               "Create a Matilda-specific AWS CUR 2.0 export using the selected same-account S3 bucket.",
					Reason:                    "Matilda Rapid Assessment - Billing Based needs a CUR 2.0 billing export when no reusable export is selected.",
					ApprovalKind:              "cloud_mutation",
					CurrentState:              "No matching Matilda-generated CUR 2.0 export exists for the selected bucket and region.",
					TargetState:               "A Matilda-generated CUR 2.0 export writes monthly billing data to the selected S3 destination.",
					RequiredPermission:        "bcm-data-exports:CreateExport",
					CredentialMaterialTouched: false,
					Validation:                "The created export request uses Matilda-supported CUR 2.0 settings and the selected bucket owner.",
					Rollback:                  "The tool does not delete Data Exports resources automatically.",
					SourceHandles:             handles,
				},
			},
			Checks: []workflow.PlanCheck{{
				ID:      "aws_cur2_create_export_plan_facts",
				Status:  workflow.CheckWarn,
				Title:   "AWS CUR 2.0 setup plan",
				Message: "A Matilda-specific AWS CUR 2.0 setup plan was generated.",
				Evidence: []workflow.PlanEvidence{
					{Key: "destination_mode", Value: string(workflow.AWSCUR2DestinationExistingSameAccount)},
					{Key: "selected_s3_bucket_ref", Value: "s3b-abcdefghijklmnop"},
					{Key: "data_exports_region", Value: "us-east-1"},
					{Key: "s3_region", Value: "us-east-1"},
					{Key: "coverage_status", Value: string(workflow.CoverageSingleAccount)},
				},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}

func guidedCreateCUR2SourceHandles() []workflow.SourceHandle {
	return []workflow.SourceHandle{{
		Label: "AWS CUR 2.0 Create-New Export",
		URI:   "docs/references/aws/aws-cur2-create-new-export.md",
	}, {
		Label: "AWS CUR 2.0 Existing Bucket Selection",
		URI:   "docs/references/aws/aws-cur2-existing-bucket-selection.md",
	}}
}

func guidedAmbiguousCUR2SelectionReport(request workflow.Request) workflow.CapabilityReport {
	return guidedCapabilityReport(request, workflow.RunStatusBlocked, "aws_cur2_export_ambiguous", []workflow.PlanEvidence{
		{Key: "candidate_1_export_ref", Value: "cur2-acacacacacacacac"},
		{Key: "candidate_1_health", Value: "HEALTHY"},
		{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
		{Key: "candidate_1_compression", Value: "GZIP"},
		{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
		{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
		{Key: "candidate_1_output_type", Value: "CUSTOM"},
		{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
		{Key: "candidate_1_destination_region", Value: "us-east-1"},
		{Key: "candidate_2_export_ref", Value: "cur2-bdbdbdbdbdbdbdbd"},
		{Key: "candidate_2_health", Value: "HEALTHY"},
		{Key: "candidate_2_output_format", Value: "PARQUET"},
		{Key: "candidate_2_compression", Value: "PARQUET"},
		{Key: "candidate_2_time_granularity", Value: "DAILY"},
		{Key: "candidate_2_overwrite", Value: "OVERWRITE_REPORT"},
		{Key: "candidate_2_output_type", Value: "CUSTOM"},
		{Key: "candidate_2_refresh_cadence", Value: "SYNCHRONOUS"},
		{Key: "candidate_2_destination_region", Value: "us-west-2"},
	})
}

func guidedCreateCUR2AppliedReport(request workflow.Request) workflow.CapabilityReport {
	report := guidedCreateCUR2PlanReport(request)
	report.Status = workflow.RunStatusManualSteps
	report.SupportStatus = workflow.SupportSupported
	report.Code = "aws_cur2_create_export_created"
	report.Message = "AWS CUR 2.0 export was created. Initial delivery and previous-month data availability can still require follow-up validation."
	report.Mutated = true
	return report
}

func guidedCreateCUR2ReuseReport(request workflow.Request) workflow.CapabilityReport {
	handles := []workflow.SourceHandle{{
		Label: "AWS CUR 2.0 Create-New Export",
		URI:   "docs/references/aws/aws-cur2-create-new-export.md",
	}}
	return workflow.CapabilityReport{
		Status:        workflow.StatusReady,
		SupportStatus: workflow.SupportSupported,
		Code:          "aws_cur2_create_export_reused",
		Message:       "An existing Matilda-generated AWS CUR 2.0 export matches the setup contract.",
		Mutated:       false,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified for account-ending-9012 before CUR 2.0 setup.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageSingleAccount,
				Summary:        "AWS billing coverage is single-account for this guided setup test.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{{
				Intent:                    workflow.PlanStepReuse,
				Title:                     "Reuse existing Matilda AWS CUR 2.0 export",
				Description:               "Reuse the existing Matilda-generated AWS CUR 2.0 export that matches the setup contract.",
				Reason:                    "No new CUR 2.0 cloud resource is needed when a matching Matilda-generated export already exists.",
				ApprovalKind:              "not_required",
				CurrentState:              "A matching Matilda-generated CUR 2.0 export already exists.",
				TargetState:               "The matching CUR 2.0 export remains selected for Matilda Rapid Assessment billing.",
				RequiredPermission:        "Read-only AWS billing and S3 discovery permissions.",
				CredentialMaterialTouched: false,
				Validation:                "The existing export was matched to the Matilda CUR 2.0 setup contract.",
				Rollback:                  "No cloud change was made.",
				SourceHandles:             handles,
			}},
			Checks: []workflow.PlanCheck{{
				ID:      "aws_cur2_create_export_plan_facts",
				Status:  workflow.CheckPass,
				Title:   "AWS CUR 2.0 setup plan",
				Message: "A Matilda-generated CUR 2.0 export can be reused.",
				Evidence: []workflow.PlanEvidence{
					{Key: "mutated", Value: "false"},
					{Key: "coverage_status", Value: string(workflow.CoverageSingleAccount)},
				},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}

func guidedCreateCUR2BlockedReport(request workflow.Request) workflow.CapabilityReport {
	handles := []workflow.SourceHandle{{
		Label: "AWS CUR 2.0 Create-New Export",
		URI:   "docs/references/aws/aws-cur2-create-new-export.md",
	}}
	return workflow.CapabilityReport{
		Status:        workflow.RunStatusBlocked,
		SupportStatus: workflow.SupportBlocked,
		Code:          "aws_s3_bucket_inaccessible",
		Message:       "AWS CUR 2.0 create-export setup plan could not be built safely.",
		Mutated:       false,
		SourceHandles: handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request: request,
			OperatorIdentitySummary: workflow.OperatorIdentitySummary{
				IdentityStatus: "verified",
				Summary:        "AWS caller identity was verified for account-ending-9012 before CUR 2.0 setup.",
				SourceHandles:  handles,
			},
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageSingleAccount,
				Summary:        "AWS billing coverage is single-account for this guided setup test.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps: []workflow.PlanStep{{
				Intent:                    workflow.PlanStepBlocked,
				Title:                     "Resolve AWS S3 bucket candidate access",
				Description:               "Stop before AWS CUR 2.0 setup because AWS did not return enough S3 evidence for the generated destination bucket candidate.",
				Reason:                    "Matilda Cloud Prep creates or reuses only a generated same-account S3 bucket for this setup path, and must prove the bucket candidate is safe before creating a CUR 2.0 export.",
				ApprovalKind:              "not_required",
				CurrentState:              "The generated same-account S3 bucket candidate could not be verified as available to create or safely owned by this account.",
				TargetState:               "Matilda Cloud Prep can show an approval-required plan to create or reuse the generated bucket, update its Data Exports delivery policy, and create the CUR 2.0 export.",
				RequiredPermission:        "s3:ListBucket for existing bucket checks, plus s3:CreateBucket, s3:GetBucketPolicy, s3:PutBucketPolicy, bcm-data-exports:CreateExport, and cur:PutReportDefinition for approved setup.",
				CredentialMaterialTouched: false,
				Validation:                "Do not manually create or select arbitrary buckets for the normal guided path. Resolve S3 access ambiguity, then rerun apply-prereqs to get a new approval-required setup plan.",
				Rollback:                  "No cloud change was made.",
				SourceHandles:             handles,
			}},
			Checks: []workflow.PlanCheck{{
				ID:      "aws_s3_bucket_inaccessible",
				Status:  workflow.CheckFail,
				Title:   "AWS CUR 2.0 setup blocker",
				Message: "A required AWS S3 prerequisite could not be verified safely.",
				Evidence: []workflow.PlanEvidence{
					{Key: "code", Value: "aws_s3_bucket_inaccessible"},
					{Key: "mutated", Value: "false"},
				},
				SourceHandles: handles,
			}},
			SourceHandles: handles,
		},
	}
}

func TestAWSBillingFollowupCommandUsesCreateCUR2Operation(t *testing.T) {
	source := billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "default",
		Region:  "us-east-1",
	}
	result := workflow.Result{
		Code: "aws_cur2_create_export_approval_required",
		ExecutionOptions: workflow.ExecutionOptions{
			AWSBillingOperation: workflow.AWSBillingOperationCreateCUR2Export,
		},
	}

	label, command := awsBillingFollowupCommand(source, result)

	if label != "Next command:" {
		t.Fatalf("label = %q, want Next command", label)
	}
	if !strings.Contains(command, "apply-prereqs") || !strings.Contains(command, "--create-cur2-export") {
		t.Fatalf("command = %q, want create-new apply-prereqs command", command)
	}
	if strings.Contains(command, "preflight") || strings.Contains(command, "--export-ref") {
		t.Fatalf("command = %q, want no preflight or export selector", command)
	}
	assertGuidedOutputSafe(t, command)
}

func TestRunCreateCUR2SetupPlanBlocksUnsafeCredentialSource(t *testing.T) {
	called := false
	registry := testAWSBillingRegistry(t,
		workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			called = true
			return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
		}),
		workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			called = true
			return guidedCreateCUR2PlanReport(got)
		}),
	)

	result := runCreateCUR2SetupPlan(context.Background(), registry, billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "/private/tmp/profile",
		Region:  "us-east-1",
	})

	if called {
		t.Fatal("registry should not run for unsafe AWS credential source")
	}
	if result.Status != workflow.RunStatusBlocked || result.Code != "aws_config_invalid_selector" {
		t.Fatalf("result = %#v, want unsafe selector block", result)
	}
	assertGuidedOutputSafe(t, result.Message)
}

func TestShouldOfferCreateCUR2GuidedApplyRequiresCurrentApprovablePlan(t *testing.T) {
	base := guidedCreateCUR2PlanResult(t)
	if !shouldOfferCreateCUR2GuidedApply(base) {
		t.Fatalf("shouldOfferCreateCUR2GuidedApply returned false for approvable create-new plan: %#v", base.Plan.Approval)
	}

	tests := []struct {
		name   string
		mutate func(workflow.Result) workflow.Result
	}{
		{
			name: "not create-new result",
			mutate: func(result workflow.Result) workflow.Result {
				result.Request = awsBillingRequest()
				result.ExecutionOptions.AWSBillingOperation = ""
				result.Code = "aws_cur2_preflight_ready"
				return result
			},
		},
		{
			name: "already mutated",
			mutate: func(result workflow.Result) workflow.Result {
				result.Mutated = true
				return result
			},
		},
		{
			name: "missing plan",
			mutate: func(result workflow.Result) workflow.Result {
				result.Plan = nil
				return result
			},
		},
		{
			name: "approval not required",
			mutate: func(result workflow.Result) workflow.Result {
				plan := *result.Plan
				plan.Approval.Required = false
				result.Plan = &plan
				return result
			},
		},
		{
			name: "approval blocked",
			mutate: func(result workflow.Result) workflow.Result {
				plan := *result.Plan
				plan.Approval.Blocked = true
				result.Plan = &plan
				return result
			},
		},
		{
			name: "already approved",
			mutate: func(result workflow.Result) workflow.Result {
				plan := *result.Plan
				plan.Approval.Approved = true
				result.Plan = &plan
				return result
			},
		},
		{
			name: "missing approval plan ID",
			mutate: func(result workflow.Result) workflow.Result {
				plan := *result.Plan
				plan.Approval.ApprovalPlanID = ""
				result.Plan = &plan
				return result
			},
		},
		{
			name: "no mutating step",
			mutate: func(result workflow.Result) workflow.Result {
				plan := *result.Plan
				plan.Steps = append([]workflow.PlanStep(nil), result.Plan.Steps...)
				for index := range plan.Steps {
					plan.Steps[index].RequiresApproval = false
				}
				result.Plan = &plan
				return result
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if shouldOfferCreateCUR2GuidedApply(tt.mutate(base)) {
				t.Fatal("shouldOfferCreateCUR2GuidedApply returned true for non-applicable plan")
			}
		})
	}
}

func TestCreateCUR2SetupApprovalOptionsBuildsPlanBoundApprovals(t *testing.T) {
	source := billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}
	preview := guidedCreateCUR2PlanResult(t)

	options, err := createCUR2SetupApprovalOptions(source, preview)
	if err != nil {
		t.Fatalf("createCUR2SetupApprovalOptions returned error: %v", err)
	}
	if options.AWSBillingOperation != workflow.AWSBillingOperationCreateCUR2Export {
		t.Fatalf("operation = %q, want create_cur2_export", options.AWSBillingOperation)
	}
	if options.Selectors == nil || options.Selectors.AWS == nil {
		t.Fatalf("AWS selectors missing: %#v", options.Selectors)
	}
	if options.Selectors.AWS.Profile != "default" || options.Selectors.AWS.Region != "us-east-1" {
		t.Fatalf("AWS selectors = %#v, want selected profile and region", options.Selectors.AWS)
	}
	if options.Selectors.AWS.CUR2ExportRef != "" {
		t.Fatalf("CUR2ExportRef = %q, want empty create-new selector", options.Selectors.AWS.CUR2ExportRef)
	}
	for _, stepID := range []string{
		workflow.AWSCUR2CreateBucketOperationID,
		workflow.AWSCUR2CreateExportOperationID,
	} {
		if !workflow.HasApprovedPlanStep(options, preview.Plan.Approval.ApprovalPlanID, stepID) {
			t.Fatalf("approvals = %#v, want approved step %s", options.Approvals, stepID)
		}
	}
}

func TestCreateCUR2SetupApprovalOptionsRejectsUnsafeOrIncompletePreview(t *testing.T) {
	source := billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}
	base := guidedCreateCUR2PlanResult(t)

	tests := []struct {
		name    string
		source  billingguide.CredentialSource
		preview workflow.Result
	}{
		{
			name:    "unsafe source",
			source:  billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "/private/tmp/default", Region: "us-east-1"},
			preview: base,
		},
		{
			name:    "missing plan",
			source:  source,
			preview: workflow.Result{},
		},
		{
			name:   "missing approval plan ID",
			source: source,
			preview: func() workflow.Result {
				result := base
				plan := *base.Plan
				plan.Approval.ApprovalPlanID = ""
				result.Plan = &plan
				return result
			}(),
		},
		{
			name:   "approval not required",
			source: source,
			preview: func() workflow.Result {
				result := base
				plan := *base.Plan
				plan.Approval.Required = false
				result.Plan = &plan
				return result
			}(),
		},
		{
			name:   "approval blocked",
			source: source,
			preview: func() workflow.Result {
				result := base
				plan := *base.Plan
				plan.Approval.Blocked = true
				result.Plan = &plan
				return result
			}(),
		},
		{
			name:   "already approved",
			source: source,
			preview: func() workflow.Result {
				result := base
				plan := *base.Plan
				plan.Approval.Approved = true
				result.Plan = &plan
				return result
			}(),
		},
		{
			name:   "no mutating steps",
			source: source,
			preview: func() workflow.Result {
				result := base
				plan := *base.Plan
				plan.Steps = append([]workflow.PlanStep(nil), base.Plan.Steps...)
				for index := range plan.Steps {
					plan.Steps[index].RequiresApproval = false
				}
				result.Plan = &plan
				return result
			}(),
		},
		{
			name:   "unsupported operation ID",
			source: source,
			preview: func() workflow.Result {
				result := base
				plan := *base.Plan
				plan.Steps = append([]workflow.PlanStep(nil), base.Plan.Steps...)
				plan.Steps[0].ID = "aws.billing.cur2.unknown.create"
				result.Plan = &plan
				return result
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := createCUR2SetupApprovalOptions(tt.source, tt.preview); err == nil {
				t.Fatal("createCUR2SetupApprovalOptions returned nil error")
			}
		})
	}
}

func TestRunApprovedCreateCUR2SetupPlanBlocksWhenApprovalCannotBeBuilt(t *testing.T) {
	called := false
	registry := testAWSBillingRegistry(t,
		workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			called = true
			return guidedAmbiguousCUR2SelectionReport(got)
		}),
		workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
			called = true
			return guidedCreateCUR2AppliedReport(got)
		}),
	)

	result := runApprovedCreateCUR2SetupPlan(context.Background(), registry, billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "default",
		Region:  "us-east-1",
	}, workflow.Result{})

	if called {
		t.Fatal("registry should not run when approval options cannot be built")
	}
	if result.Status != workflow.RunStatusBlocked || result.Code != "aws_cur2_create_export_approval_unavailable" {
		t.Fatalf("result = %#v, want approval-unavailable block", result)
	}
	assertGuidedOutputSafe(t, result.Message)
}

func TestRunAWSBillingDoesNotAutoSelectUnsafeSingleCandidate(t *testing.T) {
	tests := []struct {
		name     string
		evidence []workflow.PlanEvidence
		want     string
	}{
		{
			name: "unhealthy",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-eaeaeaeaeaeaeaea"},
				{Key: "candidate_1_health", Value: "UNHEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
			},
			want: "Blocker: AWS reports this export as unhealthy.",
		},
		{
			name: "incomplete",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-ebebebebebebebeb"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
			},
			want: "Blocker: pre-selection metadata is incomplete: missing file versioning.",
		},
		{
			name: "missing destination",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-edededededededed"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
			},
			want: "Blocker: pre-selection metadata is incomplete: missing destination region.",
		},
		{
			name: "unsupported",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-eccececcececcece"},
				{Key: "candidate_1_health", Value: "WARNING"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
			},
			want: "Blocker: pre-selection metadata has unsupported settings: health status WARNING.",
		},
		{
			name: "unsupported output type",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-efefefefefefefef"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_1_output_type", Value: "ATHENA"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
			},
			want: "Blocker: pre-selection metadata has unsupported settings: output type ATHENA.",
		},
		{
			name: "missing output type",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-eieieieieieieiei"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
			},
			want: "Blocker: pre-selection metadata is incomplete: missing output type.",
		},
		{
			name: "unsupported refresh cadence",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-eggeggeggeggegeg"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "ASYNCHRONOUS"},
			},
			want: "Blocker: pre-selection metadata has unsupported settings: refresh cadence ASYNCHRONOUS.",
		},
		{
			name: "missing refresh cadence",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-ejejejejejejejej"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
			},
			want: "Blocker: pre-selection metadata is incomplete: missing refresh cadence.",
		},
		{
			name: "unsupported include resources",
			evidence: []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-eheheheheheheheh"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_1_include_resources", Value: "UNKNOWN"},
			},
			want: "Blocker: pre-selection metadata has unsupported settings: include resources UNKNOWN.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := []workflow.ExecutionOptions{}
			registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
				calls = append(calls, options)
				if options.Selectors != nil && options.Selectors.AWS != nil && options.Selectors.AWS.CUR2ExportRef != "" {
					t.Fatalf("selected preflight should not run for unsafe single candidate: %#v", options.Selectors.AWS)
				}
				return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_selection_required", tt.evidence)
			}))
			guide := &fakeAWSBillingGuide{
				sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
				verified: map[string]billingguide.VerifiedIdentity{
					"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
				},
			}

			output, err := runGuidedWithConfig("1\n1\ny\ncancel\n", Config{Registry: registry, AWSBilling: guide})

			if !errors.Is(err, ErrInputCancelled) {
				t.Fatalf("RunWithConfig error = %v, want ErrInputCancelled", err)
			}
			if len(calls) != 1 {
				t.Fatalf("preflight calls = %d, want initial discovery only", len(calls))
			}
			for _, want := range []string{
				"One AWS CUR 2.0 export candidate needs review.",
				tt.want,
				"Full readiness checks run after selection.",
				"Select AWS CUR 2.0 action",
				"Review this CUR 2.0 export with full readiness preflight",
				"Prepare a new Matilda CUR 2.0 setup plan",
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("output = %q, want to contain %q", output, want)
				}
			}
			for _, forbidden := range []string{
				"Auto-selected CUR 2.0 export",
				"Running readiness preflight for selected CUR 2.0 export",
				"Review with:",
				"matilda-prep rapid-assessment billing aws preflight --profile default --region us-east-1 --export-ref",
			} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("output = %q, want no %q", output, forbidden)
				}
			}
			assertGuidedOutputSafe(t, output)
		})
	}
}

func TestRunAWSBillingSelectedExportThrottlingIsRetryable(t *testing.T) {
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		exportRef := ""
		if options.Selectors != nil && options.Selectors.AWS != nil {
			exportRef = options.Selectors.AWS.CUR2ExportRef
		}
		switch exportRef {
		case "":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_selection_required", []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-dadadadadadadada"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_1_output_type", Value: "CUSTOM"},
				{Key: "candidate_1_refresh_cadence", Value: "SYNCHRONOUS"},
				{Key: "candidate_1_include_resources", Value: "FALSE"},
				{Key: "candidate_1_pre_selection_metadata_status", Value: "supported"},
				{Key: "candidate_1_matilda_support", Value: "supported"},
				{Key: "candidate_1_primary_issue", Value: "none"},
				{Key: "candidate_1_required_next_action", Value: "run full readiness preflight after selection."},
			})
		case "cur2-dadadadadadadada":
			report := guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_data_exports_throttled", nil)
			report.Message = "AWS Data Exports throttled a read-only preflight check. Wait briefly, then rerun preflight."
			return report
		default:
			t.Fatalf("unexpected export ref %q", exportRef)
			return workflow.CapabilityReport{}
		}
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	for _, want := range []string{
		"Running readiness preflight for selected CUR 2.0 export cur2-dadadadadadadada",
		"Result: blocked",
		"Support code: aws_data_exports_throttled",
		"AWS Data Exports throttled a read-only preflight check.",
		"Wait briefly, then rerun preflight.",
		"--export-ref cur2-dadadadadadadada",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"remediation",
		"invalid",
		"not a CUR",
	} {
		if strings.Contains(strings.ToLower(output), forbidden) {
			t.Fatalf("output = %q, want no %q", output, forbidden)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestCur2CandidatesSanitizesMetadataAtDisplayBoundary(t *testing.T) {
	result := workflow.Result{Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
		Evidence: []workflow.PlanEvidence{
			{Key: "candidate_1_export_ref", Value: "cur2-aaaaaaaaaaaaaaaa"},
			{Key: "candidate_1_health", Value: "HEALTHY"},
			{Key: "candidate_1_output_format", Value: "PARQUET"},
			{Key: "candidate_1_compression", Value: "PARQUET"},
			{Key: "candidate_1_time_granularity", Value: "DAILY"},
			{Key: "candidate_1_overwrite", Value: "OVERWRITE_REPORT"},
			{Key: "candidate_1_destination_region", Value: "us-east-1"},
			{Key: "candidate_2_export_ref", Value: "cur2-bbbbbbbbbbbbbbbb"},
			{Key: "candidate_2_health", Value: "arn:aws:iam::123456789012:role/operator"},
			{Key: "candidate_2_output_format", Value: "private_key=/Users/example/key.pem"},
			{Key: "candidate_2_overwrite", Value: "token=plain-token"},
			{Key: "candidate_2_destination_region", Value: "token=plain-token"},
			{Key: "candidate_3_export_ref", Value: "cur2-cccccccccccccccc"},
			{Key: "candidate_3_health", Value: "123456789012"},
			{Key: "candidate_4_export_ref", Value: "cur2-dddddddddddddddd"},
			{Key: "candidate_4_output_format", Value: "AKIAIOSFODNN7EXAMPLE"},
			{Key: "candidate_5_export_ref", Value: "cur2-eeeeeeeeeeeeeeee"},
			{Key: "candidate_5_destination_region", Value: "ASIAIOSFODNN7EXAMPLE"},
		},
	}}}}

	candidates := cur2Candidates(result)

	if len(candidates) != 5 {
		t.Fatalf("candidates = %#v, want five safe refs with unsafe metadata dropped", candidates)
	}
	output := strings.Join([]string{
		candidateLabel(candidates[0]),
		candidateLabel(candidates[1]),
		candidateLabel(candidates[2]),
		candidateLabel(candidates[3]),
		candidateLabel(candidates[4]),
	}, "\n")
	for _, want := range []string{
		"cur2-aaaaaaaaaaaaaaaa, health HEALTHY, output PARQUET, compression PARQUET, granularity DAILY, versioning OVERWRITE_REPORT, region us-east-1",
		"cur2-bbbbbbbbbbbbbbbb",
		"cur2-cccccccccccccccc",
		"cur2-dddddddddddddddd",
		"cur2-eeeeeeeeeeeeeeee",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	for _, forbidden := range []string{
		"arn:aws",
		"123456789012",
		"private_key",
		"/Users/",
		"plain-token",
		"token=",
		"AKIAIOSFODNN7EXAMPLE",
		"ASIAIOSFODNN7EXAMPLE",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output leaked unsafe candidate metadata %q: %s", forbidden, output)
		}
	}
}

func TestRecommendationRequiresCanonicalProviderPreferredFacts(t *testing.T) {
	for _, candidate := range []cur2Candidate{
		{
			Ref:         "cur2-aaaaaaaaaaaaaaaa",
			Health:      "WARNING",
			Output:      "TEXT_OR_CSV",
			Compression: "GZIP",
			Granularity: "MONTHLY",
			Overwrite:   "CREATE_NEW_REPORT",
			Destination: "us-east-1",
		},
		{
			Ref:         "cur2-bbbbbbbbbbbbbbbb",
			Output:      "TEXT_OR_CSV",
			Compression: "GZIP",
			Granularity: "MONTHLY",
			Overwrite:   "CREATE_NEW_REPORT",
			Destination: "us-east-1",
		},
		{
			Ref:            "cur2-cccccccccccccccc",
			Health:         "HEALTHY",
			Output:         "TEXT_OR_CSV",
			Compression:    "GZIP",
			Granularity:    "MONTHLY",
			Overwrite:      "CREATE_NEW_REPORT",
			OutputType:     "CUSTOM",
			RefreshCadence: "SYNCHRONOUS",
			Destination:    "us-east-1",
		},
		{
			Ref:            "cur2-dddddddddddddddd",
			Health:         "HEALTHY",
			Output:         "TEXT_OR_CSV",
			Compression:    "GZIP",
			Granularity:    "MONTHLY",
			Overwrite:      "CREATE_NEW_REPORT",
			OutputType:     "CUSTOM",
			RefreshCadence: "SYNCHRONOUS",
			Destination:    "us-east-1",
			MetadataStatus: "supported",
			MatildaSupport: "supported",
		},
		{
			Ref:            "cur2-ecececececececec",
			Health:         "HEALTHY",
			Output:         "TEXT_OR_CSV",
			Compression:    "GZIP",
			Granularity:    "MONTHLY",
			Overwrite:      "CREATE_NEW_REPORT",
			OutputType:     "CUSTOM",
			RefreshCadence: "SYNCHRONOUS",
			Destination:    "us-east-1",
			MetadataStatus: "preferred",
		},
		{
			Ref:            "cur2-edededededededed",
			Health:         "HEALTHY",
			Output:         "TEXT_OR_CSV",
			Compression:    "GZIP",
			Granularity:    "MONTHLY",
			Overwrite:      "CREATE_NEW_REPORT",
			OutputType:     "CUSTOM",
			RefreshCadence: "SYNCHRONOUS",
			Destination:    "us-east-1",
			MetadataStatus: "preferred",
			MatildaSupport: "supported",
		},
	} {
		if isRecommendedCUR2Candidate(candidate) {
			t.Fatalf("isRecommendedCUR2Candidate(%#v) = true, want false without canonical preferred provider facts", candidate)
		}
	}

	canonicalPreferred := cur2Candidate{
		Ref:            "cur2-eeeeeeeeeeeeeeee",
		Health:         "HEALTHY",
		Output:         "TEXT_OR_CSV",
		Compression:    "GZIP",
		Granularity:    "MONTHLY",
		Overwrite:      "CREATE_NEW_REPORT",
		OutputType:     "CUSTOM",
		RefreshCadence: "SYNCHRONOUS",
		Destination:    "us-east-1",
		MetadataStatus: "preferred",
		MatildaSupport: "preferred",
	}
	if !isRecommendedCUR2Candidate(canonicalPreferred) {
		t.Fatalf("isRecommendedCUR2Candidate(%#v) = false, want true for canonical preferred provider facts", canonicalPreferred)
	}

	inconsistentPreferred := canonicalPreferred
	inconsistentPreferred.MetadataStatus = "incomplete"
	inconsistentPreferred.PrimaryIssue = "pre-selection metadata is incomplete: missing destination region."
	if isRecommendedCUR2Candidate(inconsistentPreferred) {
		t.Fatalf("isRecommendedCUR2Candidate(%#v) = true, want false without canonical preferred status", inconsistentPreferred)
	}
}

func TestWriteCUR2CandidateSelectionFactsShowsHourlyAsValidButNonPreferred(t *testing.T) {
	var output strings.Builder

	writeCUR2CandidateSelectionFacts(&output, cur2Candidate{Granularity: "HOURLY"}, "  ", false)

	text := output.String()
	for _, want := range []string{
		"hourly is valid AWS CUR 2.0",
		"monthly is preferred",
		"Full readiness checks run after selection.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("selection facts = %q, want %q", text, want)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteCUR2CandidateSelectionFactsExplainsBlockedMetadata(t *testing.T) {
	tests := []struct {
		name      string
		candidate cur2Candidate
		want      string
	}{
		{
			name: "unhealthy",
			candidate: cur2Candidate{
				Health:      "UNHEALTHY",
				Output:      "TEXT_OR_CSV",
				Compression: "GZIP",
				Granularity: "MONTHLY",
				Overwrite:   "CREATE_NEW_REPORT",
				Destination: "us-east-1",
			},
			want: "Blocker: AWS reports this export as unhealthy.",
		},
		{
			name: "unsupported health",
			candidate: cur2Candidate{
				Health:         "WARNING",
				Output:         "TEXT_OR_CSV",
				Compression:    "GZIP",
				Granularity:    "MONTHLY",
				Overwrite:      "CREATE_NEW_REPORT",
				OutputType:     "CUSTOM",
				RefreshCadence: "SYNCHRONOUS",
				Destination:    "us-east-1",
			},
			want: "Blocker: pre-selection metadata has unsupported settings: health status WARNING.",
		},
		{
			name: "missing health",
			candidate: cur2Candidate{
				Output:         "TEXT_OR_CSV",
				Compression:    "GZIP",
				Granularity:    "MONTHLY",
				Overwrite:      "CREATE_NEW_REPORT",
				OutputType:     "CUSTOM",
				RefreshCadence: "SYNCHRONOUS",
				Destination:    "us-east-1",
			},
			want: "Blocker: pre-selection metadata is incomplete: missing health status.",
		},
		{
			name: "missing setting",
			candidate: cur2Candidate{
				Health:         "HEALTHY",
				Output:         "TEXT_OR_CSV",
				Compression:    "GZIP",
				Granularity:    "MONTHLY",
				OutputType:     "CUSTOM",
				RefreshCadence: "SYNCHRONOUS",
				Destination:    "us-east-1",
			},
			want: "Blocker: pre-selection metadata is incomplete: missing file versioning.",
		},
		{
			name: "missing destination",
			candidate: cur2Candidate{
				Health:         "HEALTHY",
				Output:         "TEXT_OR_CSV",
				Compression:    "GZIP",
				Granularity:    "MONTHLY",
				Overwrite:      "CREATE_NEW_REPORT",
				OutputType:     "CUSTOM",
				RefreshCadence: "SYNCHRONOUS",
			},
			want: "Blocker: pre-selection metadata is incomplete: missing destination region.",
		},
		{
			name: "unsupported settings",
			candidate: cur2Candidate{
				Health:         "HEALTHY",
				Output:         "TEXT_OR_CSV",
				Compression:    "ZIP",
				Granularity:    "MONTHLY",
				Overwrite:      "CREATE_NEW_REPORT",
				OutputType:     "CUSTOM",
				RefreshCadence: "SYNCHRONOUS",
				Destination:    "us-east-1",
			},
			want: "Blocker: pre-selection metadata has unsupported settings: output/compression TEXT_OR_CSV/ZIP.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder

			writeCUR2CandidateSelectionFacts(&output, tt.candidate, "  ", false)

			text := output.String()
			if !strings.Contains(text, tt.want) {
				t.Fatalf("selection facts = %q, want %q", text, tt.want)
			}
			if !strings.Contains(text, "Full readiness checks run after selection.") {
				t.Fatalf("selection facts = %q, want deferred readiness note", text)
			}
			assertGuidedOutputSafe(t, text)
		})
	}
}

func TestRunSelectedCUR2PreflightFailsClosedForUnsafeSelectors(t *testing.T) {
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		t.Fatal("registry should not run for unsafe selectors")
		return workflow.CapabilityReport{}
	}))

	unsafeSource := runSelectedCUR2Preflight(context.Background(), registry, billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "../default",
		Region:  "us-east-1",
	}, "cur2-aaaaaaaaaaaaaaaa")
	if unsafeSource.Code != "aws_config_invalid_selector" {
		t.Fatalf("unsafe source code = %q, want aws_config_invalid_selector", unsafeSource.Code)
	}

	unsafeRef := runSelectedCUR2Preflight(context.Background(), registry, billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "default",
		Region:  "us-east-1",
	}, "bad-ref")
	if unsafeRef.Code != "aws_cur2_export_ref_invalid" {
		t.Fatalf("unsafe ref code = %q, want aws_cur2_export_ref_invalid", unsafeRef.Code)
	}
}

func TestRepairableCUR2CandidatesExcludesInaccessiblePolicyResults(t *testing.T) {
	classified := []classifiedCUR2Candidate{
		{
			Candidate: cur2Candidate{Ref: "cur2-faaaaaaaaaaaaaaa"},
			Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_s3_delivery_policy_missing"},
		},
		{
			Candidate: cur2Candidate{Ref: "cur2-fbbbbbbbbbbbbbbb"},
			Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_s3_bucket_policy_inaccessible"},
		},
		{
			Candidate: cur2Candidate{Ref: "cur2-fccccccccccccccc"},
			Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_cur2_output_settings_blocked"},
		},
		{
			Candidate: cur2Candidate{Ref: "cur2-fddddddddddddddd"},
			Result:    workflow.Result{Status: workflow.RunStatusFailed, Code: "aws_data_exports_transient"},
		},
	}

	repairable := repairableCUR2Candidates(classified)

	got := []string{}
	for _, item := range repairable {
		got = append(got, item.Candidate.Ref)
	}
	want := []string{"cur2-faaaaaaaaaaaaaaa"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("repairable refs = %#v, want %#v", got, want)
	}
}

func TestWriteNonReadyCUR2CandidateUsesSafeFactsAndPolicyAccessAction(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{
			Ref:    "cur2-fbbbbbbbbbbbbbbb",
			Output: "PARQUET",
		},
		Result: workflow.Result{
			Status: workflow.RunStatusBlocked,
			Code:   "aws_s3_bucket_policy_inaccessible",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
				Evidence: []workflow.PlanEvidence{
					{Key: "compression", Value: "PARQUET"},
					{Key: "time_granularity", Value: "DAILY"},
					{Key: "previous_billing_period", Value: "2026-06"},
					{Key: "missing_previous_month_component", Value: "manifest"},
					{Key: "policy_gap", Value: "arn:aws:s3:::private-bucket/policy"},
				},
			}}},
		},
	}
	var output strings.Builder

	writeNonReadyCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"cur2-fbbbbbbbbbbbbbbb",
		"Readiness: not ready",
		"Support code: aws_s3_bucket_policy_inaccessible",
		"Export: PARQUET / PARQUET, DAILY",
		"Previous month: 2026-06 missing manifest",
		"Next action: grant read access to inspect the S3 bucket policy, then rerun preflight.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("repairable candidate output = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "arn:aws") || strings.Contains(text, "private-bucket") {
		t.Fatalf("repairable candidate output leaked unsafe policy evidence: %s", text)
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteRepairableCUR2CandidateMapsPolicyGapsToPlainLanguage(t *testing.T) {
	tests := []struct {
		name       string
		gap        string
		wantPhrase string
	}{
		{
			name:       "service principal",
			gap:        "service_principal_missing",
			wantPhrase: "the AWS Data Exports service principal",
		},
		{
			name:       "put object action",
			gap:        "put_object_action_missing",
			wantPhrase: "permission for AWS Data Exports to write CUR objects",
		},
		{
			name:       "resource coverage",
			gap:        "put_object_resource_not_covered",
			wantPhrase: "the selected CUR 2.0 S3 destination prefix",
		},
		{
			name:       "source account",
			gap:        "source_account_condition_missing",
			wantPhrase: "the expected aws:SourceAccount condition",
		},
		{
			name:       "source arn",
			gap:        "source_arn_condition_missing",
			wantPhrase: "the expected aws:SourceArn condition",
		},
		{
			name:       "unparseable policy",
			gap:        "policy_unparseable",
			wantPhrase: "a parsable S3 bucket policy",
		},
		{
			name:       "matching statement",
			gap:        "matching_allow_statement_missing",
			wantPhrase: "a matching AWS Data Exports delivery statement",
		},
		{
			name:       "unknown gap",
			gap:        "unexpected_raw_gap",
			wantPhrase: "the required AWS Data Exports S3 delivery policy",
		},
		{
			name:       "unknown bucket-shaped gap",
			gap:        "matilda-cur2-billing",
			wantPhrase: "the required AWS Data Exports S3 delivery policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := classifiedCUR2Candidate{
				Candidate: cur2Candidate{Ref: "cur2-faaaaaaaaaaaaaaa", Output: "TEXT_OR_CSV", Compression: "GZIP"},
				Result: workflow.Result{
					Status: workflow.RunStatusBlocked,
					Code:   "aws_s3_delivery_policy_missing",
					Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
						Evidence: []workflow.PlanEvidence{
							{Key: "policy_gap", Value: tt.gap},
						},
					}}},
				},
			}
			var output strings.Builder

			writeRepairableCUR2Candidate(&output, item)

			text := output.String()
			for _, want := range []string{
				"Blocker: S3 delivery policy does not satisfy " + tt.wantPhrase + ".",
				"Next action: update the S3 delivery policy to include " + tt.wantPhrase + ", then rerun preflight.",
			} {
				if !strings.Contains(text, want) {
					t.Fatalf("repairable candidate output = %q, want %q", text, want)
				}
			}
			if strings.Contains(text, tt.gap) {
				t.Fatalf("repairable candidate output exposed raw policy gap %q: %s", tt.gap, text)
			}
			assertGuidedOutputSafe(t, text)
		})
	}
}

func TestWriteRepairableCUR2CandidateDropsUnsafeObjectKeyPolicyGap(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fggggggggggggggg", Output: "TEXT_OR_CSV", Compression: "GZIP"},
		Result: workflow.Result{
			Status: workflow.RunStatusBlocked,
			Code:   "aws_s3_delivery_policy_missing",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
				Evidence: []workflow.PlanEvidence{
					{Key: "policy_gap", Value: "matilda/cur2/private-prefix/BILLING_PERIOD=2026-06/part-000.gz"},
				},
			}}},
		},
	}
	var output strings.Builder

	writeRepairableCUR2Candidate(&output, item)

	text := output.String()
	if strings.Contains(text, "matilda/cur2/private-prefix") || strings.Contains(text, "BILLING_PERIOD=2026-06/part-000.gz") {
		t.Fatalf("repairable candidate output leaked raw object key policy evidence: %s", text)
	}
	if !strings.Contains(text, "Next action: update the S3 delivery policy using the direct preflight finding, then rerun preflight.") {
		t.Fatalf("repairable candidate output = %q, want safe direct-finding fallback", text)
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateShowsPolicyWarningWithoutBlocker(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-feeeeeeeeeeeeeee"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_s3_delivery_policy_missing",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"}),
				cur2PlanCheck(workflow.CheckWarn, "aws_s3_delivery_policy_missing",
					workflow.PlanEvidence{Key: "policy_gap", Value: "source_arn_condition_missing"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"cur2-feeeeeeeeeeeeeee",
		"Readiness: ready",
		"Support code: aws_s3_delivery_policy_missing",
		"S3 delivery policy: action needed",
		"Next action: continue with this CUR 2.0 export; review the S3 delivery policy before relying on future delivery or backfill.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("policy-warning output = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{
		"Blocker:",
		"source_arn_condition_missing",
		"Readiness: ready (aws_s3_delivery_policy_missing)",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("policy-warning output contains forbidden value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateShowsOverwriteAsReady(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fqqqqqqqqqqqqqqq"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_cur2_preflight_ready",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_output_format_ready",
					workflow.PlanEvidence{Key: "output_format", Value: "TEXT_OR_CSV"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_compression_ready",
					workflow.PlanEvidence{Key: "compression", Value: "GZIP"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_time_granularity_ready",
					workflow.PlanEvidence{Key: "time_granularity", Value: "MONTHLY"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_overwrite_supported",
					workflow.PlanEvidence{Key: "overwrite", Value: "OVERWRITE_REPORT"},
					workflow.PlanEvidence{Key: "matilda_output_preference", Value: "supported_not_preferred"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"cur2-fqqqqqqqqqqqqqqq",
		"Readiness: ready",
		"Support code: aws_cur2_preflight_ready",
		"Export: TEXT_OR_CSV / GZIP, MONTHLY, OVERWRITE_REPORT",
		"Next action: continue with this CUR 2.0 export.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("overwrite ready output = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{
		"Blocker:",
		"overwrite file versioning is not verified",
		"confirm Matilda support for OVERWRITE_REPORT",
		"use a CUR 2.0 export with CREATE_NEW_REPORT",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("overwrite ready output contains forbidden value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateShowsSafeHandoffLocations(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-frrrrrrrrrrrrrrr"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_cur2_preflight_ready",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_output_format_ready",
					workflow.PlanEvidence{Key: "output_format", Value: "TEXT_OR_CSV"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_compression_ready",
					workflow.PlanEvidence{Key: "compression", Value: "GZIP"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_time_granularity_ready",
					workflow.PlanEvidence{Key: "time_granularity", Value: "MONTHLY"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_overwrite_ready",
					workflow.PlanEvidence{Key: "overwrite", Value: "CREATE_NEW_REPORT"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_bucket", Value: "matilda-cur2-billing"},
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda/cur2"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us-east-1"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"},
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"Report location: s3://matilda-cur2-billing/matilda/cur2",
		"Billing data prefix: s3://matilda-cur2-billing/matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/",
		"Manifest prefix: s3://matilda-cur2-billing/matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/",
		"Destination region: us-east-1",
		"Matilda next step: use an AWS cloud account with Skip Configuration, then create Rapid Assessment - Billing Based and provide the CUR 2.0 billing data from this S3 location.",
		"Large data note: CSV and Parquet are supported; if direct upload size is too large, use Matilda's larger-file utility path after this tool completes.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("handoff output = %q, want %q", text, want)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateShowsSafePrefixesWhenBucketWithheld(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fsafetyprefix"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_cur2_preflight_ready",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda-rapid-cur2"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us-east-1"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-07"},
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda-rapid-cur2/matilda-rapid-cur2/data/BILLING_PERIOD=2026-07/"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda-rapid-cur2/matilda-rapid-cur2/metadata/BILLING_PERIOD=2026-07/"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"S3 bucket: not shown because the bucket value may contain a sensitive identifier.",
		"Configured report prefix: matilda-rapid-cur2",
		"Billing data prefix: matilda-rapid-cur2/matilda-rapid-cur2/data/BILLING_PERIOD=2026-07/",
		"Manifest prefix: matilda-rapid-cur2/matilda-rapid-cur2/metadata/BILLING_PERIOD=2026-07/",
		"Destination region: us-east-1",
		"Matilda next step: use an AWS cloud account with Skip Configuration, then create Rapid Assessment - Billing Based and provide the CUR 2.0 billing data from the selected export destination.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("handoff output = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "s3://") {
		t.Fatalf("handoff output built a full S3 URI without a safe bucket: %s", text)
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateSuppressesUnsafeHandoffLocationEvidence(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fsssssssssssssss"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_cur2_preflight_ready",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_bucket", Value: "token=plain-token"},
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda/cur2/private-prefix"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us-east-1"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda/cur2/private-prefix/BILLING_PERIOD=2026-06/part-000.gz"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda/cur2/private-prefix/metadata/BILLING_PERIOD=2026-06/Manifest.json"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, forbidden := range []string{
		"Report location:",
		"Billing data prefix:",
		"Manifest prefix:",
		"token=plain-token",
		"matilda/cur2/private-prefix",
		"part-000.gz",
		"Manifest.json",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("handoff output leaked unsafe value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateShowsExpectedLocationForManualBackfill(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fttttttttttttttt"},
		Result: workflow.Result{
			Status: workflow.RunStatusManualSteps,
			Code:   "aws_backfill_manual_step_required",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_bucket", Value: "matilda-cur2-billing"},
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda/cur2"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us-east-1"}),
				cur2PlanCheck(workflow.CheckWarn, "aws_backfill_manual_step_required",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"},
					workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "data_partition"},
					workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "manifest"},
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"Readiness: manual step required",
		"Previous month: 2026-06 missing data partition, manifest",
		"Report location: s3://matilda-cur2-billing/matilda/cur2",
		"Expected billing data prefix: s3://matilda-cur2-billing/matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/",
		"Expected manifest prefix: s3://matilda-cur2-billing/matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/",
		"Matilda next step: complete previous-month billing data backfill first; after preflight is ready, use an AWS cloud account with Skip Configuration and provide the CUR 2.0 billing data from this S3 location.",
		"Next action: request or complete previous-month billing data backfill, then rerun preflight.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("manual handoff output = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "then create Rapid Assessment - Billing Based and provide the CUR 2.0 billing data from this S3 location.") {
		t.Fatalf("manual handoff output implied data was ready: %s", text)
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateShowsSafePrefixesForManualBackfillWhenBucketWithheld(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fmanualprefix"},
		Result: workflow.Result{
			Status: workflow.RunStatusManualSteps,
			Code:   "aws_backfill_manual_step_required",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda-rapid-cur2"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us-east-1"}),
				cur2PlanCheck(workflow.CheckWarn, "aws_backfill_manual_step_required",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-07"},
					workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "manifest"},
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda-rapid-cur2/matilda-rapid-cur2/data/BILLING_PERIOD=2026-07/"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda-rapid-cur2/matilda-rapid-cur2/metadata/BILLING_PERIOD=2026-07/"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"Readiness: manual step required",
		"S3 bucket: not shown because the bucket value may contain a sensitive identifier.",
		"Configured report prefix: matilda-rapid-cur2",
		"Previous month: 2026-07 missing manifest",
		"Expected billing data prefix: matilda-rapid-cur2/matilda-rapid-cur2/data/BILLING_PERIOD=2026-07/",
		"Expected manifest prefix: matilda-rapid-cur2/matilda-rapid-cur2/metadata/BILLING_PERIOD=2026-07/",
		"Matilda next step: complete previous-month billing data backfill first; after preflight is ready, use an AWS cloud account with Skip Configuration and provide the CUR 2.0 billing data from the selected export destination.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("manual prefix-only handoff output = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "s3://") {
		t.Fatalf("manual prefix-only handoff output built a full S3 URI without a safe bucket: %s", text)
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateSuppressesUnsafeRegionEvidence(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-funsafe-region"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_cur2_preflight_ready",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda-rapid-cur2"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us_east_1"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-07"},
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda-rapid-cur2/matilda-rapid-cur2/data/BILLING_PERIOD=2026-07/"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda-rapid-cur2/matilda-rapid-cur2/metadata/BILLING_PERIOD=2026-07/"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, forbidden := range []string{
		"us_east_1",
		"Configured report prefix:",
		"Billing data prefix:",
		"Manifest prefix:",
		"Matilda next step:",
		"Large data note:",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("unsafe region output leaked handoff value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateIncludesPolicyActionWhenTopLevelCodeIsAnotherWarning(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fdddddddddddddd"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_cur2_delivery_not_started",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheckWithMessage(workflow.CheckWarn, "aws_cur2_delivery_not_started", "Latest AWS Data Exports delivery is still in progress."),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"}),
				cur2PlanCheck(workflow.CheckWarn, "aws_s3_delivery_policy_missing",
					workflow.PlanEvidence{Key: "policy_gap", Value: "source_account_condition_missing"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"Support code: aws_cur2_delivery_not_started",
		"S3 delivery policy: action needed",
		"Next action: continue with this CUR 2.0 export; review the S3 delivery policy before relying on future delivery or backfill.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("multi-warning selectable output = %q, want %q", text, want)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteRepairableCUR2CandidateShowsFallbacksWhenFactsAreMissing(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{
			Ref:    "cur2-ffffffffffffffff",
			Output: "TEXT_OR_CSV",
		},
		Result: workflow.Result{
			Status: workflow.RunStatusBlocked,
			Code:   "aws_repairable_unknown",
		},
	}
	var output strings.Builder

	writeRepairableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"cur2-ffffffffffffffff",
		"Readiness: repair required",
		"Support code: aws_repairable_unknown",
		"Export: TEXT_OR_CSV / unverified",
		"Next action: review the direct preflight result and rerun after remediation.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("repairable fallback output = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "Previous month:") || strings.Contains(text, "Policy:") {
		t.Fatalf("repairable fallback output should omit unknown previous-month and policy facts: %s", text)
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteRepairableCUR2CandidateDoesNotPrintHandoffInstructions(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-frepairblocked"},
		Result: workflow.Result{
			Status: workflow.RunStatusBlocked,
			Code:   "aws_s3_delivery_policy_missing",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_bucket", Value: "matilda-cur2-billing"},
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda/cur2"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us-east-1"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"},
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/"}),
				cur2PlanCheck(workflow.CheckWarn, "aws_s3_delivery_policy_missing",
					workflow.PlanEvidence{Key: "policy_gap", Value: "source_account_condition_missing"}),
			}},
		},
	}
	var output strings.Builder

	writeRepairableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"Readiness: repair required",
		"S3 delivery policy: action needed",
		"Blocker: S3 delivery policy does not satisfy the expected aws:SourceAccount condition.",
		"Next action: update the S3 delivery policy to include the expected aws:SourceAccount condition, then rerun preflight.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("repairable blocked output = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{
		"Report location:",
		"Billing data prefix:",
		"Manifest prefix:",
		"Matilda next step:",
		"Large data note:",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("repairable blocked output contains forbidden value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteRepairableCUR2CandidatesOmitsOtherSectionWhenAllAreRepairable(t *testing.T) {
	classified := []classifiedCUR2Candidate{
		{
			Candidate: cur2Candidate{Ref: "cur2-faaaaaaaaaaaaaaa", Output: "TEXT_OR_CSV", Compression: "GZIP"},
			Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_s3_delivery_policy_missing"},
		},
		{
			Candidate: cur2Candidate{Ref: "cur2-fbbbbbbbbbbbbbbb", Output: "PARQUET", Compression: "PARQUET"},
			Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_s3_delivery_policy_missing"},
		},
	}
	var output strings.Builder

	writeRepairableCUR2Candidates(&output, classified, classified)

	text := output.String()
	for _, want := range []string{
		"No AWS CUR 2.0 export is ready yet.",
		"Repairable CUR 2.0 export candidates",
		"cur2-faaaaaaaaaaaaaaa",
		"cur2-fbbbbbbbbbbbbbbb",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("repairable list output = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "Other CUR 2.0 candidates") {
		t.Fatalf("repairable list output should not show other section when all candidates are repairable: %s", text)
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteRepairableCUR2CandidatesShowsInaccessiblePolicyAsOtherNonReady(t *testing.T) {
	classified := []classifiedCUR2Candidate{
		{
			Candidate: cur2Candidate{Ref: "cur2-faaaaaaaaaaaaaaa", Output: "TEXT_OR_CSV", Compression: "GZIP"},
			Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_s3_delivery_policy_missing"},
		},
		{
			Candidate: cur2Candidate{Ref: "cur2-fbbbbbbbbbbbbbbb", Output: "PARQUET", Compression: "PARQUET"},
			Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_s3_bucket_policy_inaccessible"},
		},
	}
	var output strings.Builder

	writeRepairableCUR2Candidates(&output, repairableCUR2Candidates(classified), classified)

	text := output.String()
	for _, want := range []string{
		"Repairable CUR 2.0 export candidates",
		"Other CUR 2.0 candidates",
		"cur2-fbbbbbbbbbbbbbbb",
		"Readiness: not ready",
		"Support code: aws_s3_bucket_policy_inaccessible",
		"Next action: grant read access to inspect the S3 bucket policy, then rerun preflight.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("repairable list output = %q, want %q", text, want)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateMapsPreviousMonthComponentsToPlainLanguage(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fhhhhhhhhhhhhhhh"},
		Result: workflow.Result{
			Status: workflow.RunStatusManualSteps,
			Code:   "aws_backfill_manual_step_required",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckWarn, "aws_backfill_manual_step_required",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"},
					workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "data_partition"},
					workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "manifest"},
					workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "unexpected_component"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"Previous month: 2026-06 missing data partition, manifest, previous-month component",
		"Next action: request or complete previous-month billing data backfill, then rerun preflight.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("previous-month component output = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{
		"data_partition",
		"unexpected_component",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("previous-month component output exposed raw evidence value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateUsesPlanChecksForBackfillWithoutPolicyRepair(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fiiiiiiiiiiiiiii"},
		Result: workflow.Result{
			Status: workflow.RunStatusManualSteps,
			Code:   "aws_backfill_manual_step_required",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_output_format_ready",
					workflow.PlanEvidence{Key: "output_format", Value: "TEXT_OR_CSV"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_compression_ready",
					workflow.PlanEvidence{Key: "compression", Value: "GZIP"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_time_granularity_ready",
					workflow.PlanEvidence{Key: "time_granularity", Value: "MONTHLY"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_overwrite_ready",
					workflow.PlanEvidence{Key: "overwrite", Value: "CREATE_NEW_REPORT"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_delivery_ready"),
				cur2PlanCheck(workflow.CheckPass, "aws_s3_delivery_policy_ready"),
				cur2PlanCheck(workflow.CheckWarn, "aws_backfill_manual_step_required",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"},
					workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "data_partition"},
					workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "manifest"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"cur2-fiiiiiiiiiiiiiii",
		"Readiness: manual step required",
		"Support code: aws_backfill_manual_step_required",
		"Export: TEXT_OR_CSV / GZIP, MONTHLY, CREATE_NEW_REPORT",
		"AWS delivery: ready",
		"S3 delivery policy: ready",
		"Previous month: 2026-06 missing data partition, manifest",
		"Next action: request or complete previous-month billing data backfill, then rerun preflight.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("selectable backfill output = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{
		"repair S3 delivery policy",
		"Policy: source_account_condition_missing",
		"Fix delivery permission",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("selectable backfill output contains forbidden value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteBlockedCUR2CandidateDoesNotTreatOverwriteAsBlocker(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fjjjjjjjjjjjjjjj"},
		Result: workflow.Result{
			Status: workflow.RunStatusBlocked,
			Code:   "aws_cur2_output_settings_blocked",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckWarn, "aws_cur2_time_granularity_not_preferred",
					workflow.PlanEvidence{Key: "time_granularity", Value: "DAILY"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_output_format_supported",
					workflow.PlanEvidence{Key: "output_format", Value: "PARQUET"},
					workflow.PlanEvidence{Key: "matilda_format_support", Value: "supported"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_compression_supported",
					workflow.PlanEvidence{Key: "compression", Value: "PARQUET"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_overwrite_supported",
					workflow.PlanEvidence{Key: "overwrite", Value: "OVERWRITE_REPORT"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_s3_destination_ready",
					workflow.PlanEvidence{Key: "s3_bucket", Value: "matilda-cur2-billing"},
					workflow.PlanEvidence{Key: "s3_prefix", Value: "matilda/cur2"},
					workflow.PlanEvidence{Key: "s3_region", Value: "us-east-1"}),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"},
					workflow.PlanEvidence{Key: "cur2_data_prefix", Value: "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/"},
					workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/"}),
				cur2PlanCheck(workflow.CheckFail, "aws_cur2_output_settings_blocked",
					workflow.PlanEvidence{Key: "output_format", Value: "JSON"}),
			}},
		},
	}
	var output strings.Builder

	writeNonReadyCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"cur2-fjjjjjjjjjjjjjjj",
		"Readiness: not ready",
		"Support code: aws_cur2_output_settings_blocked",
		"Export: JSON / PARQUET, DAILY, OVERWRITE_REPORT",
		"Next action: review the CUR 2.0 output settings and rerun after they match a Matilda-supported AWS-standard shape.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("overwrite output = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{
		"PARQUET output is not supported",
		"daily granularity is not supported",
		"use a CUR 2.0 export with CREATE_NEW_REPORT",
		"overwrite file versioning is not verified",
		"confirm Matilda support for OVERWRITE_REPORT",
		"Blocker: overwrite",
		"Report location:",
		"Billing data prefix:",
		"Manifest prefix:",
		"Matilda next step:",
		"Large data note:",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("overwrite output contains forbidden value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateShowsEvidenceDerivedDeliveryAndPolicyStatuses(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fkkkkkkkkkkkkkkk"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_cur2_delivery_not_started",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_output_format_ready",
					workflow.PlanEvidence{Key: "output_format", Value: "TEXT_OR_CSV"}),
				cur2PlanCheckWithMessage(workflow.CheckWarn, "aws_cur2_delivery_not_started", "Latest AWS Data Exports delivery is still in progress."),
				cur2PlanCheck(workflow.CheckPass, "aws_s3_delivery_policy_ready"),
				cur2PlanCheck(workflow.CheckPass, "aws_cur2_previous_month_ready",
					workflow.PlanEvidence{Key: "previous_billing_period", Value: "2026-06"}),
			}},
		},
	}
	var output strings.Builder

	writeSelectableCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"Readiness: ready",
		"Support code: aws_cur2_delivery_not_started",
		"Export: TEXT_OR_CSV / unverified",
		"AWS delivery: in progress",
		"S3 delivery policy: ready",
		"Next action: continue with this CUR 2.0 export.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("evidence status output = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "Previous month:") {
		t.Fatalf("evidence status output should not show missing previous-month text when previous month is ready: %s", text)
	}
	if strings.Contains(text, "Readiness: ready (aws_cur2_delivery_not_started)") {
		t.Fatalf("evidence status output combines readiness and support code: %s", text)
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteSelectableCUR2CandidateDistinguishesDeliveryWarningMessages(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		want      string
		forbidden []string
	}{
		{
			name:      "inconclusive",
			message:   "AWS Data Exports delivery status is not conclusive yet.",
			want:      "AWS delivery: not conclusive",
			forbidden: []string{"AWS delivery: in progress"},
		},
		{
			name:      "in progress",
			message:   "Latest AWS Data Exports delivery is still in progress.",
			want:      "AWS delivery: in progress",
			forbidden: []string{"AWS delivery: not conclusive", "AWS delivery: not started yet"},
		},
		{
			name:      "not started yet",
			message:   "No AWS Data Exports delivery execution has started yet.",
			want:      "AWS delivery: not started yet",
			forbidden: []string{"AWS delivery: in progress"},
		},
		{
			name:      "unknown warning falls back",
			message:   "AWS CUR 2.0 test check.",
			want:      "AWS delivery: not conclusive",
			forbidden: []string{"AWS delivery: in progress"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := classifiedCUR2Candidate{
				Candidate: cur2Candidate{Ref: "cur2-fkkkkkkkkkkkkkkk"},
				Result: workflow.Result{
					Status: workflow.StatusReady,
					Code:   "aws_cur2_delivery_not_started",
					Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
						cur2PlanCheckWithMessage(workflow.CheckWarn, "aws_cur2_delivery_not_started", tt.message),
						cur2PlanCheck(workflow.CheckPass, "aws_s3_delivery_policy_ready"),
					}},
				},
			}
			var output strings.Builder

			writeSelectableCUR2Candidate(&output, item)

			text := output.String()
			if !strings.Contains(text, tt.want) {
				t.Fatalf("delivery warning output = %q, want %q", text, tt.want)
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("delivery warning output contains forbidden value %q: %s", forbidden, text)
				}
			}
			assertGuidedOutputSafe(t, text)
		})
	}
}

func TestWriteNonReadyCUR2CandidateShowsDeliveryNotStartedStatus(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-flllllllllllllll"},
		Result: workflow.Result{
			Status: workflow.RunStatusBlocked,
			Code:   "aws_cur2_delivery_not_started",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckFail, "aws_cur2_delivery_not_started"),
			}},
		},
	}
	var output strings.Builder

	writeNonReadyCUR2Candidate(&output, item)

	text := output.String()
	for _, want := range []string{
		"Readiness: not ready",
		"Support code: aws_cur2_delivery_not_started",
		"AWS delivery: not started",
		"Next action: review the direct preflight result and rerun after remediation.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("delivery-not-started output = %q, want %q", text, want)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestWriteAWSBillingSummaryFactsSkipsResultsWithoutCUR2PlanFacts(t *testing.T) {
	tests := []struct {
		name   string
		result workflow.Result
	}{
		{name: "nil plan"},
		{
			name: "unrelated plan",
			result: workflow.Result{Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
				ID:     "aws_config_missing_credentials",
				Status: workflow.CheckFail,
				Evidence: []workflow.PlanEvidence{
					{Key: "code", Value: "aws_config_missing_credentials"},
				},
			}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder

			writeAWSBillingSummaryFacts(&output, tt.result)

			if output.String() != "" {
				t.Fatalf("summary facts output = %q, want empty", output.String())
			}
		})
	}
}

func TestSummaryCUR2ReadinessAndNextActionUsesRepairablePolicyResult(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fmmmmmmmmmmmmmmm"},
		Result: workflow.Result{
			Status: workflow.RunStatusBlocked,
			Code:   "aws_s3_delivery_policy_missing",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
				ID:     "aws_s3_delivery_policy_missing",
				Status: workflow.CheckFail,
				Evidence: []workflow.PlanEvidence{
					{Key: "policy_gap", Value: "service_principal_missing"},
				},
			}}},
		},
	}

	readiness, nextAction := summaryCUR2ReadinessAndNextAction(item)

	if readiness != "repair required" {
		t.Fatalf("readiness = %q, want repair required", readiness)
	}
	if !strings.Contains(nextAction, "the AWS Data Exports service principal") {
		t.Fatalf("next action = %q, want service-principal remediation", nextAction)
	}
}

func TestSafeCandidateLabelValueRejectsSensitiveIdentifierShapesWithoutOverblocking(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "safe health value",
			value: "HEALTHY",
			want:  "HEALTHY",
		},
		{
			name:  "safe output format",
			value: "PARQUET",
			want:  "PARQUET",
		},
		{
			name:  "safe region",
			value: "us-east-1",
			want:  "us-east-1",
		},
		{
			name:  "blank metadata",
			value: "   ",
		},
		{
			name:  "raw account id",
			value: "123456789012",
		},
		{
			name:  "embedded account id",
			value: "cur2123456789012billing",
		},
		{
			name:  "access key id shape",
			value: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "embedded access key id shape",
			value: "prefix-AKIAIOSFODNN7EXAMPLE-safe",
		},
		{
			name:  "temporary access key id shape",
			value: "ASIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "embedded temporary access key id shape",
			value: "prefix-ASIAIOSFODNN7EXAMPLE-safe",
		},
		{
			name:  "lowercase access key id shape",
			value: "akiaiosfodnn7example",
		},
		{
			name:  "non account id numeric-looking metadata",
			value: "12345678901A",
			want:  "12345678901A",
		},
		{
			name:  "non access key id metadata",
			value: "AKIAIOSFODNN7EXAMPL_",
			want:  "AKIAIOSFODNN7EXAMPL_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeCandidateLabelValue(tt.value); got != tt.want {
				t.Fatalf("safeCandidateLabelValue(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func cur2PlanCheck(status workflow.CheckStatus, code string, evidence ...workflow.PlanEvidence) workflow.PlanCheck {
	return workflow.PlanCheck{
		ID:            code,
		Status:        status,
		Title:         "AWS CUR 2.0 test check",
		Message:       "AWS CUR 2.0 test check.",
		Evidence:      evidence,
		SourceHandles: guidedTestSourceHandles(),
	}
}

func cur2PlanCheckWithMessage(status workflow.CheckStatus, code string, message string, evidence ...workflow.PlanEvidence) workflow.PlanCheck {
	check := cur2PlanCheck(status, code, evidence...)
	check.Message = message
	return check
}

func TestPlanCheckCodePrefersSemanticIDAndFallsBackToLegacyEvidence(t *testing.T) {
	withID := workflow.PlanCheck{
		ID: "aws_s3_delivery_policy_ready",
		Evidence: []workflow.PlanEvidence{
			{Key: "code", Value: "aws_s3_delivery_policy_missing"},
		},
	}
	if got := planCheckCode(withID); got != "aws_s3_delivery_policy_ready" {
		t.Fatalf("planCheckCode with ID = %q, want aws_s3_delivery_policy_ready", got)
	}

	legacy := workflow.PlanCheck{Evidence: []workflow.PlanEvidence{
		{Key: "code", Value: "aws_cur2_delivery_not_started"},
	}}
	if got := planCheckCode(legacy); got != "aws_cur2_delivery_not_started" {
		t.Fatalf("planCheckCode legacy fallback = %q, want aws_cur2_delivery_not_started", got)
	}

	if got := planCheckCode(workflow.PlanCheck{}); got != "" {
		t.Fatalf("planCheckCode empty check = %q, want empty string", got)
	}
}

func TestRunAWSBillingRunsSelectedPreflightAndShowsRepairableResult(t *testing.T) {
	calls := []workflow.ExecutionOptions{}
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls = append(calls, options)
		exportRef := ""
		if options.Selectors != nil && options.Selectors.AWS != nil {
			exportRef = options.Selectors.AWS.CUR2ExportRef
		}
		switch exportRef {
		case "":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_ambiguous", []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-cccccccccccccccc"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_compression", Value: "GZIP"},
				{Key: "candidate_1_time_granularity", Value: "MONTHLY"},
				{Key: "candidate_1_overwrite", Value: "CREATE_NEW_REPORT"},
				{Key: "candidate_2_export_ref", Value: "cur2-dddddddddddddddd"},
				{Key: "candidate_2_output_format", Value: "PARQUET"},
				{Key: "candidate_2_compression", Value: "PARQUET"},
				{Key: "candidate_2_time_granularity", Value: "DAILY"},
				{Key: "candidate_2_overwrite", Value: "OVERWRITE_REPORT"},
			})
		case "cur2-cccccccccccccccc":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_s3_delivery_policy_missing", []workflow.PlanEvidence{
				{Key: "output_format", Value: "TEXT_OR_CSV"},
				{Key: "compression", Value: "GZIP"},
				{Key: "time_granularity", Value: "MONTHLY"},
				{Key: "previous_billing_period", Value: "2026-06"},
				{Key: "missing_previous_month_component", Value: "data_partition"},
				{Key: "missing_previous_month_component", Value: "manifest"},
				{Key: "policy_gap", Value: "source_account_condition_missing"},
			})
		case "cur2-dddddddddddddddd":
			return guidedCapabilityReport(got, workflow.RunStatusFailed, "aws_data_exports_transient", nil)
		default:
			t.Fatalf("unexpected export ref %q", exportRef)
			return workflow.CapabilityReport{}
		}
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n1\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("preflight calls = %d, want initial discovery plus selected export preflight", len(calls))
	}
	if calls[1].Selectors.AWS.CUR2ExportRef != "cur2-cccccccccccccccc" {
		t.Fatalf("selected preflight ref = %q, want chosen candidate ref", calls[1].Selectors.AWS.CUR2ExportRef)
	}
	for _, want := range []string{
		"Select AWS CUR 2.0 export",
		"Full readiness checks run after selection.",
		"Running readiness preflight for selected CUR 2.0 export cur2-cccccccccccccccc",
		"cur2-cccccccccccccccc",
		"Readiness: repair required",
		"Support code: aws_s3_delivery_policy_missing",
		"Export: TEXT_OR_CSV / GZIP, MONTHLY",
		"S3 delivery policy: action needed",
		"Previous month: 2026-06 missing data partition, manifest",
		"Blocker: S3 delivery policy does not satisfy the expected aws:SourceAccount condition.",
		"Next action: update the S3 delivery policy to include the expected aws:SourceAccount condition, then rerun preflight.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	if strings.Contains(output, "source_account_condition_missing") {
		t.Fatalf("output exposed raw policy gap enum: %s", output)
	}
	if strings.Contains(output, "data_partition") {
		t.Fatalf("output exposed raw previous-month component enum: %s", output)
	}
	if strings.Contains(output, "Other CUR 2.0 candidates") {
		t.Fatalf("output used old pre-selection repairable/other grouping: %s", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestWriteBlockedClassificationsUsesSafeFactsAndNextAction(t *testing.T) {
	classified := []classifiedCUR2Candidate{
		{
			Candidate: cur2Candidate{Ref: "cur2-fnnnnnnnnnnnnnnn", Output: "TEXT_OR_CSV", Compression: "GZIP"},
			Result:    workflow.Result{Status: workflow.StatusReady, Code: "aws_cur2_preflight_ready"},
		},
		{
			Candidate: cur2Candidate{Ref: "cur2-fjjjjjjjjjjjjjjj", Output: "PARQUET", Compression: "PARQUET"},
			Result: workflow.Result{
				Status: workflow.RunStatusBlocked,
				Code:   "aws_cur2_output_settings_blocked",
				Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
					Evidence: []workflow.PlanEvidence{
						{Key: "time_granularity", Value: "DAILY"},
						{Key: "overwrite", Value: "OVERWRITE_REPORT"},
						{Key: "output_format", Value: "JSON"},
						{Key: "policy_gap", Value: "arn:aws:s3:::private-bucket/policy"},
					},
				}}},
			},
		},
	}
	var output strings.Builder

	writeBlockedClassifications(&output, classified)

	text := output.String()
	for _, want := range []string{
		"cur2-fjjjjjjjjjjjjjjj",
		"Readiness: not ready",
		"Support code: aws_cur2_output_settings_blocked",
		"Export: JSON / PARQUET, DAILY, OVERWRITE_REPORT",
		"Next action: review the CUR 2.0 output settings and rerun after they match a Matilda-supported AWS-standard shape.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("blocked classification output = %q, want %q", text, want)
		}
	}
	for _, forbidden := range []string{
		"cur2-fnnnnnnnnnnnnnnn",
		"not ready: aws_cur2_output_settings_blocked",
		"arn:aws",
		"private-bucket",
		"overwrite file versioning is not verified",
		"confirm Matilda support for OVERWRITE_REPORT",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("blocked classification output contains forbidden value %q: %s", forbidden, text)
		}
	}
	assertGuidedOutputSafe(t, text)
}

func TestNonReadyCUR2NextActionExplainsOutputSettingsWithoutOverwriteFact(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fooooooooooooooo", Output: "JSON"},
		Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_cur2_output_settings_blocked"},
	}

	got := nonReadyCUR2NextAction(item)

	const want = "review the CUR 2.0 output settings and rerun after they match a Matilda-supported AWS-standard shape."
	if got != want {
		t.Fatalf("nonReadyCUR2NextAction() = %q, want %q", got, want)
	}
	assertGuidedOutputSafe(t, got)
}

func TestNonReadyCUR2NextActionExplainsDataExportsThrottlingAsRetryable(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fqqqqqqqqqqqqqqq"},
		Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_data_exports_throttled"},
	}

	got := nonReadyCUR2NextAction(item)

	const want = "AWS throttled the Data Exports read-only check. Wait briefly, then rerun preflight."
	if got != want {
		t.Fatalf("nonReadyCUR2NextAction() = %q, want %q", got, want)
	}
	for _, forbidden := range []string{
		"remediation",
		"invalid",
		"not a CUR",
	} {
		if strings.Contains(strings.ToLower(got), forbidden) {
			t.Fatalf("nonReadyCUR2NextAction() = %q, want no %q", got, forbidden)
		}
	}
	assertGuidedOutputSafe(t, got)
}

func TestNonReadyCUR2NextActionUsesGenericFallbackForUnknownCode(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fppppppppppppppp"},
		Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_cur2_unknown_blocker"},
	}

	got := nonReadyCUR2NextAction(item)

	const want = "review the direct preflight result and rerun after remediation."
	if got != want {
		t.Fatalf("nonReadyCUR2NextAction() = %q, want %q", got, want)
	}
	assertGuidedOutputSafe(t, got)
}

func TestSelectableCUR2ReadinessAndNextActionUseSafeFallbacks(t *testing.T) {
	manual := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-gaaaaaaaaaaaaaaa"},
		Result:    workflow.Result{Status: workflow.RunStatusManualSteps, Code: "aws_cur2_manual_generic"},
	}
	unknown := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-gbbbbbbbbbbbbbbb"},
		Result:    workflow.Result{Status: workflow.RunStatusBlocked, Code: "aws_cur2_unknown_status"},
	}

	if got, want := selectableCUR2Readiness(manual), "manual step required"; got != want {
		t.Fatalf("selectableCUR2Readiness(manual) = %q, want %q", got, want)
	}
	if got, want := selectableCUR2NextAction(manual), "complete the manual step shown by preflight, then rerun preflight."; got != want {
		t.Fatalf("selectableCUR2NextAction(manual) = %q, want %q", got, want)
	}
	if got, want := selectableCUR2Readiness(unknown), "selected"; got != want {
		t.Fatalf("selectableCUR2Readiness(unknown) = %q, want %q", got, want)
	}
	if got, want := selectableCUR2NextAction(unknown), "review the direct preflight result and rerun after remediation."; got != want {
		t.Fatalf("selectableCUR2NextAction(unknown) = %q, want %q", got, want)
	}
	assertGuidedOutputSafe(t, selectableCUR2NextAction(manual))
	assertGuidedOutputSafe(t, selectableCUR2NextAction(unknown))
}

func TestRunAWSBillingAmbiguousWithoutSafeCandidateRefsShowsOriginalSummary(t *testing.T) {
	calls := 0
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		calls++
		return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_ambiguous", []workflow.PlanEvidence{
			{Key: "candidate_1_export_ref", Value: "cur2-1234"},
			{Key: "candidate_2_export_ref", Value: "cur2-ABCDEF1234567890"},
		})
	}))
	guide := &fakeAWSBillingGuide{
		sources: []billingguide.CredentialSource{{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}},
		verified: map[string]billingguide.VerifiedIdentity{
			"profile:default": {Source: billingguide.CredentialSource{Kind: billingguide.CredentialSourceProfile, Profile: "default", Region: "us-east-1"}, AccountLabel: "account-ending-9012", CallerRef: "sha256:abcdef123456", Region: "us-east-1"},
		},
	}

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("preflight calls = %d, want no candidate classification for unsafe refs", calls)
	}
	if strings.Contains(output, "Classifying") || strings.Contains(output, "cur2-1234") || strings.Contains(output, "cur2-ABCDEF1234567890") {
		t.Fatalf("output should not classify or display unsafe refs: %s", output)
	}
	for _, want := range []string{
		"Result: blocked",
		"Support code: aws_cur2_export_ambiguous",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want ambiguous summary value %q", output, want)
		}
	}
	if strings.Contains(output, "Result: blocked (aws_cur2_export_ambiguous)") {
		t.Fatalf("output uses old status/code summary format: %s", output)
	}
	assertGuidedOutputSafe(t, output)
}

func TestCur2CandidatesKeepsOnlySafeGeneratedRefsInStableOrder(t *testing.T) {
	result := workflow.Result{Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{{
		Evidence: []workflow.PlanEvidence{
			{Key: "candidate_3_export_ref", Value: "cur2-cccccccccccccccc"},
			{Key: "candidate_3_health", Value: "HEALTHY"},
			{Key: "candidate_2_export_ref", Value: "cur2-1234"},
			{Key: "candidate_1_export_ref", Value: "cur2-aaaaaaaaaaaaaaaa"},
			{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
			{Key: "candidate_zero_export_ref", Value: "cur2-bbbbbbbbbbbbbbbb"},
			{Key: "candidate_0_export_ref", Value: "cur2-bbbbbbbbbbbbbbbb"},
			{Key: "not_candidate_1_export_ref", Value: "cur2-dddddddddddddddd"},
		},
	}}}}

	candidates := cur2Candidates(result)

	got := []string{}
	for _, candidate := range candidates {
		got = append(got, candidate.Ref+"|"+candidate.Health+"|"+candidate.Output)
	}
	want := []string{
		"cur2-aaaaaaaaaaaaaaaa||TEXT_OR_CSV",
		"cur2-cccccccccccccccc|HEALTHY|",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestCur2CandidatesReturnsNilWhenPlanIsMissing(t *testing.T) {
	if candidates := cur2Candidates(workflow.Result{}); candidates != nil {
		t.Fatalf("candidates = %#v, want nil when result has no plan", candidates)
	}
}
