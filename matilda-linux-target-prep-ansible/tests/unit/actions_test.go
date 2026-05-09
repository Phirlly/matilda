package unit

import (
	"testing"

	"matilda-discovery-readiness/internal/app"
)

func TestWorkflowActionGroupsKeepSharedUIOrder(t *testing.T) {
	groups := app.WorkflowActionGroups()
	if len(groups) != 3 {
		t.Fatalf("expected 3 action groups, got %d", len(groups))
	}
	for index, want := range []string{"Local", "Handoff", "Remote"} {
		if groups[index].Name != want {
			t.Fatalf("group %d = %q, want %q", index, groups[index].Name, want)
		}
	}

	remote := groups[2].Actions
	if len(remote) != 4 {
		t.Fatalf("expected 4 remote actions, got %d", len(remote))
	}
	if !remote[1].Mutating || remote[1].ID != "setup" {
		t.Fatalf("setup should be marked as a mutating remote action: %+v", remote[1])
	}
	if !remote[3].Mutating || remote[3].ID != "rollback-sudoers" {
		t.Fatalf("rollback should be marked as a mutating remote action: %+v", remote[3])
	}
}
