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
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/s3handoff"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	maxDataExportsListPages = 100
	maxExportDetailChecks   = 100
	maxListObjectPages      = 5
)

type RunnerConfig struct {
	Client        Client
	ClientFactory func(workflow.ExecutionOptions) Client
	Now           time.Time
}

type Runner struct {
	client        Client
	clientFactory func(workflow.ExecutionOptions) Client
	now           time.Time
}

func NewRunner(config RunnerConfig) Runner {
	now := config.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Runner{
		client:        config.Client,
		clientFactory: config.ClientFactory,
		now:           now.UTC(),
	}
}

func (runner Runner) Run(ctx context.Context, request workflow.Request, options workflow.ExecutionOptions) workflow.CapabilityReport {
	state := newRunState(request)
	client := runner.clientFor(options)
	if isNilClient(client) {
		state.add(failFinding("aws_provider_capability_blocked", "AWS preflight client", "AWS CUR 2.0 preflight client is not configured."))
		return state.report("unknown", "AWS caller identity was not checked.")
	}

	config, err := client.CheckConfiguration(ctx)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_config_missing_credentials"), "AWS SDK configuration", configurationFailureMessage(err)))
		return state.report("unknown", "AWS caller identity was not checked because configuration failed.")
	}
	if strings.TrimSpace(config.Region) == "" {
		state.add(failFinding("aws_config_missing_region", "AWS SDK configuration", "AWS Region is not configured."))
		return state.report("unknown", "AWS caller identity was not checked because configuration failed.")
	}
	state.add(withEvidence(passFinding("aws_config_ready", "AWS SDK configuration", "AWS Region and credential provider configuration are available."), workflow.PlanEvidence{Key: "region_configured", Value: "true"}))

	identity, err := client.GetCallerIdentity(ctx)
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

	tables, err := listAllTables(ctx, client)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_data_exports_access_denied"), "AWS Data Exports table metadata", "AWS Data Exports table metadata could not be listed."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	if !hasTable(tables, cur2TableName) {
		state.add(failFinding("aws_cur2_table_unavailable", "CUR 2.0 table availability", "COST_AND_USAGE_REPORT table metadata is not available."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}

	summaries, err := listAllExports(ctx, client)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_data_exports_access_denied"), "AWS Data Exports discovery", "AWS Data Exports definitions could not be listed."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	exports, err := runner.inspectListedExports(ctx, client, summaries)
	if err != nil {
		state.add(failFinding(providerErrorCode(err, "aws_cur2_export_invalid_shape"), "AWS CUR 2.0 export definition", "Listed AWS exports could not be inspected."))
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	requestedExportRef := awsCUR2ExportRefOption(options)
	export, selectedExportRef, finding := selectCUR2Export(exports, requestedExportRef, guidedSelectionRequired(options, requestedExportRef))
	if finding.Code != "" {
		state.add(finding)
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}
	state.add(withEvidence(passFinding("aws_cur2_export_selected", "AWS CUR 2.0 export discovery", "Exactly one CUR 2.0 export candidate was selected for read-only inspection."),
		workflow.PlanEvidence{Key: "cur_version", Value: "CUR2.0"},
		workflow.PlanEvidence{Key: "selected_export_ref", Value: selectedExportRef},
	))

	if export.SourceARN == "" {
		export.SourceARN = export.ExportARN
	}
	if export.SourceAccount == "" {
		export.SourceAccount = identity.AccountID
	}

	tableConfig := cur2TableConfiguration(export)
	table, err := client.GetTable(ctx, cur2TableName, tableConfig)
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
	shapeFindings := validateExportShape(export, table.Properties)
	state.add(shapeFindings...)
	if hasFailedFinding(shapeFindings) {
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}

	bucketAccess, err := client.HeadBucket(ctx, export.Destination.Bucket)
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

	policyFinding := inspectBucketPolicy(ctx, client, export)

	for _, finding := range runner.deliveryFindings(ctx, client, export) {
		state.add(finding)
		if finding.Status == workflow.CheckFail {
			return state.report("verified", identitySummary(accountEvidence, callerEvidence))
		}
	}

	period := previousBillingPeriod(runner.now)
	partitionFinding := runner.previousMonthFinding(ctx, client, export, period)
	state.add(partitionFinding)
	if partitionFinding.Status == workflow.CheckFail {
		return state.report("verified", identitySummary(accountEvidence, callerEvidence))
	}

	state.add(policyFindingAfterCurrentDataProof(policyFinding, partitionFinding))

	return state.report("verified", identitySummary(accountEvidence, callerEvidence))
}

func (runner Runner) clientFor(options workflow.ExecutionOptions) Client {
	if !isNilClient(runner.client) {
		return runner.client
	}
	if runner.clientFactory == nil {
		return nil
	}
	return runner.clientFactory(options)
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

func (runner Runner) inspectListedExports(ctx context.Context, client Client, summaries []ExportSummary) ([]Export, error) {
	if len(summaries) > maxExportDetailChecks {
		return nil, dataExportsPaginationError()
	}

	exports := make([]Export, 0, len(summaries))
	for _, summary := range summaries {
		exportARN := strings.TrimSpace(summary.ExportARN)
		if exportARN == "" {
			return nil, NewProviderError("aws_data_exports_incomplete_export_summary", "AWS Data Exports returned an export summary without an export ARN.")
		}
		export, err := client.GetExport(ctx, exportARN)
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

func awsCUR2ExportRefOption(options workflow.ExecutionOptions) string {
	if options.Selectors == nil || options.Selectors.AWS == nil {
		return ""
	}
	return strings.TrimSpace(options.Selectors.AWS.CUR2ExportRef)
}

func guidedSelectionRequired(options workflow.ExecutionOptions, requestedRef string) bool {
	return options.InterfaceMode == workflow.InterfaceModeGuided && requestedRef == ""
}

func selectCUR2Export(exports []Export, requestedRef string, selectionRequired bool) (Export, string, checkFinding) {
	if len(exports) == 0 {
		return Export{}, "", failFinding("aws_cur2_export_not_found", "AWS CUR 2.0 export discovery", "No AWS Data Exports definitions were found.")
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
		return Export{}, "", failFinding("aws_non_cur2_source_out_of_scope", "AWS billing export source", "Only non-CUR-2.0 billing exports were found for this path.")
	}
	if len(candidates) == 0 {
		return Export{}, "", failFinding("aws_cur2_export_not_found", "AWS CUR 2.0 export discovery", "No CUR 2.0 export candidate was found.")
	}

	refs, err := cur2ExportRefs(candidates)
	if err != nil {
		return Export{}, "", failFinding("aws_cur2_export_ref_collision", "AWS CUR 2.0 export refs", "CUR 2.0 export candidates could not be assigned unique safe refs.")
	}

	if requestedRef != "" {
		for index, candidate := range candidates {
			if refs[index] == requestedRef {
				return candidate, refs[index], checkFinding{}
			}
		}
		return Export{}, "", withEvidence(failFinding("aws_cur2_export_ref_not_found", "AWS CUR 2.0 export selection", "Requested CUR 2.0 export ref did not match a discovered candidate."),
			candidateEvidence(candidates, refs)...,
		)
	}
	if len(candidates) > 1 {
		return Export{}, "", withEvidence(failFinding("aws_cur2_export_ambiguous", "AWS CUR 2.0 export discovery", "Multiple CUR 2.0 export candidates were found."),
			candidateEvidence(candidates, refs)...,
		)
	}
	if selectionRequired {
		return Export{}, "", withEvidence(failFinding("aws_cur2_export_selection_required", "AWS CUR 2.0 export selection", "One CUR 2.0 export candidate was found for guided selection."),
			candidateEvidence(candidates, refs)...,
		)
	}
	return candidates[0], refs[0], checkFinding{}
}

func cur2ExportRefs(exports []Export) ([]string, error) {
	for _, length := range []int{16, 24, 32} {
		refs := make([]string, len(exports))
		seen := map[string]struct{}{}
		collision := false
		for index, export := range exports {
			refs[index] = cur2ExportRefWithLength(export.ExportARN, length)
			if _, exists := seen[refs[index]]; exists {
				collision = true
				break
			}
			seen[refs[index]] = struct{}{}
		}
		if !collision {
			return refs, nil
		}
	}
	return nil, fmt.Errorf("CUR 2.0 export refs are not unique")
}

func cur2ExportRef(exportARN string) string {
	return cur2ExportRefWithLength(exportARN, 16)
}

func cur2ExportRefWithLength(exportARN string, length int) string {
	sum := sha256.Sum256([]byte("aws:bcm-data-exports:export-arn:" + exportARN))
	return "cur2-" + letterEncodeHash(sum[:], length)
}

func letterEncodeHash(hash []byte, length int) string {
	const alphabet = "abcdefghijklmnop"
	var builder strings.Builder
	builder.Grow(length)
	for _, value := range hash {
		if builder.Len() >= length {
			break
		}
		builder.WriteByte(alphabet[value>>4])
		if builder.Len() >= length {
			break
		}
		builder.WriteByte(alphabet[value&0x0f])
	}
	return builder.String()
}

func candidateEvidence(candidates []Export, refs []string) []workflow.PlanEvidence {
	evidence := make([]workflow.PlanEvidence, 0, len(candidates)*16)
	for index, candidate := range candidates {
		prefix := fmt.Sprintf("candidate_%d", index+1)
		facts := candidatePreSelectionFacts(candidate)
		evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_export_ref", Value: refs[index]})
		if health := safeEvidenceValue(candidate.HealthStatus); health != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_health", Value: health})
		}
		if output := safeEvidenceValue(candidate.Destination.Output.Format); output != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_output_format", Value: output})
		}
		if compression := safeEvidenceValue(candidate.Destination.Output.Compression); compression != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_compression", Value: compression})
		}
		if granularity := safeEvidenceValue(normalizedTableProperty(cur2TableConfiguration(candidate), "TIME_GRANULARITY")); granularity != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_time_granularity", Value: granularity})
		}
		if overwrite := safeEvidenceValue(candidate.Destination.Output.Overwrite); overwrite != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_overwrite", Value: overwrite})
		}
		if outputType := safeEvidenceValue(candidate.Destination.Output.OutputType); outputType != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_output_type", Value: outputType})
		}
		if refreshCadence := safeEvidenceValue(candidate.RefreshCadence); refreshCadence != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_refresh_cadence", Value: refreshCadence})
		}
		if includeResources := safeEvidenceValue(normalizedTableProperty(cur2TableConfiguration(candidate), "INCLUDE_RESOURCES")); includeResources != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_include_resources", Value: includeResources})
		}
		if region := safeEvidenceValue(candidate.Destination.Region); region != "" {
			evidence = append(evidence, workflow.PlanEvidence{Key: prefix + "_destination_region", Value: region})
		}
		evidence = append(evidence,
			workflow.PlanEvidence{Key: prefix + "_provider_source_type", Value: facts.ProviderSourceType},
			workflow.PlanEvidence{Key: prefix + "_cloud_validity", Value: facts.CloudValidity},
			workflow.PlanEvidence{Key: prefix + "_matilda_support", Value: facts.MatildaSupport},
			workflow.PlanEvidence{Key: prefix + "_pre_selection_metadata_status", Value: facts.MetadataStatus},
			workflow.PlanEvidence{Key: prefix + "_primary_issue", Value: facts.PrimaryIssue},
			workflow.PlanEvidence{Key: prefix + "_required_next_action", Value: facts.RequiredNextAction},
		)
	}
	return evidence
}

type candidateSelectionFacts struct {
	ProviderSourceType string
	CloudValidity      string
	MatildaSupport     string
	MetadataStatus     string
	PrimaryIssue       string
	RequiredNextAction string
}

func candidatePreSelectionFacts(candidate Export) candidateSelectionFacts {
	settings := collectCandidateSafeSettings(candidate)
	switch {
	case settings.Health == "UNHEALTHY":
		return blockedCandidateFacts("unsupported", "unhealthy", "AWS reports this export as unhealthy.")
	case len(settings.Missing) > 0:
		return blockedCandidateFacts("unverified", "incomplete", "pre-selection metadata is incomplete: missing "+strings.Join(settings.Missing, ", ")+".")
	case len(settings.Unsupported) > 0:
		return blockedCandidateFacts("unsupported", "unsupported", "pre-selection metadata has unsupported settings: "+strings.Join(settings.Unsupported, ", ")+".")
	case settings.Preferred:
		return candidateSelectionFacts{
			ProviderSourceType: "aws_data_exports_cur2",
			CloudValidity:      "cur2_candidate",
			MatildaSupport:     "preferred",
			MetadataStatus:     "preferred",
			PrimaryIssue:       "none",
			RequiredNextAction: "run full readiness preflight after selection.",
		}
	default:
		return candidateSelectionFacts{
			ProviderSourceType: "aws_data_exports_cur2",
			CloudValidity:      "cur2_candidate",
			MatildaSupport:     "supported",
			MetadataStatus:     "supported",
			PrimaryIssue:       "none",
			RequiredNextAction: "run full readiness preflight after selection.",
		}
	}
}

func blockedCandidateFacts(support string, status string, issue string) candidateSelectionFacts {
	return candidateSelectionFacts{
		ProviderSourceType: "aws_data_exports_cur2",
		CloudValidity:      "cur2_candidate",
		MatildaSupport:     support,
		MetadataStatus:     status,
		PrimaryIssue:       issue,
		RequiredNextAction: "review the candidate with direct preflight before selection.",
	}
}

type candidateSafeSettings struct {
	Health          string
	Output          string
	Compression     string
	Granularity     string
	Overwrite       string
	OutputType      string
	RefreshCadence  string
	IncludeResource string
	Destination     string
	Missing         []string
	Unsupported     []string
	Preferred       bool
}

func collectCandidateSafeSettings(candidate Export) candidateSafeSettings {
	tableConfig := cur2TableConfiguration(candidate)
	settings := candidateSafeSettings{
		Health:          safeEvidenceValue(normalizedOutputSetting(candidate.HealthStatus)),
		Output:          safeEvidenceValue(normalizedOutputSetting(candidate.Destination.Output.Format)),
		Compression:     safeEvidenceValue(normalizedOutputSetting(candidate.Destination.Output.Compression)),
		Granularity:     safeEvidenceValue(normalizedTableProperty(tableConfig, "TIME_GRANULARITY")),
		Overwrite:       safeEvidenceValue(normalizedOutputSetting(candidate.Destination.Output.Overwrite)),
		OutputType:      safeEvidenceValue(normalizedOutputSetting(candidate.Destination.Output.OutputType)),
		RefreshCadence:  safeEvidenceValue(normalizedOutputSetting(candidate.RefreshCadence)),
		IncludeResource: safeEvidenceValue(normalizedTableProperty(tableConfig, "INCLUDE_RESOURCES")),
		Destination:     safeEvidenceValue(candidate.Destination.Region),
	}
	settings.Missing = missingCandidateSettings(settings)
	settings.Unsupported = unsupportedCandidateSettings(settings)
	settings.Preferred = candidateSettingsPreferred(settings)
	return settings
}

func missingCandidateSettings(settings candidateSafeSettings) []string {
	missing := []string{}
	if settings.Health == "" {
		missing = append(missing, "health status")
	}
	if settings.Output == "" {
		missing = append(missing, "output format")
	}
	if settings.Compression == "" {
		missing = append(missing, "compression")
	}
	if settings.Granularity == "" {
		missing = append(missing, "time granularity")
	}
	if settings.Overwrite == "" {
		missing = append(missing, "file versioning")
	}
	if settings.OutputType == "" {
		missing = append(missing, "output type")
	}
	if settings.RefreshCadence == "" {
		missing = append(missing, "refresh cadence")
	}
	if settings.Destination == "" {
		missing = append(missing, "destination region")
	}
	return missing
}

func unsupportedCandidateSettings(settings candidateSafeSettings) []string {
	unsupported := []string{}
	if settings.Health != "" && settings.Health != "HEALTHY" && settings.Health != "UNHEALTHY" {
		unsupported = append(unsupported, "health status "+settings.Health)
	}
	if settings.Output != "" && settings.Compression != "" && !supportedCandidateOutput(settings.Output, settings.Compression) {
		unsupported = append(unsupported, "output/compression "+settings.Output+"/"+settings.Compression)
	}
	if settings.Granularity != "" && !supportedCandidateGranularity(settings.Granularity) {
		unsupported = append(unsupported, "time granularity "+settings.Granularity)
	}
	if settings.Overwrite != "" && !supportedCandidateOverwrite(settings.Overwrite) {
		unsupported = append(unsupported, "file versioning "+settings.Overwrite)
	}
	if settings.OutputType != "" && settings.OutputType != "CUSTOM" {
		unsupported = append(unsupported, "output type "+settings.OutputType)
	}
	if settings.RefreshCadence != "" && settings.RefreshCadence != "SYNCHRONOUS" {
		unsupported = append(unsupported, "refresh cadence "+settings.RefreshCadence)
	}
	if settings.IncludeResource != "" && settings.IncludeResource != "TRUE" && settings.IncludeResource != "FALSE" {
		unsupported = append(unsupported, "include resources "+settings.IncludeResource)
	}
	return unsupported
}

func candidateSettingsPreferred(settings candidateSafeSettings) bool {
	return settings.Health == "HEALTHY" &&
		settings.Output == "TEXT_OR_CSV" &&
		settings.Compression == "GZIP" &&
		settings.Granularity == "MONTHLY" &&
		settings.Overwrite == "CREATE_NEW_REPORT" &&
		settings.OutputType == "CUSTOM" &&
		settings.RefreshCadence == "SYNCHRONOUS" &&
		(settings.IncludeResource == "" || settings.IncludeResource == "FALSE") &&
		settings.Destination != "" &&
		len(settings.Missing) == 0 &&
		len(settings.Unsupported) == 0
}

func supportedCandidateOutput(output string, compression string) bool {
	return output == "TEXT_OR_CSV" && compression == "GZIP" ||
		output == "PARQUET" && compression == "PARQUET"
}

func supportedCandidateGranularity(granularity string) bool {
	switch granularity {
	case "HOURLY", "DAILY", "MONTHLY":
		return true
	default:
		return false
	}
}

func supportedCandidateOverwrite(overwrite string) bool {
	switch overwrite {
	case "CREATE_NEW_REPORT", "OVERWRITE_REPORT":
		return true
	default:
		return false
	}
}

func hasFailedFinding(findings []checkFinding) bool {
	for _, finding := range findings {
		if finding.Status == workflow.CheckFail {
			return true
		}
	}
	return false
}

func safeEvidenceValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"/users/",
		"/private/",
		"/tmp/",
		"/home/",
		"\\",
		".pem",
		"access_key",
		"apikey",
		"api_key",
		"arn:",
		"bearer ",
		"client-secret",
		"client_secret",
		"ocid1.",
		"passphrase",
		"password",
		"plain-secret",
		"plain-token",
		"private-key",
		"private_key",
		"projects/",
		"refresh_token",
		"secret_key",
		"service_account_json",
		"session_token",
		"token=",
	} {
		if strings.Contains(lower, forbidden) {
			return ""
		}
	}
	if s3handoff.SensitiveIdentifierLike(value) {
		return ""
	}
	if strings.ContainsAny(value, `/\`) {
		return ""
	}
	return value
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

func (runner Runner) deliveryFindings(ctx context.Context, client Client, export Export) []checkFinding {
	executions, err := listAllExecutions(ctx, client, export.ExportARN)
	if err != nil {
		return []checkFinding{failFinding(providerErrorCode(err, "aws_data_exports_access_denied"), "AWS export delivery status", "AWS Data Exports delivery executions could not be inspected.")}
	}
	if len(executions) == 0 {
		if !export.CreatedAt.IsZero() && runner.now.Sub(export.CreatedAt) >= 24*time.Hour {
			return []checkFinding{failFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "No AWS Data Exports delivery execution has started after the expected delivery delay.")}
		}
		return []checkFinding{warnFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "No AWS Data Exports delivery execution has started yet.", true)}
	}

	latest, ok := latestObservedExecution(executions)
	if !ok {
		return []checkFinding{warnFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "AWS Data Exports delivery status is not conclusive yet.", true)}
	}
	if latest.ID != "" {
		inspected, err := client.GetExecution(ctx, export.ExportARN, latest.ID)
		if err != nil {
			return []checkFinding{failFinding(providerErrorCode(err, "aws_data_exports_access_denied"), "AWS export delivery status", "AWS Data Exports delivery execution could not be inspected.")}
		}
		latest = inspected
	}
	switch strings.ToUpper(strings.TrimSpace(latest.Status)) {
	case "DELIVERY_SUCCESS":
		return []checkFinding{passFinding("aws_cur2_delivery_ready", "AWS export delivery status", "Latest AWS Data Exports delivery status is healthy.")}
	case "INITIATION_IN_PROCESS", "QUERY_QUEUED", "QUERY_IN_PROCESS", "DELIVERY_IN_PROCESS":
		return []checkFinding{warnFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "Latest AWS Data Exports delivery is still in progress.", true)}
	case "QUERY_FAILURE", "DELIVERY_FAILURE":
		return []checkFinding{failFinding("aws_cur2_export_invalid_shape", "AWS export delivery status", "Latest AWS Data Exports delivery reports a failure.")}
	}
	return []checkFinding{warnFinding("aws_cur2_delivery_not_started", "AWS export delivery status", "AWS Data Exports delivery status is not conclusive yet.", true)}
}

func latestObservedExecution(executions []Execution) (Execution, bool) {
	var latest Execution
	ambiguous := false
	for _, execution := range executions {
		if execution.StatusObservedAt.IsZero() {
			return Execution{}, false
		}
		if latest.StatusObservedAt.IsZero() || execution.StatusObservedAt.After(latest.StatusObservedAt) {
			latest = execution
			ambiguous = false
			continue
		}
		if execution.StatusObservedAt.Equal(latest.StatusObservedAt) {
			ambiguous = true
		}
	}
	if latest.StatusObservedAt.IsZero() || ambiguous {
		return Execution{}, false
	}
	return latest, true
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

func inspectBucketPolicy(ctx context.Context, client Client, export Export) checkFinding {
	policy, err := client.GetBucketPolicy(ctx, export.Destination.Bucket)
	if err != nil {
		return failFinding(providerErrorCode(err, "aws_s3_bucket_policy_inaccessible"), "AWS S3 bucket policy visibility", "S3 bucket policy could not be inspected.")
	}
	return validateBucketPolicy(policy, export.SourceAccount, export.SourceARN, export)
}

func policyFindingAfterCurrentDataProof(policyFinding checkFinding, previousMonthFinding checkFinding) checkFinding {
	if policyFinding.Status != workflow.CheckFail || previousMonthFinding.Code != "aws_cur2_previous_month_ready" {
		return policyFinding
	}
	policyFinding.Status = workflow.CheckWarn
	policyFinding.TopLevel = true
	policyFinding.ManualAction = false
	switch policyFinding.Code {
	case "aws_s3_bucket_policy_inaccessible":
		policyFinding.Message = "Previous-month CUR 2.0 billing data and manifest are present, but the S3 bucket policy could not be inspected. Future AWS Data Exports delivery or backfill readiness is not proven."
	case "aws_s3_delivery_policy_missing":
		policyFinding.Message = "Previous-month CUR 2.0 billing data and manifest are present, but the S3 bucket policy does not prove future AWS Data Exports delivery or backfill readiness."
	default:
		policyFinding.Message = "Previous-month CUR 2.0 billing data and manifest are present, but future AWS Data Exports delivery or backfill readiness is not proven."
	}
	return policyFinding
}

func (runner Runner) previousMonthFinding(ctx context.Context, client Client, export Export, period string) checkFinding {
	dataPrefix := previousMonthDataPrefix(export, period)
	manifestPrefix := previousMonthManifestPrefix(export, period)

	dataFound, dataIncomplete, err := listPrefixHasMatchingObject(ctx, client, export.Destination.Bucket, dataPrefix, func(key string) bool {
		return matchesPreviousMonthDataKey(key, export, period)
	})
	if err != nil {
		return failFinding(providerErrorCode(err, "aws_s3_bucket_inaccessible"), "AWS previous-month billing data", "S3 previous-month billing data could not be listed.")
	}
	if dataIncomplete {
		return failFinding("aws_cur2_previous_month_missing", "AWS previous-month billing data", "Bounded S3 pagination ended before previous-month billing data availability could be proven.")
	}

	manifestFound, manifestIncomplete, err := listPrefixHasMatchingObject(ctx, client, export.Destination.Bucket, manifestPrefix, func(key string) bool {
		return matchesPreviousMonthManifestKey(key, export, period)
	})
	if err != nil {
		return failFinding(providerErrorCode(err, "aws_s3_bucket_inaccessible"), "AWS previous-month billing data", "S3 previous-month billing data could not be listed.")
	}
	if manifestIncomplete {
		return failFinding("aws_cur2_previous_month_missing", "AWS previous-month billing data", "Bounded S3 pagination ended before previous-month billing data availability could be proven.")
	}
	if dataFound && manifestFound {
		return withEvidence(passFinding("aws_cur2_previous_month_ready", "AWS previous-month billing data", "Previous-month CUR 2.0 billing partition and manifest are present."),
			s3handoff.PreviousMonthEvidence(period, dataPrefix, manifestPrefix)...,
		)
	}

	evidence := s3handoff.PreviousMonthEvidence(period, dataPrefix, manifestPrefix)
	if !dataFound {
		evidence = append(evidence, workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "data_partition"})
	}
	if !manifestFound {
		evidence = append(evidence, workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "manifest"})
	}
	return withEvidence(manualFinding("aws_backfill_manual_step_required", "AWS previous-month billing data", "Previous-month CUR 2.0 billing data or manifest is missing and may require AWS support backfill."),
		evidence...,
	)
}

func listPrefixHasMatchingObject(ctx context.Context, client Client, bucket string, prefix string, matches func(string) bool) (bool, bool, error) {
	if strings.TrimSpace(prefix) == "" {
		return false, false, nil
	}
	token := ""
	for pageNumber := 0; pageNumber < maxListObjectPages; pageNumber++ {
		page, err := client.ListObjects(ctx, bucket, prefix, token, 1000)
		if err != nil {
			return false, false, err
		}
		for _, key := range page.Keys {
			if matches(key) {
				return true, false, nil
			}
		}
		if page.NextToken == "" {
			return false, false, nil
		}
		token = page.NextToken
	}
	return false, true, nil
}

func previousMonthDataPrefix(export Export, period string) string {
	return previousMonthPrefix(export, period, "data")
}

func previousMonthManifestPrefix(export Export, period string) string {
	return previousMonthPrefix(export, period, "metadata")
}

func previousMonthPrefix(export Export, period string, kind string) string {
	prefix := strings.Trim(strings.TrimSpace(export.Destination.Prefix), "/")
	name := strings.Trim(strings.TrimSpace(export.Name), "/")
	if prefix == "" || name == "" || period == "" || kind == "" {
		return ""
	}
	return prefix + "/" + name + "/" + kind + "/BILLING_PERIOD=" + period + "/"
}

func matchesPreviousMonthDataKey(key string, export Export, period string) bool {
	expected := previousMonthDataPrefix(export, period)
	if expected == "" || !strings.HasPrefix(key, expected) {
		return false
	}
	remainder := strings.TrimPrefix(key, expected)
	return remainder != "" && !strings.HasSuffix(remainder, "/")
}

func matchesPreviousMonthManifestKey(key string, export Export, period string) bool {
	expected := previousMonthManifestPrefix(export, period)
	if expected == "" || !strings.HasPrefix(key, expected) {
		return false
	}
	return strings.HasPrefix(key, expected) && strings.HasSuffix(key, "Manifest.json")
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
	case "aws_config_timeout":
		return "AWS SDK configuration timed out."
	case "aws_config_cancelled":
		return "AWS SDK configuration was cancelled."
	case "aws_config_profile_shadowed":
		return "AWS profile selection is blocked because credential environment variables would take precedence."
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
		{
			Label: "AWS CUR 2.0 Guided Selection UX",
			URI:   "docs/references/aws/aws-cur2-export-selection-guided-ux.md",
		},
		{
			Label: "AWS Rapid Assessment Billing Handoff Schema",
			URI:   "docs/references/aws/aws-rapid-assessment-billing-handoff-schema.md",
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
		Message:       messageFor(status, code),
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

func messageFor(status workflow.RunStatus, code string) string {
	switch code {
	case "aws_cur2_preflight_ready":
		return "AWS CUR 2.0 billing preflight is ready."
	case "aws_cur2_time_granularity_not_preferred":
		return "AWS CUR 2.0 billing preflight is ready with a non-preferred time granularity."
	case "aws_cur2_time_granularity_unverified":
		return "AWS CUR 2.0 billing preflight is ready, but time granularity could not be confirmed."
	case "aws_cur2_delivery_not_started":
		return "AWS CUR 2.0 billing preflight is ready, but export delivery is not complete yet."
	case "aws_backfill_manual_step_required":
		return "AWS CUR 2.0 billing preflight requires previous-month billing backfill or manual remediation."
	case "aws_cur2_previous_month_missing":
		return "AWS CUR 2.0 billing preflight could not prove previous-month billing data availability."
	case "aws_s3_delivery_policy_missing":
		if status == workflow.StatusReady {
			return "AWS CUR 2.0 billing preflight is ready for current previous-month data, but S3 bucket policy does not prove future AWS Data Exports delivery."
		}
		return "AWS CUR 2.0 billing preflight found an S3 bucket policy blocker for AWS Data Exports delivery."
	case "aws_s3_bucket_policy_inaccessible":
		if status == workflow.StatusReady {
			return "AWS CUR 2.0 billing preflight is ready for current previous-month data, but S3 bucket policy could not be inspected for future AWS Data Exports delivery."
		}
		return "AWS CUR 2.0 billing preflight could not inspect the S3 bucket policy needed for AWS Data Exports delivery."
	case "aws_data_exports_throttled":
		return "AWS Data Exports throttled a read-only preflight check. Wait briefly, then rerun preflight."
	case "aws_data_exports_transient":
		return "AWS Data Exports returned a transient provider error during read-only preflight. Wait briefly, then rerun preflight."
	case "aws_cur2_export_selection_required":
		return "AWS CUR 2.0 billing preflight found one candidate and is waiting for guided selection."
	case "aws_cur2_export_health_unverified":
		return "AWS CUR 2.0 billing preflight could not verify the selected export health status."
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
			ID:            finding.Code,
			Status:        finding.Status,
			Title:         finding.Title,
			Message:       finding.Message,
			Evidence:      evidence,
			SourceHandles: state.handles,
		})
	}
	if len(checks) == 0 {
		checks = append(checks, workflow.PlanCheck{
			ID:            "aws_provider_capability_blocked",
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
