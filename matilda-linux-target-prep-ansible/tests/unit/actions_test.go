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
	for index, want := range []string{"Local", "Guidance", "Remote"} {
		if groups[index].Name != want {
			t.Fatalf("group %d = %q, want %q", index, groups[index].Name, want)
		}
	}

	guidance := groups[1].Actions
	if len(guidance) != 2 {
		t.Fatalf("expected 2 guidance actions, got %d", len(guidance))
	}
	if guidance[0].Label != "Generate Windows readiness package" {
		t.Fatalf("unexpected Windows guidance label: %+v", guidance[0])
	}
	if guidance[1].Label != "Generate UNIX admin instructions" {
		t.Fatalf("unexpected UNIX guidance label: %+v", guidance[1])
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
