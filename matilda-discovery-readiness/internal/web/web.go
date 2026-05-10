package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/ui"
)

type pageData struct {
	Title        string
	Snapshot     app.Snapshot
	ActionGroups []app.ActionGroup
	Inventory    string
	SummaryText  string
	Action       *app.ActionResult
}

func Serve(rt *app.Runtime, args []string) error {
	addr := "127.0.0.1:8787"
	if len(args) > 0 && strings.HasPrefix(args[0], "--addr=") {
		addr = strings.TrimPrefix(args[0], "--addr=")
	}

	renderer := ui.New(rt.Out)
	renderer.Header("Browser UI", "Local web interface for the same readiness workflow used by the Matilda Terminal Console.")
	renderer.Section("Server")
	renderer.KeyValues([]ui.KV{
		{Key: "Address", Value: "http://" + addr},
		{Key: "Safety", Value: "mutating remote actions require confirmation"},
	})
	renderer.Next("Open the address above in a browser. Press Ctrl+C here to stop the server.")
	return http.ListenAndServe(addr, Handler(rt))
}

func Handler(rt *app.Runtime) http.Handler {
	jobs := newJobManager(rt)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		render(w, rt, nil)
	})
	mux.HandleFunc("/action", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Redirect(w, req, "/", http.StatusSeeOther)
			return
		}
		if err := parseActionForm(req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result := rt.RunWorkflowAction(req.FormValue("action"), req.FormValue("confirmed") == "yes")
		render(w, rt, &result)
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, rt.Snapshot())
	})
	mux.HandleFunc("/api/actions/start", func(w http.ResponseWriter, req *http.Request) {
		serveStartAction(w, req, jobs)
	})
	mux.HandleFunc("/api/actions/", func(w http.ResponseWriter, req *http.Request) {
		serveActionAPI(w, req, jobs)
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, req *http.Request) {
		result := rt.RunLocalAction("report")
		if !result.OK {
			render(w, rt, &result)
			return
		}
		http.ServeFile(w, req, filepath.Join(rt.Root, "reports", "readiness.html"))
	})
	mux.HandleFunc("/readiness.html", func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, filepath.Join(rt.Root, "reports", "readiness.html"))
	})
	mux.HandleFunc("/inventory", func(w http.ResponseWriter, req *http.Request) {
		serveText(w, filepath.Join(rt.Root, "inventory.yml"))
	})
	mux.HandleFunc("/summary", func(w http.ResponseWriter, req *http.Request) {
		serveText(w, filepath.Join(rt.Root, "reports", "validation-summary.txt"))
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/download/")
		path, ok := downloadPath(rt.Root, name)
		if !ok {
			http.NotFound(w, req)
			return
		}
		http.ServeFile(w, req, path)
	})
	return mux
}

func serveStartAction(w http.ResponseWriter, req *http.Request, jobs *jobManager) {
	if req.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "action start requires POST")
		return
	}
	if err := parseActionForm(req); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	snapshot, err := jobs.Start(req.FormValue("action"), req.FormValue("confirmed") == "yes")
	if err != nil {
		writeAPIError(w, statusForJobError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, snapshot)
}

func parseActionForm(req *http.Request) error {
	if strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data") {
		return req.ParseMultipartForm(1 << 20)
	}
	return req.ParseForm()
}

func serveActionAPI(w http.ResponseWriter, req *http.Request, jobs *jobManager) {
	rest := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/actions/"), "/")
	if rest == "" {
		http.NotFound(w, req)
		return
	}
	parts := strings.Split(rest, "/")
	id := parts[0]

	if len(parts) == 1 && req.Method == http.MethodGet {
		serveJobSnapshot(w, jobs, id)
		return
	}
	if len(parts) == 2 && parts[1] == "events" && req.Method == http.MethodGet {
		serveJobEvents(w, req, jobs, id)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && req.Method == http.MethodPost {
		serveCancelJob(w, jobs, id)
		return
	}
	http.NotFound(w, req)
}

func serveJobSnapshot(w http.ResponseWriter, jobs *jobManager, id string) {
	snapshot, ok := jobs.Snapshot(id)
	if !ok {
		writeAPIError(w, http.StatusNotFound, errJobNotFound.Error())
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func serveJobEvents(w http.ResponseWriter, req *http.Request, jobs *jobManager, id string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	snapshot, events, unsubscribe, ok := jobs.Subscribe(id)
	if !ok {
		writeAPIError(w, http.StatusNotFound, errJobNotFound.Error())
		return
	}
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeSSE(w, "started", jobEvent{JobID: snapshot.ID, Action: snapshot.Action, Status: snapshot.Status})
	if snapshot.Output != "" {
		writeSSE(w, "output", jobEvent{JobID: snapshot.ID, Action: snapshot.Action, Status: snapshot.Status, Text: snapshot.Output})
	}
	if snapshot.Status != jobRunning {
		writeSSE(w, finalEventName(snapshot.Status), jobEvent{JobID: snapshot.ID, Action: snapshot.Action, Status: snapshot.Status, Error: snapshot.Error})
		flusher.Flush()
		return
	}
	flusher.Flush()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			name := "output"
			if event.Text == "" {
				name = finalEventName(event.Status)
			}
			writeSSE(w, name, event)
			flusher.Flush()
			if event.Text == "" && event.Status != jobRunning {
				return
			}
		case <-req.Context().Done():
			return
		}
	}
}

func serveCancelJob(w http.ResponseWriter, jobs *jobManager, id string) {
	snapshot, err := jobs.Cancel(id)
	if err != nil {
		writeAPIError(w, statusForJobError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func writeSSE(w http.ResponseWriter, eventName string, payload jobEvent) {
	content, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\n", eventName)
	fmt.Fprintf(w, "data: %s\n\n", content)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func statusForJobError(err error) int {
	switch {
	case errors.Is(err, errUnknownAction), errors.Is(err, errConfirmationMissing):
		return http.StatusBadRequest
	case errors.Is(err, errJobRunning):
		return http.StatusConflict
	case errors.Is(err, errJobNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func render(w http.ResponseWriter, rt *app.Runtime, action *app.ActionResult) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := pageData{
		Title:        "Matilda Discovery Readiness Toolkit",
		Snapshot:     rt.Snapshot(),
		ActionGroups: app.WorkflowActionGroups(),
		Inventory:    readText(filepath.Join(rt.Root, "inventory.yml")),
		SummaryText:  readText(filepath.Join(rt.Root, "reports", "validation-summary.txt")),
		Action:       action,
	}
	_ = pageTemplate().Execute(w, data)
}

func pageTemplate() *template.Template {
	return template.Must(template.New("page").Funcs(template.FuncMap{
		"statusClass": func(ok bool) string {
			if ok {
				return "ok"
			}
			return "bad"
		},
		"readyClass": func(value string) string {
			if strings.EqualFold(value, "YES") || strings.EqualFold(value, "OK") {
				return "ok"
			}
			if strings.EqualFold(value, "NO") || strings.EqualFold(value, "FAIL") {
				return "bad"
			}
			return "warn"
		},
		"existsText": func(ok bool) string {
			if ok {
				return "Ready"
			}
			return "Missing"
		},
		"actionClass": func(action app.ActionSpec) string {
			if action.Mutating {
				return "danger"
			}
			if action.Remote {
				return "remote"
			}
			return ""
		},
		"base": filepath.Base,
	}).Parse(`<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root{color-scheme:light;--ink:#15191d;--muted:#66717c;--line:#d9dfE6;--bg:#f5f6f4;--panel:#fff;--panel2:#f8faf8;--terminal:#111820;--terminal2:#18222b;--termline:#26323d;--ok:#177245;--bad:#aa3434;--warn:#9a6700;--action:#215f8f;--remote:#6854a3}
    *{box-sizing:border-box}
    body{margin:0;background:var(--bg);color:var(--ink);font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif}
    header{background:#fff;border-bottom:1px solid var(--line);padding:20px 28px}
    .top,main{max-width:1360px;margin:0 auto}
    h1{font-size:24px;line-height:1.15;margin:0 0 6px;letter-spacing:0}
    h3{font-size:12px;margin:14px 0 7px;color:var(--muted);text-transform:uppercase;letter-spacing:0}
    p{margin:0;color:var(--muted)}
    a,.link{color:var(--action);font-weight:650;text-decoration:none}
    button{border:1px solid transparent;background:var(--action);color:white;border-radius:5px;padding:9px 12px;font-weight:700;cursor:pointer;min-height:36px;white-space:nowrap;display:inline-flex;align-items:center;justify-content:center}
    button.remote{background:var(--remote)}
    button.danger{background:#8b3a32}
    main{padding:22px 28px 38px}
    .metrics{display:grid;grid-template-columns:repeat(4,minmax(130px,1fr));gap:10px;margin-bottom:14px}
    .metric{background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:13px;min-height:78px}
    .metric span{display:block;font-size:12px;color:var(--muted);text-transform:uppercase}
    .metric strong{display:block;font-size:28px;margin-top:5px;line-height:1.1}
    .ok{color:var(--ok)}.bad{color:var(--bad)}.warn{color:var(--warn)}
    .shell{background:var(--terminal);border:1px solid var(--termline);border-radius:7px;color:#dde7ef;overflow:hidden;box-shadow:0 12px 34px rgba(17,24,32,.10);margin-bottom:18px}
    .shell-head{display:flex;align-items:center;background:var(--terminal2);border-bottom:1px solid var(--termline);padding:13px 15px}
    .shell-title{font-weight:750}.shell-subtitle{color:#91a1ad;font-size:13px}
    .shell-body{display:block}
    .workspace{padding:15px;min-width:0}
    .terminal-label{font-size:13px;font-weight:750;color:#f1f6fa;margin-bottom:8px}
    .next{background:#f0f6f4;border-left:4px solid var(--ok);border-radius:5px;color:var(--ink);padding:11px 12px;margin-bottom:14px}
    section,.panel{background:var(--panel);border:1px solid var(--line);border-radius:6px;min-width:0;box-shadow:0 1px 2px rgba(20,27,36,.04)}
    section{overflow-x:auto;max-width:100%}
    section h2,.panel h2{font-size:15px;margin:0;padding:12px 14px;border-bottom:1px solid var(--line);letter-spacing:0}
    .body{padding:15px}
    .actions{display:grid;gap:10px}
    .action-row{display:grid;grid-template-columns:minmax(190px,260px) minmax(220px,1fr) 136px;grid-template-areas:"copy confirm button";gap:12px;align-items:center;margin:0;padding:10px;border:1px solid var(--termline);border-radius:6px;background:#141d25;min-height:58px}
    .action-copy{grid-area:copy;min-width:0}
    .action-copy strong{display:block;color:#f2f7fb;font-size:14px;line-height:1.25;margin-bottom:3px}
    .action-copy span{display:block;font-size:13px;color:#aebac4;line-height:1.3}
    .action-confirm{grid-area:confirm;display:flex;align-items:center;justify-content:flex-end;min-height:34px}
    .action-row button{grid-area:button;width:100%}
    .confirm,.confirm-spacer{font-size:12px;color:#d7c6bd;white-space:nowrap}
    .confirm input{vertical-align:-2px;margin-right:6px}
    .action-confirm.empty{visibility:hidden}
    .log{margin-bottom:18px}
    .log pre{max-height:300px;border-top:0;border-radius:0 0 6px 6px;background:#101820;color:#dce7ef}
    .grid{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:18px;margin-top:18px}
    pre{white-space:pre-wrap;margin:0;padding:14px;overflow:auto;max-height:430px;background:#fbfcfd;border-top:1px solid var(--line);border-radius:0 0 6px 6px;font-size:13px;line-height:1.45}
    table{border-collapse:collapse;width:100%;font-size:13px}
    th,td{border-top:1px solid var(--line);padding:9px 10px;text-align:left;vertical-align:top}
    th{background:var(--panel2);font-size:12px;color:var(--muted);text-transform:uppercase}
    .mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
    .pill{display:inline-block;border:1px solid var(--line);border-radius:999px;padding:3px 8px;background:var(--panel2);font-weight:650;margin:0 6px 6px 0}
    @media (max-width:980px){header{padding:18px}main{padding:18px}.grid,.metrics{grid-template-columns:1fr}.action-row{grid-template-columns:minmax(0,1fr);grid-template-areas:"copy" "confirm" "button"}.action-confirm{justify-content:flex-start}.action-confirm.empty{display:none;min-height:0}.confirm-spacer{display:none}.action-row button{width:100%}}
  </style>
</head>
<body>
  <header>
    <div class="top">
      <div>
        <h1>{{.Title}}</h1>
        <p>Target readiness, validated IPs, reports, and platform guidance in one local workspace.</p>
      </div>
    </div>
  </header>
  <main>
    <div class="metrics">
      <div class="metric"><span>Inventory</span><strong id="metric-inventory" class="{{statusClass .Snapshot.InventoryOK}}">{{if .Snapshot.InventoryOK}}OK{{else}}Fix{{end}}</strong></div>
      <div class="metric"><span>Targets</span><strong id="metric-targets">{{.Snapshot.TargetCount}}</strong></div>
      <div class="metric"><span>Ready</span><strong id="metric-ready" class="ok">{{.Snapshot.ReportSummary.Ready}}</strong></div>
      <div class="metric"><span>Needs remediation</span><strong id="metric-remediation" class="bad">{{.Snapshot.ReportSummary.NotReady}}</strong></div>
    </div>
    <div class="shell">
      <div class="shell-head">
        <div>
          <div class="shell-title">Workflow Actions</div>
          <div class="shell-subtitle">Run checks, generate platform guidance, validate targets, and review output.</div>
        </div>
      </div>
      <div class="shell-body">
        <div class="workspace">
          <div class="next"><strong>Next:</strong> <span id="next-step-text">{{.Snapshot.NextStep}}</span></div>
          <div class="terminal-label">Actions</div>
          <div class="actions">
            {{range .ActionGroups}}
              <h3>{{.Name}}</h3>
              {{range .Actions}}
                <form method="post" action="/action" class="action-row">
                  <input type="hidden" name="action" value="{{.ID}}">
                  <div class="action-copy"><strong>{{.Label}}</strong><span>{{.Description}}</span></div>
                  <div class="action-confirm {{if .Mutating}}needs-confirm{{else}}empty{{end}}">{{if .Mutating}}<label class="confirm"><input type="checkbox" name="confirmed" value="yes">Confirm target change</label>{{else}}<span class="confirm-spacer"></span>{{end}}</div>
                  <button class="{{actionClass .}}" data-default-label="Run">Run</button>
                </form>
              {{end}}
            {{end}}
          </div>
        </div>
      </div>
    </div>
    <section class="log">
      <h2>Activity Log</h2>
      <div class="body" style="padding-bottom:0"><button id="cancel-job" class="danger" style="display:none">Cancel running action</button></div>
      {{if .Action}}
        <pre id="activity-log" class="mono">{{.Action.Action}} {{if .Action.OK}}completed{{else}}failed: {{.Action.Error}}{{end}}

{{.Action.Output}}</pre>
      {{else}}
        <pre id="activity-log" class="mono">No browser action has run in this view.</pre>
      {{end}}
    </section>
    <section style="margin-top:18px">
      <h2>Target Readiness</h2>
      <table>
        <thead><tr><th>Host</th><th>Discovery IP</th><th>Ready</th><th>Local sudo</th><th>Denied command</th><th>Probe SSH</th><th>Remediation</th></tr></thead>
        <tbody id="target-rows">
          {{if .Snapshot.ReportRows}}
            {{range .Snapshot.ReportRows}}<tr><td>{{.Host}}</td><td class="mono">{{.DiscoveryIP}}</td><td class="{{readyClass .Ready}}">{{.Ready}}</td><td class="{{readyClass .LocalSudo}}">{{.LocalSudo}}</td><td class="{{readyClass .DeniedCommand}}">{{.DeniedCommand}}</td><td class="{{readyClass .ProbeSSH}}">{{.ProbeSSH}}</td><td>{{.Remediation}}</td></tr>{{end}}
          {{else}}
            <tr><td colspan="7">No validation rows yet. Run validate to populate target readiness.</td></tr>
          {{end}}
        </tbody>
      </table>
    </section>
    <div class="grid">
      <section>
        <h2>Validated IPs</h2>
        <div id="validated-ips" class="body">
          {{if .Snapshot.ValidatedIPs}}
            {{range .Snapshot.ValidatedIPs}}<div class="mono pill">{{.}}</div>{{end}}
          {{else}}
            <p>No validated IPs yet.</p>
          {{end}}
        </div>
      </section>
      <section>
        <h2>Report Files</h2>
        <table>
          <thead><tr><th>File</th><th>Status</th><th>Size</th></tr></thead>
          <tbody id="report-files">
            {{range .Snapshot.ReportFiles}}
              <tr><td>{{if .Exists}}<a href="/download/{{base .Path}}">{{.Name}}</a>{{else}}{{.Name}}{{end}}</td><td class="{{if .Exists}}ok{{else}}warn{{end}}">{{existsText .Exists}}</td><td>{{.Size}}</td></tr>
            {{end}}
          </tbody>
        </table>
      </section>
    </div>
    <div class="grid">
      <section><h2>Inventory</h2><pre>{{.Inventory}}</pre></section>
      <section><h2>Validation Summary</h2><pre>{{.SummaryText}}</pre></section>
    </div>
  </main>

  <script>
    (() => {
      const log = document.getElementById('activity-log');
      const cancelButton = document.getElementById('cancel-job');
      let activeSource = null;
      let activeJobID = "";

      function escapeHTML(value) {
        return String(value ?? '').replace(/[&<>"']/g, (ch) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
      }
      function readyClass(value) {
        const normalized = String(value || '').toUpperCase();
        if (normalized === 'YES' || normalized === 'OK') return 'ok';
        if (normalized === 'NO' || normalized === 'FAIL') return 'bad';
        return 'warn';
      }
      function fileBase(path) {
        return String(path || '').split('/').pop();
      }
      function setButtonsDisabled(disabled) {
        document.querySelectorAll('.action-row button').forEach((button) => {
          button.disabled = disabled;
          if (!disabled) button.textContent = button.dataset.defaultLabel || 'Run';
        });
      }
      function resetLog(label) {
        log.textContent = label + ' started\n\n';
        log.scrollTop = log.scrollHeight;
        document.querySelector('.log').scrollIntoView({block:'start'});
      }
      function appendLog(text) {
        if (!text) return;
        log.textContent += text;
        log.scrollTop = log.scrollHeight;
      }
      async function apiError(response) {
        try {
          const payload = await response.json();
          return payload.error || response.statusText;
        } catch {
          return response.statusText;
        }
      }
      async function refreshStatus() {
        const response = await fetch('/api/status');
        if (!response.ok) return;
        const snapshot = await response.json();
        const summary = snapshot.report_summary || {};
        const inventory = document.getElementById('metric-inventory');
        inventory.textContent = snapshot.inventory_ok ? 'OK' : 'Fix';
        inventory.className = snapshot.inventory_ok ? 'ok' : 'bad';
        document.getElementById('metric-targets').textContent = snapshot.target_count || 0;
        document.getElementById('metric-ready').textContent = summary.Ready ?? summary.ready ?? 0;
        document.getElementById('metric-remediation').textContent = summary.NotReady ?? summary.not_ready ?? 0;
        document.getElementById('next-step-text').textContent = snapshot.next_step || '';

        const targetRows = document.getElementById('target-rows');
        const rows = snapshot.report_rows || [];
        targetRows.innerHTML = rows.length ? rows.map((row) => '<tr><td>' + escapeHTML(row.host) + '</td><td class="mono">' + escapeHTML(row.discovery_ip) + '</td><td class="' + readyClass(row.ready) + '">' + escapeHTML(row.ready) + '</td><td class="' + readyClass(row.local_sudo) + '">' + escapeHTML(row.local_sudo) + '</td><td class="' + readyClass(row.denied_command) + '">' + escapeHTML(row.denied_command) + '</td><td class="' + readyClass(row.probe_ssh) + '">' + escapeHTML(row.probe_ssh) + '</td><td>' + escapeHTML(row.remediation) + '</td></tr>').join('') : '<tr><td colspan="7">No validation rows yet. Run validate to populate target readiness.</td></tr>';

        const ips = snapshot.validated_ips || [];
        document.getElementById('validated-ips').innerHTML = ips.length ? ips.map((ip) => '<div class="mono pill">' + escapeHTML(ip) + '</div>').join('') : '<p>No validated IPs yet.</p>';

        const files = snapshot.report_files || [];
        document.getElementById('report-files').innerHTML = files.map((file) => {
          const name = escapeHTML(file.name);
          const status = file.exists ? 'Ready' : 'Missing';
          const statusClass = file.exists ? 'ok' : 'warn';
          const fileName = encodeURIComponent(fileBase(file.path));
          const label = file.exists ? '<a href="/download/' + fileName + '">' + name + '</a>' : name;
          return '<tr><td>' + label + '</td><td class="' + statusClass + '">' + status + '</td><td>' + escapeHTML(file.size || 0) + '</td></tr>';
        }).join('');
      }
      function finishStream(status, error) {
        if (error) appendLog('\n' + status + ': ' + error + '\n');
        else appendLog('\n' + status + '\n');
        if (activeSource) activeSource.close();
        activeSource = null;
        activeJobID = "";
        cancelButton.disabled = false;
        cancelButton.style.display = 'none';
        setButtonsDisabled(false);
        refreshStatus();
      }
      function openStream(jobID) {
        activeJobID = jobID;
        cancelButton.style.display = 'inline-flex';
        activeSource = new EventSource('/api/actions/' + encodeURIComponent(jobID) + '/events');
        activeSource.addEventListener('output', (event) => {
          const payload = JSON.parse(event.data);
          appendLog(payload.text || '');
        });
        activeSource.addEventListener('completed', (event) => {
          const payload = JSON.parse(event.data);
          finishStream('completed', payload.error || '');
        });
        activeSource.addEventListener('failed', (event) => {
          const payload = JSON.parse(event.data);
          finishStream('failed', payload.error || '');
        });
        activeSource.addEventListener('cancelled', (event) => {
          const payload = JSON.parse(event.data);
          finishStream('cancelled', payload.error || '');
        });
        activeSource.onerror = () => {
          appendLog('\nStream interrupted. Refresh status before starting another action.\n');
          if (activeSource) activeSource.close();
          activeSource = null;
          activeJobID = "";
          cancelButton.disabled = false;
          cancelButton.style.display = 'none';
          setButtonsDisabled(false);
        };
      }
      document.querySelectorAll('form.action-row').forEach((form) => {
        form.addEventListener('submit', async (event) => {
          event.preventDefault();
          const label = form.querySelector('.action-copy strong').textContent;
          const button = form.querySelector('button');
          setButtonsDisabled(true);
          button.textContent = 'Running';
          resetLog(label);
          try {
            const response = await fetch('/api/actions/start', {method: 'POST', body: new FormData(form)});
            if (!response.ok) {
              appendLog('Error: ' + await apiError(response) + '\n');
              setButtonsDisabled(false);
              return;
            }
            const job = await response.json();
            openStream(job.id);
          } catch (err) {
            appendLog('Error: ' + err.message + '\n');
            setButtonsDisabled(false);
          }
        });
      });
      cancelButton.addEventListener('click', async () => {
        if (!activeJobID) return;
        cancelButton.disabled = true;
        await fetch('/api/actions/' + encodeURIComponent(activeJobID) + '/cancel', {method: 'POST'});
        appendLog('\nCancellation requested.\n');
      });
    })();
  </script>
</body>
</html>`))
}

func serveText(w http.ResponseWriter, path string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, readText(path))
}

func readText(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return "Not available: " + err.Error()
	}
	return string(content)
}

func downloadPath(root string, name string) (string, bool) {
	allowed := map[string]string{
		"validated-discovery-ips.txt": "validated-discovery-ips.txt",
		"validation-summary.txt":      "validation-summary.txt",
		"readiness.csv":               "readiness.csv",
		"readiness.json":              "readiness.json",
		"readiness.md":                "readiness.md",
		"readiness.html":              "readiness.html",
	}
	file, ok := allowed[name]
	if !ok {
		return "", false
	}
	return filepath.Join(root, "reports", file), true
}
