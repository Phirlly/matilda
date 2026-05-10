package app

import (
	"io"

	"matilda-discovery-readiness/internal/runner"
	"matilda-discovery-readiness/internal/ui"
)

func heading(out io.Writer, title string, subtitle string) {
	ui.New(out).Header(titleCase(title), subtitle)
}

func section(out io.Writer, title string) {
	ui.New(out).Section(title)
}

func nextLine(out io.Writer, text string) {
	ui.New(out).Next(text)
}

func successLine(out io.Writer, text string) {
	ui.New(out).Done(text)
}

func cancelledLine(out io.Writer, text string) {
	ui.New(out).Cancelled(text)
}

func printItems(out io.Writer, items []string) {
	ui.New(out).Items(items)
}

func printChecks(out io.Writer, results []runner.Result) {
	ui.New(out).Checks(results)
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
