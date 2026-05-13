package unit

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/safety"
)

func TestPrepareDestinationAllowsMissingDestination(t *testing.T) {
	var out bytes.Buffer
	dest := filepath.Join(t.TempDir(), "new-file.txt")

	if err := safety.PrepareDestination(strings.NewReader(""), &out, dest); err != nil {
		t.Fatalf("PrepareDestination should allow missing destinations: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("missing destination should not prompt, got:\n%s", out.String())
	}
}

func TestPrepareDestinationKeepsExistingFile(t *testing.T) {
	for name, input := range map[string]string{
		"default":  "\n",
		"explicit": "1\n",
		"invalid":  "not-a-choice\n",
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			dest := writeExistingFile(t, "existing content")

			err := safety.PrepareDestination(strings.NewReader(input), &out, dest)
			if !errors.Is(err, safety.ErrSkip) {
				t.Fatalf("expected ErrSkip, got %v", err)
			}
			assertFileContent(t, dest, "existing content")
			if matches, err := filepath.Glob(dest + ".backup-*"); err != nil || len(matches) != 0 {
				t.Fatalf("keep choice should not create backups, matches=%v err=%v", matches, err)
			}
			if !strings.Contains(out.String(), "File Exists") || !strings.Contains(out.String(), "keep existing file") {
				t.Fatalf("prompt output missing keep guidance:\n%s", out.String())
			}
			if name == "invalid" && !strings.Contains(out.String(), "Invalid choice") {
				t.Fatalf("invalid choice should explain that the existing file was kept:\n%s", out.String())
			}
		})
	}
}

func TestPrepareDestinationBacksUpExistingFile(t *testing.T) {
	var out bytes.Buffer
	dest := writeExistingFile(t, "existing content")

	if err := safety.PrepareDestination(strings.NewReader("2\n"), &out, dest); err != nil {
		t.Fatalf("backup choice should continue after creating backup: %v", err)
	}
	assertFileContent(t, dest, "existing content")
	matches, err := filepath.Glob(dest + ".backup-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup file, got %v", matches)
	}
	assertFileContent(t, matches[0], "existing content")
	if !strings.Contains(out.String(), "Backed up") || !strings.Contains(out.String(), matches[0]) {
		t.Fatalf("backup output missing backup path:\n%s", out.String())
	}
}

func TestPrepareDestinationOverwritesWithoutBackup(t *testing.T) {
	var out bytes.Buffer
	dest := writeExistingFile(t, "existing content")

	if err := safety.PrepareDestination(strings.NewReader("3\n"), &out, dest); err != nil {
		t.Fatalf("overwrite choice should continue without backup: %v", err)
	}
	assertFileContent(t, dest, "existing content")
	if matches, err := filepath.Glob(dest + ".backup-*"); err != nil || len(matches) != 0 {
		t.Fatalf("overwrite choice should not create backups, matches=%v err=%v", matches, err)
	}
	if !strings.Contains(out.String(), "Overwriting") || !strings.Contains(out.String(), dest) {
		t.Fatalf("overwrite output missing warning:\n%s", out.String())
	}
}

func writeExistingFile(t *testing.T, content string) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(dest, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return dest
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, string(got), want)
	}
}
