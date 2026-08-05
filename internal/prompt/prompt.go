// Package prompt assembles the system prompt both front ends hand to
// cognitive.NewAgent, per ARCHITECTURE.md §11: identity, environment, user-global
// rules, project rules — most specific last, so project rules win on conflict
// (models weight later context higher). Read only at bootstrap (app start,
// project switch, model switch) — not per turn.
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/learned"
)

// Surface distinguishes the one sentence of identity text that differs
// between front ends today.
type Surface string

const (
	SurfaceCLI     Surface = "cli"
	SurfaceDesktop Surface = "desktop"
)

// maxLayerBytes caps how much of a single context file is folded into the
// prompt, so one oversized AETOX.md can't blow out the context window.
const maxLayerBytes = 16 << 10

// ProjectContextFileNames are checked in order under the sandbox root; the
// first one found is the project layer. AETOX.md takes priority; AGENTS.md is
// the ecosystem-convention fallback (OpenCode/Codex/Gemini CLI all use it),
// then CLAUDE.md (Claude Code) — so a repo already set up for either works
// with Aetox without a new file.
var ProjectContextFileNames = []string{"AETOX.md", "AGENTS.md", "CLAUDE.md"}

// Loaded reports which optional layers actually fed the prompt, so a caller
// (the desktop's project-status badge) can report the truth instead of just
// checking file existence separately and hoping it matches.
type Loaded struct {
	UserGlobalPaths []string // every identity file actually folded in, nil if none
	MemoryPath      string   // agent-written memory, "" when it has learned nothing
	DeskMemoryPath  string   // what this desk taught it, "" when the desk has learned nothing
	ProjectPath     string   // "" if not found/empty
}

// Desk is the mode a session was opened at (ARCHITECTURE.md §83), as much of
// it as the prompt needs: a name to scope memory by, and the direction its
// manifest carries. The zero value is every session from before desks existed
// and produces the prompt byte-for-byte as it was.
//
// A struct of two strings rather than *mode.Mode because prompt must not
// import mode — mode reads skill, and the dependency would run the wrong way
// round for a package this low. It also keeps the boundary honest: a desk
// hands this layer *direction*, never identity. What the assistant is stays in
// internal/prompt and the identity directory, whatever desk it sits at (§44.0).
type Desk struct {
	Name      string
	Direction string
}

// ProjectContextFile returns the path of whichever project context file
// exists directly under root (checked in ProjectContextFileNames order), or
// "" if none is present.
func ProjectContextFile(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	for _, name := range ProjectContextFileNames {
		p := filepath.Join(root, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// Scope is the set of folders the session may touch, mirroring
// skill.RegistryOptions field for field (Root/OpenSandbox/ExtraRoots). The
// prompt has to describe the same workspace the tools actually enforce, or the
// model answers "I can't" to things it can — which is exactly what happened
// once: the unfocused desktop opened the sandbox, and the model kept refusing
// absolute paths because this text still said they were rejected.
//
// A struct rather than three positional arguments because two of the three are
// a bool and a slice, and the compiler has nothing to say when a call site
// swaps them.
type Scope struct {
	// Root is the folder the session is rooted in — the focused project, or
	// the working root when none is focused.
	Root string
	// Open means no project is focused and the machine is the workspace.
	Open bool
	// Extra are folders the user added to the focused project. Named in the
	// prompt in full, because the model cannot use a folder it has not been
	// told about.
	Extra []string
}

// Build assembles the full system prompt for the given front end and scope, at
// the full desk. Sessions that were opened at one use BuildForDesk.
func Build(surface Surface, scope Scope) string {
	text, _ := BuildWithReport(surface, scope, Desk{})
	return text
}

// BuildForDesk is Build for a session opened at a desk: the same prompt, plus
// that desk's direction and whatever working at it has taught the agent.
func BuildForDesk(surface Surface, scope Scope, desk Desk) string {
	text, _ := BuildWithReport(surface, scope, desk)
	return text
}

// BuildWithReport is Build plus which optional layers were actually found.
//
// There is deliberately no per-agent role layer here: the assistant has one
// identity, and the identity directory (§11) is where it is configured. A second
// mechanism answering "who is the AI" would drift from it — see §44.0. A desk
// adds direction to that identity and never replaces it: "this session is
// coding work", never "you are a coding assistant".
//
// Order is the whole precedence policy, as it was before desks: engine text
// first, then what the user told the agent, then what the agent concluded, then
// what this project requires. The desk's direction sits with the engine text
// because it is engine text — an identity file the user wrote outranks it, and
// says so simply by coming later.
func BuildWithReport(surface Surface, scope Scope, desk Desk) (string, Loaded) {
	sandboxRoot := scope.Root
	var b strings.Builder
	b.WriteString(identity(surface))
	b.WriteString(environment(scope))
	b.WriteString(capability())
	b.WriteString(fileEditing())
	b.WriteString(batchWork())
	b.WriteString(narration())
	b.WriteString(clarify())
	if direction := strings.TrimSpace(desk.Direction); direction != "" {
		b.WriteString("\n" + direction + "\n")
	}

	var loaded Loaded
	loaded.UserGlobalPaths = foldIdentityLayers(&b)
	loaded.MemoryPath = foldLearnedMemory(&b, learned.MainScope,
		"What you have learned and the user approved")
	if desk.Name != "" {
		loaded.DeskMemoryPath = foldLearnedMemory(&b, learned.ModeScope(desk.Name),
			"What working on "+desk.Name+" has taught you, and the user approved")
	}
	if path := ProjectContextFile(sandboxRoot); path != "" {
		if content := readCapped(path); content != "" {
			b.WriteString(layer("Project rules", path, content))
			loaded.ProjectPath = path
		}
	}

	return strings.TrimRight(b.String(), "\n"), loaded
}

// foldIdentityLayers folds every *.md file in the user's identity directory
// (config.IdentityDir) into b, sorted by filename (os.ReadDir's own order),
// and returns the paths that actually contributed content.
func foldIdentityLayers(b *strings.Builder) []string {
	dir, err := config.IdentityDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var loaded []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		content := readCapped(path)
		if content == "" {
			continue
		}
		b.WriteString(layer("Personal instructions — "+e.Name(), path, content))
		loaded = append(loaded, path)
	}
	return loaded
}

// foldLearnedMemory adds what the agent has worked out for itself and had
// approved (internal/learned), and returns the file it came from.
//
// It sits **after** the user's identity files and **before** the project's
// rules, and the position is the policy: what the user told the agent outranks
// what the agent concluded, and what this project requires outranks both.
// Models weight later context more heavily, so ordering is the only mechanism
// needed to say so — no precedence language in the prompt, nothing to keep in
// sync with it.
//
// Only the main agent's scopes are read here — the cross-desk file, and the
// one belonging to the desk this session was opened at. A delegate's memory is
// folded into that delegate's own prompt (internal/subagent) and never into
// this one: carrying every sub-agent's accumulated knowledge in the main
// context is the thing that makes a single-brain agent's prompt grow with
// everything it has ever learned, and it is the cost Aetox is built to not pay.
// The desk scope is the same boundary along a second axis — what coding work
// taught the agent is not something the assistant desk pays to carry.
//
// A scope with nothing in it writes nothing at all, so a session at a desk that
// has learned nothing produces exactly the prompt it produced before any of
// this existed.
func foldLearnedMemory(b *strings.Builder, scope, title string) string {
	content := learned.Read(scope)
	if content == "" {
		return ""
	}
	path, err := learned.FileFor(scope)
	if err != nil {
		return ""
	}
	b.WriteString(layer(title, path, content))
	return path
}

func identity(surface Surface) string {
	place := "a terminal conversation"
	if surface == SurfaceDesktop {
		place = "a desktop chat UI"
	}
	return fmt.Sprintf("You are Aetox, a concise assistant in Thai and English that helps users through %s.\n", place)
}

// capability tells the model that the tools listed for it are not the whole
// inventory. It sits next to environment because they answer the same question
// from two sides: that one says where this session can reach, this one says
// what it can do.
//
// Nothing used to say it, and the gap is structural rather than hypothetical —
// skills are *already* behind a door. `skills_list` returns them on request and
// their bodies are never sent, which is what keeps a library of three hundred
// as cheap as a library of three (§71). But the only thing telling the model a
// skill exists at all was that one tool's own description, so whether it ever
// knocked was left to chance. A skill the user wrote for exactly this task is
// worth nothing if nobody opens it.
//
// Written as a principle rather than "call skills_list when the user mentions
// X", because the trigger is not a topic — it is a state: being about to say
// no, or about to build from nothing. That state is recognizable from the
// inside, and it stays recognizable when the next thing moves behind a door.
//
// The cost sentence is load-bearing. A model deciding whether to spend a round
// on a lookup needs the asymmetry: a lookup that finds nothing costs one cheap
// round, while a false "I can't" is indistinguishable to the user from a real
// limit, so they stop asking and the capability may as well not exist. It is
// the one wrong answer that hides its own mistake.
func capability() string {
	return "The tools listed for you are not everything this machine can do. Skill documents — " +
		"instructions the user installed for particular jobs — are never sent to you; skills_list " +
		"returns them on request. Work that someone already wrote down for exactly the task in " +
		"front of you is invisible until you go and look.\n" +
		"Look at two moments: when you are about to say something cannot be done, and when you are " +
		"about to build from scratch something that sounds like a job this user does more than once. " +
		"A lookup that finds nothing costs one cheap round. Telling the user you cannot do something " +
		"you were simply never shown reads to them exactly like a real limit — so they stop asking, " +
		"and it is the one wrong answer that hides its own mistake.\n"
}

// fileEditing tells the model how to change a file it has already written.
// Nothing used to, and the tool descriptions alone ("Create or overwrite a
// file" vs "Replace an exact string") give no reason to prefer one — so a
// small fix to an 800-line file was answered by streaming all 800 lines back
// through `write` again. Those lines are output tokens, paid for and slow:
// the user watches a minute of silence per edit, every edit.
func fileEditing() string {
	return "When changing a file that already exists, use the edit tool on just the part that changes. " +
		"Do NOT re-send the whole file through write — its content is output tokens, so rewriting an " +
		"800-line file to fix one line costs 800 lines of generation every time, and every one of those " +
		"lines also lands in the conversation.\n" +
		"Use write only to create a new file, or when genuinely replacing nearly all of an existing one.\n" +
		"Changing more than one place? Use apply_patch to make all the edits in a single atomic call — " +
		"either every edit applies or none do, and it costs one round instead of one per edit.\n" +
		"After changing source files, call diagnostics on them to confirm the change compiles before " +
		"moving on. Finding out later, from the user, means having built more work on a broken file.\n" +
		"To find the exact text to match, grep for it with a context of a few lines (and a glob when you " +
		"know the file type) — that usually gives you enough to write the edit without reading the file " +
		"at all. Otherwise read with offset and limit around the part you care about. Do not read a large " +
		"file end to end just to change one line in it.\n"
}

// batchWork tells the model to collapse list-shaped work into one script.
// Nothing used to, and the default failure mode is expensive in a way the
// model cannot see: renaming 200 files as 200 shell calls costs 200 rounds of
// schemas and results — a small-context model loses the thread long before
// the list ends, and a paid one re-reads the whole growing conversation every
// round. One script that loops is a single round at constant context, and the
// cheap models Aetox targets can write a 10-line loop far more reliably than
// they can stay coherent across 200 turns.
func batchWork() string {
	return "When the work is the same operation over many items — renaming files, converting a folder of " +
		"documents, applying one change to every match — do NOT loop by calling a tool once per item. " +
		"Write one shell script (or one command with a loop or glob) that does the whole list, run it with " +
		"shell, and check its summary output. Each tool call costs a full round of conversation; a script " +
		"costs one round for any list length. Spot-check a result or two afterwards instead of verifying " +
		"every item with its own call. Stay with individual tool calls when items genuinely need separate " +
		"judgment — code edits that differ per file are per-item work, not batch work.\n"
}

// narration asks for the one line per tool round that the timeline shows as
// the model working out loud (§59). Measured before adding this (2026-07-28,
// 42 debug logs): 28% of tool rounds already carried narration unprompted —
// the line raises that rate, it does not invent the behavior. Kept to a
// sentence: the narration is output tokens on every round of the loop.
func narration() string {
	return "When you are about to call tools, first say in one short sentence — in the user's language — " +
		"what you are about to do or what you just found, especially when you change direction. " +
		"The user watches this live; a silent stretch of tool calls reads as a frozen app.\n"
}

// clarify tells the model what an empty brief calls for: one question, before
// creating anything. Nothing used to, and an underspecified "create a file"
// forked two bad ways — the model invents a deliverable the user never asked
// for (paid for in output tokens, then paid for again in the round of "no,
// not that"), or it refuses and reports what it cannot do. The ask_user tool
// exists for exactly this moment, but its own description ("only when
// blocked") never fires here: a model with an empty brief does not feel
// blocked — it feels free.
//
// Owner constraint (2026-08-04): teach principles the model can weigh, never
// case rules. A first draft said 'a request for "slides" with no format named
// must be asked about' — an if-else written in prose, which answers one
// remembered failure and nothing else. The paragraph below states what the
// failure generalized to: a tool's usual mapping is a default, and defaults
// lose to anything the user actually said.
func clarify() string {
	return "When asked to create something without enough of a brief to know what the user actually wants — " +
		"no subject, format, or content named — ask ONE question to pin the brief down before creating anything. " +
		"Use the ask_user tool when you have it, offering concrete options; otherwise just ask in text. " +
		"A deliverable you invented costs the user a whole round of correcting you; one question is cheaper. " +
		"Ask only when the answer changes what you would build. Details the user would not care to decide, " +
		"decide yourself — and never ask more than once for the same request.\n" +
		"A tool's usual mapping is a default, not a decision. Before building a deliverable, weigh two " +
		"things: has the user already chosen its shape — anywhere, including a correction later in the " +
		"conversation — then follow that exactly, over any habit; and if not, could genuinely different " +
		"shapes each satisfy the request in ways the user would care about — then the choice is theirs, " +
		"and worth the one question. Otherwise decide sensibly and build.\n" +
		"A request can be perfectly clear and still rest on something that is not here — a project, a " +
		"file, an account. When two honest looks come back empty, that is the answer, not a reason to " +
		"look harder: widening the search spends the user's time to avoid one question they can settle " +
		"in a word. Say what you looked for, say you did not find it, and ask where it is.\n"
}

// environment used to state the sandbox root as an absolute path and then
// spend a second sentence telling the model not to repeat it — a machine-
// specific path, with the user's account name in it, sent to whichever
// provider is configured on every single request.
//
// It bought nothing. Every file tool rejects an absolute path (see
// resolveSandboxPath in internal/skill), so the root could not be used to
// call a tool even if the model wanted to, and its one real use — answering
// "where is that file on my machine" — is covered by write's own receipt,
// which now names the on-disk path. What replaces it is the rule that was
// actually missing, and whose absence caused the wrong answer: repeat the
// path a tool gave you, never assemble one.
//
// The three variants exist because the workspace itself is the user's choice
// (skill.sandboxPolicy): unfocused chats may roam the machine, a focused
// project may have folders added to it, and a model told "absolute paths are
// rejected" answers "I can't search this machine" while holding tools that can.
//
// Added folders are named in full, which is the one place a machine-specific
// path is worth sending to the provider. The root is not, and does not need to
// be — relative paths reach it. An added folder has no other name.
func environment(scope Scope) string {
	var b strings.Builder
	switch {
	case scope.Open:
		b.WriteString("No project is focused: file tools accept any path on this machine — absolute paths and paths " +
			"relative to your working folder both work. Credential stores (.ssh, .aws, browser profile data " +
			"and the like) are refused by every tool; do not try to work around that.\n" +
			"Create new files with a bare filename — they land in this chat's own output folder automatically, " +
			"so everything a chat produced sits in one place for the user to inspect.\n")
	case len(scope.Extra) > 0:
		b.WriteString("You are working in a focused project. A bare path is relative to the project folder.\n" +
			"The user has added these folders to this session, and file tools reach them by full path:\n")
		for _, dir := range scope.Extra {
			b.WriteString("  - " + dir + "\n")
		}
		b.WriteString("They carry the same rights as the project folder: you can read and edit them. " +
			"The user added them so you could go look — a problem here often starts somewhere else. " +
			"When you change a file in one of them, say which folder it was in, because the user is looking at " +
			"the project and will not assume you went outside it.\n" +
			"Anything outside the project and those folders is refused. The way through is to ask the user to add " +
			"that folder, not to look for another route in.\n")
	default:
		b.WriteString("You are working in a focused project: every file tool is confined to the project folder, and " +
			"a bare path is relative to it. Anything outside is refused — if the work needs a file from somewhere " +
			"else, ask the user to add that folder to the session; they can, and that is the intended way.\n")
	}
	b.WriteString("When you tell the user where a file is, repeat the path the tool reported back to you. Do NOT assemble " +
		"one yourself out of a folder and a filename — where a file lands is the tool's decision and it tells you, " +
		"so a path you construct is a guess.\n")
	// shell used to be the one tool outside this rule, so a model that hit a
	// refusal in read would reach for shell and get through. Now it does not,
	// and the model has to know that before it wastes a turn discovering it.
	b.WriteString("This applies to shell too: a command naming a path outside these folders is refused before it runs, " +
		"and reaching for shell after another tool refused a path will get the same answer. Write paths out literally " +
		"in commands — one assembled from a variable or a sub-command cannot be checked, so it is refused as well.\n")
	return b.String()
}

func layer(title, path, content string) string {
	return fmt.Sprintf("\n---\n# %s (%s)\n%s\n", title, path, content)
}

// readCapped reads path, trims it, and truncates to maxLayerBytes. Missing or
// unreadable files return "" rather than an error — every layer here is optional.
func readCapped(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > maxLayerBytes {
		data = data[:maxLayerBytes]
	}
	return strings.TrimSpace(string(data))
}
