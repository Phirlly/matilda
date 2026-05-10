package unit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/app"
)

func TestRuntimeSnapshotSummarizesInventoryAndReports(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "inventory.yml"), unitLinuxGroupedInventory())
	writeUnitFile(t, filepath.Join(root, "reports", "validation-summary.txt"), unitValidationSummary())
	writeUnitFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")

	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	snap := rt.Snapshot()

	if !snap.InventoryOK {
		t.Fatalf("expected inventory OK: %s", snap.InventoryError)
	}
	if snap.InventoryFormat != "linux-groups" || snap.TargetCount != 1 {
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
	if result.OK || !strings.Contains(result.Error, "require .env") {
		t.Fatalf("expected .env requirement for interactive remote action, got %+v", result)
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

func unitLinuxGroupedInventory() string {
	return `all:
  children:
    public_targets:
      hosts:
        app01:
          ansible_host: 203.0.113.10
          private_ip: 10.0.0.10
          discovery_ip: 10.0.0.10
    private_targets:
      hosts: {}
`
}

func unitValidationSummary() string {
	return "Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation\napp01,10.0.0.10,ifconfig,NO,OK,OK,OK,YES,None\n"
}
