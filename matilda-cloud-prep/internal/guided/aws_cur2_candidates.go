package guided

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func handleAWSBillingResult(ctx context.Context, reader *bufio.Scanner, stdout io.Writer, config Config, selected awsVerifiedSource, result workflow.Result) error {
	if result.Code != "aws_cur2_export_ambiguous" {
		writeAWSBillingSummary(stdout, selected.Identity.Source, result)
		return nil
	}

	candidates := cur2Candidates(result)
	if len(candidates) == 0 {
		writeAWSBillingSummary(stdout, selected.Identity.Source, result)
		return nil
	}

	fmt.Fprintf(stdout, "Classifying %d CUR 2.0 export candidates\n", len(candidates))
	classified := classifyCUR2Candidates(ctx, config.Registry, selected.Identity.Source, candidates)
	selectable := selectableCUR2Candidates(classified)
	switch len(selectable) {
	case 0:
		fmt.Fprintln(stdout, "No selectable CUR 2.0 export candidate was found.")
		for _, item := range classified {
			fmt.Fprintf(stdout, "  %s blocked: %s\n", item.Candidate.Ref, item.Result.Code)
		}
		return nil
	case 1:
		item := selectable[0]
		fmt.Fprintf(stdout, "Auto-selected CUR 2.0 export %s\n", item.Candidate.Ref)
		writeBlockedClassifications(stdout, classified)
		writeAWSBillingSummary(stdout, selected.Identity.Source, item.Result)
		return nil
	default:
		fmt.Fprintln(stdout, "Select AWS CUR 2.0 export")
		for index, item := range selectable {
			fmt.Fprintf(stdout, "  %d. %s\n", index+1, candidateLabel(item.Candidate))
			fmt.Fprintf(stdout, "     %s\n", item.Result.Code)
		}
		index, err := readChoice(reader, stdout, fmt.Sprintf("Select AWS CUR 2.0 export [1-%d]: ", len(selectable)), "AWS CUR 2.0 export", len(selectable))
		if err != nil {
			return err
		}
		writeBlockedClassifications(stdout, classified)
		writeAWSBillingSummary(stdout, selected.Identity.Source, selectable[index].Result)
		return nil
	}
}

type cur2Candidate struct {
	Index       int
	Ref         string
	Health      string
	Output      string
	Destination string
}

type classifiedCUR2Candidate struct {
	Candidate cur2Candidate
	Result    workflow.Result
}

func cur2Candidates(result workflow.Result) []cur2Candidate {
	byIndex := map[int]*cur2Candidate{}
	if result.Plan == nil {
		return nil
	}
	for _, check := range result.Plan.Checks {
		for _, evidence := range check.Evidence {
			index, field, ok := candidateEvidenceKey(evidence.Key)
			if !ok {
				continue
			}
			candidate := byIndex[index]
			if candidate == nil {
				candidate = &cur2Candidate{Index: index}
				byIndex[index] = candidate
			}
			switch field {
			case "export_ref":
				if safeCUR2ExportRef(evidence.Value) {
					candidate.Ref = strings.TrimSpace(evidence.Value)
				}
			case "health":
				candidate.Health = safeCandidateLabelValue(evidence.Value)
			case "output_format":
				candidate.Output = safeCandidateLabelValue(evidence.Value)
			case "destination_region":
				candidate.Destination = safeCandidateLabelValue(evidence.Value)
			}
		}
	}

	indexes := make([]int, 0, len(byIndex))
	for index := range byIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	candidates := make([]cur2Candidate, 0, len(indexes))
	for _, index := range indexes {
		candidate := *byIndex[index]
		if candidate.Ref != "" {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func safeCUR2ExportRef(value string) bool {
	options := workflow.ExecutionOptions{
		Selectors: &workflow.ExecutionSelectors{
			AWS: &workflow.AWSExecutionSelectors{
				CUR2ExportRef: value,
			},
		},
	}
	_, err := workflow.NormalizeExecutionOptions(options)
	return err == nil
}

func candidateEvidenceKey(key string) (int, string, bool) {
	const prefix = "candidate_"
	if !strings.HasPrefix(key, prefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(key, prefix)
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	index, err := strconv.Atoi(parts[0])
	if err != nil || index <= 0 {
		return 0, "", false
	}
	return index, parts[1], true
}

func classifyCUR2Candidates(ctx context.Context, registry workflow.Registry, source billingguide.CredentialSource, candidates []cur2Candidate) []classifiedCUR2Candidate {
	classified := make([]classifiedCUR2Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		options, err := awsBillingOptions(source)
		if err != nil {
			continue
		}
		if options.Selectors == nil {
			options.Selectors = &workflow.ExecutionSelectors{}
		}
		if options.Selectors.AWS == nil {
			options.Selectors.AWS = &workflow.AWSExecutionSelectors{}
		}
		options.Selectors.AWS.CUR2ExportRef = candidate.Ref
		options, err = workflow.NormalizeExecutionOptions(options)
		if err != nil {
			continue
		}
		result := registry.ExecuteContext(ctx, awsBillingRequest(), options)
		classified = append(classified, classifiedCUR2Candidate{Candidate: candidate, Result: result})
	}
	return classified
}

func selectableCUR2Candidates(classified []classifiedCUR2Candidate) []classifiedCUR2Candidate {
	selectable := []classifiedCUR2Candidate{}
	for _, item := range classified {
		if item.Result.Status == workflow.StatusReady || item.Result.Status == workflow.RunStatusManualSteps {
			selectable = append(selectable, item)
		}
	}
	return selectable
}

func writeBlockedClassifications(stdout io.Writer, classified []classifiedCUR2Candidate) {
	for _, item := range classified {
		if item.Result.Status == workflow.StatusReady || item.Result.Status == workflow.RunStatusManualSteps {
			continue
		}
		fmt.Fprintf(stdout, "  %s blocked: %s\n", item.Candidate.Ref, item.Result.Code)
	}
}

func candidateLabel(candidate cur2Candidate) string {
	parts := []string{candidate.Ref}
	if health := safeCandidateLabelValue(candidate.Health); health != "" {
		parts = append(parts, "health "+health)
	}
	if output := safeCandidateLabelValue(candidate.Output); output != "" {
		parts = append(parts, "output "+output)
	}
	if destination := safeCandidateLabelValue(candidate.Destination); destination != "" {
		parts = append(parts, "region "+destination)
	}
	return strings.Join(parts, ", ")
}

func safeCandidateLabelValue(value string) string {
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
	if sensitiveCandidateIdentifierLikeValue(value) {
		return ""
	}
	if strings.ContainsAny(value, `/\`) {
		return ""
	}
	return value
}

func sensitiveCandidateIdentifierLikeValue(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 12 && allDigits(value) {
		return true
	}
	upper := strings.ToUpper(value)
	return len(upper) == 20 &&
		(strings.HasPrefix(upper, "AKIA") || strings.HasPrefix(upper, "ASIA")) &&
		allUpperAlphaNumeric(upper)
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func allUpperAlphaNumeric(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return value != ""
}
