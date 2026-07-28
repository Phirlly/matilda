package bootstrap

import (
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight/awsclient"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/guided"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

type RegistryConfig struct {
	AWSBillingPreflightClient        cur2preflight.Client
	AWSBillingPreflightClientFactory func(workflow.ExecutionOptions) cur2preflight.Client
}

func DefaultRegistry() workflow.Registry {
	return Registry(RegistryConfig{
		AWSBillingPreflightClientFactory: defaultAWSBillingClientFactory(),
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
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: cur2preflight.AWSBillingPreflightRequest(),
		Runner: cur2preflight.NewRunner(cur2preflight.RunnerConfig{
			Client:        config.AWSBillingPreflightClient,
			ClientFactory: config.AWSBillingPreflightClientFactory,
		}),
	})
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
		return awsclient.New(awsclient.Config{
			Profile: awsOptions.Profile,
			Region:  awsOptions.Region,
		})
	}
}
