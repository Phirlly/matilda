package sourcehandlegate

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunValidatesProductionSourceHandlesAgainstLocalCache(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/report.go", `package provider

var source = "docs/references/aws/current.md"
`)
	writeCachedReference(t, root, "docs/references/aws/current.md")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("Run exit code = %d, want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 cached source handle") {
		t.Fatalf("stdout = %q, want verified handle count", stdout.String())
	}
}

func TestRunFailsWhenProductionSourceHandleIsMissing(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/report.go", `package provider

var source = "docs/references/aws/missing.md"
`)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != ExitGateFailed {
		t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
	}
	if !strings.Contains(stderr.String(), "missing.md") {
		t.Fatalf("stderr = %q, want missing source handle path", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output on failed gate", stdout.String())
	}
}

func TestRunIgnoresTestFileSourceHandleFixtures(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/report.go", `package provider

var source = "docs/references/aws/current.md"
`)
	writeSourceFile(t, root, "internal/provider/report_test.go", `package provider

var fixture = "docs/references/aws/test-only-missing.md"
`)
	writeCachedReference(t, root, "docs/references/aws/current.md")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("Run exit code = %d, want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if strings.Contains(stderr.String(), "test-only-missing.md") {
		t.Fatalf("stderr = %q, want test fixture source handle ignored", stderr.String())
	}
}

func TestRunValidatesSingleProductionFileScan(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/report.go", `package provider

var source = "docs/references/aws/current.md"
`)
	writeCachedReference(t, root, "docs/references/aws/current.md")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal/provider/report.go"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("Run exit code = %d, want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 cached source handle") {
		t.Fatalf("stdout = %q, want verified handle count", stdout.String())
	}
}

func TestRunSkipsVendorDirectories(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/report.go", `package provider

var source = "docs/references/aws/current.md"
`)
	writeSourceFile(t, root, "internal/vendor/dependency/report.go", `package dependency

var fixture = "docs/references/aws/vendor-missing.md"
`)
	writeCachedReference(t, root, "docs/references/aws/current.md")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("Run exit code = %d, want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if strings.Contains(stderr.String(), "vendor-missing.md") {
		t.Fatalf("stderr = %q, want vendor source handle ignored", stderr.String())
	}
}

func TestRunFailsWhenNoProductionSourceHandlesAreFound(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/report.go", `package provider

var message = "no source handles here"
`)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != ExitGateFailed {
		t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
	}
	if !strings.Contains(stderr.String(), "no cached source handles") {
		t.Fatalf("stderr = %q, want no source handles context", stderr.String())
	}
}

func TestRunDoesNotCountReferenceCachePolicyReadmeLiteralAsSourceHandle(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/report.go", `package provider

var policy = "docs/references/README.md"
`)
	writeSourceFile(t, root, "docs/references/README.md", "# Reference Cache\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != ExitGateFailed {
		t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
	}
	if !strings.Contains(stderr.String(), "no cached source handles") {
		t.Fatalf("stderr = %q, want no cached source handles context", stderr.String())
	}
}

func TestRunFailsWhenScanFileIsNotProductionGo(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/README.md", "# Provider\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal/provider/README.md"}, &stdout, &stderr)

	if code != ExitGateFailed {
		t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
	}
	if !strings.Contains(stderr.String(), "no cached source handles") {
		t.Fatalf("stderr = %q, want no source handles context", stderr.String())
	}
}

func TestRunRejectsInvalidProductionGo(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "internal/provider/report.go", `package provider

var source =
`)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != ExitGateFailed {
		t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
	}
	if !strings.Contains(stderr.String(), "parse") {
		t.Fatalf("stderr = %q, want parse context", stderr.String())
	}
}

func TestRunRejectsMissingScanPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", t.TempDir(), "-scan", "internal"}, &stdout, &stderr)

	if code != ExitGateFailed {
		t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
	}
	if !strings.Contains(stderr.String(), "cannot be accessed") {
		t.Fatalf("stderr = %q, want missing scan path context", stderr.String())
	}
}

func TestRunReportsDirectoryWalkErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission semantics differ on Windows")
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "unreadable"), 0o755); err != nil {
		t.Fatalf("create unreadable directory: %v", err)
	}
	if err := os.Chmod(filepath.Join(root, "internal", "unreadable"), 0); err != nil {
		t.Fatalf("chmod unreadable directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(root, "internal", "unreadable"), 0o755)
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-root", root, "-scan", "internal"}, &stdout, &stderr)

	if code != ExitGateFailed {
		t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
	}
	if !strings.Contains(stderr.String(), "scan path") {
		t.Fatalf("stderr = %q, want scan path context", stderr.String())
	}
}

func TestRunRejectsUnexpectedArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"unexpected"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("Run exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr.String(), "unexpected source handle gate argument") {
		t.Fatalf("stderr = %q, want usage context", stderr.String())
	}
}

func TestRunRejectsEmptyScanFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"-scan", ""}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("Run exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr.String(), "scan path must not be empty") {
		t.Fatalf("stderr = %q, want empty scan context", stderr.String())
	}
}

func writeSourceFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
}

func writeCachedReference(t *testing.T, root, uri string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(uri))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create reference directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(`# Reference

Source type: official documentation.

Retrieval date: 2026-08-01

Why this reference is needed:

This reference note is needed to verify source-handle gate behavior.

## Retrieval Handles

- [Example](https://example.com/reference)

## Project-Specific Summary

This note summarizes the project-specific reference facts used by tests.

## Implementation Notes

- Validate reference-cache note shape without storing secrets.
`), 0o644); err != nil {
		t.Fatalf("write cached reference: %v", err)
	}
}
