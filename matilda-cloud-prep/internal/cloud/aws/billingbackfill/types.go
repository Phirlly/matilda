package billingbackfill

import (
	"context"
	"fmt"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
)

type Client interface {
	cur2preflight.Client
	DescribeServices(context.Context, DescribeServicesRequest) ([]SupportService, error)
	DescribeSeverityLevels(context.Context, DescribeSeverityLevelsRequest) ([]SupportSeverity, error)
	DescribeCreateCaseOptions(context.Context, DescribeCreateCaseOptionsRequest) (SupportCreateCaseOptions, error)
	DescribeCases(context.Context, DescribeCasesRequest) ([]SupportCase, error)
	CreateCase(context.Context, CreateCaseRequest) (CreateCaseResult, error)
}

type DescribeServicesRequest struct {
	Language string
}

type SupportService struct {
	Code       string
	Name       string
	Categories []SupportCategory
}

type SupportCategory struct {
	Code string
	Name string
}

type DescribeSeverityLevelsRequest struct {
	Language string
}

type SupportSeverity struct {
	Code string
	Name string
}

type DescribeCreateCaseOptionsRequest struct {
	Language     string
	IssueType    string
	ServiceCode  string
	CategoryCode string
}

type SupportCreateCaseOptions struct {
	Available bool
}

type DescribeCasesRequest struct {
	CaseIDs                  []string
	IncludeResolved          bool
	IncludeCommunications    bool
	IncludeCommunicationsSet bool
	MaxResults               int32
}

type SupportCase struct {
	CaseID    string
	DisplayID string
	Subject   string
	Status    string
}

type CreateCaseRequest struct {
	Language     string
	IssueType    string
	ServiceCode  string
	CategoryCode string
	SeverityCode string
	Subject      string
	Body         string
}

type CreateCaseResult struct {
	CaseID string
}

type ProviderError struct {
	Code    string
	Message string
}

func NewProviderError(code string, message string) error {
	return ProviderError{Code: code, Message: message}
}

func (err ProviderError) Error() string {
	return fmt.Sprintf("%s: %s", err.Code, err.Message)
}
