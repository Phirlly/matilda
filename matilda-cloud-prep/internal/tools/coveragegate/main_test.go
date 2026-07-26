package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDelegatesToCoverageGate(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(profile, []byte(strings.Join([]string{
		"mode: set",
		"covered.go:1.1,10.2 9 1",
		"uncovered.go:1.1,10.2 1 0",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", profile, err)
	}

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{"-profile", profile, "-min", "90.0"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "coverage 90.00%") {
		t.Fatalf("stdout = %q, want coverage result", stdout.String())
	}
}
