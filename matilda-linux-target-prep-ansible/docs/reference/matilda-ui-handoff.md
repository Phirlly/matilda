# Matilda UI Handoff

After validation, use only IPs from:

```text
reports/validated-discovery-ips.txt
```

Typical Matilda Network Discovery values:

- Discovery Mode: Network Discovery
- Network Address: IPs from `reports/validated-discovery-ips.txt`
- Credential Group: Linux PEM credential group for `matilda-svc`
- Probe: registered Matilda Probe
- Execution Mode: sudo
- Common login: yes when all listed targets use the same credential

The Matilda discovery private key must remain on MatildaProbeVM and must not be copied to target systems.
