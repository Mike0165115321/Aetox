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
	// The first pack, born in desktop/browser_tool.go a day before this file
	// existed. Declared here even though its implementation lives in the
	// desktop, because this table is the one place that knows a packed tool's
	// vocabulary — a browser vocabulary declared next to the browser would be
	// the second list this file's header warns about. The desktop's browserSkill
	// reads it back through the same PackedActions/Narrow the in-package tools
	// use, which is what retired its private profile-reading gate on 2026-08-10.
	"browser": {
		tool:    "browser",
		actions: []string{"open", "read", "click", "type", "wait", "back", "scroll", "capture", "tabs", "dialog", "console", "network"},
		names: map[string]string{
			"open":  "browser_open",
			"read":  "browser_read",
			"click": "browser_click",
			"type":  "browser_type",
			// capture is a right of its own rather than part of read, because
			// "may read this page" and "may see it" are not the same permission:
			// a picture carries everything the text left out, including whatever
			// the user happens to have on screen in it.
			"capture": "browser_capture",
			// tabs is a right of its own for the same reason capture is: it is
			// the only action that can make a page disappear from under the
			// user, and a profile that grants "may look at pages" should not be
			// granting "may close them" by implication.
			"tabs": "browser_tabs",
			// Three added 2026-08-17 for one reason each, and none of them is
			// "a browser can do this too":
			//
			//   wait   — the only thing that can tell "not yet" from "not there".
			//   back   — removes a prediction rather than adding a capability.
			//   dialog — turns a page that stops the tab dead into an answerable
			//            question.
			//
			// Separate rights because they are separate acts. `dialog` in
			// particular can answer OK to a confirm() guarding a deletion, which
			// is not something "may read pages" should ever imply.
			"wait":   "browser_wait",
			"back":   "browser_back",
			"dialog": "browser_dialog",
			// Two more added 2026-08-22, and two rights rather than one for the
			// reason the rest of this table keeps repeating: they are different
			// acts. The console is what the page said about itself; the network
			// list is every address it called, which is a map of the services
			// behind a page and can carry a credential in a query string. A
			// profile granted "may see this page's errors" is not thereby
			// granted the second.
			"console": "browser_console",
			"network": "browser_network",
			// Added 2026-08-24, and it rides on browser_read's right rather than
			// earning one of its own: scrolling reveals what a page was always
			// going to show the reader, and a profile granted "may read this
			// page" was never granted only its first screen. Nothing becomes
			// reachable that reading could not already reach — the page just
			// finishes arriving.
			"scroll": "browser_read",
		},
	},
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
	// Delegation, packed on 2026-08-16 when a fourth half of it — declaring a run
	// before starting one (internal/subagent/run.go) — turned out not to fit: the
	// block was at 10,004 of its 10,100 tokens, and this family alone was 2,277 of
	// them, 22% spent on one mechanism that the model calls with one verb at a
	// time. Owner's call, and it is the same call §99 made for shell: the answer
	// to "there is no room for the next tool" is not a bigger block.
	//
	// `start` is the fallback for the same reason `run` is shell's — every
	// existing call, every desk manifest and every habit says `task` with a brief
	// and no action, and an action the model must now remember to name would be a
	// rename wearing a schema.
	"task": {
		tool:     "task",
		fallback: "start",
		actions:  []string{"start", "collect", "answer", "plan"},
		names: map[string]string{
			"start":   "task",
			"collect": "task_result",
			"answer":  "task_answer",
			"plan":    "task_plan",
		},
	},
	// The desk itself: the surface the user is looking at. Packed 2026-08-20 on
	// the owner's call — "desk นี่ไงที่ผมอยากแพ็ค" — and it is the clearest
	// one-object case left, because a desk is one object by definition.
	//
	// What is NOT in it is the point. `desk_terminal` stays its own tool, and
	// the browser has been its own pack since the beginning: those are things
	// that LIVE on the surface and carry their own back-and-forth. This pack is
	// the surface — put something on it, see what is on it, take something off.
	//
	// Leaving the terminal out is what makes this pack free on both axes a pack
	// is judged by. Every action here is CategoryAgent, so a desk that carries
	// one carries all three (`desk_terminal` is CategoryShell, and the
	// specialized desk refuses shell on purpose); and every action here only
	// ever looks, so วางแผน can carry the whole tool without gaining a way to
	// change the machine. A pack whose members disagree on either axis has to
	// be cut by name, and neither the desk gate nor the stance can cut inside a
	// pack — only a profile can (subagent.FilterRegistry).
	"desk": {
		tool:     "desk",
		fallback: "open",
		actions:  []string{"open", "list", "close"},
		names: map[string]string{
			"open":  "desk_open",
			"list":  "desk_list",
			"close": "desk_close",
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
	// Finding where something is, at three depths (search_pack.go). The action
	// names ARE the permission names here, and that is not laziness: `list`,
	// `glob` and `grep` were the tool names, they are what every desk manifest,
	// sub-agent profile and user permission rule already says, and renaming
	// them to buy tidiness would have silently widened or narrowed every one of
	// those lists.
	"search": {
		tool: "search",
		// No fallback. Listing a folder and grepping it are not one act with a
		// different argument, and no one of the three is common enough that
		// leaving it unnamed reads as obvious.
		actions: []string{"list", "glob", "grep"},
		names: map[string]string{
			"list": "list",
			"glob": "glob",
			"grep": "grep",
		},
	},
	// Changing what is on disk, at four sizes (change_pack.go). Same rule as
	// `search` above: every act here writes, so the pack falls on one side of
	// every gate that already sorts reads from writes.
	//
	// `append` is the one action whose name is not its permission. It rides on
	// `edit` because it IS editing - the same call with mode=append - and it is
	// a verb of its own in the block because a model choosing between five
	// verbs makes fewer mistakes than one choosing a verb and remembering a
	// flag. `browser`'s `scroll` sits on `browser_read` for the same reason.
	"change": {
		tool: "change",
		// No fallback. Writing a whole file and deleting one are not one act
		// with a different argument, and getting the wrong one is not a typo
		// the user can undo.
		actions: []string{"write", "edit", "append", "batch", "delete"},
		names: map[string]string{
			"write":  "write",
			"edit":   "edit",
			"append": "edit",
			"batch":  "edits",
			"delete": "delete",
		},
	},
	// Reading a file a model has no sense for (media_pack.go). Action words
	// rather than the old tool names because "image" IS the act here - the
	// _ocr and _transcribe halves say how it is done, which is the tool's
	// business and not the caller's.
	"media_read": {
		tool:    "media_read",
		actions: []string{"image", "video", "audio"},
		names: map[string]string{
			"image": "image_ocr",
			"video": "video_ocr",
			"audio": "audio_transcribe",
		},
	},
	// Asking the code about itself (codebase_pack.go). `rename` is not here:
	// same language server, other side of every gate, because it writes.
	"codebase": {
		tool:    "codebase",
		actions: []string{"errors", "symbol", "map"},
		names: map[string]string{
			"errors": "diagnostics",
			"symbol": "symbol",
			"map":    "repo_map",
		},
	},
	// Pull requests (pr_pack.go). The first pack that straddles the read/write
	// line, and it may because วางแผน can now narrow one act at a time (Step 0,
	// mode.AllowsAction): the three reads stay, the two that announce something
	// do not. The rule it replaces held only while a stance took tools whole.
	"pr": {
		tool:    "pr",
		actions: []string{"list", "read", "checks", "create", "comment"},
		names: map[string]string{
			"list":    "pr_list",
			"read":    "pr_read",
			"checks":  "pr_checks",
			"create":  "pr_create",
			"comment": "pr_comment",
		},
	},
	// The two engines, packed the same day as shell and github and for a
	// sharper reason: these are the tools about to grow. §100's proof loop
	// wants run and executions on n8n, and as five standalone names that work
	// would have been three more entries in the block of everyone who connected
	// an engine — packed, it is three more lines in one description. The server
	// starters (`n8n_server_start`, `windmill_server_start`) are not actions
	// here: they live in the desktop, switch a process on this machine rather
	// than speak to one, and desktop tools cannot be routed by a wrapper that
	// lives below them.
	"n8n": {
		tool:    "n8n",
		actions: []string{"list", "read", "create", "update", "activate"},
		names: map[string]string{
			"list":     "n8n_workflow_list",
			"read":     "n8n_workflow_read",
			"create":   "n8n_workflow_create",
			"update":   "n8n_workflow_update",
			"activate": "n8n_workflow_activate",
		},
	},
	"windmill": {
		tool: "windmill",
		// workspaces first because it is first in every session: everything
		// else on this engine is scoped to a workspace id that only it can
		// report.
		actions: []string{"workspaces", "list", "read", "create", "update"},
		names: map[string]string{
			"workspaces": "windmill_workspace_list",
			"list":       "windmill_flow_list",
			"read":       "windmill_flow_read",
			"create":     "windmill_flow_create",
			"update":     "windmill_flow_update",
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
