package cur2preflight

import (
	"strings"
	"testing"
	"time"
)

func TestPublicHelpersMirrorPreflightSelectionAndPreviousMonthRules(t *testing.T) {
	export := Export{
		Name:           "matilda-cur2",
		ExportARN:      "arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda-cur2",
		QueryStatement: "SELECT line_item_product_code FROM COST_AND_USAGE_REPORT",
		TableConfigurations: map[string]map[string]string{
			"COST_AND_USAGE_REPORT": {"TIME_GRANULARITY": "MONTHLY"},
		},
		Destination: S3Destination{
			Prefix: "matilda/cur2",
		},
	}

	ref := SafeCUR2ExportRef(export.ExportARN)
	if !strings.HasPrefix(ref, "cur2-") {
		t.Fatalf("SafeCUR2ExportRef = %q, want cur2- prefix", ref)
	}
	refs, err := SafeCUR2ExportRefs([]Export{export})
	if err != nil {
		t.Fatalf("SafeCUR2ExportRefs returned error: %v", err)
	}
	if len(refs) != 1 || refs[0] != ref {
		t.Fatalf("refs = %#v, want single ref %q", refs, ref)
	}
	if !IsCUR2ExportCandidate(export) {
		t.Fatal("IsCUR2ExportCandidate returned false for CUR 2.0 query/table configuration")
	}

	period := PreviousBillingPeriod(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	if period != "2026-06" {
		t.Fatalf("PreviousBillingPeriod = %q, want 2026-06", period)
	}
	dataPrefix := PreviousMonthDataPrefix(export, period)
	manifestPrefix := PreviousMonthManifestPrefix(export, period)
	if dataPrefix != "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/" {
		t.Fatalf("dataPrefix = %q", dataPrefix)
	}
	if manifestPrefix != "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/" {
		t.Fatalf("manifestPrefix = %q", manifestPrefix)
	}
	if !MatchesPreviousMonthDataKey(dataPrefix+"part-000.gz", export, period) {
		t.Fatal("MatchesPreviousMonthDataKey rejected valid data key")
	}
	if !MatchesPreviousMonthManifestKey(manifestPrefix+"Manifest.json", export, period) {
		t.Fatal("MatchesPreviousMonthManifestKey rejected valid manifest key")
	}
}
