package billingbackfill

import (
	"context"
	"fmt"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	maxDataExportsListPages = 100
	maxExportDetailChecks   = 100
)

type backfillContext struct {
	Export                cur2preflight.Export
	ExportRef             string
	Period                string
	MissingDataPartition  bool
	MissingManifest       bool
	SelectedReportVisible bool
}

func (context backfillContext) evidence() []workflow.PlanEvidence {
	evidence := []workflow.PlanEvidence{
		{Key: "cur_version", Value: "CUR2.0"},
		{Key: "selected_export_ref", Value: context.ExportRef},
		{Key: "previous_billing_period", Value: context.Period},
	}
	if context.MissingDataPartition {
		evidence = append(evidence, workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "data_partition"})
	}
	if context.MissingManifest {
		evidence = append(evidence, workflow.PlanEvidence{Key: "missing_previous_month_component", Value: "manifest"})
	}
	return evidence
}

func (runner Runner) resolveBackfillContext(ctx context.Context, client Client, options workflow.ExecutionOptions) (backfillContext, error) {
	export, ref, err := selectCUR2Export(ctx, client, awsCUR2ExportRefOption(options))
	if err != nil {
		return backfillContext{}, err
	}
	period := cur2preflight.PreviousBillingPeriod(runner.now)
	dataFound, err := prefixHasMatchingObject(ctx, client, export, cur2preflight.PreviousMonthDataPrefix(export, period), func(key string) bool {
		return cur2preflight.MatchesPreviousMonthDataKey(key, export, period)
	})
	if err != nil {
		return backfillContext{}, err
	}
	manifestFound, err := prefixHasMatchingObject(ctx, client, export, cur2preflight.PreviousMonthManifestPrefix(export, period), func(key string) bool {
		return cur2preflight.MatchesPreviousMonthManifestKey(key, export, period)
	})
	if err != nil {
		return backfillContext{}, err
	}
	return backfillContext{
		Export:               export,
		ExportRef:            ref,
		Period:               period,
		MissingDataPartition: !dataFound,
		MissingManifest:      !manifestFound,
	}, nil
}

func selectCUR2Export(ctx context.Context, client Client, requestedRef string) (cur2preflight.Export, string, error) {
	summaries, err := listAllExportSummaries(ctx, client)
	if err != nil {
		return cur2preflight.Export{}, "", err
	}
	if len(summaries) > maxExportDetailChecks {
		return cur2preflight.Export{}, "", dataExportsPaginationError()
	}

	exports := []cur2preflight.Export{}
	for _, summary := range summaries {
		if strings.TrimSpace(summary.ExportARN) == "" {
			continue
		}
		export, err := client.GetExport(ctx, summary.ExportARN)
		if err != nil {
			return cur2preflight.Export{}, "", err
		}
		if export.Name == "" {
			export.Name = summary.Name
		}
		if export.ExportARN == "" {
			export.ExportARN = summary.ExportARN
		}
		if cur2preflight.IsCUR2ExportCandidate(export) {
			exports = append(exports, export)
		}
	}
	if len(exports) == 0 {
		return cur2preflight.Export{}, "", NewProviderError("aws_cur2_creation_required", "No AWS CUR 2.0 export exists.")
	}
	refs, err := cur2preflight.SafeCUR2ExportRefs(exports)
	if err != nil {
		return cur2preflight.Export{}, "", NewProviderError("aws_cur2_export_ref_collision", "CUR 2.0 export refs are not unique.")
	}
	if requestedRef != "" {
		for index, ref := range refs {
			if ref == requestedRef {
				return exports[index], ref, nil
			}
		}
		return cur2preflight.Export{}, "", NewProviderError("aws_cur2_export_ref_not_found", "Requested CUR 2.0 export ref did not match a discovered candidate.")
	}
	if len(exports) != 1 {
		return cur2preflight.Export{}, "", NewProviderError("aws_cur2_export_ambiguous", "Multiple CUR 2.0 export candidates were found.")
	}
	return exports[0], refs[0], nil
}

func listAllExportSummaries(ctx context.Context, client Client) ([]cur2preflight.ExportSummary, error) {
	exports := []cur2preflight.ExportSummary{}
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

func tokenWasSeen(seen map[string]struct{}, token string) bool {
	if _, exists := seen[token]; exists {
		return true
	}
	seen[token] = struct{}{}
	return false
}

func dataExportsPaginationError() error {
	return NewProviderError("aws_data_exports_pagination_unbounded", "AWS Data Exports pagination did not converge within the bounded apply-prereqs inspection window.")
}

func awsCUR2ExportRefOption(options workflow.ExecutionOptions) string {
	if options.Selectors == nil || options.Selectors.AWS == nil {
		return ""
	}
	return strings.TrimSpace(options.Selectors.AWS.CUR2ExportRef)
}

func prefixHasMatchingObject(ctx context.Context, client Client, export cur2preflight.Export, prefix string, matches func(string) bool) (bool, error) {
	if strings.TrimSpace(prefix) == "" {
		return false, fmt.Errorf("previous-month prefix is unavailable")
	}
	token := ""
	for pageNumber := 0; pageNumber < maxListObjectPages; pageNumber++ {
		page, err := client.ListObjects(ctx, export.Destination.Bucket, prefix, token, 1000)
		if err != nil {
			return false, err
		}
		for _, key := range page.Keys {
			if matches(key) {
				return true, nil
			}
		}
		if page.NextToken == "" {
			return false, nil
		}
		token = page.NextToken
	}
	return false, NewProviderError("aws_cur2_previous_month_missing", "Bounded S3 pagination ended before previous-month billing data availability could be proven.")
}
