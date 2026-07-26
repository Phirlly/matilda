package coveragegate

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const (
	ExitOK = iota
	ExitGateFailed
	ExitUsage
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("coveragegate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var profile string
	var minimumPercent float64
	flags.StringVar(&profile, "profile", "", "Go coverage profile path")
	flags.Float64Var(&minimumPercent, "min", DefaultMinimumPercent, "minimum coverage percentage")

	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "invalid coverage gate arguments: %v\n", err)
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected coverage gate argument %q\n", flags.Arg(0))
		return ExitUsage
	}
	if profile == "" {
		fmt.Fprintln(stderr, "-profile is required")
		return ExitUsage
	}
	if err := validateMinimum(minimumPercent); err != nil {
		fmt.Fprintf(stderr, "invalid coverage gate arguments: %v\n", err)
		return ExitUsage
	}

	summary, err := EvaluateFile(profile, minimumPercent)
	if err != nil {
		var below BelowMinimumError
		if errors.As(err, &below) {
			writeGateResult(stderr, below.Summary, below.MinimumPercent, "is below required")
			return ExitGateFailed
		}

		fmt.Fprintf(stderr, "coverage gate failed: %v\n", err)
		return ExitGateFailed
	}

	writeGateResult(stdout, summary, minimumPercent, "meets required")
	return ExitOK
}

func writeGateResult(output io.Writer, summary Summary, minimumPercent float64, status string) {
	fmt.Fprintf(
		output,
		"coverage %.2f%% %s %.2f%% (%d/%d statements)\n",
		summary.Percent,
		status,
		minimumPercent,
		summary.CoveredStatements,
		summary.TotalStatements,
	)
}
