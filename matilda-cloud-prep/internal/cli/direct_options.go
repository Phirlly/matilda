package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	defaultDirectTimeout = time.Duration(workflow.DefaultExecutionTimeoutSeconds) * time.Second
	minDirectTimeout     = 10 * time.Second
	maxDirectTimeout     = 30 * time.Minute
)

func parseExecutionOptions(request workflow.Request, args []string) (workflow.ExecutionOptions, error) {
	var profile string
	var region string
	var exportRef string
	var timeoutValue string
	var requestBackfill bool
	var confirmCreateSupportCase bool

	flags := flag.NewFlagSet("matilda-prep", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&profile, "profile", "", "AWS shared config profile")
	flags.StringVar(&region, "region", "", "AWS region")
	flags.StringVar(&exportRef, "export-ref", "", "Matilda-generated AWS CUR 2.0 export ref")
	flags.BoolVar(&requestBackfill, "request-backfill", false, "request previous-month AWS CUR 2.0 backfill")
	flags.BoolVar(&confirmCreateSupportCase, "confirm-create-support-case", false, "confirm AWS Support case creation")
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
	if awsSelectorUsed && !isAWSBillingSelectorCommand(request) {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS selector flags are supported only for matilda-prep rapid-assessment billing aws preflight or apply-prereqs")
	}

	backfillApprovalUsed := provided["request-backfill"] || provided["confirm-create-support-case"]
	if backfillApprovalUsed && !isAWSBillingApplyPrereqs(request) {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS backfill approval flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs")
	}
	if requestBackfill != confirmCreateSupportCase {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS backfill support case approval requires both --request-backfill and --confirm-create-support-case")
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
	if requestBackfill && confirmCreateSupportCase {
		options.Approvals = []workflow.ExecutionApproval{{
			OperationID: workflow.AWSBackfillSupportCaseOperationID,
			Intent:      workflow.ApprovalIntentRequestBackfillSupportCase,
			Confirmed:   true,
		}}
	}
	return workflow.NormalizeExecutionOptionsForRequest(request, options)
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

func isAWSBillingSelectorCommand(request workflow.Request) bool {
	return request.Goal == assessment.RapidAssessment &&
		request.CollectionPath == assessment.CollectionBilling &&
		request.Provider == assessment.ProviderAWS &&
		(request.Action == assessment.ActionPreflight || request.Action == assessment.ActionApplyPrereqs)
}

func isAWSBillingApplyPrereqs(request workflow.Request) bool {
	return request.Goal == assessment.RapidAssessment &&
		request.CollectionPath == assessment.CollectionBilling &&
		request.Provider == assessment.ProviderAWS &&
		request.Action == assessment.ActionApplyPrereqs
}
