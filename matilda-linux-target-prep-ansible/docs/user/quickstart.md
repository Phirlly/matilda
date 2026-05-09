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

For platform guidance that does not touch targets:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

`generate windows` creates a local Windows readiness package. `generate unix` creates local admin instructions for AIX, Solaris, and HP-UX planning.

Matilda discovery itself remains agentless and read-only. Do not copy private keys to target systems.
