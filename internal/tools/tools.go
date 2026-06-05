package tools

import (
	"context"

	"aetox-cli/internal/contracts"
)

type Tool interface {
	Name() string
	Description() string
	Actions() []string
	Risk(action string, params map[string]any) contracts.RiskLevel
	Execute(ctx context.Context, action string, params map[string]any) (contracts.StepResult, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: map[string]Tool{},
	}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

