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
	// ProjectMemoryPath is what working in THIS project settled — the agent's
	// own record, approved by the user. Distinct from ProjectPath below, which
	// is the file the USER wrote for this repository. "" when no project is
	// focused, or when this one has settled nothing yet.
	ProjectMemoryPath string
	ProjectPath       string // "" if not found/empty
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
	// Carries reports whether a tool is on this desk. nil means every tool is,
	// which is the zero Desk: a session from before desks existed, running the
	// whole registry.
	//
	// It exists because the engine layers below were desk-blind and taught
	// moves the desk could not make. fileEditing told the assistant — which
	// carries no `diagnostics`, that being a `code` tool — to call it after
	// every source edit, so the one desk aimed at people who have never opened
	// a terminal spent a round discovering a tool it was never given. A layer
	// that names a tool has to be able to ask whether the tool is here.
	//
	// A closure rather than a []string so this package still does not import
	// internal/mode: the caller already holds the manifest and its AllowsTool,
	// and a copied list is a second answer to a question mode already answers.
	Carries func(name string) bool
	// Delegates reports whether this desk may hand a whole job to another desk
	// (COMPANY.md §3's hiring door — `dispatch:` in the manifest). The coding
	// desk declares none, and its `task` tool does not even list the office's
	// agents (internal/subagent.available), so telling it to hand deliverable
	// work over describes a move with no target on the other side.
	//
	// Read through delegates(), never directly: a zero Desk is a session from
	// before desks existed, and that one could always reach every agent.
	Delegates bool
	// StanceDirection is what the session's *stance* folds in — the second axis
	// (DECISIONS.md §106): not what is on the desk, but how this turn runs.
	// Empty for ลงมือ, which adds nothing because it changes nothing.
	//
	// A second string beside Direction rather than concatenated into it,
	// because the two have different lifetimes and the ordering between them is
	// the policy: a desk is fixed for the session, a stance is the dial the user
	// just turned, and the later one wins. Folding them at the call site would
	// hide that rule in the caller.
	//
	// A string, like Direction, so this package still does not import
	// internal/mode.
	StanceDirection string
	// ToolLess says this session carries no tool definitions at all — คู่คิด,
	// and any later stance built the same way.
	//
	// It exists because Carries answers one name at a time and half the layers
	// below do not name anything: batchWork is about shell, narration is about
	// the pause before a tool round, clarify is about ask_user. Under a stance
	// that carries nothing, every one of them describes a move the model cannot
	// make — which is the exact failure Carries was added to stop, arriving
	// through the door Carries cannot watch.
	//
	// Skipping them is also most of what คู่คิด is for. The tool block is the
	// headline saving; these paragraphs are the rest of it, and they were only
	// ever instructions for using tools.
	//
	// Deliberately does NOT skip drawing/panel: those describe how the *answer*
	// is rendered, not how a tool is called, and a diagram is very much
	// something a conversation produces.
	ToolLess bool
	// Planning says this session's whole answer is a plan — the วางแผน stance,
	// and anything later built the same way.
	//
	// Separate from StanceDirection, which already carries *what a plan is*,
	// because this asks a different question: whether the surface in front of
	// the user can draw one as an object. That is not the stance's to know. The
	// same stance runs in a terminal, where the wrapper below is a fence the
	// user reads as punctuation, so the decision belongs here beside drawing()
	// and panel() — the two layers that were already gated on what the surface
	// can render rather than on what the session is doing.
	//
	// A bool rather than the stance name, for the reason Carries is a function
	// rather than a desk name: this package still does not import
	// internal/mode, and a second stance that also answers with a plan should
	// get the card by saying so, not by being added to a list here.
	Planning bool
}

// carries answers Desk.Carries for the zero value too: a desk that was never
// told what it holds holds everything.
func (d Desk) carries(name string) bool {
	if d.Carries == nil {
		return true
	}
	return d.Carries(name)
}

// delegates is carries' counterpart, and leans on the same sentinel. A nil
// Carries means nobody described this desk, which is the pre-desks full desk —
// and that one carried every tool *and* could hand work to any agent. Reading
// the bool alone would make the zero value a desk that is full of tools and
// forbidden to delegate, which was never a desk that existed.
func (d Desk) delegates() bool {
	return d.Carries == nil || d.Delegates
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
	// CanAsk means this host can put a card in front of the user when a path
	// lands outside the workspace, offering to add the folder it lives in
	// (skill.WidenFunc). It changes what the model should DO about a folder it
	// needs: with a door, naming the path is the request; without one — the CLI,
	// anything headless — the refusal is final and the model has to say so.
	//
	// Told to the model only because the two situations call for different
	// behaviour. Nothing here grants anything.
	CanAsk bool
	// Space is the โปรเจกต์ this session is being held inside, if any. It
	// changes nothing about what the session may reach — see the Space type.
	Space Space
}

// Space is the storefront door's โปรเจกต์: a named folder that groups chats and
// keeps a few files every session inside it should start knowing about
// (COMPANY.md §84).
//
// It is not a scope, despite living on one. Root, Open and Extra all answer
// "what may this session touch"; this answers "what is this session about", and
// the sandbox is exactly the same with it as without. Keeping it here anyway is
// deliberate: everything the prompt says about where the session stands is
// assembled from one struct, and a second struct for the half that grants
// nothing is how the two descriptions start disagreeing.
type Space struct {
	Name        string
	ContextPath string
	// Files are the names in ContextPath — never their contents. See the
	// workingIn layer for why naming them is the whole job.
	Files []string
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
// That holds for a chair too. A direct chat with one of the team runs on that
// worker's own brief as its direction — it is still Aetox, specialised, not a
// second personality (§44.0). What the brief needed was to be *read*, which is
// a matter of where it sits, not of replacing the identity above it.
//
// Order is the whole precedence policy, as it was before desks: engine text
// first, then what the user told the agent, then what the agent concluded, then
// what this project requires. The desk's direction sits with the engine text
// because it is engine text — an identity file the user wrote outranks it, and
// says so simply by coming later.
func BuildWithReport(surface Surface, scope Scope, desk Desk) (string, Loaded) {
	sandboxRoot := scope.Root
	var b strings.Builder
	b.WriteString(identity())
	// Second, always — one rule, no branch on what kind of direction it is.
	//
	// It used to be written twelve sections down, which put a chair's brief at
	// 68% of a 15.7k prompt: the worker was asked who it was and answered from
	// the assistant's opening line, correctly, because that was the first thing
	// it read. Direction is the answer to "what is this session", and an answer
	// filed after ten thousand characters of machine rules is not one.
	//
	// The cost is real and small: two sessions at different desks now share only
	// this opening line as a cached prefix instead of the whole engine block.
	// Sessions at the same desk — which is how anyone actually works — share
	// everything they did before.
	if direction := strings.TrimSpace(desk.Direction); direction != "" {
		b.WriteString("\n" + direction + "\n\n")
	}
	// Directly after the desk's direction, and after on purpose (§106.4). The
	// two answer the same question at two scales — what is this session, then
	// what is this turn — and where they disagree the dial the user just turned
	// has to be the one that wins. Position is the only mechanism there is for
	// saying so.
	//
	// Not filed at the end with the machine rules, either. The paragraph above
	// spends itself explaining why direction cannot sit ten thousand characters
	// in; a stance is the same kind of answer and inherits the same reason.
	if stance := strings.TrimSpace(desk.StanceDirection); stance != "" {
		b.WriteString(stance + "\n\n")
	}
	// Third, immediately before environment, because the two are the same
	// question on two axes: this one is where the answer lands, that one is
	// where the session can reach. Read in order the prompt now narrows — who
	// is speaking, what this session is, where the answer goes, what it can
	// touch.
	//
	// Not first. It was, for about ten minutes on 2026-08-11, and that put it
	// between identity and the direction above — which the comment on that
	// block spends a paragraph explaining must be second. A layer worth 142
	// bytes does not get to erode a placement that was moved up on purpose.
	//
	// Not down beside drawing()/panel() either, though the craft it enables
	// lives there: whether markdown renders at all decides how every answer is
	// written, not only the ones with a picture in them, and the terminal half
	// has no drawing() to sit beside.
	b.WriteString(surfaceLayer(surface))
	b.WriteString(environment(scope))
	// Directly after environment, which is where the session's reach is
	// described: the project is the one nearby fact that does *not* change that
	// reach, and it reads as a correction to the sentence above it rather than
	// as a new rule of its own.
	b.WriteString(workingIn(scope.Space))
	// Everything from here to clarify() is instruction for using tools. A
	// session carrying none reads it as a description of moves it cannot make,
	// so the whole block is skipped rather than gated line by line — see
	// Desk.ToolLess.
	if !desk.ToolLess {
		b.WriteString(capability())
		// Each gated on a tool it is entirely about, which is what Desk.Carries
		// is for. A desk has never withheld these — every desk writes files and
		// two of the three have a shell — but a *stance* does: วางแผน keeps
		// every reading tool and takes the writing and running ones away, and
		// without these gates it would be handed three paragraphs on how to use
		// them. The gate is on the tool each layer opens with, not on a stance
		// name, so a later stance that withholds the same thing is covered by
		// the line that is already here.
		if desk.carries("edit") || desk.carries("write") {
			b.WriteString(fileEditing(desk))
		}
		// Ungated on purpose, unlike its neighbours: this one is about how to
		// send calls at all, not about any particular tool, so it applies to
		// every desk that has more than nothing. A stance that withholds the
		// writing tools still reads and greps, which is exactly the shape of
		// work this saves the most round trips on.
		b.WriteString(parallelCalls())
		if desk.carries("shell") {
			b.WriteString(batchWork())
		}
		b.WriteString(computing())
	}
	// Only where the surface can draw them. Both layers open with "your answer
	// is rendered as markdown, and inline SVG/HTML in it is drawn" — true of
	// the desktop's chat, false of a terminal, and a model told its terminal
	// renders SVG hands the user a page of coordinates where the picture was
	// meant to be. identity() has drawn this same line since it existed.
	if surface == SurfaceDesktop {
		b.WriteString(drawing())
		b.WriteString(panel())
		// Gated on both, and the pair is the point: the stance decides that this
		// turn produces a plan, the surface decides whether a plan can be drawn
		// as an object. In a terminal the same stance produces the same plan and
		// this layer is simply absent, so nothing tells the model to write a
		// fence the user would read as punctuation.
		if desk.Planning {
			b.WriteString(planCard())
		}
	}
	// longform is about writing the answer to a file with `write`, narration is
	// about the sentence before a tool round, clarify is about ask_user. Same
	// block, same reason as above — kept separate only because drawing/panel
	// sit between them and those two stay.
	if !desk.ToolLess {
		// longform's whole instruction is "write it to a .md file yourself with
		// write". Under a stance that withheld write it told the model to reach
		// for the one tool it had just been refused — and the answer to a long
		// question there is to give it inline, which is what happens anyway
		// once nothing is telling it otherwise.
		if desk.carries("write") {
			b.WriteString(longform(desk))
		}
		b.WriteString(narration())
		b.WriteString(clarify())
	}

	var loaded Loaded
	loaded.UserGlobalPaths = foldIdentityLayers(&b)
	loaded.MemoryPath = foldLearnedMemory(&b, learned.MainScope,
		"What you have learned and the user approved")
	if desk.Name != "" {
		loaded.DeskMemoryPath = foldLearnedMemory(&b, learned.ModeScope(desk.Name),
			"What working on "+desk.Name+" has taught you, and the user approved")
	}
	// What working in THIS project settled. Only for a session focused on one:
	// an open-sandbox session is rooted at the machine, and a memory keyed to
	// that folder would be a junk drawer every unfocused session shared.
	//
	// Between the desk's memory and the project's own rules on purpose. A desk
	// is the same desk in every repository, so what one project settled must
	// not outrank it there; and what the user wrote in AETOX.md outranks
	// anything the agent concluded about the same code.
	if !scope.Open && sandboxRoot != "" {
		loaded.ProjectMemoryPath = foldLearnedMemory(&b, learned.ProjectScope(sandboxRoot),
			"What working in "+filepath.Base(sandboxRoot)+" has settled, and the user approved")
	}
	if path := ProjectContextFile(sandboxRoot); path != "" {
		if content := readCapped(path); content != "" {
			b.WriteString(layer("Project rules", filepath.Base(path), content))
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
		// The name goes in layer's own slot rather than into the title, so all
		// four user-named layers spell it one way.
		b.WriteString(layer("Personal instructions", e.Name(), content))
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
	// Resolved for the caller's report, not for the prompt: these three titles
	// each name their own scope already, and the file behind them was named by
	// Aetox rather than by the user (see layer).
	path, err := learned.FileFor(scope)
	if err != nil {
		return ""
	}
	b.WriteString(layer(title, "", content))
	return path
}

// identity answers one question and stops: who is speaking, and how.
//
// It used to answer three — name, language and which surface the answer lands
// on — and the third was the one that hurt. "what happens to what I write" was
// stated here, decided again by the switch in BuildWithReport, and restated a
// third and fourth time in the opening sentence of drawing() and panel(). Four
// places, one question; a third surface would have had to find all four. That
// is what surface() below is for, and identity() is now a constant.
//
// The language line is a rule, not a list. It said "in Thai and English", which
// is this build's first user rather than anything true about Aetox — a French
// speaker reading it would reasonably wonder which of the two to write in
// (owner's call, 2026-08-11: "ไม่ผูกกับภาษาสิ เพราะบางทีผู้ใช้อื่นๆอาจจะงงได้").
// It stays at all, rather than being left to a model that mirrors its user
// anyway, because everything around it is in English while several tool
// descriptions are in Thai, and one sentence is cheaper than that ambiguity.
func identity() string {
	return "You are Aetox, a concise assistant. Speak the user's language.\n"
}

// surface owns the whole of "where does what I write end up" — the question
// identity() used to open and drawing()/panel() used to re-open.
//
// The terminal half is the reason this is worth a layer of its own. It was
// never stated: a CLI session got the words "a terminal conversation" and was
// left to infer that markdown does not render and SVG is not drawn, because the
// two layers that say so are desktop-only. An inference is not an instruction,
// and this one is easy to miss on a model whose training is full of chat UIs.
//
// Mathematics joined the list on 16 ส.ค., when the desktop learned to typeset
// it. The delimiters are named because a model that cannot tell whether they
// will be drawn has two ways to hedge and both are worse than the equation:
// spelling the integral out in words, or reaching for unicode superscripts that
// run out at the first fraction.
func surfaceLayer(s Surface) string {
	if s == SurfaceDesktop {
		return "Your answer is rendered as markdown in a chat panel: inline <svg> is drawn, LaTeX between " +
			"\\( \\) or \\[ \\] is typeset as mathematics, and a <div> with a style attribute lays out as " +
			"it would anywhere.\n"
	}
	return "Your answer goes to a terminal as plain text. Markdown is not rendered, SVG is not drawn and " +
		"LaTeX is not typeset — write for someone reading it as characters.\n"
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
		"and it is the one wrong answer that hides its own mistake.\n" +
		// The same mistake from the other side. A tool that is genuinely absent
		// — an account nobody connected, a server that was never added — is a
		// sentence the user can act on, and silence about it is not modesty. It
		// is also not a reason to stop: what can be done without the missing
		// thing is still worth doing, and usually most of the answer.
		"When something you need really is absent, say which one and where it is switched on, and ask " +
		"for it. Then do the part that does not need it — refusing the whole job over one missing piece, " +
		"and quietly finishing without it, are both wrong.\n"
}

// fileEditing tells the model how to change a file it has already written.
// Nothing used to, and the tool descriptions alone ("Create or overwrite a
// file" vs "Replace an exact string") give no reason to prefer one — so a
// small fix to an 800-line file was answered by streaming all 800 lines back
// through `write` again. Those lines are output tokens, paid for and slow:
// the user watches a minute of silence per edit, every edit.
//
// What is stated and what is left out was cut back on 2026-08-09: a model
// already knows an edit is cheaper than a rewrite, so the layer keeps the
// instruction and drops the economics lecture behind it. What stays is what a
// model cannot know from the tool list — that apply_patch is atomic, that
// grep-with-context usually beats reading the file, and the diagnostics step,
// which is now asked for only where it exists.
func fileEditing(desk Desk) string {
	s := "When changing a file that already exists, use the edit tool on just the part that changes. " +
		"Do NOT re-send the whole file through write — rewriting an 800-line file to fix one line costs " +
		"800 lines of generation, every time.\n" +
		"Use write only to create a new file, or when genuinely replacing nearly all of an existing one.\n" +
		"Changing more than one place? Use apply_patch to make all the edits in a single atomic call — " +
		"either every edit applies or none do, and it costs one round instead of one per edit.\n"
	if desk.carries("diagnostics") {
		s += "After changing source files, call diagnostics on them to confirm the change compiles before " +
			"moving on.\n"
	}
	return s +
		"To find the exact text to match, grep for it with a context of a few lines (and a glob when you " +
		"know the file type) — that usually gives you enough to write the edit without reading the file " +
		"at all. Otherwise read with offset and limit around the part you care about. Do not read a large " +
		"file end to end just to change one line in it.\n"
}

// parallelCalls tells the model that one reply may carry several tool calls.
//
// The loop has always run every call in a response (cognitive.Agent iterates
// response.ToolCalls), so this costs no new machinery — the capability was
// there and nothing ever asked for it, which is the expensive half. A model
// that emits one call per reply turns four independent file reads into four
// round trips, and a round trip is not cheap in the way it looks: the API is
// stateless, so every one of them re-sends the entire conversation. Measured on
// the owner's own database, 1,102 DeepSeek calls carried 29.2M input tokens —
// an average of 26.5K re-sent per call, for turns whose *typing* was a sentence.
//
// Prompt caching already absorbs most of that cost (93-98% hit rate on the same
// data), which is why this is about call count rather than context size: caching
// makes each re-send cheap, and only fewer round trips makes them fewer.
//
// The dependency clause is not padding. Told plainly to parallelize, a model
// will also parallelize a read of the file it is about to write — so the rule
// has to name the test (does this call need that call's output?) rather than
// the aspiration. Every one of opencode's seven per-model prompt files carries
// this instruction with that same clause, which is what convinced the owner it
// is a real technique and not a micro-optimization.
func parallelCalls() string {
	return "You can put several tool calls in one reply, and they run together. When the next calls do not " +
		"need each other's output — reading three files, a grep and a glob, checking two directories — send " +
		"them in a single reply instead of one per turn.\n" +
		"The test is dependency, not similarity: if one call's result decides what the next call should ask " +
		"for, they are sequential and must stay that way. Never guess an argument in order to send something " +
		"in parallel.\n" +
		"This matters more than it looks. Each reply re-sends the whole conversation to the model, so four " +
		"round trips cost four copies of everything said so far, where one reply carrying four calls costs " +
		"one.\n"
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
		"shell, and check its summary output: one round for any list length. Spot-check a result or two " +
		"afterwards instead of verifying every item with its own call. Stay with individual tool calls " +
		"when items genuinely need separate judgment — code edits that differ per file are per-item work, " +
		"not batch work.\n"
}

// computing says that a number in an answer is either worked out or made up,
// and that there is now something here to work it out with.
//
// The same shape as drawing: the tool cannot ask to be used, so without a layer
// saying when, it is a capability that ships and never fires. What makes this
// one worth a layer of its own is that its failure is invisible from the
// outside — a wrong sum reads exactly like a right one, in the same confident
// sentence, and neither the model nor the user finds out. Every other thing the
// model gets wrong eventually announces itself.
//
// Where the line sits is the owner's call and it is not at "any arithmetic":
// a model doing 20% of 500 in its head is right, and a tool call to prove it
// costs a round trip and reads as ceremony. The line is at long work.
//
// But "long" had to be written as something countable — digits carried, steps
// that feed the next, a repetition down a list, a figure someone will act on —
// rather than as "when it is hard". Difficulty is the one thing a model cannot
// judge about its own arithmetic: 47 × 93 and 4.7 × 9.3 feel identical from the
// inside, and so does getting one of them wrong. A threshold made of feelings
// selects for the calculations that already looked easy, which is exactly the
// set where the silent mistakes are.
func computing() string {
	return "Short arithmetic is yours to do — one operation on small numbers, a round percentage, the days " +
		"between two dates: say it and move on.\n" +
		"Reach for calc when the work is long, not when it feels hard, because a wrong sum feels exactly " +
		"like a right one. Long means: numbers of several digits each, steps that feed the one after them " +
		"(compounding, instalments, a running balance), the same operation repeated down a list of more " +
		"than a handful, or a figure the user is going to act on — a price, a payroll line, a deadline. " +
		"The user is shown the script beside the result, so a mistake becomes a line somebody can point " +
		"at instead of a number they had to trust.\n" +
		"calc runs inside this app: it keeps nothing between calls, needs nothing installed, and cannot " +
		"reach a file or the network. When the numbers live in a file, or there are more of them than " +
		"you would type out, or the work needs a real library, that is write plus shell — which touches " +
		"the user's machine, and is worth the trip only when calc genuinely cannot answer.\n"
}

// panel says the answer surface can lay things out, not only draw them — the
// third capability found already working and never used.
//
// The chat renders an answer as sanitized markdown in the app's own document.
// Two consequences nothing had said out loud: a <div> with a style attribute
// lays out exactly as it would anywhere, and `var(--surface-panel)` inside that
// attribute resolves against the user's live theme, because it is the same
// document the app is painted in. A panel written this way is not styled to
// look like Aetox — it *is* Aetox's surface, on all fourteen themes, including
// the ones written after it.
//
// Separate from drawing() rather than folded into it because they answer
// different questions. A drawing is for how several things relate; a panel is
// for several things of the same kind, each with the same few facts — the shape
// a person scans down a column rather than reads. Merged, the layer would say
// "make something visual", which is the instruction that produces decoration.
//
// The hazards are the ones the sanitizer already enforces (no <style> element,
// no script) plus the one it cannot: a fixed pixel width overflows a bubble
// whose width the model never learns, exactly as an unsized <svg> did.
func panel() string {
	return "A laid-out block — a row of cards, a small table of figures, a set of bars beside their " +
		"labels — is worth reaching for when the answer is several things of the same kind, each " +
		"carrying the same few facts: that is a shape a person scans, and prose makes them read it " +
		"instead.\n" +
		"Colour it with the app's own variables — var(--surface-panel), var(--border-subtle), " +
		"var(--text-primary), var(--text-dim), var(--interactive) — never a hex value. They resolve " +
		"against whichever theme the user is running, so a panel written this way is the app's surface " +
		"rather than something pasted onto it, and it stays right on a theme that did not exist when you " +
		"wrote it.\n" +
		"Style only through style=\"…\" attributes: a <style> element and a <script> are both removed " +
		"before the answer is shown, so anything that depended on them is silently gone. Size everything " +
		"in percentages, fr units and minmax(0, 1fr) — you do not know how wide the panel is, and a fixed " +
		"pixel width spills out of it. Keep it to what the answer needs; a panel is a way of saying " +
		"something, and one built around a single fact is decoration.\n"
}

// planCard tells the model that on this surface a plan is drawn as an object of
// its own — a titled card in the transcript — and what to wrap it in so that
// happens.
//
// **Only the wrapper is here. What a plan *is* stays in the stance** (§106.11,
// mode.planShape): the four headings are policy that holds on every surface,
// and this layer is the one sentence that is true only where something can draw
// a card. Splitting it the other way would put the shape in two places, which
// is the debt §106.11 was written to avoid — and it would mean a terminal
// session silently lost its headings along with its card.
//
// A fenced block rather than a marker tag, because the renderer already has
// exactly this seam: markdown.ts intercepts a fence by its language and builds
// chrome around it, which is how a code block gets its header and its copy
// button. A plan is the same move with a different box, so the card costs a
// branch rather than a parser.
//
// The sentence about not fencing anything inside is the one that fails silently
// and therefore has to be stated. A ``` inside the plan closes the plan's own
// fence, and what the user gets is a card holding the first third of a plan with
// the rest spilled underneath it as loose prose — no error, and nothing about
// the result points at the cause.
func planCard() string {
	return "Your plan is drawn here as a card of its own — titled, and set apart from the conversation " +
		"around it — so write it inside a fenced block tagged `plan`, and make the first line inside that " +
		"block a `# ` heading naming the job in one line. The four headings go under it, unchanged.\n" +
		"Nothing else belongs in the block, and almost nothing belongs outside it: a sentence before the " +
		"card if something genuinely has to be said first, and no summary after it — the card is the " +
		"answer, and repeating it underneath is the same plan twice.\n" +
		"Do not open a fenced block anywhere inside the plan. It closes the plan's own fence, and the " +
		"result is a card holding the first part of your plan with the rest spilled out below it. " +
		"Inline `backticks` are safe and are how a filename or a setting should be written.\n"
}

// drawing tells the model that the answer surface can render a picture, and
// what kind of question a picture actually answers.
//
// The capability was already there and unused: the chat renders markdown
// through DOMPurify, which passes SVG and strips scripts and handlers, so an
// <svg> in an answer has always drawn. Nothing said so, so nothing ever drew.
//
// Written as the one condition where a picture wins — the answer is about how
// several things relate — rather than as a list of occasions to draw. A list
// of occasions produces drawings on the occasions and prose everywhere else,
// including the places where three boxes and two arrows would have ended the
// conversation. The size limit is the same reasoning as batchWork: every path
// in the drawing is output tokens, paid on the turn that makes it.
//
// The paragraph about the surface is there because every one of its rules fails
// silently. A model that lays a legend out in <foreignObject> — the obvious way
// to put wrapped text in a picture — gets a drawing with a hole in it and no
// error, and the next drawing is built the same way. Stating what the renderer
// is (a sanitizer, so anything that could execute is gone) rather than listing
// forbidden tags is what makes that generalize past the three tags named.
func drawing() string {
	return "When what you are explaining is how several things relate — an order, a split, what feeds " +
		"what, before against after — draw it instead of describing it. A reader gets a shape in one " +
		"look and a paragraph in four sentences.\n" +
		"Keep it small: a viewBox, a dozen shapes at most, no gradients or filters. Size everything in " +
		"viewBox units and set width=\"100%\", because you do not know how wide the panel is. Use " +
		"fill=\"currentColor\" and var(--text-secondary)/var(--border-default)/var(--surface-raised) for " +
		"colour — the user's theme decides the palette, and a hardcoded #333 disappears on half of them. " +
		"Put every <text> at a real font-size in viewBox units; text with no size renders at 16px " +
		"regardless of scale and overflows the drawing.\n" +
		"The surface that draws it is a sanitizer, not a browser. <foreignObject>, <use> and <animate> " +
		"are removed from it without a word, and whatever you built inside one leaves a hole the size of " +
		"the space it held — so every label is a <text> at its own x/y. Movement survives by exactly one " +
		"route: a <style> inside the <svg>, with @keyframes in it, driving transform or opacity on classes " +
		"you set there. Its rules and its animation names are scoped to your own drawing, so they cannot " +
		"reach the app or collide with a second drawing in the same answer; @property and @import are " +
		"dropped, and a <style> outside an <svg> is deleted whole. Animate only what the movement itself " +
		"is saying — a thing still running, a flow going one way — never as decoration on a still picture. " +
		"Write the whole drawing at the left margin with no blank line inside it and never inside a fenced " +
		"block: a blank line hands the rest of it to the markdown parser, and a fence shows it as source " +
		"instead of drawing it. A drawing is shown at the size of its own viewBox — 600 units wide draws " +
		"600 pixels wide — shrinking to fit only when the window is narrower than that, so choose viewBox " +
		"units as though they were pixels and pick font sizes that would read at that size. It is capped " +
		"at 720px wide and 420px tall, so lay a drawing out across rather than down, and point at nothing " +
		"on the network.\n" +
		"A drawing is not a decoration. Do not draw the shape of an answer that is one fact, one number, " +
		"or one instruction — say it.\n"
}

// longform says what a long written answer is made of: a markdown file the
// model writes itself.
//
// Nothing used to, and the gap had a direction. `doc_write` announces itself as
// the way to hand back writing, and the assistant desk told the model that
// written work is not its to produce — so an explanation, a plan, a set of
// notes, anything past a few paragraphs, went out to the document writer and
// came back a .docx. The user ends up with a folder of one-off documents where
// they wanted one readable file, and every one of them cost a delegated agent
// with its own context, its own reading, and its own round trip.
//
// Stated as what the two things are for rather than as "don't call the writer":
// the writers are not wrong, they are for deliverables — something to open in
// another program because the user asked for that. Long-form writing is not a
// deliverable, it is the answer, and the answer's plain-text home is .md. That
// distinction keeps working when a fourth writer is added; a prohibition on one
// tool name does not.
//
// Since the `chairs:` split (mode.CarriesForChair, 2026-08-06) the writers are
// not on any desk the main agent sits at — an agent holds them and the desk
// hands the job over. So the second paragraph names the *act* rather than the
// tools: "hand it to the agent whose craft it is" is true at every desk, while
// "use doc_write when…" would be advice about tools this agent cannot see.
//
// The chat surface renders markdown (see drawing), so the same file the user
// keeps is also the thing they can read in place — which is why there is no
// third option here to weigh.
func longform(desk Desk) string {
	s := "When your answer is long-form writing — an explanation, a plan, notes, findings, a comparison, " +
		"anything that runs past a few paragraphs and the user will want again later — write it to a .md " +
		"file yourself with write, and reply with a line or two saying what it is. Markdown is the default " +
		"for writing you produce: it is plain text, it renders here, the user can open it in anything, and " +
		"correcting it costs one edit.\n" +
		"A document, workbook or deck is a different request — a file the user asked for so they can open " +
		"it in another program. Length alone is not that request: do not turn writing into one because the " +
		"answer got long.\n"
	// The handover is the half that is not true everywhere. A desk with no
	// `dispatch:` cannot reach the agents who hold the writers, and its `task`
	// tool does not even list them — so at the coding desk this sentence used
	// to describe a move with nobody on the other end of it. The lesson above
	// is the part that holds at every desk, which is why only this is gated.
	if desk.delegates() {
		s += "You do not build those yourself: hand the job to the agent whose craft it is and collect " +
			"the file.\n"
	}
	return s +
		"One file per thing you were asked, named for what it holds, alongside the work it is about. A new " +
		"file for every explanation leaves the user hunting through a pile — if you are adding to something " +
		"you already wrote, edit that file instead.\n"
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
// It bought nothing *in a focused project*. There every file tool rejects a
// path outside the root, so the root could not be used to call a tool even if
// the model wanted to, and its one real use — answering "where is that file on
// my machine" — is covered by write's own receipt, which names the on-disk
// path. What replaced it is the rule that was actually missing, and whose
// absence caused the wrong answer: repeat the path a tool gave you, never
// assemble one.
//
// The three variants exist because the workspace itself is the user's choice
// (skill.sandboxPolicy): unfocused chats may roam the machine, a focused
// project may have folders added to it, and a model told "absolute paths are
// rejected" answers "I can't search this machine" while holding tools that can.
//
// Which is where the paragraph above stopped being true, and cost a whole
// session on 2026-08-11 to find out. Unfocused, the wall came down in the tools
// on 2026-08-04 (desktop.unfocusedRoot's note: "being unable to find a PDF the
// user knows is on disk made the mode useless") — but this text kept withholding
// every landmark, and a model told it may roam a machine it cannot name any
// address on can only guess. Asked for the user's Downloads it sent a bare
// `Downloads`, which is relative and resolved under <home>/aetox, read the
// "cannot find the file" as a wall, told the user it could not reach their disk
// at all, and handed the job back. Every tool it needed was in its hands.
//
// What it cost to fix was two wrong answers before the right one, and both are
// worth keeping because both are the same mistake in different clothes: writing
// down what the model already knows.
//
// The first named the home folder and listed "Downloads, Documents, Desktop,
// Pictures" — a case hardcoded into a prompt, wrong on any machine where the
// user moved them, and paid for on every request forever. The second moved that
// paragraph into the tool's own not-found error, which sounded better (the
// answer travelling with the refusal that needs it) and was the same error one
// layer down: a model reading "cannot find C:\Users\x\aetox\Downloads" does not
// need to be told that a bare path is relative, or that C:\Users\x exists. Both
// were deleted.
//
// What was actually missing is one fact and one deletion. The fact: the working
// folder is Aetox's own, which a model would otherwise reasonably read as the
// user's home. The deletion: shellIsWalledIn, which was telling it to stop.
// Everything else about roaming a filesystem, it can already do.
//
// So no scope names a path here. Relative paths reach the root; an added folder
// has no other name and is the one exception.
func environment(scope Scope) string {
	var b strings.Builder
	switch {
	case scope.Open:
		b.WriteString("No project is focused: the whole machine is the workspace. File tools and shell both take any " +
			"absolute path on it, and a bare path is relative to your working folder — which is Aetox's own, not " +
			"the user's home.\n" +
			"Credential stores (.ssh, .aws, browser profile data and the like) are refused by every tool; " +
			"do not try to work around that.\n" +
			"Create new files with a bare filename — they land in this chat's own output folder automatically, " +
			"so everything a chat produced sits in one place for the user to inspect.\n" +
			"That folder is chosen for you, and only the file tools know about it. A script you write and then " +
			"run does not: a path typed inside it is followed exactly, so a hardcoded one drops its results in " +
			"the working root while the script itself sits in the output folder, and the pair the user came for " +
			"ends up in two places. " +
			// No named idiom here. This used to read "$PSScriptRoot in
			// PowerShell, the script's own directory anywhere else", which spent
			// a Windows-only token on every session — including the ones whose
			// commands run inside a distro — while the general clause beside it
			// was already the whole instruction. A model is told which shell it
			// writes for, by a tool description built from that shell; how that
			// shell names a script's own directory follows from it. Naming one
			// is the case list this file refuses to keep (§99).
			"Have a script write beside itself, using whatever its own language calls the directory it is " +
			"in — or take the output path as an argument and pass it in.\n")
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
			outsideTheWorkspace(scope.CanAsk) + shellIsWalledIn)
	default:
		b.WriteString("You are working in a focused project: every file tool is confined to the project folder, and " +
			"a bare path is relative to it.\n" +
			outsideTheWorkspace(scope.CanAsk) + shellIsWalledIn)
	}
	b.WriteString("When you tell the user where a file is, repeat the path the tool reported back to you. Do NOT assemble " +
		"one yourself out of a folder and a filename — where a file lands is the tool's decision and it tells you, " +
		"so a path you construct is a guess.\n" +
		// True in every scope, unlike the confinement sentence above it: the
		// command scanner has to see a path to check it, in any workspace.
		"Write paths out literally in shell commands — one assembled from a variable or a sub-command cannot be " +
		"checked, so it is refused.\n")
	return b.String()
}

// outsideTheWorkspace says what a path outside the workspace means — and the
// two answers are genuinely different work, which is why this is not one
// sentence with a clause.
//
// With a door, naming the path IS the request: the user gets a card offering to
// add that folder, and a yes puts the same call through. Telling the model to
// "ask the user to add the folder" there would produce a paragraph asking for
// something a tool call would have asked for better — the failure this whole
// change exists to remove, arriving through the prompt instead of the gate.
//
// Without one (the CLI, anything headless) the refusal really is the end, and
// the useful thing is to say which folder was needed so the user can add it and
// run again.
//
// The "no" clause matters as much as the "yes" one. A refused card that the
// model retries is the same question in a loop, which is how a user learns to
// click through every card without reading it.
func outsideTheWorkspace(canAsk bool) string {
	if canAsk {
		return "Anything outside the project and those folders is refused — but you do not have to work around " +
			"that: name the path you need and the user is shown a card offering to add the folder it lives in. " +
			"Accept and the same call goes through; the folder joins the session's list and stays there. " +
			"If the user declines, that is their answer — say what you could not reach and carry on with the " +
			"rest, and do not raise the same folder again.\n"
	}
	return "Anything outside the project and those folders is refused, and this session has no way to ask for " +
		"more. Say which folder the work needed; the user can add it and run this again.\n"
}

// shellIsWalledIn closes the escape route a walled-in session would otherwise
// waste a turn discovering: shell used to be the one tool outside the sandbox,
// so a model refused by read would reach for it and get through. It is not any
// more.
//
// A constant used by the two focused branches rather than a line appended to
// all three, which is what it was until 2026-08-11. Appended, an unfocused
// session — the one workspace where shell genuinely does reach the whole
// machine — read "reaching for shell after another tool refused a path will get
// the same answer" and stopped. That is precisely what happened: after one
// mistyped relative path the model never called shell again, and shell was the
// tool that would have found the folder in one line. A sentence carried into a
// scope it was not written for is not a harmless extra; here it was the
// instruction that closed the last door.
//
// Note also what it says: "these folders" — a phrase with no referent at all in
// a workspace that is the whole machine.
const shellIsWalledIn = "This applies to shell as well: a command naming a path outside these folders is refused " +
	"before it runs, and reaching for shell after another tool refused a path gets the same answer.\n"

// workingIn says which โปรเจกต์ this conversation is being held inside, and
// where that project keeps its files.
//
// Written as a fact about the conversation rather than as an instruction,
// because that is what it is: nothing here grants a right, moves the sandbox or
// tells the assistant to do anything differently. The project is a folder of
// conversations, not a fence (COMPANY.md §84), and a layer that read like a
// permission would make it one in the model's understanding even though the
// gate never moved.
//
// The context files are named and not pasted. Naming them costs a line and buys
// the only thing the assistant is missing — knowing they are there — after
// which read/grep are already in its hands. Pasting the contents would spend
// the whole of every context window on files most turns never open, on every
// turn, forever, which is the version of this feature that quietly makes the
// assistant worse at everything else.
//
// A project with an empty context folder still says so. "There is nothing in it
// yet" is a different and more useful fact than silence, which the assistant
// would read as "no such folder" and never mention to the user.
func workingIn(space Space) string {
	if strings.TrimSpace(space.Name) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("This conversation is being held inside the user's project \"" + space.Name + "\". " +
		"That groups it with their other chats about the same thing; it does not narrow what you can reach, " +
		"and the folders you may work in are exactly the ones named above.\n")
	if len(space.Files) == 0 {
		b.WriteString("The project keeps its material in " + space.ContextPath + ", which is empty so far. " +
			"When the user hands over something this project should keep — a brief, a spec, a list they " +
			"keep coming back to — that is where it belongs, and every future chat in this project will see it.\n")
		return b.String()
	}
	b.WriteString("The project keeps its material in " + space.ContextPath + ". These files are in it:\n")
	for _, name := range space.Files {
		b.WriteString("  - " + name + "\n")
	}
	b.WriteString("They are named here, not included: read the ones a question actually needs. " +
		"Do not read all of them at the start of a conversation to be thorough — that is the user's whole " +
		"context window spent before they have asked anything.\n")
	return b.String()
}

// layer heads one folded file with what it is, and names the file only when the
// USER is the one who named it.
//
// It used to head every layer with the absolute path, which put the account name
// into every request that had a memory or an identity file to fold — the same
// charge environment() was rewritten to stop paying for the sandbox root,
// arriving by a different door and going unnoticed for longer because it only
// appears once the user has something to fold at all.
//
// The line between the two halves is who chose the name. An identity file and a
// project's rules were named by the user, who may well refer to them that way,
// and those names read the same on any machine. The agent's own memory files
// were named by Aetox, their titles already say which one each is, and one of
// them — projects/<name>-<hash>.md — hashes the absolute project root, so
// printing its name would put a machine-varying token back in the prompt to say
// something the title had already said. Half a leak is still a leak.
//
// Nothing is lost by that on the model's side. The memory files are not edited
// from there: they go through the `memory` tool and an approval, and the rule
// already in this prompt is to repeat a path a tool reported rather than
// assemble one. The person who wants the folder has the button on the settings
// page. Every path is still returned in Loaded, which is where a caller that
// genuinely needs one gets it.
func layer(title, name, content string) string {
	if name != "" {
		title += " (" + name + ")"
	}
	return fmt.Sprintf("\n---\n# %s\n%s\n", title, content)
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
