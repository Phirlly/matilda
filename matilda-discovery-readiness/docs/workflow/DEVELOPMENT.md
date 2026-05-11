# Development Workflow

This repository treats every implementation as user-impacting work. Follow this
workflow for code, documentation, workflow, schema, template, CLI, UI, Ansible,
release, and test changes.

## Required Flow

1. Start from current `dev`.
2. Create a short-lived branch from `dev`.
3. Confirm the working tree is clean or existing changes are understood.
4. Create or update local `PLAN.md` before editing implementation files.
5. Read the current implementation and related files for context.
6. Read related docs, examples, schemas, templates, CLI help, tests, and workflow files.
7. Gather required external documentation before implementation.
8. Cache relevant reference material in `docs/matilda-docs-cache/` when external docs are needed.
9. Update `PLAN.md` with the confirmed approach.
10. Write or update tests first whenever practical.
11. Implement the scoped change.
12. Update user docs, reference docs, workflow docs, schemas, examples, templates, or generated guidance when behavior changes.
13. Run targeted validation for the changed area.
14. Run the standard local validation gate.
15. Review the full diff before pushing.
16. Open a pull request into `dev`.
17. Wait for CI and review before merge.
18. Promote `dev` to `main` only through a separate pull request after validation.

## Planning Requirements

`PLAN.md` is local-only and must not be committed.

For every implementation, `PLAN.md` should include:

- Goal.
- Current branch.
- Files reviewed.
- Documentation reviewed.
- External references gathered, if any.
- Assumptions and how they were verified.
- Proposed implementation.
- Tests to write or update first.
- Validation commands to run.
- User-facing docs or examples to update.
- Risks, rollback notes, or migration notes.

Do not proceed with implementation when the plan depends on an unverified
assumption that can be checked from code, tests, docs, or authoritative external
references.

## Documentation Cache

Use `docs/matilda-docs-cache/` for cached external reference material when
external documentation is needed to implement correctly.

Cached docs should include enough context to be useful later:

- Source name.
- Source URL or retrieval handle.
- Retrieval date.
- Why the reference was needed.
- Relevant excerpt or summary.

Do not cache secrets, customer data, private keys, live inventory, `.env`
content, or generated runtime state.

`docs/matilda-docs-cache/` is local-only unless the project explicitly decides
to publish a specific reference artifact.

## Test-First Rule

Write or update tests before implementation whenever practical.

Use the closest useful test level:

- Unit tests for parsers, validation, formatting, state, reports, and helpers.
- Integration tests for CLI behavior, command routing, generated files, and user-visible output.
- Browser tests for browser UI behavior.
- Ansible syntax checks for Ansible-adjacent changes.
- Operator smoke tests for release, packaging, and first-run workflow changes.

If a test cannot reasonably be added, explain why in the pull request.

## Implementation Rules

Keep changes scoped to the plan.

Do not perform opportunistic refactors while implementing a focused change. Do
not mix unrelated workflow, docs, and product changes in one branch unless they
are required for the same milestone.

Prefer the existing project patterns and simple implementation over new
abstractions.

User-facing behavior must stay simple, explicit, and operator-friendly.

Mutating commands must keep confirmation behavior.

Secrets, private keys, `.env`, live inventory, runtime state, local caches, and
release artifacts must not be committed.

## Required Review Before Push

Before pushing a branch, review:

```bash
git status
git diff --stat
git diff
git diff --check
```

Confirm local-only files are not staged, including:

- `PLAN.md`
- `AGENTS.md`
- `.env`
- `inventory.yml`
- `inventory.v1.yml`
- `.matilda/`
- `.bin/`
- `.ansible/`
- `.gocache/`
- `dist/`
- `docs/matilda-docs-cache/`

## Standard Local Validation Gate

Run these for normal repo changes:

```bash
GOCACHE=/private/tmp/matilda-gocache go test ./...
GOCACHE=/private/tmp/matilda-gocache go vet ./...
GOCACHE=/private/tmp/matilda-gocache go build -o /private/tmp/matilda-prep-check ./cmd/matilda-prep
git diff --check
bash -n matilda-prep
```

For operator UX or command routing changes, also run:

```bash
./matilda-prep help
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep status
```

For Ansible, inventory runner, setup, validation, rollback, or report changes,
also run:

```bash
ANSIBLE_CONFIG=ansible/ansible.cfg ANSIBLE_LOCAL_TEMP=/private/tmp/matilda-ansible-local ansible-playbook --syntax-check ansible/playbooks/linux/preflight.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ANSIBLE_LOCAL_TEMP=/private/tmp/matilda-ansible-local ansible-playbook --syntax-check ansible/playbooks/linux/setup.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ANSIBLE_LOCAL_TEMP=/private/tmp/matilda-ansible-local ansible-playbook --syntax-check ansible/playbooks/linux/validate.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ANSIBLE_LOCAL_TEMP=/private/tmp/matilda-ansible-local ansible-playbook --syntax-check ansible/playbooks/linux/rollback.yml
```

For browser UI changes, verify the UI in a browser at relevant desktop and
mobile widths.

For release candidates or release promotion, run the operator smoke test from a
fresh clone or release package.

## Pull Request Requirements

Pull requests into `dev` must include:

- Summary.
- Files or areas changed.
- Tests added or updated.
- Validation commands run.
- Documentation updates.
- Known risks or test gaps.

Do not merge into `dev` until:

- CI passes.
- The diff has been reviewed.
- Tests and docs match behavior.
- User-facing language is appropriate.
- Local-only artifacts are not included.

Pull requests from `dev` into `main` are promotion pull requests. They must not
contain new direct edits on `main`.

Do not merge into `main` until:

- Source branch is `dev`.
- CI passes.
- Promotion diff is reviewed.
- Required release or smoke validation is complete.
