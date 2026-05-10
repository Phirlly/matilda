# Reports

Validation writes the baseline reports:

- `reports/validated-discovery-ips.txt`
- `reports/validation-summary.txt`

`validated-discovery-ips.txt` contains only targets that passed readiness validation. Use those IPs in Matilda Network Discovery.

`validation-summary.txt` is the raw local validation summary. Most operators should use the generated HTML or Markdown report for review.

Run:

```bash
./matilda-prep report
```

to generate:

- `reports/readiness.csv`
- `reports/readiness.json`
- `reports/readiness.md`
- `reports/readiness.html`

Generated reports include target-specific remediation for SSH reachability, SSH key authentication, sudo password or policy issues, denied-command checks, Probe reachability, missing service account, locked service account, and missing validation commands.

Use only targets marked `Ready=YES` in Matilda Network Discovery.

Run history is stored locally under `.matilda/runs/`. It records action status, timestamps, readiness counts, report paths, and summarized outcome. It does not store private key contents.
