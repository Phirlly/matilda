package unit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/reports"
)

func TestRuntimeSnapshotSummarizesInventoryAndReports(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, "reports", "validation-summary.txt"), unitValidationSummary())
	writeUnitFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")

	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	snap := rt.Snapshot()

	if !snap.InventoryOK {
		t.Fatalf("expected inventory OK: %s", snap.InventoryError)
	}
	if snap.InventoryPath != filepath.Join(root, "targets.csv") {
		t.Fatalf("unexpected inventory path: %s", snap.InventoryPath)
	}
	if snap.InventoryFormat != "csv" || snap.TargetCount != 1 {
		t.Fatalf("unexpected inventory snapshot: %+v", snap)
	}
	if snap.ReportSummary.Total != 1 || snap.ReportSummary.Ready != 1 || snap.ReportSummary.NotReady != 0 {
		t.Fatalf("unexpected report summary: %+v", snap.ReportSummary)
	}
	if len(snap.ReportRows) != 1 || snap.ReportRows[0].Host != "app01" {
		t.Fatalf("unexpected report rows: %+v", snap.ReportRows)
	}
	if len(snap.ValidatedIPs) != 1 || snap.ValidatedIPs[0] != "10.0.0.10" {
		t.Fatalf("unexpected validated IPs: %+v", snap.ValidatedIPs)
	}
	if !strings.Contains(snap.NextStep, "validated discovery IPs") {
		t.Fatalf("unexpected next step: %s", snap.NextStep)
	}
}

func TestSnapshotReadinessTotalFallsBackToInventoryCount(t *testing.T) {
	snap := app.Snapshot{TargetCount: 2}
	if got := snap.ReadinessTotal(); got != 2 {
		t.Fatalf("readiness total without reports = %d, want 2", got)
	}

	snap.ReportSummary = reports.Summary{Total: 1}
	if got := snap.ReadinessTotal(); got != 1 {
		t.Fatalf("readiness total with reports = %d, want 1", got)
	}
}

func TestRunLocalActionRejectsUnsupportedAction(t *testing.T) {
	rt := app.New(t.TempDir(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	result := rt.RunLocalAction("setup")
	if result.OK || !strings.Contains(result.Error, "unsupported local action") {
		t.Fatalf("expected unsupported action error, got %+v", result)
	}
}

func TestInteractiveRemoteActionRequiresEnvFile(t *testing.T) {
	rt := app.New(t.TempDir(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	result := rt.RunWorkflowAction("preflight", false)
	if result.OK || !strings.Contains(result.Error, "require .env") || !strings.Contains(result.Error, "browser cannot prompt") {
		t.Fatalf("expected .env requirement for interactive remote action, got %+v", result)
	}
}

func TestInteractiveRemoteActionReportsAllEnvIssues(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, ".env"), strings.Join([]string{
		"TARGET_ADMIN_USER=<target-admin-user>",
		"TARGET_ADMIN_PRIVATE_KEY_FILE=/missing/target-admin-key",
		"MATILDA_PROBE_ANSIBLE_HOST=203.0.113.20",
		"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE=/missing/probe-admin-key",
		"MATILDA_PUBLIC_KEY_FILE=/missing/matilda.pub",
		"MATILDA_PROBE_PRIVATE_KEY_ON_PROBE=/home/opc/MatildaProbeKey.pem",
		"",
	}, "\n"))
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	result := rt.RunWorkflowAction("preflight", false)
	if result.OK {
		t.Fatalf("expected preflight to fail with incomplete .env")
	}
	for _, want := range []string{
		"TARGET_ADMIN_USER still has a placeholder",
		"TARGET_ADMIN_PRIVATE_KEY_FILE file does not exist",
		"MATILDA_PROBE_USER is missing",
		"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE file does not exist",
		"MATILDA_PUBLIC_KEY_FILE file does not exist",
	} {
		if !strings.Contains(result.Error, want) {
			t.Fatalf("remote action error missing %q:\n%s", want, result.Error)
		}
	}
}

func TestWorkflowMutatingActionsRequireConfirmation(t *testing.T) {
	for _, action := range []string{
		"setup",
		"run",
		"rollback-sudoers",
		"rollback-remove-key",
		"rollback-lock-user",
		"rollback-delete-user",
	} {
		t.Run(action, func(t *testing.T) {
			rt := app.New(t.TempDir(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

			result := rt.RunWorkflowAction(action, false)
			if result.OK || !strings.Contains(result.Error, "requires confirmation") {
				t.Fatalf("expected confirmation error, got %+v", result)
			}

			confirmed := rt.RunWorkflowAction(action, true)
			if confirmed.OK || strings.Contains(confirmed.Error, "requires confirmation") || !strings.Contains(confirmed.Error, "require .env") {
				t.Fatalf("expected confirmed action to proceed to remote input checks, got %+v", confirmed)
			}
		})
	}
}

func TestSnapshotNextStepGuidesMissingInventorySetup(t *testing.T) {
	rt := app.New(t.TempDir(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	snap := rt.Snapshot()
	if snap.InventoryOK {
		t.Fatalf("expected missing inventory to fail")
	}
	if !strings.Contains(snap.NextStep, "./matilda-prep init") || !strings.Contains(snap.NextStep, "targets.example.csv") {
		t.Fatalf("unexpected next step for missing inventory: %s", snap.NextStep)
	}
}

func TestSnapshotNextStepGuidesPreflightBeforeReportsExist(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	snap := rt.Snapshot()
	if !snap.InventoryOK {
		t.Fatalf("expected inventory OK: %s", snap.InventoryError)
	}
	if snap.NextStep != "Run preflight before setup." {
		t.Fatalf("unexpected next step before reports exist: %s", snap.NextStep)
	}
}

func writeUnitFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func unitTargetsCSV() string {
	return "hostname,platform,os_family,ansible_host,discovery_ip,access_path,privilege_method,private_ip,public_ip,cloud_provider\napp01,linux,oracle_linux,203.0.113.10,10.0.0.10,direct,sudo,10.0.0.10,203.0.113.10,oci\n"
}

func unitValidationSummary() string {
	return "Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation\napp01,10.0.0.10,ifconfig,NO,OK,OK,OK,YES,None\n"
}
