package awsclient

import (
	"context"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsbcm "github.com/aws/aws-sdk-go-v2/service/bcmdataexports"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
)

const dataExportsRegion = "us-east-1"

type Config struct {
	LoadConfig    func(context.Context) (aws.Config, error)
	ClientFactory ClientFactory
}

type ClientFactory interface {
	STSClient(aws.Config) stsAPI
	DataExportsClient(aws.Config) dataExportsAPI
	S3Client(aws.Config) s3API
}

type stsAPI interface {
	GetCallerIdentity(context.Context, *awssts.GetCallerIdentityInput, ...func(*awssts.Options)) (*awssts.GetCallerIdentityOutput, error)
}

type dataExportsAPI interface {
	ListTables(context.Context, *awsbcm.ListTablesInput, ...func(*awsbcm.Options)) (*awsbcm.ListTablesOutput, error)
	GetTable(context.Context, *awsbcm.GetTableInput, ...func(*awsbcm.Options)) (*awsbcm.GetTableOutput, error)
	ListExports(context.Context, *awsbcm.ListExportsInput, ...func(*awsbcm.Options)) (*awsbcm.ListExportsOutput, error)
	GetExport(context.Context, *awsbcm.GetExportInput, ...func(*awsbcm.Options)) (*awsbcm.GetExportOutput, error)
	ListExecutions(context.Context, *awsbcm.ListExecutionsInput, ...func(*awsbcm.Options)) (*awsbcm.ListExecutionsOutput, error)
	GetExecution(context.Context, *awsbcm.GetExecutionInput, ...func(*awsbcm.Options)) (*awsbcm.GetExecutionOutput, error)
}

type s3API interface {
	HeadBucket(context.Context, *awss3.HeadBucketInput, ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error)
	GetBucketPolicy(context.Context, *awss3.GetBucketPolicyInput, ...func(*awss3.Options)) (*awss3.GetBucketPolicyOutput, error)
	ListObjectsV2(context.Context, *awss3.ListObjectsV2Input, ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error)
}

type Client struct {
	loadConfig func(context.Context) (aws.Config, error)
	factory    ClientFactory
	configured bool
	config     cur2preflight.Configuration
	sts        stsAPI
	data       dataExportsAPI
	s3         s3API
}

func New(config Config) *Client {
	loader := config.LoadConfig
	if loader == nil {
		loader = func(ctx context.Context) (aws.Config, error) {
			return awsconfig.LoadDefaultConfig(ctx)
		}
	}
	factory := config.ClientFactory
	if factory == nil {
		factory = defaultFactory{}
	}
	return &Client{
		loadConfig: loader,
		factory:    factory,
	}
}

func NewDefault() *Client {
	return New(Config{})
}

func (client *Client) CheckConfiguration(ctx context.Context) (cur2preflight.Configuration, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.Configuration{}, err
	}
	return client.config, nil
}

func (client *Client) ensureConfigured(ctx context.Context) error {
	if client.configured {
		return nil
	}

	cfg, err := client.loadConfig(ctx)
	if err != nil {
		return providerError("aws_config_missing_credentials")
	}
	if cfg.Region == "" {
		return providerError("aws_config_missing_region")
	}
	if cfg.Credentials == nil {
		return providerError("aws_config_missing_credentials")
	}
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		return providerError("aws_config_missing_credentials")
	}

	dataCfg := cfg
	dataCfg.Region = dataExportsRegion

	client.config = cur2preflight.Configuration{Region: cfg.Region}
	client.sts = client.factory.STSClient(cfg)
	client.data = client.factory.DataExportsClient(dataCfg)
	client.s3 = client.factory.S3Client(cfg)
	client.configured = true
	return nil
}

type defaultFactory struct{}

func (defaultFactory) STSClient(config aws.Config) stsAPI {
	return awssts.NewFromConfig(config)
}

func (defaultFactory) DataExportsClient(config aws.Config) dataExportsAPI {
	return awsbcm.NewFromConfig(config, awsbcm.WithSigV4SigningRegion(dataExportsRegion))
}

func (defaultFactory) S3Client(config aws.Config) s3API {
	return awss3.NewFromConfig(config)
}

var _ cur2preflight.Client = (*Client)(nil)
