package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matilda-discovery-readiness/internal/reports"
)

func TestGenerateReports(t *testing.T) {
	dir := t.TempDir()
	summary := "Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation\napp01,10.0.0.10,ifconfig,NO,OK,OK,OK,YES,None\napp02,10.10.0.20,ifconfig,NO,FAIL,NOT_RUN,NOT_RUN,NO,Fix sudo\n"
	if err := os.WriteFile(filepath.Join(dir, "validation-summary.txt"), []byte(summary), 0600); err != nil {
		t.Fatal(err)
	}

	paths, err := reports.Generate(dir)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(paths) != 4 {
		t.Fatalf("expected 4 report paths, got %d", len(paths))
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected report %s: %v", path, err)
		}
	}

	htmlReport, err := os.ReadFile(filepath.Join(dir, "readiness.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlReport), "Matilda Discovery Readiness Report") {
		t.Fatalf("html report missing title:\n%s", string(htmlReport))
	}

	summaryCounts, err := reports.Summarize(dir)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summaryCounts.Total != 2 || summaryCounts.Ready != 1 || summaryCounts.NotReady != 1 {
		t.Fatalf("unexpected summary counts: %+v", summaryCounts)
	}
}
