package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"matilda-discovery-readiness/internal/config"
	"matilda-discovery-readiness/internal/inventory"
	"matilda-discovery-readiness/internal/reports"
	"matilda-discovery-readiness/internal/runner"
	"matilda-discovery-readiness/internal/safety"
	"matilda-discovery-readiness/internal/ui"
)

var ErrCancelled = errors.New("operation cancelled")

type Runtime struct {
	Root    string
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Context context.Context
}

func New(root string, in io.Reader, out io.Writer, errOut io.Writer) *Runtime {
	return &Runtime{Root: root, In: in, Out: out, Err: errOut, Context: context.Background()}
}

func (r *Runtime) WithContext(ctx context.Context) *Runtime {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Runtime{Root: r.Root, In: r.In, Out: r.Out, Err: r.Err, Context: ctx}
}

func (r *Runtime) Init() error {
	heading(r.Out, "INIT", "local file setup only; no target changes")
	reader := bufio.NewReader(r.In)
	mode := promptWithReader(reader, r.Out, "Choose init mode: 1) guided wizard  2) copy examples only", "1")

	switch strings.TrimSpace(mode) {
	case "1":
		if err := r.createEnvGuidedWithReader(reader); err != nil {
			return err
		}
		if err := r.createTargetsCSVWithReader(reader); err != nil {
			return err
		}
	case "2":
		if err := copyWithSafetyWithReader(r, reader, filepath.Join(r.Root, "examples", "env.example"), filepath.Join(r.Root, ".env")); err != nil {
			return err
		}
		if err := r.createTargetsCSVWithReader(reader); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid init mode %q", mode)
	}

	fmt.Fprintln(r.Out)
	successLine(r.Out, "Init complete.")
	nextLine(r.Out, "./matilda-prep doctor")
	return nil
}

func (r *Runtime) Doctor() error {
	heading(r.Out, "DOCTOR", "local checks; no target changes")
	checks := []runner.Check{
		runner.FileCheck("targets.csv", r.targetsCSVPath()),
		runner.FileCheck("ansible config", filepath.Join(r.Root, "ansible", "ansible.cfg")),
		runner.FileCheck("linux preflight playbook", filepath.Join(r.Root, "ansible", "playbooks", "linux", "preflight.yml")),
		runner.FileCheck("linux setup playbook", filepath.Join(r.Root, "ansible", "playbooks", "linux", "setup.yml")),
		runner.FileCheck("linux validate playbook", filepath.Join(r.Root, "ansible", "playbooks", "linux", "validate.yml")),
		runner.FileCheck("linux rollback playbook", filepath.Join(r.Root, "ansible", "playbooks", "linux", "rollback.yml")),
		runner.FileCheck("linux sudoers template", filepath.Join(r.Root, "templates", "sudoers", "linux-full-documented.j2")),
		runner.FileCheck("windows readiness template", filepath.Join(r.Root, "templates", "powershell", "windows-readiness.ps1.tmpl")),
		runner.FileCheck("inventory v1 schema", filepath.Join(r.Root, "schemas", "inventory.v1.schema.json")),
		runner.DirCheck("reports", filepath.Join(r.Root, "reports")),
		runner.FileCheck("env example", filepath.Join(r.Root, "examples", "env.example")),
		runner.FileCheck("target CSV example", filepath.Join(r.Root, "examples", "targets.example.csv")),
	}

	failed := false
	goResult := goDoctorCheck()
	results := []runner.Result{goResult}
	if goResult.Status == runner.StatusFail {
		failed = true
	}
	for _, check := range checks {
		result := check.Run()
		results = append(results, result)
		if result.Status == runner.StatusFail {
			failed = true
		}
	}

	for _, check := range []struct {
		name string
		cmd  string
		args []string
	}{
		{name: "ansible-playbook", cmd: "ansible-playbook", args: []string{"--version"}},
		{name: "ansible-doc", cmd: "ansible-doc", args: []string{"--version"}},
	} {
		out, err := runner.RunCapture(r.Root, check.cmd, check.args...)
		if err != nil {
			results = append(results, runner.Result{Name: check.name, Status: runner.StatusFail, Detail: localPrerequisiteDetail(check.name, err)})
			failed = true
		} else {
			results = append(results, runner.Result{Name: check.name, Status: runner.StatusPass, Detail: firstLine(out)})
		}
	}

	envPath := filepath.Join(r.Root, ".env")
	env, envErr := config.LoadEnv(envPath)
	if envErr != nil {
		results = append(results, runner.Result{Name: ".env", Status: runner.StatusSkip, Detail: "missing; run ./matilda-prep init or copy examples/env.example to .env for repeatable runs"})
	} else {
		results = append(results, runner.Result{Name: ".env", Status: runner.StatusPass, Detail: envPath})
	}
	for _, key := range config.RequiredKeys {
		value := strings.TrimSpace(env[key])
		if value == "" {
			results = append(results, runner.Result{Name: ".env " + key, Status: runner.StatusSkip, Detail: "add to .env or answer the terminal prompt when running a remote command"})
			continue
		}
		if config.LooksLikePlaceholder(value) {
			results = append(results, runner.Result{Name: ".env " + key, Status: runner.StatusFail, Detail: "replace placeholder value"})
			failed = true
			continue
		}
		if config.IsLocalFileKey(key) {
			if _, err := os.Stat(config.ExpandPath(value)); err != nil {
				results = append(results, runner.Result{Name: ".env " + key, Status: runner.StatusFail, Detail: "file not found: " + config.ExpandPath(value)})
				failed = true
			} else {
				results = append(results, runner.Result{Name: ".env " + key, Status: runner.StatusPass, Detail: "file exists"})
			}
		} else {
			results = append(results, runner.Result{Name: ".env " + key, Status: runner.StatusPass, Detail: "set"})
		}
	}

	section(r.Out, "Checks")
	printChecks(r.Out, results)
	if failed {
		fmt.Fprintln(r.Out)
		nextLine(r.Out, "Fix failed checks above. If toolkit files are missing, run from the source checkout or extracted release package root.")
		return errors.New("doctor found issues to fix before a seamless run")
	}
	successLine(r.Out, "Local environment looks ready.")
	return nil
}

func goDoctorCheck() runner.Result {
	if _, err := exec.LookPath("go"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return runner.Result{Name: "go", Status: runner.StatusSkip, Detail: "not required when using a packaged or prebuilt matilda-prep binary"}
		}
		return runner.Result{Name: "go", Status: runner.StatusFail, Detail: err.Error()}
	}

	result := runner.CommandCheck("go", "go", "version").Run()
	return result
}

func firstLine(text string) string {
	for i, r := range text {
		if r == '\n' || r == '\r' {
			return text[:i]
		}
	}
	return text
}

func (r *Runtime) InventoryValidate() error {
	return r.validateInventory(true)
}

func (r *Runtime) validateInventory(showHeading bool) error {
	if showHeading {
		heading(r.Out, "INVENTORY VALIDATE", "read-only inventory checks")
	}
	section(r.Out, "Inventory")
	result, targets, err := inventory.ValidateCSVFile(r.targetsCSVPath())
	printChecks(r.Out, result.Checks)
	if err != nil {
		nextLine(r.Out, "Fix targets.csv, then run ./matilda-prep inventory validate again.")
		return err
	}
	generatedPath, err := r.writeGeneratedInventory(targets)
	if err != nil {
		return err
	}
	successLine(r.Out, fmt.Sprintf("Inventory valid: %d target(s) detected.", result.TargetCount))
	nextLine(r.Out, "Generated normalized inventory: "+displayPath(r.Root, generatedPath))
	return nil
}

func (r *Runtime) InventoryImport(csvPath string) error {
	heading(r.Out, "INVENTORY IMPORT", "CSV to targets.csv")
	targets, err := inventory.ReadCSV(csvPath)
	if err != nil {
		section(r.Out, "Inventory")
		printChecks(r.Out, []runner.Result{{Name: "source CSV", Status: runner.StatusFail, Detail: err.Error()}})
		nextLine(r.Out, "Fix the source CSV, then run ./matilda-prep inventory import CSV again.")
		return err
	}
	outPath := r.targetsCSVPath()
	if samePath(csvPath, outPath) {
		generatedPath, err := r.writeGeneratedInventory(targets)
		if err != nil {
			return err
		}
		successLine(r.Out, fmt.Sprintf("Validated %s with %d target(s).", outPath, len(targets)))
		nextLine(r.Out, "Generated normalized inventory: "+displayPath(r.Root, generatedPath))
		nextLine(r.Out, "./matilda-prep inventory validate")
		return nil
	}
	if err := safety.PrepareDestination(r.In, r.Out, outPath); err != nil {
		if errors.Is(err, safety.ErrSkip) {
			successLine(r.Out, "Kept existing targets.csv.")
			return nil
		}
		return err
	}
	content, err := os.ReadFile(csvPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, content, 0600); err != nil {
		return err
	}
	generatedPath, err := r.writeGeneratedInventory(targets)
	if err != nil {
		return err
	}
	successLine(r.Out, fmt.Sprintf("Created %s with %d target(s).", outPath, len(targets)))
	nextLine(r.Out, "Generated normalized inventory: "+displayPath(r.Root, generatedPath))
	nextLine(r.Out, "./matilda-prep inventory validate")
	return nil
}

func (r *Runtime) Preflight() error {
	heading(r.Out, "PREFLIGHT", "read-only Linux readiness checks")
	if err := r.validateInventory(false); err != nil {
		return err
	}
	values, extra, err := r.collectRuntimeValuesAndExtra(config.RequiredKeys)
	if err != nil {
		return err
	}
	inventoryPath, err := r.prepareLinuxRunnerInventory(values)
	if err != nil {
		return err
	}
	section(r.Out, "Ansible")
	return r.runAnsible("ansible/playbooks/linux/preflight.yml", inventoryPath, extra)
}

func (r *Runtime) Setup() error {
	heading(r.Out, "SETUP", "modifies Linux target systems")
	if err := r.checkSetupDependencies(); err != nil {
		return err
	}
	if err := r.validateInventory(false); err != nil {
		return err
	}
	values, extra, err := r.collectRuntimeValuesAndExtra(config.RequiredKeys)
	if err != nil {
		return err
	}
	inventoryPath, err := r.prepareLinuxRunnerInventory(values)
	if err != nil {
		return err
	}

	section(r.Out, "Target Changes")
	printItems(r.Out, []string{
		"create or update matilda-svc",
		"install the Matilda public key",
		"write sudoers configuration",
	})
	fmt.Fprintln(r.Out)
	section(r.Out, "Confirm")
	if !confirm(r.In, r.Out, "Continue with setup?") {
		cancelledLine(r.Out, "Setup cancelled. No target changes were made.")
		return ErrCancelled
	}
	section(r.Out, "Ansible")
	return r.runAnsible("ansible/playbooks/linux/setup.yml", inventoryPath, extra)
}

func (r *Runtime) Validate() error {
	heading(r.Out, "VALIDATE", "Linux readiness validation and reports")
	if err := r.validateInventory(false); err != nil {
		return err
	}
	values, extra, err := r.collectRuntimeValuesAndExtra(config.RequiredKeys)
	if err != nil {
		return err
	}
	inventoryPath, err := r.prepareLinuxRunnerInventory(values)
	if err != nil {
		return err
	}
	section(r.Out, "Ansible")
	runErr := r.runAnsible("ansible/playbooks/linux/validate.yml", inventoryPath, extra)
	reportErr := r.generateReport(false)
	if runErr != nil {
		if reportErr != nil {
			return fmt.Errorf("%v; additionally report generation failed: %w", runErr, reportErr)
		}
		return runErr
	}
	return reportErr
}

func (r *Runtime) Run() error {
	heading(r.Out, "RUN", "preflight -> setup -> validate -> report")
	if err := r.Preflight(); err != nil {
		return err
	}
	if err := r.Setup(); err != nil {
		return err
	}
	return r.Validate()
}

func (r *Runtime) Report() error {
	return r.generateReport(true)
}

func (r *Runtime) generateReport(showHeading bool) error {
	if showHeading {
		heading(r.Out, "REPORT", "generate readiness report files")
	}
	section(r.Out, "Report Files")
	paths, err := reports.Generate(filepath.Join(r.Root, "reports"))
	if err != nil {
		return err
	}
	ui.New(r.Out).Files(r.displayPaths(paths))
	successLine(r.Out, "Readiness reports generated.")
	nextLine(r.Out, "Open reports/readiness.html or use the validated discovery IPs when creating the Matilda discovery task.")
	return nil
}

func (r *Runtime) Generate(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		PrintGenerateHelp(r.Out)
		return nil
	}

	platform := strings.ToLower(args[0])
	if platform != "windows" && platform != "unix" {
		return fmt.Errorf("unsupported generate target %q; use windows or unix", platform)
	}

	heading(r.Out, "GENERATE", "local readiness guidance only; no target changes")
	section(r.Out, "Artifacts")
	var paths []string
	switch platform {
	case "windows":
		dir := filepath.Join(r.Root, "reports", "guidance", "windows")
		scriptTemplate := filepath.Join(r.Root, "templates", "powershell", "windows-readiness.ps1.tmpl")
		content, err := os.ReadFile(scriptTemplate)
		if err != nil {
			return err
		}
		scriptPath := filepath.Join(dir, "windows-readiness.ps1")
		readmePath := filepath.Join(dir, "README.md")
		if err := writeArtifact(scriptPath, content, 0644); err != nil {
			return err
		}
		if err := writeArtifact(readmePath, []byte(windowsReadinessPackageReadme()), 0644); err != nil {
			return err
		}
		paths = append(paths, scriptPath, readmePath)
	case "unix":
		dir := filepath.Join(r.Root, "reports", "guidance", "unix")
		path := filepath.Join(dir, "unix-readiness.md")
		if err := writeArtifact(path, []byte(unixAdminInstructionsReadme()), 0644); err != nil {
			return err
		}
		paths = append(paths, path)
	}
	ui.New(r.Out).Files(r.displayPaths(paths))
	successLine(r.Out, "Platform guidance generated locally.")
	nextLine(r.Out, "Review the generated files with the platform owner before any customer-managed changes.")
	return nil
}

func (r *Runtime) Rollback(args []string) error {
	mode, err := parseRollbackMode(args)
	if err != nil {
		return err
	}
	if mode == "help" {
		PrintRollbackHelp(r.Out)
		return nil
	}

	heading(r.Out, "ROLLBACK", "explicit Linux rollback mode; modifies target systems")
	if err := r.validateInventory(false); err != nil {
		return err
	}
	keys := r.connectionKeys()
	if mode == "remove_key" {
		keys = append(keys, "MATILDA_PUBLIC_KEY_FILE")
	}
	values, extra, err := r.collectRuntimeValuesAndExtra(keys)
	if err != nil {
		return err
	}
	inventoryPath, err := r.prepareLinuxRunnerInventory(values)
	if err != nil {
		return err
	}
	extra = append(extra, "--extra-vars", "matilda_rollback_mode="+mode)

	section(r.Out, "Target Changes")
	changes := []string{
		fmt.Sprintf("rollback mode: %s", mode),
		"remove Matilda readiness configuration from Linux targets",
	}
	if mode == "delete_user" {
		changes = append(changes, "delete-user removes the service account and its home directory")
	}
	printItems(r.Out, changes)
	fmt.Fprintln(r.Out)
	section(r.Out, "Confirm")
	if !confirm(r.In, r.Out, "Continue with rollback?") {
		cancelledLine(r.Out, "Rollback cancelled. No target changes were made.")
		return ErrCancelled
	}
	section(r.Out, "Ansible")
	return r.runAnsible("ansible/playbooks/linux/rollback.yml", inventoryPath, extra)
}

func (r *Runtime) runAnsible(playbook string, inventoryPath string, extra []string) error {
	if err := requireLocalCommand("ansible-playbook", "remote workflows"); err != nil {
		return err
	}
	var args []string
	if strings.TrimSpace(inventoryPath) != "" {
		args = append(args, "-i", inventoryPath)
	}
	args = append(args, playbook)
	args = append(args, extra...)
	return runner.RunStreamContext(r.Context, r.Root, r.Out, r.Err, "ansible-playbook", args...)
}

func (r *Runtime) prepareLinuxRunnerInventory(values map[string]string) (string, error) {
	normalizedPath, err := r.ensureGeneratedInventory()
	if err != nil {
		return "", err
	}
	plan, err := inventory.PlanLinuxRunner(normalizedPath)
	if err != nil {
		return "", err
	}
	generatedDir := filepath.Join(r.Root, ".matilda", "runner")
	if err := os.MkdirAll(generatedDir, 0700); err != nil {
		return "", err
	}
	generatedPath := filepath.Join(generatedDir, "inventory.linux.yml")
	conn := inventory.LinuxConnectionConfig{
		TargetAdminUser:           values["TARGET_ADMIN_USER"],
		TargetAdminPrivateKeyFile: values["TARGET_ADMIN_PRIVATE_KEY_FILE"],
		ProbeHost:                 values["MATILDA_PROBE_ANSIBLE_HOST"],
		ProbeUser:                 values["MATILDA_PROBE_USER"],
		ProbeAdminPrivateKeyFile:  values["MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE"],
	}
	if err := inventory.WriteLinuxGroupedInventoryWithConnection(generatedPath, plan.Targets, conn); err != nil {
		return "", err
	}
	section(r.Out, "Inventory Plan")
	successLine(r.Out, fmt.Sprintf("Prepared Linux runner inventory from v1: %s", displayPath(r.Root, generatedPath)))
	if len(plan.SkippedTargets) > 0 {
		var skipped []string
		for _, target := range plan.SkippedTargets {
			skipped = append(skipped, fmt.Sprintf("%s (%s)", target.Hostname, target.Platform))
		}
		nextLine(r.Out, "Skipped non-Linux v1 targets for this Linux workflow: "+strings.Join(skipped, ", "))
	}
	return generatedPath, nil
}

func (r *Runtime) checkSetupDependencies() error {
	for _, cmd := range []string{"ansible-playbook", "ansible-doc"} {
		if _, err := runner.RunCapture(r.Root, cmd, "--version"); err != nil {
			return fmt.Errorf("%s was not found or could not run: %s", cmd, localPrerequisiteDetail(cmd, err))
		}
	}
	return nil
}

func requireLocalCommand(name string, workflow string) error {
	if _, err := exec.LookPath(name); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%s is not installed or not on PATH; run ./matilda-prep doctor and install Ansible before running %s", name, workflow)
		}
		return fmt.Errorf("%s could not be checked: %w", name, err)
	}
	return nil
}

func localPrerequisiteDetail(name string, err error) string {
	if errors.Is(err, exec.ErrNotFound) || strings.Contains(err.Error(), "executable file not found") {
		return fmt.Sprintf("%s is not installed or not on PATH; install Ansible and rerun ./matilda-prep doctor", name)
	}
	return err.Error()
}

func (r *Runtime) collectRuntimeExtraVars() ([]string, error) {
	return r.collectRuntimeExtraVarsFor(config.RequiredKeys)
}

func (r *Runtime) collectRuntimeExtraVarsFor(keys []string) ([]string, error) {
	_, extra, err := r.collectRuntimeValuesAndExtra(keys)
	return extra, err
}

func (r *Runtime) collectRuntimeValuesAndExtra(keys []string) (map[string]string, []string, error) {
	values, err := r.collectRuntimeValuesFor(keys)
	if err != nil {
		return nil, nil, err
	}
	return values, config.ExtraVarsFor(keys, values), nil
}

func (r *Runtime) collectRuntimeValuesFor(keys []string) (map[string]string, error) {
	envPath := filepath.Join(r.Root, ".env")
	values, _ := config.LoadEnv(envPath)
	reader := bufio.NewReader(r.In)

	for _, key := range keys {
		if strings.TrimSpace(values[key]) != "" {
			continue
		}
		defaultValue := config.DefaultFor(key, values)
		label := config.LabelFor(key)
		value := promptWithReader(reader, r.Out, label, defaultValue)
		values[key] = value
	}

	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			return nil, fmt.Errorf("%s is required; add %s to .env or rerun ./matilda-prep init", config.LabelFor(key), key)
		}
		if config.LooksLikePlaceholder(value) {
			return nil, fmt.Errorf("%s still contains a placeholder; update %s in .env", config.LabelFor(key), key)
		}
	}

	for _, key := range keys {
		if !config.IsLocalFileKey(key) {
			continue
		}
		value := config.ExpandPath(values[key])
		values[key] = value
		if _, err := os.Stat(value); err != nil {
			return nil, fmt.Errorf("%s not found: %s", config.LabelFor(key), value)
		}
	}

	return values, nil
}

func (r *Runtime) connectionKeys() []string {
	keys := []string{"TARGET_ADMIN_USER", "TARGET_ADMIN_PRIVATE_KEY_FILE"}
	normalizedPath, genErr := r.ensureGeneratedInventory()
	if genErr != nil {
		return keys
	}
	needsProbe, err := inventory.RequiresProbe(normalizedPath)
	if err == nil && needsProbe {
		keys = append(keys, "MATILDA_PROBE_ANSIBLE_HOST", "MATILDA_PROBE_USER", "MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE")
	}
	return keys
}

func (r *Runtime) createEnvGuided() error {
	return r.createEnvGuidedWithReader(bufio.NewReader(r.In))
}

func (r *Runtime) createEnvGuidedWithReader(reader *bufio.Reader) error {
	dest := filepath.Join(r.Root, ".env")
	if err := safety.PrepareDestination(reader, r.Out, dest); err != nil {
		if errors.Is(err, safety.ErrSkip) {
			successLine(r.Out, "Kept existing .env.")
			return nil
		}
		return err
	}

	values := map[string]string{}
	for _, key := range config.RequiredKeys {
		values[key] = promptWithReader(reader, r.Out, config.LabelFor(key), config.DefaultFor(key, values))
	}

	var b strings.Builder
	b.WriteString("# Local runtime values for Matilda Discovery Readiness Toolkit.\n")
	b.WriteString("# Do not commit this file.\n\n")
	for _, key := range config.RequiredKeys {
		fmt.Fprintf(&b, "%s=%s\n", key, config.ShellQuote(values[key]))
		if key == "TARGET_ADMIN_PRIVATE_KEY_FILE" || key == "MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE" {
			b.WriteString("\n")
		}
	}
	return os.WriteFile(dest, []byte(b.String()), 0600)
}

func (r *Runtime) createTargetsCSVWithReader(reader *bufio.Reader) error {
	return copyWithSafetyWithReader(r, reader, filepath.Join(r.Root, "examples", "targets.example.csv"), r.targetsCSVPath())
}

func (r *Runtime) targetsCSVPath() string {
	return filepath.Join(r.Root, "targets.csv")
}

func (r *Runtime) generatedInventoryPath() string {
	return filepath.Join(r.Root, ".matilda", "generated", "inventory.yml")
}

func (r *Runtime) ensureGeneratedInventory() (string, error) {
	_, targets, err := inventory.ValidateCSVFile(r.targetsCSVPath())
	if err != nil {
		return "", err
	}
	return r.writeGeneratedInventory(targets)
}

func (r *Runtime) writeGeneratedInventory(targets []inventory.Target) (string, error) {
	path := r.generatedInventoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	if err := inventory.WriteV1Inventory(path, targets); err != nil {
		return "", err
	}
	return path, nil
}

func copyWithSafety(r *Runtime, source string, dest string) error {
	return copyWithSafetyWithReader(r, r.In, source, dest)
}

func copyWithSafetyWithReader(r *Runtime, reader io.Reader, source string, dest string) error {
	if err := safety.PrepareDestination(reader, r.Out, dest); err != nil {
		if errors.Is(err, safety.ErrSkip) {
			successLine(r.Out, fmt.Sprintf("Kept existing %s.", filepath.Base(dest)))
			return nil
		}
		return err
	}
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest, content, 0600); err != nil {
		return err
	}
	successLine(r.Out, fmt.Sprintf("Created %s.", dest))
	return nil
}

func writeArtifact(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, mode)
}

func samePath(a string, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && absA == absB
}

func (r *Runtime) displayPaths(paths []string) []string {
	var display []string
	for _, path := range paths {
		if rel, err := filepath.Rel(r.Root, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			display = append(display, rel)
			continue
		}
		display = append(display, path)
	}
	return display
}

func parseRollbackMode(args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("rollback requires one mode: --sudoers-only, --remove-key, --lock-user, or --delete-user")
	}

	modes := map[string]string{
		"--sudoers-only": "sudoers_only",
		"--remove-key":   "remove_key",
		"--lock-user":    "lock_user",
		"--delete-user":  "delete_user",
	}
	selected := ""
	for _, arg := range args {
		if arg == "help" || arg == "-h" || arg == "--help" {
			return "help", nil
		}
		mode, ok := modes[arg]
		if !ok {
			return "", fmt.Errorf("unknown rollback option %q", arg)
		}
		if selected != "" {
			return "", errors.New("rollback accepts exactly one mode at a time")
		}
		selected = mode
	}
	if selected == "" {
		return "", errors.New("rollback requires one explicit mode")
	}
	return selected, nil
}

func PrintRollbackHelp(out io.Writer) {
	r := ui.New(out)
	r.Header("Rollback", "Linux rollback modes. Every rollback modifies target systems and requires confirmation.")
	r.Section("Modes")
	r.KeyValues([]ui.KV{
		{Key: "./matilda-prep rollback --sudoers-only", Value: "remove the Matilda sudoers drop-in"},
		{Key: "./matilda-prep rollback --remove-key", Value: "remove the Matilda public key from authorized_keys"},
		{Key: "./matilda-prep rollback --lock-user", Value: "lock the matilda-svc account"},
		{Key: "./matilda-prep rollback --delete-user", Value: "remove the matilda-svc account and home directory"},
	})
	r.Next("Use one rollback mode per run so the target mutation is auditable.")
}

func PrintGenerateHelp(out io.Writer) {
	r := ui.New(out)
	r.Header("Generate", "Create local platform guidance only. These commands do not change targets.")
	r.Section("Targets")
	r.KeyValues([]ui.KV{
		{Key: "./matilda-prep generate windows", Value: "write a Windows readiness package under reports/guidance/windows/"},
		{Key: "./matilda-prep generate unix", Value: "write UNIX admin instructions under reports/guidance/unix/"},
	})
	r.Section("Safety")
	r.Items([]string{
		"generated files are local review artifacts",
		"Windows and UNIX remote configuration is not automated yet",
		"private keys must not be copied to targets",
	})
	r.Next("Share the generated guidance with the platform owner for review.")
}

func windowsReadinessPackageReadme() string {
	return `# Windows Readiness Package

This package is a generated starting point for Windows platform owners. It is local guidance only and does not change Windows targets.

- Review WinRM listener and firewall readiness.
- Confirm SMB TCP/445 access only where Matilda documentation requires it.
- Validate IIS discovery prerequisites when IIS is in scope.
- Review EDR/AV policy before running discovery checks.
- Do not copy private keys to Windows targets.

Remote Windows configuration is intentionally not automated until the WinRM workflow is validated.
`
}

func unixAdminInstructionsReadme() string {
	return `# UNIX Admin Instructions

Use these generated instructions for AIX, Solaris, and HP-UX planning. They are local guidance only and do not change UNIX targets.

- Do not assume Linux paths, packages, sudo behavior, or shell features.
- Prefer customer-reviewed commands and generated instructions first.
- Use matilda-svc as the default service account unless the customer requires another name.
- Model privilege with sudo, dzdo, pbrun, suexec, none, or another documented customer-managed workflow.
- Generate command allow-lists and validation instructions before automating.
- Do not copy private keys to targets.

Remote UNIX automation should be added per OS family only after it is validated on that platform.
`
}

func confirm(in io.Reader, out io.Writer, prompt string) bool {
	reader := bufio.NewReader(in)
	return ui.New(out).Confirm(reader, prompt)
}

func promptDefault(in io.Reader, out io.Writer, prompt string, def string) string {
	reader := bufio.NewReader(in)
	return promptWithReader(reader, out, prompt, def)
}

func promptWithReader(reader *bufio.Reader, out io.Writer, prompt string, def string) string {
	return ui.New(out).Prompt(reader, prompt, def)
}
