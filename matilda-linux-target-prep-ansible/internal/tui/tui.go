package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"matilda-discovery-readiness/internal/app"
)

type menuItem struct {
	Key         string
	Label       string
	Description string
	Group       string
	Run         func() error
}

type style struct {
	enabled bool
	width   int
}

func Run(rt *app.Runtime) error {
	reader := bufio.NewReader(rt.In)
	session := *rt
	session.In = reader
	theme := style{enabled: shouldUseColor(rt.Out), width: terminalWidth()}

	for {
		theme.width = terminalWidth()
		printScreen(session.Out, theme, session.Snapshot())
		items := menuItems(&session)
		printMenu(session.Out, theme, items)

		fmt.Fprint(session.Out, theme.dim("Select action [q]: "))
		answer, _ := reader.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "" || answer == "q" || answer == "quit" {
			return nil
		}

		item, ok := findItem(items, answer)
		if !ok {
			fmt.Fprintln(session.Out, theme.bad("\nInvalid selection."))
			pause(reader, session.Out, theme)
			continue
		}

		printActionLogHeader(session.Out, theme, item.Label)
		if err := item.Run(); err != nil {
			fmt.Fprintf(session.Out, "\n%s %v\n", theme.bad("Action failed:"), err)
		}
		pause(reader, session.Out, theme)
	}
}

func menuItems(rt *app.Runtime) []menuItem {
	var items []menuItem
	for _, spec := range app.WorkflowActions() {
		items = append(items, menuItem{
			Key:         spec.Key,
			Label:       spec.Label,
			Description: spec.Description,
			Group:       spec.Group,
			Run:         runFuncForAction(rt, spec.ID),
		})
	}
	return items
}

func runFuncForAction(rt *app.Runtime, id string) func() error {
	switch id {
	case "doctor":
		return rt.Doctor
	case "inventory-validate":
		return rt.InventoryValidate
	case "report":
		return rt.Report
	case "validated-ips":
		return func() error { return showValidatedIPs(rt) }
	case "generate-windows":
		return func() error { return rt.Generate([]string{"windows"}) }
	case "generate-unix":
		return func() error { return rt.Generate([]string{"unix"}) }
	case "preflight":
		return rt.Preflight
	case "setup":
		return rt.Setup
	case "validate":
		return rt.Validate
	case "rollback-sudoers":
		return func() error { return rt.Rollback([]string{"--sudoers-only"}) }
	default:
		return func() error { return fmt.Errorf("unsupported action %q", id) }
	}
}

func printScreen(out io.Writer, s style, snap app.Snapshot) {
	clearIfInteractive(out)
	fmt.Fprintln(out)
	fmt.Fprintln(out, s.title("Matilda Discovery Readiness"))
	fmt.Fprintln(out, s.dim("Target preparation, validation, reports, and platform guidance"))
	fmt.Fprintln(out)

	printStatus(out, s, snap)
	fmt.Fprintln(out)
	printWorkflow(out, s, snap)
	fmt.Fprintln(out)
	printReadiness(out, s, snap)
	fmt.Fprintln(out)
	printNextStep(out, s, snap)
}

func printStatus(out io.Writer, s style, snap app.Snapshot) {
	fmt.Fprintln(out, s.section("Status"))
	items := []kv{
		{key: "Inventory", value: statusValue(s, snap.InventoryOK, "OK", "Fix")},
		{key: "Targets", value: fmt.Sprintf("%d", snap.TargetCount)},
		{key: "Ready", value: s.ok(fmt.Sprintf("%d/%d", snap.ReportSummary.Ready, snap.ReportSummary.Total))},
		{key: "Reports", value: reportStatus(s, snap.ReportError)},
	}
	printKVList(out, s, items)
}

func printWorkflow(out io.Writer, s style, snap app.Snapshot) {
	fmt.Fprintln(out, s.section("Workflow"))
	steps := []kv{
		{key: "Inventory", value: workflowState(s, snap.InventoryOK)},
		{key: "Preflight", value: workflowState(s, snap.InventoryOK)},
		{key: "Setup", value: workflowState(s, snap.ReportSummary.Ready > 0)},
		{key: "Validate", value: workflowState(s, snap.ReportError == "")},
		{key: "Report", value: workflowState(s, snap.ReportError == "")},
	}
	printKVList(out, s, steps)
}

func printReadiness(out io.Writer, s style, snap app.Snapshot) {
	fmt.Fprintln(out, s.section("Target Readiness"))
	if len(snap.ReportRows) == 0 {
		fmt.Fprintln(out, s.dim("No validation rows yet. Run validate to populate target readiness."))
		return
	}
	if s.width < 96 {
		for _, row := range snap.ReportRows {
			ready := readyText(s, row.Ready)
			fmt.Fprintf(out, "%s  %s  %s\n", truncate(row.Host, max(20, s.width/3)), row.DiscoveryIP, ready)
			fmt.Fprintf(out, "  sudo %s  denied %s  probe %s  %s\n", row.LocalSudo, row.DeniedCommand, row.ProbeSSH, truncate(row.Remediation, max(18, s.width-42)))
		}
		return
	}

	widths := readinessWidths(s.width, snap)
	fmt.Fprintf(out, "%-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
		widths.host, "Host",
		widths.ip, "Discovery IP",
		widths.ready, "Ready",
		widths.sudo, "Sudo",
		widths.denied, "Denied",
		widths.probe, "Probe",
		"Remediation",
	)
	for _, row := range snap.ReportRows {
		fmt.Fprintf(out, "%-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
			widths.host, truncate(row.Host, widths.host),
			widths.ip, truncate(row.DiscoveryIP, widths.ip),
			widths.ready, readyText(s, row.Ready),
			widths.sudo, truncate(row.LocalSudo, widths.sudo),
			widths.denied, truncate(row.DeniedCommand, widths.denied),
			widths.probe, truncate(row.ProbeSSH, widths.probe),
			truncate(row.Remediation, widths.remediation),
		)
	}
}

func printNextStep(out io.Writer, s style, snap app.Snapshot) {
	fmt.Fprintln(out, s.section("Next Step"))
	fmt.Fprintln(out, wrapLine(snap.NextStep, s.width))
	if len(snap.ValidatedIPs) > 0 {
		fmt.Fprintf(out, "%s %s\n", s.dim("Validated IPs:"), strings.Join(snap.ValidatedIPs, ", "))
	}
	if snap.InventoryError != "" {
		fmt.Fprintf(out, "%s %s\n", s.bad("Inventory issue:"), wrapLine(snap.InventoryError, s.width))
	}
	if snap.ReportError != "" {
		fmt.Fprintf(out, "%s %s\n", s.warn("Report status:"), wrapLine(snap.ReportError, s.width))
	}
}

func printMenu(out io.Writer, s style, items []menuItem) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, s.section("Actions"))
	labelWidth := 0
	for _, item := range items {
		labelWidth = max(labelWidth, len(item.Label))
	}
	labelWidth = min(labelWidth, max(18, min(38, s.width/2)))
	currentGroup := ""
	for _, item := range items {
		if item.Group != currentGroup {
			currentGroup = item.Group
			fmt.Fprintf(out, "%s\n", s.dim(currentGroup))
		}
		descWidth := max(12, s.width-labelWidth-10)
		fmt.Fprintf(out, "  %2s  %-*s  %s\n", s.key(item.Key), labelWidth, truncate(item.Label, labelWidth), s.dim(truncate(item.Description, descWidth)))
	}
	fmt.Fprintf(out, "  %2s  %-*s  %s\n", s.key("q"), labelWidth, "Quit", s.dim("leave the TUI"))
	fmt.Fprintln(out)
}

func printActionLogHeader(out io.Writer, s style, label string) {
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s\n\n", s.section("Activity Log"), s.dim(label))
}

type kv struct {
	key   string
	value string
}

func printKVList(out io.Writer, s style, items []kv) {
	keyWidth := 0
	for _, item := range items {
		keyWidth = max(keyWidth, len(item.key))
	}
	keyWidth = min(keyWidth, 14)
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
		w.host = min(max(w.host, len(row.Host)), 30)
		w.ip = min(max(w.ip, len(row.DiscoveryIP)), 18)
	}
	used := w.host + w.ip + w.ready + w.sudo + w.denied + w.probe + 12
	w.remediation = max(16, width-used)
	return w
}

func findItem(items []menuItem, key string) (menuItem, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return menuItem{}, false
}

func showValidatedIPs(rt *app.Runtime) error {
	snap := rt.Snapshot()
	if len(snap.ValidatedIPs) == 0 {
		fmt.Fprintln(rt.Out, "No validated discovery IPs yet. Run validate first.")
		return nil
	}
	fmt.Fprintln(rt.Out, "Validated discovery IPs:")
	for _, ip := range snap.ValidatedIPs {
		fmt.Fprintf(rt.Out, "  %s\n", ip)
	}
	return nil
}

func statusValue(s style, ok bool, good string, bad string) string {
	if ok {
		return s.ok(good)
	}
	return s.bad(bad)
}

func workflowState(s style, done bool) string {
	if done {
		return s.ok("ready")
	}
	return s.dim("pending")
}

func reportStatus(s style, err string) string {
	if err == "" {
		return s.ok("Ready")
	}
	return s.warn("Pending")
}

func readyText(s style, value string) string {
	if strings.EqualFold(value, "YES") || strings.EqualFold(value, "OK") {
		return s.ok(value)
	}
	if strings.EqualFold(value, "NO") || strings.EqualFold(value, "FAIL") {
		return s.bad(value)
	}
	return value
}

func pause(reader *bufio.Reader, out io.Writer, s style) {
	fmt.Fprint(out, s.dim("\nPress Enter to return to the dashboard..."))
	_, _ = reader.ReadString('\n')
}

func truncate(text string, width int) string {
	text = strings.TrimSpace(text)
	if len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

func wrapLine(text string, width int) string {
	width = max(40, width)
	if len(text) <= width {
		return text
	}
	var lines []string
	remaining := text
	for len(remaining) > width {
		cut := strings.LastIndex(remaining[:width], " ")
		if cut < 24 {
			cut = width
		}
		lines = append(lines, strings.TrimSpace(remaining[:cut]))
		remaining = strings.TrimSpace(remaining[cut:])
	}
	if remaining != "" {
		lines = append(lines, remaining)
	}
	return strings.Join(lines, "\n")
}

func terminalWidth() int {
	if raw := os.Getenv("COLUMNS"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return min(max(value, 50), 160)
		}
	}
	return 100
}

func shouldUseColor(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func clearIfInteractive(out io.Writer) {
	file, ok := out.(*os.File)
	if !ok {
		return
	}
	info, err := file.Stat()
	if err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
		fmt.Fprint(out, "\033[H\033[2J")
	}
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s style) title(text string) string {
	if !s.enabled {
		return text
	}
	return "\033[1;38;5;81m" + text + "\033[0m"
}

func (s style) section(text string) string {
	if !s.enabled {
		return text
	}
	return "\033[1m" + text + "\033[0m"
}

func (s style) ok(text string) string {
	if !s.enabled {
		return text
	}
	return "\033[32m" + text + "\033[0m"
}

func (s style) bad(text string) string {
	if !s.enabled {
		return text
	}
	return "\033[31m" + text + "\033[0m"
}

func (s style) warn(text string) string {
	if !s.enabled {
		return text
	}
	return "\033[33m" + text + "\033[0m"
}

func (s style) dim(text string) string {
	if !s.enabled {
		return text
	}
	return "\033[2m" + text + "\033[0m"
}

func (s style) key(text string) string {
	if !s.enabled {
		return text
	}
	return "\033[1;38;5;117m" + text + "\033[0m"
}
