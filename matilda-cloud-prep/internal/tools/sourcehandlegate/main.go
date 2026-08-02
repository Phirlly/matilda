package main

import (
	"io"
	"os"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/sourcehandlegate"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	return sourcehandlegate.Run(args, stdout, stderr)
}
