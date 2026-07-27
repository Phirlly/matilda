package awsclient

import (
	"context"
	"net/http"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

func (client *Client) HeadBucket(ctx context.Context, bucket string) (cur2preflight.BucketAccess, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.BucketAccess{}, err
	}
	output, err := client.s3.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		if access, ok := headBucketStatus(err); ok && isAmbiguousHeadBucketStatus(access.StatusCode) {
			return access, nil
		}
		if status := statusCode(err); status != 0 {
			return cur2preflight.BucketAccess{Accessible: false, StatusCode: status}, nil
		}
		return cur2preflight.BucketAccess{}, classifyS3Error(err, "aws_s3_bucket_inaccessible")
	}
	access := cur2preflight.BucketAccess{
		Accessible: true,
		StatusCode: http.StatusOK,
	}
	if output != nil {
		access.Region = aws.ToString(output.BucketRegion)
	}
	return access, nil
}

func (client *Client) GetBucketPolicy(ctx context.Context, bucket string) (string, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return "", err
	}
	output, err := client.s3.GetBucketPolicy(ctx, &awss3.GetBucketPolicyInput{Bucket: aws.String(bucket)})
	if err != nil {
		return "", classifyS3Error(err, "aws_s3_bucket_policy_inaccessible")
	}
	if output == nil {
		return "", nil
	}
	return aws.ToString(output.Policy), nil
}

func (client *Client) ListObjects(ctx context.Context, bucket string, prefix string, token string, maxKeys int32) (cur2preflight.ObjectPage, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.ObjectPage{}, err
	}
	input := &awss3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(maxKeys),
	}
	if token != "" {
		input.ContinuationToken = aws.String(token)
	}
	output, err := client.s3.ListObjectsV2(ctx, input)
	if err != nil {
		return cur2preflight.ObjectPage{}, classifyS3Error(err, "aws_s3_bucket_inaccessible")
	}
	if output == nil {
		return cur2preflight.ObjectPage{}, nil
	}
	page := cur2preflight.ObjectPage{NextToken: aws.ToString(output.NextContinuationToken)}
	for _, object := range output.Contents {
		if key := aws.ToString(object.Key); key != "" {
			page.Keys = append(page.Keys, key)
		}
	}
	return page, nil
}
