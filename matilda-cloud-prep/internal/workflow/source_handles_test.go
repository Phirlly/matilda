package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCachedSourceHandleFilesAcceptsBuiltInReferences(t *testing.T) {
	root := t.TempDir()
	for _, handle := range providerNeutralSourceHandles() {
		writeTestCachedReference(t, root, handle.URI)
	}

	if err := ValidateCachedSourceHandleFiles(root, providerNeutralSourceHandles()); err != nil {
		t.Fatalf("ValidateCachedSourceHandleFiles returned error: %v", err)
	}
}

func TestValidateCachedSourceHandleFilesAcceptsExistingCachedReference(t *testing.T) {
	root := t.TempDir()
	writeTestCachedReference(t, root, "docs/references/example/reference.md")

	err := ValidateCachedSourceHandleFiles(root, []SourceHandle{
		{Label: "Cached", URI: "docs/references/example/reference.md"},
	})
	if err != nil {
		t.Fatalf("ValidateCachedSourceHandleFiles returned error: %v", err)
	}
}

func TestValidateCachedSourceHandleFilesUsesCurrentDirectoryForEmptyRoot(t *testing.T) {
	root := t.TempDir()
	writeTestCachedReference(t, root, "docs/references/example/current-directory.md")
	t.Chdir(root)

	err := ValidateCachedSourceHandleFiles("", []SourceHandle{
		{Label: "Current Directory", URI: "docs/references/example/current-directory.md"},
	})
	if err != nil {
		t.Fatalf("ValidateCachedSourceHandleFiles returned error: %v", err)
	}
}

func TestValidateCachedSourceHandleFilesRejectsMissingCachedReference(t *testing.T) {
	err := ValidateCachedSourceHandleFiles(t.TempDir(), []SourceHandle{
		{Label: "Missing", URI: "docs/references/example/missing.md"},
	})
	if err == nil {
		t.Fatal("ValidateCachedSourceHandleFiles accepted a missing cached reference")
	}
	if !strings.Contains(err.Error(), "existing cached docs file") {
		t.Fatalf("error = %q, want existing cached docs file context", err)
	}
}

func TestValidateCachedSourceHandleFilesRejectsCachedReferenceDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs/references/example/directory.md"), 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}

	err := ValidateCachedSourceHandleFiles(root, []SourceHandle{
		{Label: "Directory", URI: "docs/references/example/directory.md"},
	})
	if err == nil {
		t.Fatal("ValidateCachedSourceHandleFiles accepted a directory source handle")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %q, want directory context", err)
	}
}

func writeTestCachedReference(t *testing.T, root, uri string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(uri))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create reference directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("# Reference\n"), 0o644); err != nil {
		t.Fatalf("write reference: %v", err)
	}
}
