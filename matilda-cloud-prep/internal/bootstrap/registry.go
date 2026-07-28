package bootstrap

import (
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight/awsclient"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

type RegistryConfig struct {
	AWSBillingPreflightClient        cur2preflight.Client
	AWSBillingPreflightClientFactory func(workflow.ExecutionOptions) cur2preflight.Client
}

func DefaultRegistry() workflow.Registry {
	return Registry(RegistryConfig{
		AWSBillingPreflightClientFactory: func(options workflow.ExecutionOptions) cur2preflight.Client {
			awsOptions := workflow.AWSExecutionSelectors{}
			if options.Selectors != nil && options.Selectors.AWS != nil {
				awsOptions = *options.Selectors.AWS
			}
			return awsclient.New(awsclient.Config{
				Profile: awsOptions.Profile,
				Region:  awsOptions.Region,
			})
		},
	})
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
