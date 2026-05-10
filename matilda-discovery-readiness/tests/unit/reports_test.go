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

func TestGenerateReportsNormalizesKnownFailureCodes(t *testing.T) {
	dir := t.TempDir()
	summary := strings.Join([]string{
		"Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation,FailureCode",
		"app01,10.0.0.10,ifconfig,NO,FAIL,NOT_RUN,NOT_RUN,NO,sudo: a password is required,SUDO_PASSWORD_REQUIRED",
		"app02,10.0.0.20,ifconfig,NO,OK,OK,FAIL,NO,This account is currently not available.,SERVICE_ACCOUNT_LOCKED",
		"app03,10.0.0.30,ifconfig,NO,OK,FAIL,NOT_RUN,NO,unapproved sudo command was not denied,DENIED_COMMAND_ALLOWED",
		"app04,10.0.0.40,ip,NO,FAIL,NOT_RUN,NOT_RUN,NO,neither ifconfig nor ip is available,VALIDATION_COMMAND_MISSING",
		"app05,10.0.0.50,ifconfig,NO,OK,OK,FAIL,NO,probe command failed,PROBE_VALIDATION_FAILED",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "validation-summary.txt"), []byte(summary), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := reports.Generate(dir); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	csvReport, err := os.ReadFile(filepath.Join(dir, "readiness.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(csvReport), "FailureCode") {
		t.Fatalf("exported CSV should not expose internal failure codes:\n%s", string(csvReport))
	}

	jsonReport, err := os.ReadFile(filepath.Join(dir, "readiness.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Passwordless sudo is not available",
		"Observed failure: sudo: a password is required",
		"service account is locked",
		"sudoers allow-list allowed an unapproved command",
		"Neither ifconfig nor ip is available",
		"Probe-to-target validation failed",
	} {
		if !strings.Contains(string(jsonReport), want) {
			t.Fatalf("JSON report missing normalized remediation %q:\n%s", want, string(jsonReport))
		}
	}
	if strings.Contains(string(jsonReport), "failure_code") {
		t.Fatalf("JSON report should not expose internal failure codes:\n%s", string(jsonReport))
	}
}

func TestGenerateReportsNormalizesLegacyRawFailures(t *testing.T) {
	dir := t.TempDir()
	summary := "Host,DiscoveryIP,Command,FallbackUsed,LocalSudo,DeniedCommand,ProbeSSH,Ready,Remediation\napp01,10.0.0.10,ifconfig,NO,OK,OK,FAIL,NO,matilda-svc@10.0.0.10: Permission denied (publickey).\napp02,10.0.0.20,ifconfig,NO,FAIL,NOT_RUN,NOT_RUN,NO,sudo: unknown user: matilda-svc\n"
	if err := os.WriteFile(filepath.Join(dir, "validation-summary.txt"), []byte(summary), 0600); err != nil {
		t.Fatal(err)
	}

	rows, err := reports.Rows(dir)
	if err != nil {
		t.Fatalf("Rows failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	for _, want := range []string{
		"SSH public key authentication failed",
		"service account is missing",
	} {
		found := false
		for _, row := range rows {
			if strings.Contains(row.Remediation, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("rows missing normalized remediation %q: %+v", want, rows)
		}
	}
}
