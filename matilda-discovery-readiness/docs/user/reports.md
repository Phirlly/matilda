# Reports

Validation writes the baseline reports:

- `reports/validated-discovery-ips.txt`
- `reports/validation-summary.txt`

`validation-summary.txt` is the raw local validation output. It may include an
internal `FailureCode` column so the toolkit can map Ansible failures to clearer
operator remediation.

Run:

```bash
./matilda-prep report
```

to generate:

- `reports/readiness.csv`
- `reports/readiness.json`
- `reports/readiness.md`
- `reports/readiness.html`

Generated reports keep the stable user-facing columns and replace known raw
failure details with target-specific remediation for SSH, sudo, denied command,
Probe reachability, missing service account, locked service account, and missing
validation commands.

Use only targets marked `Ready=YES` in Matilda Network Discovery.

Run history is stored locally under `.matilda/runs/`. Each record includes the
action, status, timestamps, command label, readiness counts, report paths, and
summarized outcome. It does not store private key contents or full command
output.
