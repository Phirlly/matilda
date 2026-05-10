# Matilda Terminal UX Unification Plan

## Purpose

This is the execution plan for making the whole Matilda Discovery Readiness Toolkit feel like one modern terminal product instead of a mix of plain CLI output, a separate console, and a browser UI.

This plan is intentionally repository-wide. It covers command routing, terminal rendering, prompts, browser workflow alignment, cleanup, and tests. It should be updated as implementation proceeds.

## Product Direction

The primary terminal experience is the Matilda Terminal Console.

Running the current entrypoint with no command opens the console:

```bash
./matilda-prep
```

Direct commands remain available for advanced users and automation:

```bash
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep report
./matilda-prep ui
```

Every direct command should render with the same visual system as the console:

1. Context
2. Checks or action scope
3. Progress or activity output
4. Result summary
5. Remediation when there are failures
6. Next recommended action

The browser UI should use the same workflow labels, action grouping, readiness language, and next-step/remediation wording as the terminal. It does not need to look like a literal terminal emulator, but it must feel like the same product.

## Non-Negotiable Constraints

- Keep `matilda-prep` as the current entrypoint unless a rename or install alias is explicitly implemented later.
- Do not reintroduce the old optional terminal command or implementation terminology.
- Keep `./matilda-prep console`, `./matilda-prep start`, and `./matilda-prep status`.
- Keep Go and Ansible as the implementation stack.
- Do not add Python or TypeScript application code.
- Keep Ansible focused on remote setup and validation.
- Preserve the tested Linux workflow while improving the interface.
- Keep Windows and UNIX as generated guidance until remote automation is validated on those platforms.
- Do not hardcode Probe sizing, filesystem, package, URL, or port requirements from informal notes. Curate those requirements into tracked docs or configuration before implementing checks.
- Do not copy private keys to targets.
- Do not delete root `group_vars/` in cleanup unless the Ansible inventory model is intentionally redesigned and tested.
- Keep generated reports, live inventory, `.env`, real keys, and cached docs untracked.

## Current Repository Findings

Implemented now:

- `./matilda-prep` opens `internal/console`.
- `./matilda-prep console` and `./matilda-prep start` open the same console.
- `./matilda-prep status` prints a non-interactive status summary and exits.
- `internal/app/actions.go` provides a shared action list used by the console and browser UI.
- Root, inventory, generate, and rollback help use console-style sections.
- `internal/ui/` provides shared terminal rendering, prompts, errors, and row formatting for direct commands and console flows.
- `internal/console/` is a Bubble Tea console with a simple action menu, full result view, scrollable command output, and confirmation modals.
- The old optional terminal package file is deleted in the working tree.
- Browser UI has been simplified and uses `reports/guidance/` for generated platform guidance.
- The discovery launch reference uses `docs/reference/matilda-discovery-launch.md` instead of ambiguous browser UI wording.

Known gaps that still need implementation:

- There is no structured workflow event layer yet.
- Ansible output still streams raw text.
- Some runtime methods still own command execution and direct sequencing; a later workflow-event phase should split command orchestration from rendering.
- Tests now cover the absence of legacy direct-command output such as `Command` and `Scope` headers.
- A first-pass `.matilda/state.json` state store exists and records latest action status, readiness counts, and report paths.

## Immediate Design Target

Use one terminal rendering system for the current codebase before adding larger workflow-event architecture.

Implemented short-term package structure:

```text
internal/ui/
  terminal.go     shared terminal rendering, prompts, errors, and formatting

internal/console/
  console.go      interactive console screens
  help.go         help screens
```

Avoid adding unused placeholder packages. If `internal/ui`, `internal/workflow`, or `internal/state` are introduced, they must be used by real code in the same implementation phase.

The shared renderer should provide:

- product header
- context panel
- status cards or status rows
- section headings
- key/value rows
- check rows
- action rows
- activity log headings
- file output rows
- warning/error rows
- confirmation prompts
- cancellation output
- summary blocks
- remediation blocks
- next-step blocks

## Command Surface

Keep this clean command surface:

```bash
./matilda-prep
./matilda-prep help
./matilda-prep console
./matilda-prep start
./matilda-prep status
./matilda-prep init
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep inventory import <csv>
./matilda-prep inventory migrate
./matilda-prep preflight
./matilda-prep setup
./matilda-prep validate
./matilda-prep run
./matilda-prep report
./matilda-prep generate windows
./matilda-prep generate unix
./matilda-prep ui
./matilda-prep rollback --sudoers-only
./matilda-prep rollback --remove-key
./matilda-prep rollback --lock-user
./matilda-prep rollback --delete-user
```

Remove or keep removed from user-facing output:

- the old optional terminal command
- `Usage:` / `Commands:` legacy help style
- old generated guidance path wording
- browser duplicate status summaries
- stale launch-reference wording where it conflicts with browser UI terminology

## Immediate Keyboard Console Refactor

This pass is implemented in the current working tree. The console now uses live keyboard navigation while preserving the existing workflow behavior.

Implemented approach:

- Use Bubble Tea for the interactive terminal event loop.
- Use Bubbles for scrollable command output where it fits naturally.
- Use Lip Gloss for consistent layout styling.
- Keep these dependencies limited to the interactive console path.
- Keep direct command output on the shared `internal/ui` renderer.
- Non-interactive invocations print a static status summary and action list instead of entering the event loop.

Package shape for this phase:

```text
internal/console/
  console.go     Run and PrintStatus entrypoints
  model.go       terminal model, selected action, focused pane, modal state
  view.go        simple action menu, full result view, and scrollable activity log
  keys.go        key bindings and key help
  actions.go     action execution adapters
  help.go        help screens
```

Keyboard behavior:

- Up/Down and `k`/`j` move through actions.
- Enter runs the selected action.
- Up/Down and `k`/`j` choose actions in the menu.
- Enter runs the selected action.
- After a workflow action runs, the console switches to a full result view so long output can be reviewed immediately.
- Up/Down, PageUp/PageDown, Home, and End scroll the result view.
- `b` or Esc returns from result view to the action menu.
- Home/End jump to the first or last action, or the top/bottom of the focused pane.
- `r` refreshes the snapshot and local state.
- `q`, Esc, and Ctrl+C quit the console or close the active modal.
- Number keys may remain as secondary shortcuts, but they are no longer the primary interaction model.

Safety behavior:

- Mutating actions open a confirmation modal before execution.
- `y` confirms.
- `n` and Esc cancel.
- Enter alone must not run `setup` or rollback.
- Cancellation is recorded as cancellation, not failure.

Testing required for this phase:

- Unit tests for all key bindings.
- Unit tests for focus movement and scroll bounds.
- Unit tests for mutating confirmation and cancellation.
- Integration tests for default console startup, `console`, `start`, `status`, and help routing.
- Integration tests proving mutating console actions do not execute without confirmation.
- Browser tests confirming action labels and groups remain aligned with the terminal action model.

## Execution Phases

### Phase 1: Plan Reconciliation And Guardrails

Goal: make the repository guidance unambiguous before coding.

Tasks:

- Keep this file as the terminal UX execution plan.
- Update `docs/reference/repository-cleanup-plan.md` so it separates completed UX work from final validation and commit cleanup.
- Treat the partial help refactor as in-progress work, not complete implementation.
- Keep implementation paused until this plan is accepted.

Acceptance:

- The saved docs clearly separate completed cleanup from remaining terminal/browser UX work.
- The next implementation pass can follow this plan without guessing.

### Phase 2: Shared Terminal Renderer

Goal: remove duplicated terminal formatting and make every command use the same components.

Tasks:

- Add shared render components under `internal/console`.
- Move reusable behavior from `internal/app/terminal.go` into those components.
- Replace `Command` and `Scope` headers with the console visual pattern.
- Provide reusable helpers for context, checks, activity, summary, remediation, next step, and file outputs.
- Preserve color behavior and `NO_COLOR`.
- Keep output readable in narrow terminals.

Acceptance:

- Direct command output no longer contains `Command  ...` or `Scope    ...`.
- Direct commands and console screens use the same section names and row styles.
- There are no decorative separator lines such as repeated `----` or `====`.

### Phase 3: Help And Command Guidance

Goal: make all help screens look like the product console, not legacy Cobra-style output.

Tasks:

- Finish root help.
- Finish inventory help.
- Convert generate help.
- Convert rollback help.
- Convert unknown-command help.
- Ensure help clearly explains mutating versus read-only actions.
- Keep the browser UI entrypoint visible but not duplicated.

Acceptance:

- `./matilda-prep help` uses console sections.
- `./matilda-prep inventory help` uses console sections.
- `./matilda-prep generate help` uses console sections.
- `./matilda-prep rollback help` uses console sections.
- Help contains no old optional terminal command.
- Help contains no `Usage:` or `Commands:` legacy blocks.

### Phase 4: Direct Command Output Unification

Goal: every terminal command feels like the same Matilda Terminal Console.

Tasks:

- Convert `init` output and prompts.
- Convert `doctor` output.
- Convert `inventory validate`, `inventory import`, and `inventory migrate`.
- Convert `preflight`, `setup`, `validate`, and `run` wrappers while preserving Ansible execution behavior.
- Convert `report` output.
- Convert `generate windows` and `generate unix`.
- Convert `rollback`.
- Convert validated IP display.
- Add consistent summaries and next steps for success, partial failure, cancellation, and validation failure.

Acceptance:

- Every direct command follows the sequence: context, action/checks, activity, summary, remediation if needed, next step.
- Mutating actions still require explicit confirmation.
- Validation still writes reports even when target validation fails.
- Setup cancellation exits cleanly without continuing into validate/report.

### Phase 5: Prompt And Error UX

Goal: prompts and errors should be as polished as normal output.

Tasks:

- Replace overwrite prompts in `internal/safety/files.go` with shared prompt rendering.
- Replace setup and rollback confirmations with shared confirmation rendering.
- Replace init wizard prompts with shared prompt rendering.
- Route top-level errors through a shared error renderer in `cmd/matilda-prep/main.go` or `internal/cli`.
- Include next-step guidance for common errors:
  - missing `.env`
  - missing `inventory.yml`
  - missing Ansible
  - missing `ansible.posix`
  - unsupported normalized inventory for current Linux runner
  - missing report summary
  - missing local key path

Acceptance:

- Error output is consistent across direct commands and console actions.
- Cancelled mutating actions are visually distinct from failures.
- Common failures tell the user exactly what to run next.

### Phase 6: Browser UI Alignment

Goal: browser UI should be simpler, cleaner, and aligned with the terminal console.

Tasks:

- Keep the top metrics concise.
- Avoid repeating the same status summary in multiple sections.
- Keep action buttons visually even.
- Keep the activity log below the status/action area.
- Use the same action groups and labels as the terminal console.
- Use the same next-step and remediation wording as the terminal console.
- Keep inventory and report views useful without crowding the first screen.
- Keep all browser UI code Go-served with no TypeScript build chain.

Acceptance:

- Browser UI has no redundant status blocks.
- Action rows are visually balanced on desktop and mobile.
- Browser workflow labels match terminal workflow labels.
- Mutating remote actions require explicit confirmation.

### Phase 7: Repository Cleanup

Goal: remove stale implementation after the shared UX is in place.

Tasks:

- Remove old terminal implementation files after replacement is complete.
- Remove tests that only protect removed legacy behavior.
- Update tests that currently expect legacy direct-command output.
- Remove stale docs references to obsolete terminal terminology.
- Keep the Matilda discovery launch reference separate from browser UI documentation.
- Keep only `reports/.gitkeep` tracked under `reports/`.
- Confirm `docs/matilda-docs-cache/` remains untracked.
- Keep platform scaffolds that are documented and syntax-tested.
- Remove only files proven duplicate, stale, or unreachable.

Acceptance:

- No tracked source or docs mention the removed terminal alias.
- No tracked source references the old generated guidance path.
- No generated report artifacts are staged.
- No secrets, live values, real keys, `.env`, or live inventory are staged.

### Phase 8: Tests

Goal: make the UX refactor testable and prevent regression.

Unit tests under `tests/unit/`:

- renderer header/section/check row output
- prompt rendering behavior
- error/next-step rendering
- action grouping labels
- report path rendering
- cancellation result handling

Integration tests under `tests/integration/`:

- default command opens the console
- `console` opens the same console
- `start` opens the same console
- `status` prints the status summary and exits
- root help uses console sections
- inventory help uses console sections
- generate help uses console sections
- rollback help uses console sections
- legacy terminal alias is rejected
- direct commands do not emit `Command` / `Scope` headers
- direct commands do not emit legacy separator lines
- browser labels match workflow action labels
- browser UI has no duplicate status block labels
- Windows guidance writes under `reports/guidance/windows/`
- UNIX guidance writes under `reports/guidance/unix/`

Validation commands:

```bash
go test ./...
go vet ./...
git diff --check
bash -n matilda-prep
```

Command smoke checks:

```bash
./matilda-prep help
./matilda-prep inventory help
./matilda-prep generate help
./matilda-prep rollback help
printf 'q\n' | ./matilda-prep
./matilda-prep status
./matilda-prep doctor
./matilda-prep inventory validate
./matilda-prep generate windows
./matilda-prep generate unix
```

Ansible validation:

- Run syntax checks for all playbooks after Ansible-adjacent changes.
- Run live Linux workflow only when explicitly requested and when valid instances are intentionally in scope.

### Phase 9: Workflow Events And State Store

Goal: make terminal, direct commands, browser UI, JSON output, and future automation share one execution model.

This phase should come after the immediate UX cleanup unless implementation naturally requires it earlier.

Implemented foundation packages:

```text
internal/workflow/
  workflow.go

internal/state/
  store.go
```

Current implementation:

- Direct commands, console actions, and browser actions route through tracked action helpers.
- Each tracked action records started/completed/failed/cancelled result metadata.
- `.matilda/state.json` records workspace, inventory path, latest action, per-action latest status, readiness counts, and report paths.
- `.matilda/` is ignored.

Still future:

- granular check-level event streams for every runtime command
- browser live event streaming while long Ansible runs are active
- stable `--json`, `--plain`, and `--no-color` renderer flags
- full run history beyond latest action state

Target renderers:

- interactive console
- pretty terminal
- plain
- JSON
- browser event stream

State path:

```text
.matilda/state.json
```

State must not contain private keys, secrets, or copied credentials.

Acceptance:

- Commands emit structured start/finish results and can be expanded to granular events.
- Browser UI records the same action state as terminal commands.
- `--json`, `--plain`, and `--no-color` can be added without duplicating command logic.
- Console status can read current files and the local state foundation.

## Definition Of Done

The terminal UX refactor is complete when:

- `./matilda-prep` opens the primary console.
- Direct commands look like the same product, not a separate plain CLI.
- Browser UI uses the same labels, actions, next steps, and remediation language.
- User-facing old optional terminal terminology is gone.
- Legacy `Usage:` / `Commands:` help blocks are gone.
- Direct command `Command` / `Scope` headers are gone.
- Prompts, errors, cancellations, reports, and generated guidance are consistently rendered.
- Tests cover the unified UX.
- `go test ./...`, `go vet ./...`, `git diff --check`, launcher syntax, smoke checks, and Ansible syntax checks pass.
- The working tree contains only intentional tracked changes for the PR.
