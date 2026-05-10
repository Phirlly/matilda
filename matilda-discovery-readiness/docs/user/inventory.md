# Inventory

The Linux automation accepts the current grouped inventory format:

```yaml
public_targets:
private_targets:
```

Use `public_targets` when the operator machine can SSH directly to the target. Use `private_targets` when Ansible must reach the target through MatildaProbeVM.

Each target must define:

- `ansible_host`: address Ansible uses to configure the target.
- `discovery_ip`: address MatildaProbeVM uses for discovery and validation.

CSV import is available:

```bash
./matilda-prep inventory import examples/targets.example.csv
```

Normalized inventory v1 is also executable for Linux targets. The runner
converts v1 Linux targets into an ignored temporary Ansible inventory under
`.matilda/runner/` at runtime. Current grouped inventory users do not need to
change files.

Executable v1 fields today:

- `platform: linux`
- `access_path: direct` or `via_probe`
- `ansible_host`
- `discovery_ip`
- `privilege_method: sudo`
- optional `public_ip` and `private_ip`

Non-Linux v1 targets are valid inventory data, but Linux remote actions skip
them with a clear message. Unsupported Linux privilege methods fail before
remote execution.

Use:

```bash
./matilda-prep inventory migrate
```

to create `inventory.v1.yml` from the current grouped inventory. You can copy
the v1 content into `inventory.yml` when you are ready to use v1 as the runner
source.
