package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/audit"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/guided"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	ExitOK             = 0
	ExitUsage          = 2
	ExitNotImplemented = 3
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), stdout, stderr)
}

func RunWithInput(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
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
		if err := guided.Run(stdin, stdout); err != nil {
			writeError(stderr, err.Error())
			return ExitUsage
		}
		return ExitOK
	}

	if assessment.IsProvider(args[0]) {
		writeProviderFirstCorrection(stderr, args[0])
		return ExitUsage
	}

	requestArgs := args
	if hasTrailingHelp(args) {
		requestArgs = args[:len(args)-1]
	}

	request, err := parseRequest(requestArgs)
	if err != nil {
		writeError(stderr, err.Error())
		return ExitUsage
	}

	if hasTrailingHelp(args) {
		writeActionHelp(stdout, request)
		return ExitOK
	}

	result := workflow.DefaultRegistry().Execute(request)
	if err := writeJSON(stdout, result); err != nil {
		writeError(stderr, err.Error())
		return ExitUsage
	}

	if result.Status == workflow.StatusNotImplemented {
		return ExitNotImplemented
	}
	return ExitOK
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
