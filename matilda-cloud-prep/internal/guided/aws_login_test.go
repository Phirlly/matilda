package guided

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
)

func TestParseAWSCLIVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  awsCLIVersion
		ok    bool
	}{
		{
			name:  "current aws cli",
			input: "aws-cli/2.36.34 Python/3.14.7 Darwin/25.5.0 source/arm64",
			want:  awsCLIVersion{Major: 2, Minor: 36, Patch: 34},
			ok:    true,
		},
		{
			name:  "missing patch",
			input: "aws-cli/2.36 Python/3.14.7",
			ok:    false,
		},
		{
			name:  "not aws cli",
			input: "aws-shell/0.2.2",
			ok:    false,
		},
		{
			name:  "unsafe suffix ignored",
			input: "aws-cli/2.32.0 arn:aws:iam::123456789012:user/example",
			want:  awsCLIVersion{Major: 2, Minor: 32, Patch: 0},
			ok:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseAWSCLIVersion(tt.input)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("version = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestAWSCLIVersionSupportsLogin(t *testing.T) {
	tests := []struct {
		version awsCLIVersion
		want    bool
	}{
		{version: awsCLIVersion{Major: 1, Minor: 40, Patch: 0}, want: false},
		{version: awsCLIVersion{Major: 2, Minor: 31, Patch: 99}, want: false},
		{version: awsCLIVersion{Major: 2, Minor: 32, Patch: 0}, want: true},
		{version: awsCLIVersion{Major: 2, Minor: 36, Patch: 34}, want: true},
		{version: awsCLIVersion{Major: 3, Minor: 0, Patch: 0}, want: true},
	}

	for _, tt := range tests {
		if got := tt.version.supportsLogin(); got != tt.want {
			t.Fatalf("supportsLogin(%#v) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestAWSCLILoginRunnerSupportsLogin(t *testing.T) {
	tests := []struct {
		name       string
		lookErr    error
		versionOut string
		versionErr error
		want       AWSLoginSupport
	}{
		{
			name:       "supported",
			versionOut: "aws-cli/2.36.34 Python/3.14.7",
			want:       AWSLoginSupport{Available: true, Version: "2.36.34"},
		},
		{
			name:    "missing cli",
			lookErr: errors.New("missing"),
			want:    AWSLoginSupport{Available: false, Reason: "aws_cli_unavailable", Message: "AWS CLI was not found on PATH."},
		},
		{
			name:       "version command failed",
			versionErr: errors.New("failed"),
			want:       AWSLoginSupport{Available: false, Reason: "aws_cli_version_unavailable", Message: "AWS CLI version could not be checked."},
		},
		{
			name:       "unrecognized version",
			versionOut: "aws-shell/0.2.2",
			want:       AWSLoginSupport{Available: false, Reason: "aws_cli_version_unrecognized", Message: "AWS CLI version could not be recognized."},
		},
		{
			name:       "unsupported version",
			versionOut: "aws-cli/2.31.99 Python/3.14.7",
			want:       AWSLoginSupport{Available: false, Version: "2.31.99", Reason: "aws_cli_login_unsupported_version", Message: "AWS CLI 2.32.0 or later is required for in-flow login."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookPathCalled := false
			versionCalled := false
			runner := awsCLILoginRunner{
				lookPath: func(name string) (string, error) {
					lookPathCalled = true
					if name != "aws" {
						t.Fatalf("lookPath name = %q, want aws", name)
					}
					if tt.lookErr != nil {
						return "", tt.lookErr
					}
					return "/usr/local/bin/aws", nil
				},
				versionOutput: func(ctx context.Context, path string) ([]byte, error) {
					versionCalled = true
					if path != "/usr/local/bin/aws" {
						t.Fatalf("version path = %q, want /usr/local/bin/aws", path)
					}
					if tt.versionErr != nil {
						return nil, tt.versionErr
					}
					return []byte(tt.versionOut), nil
				},
			}

			got := runner.SupportsLogin(context.Background())

			if got != tt.want {
				t.Fatalf("support = %#v, want %#v", got, tt.want)
			}
			if !lookPathCalled {
				t.Fatal("lookPath was not called")
			}
			if versionCalled == (tt.lookErr != nil) {
				t.Fatalf("versionCalled = %v with lookErr %v", versionCalled, tt.lookErr)
			}
		})
	}
}

func TestAWSCLILoginRunnerLoginRunsSafeProfile(t *testing.T) {
	var gotPath string
	var gotSource billingguide.CredentialSource
	runner := awsCLILoginRunner{
		lookPath: func(name string) (string, error) {
			if name != "aws" {
				t.Fatalf("lookPath name = %q, want aws", name)
			}
			return "/usr/local/bin/aws", nil
		},
		runLogin: func(ctx context.Context, path string, source billingguide.CredentialSource) error {
			gotPath = path
			gotSource = source
			return nil
		},
	}

	err := runner.Login(context.Background(), billingguide.CredentialSource{
		Kind:            billingguide.CredentialSourceProfile,
		Profile:         "default",
		Region:          "us-east-1",
		HasLoginSession: true,
	})

	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if gotPath != "/usr/local/bin/aws" {
		t.Fatalf("login path = %q, want /usr/local/bin/aws", gotPath)
	}
	if gotSource.Profile != "default" || gotSource.Region != "us-east-1" {
		t.Fatalf("login source = %#v, want default/us-east-1", gotSource)
	}
}

func TestAWSCLILoginRunnerLoginRejectsUnsafeSourceBeforeCLI(t *testing.T) {
	called := false
	runner := awsCLILoginRunner{
		lookPath: func(name string) (string, error) {
			called = true
			return "/usr/local/bin/aws", nil
		},
		runLogin: func(ctx context.Context, path string, source billingguide.CredentialSource) error {
			called = true
			return nil
		},
	}

	err := runner.Login(context.Background(), billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "/private/tmp/profile",
		Region:  "us-east-1",
	})

	if err == nil {
		t.Fatal("Login returned nil error for unsafe source")
	}
	if called {
		t.Fatal("AWS CLI should not be called for unsafe source")
	}
}

func TestAWSCLILoginRunnerLoginReportsMissingCLI(t *testing.T) {
	called := false
	runner := awsCLILoginRunner{
		lookPath: func(name string) (string, error) {
			return "", errors.New("missing")
		},
		runLogin: func(ctx context.Context, path string, source billingguide.CredentialSource) error {
			called = true
			return nil
		},
	}

	err := runner.Login(context.Background(), billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "default",
		Region:  "us-east-1",
	})

	if err == nil {
		t.Fatal("Login returned nil error when AWS CLI is missing")
	}
	if called {
		t.Fatal("login command should not run when AWS CLI lookup fails")
	}
}

func TestAWSCLILoginRunnerLoginReportsCommandFailure(t *testing.T) {
	runner := awsCLILoginRunner{
		lookPath: func(name string) (string, error) {
			if name != "aws" {
				t.Fatalf("lookPath name = %q, want aws", name)
			}
			return "/usr/local/bin/aws", nil
		},
		runLogin: func(ctx context.Context, path string, source billingguide.CredentialSource) error {
			if path != "/usr/local/bin/aws" {
				t.Fatalf("login path = %q, want /usr/local/bin/aws", path)
			}
			if source.Profile != "default" {
				t.Fatalf("login profile = %q, want default", source.Profile)
			}
			return errors.New("login failed")
		},
	}

	err := runner.Login(context.Background(), billingguide.CredentialSource{
		Kind:    billingguide.CredentialSourceProfile,
		Profile: "default",
		Region:  "us-east-1",
	})

	if err == nil {
		t.Fatal("Login returned nil error for command failure")
	}
}

func TestNewAWSCLILoginRunnerUsesDefaultAWSCLICommands(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	awsPath := filepath.Join(dir, "aws")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
	printf 'aws-cli/2.36.34 Python/3.14.7\n'
	exit 0
fi
printf '%s\n' "$@" > "$AWS_LOGIN_ARG_FILE"
`
	if err := os.WriteFile(awsPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake aws cli: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AWS_LOGIN_ARG_FILE", argsFile)

	runner := NewAWSCLILoginRunner()

	support := runner.SupportsLogin(context.Background())
	if !support.Available || support.Version != "2.36.34" {
		t.Fatalf("support = %#v, want available AWS CLI 2.36.34", support)
	}

	err := runner.Login(context.Background(), billingguide.CredentialSource{
		Kind:            billingguide.CredentialSourceProfile,
		Profile:         "default",
		Region:          "us-east-1",
		HasLoginSession: true,
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	rawArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read fake aws login args: %v", err)
	}
	if got := strings.TrimSpace(string(rawArgs)); got != "login\n--profile\ndefault" {
		t.Fatalf("aws login args = %q, want login --profile default", got)
	}
}
