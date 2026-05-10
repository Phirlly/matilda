# Quickstart

Matilda Discovery Readiness Toolkit prepares and validates target readiness, Probe readiness, and platform readiness for Matilda Probe-based discovery.

The release candidate baseline automates Linux target readiness for direct and Probe-routed targets. Windows and UNIX commands generate local guidance only, and cloud/Kubernetes readiness remains scaffolded for future modules.

Open the Matilda Terminal Console:

```bash
./matilda-prep
```

Use arrow keys or `k`/`j` to choose an action, Enter to run it, `r` to refresh, and `q` or Esc to quit. After a command runs, the console switches to a full result view. Use Up/Down, PageUp/PageDown, Home, and End to scroll output, then press `b` or Esc to return to the action menu.

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

For a fresh clone or release candidate checkout, run the operator smoke test in [operator-smoke-test.md](operator-smoke-test.md) before changing targets.

Use `./matilda-prep status` when you want a non-interactive status summary without entering the interactive console.

`setup` modifies target systems by creating or updating the Matilda service account, installing the Matilda discovery public key, and writing sudoers configuration.

For platform guidance that does not touch targets:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

`generate windows` creates a local Windows readiness package. `generate unix` creates local admin instructions for AIX, Solaris, and HP-UX planning.

Matilda discovery itself remains agentless and read-only. Do not copy private keys to target systems.
