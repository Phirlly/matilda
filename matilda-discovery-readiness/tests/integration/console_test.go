package integration

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/cli"
)

func TestConsoleStatusAndActions(t *testing.T) {
	root := withTempProject(t, validLinuxGroupedInventory(), validationSummary())
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
	withTempProject(t, validLinuxGroupedInventory(), validationSummary())

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
	withTempProject(t, validLinuxGroupedInventory(), validationSummary())

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

func TestStartAliasOpensConsole(t *testing.T) {
	withTempProject(t, validLinuxGroupedInventory(), validationSummary())

	var out bytes.Buffer
	err := cli.Execute([]string{"start"}, strings.NewReader("q\n"), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("start alias failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Matilda Discovery Readiness") {
		t.Fatalf("start alias should open console:\n%s", out.String())
	}
}
