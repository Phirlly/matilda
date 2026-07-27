package cur2preflight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	maxDataExportsListPages = 100
	maxExportDetailChecks   = 100
	maxListObjectPages      = 5
)

type RunnerConfig struct {
	Client Client
	Now    time.Time
}

type Runner struct {
	client Client
	now    time.Time
}

func NewRunner(config RunnerConfig) Runner {
	now := config.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Runner{
		client: config.Client,
		now:    now.UTC(),
	}
}

func (runner Runner) Run(ctx context.Context, request workflow.Request) workflow.CapabilityReport {
	state := newRunState(request)
	if isNilClient(runner.client) {
		state.add(failFinding("aws_provider_capability_blocked", "AWS preflight client", "AWS CUR 2.0 preflight client is not configured."))
		return state.report("unknown", "AWS caller identity was not checked.")
	}

	config, err := runner.client.CheckConfiguration(ctx)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_config_missing_credentials"), "AWS SDK configuration", configurationFailureMessage(err)))
		return state.report("unknown", "AWS caller identity was not checked because configuration failed.")
	}
	if strings.TrimSpace(config.Region) == "" {
		state.add(failFinding("aws_config_missing_region", "AWS SDK configuration", "AWS Region is not configured."))
		return state.report("unknown", "AWS caller identity was not checked because configuration failed.")
	}
	state.add(withEvidence(passFinding("aws_config_ready", "AWS SDK configuration", "AWS Region and credential provider configuration are available."), workflow.PlanEvidence{Key: "region_configured", Value: "true"}))

	identity, err := runner.client.GetCallerIdentity(ctx)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_auth_failed"), "AWS caller identity", "AWS caller identity could not be verified."))
		return state.report("unavailable", "AWS caller identity could not be verified.")
	}
	if strings.TrimSpace(identity.AccountID) == "" || strings.TrimSpace(identity.CallerARN) == "" {
		state.add(failFinding("aws_identity_unavailable", "AWS caller identity", "AWS caller identity response did not include required safe evidence."))
		return state.report("unavailable", "AWS caller identity response was incomplete.")
	}
	accountEvidence := maskedAccount(identity.AccountID)
	callerEvidence := hashedRef(identity.CallerARN)
	state.add(withEvidence(passFinding("aws_identity_verified", "AWS caller identity", "AWS caller identity was verified without exposing raw ARN values."),
		workflow.PlanEvidence{Key: "caller_account", Value: accountEvidence},
		workflow.PlanEvidence{Key: "caller_ref", Value: callerEvidence},
	))

	tables, err := listAllTables(ctx, runner.client)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_data_exports_access_denied"), "AWS Data Exports table metadata", "AWS Data Exports table metadata could not be listed."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	if !hasTable(tables, cur2TableName) {
		state.add(failFinding("aws_cur2_table_unavailable", "CUR 2.0 table availability", "COST_AND_USAGE_REPORT table metadata is not available."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}

	summaries, err := listAllExports(ctx, runner.client)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_data_exports_access_denied"), "AWS Data Exports discovery", "AWS Data Exports definitions could not be listed."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	exports, err := runner.inspectListedExports(ctx, summaries)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_cur2_export_invalid_shape"), "AWS CUR 2.0 export definition", "Listed AWS exports could not be inspected."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	export, finding := selectCUR2Export(exports)
	if finding.Code != "" {
		state.add(finding)
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	state.add(withEvidence(passFinding("aws_cur2_export_selected", "AWS CUR 2.0 export discovery", "Exactly one CUR 2.0 export candidate was selected for read-only inspection."),
		workflow.PlanEvidence{Key: "cur_version", Value: "CUR2.0"},
	))

	if export.SourceARN == "" {
		export.SourceARN = export.ExportARN
	}
	if export.SourceAccount == "" {
		export.SourceAccount = identity.AccountID
	}

	tableConfig := export.TableConfigurations[cur2TableName]
	if tableConfig == nil {
		state.add(failFinding("aws_cur2_export_invalid_shape", "CUR 2.0 table configuration", "AWS export is missing COST_AND_USAGE_REPORT table configuration."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	table, err := runner.client.GetTable(ctx, cur2TableName, tableConfig)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_cur2_table_unavailable"), "CUR 2.0 table availability", "COST_AND_USAGE_REPORT table metadata could not be inspected."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	tableFinding := validateTableColumns(table.Columns)
	state.add(tableFinding)
	if tableFinding.Status == workflow.CheckFail {
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}

	queryFinding := validateCUR2Query(export.QueryStatement, table.Columns)
	state.add(queryFinding)
	if queryFinding.Status == workflow.CheckFail {
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	for _, finding := range validateExportShape(export) {
		state.add(finding)
		if finding.Status == workflow.CheckFail {
			return state.report("verified", identitySummary(accountEvidence, callerEvidence))
		}
	}

	bucketAccess, err := runner.client.HeadBucket(ctx, export.Destination.Bucket)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_s3_bucket_inaccessible"), "AWS S3 bucket reachability", "S3 destination bucket could not be checked."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	if !bucketAccess.Accessible {
		state.add(withEvidence(failFinding("aws_s3_bucket_inaccessible", "AWS S3 bucket reachability", "S3 destination bucket is inaccessible or permissions are insufficient."),
			workflow.PlanEvidence{Key: "head_bucket_status", Value: safeStatusCode(bucketAccess.StatusCode)},
		))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	if bucketAccess.Region != "" && bucketAccess.Region != export.Destination.Region {
		state.add(failFinding("aws_s3_bucket_inaccessible", "AWS S3 bucket region", "S3 destination bucket region does not match the AWS export destination region."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	state.add(passFinding("aws_s3_bucket_reachable", "AWS S3 bucket reachability", "S3 destination bucket is reachable through read-only preflight."))

	policy, err := runner.client.GetBucketPolicy(ctx, export.Destination.Bucket)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_s3_bucket_policy_inaccessible"), "AWS S3 bucket policy visibility", "S3 bucket policy could not be inspected."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	policyFinding := validateBucketPolicy(policy, export.SourceAccount, export.SourceARN, export.Destination)
	state.add(policyFinding)
	if policyFinding.Status == workflow.CheckFail {
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}

	for _, finding := range runner.deliveryFindings(ctx, export) {
		state.add(finding)
		if finding.Status == workflow.CheckFail {
			return state.report("verified", identitySummary(accountEvidence, callerEvidence))
		}
	}

	period := previousBillingPeriod(runner.now)
	partitionFinding := runner.previousMonthFinding(ctx, export, period)
	state.add(partitionFinding)

	return state.report("verified", identitySummary(accountEvidence, callerEvidence))
}

func listAllTables(ctx context.Context, client Client) ([]TableSummary, error) {
	tables := []TableSummary{}
	token := ""
	seenTokens := map[string]struct{}{}
	for pageNumber := 0; pageNumber < maxDataExportsListPages; pageNumber++ {
		page, err := client.ListTables(ctx, token)
		if err != nil {
			return nil, err
		}
		tables = append(tables, page.Tables...)
		if page.NextToken == "" {
			return tables, nil
		}
		if tokenWasSeen(seenTokens, page.NextToken) {
			return nil, dataExportsPaginationError()
		}
		token = page.NextToken
	}
	return nil, dataExportsPaginationError()
}

func listAllExports(ctx context.Context, client Client) ([]ExportSummary, error) {
	exports := []ExportSummary{}
	token := ""
	seenTokens := map[string]struct{}{}
	for pageNumber := 0; pageNumber < maxDataExportsListPages; pageNumber++ {
		page, err := client.ListExports(ctx, token)
		if err != nil {
			return nil, err
		}
		exports = append(exports, page.Exports...)
		if page.NextToken == "" {
			return exports, nil
		}
		if tokenWasSeen(seenTokens, page.NextToken) {
			return nil, dataExportsPaginationError()
		}
		token = page.NextToken
	}
	return nil, dataExportsPaginationError()
}

func (runner Runner) inspectListedExports(ctx context.Context, summaries []ExportSummary) ([]Export, error) {
	if len(summaries) > maxExportDetailChecks {
		return nil, dataExportsPaginationError()
	}

	exports := make([]Export, 0, len(summaries))
	for _, summary := range summaries {
		exportARN := strings.TrimSpace(summary.ExportARN)
		if exportARN == "" {
			exports = append(exports, Export{Name: summary.Name})
			continue
		}
		export, err := runner.client.GetExport(ctx, exportARN)
		if err != nil {
			return nil, err
		}
		if export.Name == "" {
			export.Name = summary.Name
		}
		if export.ExportARN == "" {
			export.ExportARN = exportARN
		}
		if export.SourceARN == "" {
			export.SourceARN = export.ExportARN
		}
		exports = append(exports, export)
	}
	return exports, nil
}

func selectCUR2Export(exports []Export) (Export, checkFinding) {
	if len(exports) == 0 {
		return Export{}, failFinding("aws_cur2_export_not_found", "AWS CUR 2.0 export discovery", "No AWS Data Exports definitions were found.")
	}

	candidates := []Export{}
	outOfScope := false
	for _, export := range exports {
		switch {
		case hasCUR2TableConfiguration(export) || referencesCUR2QuerySource(export.QueryStatement):
			candidates = append(candidates, export)
		case export.QueryStatement != "" || len(export.TableConfigurations) > 0:
			outOfScope = true
		}
	}
	if len(candidates) == 0 && outOfScope {
		return Export{}, failFinding("aws_non_cur2_source_out_of_scope", "AWS billing export source", "Only non-CUR-2.0 billing exports were found for this path.")
	}
	if len(candidates) == 0 {
		return Export{}, failFinding("aws_cur2_export_not_found", "AWS CUR 2.0 export discovery", "No CUR 2.0 export candidate was found.")
	}
	if len(candidates) > 1 {
		return Export{}, failFinding("aws_cur2_export_ambiguous", "AWS CUR 2.0 export discovery", "Multiple CUR 2.0 export candidates were found.")
	}
	return candidates[0], checkFinding{}
}

func hasCUR2TableConfiguration(export Export) bool {
	if export.TableConfigurations == nil {
		return false
	}
	_, ok := export.TableConfigurations[cur2TableName]
	return ok
}

func hasTable(tables []TableSummary, name string) bool {
	for _, table := range tables {
		if table.Name == name {
			return true
		}
	}
	return false
}

func (runner Runner) deliveryFindings(ctx context.Context, export Export) []checkFinding {
	executions, err := listAllExecutions(ctx, runner.client, export.ExportARN)
	if err != nil {
		return []checkFinding{failFinding(providerErrorCode(err, "aws_data_exports_access_denied"), "AWS export delivery status", "AWS Data Exports delivery executions could not be inspected.")}
	}
	if len(executions) == 0 {
		if !export.CreatedAt.IsZero() && runner.now.Sub(export.CreatedAt) >= 24*time.Hour {
			return []checkFinding{failFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "No AWS Data Exports delivery execution has started after the expected delivery delay.")}
		}
		return []checkFinding{warnFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "No AWS Data Exports delivery execution has started yet.", true)}
	}

	inspectedExecutions := make([]Execution, 0, len(executions))
	for _, execution := range executions {
		if execution.ID != "" {
			inspected, err := runner.client.GetExecution(ctx, export.ExportARN, execution.ID)
			if err != nil {
				return []checkFinding{failFinding(providerErrorCode(err, "aws_data_exports_access_denied"), "AWS export delivery status", "AWS Data Exports delivery execution could not be inspected.")}
			}
			execution = inspected
		}
		inspectedExecutions = append(inspectedExecutions, execution)
	}

	latest := latestExecution(inspectedExecutions)
	switch strings.ToUpper(strings.TrimSpace(latest.Status)) {
	case "DELIVERY_SUCCESS", "SUCCEEDED", "SUCCESS", "DELIVERED", "COMPLETED":
		return []checkFinding{passFinding("aws_cur2_delivery_ready", "AWS export delivery status", "Latest AWS Data Exports delivery status is healthy.")}
	case "INITIATION_IN_PROCESS", "QUERY_QUEUED", "QUERY_IN_PROCESS", "DELIVERY_IN_PROCESS", "IN_PROGRESS", "RUNNING", "STARTED":
		return []checkFinding{warnFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "Latest AWS Data Exports delivery is still in progress.", true)}
	case "QUERY_FAILURE", "DELIVERY_FAILURE", "FAILED", "FAILURE", "UNHEALTHY":
		return []checkFinding{failFinding("aws_cur2_export_invalid_shape", "AWS export delivery status", "Latest AWS Data Exports delivery reports a failure.")}
	}
	return []checkFinding{warnFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "AWS Data Exports delivery status is not conclusive yet.", true)}
}

func latestExecution(executions []Execution) Execution {
	latest := executions[0]
	for _, execution := range executions[1:] {
		if execution.StatusObservedAt.After(latest.StatusObservedAt) {
			latest = execution
		}
	}
	return latest
}

func listAllExecutions(ctx context.Context, client Client, exportARN string) ([]Execution, error) {
	executions := []Execution{}
	token := ""
	seenTokens := map[string]struct{}{}
	for pageNumber := 0; pageNumber < maxDataExportsListPages; pageNumber++ {
		page, err := client.ListExecutions(ctx, exportARN, token)
		if err != nil {
			return nil, err
		}
		executions = append(executions, page.Executions...)
		if page.NextToken == "" {
			return executions, nil
		}
		if tokenWasSeen(seenTokens, page.NextToken) {
			return nil, dataExportsPaginationError()
		}
		token = page.NextToken
	}
	return nil, dataExportsPaginationError()
}

func tokenWasSeen(seen map[string]struct{}, token string) bool {
	if _, exists := seen[token]; exists {
		return true
	}
	seen[token] = struct{}{}
	return false
}

func dataExportsPaginationError() error {
	return NewProviderError("aws_data_exports_pagination_unbounded", "AWS Data Exports pagination did not converge within the bounded preflight inspection window.")
}

func (runner Runner) previousMonthFinding(ctx context.Context, export Export, period string) checkFinding {
	token := ""
	for pageNumber := 0; pageNumber < maxListObjectPages; pageNumber++ {
		page, err := runner.client.ListObjects(ctx, export.Destination.Bucket, export.Destination.Prefix, token, 1000)
		if err != nil {
			return failFinding(providerErrorCode(err, "aws_s3_bucket_inaccessible"), "AWS previous-month billing partition", "S3 previous-month billing partition could not be listed.")
		}
		for _, key := range page.Keys {
			if matchesPreviousMonthDataKey(key, export, period) {
				return withEvidence(passFinding("aws_cur2_previous_month_ready", "AWS previous-month billing partition", "Previous-month CUR 2.0 billing partition is present."),
					workflow.PlanEvidence{Key: "previous_billing_period", Value: period},
				)
			}
		}
		if page.NextToken == "" {
			token = ""
			break
		}
		token = page.NextToken
	}
	if token != "" {
		return failFinding("aws_cur2_previous_month_missing", "AWS previous-month billing partition", "Bounded S3 pagination ended before previous-month partition availability could be proven.")
	}

	return withEvidence(manualFinding("aws_backfill_manual_step_required", "AWS previous-month billing partition", "Previous-month CUR 2.0 billing partition is missing and may require AWS support backfill."),
		workflow.PlanEvidence{Key: "previous_billing_period", Value: period},
	)
}

func matchesPreviousMonthDataKey(key string, export Export, period string) bool {
	prefix := strings.Trim(strings.TrimSpace(export.Destination.Prefix), "/")
	name := strings.Trim(strings.TrimSpace(export.Name), "/")
	if prefix == "" || name == "" {
		return false
	}
	expected := prefix + "/" + name + "/data/BILLING_PERIOD=" + period + "/"
	return strings.HasPrefix(key, expected)
}

func providerErrorCode(err error, fallback string) string {
	var providerErr ProviderError
	if errors.As(err, &providerErr) && providerErr.Code != "" {
		return providerErr.Code
	}
	return fallback
}

func configurationFailureMessage(err error) string {
	switch providerErrorCode(err, "aws_config_missing_credentials") {
	case "aws_config_missing_region":
		return "AWS Region is not configured."
	case "aws_config_missing_credentials":
		return "AWS credentials are not available."
	default:
		return "AWS SDK configuration could not be loaded."
	}
}

func previousBillingPeriod(now time.Time) string {
	year, month, _ := now.UTC().Date()
	previous := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	return previous.Format("2006-01")
}

func maskedAccount(accountID string) string {
	trimmed := strings.TrimSpace(accountID)
	if len(trimmed) < 4 {
		return "account-ending-unknown"
	}
	return "account-ending-" + trimmed[len(trimmed)-4:]
}

func hashedRef(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}

func identitySummary(accountEvidence string, callerEvidence string) string {
	return fmt.Sprintf("AWS caller identity was verified with %s and caller hash %s.", accountEvidence, callerEvidence)
}

func safeStatusCode(statusCode int) string {
	if statusCode == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%d", statusCode)
}

func sourceHandles() []workflow.SourceHandle {
	return []workflow.SourceHandle{
		{
			Label: "AWS CUR 2.0 Preflight Source Bundle",
			URI:   "docs/references/aws/aws-rapid-assessment-billing-cur2-preflight-source-bundle.md",
		},
		{
			Label: "AWS Official Implementation References",
			URI:   "docs/references/aws/official-implementation-references.md",
		},
		{
			Label: "AWS SDK Go v2 Read-Only Adapter References",
			URI:   "docs/references/aws/aws-sdk-go-v2-readonly-adapter.md",
		},
	}
}

func isNilClient(client Client) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

type runState struct {
	request  workflow.Request
	handles  []workflow.SourceHandle
	findings []checkFinding
}

func newRunState(request workflow.Request) *runState {
	return &runState{
		request: request,
		handles: sourceHandles(),
	}
}

func (state *runState) add(findings ...checkFinding) {
	for _, finding := range findings {
		if finding.Code == "" {
			continue
		}
		state.findings = append(state.findings, finding)
	}
}

func (state *runState) report(identityStatus string, identitySummary string) workflow.CapabilityReport {
	status, code := state.statusAndCode()
	return workflow.CapabilityReport{
		Status:        status,
		SupportStatus: supportStatusFor(status),
		Code:          code,
		Message:       messageFor(code),
		Mutated:       false,
		SourceHandles: state.handles,
		PlanInput: &workflow.ExecutionPlanInput{
			Request:                 state.request,
			OperatorIdentitySummary: state.operatorIdentity(identityStatus, identitySummary),
			CoverageRecommendation: workflow.CoverageRecommendation{
				CoverageStatus: workflow.CoverageUnknown,
				Summary:        "AWS billing coverage is evaluated from the selected CUR 2.0 export and account context.",
			},
			PackageSchemaStatus: workflow.PackageSchemaProviderSchemaRequired,
			Steps:               state.steps(status),
			Checks:              state.checks(),
			SourceHandles:       state.handles,
		},
	}
}

func (state *runState) statusAndCode() (workflow.RunStatus, string) {
	if len(state.findings) == 0 {
		return workflow.RunStatusBlocked, "aws_provider_capability_blocked"
	}
	for _, finding := range state.findings {
		if finding.Status == workflow.CheckFail {
			return workflow.RunStatusBlocked, finding.Code
		}
	}
	for _, finding := range state.findings {
		if finding.ManualAction {
			return workflow.RunStatusManualSteps, finding.Code
		}
	}
	for _, finding := range state.findings {
		if finding.TopLevel && finding.Status == workflow.CheckWarn {
			return workflow.StatusReady, finding.Code
		}
	}
	return workflow.StatusReady, "aws_cur2_preflight_ready"
}

func supportStatusFor(status workflow.RunStatus) workflow.SupportStatus {
	if status == workflow.RunStatusBlocked {
		return workflow.SupportBlocked
	}
	return workflow.SupportSupported
}

func messageFor(code string) string {
	switch code {
	case "aws_cur2_preflight_ready":
		return "AWS CUR 2.0 billing preflight is ready."
	case "aws_cur2_delivery_not_started":
		return "AWS CUR 2.0 billing preflight is ready, but export delivery is not complete yet."
	case "aws_backfill_manual_step_required":
		return "AWS CUR 2.0 billing preflight requires previous-month billing backfill or manual remediation."
	default:
		return "AWS CUR 2.0 billing preflight found a blocker."
	}
}

func (state *runState) operatorIdentity(status string, summary string) workflow.OperatorIdentitySummary {
	if strings.TrimSpace(status) == "" {
		status = "unknown"
	}
	if strings.TrimSpace(summary) == "" {
		summary = "AWS caller identity was not checked."
	}
	return workflow.OperatorIdentitySummary{
		IdentityStatus: status,
		Summary:        summary,
		SourceHandles:  state.handles,
	}
}

func (state *runState) steps(status workflow.RunStatus) []workflow.PlanStep {
	intent := workflow.PlanStepReuse
	title := "Review existing AWS CUR 2.0 export"
	current := "Existing AWS CUR 2.0 export metadata is visible through read-only checks."
	target := "Existing AWS CUR 2.0 export satisfies the billing preflight rules."

	if status == workflow.RunStatusBlocked {
		intent = workflow.PlanStepBlocked
		title = "Resolve AWS CUR 2.0 preflight blocker"
		current = "AWS CUR 2.0 billing preflight has a blocking readiness issue."
		target = "Blocking AWS CUR 2.0 readiness issues are resolved before Matilda onboarding."
	}
	if status == workflow.RunStatusManualSteps {
		intent = workflow.PlanStepGuide
		title = "Complete AWS billing backfill remediation"
		current = "Previous-month CUR 2.0 billing partition is not visible to preflight."
		target = "Previous-month CUR 2.0 billing data is available for Matilda Rapid Assessment."
	}

	return []workflow.PlanStep{{
		Intent:                    intent,
		Title:                     title,
		Description:               "Use read-only AWS checks to evaluate an existing CUR 2.0 Data Export for Matilda Rapid Assessment - Billing Based.",
		Reason:                    "Matilda Rapid Assessment - Billing Based requires exported AWS billing data for the assessment period.",
		ApprovalKind:              "not_required",
		CurrentState:              current,
		TargetState:               target,
		RequiredPermission:        "bcm-data-exports:ListExports, bcm-data-exports:GetExport, bcm-data-exports:ListTables, bcm-data-exports:GetTable, bcm-data-exports:ListExecutions, bcm-data-exports:GetExecution, s3:ListBucket, s3:GetBucketPolicy",
		CredentialMaterialTouched: false,
		Validation:                "Read-only AWS CUR 2.0 preflight checks produce pass, warn, or fail signals without reading billing object contents.",
		Rollback:                  "No cloud change is made by preflight.",
		SourceHandles:             state.handles,
	}}
}

func (state *runState) checks() []workflow.PlanCheck {
	checks := make([]workflow.PlanCheck, 0, len(state.findings))
	for _, finding := range state.findings {
		evidence := finding.Evidence
		if len(evidence) == 0 {
			evidence = []workflow.PlanEvidence{{Key: "code", Value: finding.Code}}
		}
		checks = append(checks, workflow.PlanCheck{
			Status:        finding.Status,
			Title:         finding.Title,
			Message:       finding.Message,
			Evidence:      evidence,
			SourceHandles: state.handles,
		})
	}
	if len(checks) == 0 {
		checks = append(checks, workflow.PlanCheck{
			Status:        workflow.CheckFail,
			Title:         "AWS CUR 2.0 preflight",
			Message:       "AWS CUR 2.0 preflight did not produce checks.",
			Evidence:      []workflow.PlanEvidence{{Key: "code", Value: "aws_provider_capability_blocked"}},
			SourceHandles: state.handles,
		})
	}
	return checks
}

var _ workflow.CapabilityRunner = Runner{}

func AWSBillingPreflightRequest() workflow.Request {
	return workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: assessment.CollectionBilling,
		Provider:       assessment.ProviderAWS,
		Action:         assessment.ActionPreflight,
	}
}
