package unit

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/console"
)

func TestConsoleModelMovesSelectionWithKeyboard(t *testing.T) {
	model := newConsoleModel(t)

	model = press(model, keyDown())
	if model.SelectedIndex() != 1 {
		t.Fatalf("down selected index = %d, want 1", model.SelectedIndex())
	}
	model = press(model, keyText("j"))
	if model.SelectedIndex() != 2 {
		t.Fatalf("j selected index = %d, want 2", model.SelectedIndex())
	}
	model = press(model, keyUp())
	if model.SelectedIndex() != 1 {
		t.Fatalf("up selected index = %d, want 1", model.SelectedIndex())
	}
	model = press(model, keyEnd())
	wantLast := len(app.WorkflowActions()) - 1
	if model.SelectedIndex() != wantLast {
		t.Fatalf("end selected index = %d, want %d", model.SelectedIndex(), wantLast)
	}
	model = press(model, keyHome())
	if model.SelectedIndex() != 0 {
		t.Fatalf("home selected index = %d, want 0", model.SelectedIndex())
	}
}

func TestConsoleResultViewScrollAndBackKeys(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, "reports", "validation-summary.txt"), unitValidationSummary())
	writeUnitFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), manyValidatedIPs(40))
	model := modelForRoot(root)

	model, cmd := pressWithCmd(model, keyText("4"))
	if model.Screen() != console.ScreenResult {
		t.Fatalf("screen = %s, want result", model.Screen())
	}
	if !model.Running() {
		t.Fatalf("expected action to start in running state")
	}
	if model.LastResult() != nil {
		t.Fatalf("result should not be set until action command completes")
	}
	model = finishFirstCommand(t, model, cmd)
	if model.FocusTotalLines() <= 20 {
		t.Fatalf("expected enough result output to scroll, got %d lines", model.FocusTotalLines())
	}
	model = press(model, keyPageDown())
	if model.FocusOffset() == 0 {
		t.Fatalf("expected result view to scroll down")
	}
	model = press(model, keyHome())
	if model.FocusOffset() != 0 {
		t.Fatalf("home offset = %d, want 0", model.FocusOffset())
	}
	model = press(model, keyEsc())
	if model.Screen() != console.ScreenMenu {
		t.Fatalf("esc should return from result to menu, got %s", model.Screen())
	}
}

func TestConsoleMutatingActionRequiresConfirmation(t *testing.T) {
	model := newConsoleModel(t)

	model = press(model, keyText("8"))
	if !model.ConfirmationOpen() {
		t.Fatalf("setup shortcut should open confirmation")
	}
	if model.LastResult() != nil {
		t.Fatalf("mutating action should not run before confirmation")
	}

	model = press(model, keyText("n"))
	if model.ConfirmationOpen() {
		t.Fatalf("confirmation should close after cancellation")
	}
	if !strings.Contains(model.ActivityLog(), "No target changes were made") {
		t.Fatalf("expected cancellation activity, got:\n%s", model.ActivityLog())
	}
}

func TestConsoleExpandedMutatingShortcutsOpenConfirmation(t *testing.T) {
	for _, key := range []string{"0", "s", "x", "l", "d"} {
		t.Run(key, func(t *testing.T) {
			model := newConsoleModel(t)

			model = press(model, keyText(key))
			if !model.ConfirmationOpen() {
				t.Fatalf("shortcut %q should open confirmation", key)
			}
		})
	}
}

func TestConsoleConfirmRunsMutatingAction(t *testing.T) {
	model := newConsoleModel(t)

	model = press(model, keyText("8"))
	model, cmd := pressWithCmd(model, keyText("y"))
	if !model.Running() {
		t.Fatalf("expected confirmed setup action to start running")
	}
	model = finishFirstCommand(t, model, cmd)

	result := model.LastResult()
	if result == nil {
		t.Fatalf("expected confirmed setup action to run")
	}
	if result.OK || !strings.Contains(result.Error, "require .env") {
		t.Fatalf("expected confirmed setup to reach .env guard, got %+v", result)
	}
}

func TestConsoleReadOnlyActionRunsWithoutConfirmation(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, "reports", "validation-summary.txt"), unitValidationSummary())
	writeUnitFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")
	model := modelForRoot(root)

	model, cmd := pressWithCmd(model, keyText("4"))
	if model.ConfirmationOpen() {
		t.Fatalf("validated IPs should not ask for confirmation")
	}
	if !model.Running() {
		t.Fatalf("validated IPs action should start asynchronously")
	}
	if model.LastResult() != nil {
		t.Fatalf("validated IPs result should not be populated before command completion")
	}
	model = finishFirstCommand(t, model, cmd)
	result := model.LastResult()
	if result == nil || !result.OK {
		t.Fatalf("expected validated IPs action to succeed, got %+v", result)
	}
	if model.Screen() != console.ScreenResult {
		t.Fatalf("read-only action should open result screen, got %s", model.Screen())
	}
	if !strings.Contains(model.ActivityLog(), "10.0.0.10") {
		t.Fatalf("activity log missing validated IP:\n%s", model.ActivityLog())
	}
	if strings.Contains(model.ActivityLog(), "Validated IPs running") {
		t.Fatalf("completed activity should not keep stale running text:\n%s", model.ActivityLog())
	}
}

func TestConsoleStreamsOutputBeforeActionFinishes(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, "reports", "validation-summary.txt"), unitValidationSummary())
	writeUnitFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")
	model := modelForRoot(root)

	model, cmd := pressWithCmd(model, keyText("4"))
	waitCmd := startActionCommand(t, cmd)
	msg := waitCmd()
	updated, next := model.Update(msg)
	model = updated.(console.Model)

	if !model.Running() {
		t.Fatalf("action should still be running while streamed output is consumed")
	}
	if model.LastResult() != nil {
		t.Fatalf("result should not be set before the finish event")
	}
	if !strings.Contains(model.ActivityLog(), "Validated IPs") {
		t.Fatalf("expected streamed output before finish, got:\n%s", model.ActivityLog())
	}

	model = drainActionMessages(t, model, next)
	if result := model.LastResult(); result == nil || !result.OK {
		t.Fatalf("expected action to finish successfully, got %+v", result)
	}
}

func TestConsoleRunningActionCanRequestCancellation(t *testing.T) {
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, "reports", "validation-summary.txt"), unitValidationSummary())
	writeUnitFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")
	model := modelForRoot(root)

	model, _ = pressWithCmd(model, keyText("4"))
	if !model.Running() {
		t.Fatalf("expected action to be running")
	}
	model = press(model, keyText("q"))
	if !model.Running() {
		t.Fatalf("cancel request should wait for action completion")
	}
	if !strings.Contains(model.ActivityLog(), "Cancellation requested") {
		t.Fatalf("activity log should show cancellation request:\n%s", model.ActivityLog())
	}
}

func TestConsoleQuitKeyReturnsQuitCommand(t *testing.T) {
	model := newConsoleModel(t)

	_, cmd := model.Update(keyText("q"))
	if cmd == nil {
		t.Fatalf("q should return a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q command should produce QuitMsg")
	}

	_, cmd = model.Update(keyEsc())
	if cmd == nil {
		t.Fatalf("esc should return a quit command when no confirmation is open")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("esc command should produce QuitMsg")
	}
}

func newConsoleModel(t *testing.T) console.Model {
	t.Helper()
	return newConsoleModelWithSummary(t, unitValidationSummary())
}

func newConsoleModelWithSummary(t *testing.T, summary string) console.Model {
	t.Helper()
	root := t.TempDir()
	writeUnitFile(t, filepath.Join(root, "targets.csv"), unitTargetsCSV())
	writeUnitFile(t, filepath.Join(root, "reports", "validation-summary.txt"), summary)
	return modelForRoot(root)
}

func modelForRoot(root string) console.Model {
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	model := console.NewModel(rt)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	return updated.(console.Model)
}

func press(model console.Model, key tea.KeyPressMsg) console.Model {
	updated, _ := model.Update(key)
	return updated.(console.Model)
}

func pressWithCmd(model console.Model, key tea.KeyPressMsg) (console.Model, tea.Cmd) {
	updated, cmd := model.Update(key)
	return updated.(console.Model), cmd
}

func finishFirstCommand(t *testing.T, model console.Model, cmd tea.Cmd) console.Model {
	t.Helper()
	return drainActionMessages(t, model, startActionCommand(t, cmd))
}

func startActionCommand(t *testing.T, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected command")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		if len(batch) < 2 {
			t.Fatalf("expected action and stream commands in batch")
		}
		if setupMsg := batch[0](); setupMsg != nil {
			t.Fatalf("expected action setup command to stream through channel, got %T", setupMsg)
		}
		return batch[1]
	}
	return func() tea.Msg { return msg }
}

func drainActionMessages(t *testing.T, model console.Model, cmd tea.Cmd) console.Model {
	t.Helper()
	for i := 0; i < 200 && model.Running(); i++ {
		if cmd == nil {
			t.Fatalf("expected command while action is running")
		}
		msg := cmd()
		if msg == nil {
			continue
		}
		updated, next := model.Update(msg)
		model = updated.(console.Model)
		cmd = next
	}
	if model.Running() {
		t.Fatalf("action did not finish")
	}
	return model
}

func keyText(text string) tea.KeyPressMsg {
	runes := []rune(text)
	if len(runes) == 0 {
		return tea.KeyPressMsg(tea.Key{})
	}
	return tea.KeyPressMsg(tea.Key{Text: text, Code: runes[0]})
}

func keyUp() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
}

func keyDown() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
}

func keyTab() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})
}

func keyShiftTab() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift})
}

func keyEsc() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
}

func keyPageDown() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown})
}

func keyHome() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyHome})
}

func keyEnd() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd})
}

func manyValidationRows(count int) string {
	var b strings.Builder
	b.WriteString("Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation\n")
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&b, "app%02d,10.0.0.%d,ifconfig,NO,OK,OK,OK,YES,None\n", i, i)
	}
	return b.String()
}

func manyValidatedIPs(count int) string {
	var b strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&b, "10.0.0.%d\n", i)
	}
	return b.String()
}
