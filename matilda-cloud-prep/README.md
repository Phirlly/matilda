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
Implemented provider paths run verified read-only checks. Unsupported or
unverified provider discovery, prerequisite creation, validation, and packaging
remain fail-closed until those paths are implemented from verified Matilda and
official provider references.

For AWS Rapid Assessment - Billing Based, guided mode discovers safe local AWS
credential sources, verifies the AWS caller with masked account evidence, and
asks before continuing with the selected account. When multiple verified
sources exist, the user can choose and confirm the intended account. If the
shown account is not the right one, guided mode lets the user choose another
discovered source, run an explicit in-flow AWS CLI browser login for a safe
discovered login-session profile with missing credentials, or wait while the
user signs in/configures another AWS profile outside the current prompt and
then re-scan safe local credential sources. Manual entry of an existing AWS
profile name is kept as an advanced fallback and requires confirmation because
normal AWS SDK credential resolution may run configured local credential
providers such as `credential_process`.

Guided mode never runs `aws login` automatically. When compatible AWS CLI login
support is available, a safe discovered login-session profile with missing
credentials can be selected for `aws login --profile <profile>` after an
explicit default-no confirmation. The AWS CLI may open a browser or print its
own login URL and prompts directly in the terminal, but Matilda Cloud Prep does
not capture, replay, read, cache, or package AWS login output or login cache
material. After the AWS CLI returns, guided mode re-scans credential sources
and still requires masked account confirmation before billing export
inspection. Direct commands never launch browser login. If AWS credential
environment variables would take precedence over a selected profile, the tool
fails closed and tells the user to unset those variables and restart before
retrying that profile.

After account confirmation, guided mode inspects AWS CUR 2.0 exports. If
usable exports are discovered, the user can continue with the detected export
or choose from the discovered safe refs. If none of the discovered exports is
the one the user wants, guided mode can prepare a new Matilda-managed CUR 2.0
setup plan for the connected AWS account. The recommended create-new path uses
a generated same-account S3 bucket and prefix. Guided mode can also list
existing S3 buckets owned by the connected AWS account and let the user select
one by safe generated bucket ref; users are not asked to invent or type bucket
names in the normal flow. If the selected destination can be verified safely,
guided mode shows the planned destination, bucket-policy, and CUR 2.0 export
changes before asking for approval. The plan remains
non-mutating until the user explicitly approves the current setup plan in
guided mode or reruns the create-new command with the returned plan ID and
approved mutating step IDs. Blocked setup plans stop safely and explain what
must be resolved before approval.

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

AWS Rapid Assessment - Billing Based preflight also supports direct selectors
for repeatable runs:

```bash
matilda-prep rapid-assessment billing aws preflight --profile default --region us-east-1
matilda-prep rapid-assessment billing aws preflight --export-ref cur2-abcdefghijklmnop
matilda-prep rapid-assessment billing aws preflight --timeout 5m
```

If more than one CUR 2.0 export is discovered, preflight fails closed and
returns safe generated export refs. Rerun with `--export-ref` to choose one.
Generated refs use `cur2-` plus lowercase `a` through `p` characters. Raw AWS
export ARNs are not printed as user-facing selectors.

AWS Rapid Assessment - Billing Based `apply-prereqs` uses explicit operations.
Without an operation flag, it returns guidance only.

```bash
matilda-prep rapid-assessment billing aws apply-prereqs \
  --export-ref cur2-abcdefghijklmnop \
  --request-backfill
matilda-prep rapid-assessment billing aws apply-prereqs --create-cur2-export
```

Cloud changes are two-step. First run the operation to review the generated
plan. Then rerun with the returned plan ID and only the step IDs from that
current plan that you approve.

```bash
matilda-prep rapid-assessment billing aws apply-prereqs \
  --export-ref cur2-abcdefghijklmnop \
  --request-backfill \
  --confirm-create-support-case \
  --approve-plan plan_abcdefghijklmnop \
  --approve-step aws.billing.cur2.previous_month_backfill_support_case
```

Creating a new AWS CUR 2.0 export uses the same plan-bound pattern:

```bash
matilda-prep rapid-assessment billing aws apply-prereqs \
  --create-cur2-export \
  --cur2-destination existing-same-account \
  --cur2-s3-bucket-ref s3b-abcdefghijklmnop \
  --approve-plan plan_abcdefghijklmnop \
  --approve-step <plan-step-id>
```

Repeat `--approve-step` for each mutating step ID returned by the current
plan.

`--request-backfill` by itself is plan-only. It does not create an AWS Support
case unless the confirmation and plan-bound approval flags are also supplied.
`--create-cur2-export` does not use `--export-ref`; it creates or reuses the
Matilda-managed CUR 2.0 setup plan for the connected AWS account. Use
`--cur2-destination existing-same-account` with a `--cur2-s3-bucket-ref`
returned by a prior create-new setup discovery result to target an existing
owned bucket. Without those destination flags, create-new setup uses the
recommended generated same-account Matilda bucket path.

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
