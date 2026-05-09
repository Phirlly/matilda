package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"matilda-discovery-readiness/internal/config"
	"matilda-discovery-readiness/internal/inventory"
	"matilda-discovery-readiness/internal/reports"
	"matilda-discovery-readiness/internal/runner"
)

type FileStatus struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

type Snapshot struct {
	InventoryPath   string          `json:"inventory_path"`
	InventoryFormat string          `json:"inventory_format"`
	InventoryOK     bool            `json:"inventory_ok"`
	InventoryError  string          `json:"inventory_error"`
	InventoryChecks []runner.Result `json:"inventory_checks"`
	TargetCount     int             `json:"target_count"`
	ReportSummary   reports.Summary `json:"report_summary"`
	ReportRows      []reports.Row   `json:"report_rows"`
	ReportError     string          `json:"report_error"`
	ValidatedIPs    []string        `json:"validated_ips"`
	ReportFiles     []FileStatus    `json:"report_files"`
	NextStep        string          `json:"next_step"`
}

type ActionResult struct {
	Action string `json:"action"`
	OK     bool   `json:"ok"`
	Output string `json:"output"`
	Error  string `json:"error"`
}

func (r *Runtime) Snapshot() Snapshot {
	inventoryPath := filepath.Join(r.Root, "inventory.yml")
	reportDir := filepath.Join(r.Root, "reports")
	result, invErr := inventory.ValidateFile(inventoryPath)

	snap := Snapshot{
		InventoryPath:   inventoryPath,
		InventoryFormat: result.Format,
		InventoryOK:     invErr == nil,
		InventoryChecks: result.Checks,
		TargetCount:     result.TargetCount,
		ValidatedIPs:    readLines(filepath.Join(reportDir, "validated-discovery-ips.txt")),
		ReportFiles: []FileStatus{
			fileStatus("Validated IPs", filepath.Join(reportDir, "validated-discovery-ips.txt")),
			fileStatus("Validation Summary", filepath.Join(reportDir, "validation-summary.txt")),
			fileStatus("CSV Report", filepath.Join(reportDir, "readiness.csv")),
			fileStatus("JSON Report", filepath.Join(reportDir, "readiness.json")),
			fileStatus("Markdown Report", filepath.Join(reportDir, "readiness.md")),
			fileStatus("HTML Report", filepath.Join(reportDir, "readiness.html")),
		},
	}
	if invErr != nil {
		snap.InventoryError = invErr.Error()
	}

	summary, reportErr := reports.Summarize(reportDir)
	snap.ReportSummary = summary
	if rows, err := reports.Rows(reportDir); err == nil {
		snap.ReportRows = rows
	}
	if reportErr != nil {
		snap.ReportError = reportErr.Error()
	}
	snap.NextStep = nextStep(snap)
	return snap
}

func (r *Runtime) RunLocalAction(action string) ActionResult {
	var out bytes.Buffer
	var errOut bytes.Buffer
	child := New(r.Root, strings.NewReader("\n"), &out, &errOut)
	result := ActionResult{Action: action}

	var err error
	switch action {
	case "doctor":
		err = child.Doctor()
	case "inventory-validate":
		err = child.InventoryValidate()
	case "report":
		err = child.Report()
	case "generate-windows":
		err = child.Generate([]string{"windows"})
	case "generate-unix":
		err = child.Generate([]string{"unix"})
	default:
		err = fmt.Errorf("unsupported local action %q", action)
	}

	if errOut.Len() > 0 {
		out.WriteString("\n")
		out.Write(errOut.Bytes())
	}
	result.Output = strings.TrimSpace(out.String())
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.OK = true
	return result
}

func (r *Runtime) RunWorkflowAction(action string, confirmed bool) ActionResult {
	var out bytes.Buffer
	var errOut bytes.Buffer
	input := "\n"
	if confirmed {
		input = "y\n"
	}
	child := New(r.Root, strings.NewReader(input), &out, &errOut)
	result := ActionResult{Action: action}

	var err error
	switch action {
	case "doctor":
		err = child.Doctor()
	case "inventory-validate":
		err = child.InventoryValidate()
	case "report":
		err = child.Report()
	case "validated-ips":
		ips := readLines(filepath.Join(r.Root, "reports", "validated-discovery-ips.txt"))
		if len(ips) == 0 {
			fmt.Fprintln(&out, "No validated discovery IPs yet.")
		} else {
			fmt.Fprintln(&out, strings.Join(ips, "\n"))
		}
	case "generate-windows":
		err = child.Generate([]string{"windows"})
	case "generate-unix":
		err = child.Generate([]string{"unix"})
	case "preflight":
		if err = remoteInputsReady(r.Root); err == nil {
			err = child.Preflight()
		}
	case "setup":
		if !confirmed {
			err = errors.New("setup requires confirmation because it modifies target systems")
		} else {
			err = remoteInputsReady(r.Root)
			if err == nil {
				err = child.Setup()
			}
		}
	case "validate":
		if err = remoteInputsReady(r.Root); err == nil {
			err = child.Validate()
		}
	case "rollback-sudoers":
		if !confirmed {
			err = errors.New("rollback requires confirmation because it modifies target systems")
		} else {
			err = remoteInputsReady(r.Root)
			if err == nil {
				err = child.Rollback([]string{"--sudoers-only"})
			}
		}
	default:
		err = fmt.Errorf("unsupported workflow action %q", action)
	}

	if errOut.Len() > 0 {
		out.WriteString("\n")
		out.Write(errOut.Bytes())
	}
	result.Output = strings.TrimSpace(out.String())
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.OK = true
	return result
}

func remoteInputsReady(root string) error {
	values, err := config.LoadEnv(filepath.Join(root, ".env"))
	if err != nil {
		return errors.New("browser remote actions require .env; run init or use CLI/TUI prompts first")
	}
	for _, key := range config.RequiredKeys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			return fmt.Errorf("browser remote actions require .env value %s", key)
		}
		if config.IsLocalFileKey(key) {
			path := config.ExpandPath(value)
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("browser remote actions require existing file for %s: %s", key, path)
			}
		}
	}
	return nil
}

func (s Snapshot) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

func nextStep(s Snapshot) string {
	if !s.InventoryOK {
		return "Fix inventory.yml, then run inventory validate."
	}
	if s.ReportError != "" {
		return "Run validate to create readiness reports."
	}
	if s.ReportSummary.NotReady > 0 {
		return "Review remediation, rerun setup or platform fixes, then validate again."
	}
	if s.ReportSummary.Ready > 0 && s.ReportSummary.Ready == s.ReportSummary.Total {
		return "Use validated discovery IPs in Matilda Network Discovery."
	}
	return "Run preflight before setup."
}

func fileStatus(name string, path string) FileStatus {
	status := FileStatus{Name: name, Path: path}
	info, err := os.Stat(path)
	if err != nil {
		return status
	}
	status.Exists = true
	status.Size = info.Size()
	status.ModTime = info.ModTime().Format(time.RFC3339)
	return status
}

func readLines(path string) []string {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
