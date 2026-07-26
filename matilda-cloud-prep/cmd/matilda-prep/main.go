package main

import (
	"os"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cli"
)

func main() {
	os.Exit(cli.RunWithInput(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
