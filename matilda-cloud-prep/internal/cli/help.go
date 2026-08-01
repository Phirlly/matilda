package cli

import (
	"fmt"
	"io"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

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

AWS Rapid Assessment - Billing Based preflight/apply-prereqs options:
  --profile <aws-profile-name>
  --region <aws-region>
  --export-ref <cur2-ref-from-previous-output>
  --timeout <duration>

AWS Rapid Assessment - Billing Based apply-prereqs approval options:
  --request-backfill
  --confirm-create-support-case

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
