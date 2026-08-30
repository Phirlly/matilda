package billingcur2setup

import (
	"context"
	"fmt"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	dataExportsRegion       = "us-east-1"
	cur2TableName           = "COST_AND_USAGE_REPORT"
	matildaBillingPrefix    = "matilda/rapid-assessment/billing"
	maxBucketNameCandidates = 10
)

type Client interface {
	CheckConfiguration(context.Context) (cur2preflight.Configuration, error)
	GetCallerIdentity(context.Context) (cur2preflight.Identity, error)
	DescribeOrganization(context.Context) (Organization, error)
	ListExports(context.Context, string) (cur2preflight.ExportPage, error)
	GetExport(context.Context, string) (cur2preflight.Export, error)
	ListBuckets(context.Context, ListBucketsRequest) (BucketPage, error)
	HeadBucket(context.Context, HeadBucketRequest) (cur2preflight.BucketAccess, error)
	CreateBucket(context.Context, CreateBucketRequest) error
	GetBucketPolicy(context.Context, BucketPolicyRequest) (string, error)
	PutBucketPolicy(context.Context, PutBucketPolicyRequest) error
	CreateExport(context.Context, CreateExportRequest) (CreateExportResult, error)
}

type Organization struct {
	Available           bool
	ManagementAccountID string
}

type HeadBucketRequest struct {
	Bucket        string
	Region        string
	ExpectedOwner string
}

type ListBucketsRequest struct {
	Region string
	Prefix string
	Token  string
	Limit  int32
}

type BucketSummary struct {
	Name   string
	Region string
}

type BucketPage struct {
	Buckets   []BucketSummary
	NextToken string
}

type CreateBucketRequest struct {
	Bucket string
	Region string
}

type BucketPolicyRequest struct {
	Bucket        string
	ExpectedOwner string
}

type PutBucketPolicyRequest struct {
	Bucket        string
	ExpectedOwner string
	Policy        string
}

type CreateExportRequest struct {
	Name                string
	QueryStatement      string
	TableConfigurations map[string]map[string]string
	Destination         cur2preflight.S3Destination
	RefreshCadence      string
	DataExportsRegion   string
}

type CreateExportResult struct {
	ExportARN string
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

type setupFacts struct {
	BucketName      string
	BucketOwner     string
	BucketRef       string
	DestinationMode workflow.AWSCUR2DestinationMode
	ExportName      string
	Prefix          string
	CandidateIndex  string
}

type identityContext struct {
	AccountID string
	Partition string
}

type setupPlan struct {
	Facts            setupFacts
	Identity         identityContext
	Region           string
	Coverage         coverageResult
	BucketExists     bool
	PolicyNeedsMerge bool
	Policy           string
	ManagedExport    *cur2preflight.Export
	BucketCandidates []setupFacts
	Steps            []plannedStep
}

type plannedStep struct {
	ID     string
	Intent string
}
