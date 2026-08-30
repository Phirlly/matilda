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
`)
	writeAWSBillingPreflightHelp(stdout)
	writeAWSBillingApplyPrereqsHelp(stdout)
	writeAWSBillingPackageHelp(stdout)
	fmt.Fprint(stdout, `
Use matilda-prep start for guided setup.
`)
}

func writeAWSBillingPreflightHelp(stdout io.Writer) {
	fmt.Fprint(stdout, `
AWS Rapid Assessment - Billing Based preflight/backfill selection options:
  --profile <aws-profile-name>
  --region <aws-region>
  --export-ref <cur2-ref-from-previous-output>
  --timeout <duration>
`)
}

func writeAWSBillingPackageHelp(stdout io.Writer) {
	fmt.Fprint(stdout, `
AWS Rapid Assessment - Billing Based package handoff options:
  --profile <aws-profile-name>
  --region <aws-region>
  --export-ref <cur2-ref-from-previous-output>
  --timeout <duration>
`)
}

func writeAWSBillingApplyPrereqsHelp(stdout io.Writer) {
	fmt.Fprint(stdout, `
AWS Rapid Assessment - Billing Based apply-prereqs operation options:
  --request-backfill       plan an AWS Support request for previous-month CUR 2.0 backfill
  --create-cur2-export     plan or apply AWS CUR 2.0 export creation
  --cur2-destination <generated|existing-same-account>
  --cur2-s3-bucket-ref <s3b-ref-from-previous-output>

AWS Rapid Assessment - Billing Based apply-prereqs approval options:
  --confirm-create-support-case
  --approve-plan <plan-id>
  --approve-step <plan-step-id>

Backfill support case creation requires:
  --request-backfill --confirm-create-support-case --approve-plan <plan-id> --approve-step aws.billing.cur2.previous_month_backfill_support_case

CUR 2.0 export creation requires:
  --create-cur2-export --approve-plan <plan-id> --approve-step <plan-step-id>

For CUR 2.0 export creation, repeat --approve-step for each mutating step ID returned by the current plan.
`)
}

func writeActionHelp(stdout io.Writer, request workflow.Request) {
	fmt.Fprintln(stdout, commandString(request))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, actionPurpose(request.Action))
	switch {
	case isAWSBillingPreflightRequest(request):
		writeAWSBillingPreflightHelp(stdout)
	case isAWSBillingApplyPrereqsRequest(request):
		writeAWSBillingApplyPrereqsHelp(stdout)
	case isAWSBillingPackageRequest(request):
		writeAWSBillingPackageHelp(stdout)
	}
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
		return "local output only"
	default:
		return "unknown"
	}
}

func isAWSBillingApplyPrereqsRequest(request workflow.Request) bool {
	return request.Goal == assessment.RapidAssessment &&
		request.CollectionPath == assessment.CollectionBilling &&
		request.Provider == assessment.ProviderAWS &&
		request.Action == assessment.ActionApplyPrereqs
}

func isAWSBillingPreflightRequest(request workflow.Request) bool {
	return request.Goal == assessment.RapidAssessment &&
		request.CollectionPath == assessment.CollectionBilling &&
		request.Provider == assessment.ProviderAWS &&
		request.Action == assessment.ActionPreflight
}

func isAWSBillingPackageRequest(request workflow.Request) bool {
	return request.Goal == assessment.RapidAssessment &&
		request.CollectionPath == assessment.CollectionBilling &&
		request.Provider == assessment.ProviderAWS &&
		request.Action == assessment.ActionPackage
}

func writeProviderFirstCorrection(stderr io.Writer, provider string) {
	writeError(stderr, fmt.Sprintf(`Provider-first command order is not supported.
Use guided flow: matilda-prep start
Use objective-first direct syntax:
  matilda-prep rapid-assessment billing %s preflight
  matilda-prep rapid-assessment api %s preflight
  matilda-prep deep-discovery %s preflight`, provider, provider, provider))
}
