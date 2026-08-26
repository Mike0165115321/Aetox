package skill

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Mikedev115/Aetox/internal/model"
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
	// FromWorld marks an unsuccessful result as a report about the machine's
	// current state rather than about anyone's behaviour: a server that has not
	// answered yet, an MCP connection still being made. It is the same statement
	// internal/statereport carries on an error value, said for the failures that
	// deliberately do not use one.
	//
	// A tool reports a soft failure by returning Success:false with a nil error,
	// so the model can read the reason and try something else rather than seeing
	// a crash. That choice is right and it costs the classifier its only input,
	// because the kind is inferred from the error value and there is none. Every
	// such failure therefore arrived unmarked, and the learning floor read it as
	// a lesson with a remedy to quote: "n8n did not answer within 90 seconds"
	// became a permanent problem card about a server that would be up tomorrow.
	//
	// Only meaningful when Success is false. Ignored otherwise, so a tool that
	// sets it and then succeeds is not lying about anything.
	FromWorld bool
	// LinesAdded/LinesRemoved describe what a write did to a file, for the
	// timeline's "+9 -0". Both zero on tools that touch no file.
	LinesAdded   int
	LinesRemoved int
	// Diff is those same lines, said in full: git-style unified hunks for what
	// this call changed, empty on every tool that writes nothing. See hunk.go
	// for the format and for why it is built here rather than asked of git.
	//
	// It is the counts' other half. "+42 -17" is a claim, and the โค้ด desk is
	// the room where a claim about code is not what the user came for — they
	// came to see which lines. The counts stay because a folded row needs a
	// size; this is what unfolding one shows.
	Diff string
	// Images is how a tool returns a picture rather than a description of one.
	// Only `read` sets it, only when the host has said the model can see
	// (RegistryOptions.Vision) — a tool result is text everywhere else, and a
	// blind model still gets the image_ocr path it has always had.
	Images []model.Image
	// Artifacts are sandbox-relative paths to finished files this call produced
	// *for the user* — the spreadsheet they asked for, not the config file
	// something rewrote along the way. It is the same idea as Images one line
	// up: the tool hands back the thing itself instead of a sentence about it.
	//
	// The UI turns each into a card under the answer with a button that opens
	// it. That matters because the alternative is what shipped first: the file
	// existed, the model said its name, and reaching it meant opening the file
	// panel and hunting the tree — four clicks away from a product whose
	// promise is finished work. Only tools whose whole output *is* a file set
	// this; `write` and `edit` deliberately do not, or every code edit in a
	// coding turn would print a card.
	Artifacts []string
	// ProposalID is the queued change a call is waiting on a decision for. Only
	// `memory` sets it, and for the same reason Artifacts exists one field up:
	// the tool hands back the thing itself, not a sentence about it.
	//
	// Without it the id died inside the tool and the chat could say only that
	// something called "memory" had run — what it wanted to remember, and the
	// yes or no it was waiting for, lived two pages away in Settings. The id is
	// carried rather than the text so the card always reads the queue's current
	// answer: a proposal approved from Settings must not still be offering an
	// Approve button in a session reopened a week later.
	ProposalID int64
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
	SourceSkill     Source = "skill"     // a SKILL.md the user added, instructions, not a tool
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
