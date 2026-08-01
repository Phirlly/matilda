package cli

import (
	"fmt"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/assessment"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func parseDirectCommand(args []string) (workflow.Request, workflow.ExecutionOptions, bool, error) {
	helpRequested := hasTrailingHelp(args)
	if helpRequested {
		args = args[:len(args)-1]
	}

	positionals, flagArgs, err := splitDirectCommand(args)
	if err != nil {
		return workflow.Request{}, workflow.ExecutionOptions{}, false, err
	}
	request, err := parseRequest(positionals)
	if err != nil {
		return workflow.Request{}, workflow.ExecutionOptions{}, false, err
	}
	options, err := parseExecutionOptions(request, flagArgs)
	if err != nil {
		return workflow.Request{}, workflow.ExecutionOptions{}, false, err
	}
	return request, options, helpRequested, nil
}

func splitDirectCommand(args []string) ([]string, []string, error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("usage: expected matilda-prep rapid-assessment or deep-discovery command")
	}

	var positionalCount int
	switch args[0] {
	case string(assessment.RapidAssessment):
		positionalCount = 4
	case string(assessment.DeepDiscovery):
		positionalCount = 3
	default:
		return args, nil, nil
	}
	if len(args) < positionalCount {
		return args, nil, nil
	}
	return args[:positionalCount], args[positionalCount:], nil
}

func parseRequest(args []string) (workflow.Request, error) {
	goal, err := assessment.ParseGoal(args[0])
	if err != nil {
		return workflow.Request{}, err
	}

	switch goal {
	case assessment.RapidAssessment:
		return parseRapidAssessment(args)
	case assessment.DeepDiscovery:
		return parseDeepDiscovery(args)
	default:
		return workflow.Request{}, fmt.Errorf("invalid goal %q: expected rapid-assessment or deep-discovery", args[0])
	}
}

func parseRapidAssessment(args []string) (workflow.Request, error) {
	if len(args) != 4 {
		return workflow.Request{}, fmt.Errorf("usage: matilda-prep rapid-assessment <billing|api> <provider> <action>")
	}

	collectionPath, err := assessment.ParseCollectionPath(args[1])
	if err != nil {
		return workflow.Request{}, err
	}
	provider, err := assessment.ParseProvider(args[2])
	if err != nil {
		return workflow.Request{}, err
	}
	action, err := assessment.ParseAction(args[3])
	if err != nil {
		return workflow.Request{}, err
	}

	return workflow.Request{
		Goal:           assessment.RapidAssessment,
		CollectionPath: collectionPath,
		Provider:       provider,
		Action:         action,
	}, nil
}

func parseDeepDiscovery(args []string) (workflow.Request, error) {
	if len(args) != 3 {
		return workflow.Request{}, fmt.Errorf("usage: matilda-prep deep-discovery <provider> <action>")
	}

	provider, err := assessment.ParseProvider(args[1])
	if err != nil {
		return workflow.Request{}, err
	}
	action, err := assessment.ParseAction(args[2])
	if err != nil {
		return workflow.Request{}, err
	}

	return workflow.Request{
		Goal:     assessment.DeepDiscovery,
		Provider: provider,
		Action:   action,
	}, nil
}
