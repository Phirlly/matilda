package billinghandoff

import (
	"context"
	"regexp"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/s3handoff"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/handoff"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const handoffType = "aws_rapid_assessment_billing_cur2"

var previousBillingPeriodPattern = regexp.MustCompile(`^\d{4}-\d{2}$`)

type RunnerConfig struct {
	PreflightRunner workflow.CapabilityRunner
}

type Runner struct {
	preflightRunner workflow.CapabilityRunner
}

func NewRunner(config RunnerConfig) Runner {
	return Runner{preflightRunner: config.PreflightRunner}
}

func AWSBillingPackageRequest() workflow.Request {
	return workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPackage,
	}
}

func (runner Runner) Run(ctx context.Context, request workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
	exportRef := selectedExportRef(options)
	if exportRef == "" {
		return runner.blockedReport(request, options, "aws_cur2_package_export_ref_required", "Select an AWS CUR 2.0 export before generating the billing handoff.")
	}
	if runner.preflightRunner == nil {
		return runner.blockedReport(request, options, "aws_provider_capability_blocked", "AWS CUR 2.0 preflight is not available for the package handoff path.")
	}

	preflight := runner.preflightRunner.Run(ctx, cur2preflight.AWSBillingPreflightRequest(), options)
	if preflight.Status != workflow.StatusReady {
		if preflight.Code == "aws_provider_capability_blocked" {
			return runner.blockedReport(request, options, preflight.Code, "AWS CUR 2.0 preflight provider dependency is not available.")
		}
		return runner.blockedReport(request, options, "aws_cur2_package_preflight_not_ready", "AWS CUR 2.0 preflight must be ready before generating the billing handoff.")
	}
	if preflight.PlanInput == nil {
		return runner.blockedReport(request, options, "aws_cur2_package_preflight_not_ready", "AWS CUR 2.0 preflight did not return a validated execution plan.")
	}
	if preflightReadinessBlocked(preflight.PlanInput.Checks) {
		return runner.blockedReport(request, options, "aws_cur2_package_preflight_not_ready", "AWS CUR 2.0 preflight still contains a blocking readiness check.")
	}
	if hasUnclassifiedWarning(preflight.PlanInput.Checks) {
		return runner.blockedReport(request, options, "aws_cur2_package_warning_unclassified", "AWS CUR 2.0 preflight contains an unclassified warning for package handoff.")
	}
	if !preflightCodeAllowedForPackage(preflight.Code) {
		return runner.blockedReport(request, options, "aws_cur2_package_preflight_not_ready", "AWS CUR 2.0 preflight returned an unapproved ready state for package handoff.")
	}

	evidence, ok := safeHandoffEvidence(exportRef, preflight.PlanInput.Checks)
	if !ok {
		return runner.blockedReport(request, options, "aws_cur2_package_handoff_evidence_incomplete", "AWS CUR 2.0 preflight did not include the complete safe handoff evidence.")
	}

	output := handoff.BuildOutput(handoff.Output{
		HandoffType:    handoffType,
		Assessment:     string(assessment.RapidAssessment),
		CollectionPath: string(assessment.CollectionBilling),
		Provider:       string(assessment.ProviderAWS),
		Summary:        "AWS CUR 2.0 billing handoff is ready.",
		Fields: []handoff.Field{
			{Key: "selected_export_ref", Label: "Selected CUR 2.0 export", Value: exportRef},
			{Key: "billing_source", Label: "Billing source", Value: evidence.curVersion},
			{Key: "readiness_status", Label: "Readiness", Value: "ready"},
			{Key: "output_format", Label: "Output format", Value: evidence.outputFormat},
			{Key: "compression", Label: "Compression", Value: evidence.compression},
			{Key: "time_granularity", Label: "Time granularity", Value: evidence.timeGranularity},
			{Key: "file_versioning", Label: "File versioning", Value: evidence.overwrite},
			{Key: "aws_delivery_status", Label: "AWS delivery", Value: "ready"},
			{Key: "s3_delivery_policy_readiness", Label: "S3 delivery policy readiness", Value: deliveryPolicyReadiness(preflight)},
			{Key: "s3_bucket", Label: "S3 bucket", Value: evidence.s3Bucket},
			{Key: "s3_prefix", Label: "S3 prefix", Value: evidence.s3Prefix},
			{Key: "s3_region", Label: "S3 region", Value: evidence.s3Region},
			{Key: "previous_billing_period", Label: "Previous billing period", Value: evidence.previousBillingPeriod},
			{Key: "cur2_data_prefix", Label: "CUR 2.0 data prefix", Value: evidence.dataPrefix},
			{Key: "cur2_manifest_prefix", Label: "CUR 2.0 manifest prefix", Value: evidence.manifestPrefix},
		},
		NextSteps: []string{
			"Use an AWS cloud account with Skip Configuration in Matilda SaaS for Rapid Assessment - Billing Based.",
			"Create the Billing Based assessment in Matilda SaaS, select the customer, provide an assessment name, and upload the billing file from the S3 location shown in this handoff.",
			"This tool does not download billing files and does not upload files to Matilda SaaS.",
			"If direct portal upload cannot accept the billing data size, use Matilda's alternate large-file utility process outside this tool.",
		},
		Warnings: handoffWarnings(preflight),
	})
	if err := handoff.ValidateOutput(&output); err != nil {
		return runner.blockedReport(request, options, "aws_cur2_package_handoff_evidence_incomplete", "AWS CUR 2.0 handoff evidence did not pass safe-output validation.")
	}

	return workflow.CapabilityReport{
		Status:        workflow.StatusReady,
		SupportStatus: workflow.SupportSupported,
		Code:          "aws_cur2_package_handoff_ready",
		Message:       "AWS CUR 2.0 billing handoff is ready.",
		Mutated:       false,
		SourceHandles: packageSourceHandles(),
		PlanInput:     runner.planInput(request, options, workflow.CheckPass, "aws_cur2_package_handoff_ready", "Safe AWS CUR 2.0 handoff output can be generated."),
		Handoff:       &output,
		Warnings:      output.Warnings,
	}
}

func selectedExportRef(options workflow.ExecutionOptions) string {
	if options.Selectors == nil || options.Selectors.AWS == nil {
		return ""
	}
	return strings.TrimSpace(options.Selectors.AWS.CUR2ExportRef)
}

func preflightCodeAllowedForPackage(code string) bool {
	switch strings.TrimSpace(code) {
	case "aws_cur2_preflight_ready",
		"aws_cur2_time_granularity_not_preferred",
		"aws_cur2_time_granularity_unverified",
		"aws_s3_delivery_policy_missing",
		"aws_s3_bucket_policy_inaccessible":
		return true
	default:
		return false
	}
}

func preflightReadinessBlocked(checks []workflow.PlanCheck) bool {
	if len(checks) == 0 {
		return true
	}
	for _, check := range checks {
		switch check.Status {
		case workflow.CheckFail, workflow.CheckUnknown, workflow.CheckSkipped:
			return true
		}
		code := evidenceValue(check.Evidence, "code")
		if code == "aws_cur2_delivery_not_started" || code == "aws_backfill_manual_step_required" {
			return true
		}
		for _, item := range check.Evidence {
			if item.Key == "missing_previous_month_component" {
				return true
			}
		}
	}
	return false
}

func hasUnclassifiedWarning(checks []workflow.PlanCheck) bool {
	for _, check := range checks {
		if check.Status != workflow.CheckWarn {
			continue
		}
		code := evidenceValue(check.Evidence, "code")
		if !warningCodeClassified(code) {
			return true
		}
	}
	return false
}

func warningCodeClassified(code string) bool {
	return handoffWarningCodeAllowed(code) || preflightOnlyWarningCodeIgnored(code)
}

func preflightOnlyWarningCodeIgnored(code string) bool {
	switch strings.TrimSpace(code) {
	case "aws_cur2_table_configuration_defaulted",
		"aws_cur2_include_resources_enabled",
		"aws_cur2_include_resources_not_required":
		return true
	default:
		return false
	}
}

type handoffEvidence struct {
	curVersion            string
	outputFormat          string
	compression           string
	timeGranularity       string
	overwrite             string
	s3Bucket              string
	s3Prefix              string
	s3Region              string
	previousBillingPeriod string
	dataPrefix            string
	manifestPrefix        string
}

func safeHandoffEvidence(exportRef string, checks []workflow.PlanCheck) (handoffEvidence, bool) {
	values := evidenceValues(checks)
	if strings.TrimSpace(values["selected_export_ref"]) != exportRef {
		return handoffEvidence{}, false
	}

	evidence := handoffEvidence{
		curVersion:            safeSimpleEvidence(values["cur_version"]),
		outputFormat:          safeSimpleEvidence(values["output_format"]),
		compression:           safeSimpleEvidence(values["compression"]),
		timeGranularity:       safeSimpleEvidence(values["time_granularity"]),
		overwrite:             safeSimpleEvidence(values["overwrite"]),
		s3Bucket:              s3handoff.Bucket(values["s3_bucket"]),
		s3Prefix:              s3handoff.ConfiguredPrefix(values["s3_prefix"]),
		s3Region:              s3handoff.Region(values["s3_region"]),
		previousBillingPeriod: safePreviousBillingPeriod(values["previous_billing_period"]),
		dataPrefix:            s3handoff.ReportPrefix(values["cur2_data_prefix"]),
		manifestPrefix:        s3handoff.ReportPrefix(values["cur2_manifest_prefix"]),
	}
	return evidence, evidence.complete()
}

func (evidence handoffEvidence) complete() bool {
	return evidence.curVersion != "" &&
		evidence.outputFormat != "" &&
		evidence.compression != "" &&
		evidence.timeGranularity != "" &&
		evidence.overwrite != "" &&
		evidence.s3Bucket != "" &&
		evidence.s3Prefix != "" &&
		evidence.s3Region != "" &&
		evidence.previousBillingPeriod != "" &&
		evidence.dataPrefix != "" &&
		evidence.manifestPrefix != ""
}

func evidenceValues(checks []workflow.PlanCheck) map[string]string {
	values := map[string]string{}
	for _, check := range checks {
		for _, item := range check.Evidence {
			if strings.TrimSpace(item.Key) == "" || strings.TrimSpace(item.Value) == "" {
				continue
			}
			if _, exists := values[item.Key]; !exists {
				values[item.Key] = strings.TrimSpace(item.Value)
			}
		}
	}
	return values
}

func evidenceValue(evidence []workflow.PlanEvidence, key string) string {
	for _, item := range evidence {
		if item.Key == key {
			return strings.TrimSpace(item.Value)
		}
	}
	return ""
}

func safeSimpleEvidence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '.' {
			return ""
		}
	}
	return value
}

func safePreviousBillingPeriod(value string) string {
	value = strings.TrimSpace(value)
	if !previousBillingPeriodPattern.MatchString(value) {
		return ""
	}
	return value
}

func deliveryPolicyReadiness(preflight workflow.CapabilityReport) string {
	for _, code := range handoffWarningCodes(preflight) {
		if isS3DeliveryPolicyWarning(code) {
			return "future_delivery_not_proven"
		}
	}
	return "ready"
}

func handoffWarnings(preflight workflow.CapabilityReport) []handoff.Warning {
	codes := handoffWarningCodes(preflight)
	if len(codes) == 0 {
		return nil
	}
	warnings := make([]handoff.Warning, 0, len(codes))
	for _, code := range codes {
		switch code {
		case "aws_cur2_time_granularity_not_preferred":
			warnings = append(warnings, handoff.Warning{
				Code:    code,
				Message: "The selected CUR 2.0 export is valid, but monthly granularity is preferred for Rapid Assessment - Billing Based.",
			})
		case "aws_cur2_time_granularity_unverified":
			warnings = append(warnings, handoff.Warning{
				Code:    code,
				Message: "The selected CUR 2.0 export is valid, but time granularity could not be confirmed from AWS table metadata.",
			})
		case "aws_s3_delivery_policy_missing", "aws_s3_bucket_policy_inaccessible":
			warnings = append(warnings, handoff.Warning{
				Code:    code,
				Message: "Previous-month billing data is present, but future AWS Data Exports delivery readiness is not proven.",
			})
		}
	}
	return warnings
}

func handoffWarningCodes(preflight workflow.CapabilityReport) []string {
	var codes []string
	seen := map[string]bool{}
	add := func(code string) {
		code = strings.TrimSpace(code)
		if !handoffWarningCodeAllowed(code) || seen[code] {
			return
		}
		seen[code] = true
		codes = append(codes, code)
	}

	add(preflight.Code)
	if preflight.PlanInput == nil {
		return codes
	}
	for _, check := range preflight.PlanInput.Checks {
		if check.Status != workflow.CheckWarn {
			continue
		}
		add(evidenceValue(check.Evidence, "code"))
	}
	return codes
}

func handoffWarningCodeAllowed(code string) bool {
	switch code {
	case "aws_cur2_time_granularity_not_preferred",
		"aws_cur2_time_granularity_unverified",
		"aws_s3_delivery_policy_missing",
		"aws_s3_bucket_policy_inaccessible":
		return true
	default:
		return false
	}
}

func isS3DeliveryPolicyWarning(code string) bool {
	return code == "aws_s3_delivery_policy_missing" || code == "aws_s3_bucket_policy_inaccessible"
}

func (runner Runner) blockedReport(request workflow.Request, options workflow.ExecutionOptions, code string, message string) workflow.CapabilityReport {
	return workflow.CapabilityReport{
		Status:        workflow.RunStatusBlocked,
		SupportStatus: workflow.SupportBlocked,
		Code:          code,
		Message:       message,
		Mutated:       false,
		SourceHandles: packageSourceHandles(),
		PlanInput:     runner.planInput(request, options, workflow.CheckFail, code, message),
	}
}

func (runner Runner) planInput(request workflow.Request, options workflow.ExecutionOptions, checkStatus workflow.CheckStatus, code string, message string) *workflow.ExecutionPlanInput {
	handles := packageSourceHandles()
	stepIntent := workflow.PlanStepBlocked
	stepTitle := "AWS CUR 2.0 handoff blocked"
	stepDescription := "AWS CUR 2.0 billing handoff output cannot be generated yet."
	currentState := "Safe handoff output is unavailable."
	targetState := "Safe handoff output is generated from a selected, ready AWS CUR 2.0 export."
	if checkStatus == workflow.CheckPass {
		stepIntent = workflow.PlanStepSkip
		stepTitle = "AWS CUR 2.0 handoff output ready"
		stepDescription = "Safe AWS CUR 2.0 billing handoff output can be shown on stdout."
		currentState = "Selected AWS CUR 2.0 export passed read-only preflight for handoff output."
		targetState = "Use the safe handoff output values in Matilda SaaS."
	}
	return &workflow.ExecutionPlanInput{
		Request:          request,
		ExecutionOptions: options,
		OperatorIdentitySummary: workflow.OperatorIdentitySummary{
			IdentityStatus: "not_required",
			Summary:        "AWS operator identity is verified by the read-only CUR 2.0 preflight that this package action reruns.",
			SourceHandles:  handles,
		},
		CoverageRecommendation: workflow.CoverageRecommendation{
			CoverageStatus: workflow.CoverageUnknown,
			Summary:        "AWS billing coverage is determined by the selected CUR 2.0 export and its S3 delivery scope.",
		},
		PackageSchemaStatus: workflow.PackageSchemaStructuredStdoutHandoff,
		Steps: []workflow.PlanStep{{
			Intent:                    stepIntent,
			Title:                     stepTitle,
			Description:               stepDescription,
			Reason:                    "Matilda Rapid Assessment - Billing Based needs the selected AWS CUR 2.0 billing source location and readiness evidence.",
			ApprovalKind:              "not_required",
			CurrentState:              currentState,
			TargetState:               targetState,
			RequiredPermission:        "Read-only AWS CUR 2.0 preflight permissions for Data Exports and S3 evidence inspection.",
			CredentialMaterialTouched: false,
			Validation:                "The package action reruns preflight, scans all checks, and emits only whitelisted handoff fields.",
			Rollback:                  "No cloud or local file change is made by this action.",
			SourceHandles:             handles,
		}},
		Checks: []workflow.PlanCheck{{
			Status:  checkStatus,
			Title:   "AWS CUR 2.0 handoff readiness",
			Message: message,
			Evidence: []workflow.PlanEvidence{
				{Key: "code", Value: code},
				{Key: "mutated", Value: "false"},
			},
			SourceHandles: handles,
		}},
		SourceHandles: handles,
	}
}

func packageSourceHandles() []workflow.SourceHandle {
	return []workflow.SourceHandle{
		{
			Label: "AWS Rapid Assessment Billing Handoff Schema",
			URI:   "docs/references/aws/aws-rapid-assessment-billing-handoff-schema.md",
		},
		{
			Label: "AWS CUR 2.0 Preflight Source Bundle",
			URI:   "docs/references/aws/aws-rapid-assessment-billing-cur2-preflight-source-bundle.md",
		},
	}
}
