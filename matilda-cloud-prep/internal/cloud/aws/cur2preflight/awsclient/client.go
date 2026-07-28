package awsclient

import (
	"context"
	"os"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsbcm "github.com/aws/aws-sdk-go-v2/service/bcmdataexports"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
)

const dataExportsRegion = "us-east-1"

type Config struct {
	Profile       string
	Region        string
	EnvLookup     func(string) (string, bool)
	LoadConfig    func(context.Context, LoadRequest) (aws.Config, error)
	ClientFactory ClientFactory
}

type LoadRequest struct {
	Profile string
	Region  string
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
	loadConfig func(context.Context, LoadRequest) (aws.Config, error)
	factory    ClientFactory
	profile    string
	region     string
	envLookup  func(string) (string, bool)
	configured bool
	config     cur2preflight.Configuration
	sts        stsAPI
	data       dataExportsAPI
	s3         s3API
}

func New(config Config) *Client {
	loader := config.LoadConfig
	if loader == nil {
		loader = func(ctx context.Context, request LoadRequest) (aws.Config, error) {
			options := []func(*awsconfig.LoadOptions) error{}
			if request.Profile != "" {
				options = append(options, awsconfig.WithSharedConfigProfile(request.Profile))
			}
			if request.Region != "" {
				options = append(options, awsconfig.WithRegion(request.Region))
			}
			return awsconfig.LoadDefaultConfig(ctx, options...)
		}
	}
	factory := config.ClientFactory
	if factory == nil {
		factory = defaultFactory{}
	}
	envLookup := config.EnvLookup
	if envLookup == nil {
		envLookup = os.LookupEnv
	}
	return &Client{
		loadConfig: loader,
		factory:    factory,
		profile:    config.Profile,
		region:     config.Region,
		envLookup:  envLookup,
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

	if client.profile != "" && client.hasCredentialEnvironment() {
		return providerError("aws_config_profile_shadowed")
	}

	cfg, err := client.loadConfig(ctx, LoadRequest{
		Profile: client.profile,
		Region:  client.region,
	})
	if err != nil {
		return classifyConfigurationError(err)
	}
	if cfg.Region == "" {
		return providerError("aws_config_missing_region")
	}
	if cfg.Credentials == nil {
		return providerError("aws_config_missing_credentials")
	}
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		return classifyConfigurationError(err)
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

func (client *Client) hasCredentialEnvironment() bool {
	for _, name := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_ACCESS_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_WEB_IDENTITY_TOKEN_FILE",
	} {
		if _, ok := client.envLookup(name); ok {
			return true
		}
	}
	return false
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
