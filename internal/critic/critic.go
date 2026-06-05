package critic

import (
	"strings"

	"aetox-cli/internal/contracts"
)

type Critic struct{}

func NewCritic() *Critic {
	return &Critic{}
}

func (c *Critic) Evaluate(_ contracts.TaskStep, stepResult contracts.StepResult) contracts.CriticVerdict {
	if stepResult.Status == contracts.StatusSuccess {
		return contracts.CriticVerdict{
			Verdict: contracts.CriticPass,
			Score:   1.0,
		}
	}

	if stepResult.Error != "" {
		return contracts.CriticVerdict{
			Verdict:    contracts.CriticRetry,
			Score:      0.2,
			Issues:     []string{stepResult.Error},
			Suggestion: "Retry the step with a stable path and retry limit",
		}
	}

	if strings.TrimSpace(stepResult.Output) == "" {
		return contracts.CriticVerdict{
			Verdict:    contracts.CriticRetry,
			Score:      0.6,
			Issues:     []string{"empty output"},
			Suggestion: "Retry once; if still empty, escalate if intentional",
		}
	}

	return contracts.CriticVerdict{
		Verdict:    contracts.CriticRetry,
		Score:      0.4,
		Suggestion: "Unknown issue, try again",
	}
}
