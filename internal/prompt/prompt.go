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
// so a repo that already has one works with Aetox without a new file.
var ProjectContextFileNames = []string{"AETOX.md", "AGENTS.md"}

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
func BuildWithReport(surface Surface, sandboxRoot string) (string, Loaded) {
	var b strings.Builder
	b.WriteString(identity(surface))
	b.WriteString(environment(sandboxRoot))

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

func environment(sandboxRoot string) string {
	root := strings.TrimSpace(sandboxRoot)
	if root == "" {
		root = "(unknown)"
	}
	return "Current working sandbox root is: " + root + ".\n" +
		"Do NOT proactively mention or leak this path to the user in general greetings or unrelated conversation " +
		"unless they explicitly ask about files, directories, paths, or workspace locations.\n"
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
