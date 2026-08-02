package billingcur2setup

import (
	"context"
	"errors"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

type coverageResult struct {
	Status  workflow.CoverageStatus
	Summary string
}

func classifyCoverage(ctx context.Context, client Client, accountID string) coverageResult {
	organization, err := client.DescribeOrganization(ctx)
	if err != nil {
		var providerErr ProviderError
		if errors.As(err, &providerErr) && providerErr.Code == "aws_organizations_not_in_use" {
			return coverageResult{
				Status:  workflow.CoverageSingleAccount,
				Summary: "AWS Organizations is not in use for this account; CUR 2.0 billing coverage is single-account.",
			}
		}
		return coverageResult{
			Status:  workflow.CoverageUnverified,
			Summary: "AWS Organizations coverage could not be verified; the tool will not claim organization-wide billing coverage.",
		}
	}
	if !organization.Available || organization.ManagementAccountID == "" {
		return coverageResult{
			Status:  workflow.CoverageUnverified,
			Summary: "AWS Organizations returned incomplete coverage information; the tool will not claim organization-wide billing coverage.",
		}
	}
	if organization.ManagementAccountID == accountID {
		return coverageResult{
			Status:  workflow.CoverageOrganizationWide,
			Summary: "The connected AWS account is the organization management account, so CUR 2.0 can cover consolidated organization billing.",
		}
	}
	return coverageResult{
		Status:  workflow.CoverageAccountOnly,
		Summary: "The connected AWS account is an organization member account, so this CUR 2.0 export covers only that account.",
	}
}
