package cur2preflight

import (
	"context"
	"fmt"
	"time"
)

type Client interface {
	CheckConfiguration(context.Context) (Configuration, error)
	GetCallerIdentity(context.Context) (Identity, error)
	ListTables(context.Context, string) (TablePage, error)
	GetTable(context.Context, string, map[string]string) (Table, error)
	ListExports(context.Context, string) (ExportPage, error)
	GetExport(context.Context, string) (Export, error)
	HeadBucket(context.Context, string) (BucketAccess, error)
	GetBucketPolicy(context.Context, string) (string, error)
	ListExecutions(context.Context, string, string) (ExecutionPage, error)
	GetExecution(context.Context, string, string) (Execution, error)
	ListObjects(context.Context, string, string, string, int32) (ObjectPage, error)
}

type Configuration struct {
	Region string
}

type Identity struct {
	AccountID string
	CallerARN string
}

type TableSummary struct {
	Name string
}

type TablePage struct {
	Tables    []TableSummary
	NextToken string
}

type Table struct {
	Name       string
	Columns    []string
	Properties map[string]string
}

type ExportSummary struct {
	Name       string
	ExportARN  string
	TableName  string
	SourceType string
}

type ExportPage struct {
	Exports   []ExportSummary
	NextToken string
}

type Export struct {
	Name                string
	ExportARN           string
	SourceARN           string
	SourceAccount       string
	QueryStatement      string
	TableConfigurations map[string]map[string]string
	Destination         S3Destination
	RefreshCadence      string
	CreatedAt           time.Time
	HealthStatus        string
}

type S3Destination struct {
	Bucket string
	Prefix string
	Region string
	Output S3Output
}

type S3Output struct {
	Format      string
	Compression string
	Overwrite   string
	OutputType  string
}

type BucketAccess struct {
	Accessible bool
	StatusCode int
	Region     string
}

type Execution struct {
	ID               string
	Status           string
	StatusObservedAt time.Time
}

type ExecutionPage struct {
	Executions []Execution
	NextToken  string
}

type ObjectPage struct {
	Keys      []string
	NextToken string
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
