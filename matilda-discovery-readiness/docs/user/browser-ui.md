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
- activity log below the action/status area
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

Remote browser actions require a populated `.env` file because the browser page cannot collect interactive runtime prompts. Mutating remote actions, such as setup and rollback, also require an explicit confirmation checkbox before they run.

The browser UI also exposes `/api/status` for a JSON readiness snapshot.
