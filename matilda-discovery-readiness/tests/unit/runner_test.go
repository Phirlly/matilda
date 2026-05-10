package unit

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/runner"
)

func TestRunnerUsesRepoConfigAndShortSSHControlPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Ansible controller workflows are exercised on Unix-like systems")
	}

	root := t.TempDir()
	out, err := runner.RunCapture(root, "sh", "-c", "printf '%s\n%s\n%s\n' \"$ANSIBLE_CONFIG\" \"$ANSIBLE_LOCAL_TEMP\" \"$ANSIBLE_SSH_CONTROL_PATH_DIR\"")
	if err != nil {
		t.Fatalf("RunCapture failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 environment lines, got %d: %q", len(lines), out)
	}
	if lines[0] != filepath.Join(root, "ansible", "ansible.cfg") {
		t.Fatalf("unexpected ANSIBLE_CONFIG: %q", lines[0])
	}
	if lines[1] != filepath.Join(root, ".ansible", "tmp") {
		t.Fatalf("unexpected ANSIBLE_LOCAL_TEMP: %q", lines[1])
	}
	if strings.Contains(lines[2], root) {
		t.Fatalf("SSH control path should not use the repo path because long paths break OpenSSH sockets: %q", lines[2])
	}
	if !strings.HasSuffix(lines[2], "matilda-prep-cp") {
		t.Fatalf("unexpected SSH control path directory: %q", lines[2])
	}
}
