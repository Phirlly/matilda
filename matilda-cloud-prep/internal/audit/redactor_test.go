package audit

import (
	"strings"
	"testing"
)

func TestRedactStringRemovesSecretLikeValues(t *testing.T) {
	input := strings.Join([]string{
		"client_secret=plain-secret",
		"token: plain-token",
		"api_key=plain-api-key",
		"private_key_id=plain-private-key-id",
		"service_account_key=plain-service-account-key",
		"service-account-key=plain-service-account-key-dashed",
		"serviceaccount_key=plain-serviceaccount-key",
		"sa-key=plain-sa-key",
		"secret_key=plain-secret-key",
		"key_content=plain-key-content",
		"key_phrase=plain-key-phrase",
		"session_token=plain-session-token",
		"cookie=session=plain-cookie; path=/",
		"authorization=Bearer plain-authorization",
		"header=Bearer plain-header-secret",
		"Key Content=plain-oci-key-content",
		"Key Phrase=plain-oci-key-phrase",
		`"private_key":"-----BEGIN PRIVATE KEY-----plain-key-----END PRIVATE KEY-----"`,
	}, " ")

	got := RedactString(input)

	for _, leaked := range []string{
		"plain-secret",
		"plain-token",
		"plain-api-key",
		"plain-private-key-id",
		"plain-service-account-key",
		"plain-service-account-key-dashed",
		"plain-serviceaccount-key",
		"plain-sa-key",
		"plain-secret-key",
		"plain-key-content",
		"plain-key-phrase",
		"plain-session-token",
		"plain-cookie",
		"plain-authorization",
		"plain-header-secret",
		"plain-oci-key-content",
		"plain-oci-key-phrase",
		"BEGIN PRIVATE KEY",
		"plain-key",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("RedactString leaked %q in %q", leaked, got)
		}
	}

	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("RedactString(%q) = %q, want redaction marker", input, got)
	}
}
