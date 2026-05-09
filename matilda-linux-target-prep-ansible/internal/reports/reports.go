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
		return nil, fmt.Errorf("validation summary not found; run validate first: %w", err)
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
		for len(record) < 9 {
			record = append(record, "")
		}
		rows = append(rows, Row{
			Host:          record[0],
			DiscoveryIP:   record[1],
			Command:       record[2],
			FallbackUsed:  record[3],
			LocalSudo:     record[4],
			DeniedCommand: record[5],
			ProbeSSH:      record[6],
			Ready:         record[7],
			Remediation:   record[8],
		})
	}
	return rows, nil
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
