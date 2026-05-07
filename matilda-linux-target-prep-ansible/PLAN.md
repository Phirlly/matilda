# Matilda Linux Target Prep Automation — Complete Plan

## 1. Purpose

Build a reusable Ansible-based automation package that prepares Linux / Oracle Linux target VMs for Matilda Probe-based discovery.

The automation will configure each target VM so Matilda can discover it using:

```text
Probe:             MatildaProbeVM
Discovery user:    matilda-svc
Authentication:    SSH PEM key
Execution mode:    sudo
Credential group:  Linux-OCI-MatildaSvc-Key
```

This plan is based on:

- `docs/Matilda Discovery Probe Prerequisites, Installation & Service Accounts.pdf`
- `docs/matilda-docs-cache/articles/matilda-probe-guide-for-saas.md`
- `docs/matilda-docs-cache/articles/pre-requisites-and-access.md`
- `docs/matilda-docs-cache/articles/discovery-settings.md`
- `docs/matilda-docs-cache/articles/how-to-initiate-data-center-discovery.md`
- Manual validation completed in this session

---

## 2. Documented Matilda Requirements

Per the Matilda Discovery Probe PDF:

### Linux / UNIX target requirements

Each Linux / Oracle Linux target VM must have:

```text
Account name:         matilda-svc
Authentication:       SSH public-key authentication
Sudo access:          Passwordless sudo for explicit read-only command allow-list
TTY behavior:         Non-TTY execution enabled with !requiretty
secure_path:          Restricted sudo secure_path
Validation:           local and Probe-based SSH/sudo validation
```

### Required PDF sections

```text
Section 6.2 — Create the Service Account
Section 6.3 — Configure SSH Key-Based Authentication
Section 6.4 — Apply the Matilda Sudoers File
Section 6.5 — Validate the Configuration
Section 8   — Network & Port Matrix
```

### Required network path

For Linux targets:

```text
MatildaProbeVM → Linux target → TCP/22
```

---

## 3. Current Known Working State

Already validated manually:

```text
MatildaProbeVM installed and registered
serviceexecutor pod running
discoveryexecutor pod running
Credential group Linux-OCI-MatildaSvc-Key created and validated
First target VM 10.0.0.195 discovered successfully in Matilda UI
Additional targets validated manually over public/private IPs
```

Validated commands included:

```bash
ssh -i ~/.ssh/MatildaProbeKey.pem matilda-svc@<target-ip> "sudo /sbin/ifconfig"
```

This proved:

```text
Probe-to-target SSH works
matilda-svc key authentication works
passwordless sudo works
Matilda discovery succeeds for prepared targets
```

---

## 4. Project Folder Name

Use:

```text
matilda-linux-target-prep-ansible
```

Recommended path:

```text
/Users/lly/Library/CloudStorage/OneDrive-OracleCorporation/Documents/Matilda/matilda-linux-target-prep-ansible
```

Reason:

```text
matilda                  clear product context
linux-target-prep        clear purpose
ansible                  clear automation tool
```

---

## 5. Project Structure

```text
matilda-linux-target-prep-ansible/
  README.md
  ansible.cfg
  inventory.example.yml
  inventory.yml
  group_vars/
    all.yml
  files/
    MatildaProbeKey.pem.pub
  templates/
    matilda-discovery-sudoers.j2
  playbooks/
    01-preflight.yml
    02-setup-linux-targets.yml
    03-validate-linux-targets.yml
  scripts/
    run-preflight.sh
    run-setup.sh
    run-validate.sh
  reports/
    .gitkeep
```

---

## 6. Connection Models to Support

Automation must support three connection paths.

### A. Mac → Public target VM

Used when the target VM has a public IP and allows SSH from your Mac.

```text
Mac → target public IP
```

### B. Mac → MatildaProbeVM → Private target VM

Used when the target VM is private-only.

```text
Mac → MatildaProbeVM → target private IP
```

Implemented with SSH ProxyJump.

### C. MatildaProbeVM → Target VM

Used by Matilda discovery and final validation.

```text
MatildaProbeVM → target discovery_ip
```

A host is not considered discovery-ready until this path works:

```bash
ssh -i /home/opc/.ssh/MatildaProbeKey.pem matilda-svc@<discovery_ip> "sudo /sbin/ifconfig"
```

---

## 7. Inventory Design

Every host must define two important addresses:

```text
ansible_host   = how Ansible reaches the VM for setup
discovery_ip   = how MatildaProbeVM reaches the VM for discovery
```

### Public subnet VM discovered by public IP

```yaml
public_targets:
  hosts:
    matildatargetvm002:
      ansible_host: 129.213.103.78
      public_ip: 129.213.103.78
      private_ip: 10.0.0.25
      discovery_ip: 129.213.103.78
      ansible_user: opc
      ansible_ssh_private_key_file: ~/.ssh/oci_admin_key.pem
```

### Public subnet VM discovered by private IP

```yaml
public_targets:
  hosts:
    matildatargetvm002:
      ansible_host: 129.213.103.78
      public_ip: 129.213.103.78
      private_ip: 10.0.0.25
      discovery_ip: 10.0.0.25
      ansible_user: opc
      ansible_ssh_private_key_file: ~/.ssh/oci_admin_key.pem
```

### Private subnet VM through Probe jump host

```yaml
private_targets:
  hosts:
    matildaprivatetarget001:
      ansible_host: 10.0.1.25
      private_ip: 10.0.1.25
      discovery_ip: 10.0.1.25
      ansible_user: opc
      ansible_ssh_private_key_file: ~/.ssh/oci_admin_key.pem
      ansible_ssh_common_args: >-
        -o ProxyJump=opc@150.136.236.167
```

---

## 8. Key Management Plan

### On Mac

```text
~/.ssh/oci_admin_key.pem          admin key for opc access
~/.ssh/MatildaProbeKey.pem        Matilda discovery private key
~/.ssh/MatildaProbeKey.pem.pub    Matilda discovery public key
```

### In Ansible project

Only store the public key:

```text
files/MatildaProbeKey.pem.pub
```

Never store private keys inside the project folder.

### On target VMs

Install only the public key:

```text
/home/matilda-svc/.ssh/authorized_keys
```

### On MatildaProbeVM

Private key must exist for validation and discovery:

```text
/home/opc/.ssh/MatildaProbeKey.pem
```

Required permission:

```text
-rw------- opc opc /home/opc/.ssh/MatildaProbeKey.pem
```

---

## 9. Global Configuration

`group_vars/all.yml` should define:

```yaml
matilda_service_user: matilda-svc
matilda_service_home: /home/matilda-svc
matilda_public_key_file: files/MatildaProbeKey.pem.pub
matilda_sudoers_file: /etc/sudoers.d/matilda-discovery
matilda_probe_host: MatildaProbeVM
matilda_probe_user: opc
matilda_probe_public_ip: 150.136.236.167
matilda_probe_private_key_on_probe: /home/opc/.ssh/MatildaProbeKey.pem
validate_from_probe: true
```

---

## 10. Ansible Config

`ansible.cfg` should define:

```ini
[defaults]
inventory = inventory.yml
retry_files_enabled = False
timeout = 30
host_key_checking = False
stdout_callback = default
interpreter_python = auto_silent
```

Note:

```text
host_key_checking = False
```

is acceptable for lab use. For production, document how to enable host key checking.

---

## 11. Sudoers Template

`templates/matilda-discovery-sudoers.j2` must implement PDF Section 6.4.

It must include:

```text
User_Alias MATILDA_USER = matilda-svc
Defaults:MATILDA_USER !requiretty
Defaults:MATILDA_USER secure_path="/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
Cmnd_Alias MATILDA_DISCOVERY = <approved read-only command paths>
MATILDA_USER ALL=(ALL) NOPASSWD: MATILDA_DISCOVERY
```

It must include Linux discovery commands for:

```text
Network:     ip, ifconfig, route, arp, netstat, ss, ethtool
Storage:     lsblk, blkid, df, mount, findmnt, fdisk, parted, pvs, vgs, lvs
System:      dmidecode, lsmod, lshw, lspci, dmesg, mokutil, uptime, iostat
Process:     ps, pgrep, pwdx, readlink, strings
Config:      find, cat, crontab, yum
Container:   docker, kubectl
Firewall:    iptables, ip6tables
```

The sudoers file must be validated with:

```bash
visudo -cf /etc/sudoers.d/matilda-discovery
```

---

## 12. Playbook 1 — Preflight

File:

```text
playbooks/01-preflight.yml
```

Purpose:

```text
Check whether each target is safe and reachable before making changes.
```

Checks:

```text
Mac/Ansible can SSH to target as opc
opc can sudo
OS is Linux / Oracle Linux / RHEL-compatible
Target has Python or can support Ansible modules
Target has reachable home filesystem
MatildaProbeVM can reach discovery_ip on TCP/22
MatildaProbeVM has MatildaProbeKey.pem with 600 permissions
```

Probe reachability check should effectively test:

```bash
nc -vz <discovery_ip> 22
```

from MatildaProbeVM.

Preflight must not change target systems.

---

## 13. Playbook 2 — Setup Linux Targets

File:

```text
playbooks/02-setup-linux-targets.yml
```

Purpose:

```text
Apply Matilda PDF Section 6.2–6.4 to each target VM.
```

Actions:

### 1. Create service account

Equivalent to:

```bash
useradd -r -s /bin/bash -d /home/matilda-svc -m matilda-svc
```

Must handle:

```text
user already exists
home exists before user exists
missing group
wrong ownership
```

### 2. Configure SSH public key

Equivalent to:

```bash
mkdir -p /home/matilda-svc/.ssh
install public key into authorized_keys
chmod 700 /home/matilda-svc
chmod 700 /home/matilda-svc/.ssh
chmod 600 /home/matilda-svc/.ssh/authorized_keys
chown -R matilda-svc:matilda-svc /home/matilda-svc
```

Use Ansible `authorized_key` module to avoid duplicate public keys.

### 3. Configure sudoers

Write:

```text
/etc/sudoers.d/matilda-discovery
```

Set:

```bash
chmod 440 /etc/sudoers.d/matilda-discovery
```

Validate:

```bash
visudo -cf /etc/sudoers.d/matilda-discovery
```

If validation fails:

```text
fail the host
report the error
avoid declaring host ready
```

---

## 14. Playbook 3 — Validate Linux Targets

File:

```text
playbooks/03-validate-linux-targets.yml
```

Purpose:

```text
Confirm every target is actually ready for Matilda discovery.
```

### Local validation on target

Run commands as `matilda-svc`:

```bash
sudo <ifconfig-path>
sudo <ip-path> addr show
sudo <netstat-path> -tuln
```

Validation must be path-aware.

Detect paths:

```bash
command -v ifconfig
command -v ip
command -v netstat
command -v ss
```

Use whichever approved path exists.

### Probe-to-target validation

From MatildaProbeVM:

```bash
ssh -i /home/opc/.ssh/MatildaProbeKey.pem matilda-svc@<discovery_ip> "sudo /sbin/ifconfig"
```

If `/sbin/ifconfig` is not available, use detected path.

A host is ready only if:

```text
local sudo validation passes
Probe SSH validation passes
```

---

## 15. Reporting

The automation should write reports under:

```text
reports/
```

Reports:

```text
reports/preflight-summary.txt
reports/setup-summary.txt
reports/validation-summary.txt
reports/validated-discovery-ips.txt
```

### Validation summary example

```text
Host                    Discovery IP       Admin SSH   Sudo   User   Key   Sudoers   Local Sudo   Probe SSH   Ready
matildatargetvm002       129.213.103.78     OK          OK     OK     OK    OK        OK           OK          YES
matildatargetvm003       129.213.107.133    OK          OK     OK     OK    OK        OK           OK          YES
private-target001        10.0.1.25          OK          OK     OK     OK    OK        OK           OK          YES
```

### Validated IP list

```text
reports/validated-discovery-ips.txt
```

should contain only IPs ready to paste into Matilda Network Discovery:

```text
129.213.103.78
129.213.107.133
10.0.1.25
```

---

## 16. Wrapper Scripts

### `scripts/run-preflight.sh`

Runs:

```bash
ansible-playbook playbooks/01-preflight.yml
```

### `scripts/run-setup.sh`

Runs:

```bash
ansible-playbook playbooks/02-setup-linux-targets.yml
```

### `scripts/run-validate.sh`

Runs:

```bash
ansible-playbook playbooks/03-validate-linux-targets.yml
```

Scripts should be simple convenience wrappers.

---

## 17. User Workflow

### Step 1 — Create test instances

Create:

```text
matilda-public-target-test001      public subnet, public + private IP
matilda-private-target-test001     private subnet, private IP only
```

### Step 2 — Confirm Mac access

Public target:

```bash
ssh -i ~/.ssh/oci_admin_key.pem opc@<public-ip>
```

Private target through Probe:

```bash
ssh -J opc@<probe-public-ip> opc@<private-ip>
```

### Step 3 — Fill inventory

Set:

```text
ansible_host = IP Ansible uses
private_ip = OCI private IP
public_ip = OCI public IP if present
discovery_ip = IP MatildaProbeVM should use
```

### Step 4 — Run preflight

```bash
./scripts/run-preflight.sh
```

### Step 5 — Run setup

```bash
./scripts/run-setup.sh
```

### Step 6 — Run validation

```bash
./scripts/run-validate.sh
```

### Step 7 — Use validated IPs in Matilda UI

In Matilda:

```text
Discovery → Datacenter → Initiate Discovery
Discovery Mode: Network Discovery
Network Address: values from reports/validated-discovery-ips.txt
Credential Group: Linux-OCI-MatildaSvc-Key
Probe: MatildaProbeVM
Execution Mode: sudo
SNMP: No
Common login: Yes
Promote to discovery after precheck: Yes
Promote over-utilized resources: Yes
```

---

## 18. Public vs Private Usage Rules

### If VM has public IP and Mac can SSH to it

Use:

```text
public_targets
ansible_host = public IP
```

Discovery can use public or private IP.

### If VM is private-only

Use:

```text
private_targets
ansible_host = private IP
ProxyJump through MatildaProbeVM
```

Discovery should use private IP.

### If using public IP for discovery

Validate:

```bash
MatildaProbeVM → public IP:22
MatildaProbeVM → public IP as matilda-svc with sudo
```

### If using private IP for discovery

Validate:

```bash
MatildaProbeVM → private IP:22
MatildaProbeVM → private IP as matilda-svc with sudo
```

---

## 19. Safety Requirements

Automation must:

```text
Never copy private keys to target VMs
Only install public key on targets
Avoid duplicate authorized_keys entries
Be safe to rerun
Handle partially configured hosts
Validate sudoers syntax before success
Report per-host failures clearly
Not modify unrelated sudoers files
Not overwrite unrelated authorized_keys content
```

---

## 20. Scope Boundaries

This automation is for:

```text
Linux / Oracle Linux / RHEL-like targets only
```

Not included in this phase:

```text
Windows target setup
Database credential creation
Matilda UI API automation
Running the final Matilda discovery task automatically
```

Windows will be handled separately using PDF Section 7:

```text
WinRM
Remote Management Users
Local Administrators
Ports 5985/5986 and 445
```

---

## 21. Success Criteria

A target is considered ready only when all are true:

```text
admin SSH works
admin sudo works
OS is supported Linux/OEL/RHEL-like
Probe can reach discovery_ip:22
matilda-svc exists
SSH public key installed
permissions are correct
sudoers file exists
visudo parse OK
local sudo validation passes
Probe-to-target SSH/sudo validation passes
```

---

## 22. Implementation Order

When ready to implement:

```text
1. Create project folder
2. Add README.md
3. Add inventory.example.yml
4. Add ansible.cfg
5. Add group_vars/all.yml
6. Add public key file
7. Add sudoers template
8. Add preflight playbook
9. Add setup playbook
10. Add validation playbook
11. Add wrapper scripts
12. Add reports directory
13. Test public target
14. Test private target through ProxyJump
15. Finalize README based on test results
```

---

## 23. Recommended Immediate Next Step

Create the test instances:

```text
matilda-public-target-test001
matilda-private-target-test001
```

Then collect:

```text
public IP if present
private IP
admin username
admin SSH key path
which IP should be used as discovery_ip
```