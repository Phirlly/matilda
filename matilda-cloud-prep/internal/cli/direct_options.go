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
	var cur2Destination string
	var cur2S3BucketRef string
	var timeoutValue string
	var requestBackfill bool
	var confirmCreateSupportCase bool
	var createCUR2Export bool
	var approvePlan string
	var approveSteps repeatedStringFlag

	flags := flag.NewFlagSet("matilda-prep", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&profile, "profile", "", "AWS shared config profile")
	flags.StringVar(&region, "region", "", "AWS region")
	flags.StringVar(&exportRef, "export-ref", "", "Matilda-generated AWS CUR 2.0 export ref")
	flags.StringVar(&cur2Destination, "cur2-destination", "", "AWS CUR 2.0 create-new destination mode")
	flags.StringVar(&cur2S3BucketRef, "cur2-s3-bucket-ref", "", "Matilda-generated AWS S3 bucket ref")
	flags.BoolVar(&requestBackfill, "request-backfill", false, "request previous-month AWS CUR 2.0 backfill")
	flags.BoolVar(&confirmCreateSupportCase, "confirm-create-support-case", false, "confirm AWS Support case creation")
	flags.BoolVar(&createCUR2Export, "create-cur2-export", false, "plan or apply AWS CUR 2.0 export creation")
	flags.StringVar(&approvePlan, "approve-plan", "", "execution plan id for approved cloud mutation steps")
	flags.Var(&approveSteps, "approve-step", "execution plan step id to approve; repeat for each approved step")
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
	if provided["cur2-destination"] && strings.TrimSpace(cur2Destination) == "" {
		return workflow.ExecutionOptions{}, fmt.Errorf("cur2-destination cannot be empty")
	}
	if provided["cur2-s3-bucket-ref"] && strings.TrimSpace(cur2S3BucketRef) == "" {
		return workflow.ExecutionOptions{}, fmt.Errorf("cur2-s3-bucket-ref cannot be empty")
	}
	if provided["approve-plan"] && strings.TrimSpace(approvePlan) == "" {
		return workflow.ExecutionOptions{}, fmt.Errorf("approve-plan cannot be empty")
	}

	cur2DestinationUsed := provided["cur2-destination"] || provided["cur2-s3-bucket-ref"]
	awsSelectorUsed := provided["profile"] || provided["region"] || provided["export-ref"] || cur2DestinationUsed
	if awsSelectorUsed && !isAWSBillingSelectorCommand(request) {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS selector flags are supported only for matilda-prep rapid-assessment billing aws preflight, apply-prereqs, or package")
	}

	createOperationUsed := provided["create-cur2-export"]
	backfillOperationUsed := provided["request-backfill"] || provided["confirm-create-support-case"]
	approvalUsed := provided["approve-plan"] || len(approveSteps) > 0
	if backfillOperationUsed && !isAWSBillingApplyPrereqs(request) {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS backfill operation flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs")
	}
	if (createOperationUsed || approvalUsed) && !isAWSBillingApplyPrereqs(request) {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS billing operation flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs")
	}
	if cur2DestinationUsed && !isAWSBillingApplyPrereqs(request) {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS CUR 2.0 destination selector flags are supported only for matilda-prep rapid-assessment billing aws apply-prereqs --create-cur2-export")
	}
	if cur2DestinationUsed && !createCUR2Export {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS CUR 2.0 destination selector flags require --create-cur2-export")
	}
	if createCUR2Export && backfillOperationUsed {
		return workflow.ExecutionOptions{}, fmt.Errorf("aws_billing_prereqs_operation_conflict")
	}
	if createCUR2Export && provided["export-ref"] {
		return workflow.ExecutionOptions{}, fmt.Errorf("export-ref selects an existing AWS CUR 2.0 export for preflight, package handoff, or apply-prereqs --request-backfill; it cannot be used with --create-cur2-export")
	}
	if confirmCreateSupportCase && !requestBackfill {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS backfill support case confirmation requires --request-backfill")
	}
	if approvalUsed && !createCUR2Export && !requestBackfill {
		return workflow.ExecutionOptions{}, fmt.Errorf("approval flags require a matching AWS billing operation")
	}
	if createCUR2Export && ((strings.TrimSpace(approvePlan) == "") != (len(approveSteps) == 0)) {
		return workflow.ExecutionOptions{}, fmt.Errorf("plan-bound approval requires both --approve-plan and at least one --approve-step")
	}
	if requestBackfill && (confirmCreateSupportCase || approvalUsed) &&
		(!confirmCreateSupportCase || strings.TrimSpace(approvePlan) == "" || len(approveSteps) == 0) {
		return workflow.ExecutionOptions{}, fmt.Errorf("AWS backfill support case approval requires --confirm-create-support-case, --approve-plan, and at least one --approve-step")
	}
	destinationMode, err := parseCUR2DestinationMode(cur2Destination)
	if err != nil {
		return workflow.ExecutionOptions{}, err
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
				Profile:             profile,
				Region:              region,
				CUR2ExportRef:       exportRef,
				CUR2DestinationMode: destinationMode,
				CUR2S3BucketRef:     cur2S3BucketRef,
			},
		}
	}
	if requestBackfill {
		options.AWSBillingOperation = workflow.AWSBillingOperationRequestBackfill
		if confirmCreateSupportCase {
			for _, step := range approveSteps {
				options.Approvals = append(options.Approvals, workflow.ExecutionApproval{
					OperationID: step,
					Intent:      workflow.ApprovalIntentRequestBackfillSupportCase,
					PlanID:      approvePlan,
					Confirmed:   true,
				})
			}
		}
	}
	if createCUR2Export {
		options.AWSBillingOperation = workflow.AWSBillingOperationCreateCUR2Export
		for _, step := range approveSteps {
			options.Approvals = append(options.Approvals, workflow.ExecutionApproval{
				OperationID: step,
				PlanID:      approvePlan,
				Confirmed:   true,
			})
		}
	}
	return workflow.NormalizeExecutionOptionsForRequest(request, options)
}

func parseCUR2DestinationMode(input string) (workflow.AWSCUR2DestinationMode, error) {
	switch strings.TrimSpace(input) {
	case "":
		return "", nil
	case string(workflow.AWSCUR2DestinationGenerated):
		return workflow.AWSCUR2DestinationGenerated, nil
	case "existing-same-account", string(workflow.AWSCUR2DestinationExistingSameAccount):
		return workflow.AWSCUR2DestinationExistingSameAccount, nil
	default:
		return "", fmt.Errorf("cur2-destination is unsupported")
	}
}

type repeatedStringFlag []string

func (values *repeatedStringFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *repeatedStringFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("approve-step cannot be empty")
	}
	*values = append(*values, value)
	return nil
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
		(request.Action == assessment.ActionPreflight || request.Action == assessment.ActionApplyPrereqs || request.Action == assessment.ActionPackage)
}

func isAWSBillingApplyPrereqs(request workflow.Request) bool {
	return request.Goal == assessment.RapidAssessment &&
		request.CollectionPath == assessment.CollectionBilling &&
		request.Provider == assessment.ProviderAWS &&
		request.Action == assessment.ActionApplyPrereqs
}
