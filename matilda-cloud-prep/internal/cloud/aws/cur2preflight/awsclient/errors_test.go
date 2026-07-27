package awsclient

import (
	"context"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
	smithy "github.com/aws/smithy-go"
)

func TestSDKErrorsClassifyWithoutRawMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		call func(*Client) error
		code string
	}{
		{
			name: "sts auth",
			err:  apiError("InvalidClientTokenId", "raw AKIA bad token", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.GetCallerIdentity(context.Background())
				return err
			},
			code: "aws_auth_failed",
		},
		{
			name: "data exports access denied",
			err:  apiError("AccessDeniedException", "raw arn:aws denied", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.ListExports(context.Background(), "")
				return err
			},
			code: "aws_data_exports_access_denied",
		},
		{
			name: "data exports throttled",
			err:  apiError("ThrottlingException", "raw request throttled", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.ListTables(context.Background(), "")
				return err
			},
			code: "aws_data_exports_throttled",
		},
		{
			name: "data exports transient",
			err:  apiError("InternalServerException", "raw request id", smithy.FaultServer),
			call: func(client *Client) error {
				_, err := client.ListExecutions(context.Background(), "export-arn", "")
				return err
			},
			code: "aws_data_exports_transient",
		},
		{
			name: "get table not found",
			err:  apiError("ResourceNotFoundException", "raw not found", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.GetTable(context.Background(), "COST_AND_USAGE_REPORT", nil)
				return err
			},
			code: "aws_cur2_table_unavailable",
		},
		{
			name: "get export validation",
			err:  apiError("ValidationException", "raw validation", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.GetExport(context.Background(), "export-arn")
				return err
			},
			code: "aws_cur2_export_invalid_shape",
		},
		{
			name: "get execution validation",
			err:  apiError("ValidationException", "raw execution validation", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.GetExecution(context.Background(), "export-arn", "execution-id")
				return err
			},
			code: "aws_cur2_export_invalid_shape",
		},
		{
			name: "get execution not found",
			err:  apiError("ResourceNotFoundException", "raw execution not found", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.GetExecution(context.Background(), "export-arn", "execution-id")
				return err
			},
			code: "aws_data_exports_access_denied",
		},
		{
			name: "s3 policy inaccessible",
			err:  apiError("MethodNotAllowed", "raw host id", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.GetBucketPolicy(context.Background(), "bucket")
				return err
			},
			code: "aws_s3_bucket_policy_inaccessible",
		},
		{
			name: "s3 list inaccessible",
			err:  apiError("NoSuchBucket", "raw bucket missing", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.ListObjects(context.Background(), "bucket", "prefix", "", 1000)
				return err
			},
			code: "aws_s3_bucket_inaccessible",
		},
		{
			name: "data exports unknown api error uses fallback",
			err:  apiError("UnknownProviderProblem", "raw request id", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.GetExecution(context.Background(), "export-arn", "execution-id")
				return err
			},
			code: "aws_data_exports_access_denied",
		},
		{
			name: "s3 unknown api error uses fallback",
			err:  apiError("UnknownS3Problem", "raw host id", smithy.FaultClient),
			call: func(client *Client) error {
				_, err := client.GetBucketPolicy(context.Background(), "bucket")
				return err
			},
			code: "aws_s3_bucket_policy_inaccessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stsClient := &fakeSTS{err: tt.err}
			data := &fakeDataExports{err: tt.err}
			s3Client := &fakeS3{policyErr: tt.err, listObjectsErr: tt.err}
			client := readyClient(t, stsClient, data, s3Client)

			err := tt.call(client)

			assertProviderCode(t, err, tt.code)
			assertSafeError(t, err)
		})
	}
}

func TestNilAndPartialResponsesFailClosedWithoutPanics(t *testing.T) {
	client := readyClient(t, &fakeSTS{output: &awssts.GetCallerIdentityOutput{}}, &fakeDataExports{
		getTableOutput:  nil,
		getExportOutput: nil,
	}, &fakeS3{
		policyOutput:      &awss3.GetBucketPolicyOutput{},
		listObjectsOutput: nil,
	})

	identity, err := client.GetCallerIdentity(context.Background())
	if err != nil {
		t.Fatalf("empty STS output should map without adapter error: %v", err)
	}
	if identity.AccountID != "" || identity.CallerARN != "" {
		t.Fatalf("identity = %#v, want empty values for engine validation", identity)
	}

	_, err = client.GetTable(context.Background(), "COST_AND_USAGE_REPORT", nil)
	assertProviderCode(t, err, "aws_cur2_table_unavailable")

	_, err = client.GetExport(context.Background(), "export-arn")
	assertProviderCode(t, err, "aws_cur2_export_invalid_shape")

	policy, err := client.GetBucketPolicy(context.Background(), "bucket")
	if err != nil {
		t.Fatalf("empty policy output should map to empty policy text: %v", err)
	}
	if policy != "" {
		t.Fatalf("policy = %q, want empty", policy)
	}

	objects, err := client.ListObjects(context.Background(), "bucket", "prefix", "", 1000)
	if err != nil {
		t.Fatalf("nil object page should map to empty page: %v", err)
	}
	if len(objects.Keys) != 0 || objects.NextToken != "" {
		t.Fatalf("objects = %#v, want empty page", objects)
	}

	client = readyClient(t, &fakeSTS{}, &fakeDataExports{getExecutionOutput: nil}, &fakeS3{})
	_, err = client.GetExecution(context.Background(), "export-arn", "execution-id")
	assertProviderCode(t, err, "aws_data_exports_access_denied")
}
