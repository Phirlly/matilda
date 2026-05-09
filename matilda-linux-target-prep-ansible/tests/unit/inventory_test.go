package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/inventory"
)

func TestValidateLinuxGroupedInventory(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `all:
  children:
    public_targets:
      hosts:
        app01:
          ansible_host: 203.0.113.10
          private_ip: 10.0.0.10
          discovery_ip: 10.0.0.10
    private_targets:
      hosts: {}
`)

	result, err := inventory.ValidateFile(path)
	if err != nil {
		t.Fatalf("expected valid inventory: %v", err)
	}
	if result.TargetCount != 1 {
		t.Fatalf("expected 1 target, got %d", result.TargetCount)
	}
	if result.Format != "linux-groups" {
		t.Fatalf("expected Linux grouped format, got %q", result.Format)
	}
}

func TestValidateRejectsLinuxGroupedInventoryMissingDiscoveryIP(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `all:
  children:
    public_targets:
      hosts:
        app01:
          ansible_host: 203.0.113.10
          discovery_ip: 10.0.0.10
        app02:
          ansible_host: 203.0.113.20
`)

	_, err := inventory.ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "discovery_ip") {
		t.Fatalf("expected discovery_ip validation error, got %v", err)
	}
}

func TestValidateRejectsPlaceholderInventoryValues(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `all:
  children:
    public_targets:
      hosts:
        app01:
          ansible_host: <target-public-ip>
          discovery_ip: <target-private-ip>
`)

	_, err := inventory.ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "target validation") {
		t.Fatalf("expected placeholder validation error, got %v", err)
	}
}

func TestValidateNormalizedV1Inventory(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `version: 1

targets:
  app01:
    platform: linux
    os_family: oracle_linux
    access_path: direct
    ansible_host: 203.0.113.10
    discovery_ip: 10.0.0.10
    privilege_method: sudo
    configure_mode: remote
  win01:
    platform: windows
    access_path: direct
    ansible_host: 10.0.0.20
    privilege_method: winrm
`)

	result, err := inventory.ValidateFile(path)
	if err != nil {
		t.Fatalf("expected valid normalized inventory: %v", err)
	}
	if result.TargetCount != 2 {
		t.Fatalf("expected 2 targets, got %d", result.TargetCount)
	}
	if result.Format != "v1" {
		t.Fatalf("expected v1 format, got %q", result.Format)
	}
}

func TestReadCSVAndWriteLinuxGroupedInventory(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "targets.csv")
	content := "hostname,platform,os_family,ansible_host,discovery_ip,access_path,privilege_method,private_ip,public_ip,cloud_provider\napp01,linux,oracle_linux,203.0.113.10,10.0.0.10,direct,sudo,10.0.0.10,203.0.113.10,oci\napp02,linux,oracle_linux,10.0.1.20,10.0.1.20,via_probe,sudo,10.0.1.20,,oci\n"
	if err := os.WriteFile(csvPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	targets, err := inventory.ReadCSV(csvPath)
	if err != nil {
		t.Fatalf("ReadCSV failed: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	outPath := filepath.Join(dir, "inventory.yml")
	if err := inventory.WriteLinuxGroupedInventory(outPath, targets); err != nil {
		t.Fatalf("WriteLinuxGroupedInventory failed: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{"public_targets:", "private_targets:", "app01:", "app02:", "discovery_ip: 10.0.1.20"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Linux grouped inventory missing %q:\n%s", want, text)
		}
	}
}

func TestReadCSVRejectsNonLinuxForCurrentImporter(t *testing.T) {
	path := writeTempFile(t, "targets.csv", "hostname,platform,ansible_host,discovery_ip,access_path,privilege_method\nwin01,windows,10.0.0.5,10.0.0.5,via_probe,winrm\n")

	_, err := inventory.ReadCSV(path)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}

func TestReadCSVRejectsMissingRequiredValues(t *testing.T) {
	path := writeTempFile(t, "targets.csv", "hostname,platform,ansible_host,discovery_ip,access_path,privilege_method\napp01,linux,,10.0.0.5,direct,sudo\n")

	_, err := inventory.ReadCSV(path)
	if err == nil || !strings.Contains(err.Error(), "ansible_host") {
		t.Fatalf("expected ansible_host value error, got %v", err)
	}
}

func TestMigrateLinuxGroupedToV1(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "inventory.yml")
	output := filepath.Join(dir, "inventory.v1.yml")
	content := `all:
  children:
    public_targets:
      hosts:
        app01:
          ansible_host: 203.0.113.10
          discovery_ip: 10.0.0.10
    private_targets:
      hosts:
        app02:
          ansible_host: 10.0.1.20
          discovery_ip: 10.0.1.20
`
	if err := os.WriteFile(input, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := inventory.MigrateLinuxGroupedToV1(input, output); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{"version: 1", "app01:", "access_path: direct", "app02:", "access_path: via_probe"} {
		if !strings.Contains(text, want) {
			t.Fatalf("migrated output missing %q:\n%s", want, text)
		}
	}
}

func writeTempFile(t *testing.T, name string, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
