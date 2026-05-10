# Troubleshooting

Start with:

```bash
./matilda-prep doctor
./matilda-prep inventory validate
```

Common issues:

- Missing Ansible: install Ansible on the operator machine, then rerun `./matilda-prep doctor`.
- Missing `ansible.posix.authorized_key`: run `ansible-galaxy collection install ansible.posix`.
- Missing inventory values: replace placeholder `ansible_host` and `discovery_ip` values before running remote actions.
- Probe cannot reach target TCP/22: check route tables, security lists, NSGs, and target firewalls.
- SSH as `matilda-svc` fails: verify the target `authorized_keys` entry matches the private key on MatildaProbeVM.
- Sudo requires a password: rerun setup or validate `/etc/sudoers.d/matilda-discovery` with `visudo -cf`.
- A denied command was allowed: review the Matilda sudoers drop-in and restrict access to documented discovery commands.
- Service account is locked: rerun setup or unlock `matilda-svc` and restore an interactive shell.
- Service account is missing: rerun setup to recreate `matilda-svc`, its home directory, key, and sudoers drop-in.
- Neither `ifconfig` nor `ip` exists: install `net-tools` or make `iproute` available, then rerun validate.

Private keys must not be copied to target systems.
