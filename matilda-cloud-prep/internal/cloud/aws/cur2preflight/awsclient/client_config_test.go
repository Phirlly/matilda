package awsclient

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestCheckConfigurationLoadsRegionCredentialsAndBuildsClientsLazily(t *testing.T) {
	factory := &fakeFactory{}
	client := New(Config{
		LoadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
			return staticConfig("us-west-2"), nil
		},
		ClientFactory: factory,
	})

	config, err := client.CheckConfiguration(context.Background())
	if err != nil {
		t.Fatalf("CheckConfiguration returned error: %v", err)
	}
	if config.Region != "us-west-2" {
		t.Fatalf("Region = %q, want us-west-2", config.Region)
	}
	if factory.stsCalls != 1 || factory.dataExportsCalls != 1 || factory.s3Calls != 1 {
		t.Fatalf("factory calls = sts:%d data:%d s3:%d, want one each", factory.stsCalls, factory.dataExportsCalls, factory.s3Calls)
	}
	if factory.dataExportsRegion != "us-east-1" {
		t.Fatalf("Data Exports region = %q, want us-east-1", factory.dataExportsRegion)
	}
	if factory.s3Region != "us-west-2" {
		t.Fatalf("S3 region = %q, want us-west-2", factory.s3Region)
	}

	_, err = client.CheckConfiguration(context.Background())
	if err != nil {
		t.Fatalf("second CheckConfiguration returned error: %v", err)
	}
	if factory.stsCalls != 1 || factory.dataExportsCalls != 1 || factory.s3Calls != 1 {
		t.Fatalf("factory should remain lazy/cached after second check, got sts:%d data:%d s3:%d", factory.stsCalls, factory.dataExportsCalls, factory.s3Calls)
	}
}

func TestCheckConfigurationPassesProfileAndRegionToLoader(t *testing.T) {
	var gotRequest LoadRequest
	client := New(Config{
		Profile: "default",
		Region:  "us-west-2",
		LoadConfig: func(_ context.Context, request LoadRequest) (aws.Config, error) {
			gotRequest = request
			return staticConfig(request.Region), nil
		},
		ClientFactory: &fakeFactory{},
	})

	config, err := client.CheckConfiguration(context.Background())

	if err != nil {
		t.Fatalf("CheckConfiguration returned error: %v", err)
	}
	if gotRequest.Profile != "default" {
		t.Fatalf("LoadRequest.Profile = %q, want default", gotRequest.Profile)
	}
	if gotRequest.Region != "us-west-2" {
		t.Fatalf("LoadRequest.Region = %q, want us-west-2", gotRequest.Region)
	}
	if config.Region != "us-west-2" {
		t.Fatalf("Configuration.Region = %q, want us-west-2", config.Region)
	}
}

func TestCheckConfigurationFailsClosedWhenProfileWouldBeShadowedByEnvCredentials(t *testing.T) {
	client := New(Config{
		Profile: "default",
		Region:  "us-west-2",
		EnvLookup: func(name string) (string, bool) {
			if name == "AWS_ACCESS_KEY_ID" {
				return "test-access-key", true
			}
			return "", false
		},
		LoadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
			t.Fatal("LoadConfig should not run when profile is shadowed by AWS credential environment variables")
			return aws.Config{}, nil
		},
		ClientFactory: &fakeFactory{},
	})

	_, err := client.CheckConfiguration(context.Background())

	assertProviderCode(t, err, "aws_config_profile_shadowed")
	assertSafeError(t, err)
}

func TestDefaultConstructorsAreInertUntilConfigurationCheck(t *testing.T) {
	client := NewDefault()
	if client == nil {
		t.Fatal("NewDefault returned nil")
	}

	factory := defaultFactory{}
	cfg := staticConfig("us-west-2")
	if factory.STSClient(cfg) == nil {
		t.Fatal("default STS client is nil")
	}
	dataCfg := cfg
	dataCfg.Region = dataExportsRegion
	if factory.DataExportsClient(dataCfg) == nil {
		t.Fatal("default Data Exports client is nil")
	}
	if factory.S3Client(cfg) == nil {
		t.Fatal("default S3 client is nil")
	}
}

func TestCheckConfigurationClassifiesMissingRegionAndCredentials(t *testing.T) {
	tests := []struct {
		name       string
		loadConfig func(context.Context, LoadRequest) (aws.Config, error)
		code       string
	}{
		{
			name: "missing region",
			loadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
				return staticConfig(""), nil
			},
			code: "aws_config_missing_region",
		},
		{
			name: "loader failure",
			loadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
				return aws.Config{}, errors.New("AKIAEXAMPLE raw credential failure")
			},
			code: "aws_config_missing_credentials",
		},
		{
			name: "credential retrieval failure",
			loadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
				cfg := staticConfig("us-east-1")
				cfg.Credentials = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
					return aws.Credentials{}, errors.New("secret raw credential failure")
				})
				return cfg, nil
			},
			code: "aws_config_missing_credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(Config{LoadConfig: tt.loadConfig, ClientFactory: &fakeFactory{}})

			_, err := client.CheckConfiguration(context.Background())

			assertProviderCode(t, err, tt.code)
			assertSafeError(t, err)
		})
	}
}

func TestReadMethodsLoadConfigurationWhenCalledDirectly(t *testing.T) {
	stsClient := &fakeSTS{output: &awssts.GetCallerIdentityOutput{Account: aws.String("123456789012"), Arn: aws.String("arn:aws:iam::123456789012:role/operator")}}
	factory := &fakeFactory{stsClient: stsClient}
	client := New(Config{
		LoadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
			return staticConfig("us-east-1"), nil
		},
		ClientFactory: factory,
	})

	identity, err := client.GetCallerIdentity(context.Background())

	if err != nil {
		t.Fatalf("GetCallerIdentity returned error: %v", err)
	}
	if identity.AccountID != "123456789012" {
		t.Fatalf("AccountID = %q, want mapped identity", identity.AccountID)
	}
	if factory.stsCalls != 1 {
		t.Fatalf("STS factory calls = %d, want 1", factory.stsCalls)
	}
}

func TestReadMethodsFailClosedWhenConfigurationIsMissing(t *testing.T) {
	methods := []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "identity",
			call: func(client *Client) error {
				_, err := client.GetCallerIdentity(context.Background())
				return err
			},
		},
		{
			name: "list tables",
			call: func(client *Client) error {
				_, err := client.ListTables(context.Background(), "")
				return err
			},
		},
		{
			name: "get table",
			call: func(client *Client) error {
				_, err := client.GetTable(context.Background(), "COST_AND_USAGE_REPORT", nil)
				return err
			},
		},
		{
			name: "list exports",
			call: func(client *Client) error {
				_, err := client.ListExports(context.Background(), "")
				return err
			},
		},
		{
			name: "get export",
			call: func(client *Client) error {
				_, err := client.GetExport(context.Background(), "export-arn")
				return err
			},
		},
		{
			name: "list executions",
			call: func(client *Client) error {
				_, err := client.ListExecutions(context.Background(), "export-arn", "")
				return err
			},
		},
		{
			name: "get execution",
			call: func(client *Client) error {
				_, err := client.GetExecution(context.Background(), "export-arn", "execution-id")
				return err
			},
		},
		{
			name: "head bucket",
			call: func(client *Client) error {
				_, err := client.HeadBucket(context.Background(), "bucket")
				return err
			},
		},
		{
			name: "get bucket policy",
			call: func(client *Client) error {
				_, err := client.GetBucketPolicy(context.Background(), "bucket")
				return err
			},
		},
		{
			name: "list objects",
			call: func(client *Client) error {
				_, err := client.ListObjects(context.Background(), "bucket", "prefix", "", 1000)
				return err
			},
		},
	}

	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			client := New(Config{
				LoadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
					return staticConfig(""), nil
				},
				ClientFactory: &fakeFactory{},
			})

			err := method.call(client)

			assertProviderCode(t, err, "aws_config_missing_region")
			assertSafeError(t, err)
		})
	}
}
