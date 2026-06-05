package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"aetox-cli/internal/config"
	"aetox-cli/internal/contracts"
	"aetox-cli/internal/critic"
	"aetox-cli/internal/executor"
	"aetox-cli/internal/memory"
	"aetox-cli/internal/planner"
	"aetox-cli/internal/safety"
)

type Report struct {
	PlanID      string
	Success     bool
	Error       string
	StepResults []contracts.StepResult
	LastHint    string
}

type Dispatcher struct {
	executor    *executor.Executor
	critic      *critic.Critic
	safety      *safety.Manager
	memory      *memory.SessionContext
	planner     *planner.Planner
	cfg         config.Config
	maxRetry    int
	maxPlanLoop int
}

func NewDispatcher(
	exec *executor.Executor,
	c *critic.Critic,
	s *safety.Manager,
	m *memory.SessionContext,
	p *planner.Planner,
	cfg config.Config,
	maxRetry int,
	maxPlanLoop int,
) *Dispatcher {
	return &Dispatcher{
		executor:    exec,
		critic:      c,
		safety:      s,
		memory:      m,
		planner:     p,
		cfg:         cfg,
		maxRetry:    maxRetry,
		maxPlanLoop: maxPlanLoop,
	}
}

func (d *Dispatcher) Run(ctx context.Context, initialPlan contracts.TaskPlan) (*Report, error) {
	report := &Report{
		PlanID:      initialPlan.ID,
		StepResults: []contracts.StepResult{},
		Success:     true,
	}

	currentPlan := initialPlan
	planLoop := 0

	for {
		if planLoop > d.maxPlanLoop {
			report.Success = false
			report.Error = "max plan retries reached"
			return report, errors.New(report.Error)
		}

		planLoop++
		planUpdated := false

		for stepIndex := 0; stepIndex < len(currentPlan.Steps); stepIndex++ {
			step := currentPlan.Steps[stepIndex]

			stepResult, verdict, err := d.runStepWithRetry(ctx, step)
			if stepResult.StepID == 0 {
				stepResult.StepID = step.ID
			}
			report.StepResults = append(report.StepResults, stepResult)

			if verdict.Verdict == contracts.CriticPass {
				d.memory.AddResult(stepResult)
				continue
			}

			if verdict.Verdict == contracts.CriticRetry {
				if err != nil {
					report.Error = err.Error()
				} else {
					report.Error = fmt.Sprintf("step %d requires retry but could not be passed", step.ID)
				}
				report.Success = false
				return report, err
			}

			// Critic escalates: build a new plan from feedback and keep running.
			hint := strings.TrimSpace(verdict.Suggestion)
			report.LastHint = hint

			nextPlan, planErr := d.planner.BuildPlanFromHint(ctx, currentPlan.Goal, d.cfg, hint, verdict, stepResult)
			if planErr != nil {
				report.Success = false
				report.Error = planErr.Error()
				return report, planErr
			}

			currentPlan = nextPlan
			report.PlanID = currentPlan.ID
			planUpdated = true
			break
		}

		if !planUpdated {
			return report, nil
		}
	}
}

func (d *Dispatcher) runStepWithRetry(ctx context.Context, step contracts.TaskStep) (contracts.StepResult, contracts.CriticVerdict, error) {
	approved, err := d.safety.Approve(ctx, step)
	if err != nil {
		return contracts.StepResult{
			StepID:  step.ID,
			Status:   contracts.StatusFailure,
			Error:    err.Error(),
			MemoryUpdates: map[string]any{"step": step.ID},
		}, contracts.CriticVerdict{
			Verdict:    contracts.CriticEscalate,
			Score:      0,
			Suggestion: "Approve step could not be completed",
		}, err
	}
	if !approved {
		return contracts.StepResult{
			StepID:        step.ID,
			Status:        contracts.StatusFailure,
			Error:         "user denied approval",
			MemoryUpdates: map[string]any{"step": step.ID},
		}, contracts.CriticVerdict{
			Verdict:    contracts.CriticEscalate,
			Score:      0,
			Suggestion: "User denied approval",
		}, errors.New("user denied approval")
	}

	hint := ""
	for attempt := 0; ; attempt++ {
		working := cloneParams(step.Params)
		if hint != "" {
			working["critic_hint"] = hint
		}
		step.Params = working

		result, execErr := d.executor.ExecuteStep(ctx, step)
		if execErr != nil && result.Error == "" {
			result.Error = execErr.Error()
		}
		if result.Status == "" {
			result.Status = contracts.StatusFailure
		}

		verdict := d.critic.Evaluate(step, result)
		result.MemoryUpdates = ensureMemoryMap(result.MemoryUpdates, map[string]any{
			"step":        step.ID,
			"attempt":     attempt + 1,
			"last_action": step.Action,
		})

		if verdict.Verdict == contracts.CriticPass {
			return result, verdict, nil
		}
		if verdict.Verdict != contracts.CriticRetry {
			return result, verdict, nil
		}

		hint = strings.TrimSpace(verdict.Suggestion)
		if hint == "" {
			hint = "Retry with corrected parameters"
		}
		if attempt >= d.maxRetry {
			return result, contracts.CriticVerdict{
				Verdict:    contracts.CriticEscalate,
				Score:      verdict.Score,
				Suggestion: fmt.Sprintf("max retries reached; %s", hint),
			}, errors.New("max retries reached")
		}
	}
}

func cloneParams(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func ensureMemoryMap(base map[string]any, fallback map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for key, value := range fallback {
		base[key] = value
	}
	return base
}
