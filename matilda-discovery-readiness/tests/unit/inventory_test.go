package unit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/inventory"
)

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
    ansible_host: 10.10.0.20
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

func TestValidateRejectsLegacyLinuxGroupedInventory(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `all:
  children:
    public_targets:
      hosts:
        app01:
          ansible_host: 203.0.113.10
          discovery_ip: 10.0.0.10
`)

	_, err := inventory.ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "version: 1") {
		t.Fatalf("expected v1-only validation error, got %v", err)
	}
}

func TestValidateRejectsMalformedV1YAML(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `version: 1
targets:
  app01:
    platform: [linux
`)

	_, err := inventory.ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "parse inventory.yml") {
		t.Fatalf("expected YAML parse error, got %v", err)
	}
}

func TestValidateRejectsUnknownV1Fields(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `version: 1

targets:
  app01:
    platform: linux
    access_path: direct
    ansible_hots: 203.0.113.10
    discovery_ip: 10.0.0.10
    privilege_method: sudo
`)

	_, err := inventory.ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "ansible_hots") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestValidateRejectsPlaceholderInventoryValues(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `version: 1

targets:
  app01:
    platform: linux
    access_path: direct
    ansible_host: <target-public-ip>
    discovery_ip: <target-private-ip>
    privilege_method: sudo
`)

	_, err := inventory.ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "target validation") {
		t.Fatalf("expected placeholder validation error, got %v", err)
	}
}

func TestInventoryV1SchemaMatchesImplementedFields(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test file")
	}
	schemaPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "schemas", "inventory.v1.schema.json")
	content, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}

	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatalf("inventory v1 schema is not valid JSON: %v", err)
	}
	targetSchema := schema["properties"].(map[string]any)["targets"].(map[string]any)["additionalProperties"].(map[string]any)
	required := stringSet(targetSchema["required"].([]any))
	for _, want := range []string{"platform", "privilege_method"} {
		if !required[want] {
			t.Fatalf("inventory v1 target schema should require %s globally", want)
		}
	}
	for _, notGlobal := range []string{"access_path", "ansible_host", "discovery_ip"} {
		if required[notGlobal] {
			t.Fatalf("inventory v1 target schema should not require %s globally because non-Linux scaffold targets may omit it", notGlobal)
		}
	}
	properties := targetSchema["properties"].(map[string]any)
	for _, want := range []string{"public_ip", "private_ip"} {
		if _, ok := properties[want]; !ok {
			t.Fatalf("inventory v1 schema missing implemented optional field %s", want)
		}
	}
	if len(targetSchema["allOf"].([]any)) == 0 {
		t.Fatalf("inventory v1 schema should include platform-specific requirements")
	}
}

func TestPlanLinuxRunnerFromV1Inventory(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `version: 1

targets:
  app01:
    platform: linux
    access_path: direct
    ansible_host: 203.0.113.10
    public_ip: 203.0.113.10
    private_ip: 10.0.0.10
    discovery_ip: 10.0.0.10
    privilege_method: sudo
  app02:
    platform: linux
    access_path: via_probe
    ansible_host: 10.0.1.20
    discovery_ip: 10.0.1.20
    privilege_method: sudo
  win01:
    platform: windows
    access_path: direct
    ansible_host: 10.10.0.20
    privilege_method: winrm
`)

	plan, err := inventory.PlanLinuxRunner(path)
	if err != nil {
		t.Fatalf("PlanLinuxRunner failed: %v", err)
	}
	if plan.Format != "v1" || len(plan.Targets) != 2 || len(plan.SkippedTargets) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	outPath := filepath.Join(t.TempDir(), "inventory.linux.yml")
	if err := inventory.WriteLinuxGroupedInventory(outPath, plan.Targets); err != nil {
		t.Fatalf("WriteLinuxGroupedInventory failed: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{"public_targets:", "private_targets:", "app01:", "public_ip: 203.0.113.10", "private_ip: 10.0.0.10", "app02:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("runner inventory missing %q:\n%s", want, text)
		}
	}
}

func TestWriteLinuxGroupedInventoryAddsSSHConnectionVars(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "inventory.linux.yml")
	targets := []inventory.Target{
		{
			Hostname:    "app01",
			AccessPath:  "direct",
			AnsibleHost: "203.0.113.10",
			DiscoveryIP: "10.0.0.10",
			PublicIP:    "203.0.113.10",
			PrivateIP:   "10.0.0.10",
		},
		{
			Hostname:    "app02",
			AccessPath:  "via_probe",
			AnsibleHost: "10.0.1.20",
			DiscoveryIP: "10.0.1.20",
			PrivateIP:   "10.0.1.20",
		},
	}
	conn := inventory.LinuxConnectionConfig{
		TargetAdminUser:           "opc",
		TargetAdminPrivateKeyFile: "/Users/lly/.ssh/SandboxKey",
		ProbeUser:                 "opc",
		ProbeHost:                 "150.136.237.22",
		ProbeAdminPrivateKeyFile:  "/Users/lly/.ssh/Probe Key.pem",
	}

	if err := inventory.WriteLinuxGroupedInventoryWithConnection(outPath, targets, conn); err != nil {
		t.Fatalf("WriteLinuxGroupedInventoryWithConnection failed: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{
		`ansible_user: "opc"`,
		`ansible_ssh_private_key_file: "/Users/lly/.ssh/SandboxKey"`,
		`ansible_ssh_common_args:`,
		`ProxyCommand=`,
		`150.136.237.22`,
		`/Users/lly/.ssh/Probe Key.pem`,
		`app01:`,
		`app02:`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("runner inventory missing %q:\n%s", want, text)
		}
	}
}

func TestPlanLinuxRunnerRejectsUnsupportedV1Privilege(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `version: 1

targets:
  app01:
    platform: linux
    access_path: direct
    ansible_host: 203.0.113.10
    discovery_ip: 10.0.0.10
    privilege_method: pbrun
`)

	_, err := inventory.PlanLinuxRunner(path)
	if err == nil || !strings.Contains(err.Error(), "privilege_method must be sudo") {
		t.Fatalf("expected privilege method error, got %v", err)
	}
}

func TestRequiresProbeIgnoresNonLinuxV1Targets(t *testing.T) {
	path := writeTempFile(t, "inventory.yml", `version: 1

targets:
  app01:
    platform: linux
    access_path: direct
    ansible_host: 203.0.113.10
    discovery_ip: 10.0.0.10
    privilege_method: sudo
  win01:
    platform: windows
    access_path: via_probe
    ansible_host: 10.10.0.20
    privilege_method: winrm
`)

	needsProbe, err := inventory.RequiresProbe(path)
	if err != nil {
		t.Fatalf("RequiresProbe failed: %v", err)
	}
	if needsProbe {
		t.Fatalf("non-Linux v1 targets should not require Probe inputs for Linux runner actions")
	}
}

func TestReadCSVAndWriteV1Inventory(t *testing.T) {
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
	if err := inventory.WriteV1Inventory(outPath, targets); err != nil {
		t.Fatalf("WriteV1Inventory failed: %v", err)
	}
	result, err := inventory.ValidateFile(outPath)
	if err != nil {
		t.Fatalf("generated v1 inventory should validate: %v", err)
	}
	if result.Format != "v1" || result.TargetCount != 2 {
		t.Fatalf("unexpected generated inventory result: %+v", result)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{"version: 1", "targets:", "app01:", "app02:", "access_path: via_probe"} {
		if !strings.Contains(text, want) {
			t.Fatalf("v1 inventory missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "public_targets:") || strings.Contains(text, "private_targets:") {
		t.Fatalf("user-facing inventory should not expose Ansible runner groups:\n%s", text)
	}
}

func TestReadCSVRejectsMissingRequiredColumnsTogether(t *testing.T) {
	path := writeTempFile(t, "targets.csv", "hostname,discovery_ip\napp01,10.0.0.5\n")

	_, err := inventory.ReadCSV(path)
	if err == nil {
		t.Fatal("expected missing required columns error")
	}
	for _, want := range []string{"CSV missing required columns", "platform", "ansible_host", "access_path", "privilege_method"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing required columns error missing %q: %v", want, err)
		}
	}
}

func TestReadCSVRejectsDuplicateHostnames(t *testing.T) {
	path := writeTempFile(t, "targets.csv", strings.Join([]string{
		"hostname,platform,ansible_host,discovery_ip,access_path,privilege_method",
		"app01,linux,203.0.113.10,10.0.0.10,direct,sudo",
		"app01,linux,203.0.113.11,10.0.0.11,direct,sudo",
		"",
	}, "\n"))

	_, err := inventory.ReadCSV(path)
	if err == nil || !strings.Contains(err.Error(), `duplicate hostname "app01"`) || !strings.Contains(err.Error(), "row 3") {
		t.Fatalf("expected duplicate hostname with row number, got %v", err)
	}
}

func TestReadCSVRejectsDuplicateDiscoveryIPs(t *testing.T) {
	path := writeTempFile(t, "targets.csv", strings.Join([]string{
		"hostname,platform,ansible_host,discovery_ip,access_path,privilege_method",
		"app01,linux,203.0.113.10,10.0.0.10,direct,sudo",
		"app02,linux,203.0.113.11,10.0.0.10,via_probe,sudo",
		"",
	}, "\n"))

	_, err := inventory.ReadCSV(path)
	if err == nil || !strings.Contains(err.Error(), `duplicate discovery_ip "10.0.0.10"`) || !strings.Contains(err.Error(), "row 3") || !strings.Contains(err.Error(), "row 2") {
		t.Fatalf("expected duplicate discovery_ip with current and first row numbers, got %v", err)
	}
}

func TestReadCSVIgnoresBlankRowsAndRejectsPartialRows(t *testing.T) {
	path := writeTempFile(t, "targets.csv", strings.Join([]string{
		"hostname,platform,ansible_host,discovery_ip,access_path,privilege_method",
		"app01,linux,203.0.113.10,10.0.0.10,direct,sudo",
		",,,,,",
		"app02,linux,,10.0.0.20,direct,sudo",
		"",
	}, "\n"))

	_, err := inventory.ReadCSV(path)
	if err == nil || !strings.Contains(err.Error(), "row 4") || !strings.Contains(err.Error(), "ansible_host") {
		t.Fatalf("expected partial row error with row number, got %v", err)
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

func writeTempFile(t *testing.T, name string, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func stringSet(values []any) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		if text, ok := value.(string); ok {
			result[text] = true
		}
	}
	return result
}
