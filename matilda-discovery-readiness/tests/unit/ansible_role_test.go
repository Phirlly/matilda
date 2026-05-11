package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxRoleDefaultsDefineReadinessVariables(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "ansible", "roles", "matilda_linux_target", "defaults", "main.yml"))
	if err != nil {
		t.Fatalf("expected Linux role defaults: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"matilda_service_user: matilda-svc",
		"matilda_service_home: /home/matilda-svc",
		"matilda_sudoers_file: /etc/sudoers.d/matilda-discovery",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Linux role defaults missing %q:\n%s", want, text)
		}
	}
}
