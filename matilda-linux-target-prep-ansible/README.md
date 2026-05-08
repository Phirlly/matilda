# Matilda Linux Target Prep Ansible

Prepare Linux targets for Matilda Probe-based discovery.

This project automates the Linux target preparation steps documented for Matilda Probe-based discovery:

- create the `matilda-svc` service account
- install SSH public-key authentication for `matilda-svc`
- configure the Matilda sudoers allow-list
- validate sudoers syntax
- validate local sudo as `matilda-svc`
- validate Probe-to-target SSH/sudo

This project currently supports Linux targets, with implementation and testing focused on Oracle Linux / RHEL-like systems.

Windows, AIX, Solaris, and HP-UX setup are not included in the current implementation.

---

## Important security note

Matilda discovery is agentless and read-only.

This preparation automation is different: it modifies target VMs during setup by creating a service account, installing an SSH public key, and writing a sudoers file.

Do not copy the Matilda discovery private key to target VMs.

Only the Matilda discovery public key is installed on target VMs.

---

## What you need before running

You need:

- Ansible installed on the machine where you run this project, including the `ansible.posix` collection
- SSH access from this machine to MatildaProbeVM
- SSH access from this machine to any targets listed under `public_targets`
- target admin user with non-interactive sudo access
- network access from MatildaProbeVM to each target `discovery_ip` on TCP/22
- Matilda discovery public key on this machine
- matching Matilda discovery private key already available on MatildaProbeVM
- Matilda discovery private key on MatildaProbeVM with permission `600`
- `nc` or `ncat` available on MatildaProbeVM for TCP/22 reachability checks

For Oracle Linux / RHEL Probe hosts, `nc` is commonly provided by `nmap-ncat`.

This project uses `ansible.posix.authorized_key` to manage the `matilda-svc` SSH public key. If your Ansible installation does not include the `ansible.posix` collection, install it before running setup:

```bash
ansible-galaxy collection install ansible.posix
```

The setup wrapper checks for this dependency before modifying targets and returns a user-friendly error if it is missing.

The validation workflow prefers `ifconfig` because the Matilda prerequisite PDF uses `ifconfig` in the sample validation command. If `ifconfig` is not installed, validation uses the documented `ip addr show` command as a fallback and records that fallback in the validation summary.

---

## What users usually edit

Most users only edit:

```text
inventory.yml
```

This file contains the target VM list and the IPs Matilda should discover.

Use `inventory.example.yml` as the safe example.

Do not distribute an `inventory.yml` that contains real customer or lab IP addresses.

`inventory.yml` is local-only and ignored by git.

---

## Inventory model

Each target uses two important addresses:

```text
ansible_host = address Ansible uses to configure the target
discovery_ip = address MatildaProbeVM uses to discover and validate the target
```

Use private IPs for `discovery_ip` when MatildaProbeVM can reach them.

Use public IPs only if:

- you intentionally want Matilda to discover public IPs
- MatildaProbeVM can reach those public IPs on TCP/22
- security rules allow it safely

---

## Public target example

Use `public_targets` when Ansible can connect directly from your machine to the target.

```yaml
all:
  children:
    public_targets:
      hosts:
        my-public-target:
          ansible_host: <target-public-ip>
          public_ip: <target-public-ip>
          private_ip: <target-private-ip>
          discovery_ip: <target-private-or-public-ip-used-by-probe>
```

---

## Private target example

Use `private_targets` when Ansible must connect to the target through MatildaProbeVM.

```yaml
all:
  children:
    private_targets:
      hosts:
        my-private-target:
          ansible_host: <target-private-ip>
          private_ip: <target-private-ip>
          discovery_ip: <target-private-ip>
```

Private targets are reached through MatildaProbeVM using the SSH proxy configuration in `group_vars/private_targets.yml`.

---

## Runtime values

When you run a script, it asks for environment-specific values unless they are already provided in `.env`.

Required runtime values:

```text
Target admin SSH user
Target admin private key path
MatildaProbeVM SSH host/IP
MatildaProbeVM SSH user
MatildaProbeVM admin private key path
Matilda discovery public key path on this machine
Matilda discovery private key path on MatildaProbeVM
```

These values are not hardcoded in the playbooks.

---

## Key types

### Target admin private key

Used by Ansible to SSH into target VMs as the admin user.

Example:

```text
~/.ssh/<target-admin-private-key>
```

### Probe admin private key

Used by Ansible to SSH into MatildaProbeVM.

Example:

```text
~/.ssh/<probe-admin-private-key>
```

This may be the same as the target admin key in simple labs, but it is a separate input.

### Matilda discovery public key

Installed on target VMs for the `matilda-svc` account.

Example:

```text
~/.ssh/MatildaProbeKey.pem.pub
```

### Matilda discovery private key on Probe

Stored on MatildaProbeVM and used by the Probe to SSH into targets as `matilda-svc`.

Example:

```text
/home/opc/.ssh/MatildaProbeKey.pem
```

Expected permission on the Probe:

```text
600
```

Do not copy this private key to target VMs.

---

## Optional `.env` file

You do not have to create `.env`.

If `.env` does not exist, the scripts prompt for required values.

If you do not want to type the same values every time, copy:

```bash
cp .env.example .env
```

Then edit `.env` with your values.

Example `.env` values:

```bash
TARGET_ADMIN_USER=opc
TARGET_ADMIN_PRIVATE_KEY_FILE=/path/to/target-admin-private-key
MATILDA_PROBE_ANSIBLE_HOST=<probe-public-ip-or-hostname>
MATILDA_PROBE_USER=opc
MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE=/path/to/probe-admin-private-key
MATILDA_PUBLIC_KEY_FILE=/path/to/MatildaProbeKey.pem.pub
MATILDA_PROBE_PRIVATE_KEY_ON_PROBE=/home/opc/.ssh/MatildaProbeKey.pem
```

`.env` is ignored by git and should not be committed.

---

## Run order

Preferred workflow:

```bash
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
```

Direct script workflow:

```bash
./scripts/run-preflight.sh
./scripts/run-setup.sh
./scripts/run-validate.sh
```

Do not skip steps.

For a one-shot lab workflow, you can run:

```bash
./matilda-prep run
```

The one-shot workflow still runs setup confirmation before modifying target VMs.

---

## Step 1: Preflight

Preferred:

```bash
./matilda-prep preflight
```

Direct script:

```bash
./scripts/run-preflight.sh
```

Preflight checks:

- admin SSH works
- admin sudo works
- targets are Linux
- MatildaProbeVM has the discovery private key
- MatildaProbeVM discovery private key permission is `600`
- MatildaProbeVM has `nc` or `ncat`
- MatildaProbeVM can reach each `discovery_ip` on TCP/22

Preflight is intended to be read-only and does not modify target VMs.

---

## Step 2: Setup

Preferred:

```bash
./matilda-prep setup
```

Alias:

```bash
./matilda-prep apply
```

Direct script:

```bash
./scripts/run-setup.sh
```

Before setup runs, the wrapper checks that the local Ansible environment can resolve `ansible.posix.authorized_key`. If it cannot, setup stops before modifying targets and prints a clear fix.

Setup configures each target VM by:

- creating the `matilda-svc` group
- creating the `matilda-svc` user
- creating `/home/matilda-svc/.ssh`
- creating `/home/matilda-svc/.ansible/tmp`
- installing the Matilda discovery public key
- writing `/etc/sudoers.d/matilda-discovery`
- validating sudoers syntax with `visudo`

Setup modifies target VMs and asks for confirmation before running.

---

## Step 3: Validate

Preferred:

```bash
./matilda-prep validate
```

Direct script:

```bash
./scripts/run-validate.sh
```

Validation checks:

- local sudo works as `matilda-svc`
- an unapproved sudo command is denied
- Probe can SSH to each target as `matilda-svc`
- Probe can run a sudo discovery command remotely

For the main network validation command, the playbook prefers `ifconfig`. If `ifconfig` is missing, it uses `ip addr show` as a documented fallback and records this in the validation summary.

Only targets that pass validation should be used for Matilda Network Discovery.

---

## Wrapper help

Show available wrapper commands:

```bash
./matilda-prep help
```

Available commands:

```text
help
preflight
setup
apply
validate
run
```

---

## Output files

Validated discovery IPs:

```text
reports/validated-discovery-ips.txt
```

Validation summary:

```text
reports/validation-summary.txt
```

The validation summary includes:

```text
Host
DiscoveryIP
Command
FallbackUsed
LocalSudo
DeniedCommand
ProbeSSH
Ready
Remediation
```

Use only IPs from `reports/validated-discovery-ips.txt` in Matilda Network Discovery.

Failed targets are not added to `reports/validated-discovery-ips.txt`.

Report `.txt` files are ignored by git.

---

## Matilda UI values after validation

In Matilda, use the validated IPs from this project when creating or running Network Discovery.

Typical values:

```text
Discovery Mode: Network Discovery
Network Address: IPs from reports/validated-discovery-ips.txt
Credential Group: <your Linux PEM credential group for matilda-svc>
Probe: <your registered Matilda Probe>
Execution Mode: sudo
Common login: Yes, when all listed targets use the same credential
SNMP: Based on customer requirements
```

Matilda Network Discovery requires a valid Credential Group and a selected Probe. For SaaS environments, use the registered Probe instead of the default Local_Probe.

Other UI options may vary by project or customer requirements.

---

## Security notes

- Do not copy private keys to target VMs.
- Only the Matilda discovery public key is installed on target VMs.
- The Matilda discovery private key is used only from MatildaProbeVM.
- The `matilda-svc` account is dedicated to Matilda discovery.
- The sudoers file is restricted to Matilda-documented discovery commands.
- The sudoers file uses `!requiretty` for non-interactive automation.
- The sudoers file uses a restricted `secure_path`.
- Prefer private IPs for discovery when possible.
- Review security lists, NSGs, route tables, and firewalls before using public IPs.

---

## Troubleshooting

### `./matilda-prep` returns permission denied

The wrapper must be executable.

Run this from the project root:

```bash
chmod +x matilda-prep
```

Then retry the command.

### Admin SSH fails

Check:

- `ansible_host` in `inventory.yml`
- target admin SSH user
- target admin private key path
- security list / NSG for SSH
- target OS firewall

### Admin sudo fails

The target admin user must be able to run sudo non-interactively because the playbooks use Ansible privilege escalation.

Check the target admin account sudo configuration before rerunning preflight.

### Ansible cannot resolve `ansible.posix.authorized_key`

The setup playbook uses `ansible.posix.authorized_key` to manage the `matilda-svc` SSH public key.

If Ansible cannot resolve this module, the `ansible.posix` collection is missing from the Ansible environment where you run this project.

Install it, then rerun setup:

```bash
ansible-galaxy collection install ansible.posix
```

### Probe SSH fails

Check:

- Probe host/IP
- Probe SSH user
- Probe admin private key path
- security list / NSG for SSH to Probe

### Probe cannot reach target on port 22

Check:

- `discovery_ip`
- route tables
- security lists / NSGs
- target OS firewall
- whether MatildaProbeVM can route to the target network

### Probe port check fails because `nc` / `ncat` is missing

The preflight check uses `nc` or `ncat` on MatildaProbeVM to test TCP/22 reachability.

For Oracle Linux / RHEL Probe hosts, install or verify the package that provides `nc` / `ncat`, commonly `nmap-ncat`.

### SSH as `matilda-svc` fails

Check on the target VM:

```text
/home/matilda-svc/.ssh/authorized_keys
/home/matilda-svc/.ssh permissions = 700
authorized_keys permissions = 600
owner = matilda-svc:matilda-svc
```

Also confirm the public key installed on the target matches the private key on MatildaProbeVM.

### Sudo validation fails

Check on the target VM:

```bash
visudo -cf /etc/sudoers.d/matilda-discovery
```

Also confirm the sudoers file contains the Matilda discovery command allow-list and the `matilda-svc` account is included through `MATILDA_USER`.

### Unapproved sudo command is not denied

Validation expects an unapproved sudo command such as `/bin/rm` to be denied.

If this check fails, review the target sudo configuration and confirm `matilda-svc` is restricted to the Matilda discovery allow-list.

### `ifconfig` is missing

The validation playbook prefers `ifconfig` because the Matilda prerequisite PDF uses `sudo /sbin/ifconfig` in the sample validation command.

If `ifconfig` is missing, validation uses `ip addr show` as a documented fallback.

The validation summary records this as:

```text
FallbackUsed=YES
```

If strict parity with the PDF sample command is required, install the package that provides `ifconfig`, commonly `net-tools` on minimal Oracle Linux / RHEL images.

---

## Current scope

Included now:

```text
Linux target preparation
Oracle Linux / RHEL-like target workflow
Public targets reachable directly from the operator machine
Private targets reachable through MatildaProbeVM
Probe-to-target SSH/sudo validation
```

Not included now:

```text
Windows target setup
AIX target setup
Solaris target setup
HP-UX target setup
Database credential onboarding
Matilda UI automation
Automatic discovery task launch
```

Future support for UNIX and Windows should use separate OS-specific workflows instead of being added directly into the Linux playbooks.