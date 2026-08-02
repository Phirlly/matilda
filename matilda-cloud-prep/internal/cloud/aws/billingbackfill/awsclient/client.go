package awsclient

import (
	"context"
	"errors"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingbackfill"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	cur2awsclient "github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight/awsclient"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssupport "github.com/aws/aws-sdk-go-v2/service/support"
	supporttypes "github.com/aws/aws-sdk-go-v2/service/support/types"
	"github.com/aws/smithy-go"
)

const defaultSupportRegion = "us-east-1"

type Config struct {
	Profile         string
	Region          string
	PreflightClient cur2preflight.Client
	SupportClient   supportAPI
	LoadConfig      func(context.Context, LoadRequest) (aws.Config, error)
	ClientFactory   ClientFactory
}

type LoadRequest struct {
	Profile string
	Region  string
}

type ClientFactory interface {
	SupportClient(aws.Config) supportAPI
}

type supportAPI interface {
	DescribeServices(context.Context, *awssupport.DescribeServicesInput, ...func(*awssupport.Options)) (*awssupport.DescribeServicesOutput, error)
	DescribeSeverityLevels(context.Context, *awssupport.DescribeSeverityLevelsInput, ...func(*awssupport.Options)) (*awssupport.DescribeSeverityLevelsOutput, error)
	DescribeCreateCaseOptions(context.Context, *awssupport.DescribeCreateCaseOptionsInput, ...func(*awssupport.Options)) (*awssupport.DescribeCreateCaseOptionsOutput, error)
	DescribeCases(context.Context, *awssupport.DescribeCasesInput, ...func(*awssupport.Options)) (*awssupport.DescribeCasesOutput, error)
	CreateCase(context.Context, *awssupport.CreateCaseInput, ...func(*awssupport.Options)) (*awssupport.CreateCaseOutput, error)
}

type Client struct {
	profile   string
	region    string
	preflight cur2preflight.Client
	support   supportAPI
	loader    func(context.Context, LoadRequest) (aws.Config, error)
	factory   ClientFactory
}

func New(config Config) *Client {
	loader := config.LoadConfig
	if loader == nil {
		loader = func(ctx context.Context, request LoadRequest) (aws.Config, error) {
			options := []func(*awsconfig.LoadOptions) error{
				awsconfig.WithRegion(request.Region),
			}
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
		profile:   config.Profile,
		region:    config.Region,
		preflight: preflight,
		support:   config.SupportClient,
		loader:    loader,
		factory:   factory,
	}
}

func (client *Client) DescribeServices(ctx context.Context, request billingbackfill.DescribeServicesRequest) ([]billingbackfill.SupportService, error) {
	if err := client.ensureSupport(ctx); err != nil {
		return nil, err
	}
	output, err := client.support.DescribeServices(ctx, &awssupport.DescribeServicesInput{
		Language: aws.String(request.Language),
	})
	if err != nil {
		return nil, classifySupportError(err)
	}
	if output == nil {
		return nil, billingbackfill.NewProviderError("aws_support_api_unavailable", "AWS Support DescribeServices response was empty.")
	}
	services := make([]billingbackfill.SupportService, 0, len(output.Services))
	for _, service := range output.Services {
		categories := make([]billingbackfill.SupportCategory, 0, len(service.Categories))
		for _, category := range service.Categories {
			categories = append(categories, billingbackfill.SupportCategory{
				Code: aws.ToString(category.Code),
				Name: aws.ToString(category.Name),
			})
		}
		services = append(services, billingbackfill.SupportService{
			Code:       aws.ToString(service.Code),
			Name:       aws.ToString(service.Name),
			Categories: categories,
		})
	}
	return services, nil
}

func (client *Client) DescribeSeverityLevels(ctx context.Context, request billingbackfill.DescribeSeverityLevelsRequest) ([]billingbackfill.SupportSeverity, error) {
	if err := client.ensureSupport(ctx); err != nil {
		return nil, err
	}
	output, err := client.support.DescribeSeverityLevels(ctx, &awssupport.DescribeSeverityLevelsInput{
		Language: aws.String(request.Language),
	})
	if err != nil {
		return nil, classifySupportError(err)
	}
	if output == nil {
		return nil, billingbackfill.NewProviderError("aws_support_api_unavailable", "AWS Support DescribeSeverityLevels response was empty.")
	}
	levels := make([]billingbackfill.SupportSeverity, 0, len(output.SeverityLevels))
	for _, level := range output.SeverityLevels {
		levels = append(levels, billingbackfill.SupportSeverity{
			Code: aws.ToString(level.Code),
			Name: aws.ToString(level.Name),
		})
	}
	return levels, nil
}

func (client *Client) DescribeCreateCaseOptions(ctx context.Context, request billingbackfill.DescribeCreateCaseOptionsRequest) (billingbackfill.SupportCreateCaseOptions, error) {
	if err := client.ensureSupport(ctx); err != nil {
		return billingbackfill.SupportCreateCaseOptions{}, err
	}
	output, err := client.support.DescribeCreateCaseOptions(ctx, &awssupport.DescribeCreateCaseOptionsInput{
		Language:     aws.String(request.Language),
		IssueType:    aws.String(request.IssueType),
		ServiceCode:  aws.String(request.ServiceCode),
		CategoryCode: aws.String(request.CategoryCode),
	})
	if err != nil {
		return billingbackfill.SupportCreateCaseOptions{}, classifySupportError(err)
	}
	if output == nil {
		return billingbackfill.SupportCreateCaseOptions{}, billingbackfill.NewProviderError("aws_support_api_unavailable", "AWS Support DescribeCreateCaseOptions response was empty.")
	}
	return billingbackfill.SupportCreateCaseOptions{
		Available: strings.EqualFold(aws.ToString(output.LanguageAvailability), "available") && len(output.CommunicationTypes) > 0,
	}, nil
}

func (client *Client) DescribeCases(ctx context.Context, request billingbackfill.DescribeCasesRequest) ([]billingbackfill.SupportCase, error) {
	if err := client.ensureSupport(ctx); err != nil {
		return nil, err
	}

	includeCommunications := request.IncludeCommunications
	maxResults := request.MaxResults
	if maxResults == 0 {
		maxResults = 100
	}
	input := &awssupport.DescribeCasesInput{
		CaseIdList:           append([]string(nil), request.CaseIDs...),
		IncludeResolvedCases: request.IncludeResolved,
		Language:             aws.String("en"),
		MaxResults:           aws.Int32(maxResults),
	}
	if request.IncludeCommunicationsSet {
		input.IncludeCommunications = aws.Bool(includeCommunications)
	}

	cases := []billingbackfill.SupportCase{}
	for page := 0; page < 10; page++ {
		output, err := client.support.DescribeCases(ctx, input)
		if err != nil {
			return nil, classifySupportError(err)
		}
		if output == nil {
			return nil, billingbackfill.NewProviderError("aws_support_describe_cases_failed", "AWS Support DescribeCases response was empty.")
		}
		cases = append(cases, mapSupportCases(output.Cases)...)
		if output.NextToken == nil || aws.ToString(output.NextToken) == "" {
			return cases, nil
		}
		input.NextToken = output.NextToken
	}
	return nil, billingbackfill.NewProviderError("aws_support_describe_cases_failed", "AWS Support case pagination did not converge.")
}

func (client *Client) CreateCase(ctx context.Context, request billingbackfill.CreateCaseRequest) (billingbackfill.CreateCaseResult, error) {
	if err := client.ensureSupport(ctx); err != nil {
		return billingbackfill.CreateCaseResult{}, err
	}
	output, err := client.support.CreateCase(ctx, &awssupport.CreateCaseInput{
		Language:          aws.String(request.Language),
		IssueType:         aws.String(request.IssueType),
		ServiceCode:       aws.String(request.ServiceCode),
		CategoryCode:      aws.String(request.CategoryCode),
		SeverityCode:      aws.String(request.SeverityCode),
		Subject:           aws.String(request.Subject),
		CommunicationBody: aws.String(request.Body),
	})
	if err != nil {
		return billingbackfill.CreateCaseResult{}, classifySupportError(err)
	}
	if output == nil {
		return billingbackfill.CreateCaseResult{}, billingbackfill.NewProviderError("aws_support_create_case_response_incomplete", "AWS Support CreateCase response was empty.")
	}
	if strings.TrimSpace(aws.ToString(output.CaseId)) == "" {
		return billingbackfill.CreateCaseResult{}, billingbackfill.NewProviderError("aws_support_create_case_response_incomplete", "AWS Support CreateCase response did not include a case ID.")
	}
	return billingbackfill.CreateCaseResult{CaseID: aws.ToString(output.CaseId)}, nil
}

func (client *Client) ensureSupport(ctx context.Context) error {
	if client.support != nil {
		return nil
	}
	region := supportRegionFor(client.region)
	config, err := client.loader(ctx, LoadRequest{
		Profile: client.profile,
		Region:  region,
	})
	if err != nil {
		return billingbackfill.NewProviderError("aws_support_api_unavailable", "AWS Support SDK configuration could not be loaded.")
	}
	if config.Region == "" {
		config.Region = region
	}
	client.support = client.factory.SupportClient(config)
	return nil
}

func supportRegionFor(region string) string {
	switch strings.TrimSpace(region) {
	case "us-east-1", "us-west-2", "eu-west-1":
		return strings.TrimSpace(region)
	default:
		return defaultSupportRegion
	}
}

func mapSupportCases(input []supporttypes.CaseDetails) []billingbackfill.SupportCase {
	cases := make([]billingbackfill.SupportCase, 0, len(input))
	for _, supportCase := range input {
		cases = append(cases, billingbackfill.SupportCase{
			CaseID:    aws.ToString(supportCase.CaseId),
			DisplayID: aws.ToString(supportCase.DisplayId),
			Subject:   aws.ToString(supportCase.Subject),
			Status:    aws.ToString(supportCase.Status),
		})
	}
	return cases
}

func classifySupportError(err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "SubscriptionRequiredException":
			return billingbackfill.NewProviderError("aws_support_subscription_required", "AWS Support API access requires an eligible support plan.")
		case "AccessDeniedException", "AccessDenied", "UnauthorizedOperation":
			return billingbackfill.NewProviderError("aws_support_access_denied", "AWS Support API access was denied.")
		case "ThrottlingException", "Throttling", "TooManyRequestsException":
			return billingbackfill.NewProviderError("aws_support_api_unavailable", "AWS Support API is currently throttling requests.")
		default:
			return billingbackfill.NewProviderError("aws_support_api_unavailable", "AWS Support API request failed.")
		}
	}
	return err
}

type defaultFactory struct{}

func (defaultFactory) SupportClient(config aws.Config) supportAPI {
	return awssupport.NewFromConfig(config)
}

var _ billingbackfill.Client = (*Client)(nil)
