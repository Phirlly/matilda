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
