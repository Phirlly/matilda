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
	withTempProject(t, validTargetsCSV(), "")

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
	for _, want := range []string{`"last_action": "inventory-validate"`, `"last_status": "completed"`, `"inventory": "targets.csv"`} {
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

func TestCLIInventoryValidateFailureShowsFixTargetsCSVNextStep(t *testing.T) {
	withTempProject(t, "hostname,platform,ansible_host,discovery_ip,access_path,privilege_method\napp01,linux,,10.0.0.10,direct,sudo\n", "")

	var out bytes.Buffer
	err := cli.Execute([]string{"inventory", "validate"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("inventory validate should fail for invalid targets.csv:\n%s", out.String())
	}
	for _, want := range []string{
		"Inventory Validate",
		"FAIL  targets.csv",
		"row 2 missing required values: ansible_host",
		"Fix targets.csv, then run ./matilda-prep inventory validate again.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("inventory validation failure output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIInventoryValidateMissingTargetsCSVShowsInitNextStep(t *testing.T) {
	withTempProject(t, "", "")

	var out bytes.Buffer
	err := cli.Execute([]string{"inventory", "validate"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("inventory validate should fail when targets.csv is missing:\n%s", out.String())
	}
	for _, want := range []string{
		"Inventory Validate",
		"FAIL  targets.csv",
		"missing:",
		"Run ./matilda-prep init to create targets.csv",
		"copy examples/targets.example.csv",
		"target values.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing inventory output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIInventoryImportWritesTargetsCSVAndGeneratedInventory(t *testing.T) {
	root := withTempProject(t, "", "")
	csvPath := filepath.Join(root, "import-source.csv")
	writeFile(t, csvPath, "hostname,platform,os_family,ansible_host,discovery_ip,access_path,privilege_method,private_ip,public_ip,cloud_provider\napp01,linux,oracle_linux,203.0.113.10,10.0.0.10,direct,sudo,10.0.0.10,203.0.113.10,oci\n")

	var out bytes.Buffer
	err := cli.Execute([]string{"inventory", "import", csvPath}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("inventory import failed: %v\n%s", err, out.String())
	}
	targetCSV, err := os.ReadFile(filepath.Join(root, "targets.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(targetCSV), "app01,linux,oracle_linux") {
		t.Fatalf("import should create user-facing targets.csv:\n%s", string(targetCSV))
	}
	if _, err := os.Stat(filepath.Join(root, "inventory.yml")); !os.IsNotExist(err) {
		t.Fatalf("import should not write root inventory.yml, got err=%v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, ".matilda", "generated", "inventory.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{"version: 1", "targets:", "app01:", "platform: linux"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated inventory missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "public_targets:") || strings.Contains(text, "private_targets:") {
		t.Fatalf("generated inventory should not expose Ansible runner groups:\n%s", text)
	}
}

func TestCLIInventoryImportFailureShowsSourceCSVGuidance(t *testing.T) {
	root := withTempProject(t, validTargetsCSV(), "")
	csvArg := "import-source.csv"
	csvPath := filepath.Join(root, csvArg)
	writeFile(t, csvPath, "hostname,platform,ansible_host,discovery_ip,access_path,privilege_method\napp02,linux,,10.0.0.20,direct,sudo\n")

	var out bytes.Buffer
	err := cli.Execute([]string{"inventory", "import", csvArg}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("inventory import should fail for invalid source CSV:\n%s", out.String())
	}
	for _, want := range []string{
		"Inventory Import",
		"FAIL  source CSV",
		"row 2 missing required values: ansible_host",
		"Fix the source CSV, then run ./matilda-prep inventory import " + csvArg + " again.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("inventory import failure output missing %q:\n%s", want, out.String())
		}
	}
	targetCSV, err := os.ReadFile(filepath.Join(root, "targets.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(targetCSV), "app01,linux") || strings.Contains(string(targetCSV), "app02,linux") {
		t.Fatalf("invalid import should not replace existing targets.csv:\n%s", string(targetCSV))
	}
}

func TestCLIInventoryHelpShowsOptionalColumns(t *testing.T) {
	withTempProject(t, "", "")

	var out bytes.Buffer
	err := cli.Execute([]string{"inventory", "help"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("inventory help failed: %v", err)
	}
	required := sectionBetween(out.String(), "Required CSV Columns", "Optional CSV Columns")
	optional := sectionBetween(out.String(), "Optional CSV Columns", "")
	if strings.Contains(required, "os_family") {
		t.Fatalf("inventory help should not list os_family as required:\n%s", out.String())
	}
	if !strings.Contains(optional, "os_family") {
		t.Fatalf("inventory help should list os_family as optional:\n%s", out.String())
	}
	for _, want := range []string{"admin_user", "admin_private_key_file"} {
		if strings.Contains(required, want) {
			t.Fatalf("inventory help should not list %s as required:\n%s", want, out.String())
		}
		if !strings.Contains(optional, want) {
			t.Fatalf("inventory help should list %s as optional:\n%s", want, out.String())
		}
	}
}

func TestCLIInventoryMigrateCommandRemoved(t *testing.T) {
	withTempProject(t, validTargetsCSV(), "")

	err := cli.Execute([]string{"inventory", "migrate"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `unknown inventory command "migrate"`) {
		t.Fatalf("expected migrate command to be removed, got %v", err)
	}
}

func TestCLIInitGuidedWritesTargetsCSV(t *testing.T) {
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
		"",
	}, "\n")

	var out bytes.Buffer
	err := cli.Execute([]string{"init"}, strings.NewReader(input), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "targets.csv"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{"hostname,platform", "app01,linux", "access_path", "privilege_method"} {
		if !strings.Contains(text, want) {
			t.Fatalf("guided targets CSV missing %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "inventory.yml")); !os.IsNotExist(err) {
		t.Fatalf("guided init should not write root inventory.yml, got err=%v", err)
	}
}

func TestCLIDoctorDoesNotRequireGoAfterBinaryStarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell scripts to stub local commands")
	}

	root := withTempProject(t, validTargetsCSV(), "")
	writeFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=opc\n")
	writeFile(t, filepath.Join(root, "examples", "targets.example.csv"), validTargetsCSV())
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
	root := withTempProject(t, validTargetsCSV(), "")
	writeFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=opc\n")
	writeFile(t, filepath.Join(root, "examples", "targets.example.csv"), validTargetsCSV())
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

	root := withTempProject(t, validTargetsCSV(), "")
	writeFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=opc\n")
	writeFile(t, filepath.Join(root, "examples", "targets.example.csv"), validTargetsCSV())
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

	root := withTempProject(t, validTargetsCSV(), "")
	writeFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=opc\n")
	writeFile(t, filepath.Join(root, "examples", "targets.example.csv"), validTargetsCSV())
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
		if args[0] == "inventory" && strings.Contains(out.String(), "...") {
			t.Fatalf("inventory help should not truncate command examples:\n%s", out.String())
		}
	}
}

func TestCLIGenerateUnknownTargetDoesNotPrintEmptyArtifactsSection(t *testing.T) {
	withTempProject(t, validTargetsCSV(), "")

	var out bytes.Buffer
	err := cli.Execute([]string{"generate", "bad-target"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `unsupported generate target "bad-target"`) {
		t.Fatalf("expected unsupported generate target error, got err=%v out=%q", err, out.String())
	}
	if strings.Contains(out.String(), "Artifacts") {
		t.Fatalf("invalid generate target should not print an empty Artifacts section:\n%s", out.String())
	}
}

func TestCLIReportWritesExpectedFormats(t *testing.T) {
	root := withTempProject(t, validTargetsCSV(), validationSummary())

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

func TestCLILiveWorkflowPlansCSVInventoryForLinuxRunner(t *testing.T) {
	root := withTempProject(t, mixedCredentialTargetsCSV(), "")
	writeRemoteEnvFixture(t, root)

	var out bytes.Buffer
	err := cli.Execute([]string{"preflight"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "Linux direct/via-Probe groups") {
		t.Fatalf("expected CSV-generated runner planning to pass the old guard, got %v\n%s", err, out.String())
	}
	generated := filepath.Join(root, ".matilda", "runner", "inventory.linux.yml")
	content, readErr := os.ReadFile(generated)
	if readErr != nil {
		t.Fatalf("expected generated runner inventory: %v\n%s", readErr, out.String())
	}
	if !strings.Contains(string(content), "public_targets:") || !strings.Contains(string(content), "app01:") {
		t.Fatalf("generated runner inventory missing Linux target:\n%s", string(content))
	}
	for _, want := range []string{`ansible_user: "opc"`, `ansible_ssh_private_key_file:`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("generated runner inventory missing connection var %q:\n%s", want, string(content))
		}
	}
	for _, want := range []string{
		`app02:`,
		`ansible_user: "oracle"`,
		`ansible_ssh_private_key_file: "/keys/app02.pem"`,
		`app03:`,
		`ansible_user: "ubuntu"`,
		`ansible_ssh_private_key_file: "/keys/app03.pem"`,
		`ProxyCommand=`,
		`probe.key`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("generated mixed-credential runner inventory missing %q:\n%s", want, string(content))
		}
	}
}

func TestCLIGenerateWindowsPackage(t *testing.T) {
	root := withTempProject(t, validTargetsCSV(), "")
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
	root := withTempProject(t, validTargetsCSV(), "")

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
	withTempProject(t, validTargetsCSV(), "")

	err := cli.Execute([]string{"rollback"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires one mode") {
		t.Fatalf("expected rollback mode error, got %v", err)
	}
}

func withTempProject(t *testing.T, targetCSV string, summary string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# test project\n")
	writeFile(t, filepath.Join(root, "ansible", "ansible.cfg"), "[defaults]\ninventory = ../.matilda/runner/inventory.linux.yml\nroles_path = roles\n")
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
	writeFile(t, filepath.Join(root, "examples", "targets.example.csv"), validTargetsCSV())
	if targetCSV != "" {
		writeFile(t, filepath.Join(root, "targets.csv"), targetCSV)
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

func writeRemoteEnvFixture(t *testing.T, root string) {
	t.Helper()
	targetKey := filepath.Join(root, "keys", "target.key")
	probeKey := filepath.Join(root, "keys", "probe.key")
	publicKey := filepath.Join(root, "keys", "matilda.pub")
	writeFile(t, targetKey, "target key\n")
	writeFile(t, probeKey, "probe key\n")
	writeFile(t, publicKey, "public key\n")
	writeFile(t, filepath.Join(root, ".env"), strings.Join([]string{
		"TARGET_ADMIN_USER=opc",
		"TARGET_ADMIN_PRIVATE_KEY_FILE=" + targetKey,
		"MATILDA_PROBE_ANSIBLE_HOST=192.0.2.50",
		"MATILDA_PROBE_USER=opc",
		"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE=" + probeKey,
		"MATILDA_PUBLIC_KEY_FILE=" + publicKey,
		"MATILDA_PROBE_PRIVATE_KEY_ON_PROBE=/home/opc/.ssh/MatildaProbeKey.pem",
		"",
	}, "\n"))
}

func rootFromCwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

func validTargetsCSV() string {
	return "hostname,platform,os_family,ansible_host,discovery_ip,access_path,privilege_method,private_ip,public_ip,cloud_provider\napp01,linux,oracle_linux,203.0.113.10,10.0.0.10,direct,sudo,10.0.0.10,203.0.113.10,oci\n"
}

func mixedCredentialTargetsCSV() string {
	return strings.Join([]string{
		"hostname,platform,os_family,ansible_host,discovery_ip,access_path,privilege_method,private_ip,public_ip,cloud_provider,admin_user,admin_private_key_file",
		"app01,linux,oracle_linux,203.0.113.10,10.0.0.10,direct,sudo,10.0.0.10,203.0.113.10,oci,,",
		"app02,linux,oracle_linux,203.0.113.20,10.0.0.20,direct,sudo,10.0.0.20,203.0.113.20,oci,oracle,/keys/app02.pem",
		"app03,linux,oracle_linux,10.0.1.30,10.0.1.30,via_probe,sudo,10.0.1.30,,oci,ubuntu,/keys/app03.pem",
		"",
	}, "\n")
}

func validationSummary() string {
	return "Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation\napp01,10.0.0.10,ifconfig,NO,OK,OK,OK,YES,None\n"
}

func sectionBetween(text string, start string, end string) string {
	startIndex := strings.Index(text, start)
	if startIndex < 0 {
		return ""
	}
	section := text[startIndex+len(start):]
	if end == "" {
		return section
	}
	if endIndex := strings.Index(section, end); endIndex >= 0 {
		return section[:endIndex]
	}
	return section
}
