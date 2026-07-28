package authdiscovery

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverProfilesParsesSharedConfigAndCredentialsSafely(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	credentialsPath := filepath.Join(dir, "credentials")

	writeFile(t, configPath, strings.Join([]string{
		"[default]",
		"region = us-east-1",
		"login_session = corp",
		"aws_access_key_id = AKIAEXAMPLE",
		"",
		"[profile prod]",
		"region = us-west-2",
		"sso_session = workforce",
		"credential_process = aws configure export-credentials --profile prod",
		"",
		"[sso-session workforce]",
		"sso_start_url = https://example.awsapps.com/start",
		"",
		"[services local-services]",
		"s3 =",
	}, "\n"))
	writeFile(t, credentialsPath, strings.Join([]string{
		"[default]",
		"aws_secret_access_key = plain-secret-key",
		"aws_session_token = plain-session-token",
		"",
		"[finance]",
		"region = us-east-2",
		"source_profile = default",
		"role_arn = arn:aws:iam::123456789012:role/finance",
	}, "\n"))

	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{
			"AWS_CONFIG_FILE":             configPath,
			"AWS_SHARED_CREDENTIALS_FILE": credentialsPath,
		}),
		ReadFile: os.ReadFile,
		HomeDir:  func() (string, error) { return dir, nil },
	}.Discover(context.Background())

	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	got := profileSummaries(profiles)
	want := []string{
		"default|region=us-east-1|login=true|sso=false|process=false|source=false|role=false",
		"finance|region=us-east-2|login=false|sso=false|process=false|source=true|role=true",
		"prod|region=us-west-2|login=false|sso=true|process=true|source=false|role=false",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles = %#v, want %#v", got, want)
	}
	for _, forbidden := range []string{"AKIA", "plain-secret", "plain-session-token", "arn:aws", configPath, credentialsPath} {
		if strings.Contains(fmt.Sprintf("%#v", profiles), forbidden) {
			t.Fatalf("profiles leaked forbidden value %q: %#v", forbidden, profiles)
		}
	}
}

func TestDiscoverProfilesTreatsMissingAndUnreadableFilesAsRecoverable(t *testing.T) {
	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{
			"AWS_CONFIG_FILE":             "/missing/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/missing/credentials",
		}),
		ReadFile: func(path string) ([]byte, error) {
			return nil, os.ErrPermission
		},
		HomeDir: func() (string, error) { return "", os.ErrNotExist },
	}.Discover(context.Background())

	if err != nil {
		t.Fatalf("Discover returned error for recoverable file failures: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("profiles = %#v, want none", profiles)
	}
}

func TestDiscoverProfilesStreamsSharedFilesWhenOpenFileIsConfigured(t *testing.T) {
	opened := []string{}
	closed := 0
	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{
			"AWS_CONFIG_FILE":             "/safe/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/safe/credentials",
		}),
		OpenFile: func(path string) (io.ReadCloser, error) {
			opened = append(opened, path)
			switch path {
			case "/safe/config":
				return closeCounter{Reader: strings.NewReader("[profile streamed]\nregion = us-east-1\ncredential_process = /Users/example/export --token=plain-token\n"), close: func() { closed++ }}, nil
			case "/safe/credentials":
				return closeCounter{Reader: strings.NewReader("[streamed-creds]\nregion = us-west-2\naws_secret_access_key = plain-secret-key\n"), close: func() { closed++ }}, nil
			default:
				t.Fatalf("unexpected open path %q", path)
				return nil, os.ErrNotExist
			}
		},
		ReadFile: func(path string) ([]byte, error) {
			t.Fatalf("ReadFile should not be used when OpenFile is configured for %s", path)
			return nil, nil
		},
	}.Discover(context.Background())

	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if !reflect.DeepEqual(opened, []string{"/safe/config", "/safe/credentials"}) {
		t.Fatalf("opened paths = %#v, want config then credentials", opened)
	}
	if closed != 2 {
		t.Fatalf("closed files = %d, want 2", closed)
	}
	got := profileSummaries(profiles)
	want := []string{
		"streamed|region=us-east-1|login=false|sso=false|process=true|source=false|role=false",
		"streamed-creds|region=us-west-2|login=false|sso=false|process=false|source=false|role=false",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles = %#v, want %#v", got, want)
	}
	for _, forbidden := range []string{"/Users/", "plain-token", "plain-secret"} {
		if strings.Contains(fmt.Sprintf("%#v", profiles), forbidden) {
			t.Fatalf("profiles leaked forbidden streamed value %q: %#v", forbidden, profiles)
		}
	}
}

func TestDiscoverProfilesTreatsOpenFileErrorsAsRecoverable(t *testing.T) {
	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{
			"AWS_CONFIG_FILE":             "/missing/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/missing/credentials",
		}),
		OpenFile: func(path string) (io.ReadCloser, error) {
			return nil, os.ErrPermission
		},
	}.Discover(context.Background())

	if err != nil {
		t.Fatalf("Discover returned error for recoverable open failures: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("profiles = %#v, want none", profiles)
	}
}

func TestDiscoverProfilesHonorsContextCancellationBeforeReadingFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	readCalled := false

	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{
			"AWS_CONFIG_FILE":             "/safe/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/safe/credentials",
		}),
		ReadFile: func(path string) ([]byte, error) {
			readCalled = true
			return nil, nil
		},
	}.Discover(ctx)

	if err != context.Canceled {
		t.Fatalf("Discover error = %v, want context canceled", err)
	}
	if readCalled {
		t.Fatal("ReadFile should not be called after context cancellation")
	}
	if len(profiles) != 0 {
		t.Fatalf("profiles = %#v, want none", profiles)
	}
}

func TestDiscoverProfilesUsesDefaultSharedFileLocationsWhenEnvOverridesAreUnset(t *testing.T) {
	dir := t.TempDir()
	awsDir := filepath.Join(dir, ".aws")
	if err := os.Mkdir(awsDir, 0o700); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	configPath := filepath.Join(awsDir, "config")
	credentialsPath := filepath.Join(awsDir, "credentials")
	writeFile(t, configPath, "[profile defaulted]\nregion = us-west-1\n")
	writeFile(t, credentialsPath, "[creds]\nregion = us-east-2\n")
	var readPaths []string

	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{}),
		ReadFile: func(path string) ([]byte, error) {
			readPaths = append(readPaths, path)
			return os.ReadFile(path)
		},
		HomeDir: func() (string, error) { return dir, nil },
	}.Discover(context.Background())

	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if !reflect.DeepEqual(readPaths, []string{configPath, credentialsPath}) {
		t.Fatalf("read paths = %#v, want default config and credentials paths", readPaths)
	}
	got := profileSummaries(profiles)
	want := []string{
		"creds|region=us-east-2|login=false|sso=false|process=false|source=false|role=false",
		"defaulted|region=us-west-1|login=false|sso=false|process=false|source=false|role=false",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles = %#v, want %#v", got, want)
	}
}

func TestDiscoverProfilesSkipsDefaultFilesWhenHomeDirectoryIsUnavailable(t *testing.T) {
	readCalled := false

	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{}),
		ReadFile: func(path string) ([]byte, error) {
			readCalled = true
			return nil, nil
		},
		HomeDir: func() (string, error) { return "", os.ErrNotExist },
	}.Discover(context.Background())

	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if readCalled {
		t.Fatal("ReadFile should not run when no shared file path can be resolved")
	}
	if len(profiles) != 0 {
		t.Fatalf("profiles = %#v, want none", profiles)
	}
}

func TestDiscoverProfilesStopsAfterConfigWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	credentialsPath := filepath.Join(dir, "credentials")
	readPaths := []string{}

	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{
			"AWS_CONFIG_FILE":             configPath,
			"AWS_SHARED_CREDENTIALS_FILE": credentialsPath,
		}),
		ReadFile: func(path string) ([]byte, error) {
			readPaths = append(readPaths, path)
			cancel()
			return []byte("[profile config-only]\nregion = us-east-1\n"), nil
		},
	}.Discover(ctx)

	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if !reflect.DeepEqual(readPaths, []string{configPath}) {
		t.Fatalf("read paths = %#v, want only config before cancellation", readPaths)
	}
	got := profileSummaries(profiles)
	want := []string{"config-only|region=us-east-1|login=false|sso=false|process=false|source=false|role=false"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles = %#v, want %#v", got, want)
	}
}

func TestDiscoverProfilesFiltersUnsafeMetadataAndIgnoresNonProfileSections(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	credentialsPath := filepath.Join(dir, "credentials")
	writeFile(t, configPath, strings.Join([]string{
		"# comment",
		"; another comment",
		"orphan = ignored",
		"[profile safe]",
		"region = /Users/example/.aws/private",
		"not-a-key-value",
		"[services local]",
		"region = us-west-2",
		"[sso-session workforce]",
		"sso_region = us-east-1",
	}, "\n"))
	writeFile(t, credentialsPath, strings.Join([]string{
		"[also-safe]",
		"region = us-east-1",
		"aws_access_key_id = AKIAEXAMPLE",
		"aws_secret_access_key = plain-secret-key",
		"[profile should-not-load]",
		"region = us-west-2",
		"[sso-session ignored]",
		"region = us-east-2",
		"[services ignored]",
		"region = us-west-1",
	}, "\n"))

	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{
			"AWS_CONFIG_FILE":             configPath,
			"AWS_SHARED_CREDENTIALS_FILE": credentialsPath,
		}),
		ReadFile: os.ReadFile,
		HomeDir:  func() (string, error) { return dir, nil },
	}.Discover(context.Background())

	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	got := profileSummaries(profiles)
	want := []string{
		"also-safe|region=us-east-1|login=false|sso=false|process=false|source=false|role=false",
		"safe|region=|login=false|sso=false|process=false|source=false|role=false",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles = %#v, want %#v", got, want)
	}
	for _, forbidden := range []string{"AKIA", "plain-secret", "/Users/", "services", "sso-session", "should-not-load"} {
		if strings.Contains(fmt.Sprintf("%#v", profiles), forbidden) {
			t.Fatalf("profiles leaked forbidden value or non-profile section %q: %#v", forbidden, profiles)
		}
	}
}

func TestDiscoverProfilesRejectsUnsafeProfileNamesFromOutput(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	writeFile(t, configPath, strings.Join([]string{
		"[profile arn:aws:iam::123456789012:role/operator]",
		"region = us-east-1",
		"[profile 123456789012]",
		"region = us-east-1",
		"[profile AKIAIOSFODNN7EXAMPLE]",
		"region = us-east-1",
		"[profile /Users/lly/.aws/private]",
		"region = us-east-1",
		"[profile safe]",
		"region = us-east-2",
	}, "\n"))

	profiles, err := Discoverer{
		EnvLookup: envLookup(map[string]string{"AWS_CONFIG_FILE": configPath}),
		ReadFile:  os.ReadFile,
		HomeDir:   func() (string, error) { return dir, nil },
	}.Discover(context.Background())

	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	got := profileSummaries(profiles)
	want := []string{"safe|region=us-east-2|login=false|sso=false|process=false|source=false|role=false"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles = %#v, want %#v", got, want)
	}
}

func TestMetadataLineSkipsSensitiveAndBooleanValuesBeforeParsing(t *testing.T) {
	tests := []struct {
		line      string
		wantKey   string
		wantValue string
	}{
		{line: "aws_secret_access_key = plain-secret-key", wantKey: "aws_secret_access_key"},
		{line: "aws_session_token = plain-session-token", wantKey: "aws_session_token"},
		{line: "credential_process = /Users/example/bin/export-creds --token=plain-token", wantKey: "credential_process"},
		{line: "role_arn = arn:aws:iam::123456789012:role/finance", wantKey: "role_arn"},
		{line: "source_profile = default", wantKey: "source_profile"},
		{line: "region = us-east-1", wantKey: "region", wantValue: "us-east-1"},
	}

	for _, tt := range tests {
		t.Run(tt.wantKey, func(t *testing.T) {
			key, value, ok := metadataLine(tt.line)

			if !ok {
				t.Fatalf("metadataLine(%q) returned ok false", tt.line)
			}
			if key != tt.wantKey || value != tt.wantValue {
				t.Fatalf("metadataLine(%q) = (%q, %q), want (%q, %q)", tt.line, key, value, tt.wantKey, tt.wantValue)
			}
			for _, forbidden := range []string{"plain-secret", "plain-session", "plain-token", "/Users/", "arn:aws", "123456789012"} {
				if strings.Contains(value, forbidden) {
					t.Fatalf("metadataLine retained sensitive value %q from %q", forbidden, tt.line)
				}
			}
		})
	}
}

func writeFile(t *testing.T, path string, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) returned error: %v", path, err)
	}
}

func envLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func profileSummaries(profiles []Profile) []string {
	summaries := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		summaries = append(summaries, fmt.Sprintf(
			"%s|region=%s|login=%t|sso=%t|process=%t|source=%t|role=%t",
			profile.Name,
			profile.Region,
			profile.HasLoginSession,
			profile.HasSSOSession,
			profile.HasCredentialProcess,
			profile.HasSourceProfile,
			profile.HasRoleARN,
		))
	}
	return summaries
}

type closeCounter struct {
	*strings.Reader
	close func()
}

func (reader closeCounter) Close() error {
	reader.close()
	return nil
}
