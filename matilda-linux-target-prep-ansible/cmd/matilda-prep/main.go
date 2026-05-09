package main

import (
	"errors"
	"fmt"
	"os"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/cli"
)

func main() {
	if err := cli.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, app.ErrCancelled) {
			os.Exit(cli.ExitCode(err))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
