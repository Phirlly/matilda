# Matilda Discovery Readiness Toolkit

Prepare, validate, and report readiness for Matilda Probe-based discovery.

The toolkit gives users one entrypoint:

```bash
./matilda-prep
```

It is being refactored into a modular Go + Ansible solution:

- Go provides the CLI, terminal console, local browser UI, inventory validation, workflow orchestration, and reports.
- Ansible provides remote target setup and validation.

## Naming

Use these names consistently:

```text
Repository:     matilda-discovery-readiness
Go module:      matilda-discovery-readiness
Binary:         matilda-prep
Product name:   Matilda Discovery Readiness Toolkit
User language:  target readiness, Probe readiness, platform readiness
```

The repository name describes the full readiness toolkit. User-facing workflow language should stay specific: Linux target readiness, Probe-to-target readiness, Windows/UNIX platform readiness, cloud API readiness, and Kubernetes readiness.

## What It Does Today

The implemented baseline is Linux target readiness for Matilda Discovery, focused on Oracle Linux / RHEL-like systems.

Current Linux workflow:

- checks local prerequisites
- validates inventory
- runs read-only preflight checks
- creates or updates the `matilda-svc` account
- installs the Matilda discovery public key
- writes and validates sudoers configuration
- validates local sudo
- validates Probe-to-target SSH and sudo
- writes readiness reports and validated discovery IPs

Release candidate validation has passed for Linux targets reachable directly and Linux targets reached through MatildaProbeVM. The tool can also generate local Windows readiness packages and UNIX admin instructions for platform planning. These generated files do not change targets. Windows, UNIX, cloud, and Kubernetes automation remains scaffolded or guidance-only until those modules are implemented.

## Important Safety Notes

Matilda discovery is agentless and read-only.

This readiness toolkit is different during `setup`: it intentionally modifies target systems by creating a service account, installing a public key, and writing sudoers configuration.

Do not copy private keys to target systems. Only the Matilda discovery public key is installed on targets.

Local runtime files are ignored by git:

```text
.env
inventory.yml
reports/
```

## Prerequisites

Install these on the machine where you run the toolkit:

- Go
- Ansible
- SSH access to the target admin account
- SSH access to MatildaProbeVM when private targets or Probe validation are used

For Probe-to-target TCP checks, MatildaProbeVM should have `nc` or `ncat` available.

## Quick Start

Open the Matilda Terminal Console:

```bash
./matilda-prep
```

Run the guided setup:

```bash
./matilda-prep init
```

Then run the recommended Linux workflow:

```bash
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep report
```

If `.env` and `inventory.yml` already exist, you can skip `init`.

For a non-interactive status summary:

```bash
./matilda-prep status
```

For the local browser interface:

```bash
./matilda-prep ui
```

Open the printed local URL in your browser.

For a fresh-clone operator check, see [docs/user/operator-smoke-test.md](docs/user/operator-smoke-test.md).

## Configuration Files

Most users only need two local files:

```text
.env
inventory.yml
```

Use examples as a starting point:

```text
examples/env.example
examples/inventory.example.yml
examples/targets.example.csv
```

`init` can create `.env` and `inventory.yml` safely. It asks before replacing existing files and can create timestamped backups.

## Inventory Basics

Each target has two important addresses:

```text
ansible_host  = address Ansible uses to configure the target
discovery_ip  = address MatildaProbeVM uses to discover and validate the target
```

Use `public_targets` when Ansible can connect directly from your machine.

Use `private_targets` when Ansible must connect through MatildaProbeVM.

For full inventory guidance, see [docs/user/inventory.md](docs/user/inventory.md).

## Main Commands

```text
./matilda-prep help
./matilda-prep init
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep inventory import examples/targets.example.csv
./matilda-prep inventory migrate
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep report
./matilda-prep status
./matilda-prep console
./matilda-prep ui
./matilda-prep rollback --sudoers-only
```

Additional rollback modes:

```text
./matilda-prep rollback --remove-key
./matilda-prep rollback --lock-user
./matilda-prep rollback --delete-user
```

Rollback commands are explicit and ask for confirmation before changing targets.

## Reports

Reports are written under `reports/`:

```text
reports/validated-discovery-ips.txt
reports/validation-summary.txt
reports/readiness.csv
reports/readiness.json
reports/readiness.md
reports/readiness.html
```

Use only IPs from `reports/validated-discovery-ips.txt` in Matilda Network Discovery.

Failed targets are excluded from the validated IP list and include remediation details in the reports.

For report details, see [docs/user/reports.md](docs/user/reports.md).

## Browser UI

The browser UI is served locally by the Go CLI and does not require a TypeScript or Node build chain.

It shows:

- inventory, target, ready, and remediation metrics
- next recommended action
- grouped workflow actions
- activity log
- target readiness rows
- validated IPs and report files

Remote browser actions require `.env` because the browser cannot collect interactive prompts. Mutating actions require explicit confirmation.

See [docs/user/browser-ui.md](docs/user/browser-ui.md).

## Terminal Console

Running `./matilda-prep` opens a simple guided action menu. It shows only the current readiness context, the recommended next step, and the actions users need to run.

Use Up/Down or `k`/`j` to choose an action, Enter to run it, `r` to refresh, and `q` or Esc to quit. After a command runs, the console switches to a full result view where users can scroll the output with Up/Down, PageUp/PageDown, Home, and End. Press `b` or Esc to return to the action menu. Mutating actions such as setup and rollback open a confirmation prompt before changing targets.

`./matilda-prep console` and `./matilda-prep start` open the same console.

Actions record their latest status in `.matilda/state.json`. That local state file is ignored and must not contain secrets, private keys, or copied credentials.

The longer-term console architecture is documented in [docs/reference/terminal-console-architecture.md](docs/reference/terminal-console-architecture.md).

## Platform Support

Implemented now:

- Linux target readiness workflow
- Oracle Linux / RHEL-like baseline
- public/direct targets
- private targets reached through MatildaProbeVM
- Probe-to-target SSH/sudo validation
- normalized v1 inventory execution for Linux targets
- local Windows readiness package generation and UNIX admin guidance generation

Structured for future modules:

- broader Linux distributions
- AIX, Solaris, and HP-UX workflows
- Windows readiness and setup workflows
- AWS, Azure, GCP, and OCI API readiness
- Kubernetes API readiness
- additional privilege methods such as `dzdo`, `pbrun`, and `suexec`

Future platform support should remain modular and should not be added directly into the Linux role.

See [docs/reference/supported-platforms.md](docs/reference/supported-platforms.md).

## Features Still Being Developed

The repository structure already anticipates these features, but they should be treated as in-progress unless documented otherwise:

- browser inventory editor/import preview
- richer terminal progress panes
- target detail views with remediation history
- validated UNIX remote automation
- validated Windows remote automation
- cloud API readiness modules
- Kubernetes readiness module

## Documentation

User docs:

- [Quickstart](docs/user/quickstart.md)
- [Operator smoke test](docs/user/operator-smoke-test.md)
- [Linux workflow](docs/user/linux.md)
- [Inventory](docs/user/inventory.md)
- [Reports](docs/user/reports.md)
- [Browser UI](docs/user/browser-ui.md)
- [Troubleshooting](docs/user/troubleshooting.md)

Reference docs:

- [Discovery access model](docs/reference/discovery-access-model.md)
- [Supported platforms](docs/reference/supported-platforms.md)
- [Privilege methods](docs/reference/privilege-methods.md)
- [Terminal console architecture](docs/reference/terminal-console-architecture.md)
- [Matilda discovery launch reference](docs/reference/matilda-discovery-launch.md)
- [Repository maintenance](docs/reference/repository-maintenance.md)
- [Browser live streaming design](docs/reference/browser-live-streaming.md)

`docs/matilda-docs-cache/` is local reference material only and is not tracked.

## Repository Layout

```text
matilda-prep                         CLI launcher
cmd/matilda-prep/                    Go entrypoint
internal/                            Go application packages, including console and workflow services
ansible/playbooks/                   Platform playbooks
ansible/roles/                       Modular Ansible roles
examples/                            Safe starter files
schemas/                             Inventory and report schemas
templates/                           Sudoers, PowerShell, and report templates
tests/unit/                          Go unit tests
tests/integration/                   Go integration tests
docs/                                User and reference documentation
reports/                             Local generated output, ignored by git
```

## Development Checks

Run tests with a local Go cache outside the repository if you want to avoid workspace residue:

```bash
GOCACHE=/private/tmp/matilda-prep-go-test-cache go test ./...
```

Run Ansible syntax checks with local temp paths:

```bash
ANSIBLE_CONFIG=$PWD/ansible/ansible.cfg \
ANSIBLE_LOCAL_TEMP=$PWD/.ansible/tmp \
ANSIBLE_SSH_CONTROL_PATH_DIR=/tmp/matilda-prep-cp \
ansible-playbook --syntax-check ansible/playbooks/linux/preflight.yml
```

The `matilda-prep` launcher builds the ignored local binary at `.bin/matilda-prep` before running it. Use `MATILDA_PREP_USE_BINARY=1` to reuse an existing built binary.
