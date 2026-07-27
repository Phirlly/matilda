package guided

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
)

var (
	ErrInvalidSelection = errors.New("invalid selection")
	ErrInputCancelled   = errors.New("input cancelled")
)

type outcomeOption struct {
	label          string
	description    string
	goal           assessment.Goal
	collectionPath assessment.CollectionPath
}

type cloudOption struct {
	label       string
	description string
	provider    assessment.Provider
}

func Run(stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewScanner(stdin)

	writeIntro(stdout)
	writeOutcomeChoices(stdout)
	outcome, err := selectOutcome(reader, stdout)
	if err != nil {
		return err
	}

	writeCloudChoices(stdout)
	cloud, err := selectCloud(reader, stdout)
	if err != nil {
		return err
	}

	writeNextSteps(stdout, outcome, cloud)
	return nil
}

func writeIntro(stdout io.Writer) {
	fmt.Fprintln(stdout, "Matilda Cloud Prep")
	fmt.Fprintln(stdout, "Cloud-side preparation for Matilda SaaS.")
	fmt.Fprintln(stdout, "This tool does not configure the Matilda SaaS portal.")
	fmt.Fprintln(stdout)
}

func writeOutcomeChoices(stdout io.Writer) {
	fmt.Fprintln(stdout, "What do you want to prepare?")
	for index, option := range outcomeOptions() {
		fmt.Fprintf(stdout, "  %d. %s\n", index+1, option.label)
		fmt.Fprintf(stdout, "     %s\n", option.description)
	}
}

func writeCloudChoices(stdout io.Writer) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Which cloud are you using?")
	for index, option := range cloudOptions() {
		fmt.Fprintf(stdout, "  %d. %s\n", index+1, option.label)
		fmt.Fprintf(stdout, "     %s\n", option.description)
	}
}

func selectOutcome(reader *bufio.Scanner, stdout io.Writer) (outcomeOption, error) {
	options := outcomeOptions()
	index, err := readChoice(reader, stdout, "Select outcome [1-3]: ", "Matilda outcome", len(options))
	if err != nil {
		return outcomeOption{}, err
	}
	return options[index], nil
}

func selectCloud(reader *bufio.Scanner, stdout io.Writer) (cloudOption, error) {
	options := cloudOptions()
	index, err := readChoice(reader, stdout, "Select cloud [1-4]: ", "cloud", len(options))
	if err != nil {
		return cloudOption{}, err
	}
	return options[index], nil
}

func readChoice(reader *bufio.Scanner, stdout io.Writer, prompt string, name string, count int) (int, error) {
	fmt.Fprint(stdout, prompt)
	if !reader.Scan() {
		if err := reader.Err(); err != nil {
			return 0, fmt.Errorf("%w: guided setup cancelled while reading %s selection: %v", ErrInputCancelled, name, err)
		}
		return 0, fmt.Errorf("%w: guided setup cancelled before %s selection", ErrInputCancelled, name)
	}

	value := strings.TrimSpace(reader.Text())
	switch strings.ToLower(value) {
	case "q", "quit", "cancel":
		return 0, fmt.Errorf("%w: guided setup cancelled by user", ErrInputCancelled)
	}

	choice, err := strconv.Atoi(value)
	if err != nil || choice < 1 || choice > count {
		return 0, fmt.Errorf("%w %q: expected 1-%d for %s", ErrInvalidSelection, value, count, name)
	}
	return choice - 1, nil
}

func writeNextSteps(stdout io.Writer, outcome outcomeOption, cloud cloudOption) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Ready to begin with read-only preflight.")
	fmt.Fprintf(stdout, "Selected outcome: %s\n", outcome.label)
	fmt.Fprintf(stdout, "Selected cloud: %s\n", cloud.label)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  %s\n", commandFor(outcome, cloud))

	if outcome.collectionPath == assessment.CollectionBilling {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Rapid Assessment - Billing Based may use Matilda SaaS Skip Configuration for the cloud account.")
		fmt.Fprintln(stdout, "Skip Configuration does not skip cloud-side billing export/report preparation.")
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Current scaffold safety:")
	fmt.Fprintln(stdout, "  Implemented provider paths run verified read-only checks.")
	fmt.Fprintln(stdout, "  Unimplemented paths remain fail-closed and non-mutating.")
	fmt.Fprintln(stdout, "  No cloud resources or Matilda SaaS portal settings were changed.")
}

func commandFor(outcome outcomeOption, cloud cloudOption) string {
	if outcome.goal == assessment.DeepDiscovery {
		return fmt.Sprintf("matilda-prep %s %s %s", outcome.goal, cloud.provider, assessment.ActionPreflight)
	}
	return fmt.Sprintf(
		"matilda-prep %s %s %s %s",
		outcome.goal,
		outcome.collectionPath,
		cloud.provider,
		assessment.ActionPreflight,
	)
}

func outcomeOptions() []outcomeOption {
	return []outcomeOption{
		{
			label:          "Rapid Assessment - Billing Based",
			description:    "Prepare exported billing data for Rapid Assessment.",
			goal:           assessment.RapidAssessment,
			collectionPath: assessment.CollectionBilling,
		},
		{
			label:          "Rapid Assessment - API Based",
			description:    "Prepare cloud API access for Matilda discovery jobs before Rapid Assessment.",
			goal:           assessment.RapidAssessment,
			collectionPath: assessment.CollectionAPI,
		},
		{
			label:       "Deep Discovery",
			description: "Prepare verified provider-specific deeper discovery prerequisites.",
			goal:        assessment.DeepDiscovery,
		},
	}
}

func cloudOptions() []cloudOption {
	return []cloudOption{
		{
			label:       "AWS",
			description: "Use the AWS preparation path.",
			provider:    assessment.ProviderAWS,
		},
		{
			label:       "Azure",
			description: "Use the Azure preparation path.",
			provider:    assessment.ProviderAzure,
		},
		{
			label:       "GCP",
			description: "Use the GCP preparation path.",
			provider:    assessment.ProviderGCP,
		},
		{
			label:       "OCI",
			description: "Use the OCI preparation path.",
			provider:    assessment.ProviderOCI,
		},
	}
}
