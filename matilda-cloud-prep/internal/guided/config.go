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

type Config struct {
	Registry       workflow.Registry
	AWSBilling     AWSBillingGuide
	TimeoutSeconds int
}
