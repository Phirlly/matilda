# Branching Workflow

This repository uses a protected-main workflow.

The goal is simple: every code or documentation change is reviewed, tested, and
merged through `dev` before it can reach `main`.

## Branch Roles

- `main`
  - Production/release branch.
  - Must remain deployable.
  - Do not make direct code or documentation changes here.
  - Release tags are created from this branch only after validation.

- `dev`
  - Integration branch.
  - Receives completed work from short-lived branches.
  - Must pass build/test checks before being merged into `main`.
  - Do not use `dev` as a personal working branch.

- Short-lived work branches
  - Created from `dev` for focused work.
  - Use short names such as `test`, `notification`, `identity`, or `fix-health`.
  - Keep each branch scoped to one coherent milestone or fix.

## Required Workflow

1. Start from `dev`.
2. Create a short-lived branch for the work.
3. Read the current implementation and related files before editing.
4. Write a short plan for the change.
5. Implement the scoped change.
6. Update user docs, reference docs, workflow docs, examples, schemas, or tests when behavior or user guidance changes.
7. Run local validation and review the diff before opening a pull request.
8. Open a pull request into `dev`.
9. Review the pull request content and wait for CI to pass.
10. Merge completed work into `dev`.
11. Promote `dev` into `main` with a separate pull request after `dev` is validated.
12. Create release tags from `main` after release validation.

## Main To Dev Sync

Do not sync `main` back into `dev` after every promotion.

When GitHub merges `dev` into `main`, it may create a merge commit that makes
`main` appear ahead of `dev` even when both branches have identical files. That
history-only difference is not a problem.

Sync `main` back into `dev` only when there is a real need:

- `main` has file changes that are not in `dev`.
- GitHub blocks a required pull request because `dev` is behind `main`.
- The `dev` to `main` promotion has conflicts that must be resolved before merge.

When a sync is needed, use a short-lived branch and pull request back into
`dev`. Do not update protected branches directly.

## Before Editing

Before changing files, confirm:

- You are not on `main`.
- The branch was created from current `dev`.
- The working tree is clean or any existing changes are understood.
- The relevant implementation files have been read.
- Related docs, tests, schemas, templates, and examples have been checked.
- The change has a clear plan and a narrow scope.

Do not make opportunistic refactors while performing a workflow, docs, or release
change.

## Test And Documentation Rules

- Behavior changes need tests unless there is a clear reason they cannot be tested.
- User-facing behavior changes need user documentation updates.
- Schema, inventory, report, template, or generated-artifact changes need matching tests or examples when practical.
- Docs-only changes still need spelling, links, and command examples reviewed.
- Ansible-adjacent changes need Linux Ansible syntax checks.
- Browser UI changes need browser verification on desktop and mobile widths.
- Mutating workflow changes need confirmation behavior reviewed.

If no new test is added, state why in the pull request.

## Local Validation Gate

Run the checks that match the change before opening a pull request. For normal
repo changes, use:

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

For release candidates or release promotion, also run the operator smoke test
from a fresh clone or release package.

## Pre-Pull-Request Review Gate

Before pushing a short-lived branch:

- Review `git diff --stat`.
- Review the full diff for every changed file.
- Confirm local-only files are not staged, including `.env`, `inventory.yml`, `inventory.v1.yml`, `.matilda/`, `.bin/`, `.ansible/`, `.gocache/`, `dist/`, `PLAN.md`, `AGENTS.md`, and `docs/matilda-docs-cache/`.
- Confirm the pull request will explain the summary, validation, risks, and any test gaps.
- Confirm no direct change was made on `main`.

## Pull Request Gate

Pull requests into `dev` must not be merged until:

- CI passes.
- The diff has been reviewed.
- Tests and docs match the implemented behavior.
- User-facing language is appropriate for operators.
- Mutating actions still require confirmation.
- Generated files and local runtime state are not included.

Pull requests into `main` must not be merged until:

- The source is `dev`.
- `dev` already contains the completed work through reviewed pull requests.
- CI passes on the `dev` to `main` pull request.
- The promotion pull request diff is reviewed.
- Any required release or live Linux validation has been completed.

If GitHub shows `dev` behind `main`, first check whether there is a file diff.
If there is no file diff and the pull request is not blocked, continue with the
promotion. If the pull request is blocked or `main` has real file changes, follow
the main-to-dev sync rule above.

## Repository Protection Settings

Keep repository settings aligned with this workflow:

- Require pull requests before merging into `dev` and `main`.
- Require CI to pass before merging into `dev` and `main`.
- Prevent force pushes and branch deletion on `dev` and `main`.
- Require review of the pull request diff, tests, docs, and release notes when they are in scope.
- Resolve review conversations before merge when repository settings allow it.

## CI Gate

Pull requests into `dev` and `main` must pass CI before merge. CI runs Go tests,
Go vet, whitespace checks, launcher syntax checks, and Linux Ansible syntax
checks. CI also fails if local-only runtime artifacts are accidentally tracked,
or if a pull request into `main` comes from any source branch other than `dev`.

CI is required, but it is not a substitute for local review. The branch author
still owns the plan, diff review, test selection, documentation review, and final
promotion readiness.

## After Merge

After a pull request merges:

- Sync the local target branch.
- Delete merged short-lived local and remote branches.
- Keep only `main` and `dev` as long-lived branches.
- For release work, tag only from `main` after validation.
