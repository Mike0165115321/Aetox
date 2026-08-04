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
	ProjectPath     string   // "" if not found/empty
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

// Build assembles the full system prompt for the given front end and sandbox
// root. openSandbox mirrors skill.RegistryOptions.OpenSandbox — the prompt
// must describe the same wall the tools actually enforce, or the model
// answers "I can't" to things it can (which is exactly what happened: the
// unfocused desktop opened the sandbox, and the model kept refusing absolute
// paths because this text still said they were rejected).
func Build(surface Surface, sandboxRoot string, openSandbox bool) string {
	text, _ := BuildWithReport(surface, sandboxRoot, openSandbox)
	return text
}

// BuildWithReport is Build plus which optional layers were actually found.
//
// There is deliberately no per-agent role layer here: the assistant has one
// identity, and the identity directory (§11) is where it is configured. A second
// mechanism answering "who is the AI" would drift from it — see §44.0.
func BuildWithReport(surface Surface, sandboxRoot string, openSandbox bool) (string, Loaded) {
	var b strings.Builder
	b.WriteString(identity(surface))
	b.WriteString(environment(openSandbox))
	b.WriteString(fileEditing())
	b.WriteString(batchWork())
	b.WriteString(narration())
	b.WriteString(clarify())

	var loaded Loaded
	loaded.UserGlobalPaths = foldIdentityLayers(&b)
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

func identity(surface Surface) string {
	place := "a terminal conversation"
	if surface == SurfaceDesktop {
		place = "a desktop chat UI"
	}
	return fmt.Sprintf("You are Aetox, a concise assistant in Thai and English that helps users through %s.\n", place)
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
		"and worth the one question. Otherwise decide sensibly and build.\n"
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
// The open variant exists because the wall itself is now a mode
// (skill.RegistryOptions.OpenSandbox): unfocused desktop chats may roam the
// machine, and a model told "absolute paths are rejected" answers "I can't
// search this machine" while holding tools that can.
func environment(openSandbox bool) string {
	if openSandbox {
		return "No project is focused: file tools accept any path on this machine — absolute paths and paths " +
			"relative to your working folder both work. Credential stores (.ssh, .aws, browser profile data " +
			"and the like) are refused by every tool; do not try to work around that.\n" +
			"Create new files with a bare filename — they land in this chat's own output folder automatically, " +
			"so everything a chat produced sits in one place for the user to inspect.\n" +
			"When you tell the user where a file is, repeat the path the tool reported back to you. Do NOT assemble " +
			"one yourself out of a folder and a filename — where a file lands is the tool's decision and it tells you, " +
			"so a path you construct is a guess.\n"
	}
	return "Every file tool takes a path relative to the folder you are working in; absolute paths are rejected.\n" +
		"When you tell the user where a file is, repeat the path the tool reported back to you. Do NOT assemble " +
		"one yourself out of a folder and a filename — where a file lands is the tool's decision and it tells you, " +
		"so a path you construct is a guess.\n"
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
