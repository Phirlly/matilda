package cur2preflight

import "time"

func SafeCUR2ExportRef(exportARN string) string {
	return cur2ExportRef(exportARN)
}

func SafeCUR2ExportRefs(exports []Export) ([]string, error) {
	return cur2ExportRefs(exports)
}

func IsCUR2ExportCandidate(export Export) bool {
	return hasCUR2TableConfiguration(export) || referencesCUR2QuerySource(export.QueryStatement)
}

func PreviousBillingPeriod(now time.Time) string {
	return previousBillingPeriod(now)
}

func PreviousMonthDataPrefix(export Export, period string) string {
	return previousMonthDataPrefix(export, period)
}

func PreviousMonthManifestPrefix(export Export, period string) string {
	return previousMonthManifestPrefix(export, period)
}

func MatchesPreviousMonthDataKey(key string, export Export, period string) bool {
	return matchesPreviousMonthDataKey(key, export, period)
}

func MatchesPreviousMonthManifestKey(key string, export Export, period string) bool {
	return matchesPreviousMonthManifestKey(key, export, period)
}
