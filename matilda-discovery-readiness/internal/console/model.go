package console

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/ui"
)

type ScreenMode string

const (
	ScreenMenu   ScreenMode = "menu"
	ScreenResult ScreenMode = "result"
)

type Model struct {
	rt         *app.Runtime
	keys       keyMap
	actions    []app.ActionSpec
	selected   int
	screen     ScreenMode
	width      int
	height     int
	snapshot   app.Snapshot
	activityVP viewport.Model
	activity   string
	running    bool
	tick       int
	cancelRun  context.CancelFunc
	cancelled  bool
	confirming *app.ActionSpec
	lastResult *app.ActionResult
}

func NewModel(rt *app.Runtime) Model {
	width := ui.TerminalWidth()
	height := 34
	m := Model{
		rt:       rt,
		keys:     defaultKeyMap(),
		actions:  workflowActions(),
		screen:   ScreenMenu,
		width:    width,
		height:   height,
		activity: "No action has run yet.",
	}
	resultWidth, resultHeight := m.resultViewportSize()
	m.activityVP = viewport.New(viewport.WithWidth(resultWidth), viewport.WithHeight(resultHeight))
	m.refreshSnapshot()
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = ui.Max(50, msg.Width)
		m.height = ui.Max(20, msg.Height)
		m.resizePanes()
	case actionFinishedMsg:
		m = m.finishAction(msg.action, msg.result)
	case actionOutputMsg:
		if m.running {
			m.appendActivity(msg.text)
			return m, waitActionEventCmd(msg.events)
		}
	case actionStreamClosedMsg:
		m.running = false
		m.cancelRun = nil
		m.cancelled = false
	case actionTickMsg:
		if m.running {
			m.tick++
			return m, actionTickCmd()
		}
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) View() tea.View {
	view := tea.NewView(renderInteractive(m))
	view.AltScreen = true
	return view
}

func (m Model) SelectedIndex() int {
	return m.selected
}

func (m Model) Screen() ScreenMode {
	return m.screen
}

func (m Model) ConfirmationOpen() bool {
	return m.confirming != nil
}

func (m Model) ActivityLog() string {
	return m.activity
}

func (m Model) LastResult() *app.ActionResult {
	return m.lastResult
}

func (m Model) Running() bool {
	return m.running
}

func (m Model) FocusOffset() int {
	return m.activityVP.YOffset()
}

func (m Model) FocusTotalLines() int {
	return m.activityVP.TotalLineCount()
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.confirming != nil {
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m.cancelConfirmation(), nil
		case key.Matches(msg, m.keys.Cancel):
			return m.cancelConfirmation(), nil
		case key.Matches(msg, m.keys.Confirm):
			return m.startAction(*m.confirming, true)
		default:
			return m, nil
		}
	}

	if m.screen == ScreenResult {
		return m.handleResultKey(msg)
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Refresh):
		m.refreshSnapshot()
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.moveUp()
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.moveDown()
		return m, nil
	case key.Matches(msg, m.keys.PageUp):
		m.pageUp()
		return m, nil
	case key.Matches(msg, m.keys.PageDown):
		m.pageDown()
		return m, nil
	case key.Matches(msg, m.keys.Home):
		m.home()
		return m, nil
	case key.Matches(msg, m.keys.End):
		m.end()
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		return m.activateSelected()
	}

	if index, ok := findActionByKey(m.actions, msg.String()); ok {
		m.selected = index
		return m.activateSelected()
	}
	return m, nil
}

func (m Model) handleResultKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.running {
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.requestCancel()
			return m, nil
		case key.Matches(msg, m.keys.Up):
			m.activityVP.ScrollUp(1)
			return m, nil
		case key.Matches(msg, m.keys.Down):
			m.activityVP.ScrollDown(1)
			return m, nil
		case key.Matches(msg, m.keys.PageUp):
			m.activityVP.PageUp()
			return m, nil
		case key.Matches(msg, m.keys.PageDown):
			m.activityVP.PageDown()
			return m, nil
		case key.Matches(msg, m.keys.Home):
			m.activityVP.GotoTop()
			return m, nil
		case key.Matches(msg, m.keys.End):
			m.activityVP.GotoBottom()
			return m, nil
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Back):
		m.screen = ScreenMenu
		return m, nil
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Refresh):
		m.refreshSnapshot()
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.activityVP.ScrollUp(1)
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.activityVP.ScrollDown(1)
		return m, nil
	case key.Matches(msg, m.keys.PageUp):
		m.activityVP.PageUp()
		return m, nil
	case key.Matches(msg, m.keys.PageDown):
		m.activityVP.PageDown()
		return m, nil
	case key.Matches(msg, m.keys.Home):
		m.activityVP.GotoTop()
		return m, nil
	case key.Matches(msg, m.keys.End):
		m.activityVP.GotoBottom()
		return m, nil
	}
	return m, nil
}

func (m Model) activateSelected() (Model, tea.Cmd) {
	action, ok := actionAt(m.actions, m.selected)
	if !ok {
		m.activity = "No workflow action is selected."
		m.screen = ScreenResult
		m.refreshViewports()
		return m, nil
	}
	if action.Mutating {
		m.confirming = &action
		m.activity = fmt.Sprintf("%s requires confirmation. Press y to continue or n/Esc to cancel.", action.Label)
		m.screen = ScreenResult
		m.refreshViewports()
		m.activityVP.GotoTop()
		return m, nil
	}
	return m.runSelectedAction(false)
}

func (m Model) cancelConfirmation() Model {
	if m.confirming != nil {
		m.activity = fmt.Sprintf("%s cancelled. No target changes were made.", m.confirming.Label)
	}
	m.confirming = nil
	m.screen = ScreenResult
	m.refreshViewports()
	m.activityVP.GotoTop()
	return m
}

func (m *Model) refreshSnapshot() {
	m.snapshot = m.rt.Snapshot()
	m.refreshViewports()
}

func (m *Model) resizePanes() {
	resultWidth, resultHeight := m.resultViewportSize()
	m.activityVP.SetWidth(resultWidth)
	m.activityVP.SetHeight(resultHeight)
	m.refreshViewports()
}

func (m *Model) refreshViewports() {
	m.activityVP.SetContent(renderActivityText(m.activity, m.activityVP.Width()))
}

func (m *Model) appendActivity(text string) {
	if text == "" {
		return
	}
	wasAtBottom := m.activityVP.YOffset()+m.activityVP.Height() >= m.activityVP.TotalLineCount()-1
	if m.activity != "" && !strings.HasSuffix(m.activity, "\n") {
		m.activity += "\n"
	}
	m.activity += text
	m.refreshViewports()
	if wasAtBottom {
		m.activityVP.GotoBottom()
	}
}

func (m *Model) requestCancel() {
	if m.cancelled {
		return
	}
	m.cancelled = true
	if m.cancelRun != nil {
		m.cancelRun()
	}
	m.appendActivity("\nCancellation requested. Waiting for the current command to stop.\n")
}

func (m Model) resultViewportSize() (int, int) {
	return ui.Max(40, m.width-4), ui.Max(8, m.height-8)
}

func (m *Model) moveUp() {
	if m.selected > 0 {
		m.selected--
	}
}

func (m *Model) moveDown() {
	if m.selected < len(m.actions)-1 {
		m.selected++
	}
}

func (m *Model) pageUp() {
	m.selected = ui.Max(0, m.selected-5)
}

func (m *Model) pageDown() {
	if len(m.actions) == 0 {
		return
	}
	m.selected = ui.Min(len(m.actions)-1, m.selected+5)
}

func (m *Model) home() {
	m.selected = 0
}

func (m *Model) end() {
	if len(m.actions) > 0 {
		m.selected = len(m.actions) - 1
	}
}
