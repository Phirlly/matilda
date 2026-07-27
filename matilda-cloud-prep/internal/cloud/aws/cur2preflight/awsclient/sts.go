package awsclient

import (
	"context"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
)

func (client *Client) GetCallerIdentity(ctx context.Context) (cur2preflight.Identity, error) {
	if err := client.ensureConfigured(ctx); err != nil {
		return cur2preflight.Identity{}, err
	}
	output, err := client.sts.GetCallerIdentity(ctx, &awssts.GetCallerIdentityInput{})
	if err != nil {
		return cur2preflight.Identity{}, classifySTSSError(err)
	}
	if output == nil {
		return cur2preflight.Identity{}, nil
	}
	return cur2preflight.Identity{
		AccountID: aws.ToString(output.Account),
		CallerARN: aws.ToString(output.Arn),
	}, nil
}
