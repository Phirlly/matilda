# Branching and Release Workflow

This repository uses a protected-main workflow.

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

- Short-lived work branches
  - Created from `dev` for focused work.
  - Use short names such as `test`, `notification`, `identity`, or `fix-health`.
  - Keep each branch scoped to one coherent milestone or fix.

## Required Flow

1. Start from `dev`.
2. Create a short-lived branch for the work.
3. Validate the branch before opening a pull request.
4. Merge completed work into `dev`.
5. Merge `dev` into `main` only after build/test checks pass.
6. Create release tags from `main` after validation.
