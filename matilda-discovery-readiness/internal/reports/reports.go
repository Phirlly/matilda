package reports

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

type Row struct {
	Host          string `json:"host"`
	DiscoveryIP   string `json:"discovery_ip"`
	Command       string `json:"command"`
	FallbackUsed  string `json:"fallback_used"`
	LocalSudo     string `json:"local_sudo"`
	DeniedCommand string `json:"denied_command"`
	ProbeSSH      string `json:"probe_ssh"`
	Ready         string `json:"ready"`
	Remediation   string `json:"remediation"`
	FailureCode   string `json:"-"`
}

type Summary struct {
	Total    int
	Ready    int
	NotReady int
}

func Rows(reportDir string) ([]Row, error) {
	return readSummary(filepath.Join(reportDir, "validation-summary.txt"))
}

func Generate(reportDir string) ([]string, error) {
	summaryPath := filepath.Join(reportDir, "validation-summary.txt")
	rows, err := readSummary(summaryPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return nil, err
	}

	paths := []string{
		filepath.Join(reportDir, "readiness.csv"),
		filepath.Join(reportDir, "readiness.json"),
		filepath.Join(reportDir, "readiness.md"),
		filepath.Join(reportDir, "readiness.html"),
	}

	if err := writeCSV(paths[0], rows); err != nil {
		return nil, err
	}
	if err := writeJSON(paths[1], rows); err != nil {
		return nil, err
	}
	if err := writeMarkdown(paths[2], rows); err != nil {
		return nil, err
	}
	if err := writeHTML(paths[3], rows); err != nil {
		return nil, err
	}
	return paths, nil
}

func Summarize(reportDir string) (Summary, error) {
	rows, err := readSummary(filepath.Join(reportDir, "validation-summary.txt"))
	if err != nil {
		return Summary{}, err
	}
	var summary Summary
	summary.Total = len(rows)
	for _, row := range rows {
		if strings.EqualFold(row.Ready, "YES") {
			summary.Ready++
		} else {
			summary.NotReady++
		}
	}
	return summary, nil
}

func readSummary(path string) ([]Row, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("validation summary not found; run validate first")
		}
		return nil, fmt.Errorf("could not open validation summary: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, errors.New("validation summary has no target rows")
	}

	var rows []Row
	for _, record := range records[1:] {
		for len(record) < 10 {
			record = append(record, "")
		}
		row := Row{
			Host:          strings.TrimSpace(record[0]),
			DiscoveryIP:   strings.TrimSpace(record[1]),
			Command:       strings.TrimSpace(record[2]),
			FallbackUsed:  strings.TrimSpace(record[3]),
			LocalSudo:     strings.TrimSpace(record[4]),
			DeniedCommand: strings.TrimSpace(record[5]),
			ProbeSSH:      strings.TrimSpace(record[6]),
			Ready:         strings.TrimSpace(record[7]),
			Remediation:   compact(record[8]),
			FailureCode:   strings.TrimSpace(record[9]),
		}
		row.Remediation = normalizeRemediation(row)
		rows = append(rows, row)
	}
	return rows, nil
}

func normalizeRemediation(row Row) string {
	raw := compact(row.Remediation)
	if strings.EqualFold(row.Ready, "YES") {
		if raw == "" {
			return "None"
		}
		return raw
	}

	code := strings.ToUpper(strings.TrimSpace(row.FailureCode))
	if code == "" || code == "VALIDATION_FAILED" {
		if inferred := inferFailureCode(row, raw); inferred != "" {
			code = inferred
		}
	}

	switch code {
	case "SUDO_PASSWORD_REQUIRED":
		return withObserved("Passwordless sudo is not available for the Matilda service account. Rerun setup, or validate the Matilda sudoers drop-in with visudo -cf and confirm sudo -n works for matilda-svc.", raw)
	case "SUDO_NOT_ALLOWED":
		return withObserved("The Matilda service account is not allowed to run the discovery command with sudo. Rerun setup or review the Matilda sudoers drop-in so documented discovery commands are allowed.", raw)
	case "SUDO_TTY_REQUIRED":
		return withObserved("Sudo requires a TTY for the Matilda service account. Remove requiretty for matilda-svc or restore the Matilda sudoers drop-in, then rerun validate.", raw)
	case "SSH_PUBLICKEY_DENIED":
		return withObserved(sshPublicKeyMessage(row), raw)
	case "SSH_UNREACHABLE":
		return withObserved(sshReachabilityMessage(row), raw)
	case "SSH_CONNECTION_REFUSED":
		return withObserved("SSH reached the host but TCP/22 was refused. Start or enable sshd on the target, check the target firewall, then rerun preflight and validate.", raw)
	case "SSH_HOST_UNRESOLVED":
		return withObserved("SSH could not resolve the configured host. Fix inventory.yml ansible_host or DNS for the target or Probe path, then rerun inventory validate and preflight.", raw)
	case "SERVICE_ACCOUNT_LOCKED":
		return withObserved("The Matilda service account is locked or has a non-login shell. Rerun setup or unlock the account and restore an interactive shell.", raw)
	case "SERVICE_ACCOUNT_MISSING":
		return withObserved("The Matilda service account is missing. Rerun setup to recreate the account, home directory, key, and sudoers configuration.", raw)
	case "DENIED_COMMAND_ALLOWED":
		return withObserved("The sudoers allow-list allowed an unapproved command. Treat this as over-permissioned access: restore the Matilda sudoers drop-in from the template and validate it with visudo -cf.", raw)
	case "VALIDATION_COMMAND_MISSING":
		return withObserved("Neither ifconfig nor ip is available on the target. Install net-tools for ifconfig or make iproute available, then rerun validate.", raw)
	case "LOCAL_PREREQUISITE_MISSING":
		return withObserved("A local operator prerequisite is missing. Run ./matilda-prep doctor, install the missing command, then rerun the workflow.", raw)
	case "PROBE_VALIDATION_FAILED":
		return withObserved("Probe-to-target validation failed. From MatildaProbeVM, confirm target TCP/22 reachability, the Probe private key path, SSH as matilda-svc, and sudo -n for the discovery command.", raw)
	}

	if raw == "" {
		return "Validation failed. Review validation-summary.txt and the Ansible output for the target-specific error."
	}
	return raw
}

func inferFailureCode(row Row, raw string) string {
	text := strings.ToLower(raw)
	switch {
	case strings.Contains(text, "a password is required"):
		return "SUDO_PASSWORD_REQUIRED"
	case strings.Contains(text, "not in the sudoers file") ||
		strings.Contains(text, "is not allowed to execute") ||
		strings.Contains(text, "not allowed to run sudo"):
		return "SUDO_NOT_ALLOWED"
	case strings.Contains(text, "must have a tty"):
		return "SUDO_TTY_REQUIRED"
	case strings.Contains(text, "permission denied") && strings.Contains(text, "publickey"):
		return "SSH_PUBLICKEY_DENIED"
	case strings.Contains(text, "connection refused"):
		return "SSH_CONNECTION_REFUSED"
	case strings.Contains(text, "could not resolve hostname") ||
		strings.Contains(text, "name or service not known") ||
		strings.Contains(text, "temporary failure in name resolution"):
		return "SSH_HOST_UNRESOLVED"
	case strings.Contains(text, "connection timed out") ||
		strings.Contains(text, "operation timed out") ||
		strings.Contains(text, "no route to host") ||
		strings.Contains(text, "network is unreachable"):
		return "SSH_UNREACHABLE"
	case strings.Contains(text, "this account is currently not available"):
		return "SERVICE_ACCOUNT_LOCKED"
	case strings.Contains(text, "unknown user"):
		return "SERVICE_ACCOUNT_MISSING"
	case strings.Contains(text, "unapproved sudo command was not denied"):
		return "DENIED_COMMAND_ALLOWED"
	case strings.Contains(text, "neither ifconfig nor ip"):
		return "VALIDATION_COMMAND_MISSING"
	case strings.Contains(text, "executable file not found") ||
		strings.Contains(text, "no such file or directory") && strings.Contains(text, "ansible"):
		return "LOCAL_PREREQUISITE_MISSING"
	case strings.EqualFold(row.ProbeSSH, "FAIL"):
		return "PROBE_VALIDATION_FAILED"
	default:
		return ""
	}
}

func sshPublicKeyMessage(row Row) string {
	if strings.EqualFold(row.ProbeSSH, "FAIL") {
		return "SSH public key authentication failed on the Probe-to-target path. Confirm the Probe private key path on MatildaProbeVM matches the target authorized_keys entry for matilda-svc."
	}
	return "SSH public key authentication failed for the Matilda service account. Rerun setup and confirm the Probe private key matches the public key installed on the target."
}

func sshReachabilityMessage(row Row) string {
	if strings.EqualFold(row.ProbeSSH, "FAIL") {
		return "MatildaProbeVM cannot reach target TCP/22. Check routing, security lists or NSGs, and the target firewall from the Probe to the discovery IP."
	}
	return "SSH cannot reach target TCP/22. Check inventory.yml, routing, security lists or NSGs, and the target firewall, then rerun preflight."
}

func withObserved(message string, raw string) string {
	raw = compact(raw)
	if raw == "" || strings.EqualFold(raw, "None") {
		return message
	}
	if len(raw) > 220 {
		raw = strings.TrimSpace(raw[:220]) + "..."
	}
	return message + " Observed failure: " + raw
}

func compact(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func writeCSV(path string, rows []Row) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	_ = writer.Write([]string{"Host", "DiscoveryIP", "Command", "FallbackUsed", "LocalSudo", "DeniedCommand", "ProbeSSH", "Ready", "Remediation"})
	for _, row := range rows {
		_ = writer.Write([]string{row.Host, row.DiscoveryIP, row.Command, row.FallbackUsed, row.LocalSudo, row.DeniedCommand, row.ProbeSSH, row.Ready, row.Remediation})
	}
	return writer.Error()
}

func writeJSON(path string, rows []Row) error {
	content, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0644)
}

func writeMarkdown(path string, rows []Row) error {
	var b strings.Builder
	b.WriteString("# Matilda Discovery Readiness Report\n\n")
	b.WriteString("| Host | Discovery IP | Ready | Local Sudo | Denied Command | Probe SSH | Remediation |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			escapeMD(row.Host), escapeMD(row.DiscoveryIP), escapeMD(row.Ready), escapeMD(row.LocalSudo), escapeMD(row.DeniedCommand), escapeMD(row.ProbeSSH), escapeMD(row.Remediation))
	}
	b.WriteString("\nUse only targets with `Ready=YES` in Matilda Network Discovery.\n")
	b.WriteString("Preparation may modify target systems; Matilda discovery itself is agentless and read-only.\n")
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func writeHTML(path string, rows []Row) error {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>Matilda Readiness Report</title>")
	b.WriteString("<style>body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;margin:32px;color:#17202a}table{border-collapse:collapse;width:100%}th,td{border:1px solid #d5d8dc;padding:8px;text-align:left}th{background:#f4f6f7}.yes{color:#146c2e;font-weight:700}.no{color:#a93226;font-weight:700}.note{margin:16px 0;color:#566573}</style>")
	b.WriteString("</head><body><h1>Matilda Discovery Readiness Report</h1>")
	b.WriteString("<p class=\"note\">Use only targets with Ready=YES in Matilda Network Discovery. Preparation may modify target systems; Matilda discovery itself is agentless and read-only.</p>")
	b.WriteString("<table><thead><tr><th>Host</th><th>Discovery IP</th><th>Ready</th><th>Local Sudo</th><th>Denied Command</th><th>Probe SSH</th><th>Remediation</th></tr></thead><tbody>")
	for _, row := range rows {
		class := "no"
		if strings.EqualFold(row.Ready, "YES") {
			class = "yes"
		}
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td class=\"%s\">%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
			html.EscapeString(row.Host), html.EscapeString(row.DiscoveryIP), class, html.EscapeString(row.Ready), html.EscapeString(row.LocalSudo), html.EscapeString(row.DeniedCommand), html.EscapeString(row.ProbeSSH), html.EscapeString(row.Remediation))
	}
	b.WriteString("</tbody></table></body></html>\n")
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func escapeMD(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
