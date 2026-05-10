# Matilda Discovery Readiness Toolkit

Prepare, validate, and report Linux target readiness for Matilda Probe-based discovery.

Start here:

```bash
./matilda-prep
```

The toolkit provides a guided terminal console, a local browser UI, inventory checks, Linux target setup and validation, rollback actions, and readiness reports.

## Supported Today

Validated in this release:

- Linux targets reached directly from the operator machine.
- Linux targets reached through MatildaProbeVM.
- Oracle Linux / RHEL-like systems as the validated Linux baseline.
- Probe-to-target SSH and sudo validation.
- Local inventory validation and CSV import for Linux targets.
- Local browser UI and terminal workflow.
- Readiness reports and validated discovery IP output.

Guidance only:

- Windows readiness package generation.
- UNIX admin instruction generation for planning.

Not automated in this release:

- Windows remote setup or validation.
- UNIX remote setup or validation.
- Cloud API readiness.
- Kubernetes API readiness.

## Safety

Matilda discovery is agentless and read-only.

This readiness toolkit is different during `setup`: it prepares Linux targets by creating or updating the `matilda-svc` service account, installing the Matilda discovery public key, and writing sudoers configuration.

Important rules:

- Do not copy private keys to target systems.
- Only the Matilda discovery public key is installed on targets.
- `setup` and rollback actions ask for confirmation before changing targets.
- Use a disposable or approved target set when testing rollback modes.

## Requirements

Operator machine:

- macOS or Linux with Bash.
- Go, when cloning and running from source.
- Ansible.
- SSH access to the target admin account.
- SSH access to MatildaProbeVM when private targets or Probe validation are used.

Windows operator machines are not validated in this release. Use a Linux or macOS operator machine. For WSL source checkouts, configure Go, Ansible, and SSH. For WSL release packages, use the Linux package and configure Ansible and SSH.

Linux targets:

- SSH reachable directly or through MatildaProbeVM.
- Admin account with non-interactive sudo for setup.
- Probe-to-target TCP/22 reachability.

MatildaProbeVM, when private targets or Probe validation are used:

- SSH reachable from the operator machine when private targets are used.
- Runs the registered Matilda Probe and can reach private targets.
- Has the Matilda discovery private key at the path configured in `.env`.
- Has `nc` or `ncat` available for Probe-to-target TCP checks.

## Quick Start

Clone the repository and enter the toolkit directory:

```bash
git clone https://github.com/Phirlly/matilda.git
cd matilda/matilda-discovery-readiness
```

When running from a source clone, `./matilda-prep` builds the local Go binary into `.bin/` automatically and then runs it. The source-clone path is the most portable way to run the toolkit when Go is installed.

Packaged release tarballs include the project files and a prebuilt `matilda-prep` binary for a specific operating system and CPU architecture. Extract the tarball, enter the extracted `matilda-discovery-readiness` directory, and run `./matilda-prep` from there. Standalone binary assets are not one-file installs; use them from a source checkout or extracted package root so the toolkit can find its Ansible, template, schema, and documentation files.

Create local configuration files:

```bash
./matilda-prep init
```

Run local checks:

```bash
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep status
```

Run the recommended Linux readiness workflow:

```bash
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep report
```

Open the local browser UI:

```bash
./matilda-prep ui
```

Use the printed local URL in your browser.

For a fresh-clone operator check, follow [docs/user/operator-smoke-test.md](docs/user/operator-smoke-test.md).

## Local Files

Most users only need these local files:

```text
.env
inventory.yml
```

Use the examples as a starting point:

```text
examples/env.example
examples/inventory.example.yml
examples/targets.example.csv
```

`./matilda-prep init` can create `.env` and `inventory.yml` safely. It asks before replacing existing files and can create timestamped backups.

Generated local output is written under:

```text
reports/
.matilda/
```

These local files can contain environment-specific information and should not be committed.

## Inventory Basics

Each Linux target has two important addresses:

```text
ansible_host  = address Ansible uses to configure the target
discovery_ip  = address MatildaProbeVM uses to discover and validate the target
```

Use `public_targets` when Ansible can connect directly from the operator machine.

Use `private_targets` when Ansible must connect through MatildaProbeVM.

Optional inventory helpers:

```bash
./matilda-prep inventory import examples/targets.example.csv
./matilda-prep inventory migrate
```

For full inventory guidance, see [docs/user/inventory.md](docs/user/inventory.md).

## Main Commands

Local checks:

```bash
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep status
```

Linux readiness:

```bash
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep report
```

Rollback:

```bash
./matilda-prep rollback --sudoers-only
./matilda-prep rollback --remove-key
./matilda-prep rollback --lock-user
./matilda-prep rollback --delete-user
```

Platform guidance:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

Interfaces:

```bash
./matilda-prep
./matilda-prep ui
./matilda-prep help
```

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

The browser UI is served locally by `matilda-prep`.

It shows:

- inventory, target, ready, and remediation metrics
- next recommended action
- workflow actions
- activity log
- target readiness rows
- validated IPs and report files
- recent runs

Remote browser actions require `.env` because the browser cannot collect interactive prompts. Mutating actions require explicit confirmation.

See [docs/user/browser-ui.md](docs/user/browser-ui.md).

## Terminal Console

Running `./matilda-prep` opens a guided action menu.

Use Up/Down or `k`/`j` to choose an action, Enter to run it, `r` to refresh, and `q` or Esc to quit.

After a command runs, the console switches to a result view. Use Up/Down, PageUp/PageDown, Home, and End to scroll output, then press `b` or Esc to return to the action menu.

## More Documentation

- [Documentation index](docs/README.md)
- [Quickstart](docs/user/quickstart.md)
- [Operator smoke test](docs/user/operator-smoke-test.md)
- [Linux workflow](docs/user/linux.md)
- [Inventory](docs/user/inventory.md)
- [Reports](docs/user/reports.md)
- [Browser UI](docs/user/browser-ui.md)
- [Troubleshooting](docs/user/troubleshooting.md)
- [Supported platforms](docs/reference/supported-platforms.md)
