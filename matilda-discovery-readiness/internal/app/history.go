package app

import (
	"fmt"
	"strings"
	"time"

	"matilda-discovery-readiness/internal/state"
	"matilda-discovery-readiness/internal/workflow"
)

func (r *Runtime) runRecord(result workflow.Result, snap Snapshot) state.RunRecord {
	return state.RunRecord{
		ID:                runID(result),
		Action:            result.Action,
		Status:            result.Status,
		StartedAt:         result.StartedAt,
		EndedAt:           result.CompletedAt,
		Command:           actionCommand(result.Action),
		ReadinessTotal:    snap.ReportSummary.Total,
		ReadinessReady:    snap.ReportSummary.Ready,
		ReadinessNotReady: snap.ReportSummary.NotReady,
		ReportPaths:       reportPaths(r.Root, snap.ReportFiles),
		Summary:           runSummary(result, snap),
		Error:             result.Error,
	}
}

func reportPaths(root string, files []FileStatus) []string {
	var paths []string
	for _, file := range files {
		if !file.Exists {
			continue
		}
		paths = append(paths, displayPath(root, file.Path))
	}
	return paths
}

func runID(result workflow.Result) string {
	started := result.StartedAt
	if t, ok := parseWorkflowTime(started); ok {
		started = t.UTC().Format("20060102T150405.000000000Z")
	}
	if started == "" {
		started = time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	return fmt.Sprintf("%s-%s", safeRunIDPart(started), safeRunIDPart(result.Action))
}

func safeRunIDPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	previousDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			previousDash = false
			continue
		}
		if !previousDash {
			b.WriteRune('-')
			previousDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func parseWorkflowTime(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func runSummary(result workflow.Result, snap Snapshot) string {
	status := string(result.Status)
	if result.Error != "" {
		return status + ": " + result.Error
	}
	if result.Status != workflow.StatusCompleted {
		return status
	}

	switch result.Action {
	case "validate", "report", "run":
		if snap.ReportSummary.Total > 0 {
			return fmt.Sprintf("%d/%d targets ready", snap.ReportSummary.Ready, snap.ReportSummary.Total)
		}
	case "doctor":
		return "local checks completed"
	case "inventory-validate":
		return "inventory checked"
	case "inventory-import":
		return "inventory imported"
	case "inventory-migrate":
		return "inventory migrated"
	case "preflight":
		return "preflight completed"
	case "setup":
		return "target setup completed"
	case "rollback", "rollback-sudoers", "rollback-remove-key", "rollback-lock-user", "rollback-delete-user":
		return "rollback completed"
	case "generate-windows", "generate-unix":
		return "guidance generated"
	case "validated-ips":
		return fmt.Sprintf("%d validated IPs", len(snap.ValidatedIPs))
	}
	return "completed"
}

func actionCommand(action string) string {
	switch action {
	case "init":
		return "./matilda-prep init"
	case "doctor":
		return "./matilda-prep doctor"
	case "inventory-validate":
		return "./matilda-prep inventory validate"
	case "inventory-import":
		return "./matilda-prep inventory import CSV"
	case "inventory-migrate":
		return "./matilda-prep inventory migrate"
	case "preflight":
		return "./matilda-prep preflight"
	case "setup":
		return "./matilda-prep setup"
	case "validate":
		return "./matilda-prep validate"
	case "run":
		return "./matilda-prep run"
	case "report":
		return "./matilda-prep report"
	case "generate-windows":
		return "./matilda-prep generate windows"
	case "generate-unix":
		return "./matilda-prep generate unix"
	case "rollback":
		return "./matilda-prep rollback MODE"
	case "rollback-sudoers":
		return "./matilda-prep rollback --sudoers-only"
	case "rollback-remove-key":
		return "./matilda-prep rollback --remove-key"
	case "rollback-lock-user":
		return "./matilda-prep rollback --lock-user"
	case "rollback-delete-user":
		return "./matilda-prep rollback --delete-user"
	default:
		return ""
	}
}
