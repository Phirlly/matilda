# Inventory

Matilda Discovery Readiness uses `targets.csv` as the operator inventory file.
Do not hand-edit `inventory.yml`; the toolkit generates normalized runtime
inventory under `.matilda/` when it needs YAML for internal planning or Ansible.

Create a starter file with:

```bash
./matilda-prep init
```

Validate it before remote runs:

```bash
./matilda-prep inventory validate
```

Validation reports row numbers for common CSV issues, including missing required
values, placeholder addresses, duplicate hostnames, duplicate `discovery_ip`
values, unsupported platforms, unsupported access paths, and unsupported
privilege methods.

## Linux Targets

Use one target entry per system:

```csv
hostname,platform,os_family,ansible_host,discovery_ip,access_path,privilege_method,private_ip,public_ip,cloud_provider
app01,linux,oracle_linux,203.0.113.10,10.0.0.10,direct,sudo,10.0.0.10,203.0.113.10,oci
```

Required Linux fields:

- `hostname`: local target name used in reports.
- `platform`: `linux`.
- `ansible_host`: address Ansible uses to configure the target.
- `discovery_ip`: address MatildaProbeVM uses for discovery and validation.
- `access_path`: `direct` or `via_probe`.
- `privilege_method`: `sudo`.

Optional Linux fields:

- `os_family`
- `cloud_provider`
- `public_ip`
- `private_ip`
- `configure_mode`

Use `direct` when the operator machine can SSH to the target. Use `via_probe` when the target must be reached through MatildaProbeVM.

## CSV Import

Create or replace local `targets.csv` from a spreadsheet export:

```bash
./matilda-prep inventory import examples/targets.example.csv
```

The import command validates the CSV, writes local `targets.csv`, and generates
normalized runtime inventory under `.matilda/generated/`.

## Platform Planning

This release automates Linux target readiness only. Keep `targets.csv` focused
on Linux targets for the remote workflow.

Windows and UNIX readiness are guidance-only in this release:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

Cloud and Kubernetes readiness are scaffold-only in this release and are not
part of the Linux target CSV workflow.
