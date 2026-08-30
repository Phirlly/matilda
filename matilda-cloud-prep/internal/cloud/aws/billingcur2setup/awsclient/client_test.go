package awsclient

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingcur2setup"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsbcm "github.com/aws/aws-sdk-go-v2/service/bcmdataexports"
	bcmtypes "github.com/aws/aws-sdk-go-v2/service/bcmdataexports/types"
	awsorg "github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestCreateExportMapsSetupRequestToSDK(t *testing.T) {
	data := &fakeDataExports{
		createExportOutput: &awsbcm.CreateExportOutput{ExportArn: aws.String("arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda")},
	}
	client := New(Config{DataExportsClient: data})

	result, err := client.CreateExport(context.Background(), sampleCreateExportRequest())
	if err != nil {
		t.Fatalf("CreateExport returned error: %v", err)
	}
	if result.ExportARN == "" {
		t.Fatal("ExportARN is empty")
	}
	input := data.createExportInput
	if input == nil || input.Export == nil {
		t.Fatalf("CreateExport input = %#v, want export", input)
	}
	if aws.ToString(input.Export.Name) != "matilda-cur2-ra-billing-abcdefghijkl-00" {
		t.Fatalf("Export.Name = %q, want mapped name", aws.ToString(input.Export.Name))
	}
	if aws.ToString(input.Export.DataQuery.QueryStatement) != sampleCreateExportRequest().QueryStatement {
		t.Fatalf("QueryStatement = %q, want mapped query", aws.ToString(input.Export.DataQuery.QueryStatement))
	}
	if input.Export.DataQuery.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"] != "MONTHLY" {
		t.Fatalf("TableConfigurations = %#v, want monthly", input.Export.DataQuery.TableConfigurations)
	}
	if input.Export.DataQuery.TableConfigurations["COST_AND_USAGE_REPORT"]["BILLING_VIEW_ARN"] != "arn:aws:billing::123456789012:billingview/primary" {
		t.Fatalf("TableConfigurations = %#v, want primary billing view ARN", input.Export.DataQuery.TableConfigurations)
	}
	destination := input.Export.DestinationConfigurations.S3Destination
	if aws.ToString(destination.S3Bucket) != "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00" ||
		aws.ToString(destination.S3Prefix) != "matilda/rapid-assessment/billing" ||
		aws.ToString(destination.S3Region) != "us-west-2" {
		t.Fatalf("S3Destination = %#v, want mapped bucket, prefix, region", destination)
	}
	if aws.ToString(destination.S3BucketOwner) != "123456789012" {
		t.Fatalf("S3BucketOwner = %q, want caller account", aws.ToString(destination.S3BucketOwner))
	}
	output := destination.S3OutputConfigurations
	if output.Format != bcmtypes.FormatOptionTextOrCsv ||
		output.Compression != bcmtypes.CompressionOptionGzip ||
		output.Overwrite != bcmtypes.OverwriteOptionCreateNewReport ||
		output.OutputType != bcmtypes.S3OutputTypeCustom {
		t.Fatalf("S3OutputConfigurations = %#v, want preferred CUR2 settings", output)
	}
	if input.Export.RefreshCadence.Frequency != bcmtypes.FrequencyOptionSynchronous {
		t.Fatalf("RefreshCadence = %#v, want synchronous", input.Export.RefreshCadence)
	}
}

func TestS3MethodsUseExpectedBucketOwnerAndRegion(t *testing.T) {
	s3 := &fakeS3{
		headBucketOutput: &awss3.HeadBucketOutput{BucketRegion: aws.String("us-west-2")},
	}
	client := New(Config{S3Client: s3})

	access, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{
		Bucket:        "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
		Region:        "us-west-2",
		ExpectedOwner: "123456789012",
	})
	if err != nil {
		t.Fatalf("HeadBucket returned error: %v", err)
	}
	if !access.Accessible || access.Region != "us-west-2" {
		t.Fatalf("BucketAccess = %#v, want accessible in us-west-2", access)
	}
	if aws.ToString(s3.headBucketInput.ExpectedBucketOwner) != "123456789012" {
		t.Fatalf("HeadBucket ExpectedBucketOwner = %q, want caller account", aws.ToString(s3.headBucketInput.ExpectedBucketOwner))
	}

	_, err = client.GetBucketPolicy(context.Background(), billingcur2setup.BucketPolicyRequest{
		Bucket:        "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
		ExpectedOwner: "123456789012",
	})
	if err != nil {
		t.Fatalf("GetBucketPolicy returned error: %v", err)
	}
	if aws.ToString(s3.getBucketPolicyInput.ExpectedBucketOwner) != "123456789012" {
		t.Fatalf("GetBucketPolicy ExpectedBucketOwner = %q, want caller account", aws.ToString(s3.getBucketPolicyInput.ExpectedBucketOwner))
	}

	err = client.PutBucketPolicy(context.Background(), billingcur2setup.PutBucketPolicyRequest{
		Bucket:        "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
		ExpectedOwner: "123456789012",
		Policy:        "{}",
	})
	if err != nil {
		t.Fatalf("PutBucketPolicy returned error: %v", err)
	}
	if aws.ToString(s3.putBucketPolicyInput.ExpectedBucketOwner) != "123456789012" {
		t.Fatalf("PutBucketPolicy ExpectedBucketOwner = %q, want caller account", aws.ToString(s3.putBucketPolicyInput.ExpectedBucketOwner))
	}

	err = client.CreateBucket(context.Background(), billingcur2setup.CreateBucketRequest{
		Bucket: "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
		Region: "us-west-2",
	})
	if err != nil {
		t.Fatalf("CreateBucket returned error: %v", err)
	}
	if s3.createBucketInput.CreateBucketConfiguration == nil ||
		s3.createBucketInput.CreateBucketConfiguration.LocationConstraint != s3types.BucketLocationConstraintUsWest2 {
		t.Fatalf("CreateBucketConfiguration = %#v, want us-west-2 location constraint", s3.createBucketInput.CreateBucketConfiguration)
	}
}

func TestListBucketsMapsOwnedBucketPage(t *testing.T) {
	s3 := &fakeS3{
		listBucketsOutput: &awss3.ListBucketsOutput{
			Buckets: []s3types.Bucket{
				{Name: aws.String("matilda-existing-cur2"), BucketRegion: aws.String("us-west-2")},
				{BucketRegion: aws.String("us-west-2")},
				{Name: aws.String("matilda-existing-cur2-logs"), BucketRegion: aws.String("us-west-2")},
			},
			ContinuationToken: aws.String("next-token"),
		},
	}
	client := New(Config{S3Client: s3})

	page, err := client.ListBuckets(context.Background(), billingcur2setup.ListBucketsRequest{
		Region: "us-west-2",
		Prefix: "matilda",
		Token:  "input-token",
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("ListBuckets returned error: %v", err)
	}
	if s3.listBucketsInput == nil {
		t.Fatal("ListBuckets input was not captured")
	}
	if aws.ToString(s3.listBucketsInput.BucketRegion) != "us-west-2" ||
		aws.ToString(s3.listBucketsInput.Prefix) != "matilda" ||
		aws.ToString(s3.listBucketsInput.ContinuationToken) != "input-token" ||
		aws.ToInt32(s3.listBucketsInput.MaxBuckets) != 1000 {
		t.Fatalf("ListBuckets input = %#v, want region, prefix, token, limit", s3.listBucketsInput)
	}
	if page.NextToken != "next-token" || len(page.Buckets) != 2 {
		t.Fatalf("Bucket page = %#v, want two buckets and next token", page)
	}
	if page.Buckets[0] != (billingcur2setup.BucketSummary{Name: "matilda-existing-cur2", Region: "us-west-2"}) {
		t.Fatalf("first bucket = %#v, want mapped bucket", page.Buckets[0])
	}
}

func TestListBucketsRejectsEmptyAWSResponse(t *testing.T) {
	s3 := &fakeS3{listBucketsNilOutput: true}
	client := New(Config{S3Client: s3})

	_, err := client.ListBuckets(context.Background(), billingcur2setup.ListBucketsRequest{Region: "us-west-2"})

	var providerErr billingcur2setup.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "aws_s3_list_buckets_failed" {
		t.Fatalf("ListBuckets error = %#v, want aws_s3_list_buckets_failed provider error", err)
	}
}

func TestDescribeOrganizationMapsManagementAccount(t *testing.T) {
	org := &fakeOrganizations{
		output: &awsorg.DescribeOrganizationOutput{
			Organization: &orgtypes.Organization{MasterAccountId: aws.String("123456789012")},
		},
	}
	client := New(Config{OrganizationsClient: org})

	result, err := client.DescribeOrganization(context.Background())
	if err != nil {
		t.Fatalf("DescribeOrganization returned error: %v", err)
	}
	if !result.Available || result.ManagementAccountID != "123456789012" {
		t.Fatalf("Organization = %#v, want mapped management account", result)
	}
}

func TestDescribeOrganizationClassifiesExpectedProviderErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "not in organization",
			err:  &smithy.GenericAPIError{Code: "AWSOrganizationsNotInUseException", Message: "not in org"},
			want: "aws_organizations_not_in_use",
		},
		{
			name: "access denied",
			err:  &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"},
			want: "aws_organizations_access_denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(Config{OrganizationsClient: &fakeOrganizations{err: tt.err}})

			_, err := client.DescribeOrganization(context.Background())
			if err == nil {
				t.Fatal("DescribeOrganization returned nil error")
			}
			var providerErr billingcur2setup.ProviderError
			if !errors.As(err, &providerErr) || providerErr.Code != tt.want {
				t.Fatalf("error = %#v, want %s ProviderError", err, tt.want)
			}
		})
	}
}

func TestSetupClientClassifiesMutationErrors(t *testing.T) {
	apiErr := &smithy.GenericAPIError{Code: "LimitExceededException", Message: "quota"}
	client := New(Config{
		DataExportsClient: &fakeDataExports{createExportErr: apiErr},
		S3Client:          &fakeS3{putBucketPolicyErr: &smithy.GenericAPIError{Code: "AccessDenied", Message: "denied"}},
	})

	_, err := client.CreateExport(context.Background(), sampleCreateExportRequest())
	assertProviderCode(t, err, "aws_cur2_export_quota_full")

	err = client.PutBucketPolicy(context.Background(), billingcur2setup.PutBucketPolicyRequest{
		Bucket:        "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
		ExpectedOwner: "123456789012",
		Policy:        "{}",
	})
	assertProviderCode(t, err, "aws_s3_put_bucket_policy_failed")
}

func TestSetupClientNilSuccessResponsesReturnProviderErrors(t *testing.T) {
	t.Run("create export", func(t *testing.T) {
		client := New(Config{DataExportsClient: &fakeDataExports{createExportNilOutput: true}})

		_, err := client.CreateExport(context.Background(), sampleCreateExportRequest())

		assertProviderCode(t, err, "aws_cur2_create_export_failed")
	})

	t.Run("head bucket", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{headBucketNilOutput: true}})

		_, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{
			Bucket:        "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
			Region:        "us-west-2",
			ExpectedOwner: "123456789012",
		})

		assertProviderCode(t, err, "aws_s3_bucket_inaccessible")
	})

	t.Run("create bucket", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{createBucketNilOutput: true}})

		err := client.CreateBucket(context.Background(), billingcur2setup.CreateBucketRequest{
			Bucket: "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
			Region: "us-west-2",
		})

		assertProviderCode(t, err, "aws_s3_create_bucket_failed")
	})

	t.Run("put bucket policy", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{putBucketPolicyNilOutput: true}})

		err := client.PutBucketPolicy(context.Background(), billingcur2setup.PutBucketPolicyRequest{
			Bucket:        "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
			ExpectedOwner: "123456789012",
			Policy:        "{}",
		})

		assertProviderCode(t, err, "aws_s3_put_bucket_policy_failed")
	})
}

func TestPreflightMethodsDelegateToReadOnlyClient(t *testing.T) {
	preflight := &fakePreflight{
		config: cur2preflight.Configuration{Region: "us-west-2"},
		identity: cur2preflight.Identity{
			AccountID: "123456789012",
			CallerARN: "arn:aws:iam::123456789012:role/operator",
		},
		export: cur2preflight.Export{
			Name:      "existing",
			ExportARN: "arn:aws:bcm-data-exports:us-east-1:123456789012:export/existing",
		},
	}
	client := New(Config{PreflightClient: preflight})

	config, err := client.CheckConfiguration(context.Background())
	if err != nil || config.Region != "us-west-2" {
		t.Fatalf("CheckConfiguration = %#v err=%v, want delegated config", config, err)
	}
	identity, err := client.GetCallerIdentity(context.Background())
	if err != nil || identity.AccountID != "123456789012" {
		t.Fatalf("GetCallerIdentity = %#v err=%v, want delegated identity", identity, err)
	}
	page, err := client.ListExports(context.Background(), "token")
	if err != nil || page.NextToken != "next" || len(page.Exports) != 1 {
		t.Fatalf("ListExports = %#v err=%v, want delegated page", page, err)
	}
	export, err := client.GetExport(context.Background(), preflight.export.ExportARN)
	if err != nil || export.Name != "existing" {
		t.Fatalf("GetExport = %#v err=%v, want delegated export", export, err)
	}
	if preflight.listExportsToken != "token" {
		t.Fatalf("delegated list token = %q, want token", preflight.listExportsToken)
	}
}

func TestNewPreservesInjectedConfiguration(t *testing.T) {
	preflight := &fakePreflight{}
	data := &fakeDataExports{}
	s3 := &fakeS3{}
	organizations := &fakeOrganizations{}
	factory := &fakeFactory{}
	loader := func(context.Context, LoadRequest) (aws.Config, error) {
		return aws.Config{Region: "us-west-2"}, nil
	}

	client := New(Config{
		Profile:             "default",
		Region:              "us-west-2",
		PreflightClient:     preflight,
		DataExportsClient:   data,
		S3Client:            s3,
		OrganizationsClient: organizations,
		LoadConfig:          loader,
		ClientFactory:       factory,
	})

	if client.profile != "default" || client.region != "us-west-2" {
		t.Fatalf("client profile/region = %q/%q, want default/us-west-2", client.profile, client.region)
	}
	if client.preflight != preflight || client.data != data || client.s3 != s3 || client.organizations != organizations || client.factory != factory {
		t.Fatalf("client did not preserve injected dependencies: %#v", client)
	}
}

func TestConfiguredFactoryBuildsSDKClientsWithExpectedRegions(t *testing.T) {
	factory := &fakeFactory{
		data:          &fakeDataExports{},
		s3:            &fakeS3{headBucketOutput: &awss3.HeadBucketOutput{BucketRegion: aws.String("us-west-2")}},
		organizations: &fakeOrganizations{output: &awsorg.DescribeOrganizationOutput{Organization: &orgtypes.Organization{MasterAccountId: aws.String("123456789012")}}},
	}
	var loadRequests []LoadRequest
	client := New(Config{
		Profile: "default",
		Region:  "us-west-2",
		LoadConfig: func(ctx context.Context, request LoadRequest) (aws.Config, error) {
			loadRequests = append(loadRequests, request)
			return aws.Config{Region: request.Region}, nil
		},
		ClientFactory: factory,
	})

	if _, err := client.CreateExport(context.Background(), sampleCreateExportRequest()); err != nil {
		t.Fatalf("CreateExport returned error: %v", err)
	}
	if _, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{Bucket: "bucket", Region: "us-west-2", ExpectedOwner: "123456789012"}); err != nil {
		t.Fatalf("HeadBucket returned error: %v", err)
	}
	if _, err := client.DescribeOrganization(context.Background()); err != nil {
		t.Fatalf("DescribeOrganization returned error: %v", err)
	}

	if len(loadRequests) != 3 {
		t.Fatalf("load requests length = %d, want 3", len(loadRequests))
	}
	if loadRequests[0].Region != "us-east-1" {
		t.Fatalf("data exports load region = %q, want us-east-1", loadRequests[0].Region)
	}
	if loadRequests[1].Region != "us-west-2" {
		t.Fatalf("s3 load region = %q, want us-west-2", loadRequests[1].Region)
	}
	if loadRequests[2].Region != "us-west-2" {
		t.Fatalf("organizations load region = %q, want configured region", loadRequests[2].Region)
	}
	if factory.dataRegion != "us-east-1" || factory.s3Region != "us-west-2" || factory.organizationsRegion != "us-west-2" {
		t.Fatalf("factory regions = data %q s3 %q org %q, want us-east-1/us-west-2/us-west-2", factory.dataRegion, factory.s3Region, factory.organizationsRegion)
	}
}

func TestConfiguredFactoryUsesSafeRegionFallbacks(t *testing.T) {
	t.Run("s3 uses selected request region when loaded config has no region", func(t *testing.T) {
		factory := &fakeFactory{s3: &fakeS3{headBucketOutput: &awss3.HeadBucketOutput{BucketRegion: aws.String("us-west-2")}}}
		client := New(Config{
			Profile: "default",
			LoadConfig: func(ctx context.Context, request LoadRequest) (aws.Config, error) {
				if request.Profile != "default" || request.Region != "us-west-2" {
					t.Fatalf("LoadRequest = %#v, want profile default and us-west-2", request)
				}
				return aws.Config{}, nil
			},
			ClientFactory: factory,
		})

		if _, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{Bucket: "bucket", Region: "us-west-2"}); err != nil {
			t.Fatalf("HeadBucket returned error: %v", err)
		}
		if factory.s3Region != "us-west-2" {
			t.Fatalf("S3 factory region = %q, want us-west-2", factory.s3Region)
		}
	})
	t.Run("policy methods reuse configured s3 region when operation region is not part of the request", func(t *testing.T) {
		factory := &fakeFactory{s3: &fakeS3{}}
		client := New(Config{
			Region: "us-west-2",
			LoadConfig: func(ctx context.Context, request LoadRequest) (aws.Config, error) {
				return aws.Config{Region: request.Region}, nil
			},
			ClientFactory: factory,
		})

		if _, err := client.GetBucketPolicy(context.Background(), billingcur2setup.BucketPolicyRequest{Bucket: "bucket"}); err != nil {
			t.Fatalf("GetBucketPolicy returned error: %v", err)
		}
		if err := client.PutBucketPolicy(context.Background(), billingcur2setup.PutBucketPolicyRequest{Bucket: "bucket", Policy: "{}"}); err != nil {
			t.Fatalf("PutBucketPolicy returned error: %v", err)
		}
		if factory.s3Region != "us-west-2" {
			t.Fatalf("S3 factory region = %q, want configured region", factory.s3Region)
		}
	})
	t.Run("organizations uses us-east-1 when no configured region exists", func(t *testing.T) {
		factory := &fakeFactory{
			organizations: &fakeOrganizations{output: &awsorg.DescribeOrganizationOutput{Organization: &orgtypes.Organization{MasterAccountId: aws.String("123456789012")}}},
		}
		client := New(Config{
			LoadConfig: func(ctx context.Context, request LoadRequest) (aws.Config, error) {
				return aws.Config{}, nil
			},
			ClientFactory: factory,
		})

		if _, err := client.DescribeOrganization(context.Background()); err != nil {
			t.Fatalf("DescribeOrganization returned error: %v", err)
		}
		if factory.organizationsRegion != "us-east-1" {
			t.Fatalf("Organizations factory region = %q, want us-east-1", factory.organizationsRegion)
		}
	})
}

func TestSetupClientHandlesAlternateSDKResponsesAndErrors(t *testing.T) {
	t.Run("empty organization response", func(t *testing.T) {
		client := New(Config{OrganizationsClient: &fakeOrganizations{output: &awsorg.DescribeOrganizationOutput{}}})
		_, err := client.DescribeOrganization(context.Background())
		assertProviderCode(t, err, "aws_organizations_unavailable")
	})
	t.Run("no bucket policy is mergeable empty policy", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{getBucketPolicyErr: &smithy.GenericAPIError{Code: "NoSuchBucketPolicy", Message: "none"}}})
		policy, err := client.GetBucketPolicy(context.Background(), billingcur2setup.BucketPolicyRequest{Bucket: "bucket", ExpectedOwner: "123456789012"})
		if err != nil || policy != "" {
			t.Fatalf("GetBucketPolicy = %q err=%v, want empty nil for missing policy", policy, err)
		}
	})
	t.Run("create bucket in us-east-1 omits location constraint", func(t *testing.T) {
		s3 := &fakeS3{}
		client := New(Config{S3Client: s3})
		if err := client.CreateBucket(context.Background(), billingcur2setup.CreateBucketRequest{Bucket: "bucket", Region: "us-east-1"}); err != nil {
			t.Fatalf("CreateBucket returned error: %v", err)
		}
		if s3.createBucketInput.CreateBucketConfiguration != nil {
			t.Fatalf("CreateBucketConfiguration = %#v, want nil for us-east-1", s3.createBucketInput.CreateBucketConfiguration)
		}
	})
	t.Run("load config failure is classified", func(t *testing.T) {
		client := New(Config{
			LoadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
				return aws.Config{}, errors.New("load failed")
			},
			ClientFactory: &fakeFactory{data: &fakeDataExports{}},
		})
		_, err := client.CreateExport(context.Background(), sampleCreateExportRequest())
		assertProviderCode(t, err, "aws_config_missing_credentials")
	})
	t.Run("create export defaults to us-east-1 data exports region", func(t *testing.T) {
		data := &fakeDataExports{createExportOutput: &awsbcm.CreateExportOutput{ExportArn: aws.String("arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda")}}
		client := New(Config{DataExportsClient: data})
		request := sampleCreateExportRequest()
		request.DataExportsRegion = ""

		if _, err := client.CreateExport(context.Background(), request); err != nil {
			t.Fatalf("CreateExport returned error: %v", err)
		}
		if data.createExportInput == nil {
			t.Fatal("CreateExport input was not captured")
		}
	})
}

func TestSetupClientPropagatesConfigLoadFailuresByOperation(t *testing.T) {
	newClient := func() *Client {
		return New(Config{
			LoadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
				return aws.Config{}, errors.New("load failed")
			},
			ClientFactory: &fakeFactory{},
		})
	}
	tests := []struct {
		name string
		run  func(*Client) error
	}{
		{
			name: "describe organization",
			run: func(client *Client) error {
				_, err := client.DescribeOrganization(context.Background())
				return err
			},
		},
		{
			name: "head bucket",
			run: func(client *Client) error {
				_, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{Bucket: "bucket", Region: "us-west-2"})
				return err
			},
		},
		{
			name: "create bucket",
			run: func(client *Client) error {
				return client.CreateBucket(context.Background(), billingcur2setup.CreateBucketRequest{Bucket: "bucket", Region: "us-west-2"})
			},
		},
		{
			name: "get bucket policy",
			run: func(client *Client) error {
				_, err := client.GetBucketPolicy(context.Background(), billingcur2setup.BucketPolicyRequest{Bucket: "bucket"})
				return err
			},
		},
		{
			name: "put bucket policy",
			run: func(client *Client) error {
				return client.PutBucketPolicy(context.Background(), billingcur2setup.PutBucketPolicyRequest{Bucket: "bucket", Policy: "{}"})
			},
		},
		{
			name: "create export",
			run: func(client *Client) error {
				_, err := client.CreateExport(context.Background(), sampleCreateExportRequest())
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertProviderCode(t, tt.run(newClient()), "aws_config_missing_credentials")
		})
	}
}

func TestSetupClientClassifiesBucketAccessAndPolicyResponses(t *testing.T) {
	t.Run("head bucket status-only not found returns safe create-plan signal", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{headBucketErr: responseStatusError(404)}})
		_, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{
			Bucket:        "bucket",
			Region:        "us-west-2",
			ExpectedOwner: "123456789012",
		})
		assertProviderCode(t, err, "aws_s3_bucket_not_found")
	})
	t.Run("head bucket no such bucket error returns safe not found signal", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{headBucketErr: &smithy.GenericAPIError{Code: "NoSuchBucket", Message: "missing"}}})
		_, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{
			Bucket:        "bucket",
			Region:        "us-west-2",
			ExpectedOwner: "123456789012",
		})
		assertProviderCode(t, err, "aws_s3_bucket_not_found")
	})
	t.Run("head bucket generic not found code is not safe absence proof", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{headBucketErr: &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "not found"}}})
		_, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{
			Bucket:        "bucket",
			Region:        "us-west-2",
			ExpectedOwner: "123456789012",
		})
		assertProviderCode(t, err, "aws_s3_bucket_inaccessible")
	})
	t.Run("head bucket bad request status remains inaccessible", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{headBucketErr: responseStatusError(400)}})
		access, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{
			Bucket:        "bucket",
			Region:        "us-west-2",
			ExpectedOwner: "123456789012",
		})
		if err != nil {
			t.Fatalf("HeadBucket returned error: %v", err)
		}
		if access.Accessible || access.StatusCode != 400 {
			t.Fatalf("BucketAccess = %#v, want inaccessible 400", access)
		}
	})
	t.Run("head bucket forbidden with expected owner is ambiguous", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{headBucketErr: responseStatusError(403)}})
		access, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{
			Bucket:        "bucket",
			Region:        "us-west-2",
			ExpectedOwner: "123456789012",
		})
		if err != nil {
			t.Fatalf("HeadBucket returned error: %v", err)
		}
		if access.Accessible || access.StatusCode != 403 {
			t.Fatalf("BucketAccess = %#v, want inaccessible 403", access)
		}
	})
	t.Run("head bucket sdk error without status uses safe fallback", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{headBucketErr: errors.New("network failed")}})
		_, err := client.HeadBucket(context.Background(), billingcur2setup.HeadBucketRequest{
			Bucket: "bucket",
			Region: "us-west-2",
		})
		assertProviderCode(t, err, "aws_s3_bucket_inaccessible")
	})
	t.Run("get bucket policy nil output fails closed", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{getBucketPolicyNilOutput: true}})
		_, err := client.GetBucketPolicy(context.Background(), billingcur2setup.BucketPolicyRequest{
			Bucket:        "bucket",
			ExpectedOwner: "123456789012",
		})
		assertProviderCode(t, err, "aws_s3_bucket_policy_inaccessible")
	})
	t.Run("get bucket policy forbidden with expected owner stays policy inaccessible", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{getBucketPolicyErr: responseStatusError(403)}})
		_, err := client.GetBucketPolicy(context.Background(), billingcur2setup.BucketPolicyRequest{
			Bucket:        "bucket",
			ExpectedOwner: "123456789012",
		})
		assertProviderCode(t, err, "aws_s3_bucket_policy_inaccessible")
	})
	t.Run("put bucket policy forbidden with expected owner stays put policy failed", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{putBucketPolicyErr: responseStatusError(403)}})
		err := client.PutBucketPolicy(context.Background(), billingcur2setup.PutBucketPolicyRequest{
			Bucket:        "bucket",
			ExpectedOwner: "123456789012",
			Policy:        "{}",
		})
		assertProviderCode(t, err, "aws_s3_put_bucket_policy_failed")
	})
	t.Run("get bucket policy generic failure uses inaccessible fallback", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{getBucketPolicyErr: errors.New("offline")}})
		_, err := client.GetBucketPolicy(context.Background(), billingcur2setup.BucketPolicyRequest{Bucket: "bucket"})
		assertProviderCode(t, err, "aws_s3_bucket_policy_inaccessible")
	})
	t.Run("put bucket policy generic failure uses safe fallback", func(t *testing.T) {
		client := New(Config{S3Client: &fakeS3{putBucketPolicyErr: errors.New("offline")}})
		err := client.PutBucketPolicy(context.Background(), billingcur2setup.PutBucketPolicyRequest{Bucket: "bucket", Policy: "{}"})
		assertProviderCode(t, err, "aws_s3_put_bucket_policy_failed")
	})
}

func TestSetupClientClassifiesAdditionalProviderErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "data exports access denied",
			run: func() error {
				_, err := New(Config{DataExportsClient: &fakeDataExports{createExportErr: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}}}).CreateExport(context.Background(), sampleCreateExportRequest())
				return err
			},
			want: "aws_data_exports_access_denied",
		},
		{
			name: "data exports throttled",
			run: func() error {
				_, err := New(Config{DataExportsClient: &fakeDataExports{createExportErr: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "throttle"}}}).CreateExport(context.Background(), sampleCreateExportRequest())
				return err
			},
			want: "aws_data_exports_throttled",
		},
		{
			name: "data exports too many requests is throttling",
			run: func() error {
				_, err := New(Config{DataExportsClient: &fakeDataExports{createExportErr: &smithy.GenericAPIError{Code: "TooManyRequestsException", Message: "throttle"}}}).CreateExport(context.Background(), sampleCreateExportRequest())
				return err
			},
			want: "aws_data_exports_throttled",
		},
		{
			name: "data exports quota full",
			run: func() error {
				_, err := New(Config{DataExportsClient: &fakeDataExports{createExportErr: &smithy.GenericAPIError{Code: "ServiceQuotaExceededException", Message: "quota"}}}).CreateExport(context.Background(), sampleCreateExportRequest())
				return err
			},
			want: "aws_cur2_export_quota_full",
		},
		{
			name: "data exports validation",
			run: func() error {
				_, err := New(Config{DataExportsClient: &fakeDataExports{createExportErr: &smithy.GenericAPIError{Code: "ValidationException", Message: "invalid"}}}).CreateExport(context.Background(), sampleCreateExportRequest())
				return err
			},
			want: "aws_cur2_create_export_failed",
		},
		{
			name: "organizations throttled",
			run: func() error {
				_, err := New(Config{OrganizationsClient: &fakeOrganizations{err: &smithy.GenericAPIError{Code: "TooManyRequestsException", Message: "throttle"}}}).DescribeOrganization(context.Background())
				return err
			},
			want: "aws_organizations_unavailable",
		},
		{
			name: "s3 create bucket already owned by caller",
			run: func() error {
				return New(Config{S3Client: &fakeS3{createBucketErr: &smithy.GenericAPIError{Code: "BucketAlreadyOwnedByYou", Message: "owned"}}}).CreateBucket(context.Background(), billingcur2setup.CreateBucketRequest{Bucket: "bucket", Region: "us-west-2"})
			},
			want: "aws_s3_bucket_already_owned_by_caller",
		},
		{
			name: "s3 create bucket name unavailable",
			run: func() error {
				return New(Config{S3Client: &fakeS3{createBucketErr: &smithy.GenericAPIError{Code: "BucketAlreadyExists", Message: "exists"}}}).CreateBucket(context.Background(), billingcur2setup.CreateBucketRequest{Bucket: "bucket", Region: "us-west-2"})
			},
			want: "aws_s3_bucket_already_exists",
		},
		{
			name: "s3 method not allowed maps owner mismatch",
			run: func() error {
				return New(Config{S3Client: &fakeS3{putBucketPolicyErr: &smithy.GenericAPIError{Code: "MethodNotAllowed", Message: "cross-account"}}}).PutBucketPolicy(context.Background(), billingcur2setup.PutBucketPolicyRequest{
					Bucket:        "bucket",
					ExpectedOwner: "123456789012",
					Policy:        "{}",
				})
			},
			want: "aws_s3_bucket_owner_mismatch",
		},
		{
			name: "s3 create bucket validation",
			run: func() error {
				return New(Config{S3Client: &fakeS3{createBucketErr: &smithy.GenericAPIError{Code: "InvalidBucketName", Message: "bad"}}}).CreateBucket(context.Background(), billingcur2setup.CreateBucketRequest{Bucket: "bucket", Region: "us-west-2"})
			},
			want: "aws_s3_create_bucket_failed",
		},
		{
			name: "s3 create bucket not found style error",
			run: func() error {
				return New(Config{S3Client: &fakeS3{createBucketErr: &smithy.GenericAPIError{Code: "NoSuchBucket", Message: "not found"}}}).CreateBucket(context.Background(), billingcur2setup.CreateBucketRequest{Bucket: "bucket", Region: "us-west-2"})
			},
			want: "aws_s3_create_bucket_failed",
		},
		{
			name: "organizations generic failure",
			run: func() error {
				_, err := New(Config{OrganizationsClient: &fakeOrganizations{err: errors.New("offline")}}).DescribeOrganization(context.Background())
				return err
			},
			want: "aws_organizations_unavailable",
		},
		{
			name: "data exports generic failure",
			run: func() error {
				_, err := New(Config{DataExportsClient: &fakeDataExports{createExportErr: errors.New("offline")}}).CreateExport(context.Background(), sampleCreateExportRequest())
				return err
			},
			want: "aws_cur2_create_export_failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertProviderCode(t, tt.run(), tt.want)
		})
	}
}

func TestCreateExportCopiesTableConfigurations(t *testing.T) {
	data := &fakeDataExports{createExportOutput: &awsbcm.CreateExportOutput{ExportArn: aws.String("arn:aws:bcm-data-exports:us-east-1:123456789012:export/matilda")}}
	client := New(Config{DataExportsClient: data})
	request := sampleCreateExportRequest()

	if _, err := client.CreateExport(context.Background(), request); err != nil {
		t.Fatalf("CreateExport returned error: %v", err)
	}
	request.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"] = "DAILY"

	if got := data.createExportInput.Export.DataQuery.TableConfigurations["COST_AND_USAGE_REPORT"]["TIME_GRANULARITY"]; got != "MONTHLY" {
		t.Fatalf("copied TIME_GRANULARITY = %q, want MONTHLY after caller mutation", got)
	}
	if copied := copyNestedStringMap(nil); copied != nil {
		t.Fatalf("copyNestedStringMap(nil) = %#v, want nil", copied)
	}
	if copied := copyStringMap(nil); copied != nil {
		t.Fatalf("copyStringMap(nil) = %#v, want nil", copied)
	}
}

func TestDefaultFactoryBuildsSDKClientsWithoutCloudCalls(t *testing.T) {
	factory := defaultFactory{}
	config := aws.Config{Region: "us-east-1"}

	if factory.DataExportsClient(config) == nil {
		t.Fatal("DataExportsClient returned nil")
	}
	if factory.S3Client(config) == nil {
		t.Fatal("S3Client returned nil")
	}
	if factory.OrganizationsClient(config) == nil {
		t.Fatal("OrganizationsClient returned nil")
	}
}

func sampleCreateExportRequest() billingcur2setup.CreateExportRequest {
	return billingcur2setup.CreateExportRequest{
		Name:           "matilda-cur2-ra-billing-abcdefghijkl-00",
		QueryStatement: "SELECT line_item_usage_amount FROM COST_AND_USAGE_REPORT",
		TableConfigurations: map[string]map[string]string{
			"COST_AND_USAGE_REPORT": {
				"TIME_GRANULARITY": "MONTHLY",
				"BILLING_VIEW_ARN": "arn:aws:billing::123456789012:billingview/primary",
			},
		},
		Destination: cur2preflight.S3Destination{
			Bucket:      "matilda-ra-billing-aws-us-west-2-abcdefghijkl-00",
			BucketOwner: "123456789012",
			Prefix:      "matilda/rapid-assessment/billing",
			Region:      "us-west-2",
			Output: cur2preflight.S3Output{
				Format:      "TEXT_OR_CSV",
				Compression: "GZIP",
				Overwrite:   "CREATE_NEW_REPORT",
				OutputType:  "CUSTOM",
			},
		},
		RefreshCadence:    "SYNCHRONOUS",
		DataExportsRegion: "us-east-1",
	}
}

func responseStatusError(status int) error {
	return &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{
			Response: &http.Response{StatusCode: status},
		},
		Err: errors.New("aws response error"),
	}
}

func assertProviderCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	var providerErr billingcur2setup.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != want {
		t.Fatalf("error = %#v, want %s ProviderError", err, want)
	}
}

type fakeDataExports struct {
	createExportInput     *awsbcm.CreateExportInput
	createExportOutput    *awsbcm.CreateExportOutput
	createExportNilOutput bool
	createExportErr       error
}

type fakePreflight struct {
	config           cur2preflight.Configuration
	identity         cur2preflight.Identity
	export           cur2preflight.Export
	listExportsToken string
}

func (fake *fakePreflight) CheckConfiguration(context.Context) (cur2preflight.Configuration, error) {
	return fake.config, nil
}

func (fake *fakePreflight) GetCallerIdentity(context.Context) (cur2preflight.Identity, error) {
	return fake.identity, nil
}

func (fake *fakePreflight) ListTables(context.Context, string) (cur2preflight.TablePage, error) {
	return cur2preflight.TablePage{}, nil
}

func (fake *fakePreflight) GetTable(context.Context, string, map[string]string) (cur2preflight.Table, error) {
	return cur2preflight.Table{}, nil
}

func (fake *fakePreflight) ListExports(_ context.Context, token string) (cur2preflight.ExportPage, error) {
	fake.listExportsToken = token
	return cur2preflight.ExportPage{
		Exports:   []cur2preflight.ExportSummary{{Name: fake.export.Name, ExportARN: fake.export.ExportARN}},
		NextToken: "next",
	}, nil
}

func (fake *fakePreflight) GetExport(context.Context, string) (cur2preflight.Export, error) {
	return fake.export, nil
}

func (fake *fakePreflight) HeadBucket(context.Context, string) (cur2preflight.BucketAccess, error) {
	return cur2preflight.BucketAccess{}, nil
}

func (fake *fakePreflight) GetBucketPolicy(context.Context, string) (string, error) {
	return "", nil
}

func (fake *fakePreflight) ListExecutions(context.Context, string, string) (cur2preflight.ExecutionPage, error) {
	return cur2preflight.ExecutionPage{}, nil
}

func (fake *fakePreflight) GetExecution(context.Context, string, string) (cur2preflight.Execution, error) {
	return cur2preflight.Execution{}, nil
}

func (fake *fakePreflight) ListObjects(context.Context, string, string, string, int32) (cur2preflight.ObjectPage, error) {
	return cur2preflight.ObjectPage{}, nil
}

func (fake *fakeDataExports) CreateExport(ctx context.Context, input *awsbcm.CreateExportInput, optFns ...func(*awsbcm.Options)) (*awsbcm.CreateExportOutput, error) {
	fake.createExportInput = input
	if fake.createExportErr != nil {
		return nil, fake.createExportErr
	}
	if fake.createExportOutput != nil {
		return fake.createExportOutput, nil
	}
	if fake.createExportNilOutput {
		return nil, nil
	}
	return &awsbcm.CreateExportOutput{}, nil
}

type fakeS3 struct {
	listBucketsInput     *awss3.ListBucketsInput
	listBucketsOutput    *awss3.ListBucketsOutput
	listBucketsNilOutput bool
	listBucketsErr       error

	headBucketInput          *awss3.HeadBucketInput
	headBucketOutput         *awss3.HeadBucketOutput
	headBucketNilOutput      bool
	headBucketErr            error
	createBucketInput        *awss3.CreateBucketInput
	createBucketNilOutput    bool
	createBucketErr          error
	getBucketPolicyInput     *awss3.GetBucketPolicyInput
	getBucketPolicyOutput    *awss3.GetBucketPolicyOutput
	getBucketPolicyNilOutput bool
	getBucketPolicyErr       error
	putBucketPolicyInput     *awss3.PutBucketPolicyInput
	putBucketPolicyNilOutput bool
	putBucketPolicyErr       error
}

func (fake *fakeS3) ListBuckets(ctx context.Context, input *awss3.ListBucketsInput, optFns ...func(*awss3.Options)) (*awss3.ListBucketsOutput, error) {
	fake.listBucketsInput = input
	if fake.listBucketsErr != nil {
		return nil, fake.listBucketsErr
	}
	if fake.listBucketsNilOutput {
		return nil, nil
	}
	if fake.listBucketsOutput != nil {
		return fake.listBucketsOutput, nil
	}
	return &awss3.ListBucketsOutput{}, nil
}

func (fake *fakeS3) HeadBucket(ctx context.Context, input *awss3.HeadBucketInput, optFns ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error) {
	fake.headBucketInput = input
	if fake.headBucketErr != nil {
		return nil, fake.headBucketErr
	}
	if fake.headBucketOutput != nil {
		return fake.headBucketOutput, nil
	}
	if fake.headBucketNilOutput {
		return nil, nil
	}
	return &awss3.HeadBucketOutput{}, nil
}

func (fake *fakeS3) CreateBucket(ctx context.Context, input *awss3.CreateBucketInput, optFns ...func(*awss3.Options)) (*awss3.CreateBucketOutput, error) {
	fake.createBucketInput = input
	if fake.createBucketErr != nil {
		return nil, fake.createBucketErr
	}
	if fake.createBucketNilOutput {
		return nil, nil
	}
	return &awss3.CreateBucketOutput{}, nil
}

func (fake *fakeS3) GetBucketPolicy(ctx context.Context, input *awss3.GetBucketPolicyInput, optFns ...func(*awss3.Options)) (*awss3.GetBucketPolicyOutput, error) {
	fake.getBucketPolicyInput = input
	if fake.getBucketPolicyErr != nil {
		return nil, fake.getBucketPolicyErr
	}
	if fake.getBucketPolicyNilOutput {
		return nil, nil
	}
	if fake.getBucketPolicyOutput != nil {
		return fake.getBucketPolicyOutput, nil
	}
	return &awss3.GetBucketPolicyOutput{Policy: aws.String("{}")}, nil
}

func (fake *fakeS3) PutBucketPolicy(ctx context.Context, input *awss3.PutBucketPolicyInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketPolicyOutput, error) {
	fake.putBucketPolicyInput = input
	if fake.putBucketPolicyErr != nil {
		return nil, fake.putBucketPolicyErr
	}
	if fake.putBucketPolicyNilOutput {
		return nil, nil
	}
	return &awss3.PutBucketPolicyOutput{}, nil
}

type fakeOrganizations struct {
	output *awsorg.DescribeOrganizationOutput
	err    error
}

func (fake *fakeOrganizations) DescribeOrganization(ctx context.Context, input *awsorg.DescribeOrganizationInput, optFns ...func(*awsorg.Options)) (*awsorg.DescribeOrganizationOutput, error) {
	if fake.err != nil {
		return nil, fake.err
	}
	if fake.output != nil {
		return fake.output, nil
	}
	return &awsorg.DescribeOrganizationOutput{}, nil
}

type fakeFactory struct {
	data          *fakeDataExports
	s3            *fakeS3
	organizations *fakeOrganizations

	dataRegion          string
	s3Region            string
	organizationsRegion string
}

func (fake *fakeFactory) DataExportsClient(config aws.Config) dataExportsAPI {
	fake.dataRegion = config.Region
	return fake.data
}

func (fake *fakeFactory) S3Client(config aws.Config) s3API {
	fake.s3Region = config.Region
	return fake.s3
}

func (fake *fakeFactory) OrganizationsClient(config aws.Config) organizationsAPI {
	fake.organizationsRegion = config.Region
	return fake.organizations
}
