package billingbackfill

import (
	"context"
	"strings"
)

func findExistingOpenCase(ctx context.Context, client Client, reference string) (SupportCase, bool, error) {
	cases, err := client.DescribeCases(ctx, DescribeCasesRequest{
		IncludeResolved:          false,
		IncludeCommunications:    false,
		IncludeCommunicationsSet: true,
		MaxResults:               100,
	})
	if err != nil {
		return SupportCase{}, false, err
	}
	for _, supportCase := range cases {
		if strings.Contains(supportCase.Subject, reference) {
			if strings.TrimSpace(supportCase.CaseID) == "" && strings.TrimSpace(supportCase.DisplayID) == "" {
				return SupportCase{}, false, NewProviderError("aws_backfill_duplicate_check_failed", "Matching AWS Support case did not include a safe case reference.")
			}
			return supportCase, true, nil
		}
	}
	return SupportCase{}, false, nil
}
