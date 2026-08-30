package awsclient

import (
	"context"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingcur2setup"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	cur2awsclient "github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight/awsclient"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsbcm "github.com/aws/aws-sdk-go-v2/service/bcmdataexports"
	bcmtypes "github.com/aws/aws-sdk-go-v2/service/bcmdataexports/types"
	awsorg "github.com/aws/aws-sdk-go-v2/service/organizations"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Config struct {
	Profile             string
	Region              string
	PreflightClient     cur2preflight.Client
	DataExportsClient   dataExportsAPI
	S3Client            s3API
	OrganizationsClient organizationsAPI
	LoadConfig          func(context.Context, LoadRequest) (aws.Config, error)
	ClientFactory       ClientFactory
}

type LoadRequest struct {
	Profile string
	Region  string
}

type ClientFactory interface {
	DataExportsClient(aws.Config) dataExportsAPI
	S3Client(aws.Config) s3API
	OrganizationsClient(aws.Config) organizationsAPI
}

type dataExportsAPI interface {
	CreateExport(context.Context, *awsbcm.CreateExportInput, ...func(*awsbcm.Options)) (*awsbcm.CreateExportOutput, error)
}

type s3API interface {
	ListBuckets(context.Context, *awss3.ListBucketsInput, ...func(*awss3.Options)) (*awss3.ListBucketsOutput, error)
	HeadBucket(context.Context, *awss3.HeadBucketInput, ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error)
	CreateBucket(context.Context, *awss3.CreateBucketInput, ...func(*awss3.Options)) (*awss3.CreateBucketOutput, error)
	GetBucketPolicy(context.Context, *awss3.GetBucketPolicyInput, ...func(*awss3.Options)) (*awss3.GetBucketPolicyOutput, error)
	PutBucketPolicy(context.Context, *awss3.PutBucketPolicyInput, ...func(*awss3.Options)) (*awss3.PutBucketPolicyOutput, error)
}

type organizationsAPI interface {
	DescribeOrganization(context.Context, *awsorg.DescribeOrganizationInput, ...func(*awsorg.Options)) (*awsorg.DescribeOrganizationOutput, error)
}

type Client struct {
	profile       string
	region        string
	preflight     cur2preflight.Client
	data          dataExportsAPI
	s3            s3API
	organizations organizationsAPI
	loader        func(context.Context, LoadRequest) (aws.Config, error)
	factory       ClientFactory
}

func New(config Config) *Client {
	loader := config.LoadConfig
	if loader == nil {
		loader = func(ctx context.Context, request LoadRequest) (aws.Config, error) {
			options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(request.Region)}
			if request.Profile != "" {
				options = append(options, awsconfig.WithSharedConfigProfile(request.Profile))
			}
			return awsconfig.LoadDefaultConfig(ctx, options...)
		}
	}
	factory := config.ClientFactory
	if factory == nil {
		factory = defaultFactory{}
	}
	preflight := config.PreflightClient
	if preflight == nil {
		preflight = cur2awsclient.New(cur2awsclient.Config{
			Profile: config.Profile,
			Region:  config.Region,
		})
	}
	return &Client{
		profile:       config.Profile,
		region:        config.Region,
		preflight:     preflight,
		data:          config.DataExportsClient,
		s3:            config.S3Client,
		organizations: config.OrganizationsClient,
		loader:        loader,
		factory:       factory,
	}
}

func (client *Client) CheckConfiguration(ctx context.Context) (cur2preflight.Configuration, error) {
	return client.preflight.CheckConfiguration(ctx)
}

func (client *Client) GetCallerIdentity(ctx context.Context) (cur2preflight.Identity, error) {
	return client.preflight.GetCallerIdentity(ctx)
}

func (client *Client) ListExports(ctx context.Context, token string) (cur2preflight.ExportPage, error) {
	return client.preflight.ListExports(ctx, token)
}

func (client *Client) GetExport(ctx context.Context, exportARN string) (cur2preflight.Export, error) {
	return client.preflight.GetExport(ctx, exportARN)
}

func (client *Client) DescribeOrganization(ctx context.Context) (billingcur2setup.Organization, error) {
	if err := client.ensureOrganizations(ctx); err != nil {
		return billingcur2setup.Organization{}, err
	}
	output, err := client.organizations.DescribeOrganization(ctx, &awsorg.DescribeOrganizationInput{})
	if err != nil {
		return billingcur2setup.Organization{}, classifyOrganizationsError(err)
	}
	if output == nil || output.Organization == nil {
		return billingcur2setup.Organization{}, billingcur2setup.NewProviderError("aws_organizations_unavailable", "AWS Organizations response did not include organization details.")
	}
	return billingcur2setup.Organization{
		Available:           true,
		ManagementAccountID: aws.ToString(output.Organization.MasterAccountId),
	}, nil
}

func (client *Client) GetTable(ctx context.Context, name string, properties map[string]string) (cur2preflight.Table, error) {
	return client.preflight.GetTable(ctx, name, properties)
}

func (client *Client) ListBuckets(ctx context.Context, request billingcur2setup.ListBucketsRequest) (billingcur2setup.BucketPage, error) {
	if err := client.ensureS3(ctx, request.Region); err != nil {
		return billingcur2setup.BucketPage{}, err
	}
	input := &awss3.ListBucketsInput{}
	if request.Region != "" {
		input.BucketRegion = aws.String(request.Region)
	}
	if request.Prefix != "" {
		input.Prefix = aws.String(request.Prefix)
	}
	if request.Token != "" {
		input.ContinuationToken = aws.String(request.Token)
	}
	if request.Limit > 0 {
		input.MaxBuckets = aws.Int32(request.Limit)
	}
	output, err := client.s3.ListBuckets(ctx, input)
	if err != nil {
		return billingcur2setup.BucketPage{}, classifyS3Error(err, "aws_s3_list_buckets_failed")
	}
	if output == nil {
		return billingcur2setup.BucketPage{}, billingcur2setup.NewProviderError("aws_s3_list_buckets_failed", "AWS S3 ListBuckets response was empty.")
	}
	page := billingcur2setup.BucketPage{NextToken: aws.ToString(output.ContinuationToken)}
	for _, bucket := range output.Buckets {
		name := aws.ToString(bucket.Name)
		if name == "" {
			continue
		}
		page.Buckets = append(page.Buckets, billingcur2setup.BucketSummary{
			Name:   name,
			Region: aws.ToString(bucket.BucketRegion),
		})
	}
	return page, nil
}

func (client *Client) HeadBucket(ctx context.Context, request billingcur2setup.HeadBucketRequest) (cur2preflight.BucketAccess, error) {
	if err := client.ensureS3(ctx, request.Region); err != nil {
		return cur2preflight.BucketAccess{}, err
	}
	output, err := client.s3.HeadBucket(ctx, &awss3.HeadBucketInput{
		Bucket:              aws.String(request.Bucket),
		ExpectedBucketOwner: aws.String(request.ExpectedOwner),
	})
	if err != nil {
		if code := apiErrorCode(err); isNoSuchBucket(code) {
			return cur2preflight.BucketAccess{}, billingcur2setup.NewProviderError("aws_s3_bucket_not_found", "AWS S3 bucket does not exist.")
		}
		if status := statusCode(err); status != 0 {
			if status == 404 {
				// S3 HeadBucket often returns only status for a missing bucket; the setup runner still gates creation behind plan-bound approval.
				return cur2preflight.BucketAccess{}, billingcur2setup.NewProviderError("aws_s3_bucket_not_found", "AWS S3 bucket does not exist.")
			}
			return cur2preflight.BucketAccess{Accessible: false, StatusCode: status}, nil
		}
		return cur2preflight.BucketAccess{}, classifyS3Error(err, "aws_s3_bucket_inaccessible")
	}
	if output == nil {
		return cur2preflight.BucketAccess{}, billingcur2setup.NewProviderError("aws_s3_bucket_inaccessible", "AWS S3 HeadBucket response was empty.")
	}
	return cur2preflight.BucketAccess{
		Accessible: true,
		StatusCode: 200,
		Region:     aws.ToString(output.BucketRegion),
	}, nil
}

func (client *Client) CreateBucket(ctx context.Context, request billingcur2setup.CreateBucketRequest) error {
	if err := client.ensureS3(ctx, request.Region); err != nil {
		return err
	}
	input := &awss3.CreateBucketInput{
		Bucket: aws.String(request.Bucket),
	}
	if request.Region != "" && request.Region != "us-east-1" {
		input.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(request.Region),
		}
	}
	output, err := client.s3.CreateBucket(ctx, input)
	if err != nil {
		return classifyS3Error(err, "aws_s3_create_bucket_failed")
	}
	if output == nil {
		return billingcur2setup.NewProviderError("aws_s3_create_bucket_failed", "AWS S3 CreateBucket response was empty.")
	}
	return nil
}

func (client *Client) GetBucketPolicy(ctx context.Context, request billingcur2setup.BucketPolicyRequest) (string, error) {
	if err := client.ensureS3(ctx, ""); err != nil {
		return "", err
	}
	output, err := client.s3.GetBucketPolicy(ctx, &awss3.GetBucketPolicyInput{
		Bucket:              aws.String(request.Bucket),
		ExpectedBucketOwner: aws.String(request.ExpectedOwner),
	})
	if err != nil {
		if isNoBucketPolicy(err) {
			return "", nil
		}
		return "", classifyS3Error(err, "aws_s3_bucket_policy_inaccessible")
	}
	if output == nil {
		return "", billingcur2setup.NewProviderError("aws_s3_bucket_policy_inaccessible", "AWS S3 GetBucketPolicy response was empty.")
	}
	return aws.ToString(output.Policy), nil
}

func (client *Client) PutBucketPolicy(ctx context.Context, request billingcur2setup.PutBucketPolicyRequest) error {
	if err := client.ensureS3(ctx, ""); err != nil {
		return err
	}
	output, err := client.s3.PutBucketPolicy(ctx, &awss3.PutBucketPolicyInput{
		Bucket:              aws.String(request.Bucket),
		ExpectedBucketOwner: aws.String(request.ExpectedOwner),
		Policy:              aws.String(request.Policy),
	})
	if err != nil {
		return classifyS3Error(err, "aws_s3_put_bucket_policy_failed")
	}
	if output == nil {
		return billingcur2setup.NewProviderError("aws_s3_put_bucket_policy_failed", "AWS S3 PutBucketPolicy response was empty.")
	}
	return nil
}

func (client *Client) CreateExport(ctx context.Context, request billingcur2setup.CreateExportRequest) (billingcur2setup.CreateExportResult, error) {
	region := request.DataExportsRegion
	if region == "" {
		region = "us-east-1"
	}
	if err := client.ensureData(ctx, region); err != nil {
		return billingcur2setup.CreateExportResult{}, err
	}
	output, err := client.data.CreateExport(ctx, &awsbcm.CreateExportInput{
		Export: &bcmtypes.Export{
			Name: aws.String(request.Name),
			DataQuery: &bcmtypes.DataQuery{
				QueryStatement:      aws.String(request.QueryStatement),
				TableConfigurations: copyNestedStringMap(request.TableConfigurations),
			},
			DestinationConfigurations: &bcmtypes.DestinationConfigurations{
				S3Destination: &bcmtypes.S3Destination{
					S3Bucket:      aws.String(request.Destination.Bucket),
					S3BucketOwner: aws.String(request.Destination.BucketOwner),
					S3Prefix:      aws.String(request.Destination.Prefix),
					S3Region:      aws.String(request.Destination.Region),
					S3OutputConfigurations: &bcmtypes.S3OutputConfigurations{
						Format:      bcmtypes.FormatOption(request.Destination.Output.Format),
						Compression: bcmtypes.CompressionOption(request.Destination.Output.Compression),
						Overwrite:   bcmtypes.OverwriteOption(request.Destination.Output.Overwrite),
						OutputType:  bcmtypes.S3OutputType(request.Destination.Output.OutputType),
					},
				},
			},
			RefreshCadence: &bcmtypes.RefreshCadence{
				Frequency: bcmtypes.FrequencyOption(request.RefreshCadence),
			},
		},
	})
	if err != nil {
		return billingcur2setup.CreateExportResult{}, classifyDataExportsError(err, "aws_cur2_create_export_failed")
	}
	if output == nil {
		return billingcur2setup.CreateExportResult{}, billingcur2setup.NewProviderError("aws_cur2_create_export_failed", "AWS Data Exports CreateExport response was empty.")
	}
	return billingcur2setup.CreateExportResult{ExportARN: aws.ToString(output.ExportArn)}, nil
}

func (client *Client) ensureData(ctx context.Context, region string) error {
	if client.data != nil {
		return nil
	}
	config, err := client.loadConfig(ctx, region)
	if err != nil {
		return err
	}
	config.Region = region
	client.data = client.factory.DataExportsClient(config)
	return nil
}

func (client *Client) ensureS3(ctx context.Context, region string) error {
	if client.s3 != nil {
		return nil
	}
	if region == "" {
		region = client.region
	}
	config, err := client.loadConfig(ctx, region)
	if err != nil {
		return err
	}
	client.s3 = client.factory.S3Client(config)
	return nil
}

func (client *Client) ensureOrganizations(ctx context.Context) error {
	if client.organizations != nil {
		return nil
	}
	region := client.region
	if region == "" {
		region = "us-east-1"
	}
	config, err := client.loadConfig(ctx, region)
	if err != nil {
		return err
	}
	client.organizations = client.factory.OrganizationsClient(config)
	return nil
}

func (client *Client) loadConfig(ctx context.Context, region string) (aws.Config, error) {
	config, err := client.loader(ctx, LoadRequest{
		Profile: client.profile,
		Region:  region,
	})
	if err != nil {
		return aws.Config{}, billingcur2setup.NewProviderError("aws_config_missing_credentials", "AWS SDK configuration could not be loaded.")
	}
	if config.Region == "" {
		config.Region = region
	}
	return config, nil
}

type defaultFactory struct{}

func (defaultFactory) DataExportsClient(config aws.Config) dataExportsAPI {
	return awsbcm.NewFromConfig(config, awsbcm.WithSigV4SigningRegion("us-east-1"))
}

func (defaultFactory) S3Client(config aws.Config) s3API {
	return awss3.NewFromConfig(config)
}

func (defaultFactory) OrganizationsClient(config aws.Config) organizationsAPI {
	return awsorg.NewFromConfig(config)
}

func copyNestedStringMap(values map[string]map[string]string) map[string]map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]map[string]string, len(values))
	for key, value := range values {
		copied[key] = copyStringMap(value)
	}
	return copied
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

var _ billingcur2setup.Client = (*Client)(nil)
