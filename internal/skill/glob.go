package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

var errGlobLimitReached = errors.New("glob result limit reached")

type globSkill struct {
	root string
}

func (*globSkill) Name() string { return "glob" }

func (*globSkill) Description() string {
	return "Find files by path pattern under sandbox root, newest first"
}

func (*globSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": `Path pattern, e.g. "**/*.go", "src/**/*.{ts,svelte}", "*_test.go". ** matches any number of directories.`,
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Relative directory to search from (default: whole sandbox)",
			},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "glob",
			// Newest first because "what did I just touch" is the question a
			// file search is usually standing in for.
			Description: "Find files by name or path pattern. Returns paths sorted by modification time, newest first. Dependency and build directories are never searched.",
			Parameters:  payload,
		},
	}
}

func (s *globSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("glob skill unavailable")
		return newToolOutput("glob", "glob", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: glob <pattern> [path]")
		return newToolOutput("glob", "glob", "", start, false, err), err
	}
	pattern := args[0]
	searchPath := "."
	if len(args) > 1 {
		searchPath = strings.TrimSpace(strings.Join(args[1:], " "))
	}
	command := "glob " + pattern
	if searchPath != "." {
		command += " " + searchPath
	}

	basePath, err := resolveSandboxPath(s.root, searchPath)
	if err != nil {
		return newToolOutput("glob", command, "", start, false, err), err
	}
	root, err := resolveSandboxPath(s.root, ".")
	if err != nil {
		return newToolOutput("glob", command, "", start, false, err), err
	}

	const maxResults = 300
	type hit struct {
		rel string
		mod time.Time
	}
	hits := make([]hit, 0)

	walkErr := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if name := d.Name(); path != basePath && (strings.HasPrefix(name, ".") || IgnoredDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !matchesPathGlob(pattern, rel) {
			return nil
		}
		info, infoErr := d.Info()
		mod := time.Time{}
		if infoErr == nil {
			mod = info.ModTime()
		}
		hits = append(hits, hit{rel: rel, mod: mod})
		if len(hits) >= maxResults {
			return errGlobLimitReached
		}
		return nil
	})
	if errors.Is(walkErr, errGlobLimitReached) {
		walkErr = nil
	}
	if walkErr != nil {
		return newToolOutput("glob", command, "", start, false, walkErr), walkErr
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].mod.After(hits[j].mod) })
	paths := make([]string, 0, len(hits))
	for _, h := range hits {
		paths = append(paths, h.rel)
	}

	output := strings.Join(paths, "\n")
	if output == "" {
		output = "(no files matched)"
	}
	output, truncated := limitLines(output, defaultToolOutputLineLimit)
	if len(hits) >= maxResults {
		output += "\n... (max results reached — narrow the pattern)"
		truncated = true
	}
	return newToolOutput("glob", command, output, start, truncated, nil), nil
}

// matchesPathGlob matches a slash-separated relative path against a pattern.
//
// filepath.Match alone cannot do this: its wildcards never cross a separator,
// so "**/*.go" — the form every other tool uses and every model reaches for
// first — would silently match nothing. Segments are matched one at a time,
// with "**" allowed to swallow any number of them.
func matchesPathGlob(pattern, rel string) bool {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, `\`, "/"))
	if pattern == "" {
		return false
	}
	// A bare "*.go" means "anywhere", which is what a person typing it means.
	if !strings.Contains(pattern, "/") {
		return matchesGlob(pattern, filepath.Base(rel))
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(pat, seg []string) bool {
	if len(pat) == 0 {
		return len(seg) == 0
	}
	if pat[0] == "**" {
		// Zero or more directories: try every split point.
		for i := 0; i <= len(seg); i++ {
			if matchSegments(pat[1:], seg[i:]) {
				return true
			}
		}
		return false
	}
	if len(seg) == 0 {
		return false
	}
	if !matchesGlob(pat[0], seg[0]) {
		return false
	}
	return matchSegments(pat[1:], seg[1:])
}

func (s *globSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || strings.TrimSpace(pattern) == "" {
		err := errors.New("pattern is required")
		return newToolOutput("glob", "glob", "", time.Now(), false, err), err
	}
	callArgs := []string{pattern}
	if path, ok := args["path"].(string); ok && strings.TrimSpace(path) != "" {
		callArgs = append(callArgs, strings.TrimSpace(path))
	}
	return s.Execute(ctx, Input{"args": callArgs})
}
