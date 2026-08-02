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

func TestValidateCachedSourceHandleFilesRejectsMalformedReferenceNote(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs/references/example/placeholder.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create reference directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("# Placeholder\n"), 0o644); err != nil {
		t.Fatalf("write malformed reference: %v", err)
	}

	err := ValidateCachedSourceHandleFiles(root, []SourceHandle{
		{Label: "Malformed", URI: "docs/references/example/placeholder.md"},
	})
	if err == nil {
		t.Fatal("ValidateCachedSourceHandleFiles accepted a malformed reference note")
	}
	if !strings.Contains(err.Error(), "reference note") {
		t.Fatalf("error = %q, want reference note context", err)
	}
}

func TestValidateCachedSourceHandleFilesRejectsReferenceNoteMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing source type",
			content: strings.Replace(validReferenceNoteContent(), "Source type: official documentation.\n\n", "", 1),
			want:    "source type or source name",
		},
		{
			name:    "empty source type",
			content: strings.Replace(validReferenceNoteContent(), "Source type: official documentation.", "Source type:", 1),
			want:    "source type or source name",
		},
		{
			name:    "missing retrieval date",
			content: strings.Replace(validReferenceNoteContent(), "Retrieval date: 2026-08-01\n\n", "", 1),
			want:    "retrieval date",
		},
		{
			name:    "empty retrieval date",
			content: strings.Replace(validReferenceNoteContent(), "Retrieval date: 2026-08-01", "Retrieval date:", 1),
			want:    "retrieval date",
		},
		{
			name:    "missing why",
			content: strings.Replace(validReferenceNoteContent(), "Why this reference is needed:", "Why this note exists:", 1),
			want:    "why the reference is needed",
		},
		{
			name:    "empty why section",
			content: strings.Replace(validReferenceNoteContent(), "This reference note is needed to verify source-handle gate behavior.", "", 1),
			want:    "why the reference is needed",
		},
		{
			name:    "missing retrieval handle",
			content: strings.Replace(validReferenceNoteContent(), "## Retrieval Handles", "## Links", 1),
			want:    "source URL or retrieval handle",
		},
		{
			name:    "empty retrieval handle section",
			content: strings.Replace(validReferenceNoteContent(), "- [Example](https://example.com/reference)", "- Example reference without link", 1),
			want:    "source URL or retrieval handle",
		},
		{
			name:    "missing project summary",
			content: strings.Replace(validReferenceNoteContent(), "## Project-Specific Summary", "## Notes", 1),
			want:    "project-specific summary",
		},
		{
			name:    "empty project summary",
			content: strings.Replace(validReferenceNoteContent(), "This note summarizes the project-specific reference facts used by tests.", "", 1),
			want:    "project-specific summary",
		},
		{
			name:    "missing implementation guidance",
			content: strings.Replace(validReferenceNoteContent(), "## Implementation Notes", "## Notes", 1),
			want:    "implementation notes or open decisions",
		},
		{
			name:    "empty implementation guidance",
			content: strings.Replace(validReferenceNoteContent(), "- Validate reference-cache note shape without storing secrets.", "", 1),
			want:    "implementation notes or open decisions",
		},
		{
			name:    "missing top-level title",
			content: strings.Replace(validReferenceNoteContent(), "# Reference\n\n", "", 1),
			want:    "markdown title",
		},
		{
			name: "same heading cannot satisfy summary and implementation guidance",
			content: `# Reference

Source type: official documentation.

Retrieval date: 2026-08-01

Why this reference is needed:

This reference note is needed to verify source-handle gate behavior.

## Retrieval Handles

- [Example](https://example.com/reference)

## Design Decision

This note has a summary section but no distinct implementation guidance.
`,
			want: "implementation notes or open decisions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			uri := "docs/references/example/reference.md"
			path := filepath.Join(root, filepath.FromSlash(uri))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create reference directory: %v", err)
			}
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write reference: %v", err)
			}

			err := ValidateCachedSourceHandleFiles(root, []SourceHandle{
				{Label: "Reference", URI: uri},
			})
			if err == nil {
				t.Fatal("ValidateCachedSourceHandleFiles accepted malformed reference note")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateCachedSourceHandleFilesDoesNotApplyReferenceNoteShapeToWorkflowDocs(t *testing.T) {
	root := t.TempDir()
	writeWorkflowDoc(t, root, "docs/workflows/ARCHITECTURE.md")

	err := ValidateCachedSourceHandleFiles(root, []SourceHandle{
		{Label: "Architecture Workflow", URI: "docs/workflows/ARCHITECTURE.md"},
	})
	if err != nil {
		t.Fatalf("ValidateCachedSourceHandleFiles returned error for workflow doc: %v", err)
	}
}

func writeTestCachedReference(t *testing.T, root, uri string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(uri))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create reference directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(validReferenceNoteContent()), 0o644); err != nil {
		t.Fatalf("write reference: %v", err)
	}
}

func writeWorkflowDoc(t *testing.T, root, uri string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(uri))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workflow directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("# Architecture Workflow\n"), 0o644); err != nil {
		t.Fatalf("write workflow doc: %v", err)
	}
}

func validReferenceNoteContent() string {
	return `# Reference

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
`
}
