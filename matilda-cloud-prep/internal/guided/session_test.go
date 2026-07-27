package guided

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRunRapidBillingGCPShowsObjectiveFirstPreflight(t *testing.T) {
	output, err := runGuided("1\n3\n")

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, want := range []string{
		"Rapid Assessment - Billing Based",
		"GCP",
		"matilda-prep rapid-assessment billing gcp preflight",
		"Skip Configuration",
		"does not skip cloud-side billing export/report preparation",
		"Implemented provider paths run verified read-only checks",
		"fail-closed",
		"non-mutating",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want to contain %q", output, want)
		}
	}
}

func TestRunRapidAPIAWSShowsObjectiveFirstPreflight(t *testing.T) {
	output, err := runGuided("2\n1\n")

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(output, "Rapid Assessment - API Based") {
		t.Fatalf("output = %q, want Rapid Assessment - API Based", output)
	}
	if !strings.Contains(output, "matilda-prep rapid-assessment api aws preflight") {
		t.Fatalf("output = %q, want AWS API preflight command", output)
	}
}

func TestRunDeepDiscoveryOCIShowsObjectiveFirstPreflight(t *testing.T) {
	output, err := runGuided("3\n4\n")

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(output, "Deep Discovery") {
		t.Fatalf("output = %q, want Deep Discovery", output)
	}
	if !strings.Contains(output, "matilda-prep deep-discovery oci preflight") {
		t.Fatalf("output = %q, want OCI Deep Discovery preflight command", output)
	}
	if strings.Contains(output, "collection_path") {
		t.Fatalf("Deep Discovery guided output leaked Rapid-only collection path: %s", output)
	}
}

func TestRunListsOnlyMatildaOutcomesAndCloudChoices(t *testing.T) {
	output, err := runGuided("1\n3\n")

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	wantChoices := []string{
		"1. Rapid Assessment - Billing Based",
		"2. Rapid Assessment - API Based",
		"3. Deep Discovery",
		"1. AWS",
		"2. Azure",
		"3. GCP",
		"4. OCI",
	}
	if got := displayedChoices(output); !reflect.DeepEqual(got, wantChoices) {
		t.Fatalf("displayed choices = %#v, want exactly %#v\noutput:\n%s", got, wantChoices, output)
	}
	for _, invented := range []string{"billing-only", "api-discovery", "resources-only", "full"} {
		if strings.Contains(output, invented) {
			t.Fatalf("output contains invented mode %q: %s", invented, output)
		}
	}
}

func TestRunDoesNotAskCloudHierarchyBeforeDiscovery(t *testing.T) {
	output, err := runGuided("1\n3\n")

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	lower := strings.ToLower(output)
	for _, forbidden := range []string{
		"payer account",
		"management group",
		"subscription",
		"project",
		"folder",
		"compartment",
		"bucket",
		"dataset",
		"report prefix",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("guided output asks for provider hierarchy term %q before discovery: %s", forbidden, output)
		}
	}
}

func TestRunInvalidOutcomeSelectionReturnsUsageError(t *testing.T) {
	output, err := runGuided("9\n")

	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("Run error = %v, want ErrInvalidSelection", err)
	}
	if !strings.Contains(err.Error(), "expected 1-3") {
		t.Fatalf("error = %q, want outcome range", err.Error())
	}
	if !strings.Contains(output, "What do you want to prepare?") {
		t.Fatalf("output = %q, want outcome prompt before error", output)
	}
}

func TestRunInvalidCloudSelectionReturnsUsageError(t *testing.T) {
	output, err := runGuided("1\n9\n")

	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("Run error = %v, want ErrInvalidSelection", err)
	}
	if !strings.Contains(err.Error(), "expected 1-4") {
		t.Fatalf("error = %q, want cloud range", err.Error())
	}
	if !strings.Contains(output, "Which cloud are you using?") {
		t.Fatalf("output = %q, want cloud prompt before error", output)
	}
	if strings.Contains(output, "matilda-prep rapid-assessment billing") {
		t.Fatalf("output printed command after invalid cloud selection: %s", output)
	}
}

func TestRunUserCancelIsDistinctFromProviderFailure(t *testing.T) {
	output, err := runGuided("cancel\n")

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("Run error = %v, want ErrInputCancelled", err)
	}
	if !strings.Contains(err.Error(), "guided setup cancelled by user") {
		t.Fatalf("error = %q, want user cancellation message", err.Error())
	}
	if strings.Contains(strings.ToLower(err.Error()), "provider") {
		t.Fatalf("error = %q, want cancellation distinct from provider failure", err.Error())
	}
	if !strings.Contains(output, "What do you want to prepare?") {
		t.Fatalf("output = %q, want outcome prompt before cancellation", output)
	}
}

func TestRunInputEOFIsCancelledNotProviderFailure(t *testing.T) {
	output, err := runGuided("")

	if !errors.Is(err, ErrInputCancelled) {
		t.Fatalf("Run error = %v, want ErrInputCancelled", err)
	}
	if !strings.Contains(err.Error(), "guided setup cancelled") {
		t.Fatalf("error = %q, want cancellation message", err.Error())
	}
	if strings.Contains(strings.ToLower(err.Error()), "provider") {
		t.Fatalf("error = %q, want cancellation distinct from provider failure", err.Error())
	}
	if !strings.Contains(output, "Matilda Cloud Prep") {
		t.Fatalf("output = %q, want initial prompt before cancellation", output)
	}
}

func runGuided(input string) (string, error) {
	var output bytes.Buffer
	err := Run(strings.NewReader(input), &output)
	return output.String(), err
}

func displayedChoices(output string) []string {
	var choices []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 4 || trimmed[1] != '.' || trimmed[2] != ' ' {
			continue
		}
		if trimmed[0] < '1' || trimmed[0] > '9' {
			continue
		}
		choices = append(choices, trimmed)
	}
	return choices
}
