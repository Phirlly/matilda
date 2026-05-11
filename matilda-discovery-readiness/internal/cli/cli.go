package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/console"
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
		return console.Run(rt)
	}

	switch args[0] {
	case "help", "-h", "--help":
		console.PrintHelp(out)
		return nil
	case "console", "start":
		return console.Run(rt)
	case "status":
		console.PrintStatus(rt)
		return nil
	case "init":
		return rt.RunTrackedAction("init", rt.Init)
	case "doctor":
		return rt.RunTrackedAction("doctor", rt.Doctor)
	case "inventory":
		return inventoryCommand(rt, args[1:])
	case "preflight":
		return rt.RunTrackedAction("preflight", rt.Preflight)
	case "setup", "apply":
		return rt.RunTrackedAction("setup", rt.Setup)
	case "validate":
		return rt.RunTrackedAction("validate", rt.Validate)
	case "run":
		return rt.RunTrackedAction("run", rt.Run)
	case "report":
		return rt.RunTrackedAction("report", rt.Report)
	case "generate":
		action := generateAction(args[1:])
		if action == "" {
			return rt.Generate(args[1:])
		}
		return rt.RunTrackedAction(action, func() error { return rt.Generate(args[1:]) })
	case "ui":
		return web.Serve(rt, args[1:])
	case "rollback":
		action := rollbackAction(args[1:])
		if action == "" {
			return rt.Rollback(args[1:])
		}
		return rt.RunTrackedAction(action, func() error { return rt.Rollback(args[1:]) })
	default:
		console.PrintHelp(errOut)
		return fmt.Errorf("%w: unknown command %q", errUsage, args[0])
	}
}

func inventoryCommand(rt *app.Runtime, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		console.PrintInventoryHelp(rt.Out)
		return nil
	}

	switch args[0] {
	case "validate":
		return rt.RunTrackedAction("inventory-validate", rt.InventoryValidate)
	case "import":
		if len(args) < 2 {
			return fmt.Errorf("%w: inventory import requires a CSV path", errUsage)
		}
		return rt.RunTrackedAction("inventory-import", func() error { return rt.InventoryImport(args[1]) })
	default:
		console.PrintInventoryHelp(rt.Err)
		return fmt.Errorf("%w: unknown inventory command %q", errUsage, args[0])
	}
}

func generateAction(args []string) string {
	if len(args) == 0 || isHelpArg(args[0]) {
		return ""
	}
	return "generate-" + strings.ToLower(args[0])
}

func rollbackAction(args []string) string {
	if len(args) == 0 {
		return "rollback"
	}
	for _, arg := range args {
		if isHelpArg(arg) {
			return ""
		}
	}
	if len(args) == 1 {
		switch args[0] {
		case "--sudoers-only":
			return "rollback-sudoers"
		case "--remove-key":
			return "rollback-remove-key"
		case "--lock-user":
			return "rollback-lock-user"
		case "--delete-user":
			return "rollback-delete-user"
		}
	}
	return "rollback"
}

func isHelpArg(arg string) bool {
	return arg == "help" || arg == "-h" || arg == "--help"
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
