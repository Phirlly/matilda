package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/config"
)

func TestLoadEnvAndExtraVars(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := strings.Join([]string{
		"TARGET_ADMIN_USER=opc",
		"TARGET_ADMIN_PRIVATE_KEY_FILE='/tmp/target key.pem'",
		"MATILDA_PROBE_ANSIBLE_HOST=probe.example.com",
		"MATILDA_PROBE_USER=opc",
		"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE=/tmp/probe.pem",
		"MATILDA_PUBLIC_KEY_FILE=/tmp/matilda.pub",
		"MATILDA_PROBE_PRIVATE_KEY_ON_PROBE=/home/opc/.ssh/MatildaProbeKey.pem",
		"",
	}, "\n")
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	values, err := config.LoadEnv(envPath)
	if err != nil {
		t.Fatalf("LoadEnv failed: %v", err)
	}
	if values["TARGET_ADMIN_PRIVATE_KEY_FILE"] != "/tmp/target key.pem" {
		t.Fatalf("quoted value was not parsed correctly: %q", values["TARGET_ADMIN_PRIVATE_KEY_FILE"])
	}

	extra := config.ExtraVars(values)
	joined := strings.Join(extra, "\n")
	for _, want := range []string{
		"target_admin_user=opc",
		"matilda_probe_ansible_host=probe.example.com",
		"matilda_probe_private_key_on_probe=/home/opc/.ssh/MatildaProbeKey.pem",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("extra vars missing %q:\n%s", want, joined)
		}
	}
}

func TestShellQuote(t *testing.T) {
	got := config.ShellQuote("/tmp/key with space.pem")
	if got != "'/tmp/key with space.pem'" {
		t.Fatalf("unexpected quoted value: %s", got)
	}
}
