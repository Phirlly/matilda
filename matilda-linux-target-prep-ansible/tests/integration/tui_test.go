package integration

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/cli"
)

func TestTUIDashboardAndValidatedIPsView(t *testing.T) {
	root := withTempProject(t, validLinuxGroupedInventory(), validationSummary())
	writeFile(t, filepath.Join(root, "reports", "validated-discovery-ips.txt"), "10.0.0.10\n")

	var out bytes.Buffer
	err := cli.Execute([]string{"tui"}, strings.NewReader("4\n\nq\n"), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("tui failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Matilda Discovery Readiness", "Workflow", "Target Readiness", "Actions", "Activity Log", "Validated discovery IPs:", "10.0.0.10"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("tui output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(strings.ToLower(out.String()), "linux-groups") {
		t.Fatalf("tui should not expose internal inventory format labels:\n%s", out.String())
	}
}
