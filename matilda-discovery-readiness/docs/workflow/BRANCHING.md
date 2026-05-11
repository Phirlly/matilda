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
3. Follow the implementation process in [Development Workflow](DEVELOPMENT.md).
4. Open a pull request into `dev`.
5. Review the pull request content and wait for CI to pass.
6. Merge completed work into `dev`.
7. Promote `dev` into `main` with a separate pull request after `dev` is validated.
8. Create release tags from `main` after release validation.

## Merge Method Rules

Use the merge method that matches the branch role:

- Short-lived work branch into `dev`
  - Squash merge is acceptable when the branch represents one focused change.
  - A normal merge commit is also acceptable when preserving branch history is useful.

- `dev` into `main`
  - Use a normal merge commit.
  - Do not squash or rebase this promotion.
  - Preserving ancestry keeps future `dev` to `main` pull requests clean.

- `main` back into `dev` sync branch
  - Use a normal merge commit.
  - Do not squash or rebase this sync.
  - The sync exists to repair or preserve branch ancestry.

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

## Implementation Gate

Before opening a pull request, complete the planning, documentation, test-first,
validation, and diff-review requirements in [Development Workflow](DEVELOPMENT.md).

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
- The promotion will use a normal merge commit, not squash or rebase.

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
- Keep normal merge commits available for `dev` to `main` promotions and
  main-to-dev sync pull requests.

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
