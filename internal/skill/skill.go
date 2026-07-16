package skill

import (
	"github.com/Mike0165115321/Aetox/internal/model"
	"context"
)

type Input map[string]any

type Output struct {
	Name       string
	Content    string
	Data       any
	Command    string
	RawOutput  string
	Stderr     string
	Success    bool
	Truncated  bool
	DurationMs int64
}

type Skill interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input Input) (Output, error)
}

type Tool interface {
	Skill
	ToolDefinition() model.ToolDefinition
	ExecuteTool(ctx context.Context, args map[string]any) (Output, error)
}

type Registry struct {
	skills map[string]Skill
}

func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]Skill),
	}
}

func (r *Registry) Register(skill Skill) {
	if skill == nil || r == nil {
		return
	}
	r.skills[skill.Name()] = skill
}

func (r *Registry) Get(name string) (Skill, bool) {
	if r == nil {
		return nil, false
	}
	skill, ok := r.skills[name]
	return skill, ok
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}

func (r *Registry) Snapshot() map[string]Skill {
	if r == nil {
		return nil
	}
	snapshot := make(map[string]Skill, len(r.skills))
	for name, value := range r.skills {
		snapshot[name] = value
	}
	return snapshot
}
