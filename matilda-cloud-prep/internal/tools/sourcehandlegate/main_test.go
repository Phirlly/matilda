package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDelegatesToSourceHandleGate(t *testing.T) {
	root := t.TempDir()
	writeToolTestFile(t, root, "internal/provider/report.go", `package provider

var source = "docs/references/aws/current.md"
`)
	writeToolTestFile(t, root, "docs/references/aws/current.md", `# Reference

Source type: official documentation.

Retrieval date: 2026-08-01

Why this reference is needed:

This reference note is needed to verify source-handle gate delegation.

## Retrieval Handles

- [Example](https://example.com/reference)

## Project-Specific Summary

This note summarizes the project-specific reference facts used by tests.

## Implementation Notes

- Validate reference-cache note shape without storing secrets.
`)

	var stdout strings.Builder
	var stderr strings.Builder
	code := run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "source handle gate passed") {
		t.Fatalf("stdout = %q, want source handle gate result", stdout.String())
	}
}

func writeToolTestFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
