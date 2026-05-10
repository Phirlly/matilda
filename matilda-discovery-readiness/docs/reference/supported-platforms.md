# Supported Platforms

This release candidate automates Linux target readiness only.

## Automated Today

- Linux target readiness.
- Direct Linux targets reached from the operator machine.
- Linux targets reached through MatildaProbeVM.
- Oracle Linux / RHEL-like systems as the validated baseline.
- Probe-to-target SSH and sudo validation.

## Guidance Only

Generate local Windows readiness packages and UNIX admin instructions with:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

These commands write local guidance files only. They do not connect to Windows or UNIX targets and do not change target configuration.

## Not Automated In This Release Candidate

- Windows remote setup or validation.
- UNIX remote setup or validation for AIX, Solaris, or HP-UX.
- Cloud API readiness for AWS, Azure, GCP, or OCI.
- Kubernetes API readiness.

Use the Linux workflow only for Linux targets:

```bash
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep report
```
