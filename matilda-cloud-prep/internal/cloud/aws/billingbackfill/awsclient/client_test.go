package awsclient

import (
	"context"
	"errors"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingbackfill"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/aws/aws-sdk-go-v2/aws"
	awssupport "github.com/aws/aws-sdk-go-v2/service/support"
	supporttypes "github.com/aws/aws-sdk-go-v2/service/support/types"
	"github.com/aws/smithy-go"
)

func TestDescribeServicesAndSeverityLevelsMapSDKResponses(t *testing.T) {
	client := New(Config{SupportClient: &fakeSupport{}})

	services, err := client.DescribeServices(context.Background(), billingbackfill.DescribeServicesRequest{Language: "en"})
	if err != nil {
		t.Fatalf("DescribeServices returned error: %v", err)
	}
	if len(services) != 1 || services[0].Code != "billing" || services[0].Categories[0].Code != "cost-and-usage-reports" {
		t.Fatalf("services = %#v, want mapped billing service and category", services)
	}

	severities, err := client.DescribeSeverityLevels(context.Background(), billingbackfill.DescribeSeverityLevelsRequest{Language: "en"})
	if err != nil {
		t.Fatalf("DescribeSeverityLevels returned error: %v", err)
	}
	if len(severities) != 1 || severities[0].Code != "low" || severities[0].Name != "General guidance" {
		t.Fatalf("severities = %#v, want mapped low severity", severities)
	}
}

func TestEnsureSupportUsesConfiguredFactoryAndEndpointRegion(t *testing.T) {
	factory := &fakeFactory{support: &fakeSupport{}}
	client := New(Config{
		Profile: "default",
		Region:  "eu-frankfurt-1",
		LoadConfig: func(_ context.Context, request LoadRequest) (aws.Config, error) {
			if request.Profile != "default" {
				t.Fatalf("LoadConfig profile = %q, want default", request.Profile)
			}
			if request.Region != "us-east-1" {
				t.Fatalf("LoadConfig region = %q, want us-east-1 support endpoint fallback", request.Region)
			}
			return aws.Config{Region: request.Region}, nil
		},
		ClientFactory: factory,
	})

	_, err := client.DescribeSeverityLevels(context.Background(), billingbackfill.DescribeSeverityLevelsRequest{Language: "en"})
	if err != nil {
		t.Fatalf("DescribeSeverityLevels returned error: %v", err)
	}
	if factory.calls != 1 {
		t.Fatalf("factory calls = %d, want 1", factory.calls)
	}
	if supportRegionFor("us-west-2") != "us-west-2" || supportRegionFor("eu-west-1") != "eu-west-1" {
		t.Fatal("supportRegionFor did not preserve official Support API endpoint regions")
	}
}

func TestDefaultFactoryConstructsSupportClient(t *testing.T) {
	client := defaultFactory{}.SupportClient(aws.Config{Region: "us-east-1"})
	if client == nil {
		t.Fatal("defaultFactory returned nil support client")
	}
}

func TestEnsureSupportClassifiesLoadConfigFailure(t *testing.T) {
	client := New(Config{
		LoadConfig: func(context.Context, LoadRequest) (aws.Config, error) {
			return aws.Config{}, errors.New("load failed")
		},
		ClientFactory: &fakeFactory{support: &fakeSupport{}},
	})

	_, err := client.DescribeSeverityLevels(context.Background(), billingbackfill.DescribeSeverityLevelsRequest{Language: "en"})
	if err == nil {
		t.Fatal("DescribeSeverityLevels returned nil error")
	}
	var providerErr billingbackfill.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %#v, want ProviderError", err)
	}
	if providerErr.Code != "aws_support_api_unavailable" {
		t.Fatalf("ProviderError.Code = %q, want aws_support_api_unavailable", providerErr.Code)
	}
}

func TestDescribeCasesExcludesCommunicationsAndPaginates(t *testing.T) {
	supportClient := &fakeSupport{
		describeCasesOutputs: []*awssupport.DescribeCasesOutput{
			{
				Cases: []supporttypes.CaseDetails{{
					CaseId:    aws.String("case-1"),
					Subject:   aws.String("Request [ref]"),
					Status:    aws.String("opened"),
					DisplayId: aws.String("1001"),
				}},
				NextToken: aws.String("next"),
			},
			{
				Cases: []supporttypes.CaseDetails{{
					CaseId:  aws.String("case-2"),
					Subject: aws.String("Request [other]"),
					Status:  aws.String("opened"),
				}},
			},
		},
	}
	client := New(Config{SupportClient: supportClient})

	cases, err := client.DescribeCases(context.Background(), billingbackfill.DescribeCasesRequest{
		IncludeResolved:          false,
		IncludeCommunications:    false,
		IncludeCommunicationsSet: true,
		MaxResults:               100,
	})
	if err != nil {
		t.Fatalf("DescribeCases returned error: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("cases length = %d, want 2", len(cases))
	}
	if len(supportClient.describeCasesInputs) != 2 {
		t.Fatalf("DescribeCases calls = %d, want 2", len(supportClient.describeCasesInputs))
	}
	first := supportClient.describeCasesInputs[0]
	if first.IncludeCommunications == nil || *first.IncludeCommunications {
		t.Fatalf("IncludeCommunications = %#v, want explicit false", first.IncludeCommunications)
	}
	if first.IncludeResolvedCases {
		t.Fatal("IncludeResolvedCases = true, want false")
	}
	if first.MaxResults == nil || *first.MaxResults != 100 {
		t.Fatalf("MaxResults = %#v, want 100", first.MaxResults)
	}
	if supportClient.describeCasesInputs[1].NextToken == nil || *supportClient.describeCasesInputs[1].NextToken != "next" {
		t.Fatalf("second NextToken = %#v, want next", supportClient.describeCasesInputs[1].NextToken)
	}
}

func TestCreateCaseMapsSupportRequest(t *testing.T) {
	supportClient := &fakeSupport{
		createCaseOutput: &awssupport.CreateCaseOutput{CaseId: aws.String("case-created")},
	}
	client := New(Config{SupportClient: supportClient})

	result, err := client.CreateCase(context.Background(), billingbackfill.CreateCaseRequest{
		Language:     "en",
		IssueType:    "technical",
		ServiceCode:  "billing",
		CategoryCode: "cost-and-usage-reports",
		SeverityCode: "low",
		Subject:      "Request AWS CUR 2.0 backfill [ref]",
		Body:         "case body",
	})
	if err != nil {
		t.Fatalf("CreateCase returned error: %v", err)
	}
	if result.CaseID != "case-created" {
		t.Fatalf("CaseID = %q, want case-created", result.CaseID)
	}
	input := supportClient.createCaseInput
	if aws.ToString(input.Language) != "en" ||
		aws.ToString(input.IssueType) != "technical" ||
		aws.ToString(input.ServiceCode) != "billing" ||
		aws.ToString(input.CategoryCode) != "cost-and-usage-reports" ||
		aws.ToString(input.SeverityCode) != "low" ||
		aws.ToString(input.Subject) == "" ||
		aws.ToString(input.CommunicationBody) != "case body" {
		t.Fatalf("CreateCase input = %#v, want mapped support request", input)
	}
}

func TestSupportMethodErrorsAreClassified(t *testing.T) {
	apiErr := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	tests := []struct {
		name string
		run  func(*Client) error
	}{
		{
			name: "severity levels",
			run: func(client *Client) error {
				_, err := client.DescribeSeverityLevels(context.Background(), billingbackfill.DescribeSeverityLevelsRequest{Language: "en"})
				return err
			},
		},
		{
			name: "create case options",
			run: func(client *Client) error {
				_, err := client.DescribeCreateCaseOptions(context.Background(), billingbackfill.DescribeCreateCaseOptionsRequest{
					Language:     "en",
					IssueType:    "technical",
					ServiceCode:  "billing",
					CategoryCode: "cost-and-usage-reports",
				})
				return err
			},
		},
		{
			name: "describe cases",
			run: func(client *Client) error {
				_, err := client.DescribeCases(context.Background(), billingbackfill.DescribeCasesRequest{})
				return err
			},
		},
		{
			name: "create case",
			run: func(client *Client) error {
				_, err := client.CreateCase(context.Background(), billingbackfill.CreateCaseRequest{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(Config{SupportClient: &fakeSupport{
				describeSeverityErr:          apiErr,
				describeCreateCaseOptionsErr: apiErr,
				describeCasesErr:             apiErr,
				createCaseErr:                apiErr,
			}})
			err := tt.run(client)
			if err == nil {
				t.Fatal("method returned nil error")
			}
			var providerErr billingbackfill.ProviderError
			if !errors.As(err, &providerErr) || providerErr.Code != "aws_support_access_denied" {
				t.Fatalf("error = %#v, want aws_support_access_denied ProviderError", err)
			}
		})
	}
}

func TestDescribeCreateCaseOptionsRequiresAvailableLanguageAndCommunicationTypes(t *testing.T) {
	tests := []struct {
		name   string
		output *awssupport.DescribeCreateCaseOptionsOutput
		want   bool
	}{
		{
			name: "available with communication type",
			output: &awssupport.DescribeCreateCaseOptionsOutput{
				LanguageAvailability: aws.String("available"),
				CommunicationTypes:   []supporttypes.CommunicationTypeOptions{{Type: aws.String("web")}},
			},
			want: true,
		},
		{
			name: "best effort fails closed",
			output: &awssupport.DescribeCreateCaseOptionsOutput{
				LanguageAvailability: aws.String("best_effort"),
				CommunicationTypes:   []supporttypes.CommunicationTypeOptions{{Type: aws.String("web")}},
			},
		},
		{
			name: "no communication types fails closed",
			output: &awssupport.DescribeCreateCaseOptionsOutput{
				LanguageAvailability: aws.String("available"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(Config{SupportClient: &fakeSupport{createCaseOptionsOutput: tt.output}})
			options, err := client.DescribeCreateCaseOptions(context.Background(), billingbackfill.DescribeCreateCaseOptionsRequest{
				Language:     "en",
				IssueType:    "technical",
				ServiceCode:  "billing",
				CategoryCode: "cost-and-usage-reports",
			})
			if err != nil {
				t.Fatalf("DescribeCreateCaseOptions returned error: %v", err)
			}
			if options.Available != tt.want {
				t.Fatalf("Available = %v, want %v", options.Available, tt.want)
			}
		})
	}
}

func TestSupportAPIErrorsAreClassified(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "subscription required",
			err:  &smithy.GenericAPIError{Code: "SubscriptionRequiredException", Message: "not subscribed"},
			want: "aws_support_subscription_required",
		},
		{
			name: "access denied",
			err:  &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"},
			want: "aws_support_access_denied",
		},
		{
			name: "throttled",
			err:  &smithy.GenericAPIError{Code: "ThrottlingException", Message: "slow down"},
			want: "aws_support_api_unavailable",
		},
		{
			name: "generic api error",
			err:  &smithy.GenericAPIError{Code: "InternalServerError", Message: "try later"},
			want: "aws_support_api_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(Config{SupportClient: &fakeSupport{describeServicesErr: tt.err}})

			_, err := client.DescribeServices(context.Background(), billingbackfill.DescribeServicesRequest{Language: "en"})
			if err == nil {
				t.Fatal("DescribeServices returned nil error")
			}
			var providerErr billingbackfill.ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("error = %#v, want ProviderError", err)
			}
			if providerErr.Code != tt.want {
				t.Fatalf("ProviderError.Code = %q, want %s", providerErr.Code, tt.want)
			}
		})
	}
}

func TestNonAPIErrorsPassThroughForCallerClassification(t *testing.T) {
	plainErr := errors.New("plain failure")
	if got := classifySupportError(plainErr); !errors.Is(got, plainErr) {
		t.Fatalf("classifySupportError = %#v, want original non-API error", got)
	}
}

func TestPreflightMethodsDelegateToConfiguredClient(t *testing.T) {
	preflight := &fakePreflight{}
	client := New(Config{
		PreflightClient: preflight,
		SupportClient:   &fakeSupport{},
	})
	ctx := context.Background()

	if _, err := client.CheckConfiguration(ctx); err != nil {
		t.Fatalf("CheckConfiguration returned error: %v", err)
	}
	if _, err := client.GetCallerIdentity(ctx); err != nil {
		t.Fatalf("GetCallerIdentity returned error: %v", err)
	}
	if _, err := client.ListTables(ctx, ""); err != nil {
		t.Fatalf("ListTables returned error: %v", err)
	}
	if _, err := client.GetTable(ctx, "COST_AND_USAGE_REPORT", map[string]string{}); err != nil {
		t.Fatalf("GetTable returned error: %v", err)
	}
	if _, err := client.ListExports(ctx, ""); err != nil {
		t.Fatalf("ListExports returned error: %v", err)
	}
	if _, err := client.GetExport(ctx, "export-arn"); err != nil {
		t.Fatalf("GetExport returned error: %v", err)
	}
	if _, err := client.HeadBucket(ctx, "bucket"); err != nil {
		t.Fatalf("HeadBucket returned error: %v", err)
	}
	if _, err := client.GetBucketPolicy(ctx, "bucket"); err != nil {
		t.Fatalf("GetBucketPolicy returned error: %v", err)
	}
	if _, err := client.ListExecutions(ctx, "export-arn", ""); err != nil {
		t.Fatalf("ListExecutions returned error: %v", err)
	}
	if _, err := client.GetExecution(ctx, "export-arn", "execution"); err != nil {
		t.Fatalf("GetExecution returned error: %v", err)
	}
	if _, err := client.ListObjects(ctx, "bucket", "prefix", "", 1); err != nil {
		t.Fatalf("ListObjects returned error: %v", err)
	}
	if preflight.calls != 11 {
		t.Fatalf("preflight delegate calls = %d, want 11", preflight.calls)
	}
}

type fakeSupport struct {
	describeCasesInputs  []*awssupport.DescribeCasesInput
	describeCasesOutputs []*awssupport.DescribeCasesOutput

	createCaseInput  *awssupport.CreateCaseInput
	createCaseOutput *awssupport.CreateCaseOutput

	createCaseOptionsOutput      *awssupport.DescribeCreateCaseOptionsOutput
	describeServicesErr          error
	describeSeverityErr          error
	describeCreateCaseOptionsErr error
	describeCasesErr             error
	createCaseErr                error
}

type fakeFactory struct {
	support supportAPI
	calls   int
}

func (factory *fakeFactory) SupportClient(aws.Config) supportAPI {
	factory.calls++
	return factory.support
}

func (fake *fakeSupport) DescribeServices(context.Context, *awssupport.DescribeServicesInput, ...func(*awssupport.Options)) (*awssupport.DescribeServicesOutput, error) {
	if fake.describeServicesErr != nil {
		return nil, fake.describeServicesErr
	}
	return &awssupport.DescribeServicesOutput{
		Services: []supporttypes.Service{{
			Code: aws.String("billing"),
			Name: aws.String("Billing"),
			Categories: []supporttypes.Category{{
				Code: aws.String("cost-and-usage-reports"),
				Name: aws.String("Cost and Usage Reports"),
			}},
		}},
	}, nil
}

func (fake *fakeSupport) DescribeSeverityLevels(context.Context, *awssupport.DescribeSeverityLevelsInput, ...func(*awssupport.Options)) (*awssupport.DescribeSeverityLevelsOutput, error) {
	if fake.describeSeverityErr != nil {
		return nil, fake.describeSeverityErr
	}
	return &awssupport.DescribeSeverityLevelsOutput{
		SeverityLevels: []supporttypes.SeverityLevel{{Code: aws.String("low"), Name: aws.String("General guidance")}},
	}, nil
}

func (fake *fakeSupport) DescribeCreateCaseOptions(_ context.Context, input *awssupport.DescribeCreateCaseOptionsInput, _ ...func(*awssupport.Options)) (*awssupport.DescribeCreateCaseOptionsOutput, error) {
	if fake.describeCreateCaseOptionsErr != nil {
		return nil, fake.describeCreateCaseOptionsErr
	}
	if fake.createCaseOptionsOutput != nil {
		return fake.createCaseOptionsOutput, nil
	}
	return &awssupport.DescribeCreateCaseOptionsOutput{
		LanguageAvailability: aws.String("available"),
		CommunicationTypes:   []supporttypes.CommunicationTypeOptions{{Type: aws.String("web")}},
	}, nil
}

func (fake *fakeSupport) DescribeCases(_ context.Context, input *awssupport.DescribeCasesInput, _ ...func(*awssupport.Options)) (*awssupport.DescribeCasesOutput, error) {
	if fake.describeCasesErr != nil {
		return nil, fake.describeCasesErr
	}
	fake.describeCasesInputs = append(fake.describeCasesInputs, input)
	if len(fake.describeCasesOutputs) == 0 {
		return &awssupport.DescribeCasesOutput{}, nil
	}
	output := fake.describeCasesOutputs[0]
	fake.describeCasesOutputs = fake.describeCasesOutputs[1:]
	return output, nil
}

func (fake *fakeSupport) CreateCase(_ context.Context, input *awssupport.CreateCaseInput, _ ...func(*awssupport.Options)) (*awssupport.CreateCaseOutput, error) {
	if fake.createCaseErr != nil {
		return nil, fake.createCaseErr
	}
	fake.createCaseInput = input
	if fake.createCaseOutput != nil {
		return fake.createCaseOutput, nil
	}
	return &awssupport.CreateCaseOutput{CaseId: aws.String("case-created")}, nil
}

type fakePreflight struct {
	calls int
}

func (fake *fakePreflight) record() {
	fake.calls++
}

func (fake *fakePreflight) CheckConfiguration(context.Context) (cur2preflight.Configuration, error) {
	fake.record()
	return cur2preflight.Configuration{Region: "us-east-1"}, nil
}

func (fake *fakePreflight) GetCallerIdentity(context.Context) (cur2preflight.Identity, error) {
	fake.record()
	return cur2preflight.Identity{AccountID: "123456789012", CallerARN: "arn:aws:iam::123456789012:role/operator"}, nil
}

func (fake *fakePreflight) ListTables(context.Context, string) (cur2preflight.TablePage, error) {
	fake.record()
	return cur2preflight.TablePage{}, nil
}

func (fake *fakePreflight) GetTable(context.Context, string, map[string]string) (cur2preflight.Table, error) {
	fake.record()
	return cur2preflight.Table{}, nil
}

func (fake *fakePreflight) ListExports(context.Context, string) (cur2preflight.ExportPage, error) {
	fake.record()
	return cur2preflight.ExportPage{}, nil
}

func (fake *fakePreflight) GetExport(context.Context, string) (cur2preflight.Export, error) {
	fake.record()
	return cur2preflight.Export{}, nil
}

func (fake *fakePreflight) HeadBucket(context.Context, string) (cur2preflight.BucketAccess, error) {
	fake.record()
	return cur2preflight.BucketAccess{}, nil
}

func (fake *fakePreflight) GetBucketPolicy(context.Context, string) (string, error) {
	fake.record()
	return "{}", nil
}

func (fake *fakePreflight) ListExecutions(context.Context, string, string) (cur2preflight.ExecutionPage, error) {
	fake.record()
	return cur2preflight.ExecutionPage{}, nil
}

func (fake *fakePreflight) GetExecution(context.Context, string, string) (cur2preflight.Execution, error) {
	fake.record()
	return cur2preflight.Execution{}, nil
}

func (fake *fakePreflight) ListObjects(context.Context, string, string, string, int32) (cur2preflight.ObjectPage, error) {
	fake.record()
	return cur2preflight.ObjectPage{}, nil
}
