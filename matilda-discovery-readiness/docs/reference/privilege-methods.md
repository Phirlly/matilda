# Privilege Methods

Linux automation supports `sudo` today.

Recognized inventory values:

- `sudo`
- `dzdo`
- `pbrun`
- `suexec`
- `winrm`
- `cloud_api`
- `k8s_api`
- `none`

Only `sudo` is automated for Linux target readiness in this release. Other values can be recorded in inventory for planning, but they do not enable remote automation.
