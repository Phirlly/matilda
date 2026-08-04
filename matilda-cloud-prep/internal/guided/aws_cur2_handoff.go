package guided

import (
	"fmt"
	"io"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/s3handoff"
)

func writeCUR2HandoffLocationFacts(stdout io.Writer, facts cur2CandidateFacts, indent string) {
	if facts.UnsafeHandoffEvidence {
		return
	}
	safeBucket := s3handoff.Bucket(facts.S3Bucket)
	configuredPrefix := s3handoff.ConfiguredPrefix(facts.S3Prefix)
	dataPrefix := s3handoff.ReportPrefix(facts.DataPrefix)
	manifestPrefix := s3handoff.ReportPrefix(facts.ManifestPrefix)
	reportLocation := s3handoff.URI(facts.S3Bucket, s3handoff.ConfiguredPrefix(facts.S3Prefix))
	dataLocation := s3handoff.URI(facts.S3Bucket, s3handoff.ReportPrefix(facts.DataPrefix))
	manifestLocation := s3handoff.URI(facts.S3Bucket, s3handoff.ReportPrefix(facts.ManifestPrefix))
	hasRelativeLocation := configuredPrefix != "" || dataPrefix != "" || manifestPrefix != ""
	if reportLocation == "" && dataLocation == "" && manifestLocation == "" && !hasRelativeLocation {
		return
	}
	dataPrefixLabel := "Billing data prefix"
	manifestPrefixLabel := "Manifest prefix"
	if facts.previousMonthMissing() {
		dataPrefixLabel = "Expected billing data prefix"
		manifestPrefixLabel = "Expected manifest prefix"
	}
	if safeBucket == "" && hasRelativeLocation {
		fmt.Fprintf(stdout, "%sS3 bucket: not shown because the bucket value may contain a sensitive identifier.\n", indent)
	}
	if reportLocation != "" {
		fmt.Fprintf(stdout, "%sReport location: %s\n", indent, reportLocation)
	} else if configuredPrefix != "" {
		fmt.Fprintf(stdout, "%sConfigured report prefix: %s\n", indent, configuredPrefix)
	}
	if dataLocation != "" {
		fmt.Fprintf(stdout, "%s%s: %s\n", indent, dataPrefixLabel, dataLocation)
	} else if dataPrefix != "" {
		fmt.Fprintf(stdout, "%s%s: %s\n", indent, dataPrefixLabel, dataPrefix)
	}
	if manifestLocation != "" {
		fmt.Fprintf(stdout, "%s%s: %s\n", indent, manifestPrefixLabel, manifestLocation)
	} else if manifestPrefix != "" {
		fmt.Fprintf(stdout, "%s%s: %s\n", indent, manifestPrefixLabel, manifestPrefix)
	}
	if facts.S3Region != "" {
		fmt.Fprintf(stdout, "%sDestination region: %s\n", indent, facts.S3Region)
	}
	if facts.previousMonthMissing() {
		if safeBucket == "" {
			fmt.Fprintf(stdout, "%sMatilda next step: complete previous-month billing data backfill first; after preflight is ready, use an AWS cloud account with Skip Configuration and provide the CUR 2.0 billing data from the selected export destination.\n", indent)
		} else {
			fmt.Fprintf(stdout, "%sMatilda next step: complete previous-month billing data backfill first; after preflight is ready, use an AWS cloud account with Skip Configuration and provide the CUR 2.0 billing data from this S3 location.\n", indent)
		}
	} else if safeBucket == "" {
		fmt.Fprintf(stdout, "%sMatilda next step: use an AWS cloud account with Skip Configuration, then create Rapid Assessment - Billing Based and provide the CUR 2.0 billing data from the selected export destination.\n", indent)
	} else {
		fmt.Fprintf(stdout, "%sMatilda next step: use an AWS cloud account with Skip Configuration, then create Rapid Assessment - Billing Based and provide the CUR 2.0 billing data from this S3 location.\n", indent)
	}
	fmt.Fprintf(stdout, "%sLarge data note: CSV and Parquet are supported; if direct upload size is too large, use Matilda's larger-file utility path after this tool completes.\n", indent)
}
