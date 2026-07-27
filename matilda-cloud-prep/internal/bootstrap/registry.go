package bootstrap

import (
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func DefaultRegistry() workflow.Registry {
	registry, err := workflow.NewRegistry(workflow.Capability{
		Request: cur2preflight.AWSBillingPreflightRequest(),
		Runner:  cur2preflight.NewRunner(cur2preflight.RunnerConfig{}),
	})
	if err != nil {
		panic(err)
	}
	return registry
}
