package app

type ActionSpec struct {
	ID          string
	Key         string
	Label       string
	Description string
	Group       string
	Remote      bool
	Mutating    bool
}

type ActionGroup struct {
	Name    string
	Actions []ActionSpec
}

func WorkflowActions() []ActionSpec {
	return []ActionSpec{
		{ID: "doctor", Key: "1", Label: "Doctor", Description: "local prerequisite checks", Group: "Local"},
		{ID: "inventory-validate", Key: "2", Label: "Inventory validate", Description: "read-only inventory checks", Group: "Local"},
		{ID: "report", Key: "3", Label: "Generate reports", Description: "CSV, JSON, Markdown, HTML", Group: "Local"},
		{ID: "validated-ips", Key: "4", Label: "Validated IPs", Description: "ready IPs for Matilda", Group: "Local"},
		{ID: "generate-windows", Key: "5", Label: "Generate Windows readiness package", Description: "PowerShell checks and review notes", Group: "Guidance"},
		{ID: "generate-unix", Key: "6", Label: "Generate UNIX admin instructions", Description: "AIX, Solaris, and HP-UX guidance", Group: "Guidance"},
		{ID: "preflight", Key: "7", Label: "Preflight", Description: "read-only remote checks", Group: "Remote", Remote: true},
		{ID: "setup", Key: "8", Label: "Setup", Description: "modifies targets; asks again", Group: "Remote", Remote: true, Mutating: true},
		{ID: "validate", Key: "9", Label: "Validate", Description: "remote checks and reports", Group: "Remote", Remote: true},
		{ID: "run", Key: "0", Label: "Run full workflow", Description: "preflight, setup, validate; asks again", Group: "Remote", Remote: true, Mutating: true},
		{ID: "rollback-sudoers", Key: "s", Label: "Rollback sudoers", Description: "remove sudoers; asks again", Group: "Remote", Remote: true, Mutating: true},
		{ID: "rollback-remove-key", Key: "x", Label: "Rollback remove key", Description: "remove public key; asks again", Group: "Remote", Remote: true, Mutating: true},
		{ID: "rollback-lock-user", Key: "l", Label: "Rollback lock user", Description: "lock service account; asks again", Group: "Remote", Remote: true, Mutating: true},
		{ID: "rollback-delete-user", Key: "d", Label: "Rollback delete user", Description: "delete account and home; asks again", Group: "Remote", Remote: true, Mutating: true},
	}
}

func WorkflowActionByID(id string) (ActionSpec, bool) {
	for _, action := range WorkflowActions() {
		if action.ID == id {
			return action, true
		}
	}
	return ActionSpec{}, false
}

func WorkflowActionGroups() []ActionGroup {
	var groups []ActionGroup
	index := map[string]int{}
	for _, action := range WorkflowActions() {
		position, ok := index[action.Group]
		if !ok {
			index[action.Group] = len(groups)
			groups = append(groups, ActionGroup{Name: action.Group})
			position = len(groups) - 1
		}
		groups[position].Actions = append(groups[position].Actions, action)
	}
	return groups
}
