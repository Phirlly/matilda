package billingbackfill

import (
	"context"
	"fmt"
	"strings"
)

type supportClassification struct {
	Language     string
	IssueType    string
	ServiceCode  string
	CategoryCode string
	SeverityCode string
}

func resolveSupportClassification(ctx context.Context, client Client) (supportClassification, error) {
	const language = "en"
	services, err := client.DescribeServices(ctx, DescribeServicesRequest{Language: language})
	if err != nil {
		return supportClassification{}, err
	}
	severity, err := selectLowSeverity(ctx, client, language)
	if err != nil {
		return supportClassification{}, err
	}

	candidates := supportCategoryCandidates(services)
	if len(candidates) == 0 {
		return supportClassification{}, NewProviderError("aws_support_case_classification_unavailable", "No billing-related AWS Support category was discovered.")
	}

	valid := []supportClassification{}
	var firstOptionsErr error
	for _, candidate := range candidates {
		for _, issueType := range []string{"technical", "customer-service"} {
			request := DescribeCreateCaseOptionsRequest{
				Language:     language,
				IssueType:    issueType,
				ServiceCode:  candidate.serviceCode,
				CategoryCode: candidate.categoryCode,
			}
			options, err := client.DescribeCreateCaseOptions(ctx, request)
			if err != nil {
				if firstOptionsErr == nil {
					firstOptionsErr = err
				}
				continue
			}
			if options.Available {
				valid = append(valid, supportClassification{
					Language:     language,
					IssueType:    issueType,
					ServiceCode:  candidate.serviceCode,
					CategoryCode: candidate.categoryCode,
					SeverityCode: severity.Code,
				})
			}
		}
	}
	if firstOptionsErr != nil {
		if len(valid) == 0 {
			return supportClassification{}, firstOptionsErr
		}
		return supportClassification{}, NewProviderError("aws_support_create_case_options_unverified", "AWS Support create-case options could not be verified for every backfill candidate.")
	}
	if len(valid) != 1 {
		return supportClassification{}, NewProviderError("aws_support_case_classification_ambiguous", fmt.Sprintf("AWS Support returned %d possible backfill classifications.", len(valid)))
	}
	return valid[0], nil
}

type supportCategoryCandidate struct {
	serviceCode  string
	categoryCode string
}

func supportCategoryCandidates(services []SupportService) []supportCategoryCandidate {
	candidates := []supportCategoryCandidate{}
	for _, service := range services {
		serviceCode := strings.TrimSpace(service.Code)
		if serviceCode == "" {
			continue
		}
		for _, category := range service.Categories {
			categoryCode := strings.TrimSpace(category.Code)
			if categoryCode == "" {
				continue
			}
			if supportTextMatchesBackfillIntent(service.Code, service.Name, category.Code, category.Name) {
				candidates = append(candidates, supportCategoryCandidate{
					serviceCode:  serviceCode,
					categoryCode: categoryCode,
				})
			}
		}
	}
	return candidates
}

func supportTextMatchesBackfillIntent(values ...string) bool {
	combined := strings.ToLower(strings.Join(values, " "))
	combined = strings.ReplaceAll(combined, "&", " and ")
	combined = strings.Join(strings.Fields(combined), " ")
	return strings.Contains(combined, "cost and usage report") ||
		strings.Contains(combined, "cost usage report") ||
		containsWordToken(combined, "cur")
}

func containsWordToken(value string, token string) bool {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, field := range fields {
		if field == token {
			return true
		}
	}
	return false
}

func selectLowSeverity(ctx context.Context, client Client, language string) (SupportSeverity, error) {
	levels, err := client.DescribeSeverityLevels(ctx, DescribeSeverityLevelsRequest{Language: language})
	if err != nil {
		return SupportSeverity{}, err
	}
	for _, level := range levels {
		code := strings.TrimSpace(level.Code)
		if code == "" {
			continue
		}
		if strings.EqualFold(code, "low") {
			level.Code = code
			return level, nil
		}
	}
	return SupportSeverity{}, NewProviderError("aws_support_low_severity_unavailable", "AWS Support low severity is unavailable for this account.")
}
