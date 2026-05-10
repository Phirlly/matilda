# Troubleshooting

Start with:

```bash
./matilda-prep doctor
./matilda-prep inventory validate
```

When `validate` fails, still check the generated report files. The `Remediation` column in `reports/readiness.html`, `reports/readiness.csv`, and the browser Target Readiness table includes the target-specific fix and the observed failure text when available.

Common issues:

- Missing Ansible: install Ansible on the operator machine, then rerun `./matilda-prep doctor`.
- Missing toolkit files in `doctor`: run the command from the source checkout root or extracted release package root. Do not move the standalone binary away from the repository files it needs.
- Missing inventory values: replace placeholder `ansible_host` and `discovery_ip` values before running remote actions.
- Browser remote action says `.env` is incomplete: fix every listed missing, placeholder, or missing-file value in `.env`. Browser actions cannot stop for interactive SSH prompts.
- SSH cannot reach TCP/22: confirm the target address, routing, security lists or NSGs, and target firewalls from the operator or MatildaProbeVM path being tested.
- Probe cannot reach target TCP/22: check route tables, security lists, NSGs, and target firewalls.
- SSH as `matilda-svc` fails: verify the target `authorized_keys` entry matches the private key on MatildaProbeVM.
- Sudo requires a password: rerun setup or validate `/etc/sudoers.d/matilda-discovery` with `visudo -cf`.
- Sudo says the service account is not allowed: verify the Matilda sudoers drop-in allows the documented discovery commands for `matilda-svc`.
- A denied command was allowed: review the Matilda sudoers drop-in and restrict access to documented discovery commands.
- Service account is locked: rerun setup or unlock `matilda-svc` and restore an interactive shell.
- Service account is missing: rerun setup to recreate `matilda-svc`, its home directory, key, and sudoers drop-in.
- Neither `ifconfig` nor `ip` exists: install `net-tools` or make `iproute` available, then rerun validate.

Private keys must not be copied to target systems.
