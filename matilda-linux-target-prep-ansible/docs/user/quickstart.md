# Quickstart

Matilda Discovery Readiness Toolkit prepares and validates targets for Matilda Probe-based discovery.

Recommended Linux workflow:

```bash
./matilda-prep init
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep report
```

`setup` modifies target systems by creating or updating the Matilda service account, installing the Matilda discovery public key, and writing sudoers configuration.

For planning handoffs that do not touch targets:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

Matilda discovery itself remains agentless and read-only. Do not copy private keys to target systems.
