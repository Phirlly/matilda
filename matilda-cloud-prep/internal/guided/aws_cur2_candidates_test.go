package guided

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestRunAWSBillingClassifiesAmbiguousExportsBeforePrompting(t *testing.T) {
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
				{Key: "candidate_1_export_ref", Value: "cur2-1111111111111111"},
				{Key: "candidate_1_health", Value: "HEALTHY"},
				{Key: "candidate_1_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_2_export_ref", Value: "cur2-2222222222222222"},
				{Key: "candidate_2_health", Value: "WARNING"},
				{Key: "candidate_2_output_format", Value: "PARQUET"},
				{Key: "candidate_2_destination_region", Value: "us-west-2"},
			})
		case "cur2-1111111111111111":
			return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
		case "cur2-2222222222222222":
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

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("preflight calls = %d, want initial plus two classification calls", len(calls))
	}
	if calls[1].Selectors.AWS.CUR2ExportRef != "cur2-1111111111111111" ||
		calls[2].Selectors.AWS.CUR2ExportRef != "cur2-2222222222222222" {
		t.Fatalf("classification refs = %#v, want both candidate refs", calls)
	}
	if strings.Contains(output, "Select AWS CUR 2.0 export") {
		t.Fatalf("output prompted even though only one candidate classified as selectable: %s", output)
	}
	for _, want := range []string{
		"Classifying 2 CUR 2.0 export candidates",
		"Auto-selected CUR 2.0 export cur2-1111111111111111",
		"aws_cur2_output_settings_blocked",
		"--export-ref cur2-1111111111111111",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
}

func TestRunAWSBillingPromptsForMultipleSelectableExports(t *testing.T) {
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
				{Key: "candidate_1_destination_region", Value: "us-east-1"},
				{Key: "candidate_2_export_ref", Value: "cur2-bbbbbbbbbbbbbbbb"},
				{Key: "candidate_2_health", Value: "HEALTHY"},
				{Key: "candidate_2_output_format", Value: "TEXT_OR_CSV"},
				{Key: "candidate_2_destination_region", Value: "us-west-2"},
			})
		case "cur2-aaaaaaaaaaaaaaaa":
			return guidedCapabilityReport(got, workflow.StatusReady, "aws_cur2_preflight_ready", nil)
		case "cur2-bbbbbbbbbbbbbbbb":
			return guidedCapabilityReport(got, workflow.RunStatusManualSteps, "aws_cur2_previous_month_manual", nil)
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
	if len(calls) != 3 {
		t.Fatalf("preflight calls = %d, want initial plus two classification calls", len(calls))
	}
	for _, want := range []string{
		"Select AWS CUR 2.0 export",
		"cur2-aaaaaaaaaaaaaaaa, health HEALTHY, output TEXT_OR_CSV, region us-east-1",
		"cur2-bbbbbbbbbbbbbbbb, health HEALTHY, output TEXT_OR_CSV, region us-west-2",
		"aws_cur2_previous_month_manual",
		"--export-ref cur2-bbbbbbbbbbbbbbbb",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
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
			{Key: "candidate_1_destination_region", Value: "us-east-1"},
			{Key: "candidate_2_export_ref", Value: "cur2-bbbbbbbbbbbbbbbb"},
			{Key: "candidate_2_health", Value: "arn:aws:iam::123456789012:role/operator"},
			{Key: "candidate_2_output_format", Value: "private_key=/Users/example/key.pem"},
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
		"cur2-aaaaaaaaaaaaaaaa, health HEALTHY, output PARQUET, region us-east-1",
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
			name:  "access key id shape",
			value: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "temporary access key id shape",
			value: "ASIAIOSFODNN7EXAMPLE",
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

func TestRunAWSBillingStopsWhenNoCUR2CandidateIsSelectable(t *testing.T) {
	registry := testRegistry(t, workflow.RunnerFunc(func(ctx context.Context, got workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
		exportRef := ""
		if options.Selectors != nil && options.Selectors.AWS != nil {
			exportRef = options.Selectors.AWS.CUR2ExportRef
		}
		switch exportRef {
		case "":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_export_ambiguous", []workflow.PlanEvidence{
				{Key: "candidate_1_export_ref", Value: "cur2-cccccccccccccccc"},
				{Key: "candidate_2_export_ref", Value: "cur2-dddddddddddddddd"},
			})
		case "cur2-cccccccccccccccc":
			return guidedCapabilityReport(got, workflow.RunStatusBlocked, "aws_cur2_output_settings_blocked", nil)
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

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

	if err != nil {
		t.Fatalf("RunWithConfig returned error: %v", err)
	}
	if strings.Contains(output, "Select AWS CUR 2.0 export") {
		t.Fatalf("output prompted even though no candidate was selectable: %s", output)
	}
	for _, want := range []string{
		"No selectable CUR 2.0 export candidate was found.",
		"cur2-cccccccccccccccc blocked: aws_cur2_output_settings_blocked",
		"cur2-dddddddddddddddd blocked: aws_data_exports_transient",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
	assertGuidedOutputSafe(t, output)
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
	if !strings.Contains(output, "Result: blocked (aws_cur2_export_ambiguous)") {
		t.Fatalf("output = %q, want original ambiguous summary", output)
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
