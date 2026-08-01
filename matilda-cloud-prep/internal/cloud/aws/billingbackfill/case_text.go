package billingbackfill

import (
	"crypto/sha256"
	"encoding/hex"
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
