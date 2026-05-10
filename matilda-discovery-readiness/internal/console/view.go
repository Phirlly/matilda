package console

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/ui"
)

type consoleStyles struct {
	title       lipgloss.Style
	subtitle    lipgloss.Style
	muted       lipgloss.Style
	label       lipgloss.Style
	selected    lipgloss.Style
	ok          lipgloss.Style
	bad         lipgloss.Style
	warn        lipgloss.Style
	confirm     lipgloss.Style
	confirmText lipgloss.Style
}

func newConsoleStyles() consoleStyles {
	return consoleStyles{
		title:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")),
		subtitle:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		label:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250")),
		selected:    lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("24")).Bold(true),
		ok:          lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Bold(true),
		bad:         lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
		warn:        lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		confirm:     lipgloss.NewStyle().Border(lipgloss.ASCIIBorder()).BorderForeground(lipgloss.Color("214")).Padding(0, 1),
		confirmText: lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
	}
}

func renderInteractive(m Model) string {
	s := newConsoleStyles()
	width := ui.Max(70, m.width)
	if m.screen == ScreenResult {
		return renderResultScreen(s, m, width)
	}

	header := renderHeader(s, m.snapshot, width)
	next := renderNextStep(s, m.snapshot, width)
	menu := renderMenu(s, m, width)
	footer := renderMenuFooter(s)

	parts := []string{header, next, menu}
	if m.confirming != nil {
		parts = append(parts, renderConfirmation(s, m))
	}
	parts = append(parts, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func renderHeader(s consoleStyles, snap app.Snapshot, width int) string {
	status := fmt.Sprintf(
		"Inventory %s  Targets %d  Ready %s  Reports %s",
		inlineStatusText(s, snap.InventoryOK, "OK", "Fix"),
		snap.TargetCount,
		s.ok.Render(fmt.Sprintf("%d/%d", snap.ReportSummary.Ready, snap.ReadinessTotal())),
		reportState(s, snap.ReportError),
	)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		s.title.Render("Matilda Discovery Readiness"),
		s.muted.Render(status),
	)
}

func renderNextStep(s consoleStyles, snap app.Snapshot, width int) string {
	lines := []string{s.label.Render("Next: ") + lipgloss.Wrap(snap.NextStep, ui.Max(40, width-8), " ")}
	if len(snap.ValidatedIPs) > 0 {
		lines = append(lines, s.muted.Render("IPs: ")+strings.Join(snap.ValidatedIPs, ", "))
	}
	if snap.InventoryError != "" {
		lines = append(lines, s.bad.Render("Inventory: ")+lipgloss.Wrap(snap.InventoryError, ui.Max(40, width-12), " "))
	}
	if snap.ReportError != "" {
		lines = append(lines, s.warn.Render("Report: ")+lipgloss.Wrap(snap.ReportError, ui.Max(40, width-10), " "))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func renderMenu(s consoleStyles, m Model, width int) string {
	var lines []string
	lines = append(lines, s.label.Render(actionPanelTitle(m)))
	currentGroup := ""
	start, end := actionWindow(m)
	if start > 0 {
		lines = append(lines, s.muted.Render("↑ more actions"))
	}
	for i := start; i < end; i++ {
		action := m.actions[i]
		if action.Group != currentGroup {
			currentGroup = action.Group
			lines = append(lines, s.muted.Render(currentGroup))
		}
		row := renderActionRow(s, action, i == m.selected, ui.Max(24, width-6))
		lines = append(lines, row)
	}
	if end < len(m.actions) {
		lines = append(lines, s.muted.Render("↓ more actions"))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func actionPanelTitle(m Model) string {
	if len(m.actions) == 0 {
		return "Actions"
	}
	return fmt.Sprintf("Actions %d/%d", m.selected+1, len(m.actions))
}

func actionWindow(m Model) (int, int) {
	total := len(m.actions)
	if total == 0 {
		return 0, 0
	}
	limit := total
	if m.width < 112 {
		limit = ui.Max(3, ui.Min(5, m.height/6))
	}
	if limit >= total {
		return 0, total
	}
	start := m.selected - limit/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > total {
		end = total
		start = ui.Max(0, end-limit)
	}
	return start, end
}

func renderActionRow(s consoleStyles, action app.ActionSpec, selected bool, width int) string {
	marker := " "
	if selected {
		marker = ">"
	}
	mutation := ""
	if action.Mutating {
		mutation = " confirm"
	}
	text := fmt.Sprintf("%s %-34s %s%s", marker, ui.Truncate(action.Label, 34), ui.Truncate(action.Description, ui.Max(16, width-44)), mutation)
	if action.Key != "" {
		text = fmt.Sprintf("%s [%s]", text, action.Key)
	}
	text = ui.Truncate(text, width)
	if selected {
		return s.selected.Width(width).Render(text)
	}
	return text
}

type viewportInfo struct {
	offset int
	height int
	total  int
}

func viewportMeta(v viewportInfo) string {
	if v.total <= 0 {
		return "(0 lines)"
	}
	start := v.offset + 1
	end := ui.Min(v.total, v.offset+v.height)
	if end < start {
		end = start
	}
	return fmt.Sprintf("(%d-%d/%d)", start, end, v.total)
}

func renderConfirmation(s consoleStyles, m Model) string {
	if m.confirming == nil {
		return ""
	}
	width := ui.Max(48, ui.Min(m.width-4, 88))
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		s.confirmText.Render("Confirm Target Change"),
		fmt.Sprintf("%s modifies target systems.", m.confirming.Label),
		"Press y to continue. Press n or Esc to cancel.",
	)
	return s.confirm.Width(width).Render(body)
}

func renderMenuFooter(s consoleStyles) string {
	return s.muted.Render("up/down choose  enter run  pgup/pgdn jump  r refresh  q quit")
}

func renderResultScreen(s consoleStyles, m Model, width int) string {
	title := resultTitle(m)
	status := resultStatus(s, m)
	meta := viewportMeta(viewportInfo{
		offset: m.activityVP.YOffset(),
		height: m.activityVP.Height(),
		total:  m.activityVP.TotalLineCount(),
	})
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		s.title.Render("Matilda Discovery Readiness"),
		s.label.Render(title),
		s.muted.Render(strings.TrimSpace(status+"  "+meta)),
	)
	body := m.activityVP.View()
	footer := s.muted.Render("up/down scroll  pgup/pgdn page  home/end jump  b back  q quit")
	if m.running {
		footer = s.muted.Render("running; output streams live  up/down scroll  q/esc cancel")
	}
	parts := []string{header, body}
	if m.confirming != nil {
		parts = append(parts, renderConfirmation(s, m))
	}
	parts = append(parts, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func resultTitle(m Model) string {
	if action, ok := actionAt(m.actions, m.selected); ok {
		return action.Label
	}
	return "Result"
}

func resultStatus(s consoleStyles, m Model) string {
	if m.running {
		return s.warn.Render("running " + spinnerFrame(m.tick))
	}
	if m.lastResult == nil {
		return ""
	}
	if m.lastResult.OK {
		return s.ok.Render("completed")
	}
	return s.bad.Render("failed")
}

func spinnerFrame(tick int) string {
	frames := []string{"-", "\\", "|", "/"}
	return frames[tick%len(frames)]
}

func renderPlainStatus(s ui.Style, snap app.Snapshot) string {
	var b strings.Builder
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, s.Title("Matilda Discovery Readiness"))
	fmt.Fprintln(&b, s.Dim("Target, Probe, and platform readiness for Matilda discovery"))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, s.Section("Status"))
	printPlainKV(&b, s, []plainKV{
		{key: "Inventory", value: plainStatus(s, snap.InventoryOK, "OK", "Fix")},
		{key: "Targets", value: fmt.Sprintf("%d", snap.TargetCount)},
		{key: "Ready", value: s.OK(fmt.Sprintf("%d/%d", snap.ReportSummary.Ready, snap.ReadinessTotal()))},
		{key: "Reports", value: plainReportStatus(s, snap.ReportError)},
	})
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, s.Section("Workflow"))
	printPlainKV(&b, s, []plainKV{
		{key: "Inventory", value: plainWorkflowState(s, snap.InventoryOK)},
		{key: "Preflight", value: plainWorkflowState(s, snap.InventoryOK)},
		{key: "Setup", value: plainWorkflowState(s, snap.ReportSummary.Ready > 0)},
		{key: "Validate", value: plainWorkflowState(s, snap.ReportError == "")},
		{key: "Report", value: plainWorkflowState(s, snap.ReportError == "")},
	})
	if len(snap.Runs) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, s.Section("Recent Runs"))
		fmt.Fprint(&b, renderRunsText(snap, s.Width))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, s.Section("Target Readiness"))
	fmt.Fprint(&b, renderReadinessText(snap, s.Width))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, s.Section("Next Step"))
	fmt.Fprintln(&b, ui.Wrap(snap.NextStep, s.Width))
	if len(snap.ValidatedIPs) > 0 {
		fmt.Fprintf(&b, "%s %s\n", s.Dim("Validated IPs:"), strings.Join(snap.ValidatedIPs, ", "))
	}
	if snap.InventoryError != "" {
		fmt.Fprintf(&b, "%s %s\n", s.Bad("Inventory issue:"), ui.Wrap(snap.InventoryError, s.Width))
	}
	if snap.ReportError != "" {
		fmt.Fprintf(&b, "%s %s\n", s.Warn("Report status:"), ui.Wrap(snap.ReportError, s.Width))
	}
	return b.String()
}

func renderRunsText(snap app.Snapshot, width int) string {
	var b strings.Builder
	for _, run := range snap.Runs {
		when := run.EndedAt
		if when == "" {
			when = run.StartedAt
		}
		summary := run.Summary
		if summary == "" {
			summary = string(run.Status)
		}
		line := fmt.Sprintf("%s  %s  %s", run.Action, run.Status, summary)
		if run.Command != "" {
			line = fmt.Sprintf("%s  %s", line, run.Command)
		}
		if when != "" {
			line = fmt.Sprintf("%s  %s", when, line)
		}
		fmt.Fprintln(&b, ui.Truncate(line, width))
	}
	return b.String()
}

func printStaticActions(out io.Writer, s ui.Style, actions []app.ActionSpec) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, s.Section("Actions"))
	currentGroup := ""
	for _, action := range actions {
		if action.Group != currentGroup {
			currentGroup = action.Group
			fmt.Fprintf(out, "%s\n", s.Dim(currentGroup))
		}
		fmt.Fprintf(out, "  %-34s  %s\n", ui.Truncate(action.Label, 34), s.Dim(ui.Truncate(action.Description, ui.Max(16, s.Width-40))))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, s.Dim("Open in an interactive terminal to use arrow-key navigation."))
}

func renderReadinessText(snap app.Snapshot, width int) string {
	if len(snap.ReportRows) == 0 {
		return "No validation rows yet. Run validate to populate target readiness.\n"
	}
	var b strings.Builder
	if width < 90 {
		for _, row := range snap.ReportRows {
			fmt.Fprintf(&b, "%s  %s  %s\n", ui.Truncate(row.Host, ui.Max(20, width/3)), row.DiscoveryIP, row.Ready)
			fmt.Fprintf(&b, "  sudo %s  denied %s  probe %s  %s\n", row.LocalSudo, row.DeniedCommand, row.ProbeSSH, ui.Truncate(row.Remediation, ui.Max(18, width-42)))
		}
		return b.String()
	}

	widths := readinessWidths(width, snap)
	fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
		widths.host, "Host",
		widths.ip, "Discovery IP",
		widths.ready, "Ready",
		widths.sudo, "Sudo",
		widths.denied, "Denied",
		widths.probe, "Probe",
		"Remediation",
	)
	for _, row := range snap.ReportRows {
		fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
			widths.host, ui.Truncate(row.Host, widths.host),
			widths.ip, ui.Truncate(row.DiscoveryIP, widths.ip),
			widths.ready, row.Ready,
			widths.sudo, ui.Truncate(row.LocalSudo, widths.sudo),
			widths.denied, ui.Truncate(row.DeniedCommand, widths.denied),
			widths.probe, ui.Truncate(row.ProbeSSH, widths.probe),
			ui.Truncate(row.Remediation, widths.remediation),
		)
	}
	return b.String()
}

func renderReportFilesText(snap app.Snapshot) string {
	var b strings.Builder
	for _, file := range snap.ReportFiles {
		state := "missing"
		if file.Exists {
			state = "ready"
		}
		fmt.Fprintf(&b, "%-20s  %-7s  %s\n", ui.Truncate(file.Name, 20), state, file.Path)
	}
	return b.String()
}

func renderActivityText(text string, width int) string {
	width = ui.Max(40, width)
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			fmt.Fprintln(&b)
			continue
		}
		fmt.Fprintln(&b, ui.Wrap(line, width))
	}
	return strings.TrimRight(b.String(), "\n")
}

type plainKV struct {
	key   string
	value string
}

func printPlainKV(out io.Writer, s ui.Style, items []plainKV) {
	keyWidth := 0
	for _, item := range items {
		keyWidth = ui.Max(keyWidth, len(item.key))
	}
	keyWidth = ui.Min(keyWidth, 14)
	for _, item := range items {
		fmt.Fprintf(out, "  %-*s  %s\n", keyWidth, item.key, item.value)
	}
}

type tableWidths struct {
	host        int
	ip          int
	ready       int
	sudo        int
	denied      int
	probe       int
	remediation int
}

func readinessWidths(width int, snap app.Snapshot) tableWidths {
	w := tableWidths{host: 22, ip: 15, ready: 6, sudo: 6, denied: 7, probe: 6}
	for _, row := range snap.ReportRows {
		w.host = ui.Min(ui.Max(w.host, len(row.Host)), 30)
		w.ip = ui.Min(ui.Max(w.ip, len(row.DiscoveryIP)), 18)
	}
	used := w.host + w.ip + w.ready + w.sudo + w.denied + w.probe + 12
	w.remediation = ui.Max(16, width-used)
	return w
}

func plainStatus(s ui.Style, ok bool, good string, bad string) string {
	if ok {
		return s.OK(good)
	}
	return s.Bad(bad)
}

func plainWorkflowState(s ui.Style, done bool) string {
	if done {
		return s.OK("ready")
	}
	return s.Dim("pending")
}

func plainReportStatus(s ui.Style, err string) string {
	if err == "" {
		return s.OK("Ready")
	}
	return s.Warn("Pending")
}

func statusText(s consoleStyles, ok bool, good string, bad string) string {
	if ok {
		return s.ok.Render(good)
	}
	return s.bad.Render(bad)
}

func inlineStatusText(s consoleStyles, ok bool, good string, bad string) string {
	return statusText(s, ok, good, bad)
}

func reportState(s consoleStyles, err string) string {
	if err == "" {
		return s.ok.Render("Ready")
	}
	return s.warn.Render("Pending")
}
