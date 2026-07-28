package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/audit"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/bootstrap"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/guided"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	ExitOK             = 0
	ExitFailed         = 1
	ExitUsage          = 2
	ExitNotImplemented = 3
	ExitBlocked        = 4
)

const (
	defaultDirectTimeout = time.Duration(workflow.DefaultExecutionTimeoutSeconds) * time.Second
	minDirectTimeout     = 10 * time.Second
	maxDirectTimeout     = 30 * time.Minute
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), stdout, stderr)
}

func RunWithInput(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	registry := bootstrap.DefaultRegistry()
	return runWithRuntime(args, stdin, stdout, stderr, registry, bootstrap.DefaultGuidedConfig(registry))
}

func RunWithRegistry(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, registry workflow.Registry) int {
	return runWithRuntime(args, stdin, stdout, stderr, registry, guided.Config{Registry: registry})
}

func runWithRuntime(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, registry workflow.Registry, guidedConfig guided.Config) int {
	if len(args) == 0 {
		writeError(stderr, "usage: expected matilda-prep start, rapid-assessment, deep-discovery, --help, or --version")
		return ExitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		writeHelp(stdout)
		return ExitOK
	case "--version", "version":
		fmt.Fprintln(stdout, "matilda-prep dev")
		return ExitOK
	case "start":
		if len(args) != 1 {
			writeError(stderr, "usage: matilda-prep start")
			return ExitUsage
		}
		if err := guided.RunWithConfig(stdin, stdout, guidedConfig); err != nil {
			writeError(stderr, err.Error())
			return ExitUsage
		}
		return ExitOK
	}

	if assessment.IsProvider(args[0]) {
		writeProviderFirstCorrection(stderr, args[0])
		return ExitUsage
	}

	request, options, helpRequested, err := parseDirectCommand(args)
	if err != nil {
		writeError(stderr, err.Error())
		return ExitUsage
	}

	if helpRequested {
		writeActionHelp(stdout, request)
		return ExitOK
	}

	ctx := context.Background()
	if options.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(options.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	result := registry.ExecuteContext(ctx, request, options)
	if err := writeJSON(stdout, result); err != nil {
		writeError(stderr, err.Error())
		return ExitUsage
	}

	switch result.Status {
	case workflow.StatusNotImplemented:
		return ExitNotImplemented
	case workflow.RunStatusBlocked:
		return ExitBlocked
	case workflow.RunStatusFailed:
		return ExitFailed
	default:
		return ExitOK
	}
}

func parseDirectCommand(args []string) (workflow.Request, workflow.ExecutionOptions, bool, error) {
	helpRequested := hasTrailingHelp(args)
	if helpRequested {
		args = args[:len(args)-1]
	}

	positionals, flagArgs, err := splitDirectCommand(args)
	if err != nil {
		return workflow.Request{}, workflow.ExecutionOptions{}, false, err
	}
	request, err := parseRequest(positionals)
	if err != nil {
		return workflow.Request{}, workflow.ExecutionOptions{}, false, err
	}
	options, err := parseExecutionOptions(request, flagArgs)
	if err != nil {
		return workflow.Request{}, workflow.ExecutionOptions{}, false, err
	}
	return request, options, helpRequested, nil
}

func splitDirectCommand(args []string) ([]string, []string, error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("usage: expected matilda-prep rapid-assessment or deep-discovery command")
	}

	var positionalCount int
	switch args[0] {
	case string(assessment.RapidAssessment):
		positionalCount = 4
	case string(assessment.DeepDiscovery):
		positionalCount = 3
	default:
		return args, nil, nil
	}
	if len(args) < positionalCount {
		return args, nil, nil
	}
	return args[:positionalCount], args[positionalCount:], nil
}

func parseExecutionOptions(request workflow.Request, args []string) (workflow.ExecutionOptions, error) {
	var profile string
	var region string
	var exportRef string
	var timeoutValue string

	flags := flag.NewFlagSet("matilda-prep", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&profile, "profile", "", "AWS shared config profile")
	flags.StringVar(&region, "region", "", "AWS region")
	flags.StringVar(&exportRef, "export-ref", "", "Matilda-generated AWS CUR 2.0 export ref")
	flags.StringVar(&timeoutValue, "timeout", defaultDirectTimeout.String(), "execution timeout")
	if err := flags.Parse(args); err != nil {
		return workflow.ExecutionOptions{}, safeFlagParseError(err)
	}
	if flags.NArg() != 0 {
		return workflow.ExecutionOptions{}, fmt.Errorf("unexpected argument after command")
	}

	provided := map[string]bool{}
	flags.Visit(func(f *flag.Flag) {
		provided[f.Name] = true
	})
	if provided["profile"] && strings.TrimSpace(profile) == "" {
		return workflow.ExecutionOptions{}, fmt.Errorf("profile cannot be empty")
	}
	if provided["region"] && strings.TrimSpace(region) == "" {
		return workflow.ExecutionOptions{}, fmt.Errorf("region cannot be empty")
	}
	if provided["export-ref"] && strings.TrimSpace(exportRef) == "" {
		return workflow.ExecutionOptions{}, fmt.Errorf("export-ref cannot be empty")
	}

	awsSelectorUsed := provided["profile"] || provided["region"] || provided["export-ref"]
	if awsSelectorUsed && !isAWSBillingPreflight(request) {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS selector flags are supported only for matilda-prep rapid-assessment billing aws preflight")
	}

	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil {
		return workflow.ExecutionOptions{}, fmt.Errorf("invalid timeout value")
	}
	if timeout < minDirectTimeout || timeout > maxDirectTimeout {
		return workflow.ExecutionOptions{}, fmt.Errorf("timeout must be between 10s and 30m")
	}
	if timeout%time.Second != 0 {
		return workflow.ExecutionOptions{}, fmt.Errorf("timeout must use whole seconds")
	}

	options := workflow.ExecutionOptions{
		InterfaceMode:  workflow.InterfaceModeDirect,
		TimeoutSeconds: int(timeout.Seconds()),
	}
	if awsSelectorUsed {
		options.Selectors = &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				Profile:       profile,
				Region:        region,
				CUR2ExportRef: exportRef,
			},
		}
	}
	return workflow.NormalizeExecutionOptions(options)
}

func safeFlagParseError(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "flag provided but not defined"):
		return fmt.Errorf("flag provided but not defined")
	case strings.Contains(message, "flag needs an argument"):
		return fmt.Errorf("flag needs an argument")
	default:
		return fmt.Errorf("invalid command flags")
	}
}

func isAWSBillingPreflight(request workflow.Request) bool {
	return request.Goal == assessment.RapidAssessment &&
		request.CollectionPath == assessment.CollectionBilling &&
		request.Provider == assessment.ProviderAWS &&
		request.Action == assessment.ActionPreflight
}

func parseRequest(args []string) (workflow.Request, error) {
	goal, err := assessment.ParseGoal(args[0])
	if err != nil {
		return workflow.Request{}, err
	}

	switch goal {
	case assessment.RapidAssessment:
		return parseRapidAssessment(args)
	case assessment.DeepDiscovery:
		return parseDeepDiscovery(args)
	default:
		return workflow.Request{}, fmt.Errorf("invalid goal %q: expected rapid-assessment or deep-discovery", args[0])
	}
}

func parseRapidAssessment(args []string) (workflow.Request, error) {
	if len(args) != 4 {
		return workflow.Request{}, fmt.Errorf("usage: matilda-prep rapid-assessment <billing|api> <provider> <action>")
	}

	collectionPath, err := assessment.ParseCollectionPath(args[1])
	if err != nil {
		return workflow.Request{}, err
	}
	provider, err := assessment.ParseProvider(args[2])
	if err != nil {
		return workflow.Request{}, err
	}
	action, err := assessment.ParseAction(args[3])
	if err != nil {
		return workflow.Request{}, err
	}

	return workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: collectionPath,
		Provider:       provider,
		Action:         action,
	}, nil
}

func parseDeepDiscovery(args []string) (workflow.Request, error) {
	if len(args) != 3 {
		return workflow.Request{}, fmt.Errorf("usage: matilda-prep deep-discovery <provider> <action>")
	}

	provider, err := assessment.ParseProvider(args[1])
	if err != nil {
		return workflow.Request{}, err
	}
	action, err := assessment.ParseAction(args[2])
	if err != nil {
		return workflow.Request{}, err
	}

	return workflow.Request{
		Goal:     assessment.DeepDiscovery,
		Provider: provider,
		Action:   action,
	}, nil
}

func hasTrailingHelp(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[len(args)-1] {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func writeHelp(stdout io.Writer) {
	fmt.Fprint(stdout, `matilda-prep prepares cloud-side prerequisites for Matilda.

Usage:
  matilda-prep start
  matilda-prep rapid-assessment <billing|api> <provider> <action>
  matilda-prep deep-discovery <provider> <action>

Providers:
  aws, azure, gcp, oci

Actions:
  preflight, apply-prereqs, validate, package

AWS Rapid Assessment - Billing Based preflight options:
  --profile <aws-profile-name>
  --region <aws-region>
  --export-ref <cur2-ref-from-previous-output>
  --timeout <duration>

Use matilda-prep start for guided setup.
`)
}

func writeActionHelp(stdout io.Writer, request workflow.Request) {
	fmt.Fprintln(stdout, commandString(request))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, actionPurpose(request.Action))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Use matilda-prep start for guided setup.")
}

func commandString(request workflow.Request) string {
	if request.Goal == assessment.DeepDiscovery {
		return fmt.Sprintf("matilda-prep %s %s %s", request.Goal, request.Provider, request.Action)
	}
	return fmt.Sprintf("matilda-prep %s %s %s %s", request.Goal, request.CollectionPath, request.Provider, request.Action)
}

func actionPurpose(action assessment.Action) string {
	contract, ok := workflow.ActionContractFor(action)
	if !ok {
		return "Cloud mutation: unknown. Unsupported actions fail closed."
	}
	return fmt.Sprintf("Cloud mutation: %s. %s", mutationHelp(contract.MutationLevel), contract.Purpose)
}

func mutationHelp(level workflow.MutationLevel) string {
	switch level {
	case workflow.MutationNone:
		return "no"
	case workflow.MutationCloud:
		return "yes, only after explicit approval in implemented provider paths"
	case workflow.MutationLocalOnly:
		return "local files only"
	default:
		return "unknown"
	}
}

func writeProviderFirstCorrection(stderr io.Writer, provider string) {
	writeError(stderr, fmt.Sprintf(`Provider-first command order is not supported.
Use guided flow: matilda-prep start
Use objective-first direct syntax:
  matilda-prep rapid-assessment billing %s preflight
  matilda-prep rapid-assessment api %s preflight
  matilda-prep deep-discovery %s preflight`, provider, provider, provider))
}

func writeJSON(stdout io.Writer, result workflow.Result) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, audit.RedactString(string(encoded)))
	return err
}

func writeError(stderr io.Writer, message string) {
	fmt.Fprintln(stderr, audit.RedactString(message))
}
