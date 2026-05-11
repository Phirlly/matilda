package inventory

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"matilda-discovery-readiness/internal/runner"
)

type Target struct {
	Hostname        string
	Platform        string
	OSFamily        string
	CloudProvider   string
	AccessPath      string
	AnsibleHost     string
	DiscoveryIP     string
	PublicIP        string
	PrivateIP       string
	PrivilegeMethod string
	ConfigureMode   string
}

type ValidationResult struct {
	Checks      []runner.Result
	TargetCount int
	Format      string
}

type LinuxRunnerPlan struct {
	Format         string
	Targets        []Target
	SkippedTargets []Target
}

var csvRequiredColumns = []string{"hostname", "platform", "ansible_host", "discovery_ip", "access_path", "privilege_method"}

type inventoryV1 struct {
	Version  int                 `yaml:"version"`
	Profiles map[string]any      `yaml:"profiles,omitempty"`
	Targets  map[string]targetV1 `yaml:"targets"`
}

type targetV1 struct {
	Platform            string `yaml:"platform,omitempty"`
	OSFamily            string `yaml:"os_family,omitempty"`
	OSVersion           string `yaml:"os_version,omitempty"`
	CloudProvider       string `yaml:"cloud_provider,omitempty"`
	AccessPath          string `yaml:"access_path,omitempty"`
	AnsibleHost         string `yaml:"ansible_host,omitempty"`
	DiscoveryIP         string `yaml:"discovery_ip,omitempty"`
	PublicIP            string `yaml:"public_ip,omitempty"`
	PrivateIP           string `yaml:"private_ip,omitempty"`
	AdminUser           string `yaml:"admin_user,omitempty"`
	AdminPrivateKeyFile string `yaml:"admin_private_key_file,omitempty"`
	PrivilegeMethod     string `yaml:"privilege_method,omitempty"`
	ConfigureMode       string `yaml:"configure_mode,omitempty"`
}

func ValidateFile(path string) (ValidationResult, error) {
	var result ValidationResult
	content, err := os.ReadFile(path)
	if err != nil {
		result.Checks = append(result.Checks, runner.Result{Name: "inventory.yml", Status: runner.StatusFail, Detail: "missing: " + path})
		return result, err
	}
	result.Checks = append(result.Checks, runner.Result{Name: "inventory.yml", Status: runner.StatusPass, Detail: path})

	targets, err := parseInventoryV1Content(content)
	if err != nil {
		result.Checks = append(result.Checks, runner.Result{Name: "inventory format", Status: runner.StatusFail, Detail: err.Error()})
		return result, err
	}

	result.TargetCount = len(targets)
	result.Format = "v1"
	result.Checks = append(result.Checks, runner.Result{Name: "inventory format", Status: runner.StatusPass, Detail: "normalized version: 1"})
	checks, validateErr := validateTargets(targets, false)
	result.Checks = append(result.Checks, checks...)
	return result, validateErr
}

func DetectFormat(path string) (string, error) {
	if _, err := readInventoryV1(path); err != nil {
		return "", err
	}
	return "v1", nil
}

func ValidateCSVFile(path string) (ValidationResult, []Target, error) {
	var result ValidationResult
	targets, err := ReadCSV(path)
	if err != nil {
		if os.IsNotExist(err) {
			result.Checks = append(result.Checks, runner.Result{Name: "targets.csv", Status: runner.StatusFail, Detail: "missing: " + path})
		} else {
			result.Checks = append(result.Checks, runner.Result{Name: "targets.csv", Status: runner.StatusFail, Detail: err.Error()})
		}
		return result, nil, err
	}

	result.Format = "csv"
	result.TargetCount = len(targets)
	result.Checks = append(result.Checks, runner.Result{Name: "targets.csv", Status: runner.StatusPass, Detail: path})
	result.Checks = append(result.Checks, runner.Result{Name: "inventory format", Status: runner.StatusPass, Detail: "CSV target inventory"})
	checks, validateErr := validateTargets(targets, false)
	result.Checks = append(result.Checks, checks...)
	return result, targets, validateErr
}

func RequiresProbe(path string) (bool, error) {
	targets, err := readInventoryV1(path)
	if err != nil {
		return false, err
	}
	for _, target := range targets {
		if target.AccessPath == "via_probe" && target.Platform == "linux" {
			return true, nil
		}
	}
	return false, nil
}

func PlanLinuxRunner(path string) (LinuxRunnerPlan, error) {
	targets, err := readInventoryV1(path)
	if err != nil {
		return LinuxRunnerPlan{}, err
	}

	var linuxTargets []Target
	var skipped []Target
	var problems []string
	for _, target := range targets {
		platform := strings.ToLower(target.Platform)
		if platform != "linux" {
			skipped = append(skipped, target)
			continue
		}
		validateAccessTarget(&problems, target.Hostname, target, "sudo")
		linuxTargets = append(linuxTargets, target)
	}
	if len(problems) > 0 {
		return LinuxRunnerPlan{}, fmt.Errorf("inventory v1 Linux runner planning failed: %s", strings.Join(problems, "; "))
	}
	if len(linuxTargets) == 0 {
		return LinuxRunnerPlan{}, errors.New("inventory v1 contains no Linux targets for the current Linux workflow; non-Linux targets are valid inventory data but are skipped by Linux remote actions")
	}
	return LinuxRunnerPlan{Format: "v1", Targets: linuxTargets, SkippedTargets: skipped}, nil
}

func ReadCSV(path string) ([]Target, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, errors.New("CSV must include a header and at least one target row")
	}

	headers := map[string]int{}
	var duplicateHeaders []string
	for i, header := range rows[0] {
		header = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(header)), "\ufeff")
		if header == "" {
			continue
		}
		if _, exists := headers[header]; exists {
			duplicateHeaders = append(duplicateHeaders, header)
			continue
		}
		headers[header] = i
	}
	if len(duplicateHeaders) > 0 {
		return nil, fmt.Errorf("CSV duplicate column(s): %s", strings.Join(duplicateHeaders, ", "))
	}
	var missingColumns []string
	for _, key := range csvRequiredColumns {
		if _, ok := headers[key]; !ok {
			missingColumns = append(missingColumns, key)
		}
	}
	if len(missingColumns) > 0 {
		return nil, fmt.Errorf("CSV missing required columns: %s", strings.Join(missingColumns, ", "))
	}

	var targets []Target
	seenHosts := map[string]int{}
	for rowIndex, row := range rows[1:] {
		rowNumber := rowIndex + 2
		if emptyCSVRow(row) {
			continue
		}
		target := Target{
			Hostname:        get(row, headers, "hostname"),
			Platform:        strings.ToLower(get(row, headers, "platform")),
			OSFamily:        strings.ToLower(get(row, headers, "os_family")),
			CloudProvider:   strings.ToLower(get(row, headers, "cloud_provider")),
			AccessPath:      strings.ToLower(get(row, headers, "access_path")),
			AnsibleHost:     get(row, headers, "ansible_host"),
			DiscoveryIP:     get(row, headers, "discovery_ip"),
			PublicIP:        get(row, headers, "public_ip"),
			PrivateIP:       get(row, headers, "private_ip"),
			PrivilegeMethod: strings.ToLower(get(row, headers, "privilege_method")),
			ConfigureMode:   strings.ToLower(get(row, headers, "configure_mode")),
		}
		if missing := missingRequiredCSVValues(row, headers); len(missing) > 0 {
			return nil, fmt.Errorf("row %d missing required values: %s", rowNumber, strings.Join(missing, ", "))
		}
		if firstRow, exists := seenHosts[target.Hostname]; exists {
			return nil, fmt.Errorf("row %d duplicate hostname %q; first seen on row %d", rowNumber, target.Hostname, firstRow)
		}
		seenHosts[target.Hostname] = rowNumber
		if target.Platform != "linux" {
			return nil, fmt.Errorf("row %d platform %q is not supported by current Linux inventory import", rowNumber, target.Platform)
		}
		if target.AnsibleHost == "" || isPlaceholder(target.AnsibleHost) {
			return nil, fmt.Errorf("row %d ansible_host must be a real target address", rowNumber)
		}
		if target.DiscoveryIP == "" || isPlaceholder(target.DiscoveryIP) {
			return nil, fmt.Errorf("row %d discovery_ip must be the address MatildaProbeVM will use", rowNumber)
		}
		if target.AccessPath != "direct" && target.AccessPath != "via_probe" {
			return nil, fmt.Errorf("row %d access_path must be direct or via_probe", rowNumber)
		}
		if target.PrivilegeMethod == "" {
			return nil, fmt.Errorf("row %d privilege_method is required; current Linux automation supports sudo", rowNumber)
		}
		if target.PrivilegeMethod != "sudo" {
			return nil, fmt.Errorf("row %d privilege_method %q is not automated yet", rowNumber, target.PrivilegeMethod)
		}
		if target.PrivateIP == "" {
			target.PrivateIP = target.DiscoveryIP
		}
		if target.PublicIP == "" && target.AccessPath == "direct" {
			target.PublicIP = target.AnsibleHost
		}
		if target.ConfigureMode == "" {
			target.ConfigureMode = "remote"
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return nil, errors.New("CSV did not contain any usable target rows")
	}
	return targets, nil
}

func WriteV1Inventory(path string, targets []Target) error {
	if len(targets) == 0 {
		return errors.New("inventory must include at least one target")
	}

	doc := inventoryV1{
		Version: 1,
		Targets: map[string]targetV1{},
	}
	for _, target := range targets {
		hostname := strings.TrimSpace(target.Hostname)
		if hostname == "" {
			return errors.New("inventory target hostname is required")
		}
		if _, exists := doc.Targets[hostname]; exists {
			return fmt.Errorf("duplicate inventory hostname %q", hostname)
		}
		doc.Targets[hostname] = targetToV1(target)
	}

	content, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0600)
}

func WriteLinuxGroupedInventory(path string, targets []Target) error {
	var publicTargets []Target
	var privateTargets []Target
	for _, target := range targets {
		switch target.AccessPath {
		case "direct", "public", "":
			publicTargets = append(publicTargets, target)
		case "via_probe", "private":
			privateTargets = append(privateTargets, target)
		default:
			return fmt.Errorf("unsupported access_path %q for %s", target.AccessPath, target.Hostname)
		}
	}

	sort.Slice(publicTargets, func(i, j int) bool { return publicTargets[i].Hostname < publicTargets[j].Hostname })
	sort.Slice(privateTargets, func(i, j int) bool { return privateTargets[i].Hostname < privateTargets[j].Hostname })

	var b strings.Builder
	b.WriteString("all:\n")
	b.WriteString("  children:\n")
	writeLinuxGroup(&b, "public_targets", publicTargets)
	b.WriteString("\n")
	writeLinuxGroup(&b, "private_targets", privateTargets)
	return os.WriteFile(path, []byte(b.String()), 0600)
}

func readInventoryV1(path string) ([]Target, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseInventoryV1Content(content)
}

func parseInventoryV1Content(content []byte) ([]Target, error) {
	if looksLikeLegacyLinuxGroupedInventory(string(content)) {
		return nil, errors.New("inventory.yml must use version: 1; public_targets/private_targets are internal runner output only")
	}

	var doc inventoryV1
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse inventory.yml as version: 1 YAML: %w", err)
	}
	if doc.Version != 1 {
		return nil, errors.New("inventory.yml must use version: 1")
	}
	if len(doc.Targets) == 0 {
		return nil, errors.New("inventory.yml version: 1 must include at least one target")
	}

	names := make([]string, 0, len(doc.Targets))
	for name := range doc.Targets {
		names = append(names, name)
	}
	sort.Strings(names)

	targets := make([]Target, 0, len(names))
	for _, name := range names {
		targets = append(targets, v1ToTarget(name, doc.Targets[name]))
	}
	return targets, nil
}

func looksLikeLegacyLinuxGroupedInventory(text string) bool {
	hasGroup := strings.Contains(text, "public_targets:") || strings.Contains(text, "private_targets:")
	return hasGroup && (!strings.Contains(text, "version:") || strings.Contains(text, "children:"))
}

func targetToV1(target Target) targetV1 {
	return targetV1{
		Platform:        normalizeLower(target.Platform),
		OSFamily:        normalizeLower(target.OSFamily),
		CloudProvider:   normalizeLower(target.CloudProvider),
		AccessPath:      normalizeLower(target.AccessPath),
		AnsibleHost:     strings.TrimSpace(target.AnsibleHost),
		DiscoveryIP:     strings.TrimSpace(target.DiscoveryIP),
		PublicIP:        strings.TrimSpace(target.PublicIP),
		PrivateIP:       strings.TrimSpace(target.PrivateIP),
		PrivilegeMethod: normalizeLower(target.PrivilegeMethod),
		ConfigureMode:   normalizeLower(target.ConfigureMode),
	}
}

func v1ToTarget(hostname string, target targetV1) Target {
	return Target{
		Hostname:        strings.TrimSpace(hostname),
		Platform:        normalizeLower(target.Platform),
		OSFamily:        normalizeLower(target.OSFamily),
		CloudProvider:   normalizeLower(target.CloudProvider),
		AccessPath:      normalizeLower(target.AccessPath),
		AnsibleHost:     strings.TrimSpace(target.AnsibleHost),
		DiscoveryIP:     strings.TrimSpace(target.DiscoveryIP),
		PublicIP:        strings.TrimSpace(target.PublicIP),
		PrivateIP:       strings.TrimSpace(target.PrivateIP),
		PrivilegeMethod: normalizeLower(target.PrivilegeMethod),
		ConfigureMode:   normalizeLower(target.ConfigureMode),
	}
}

func normalizeLower(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func writeLinuxGroup(b *strings.Builder, group string, targets []Target) {
	fmt.Fprintf(b, "    %s:\n", group)
	if len(targets) == 0 {
		b.WriteString("      hosts: {}\n")
		return
	}
	b.WriteString("      hosts:\n")
	for _, target := range targets {
		fmt.Fprintf(b, "        %s:\n", target.Hostname)
		fmt.Fprintf(b, "          ansible_host: %s\n", target.AnsibleHost)
		if target.AccessPath == "direct" || target.AccessPath == "" {
			if target.PublicIP != "" {
				fmt.Fprintf(b, "          public_ip: %s\n", target.PublicIP)
			}
		}
		if target.PrivateIP != "" {
			fmt.Fprintf(b, "          private_ip: %s\n", target.PrivateIP)
		}
		fmt.Fprintf(b, "          discovery_ip: %s\n", target.DiscoveryIP)
	}
}

func validateTargets(targets []Target, linuxGrouped bool) ([]runner.Result, error) {
	var checks []runner.Result
	if len(targets) == 0 {
		checks = append(checks, runner.Result{Name: "targets", Status: runner.StatusFail, Detail: "no targets detected"})
		return checks, errors.New("inventory contains no targets")
	}
	checks = append(checks, runner.Result{Name: "targets", Status: runner.StatusPass, Detail: fmt.Sprintf("%d target(s)", len(targets))})

	var problems []string
	platforms := map[string]bool{}
	for _, target := range targets {
		host := target.Hostname
		if host == "" {
			host = "<unnamed>"
		}
		platform := strings.ToLower(target.Platform)
		if platform == "" && linuxGrouped {
			platform = "linux"
		}
		platforms[platform] = true

		requireValue(&problems, host, "hostname", target.Hostname)
		requireValue(&problems, host, "platform", platform)
		if platform != "" && !supportedPlatform(platform) {
			problems = append(problems, fmt.Sprintf("%s has unsupported platform %q", host, platform))
		}

		switch platform {
		case "linux":
			validateAccessTarget(&problems, host, target, "sudo")
		case "unix":
			validateTargetAccessPath(&problems, host, target, "", map[string]bool{
				"direct":           true,
				"via_probe":        true,
				"customer_managed": true,
			})
		case "windows":
			requireValue(&problems, host, "ansible_host", target.AnsibleHost)
			requirePrivilege(&problems, host, target.PrivilegeMethod, "winrm")
		case "cloud":
			requireValue(&problems, host, "cloud_provider", target.CloudProvider)
			if target.AccessPath != "" && target.AccessPath != "api" && target.AccessPath != "customer_managed" {
				problems = append(problems, fmt.Sprintf("%s access_path must be api or customer_managed", host))
			}
			requirePrivilege(&problems, host, target.PrivilegeMethod, "cloud_api")
		case "kubernetes":
			if target.AccessPath != "" && target.AccessPath != "api" && target.AccessPath != "customer_managed" {
				problems = append(problems, fmt.Sprintf("%s access_path must be api or customer_managed", host))
			}
			requirePrivilege(&problems, host, target.PrivilegeMethod, "k8s_api")
		}
	}

	if len(problems) > 0 {
		detail := strings.Join(problems, "; ")
		checks = append(checks, runner.Result{Name: "target fields", Status: runner.StatusFail, Detail: detail})
		return checks, fmt.Errorf("inventory target validation failed: %s", detail)
	}

	checks = append(checks, runner.Result{Name: "target fields", Status: runner.StatusPass, Detail: "required fields are present"})
	if !linuxGrouped && hasOnlyLinux(platforms) {
		checks = append(checks, runner.Result{Name: "platform support", Status: runner.StatusPass, Detail: "Linux targets are compatible with the current automation baseline"})
	} else if !linuxGrouped {
		checks = append(checks, runner.Result{Name: "platform support", Status: runner.StatusSkip, Detail: "non-Linux targets are structurally valid; automation remains generated/scaffolded by platform"})
	}
	return checks, nil
}

func validateAccessTarget(problems *[]string, host string, target Target, requiredPrivilege string) {
	validateTargetAccessPath(problems, host, target, requiredPrivilege, map[string]bool{
		"direct":    true,
		"via_probe": true,
	})
}

func validateTargetAccessPath(problems *[]string, host string, target Target, requiredPrivilege string, allowedAccess map[string]bool) {
	requireValue(problems, host, "access_path", target.AccessPath)
	requireValue(problems, host, "ansible_host", target.AnsibleHost)
	requireValue(problems, host, "discovery_ip", target.DiscoveryIP)
	if target.AccessPath != "" && !allowedAccess[target.AccessPath] {
		var allowed []string
		for value := range allowedAccess {
			allowed = append(allowed, value)
		}
		sort.Strings(allowed)
		*problems = append(*problems, fmt.Sprintf("%s access_path must be %s", host, strings.Join(allowed, " or ")))
	}
	if requiredPrivilege != "" {
		requirePrivilege(problems, host, target.PrivilegeMethod, requiredPrivilege)
	} else if target.PrivilegeMethod != "" && !supportedPrivilege(target.PrivilegeMethod) {
		*problems = append(*problems, fmt.Sprintf("%s has unsupported privilege_method %q", host, target.PrivilegeMethod))
	}
}

func requireValue(problems *[]string, host string, field string, value string) {
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, fmt.Sprintf("%s missing %s", host, field))
		return
	}
	if isPlaceholder(value) {
		*problems = append(*problems, fmt.Sprintf("%s %s still contains placeholder %q", host, field, value))
	}
}

func requirePrivilege(problems *[]string, host string, actual string, expected string) {
	requireValue(problems, host, "privilege_method", actual)
	if actual != "" && actual != expected {
		*problems = append(*problems, fmt.Sprintf("%s privilege_method must be %s for this readiness module", host, expected))
	}
}

func supportedPlatform(platform string) bool {
	switch platform {
	case "linux", "unix", "windows", "cloud", "kubernetes":
		return true
	default:
		return false
	}
}

func supportedPrivilege(method string) bool {
	switch method {
	case "sudo", "dzdo", "pbrun", "suexec", "winrm", "cloud_api", "k8s_api", "none":
		return true
	default:
		return false
	}
}

func hasOnlyLinux(platforms map[string]bool) bool {
	return len(platforms) == 1 && platforms["linux"]
}

func isPlaceholder(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")
}

func get(row []string, headers map[string]int, key string) string {
	index, ok := headers[key]
	if !ok || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func emptyCSVRow(row []string) bool {
	for _, field := range row {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func missingRequiredCSVValues(row []string, headers map[string]int) []string {
	var missing []string
	for _, key := range csvRequiredColumns {
		if get(row, headers, key) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}
