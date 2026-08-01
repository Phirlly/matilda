package awsclient

import (
	"context"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
)

func (client *Client) CheckConfiguration(ctx context.Context) (cur2preflight.Configuration, error) {
	return client.preflight.CheckConfiguration(ctx)
}

func (client *Client) GetCallerIdentity(ctx context.Context) (cur2preflight.Identity, error) {
	return client.preflight.GetCallerIdentity(ctx)
}

func (client *Client) ListTables(ctx context.Context, token string) (cur2preflight.TablePage, error) {
	return client.preflight.ListTables(ctx, token)
}

func (client *Client) GetTable(ctx context.Context, name string, properties map[string]string) (cur2preflight.Table, error) {
	return client.preflight.GetTable(ctx, name, properties)
}

func (client *Client) ListExports(ctx context.Context, token string) (cur2preflight.ExportPage, error) {
	return client.preflight.ListExports(ctx, token)
}

func (client *Client) GetExport(ctx context.Context, exportARN string) (cur2preflight.Export, error) {
	return client.preflight.GetExport(ctx, exportARN)
}

func (client *Client) HeadBucket(ctx context.Context, bucket string) (cur2preflight.BucketAccess, error) {
	return client.preflight.HeadBucket(ctx, bucket)
}

func (client *Client) GetBucketPolicy(ctx context.Context, bucket string) (string, error) {
	return client.preflight.GetBucketPolicy(ctx, bucket)
}

func (client *Client) ListExecutions(ctx context.Context, exportARN string, token string) (cur2preflight.ExecutionPage, error) {
	return client.preflight.ListExecutions(ctx, exportARN, token)
}

func (client *Client) GetExecution(ctx context.Context, exportARN string, executionID string) (cur2preflight.Execution, error) {
	return client.preflight.GetExecution(ctx, exportARN, executionID)
}

func (client *Client) ListObjects(ctx context.Context, bucket string, prefix string, token string, maxKeys int32) (cur2preflight.ObjectPage, error) {
	return client.preflight.ListObjects(ctx, bucket, prefix, token, maxKeys)
}
