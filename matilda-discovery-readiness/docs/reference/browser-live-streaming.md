# Browser Live Streaming Design

Maintainer reference. Operators do not need this document to run the toolkit; start with the root [README](../../README.md) or [Browser UI](../user/browser-ui.md).

The browser UI streams workflow action output so it matches the Matilda Terminal Console behavior.

## Goals

- Start browser actions without a full page reload.
- Stream stdout and stderr into the Activity Log while actions run.
- Reuse `app.RunWorkflowActionTo` so command behavior stays shared with the terminal console.
- Keep the UI Go-served with plain inline JavaScript.
- Keep mutating target actions behind explicit confirmation.
- Keep remote browser actions dependent on `.env`, because the browser cannot collect interactive prompts.

## Safety Rules

- Only one browser action may run at a time per local server.
- A second action request while one is running returns `409 Conflict`.
- Mutating actions return `400 Bad Request` unless `confirmed=yes` is provided.
- Unknown actions return `400 Bad Request`.
- Browser jobs support cancellation through a cancel endpoint.
- Job output is kept in memory only and is bounded so long Ansible output cannot grow without limit.
- Job output and local state must not contain secrets, private keys, or copied credentials.

## Backend Shape

```text
internal/web/jobs.go
```

The job manager owns:

- job id generation
- active-job enforcement
- action validation
- output fan-out to Server-Sent Events subscribers
- bounded output snapshots for reconnects
- cancellation through context cancellation
- final status: `running`, `completed`, `failed`, or `cancelled`

## HTTP API

```text
POST /api/actions/start
GET  /api/actions/{id}
GET  /api/actions/{id}/events
POST /api/actions/{id}/cancel
```

`POST /api/actions/start` accepts form values:

```text
action=<workflow-action-id>
confirmed=yes
```

The response is a job snapshot that includes `id`, `action`, `status`, `output`, and `error`.

## SSE Events

The event stream emits:

```text
started
output
completed
failed
cancelled
```

Each event uses JSON data:

```json
{
  "job_id": "job-...",
  "action": "validate",
  "status": "running",
  "text": "..."
}
```

## Browser Behavior

- Action forms remain valid POST fallbacks.
- JavaScript intercepts action form submissions.
- The Activity Log clears for the new action.
- The browser posts to `/api/actions/start`.
- The browser opens an `EventSource` to `/api/actions/{id}/events`.
- Output events append live to Activity Log.
- Final events re-enable action buttons and refresh readiness metrics from `/api/status`.
- Cancel requests post to `/api/actions/{id}/cancel`.

## Dashboard Layout

The browser dashboard keeps the readiness workflow visible without duplicating the terminal status summary:

- Top metrics show inventory health, target count, ready targets, and remediation count.
- Workflow actions are compact rows grouped by Local, Guidance, and Remote.
- Read-only action rows do not reserve confirmation space.
- Mutating action rows show the `Confirm target change` checkbox before they can start.
- Activity Log stays directly below the action palette so streaming output is close to the command that started it.
- Target Readiness, Validated IPs, and Report Files remain first-class dashboard sections.
- Raw `inventory.yml` and `validation-summary.txt` content lives in collapsed detail panels named `Inventory file` and `Validation details`.
- On mobile, Target Readiness and Report Files use labeled stacked rows instead of forcing page-level horizontal scrolling.

## Tests

Required coverage:

- browser HTML contains streaming client hooks.
- browser HTML contains compact action rows and collapsed detail panels.
- browser HTML contains responsive Target Readiness and Report Files table labels.
- read-only action start returns a job id.
- SSE emits started, output, and completed events.
- job status endpoint returns completed output.
- mutating action without confirmation returns `400`.
- unknown action returns `400`.
- concurrent action while one job is running returns `409`.
- cancellation emits a `cancelled` terminal event.

## Validation

Run:

```bash
GOCACHE=/private/tmp/matilda-gocache go test ./...
GOCACHE=/private/tmp/matilda-gocache go vet ./...
GOCACHE=/private/tmp/matilda-gocache go build -o /private/tmp/matilda-prep-check ./cmd/matilda-prep
git diff --check
bash -n matilda-prep
./matilda-prep help
./matilda-prep status
./matilda-prep inventory validate
```

Run Ansible syntax checks after remote workflow changes.
