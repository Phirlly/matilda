```markdown
# Matilda Linux Target Prep Ansible

Prepare Linux / Oracle Linux target VMs for Matilda Probe-based discovery.

This automation configures each target VM with:

- `matilda-svc` service account
- SSH public-key authentication
- Matilda sudoers allow-list
- local sudo validation
- Probe-to-target SSH/sudo validation

This project is for Linux / Oracle Linux / RHEL-like targets only.

Windows target setup is not included.

---

## What users need to edit

Most users only edit:

```text
inventory.yml
```

This file contains the target VM list and the IPs Matilda should discover.

Example public target:

```yaml
public_targets:
  hosts:
    my-public-target:
      ansible_host: <target-public-ip>
      public_ip: <target-public-ip>
      private_ip: <target-private-ip>
      discovery_ip: <target-private-or-public-ip-used-by-probe>
```

Example private-only target:

```yaml
private_targets:
  hosts:
    my-private-target:
      ansible_host: <target-private-ip>
      private_ip: <target-private-ip>
      discovery_ip: <target-private-ip>
```

Use `inventory.example.yml` for more examples.

---

## Runtime values

When you run a script, it asks for environment-specific values unless they are already provided in `.env`.

Required values:

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

## Public vs private discovery IP

Use private IPs for `discovery_ip` when MatildaProbeVM can reach them.

Recommended:

```text
discovery_ip = target private IP
```

Use public IPs only if:

- you intentionally want Matilda to discover public IPs
- MatildaProbeVM can reach those public IPs on TCP/22
- security rules allow it safely

---

## Run order

Run from the project root:

```bash
./scripts/run-preflight.sh
./scripts/run-setup.sh
./scripts/run-validate.sh
```

Do not skip steps.

---

## Step 1: Preflight

```bash
./scripts/run-preflight.sh
```

Checks:

- admin SSH works
- admin sudo works
- targets are Linux
- MatildaProbeVM has the discovery private key
- MatildaProbeVM can reach each `discovery_ip` on port 22

Preflight does not modify target VMs.

---

## Step 2: Setup

```bash
./scripts/run-setup.sh
```

Configures each target VM:

- creates `matilda-svc`
- installs the Matilda public key
- writes `/etc/sudoers.d/matilda-discovery`
- validates sudoers syntax

Setup modifies target VMs and asks for confirmation before running.

---

## Step 3: Validate

```bash
./scripts/run-validate.sh
```

Validates:

- local sudo works as `matilda-svc`
- Probe can SSH to each target as `matilda-svc`
- Probe can run a sudo discovery command remotely

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

Only use IPs from `validated-discovery-ips.txt` in Matilda Network Discovery.

---

## Matilda UI values after validation

In Matilda:

```text
Discovery Mode: Network Discovery
Network Address: IPs from reports/validated-discovery-ips.txt
Credential Group: <your Linux PEM credential group>
Probe: <your registered Matilda Probe>
Execution Mode: sudo
SNMP: No
Common login: Yes
Promote to discovery after precheck: Yes
Promote over-utilized resources: Yes
```

---

## Security notes

- Do not copy private keys to target VMs.
- Only the Matilda public key is installed on target VMs.
- The Matilda discovery private key is used only from MatildaProbeVM.
- The sudoers file is restricted to Matilda discovery commands.
- Prefer private IPs for discovery when possible.
- Review security lists / NSGs before using public IPs.

---

## Troubleshooting

### Admin SSH fails

Check:

- `ansible_host` in `inventory.yml`
- target admin SSH user
- target admin private key path
- security list / NSG for SSH

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

### SSH as `matilda-svc` fails

Check on the target VM:

```text
/home/matilda-svc/.ssh/authorized_keys
/home/matilda-svc/.ssh permissions = 700
authorized_keys permissions = 600
owner = matilda-svc:matilda-svc
```

### Sudo validation fails

Check on the target VM:

```bash
visudo -cf /etc/sudoers.d/matilda-discovery
```