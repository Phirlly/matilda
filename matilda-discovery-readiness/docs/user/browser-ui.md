# Browser UI

Start the local browser UI:

```bash
./matilda-prep ui
```

The browser UI follows the same workflow model as the Matilda Terminal Console. It shows inventory status, readiness counts, validated IPs, target readiness rows, report files, recent runs, and validation details.

The page is organized as:

- status metrics
- Local, Guidance, and Remote actions
- Activity Log with live streamed output
- target readiness, validated IPs, report files, recent runs, inventory, and validation details

Browser actions match the terminal console menu:

- `doctor`
- `inventory validate`
- `report`
- Validated IPs
- Generate Windows readiness package
- Generate UNIX admin instructions
- `preflight`
- `setup`
- `validate`
- Rollback sudoers

Browser actions start without a full page reload. Output streams into the Activity Log while the action runs, and readiness metrics refresh when the action completes.

Only one browser action can run at a time. This avoids overlapping remote setup, rollback, and validation runs against the same workspace. Running actions can be cancelled from the Activity Log.

Remote browser actions require a populated `.env` file because the browser page cannot collect interactive runtime prompts. Mutating remote actions, such as setup and rollback, also require an explicit confirmation checkbox before they run.

Use `./matilda-prep status` for the same readiness summary in the terminal.
