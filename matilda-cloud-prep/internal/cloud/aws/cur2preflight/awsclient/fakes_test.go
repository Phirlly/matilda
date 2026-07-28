package awsclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsbcm "github.com/aws/aws-sdk-go-v2/service/bcmdataexports"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
	smithy "github.com/aws/smithy-go"
)

func readyClient(t *testing.T, stsClient *fakeSTS, data *fakeDataExports, s3Client *fakeS3) *Client {
	t.Helper()

	client := New(Config{
		LoadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
			return staticConfig("us-west-2"), nil
		},
		ClientFactory: &fakeFactory{
			stsClient:         stsClient,
			dataExportsClient: data,
			s3Client:          s3Client,
		},
	})
	if _, err := client.CheckConfiguration(context.Background()); err != nil {
		t.Fatalf("CheckConfiguration returned error: %v", err)
	}
	return client
}

func staticConfig(region string) aws.Config {
	return aws.Config{
		Region: region,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Source:          "test",
			}, nil
		}),
	}
}

func apiError(code string, message string, fault smithy.ErrorFault) error {
	return &smithy.OperationError{
		ServiceID:     "test",
		OperationName: "test",
		Err: &smithy.GenericAPIError{
			Code:    code,
			Message: message,
			Fault:   fault,
		},
	}
}

func assertProviderCode(t *testing.T, err error, code string) {
	t.Helper()

	var providerErr cur2preflight.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error %T %[1]v is not ProviderError", err)
	}
	if providerErr.Code != code {
		t.Fatalf("ProviderError.Code = %q, want %q", providerErr.Code, code)
	}
}

func assertSafeError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	for _, forbidden := range []string{"AKIA", "secret", "arn:aws", "request id", "host id"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("error leaked %q in %q", forbidden, text)
		}
	}
}

type fakeFactory struct {
	stsClient         *fakeSTS
	dataExportsClient *fakeDataExports
	s3Client          *fakeS3
	stsCalls          int
	dataExportsCalls  int
	s3Calls           int
	dataExportsRegion string
	s3Region          string
}

func (factory *fakeFactory) STSClient(aws.Config) stsAPI {
	factory.stsCalls++
	if factory.stsClient == nil {
		factory.stsClient = &fakeSTS{}
	}
	return factory.stsClient
}

func (factory *fakeFactory) DataExportsClient(config aws.Config) dataExportsAPI {
	factory.dataExportsCalls++
	factory.dataExportsRegion = config.Region
	if factory.dataExportsClient == nil {
		factory.dataExportsClient = &fakeDataExports{}
	}
	return factory.dataExportsClient
}

func (factory *fakeFactory) S3Client(config aws.Config) s3API {
	factory.s3Calls++
	factory.s3Region = config.Region
	if factory.s3Client == nil {
		factory.s3Client = &fakeS3{}
	}
	return factory.s3Client
}

type fakeSTS struct {
	output *awssts.GetCallerIdentityOutput
	err    error
}

func (client *fakeSTS) GetCallerIdentity(context.Context, *awssts.GetCallerIdentityInput, ...func(*awssts.Options)) (*awssts.GetCallerIdentityOutput, error) {
	if client.err != nil {
		return nil, client.err
	}
	return client.output, nil
}

type fakeDataExports struct {
	listTablesOutput     *awsbcm.ListTablesOutput
	getTableOutput       *awsbcm.GetTableOutput
	listExportsOutput    *awsbcm.ListExportsOutput
	getExportOutput      *awsbcm.GetExportOutput
	listExecutionsOutput *awsbcm.ListExecutionsOutput
	getExecutionOutput   *awsbcm.GetExecutionOutput
	err                  error
	listTablesInputs     []*awsbcm.ListTablesInput
	getTableInputs       []*awsbcm.GetTableInput
	listExportsInputs    []*awsbcm.ListExportsInput
	getExportInputs      []*awsbcm.GetExportInput
	listExecutionsInputs []*awsbcm.ListExecutionsInput
	getExecutionInputs   []*awsbcm.GetExecutionInput
}

func (client *fakeDataExports) ListTables(_ context.Context, input *awsbcm.ListTablesInput, _ ...func(*awsbcm.Options)) (*awsbcm.ListTablesOutput, error) {
	client.listTablesInputs = append(client.listTablesInputs, input)
	if client.err != nil {
		return nil, client.err
	}
	return client.listTablesOutput, nil
}

func (client *fakeDataExports) GetTable(_ context.Context, input *awsbcm.GetTableInput, _ ...func(*awsbcm.Options)) (*awsbcm.GetTableOutput, error) {
	client.getTableInputs = append(client.getTableInputs, input)
	if client.err != nil {
		return nil, client.err
	}
	return client.getTableOutput, nil
}

func (client *fakeDataExports) ListExports(_ context.Context, input *awsbcm.ListExportsInput, _ ...func(*awsbcm.Options)) (*awsbcm.ListExportsOutput, error) {
	client.listExportsInputs = append(client.listExportsInputs, input)
	if client.err != nil {
		return nil, client.err
	}
	return client.listExportsOutput, nil
}

func (client *fakeDataExports) GetExport(_ context.Context, input *awsbcm.GetExportInput, _ ...func(*awsbcm.Options)) (*awsbcm.GetExportOutput, error) {
	client.getExportInputs = append(client.getExportInputs, input)
	if client.err != nil {
		return nil, client.err
	}
	return client.getExportOutput, nil
}

func (client *fakeDataExports) ListExecutions(_ context.Context, input *awsbcm.ListExecutionsInput, _ ...func(*awsbcm.Options)) (*awsbcm.ListExecutionsOutput, error) {
	client.listExecutionsInputs = append(client.listExecutionsInputs, input)
	if client.err != nil {
		return nil, client.err
	}
	return client.listExecutionsOutput, nil
}

func (client *fakeDataExports) GetExecution(_ context.Context, input *awsbcm.GetExecutionInput, _ ...func(*awsbcm.Options)) (*awsbcm.GetExecutionOutput, error) {
	client.getExecutionInputs = append(client.getExecutionInputs, input)
	if client.err != nil {
		return nil, client.err
	}
	return client.getExecutionOutput, nil
}

type fakeS3 struct {
	headOutput        *awss3.HeadBucketOutput
	policyOutput      *awss3.GetBucketPolicyOutput
	listObjectsOutput *awss3.ListObjectsV2Output
	headErr           error
	policyErr         error
	listObjectsErr    error
	headInputs        []*awss3.HeadBucketInput
	policyInputs      []*awss3.GetBucketPolicyInput
	listObjectsInputs []*awss3.ListObjectsV2Input
}

func (client *fakeS3) HeadBucket(_ context.Context, input *awss3.HeadBucketInput, _ ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error) {
	client.headInputs = append(client.headInputs, input)
	if client.headErr != nil {
		return nil, client.headErr
	}
	return client.headOutput, nil
}

func (client *fakeS3) GetBucketPolicy(_ context.Context, input *awss3.GetBucketPolicyInput, _ ...func(*awss3.Options)) (*awss3.GetBucketPolicyOutput, error) {
	client.policyInputs = append(client.policyInputs, input)
	if client.policyErr != nil {
		return nil, client.policyErr
	}
	return client.policyOutput, nil
}

func (client *fakeS3) ListObjectsV2(_ context.Context, input *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	client.listObjectsInputs = append(client.listObjectsInputs, input)
	if client.listObjectsErr != nil {
		return nil, client.listObjectsErr
	}
	return client.listObjectsOutput, nil
}
