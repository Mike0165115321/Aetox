package dispatcher

import (
	"context"
	"errors"

	"aetox-cli/internal/contracts"
	"aetox-cli/internal/critic"
	"aetox-cli/internal/executor"
	"aetox-cli/internal/memory"
	"aetox-cli/internal/safety"
)

type Report struct {
	PlanID      string
	Success     bool
	Error       string
	StepResults []contracts.StepResult
}

type Dispatcher struct {
	executor *executor.Executor
	critic   *critic.Critic
	safety   *safety.Manager
	memory   *memory.SessionContext
	maxRetry int
}

func NewDispatcher(exec *executor.Executor, c *critic.Critic, s *safety.Manager, m *memory.SessionContext, maxRetry int) *Dispatcher {
	return &Dispatcher{
		executor: exec,
		critic:   c,
		safety:   s,
		memory:   m,
		maxRetry: maxRetry,
	}
}

func (d *Dispatcher) Run(ctx context.Context, plan contracts.TaskPlan) (*Report, error) {
	report := &Report{
		PlanID:      plan.ID,
		StepResults: []contracts.StepResult{},
		Success:     true,
	}

	for _, step := range plan.Steps {
		retries := 0
		for {
			approved, err := d.safety.Approve(ctx, step)
			if err != nil {
				report.Success = false
				report.Error = err.Error()
				return report, err
			}
			if !approved {
				report.Success = false
				report.Error = "user denied approval"
				return report, errors.New("user denied approval")
			}

			result, err := d.executor.ExecuteStep(ctx, step)
			if err != nil && result.Error == "" {
				result.Error = err.Error()
			}
			if result.Status == "" {
				result.Status = contracts.StatusFailure
			}
			verdict := d.critic.Evaluate(step, result)

			switch verdict.Verdict {
			case contracts.CriticPass:
				report.StepResults = append(report.StepResults, result)
				d.memory.AddResult(result)
				break
			case contracts.CriticRetry:
				if retries >= d.maxRetry {
					report.Success = false
					report.Error = "max retries reached"
					report.StepResults = append(report.StepResults, result)
					return report, errors.New("max retries reached")
				}
				retries++
				continue
			default:
				report.Success = false
				report.Error = "escalated by critic"
				report.StepResults = append(report.StepResults, result)
				return report, errors.New("critic escalated")
			}
			break
		}
	}
	return report, nil
}
