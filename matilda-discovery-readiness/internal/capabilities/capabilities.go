package capabilities

type Capability struct {
	Name      string
	Platform  string
	Status    string
	Lifecycle []string
}

func Registry() []Capability {
	return []Capability{
		{Name: "Linux target readiness", Platform: "linux", Status: "implemented", Lifecycle: []string{"doctor", "inventory validate", "preflight", "setup", "validate", "report"}},
		{Name: "UNIX admin instructions", Platform: "unix", Status: "scaffolded", Lifecycle: []string{"inventory validate", "generate", "report"}},
		{Name: "Windows readiness package", Platform: "windows", Status: "scaffolded", Lifecycle: []string{"inventory validate", "generate", "report"}},
		{Name: "Cloud API readiness", Platform: "cloud", Status: "scaffolded", Lifecycle: []string{"inventory validate", "validate", "report"}},
		{Name: "Kubernetes readiness", Platform: "kubernetes", Status: "scaffolded", Lifecycle: []string{"inventory validate", "validate", "report"}},
	}
}
