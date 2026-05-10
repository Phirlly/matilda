package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"matilda-discovery-readiness/internal/config"
	"matilda-discovery-readiness/internal/inventory"
	"matilda-discovery-readiness/internal/reports"
	"matilda-discovery-readiness/internal/runner"
	"matilda-discovery-readiness/internal/state"
	"matilda-discovery-readiness/internal/ui"
	"matilda-discovery-readiness/internal/workflow"
)

type FileStatus struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

type Snapshot struct {
	InventoryPath   string            `json:"inventory_path"`
	InventoryFormat string            `json:"inventory_format"`
	InventoryOK     bool              `json:"inventory_ok"`
	InventoryError  string            `json:"inventory_error"`
	InventoryChecks []runner.Result   `json:"inventory_checks"`
	TargetCount     int               `json:"target_count"`
	ReportSummary   reports.Summary   `json:"report_summary"`
	ReportRows      []reports.Row     `json:"report_rows"`
	ReportError     string            `json:"report_error"`
	ValidatedIPs    []string          `json:"validated_ips"`
	ReportFiles     []FileStatus      `json:"report_files"`
	Runs            []state.RunRecord `json:"runs"`
	StatePath       string            `json:"state_path"`
	State           state.Document    `json:"state"`
	StateError      string            `json:"state_error,omitempty"`
	NextStep        string            `json:"next_step"`
}

func (s Snapshot) ReadinessTotal() int {
	if s.ReportSummary.Total > 0 {
		return s.ReportSummary.Total
	}
	return s.TargetCount
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
	store := state.New(r.Root)

	snap := Snapshot{
		InventoryPath:   inventoryPath,
		InventoryFormat: result.Format,
		InventoryOK:     invErr == nil,
		InventoryChecks: result.Checks,
		TargetCount:     result.TargetCount,
		ValidatedIPs:    readLines(filepath.Join(reportDir, "validated-discovery-ips.txt")),
		StatePath:       store.Path(),
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
	if doc, err := store.Read(); err == nil {
		snap.State = doc
	} else if !errors.Is(err, state.ErrNotFound) {
		snap.StateError = err.Error()
	}
	if runs, err := store.ListRuns(5); err == nil {
		snap.Runs = runs
	}
	snap.NextStep = nextStep(snap)
	return snap
}

func (r *Runtime) RunTrackedAction(action string, fn func() error) error {
	result := workflow.Start(action)
	err := fn()
	result.Finish(err, errors.Is(err, ErrCancelled))
	if recordErr := r.RecordWorkflowResult(result); recordErr != nil {
		fmt.Fprintf(r.Err, "state update failed: %v\n", recordErr)
	}
	return err
}

func (r *Runtime) RecordAction(action string, err error) {
	result := workflow.Start(action)
	result.Finish(err, errors.Is(err, ErrCancelled))
	if recordErr := r.RecordWorkflowResult(result); recordErr != nil {
		fmt.Fprintf(r.Err, "state update failed: %v\n", recordErr)
	}
}

func (r *Runtime) RecordWorkflowResult(result workflow.Result) error {
	snap := r.Snapshot()
	store := state.New(r.Root)
	_, err := store.Update(state.Update{
		Workspace: r.Root,
		Inventory: displayPath(r.Root, filepath.Join(r.Root, "inventory.yml")),
		Result:    result,
		Readiness: state.ReadinessState{
			Total:    snap.ReportSummary.Total,
			Ready:    snap.ReportSummary.Ready,
			NotReady: snap.ReportSummary.NotReady,
		},
		Reports: state.ReportState{
			LatestHTML:     displayPathIfExists(r.Root, filepath.Join(r.Root, "reports", "readiness.html")),
			LatestJSON:     displayPathIfExists(r.Root, filepath.Join(r.Root, "reports", "readiness.json")),
			LatestMarkdown: displayPathIfExists(r.Root, filepath.Join(r.Root, "reports", "readiness.md")),
			LatestCSV:      displayPathIfExists(r.Root, filepath.Join(r.Root, "reports", "readiness.csv")),
			ValidatedIPs:   displayPathIfExists(r.Root, filepath.Join(r.Root, "reports", "validated-discovery-ips.txt")),
		},
	})
	if err != nil {
		return err
	}
	return store.WriteRun(r.runRecord(result, snap))
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
		r.RecordAction(action, err)
		return result
	}
	result.OK = true
	r.RecordAction(action, nil)
	return result
}

func (r *Runtime) RunWorkflowAction(action string, confirmed bool) ActionResult {
	return r.RunWorkflowActionTo(action, confirmed, io.Discard, io.Discard)
}

func (r *Runtime) RunWorkflowActionTo(action string, confirmed bool, out io.Writer, errOut io.Writer) ActionResult {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}

	var outBuffer bytes.Buffer
	var errBuffer bytes.Buffer
	input := "\n"
	if confirmed {
		input = "y\n"
	}
	child := New(
		r.Root,
		strings.NewReader(input),
		io.MultiWriter(&outBuffer, out),
		io.MultiWriter(&errBuffer, errOut),
	)
	child.Context = r.Context
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
		renderer := ui.New(child.Out)
		renderer.Header("Validated IPs", "Ready target addresses for Matilda discovery.")
		ips := readLines(filepath.Join(r.Root, "reports", "validated-discovery-ips.txt"))
		if len(ips) == 0 {
			renderer.Warning("No validated discovery IPs yet.")
			renderer.Next("Run ./matilda-prep validate first.")
		} else {
			renderer.Section("Targets")
			for _, ip := range ips {
				fmt.Fprintf(child.Out, "  %s\n", ip)
			}
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

	if errors.Is(err, context.Canceled) {
		err = ErrCancelled
	}
	if errBuffer.Len() > 0 {
		outBuffer.WriteString("\n")
		outBuffer.Write(errBuffer.Bytes())
	}
	result.Output = strings.TrimSpace(outBuffer.String())
	if err != nil {
		result.Error = err.Error()
		child.RecordAction(action, err)
		return result
	}
	result.OK = true
	child.RecordAction(action, nil)
	return result
}

func remoteInputsReady(root string) error {
	values, err := config.LoadEnv(filepath.Join(root, ".env"))
	if err != nil {
		return errors.New("browser remote actions require .env; run ./matilda-prep init or use a terminal command for prompts")
	}
	for _, key := range config.RequiredKeys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			return fmt.Errorf("browser remote actions require .env value %s", key)
		}
		if config.LooksLikePlaceholder(value) {
			return fmt.Errorf("browser remote actions require a real value for %s; replace the placeholder in .env", key)
		}
		if config.IsLocalFileKey(key) {
			path := config.ExpandPath(value)
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("browser remote actions require an existing file for %s: %s", key, path)
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

func displayPathIfExists(root string, path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return displayPath(root, path)
}

func displayPath(root string, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return rel
	}
	return path
}
