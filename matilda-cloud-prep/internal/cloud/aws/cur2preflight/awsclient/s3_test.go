package awsclient

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestS3Mapping(t *testing.T) {
	s3Client := &fakeS3{
		headOutput: &awss3.HeadBucketOutput{BucketRegion: aws.String("us-east-1")},
		policyOutput: &awss3.GetBucketPolicyOutput{
			Policy: aws.String(`{"Version":"2012-10-17","Statement":[]}`),
		},
		listObjectsOutput: &awss3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz")},
				{},
			},
			NextContinuationToken: aws.String("object-token"),
		},
	}
	client := readyClient(t, &fakeSTS{}, &fakeDataExports{}, s3Client)

	bucket, err := client.HeadBucket(context.Background(), "matilda-cur2-billing")
	if err != nil {
		t.Fatalf("HeadBucket returned error: %v", err)
	}
	if !bucket.Accessible || bucket.StatusCode != http.StatusOK || bucket.Region != "us-east-1" {
		t.Fatalf("bucket access = %#v, want reachable bucket", bucket)
	}
	if s3Client.headInputs[0].Bucket == nil || *s3Client.headInputs[0].Bucket != "matilda-cur2-billing" {
		t.Fatalf("HeadBucket input = %#v", s3Client.headInputs[0])
	}

	policy, err := client.GetBucketPolicy(context.Background(), "matilda-cur2-billing")
	if err != nil {
		t.Fatalf("GetBucketPolicy returned error: %v", err)
	}
	if policy != `{"Version":"2012-10-17","Statement":[]}` {
		t.Fatalf("policy = %q", policy)
	}

	objects, err := client.ListObjects(context.Background(), "matilda-cur2-billing", "matilda/cur2", "input-token", 25)
	if err != nil {
		t.Fatalf("ListObjects returned error: %v", err)
	}
	if !reflect.DeepEqual(objects.Keys, []string{"matilda/cur2/matilda-cur2/data/BILLING_PERIOD=2026-06/part-000.gz"}) {
		t.Fatalf("object keys = %#v", objects.Keys)
	}
	if objects.NextToken != "object-token" {
		t.Fatalf("object next token = %q, want object-token", objects.NextToken)
	}
	if got := s3Client.listObjectsInputs[0]; got.Bucket == nil || *got.Bucket != "matilda-cur2-billing" || got.Prefix == nil || *got.Prefix != "matilda/cur2" || got.ContinuationToken == nil || *got.ContinuationToken != "input-token" || got.MaxKeys == nil || *got.MaxKeys != 25 {
		t.Fatalf("ListObjects input = %#v", got)
	}
}

func TestS3EmptyAndErrorBranches(t *testing.T) {
	client := readyClient(t, &fakeSTS{}, &fakeDataExports{}, &fakeS3{
		headOutput:        nil,
		policyOutput:      nil,
		listObjectsOutput: &awss3.ListObjectsV2Output{},
	})

	bucket, err := client.HeadBucket(context.Background(), "bucket")
	if err != nil {
		t.Fatalf("HeadBucket returned error: %v", err)
	}
	if !bucket.Accessible || bucket.StatusCode != http.StatusOK || bucket.Region != "" {
		t.Fatalf("bucket access = %#v, want reachable without region", bucket)
	}

	policy, err := client.GetBucketPolicy(context.Background(), "bucket")
	if err != nil {
		t.Fatalf("GetBucketPolicy returned error: %v", err)
	}
	if policy != "" {
		t.Fatalf("policy = %q, want empty", policy)
	}

	objects, err := client.ListObjects(context.Background(), "bucket", "prefix", "", 1000)
	if err != nil {
		t.Fatalf("ListObjects returned error: %v", err)
	}
	if len(objects.Keys) != 0 || objects.NextToken != "" || client.s3.(*fakeS3).listObjectsInputs[0].ContinuationToken != nil {
		t.Fatalf("objects/input = %#v / %#v", objects, client.s3.(*fakeS3).listObjectsInputs[0])
	}
}

func TestHeadBucketMapsAmbiguousHTTPStatusWithoutRawError(t *testing.T) {
	s3Client := &fakeS3{
		headErr: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"X-Amz-Bucket-Region": []string{"us-east-1"}},
			}},
			Err: errors.New("raw arn:aws request failure"),
		},
	}
	client := readyClient(t, &fakeSTS{}, &fakeDataExports{}, s3Client)

	bucket, err := client.HeadBucket(context.Background(), "matilda-cur2-billing")

	if err != nil {
		t.Fatalf("HeadBucket returned error: %v", err)
	}
	if bucket.Accessible || bucket.StatusCode != http.StatusForbidden || bucket.Region != "us-east-1" {
		t.Fatalf("bucket access = %#v, want inaccessible status with region", bucket)
	}
}

func TestHeadBucketMapsNonAmbiguousHTTPStatusWithoutRawError(t *testing.T) {
	s3Client := &fakeS3{
		headErr: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusInternalServerError}},
			Err:      errors.New("raw request id"),
		},
	}
	client := readyClient(t, &fakeSTS{}, &fakeDataExports{}, s3Client)

	bucket, err := client.HeadBucket(context.Background(), "matilda-cur2-billing")

	if err != nil {
		t.Fatalf("HeadBucket returned error: %v", err)
	}
	if bucket.Accessible || bucket.StatusCode != http.StatusInternalServerError {
		t.Fatalf("bucket access = %#v, want inaccessible 500 status", bucket)
	}
}

func TestHeadBucketGenericErrorClassifiesSafely(t *testing.T) {
	client := readyClient(t, &fakeSTS{}, &fakeDataExports{}, &fakeS3{
		headErr: errors.New("raw arn:aws generic head failure"),
	})

	_, err := client.HeadBucket(context.Background(), "bucket")

	assertProviderCode(t, err, "aws_s3_bucket_inaccessible")
	assertSafeError(t, err)
}
