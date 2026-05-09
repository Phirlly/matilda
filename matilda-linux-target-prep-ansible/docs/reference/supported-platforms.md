# Supported Platform Direction

The toolkit is structured for:

- Linux
- UNIX: AIX, Solaris, HP-UX
- Windows
- Cloud API readiness: AWS, Azure, GCP, OCI
- Kubernetes API readiness

Only Linux target setup is implemented now.

Windows and UNIX handoff generation is available through:

```bash
./matilda-prep generate windows
./matilda-prep generate unix
```

Other platforms are scaffolded so they can be added as separate modules without modifying the Linux role.
