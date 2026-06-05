package executor

import (
	"context"
	"errors"
	"fmt"

	"aetox-cli/internal/contracts"
	"aetox-cli/internal/tools"
)

type Executor struct {
	registry *tools.Registry
}

func NewExecutor(registry *tools.Registry) *Executor {
	return &Executor{registry: registry}
}

func (e *Executor) ExecuteStep(ctx context.Context, step contracts.TaskStep) (contracts.StepResult, error) {
	if step.Tool == "" {
		return contracts.StepResult{
			Status: contracts.StatusFailure,
			Error:  "missing tool name",
		}, errors.New("missing tool name")
	}

	tool, ok := e.registry.Get(step.Tool)
	if !ok {
		return contracts.StepResult{
			Status: contracts.StatusFailure,
			Error:  fmt.Sprintf("tool not found: %s", step.Tool),
		}, fmt.Errorf("tool not found: %s", step.Tool)
	}

	stepResult, err := tool.Execute(ctx, step.Action, step.Params)
	if stepResult.StepID == 0 {
		stepResult.StepID = step.ID
	}
	return stepResult, err
}

