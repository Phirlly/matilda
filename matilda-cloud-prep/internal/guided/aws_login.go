package guided

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
)

var awsCLIVersionPattern = regexp.MustCompile(`(?:^|\s)aws-cli/([0-9]+)\.([0-9]+)\.([0-9]+)(?:\s|$)`)

type awsCLIVersion struct {
	Major int
	Minor int
	Patch int
}

func (version awsCLIVersion) supportsLogin() bool {
	if version.Major > 2 {
		return true
	}
	if version.Major < 2 {
		return false
	}
	if version.Minor > 32 {
		return true
	}
	if version.Minor < 32 {
		return false
	}
	return version.Patch >= 0
}

func (version awsCLIVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
}

func parseAWSCLIVersion(input string) (awsCLIVersion, bool) {
	matches := awsCLIVersionPattern.FindStringSubmatch(input)
	if len(matches) != 4 {
		return awsCLIVersion{}, false
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return awsCLIVersion{}, false
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return awsCLIVersion{}, false
	}
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return awsCLIVersion{}, false
	}
	return awsCLIVersion{Major: major, Minor: minor, Patch: patch}, true
}

func NewAWSCLILoginRunner() AWSLoginRunner {
	return awsCLILoginRunner{}
}

type awsCLILoginRunner struct {
	lookPath      func(string) (string, error)
	versionOutput func(context.Context, string) ([]byte, error)
	runLogin      func(context.Context, string, billingguide.CredentialSource) error
}

func (runner awsCLILoginRunner) SupportsLogin(ctx context.Context) AWSLoginSupport {
	path, err := runner.awsPath()
	if err != nil {
		return AWSLoginSupport{
			Available: false,
			Reason:    "aws_cli_unavailable",
			Message:   "AWS CLI was not found on PATH.",
		}
	}

	output, err := runner.awsVersionOutput(ctx, path)
	if err != nil {
		return AWSLoginSupport{
			Available: false,
			Reason:    "aws_cli_version_unavailable",
			Message:   "AWS CLI version could not be checked.",
		}
	}

	version, ok := parseAWSCLIVersion(string(output))
	if !ok {
		return AWSLoginSupport{
			Available: false,
			Reason:    "aws_cli_version_unrecognized",
			Message:   "AWS CLI version could not be recognized.",
		}
	}
	if !version.supportsLogin() {
		return AWSLoginSupport{
			Available: false,
			Version:   version.String(),
			Reason:    "aws_cli_login_unsupported_version",
			Message:   "AWS CLI 2.32.0 or later is required for in-flow login.",
		}
	}
	return AWSLoginSupport{
		Available: true,
		Version:   version.String(),
	}
}

func (runner awsCLILoginRunner) Login(ctx context.Context, source billingguide.CredentialSource) error {
	safeSource, ok := safeAWSCredentialSource(source)
	if !ok || safeSource.Kind != billingguide.CredentialSourceProfile || safeSource.Profile == "" {
		return fmt.Errorf("aws login source is not safe")
	}

	path, err := runner.awsPath()
	if err != nil {
		return fmt.Errorf("aws cli is unavailable")
	}
	return runner.awsLogin(ctx, path, safeSource)
}

func (runner awsCLILoginRunner) awsPath() (string, error) {
	if runner.lookPath != nil {
		return runner.lookPath("aws")
	}
	return exec.LookPath("aws")
}

func (runner awsCLILoginRunner) awsVersionOutput(ctx context.Context, path string) ([]byte, error) {
	if runner.versionOutput != nil {
		return runner.versionOutput(ctx, path)
	}
	return exec.CommandContext(ctx, path, "--version").CombinedOutput()
}

func (runner awsCLILoginRunner) awsLogin(ctx context.Context, path string, source billingguide.CredentialSource) error {
	if runner.runLogin != nil {
		return runner.runLogin(ctx, path, source)
	}

	command := exec.CommandContext(ctx, path, "login", "--profile", source.Profile)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
