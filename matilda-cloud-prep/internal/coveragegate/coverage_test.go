package coveragegate

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummarizeUsesStatementWeightedCoverage(t *testing.T) {
	summary, err := Summarize(strings.NewReader(strings.Join([]string{
		"mode: set",
		"heavy.go:1.1,10.2 99 0",
		"small.go:1.1,2.2 1 1",
		"",
	}, "\n")))
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}

	if summary.CoveredStatements != 1 {
		t.Fatalf("CoveredStatements = %d, want 1", summary.CoveredStatements)
	}
	if summary.TotalStatements != 100 {
		t.Fatalf("TotalStatements = %d, want 100", summary.TotalStatements)
	}
	if summary.Percent != 1.0 {
		t.Fatalf("Percent = %.2f, want 1.00", summary.Percent)
	}
}

func TestSummarizeMergesDuplicateBlocks(t *testing.T) {
	summary, err := Summarize(strings.NewReader(strings.Join([]string{
		"mode: set",
		"merged.go:1.1,10.2 10 0",
		"merged.go:1.1,10.2 10 1",
		"reversed.go:1.1,10.2 10 1",
		"reversed.go:1.1,10.2 10 0",
		"uncovered.go:1.1,10.2 90 0",
		"",
	}, "\n")))
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}

	if summary.CoveredStatements != 20 {
		t.Fatalf("CoveredStatements = %d, want 20", summary.CoveredStatements)
	}
	if summary.TotalStatements != 110 {
		t.Fatalf("TotalStatements = %d, want 110", summary.TotalStatements)
	}
	wantPercent := 100 * float64(20) / float64(110)
	if summary.Percent != wantPercent {
		t.Fatalf("Percent = %.2f, want %.2f", summary.Percent, wantPercent)
	}
}

func TestEvaluatePassesAtExactThreshold(t *testing.T) {
	summary, err := Evaluate(strings.NewReader(strings.Join([]string{
		"mode: set",
		"covered.go:1.1,10.2 88 1",
		"uncovered.go:1.1,10.2 12 0",
		"",
	}, "\n")), 88.0)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if summary.Percent != 88.0 {
		t.Fatalf("Percent = %.2f, want 88.00", summary.Percent)
	}
}

func TestEvaluateFailsBelowThreshold(t *testing.T) {
	summary, err := Evaluate(strings.NewReader(strings.Join([]string{
		"mode: set",
		"covered.go:1.1,10.2 87 1",
		"uncovered.go:1.1,10.2 13 0",
		"",
	}, "\n")), 88.0)
	if err == nil {
		t.Fatal("Evaluate returned nil error, want below-minimum error")
	}

	var below BelowMinimumError
	if !errors.As(err, &below) {
		t.Fatalf("Evaluate error = %T %v, want BelowMinimumError", err, err)
	}
	if !strings.Contains(err.Error(), "coverage 87.00% is below required 88.00%") {
		t.Fatalf("Evaluate error = %q, want user-facing coverage threshold detail", err)
	}
	if summary.Percent != 87.0 {
		t.Fatalf("Percent = %.2f, want 87.00", summary.Percent)
	}
	if below.Summary.Percent != 87.0 {
		t.Fatalf("BelowMinimumError summary percent = %.2f, want 87.00", below.Summary.Percent)
	}
}

func TestSummarizeRejectsInvalidProfiles(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "empty"},
		{name: "bad mode line", in: "coverage: set\n", want: "mode"},
		{name: "unsupported mode", in: "mode: histogram\nx.go:1.1,2.2 1 1\n", want: "unsupported"},
		{name: "no statements", in: "mode: set\n", want: "no statements"},
		{name: "malformed block", in: "mode: set\nx.go 1 1\n", want: "line 2"},
		{name: "blank filename", in: "mode: set\n:1.1,2.2 1 1\n", want: "filename"},
		{name: "negative count", in: "mode: set\nx.go:1.1,2.2 1 -1\n", want: "negative"},
		{name: "negative statements", in: "mode: set\nx.go:1.1,2.2 -1 1\n", want: "negative"},
		{name: "invalid statements", in: "mode: set\nx.go:1.1,2.2 nope 1\n", want: "numberofstatements"},
		{name: "invalid end column", in: "mode: set\nx.go:1.1,2.nope 1 1\n", want: "end column"},
		{name: "invalid start column", in: "mode: set\nx.go:1.nope,2.2 1 1\n", want: "start column"},
		{name: "invalid start line", in: "mode: set\nx.go:nope.1,2.2 1 1\n", want: "start line"},
		{name: "inconsistent duplicate statements", in: "mode: set\nx.go:1.1,2.2 1 1\nx.go:1.1,2.2 2 1\n", want: "inconsistent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Summarize(strings.NewReader(tt.in))
			if err == nil {
				t.Fatal("Summarize returned nil error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("Summarize error = %q, want to contain %q", err, tt.want)
			}
		})
	}
}

func TestEvaluateRejectsInvalidThresholds(t *testing.T) {
	tests := []struct {
		name    string
		minimum float64
	}{
		{name: "negative", minimum: -0.1},
		{name: "over one hundred", minimum: 100.1},
		{name: "not a number", minimum: math.NaN()},
		{name: "infinite", minimum: math.Inf(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Evaluate(strings.NewReader("mode: set\nx.go:1.1,2.2 1 1\n"), tt.minimum)
			if err == nil {
				t.Fatalf("Evaluate with minimum %.2f returned nil error", tt.minimum)
			}
			if !strings.Contains(strings.ToLower(err.Error()), "minimum") {
				t.Fatalf("Evaluate error = %q, want invalid minimum", err)
			}
		})
	}
}

func TestEvaluateFileRejectsMissingProfile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "coverage.out")

	_, err := EvaluateFile(missing, 88.0)
	if err == nil {
		t.Fatal("EvaluateFile returned nil error for missing profile")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "coverage profile") {
		t.Fatalf("EvaluateFile error = %q, want coverage profile context", err)
	}
}

func TestEvaluateFileRejectsMalformedProfile(t *testing.T) {
	profile := writeCoverageProfile(t, "mode: set\nmalformed\n")

	_, err := EvaluateFile(profile, 88.0)
	if err == nil {
		t.Fatal("EvaluateFile returned nil error for malformed profile")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "line 2") {
		t.Fatalf("EvaluateFile error = %q, want malformed line context", err)
	}
}

func TestRunReportsGateResults(t *testing.T) {
	profile := writeCoverageProfile(t, strings.Join([]string{
		"mode: set",
		"covered.go:1.1,10.2 88 1",
		"uncovered.go:1.1,10.2 12 0",
		"",
	}, "\n"))

	var stdout strings.Builder
	var stderr strings.Builder
	code := Run([]string{"-profile", profile, "-min", "88.0"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("Run exit code = %d, want %d; stderr: %s", code, ExitOK, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, want := range []string{"coverage 88.00%", "meets required 88.00%", "88/100 statements"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want to contain %q", stdout.String(), want)
		}
	}
}

func TestRunReportsBelowMinimum(t *testing.T) {
	profile := writeCoverageProfile(t, strings.Join([]string{
		"mode: set",
		"covered.go:1.1,10.2 87 1",
		"uncovered.go:1.1,10.2 13 0",
		"",
	}, "\n"))

	var stdout strings.Builder
	var stderr strings.Builder
	code := Run([]string{"-profile", profile, "-min", "88.0"}, &stdout, &stderr)

	if code != ExitGateFailed {
		t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"coverage 87.00%", "below required 88.00%", "87/100 statements"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want to contain %q", stderr.String(), want)
		}
	}
}

func TestRunReportsProfileErrorsAsGateFailures(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    string
	}{
		{name: "missing profile", profile: filepath.Join(t.TempDir(), "missing.out"), want: "read coverage profile"},
		{name: "malformed profile", profile: writeCoverageProfile(t, "mode: set\nmalformed\n"), want: "line 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout strings.Builder
			var stderr strings.Builder
			code := Run([]string{"-profile", tt.profile, "-min", "88.0"}, &stdout, &stderr)

			if code != ExitGateFailed {
				t.Fatalf("Run exit code = %d, want %d", code, ExitGateFailed)
			}
			if stdout.String() != "" {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(strings.ToLower(stderr.String()), tt.want) {
				t.Fatalf("stderr = %q, want to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing profile flag", args: []string{"-min", "88.0"}, want: "profile"},
		{name: "invalid min flag", args: []string{"-profile", "coverage.out", "-min", "not-a-number"}, want: "invalid"},
		{name: "minimum out of range", args: []string{"-profile", "coverage.out", "-min", "101"}, want: "minimum"},
		{name: "unexpected positional arg", args: []string{"-profile", "coverage.out", "extra"}, want: "unexpected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout strings.Builder
			var stderr strings.Builder
			code := Run(tt.args, &stdout, &stderr)

			if code != ExitUsage {
				t.Fatalf("Run exit code = %d, want %d", code, ExitUsage)
			}
			if stdout.String() != "" {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(strings.ToLower(stderr.String()), tt.want) {
				t.Fatalf("stderr = %q, want to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

func writeCoverageProfile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", path, err)
	}
	return path
}
