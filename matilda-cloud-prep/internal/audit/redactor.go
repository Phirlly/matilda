package audit

import "regexp"

const RedactionMarker = "[REDACTED]"

var (
	privateKeyBlock = regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	headerPair      = regexp.MustCompile(`(?i)("?\b(?:authorization|cookie|header)\b"?\s*[:=]\s*)("[^"]*"|'[^']*'|[^"\n\r,}]+)`)
	secretPair      = regexp.MustCompile(`(?i)("?\b(?:client[ _-]?secret|secret|secret[ _-]?key|secret[ _-]?access[ _-]?key|session[ _-]?token|access[ _-]?token|refresh[ _-]?token|id[ _-]?token|token|api[ _-]?key|apikey|access[ _-]?key(?:[ _-]?id)?|private[ _-]?key(?:[ _-]?id)?|service[ _-]?account[ _-]?key|sa[ _-]?key|key[ _-]?content|key[ _-]?phrase|passphrase|password)\b"?\s*[:=]\s*)("[^"]*"|'[^']*'|[^"\s,}]+)`)
)

func RedactString(value string) string {
	redacted := privateKeyBlock.ReplaceAllString(value, RedactionMarker)
	redacted = headerPair.ReplaceAllString(redacted, `${1}"`+RedactionMarker+`"`)
	return secretPair.ReplaceAllString(redacted, `${1}"`+RedactionMarker+`"`)
}
