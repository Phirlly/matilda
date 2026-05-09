package app

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"matilda-discovery-readiness/internal/runner"
)

type terminalStyle struct {
	color bool
	width int
}

func heading(out io.Writer, title string, subtitle string) {
	style := newTerminalStyle(out)
	fmt.Fprintln(out, style.title("Matilda Discovery Readiness"))
	fmt.Fprintf(out, "%-8s %s\n", "Command", titleCase(title))
	if subtitle != "" {
		fmt.Fprintf(out, "%-8s %s\n", "Scope", subtitle)
	}
	fmt.Fprintln(out)
}

func section(out io.Writer, title string) {
	fmt.Fprintln(out, newTerminalStyle(out).section(title))
}

func nextLine(out io.Writer, text string) {
	fmt.Fprintln(out)
	section(out, "Next")
	fmt.Fprintf(out, "  %s\n", text)
}

func successLine(out io.Writer, text string) {
	fmt.Fprintln(out)
	section(out, "Done")
	fmt.Fprintf(out, "  %s\n", text)
}

func cancelledLine(out io.Writer, text string) {
	fmt.Fprintln(out)
	section(out, "Cancelled")
	fmt.Fprintf(out, "  %s\n", text)
}

func printItems(out io.Writer, items []string) {
	for _, item := range items {
		fmt.Fprintf(out, "  - %s\n", item)
	}
}

func printChecks(out io.Writer, results []runner.Result) {
	style := newTerminalStyle(out)
	nameWidth := 18
	for _, result := range results {
		nameWidth = maxInt(nameWidth, len(result.Name))
	}
	nameWidth = minInt(nameWidth, maxInt(18, style.width-38))

	for _, result := range results {
		name := clip(result.Name, nameWidth)
		fmt.Fprintf(out, "  %s  %-*s  %s\n", style.status(result.Status), nameWidth, name, result.Detail)
	}
}

func titleCase(text string) string {
	switch text {
	case "INIT":
		return "Init"
	case "DOCTOR":
		return "Doctor"
	case "INVENTORY VALIDATE":
		return "Inventory Validate"
	case "INVENTORY IMPORT":
		return "Inventory Import"
	case "INVENTORY MIGRATE":
		return "Inventory Migrate"
	case "PREFLIGHT":
		return "Preflight"
	case "SETUP":
		return "Setup"
	case "VALIDATE":
		return "Validate"
	case "RUN":
		return "Run"
	case "REPORT":
		return "Report"
	case "GENERATE":
		return "Generate"
	case "ROLLBACK":
		return "Rollback"
	default:
		return text
	}
}

func newTerminalStyle(out io.Writer) terminalStyle {
	return terminalStyle{color: terminalColor(out), width: commandWidth()}
}

func commandWidth() int {
	if raw := os.Getenv("COLUMNS"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return minInt(maxInt(value, 64), 160)
		}
	}
	return 100
}

func terminalColor(out io.Writer) bool {
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

func clip(text string, width int) string {
	text = strings.TrimSpace(text)
	if len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s terminalStyle) title(text string) string {
	if !s.color {
		return text
	}
	return "\033[1;38;5;81m" + text + "\033[0m"
}

func (s terminalStyle) section(text string) string {
	if !s.color {
		return text
	}
	return "\033[1m" + text + "\033[0m"
}

func (s terminalStyle) status(text string) string {
	if !s.color {
		return text
	}
	switch text {
	case runner.StatusPass:
		return "\033[32m" + text + "\033[0m"
	case runner.StatusFail:
		return "\033[31m" + text + "\033[0m"
	case runner.StatusSkip:
		return "\033[33m" + text + "\033[0m"
	default:
		return text
	}
}
