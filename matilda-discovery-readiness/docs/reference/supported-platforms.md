# Supported Platform Direction

The toolkit is structured for:

- Linux
- UNIX: AIX, Solaris, HP-UX
- Windows
- Cloud API readiness: AWS, Azure, GCP, OCI
- Kubernetes API readiness

Only Linux target setup is implemented now. The repository and product names remain broader because the toolkit also owns Probe-to-target readiness, generated Windows/UNIX platform guidance, reporting, and future cloud/Kubernetes readiness modules.

Generate local Windows readiness packages and UNIX admin instructions with:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

These commands write local guidance files only. They do not connect to Windows or UNIX targets and do not change any target configuration.

Other platforms are scaffolded so they can be added as separate modules without modifying the Linux role.
