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
	b.WriteString(identity(surface))
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
	b.WriteString(environment(scope))
	// Directly after environment, which is where the session's reach is
	// described: the project is the one nearby fact that does *not* change that
	// reach, and it reads as a correction to the sentence above it rather than
	// as a new rule of its own.
	b.WriteString(workingIn(scope.Space))
	b.WriteString(capability())
	b.WriteString(fileEditing(desk))
	b.WriteString(batchWork())
	b.WriteString(computing())
	// Only where the surface can draw them. Both layers open with "your answer
	// is rendered as markdown, and inline SVG/HTML in it is drawn" — true of
	// the desktop's chat, false of a terminal, and a model told its terminal
	// renders SVG hands the user a page of coordinates where the picture was
	// meant to be. identity() has drawn this same line since it existed.
	if surface == SurfaceDesktop {
		b.WriteString(drawing())
		b.WriteString(panel())
	}
	b.WriteString(longform(desk))
	b.WriteString(narration())
	b.WriteString(clarify())

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
	return "The answer is drawn in the app's own document, so a <div> with a style attribute lays out the " +
		"way it would anywhere — a row of cards, a small table of figures, a set of bars beside their " +
		"labels. Reach for it when the answer is several things of the same kind, each carrying the same " +
		"few facts: that is a shape a person scans, and prose makes them read it instead.\n" +
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
	return "Your answer is rendered as markdown, and inline <svg> in it is drawn. When what you are " +
		"explaining is how several things relate — an order, a split, what feeds what, before against " +
		"after — draw it instead of describing it. A reader gets a shape in one look and a paragraph in " +
		"four sentences.\n" +
		"Keep it small: a viewBox, a dozen shapes at most, no gradients or filters. Size everything in " +
		"viewBox units and set width=\"100%\", because you do not know how wide the panel is. Use " +
		"fill=\"currentColor\" and var(--text-secondary)/var(--border-default)/var(--surface-raised) for " +
		"colour — the user's theme decides the palette, and a hardcoded #333 disappears on half of them. " +
		"Put every <text> at a real font-size in viewBox units; text with no size renders at 16px " +
		"regardless of scale and overflows the drawing.\n" +
		"The surface that draws it is a sanitizer, not a browser. <foreignObject>, <use> and <animate> " +
		"are removed from it without a word, and whatever you built inside one leaves a hole the size of " +
		"the space it held — so every label is a <text> at its own x/y, and the picture is still. Write " +
		"the whole drawing at the left margin with no blank line inside it and never inside a fenced " +
		"block: a blank line hands the rest of it to the markdown parser, and a fence shows it as source " +
		"instead of drawing it. It is capped at 420px tall on screen, so lay a drawing out across rather " +
		"than down, and point at nothing on the network.\n" +
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
			"so everything a chat produced sits in one place for the user to inspect.\n" +
			"That folder is chosen for you, and only the file tools know about it. A script you write and then " +
			"run does not: a path typed inside it is followed exactly, so a hardcoded one drops its results in " +
			"the working root while the script itself sits in the output folder, and the pair the user came for " +
			"ends up in two places. Have a script write beside itself — $PSScriptRoot in PowerShell, the " +
			"script's own directory anywhere else — or take the output path as an argument and pass it in.\n")
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
