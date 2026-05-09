package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/cli"
)

func TestCLIHelpIncludesUserEntryPoints(t *testing.T) {
	withTempProject(t, "", "")

	var out bytes.Buffer
	err := cli.Execute([]string{"help"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	for _, want := range []string{
		"inventory validate",
		"generate TARGET",
		"Windows readiness package",
		"UNIX admin instructions",
		"rollback MODE",
		"ui",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIInventoryValidateUsesProjectInventory(t *testing.T) {
	withTempProject(t, validLinuxGroupedInventory(), "")

	var out bytes.Buffer
	err := cli.Execute([]string{"inventory", "validate"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("inventory validate failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"Command  Inventory Validate",
		"Scope    read-only inventory checks",
		"Inventory",
		"Inventory valid: 1 target(s) detected",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("inventory output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "----") || strings.Contains(out.String(), "====") {
		t.Fatalf("inventory output should use clean sections instead of separator lines:\n%s", out.String())
	}
}

func TestCLIReportWritesExpectedFormats(t *testing.T) {
	root := withTempProject(t, validLinuxGroupedInventory(), validationSummary())

	var out bytes.Buffer
	err := cli.Execute([]string{"report"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("report failed: %v\n%s", err, out.String())
	}
	for _, name := range []string{"readiness.csv", "readiness.json", "readiness.md", "readiness.html"} {
		if _, err := os.Stat(filepath.Join(root, "reports", name)); err != nil {
			t.Fatalf("expected %s to be written: %v", name, err)
		}
	}
}

func TestCLILiveWorkflowRejectsV1InventoryUntilRunnerSupportsIt(t *testing.T) {
	withTempProject(t, validV1Inventory(), "")

	var out bytes.Buffer
	err := cli.Execute([]string{"preflight"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "Linux direct/via-Probe groups") {
		t.Fatalf("expected current Linux inventory guard, got %v\n%s", err, out.String())
	}
}

func TestCLIGenerateWindowsPackage(t *testing.T) {
	root := withTempProject(t, validLinuxGroupedInventory(), "")
	templatePath := filepath.Join(root, "templates", "powershell", "windows-readiness.ps1.tmpl")
	if err := os.MkdirAll(filepath.Dir(templatePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templatePath, []byte("Write-Host \"Matilda Windows readiness\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := cli.Execute([]string{"generate", "windows"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("generate windows failed: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "guidance", "windows", "windows-readiness.ps1")); err != nil {
		t.Fatalf("expected generated PowerShell package: %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(root, "reports", "guidance", "windows", "README.md"))
	if err != nil {
		t.Fatalf("expected generated Windows README: %v", err)
	}
	if !strings.Contains(string(readme), "Windows Readiness Package") {
		t.Fatalf("Windows README should use readiness package wording:\n%s", string(readme))
	}
}

func TestCLIGenerateUnixAdminInstructions(t *testing.T) {
	root := withTempProject(t, validLinuxGroupedInventory(), "")

	var out bytes.Buffer
	err := cli.Execute([]string{"generate", "unix"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("generate unix failed: %v\n%s", err, out.String())
	}
	path := filepath.Join(root, "reports", "guidance", "unix", "unix-readiness.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected generated UNIX admin instructions: %v", err)
	}
	if !strings.Contains(string(content), "UNIX Admin Instructions") {
		t.Fatalf("UNIX instructions should use admin instructions wording:\n%s", string(content))
	}
}

func TestCLIRollbackRequiresExplicitMode(t *testing.T) {
	withTempProject(t, validLinuxGroupedInventory(), "")

	err := cli.Execute([]string{"rollback"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires one mode") {
		t.Fatalf("expected rollback mode error, got %v", err)
	}
}

func withTempProject(t *testing.T, inventory string, summary string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# test project\n")
	writeFile(t, filepath.Join(root, "ansible", "ansible.cfg"), "[defaults]\ninventory = ../inventory.yml\nroles_path = roles\n")
	if inventory != "" {
		writeFile(t, filepath.Join(root, "inventory.yml"), inventory)
	}
	if summary != "" {
		writeFile(t, filepath.Join(root, "reports", "validation-summary.txt"), summary)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})
	return root
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func validLinuxGroupedInventory() string {
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

func validV1Inventory() string {
	return `version: 1

targets:
  app01:
    platform: linux
    access_path: direct
    ansible_host: 203.0.113.10
    discovery_ip: 10.0.0.10
    privilege_method: sudo
`
}

func validationSummary() string {
	return "Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation\napp01,10.0.0.10,ifconfig,NO,OK,OK,OK,YES,None\n"
}
