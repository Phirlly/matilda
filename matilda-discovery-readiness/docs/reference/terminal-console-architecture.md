# Terminal Console Architecture

Maintainer reference. Operators do not need this document to run the toolkit; start with the root [README](../../README.md).

The Matilda Discovery Readiness Toolkit uses one terminal product surface:

- `./matilda-prep` opens the Matilda Terminal Console.
- `./matilda-prep console` and `./matilda-prep start` open the same console.
- `./matilda-prep status` prints a non-interactive status summary and exits.
- Direct commands remain available for advanced users and automation.

The terminal language should stay consistent with the product naming model:

- target readiness
- Probe readiness
- platform readiness

## Design Goals

- Keep `matilda-prep` as the single entrypoint.
- Make direct commands and the interactive console feel like the same product.
- Keep mutating target actions behind explicit confirmation.
- Stream long-running action output into the console result view.
- Keep generated reports, local state, live inventory, and cached docs untracked.
- Keep Go and Ansible responsibilities clearly separated.

## Package Layout

```text
internal/ui/
  terminal.go     shared terminal rendering, prompts, errors, and formatting

internal/console/
  console.go      interactive console entrypoint and non-interactive status
  model.go        Bubble Tea model, state, keys, cancellation
  view.go         action menu, result screen, status text
  keys.go         key bindings
  actions.go      workflow action adapters and streaming
  help.go         help screens

internal/workflow/
  workflow.go     action result model

internal/state/
  store.go        ignored local state file under .matilda/state.json
```

## Command Surface

Supported user commands:

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

The old optional terminal alias is intentionally not part of the command surface.

## Interactive Console Behavior

- Up/Down and `k`/`j` move through actions.
- Enter runs the selected action.
- Number keys are secondary shortcuts, not the primary interaction model.
- Actions open a full result view with scrollable output.
- Up/Down, PageUp/PageDown, Home, and End scroll result output.
- `b` or Esc returns from result view to the action menu after completion.
- `r` refreshes the current status snapshot.
- `q`, Esc, and Ctrl+C quit from normal screens.
- During a running action, `q`, Esc, and Ctrl+C request cancellation.

Mutating actions:

- `setup` and rollback actions require confirmation.
- `y` confirms.
- `n` and Esc cancel.
- Enter alone must not confirm a mutating action.
- Cancellation is recorded distinctly from failure.

## Action Model

`internal/app/actions.go` defines the shared workflow actions used by the terminal console and browser UI.

Action groups:

- Local
- Guidance
- Remote

This keeps browser and terminal labels aligned and avoids duplicated action definitions.

## Output And State

Console workflow actions stream output through the result view while they run. Direct commands continue to write to normal stdout/stderr.

Each tracked action records latest status in:

```text
.matilda/state.json
```

The state file records workspace, inventory path, latest action, per-action latest status, readiness counts, and report paths. It must not contain private keys, secrets, or copied credentials.

## Browser Alignment

The browser UI should use the same action labels, groups, next steps, and readiness language as the terminal console. It does not need to look like a terminal emulator, but it should feel like the same product.

Current browser constraints:

- Go-served UI only.
- No TypeScript or Node build chain.
- Browser actions stream output into the Activity Log with Server-Sent Events.
- Mutating remote actions require explicit confirmation.
- Remote browser actions require `.env` because the browser cannot collect interactive prompts.
- Only one browser action can run at a time per local server.

## Future Extensions

The architecture can be extended with:

- granular check-level workflow events
- `--json`, `--plain`, and `--no-color` output modes
- run history filtering and export
- richer remediation detail views

These extensions should build on the existing shared action, state, and rendering model rather than duplicating command logic.

## Validation

Run these checks after terminal or browser UX changes:

```bash
go test ./...
go vet ./...
git diff --check
bash -n matilda-prep
./matilda-prep help
./matilda-prep status
```

Run Ansible syntax checks after remote workflow changes.
