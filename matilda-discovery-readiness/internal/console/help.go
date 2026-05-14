package console

import (
	"fmt"
	"io"
	"strings"

	"matilda-discovery-readiness/internal/ui"
)

type helpRow struct {
	Command string
	Detail  string
}

func PrintHelp(out io.Writer) {
	s := ui.NewStyle(out)
	fmt.Fprintln(out)
	fmt.Fprintln(out, s.Title("Matilda Discovery Readiness"))
	fmt.Fprintln(out, s.Dim("Target, Probe, and platform readiness automation"))
	fmt.Fprintln(out)

	printHelpSection(out, s, "Start Here", []helpRow{
		{Command: "./matilda-prep", Detail: "open the Matilda Terminal Console"},
		{Command: "./matilda-prep status", Detail: "print status and exit"},
		{Command: "./matilda-prep help", Detail: "show this command guide"},
	})
	fmt.Fprintln(out)
	printHelpSection(out, s, "Local Checks", []helpRow{
		{Command: "./matilda-prep init", Detail: "create local .env and targets.csv safely"},
		{Command: "./matilda-prep doctor", Detail: "check local prerequisites and project health"},
		{Command: "./matilda-prep inventory validate", Detail: "validate target CSV before Ansible runs"},
		{Command: "./matilda-prep inventory import CSV", Detail: "copy a Linux target CSV into targets.csv"},
	})
	fmt.Fprintln(out)
	printHelpSection(out, s, "Linux Readiness", []helpRow{
		{Command: "./matilda-prep preflight", Detail: "run read-only target and Probe checks"},
		{Command: "./matilda-prep setup", Detail: "configure Linux targets; asks for confirmation"},
		{Command: "./matilda-prep validate", Detail: "validate readiness and write reports"},
		{Command: "./matilda-prep run", Detail: "run preflight, setup, validate, and report"},
		{Command: "./matilda-prep rollback MODE", Detail: "run one explicit rollback mode"},
	})
	fmt.Fprintln(out)
	printHelpSection(out, s, "Reports And Guidance", []helpRow{
		{Command: "./matilda-prep report", Detail: "generate CSV, JSON, Markdown, and HTML reports"},
		{Command: "./matilda-prep generate TARGET", Detail: "generate Windows readiness package or UNIX admin instructions"},
		{Command: "./matilda-prep generate windows", Detail: "write local Windows readiness package"},
		{Command: "./matilda-prep generate unix", Detail: "write local UNIX admin instructions"},
	})
	fmt.Fprintln(out)
	printHelpSection(out, s, "Interfaces", []helpRow{
		{Command: "./matilda-prep console", Detail: "open the Matilda Terminal Console"},
		{Command: "./matilda-prep start", Detail: "open the Matilda Terminal Console"},
		{Command: "./matilda-prep ui", Detail: "start the local browser UI"},
	})
	fmt.Fprintln(out)
	printHelpSection(out, s, "Rollback Modes", []helpRow{
		{Command: "--sudoers-only", Detail: "remove the Matilda sudoers drop-in"},
		{Command: "--remove-key", Detail: "remove the Matilda public key from authorized_keys"},
		{Command: "--lock-user", Detail: "lock the matilda-svc account"},
		{Command: "--delete-user", Detail: "remove the matilda-svc account and home directory"},
	})
	fmt.Fprintln(out)
	printHelpSection(out, s, "Safety", []helpRow{
		{Command: "setup", Detail: "modifies target systems and asks before continuing"},
		{Command: "preflight", Detail: "read-only"},
		{Command: "inventory validate", Detail: "read-only"},
		{Command: "private keys", Detail: "must not be copied to target systems"},
	})
}

func PrintInventoryHelp(out io.Writer) {
	s := ui.NewStyle(out)
	fmt.Fprintln(out)
	fmt.Fprintln(out, s.Title("Matilda Discovery Readiness"))
	fmt.Fprintln(out, s.Dim("Inventory commands"))
	fmt.Fprintln(out)

	printHelpSection(out, s, "Commands", []helpRow{
		{Command: "./matilda-prep inventory validate", Detail: "check targets.csv shape and required target fields"},
		{Command: "./matilda-prep inventory import CSV", Detail: "copy a CSV into the local targets.csv source"},
	})
	fmt.Fprintln(out)
	printHelpSection(out, s, "Required CSV Columns", []helpRow{
		{Command: "hostname", Detail: "inventory host name"},
		{Command: "platform", Detail: "current import supports linux"},
		{Command: "ansible_host", Detail: "address used by Ansible"},
		{Command: "discovery_ip", Detail: "address used by MatildaProbeVM"},
		{Command: "access_path", Detail: "direct or via_probe"},
		{Command: "privilege_method", Detail: "sudo for current Linux automation"},
	})
	fmt.Fprintln(out)
	printHelpSection(out, s, "Optional CSV Columns", []helpRow{
		{Command: "os_family", Detail: "target OS family"},
		{Command: "public_ip", Detail: "public address when available"},
		{Command: "private_ip", Detail: "private address when available"},
		{Command: "cloud_provider", Detail: "provider label such as oci, aws, azure, or gcp"},
		{Command: "configure_mode", Detail: "defaults to remote"},
		{Command: "admin_user", Detail: "per-target SSH user override"},
		{Command: "admin_private_key_file", Detail: "per-target SSH private key path override"},
	})
}

func printHelpSection(out io.Writer, s ui.Style, title string, rows []helpRow) {
	fmt.Fprintln(out, s.Section(title))
	width := 0
	for _, row := range rows {
		width = ui.Max(width, len(row.Command))
	}
	width = ui.Min(width, ui.Max(18, ui.Min(52, s.Width/2)))
	for _, row := range rows {
		detailWidth := ui.Max(16, s.Width-width-6)
		lines := strings.Split(ui.Wrap(row.Detail, detailWidth), "\n")
		fmt.Fprintf(out, "  %-*s  %s\n", width, ui.Truncate(row.Command, width), s.Dim(lines[0]))
		for _, line := range lines[1:] {
			fmt.Fprintf(out, "  %-*s  %s\n", width, "", s.Dim(line))
		}
	}
}
