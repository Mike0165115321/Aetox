package skill

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Mike0165115321/Aetox/internal/model"
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
	// LinesAdded/LinesRemoved describe what a write did to a file, for the
	// timeline's "+9 -0". Both zero on tools that touch no file.
	LinesAdded   int
	LinesRemoved int
}

// LineDelta counts how a replacement changed a file, for Output's stats.
// Exact-replacement edits make this the true count; a whole-file write reports
// the old file against the new one.
func LineDelta(before, after string) (added, removed int) {
	countLines := func(s string) int {
		if s == "" {
			return 0
		}
		return strings.Count(s, "\n") + 1
	}
	return countLines(after), countLines(before)
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
	// The first three are tools: things the AI runs to get work done. The last
	// is not — a skill is a document telling it how to do something, and
	// lumping the two under one word ("external") is what let six of Aetox's
	// own desktop tools show up in the UI as things the user had installed.
	SourceBuiltin   Source = "builtin"   // tools compiled into the engine
	SourceWorkbench Source = "workbench" // tools only the desktop app can offer
	SourceMCP       Source = "mcp"       // tools bridged from an MCP server
	SourceSkill     Source = "skill"     // a SKILL.md the user added — instructions, not a tool
)

type registryEntry struct {
	skill  Skill
	source Source
}

// Registry is safe for concurrent use: MCP tools are registered from a
// background goroutine (see desktop applyConfig) while turns read the registry
// live through the dispatcher, so every access is guarded by mu.
type Registry struct {
	mu      sync.RWMutex
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
	r.mu.Lock()
	defer r.mu.Unlock()
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[name]
	return entry.skill, ok
}

// SourceOf reports where the skill named name came from.
func (r *Registry) SourceOf(name string) (Source, bool) {
	if r == nil {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[name]
	return entry.source, ok
}

// Names returns every registered skill name, sorted.
//
// Sorted, not map order, because the tool definitions built from this list are
// serialized into the head of every request — before the conversation itself.
// Go randomizes map iteration, so an unsorted list reshuffled the tool block on
// every turn (measured: 10 calls, 6 distinct payloads), which changed the
// prompt prefix and missed the provider's prefix cache every single time. On a
// local Ollama that cost ~2s of prompt-eval per turn (2.4s vs 0.5s sorted);
// on a paid API it is the difference between a cache hit and paying full price
// for ~2,900 tokens of unchanged tool schema. No caller depends on map order.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Snapshot() map[string]Skill {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot := make(map[string]Skill, len(r.entries))
	for name, entry := range r.entries {
		snapshot[name] = entry.skill
	}
	return snapshot
}
