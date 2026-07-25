# Matilda Cloud Prep

`matilda-cloud-prep` is the project for a planned production Go CLI named
`matilda-prep`.

The tool prepares cloud-platform-side prerequisites for Matilda SaaS onboarding
across AWS, Azure, and Google Cloud. It is intended to help customers and
operators set up, validate, and package the cloud-side inputs needed before
using Matilda SaaS for Rapid Assessment or Deep Discovery workflows.

This project does not automate Matilda SaaS portal steps.

## Assessment Paths

The CLI will use Matilda assessment terms directly.

### Rapid Assessment

Rapid Assessment has two preparation paths:

- `Billing Based`: prepares exported billing data for assessment.
- `API Based`: prepares cloud API access so Matilda discovery jobs can complete
  before assessment.

API Based may include cost or billing prerequisites where required by the
provider and Matilda workflow. Matilda prerequisite guidance identifies asset
and cost discovery as prerequisites, and describes API-based assessment as using
discovered assets, billing data, and utilization metrics for the assessment
period.

For this project, API Based Rapid Assessment prepares access for provider-native
cloud monitoring or usage signals where the Matilda/provider path supports that
collection. It must not be described as OS-level, guest-level, memory,
process-level, service-level, or application-level deep inspection unless that
deeper scope is separately verified.

### Deep Discovery

Deep Discovery is provider-specific and will be implemented only where Matilda
documentation and official cloud provider documentation verify the required
access model and API surface.

For GCP, the verified API-based Deep Discovery preparation path is cloud account
API onboarding followed by the Matilda discovery lifecycle: Precheck, Inventory
/ Asset Discovery, Cost Collection, and Optimization. Assessment/report
generation is separate and should start only after asset and cost discovery
complete.

Unsupported or unverified provider paths must fail closed with a clear message.

## Planned CLI Shape

The planned Rapid Assessment command grammar is:

```bash
matilda-prep <provider> rapid-assessment <billing|api> <action>
```

Rapid Assessment providers:

```text
aws
azure
gcp
```

The planned Deep Discovery command grammar is provider-specific:

```bash
matilda-prep <provider> deep-discovery <action>
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
| `preflight` | No | Checks whether the selected provider scope is ready for the requested Matilda path before setup. | Readiness report with pass/warn/fail checks, missing permissions, missing APIs or services, billing/export blockers, and planned prerequisites. |
| `apply-prereqs` | Yes | Creates or updates only verified cloud-side prerequisites for the selected path. | Change report showing created, updated, skipped, and already-correct resources, plus safe evidence and rollback notes where applicable. |
| `validate` | No | Confirms the configured cloud-side prerequisites work after setup. | Validation report showing whether identity, scope, API access, billing export access, and provider-specific checks satisfy the selected Matilda path. |
| `package` | Local files only | Builds a whitelisted handoff artifact from safe local evidence. | Local manifest or archive containing values and proof needed for Matilda SaaS onboarding, excluding credentials, private keys, tokens, raw logs, live inventory, customer data, and cloud state. |

`preflight` answers "what is missing before setup?" `validate` answers "does
the completed setup now satisfy the prerequisite path?"

Examples:

```bash
matilda-prep gcp rapid-assessment billing preflight
matilda-prep gcp rapid-assessment api validate
matilda-prep gcp deep-discovery preflight
matilda-prep aws rapid-assessment billing package
matilda-prep azure rapid-assessment api preflight
```

## Implementation Direction

The normal implementation path will use official Go SDKs:

- [Google Cloud Go client libraries](https://docs.cloud.google.com/go/docs/reference)
- [AWS SDK for Go v2](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/welcome.html)
- [Azure SDK for Go](https://learn.microsoft.com/en-us/azure/developer/go/overview)

Cloud provider CLIs such as `aws`, `az`, and `gcloud` may be useful diagnostic
fallbacks later, but they should not be required for normal automation.

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

This project is currently in template-readiness stage.

The next implementation slice is expected to add the Go module, CLI scaffold,
initial command contract tests, and CI validation for Go.

Until then, the only tracked project file is this README. Project workflow and
reference notes are local-only under `docs/`.
