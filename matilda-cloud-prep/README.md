# Matilda Cloud Prep

`matilda-cloud-prep` is the project for a planned production Go CLI named
`matilda-prep`.

The tool prepares cloud-platform-side prerequisites for Matilda SaaS onboarding
across AWS, Azure, Google Cloud, and OCI. It is intended to help customers and
operators set up, validate, and package the cloud-side inputs needed before
using Matilda SaaS for Rapid Assessment or Deep Discovery workflows.

This project does not automate Matilda SaaS portal steps.

## Assessment Paths

The CLI will use Matilda assessment terms directly.

### Rapid Assessment

Rapid Assessment has two preparation paths:

- `Rapid Assessment - Billing Based`: prepares exported billing data for
  assessment.
- `Rapid Assessment - API Based`: prepares cloud API access so Matilda
  discovery jobs can complete before assessment.

Rapid Assessment - API Based may include cost or billing prerequisites where
required by the provider and Matilda workflow. Matilda prerequisite guidance
identifies asset and cost discovery as prerequisites, and describes API-based
assessment as using discovered assets, billing data, and utilization metrics
for the assessment period.

For this project, Rapid Assessment - API Based prepares access for
provider-native cloud monitoring or usage signals where the Matilda/provider
path supports that collection. It must not be described as OS-level,
guest-level, memory, process-level, service-level, or application-level deep
inspection unless that deeper scope is separately verified.

### Deep Discovery

Deep Discovery is provider-specific and will be implemented only where Matilda
documentation and official cloud provider documentation verify the required
access model and API surface.

For GCP, the verified API-based Deep Discovery preparation path is cloud account
API onboarding followed by the Matilda discovery lifecycle: Precheck, Inventory
/ Asset Discovery, Cost Collection, and Optimization. Assessment/report
generation is separate and should start only after asset and cost discovery
complete.

For OCI, Deep Discovery preparation is limited to the verified cloud-account
scope. It must not imply guest OS, service dependency, database, application,
or Kubernetes-specific automation unless that deeper scope is separately
verified.

Unsupported or unverified provider paths must fail closed with a clear message.

## Planned User Experience

The current scaffold provides a guided entrypoint:

```bash
matilda-prep start
```

The guided flow asks for the Matilda outcome first, then the cloud provider,
and prints the correct objective-first `preflight` command for that path.
Provider-specific discovery, prerequisite creation, validation, and packaging
remain fail-closed until those paths are implemented from verified Matilda and
official provider references.

Future provider workflows should inspect the connected environment before
recommending coverage and changes. Customers should not need to understand
provider hierarchy terms, billing export internals, IAM/RBAC details, or
storage configuration before the tool has inspected what exists.

## Planned Direct Commands

Direct commands should use the same outcome-first order as the guided flow.
They are for repeatable automation, support, and testable command contracts.

The planned Rapid Assessment command grammar is:

```bash
matilda-prep rapid-assessment <billing|api> <provider> <action>
```

Rapid Assessment providers:

```text
aws
azure
gcp
oci
```

The planned Deep Discovery command grammar is provider-specific:

```bash
matilda-prep deep-discovery <provider> <action>
```

Deep Discovery support must be explicitly verified per provider. GCP API-based
Deep Discovery is included as cloud discovery lifecycle preparation; it does not
imply OS-level service discovery unless that scope is separately verified.

Actions:

```text
preflight
apply-prereqs
validate
package
```

## Action Model

Every provider path uses the same action names and safety contract.

| Action | Cloud Mutation | Purpose | Result |
| --- | --- | --- | --- |
| `preflight` | No | Checks whether the selected coverage is ready for the requested Matilda path before setup. | Readiness report with pass/warn/fail checks, missing permissions, missing APIs or services, billing/export blockers, and planned prerequisites. |
| `apply-prereqs` | Yes | Creates or updates only verified cloud-side prerequisites for the selected path. | Change report showing created, updated, skipped, and unchanged resources, plus safe evidence and rollback notes where applicable. |
| `validate` | No | Confirms the configured cloud-side prerequisites work after setup. | Validation report showing whether identity, coverage, API access, billing export access, and provider-specific checks satisfy the selected Matilda path. |
| `package` | Local files only | Builds a whitelisted handoff artifact from safe local evidence. | For the first scaffold, a provider-neutral `minimal_v0` manifest only. Future archives require an approved package schema. Credentials, private keys, tokens, raw logs, live inventory, customer data, and cloud state are excluded. |

`preflight` answers "what is missing before setup?" `validate` answers "does
the completed setup now satisfy the prerequisite path?"

Examples:

```bash
matilda-prep rapid-assessment billing gcp preflight
matilda-prep rapid-assessment api gcp validate
matilda-prep deep-discovery gcp preflight
matilda-prep rapid-assessment billing aws package
matilda-prep rapid-assessment api azure preflight
```

## Implementation Direction

The normal implementation path will use official Go SDKs:

- [Google Cloud Go client libraries](https://docs.cloud.google.com/go/docs/reference)
- [AWS SDK for Go v2](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/welcome.html)
- [Azure SDK for Go](https://learn.microsoft.com/en-us/azure/developer/go/overview)
- [OCI SDK for Go](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/gosdk.htm)

Cloud provider CLIs such as `aws`, `az`, `gcloud`, and `oci` may be useful
diagnostic fallbacks later, but they should not be required for normal
automation.

The planned architecture keeps the CLI entrypoint thin, cloud SDK calls inside
provider adapters, and provider-neutral workflow logic testable without live
cloud services.

## Safety And Data Handling

The tool must be conservative by default:

- run preflight and safety checks before applying cloud changes;
- keep apply operations idempotent;
- clearly separate preflight, apply, validate, and package actions;
- redact secrets from output, logs, errors, and generated package manifests;
- avoid storing credentials in generated artifacts;
- keep generated handoff packages whitelist-based;
- fail closed for unsupported or unverified provider behavior.

Do not commit credentials, private keys, tokens, live inventory, runtime state,
logs, or generated customer handoff packages.

## Repository Status

This project is in early scaffold stage.

The initial Go module, provider-neutral CLI contract, fail-closed workflow
registry, minimal manifest model, redaction behavior, and Go CI gate are being
introduced first. Provider-specific automation will be added only after the
corresponding Matilda and official cloud-provider references are verified for
that provider path.

The Go CI gate runs formatting, tests with coverage, vet, build, and a 95%
minimum coverage floor for the current scaffold.

Project workflow and reference notes remain local-only under `docs/`.
