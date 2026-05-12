package unit

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/runner"
	"matilda-discovery-readiness/internal/ui"
)

func TestTerminalRendererUsesSharedSections(t *testing.T) {
	var out bytes.Buffer
	renderer := ui.New(&out)

	renderer.Header("Inventory Validate", "read-only inventory checks")
	renderer.Section("Checks")
	renderer.Checks([]runner.Result{
		{Name: "targets.csv", Status: runner.StatusPass, Detail: "ok"},
		{Name: "ansible", Status: runner.StatusFail, Detail: "missing"},
	})
	renderer.Next("./matilda-prep doctor")

	text := out.String()
	for _, want := range []string{"Matilda Discovery Readiness", "Inventory Validate", "Checks", "PASS", "FAIL", "Next"} {
		if !strings.Contains(text, want) {
			t.Fatalf("renderer output missing %q:\n%s", want, text)
		}
	}
	for _, legacy := range []string{"Command  ", "Scope    ", "Usage:", "Commands:"} {
		if strings.Contains(text, legacy) {
			t.Fatalf("renderer output should not contain legacy marker %q:\n%s", legacy, text)
		}
	}
}

func TestTerminalRendererIndentsWrappedNextLines(t *testing.T) {
	var out bytes.Buffer
	renderer := ui.Renderer{Out: &out, Style: ui.Style{Width: 64}}

	renderer.Next("Open reports/readiness.html or use the validated discovery IPs when creating the Matilda discovery task.")

	contentLines := 0
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		if line == "" || line == "Next" {
			continue
		}
		contentLines++
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("wrapped Next line should remain indented, got %q in:\n%s", line, out.String())
		}
	}
	if contentLines < 2 {
		t.Fatalf("test message should wrap to at least 2 content lines, got %d in:\n%s", contentLines, out.String())
	}
}

func TestTerminalRendererIndentsWrappedErrorMessage(t *testing.T) {
	var out bytes.Buffer
	renderer := ui.Renderer{Out: &out, Style: ui.Style{Width: 64}}

	renderer.Error("Action failed", "Open reports/readiness.html or use the validated discovery IPs when creating the Matilda discovery task.", "")

	contentLines := 0
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		if line == "" || line == "Action failed" {
			continue
		}
		contentLines++
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("wrapped error line should remain indented, got %q in:\n%s", line, out.String())
		}
	}
	if contentLines < 2 {
		t.Fatalf("test message should wrap to at least 2 content lines, got %d in:\n%s", contentLines, out.String())
	}
}

func TestTerminalPromptUsesDefault(t *testing.T) {
	var out bytes.Buffer
	reader := bufio.NewReader(strings.NewReader("\n"))
	value := ui.New(&out).Prompt(reader, "Select", "1")
	if value != "1" {
		t.Fatalf("default prompt value = %q, want 1", value)
	}
	if !strings.Contains(out.String(), "Select [1]:") {
		t.Fatalf("prompt output missing default:\n%s", out.String())
	}
}
