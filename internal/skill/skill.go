package skill

import (
	"github.com/Mike0165115321/Aetox/internal/model"
	"context"
	"fmt"
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

// Source identifies where a registered skill came from, so callers can gate
// trust or group skills in the UI instead of guessing from the name.
// See ARCHITECTURE.md §6.4.
type Source string

const (
	SourceBuiltin  Source = "builtin"
	SourceExternal Source = "external"
)

type registryEntry struct {
	skill  Skill
	source Source
}

type Registry struct {
	entries map[string]registryEntry
}

func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]registryEntry),
	}
}

// Register adds skill under source. It returns an error instead of silently
// overwriting when the name is already registered.
func (r *Registry) Register(skill Skill, source Source) error {
	if skill == nil || r == nil {
		return nil
	}
	name := skill.Name()
	if existing, ok := r.entries[name]; ok {
		return fmt.Errorf("skill %q already registered (source=%s), refusing to overwrite with source=%s", name, existing.source, source)
	}
	r.entries[name] = registryEntry{skill: skill, source: source}
	return nil
}

func (r *Registry) Get(name string) (Skill, bool) {
	if r == nil {
		return nil, false
	}
	entry, ok := r.entries[name]
	return entry.skill, ok
}

// SourceOf reports where the skill named name came from.
func (r *Registry) SourceOf(name string) (Source, bool) {
	if r == nil {
		return "", false
	}
	entry, ok := r.entries[name]
	return entry.source, ok
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	return names
}

func (r *Registry) Snapshot() map[string]Skill {
	if r == nil {
		return nil
	}
	snapshot := make(map[string]Skill, len(r.entries))
	for name, entry := range r.entries {
		snapshot[name] = entry.skill
	}
	return snapshot
}
