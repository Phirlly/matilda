package billingguide

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/authdiscovery"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/cloud/aws/cur2preflight"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func TestProviderDiscoversEnvironmentAndProfileSourcesSafely(t *testing.T) {
	provider := New(Config{
		EnvLookup: envLookup(map[string]string{"AWS_ACCESS_KEY_ID": "test-access-key"}),
		Discoverer: fakeDiscoverer{profiles: []authdiscovery.Profile{
			{Name: "default", Region: "us-east-1", HasLoginSession: true},
			{Name: "prod", Region: "us-west-2", HasSSOSession: true, HasCredentialProcess: true},
		}},
	})

	sources, err := provider.DiscoverCredentialSources(context.Background())

	if err != nil {
		t.Fatalf("DiscoverCredentialSources returned error: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("sources = %#v, want env plus two profiles", sources)
	}
	if sources[0].Kind != CredentialSourceEnvironment {
		t.Fatalf("first source kind = %q, want environment", sources[0].Kind)
	}
	if sources[1].Profile != "default" || !sources[1].HasLoginSession {
		t.Fatalf("default source = %#v, want login-session profile", sources[1])
	}
	if sources[2].Profile != "prod" || !sources[2].HasSSOSession || !sources[2].HasCredentialProcess {
		t.Fatalf("prod source = %#v, want SSO/process metadata", sources[2])
	}
	for _, forbidden := range []string{"test-access-key", "secret", "/Users/", "arn:aws"} {
		if strings.Contains(strings.ToLower(printableSources(sources)), strings.ToLower(forbidden)) {
			t.Fatalf("sources leaked forbidden value %q: %#v", forbidden, sources)
		}
	}
}

func TestProviderDiscoverCredentialSourcesFailsClosedOnContextOrDiscovererError(t *testing.T) {
	t.Run("context cancelled before discovery", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		discoverer := &recordingDiscoverer{}
		provider := New(Config{Discoverer: discoverer})

		_, err := provider.DiscoverCredentialSources(ctx)

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context canceled", err)
		}
		if discoverer.called {
			t.Fatal("discoverer should not run after context cancellation")
		}
	})

	t.Run("discoverer error is returned without adding sources", func(t *testing.T) {
		wantErr := errors.New("profile parser failed")
		provider := New(Config{
			EnvLookup:  envLookup(map[string]string{"AWS_ACCESS_KEY_ID": "test-access-key"}),
			Discoverer: fakeDiscoverer{err: wantErr},
		})

		sources, err := provider.DiscoverCredentialSources(context.Background())

		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want discoverer error", err)
		}
		if len(sources) != 0 {
			t.Fatalf("sources = %#v, want none when discovery fails", sources)
		}
	})
}

func TestProviderDiscoversOnlySafeProfileMetadata(t *testing.T) {
	provider := New(Config{
		EnvLookup: envLookup(map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": "/tmp/token"}),
		Discoverer: fakeDiscoverer{profiles: []authdiscovery.Profile{
			{Name: "safe", Region: "arn:aws:iam::123456789012:role/operator"},
			{Name: "prod/unsafe", Region: "us-east-1"},
			{Name: "123456789012", Region: "us-east-1"},
			{Name: "AKIAIOSFODNN7EXAMPLE", Region: "us-east-1"},
			{Name: "alpha", Region: "us-east-2", HasRoleARN: true, HasSourceProfile: true},
		}},
	})

	sources, err := provider.DiscoverCredentialSources(context.Background())

	if err != nil {
		t.Fatalf("DiscoverCredentialSources returned error: %v", err)
	}
	got := printableSources(sources)
	for _, forbidden := range []string{"prod/unsafe", "arn:aws", "123456789012", "AKIA", "/tmp/token"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sources leaked forbidden or unsafe value %q: %#v", forbidden, sources)
		}
	}
	if len(sources) != 3 {
		t.Fatalf("sources = %#v, want environment plus two safe profile entries", sources)
	}
	if sources[0].Kind != CredentialSourceEnvironment {
		t.Fatalf("first source = %#v, want environment source sorted first", sources[0])
	}
	if sources[1].Profile != "alpha" || sources[1].Region != "us-east-2" || !sources[1].HasRoleARN || !sources[1].HasSourceProfile {
		t.Fatalf("alpha source = %#v, want safe metadata preserved", sources[1])
	}
	if sources[2].Profile != "safe" || sources[2].Region != "" {
		t.Fatalf("safe source = %#v, want unsafe region redacted", sources[2])
	}
}

func TestProviderVerifiesIdentityWithSelectedProfileAndMasksCaller(t *testing.T) {
	var gotOptions workflow.ExecutionOptions
	provider := New(Config{
		ClientFactory: func(options workflow.ExecutionOptions) cur2preflight.Client {
			gotOptions = options
			return fakeIdentityClient{
				config:   cur2preflight.Configuration{Region: "us-west-2"},
				identity: cur2preflight.Identity{AccountID: "123456789012", CallerARN: "arn:aws:iam::123456789012:role/operator"},
			}
		},
	})
	source := CredentialSource{Kind: CredentialSourceProfile, Profile: "default", Region: "us-west-2"}

	identity, err := provider.VerifyIdentity(context.Background(), source)

	if err != nil {
		t.Fatalf("VerifyIdentity returned error: %v", err)
	}
	if gotOptions.Selectors == nil || gotOptions.Selectors.AWS == nil {
		t.Fatalf("options selectors missing: %#v", gotOptions)
	}
	if gotOptions.Selectors.AWS.Profile != "default" || gotOptions.Selectors.AWS.Region != "us-west-2" {
		t.Fatalf("AWS selectors = %#v, want profile and region", gotOptions.Selectors.AWS)
	}
	if identity.AccountLabel != "account-ending-9012" {
		t.Fatalf("AccountLabel = %q, want masked account", identity.AccountLabel)
	}
	if !strings.HasPrefix(identity.CallerRef, "sha256:") {
		t.Fatalf("CallerRef = %q, want hash ref", identity.CallerRef)
	}
	for _, forbidden := range []string{"123456789012", "arn:aws", "operator"} {
		if strings.Contains(printableIdentity(identity), forbidden) {
			t.Fatalf("identity leaked forbidden value %q: %#v", forbidden, identity)
		}
	}
}

func TestProviderVerifyIdentityFailsClosedForClientConfigurationBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		provider      Provider
		source        CredentialSource
		wantCode      string
		factoryCalled bool
	}{
		{
			name:     "missing client factory",
			provider: New(Config{}),
			source:   CredentialSource{Kind: CredentialSourceEnvironment},
			wantCode: "aws_provider_capability_blocked",
		},
		{
			name: "nil client",
			provider: New(Config{ClientFactory: func(workflow.ExecutionOptions) cur2preflight.Client {
				return nil
			}}),
			source:        CredentialSource{Kind: CredentialSourceEnvironment},
			wantCode:      "aws_provider_capability_blocked",
			factoryCalled: true,
		},
		{
			name: "unsafe selector",
			provider: New(Config{ClientFactory: func(workflow.ExecutionOptions) cur2preflight.Client {
				t.Fatal("client factory should not run for unsafe selector")
				return nil
			}}),
			source:   CredentialSource{Kind: CredentialSourceProfile, Profile: "/Users/example/.aws/private"},
			wantCode: "aws_config_invalid_selector",
		},
		{
			name: "account id looking selector",
			provider: New(Config{ClientFactory: func(workflow.ExecutionOptions) cur2preflight.Client {
				t.Fatal("client factory should not run for account-id-looking selector")
				return nil
			}}),
			source:   CredentialSource{Kind: CredentialSourceProfile, Profile: "123456789012"},
			wantCode: "aws_config_invalid_selector",
		},
		{
			name: "access key looking selector",
			provider: New(Config{ClientFactory: func(workflow.ExecutionOptions) cur2preflight.Client {
				t.Fatal("client factory should not run for access-key-looking selector")
				return nil
			}}),
			source:   CredentialSource{Kind: CredentialSourceProfile, Profile: "AKIAIOSFODNN7EXAMPLE"},
			wantCode: "aws_config_invalid_selector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.provider.VerifyIdentity(context.Background(), tt.source)

			var verificationErr VerificationError
			if !errors.As(err, &verificationErr) {
				t.Fatalf("error %T %[1]v is not VerificationError", err)
			}
			if verificationErr.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", verificationErr.Code, tt.wantCode)
			}
			for _, forbidden := range []string{"/Users/", "arn:aws", "access_key", "secret_key"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("error leaked forbidden value %q: %v", forbidden, err)
				}
			}
		})
	}
}

func TestProviderVerifyIdentityFailsClosedWhenConfigurationOrIdentityFails(t *testing.T) {
	tests := []struct {
		name   string
		client cur2preflight.Client
		code   string
	}{
		{
			name:   "configuration",
			client: fakeIdentityClient{configErr: cur2preflight.NewProviderError("aws_config_missing_credentials", "credentials unavailable")},
			code:   "aws_config_missing_credentials",
		},
		{
			name:   "identity",
			client: fakeIdentityClient{config: cur2preflight.Configuration{Region: "us-east-1"}, identityErr: errors.New("raw arn:aws failure")},
			code:   "aws_auth_failed",
		},
		{
			name:   "incomplete identity",
			client: fakeIdentityClient{config: cur2preflight.Configuration{Region: "us-east-1"}, identity: cur2preflight.Identity{AccountID: "123456789012"}},
			code:   "aws_identity_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := New(Config{
				ClientFactory: func(workflow.ExecutionOptions) cur2preflight.Client {
					return tt.client
				},
			})

			_, err := provider.VerifyIdentity(context.Background(), CredentialSource{Kind: CredentialSourceEnvironment})

			var verificationErr VerificationError
			if !errors.As(err, &verificationErr) {
				t.Fatalf("error %T %[1]v is not VerificationError", err)
			}
			if verificationErr.Code != tt.code {
				t.Fatalf("Code = %q, want %q", verificationErr.Code, tt.code)
			}
			for _, forbidden := range []string{"arn:aws", "123456789012", "raw"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("error leaked forbidden value %q: %v", forbidden, err)
				}
			}
		})
	}
}

func TestProviderVerifyIdentityUsesSafeProviderMessagesAndUnknownShortAccounts(t *testing.T) {
	t.Run("provider messages are mapped to safe remediation text", func(t *testing.T) {
		tests := []struct {
			code        string
			wantMessage string
		}{
			{code: "aws_config_missing_region", wantMessage: "AWS Region is not configured."},
			{code: "aws_config_profile_shadowed", wantMessage: "AWS profile selection is blocked because credential environment variables would take precedence."},
			{code: "aws_config_timeout", wantMessage: "AWS SDK configuration timed out."},
			{code: "aws_config_cancelled", wantMessage: "AWS SDK configuration was cancelled."},
		}

		for _, tt := range tests {
			t.Run(tt.code, func(t *testing.T) {
				provider := New(Config{
					ClientFactory: func(workflow.ExecutionOptions) cur2preflight.Client {
						return fakeIdentityClient{configErr: cur2preflight.NewProviderError(tt.code, "raw arn:aws:iam::123456789012:role/operator")}
					},
				})

				_, err := provider.VerifyIdentity(context.Background(), CredentialSource{Kind: CredentialSourceEnvironment})

				var verificationErr VerificationError
				if !errors.As(err, &verificationErr) {
					t.Fatalf("error %T %[1]v is not VerificationError", err)
				}
				if verificationErr.Code != tt.code || verificationErr.Message != tt.wantMessage {
					t.Fatalf("verification error = %#v, want code %q and safe message %q", verificationErr, tt.code, tt.wantMessage)
				}
				if strings.Contains(err.Error(), "arn:aws") || strings.Contains(err.Error(), "123456789012") {
					t.Fatalf("error leaked raw provider message: %v", err)
				}
			})
		}
	})

	t.Run("short account identifiers are not displayed", func(t *testing.T) {
		provider := New(Config{
			ClientFactory: func(workflow.ExecutionOptions) cur2preflight.Client {
				return fakeIdentityClient{
					config:   cur2preflight.Configuration{Region: "us-east-1"},
					identity: cur2preflight.Identity{AccountID: "12", CallerARN: "arn:aws:iam::12:role/operator"},
				}
			},
		})

		identity, err := provider.VerifyIdentity(context.Background(), CredentialSource{Kind: CredentialSourceEnvironment})

		if err != nil {
			t.Fatalf("VerifyIdentity returned error: %v", err)
		}
		if identity.AccountLabel != "account-ending-unknown" {
			t.Fatalf("AccountLabel = %q, want account-ending-unknown", identity.AccountLabel)
		}
		if !strings.HasPrefix(identity.CallerRef, "sha256:") {
			t.Fatalf("CallerRef = %q, want hash ref", identity.CallerRef)
		}
	})
}

func TestVerificationErrorFormatsCodeOnlyWhenMessageIsEmpty(t *testing.T) {
	err := VerificationError{Code: "aws_auth_failed"}

	if err.Error() != "aws_auth_failed" {
		t.Fatalf("Error() = %q, want code only", err.Error())
	}
}

type fakeDiscoverer struct {
	profiles []authdiscovery.Profile
	err      error
}

func (f fakeDiscoverer) Discover(context.Context) ([]authdiscovery.Profile, error) {
	return f.profiles, f.err
}

type recordingDiscoverer struct {
	called bool
}

func (f *recordingDiscoverer) Discover(context.Context) ([]authdiscovery.Profile, error) {
	f.called = true
	return nil, nil
}

type fakeIdentityClient struct {
	config      cur2preflight.Configuration
	configErr   error
	identity    cur2preflight.Identity
	identityErr error
}

func (f fakeIdentityClient) CheckConfiguration(context.Context) (cur2preflight.Configuration, error) {
	return f.config, f.configErr
}

func (f fakeIdentityClient) GetCallerIdentity(context.Context) (cur2preflight.Identity, error) {
	return f.identity, f.identityErr
}

func (f fakeIdentityClient) ListTables(context.Context, string) (cur2preflight.TablePage, error) {
	panic("not used")
}

func (f fakeIdentityClient) GetTable(context.Context, string, map[string]string) (cur2preflight.Table, error) {
	panic("not used")
}

func (f fakeIdentityClient) ListExports(context.Context, string) (cur2preflight.ExportPage, error) {
	panic("not used")
}

func (f fakeIdentityClient) GetExport(context.Context, string) (cur2preflight.Export, error) {
	panic("not used")
}

func (f fakeIdentityClient) HeadBucket(context.Context, string) (cur2preflight.BucketAccess, error) {
	panic("not used")
}

func (f fakeIdentityClient) GetBucketPolicy(context.Context, string) (string, error) {
	panic("not used")
}

func (f fakeIdentityClient) ListExecutions(context.Context, string, string) (cur2preflight.ExecutionPage, error) {
	panic("not used")
}

func (f fakeIdentityClient) GetExecution(context.Context, string, string) (cur2preflight.Execution, error) {
	panic("not used")
}

func (f fakeIdentityClient) ListObjects(context.Context, string, string, string, int32) (cur2preflight.ObjectPage, error) {
	panic("not used")
}

func envLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func printableSources(sources []CredentialSource) string {
	var b strings.Builder
	for _, source := range sources {
		b.WriteString(string(source.Kind))
		b.WriteString(source.Profile)
		b.WriteString(source.Region)
	}
	return b.String()
}

func printableIdentity(identity VerifiedIdentity) string {
	return identity.AccountLabel + identity.CallerRef + identity.Region + identity.Source.Profile
}
