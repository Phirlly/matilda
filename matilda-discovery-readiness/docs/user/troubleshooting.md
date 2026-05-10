# Troubleshooting

Start with:

```bash
./matilda-prep doctor
./matilda-prep inventory validate
```

Common issues:

- Missing Ansible: install Ansible on the operator machine.
- Missing `ansible.posix.authorized_key`: run `ansible-galaxy collection install ansible.posix`.
- Probe cannot reach target TCP/22: check route tables, security lists, NSGs, and target firewalls.
- SSH as `matilda-svc` fails: verify the public key on the target matches the private key on MatildaProbeVM.
- Sudo validation fails: validate `/etc/sudoers.d/matilda-discovery` with `visudo -cf`.

Private keys must not be copied to target systems.
