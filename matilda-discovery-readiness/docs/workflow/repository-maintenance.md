# Repository Maintenance

Maintainer reference. Operators do not need this document to run the toolkit; start with the root [README](../../README.md).

This repository is maintained as the Matilda Discovery Readiness Toolkit.

## Current Shape

- `matilda-prep` is the single user entrypoint.
- The default terminal experience is the Matilda Terminal Console.
- `console`, `start`, and `status` are the supported terminal convenience commands.
- Go owns local UX, orchestration, inventory validation, state, reports, and the browser UI.
- Ansible owns remote target setup and validation.
- Linux remains the tested remote automation baseline.
- Windows and UNIX use generated guidance until remote automation is validated.
- Cloud and Kubernetes readiness are not automated in the current release.

## Keep Tracked

- Go source under `cmd/`, `internal/`, and `tests/`.
- Ansible playbooks and roles under `ansible/`.
- Safe examples under `examples/`.
- Schemas under `schemas/`.
- Templates under `templates/`.
- User, reference, and workflow documentation under `docs/user/`, `docs/reference/`, and `docs/workflow/`.
- `reports/.gitkeep` only, so the reports directory exists without generated reports.

## Keep Untracked

These files are local runtime material and must not be committed:

- `.env`
- `targets.csv`
- legacy root `inventory.yml` if present
- `.matilda/`
- `.bin/`
- `.gocache/`
- `.ansible/`
- `PLAN.md`
- `docs/matilda-docs-cache/`
- generated files under `reports/`
- private keys, copied credentials, or customer-specific live inventory

## Compatibility Rules

- Do not remove active Linux workflow compatibility without testing the full Linux path.
- Do not move root `group_vars/` unless the Ansible inventory/config model is intentionally redesigned and tested.
- Do not mix cloud API readiness into OS target roles.
- Do not copy private keys to target systems.
- Keep `matilda-prep` as the binary unless an install alias is explicitly introduced.

## Branch Workflow

This repository uses the protected-main workflow defined in [Branching Workflow](BRANCHING.md).
All implementation work follows [Development Workflow](DEVELOPMENT.md).

Use those workflow docs as the source of truth for branch roles, planning,
validation, pull requests, promotion, and release tags. This maintenance file
only defines repository ownership boundaries.

## Validation

Use the local validation gate in [Development Workflow](DEVELOPMENT.md).
