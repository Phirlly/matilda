package inventory

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

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

func ValidateFile(path string) (ValidationResult, error) {
	var result ValidationResult
	content, err := os.ReadFile(path)
	if err != nil {
		result.Checks = append(result.Checks, runner.Result{Name: "inventory.yml", Status: runner.StatusFail, Detail: "missing: " + path})
		return result, err
	}
	text := string(content)
	result.Checks = append(result.Checks, runner.Result{Name: "inventory.yml", Status: runner.StatusPass, Detail: path})

	if strings.Contains(text, "public_targets:") || strings.Contains(text, "private_targets:") {
		targets := parseLinuxGroupedTargets(text)
		result.TargetCount = len(targets)
		result.Format = "linux-groups"
		result.Checks = append(result.Checks, runner.Result{Name: "inventory format", Status: runner.StatusPass, Detail: "current Linux-compatible"})
		checks, validateErr := validateTargets(targets, true)
		result.Checks = append(result.Checks, checks...)
		return result, validateErr
	}

	if strings.Contains(text, "version: 1") && strings.Contains(text, "targets:") {
		targets := parseV1Targets(text)
		result.TargetCount = len(targets)
		result.Format = "v1"
		result.Checks = append(result.Checks, runner.Result{Name: "inventory format", Status: runner.StatusPass, Detail: "normalized v1"})
		checks, validateErr := validateTargets(targets, false)
		result.Checks = append(result.Checks, checks...)
		return result, validateErr
	}

	result.Checks = append(result.Checks, runner.Result{Name: "inventory format", Status: runner.StatusFail, Detail: "expected current Linux groups or normalized version: 1"})
	return result, errors.New("unsupported inventory format")
}

func DetectFormat(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(content)
	if strings.Contains(text, "public_targets:") || strings.Contains(text, "private_targets:") {
		return "linux-groups", nil
	}
	if strings.Contains(text, "version: 1") && strings.Contains(text, "targets:") {
		return "v1", nil
	}
	return "", errors.New("unsupported inventory format")
}

func RequiresProbe(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := string(content)
	var targets []Target
	if strings.Contains(text, "version: 1") && strings.Contains(text, "targets:") {
		targets = parseV1Targets(text)
	} else {
		targets = parseLinuxGroupedTargets(text)
	}
	for _, target := range targets {
		if target.AccessPath == "via_probe" && (target.Platform == "" || target.Platform == "linux") {
			return true, nil
		}
	}
	return false, nil
}

func PlanLinuxRunner(path string) (LinuxRunnerPlan, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return LinuxRunnerPlan{}, err
	}
	text := string(content)
	if strings.Contains(text, "public_targets:") || strings.Contains(text, "private_targets:") {
		targets := parseLinuxGroupedTargets(text)
		if len(targets) == 0 {
			return LinuxRunnerPlan{}, errors.New("inventory contains no Linux targets")
		}
		return LinuxRunnerPlan{Format: "linux-groups", Targets: targets}, nil
	}
	if strings.Contains(text, "version: 1") && strings.Contains(text, "targets:") {
		targets := parseV1Targets(text)
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
	return LinuxRunnerPlan{}, errors.New("unsupported inventory format")
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
	for i, header := range rows[0] {
		headers[strings.ToLower(strings.TrimSpace(header))] = i
	}
	required := []string{"hostname", "platform", "ansible_host", "discovery_ip", "access_path", "privilege_method"}
	for _, key := range required {
		if _, ok := headers[key]; !ok {
			return nil, fmt.Errorf("CSV missing required column: %s", key)
		}
	}

	var targets []Target
	for rowIndex, row := range rows[1:] {
		if len(row) == 0 || strings.TrimSpace(strings.Join(row, "")) == "" {
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
			ConfigureMode:   get(row, headers, "configure_mode"),
		}
		if target.Hostname == "" {
			return nil, fmt.Errorf("row %d missing hostname", rowIndex+2)
		}
		if target.Platform != "linux" {
			return nil, fmt.Errorf("row %d platform %q is not supported by current Linux inventory import", rowIndex+2, target.Platform)
		}
		if target.AnsibleHost == "" || isPlaceholder(target.AnsibleHost) {
			return nil, fmt.Errorf("row %d ansible_host must be a real target address", rowIndex+2)
		}
		if target.DiscoveryIP == "" || isPlaceholder(target.DiscoveryIP) {
			return nil, fmt.Errorf("row %d discovery_ip must be the address MatildaProbeVM will use", rowIndex+2)
		}
		if target.AccessPath != "direct" && target.AccessPath != "via_probe" {
			return nil, fmt.Errorf("row %d access_path must be direct or via_probe", rowIndex+2)
		}
		if target.PrivilegeMethod == "" {
			return nil, fmt.Errorf("row %d privilege_method is required; current Linux automation supports sudo", rowIndex+2)
		}
		if target.PrivilegeMethod != "sudo" {
			return nil, fmt.Errorf("row %d privilege_method %q is not automated yet", rowIndex+2, target.PrivilegeMethod)
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

func MigrateLinuxGroupedToV1(input string, output string) error {
	content, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	targets := parseLinuxGroupedTargets(string(content))
	if len(targets) == 0 {
		return errors.New("no Linux grouped targets detected")
	}

	var b strings.Builder
	b.WriteString("version: 1\n\n")
	b.WriteString("profiles:\n")
	b.WriteString("  default:\n")
	b.WriteString("    probe:\n")
	b.WriteString("      host: <probe-host-or-ip>\n")
	b.WriteString("      user: <probe-admin-user>\n")
	b.WriteString("      admin_private_key_file: <path-to-probe-admin-key>\n")
	b.WriteString("      discovery_private_key_on_probe: <discovery-private-key-path-on-probe>\n\n")
	b.WriteString("targets:\n")
	for _, target := range targets {
		fmt.Fprintf(&b, "  %s:\n", target.Hostname)
		b.WriteString("    platform: linux\n")
		b.WriteString("    os_family: linux\n")
		fmt.Fprintf(&b, "    access_path: %s\n", target.AccessPath)
		fmt.Fprintf(&b, "    ansible_host: %s\n", target.AnsibleHost)
		fmt.Fprintf(&b, "    discovery_ip: %s\n", target.DiscoveryIP)
		b.WriteString("    privilege_method: sudo\n")
		b.WriteString("    configure_mode: remote\n\n")
	}
	return os.WriteFile(output, []byte(b.String()), 0600)
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

func parseLinuxGroupedTargets(text string) []Target {
	var targets []Target
	currentGroup := ""
	var current *Target

	flush := func() {
		if current != nil && current.Hostname != "" {
			if current.AccessPath == "" {
				if currentGroup == "private_targets" {
					current.AccessPath = "via_probe"
				} else {
					current.AccessPath = "direct"
				}
			}
			targets = append(targets, *current)
		}
		current = nil
	}

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "public_targets:" || line == "private_targets:" {
			flush()
			currentGroup = strings.TrimSuffix(line, ":")
			continue
		}
		if strings.HasSuffix(line, ":") && !strings.Contains(line, " ") {
			name := strings.TrimSuffix(line, ":")
			if name != "all" && name != "children" && name != "hosts" {
				flush()
				current = &Target{Hostname: name, Platform: "linux", PrivilegeMethod: "sudo"}
			}
			continue
		}
		if current == nil || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = cleanScalar(value)
		switch key {
		case "ansible_host":
			current.AnsibleHost = value
		case "discovery_ip":
			current.DiscoveryIP = value
		case "private_ip":
			current.PrivateIP = value
		case "public_ip":
			current.PublicIP = value
		}
	}
	flush()
	return targets
}

func parseV1Targets(text string) []Target {
	var targets []Target
	var current *Target
	inTargets := false

	flush := func() {
		if current != nil && current.Hostname != "" {
			targets = append(targets, *current)
		}
		current = nil
	}

	for _, raw := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "targets:" {
			inTargets = true
			flush()
			continue
		}
		if !inTargets {
			continue
		}
		if !strings.HasPrefix(raw, " ") && strings.HasSuffix(trimmed, ":") {
			break
		}
		if strings.HasPrefix(raw, "  ") && !strings.HasPrefix(raw, "    ") && strings.HasSuffix(trimmed, ":") {
			flush()
			current = &Target{Hostname: strings.TrimSuffix(trimmed, ":")}
			continue
		}
		if current == nil || !strings.HasPrefix(raw, "    ") || !strings.Contains(trimmed, ":") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		key := strings.TrimSpace(parts[0])
		value := cleanScalar(strings.TrimSpace(parts[1]))
		switch key {
		case "platform":
			current.Platform = strings.ToLower(value)
		case "os_family":
			current.OSFamily = strings.ToLower(value)
		case "cloud_provider":
			current.CloudProvider = strings.ToLower(value)
		case "access_path":
			current.AccessPath = strings.ToLower(value)
		case "ansible_host":
			current.AnsibleHost = value
		case "discovery_ip":
			current.DiscoveryIP = value
		case "private_ip":
			current.PrivateIP = value
		case "public_ip":
			current.PublicIP = value
		case "admin_user":
			// Kept for normalized inventory compatibility; runtime credentials are still local .env values.
		case "privilege_method":
			current.PrivilegeMethod = strings.ToLower(value)
		case "configure_mode":
			current.ConfigureMode = strings.ToLower(value)
		}
	}
	flush()
	return targets
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
			validateAccessTarget(&problems, host, target, "")
		case "windows":
			requireValue(&problems, host, "ansible_host", target.AnsibleHost)
			requirePrivilege(&problems, host, target.PrivilegeMethod, "winrm")
		case "cloud":
			requireValue(&problems, host, "cloud_provider", target.CloudProvider)
			requirePrivilege(&problems, host, target.PrivilegeMethod, "cloud_api")
		case "kubernetes":
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
	requireValue(problems, host, "access_path", target.AccessPath)
	requireValue(problems, host, "ansible_host", target.AnsibleHost)
	requireValue(problems, host, "discovery_ip", target.DiscoveryIP)
	if target.AccessPath != "" && target.AccessPath != "direct" && target.AccessPath != "via_probe" {
		*problems = append(*problems, fmt.Sprintf("%s access_path must be direct or via_probe", host))
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

func cleanScalar(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			value = value[1 : len(value)-1]
		}
	}
	return strings.TrimSpace(value)
}

func get(row []string, headers map[string]int, key string) string {
	index, ok := headers[key]
	if !ok || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}
