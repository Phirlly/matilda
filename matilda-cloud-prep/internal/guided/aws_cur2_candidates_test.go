package guided

import (
	"context"
	"reflect"
	"strings"
	"testing"

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

func TestRunAWSBillingAutoSelectsSingleCandidateAndRunsSelectedPreflight(t *testing.T) {
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

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

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
		"Auto-selected CUR 2.0 export cur2-cacacacacacacaca",
		"Recommendation: preferred Rapid Assessment billing export shape.",
		"Full readiness checks run after selection.",
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

			output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

			if err != nil {
				t.Fatalf("RunWithConfig returned error: %v", err)
			}
			if len(calls) != 1 {
				t.Fatalf("preflight calls = %d, want initial discovery only", len(calls))
			}
			for _, want := range []string{
				"One AWS CUR 2.0 export candidate needs review.",
				tt.want,
				"Full readiness checks run after selection.",
				"Review with:",
				"matilda-prep rapid-assessment billing aws preflight --profile default --region us-east-1 --export-ref",
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("output = %q, want to contain %q", output, want)
				}
			}
			for _, forbidden := range []string{
				"Auto-selected CUR 2.0 export",
				"Running readiness preflight for selected CUR 2.0 export",
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

	output, err := runGuidedWithConfig("1\n1\ny\n", Config{Registry: registry, AWSBilling: guide})

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

func TestWriteSelectableCUR2CandidateIncludesPolicyActionWhenTopLevelCodeIsAnotherWarning(t *testing.T) {
	item := classifiedCUR2Candidate{
		Candidate: cur2Candidate{Ref: "cur2-fdddddddddddddd"},
		Result: workflow.Result{
			Status: workflow.StatusReady,
			Code:   "aws_cur2_delivery_not_started",
			Plan: &workflow.ExecutionPlan{Checks: []workflow.PlanCheck{
				cur2PlanCheck(workflow.CheckWarn, "aws_cur2_delivery_not_started"),
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
				cur2PlanCheck(workflow.CheckWarn, "aws_cur2_delivery_not_started"),
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
