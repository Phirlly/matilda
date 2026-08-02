package billingcur2setup

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

func generatedNameCandidates(identity identityContext, region string) []setupFacts {
	hash := namingHash(identity, region)
	candidates := make([]setupFacts, 0, maxBucketNameCandidates)
	for index := 0; index < maxBucketNameCandidates; index++ {
		suffix := fmt.Sprintf("%02d", index)
		candidates = append(candidates, setupFacts{
			BucketName:     fmt.Sprintf("matilda-ra-billing-aws-%s-%s-%s", sanitizedRegion(region), hash, suffix),
			ExportName:     fmt.Sprintf("matilda-cur2-ra-billing-%s-%s", hash, suffix),
			Prefix:         matildaBillingPrefix,
			CandidateIndex: suffix,
		})
	}
	return candidates
}

func namingHash(identity identityContext, region string) string {
	material := map[string]string{
		"account_id": identity.AccountID,
		"namespace":  "matilda",
		"partition":  identity.Partition,
		"provider":   "aws",
		"purpose":    "matilda-cloud-prep:aws:rapid-assessment:billing:cur2",
		"region":     region,
		"shorthand":  "ra-billing",
		"version":    "v1",
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(encoded)
	return letterEncodeHash(sum[:], 12)
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

type setupBindingMaterial struct {
	AccountID           string                       `json:"account_id"`
	Partition           string                       `json:"partition"`
	S3Region            string                       `json:"s3_region"`
	DataExportsRegion   string                       `json:"data_exports_region"`
	GeneratedFacts      setupFacts                   `json:"generated_facts"`
	QueryStatement      string                       `json:"query_statement"`
	TableConfigurations map[string]map[string]string `json:"table_configurations"`
	Destination         setupBindingDestination      `json:"destination"`
	RefreshCadence      string                       `json:"refresh_cadence"`
	ManagedExportARN    string                       `json:"managed_export_arn,omitempty"`
}

type setupBindingDestination struct {
	Bucket string             `json:"bucket"`
	Prefix string             `json:"prefix"`
	Region string             `json:"region"`
	Output setupBindingOutput `json:"output"`
}

type setupBindingOutput struct {
	Format      string `json:"format"`
	Compression string `json:"compression"`
	Overwrite   string `json:"overwrite"`
	OutputType  string `json:"output_type"`
}

func setupBindingRef(plan setupPlan) string {
	request := buildCreateExportRequest(plan)
	material := setupBindingMaterial{
		AccountID:           plan.Identity.AccountID,
		Partition:           plan.Identity.Partition,
		S3Region:            plan.Region,
		DataExportsRegion:   dataExportsRegion,
		GeneratedFacts:      plan.Facts,
		QueryStatement:      request.QueryStatement,
		TableConfigurations: request.TableConfigurations,
		Destination: setupBindingDestination{
			Bucket: request.Destination.Bucket,
			Prefix: request.Destination.Prefix,
			Region: request.Destination.Region,
			Output: setupBindingOutput{
				Format:      request.Destination.Output.Format,
				Compression: request.Destination.Output.Compression,
				Overwrite:   request.Destination.Output.Overwrite,
				OutputType:  request.Destination.Output.OutputType,
			},
		},
		RefreshCadence: request.RefreshCadence,
	}
	if plan.ManagedExport != nil {
		material.ManagedExportARN = plan.ManagedExport.ExportARN
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(encoded)
	return "setup_" + letterEncodeHash(sum[:], 16)
}

func partitionFromARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) == 6 && strings.TrimSpace(parts[0]) == "arn" && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func sanitizedRegion(region string) string {
	region = strings.ToLower(strings.TrimSpace(region))
	var builder strings.Builder
	for _, r := range region {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-':
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "us-east-1"
	}
	return builder.String()
}
