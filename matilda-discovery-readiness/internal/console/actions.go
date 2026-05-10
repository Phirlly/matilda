package console

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"matilda-discovery-readiness/internal/app"
)

type actionFinishedMsg struct {
	action app.ActionSpec
	result app.ActionResult
}

type actionOutputMsg struct {
	events <-chan actionEvent
	text   string
}

type actionStreamClosedMsg struct{}

type actionEvent struct {
	action app.ActionSpec
	output string
	result app.ActionResult
	done   bool
}

type actionTickMsg time.Time

func workflowActions() []app.ActionSpec {
	return app.WorkflowActions()
}

func actionAt(actions []app.ActionSpec, index int) (app.ActionSpec, bool) {
	if index < 0 || index >= len(actions) {
		return app.ActionSpec{}, false
	}
	return actions[index], true
}

func findActionByKey(actions []app.ActionSpec, keyText string) (int, bool) {
	for i, action := range actions {
		if action.Key == keyText {
			return i, true
		}
	}
	return 0, false
}

func (m Model) runSelectedAction(confirmed bool) (Model, tea.Cmd) {
	action, ok := actionAt(m.actions, m.selected)
	if !ok {
		m.activity = "No workflow action is selected."
		m.screen = ScreenResult
		m.refreshViewports()
		return m, nil
	}
	return m.startAction(action, confirmed)
}

func (m Model) startAction(action app.ActionSpec, confirmed bool) (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan actionEvent, 128)
	m.running = true
	m.tick = 0
	m.cancelRun = cancel
	m.cancelled = false
	m.lastResult = nil
	m.confirming = nil
	m.screen = ScreenResult
	m.activity = formatRunningActivity(action)
	m.refreshViewports()
	m.activityVP.GotoTop()
	return m, tea.Batch(runActionStreamCmd(ctx, m.rt, action, confirmed, events), waitActionEventCmd(events), actionTickCmd())
}

func (m Model) finishAction(action app.ActionSpec, result app.ActionResult) Model {
	m.running = false
	m.cancelRun = nil
	m.cancelled = false
	m.lastResult = &result
	m.activity = appendCompletion(m.activity, action, result)
	m.confirming = nil
	m.screen = ScreenResult
	m.refreshSnapshot()
	m.activityVP.GotoTop()
	return m
}

func runActionStreamCmd(ctx context.Context, rt *app.Runtime, action app.ActionSpec, confirmed bool, events chan<- actionEvent) tea.Cmd {
	return func() tea.Msg {
		writer := actionStreamWriter{events: events}
		result := rt.WithContext(ctx).RunWorkflowActionTo(action.ID, confirmed, writer, writer)
		events <- actionEvent{
			action: action,
			result: result,
			done:   true,
		}
		close(events)
		return nil
	}
}

func waitActionEventCmd(events <-chan actionEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return actionStreamClosedMsg{}
		}
		if event.done {
			return actionFinishedMsg{action: event.action, result: event.result}
		}
		return actionOutputMsg{events: events, text: event.output}
	}
}

func actionTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return actionTickMsg(t)
	})
}

type actionStreamWriter struct {
	events chan<- actionEvent
}

func (w actionStreamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.events <- actionEvent{output: string(append([]byte(nil), p...))}
	return len(p), nil
}

func formatRunningActivity(action app.ActionSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", action.Label)
	fmt.Fprintf(&b, "Output\n")
	if action.Remote {
		fmt.Fprintf(&b, "Remote checks can take longer because they run Ansible over SSH.\n\n")
	}
	return b.String()
}

func formatActivity(action app.ActionSpec, result app.ActionResult) string {
	var b strings.Builder
	status := "completed"
	if !result.OK {
		status = "failed"
	}
	fmt.Fprintf(&b, "%s %s\n", action.Label, status)
	if result.Error != "" {
		fmt.Fprintf(&b, "\nError\n  %s\n", result.Error)
	}
	if result.Output != "" {
		fmt.Fprintf(&b, "\nOutput\n%s\n", result.Output)
	}
	return strings.TrimSpace(b.String())
}

func appendCompletion(activity string, action app.ActionSpec, result app.ActionResult) string {
	if strings.TrimSpace(activity) == "" || !strings.Contains(activity, "Output") {
		return formatActivity(action, result)
	}

	var b strings.Builder
	b.WriteString(strings.TrimRight(activity, "\n"))
	b.WriteString("\n\nSummary\n")
	status := "completed"
	if !result.OK {
		status = "failed"
	}
	fmt.Fprintf(&b, "  %s %s\n", action.Label, status)
	if result.Error != "" {
		fmt.Fprintf(&b, "  Error: %s\n", result.Error)
	}
	return strings.TrimSpace(b.String())
}
