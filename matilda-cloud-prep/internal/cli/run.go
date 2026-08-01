package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/bootstrap"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/guided"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	ExitOK             = 0
	ExitFailed         = 1
	ExitUsage          = 2
	ExitNotImplemented = 3
	ExitBlocked        = 4
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), stdout, stderr)
}

func RunWithInput(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	registry := bootstrap.DefaultRegistry()
	return runWithRuntime(args, stdin, stdout, stderr, registry, bootstrap.DefaultGuidedConfig(registry))
}

func RunWithRegistry(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, registry workflow.Registry) int {
	return runWithRuntime(args, stdin, stdout, stderr, registry, guided.Config{Registry: registry})
}

func runWithRuntime(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, registry workflow.Registry, guidedConfig guided.Config) int {
	if len(args) == 0 {
		writeError(stderr, "usage: expected matilda-prep start, rapid-assessment, deep-discovery, --help, or --version")
		return ExitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		writeHelp(stdout)
		return ExitOK
	case "--version", "version":
		fmt.Fprintln(stdout, "matilda-prep dev")
		return ExitOK
	case "start":
		if len(args) != 1 {
			writeError(stderr, "usage: matilda-prep start")
			return ExitUsage
		}
		if err := guided.RunWithConfig(stdin, stdout, guidedConfig); err != nil {
			writeError(stderr, err.Error())
			return ExitUsage
		}
		return ExitOK
	}

	if assessment.IsProvider(args[0]) {
		writeProviderFirstCorrection(stderr, args[0])
		return ExitUsage
	}

	request, options, helpRequested, err := parseDirectCommand(args)
	if err != nil {
		writeError(stderr, err.Error())
		return ExitUsage
	}

	if helpRequested {
		writeActionHelp(stdout, request)
		return ExitOK
	}

	ctx := context.Background()
	if options.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(options.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	result := registry.ExecuteContext(ctx, request, options)
	if err := writeJSON(stdout, result); err != nil {
		writeError(stderr, err.Error())
		return ExitUsage
	}

	switch result.Status {
	case workflow.StatusNotImplemented:
		return ExitNotImplemented
	case workflow.RunStatusBlocked:
		return ExitBlocked
	case workflow.RunStatusFailed:
		return ExitFailed
	default:
		return ExitOK
	}
}
