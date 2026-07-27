package cur2preflight

import "github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"

type checkFinding struct {
	Status       workflow.CheckStatus
	Code         string
	Title        string
	Message      string
	TopLevel     bool
	ManualAction bool
	Evidence     []workflow.PlanEvidence
}

func passFinding(code string, title string, message string) checkFinding {
	return checkFinding{
		Status:  workflow.CheckPass,
		Code:    code,
		Title:   title,
		Message: message,
		Evidence: []workflow.PlanEvidence{
			{Key: "code", Value: code},
		},
	}
}

func warnFinding(code string, title string, message string, topLevel bool) checkFinding {
	return checkFinding{
		Status:   workflow.CheckWarn,
		Code:     code,
		Title:    title,
		Message:  message,
		TopLevel: topLevel,
		Evidence: []workflow.PlanEvidence{
			{Key: "code", Value: code},
		},
	}
}

func manualFinding(code string, title string, message string) checkFinding {
	finding := warnFinding(code, title, message, true)
	finding.ManualAction = true
	return finding
}

func failFinding(code string, title string, message string) checkFinding {
	return checkFinding{
		Status:   workflow.CheckFail,
		Code:     code,
		Title:    title,
		Message:  message,
		TopLevel: true,
		Evidence: []workflow.PlanEvidence{
			{Key: "code", Value: code},
		},
	}
}

func withEvidence(finding checkFinding, evidence ...workflow.PlanEvidence) checkFinding {
	finding.Evidence = append(finding.Evidence, evidence...)
	return finding
}
