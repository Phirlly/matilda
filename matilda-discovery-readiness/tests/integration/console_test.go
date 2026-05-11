package integration

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/cli"
)

func TestConsoleStatusAndActions(t *testing.T) {
	root := withTempProject(t, validTargetsCSV(), validationSummary())
	writeFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")

	var out bytes.Buffer
	err := cli.Execute([]string{"console"}, strings.NewReader("q\n"), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("console failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Matilda Discovery Readiness", "Workflow", "Target Readiness", "Actions", "Guidance", "Remote", "Validated IPs", "10.0.0.10"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("console output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Select action") || strings.Contains(out.String(), "Press Enter") {
		t.Fatalf("console should not use legacy typed prompt output:\n%s", out.String())
	}
	if strings.Contains(strings.ToLower(out.String()), "linux-groups") {
		t.Fatalf("console should not expose internal inventory format labels:\n%s", out.String())
	}
}

func TestDefaultCommandOpensConsole(t *testing.T) {
	withTempProject(t, validTargetsCSV(), validationSummary())

	var out bytes.Buffer
	err := cli.Execute(nil, strings.NewReader("q\n"), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("default console failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Matilda Discovery Readiness", "Actions", "Guidance", "Remote"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("default console output missing %q:\n%s", want, out.String())
		}
	}
}

func TestStatusPrintsSummaryAndExits(t *testing.T) {
	withTempProject(t, validTargetsCSV(), validationSummary())

	var out bytes.Buffer
	err := cli.Execute([]string{"status"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Matilda Discovery Readiness", "Status", "Workflow", "Target Readiness", "Next Step"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Select action") {
		t.Fatalf("status should not enter the interactive console:\n%s", out.String())
	}
}

func TestStatusShowsNormalizedRemediation(t *testing.T) {
	t.Setenv("COLUMNS", "160")
	summary := strings.Join([]string{
		"Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation,FailureCode",
		"app01,10.0.0.10,ifconfig,NO,OK,OK,FAIL,NO,ssh: connect to host 10.0.0.10 port 22: Connection refused,VALIDATION_FAILED",
		"",
	}, "\n")
	withTempProject(t, validTargetsCSV(), summary)

	var out bytes.Buffer
	err := cli.Execute([]string{"status"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Ready      0/1", "TCP/22 was refused", "Review remediation"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q:\n%s", want, out.String())
		}
	}
}

func TestStatusUsesInventoryCountBeforeReportsExist(t *testing.T) {
	withTempProject(t, validTargetsCSV(), "")

	var out bytes.Buffer
	err := cli.Execute([]string{"status"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Ready      0/1") {
		t.Fatalf("status should use inventory target count before reports exist:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Reports    Pending") {
		t.Fatalf("status should show reports pending before validate:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Run preflight before setup") {
		t.Fatalf("status should guide first-run operators to preflight before reports exist:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Run validate to create readiness reports") {
		t.Fatalf("status should not skip preflight in the first-run next step:\n%s", out.String())
	}
}

func TestStartAliasOpensConsole(t *testing.T) {
	withTempProject(t, validTargetsCSV(), validationSummary())

	var out bytes.Buffer
	err := cli.Execute([]string{"start"}, strings.NewReader("q\n"), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("start alias failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Matilda Discovery Readiness") {
		t.Fatalf("start alias should open console:\n%s", out.String())
	}
}
