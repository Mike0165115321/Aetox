package skill

// Packing: one name in the tool block, several rights inside it.
//
// desktop/browser_tool.go did this first, on 2026-08-10, for the reason the
// owner gave then — "browser จริง ๆ มันควรจะแพ็ครวมกัน... หากเพิ่มจะได้ไม่ต้องเสียเวลา
// มาไล่เปิดทีละตัว ๆ". Four tools were describing one object, and every
// capability added to that object used to cost another entry in the tool block
// of every request that carried it. Packed, it costs an action.
//
// This file is that idea moved off the browser and made general, because the
// browser could get away with something the next two tools cannot. It lives in
// the desktop package, so it can read the open session's profile directly
// (a.chairProfile()) and answer "which actions may this caller use?" inside its
// own ToolDefinition. `shell` and `github` live here, in internal/skill, which
// must never import desktop — so the answer has to arrive from outside, and
// arriving from outside is what Narrow below is for.
//
// The rule the whole file turns on, and the one worth reading twice:
//
//	Outside — what the model calls, what a hook matches, what the timeline
//	shows — is the packed name. Inside — what the approval gate judges, and
//	what the turn measures its patience against — is still the old per-action
//	name, because the act has not changed.
//
// That second half is not tidiness, it is the difference between working and
// broken. internal/safety keys on the literal string "shell" and answers
// RiskHigh for a call it cannot find a command in (safety.go:353); internal/
// turn stretches a turn's deadline only for a "shell_output" that was asked to
// wait (executor.go:78). Pack the three shell tools naively and reading a
// background command's output becomes a high-risk act the user is asked to
// approve, while a wait_for that was the whole point of the call gets cut off
// at sixty seconds. Unpack is what keeps both of those judging the same act
// they judged before.
//
// The table below is the single place that knows a packed tool's vocabulary.
// The tools read their action list back out of it rather than declaring one of
// their own — two lists of the same actions is exactly the second-place-
// answering-the-same-question this codebase treats as debt, and the drift would
// be silent: an action added to a tool and forgotten here would be un-judged by
// safety and unmatched by every permission rule the user wrote.

import "strings"

// pack is one packed tool's vocabulary.
type pack struct {
	// tool is the name in the block — what the model calls.
	tool string
	// fallback is the action assumed when a call names none. "" means the
	// action is required, which is right when no one action dominates.
	//
	// `shell` has one, and it matters more than it looks: the model has been
	// calling `shell` with a bare `command` since it was three tools, and every
	// desk manifest, every habit and the CLI's own text path assume that call
	// still works. An action the model must now remember to name would be a
	// rename wearing a schema.
	fallback string
	// actions, in the order a session uses them. The order reaches the model:
	// it is the enum, and it is the order the description lists.
	actions []string
	// names maps an action to the name it had before it was packed — the
	// vocabulary of permission, and the name every gate below the block still
	// judges the call by.
	names map[string]string
}

var packs = map[string]*pack{
	"shell": {
		tool:     "shell",
		fallback: "run",
		actions:  []string{"run", "output", "kill", "list"},
		names: map[string]string{
			"run":    "shell",
			"output": "shell_output",
			"kill":   "shell_kill",
			// The first action born packed, and the reason it could be: a new
			// name here costs nothing in the tool block, where a fourth
			// standalone tool would have been a hard sell for a call this
			// small. It exists because a lost handle used to be a dead end —
			// the ids were only ever named inside an error message, so the
			// model's one way to see its own jobs was to call wrong first.
			"list": "shell_list",
		},
	},
	"github": {
		tool: "github",
		// No fallback. Reading a file and searching for a repository are not
		// the same act with a different argument, and there is no call here
		// common enough that leaving it unnamed reads as obvious.
		actions: []string{"search", "repo_summary", "list_files", "read_file"},
		names: map[string]string{
			"search":       "github_search",
			"repo_summary": "github_repo_summary",
			"list_files":   "github_list_files",
			"read_file":    "github_read_file",
		},
	},
}

// packOf returns the vocabulary of a packed tool, or nil for every other tool.
func packOf(tool string) *pack {
	return packs[strings.ToLower(strings.TrimSpace(tool))]
}

// action reads the action out of a call, falling back where the pack has one.
//
// An action this pack does not have comes back "" rather than the fallback:
// the call is about to be refused by the tool itself, and quietly running the
// default instead would turn a typo into a command.
func (p *pack) action(args map[string]any) string {
	raw, _ := args["action"].(string)
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return p.fallback
	}
	if _, ok := p.names[name]; !ok {
		return ""
	}
	return name
}

// permissions are the per-action names, in the pack's own order.
func (p *pack) permissions() []string {
	out := make([]string, 0, len(p.actions))
	for _, a := range p.actions {
		out = append(out, p.names[a])
	}
	return out
}

// narrow reports which actions a caller who named these permission names may
// use, given the pack's full set.
//
// The silence rule, inherited from the browser and load-bearing: a caller that
// names none of the per-action names is not asking for a narrower tool — it
// asked for the packed one and gets it whole. Reading silence as "nothing
// allowed" would hand an agent a tool that refuses every call, which is the
// same fault as handing it a tool it does not have, only harder to see.
func (p *pack) narrow(named []string) []string {
	want := make(map[string]bool, len(named))
	for _, n := range named {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var out []string
	for _, a := range p.actions {
		if want[p.names[a]] {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), p.actions...)
	}
	return out
}

// Packed is a tool that is one name outside and several rights inside.
//
// Narrow returns a copy offering only the actions among `named` — the caller
// passes the per-action permission names, because those are what a profile's
// `tools:` line is written in and what the user reads on the Tools page. A
// caller that names none gets the tool whole; see narrow above.
type Packed interface {
	Skill
	// Actions are the per-action permission names this tool answers to.
	Actions() []string
	// Narrow returns a copy of this tool offering only these.
	Narrow(named []string) Skill
}

// Unpack reports the name the gates below the tool block should judge this call
// by: the per-action name for a packed tool, and the tool's own name for
// everything else.
//
// Called by internal/turn for safety assessment, permission rules, the approval
// prompt's wording and the turn's deadline — every reader that was written
// against the unpacked names and would otherwise start judging a call it can no
// longer recognize.
//
// A call whose action is unknown answers with the packed name. That call is
// about to be refused, and naming it after an action it does not have would put
// a word in the approval prompt that exists nowhere else in the product.
func Unpack(tool string, args map[string]any) string {
	p := packOf(tool)
	if p == nil {
		return tool
	}
	action := p.action(args)
	if action == "" {
		return p.tool
	}
	return p.names[action]
}

// PackedActions are the per-action permission names of a packed tool, or nil
// when the name is not one. internal/subagent asks it to decide whether a
// profile that names only actions should still be handed the packed tool.
func PackedActions(tool string) []string {
	p := packOf(tool)
	if p == nil {
		return nil
	}
	return p.permissions()
}

// PackedCall is one action of a packed tool, in both of its spellings: the word
// a call names it by, and the name every gate below the tool block judges it
// under.
//
// Both are needed by anything that has to explain a packed tool rather than
// merely gate one — a coverage test that must drive each action, and any screen
// that answers "what is this agent actually holding", where `shell` alone is a
// less true answer than `shell: run, output`.
type PackedCall struct {
	Action     string
	Permission string
}

// PackedCalls are a packed tool's actions in the order a session uses them, or
// nil for a tool that is not packed.
func PackedCalls(tool string) []PackedCall {
	p := packOf(tool)
	if p == nil {
		return nil
	}
	out := make([]PackedCall, 0, len(p.actions))
	for _, a := range p.actions {
		out = append(out, PackedCall{Action: a, Permission: p.names[a]})
	}
	return out
}
