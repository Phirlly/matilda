# Linux Target Readiness

The implemented workflow supports Linux targets, with Oracle Linux and RHEL-like systems as the validated baseline.

The workflow verifies or prepares:

- SSH access for an admin account used by Ansible.
- Non-interactive sudo for that admin account.
- Probe-to-target network access on TCP/22.
- A dedicated `matilda-svc` service account.
- The Matilda discovery public key installed for `matilda-svc`.
- `/etc/sudoers.d/matilda-discovery` with the documented Matilda command allow-list.

The Matilda discovery private key must remain on MatildaProbeVM. It must not be copied to target systems.
