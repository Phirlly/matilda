package unit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/state"
	"matilda-discovery-readiness/internal/workflow"
)

func TestStateStoreWritesActionAndReadiness(t *testing.T) {
	root := t.TempDir()
	store := state.New(root)
	result := workflow.Start("inventory-validate")
	result.Finish(nil, false)

	doc, err := store.Update(state.Update{
		Workspace: root,
		Inventory: "inventory.yml",
		Result:    result,
		Readiness: state.ReadinessState{Total: 2, Ready: 1, NotReady: 1},
		Reports: state.ReportState{
			LatestHTML:   "reports/readiness.html",
			ValidatedIPs: "reports/validated-discovery-ips.txt",
		},
	})
	if err != nil {
		t.Fatalf("state update failed: %v", err)
	}
	if doc.LastAction != "inventory-validate" || doc.LastStatus != workflow.StatusCompleted {
		t.Fatalf("unexpected last action: %+v", doc)
	}
	if doc.Readiness.Total != 2 || doc.Readiness.Ready != 1 || doc.Readiness.NotReady != 1 {
		t.Fatalf("unexpected readiness state: %+v", doc.Readiness)
	}
	if _, err := os.Stat(filepath.Join(root, ".matilda", "state.json")); err != nil {
		t.Fatalf("expected state file: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, ".matilda", "state.json"))
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if strings.Contains(string(content), "PRIVATE_KEY") {
		t.Fatalf("state file should not contain secret-like keys:\n%s", string(content))
	}
}

func TestStateStoreMissingReadIsExplicit(t *testing.T) {
	_, err := state.New(t.TempDir()).Read()
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("missing state err = %v, want ErrNotFound", err)
	}
}
