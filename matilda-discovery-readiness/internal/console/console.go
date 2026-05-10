package console

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/ui"
)

func Run(rt *app.Runtime) error {
	if !isInteractive(rt.In, rt.Out) {
		PrintStatus(rt)
		printStaticActions(rt.Out, ui.NewStyle(rt.Out), app.WorkflowActions())
		return nil
	}

	model := NewModel(rt)
	program := tea.NewProgram(
		model,
		tea.WithInput(rt.In),
		tea.WithOutput(rt.Out),
		tea.WithWindowSize(ui.TerminalWidth(), 34),
	)
	_, err := program.Run()
	return err
}

func PrintStatus(rt *app.Runtime) {
	style := ui.NewStyle(rt.Out)
	fmt.Fprint(rt.Out, renderPlainStatus(style, rt.Snapshot()))
}

func isInteractive(in io.Reader, out io.Writer) bool {
	return isTerminal(in) && isTerminal(out)
}

func isTerminal(value any) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}
