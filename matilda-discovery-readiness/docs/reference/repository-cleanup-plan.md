# Repository Cleanup Plan

## Purpose

Keep the Matilda Discovery Readiness Toolkit clean, simple, and aligned with the current product direction:

- `matilda-prep` opens the Matilda Terminal Console by default.
- Direct commands remain available for advanced users and automation.
- Go owns local UX, orchestration, validation, state, and reports.
- Ansible owns remote setup and validation.
- Linux remains the tested automation baseline.
- Windows and UNIX remain generated guidance until validated remote automation exists.

This plan should be updated as cleanup work proceeds.

## Current Status

Cleanup and the terminal UX unification foundation are implemented in the current working tree. Keyboard-driven console navigation is implemented and tracked in `docs/reference/terminal-console-architecture.md`. `PLAN.md` is a local ignored working plan for the current development session.

Completed:

- The old terminal command alias has been removed from CLI routing in the current working tree.
- `console`, `start`, and `status` are the intended clean terminal command surface.
- `internal/console/` is the active terminal package.
- `internal/ui/` is the shared terminal rendering package.
- `internal/workflow/` and `internal/state/` provide the first workflow result and `.matilda/state.json` foundation.
- The old optional terminal package file is deleted in the current working tree.
- Generated Windows and UNIX guidance now uses `reports/guidance/`.
- Direct command output no longer uses legacy `Command` / `Scope` headers.
- Help, prompts, top-level errors, browser startup output, report paths, and guidance output use the shared terminal renderer.
- The current console uses a simple keyboard action menu, full result view, scroll keys, and confirmation modals instead of a dashboard or typed-number prompt.
- The ambiguous browser UI launch reference was renamed to `docs/reference/matilda-discovery-launch.md`.
- Root `group_vars/` is confirmed active and should remain.

Validation completed before the keyboard console refactor:

- `go test ./...`
- `go vet ./...`
- `git diff --check`
- `bash -n matilda-prep`
- command smoke checks for help, inventory help, generate help, rollback help, default console, status, doctor, inventory validate, and platform guidance generation
- Ansible syntax checks for all playbooks

Still required before commit:

- Review the final git diff.
- Stage only intentional tracked files.
- Keep ignored local `.env`, `inventory.yml`, generated reports, caches, and real key material unstaged.

Validation completed after the keyboard console refactor:

- `go test ./...`
- `go vet ./...`
- `git diff --check`
- `bash -n matilda-prep`
- command smoke checks for help, inventory help, generate help, rollback help, default console, status, doctor, inventory validate, and platform guidance generation
- Ansible syntax checks for all playbooks with `ANSIBLE_CONFIG=ansible/ansible.cfg`

## Cleanup Principles

- Remove stale legacy implementation when it is no longer used.
- Do not remove compatibility that protects real active workflows unless explicitly decided.
- Keep user-facing terminology consistent.
- Keep generated/runtime output ignored.
- Keep active Ansible behavior intact unless the change is explicitly tested.
- Do not track secrets, private keys, live inventory, local reports, or cached documentation.

## Confirmed Decisions

- `internal/console/` is the active terminal package.
- The old terminal package is legacy and should not remain.
- The old terminal command alias should be removed for a clean product surface.
- `PLAN.md` is ignored and used as a local working plan for the current development session.
- PR-trackable reference plans belong under `docs/reference/`.
- `reports/guidance/` is the correct generated path for Windows readiness packages and UNIX admin instructions.
- Root `group_vars/` is active and must not be removed in this cleanup pass.

## Cleanup Scope

### 1. Terminal Legacy

Remove legacy terminal artifacts:

- Remove the empty local legacy terminal directory.
- Remove the old terminal alias from CLI routing.
- Remove the old terminal alias from help output.
- Remove tests that only protect the legacy terminal alias.
- Keep and test:
  - `./matilda-prep`
  - `./matilda-prep console`
  - `./matilda-prep start`
  - `./matilda-prep status`

### 2. Terminology

Use current product language:

- Matilda Terminal Console
- browser UI
- Windows readiness package
- UNIX admin instructions
- platform guidance

Remove stale wording:

- implementation-specific terminal terminology
- legacy wording for generated Windows/UNIX guidance
- legacy generated guidance path references

### 3. Generated Output

Keep only `reports/.gitkeep` tracked.

Before commit, remove local ignored generated files when they are not needed as live evidence:

- `reports/readiness.csv`
- `reports/readiness.json`
- `reports/readiness.md`
- `reports/readiness.html`
- `reports/validated-discovery-ips.txt`
- `reports/validation-summary.txt`
- `reports/guidance/`
- old generated guidance path if present

Keep `.gitignore` rules that ignore generated reports.

### 4. Ansible Layout

Keep root `group_vars/` for now.

Reason:

- `ansible/ansible.cfg` uses `inventory = ../inventory.yml`.
- The active inventory is at repository root.
- Root `group_vars/` is adjacent to the active inventory model and supplies current Linux defaults.

Do not move `group_vars/` to `ansible/group_vars/` unless the Ansible inventory/config model is intentionally redesigned and syntax/live tested.

Keep platform scaffolds that are documented and syntax-tested:

- `ansible/playbooks/unix/`
- `ansible/playbooks/windows/`
- `ansible/playbooks/cloud/`
- `ansible/playbooks/kubernetes/`
- matching scaffold roles

Remove only files proven to be duplicate, stale, or unreachable.

### 5. Documentation

README should remain focused on:

- quick start
- core commands
- safety notes
- platform status
- links to deeper docs

Detailed architecture and cleanup plans belong under `docs/reference/`.

Keep the discovery launch reference under `docs/reference/matilda-discovery-launch.md` so it is not confused with the browser UI.

### 6. Tests

Keep tests under:

- `tests/unit/`
- `tests/integration/`

Required test coverage after cleanup:

- default command opens console
- `console` opens console
- `start` opens console
- `status` prints a non-interactive status summary and exits
- console supports keyboard navigation and result output scrolling
- mutating console actions require explicit confirmation
- help output has no legacy terminal alias
- browser UI uses Guidance labels
- generated Windows files write under `reports/guidance/windows/`
- generated UNIX files write under `reports/guidance/unix/`
- no tracked source references the old generated guidance path

### 7. Final Validation

Run:

```bash
go test ./...
go vet ./...
git diff --check
bash -n matilda-prep
```

Run command smoke checks:

```bash
./matilda-prep help
printf 'q\n' | ./matilda-prep
./matilda-prep status
./matilda-prep inventory validate
./matilda-prep generate windows
./matilda-prep generate unix
```

Run Ansible syntax checks for all playbooks after Ansible-related cleanup.

## Out Of Scope For This Cleanup

- Rewriting runtime commands to granular check-level workflow events.
- Adding full run history beyond latest action state.
- Adding Probe prerequisite checks.
- Renaming the binary from `matilda-prep` to `matilda`.
- Moving `group_vars/`.
- Adding Windows or UNIX remote automation.

Those items belong to later implementation phases after the repository is clean.
