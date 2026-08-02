package billingbackfill

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func backfillRequestReference(exportARN string, period string) string {
	sum := sha256.Sum256([]byte("aws:cur2-backfill:" + strings.TrimSpace(exportARN) + ":" + strings.TrimSpace(period)))
	return "backfill-" + hex.EncodeToString(sum[:])[:16]
}

func buildCreateCaseRequest(classification supportClassification, context backfillContext, reference string) CreateCaseRequest {
	return CreateCaseRequest{
		Language:     classification.Language,
		IssueType:    classification.IssueType,
		ServiceCode:  classification.ServiceCode,
		CategoryCode: classification.CategoryCode,
		SeverityCode: classification.SeverityCode,
		Subject:      fmt.Sprintf("Request AWS CUR 2.0 previous-month backfill [%s]", reference),
		Body:         supportCaseBody(context, reference),
	}
}

type supportCaseBindingMaterial struct {
	Classification        supportClassification `json:"classification"`
	Request               CreateCaseRequest     `json:"request"`
	ExportARN             string                `json:"export_arn"`
	ExportRef             string                `json:"export_ref"`
	Period                string                `json:"period"`
	MissingDataPartition  bool                  `json:"missing_data_partition"`
	MissingManifest       bool                  `json:"missing_manifest"`
	SelectedReportVisible bool                  `json:"selected_report_visible"`
}

func supportCaseBindingRef(classification supportClassification, context backfillContext, reference string) string {
	material := supportCaseBindingMaterial{
		Classification:        classification,
		Request:               buildCreateCaseRequest(classification, context, reference),
		ExportARN:             strings.TrimSpace(context.Export.ExportARN),
		ExportRef:             strings.TrimSpace(context.ExportRef),
		Period:                strings.TrimSpace(context.Period),
		MissingDataPartition:  context.MissingDataPartition,
		MissingManifest:       context.MissingManifest,
		SelectedReportVisible: context.SelectedReportVisible,
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(encoded)
	return "support_case_" + letterEncodeHash(sum[:], 16)
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

func supportCaseBody(context backfillContext, reference string) string {
	missing := []string{}
	if context.MissingDataPartition {
		missing = append(missing, "data partition")
	}
	if context.MissingManifest {
		missing = append(missing, "manifest")
	}
	if len(missing) == 0 {
		missing = append(missing, "previous-month billing export")
	}

	return fmt.Sprintf(`Please backfill AWS CUR 2.0 cost data for the requested billing period.

Request reference: %s
Billing period: %s
Report name: %s
S3 bucket: %s
S3 prefix: %s
Missing components: %s

This request is for Matilda Rapid Assessment - Billing Based onboarding. Please backfill the missing Cost and Usage Report export data for the billing period above.`,
		reference,
		context.Period,
		context.Export.Name,
		context.Export.Destination.Bucket,
		context.Export.Destination.Prefix,
		strings.Join(missing, ", "),
	)
}
