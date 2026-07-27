package bootstrap

import (
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight/awsclient"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

type RegistryConfig struct {
	AWSBillingPreflightClient cur2preflight.Client
}

func DefaultRegistry() workflow.Registry {
	return Registry(RegistryConfig{
		AWSBillingPreflightClient: awsclient.NewDefault(),
	})
}

func Registry(config RegistryConfig) workflow.Registry {
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: cur2preflight.AWSBillingPreflightRequest(),
		Runner: cur2preflight.NewRunner(cur2preflight.RunnerConfig{
			Client: config.AWSBillingPreflightClient,
		}),
	})
	if err != nil {
		panic(err)
	}
	return registry
}
