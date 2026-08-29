package skill

import (
	"context"
	"fmt"
	"github.com/Mikedev115/Aetox/internal/command"
	"github.com/Mikedev115/Aetox/internal/model"
	"strings"
	"sync"
)

// ToolFilter answers "is this registered tool on this session's desk?" — the
// mode filter (internal/mode, ARCHITECTURE.md §83), passed in rather than
// imported because skill cannot know what a desk is.
//
// It is a token filter, not the safety gate: a tool it hides is simply never
// sent to the model, while every tool it keeps still passes exactly the
// approval rules it always did. Source is handed over because MCP tools are
// judged by their server and skills are never judged at all.
type ToolFilter func(name string, source Source) bool

// ToolActionFilter is the finer question a packed tool makes possible: not
// "is this tool on the desk" but "which of its actions are".
//
// It exists because a desk and a stance could only ever answer the coarse one,
// and packing made that answer wrong rather than merely rough. `browser` is one
// name over four rights and `shell` is one over four; วางแผน keeps every tool
// that reads and drops every tool that writes, so a pack holding both had to go
// whole — taking the reads with it (internal/mode/stance.go said so out loud
// and left this undone on purpose). The same coarseness would have cost the
// specialized desk its `list` and `glob` the day the file tools were packed,
// because its manifest names actions and a pack has one name.
//
// tool is the packed name; action is the per-action PERMISSION name — the
// spelling every gate below the tool block already judges by (Unpack), the one
// desk manifests, sub-agent profiles and the user's permission rules are
// written in. Nothing new has to be spelled anywhere for this to work.
//
// Nil means no narrowing, which is what every host had before this and what a
// full desk still gets.
type ToolActionFilter func(tool, action string) bool

type Dispatcher struct {
	registry   *Registry
	commandSet map[string]struct{}
	// allow is nil for the full desk — every session before modes existed, and
	// every host that has no modes. Nil means "carries everything", so the
	// no-mode path is the original code path rather than a filter that says yes.
	allow ToolFilter
	// narrowActions is nil for every caller that has no opinion below the tool
	// name. Set through WithActions rather than a fourth constructor parameter,
	// so the two existing constructors and every one of their callers stay as
	// they are.
	narrowActions ToolActionFilter

	// taught remembers which tools have already handed over their Guidance in
	// this session, because guidance is a once-per-session thing and this is
	// the only object in the system whose lifetime is exactly one session.
	//
	// Guarded because a turn can run tools concurrently, and two first calls
	// racing would either teach twice or teach neither.
	taughtMu sync.Mutex
	taught   map[string]bool
}

func NewDispatcher(registry *Registry) *Dispatcher {
	return NewDispatcherFor(registry, nil)
}

// NewDispatcherFor builds the dispatcher one session runs on: the same shared
// registry, seen through that session's desk.
//
// This is why the dispatcher moved from boot to session-open (§83). The
// registry stays assembled once — installing something has one home — and what
// varies per session is only which of it is visible. Note that the command set
// is snapshotted here while ToolDefinitions reads the registry live, so an MCP
// server that finishes connecting mid-session still appears (and is still
// filtered, because the filter runs at read time).
func NewDispatcherFor(registry *Registry, allow ToolFilter) *Dispatcher {
	d := &Dispatcher{registry: registry, allow: allow}
	if registry != nil {
		d.commandSet = command.BuildCommandSet(d.Names())
	}
	return d
}

// carries reports whether name is on this dispatcher's desk. An unregistered
// name answers false — there is nothing to carry.
func (d *Dispatcher) carries(name string) bool {
	if d.allow == nil {
		return true
	}
	source, ok := d.registry.SourceOf(name)
	if !ok {
		return false
	}
	return d.allow(name, source)
}

// WithActions attaches the per-action filter and hands the dispatcher back, so
// a caller can build and narrow in one expression.
func (d *Dispatcher) WithActions(f ToolActionFilter) *Dispatcher {
	if d != nil {
		d.narrowActions = f
	}
	return d
}

// cut applies the action filter to one registered skill.
//
// It answers with the skill the caller should actually use and whether it may
// be used at all — a packed tool with no surviving action is not a tool that
// refuses every call, it is a tool this desk does not have, and it must
// disappear from the block rather than sit in it as a wall.
//
// pack.narrow is deliberately not used to decide that. Its silence rule reads
// "named nothing" as "wants it whole", which is right for a sub-agent profile
// that simply did not mention tools and would be exactly wrong here: a desk
// that allows none of a pack's actions has said something, not nothing.
func (d *Dispatcher) cut(name string, s Skill) (Skill, bool) {
	if d == nil || d.narrowActions == nil {
		return s, true
	}
	packed, ok := s.(Packed)
	if !ok {
		return s, true
	}
	var allowed []string
	for _, action := range packed.Actions() {
		if d.narrowActions(name, action) {
			allowed = append(allowed, action)
		}
	}
	switch {
	case len(allowed) == 0:
		return nil, false
	case len(allowed) == len(packed.Actions()):
		// Whole, so hand back the original rather than a copy of it. A pack
		// that nobody narrowed must be byte-for-byte the tool it was.
		return s, true
	}
	return packed.Narrow(allowed), true
}

func (d *Dispatcher) Execute(ctx context.Context, input string) (Output, bool, error) {
	if d == nil || d.registry == nil {
		return Output{}, false, nil
	}

	intent := command.Parse(input, command.ParseTokens, d.commandSet)
	if intent.Kind != command.KindSkill {
		return Output{}, false, nil
	}
	name := intent.Command
	args := intent.Args

	skill, ok := d.registry.Get(name)
	if !ok || !d.carries(name) {
		return Output{}, false, nil
	}

	execInput := Input{
		"raw":  input,
		"args": args,
	}
	output, err := skill.Execute(ctx, execInput)
	if err != nil {
		return output, true, fmt.Errorf("skill %q failed: %w", name, err)
	}
	return output, true, nil
}

// Names is what this session can reach, not what is installed. The Tools page
// asks the registry instead, because "what did I install" and "what is on this
// desk" are different questions and one list cannot answer both.
func (d *Dispatcher) Names() []string {
	if d == nil || d.registry == nil {
		return nil
	}
	all := d.registry.Names()
	if d.allow == nil {
		return all
	}
	names := make([]string, 0, len(all))
	for _, name := range all {
		if d.carries(name) {
			names = append(names, name)
		}
	}
	return names
}

func (d *Dispatcher) ToolDefinitions() []model.ToolDefinition {
	if d == nil || d.registry == nil {
		return nil
	}
	names := d.Names()
	definitions := make([]model.ToolDefinition, 0, len(names))
	for _, name := range names {
		s, ok := d.registry.Get(name)
		if !ok || s == nil {
			continue
		}
		tool, ok := s.(Tool)
		if !ok {
			continue
		}
		// SourceSkill entries stay registered — the user's /skill-name command
		// still dispatches them — but their definitions are not sent to the
		// model. The model reaches skills through skills_list/skill_view
		// (progressive.go), so the tool block stays the same size whether the
		// user has installed three skills or three hundred.
		if source, ok := d.registry.SourceOf(name); ok && source == SourceSkill {
			continue
		}
		cut, ok := d.cut(name, s)
		if !ok {
			continue
		}
		if tool, ok = cut.(Tool); !ok {
			continue
		}
		definitions = append(definitions, tool.ToolDefinition())
	}
	return definitions
}

func (d *Dispatcher) ExecuteTool(ctx context.Context, name string, args map[string]any) (Output, bool, error) {
	if d == nil || d.registry == nil {
		return Output{}, false, nil
	}
	skillName := strings.ToLower(strings.TrimSpace(name))
	if skillName == "" {
		return Output{}, false, nil
	}
	skill, ok := d.registry.Get(skillName)
	if !ok || skill == nil || !d.carries(skillName) {
		// A tool this desk does not carry was never in the block the model was
		// sent, so a call for it is a hallucinated name and answers like one.
		return Output{}, false, nil
	}
	// The same cut the block was built through. Without it the definition would
	// be narrowed and the door would not: a desk that was never offered an
	// action would still run it for a model that named it anyway, which is the
	// one failure a token filter must never turn into.
	skill, ok = d.cut(skillName, skill)
	if !ok {
		return Output{}, false, nil
	}
	tool, ok := skill.(Tool)
	if !ok {
		return Output{}, false, nil
	}
	result, err := tool.ExecuteTool(ctx, args)
	result = d.teach(guidanceKey(skillName, args), skillName, skill, args, result)
	if err != nil {
		return result, true, err
	}
	return result, true, nil
}

// teach attaches a tool's Guidance to the first result it returns this session.
//
// On the result rather than in the block, because the block is paid for on
// every message and this is needed once (see guidance.go). On the FIRST result
// whether it succeeded or failed, because a first call that went wrong is
// exactly when the judgment was missing.
//
// It writes both Content and RawOutput: RawOutput is what the model actually
// reads (turn.toolRunOutput prefers it), and Content is what the timeline
// shows. Teaching only one of them would either teach nobody or teach the user
// instead of the model.
func (d *Dispatcher) teach(key, name string, s Skill, args map[string]any, out Output) Output {
	guide := guidanceFor(s, args)
	if guide == "" {
		return out
	}
	d.taughtMu.Lock()
	if d.taught == nil {
		d.taught = map[string]bool{}
	}
	already := d.taught[key]
	d.taught[key] = true
	d.taughtMu.Unlock()
	if already {
		return out
	}
	// Marked, and marked as being about the tool rather than about the page or
	// the file it just touched. Unlabelled, this reads as output — and output
	// from a tool is data the model is right to be suspicious of, which is the
	// wrong footing for the one text here that is genuinely ours.
	header := "[" + key + ", how to use this well, sent once]\n" + strings.TrimSpace(guide) + "\n\n"
	out.Content = header + out.Content
	out.RawOutput = header + out.RawOutput
	return out
}

// Snapshot is this desk's tools by name — the CLI's command descriptions are
// built from it, so a tool the session cannot run must not be offered for
// completion either.
func (d *Dispatcher) Snapshot() map[string]Skill {
	if d == nil || d.registry == nil {
		return nil
	}
	all := d.registry.Snapshot()
	if d.allow == nil {
		return all
	}
	for name := range all {
		if !d.carries(name) {
			delete(all, name)
		}
	}
	return all
}
