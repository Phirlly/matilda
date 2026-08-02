package bootstrap

import (
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingbackfill"
	billingbackfillawsclient "github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingbackfill/awsclient"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingcur2setup"
	billingcur2setupawsclient "github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingcur2setup/awsclient"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingprereqs"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	cur2preflightawsclient "github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight/awsclient"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/guided"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

type RegistryConfig struct {
	AWSBillingPreflightClient        cur2preflight.Client
	AWSBillingPreflightClientFactory func(workflow.ExecutionOptions) cur2preflight.Client
	AWSBillingBackfillClient         billingbackfill.Client
	AWSBillingBackfillClientFactory  func(workflow.ExecutionOptions) billingbackfill.Client
	AWSBillingSetupClient            billingcur2setup.Client
	AWSBillingSetupClientFactory     func(workflow.ExecutionOptions) billingcur2setup.Client
}

func DefaultRegistry() workflow.Registry {
	return Registry(RegistryConfig{
		AWSBillingPreflightClientFactory: defaultAWSBillingClientFactory(),
		AWSBillingBackfillClientFactory:  defaultAWSBillingBackfillClientFactory(),
		AWSBillingSetupClientFactory:     defaultAWSBillingSetupClientFactory(),
	})
}

func DefaultGuidedConfig(registry workflow.Registry) guided.Config {
	return guided.Config{
		Registry: registry,
		AWSBilling: billingguide.New(billingguide.Config{
			ClientFactory: defaultAWSBillingClientFactory(),
		}),
	}
}

func Registry(config RegistryConfig) workflow.Registry {
	registry, err := workflow.NewRegistry(
		workflow.Capability{
			Request: cur2preflight.AWSBillingPreflightRequest(),
			Runner: cur2preflight.NewRunner(cur2preflight.RunnerConfig{
				Client:        config.AWSBillingPreflightClient,
				ClientFactory: config.AWSBillingPreflightClientFactory,
			}),
		},
		workflow.Capability{
			Request: billingprereqs.AWSBillingApplyPrereqsRequest(),
			Runner: billingprereqs.NewRunner(billingprereqs.RunnerConfig{
				BackfillRunner: billingbackfill.NewRunner(billingbackfill.RunnerConfig{
					Client:        config.AWSBillingBackfillClient,
					ClientFactory: config.AWSBillingBackfillClientFactory,
				}),
				SetupRunner: billingcur2setup.NewRunner(billingcur2setup.RunnerConfig{
					Client:        config.AWSBillingSetupClient,
					ClientFactory: config.AWSBillingSetupClientFactory,
				}),
			}),
		},
	)
	if err != nil {
		panic(err)
	}
	return registry
}

func defaultAWSBillingClientFactory() func(workflow.ExecutionOptions) cur2preflight.Client {
	return func(options workflow.ExecutionOptions) cur2preflight.Client {
		awsOptions := workflow.AWSExecutionSelectors{}
		if options.Selectors != nil && options.Selectors.AWS != nil {
			awsOptions = *options.Selectors.AWS
		}
		return cur2preflightawsclient.New(cur2preflightawsclient.Config{
			Profile: awsOptions.Profile,
			Region:  awsOptions.Region,
		})
	}
}

func defaultAWSBillingBackfillClientFactory() func(workflow.ExecutionOptions) billingbackfill.Client {
	return func(options workflow.ExecutionOptions) billingbackfill.Client {
		awsOptions := workflow.AWSExecutionSelectors{}
		if options.Selectors != nil && options.Selectors.AWS != nil {
			awsOptions = *options.Selectors.AWS
		}
		return billingbackfillawsclient.New(billingbackfillawsclient.Config{
			Profile: awsOptions.Profile,
			Region:  awsOptions.Region,
		})
	}
}

func defaultAWSBillingSetupClientFactory() func(workflow.ExecutionOptions) billingcur2setup.Client {
	return func(options workflow.ExecutionOptions) billingcur2setup.Client {
		awsOptions := workflow.AWSExecutionSelectors{}
		if options.Selectors != nil && options.Selectors.AWS != nil {
			awsOptions = *options.Selectors.AWS
		}
		return billingcur2setupawsclient.New(billingcur2setupawsclient.Config{
			Profile: awsOptions.Profile,
			Region:  awsOptions.Region,
		})
	}
}
