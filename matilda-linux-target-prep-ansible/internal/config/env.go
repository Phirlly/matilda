package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var RequiredKeys = []string{
	"TARGET_ADMIN_USER",
	"TARGET_ADMIN_PRIVATE_KEY_FILE",
	"MATILDA_PROBE_ANSIBLE_HOST",
	"MATILDA_PROBE_USER",
	"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE",
	"MATILDA_PUBLIC_KEY_FILE",
	"MATILDA_PROBE_PRIVATE_KEY_ON_PROBE",
}

var LocalFileKeys = []string{
	"TARGET_ADMIN_PRIVATE_KEY_FILE",
	"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE",
	"MATILDA_PUBLIC_KEY_FILE",
}

var extraVarNames = map[string]string{
	"TARGET_ADMIN_USER":                    "target_admin_user",
	"TARGET_ADMIN_PRIVATE_KEY_FILE":        "target_admin_private_key_file",
	"MATILDA_PROBE_ANSIBLE_HOST":           "matilda_probe_ansible_host",
	"MATILDA_PROBE_USER":                   "matilda_probe_user",
	"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE": "matilda_probe_admin_private_key_file",
	"MATILDA_PUBLIC_KEY_FILE":              "matilda_public_key_file",
	"MATILDA_PROBE_PRIVATE_KEY_ON_PROBE":   "matilda_probe_private_key_on_probe",
}

func LoadEnv(path string) (map[string]string, error) {
	values := map[string]string{}
	file, err := os.Open(path)
	if err != nil {
		return values, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		values[strings.TrimSpace(parts[0])] = unquote(strings.TrimSpace(parts[1]))
	}
	return values, scanner.Err()
}

func ExtraVars(values map[string]string) []string {
	return ExtraVarsFor(RequiredKeys, values)
}

func ExtraVarsFor(keys []string, values map[string]string) []string {
	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		if extraName, ok := extraVarNames[key]; ok {
			args = append(args, "--extra-vars", fmt.Sprintf("%s=%s", extraName, values[key]))
		}
	}
	return args
}

func LabelFor(key string) string {
	switch key {
	case "TARGET_ADMIN_USER":
		return "Target admin SSH user"
	case "TARGET_ADMIN_PRIVATE_KEY_FILE":
		return "Target admin private key path"
	case "MATILDA_PROBE_ANSIBLE_HOST":
		return "MatildaProbeVM SSH host/IP"
	case "MATILDA_PROBE_USER":
		return "MatildaProbeVM SSH user"
	case "MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE":
		return "MatildaProbeVM admin private key path"
	case "MATILDA_PUBLIC_KEY_FILE":
		return "Matilda discovery public key path on this machine"
	case "MATILDA_PROBE_PRIVATE_KEY_ON_PROBE":
		return "Matilda discovery private key path on MatildaProbeVM"
	default:
		return key
	}
}

func DefaultFor(key string, values map[string]string) string {
	switch key {
	case "TARGET_ADMIN_USER", "MATILDA_PROBE_USER":
		return "opc"
	case "MATILDA_PROBE_PRIVATE_KEY_ON_PROBE":
		user := values["MATILDA_PROBE_USER"]
		if user == "" {
			user = "opc"
		}
		return "/home/" + user + "/.ssh/MatildaProbeKey.pem"
	default:
		return ""
	}
}

func IsLocalFileKey(key string) bool {
	for _, candidate := range LocalFileKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func ShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n'\"$`\\") {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return value
}

func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
