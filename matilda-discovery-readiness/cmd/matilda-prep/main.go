package main

import (
	"errors"
	"os"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/cli"
	"matilda-discovery-readiness/internal/ui"
)

func main() {
	if err := cli.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, app.ErrCancelled) {
			os.Exit(cli.ExitCode(err))
		}
		ui.New(os.Stderr).Error("Action failed", err.Error(), "Run ./matilda-prep help for available actions, or open ./matilda-prep for the guided console.")
		os.Exit(cli.ExitCode(err))
	}
}
