# Inventory

Linux readiness uses `inventory.yml`.

The default inventory format has two groups:

```yaml
public_targets:
private_targets:
```

Use `public_targets` when the operator machine can SSH directly to the target. Use `private_targets` when Ansible must reach the target through MatildaProbeVM.

Each target must define:

- `ansible_host`: address Ansible uses to configure the target.
- `discovery_ip`: address MatildaProbeVM uses for discovery and validation.

CSV import is available when you have a spreadsheet or exported target list:

```bash
./matilda-prep inventory import examples/targets.example.csv
```

The toolkit can also use normalized inventory v1 for Linux targets. Most users can continue using the default `inventory.yml` format created by `./matilda-prep init`.

Executable v1 fields today:

- `platform: linux`
- `access_path: direct` or `via_probe`
- `ansible_host`
- `discovery_ip`
- `privilege_method: sudo`
- optional `public_ip` and `private_ip`

Non-Linux v1 targets can be stored in inventory, but Linux remote actions skip them with a clear message. Unsupported Linux privilege methods fail before remote execution.

Use:

```bash
./matilda-prep inventory migrate
```

to create `inventory.v1.yml` from the default inventory format. You can copy the v1 content into `inventory.yml` when you are ready to use v1 directly.
