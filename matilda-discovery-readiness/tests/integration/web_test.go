package integration

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/web"
)

func TestWebStatusAndDashboardRoutes(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	writeFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")
	writeFile(t, filepath.Join(root, "reports", "readiness.md"), "# Ready\n")
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)

	dashboard := httptest.NewRecorder()
	handler.ServeHTTP(dashboard, httptest.NewRequest(http.MethodGet, "/", nil))
	if dashboard.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d", dashboard.Code)
	}
	for _, want := range []string{"Matilda Discovery Readiness Toolkit", "Workflow Actions", "Actions", "Guidance", "Activity Log", "Target Readiness", "Validated IPs", "Report Files", "Recent Runs", "Inventory file", "Validation details", "Preflight", "Setup", "Validate", "Rollback sudoers"} {
		if !strings.Contains(dashboard.Body.String(), want) {
			t.Fatalf("dashboard missing %q:\n%s", want, dashboard.Body.String())
		}
	}
	for _, redundant := range []string{`<div class="terminal-label">Status</div>`, `<div class="terminal-label">Workflow</div>`, `2/2 ready`, `<section><h2>Inventory</h2><pre>`, `<section><h2>Validation Summary</h2><pre>`, "confirm-spacer"} {
		if strings.Contains(dashboard.Body.String(), redundant) {
			t.Fatalf("dashboard should not render redundant summary %q:\n%s", redundant, dashboard.Body.String())
		}
	}
	for _, want := range []string{"action-copy", "action-confirm", "action-row readonly", "action-row mutating", "responsive-table readiness-table", "responsive-table files-table", "responsive-table runs-table", `data-label="Discovery IP"`, `data-label="File"`, `id="run-history"`, "detail-grid", "detail-panel", "Confirm target change", "/api/actions/start", "EventSource", "cancel-job"} {
		if !strings.Contains(dashboard.Body.String(), want) {
			t.Fatalf("dashboard action palette missing %q:\n%s", want, dashboard.Body.String())
		}
	}
	if strings.Contains(dashboard.Body.String(), "action-key") {
		t.Fatalf("dashboard should not make numeric keys the primary browser affordance:\n%s", dashboard.Body.String())
	}

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d", status.Code)
	}
	var snap app.Snapshot
	if err := json.Unmarshal(status.Body.Bytes(), &snap); err != nil {
		t.Fatalf("status JSON failed: %v", err)
	}
	if !snap.InventoryOK || snap.TargetCount != 1 || snap.ReportSummary.Ready != 1 {
		t.Fatalf("unexpected status payload: %+v", snap)
	}
	if len(snap.ReportRows) != 1 || snap.ReportRows[0].Host != "app01" {
		t.Fatalf("unexpected report rows: %+v", snap.ReportRows)
	}
}

func TestWebMutatingActionRequiresConfirmation(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)

	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=setup"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("action status = %d", resp.Code)
	}
	for _, want := range []string{"Activity Log", "setup failed", "requires confirmation"} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("confirmation result missing %q:\n%s", want, resp.Body.String())
		}
	}
}

func TestWebLocalActionRunsInventoryValidate(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)

	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader("action=inventory-validate"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("action status = %d", resp.Code)
	}
	for _, want := range []string{"Activity Log", "inventory-validate", "completed"} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("action result missing %q:\n%s", want, resp.Body.String())
		}
	}
	if strings.Index(resp.Body.String(), "Activity Log") < strings.Index(resp.Body.String(), "Actions") {
		t.Fatalf("activity log should render below actions:\n%s", resp.Body.String())
	}
}

type webJobResponse struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Status string `json:"status"`
	Output string `json:"output"`
	Error  string `json:"error"`
}

func TestWebStreamingActionAPI(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)

	job := startBrowserJob(t, handler, "inventory-validate", false)
	if job.ID == "" || job.Status != "running" {
		t.Fatalf("unexpected start response: %+v", job)
	}

	events := httptest.NewRecorder()
	handler.ServeHTTP(events, httptest.NewRequest(http.MethodGet, "/api/actions/"+job.ID+"/events", nil))
	if events.Code != http.StatusOK {
		t.Fatalf("events status = %d\n%s", events.Code, events.Body.String())
	}
	for _, want := range []string{"event: started", "event: output", "event: completed", "Inventory valid"} {
		if !strings.Contains(events.Body.String(), want) {
			t.Fatalf("event stream missing %q:\n%s", want, events.Body.String())
		}
	}

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/actions/"+job.ID, nil))
	if status.Code != http.StatusOK {
		t.Fatalf("job status code = %d", status.Code)
	}
	var completed webJobResponse
	if err := json.Unmarshal(status.Body.Bytes(), &completed); err != nil {
		t.Fatalf("job status JSON failed: %v", err)
	}
	if completed.Status != "completed" || !strings.Contains(completed.Output, "Inventory valid") {
		t.Fatalf("unexpected completed job: %+v", completed)
	}
	snapResp := httptest.NewRecorder()
	handler.ServeHTTP(snapResp, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var snap app.Snapshot
	if err := json.Unmarshal(snapResp.Body.Bytes(), &snap); err != nil {
		t.Fatalf("status JSON failed: %v", err)
	}
	if len(snap.Runs) == 0 || snap.Runs[0].Action != "inventory-validate" {
		t.Fatalf("browser job should create a run record: %+v", snap.Runs)
	}
}

func TestWebStreamingActionAPIAcceptsBrowserFormData(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("action", "inventory-validate"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/actions/start", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("start status = %d\n%s", resp.Code, resp.Body.String())
	}
	var job webJobResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &job); err != nil {
		t.Fatalf("start JSON failed: %v", err)
	}
	events := httptest.NewRecorder()
	handler.ServeHTTP(events, httptest.NewRequest(http.MethodGet, "/api/actions/"+job.ID+"/events", nil))
	if events.Code != http.StatusOK || !strings.Contains(events.Body.String(), "event: completed") {
		t.Fatalf("events did not complete: %d\n%s", events.Code, events.Body.String())
	}
}

func TestWebStreamingMutatingActionRequiresConfirmation(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)

	req := httptest.NewRequest(http.MethodPost, "/api/actions/start", strings.NewReader("action=setup"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("start status = %d\n%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "requires confirmation") {
		t.Fatalf("confirmation error missing:\n%s", resp.Body.String())
	}
}

func TestWebStreamingUnknownActionFails(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)

	req := httptest.NewRequest(http.MethodPost, "/api/actions/start", strings.NewReader("action=missing"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("start status = %d\n%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "unknown workflow action") {
		t.Fatalf("unknown action error missing:\n%s", resp.Body.String())
	}
}

func TestWebStreamingRejectsConcurrentAction(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	binDir := t.TempDir()
	fakeGo := filepath.Join(binDir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nsleep 1\necho go version go1.25.0 test\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)
	first := startBrowserJob(t, handler, "doctor", false)

	req := httptest.NewRequest(http.MethodPost, "/api/actions/start", strings.NewReader("action=inventory-validate"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("concurrent start status = %d\n%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "already running") {
		t.Fatalf("concurrent error missing:\n%s", resp.Body.String())
	}

	events := httptest.NewRecorder()
	handler.ServeHTTP(events, httptest.NewRequest(http.MethodGet, "/api/actions/"+first.ID+"/events", nil))
	if events.Code != http.StatusOK {
		t.Fatalf("first events status = %d\n%s", events.Code, events.Body.String())
	}
}

func TestWebStreamingActionCanBeCancelled(t *testing.T) {
	root := withTempProject(t, validV1Inventory(), validationSummary())
	binDir := t.TempDir()
	fakeAnsible := filepath.Join(binDir, "ansible-playbook")
	if err := os.WriteFile(fakeAnsible, []byte("#!/bin/sh\necho fake ansible-playbook started\nsleep 5\necho fake ansible-playbook completed\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	keyPath := filepath.Join(root, "keys", "test-key")
	writeFile(t, keyPath, "test key\n")
	writeFile(t, filepath.Join(root, ".env"), strings.Join([]string{
		"TARGET_ADMIN_USER=opc",
		"TARGET_ADMIN_PRIVATE_KEY_FILE=" + keyPath,
		"MATILDA_PROBE_ANSIBLE_HOST=10.0.0.5",
		"MATILDA_PROBE_USER=opc",
		"MATILDA_PROBE_ADMIN_PRIVATE_KEY_FILE=" + keyPath,
		"MATILDA_PUBLIC_KEY_FILE=" + keyPath,
		"MATILDA_PROBE_PRIVATE_KEY_ON_PROBE=/home/opc/.ssh/matilda",
	}, "\n")+"\n")

	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)
	job := startBrowserJob(t, handler, "preflight", false)

	cancel := httptest.NewRecorder()
	handler.ServeHTTP(cancel, httptest.NewRequest(http.MethodPost, "/api/actions/"+job.ID+"/cancel", nil))
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status = %d\n%s", cancel.Code, cancel.Body.String())
	}

	events := httptest.NewRecorder()
	handler.ServeHTTP(events, httptest.NewRequest(http.MethodGet, "/api/actions/"+job.ID+"/events", nil))
	if events.Code != http.StatusOK {
		t.Fatalf("events status = %d\n%s", events.Code, events.Body.String())
	}
	for _, want := range []string{"Cancellation requested", "event: cancelled"} {
		if !strings.Contains(events.Body.String(), want) {
			t.Fatalf("cancel stream missing %q:\n%s", want, events.Body.String())
		}
	}
}

func startBrowserJob(t *testing.T, handler http.Handler, action string, confirmed bool) webJobResponse {
	t.Helper()
	body := "action=" + action
	if confirmed {
		body += "&confirmed=yes"
	}
	req := httptest.NewRequest(http.MethodPost, "/api/actions/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("start status = %d\n%s", resp.Code, resp.Body.String())
	}
	var job webJobResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &job); err != nil {
		t.Fatalf("start JSON failed: %v", err)
	}
	return job
}
