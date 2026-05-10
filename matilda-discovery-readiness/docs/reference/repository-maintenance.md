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
- User and reference documentation under `docs/user/` and `docs/reference/`.
- `reports/.gitkeep` only, so the reports directory exists without generated reports.

## Keep Untracked

These files are local runtime material and must not be committed:

- `.env`
- `inventory.yml`
- `inventory.v1.yml`
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

- Do not make direct code or documentation updates on `main`.
- Make changes on `featureBranch` or another non-main branch.
- Run validation on the branch before merging or fast-forwarding `main`.
- Create release tags from `main` only after review and validation.
- Prefer a new tag over moving a published tag unless the tag move is explicitly approved. For RCs, use the next RC tag.

## Validation

Run these checks before opening a PR:

```bash
go test ./...
go vet ./...
git diff --check
bash -n matilda-prep
```

Run command smoke checks after UX or routing changes:

```bash
./matilda-prep help
./matilda-prep status
./matilda-prep inventory validate
./matilda-prep generate windows
./matilda-prep generate unix
```

Run Ansible syntax checks after Ansible-adjacent changes:

```bash
ANSIBLE_CONFIG=ansible/ansible.cfg ansible-playbook --syntax-check ansible/playbooks/linux/preflight.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ansible-playbook --syntax-check ansible/playbooks/linux/setup.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ansible-playbook --syntax-check ansible/playbooks/linux/validate.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ansible-playbook --syntax-check ansible/playbooks/linux/rollback.yml
```
