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

// Build assembles the full system prompt for the given front end and sandbox root.
func Build(surface Surface, sandboxRoot string) string {
	text, _ := BuildWithReport(surface, sandboxRoot)
	return text
}

// BuildWithReport is Build plus which optional layers were actually found.
//
// There is deliberately no per-agent role layer here: the assistant has one
// identity, and the identity directory (§11) is where it is configured. A second
// mechanism answering "who is the AI" would drift from it — see §44.0.
func BuildWithReport(surface Surface, sandboxRoot string) (string, Loaded) {
	var b strings.Builder
	b.WriteString(identity(surface))
	b.WriteString(environment())
	b.WriteString(fileEditing())
	b.WriteString(narration())

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
func environment() string {
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
