package main

import (
	"os"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
