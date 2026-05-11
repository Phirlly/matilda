# Inventory

Matilda Discovery Readiness uses `inventory.yml` with `version: 1`.

Create a starter file with:

```bash
./matilda-prep init
```

Validate it before remote runs:

```bash
./matilda-prep inventory validate
```

## Linux Targets

Use one target entry per system:

```yaml
version: 1

targets:
  app01:
    platform: linux
    os_family: oracle_linux
    cloud_provider: oci
    access_path: direct
    ansible_host: 203.0.113.10
    discovery_ip: 10.0.0.10
    public_ip: 203.0.113.10
    private_ip: 10.0.0.10
    privilege_method: sudo
    configure_mode: remote
```

Required Linux fields:

- `platform: linux`
- `access_path: direct` or `via_probe`
- `ansible_host`: address Ansible uses to configure the target.
- `discovery_ip`: address MatildaProbeVM uses for discovery and validation.
- `privilege_method: sudo`

Optional Linux fields:

- `os_family`
- `cloud_provider`
- `public_ip`
- `private_ip`
- `configure_mode`

Use `direct` when the operator machine can SSH to the target. Use `via_probe` when the target must be reached through MatildaProbeVM.

## CSV Import

CSV import is available when you have a spreadsheet or exported target list:

```bash
./matilda-prep inventory import examples/targets.example.csv
```

The import command writes a `version: 1` `inventory.yml`.

## Platform Planning

Non-Linux targets can be stored in `inventory.yml` for planning. Linux remote actions only run against Linux targets and skip other platforms with a clear message.

Windows and UNIX readiness are guidance-only in this release:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

Cloud and Kubernetes inventory entries are scaffold-only in this release.
