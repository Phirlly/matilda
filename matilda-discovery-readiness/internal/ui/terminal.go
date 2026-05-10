package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"matilda-discovery-readiness/internal/runner"
)

type Style struct {
	Enabled bool
	Width   int
}

type Renderer struct {
	Out   io.Writer
	Style Style
}

type KV struct {
	Key   string
	Value string
}

func New(out io.Writer) Renderer {
	return Renderer{Out: out, Style: NewStyle(out)}
}

func NewStyle(out io.Writer) Style {
	return Style{Enabled: ShouldUseColor(out), Width: TerminalWidth()}
}

func (r Renderer) Header(title string, subtitle string) {
	fmt.Fprintln(r.Out)
	fmt.Fprintln(r.Out, r.Style.Title("Matilda Discovery Readiness"))
	if title != "" {
		fmt.Fprintln(r.Out, r.Style.Emphasis(title))
	}
	if subtitle != "" {
		fmt.Fprintln(r.Out, r.Style.Dim(Wrap(subtitle, r.Style.Width)))
	}
	fmt.Fprintln(r.Out)
}

func (r Renderer) Section(title string) {
	fmt.Fprintln(r.Out, r.Style.Section(title))
}

func (r Renderer) KeyValues(items []KV) {
	keyWidth := 0
	for _, item := range items {
		keyWidth = Max(keyWidth, len(item.Key))
	}
	keyWidth = Min(keyWidth, Max(18, Min(46, r.Style.Width/2)))
	for _, item := range items {
		fmt.Fprintf(r.Out, "  %-*s  %s\n", keyWidth, item.Key, item.Value)
	}
}

func (r Renderer) Checks(results []runner.Result) {
	nameWidth := 18
	for _, result := range results {
		nameWidth = Max(nameWidth, len(result.Name))
	}
	nameWidth = Min(nameWidth, Max(18, r.Style.Width-38))

	for _, result := range results {
		name := Truncate(result.Name, nameWidth)
		fmt.Fprintf(r.Out, "  %s  %-*s  %s\n", r.Style.Status(result.Status), nameWidth, name, result.Detail)
	}
}

func (r Renderer) Items(items []string) {
	for _, item := range items {
		fmt.Fprintf(r.Out, "  - %s\n", item)
	}
}

func (r Renderer) Files(paths []string) {
	for _, path := range paths {
		fmt.Fprintf(r.Out, "  wrote  %s\n", path)
	}
}

func (r Renderer) Done(text string) {
	fmt.Fprintln(r.Out)
	r.Section("Done")
	fmt.Fprintf(r.Out, "  %s\n", text)
}

func (r Renderer) Cancelled(text string) {
	fmt.Fprintln(r.Out)
	r.Section("Cancelled")
	fmt.Fprintf(r.Out, "  %s\n", text)
}

func (r Renderer) Next(text string) {
	fmt.Fprintln(r.Out)
	r.Section("Next")
	fmt.Fprintf(r.Out, "  %s\n", Wrap(text, r.Style.Width-4))
}

func (r Renderer) Warning(text string) {
	fmt.Fprintf(r.Out, "  %s %s\n", r.Style.Warn("WARN"), text)
}

func (r Renderer) Error(title string, message string, next string) {
	fmt.Fprintln(r.Out)
	fmt.Fprintln(r.Out, r.Style.Bad(title))
	if message != "" {
		fmt.Fprintf(r.Out, "  %s\n", Wrap(message, r.Style.Width-4))
	}
	if next != "" {
		r.Next(next)
	}
}

func (r Renderer) Prompt(reader *bufio.Reader, prompt string, def string) string {
	if def != "" {
		fmt.Fprintf(r.Out, "  %s [%s]: ", prompt, def)
	} else {
		fmt.Fprintf(r.Out, "  %s: ", prompt)
	}
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		return def
	}
	return text
}

func (r Renderer) Confirm(reader *bufio.Reader, prompt string) bool {
	fmt.Fprintf(r.Out, "  %s [y/N]: ", prompt)
	answer, _ := reader.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func TerminalWidth() int {
	if raw := os.Getenv("COLUMNS"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return Min(Max(value, 50), 160)
		}
	}
	return 100
}

func ShouldUseColor(out io.Writer) bool {
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

func ClearIfInteractive(out io.Writer) {
	file, ok := out.(*os.File)
	if !ok {
		return
	}
	info, err := file.Stat()
	if err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
		fmt.Fprint(out, "\033[H\033[2J")
	}
}

func Truncate(text string, width int) string {
	text = strings.TrimSpace(text)
	if len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

func Wrap(text string, width int) string {
	width = Max(40, width)
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

func Min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func Max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s Style) Title(text string) string {
	if !s.Enabled {
		return text
	}
	return "\033[1;38;5;81m" + text + "\033[0m"
}

func (s Style) Emphasis(text string) string {
	if !s.Enabled {
		return text
	}
	return "\033[1m" + text + "\033[0m"
}

func (s Style) Section(text string) string {
	if !s.Enabled {
		return text
	}
	return "\033[1m" + text + "\033[0m"
}

func (s Style) OK(text string) string {
	if !s.Enabled {
		return text
	}
	return "\033[32m" + text + "\033[0m"
}

func (s Style) Bad(text string) string {
	if !s.Enabled {
		return text
	}
	return "\033[31m" + text + "\033[0m"
}

func (s Style) Warn(text string) string {
	if !s.Enabled {
		return text
	}
	return "\033[33m" + text + "\033[0m"
}

func (s Style) Dim(text string) string {
	if !s.Enabled {
		return text
	}
	return "\033[2m" + text + "\033[0m"
}

func (s Style) Key(text string) string {
	if !s.Enabled {
		return text
	}
	return "\033[1;38;5;117m" + text + "\033[0m"
}

func (s Style) Status(text string) string {
	if !s.Enabled {
		return text
	}
	switch text {
	case runner.StatusPass:
		return s.OK(text)
	case runner.StatusFail:
		return s.Bad(text)
	case runner.StatusSkip:
		return s.Warn(text)
	default:
		return text
	}
}
