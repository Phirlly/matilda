# Browser UI

Start the local browser UI:

```bash
./matilda-prep ui
```

The browser UI follows the same workflow model as the Matilda Terminal Console. It shows inventory status, readiness counts, validated IPs, target readiness rows, report files, and validation summaries.

The page is organized as:

- status metrics
- actions grouped as Local, Guidance, and Remote in the same order as the terminal console
- a command-palette action area with label, confirmation, and run columns
- activity log below the action/status area with live streamed output
- target readiness, validated IPs, report files, inventory, and validation summary

Browser actions match the terminal console menu:

- `doctor`
- `inventory validate`
- `report`
- validated IP display
- Generate Windows readiness package
- Generate UNIX admin instructions
- `preflight`
- `setup`
- `validate`
- sudoers-only rollback

Browser actions start without a full page reload. Output streams into the Activity Log while the action runs, and readiness metrics refresh when the action completes.

Only one browser action can run at a time. This avoids overlapping remote setup, rollback, and validation runs against the same workspace. Running actions can be cancelled from the Activity Log.

Remote browser actions require a populated `.env` file because the browser page cannot collect interactive runtime prompts. Mutating remote actions, such as setup and rollback, also require an explicit confirmation checkbox before they run.

The browser UI also exposes `/api/status` for a JSON readiness snapshot.
