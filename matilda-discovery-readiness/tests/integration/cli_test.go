package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
		"Start Here",
		"Local Checks",
		"Linux Readiness",
		"Reports And Guidance",
		"inventory validate",
		"console",
		"status",
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
	for _, legacy := range []string{"Usage:", "Commands:"} {
		if strings.Contains(out.String(), legacy) {
			t.Fatalf("help output should use console sections instead of legacy %q:\n%s", legacy, out.String())
		}
	}
	legacyAlias := "t" + "ui"
	if strings.Contains(strings.ToLower(out.String()), legacyAlias) {
		t.Fatalf("help output should not expose legacy terminal aliases:\n%s", out.String())
	}
	if strings.Contains(out.String(), "inventory migrate") {
		t.Fatalf("help output should not expose migration for a v1-default inventory:\n%s", out.String())
	}
}

func TestCLILegacyTerminalAliasRemoved(t *testing.T) {
	withTempProject(t, "", "")

	var out bytes.Buffer
	var errOut bytes.Buffer
	legacyAlias := "t" + "ui"
	err := cli.Execute([]string{legacyAlias}, strings.NewReader(""), &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), `unknown command "`+legacyAlias+`"`) {
		t.Fatalf("expected legacy terminal alias to be removed, got err=%v out=%q errOut=%q", err, out.String(), errOut.String())
	}
}

func TestCLIInventoryValidateUsesProjectInventory(t *testing.T) {
	withTempProject(t, validV1Inventory(), "")

	var out bytes.Buffer
	err := cli.Execute([]string{"inventory", "validate"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("inventory validate failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"Matilda Discovery Readiness",
		"Inventory Validate",
		"read-only inventory checks",
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
	for _, legacy := range []string{"Command  ", "Scope    "} {
		if strings.Contains(out.String(), legacy) {
			t.Fatalf("inventory output should not use legacy command headers %q:\n%s", legacy, out.String())
		}
	}
	statePath := filepath.Join(rootFromCwd(t), ".matilda", "state.json")
	stateContent, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("expected state file to be written: %v", err)
	}
	for _, want := range []string{`"last_action": "inventory-validate"`, `"last_status": "completed"`, `"inventory": "inventory.yml"`} {
		if !strings.Contains(string(stateContent), want) {
			t.Fatalf("state file missing %q:\n%s", want, string(stateContent))
		}
	}
	if strings.Contains(string(stateContent), "PRIVATE_KEY") {
		t.Fatalf("state file should not contain secret-like environment keys:\n%s", string(stateContent))
	}
	runsDir := filepath.Join(rootFromCwd(t), ".matilda", "runs")
	runEntries, err := os.ReadDir(runsDir)
	if err != nil {
		t.Fatalf("expected run history directory: %v", err)
	}
	if len(runEntries) == 0 {
		t.Fatalf("expected inventory validate to create a run history record")
	}
}

func TestCLIInventoryImportWritesV1Inventory(t *testing.T) {
	root := withTempProject(t, "", "")
	csvPath := filepath.Join(root, "targets.csv")
	writeFile(t, csvPath, "hostname,platform,os_family,ansible_host,discovery_ip,access_path,privilege_method,private_ip,public_ip,cloud_provider\napp01,linux,oracle_linux,203.0.113.10,10.0.0.10,direct,sudo,10.0.0.10,203.0.113.10,oci\n")

	var out bytes.Buffer
	err := cli.Execute([]string{"inventory", "import", csvPath}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("inventory import failed: %v\n%s", err, out.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "inventory.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{"version: 1", "targets:", "app01:", "platform: linux"} {
		if !strings.Contains(text, want) {
			t.Fatalf("imported inventory missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "public_targets:") || strings.Contains(text, "private_targets:") {
		t.Fatalf("imported inventory should not expose Ansible runner groups:\n%s", text)
	}
}

func TestCLIInventoryMigrateCommandRemoved(t *testing.T) {
	withTempProject(t, validV1Inventory(), "")

	err := cli.Execute([]string{"inventory", "migrate"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `unknown inventory command "migrate"`) {
		t.Fatalf("expected migrate command to be removed, got %v", err)
	}
}

func TestCLIInitGuidedWritesV1Inventory(t *testing.T) {
	root := withTempProject(t, "", "")
	input := strings.Join([]string{
		"1",
		"opc",
		"/private/tmp/target-admin.key",
		"203.0.113.100",
		"opc",
		"/private/tmp/probe-admin.key",
		"/private/tmp/matilda.pub",
		"/home/opc/.ssh/MatildaProbeKey.pem",
		"1",
		"app01",
		"direct",
		"203.0.113.10",
		"10.0.0.10",
		"10.0.0.10",
		"",
	}, "\n")

	var out bytes.Buffer
	err := cli.Execute([]string{"init"}, strings.NewReader(input), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "inventory.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{"version: 1", "targets:", "app01:", "access_path: direct", "privilege_method: sudo"} {
		if !strings.Contains(text, want) {
			t.Fatalf("guided inventory missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "public_targets:") || strings.Contains(text, "private_targets:") {
		t.Fatalf("guided inventory should not expose Ansible runner groups:\n%s", text)
	}
}

func TestCLIDoctorDoesNotRequireGoAfterBinaryStarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell scripts to stub local commands")
	}

	root := withTempProject(t, validV1Inventory(), "")
	writeFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=opc\n")
	writeFile(t, filepath.Join(root, "examples", "inventory.example.yml"), validV1Inventory())
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0755); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	for _, name := range []string{"ansible-playbook", "ansible-doc"} {
		writeFile(t, filepath.Join(binDir, name), "#!/bin/sh\necho "+name+" test\n")
		if err := os.Chmod(filepath.Join(binDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	err := cli.Execute([]string{"doctor"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("doctor should not require go once matilda-prep is already running: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"SKIP  go",
		"not required when using a packaged or prebuilt matilda-prep binary",
		"PASS  ansible-playbook",
		"PASS  ansible-doc",
		"Local environment looks ready",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIDoctorReportsMissingAnsibleClearly(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), "")
	writeFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=opc\n")
	writeFile(t, filepath.Join(root, "examples", "inventory.example.yml"), validV1Inventory())
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())

	var out bytes.Buffer
	err := cli.Execute([]string{"doctor"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("doctor should fail when Ansible is missing:\n%s", out.String())
	}
	for _, want := range []string{
		"FAIL  ansible-playbook",
		"ansible-playbook is not installed or not on PATH",
		"install Ansible and rerun ./matilda-prep doctor",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIDoctorReportsMissingToolkitAssetsClearly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell scripts to stub local commands")
	}

	root := withTempProject(t, validV1Inventory(), "")
	writeFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=opc\n")
	writeFile(t, filepath.Join(root, "examples", "inventory.example.yml"), validV1Inventory())
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "ansible", "playbooks", "linux", "validate.yml")); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	for _, name := range []string{"ansible-playbook", "ansible-doc"} {
		writeFile(t, filepath.Join(binDir, name), "#!/bin/sh\necho "+name+" test\n")
		if err := os.Chmod(filepath.Join(binDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	err := cli.Execute([]string{"doctor"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("doctor should fail when toolkit assets are missing:\n%s", out.String())
	}
	for _, want := range []string{
		"FAIL  linux validate playbook",
		"source checkout",
		"extracted release package root",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIDoctorFailsWhenGoExistsButIsBroken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell scripts to stub local commands")
	}

	root := withTempProject(t, validV1Inventory(), "")
	writeFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=opc\n")
	writeFile(t, filepath.Join(root, "examples", "inventory.example.yml"), validV1Inventory())
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0755); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	for _, name := range []string{"ansible-playbook", "ansible-doc"} {
		writeFile(t, filepath.Join(binDir, name), "#!/bin/sh\necho "+name+" test\n")
		if err := os.Chmod(filepath.Join(binDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(binDir, "go"), "#!/bin/sh\necho go is broken >&2\nexit 2\n")
	if err := os.Chmod(filepath.Join(binDir, "go"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	err := cli.Execute([]string{"doctor"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("doctor should fail when go exists but is broken:\n%s", out.String())
	}
	for _, want := range []string{
		"FAIL  go",
		"go is broken",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIHelpScreensUseConsoleSections(t *testing.T) {
	withTempProject(t, "", "")

	cases := [][]string{
		{"inventory", "help"},
		{"generate", "help"},
		{"rollback", "help"},
	}
	for _, args := range cases {
		var out bytes.Buffer
		err := cli.Execute(args, strings.NewReader(""), &out, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
		for _, want := range []string{"Matilda Discovery Readiness"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("%v output missing %q:\n%s", args, want, out.String())
			}
		}
		for _, legacy := range []string{"Usage:", "Commands:"} {
			if strings.Contains(out.String(), legacy) {
				t.Fatalf("%v output should not use legacy %q:\n%s", args, legacy, out.String())
			}
		}
		if args[0] == "inventory" && strings.Contains(out.String(), "inventory migrate") {
			t.Fatalf("inventory help should not expose migration for v1-default inventory:\n%s", out.String())
		}
	}
}

func TestCLIReportWritesExpectedFormats(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())

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

func TestCLILiveWorkflowPlansV1InventoryForLinuxRunner(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), "")

	var out bytes.Buffer
	err := cli.Execute([]string{"preflight"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "Linux direct/via-Probe groups") {
		t.Fatalf("expected v1 runner planning to pass the old guard, got %v\n%s", err, out.String())
	}
	generated := filepath.Join(root, ".matilda", "runner", "inventory.linux.yml")
	content, readErr := os.ReadFile(generated)
	if readErr != nil {
		t.Fatalf("expected generated runner inventory: %v\n%s", readErr, out.String())
	}
	if !strings.Contains(string(content), "public_targets:") || !strings.Contains(string(content), "app01:") {
		t.Fatalf("generated runner inventory missing Linux target:\n%s", string(content))
	}
}

func TestCLIGenerateWindowsPackage(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), "")
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
	root := withTempProject(t, validV1Inventory(), "")

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
	withTempProject(t, validV1Inventory(), "")

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
	for _, path := range []string{
		filepath.Join(root, "ansible", "playbooks", "linux", "preflight.yml"),
		filepath.Join(root, "ansible", "playbooks", "linux", "setup.yml"),
		filepath.Join(root, "ansible", "playbooks", "linux", "validate.yml"),
		filepath.Join(root, "ansible", "playbooks", "linux", "rollback.yml"),
		filepath.Join(root, "templates", "sudoers", "linux-full-documented.j2"),
		filepath.Join(root, "templates", "powershell", "windows-readiness.ps1.tmpl"),
		filepath.Join(root, "schemas", "inventory.v1.schema.json"),
	} {
		writeFile(t, path, "# test asset\n")
	}
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

func rootFromCwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
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
