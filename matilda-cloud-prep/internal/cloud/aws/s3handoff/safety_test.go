package s3handoff

import "testing"

func TestSafeS3HandoffValues(t *testing.T) {
	t.Run("safe values", func(t *testing.T) {
		if got := Bucket("matilda-cur2-billing"); got != "matilda-cur2-billing" {
			t.Fatalf("Bucket returned %q", got)
		}
		if got := Bucket("12345678901a"); got != "12345678901a" {
			t.Fatalf("Bucket returned %q", got)
		}
		if got := ConfiguredPrefix("matilda/cur2"); got != "matilda/cur2" {
			t.Fatalf("ConfiguredPrefix returned %q", got)
		}
		if got := ReportPrefix("matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/"); got != "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/" {
			t.Fatalf("ReportPrefix returned %q", got)
		}
		if got := Region("us-east-1"); got != "us-east-1" {
			t.Fatalf("Region returned %q", got)
		}
		if got := URI("matilda-cur2-billing", "matilda/cur2"); got != "s3://matilda-cur2-billing/matilda/cur2" {
			t.Fatalf("URI returned %q", got)
		}
	})

	t.Run("unsafe buckets", func(t *testing.T) {
		for _, value := range []string{
			"123456789012",
			"cur2123456789012billing",
			"arn:aws:s3:::matilda-cur2-billing",
			"token=plain-token",
			"AKIAIOSFODNN7EXAMPLE",
			"UppercaseBucket",
		} {
			if got := Bucket(value); got != "" {
				t.Fatalf("Bucket(%q) = %q, want empty", value, got)
			}
		}
	})

	t.Run("unsafe regions", func(t *testing.T) {
		for _, value := range []string{
			"us/east/1",
			"us_east_1",
			"arn:aws",
			"123456789012",
			"token=plain-token",
			"unsafe\x00region",
		} {
			if got := Region(value); got != "" {
				t.Fatalf("Region(%q) = %q, want empty", value, got)
			}
		}
	})

	t.Run("unsafe configured prefixes", func(t *testing.T) {
		for _, value := range []string{
			"../cur2",
			"/Users/lly/private",
			"s3://matilda-cur2-billing/matilda/cur2",
			"matilda//cur2",
			"matilda/cur2123456789012billing",
			"matilda/cur2/BILLING_PERIOD=2026-06/part-000.gz",
			"matilda/cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
		} {
			if got := ConfiguredPrefix(value); got != "" {
				t.Fatalf("ConfiguredPrefix(%q) = %q, want empty", value, got)
			}
		}
	})

	t.Run("unsafe report prefixes", func(t *testing.T) {
		for _, value := range []string{
			"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz",
			"matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json",
			"matilda/cur2123456789012billing/data/BILLING_PERIOD=2026-06/",
			"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06",
			"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06//",
		} {
			if got := ReportPrefix(value); got != "" {
				t.Fatalf("ReportPrefix(%q) = %q, want empty", value, got)
			}
		}
	})
}

func TestURIRequiresSafeBucketAndPrefix(t *testing.T) {
	for _, tt := range []struct {
		name   string
		bucket string
		prefix string
	}{
		{name: "empty bucket", bucket: "", prefix: "matilda/cur2"},
		{name: "empty prefix", bucket: "matilda-cur2-billing", prefix: ""},
		{name: "unsafe bucket", bucket: "token=plain-token", prefix: "matilda/cur2"},
		{name: "unsafe prefix", bucket: "matilda-cur2-billing", prefix: "../matilda/cur2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := URI(tt.bucket, tt.prefix); got != "" {
				t.Fatalf("URI(%q, %q) = %q, want empty", tt.bucket, tt.prefix, got)
			}
		})
	}
}

func TestSensitiveIdentifierLikeRejectsEmbeddedAccessKeyShapes(t *testing.T) {
	for _, value := range []string{
		"prefix-AKIAIOSFODNN7EXAMPLE-safe",
		"prefix-ASIAIOSFODNN7EXAMPLE-safe",
		"cur2123456789012billing",
	} {
		if !SensitiveIdentifierLike(value) {
			t.Fatalf("SensitiveIdentifierLike(%q) = false, want true", value)
		}
	}

	for _, value := range []string{
		"TEXT_OR_CSV",
		"GZIP",
		"MONTHLY",
		"CREATE_NEW_REPORT",
		"PARQUET",
		"OVERWRITE_REPORT",
		"us-east-1",
		"12345678901a",
		"AKIAIOSFODNN7EXAMPL_",
	} {
		if SensitiveIdentifierLike(value) {
			t.Fatalf("SensitiveIdentifierLike(%q) = true, want false", value)
		}
	}
}

func TestEvidenceBuildersUseOnlySafeValues(t *testing.T) {
	destination := DestinationEvidence("matilda-cur2-billing", "matilda/cur2", "us-east-1")
	if len(destination) != 3 {
		t.Fatalf("DestinationEvidence length = %d, want 3: %#v", len(destination), destination)
	}

	unsafeDestination := DestinationEvidence("token=plain-token", "matilda/cur2/BILLING_PERIOD=2026-06/part-000.gz", "arn:aws")
	if len(unsafeDestination) != 0 {
		t.Fatalf("unsafe DestinationEvidence = %#v, want empty", unsafeDestination)
	}

	previous := PreviousMonthEvidence("2026-06", "matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/", "matilda/cur2/matilda-cur2/metadata/BILLING_PERIOD=2026-06/")
	if len(previous) != 3 {
		t.Fatalf("PreviousMonthEvidence length = %d, want 3: %#v", len(previous), previous)
	}

	unsafePrevious := PreviousMonthEvidence("2026-06", "matilda/cur2/data/BILLING_PERIOD=2026-06/part-000.gz", "matilda/cur2/metadata/BILLING_PERIOD=2026-06/Manifest.json")
	if len(unsafePrevious) != 1 || unsafePrevious[0].Key != "previous_billing_period" {
		t.Fatalf("unsafe PreviousMonthEvidence = %#v, want only billing period", unsafePrevious)
	}
}
