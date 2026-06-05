package critic

import (
	"fmt"
	"strings"

	"aetox-cli/internal/contracts"
)

type Critic struct{}

func NewCritic() *Critic {
	return &Critic{}
}

func (c *Critic) Evaluate(step contracts.TaskStep, stepResult contracts.StepResult) contracts.CriticVerdict {
	if stepResult.Status == contracts.StatusSuccess {
		if strings.TrimSpace(stepResult.Output) == "" &&
			step.Action != "delete" &&
			step.Action != "move" {
			return contracts.CriticVerdict{
				Verdict:    contracts.CriticRetry,
				Score:      0.6,
				Issues:     []string{"empty output"},
				Suggestion: fmt.Sprintf("Check %s output and confirm action produced expected result", step.Action),
			}
		}
		return contracts.CriticVerdict{
			Verdict: contracts.CriticPass,
			Score:   1.0,
		}
	}

	issues := []string{}
	suggestion := "Retry with corrected inputs."
	score := 0.2

	if stepResult.Error != "" {
		issues = append(issues, stepResult.Error)
		switch step.Action {
		case "read", "write", "move", "delete":
			suggestion = "Verify path and retry with explicit quoted path in sandbox"
		case "fetch":
			suggestion = "Check URL format/network and retry with a reachable HTTPS URL"
			score = 0.3
		case "run":
			suggestion = "Fix shell command syntax or quoting and retry"
			score = 0.3
		default:
			suggestion = "Retry with corrected step inputs"
		}
	}

	outputLower := strings.ToLower(stepResult.Output)
	if strings.Contains(outputLower, "permission denied") || strings.Contains(outputLower, "access denied") {
		suggestion = "Grant access or choose a safe alternative then retry"
		score = 0.1
		issues = append(issues, "permission issue")
	}

	return contracts.CriticVerdict{
		Verdict:    contracts.CriticRetry,
		Score:      score,
		Issues:     issues,
		Suggestion: suggestion,
	}
}

