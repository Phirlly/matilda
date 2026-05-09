package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/app"
	"matilda-discovery-readiness/internal/web"
)

func TestWebStatusAndDashboardRoutes(t *testing.T) {
	root := withTempProject(t, validLinuxGroupedInventory(), validationSummary())
	writeFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")
	writeFile(t, filepath.Join(root, "reports", "readiness.md"), "# Ready\n")
	rt := app.New(root, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	handler := web.Handler(rt)

	dashboard := httptest.NewRecorder()
	handler.ServeHTTP(dashboard, httptest.NewRequest(http.MethodGet, "/", nil))
	if dashboard.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d", dashboard.Code)
	}
	for _, want := range []string{"Matilda Discovery Readiness Toolkit", "Workflow Actions", "Actions", "Activity Log", "Target Readiness", "Validated IPs", "Report Files", "Preflight", "Setup", "Validate", "Rollback sudoers"} {
		if !strings.Contains(dashboard.Body.String(), want) {
			t.Fatalf("dashboard missing %q:\n%s", want, dashboard.Body.String())
		}
	}
	for _, redundant := range []string{`<div class="terminal-label">Status</div>`, `<div class="terminal-label">Workflow</div>`, `2/2 ready`} {
		if strings.Contains(dashboard.Body.String(), redundant) {
			t.Fatalf("dashboard should not render redundant summary %q:\n%s", redundant, dashboard.Body.String())
		}
	}
	for _, want := range []string{"action-key", "action-copy", "action-confirm", "Confirm target change"} {
		if !strings.Contains(dashboard.Body.String(), want) {
			t.Fatalf("dashboard action palette missing %q:\n%s", want, dashboard.Body.String())
		}
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
	root := withTempProject(t, validLinuxGroupedInventory(), validationSummary())
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
	root := withTempProject(t, validLinuxGroupedInventory(), validationSummary())
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
