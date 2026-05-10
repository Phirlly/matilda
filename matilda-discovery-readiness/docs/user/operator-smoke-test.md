# Operator Smoke Test

Use this smoke test from a fresh clone or release package before a customer discovery session. It verifies that the local operator machine, local configuration, inventory, terminal workflow, and browser UI are usable before any target changes.

Supported today:

- Linux target readiness for direct targets.
- Linux target readiness for targets reached through MatildaProbeVM.
- Linux preflight, setup, validate, report, and explicit rollback modes.
- Local Windows readiness package generation and UNIX admin guidance generation.

Not automated in this release:

- Windows remote automation.
- UNIX remote automation.
- Cloud API readiness automation.
- Kubernetes readiness automation.

## 1. Prepare The Checkout

Use a Linux or macOS operator machine. Windows operator machines are not validated in this release. For WSL source checkouts, configure Go, Ansible, and SSH; for WSL release packages, use the Linux package and configure Ansible and SSH.

From a source checkout, install Go and Ansible on the operator machine. The `./matilda-prep` launcher builds the local Go binary into `.bin/` automatically before running it.

Source checkout:

```bash
git clone https://github.com/Phirlly/matilda.git
cd matilda/matilda-discovery-readiness
```

From a packaged release tarball that already includes a runnable binary for your operating system and CPU architecture, Go is not required. Extract the tarball, enter the extracted `matilda-discovery-readiness` directory, and run `./matilda-prep` from that directory:

```bash
tar -xzf matilda-discovery-readiness-<version>-<os>-<arch>.tar.gz
cd matilda-discovery-readiness
```

Standalone binary assets are not one-file installs. Use them from a source checkout or extracted package root so the toolkit can find its Ansible, template, schema, and documentation files.

## 2. Create Local Files

If you already have working local files from another checkout, copy them into the checkout or extracted package root:

```bash
cp /path/to/.env .env
cp /path/to/inventory.yml inventory.yml
```

Use the guided initializer:

```bash
./matilda-prep init
```

Or copy the examples and edit them:

```bash
cp examples/env.example .env
cp examples/inventory.example.yml inventory.yml
```

Before continuing, replace every placeholder value. Keep private key files on the operator machine or MatildaProbeVM only. Do not copy private keys to targets.

## 3. Run Local Smoke Checks

These commands should work before any remote target change:

```bash
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep status
```

Then start the browser UI:

```bash
./matilda-prep ui
```

Open the printed local URL, confirm the dashboard loads, then stop the server with Ctrl+C.

Expected result:

- `doctor` reports local prerequisites as passing.
- `inventory validate` reports the expected target count.
- `status` shows `Inventory OK`.
- `ui` prints a local browser URL and the dashboard loads without horizontal scrolling.

If `status` says reports are pending, that is normal before the first `validate` run.

## 4. Run Linux Readiness

Run the Linux workflow only after the local smoke checks pass:

```bash
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep report
```

`setup` modifies Linux targets. It asks for confirmation before creating or updating `matilda-svc`, installing the Matilda discovery public key, and writing sudoers configuration.

Expected result after `validate`:

- `./matilda-prep status` shows the ready count.
- `reports/validated-discovery-ips.txt` contains only validated discovery IPs.
- `reports/readiness.html` opens as the operator report.
- Failed targets have remediation in `reports/readiness.*`.

## 5. Rollback Smoke

Rollback modes are explicit and should be tested against disposable or approved targets before release:

```bash
./matilda-prep rollback --sudoers-only
./matilda-prep rollback --remove-key
./matilda-prep rollback --lock-user
./matilda-prep rollback --delete-user
```

Run one mode at a time. Restore disposable targets with `./matilda-prep setup` and `./matilda-prep validate` after rollback testing if they will be reused.

## 6. Common Fixes

- Missing `.env`: run `./matilda-prep init` or copy `examples/env.example` to `.env`.
- Missing `inventory.yml`: run `./matilda-prep init` or copy `examples/inventory.example.yml` to `inventory.yml`.
- Placeholder inventory values: edit `ansible_host`, `discovery_ip`, and related target fields.
- Missing key files: fix the path in `.env` and rerun `./matilda-prep doctor`.
- Missing Ansible: install Ansible on the operator machine and rerun `./matilda-prep doctor`.
- Probe cannot reach target TCP/22: check route tables, security lists, NSGs, and target firewalls.
- Target SSH or sudo fails: fix admin SSH access and non-interactive sudo, then rerun `preflight`.
