package assessment

import "testing"

func TestParseAssessmentTerms(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "rapid assessment", in: "rapid-assessment", want: "rapid-assessment"},
		{name: "deep discovery", in: "deep-discovery", want: "deep-discovery"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGoal(tt.in)
			if err != nil {
				t.Fatalf("ParseGoal(%q) returned error: %v", tt.in, err)
			}
			if string(got) != tt.want {
				t.Fatalf("ParseGoal(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseCollectionPath(t *testing.T) {
	for _, value := range []string{"billing", "api"} {
		got, err := ParseCollectionPath(value)
		if err != nil {
			t.Fatalf("ParseCollectionPath(%q) returned error: %v", value, err)
		}
		if string(got) != value {
			t.Fatalf("ParseCollectionPath(%q) = %q, want %q", value, got, value)
		}
	}
}

func TestParseProviders(t *testing.T) {
	for _, value := range []string{"aws", "azure", "gcp", "oci"} {
		got, err := ParseProvider(value)
		if err != nil {
			t.Fatalf("ParseProvider(%q) returned error: %v", value, err)
		}
		if string(got) != value {
			t.Fatalf("ParseProvider(%q) = %q, want %q", value, got, value)
		}
	}
}

func TestParseActions(t *testing.T) {
	for _, value := range []string{"preflight", "apply-prereqs", "validate", "package"} {
		got, err := ParseAction(value)
		if err != nil {
			t.Fatalf("ParseAction(%q) returned error: %v", value, err)
		}
		if string(got) != value {
			t.Fatalf("ParseAction(%q) = %q, want %q", value, got, value)
		}
	}
}

func TestInvalidTermsFailClosed(t *testing.T) {
	tests := []struct {
		name  string
		check func() error
	}{
		{name: "goal", check: func() error { _, err := ParseGoal("migration"); return err }},
		{name: "collection path", check: func() error { _, err := ParseCollectionPath("full"); return err }},
		{name: "provider", check: func() error { _, err := ParseProvider("digitalocean"); return err }},
		{name: "action", check: func() error { _, err := ParseAction("destroy"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.check(); err == nil {
				t.Fatalf("expected invalid %s to return an error", tt.name)
			}
		})
	}
}

func TestIsProvider(t *testing.T) {
	if !IsProvider("gcp") {
		t.Fatal("IsProvider(\"gcp\") = false, want true")
	}
	if IsProvider("rapid-assessment") {
		t.Fatal("IsProvider(\"rapid-assessment\") = true, want false")
	}
}
