package authdiscovery

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Profile struct {
	Name                 string
	Region               string
	HasLoginSession      bool
	HasSSOSession        bool
	HasCredentialProcess bool
	HasSourceProfile     bool
	HasRoleARN           bool
}

type Discoverer struct {
	EnvLookup func(string) (string, bool)
	ReadFile  func(string) ([]byte, error)
	OpenFile  func(string) (io.ReadCloser, error)
	HomeDir   func() (string, error)
}

func (discoverer Discoverer) Discover(ctx context.Context) ([]Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	envLookup := discoverer.EnvLookup
	if envLookup == nil {
		envLookup = os.LookupEnv
	}
	readFile := discoverer.ReadFile
	if readFile == nil {
		readFile = nil
	}
	openFile := discoverer.OpenFile
	if openFile == nil && readFile == nil {
		openFile = func(path string) (io.ReadCloser, error) {
			return os.Open(path)
		}
	}
	homeDir := discoverer.HomeDir
	if homeDir == nil {
		homeDir = os.UserHomeDir
	}

	profiles := map[string]*Profile{}
	if path := sharedConfigPath(envLookup, homeDir); path != "" {
		discoverSharedFile(ctx, readFile, openFile, path, true, profiles)
	}
	if path := sharedCredentialsPath(envLookup, homeDir); path != "" {
		discoverSharedFile(ctx, readFile, openFile, path, false, profiles)
	}

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]Profile, 0, len(names))
	for _, name := range names {
		result = append(result, *profiles[name])
	}
	return result, nil
}

func sharedConfigPath(envLookup func(string) (string, bool), homeDir func() (string, error)) string {
	if path, ok := envLookup("AWS_CONFIG_FILE"); ok && strings.TrimSpace(path) != "" {
		return path
	}
	return defaultAWSPath(homeDir, "config")
}

func sharedCredentialsPath(envLookup func(string) (string, bool), homeDir func() (string, error)) string {
	if path, ok := envLookup("AWS_SHARED_CREDENTIALS_FILE"); ok && strings.TrimSpace(path) != "" {
		return path
	}
	return defaultAWSPath(homeDir, "credentials")
}

func defaultAWSPath(homeDir func() (string, error), name string) string {
	home, err := homeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".aws", name)
}

func discoverSharedFile(ctx context.Context, readFile func(string) ([]byte, error), openFile func(string) (io.ReadCloser, error), path string, configFile bool, profiles map[string]*Profile) {
	if ctx.Err() != nil {
		return
	}
	if openFile != nil {
		file, err := openFile(path)
		if err != nil {
			return
		}
		defer file.Close()
		parseSharedReader(file, configFile, profiles)
		return
	}
	content, err := readFile(path)
	if err != nil {
		return
	}
	parseSharedFile(string(content), configFile, profiles)
}

func parseSharedFile(content string, configFile bool, profiles map[string]*Profile) {
	parseSharedReader(strings.NewReader(content), configFile, profiles)
}

func parseSharedReader(reader io.Reader, configFile bool, profiles map[string]*Profile) {
	var current *Profile
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if section, ok := sectionName(trimmed); ok {
			current = profileForSection(section, configFile, profiles)
			continue
		}
		if current == nil {
			continue
		}

		key, value, ok := metadataLine(trimmed)
		if !ok {
			continue
		}
		applyProfileMetadata(current, key, value)
	}
}

func sectionName(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") || !strings.Contains(line, "]") {
		return "", false
	}
	end := strings.Index(line, "]")
	name := strings.TrimSpace(line[1:end])
	return name, name != ""
}

func profileForSection(section string, configFile bool, profiles map[string]*Profile) *Profile {
	name := ""
	lower := strings.ToLower(strings.TrimSpace(section))
	if configFile {
		switch {
		case lower == "default":
			name = "default"
		case strings.HasPrefix(lower, "profile "):
			name = strings.TrimSpace(section[len("profile "):])
		default:
			return nil
		}
	} else {
		if lower == "default" {
			name = "default"
		} else if strings.HasPrefix(lower, "profile ") || strings.HasPrefix(lower, "sso-session ") || strings.HasPrefix(lower, "services ") {
			return nil
		} else {
			name = strings.TrimSpace(section)
		}
	}
	if !safeProfileName(name) {
		return nil
	}
	profile := profiles[name]
	if profile == nil {
		profile = &Profile{Name: name}
		profiles[name] = profile
	}
	return profile
}

func metadataLine(line string) (string, string, bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	if metadataValueIsNotNeeded(key) {
		return key, "", true
	}
	return key, strings.TrimSpace(parts[1]), true
}

func metadataValueIsNotNeeded(key string) bool {
	switch key {
	case "login_session", "sso_session", "credential_process", "source_profile", "role_arn",
		"aws_access_key_id", "aws_secret_access_key", "aws_session_token":
		return true
	default:
		return false
	}
}

func applyProfileMetadata(profile *Profile, key string, value string) {
	switch key {
	case "region":
		if safeMetadataValue(value) {
			profile.Region = value
		}
	case "login_session":
		profile.HasLoginSession = true
	case "sso_session":
		profile.HasSSOSession = true
	case "credential_process":
		profile.HasCredentialProcess = true
	case "source_profile":
		profile.HasSourceProfile = true
	case "role_arn":
		profile.HasRoleARN = true
	case "aws_access_key_id", "aws_secret_access_key", "aws_session_token":
		return
	}
}

func safeProfileName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, `/\`) {
		return false
	}
	return safeMetadataValue(value)
}

func safeMetadataValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if sensitiveIdentifierLikeValue(value) {
		return false
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"/users/",
		"/private/",
		"/tmp/",
		"/home/",
		"\\",
		".pem",
		"access_key",
		"apikey",
		"api_key",
		"arn:",
		"bearer ",
		"client-secret",
		"client_secret",
		"ocid1.",
		"passphrase",
		"password",
		"plain-secret",
		"plain-token",
		"private-key",
		"private_key",
		"projects/",
		"refresh_token",
		"secret_key",
		"service_account_json",
		"session_token",
		"token=",
	} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return true
}

func sensitiveIdentifierLikeValue(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 12 && allDigits(value) {
		return true
	}
	upper := strings.ToUpper(value)
	return len(upper) == 20 &&
		(strings.HasPrefix(upper, "AKIA") || strings.HasPrefix(upper, "ASIA")) &&
		allUpperAlphaNumeric(upper)
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func allUpperAlphaNumeric(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return value != ""
}
