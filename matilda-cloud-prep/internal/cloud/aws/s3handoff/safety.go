package s3handoff

import (
	"strings"
	"unicode"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func DestinationEvidence(bucket string, prefix string, region string) []workflow.PlanEvidence {
	evidence := []workflow.PlanEvidence{}
	if bucket := Bucket(bucket); bucket != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "s3_bucket", Value: bucket})
	}
	if prefix := ConfiguredPrefix(prefix); prefix != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "s3_prefix", Value: prefix})
	}
	if region := Region(region); region != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "s3_region", Value: region})
	}
	return evidence
}

func PreviousMonthEvidence(period string, dataPrefix string, manifestPrefix string) []workflow.PlanEvidence {
	evidence := []workflow.PlanEvidence{{Key: "previous_billing_period", Value: period}}
	if prefix := ReportPrefix(dataPrefix); prefix != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "cur2_data_prefix", Value: prefix})
	}
	if prefix := ReportPrefix(manifestPrefix); prefix != "" {
		evidence = append(evidence, workflow.PlanEvidence{Key: "cur2_manifest_prefix", Value: prefix})
	}
	return evidence
}

func URI(bucket string, prefix string) string {
	bucket = Bucket(bucket)
	prefix = Path(prefix)
	if bucket == "" || prefix == "" {
		return ""
	}
	return "s3://" + bucket + "/" + prefix
}

func Bucket(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || unsafeEvidenceText(value) || SensitiveIdentifierLike(value) {
		return ""
	}
	if len(value) < 3 || len(value) > 63 {
		return ""
	}
	if strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") ||
		strings.HasPrefix(value, "-") || strings.HasSuffix(value, "-") ||
		strings.Contains(value, "..") {
		return ""
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '-' {
			return ""
		}
	}
	return value
}

func ConfiguredPrefix(value string) string {
	value = Path(value)
	if value == "" || strings.Contains(strings.ToLower(value), "billing_period=") || LooksLikeObjectKey(value) {
		return ""
	}
	return strings.TrimSuffix(value, "/")
}

func ReportPrefix(value string) string {
	value = Path(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if !strings.HasSuffix(value, "/") || !strings.Contains(lower, "/billing_period=") || LooksLikeObjectKey(value) {
		return ""
	}
	return value
}

func Region(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || unsafeEvidenceText(value) || SensitiveIdentifierLike(value) || strings.ContainsAny(value, `/\`) {
		return ""
	}
	if strings.ContainsFunc(value, unicode.IsControl) || strings.Contains(value, "..") {
		return ""
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return ""
		}
	}
	if !strings.Contains(value, "-") {
		return ""
	}
	return value
}

func Path(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || unsafeEvidenceText(value) || unsafePathShape(value) {
		return ""
	}
	return value
}

func SensitiveIdentifierLike(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 12 && allDigits(value) {
		return true
	}
	if containsConsecutiveDigits(value, 12) {
		return true
	}
	upper := strings.ToUpper(value)
	if len(upper) == 20 &&
		(strings.HasPrefix(upper, "AKIA") || strings.HasPrefix(upper, "ASIA")) &&
		allUpperAlphaNumeric(upper) {
		return true
	}
	for index := 0; index+20 <= len(upper); index++ {
		candidate := upper[index : index+20]
		if (strings.HasPrefix(candidate, "AKIA") || strings.HasPrefix(candidate, "ASIA")) &&
			allUpperAlphaNumeric(candidate) {
			return true
		}
	}
	return false
}

func LooksLikeObjectKey(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "/part-") ||
		strings.HasSuffix(lower, "manifest.json") ||
		strings.HasSuffix(lower, ".gz") ||
		strings.HasSuffix(lower, ".gzip") ||
		strings.HasSuffix(lower, ".csv") ||
		strings.HasSuffix(lower, ".parquet") ||
		strings.HasSuffix(lower, ".json")
}

func unsafeEvidenceText(value string) bool {
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"/users/",
		"/private/",
		"/tmp/",
		"/home/",
		"\\",
		".pem",
		"access_key",
		"apikey",
		"api_key",
		"arn:",
		"bearer ",
		"client-secret",
		"client_secret",
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
		"x-amz-",
	} {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	return containsSensitiveSegment(value)
}

func unsafePathShape(value string) bool {
	return strings.HasPrefix(value, "/") ||
		strings.Contains(value, "://") ||
		strings.Contains(value, "..") ||
		strings.Contains(value, "//") ||
		strings.ContainsFunc(value, unicode.IsControl)
}

func containsSensitiveSegment(value string) bool {
	for _, segment := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == '.' || r == '_' || r == '-'
	}) {
		if SensitiveIdentifierLike(segment) {
			return true
		}
	}
	return false
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func containsConsecutiveDigits(value string, count int) bool {
	if count <= 0 {
		return false
	}
	run := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			run++
			if run >= count {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func allUpperAlphaNumeric(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return value != ""
}
