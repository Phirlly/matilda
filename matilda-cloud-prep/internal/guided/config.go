package guided

import (
	"context"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/billingguide"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

type AWSBillingGuide interface {
	DiscoverCredentialSources(context.Context) ([]billingguide.CredentialSource, error)
	VerifyIdentity(context.Context, billingguide.CredentialSource) (billingguide.VerifiedIdentity, error)
}

type AWSLoginRunner interface {
	SupportsLogin(context.Context) AWSLoginSupport
	Login(context.Context, billingguide.CredentialSource) error
}

type AWSLoginSupport struct {
	Available bool
	Version   string
	Reason    string
	Message   string
}

type Config struct {
	Registry       workflow.Registry
	AWSBilling     AWSBillingGuide
	AWSLogin       AWSLoginRunner
	TimeoutSeconds int
}
