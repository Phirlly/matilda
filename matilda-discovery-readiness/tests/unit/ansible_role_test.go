package unit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLinuxRoleDefaultsDefineReadinessVariables(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "ansible", "roles", "matilda_linux_target", "defaults", "main.yml"))
	if err != nil {
		t.Fatalf("expected Linux role defaults: %v", err)
	}
	var defaults map[string]any
	if err := yaml.Unmarshal(content, &defaults); err != nil {
		t.Fatalf("expected parseable Linux role defaults: %v", err)
	}
	for key, want := range map[string]string{
		"matilda_service_user": "matilda-svc",
		"matilda_service_home": "/home/matilda-svc",
		"matilda_sudoers_file": "/etc/sudoers.d/matilda-discovery",
	} {
		if got := fmt.Sprint(defaults[key]); got != want {
			t.Fatalf("Linux role default %s = %q, want %q", key, got, want)
		}
	}
}

func TestRollbackPlaybookLoadsLinuxRoleDefaultsWithoutOverridingInventory(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "ansible", "playbooks", "linux", "rollback.yml"))
	if err != nil {
		t.Fatalf("expected rollback playbook: %v", err)
	}
	var plays []ansiblePlay
	if err := yaml.Unmarshal(content, &plays); err != nil {
		t.Fatalf("expected parseable rollback playbook: %v", err)
	}
	if len(plays) != 1 {
		t.Fatalf("expected one rollback play, got %d", len(plays))
	}
	if len(plays[0].VarsFiles) > 0 {
		t.Fatalf("rollback playbook should not use play-level vars_files because it can override inventory vars: %#v", plays[0].VarsFiles)
	}

	var loadsDefaults bool
	defaulted := map[string]bool{
		"matilda_service_user": false,
		"matilda_service_home": false,
		"matilda_sudoers_file": false,
	}
	for _, task := range plays[0].Tasks {
		if task.IncludeVars["file"] == "{{ playbook_dir }}/../../roles/matilda_linux_target/defaults/main.yml" &&
			task.IncludeVars["name"] == "matilda_linux_role_defaults" {
			loadsDefaults = true
		}
		for variable := range defaulted {
			wantDefault := fmt.Sprintf("{{ matilda_linux_role_defaults.%s }}", variable)
			if task.SetFact[variable] == wantDefault && whenContains(task.When, variable+" is not defined") {
				defaulted[variable] = true
			}
		}
	}
	if !loadsDefaults {
		t.Fatal("rollback playbook should include Linux role defaults into matilda_linux_role_defaults")
	}
	for variable, found := range defaulted {
		if !found {
			t.Fatalf("rollback playbook should default %s only when it is undefined", variable)
		}
	}
}

type ansiblePlay struct {
	VarsFiles []string      `yaml:"vars_files"`
	Tasks     []ansibleTask `yaml:"tasks"`
}

type ansibleTask struct {
	IncludeVars map[string]string `yaml:"ansible.builtin.include_vars"`
	SetFact     map[string]string `yaml:"ansible.builtin.set_fact"`
	When        any               `yaml:"when"`
}

func whenContains(when any, want string) bool {
	switch value := when.(type) {
	case string:
		return strings.Contains(value, want)
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok && strings.Contains(text, want) {
				return true
			}
		}
	}
	return false
}
