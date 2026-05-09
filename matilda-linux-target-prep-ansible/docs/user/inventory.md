# Inventory

The current Linux automation accepts two Ansible inventory groups:

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

Normalized inventory v1 is scaffolded for future Linux, UNIX, Windows, cloud, and Kubernetes support. Use:

```bash
./matilda-prep inventory migrate
```
