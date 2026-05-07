# Matilda Linux Target Prep Ansible

This project prepares Linux / Oracle Linux target VMs for Matilda Probe-based discovery.

It follows the Matilda Discovery Probe service-account requirements:

- create `matilda-svc`
- install SSH public key
- configure Matilda sudoers allow-list
- validate local sudo
- validate Probe-to-target SSH and sudo

## What this is for

Use this project when you need to prepare multiple Linux / Oracle Linux target VMs so Matilda can discover them through a registered Matilda Discovery Probe.

This project is for Linux / Oracle Linux / RHEL-like targets only.

Windows target setup is not included.

---

## Required user-edited files

Most users only edit:

```text
inventory.yml
```

This file contains the target VM list and discovery IPs.

Users must also have a Matilda discovery public key file available somewhere on their local machine. The path is provided at runtime.

Example:

```text
/path/to/MatildaProbeKey.pem.pub
```

---

## Runtime inputs

When you run a script, you will be prompted for:

```text
Target admin SSH user
Target admin private key path
MatildaProbeVM SSH host/IP
MatildaProbeVM SSH user
MatildaProbeVM admin private key path
Matilda discovery public key path on this machine
Matilda discovery private key path on MatildaProbeVM
```

These are intentionally runtime prompts so users do not need to edit Ansible variable files.

---

## Key types explained

### Target admin private key

Used by Ansible to SSH into target VMs as the admin user, usually `opc` in OCI.

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

This may be the same file as the target admin key in simple labs, but it is a separate input.

### Matilda discovery public key

Installed on every target VM for the `matilda-svc` account.

Example:

```text
~/.ssh/MatildaProbeKey.pem.pub
```

### Matilda discovery private key on Probe

Already stored on MatildaProbeVM and used by the Probe to SSH into targets as `matilda-svc`.

Example:

```text
/home/opc/.ssh/MatildaProbeKey.pem
```

Expected permission on MatildaProbeVM:

```text
600
```

Do not copy the Matilda discovery private key to target VMs.

---

## Optional `.env` file

Users do not need `.env`.

If you do not want to type prompts every time, copy:

```text
.env.example
```

to:

```text
.env
```

and fill in your environment values.

`.env` is ignored by git and should not be committed.

---

## Configure target inventory

Edit:

```text
inventory.yml
```

Each target must define:

```yaml
ansible_host: <IP or hostname Ansible uses to configure the VM>
discovery_ip: <IP MatildaProbeVM uses to discover the VM>
```

These may be different.

---

## Public target example

Use this when your computer can SSH directly to the target public IP.

```yaml
public_targets:
  hosts:
    my-public-target:
      ansible_host: <target-public-ip>
      public_ip: <target-public-ip>
      private_ip: <target-private-ip>
      discovery_ip: <target-private-or-public-ip-used-by-probe>
```

Recommended:

```text
Use private_ip as discovery_ip when MatildaProbeVM can reach it.
```

---

## Private-only target example

Use this when the target has only a private IP and must be reached through MatildaProbeVM.

```yaml
private_targets:
  hosts:
    my-private-target:
      ansible_host: <target-private-ip>
      private_ip: <target-private-ip>
      discovery_ip: <target-private-ip>
```

The SSH jump configuration is handled automatically by:

```text
group_vars/private_targets.yml
```

using the Probe values provided at runtime.

---

## Public vs private IP guidance

Use private IPs for `discovery_ip` when possible.

Why:

- traffic stays inside OCI/private network
- better matches Probe-based discovery
- avoids unnecessary public SSH exposure

Use public IPs for `discovery_ip` only if:

- you intentionally want Matilda to discover public IPs
- MatildaProbeVM can reach those public IPs on TCP/22
- your security lists / NSGs allow it safely

A target is ready only if this works from MatildaProbeVM:

```bash
ssh -i <matilda-private-key-on-probe> matilda-svc@<discovery_ip> "sudo /sbin/ifconfig"
```

---

## Run order

Do not skip steps.

```bash
./scripts/run-preflight.sh
./scripts/run-setup.sh
./scripts/run-validate.sh
```

---

## What each step does

### 1. Preflight

Checks:

- admin SSH works
- admin sudo works
- OS is Linux
- MatildaProbeVM has the Matilda discovery private key
- MatildaProbeVM can reach each target `discovery_ip` on port 22

Run:

```bash
./scripts/run-preflight.sh
```

---

### 2. Setup

Configures each target:

- creates `matilda-svc`
- creates `/home/matilda-svc/.ssh`
- installs the Matilda public key
- writes `/etc/sudoers.d/matilda-discovery`
- validates sudoers syntax with `visudo -cf`

Run:

```bash
./scripts/run-setup.sh
```

---

### 3. Validate

Confirms:

- local sudo works as `matilda-svc`
- Probe can SSH to each target as `matilda-svc`
- Probe can run a sudo discovery command remotely

Run:

```bash
./scripts/run-validate.sh
```

---

## Output

Validated discovery IPs are written to:

```text
reports/validated-discovery-ips.txt
```

Validation summary is written to:

```text
reports/validation-summary.txt
```

Only use IPs from `validated-discovery-ips.txt` in Matilda Network Discovery.

---

## Matilda UI values after validation

Use the IPs from:

```text
reports/validated-discovery-ips.txt
```

In Matilda:

```text
Discovery Mode: Network Discovery
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
- Review OCI security lists / NSGs before using public IPs.
- Prefer private IPs for discovery when possible.

---

## Troubleshooting

### Admin SSH fails

Check:

- `ansible_host`
- target admin SSH user entered at runtime
- target admin private key path entered at runtime
- OCI security list / NSG for SSH

### Probe SSH fails

Check:

- Probe host/IP entered at runtime
- Probe SSH user entered at runtime
- Probe admin private key path entered at runtime
- OCI security list / NSG for SSH to Probe

### Probe cannot reach target on port 22

Check:

- `discovery_ip`
- route tables
- security lists / NSGs
- target OS firewall

### SSH as `matilda-svc` fails

Check:

- Matilda public key installed in `/home/matilda-svc/.ssh/authorized_keys`
- ownership is `matilda-svc:matilda-svc`
- `.ssh` is `700`
- `authorized_keys` is `600`
- Matilda private key exists on Probe

### Sudo validation fails

Check:

```bash
visudo -cf /etc/sudoers.d/matilda-discovery
```

and confirm command paths exist on the target.