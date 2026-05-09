# Reports

Validation writes the baseline reports:

- `reports/validated-discovery-ips.txt`
- `reports/validation-summary.txt`

Run:

```bash
./matilda-prep report
```

to generate:

- `reports/readiness.csv`
- `reports/readiness.json`
- `reports/readiness.md`
- `reports/readiness.html`

Use only targets marked `Ready=YES` in Matilda Network Discovery.
