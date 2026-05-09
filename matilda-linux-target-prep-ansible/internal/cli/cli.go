package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/tui"
	"matilda-discovery-readiness/internal/web"
)

var errUsage = errors.New("usage error")

func Execute(args []string, in io.Reader, out io.Writer, errOut io.Writer) error {
	root, err := findProjectRoot()
	if err != nil {
		return err
	}

	rt := app.New(root, in, out, errOut)
	if len(args) == 0 {
		args = []string{"help"}
	}

	switch args[0] {
	case "help", "-h", "--help":
		printHelp(out)
		return nil
	case "init":
		return rt.Init()
	case "doctor":
		return rt.Doctor()
	case "inventory":
		return inventoryCommand(rt, args[1:])
	case "preflight":
		return rt.Preflight()
	case "setup", "apply":
		return rt.Setup()
	case "validate":
		return rt.Validate()
	case "run":
		return rt.Run()
	case "report":
		return rt.Report()
	case "generate":
		return rt.Generate(args[1:])
	case "tui":
		return tui.Run(rt)
	case "ui":
		return web.Serve(rt, args[1:])
	case "rollback":
		return rt.Rollback(args[1:])
	default:
		printHelp(errOut)
		return fmt.Errorf("%w: unknown command %q", errUsage, args[0])
	}
}

func inventoryCommand(rt *app.Runtime, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printInventoryHelp(rt.Out)
		return nil
	}

	switch args[0] {
	case "validate":
		return rt.InventoryValidate()
	case "import":
		if len(args) < 2 {
			return fmt.Errorf("%w: inventory import requires a CSV path", errUsage)
		}
		return rt.InventoryImport(args[1])
	case "migrate":
		return rt.InventoryMigrate()
	default:
		printInventoryHelp(rt.Err)
		return fmt.Errorf("%w: unknown inventory command %q", errUsage, args[0])
	}
}

func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, app.ErrCancelled):
		return 2
	case errors.Is(err, errUsage):
		return 64
	default:
		return 1
	}
}

func findProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if exists(filepath.Join(wd, "README.md")) && (exists(filepath.Join(wd, "ansible.cfg")) || exists(filepath.Join(wd, "ansible", "ansible.cfg"))) {
			return wd, nil
		}
		next := filepath.Dir(wd)
		if next == wd {
			break
		}
		wd = next
	}

	return "", errors.New("could not find project root; run matilda-prep from the repository")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, strings.TrimSpace(`
Matilda Discovery Readiness Toolkit

Usage:
  ./matilda-prep <command>

Commands:
  help                  Show this help message
  init                  Create local .env and inventory.yml safely
  doctor                Check local prerequisites and project health
  inventory validate    Validate inventory shape before running Ansible
  inventory import CSV  Import Linux targets from CSV into inventory.yml
  inventory migrate     Convert current inventory.yml to inventory.v1.yml
  preflight             Run read-only Linux readiness checks
  setup                 Configure Linux targets for Matilda Discovery
  apply                 Alias for setup
  validate              Validate targets and generate readiness reports
  run                   Run preflight, setup, validate, report
  report                Generate CSV, JSON, Markdown, and HTML reports
  generate TARGET       Generate Windows readiness package or UNIX admin instructions
  tui                   Guided terminal workflow
  ui                    Start local browser UI
  rollback MODE         Run one explicit Linux rollback mode

Recommended workflow:
  ./matilda-prep init
  ./matilda-prep doctor
  ./matilda-prep inventory validate
  ./matilda-prep preflight
  ./matilda-prep setup
  ./matilda-prep validate
  ./matilda-prep report

Generated readiness guidance:
  ./matilda-prep generate windows
  ./matilda-prep generate unix

Rollback modes:
  ./matilda-prep rollback --sudoers-only
  ./matilda-prep rollback --remove-key
  ./matilda-prep rollback --lock-user
  ./matilda-prep rollback --delete-user

Notes:
  - setup modifies target systems and asks for confirmation
  - preflight and inventory validation are read-only
  - docs/matilda-docs-cache is local reference only
  - private keys must not be copied to target systems
`))
}

func printInventoryHelp(out io.Writer) {
	fmt.Fprintln(out, strings.TrimSpace(`
Inventory commands:
  ./matilda-prep inventory validate
  ./matilda-prep inventory import targets.csv
  ./matilda-prep inventory migrate

CSV import columns:
  hostname,platform,os_family,ansible_host,discovery_ip,access_path,privilege_method

Optional CSV columns:
  public_ip,private_ip,cloud_provider

Supported access_path values for current Linux automation:
  direct
  via_probe
`))
}
