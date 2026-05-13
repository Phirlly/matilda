package unit

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/config"
)

func TestSnapshotJSONMarshalsStatus(t *testing.T) {
	snap := app.Snapshot{
		InventoryPath:   "targets.csv",
		InventoryFormat: "csv",
		InventoryOK:     true,
		TargetCount:     2,
		NextStep:        "Run preflight before setup.",
	}

	content, err := snap.JSON()
	if err != nil {
		t.Fatalf("Snapshot.JSON returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("Snapshot.JSON returned invalid JSON: %v\n%s", err, content)
	}
	if decoded["target_count"] != float64(2) {
		t.Fatalf("target_count = %#v, want 2", decoded["target_count"])
	}
	if decoded["next_step"] != "Run preflight before setup." {
		t.Fatalf("next_step = %#v", decoded["next_step"])
	}
}

func TestSetupFailsBeforeInventoryWhenAnsibleMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader("y\n"), &out, &bytes.Buffer{})
	err := rt.Setup()

	if err == nil || !strings.Contains(err.Error(), "ansible-playbook") {
		t.Fatalf("expected ansible-playbook setup prerequisite error, got %v", err)
	}
	if !strings.Contains(out.String(), "Setup") {
		t.Fatalf("expected setup heading in output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Inventory") {
		t.Fatalf("setup should fail before inventory handling when Ansible is missing:\n%s", out.String())
	}
	if _, statErr := os.Stat(filepath.Join(root, ".matilda")); !os.IsNotExist(statErr) {
		t.Fatalf("setup should not create runtime artifacts before dependency checks, stat err: %v", statErr)
	}
}

func TestSetupCanBeCancelledAfterDependencyChecks(t *testing.T) {
	skipUnixToolStubOnWindows(t)
	root := t.TempDir()
	bin := t.TempDir()
	writeFakePassingTool(t, filepath.Join(bin, "ansible-playbook"))
	writeFakePassingTool(t, filepath.Join(bin, "ansible-doc"))
	prependPath(t, bin)
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeRuntimeEnv(t, root, runtimeEnvFiles{
		TargetAdminPrivateKeyFile: writeUnitRuntimeFile(t, root, "target-admin-key.pem", "target"),
		ProbeAdminPrivateKeyFile:  writeUnitRuntimeFile(t, root, "probe-admin-key.pem", "probe"),
		MatildaPublicKeyFile:      writeUnitRuntimeFile(t, root, "matilda.pub", "public"),
	})
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader("n\n"), &out, &bytes.Buffer{})
	err := rt.Setup()

	if !errors.Is(err, app.ErrCancelled) {
		t.Fatalf("expected setup cancellation, got %v", err)
	}
	for _, want := range []string{"Target Changes", "create or update matilda-svc", "Setup cancelled"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("setup output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunStopsAtPreflightInventoryFailure(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader("y\n"), &out, &bytes.Buffer{})
	err := rt.Run()

	if err == nil || !strings.Contains(err.Error(), "targets.csv") {
		t.Fatalf("expected missing targets.csv error, got %v", err)
	}
	if !strings.Contains(out.String(), "Run") || !strings.Contains(out.String(), "Preflight") {
		t.Fatalf("run output should include run and preflight headings:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Setup") {
		t.Fatalf("run should stop before setup when preflight inventory validation fails:\n%s", out.String())
	}
}

func TestPreflightFailsWhenAnsibleMissingAfterInventoryPreparation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeRuntimeEnv(t, root, runtimeEnvFiles{
		TargetAdminPrivateKeyFile: writeUnitRuntimeFile(t, root, "target-admin-key.pem", "target"),
		ProbeAdminPrivateKeyFile:  writeUnitRuntimeFile(t, root, "probe-admin-key.pem", "probe"),
		MatildaPublicKeyFile:      writeUnitRuntimeFile(t, root, "matilda.pub", "public"),
	})
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader(""), &out, &bytes.Buffer{})
	err := rt.Preflight()

	if err == nil || !strings.Contains(err.Error(), "ansible-playbook is not installed") {
		t.Fatalf("expected missing ansible-playbook error, got %v", err)
	}
	if !strings.Contains(out.String(), "Prepared Linux runner inventory") {
		t.Fatalf("expected inventory preparation before ansible command check:\n%s", out.String())
	}
}

func TestValidateRejectsMissingPromptedRuntimeValue(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader(blankRuntimePromptInput()), &out, &bytes.Buffer{})
	err := rt.Validate()

	if err == nil || !strings.Contains(err.Error(), "Target admin SSH user is required") {
		t.Fatalf("expected missing target admin user error, got %v", err)
	}
	if !strings.Contains(out.String(), "Target admin SSH user") {
		t.Fatalf("expected runtime prompt in output:\n%s", out.String())
	}
}

func TestValidateRejectsPlaceholderRuntimeValue(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, ".env"), strings.Join([]string{
		"TARGET_ADMIN_USER=<target-admin-user>",
		"",
	}, "\n"))

	rt := app.New(root, strings.NewReader(blankRuntimePromptInput()), &bytes.Buffer{}, &bytes.Buffer{})
	err := rt.Validate()

	if err == nil || !strings.Contains(err.Error(), "Target admin SSH user still contains a placeholder") {
		t.Fatalf("expected placeholder runtime value error, got %v", err)
	}
}

func TestValidateRejectsMissingRuntimeKeyFile(t *testing.T) {
	root := t.TempDir()
	missingKey := filepath.Join(root, "missing-target-admin-key.pem")
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeRuntimeEnv(t, root, runtimeEnvFiles{
		TargetAdminPrivateKeyFile: missingKey,
		ProbeAdminPrivateKeyFile:  writeUnitRuntimeFile(t, root, "probe-admin-key.pem", "probe"),
		MatildaPublicKeyFile:      writeUnitRuntimeFile(t, root, "matilda.pub", "public"),
	})

	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	err := rt.Validate()

	if err == nil || !strings.Contains(err.Error(), "Target admin private key path not found: "+missingKey) {
		t.Fatalf("expected missing target key file error, got %v", err)
	}
}

func TestValidateGeneratesReportsWhenAnsibleFails(t *testing.T) {
	skipUnixToolStubOnWindows(t)
	root := t.TempDir()
	bin := t.TempDir()
	writeFakeFailingAnsible(t, filepath.Join(bin, "ansible-playbook"))
	prependPath(t, bin)
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeRuntimeEnv(t, root, runtimeEnvFiles{
		TargetAdminPrivateKeyFile: writeUnitRuntimeFile(t, root, "target-admin-key.pem", "target"),
		ProbeAdminPrivateKeyFile:  writeUnitRuntimeFile(t, root, "probe-admin-key.pem", "probe"),
		MatildaPublicKeyFile:      writeUnitRuntimeFile(t, root, "matilda.pub", "public"),
	})
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader(""), &out, &bytes.Buffer{})
	err := rt.Validate()

	if err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("expected ansible failure, got %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, "reports", "readiness.csv"),
		filepath.Join(root, "reports", "readiness.json"),
		filepath.Join(root, "reports", "readiness.md"),
		filepath.Join(root, "reports", "readiness.html"),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected report file %s to be generated after ansible failure: %v", path, statErr)
		}
	}
	if !strings.Contains(out.String(), "Report Files") {
		t.Fatalf("expected report generation output after ansible failure:\n%s", out.String())
	}
}

func TestRollbackRemoveKeyCanBeCancelled(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeRuntimeEnv(t, root, runtimeEnvFiles{
		TargetAdminPrivateKeyFile: writeUnitRuntimeFile(t, root, "target-admin-key.pem", "target"),
		ProbeAdminPrivateKeyFile:  writeUnitRuntimeFile(t, root, "probe-admin-key.pem", "probe"),
		MatildaPublicKeyFile:      writeUnitRuntimeFile(t, root, "matilda.pub", "public"),
	})
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader("n\n"), &out, &bytes.Buffer{})
	err := rt.Rollback([]string{"--remove-key"})

	if !errors.Is(err, app.ErrCancelled) {
		t.Fatalf("expected rollback cancellation, got %v", err)
	}
	for _, want := range []string{"Rollback", "rollback mode: remove_key", "Rollback cancelled"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("rollback output missing %q:\n%s", want, out.String())
		}
	}
}

func TestReportWritesReadinessArtifacts(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "reports", "validation-summary.txt"), unitValidationSummary())
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader(""), &out, &bytes.Buffer{})
	err := rt.Report()

	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, "reports", "readiness.csv"),
		filepath.Join(root, "reports", "readiness.json"),
		filepath.Join(root, "reports", "readiness.md"),
		filepath.Join(root, "reports", "readiness.html"),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected report artifact %s: %v", path, statErr)
		}
	}
	if !strings.Contains(out.String(), "Readiness reports generated.") {
		t.Fatalf("expected report success output:\n%s", out.String())
	}
}

func TestGenerateWritesPlatformGuidance(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "templates", "powershell", "windows-readiness.ps1.tmpl"), "Write-Output 'ready'\n")
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader(""), &out, &bytes.Buffer{})
	if err := rt.Generate([]string{"windows"}); err != nil {
		t.Fatalf("Generate windows returned error: %v", err)
	}
	if err := rt.Generate([]string{"unix"}); err != nil {
		t.Fatalf("Generate unix returned error: %v", err)
	}

	for _, path := range []string{
		filepath.Join(root, "reports", "guidance", "windows", "windows-readiness.ps1"),
		filepath.Join(root, "reports", "guidance", "windows", "README.md"),
		filepath.Join(root, "reports", "guidance", "unix", "unix-readiness.md"),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected guidance artifact %s: %v", path, statErr)
		}
	}
	if !strings.Contains(out.String(), "Platform guidance generated locally.") {
		t.Fatalf("expected guidance success output:\n%s", out.String())
	}
}

func TestInventoryImportKeepsExistingTargetsCSV(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.csv")
	existing := "hostname,platform\nexisting,linux\n"
	writeUnitFile(t, sourcePath, unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, "targets.csv"), existing)
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader("1\n"), &out, &bytes.Buffer{})
	err := rt.InventoryImport(sourcePath)

	if err != nil {
		t.Fatalf("InventoryImport returned error: %v", err)
	}
	content, readErr := os.ReadFile(filepath.Join(root, "targets.csv"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != existing {
		t.Fatalf("targets.csv was changed after choosing keep:\n%s", content)
	}
	if !strings.Contains(out.String(), "Kept existing targets.csv.") {
		t.Fatalf("expected keep message in output:\n%s", out.String())
	}
}

func TestInitCopyExamplesKeepsExistingFiles(t *testing.T) {
	root := t.TempDir()
	envContent := "TARGET_ADMIN_USER=existing\n"
	targetsContent := unitTargetsCSV()
	writeUnitFile(t, filepath.Join(root, "examples", "env.example"), "TARGET_ADMIN_USER=example\n")
	writeUnitFile(t, filepath.Join(root, "examples", "targets.example.csv"), unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, ".env"), envContent)
	writeUnitFile(t, filepath.Join(root, "targets.csv"), targetsContent)
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader("2\n1\n1\n"), &out, &bytes.Buffer{})
	err := rt.Init()

	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	assertFileContent(t, filepath.Join(root, ".env"), envContent)
	assertFileContent(t, filepath.Join(root, "targets.csv"), targetsContent)
	for _, want := range []string{"Kept existing .env.", "Kept existing targets.csv.", "Init complete."} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("init output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunWorkflowActionValidatedIPsDisplaysTargets(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n10.0.0.11\n")
	var out bytes.Buffer

	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	result := rt.RunWorkflowActionTo("validated-ips", false, &out, &bytes.Buffer{})

	if !result.OK || result.Error != "" {
		t.Fatalf("expected validated-ips action to succeed, got %+v", result)
	}
	for _, want := range []string{"Validated IPs", "10.0.0.10", "10.0.0.11"} {
		if !strings.Contains(result.Output, want) || !strings.Contains(out.String(), want) {
			t.Fatalf("validated-ips output missing %q:\nresult: %s\nout: %s", want, result.Output, out.String())
		}
	}
}

type runtimeEnvFiles struct {
	TargetAdminPrivateKeyFile string
	ProbeAdminPrivateKeyFile  string
	MatildaPublicKeyFile      string
}

func writeRuntimeEnv(t *testing.T, root string, files runtimeEnvFiles) {
	t.Helper()
	writeUnitFile(t, filepath.Join(root, ".env"), strings.Join([]string{
		"TARGET_ADMIN_USER=opc",
		"TARGET_ADMIN_PRIVATE_KEY_FILE=" + files.TargetAdminPrivateKeyFile,
		"MATILDA_PROBE_ANSIBLE_HOST=203.0.113.20",
		"MATILDA_PROBE_USER=opc",
		"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE=" + files.ProbeAdminPrivateKeyFile,
		"MATILDA_PUBLIC_KEY_FILE=" + files.MatildaPublicKeyFile,
		"MATILDA_PROBE_PRIVATE_KEY_ON_PROBE=/home/opc/MatildaProbeKey.pem",
		"",
	}, "\n"))
}

func writeUnitRuntimeFile(t *testing.T, root string, name string, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	writeUnitFile(t, path, content)
	return path
}

func writeFakeFailingAnsible(t *testing.T, path string) {
	t.Helper()
	content := strings.Join([]string{
		"#!/bin/sh",
		"mkdir -p reports",
		"printf '%s\n' 'Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation' 'app01,10.0.0.10,ifconfig,NO,OK,OK,OK,YES,None' > reports/validation-summary.txt",
		"echo validate failed >&2",
		"exit 7",
		"",
	}, "\n")
	writeUnitFile(t, path, content)
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
}

func blankRuntimePromptInput() string {
	return strings.Repeat("\n", len(config.RequiredKeys))
}

func prependPath(t *testing.T, dir string) {
	t.Helper()
	current := os.Getenv("PATH")
	if current == "" {
		t.Setenv("PATH", dir)
		return
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+current)
}

func skipUnixToolStubOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell tool stubs are not portable to Windows")
	}
}

func writeFakePassingTool(t *testing.T, path string) {
	t.Helper()
	content := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\n' 'fake tool version'",
		"",
	}, "\n")
	writeUnitFile(t, path, content)
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
}
