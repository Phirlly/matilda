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

## Where to run commands

Run all commands from the root of this project folder:

```bash
cd matilda-linux-target-prep-ansible
```

If you placed the folder somewhere else, `cd` into that location first:

```bash
cd /path/to/matilda-linux-target-prep-ansible
```

---

## What you must provide

Each user must provide their own values for:

- MatildaProbeVM public IP or reachable hostname
- MatildaProbeVM SSH username
- local admin SSH private key path for `opc` or equivalent admin user
- Matilda discovery public key file
- Matilda discovery private key path on MatildaProbeVM
- target VM public/private IPs
- target VM admin username

Do not assume the sample IPs or paths match your environment.

---

## Required keys

### 1. Admin SSH private key

This is the key used by Ansible to connect to target VMs as the admin user, usually `opc` in OCI.

Example placeholder:

```text
~/.ssh/<your-oci-admin-private-key>
```

This value is configured in `inventory.yml` per target:

```yaml
ansible_ssh_private_key_file: ~/.ssh/<your-oci-admin-private-key>
```

### 2. Matilda discovery public key

This public key is installed on every target VM for the `matilda-svc` account.

Place it here:

```text
files/MatildaProbeKey.pem.pub
```

### 3. Matilda discovery private key

This private key is used by MatildaProbeVM to SSH to targets as `matilda-svc`.

It must already exist on MatildaProbeVM.

Example placeholder:

```text
/home/<probe-user>/.ssh/MatildaProbeKey.pem
```

It must have permission:

```text
600
```

Example validation on MatildaProbeVM:

```bash
ls -l /home/<probe-user>/.ssh/MatildaProbeKey.pem
```

Expected:

```text
-rw-------
```

Do not copy the Matilda discovery private key to target VMs.

---

## Configure global Probe settings

Edit:

```text
group_vars/all.yml
```

Set these values for your environment:

```yaml
matilda_probe_ansible_host: <probe-public-ip-or-reachable-hostname>
matilda_probe_user: <probe-ssh-user>
matilda_probe_admin_private_key_file: <local-private-key-used-to-ssh-to-probe>
matilda_probe_private_key_on_probe: <path-to-matilda-discovery-private-key-on-probe>
```

Example shape only:

```yaml
matilda_probe_ansible_host: <probe-public-ip>
matilda_probe_user: opc
matilda_probe_admin_private_key_file: ~/.ssh/<your-oci-admin-private-key>
matilda_probe_private_key_on_probe: /home/opc/.ssh/MatildaProbeKey.pem
```

Replace placeholders with your actual values.

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
      ansible_user: opc
      ansible_ssh_private_key_file: ~/.ssh/<your-oci-admin-private-key>
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
      ansible_user: opc
      ansible_ssh_private_key_file: ~/.ssh/<your-oci-admin-private-key>
      ansible_ssh_common_args: >-
        -o ProxyJump=<probe-user>@<probe-public-ip-or-hostname>
        -o StrictHostKeyChecking=no
        -o UserKnownHostsFile=/dev/null
```

Replace all placeholders.

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

Only use IPs from this file in Matilda Network Discovery.

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
- Only the Matilda public key is installed on targets.
- The Matilda discovery private key is used only from MatildaProbeVM.
- The sudoers file is restricted to Matilda discovery commands.
- Review OCI security lists / NSGs before using public IPs.
- Prefer private IPs for discovery when possible.

---

## Troubleshooting

### Admin SSH fails

Check:

- `ansible_host`
- `ansible_user`
- `ansible_ssh_private_key_file`
- OCI security list / NSG for SSH

### Probe cannot reach target on port 22

Check:

- `discovery_ip`
- route tables
- security lists / NSGs
- target OS firewall

### SSH as `matilda-svc` fails

Check:

- public key installed in `/home/matilda-svc/.ssh/authorized_keys`
- ownership is `matilda-svc:matilda-svc`
- `.ssh` is `700`
- `authorized_keys` is `600`

### Sudo validation fails

Check:

```bash
visudo -cf /etc/sudoers.d/matilda-discovery
```

and confirm command paths exist on the target.