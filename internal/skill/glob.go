package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
			"limit": map[string]any{
				"type":        "integer",
				"description": "Return at most this many paths.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Skip this many paths first. With limit this pages through a result set that hit the cap, instead of having to invent a narrower pattern.",
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

// globPrefix returns the pattern's leading literal directories.
//
// A pattern is only ever a filter here, so the walk used to start at the
// sandbox root regardless — which, with no project focused, is the user's
// entire home directory. The narrower the pattern the worse that got: nothing
// matched, so the maxResults early exit never fired and the walk ran to the
// last file under AppData. "aetox/output/**/*.html" took 51s on a real home
// directory to find one file it already knew the directory of. Starting the
// walk at that literal prefix makes it immediate.
func globPrefix(pattern string) string {
	segs := strings.Split(strings.ReplaceAll(strings.TrimSpace(pattern), `\`, "/"), "/")
	if len(segs) < 2 {
		return "" // no directory part: matches a basename anywhere, so the walk can't narrow
	}
	literal := make([]string, 0, len(segs)-1)
	for _, seg := range segs[:len(segs)-1] {
		if seg == "" || strings.ContainsAny(seg, "*?[{") {
			break
		}
		literal = append(literal, seg)
	}
	return strings.Join(literal, "/")
}

func (s *globSkill) Execute(ctx context.Context, input Input) (Output, error) {
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

	walkFrom := searchPath
	// Only when no explicit path was given: with one, the caller has already
	// narrowed the walk, and the pattern may well be written relative to it.
	if searchPath == "." {
		if prefix := globPrefix(pattern); prefix != "" {
			walkFrom = prefix
		}
	}
	basePath, err := resolveSandboxPath(s.root, walkFrom)
	if err != nil {
		return newToolOutput("glob", command, "", start, false, err), err
	}
	root, err := resolveSandboxPath(s.root, ".")
	if err != nil {
		return newToolOutput("glob", command, "", start, false, err), err
	}
	// A literal prefix that isn't a directory means nothing can match — say so
	// instead of walking, and never as an error: "no such directory" for a
	// pattern the user only guessed at reads like a broken tool.
	//
	// That reasoning holds for a prefix this tool took out of the pattern, and
	// only for that. The same sentence for a directory the *caller* named is the
	// tool answering a question about the filesystem with a fact it never
	// checked (searchBaseExists, and the day it cost). Which one this is, is
	// exactly whether an explicit path came in.
	if info, statErr := os.Stat(basePath); statErr != nil || !info.IsDir() {
		if searchPath != "." {
			err := searchBaseExists(searchPath, basePath)
			if err == nil {
				// It exists and is not a directory: a file cannot hold a pattern
				// with a path in it, and saying nothing matched would hide that
				// the caller pointed glob at the wrong kind of thing.
				err = errors.New(searchPath + " is a file, not a folder to search")
			}
			return newToolOutput("glob", command, "", start, false, err), err
		}
		return newToolOutput("glob", command, "(no files matched)", start, false, nil), nil
	}

	const (
		maxResults = 300  // one page, when the caller asks for no particular size
		maxScan    = 5000 // ceiling on what a single walk will hold in memory
	)
	limit := maxResults
	if want := intArg(input["limit"]); want > 0 && want < limit {
		limit = want
	}
	skip := intArg(input["offset"])
	if skip < 0 {
		skip = 0
	}
	// Collect a page's worth past the offset and stop. Sorting happens after
	// the walk, so this has to gather everything the caller might be shown
	// before it can cut anything.
	// ponytail: past maxScan the newest-first order is over the first 5,000
	// paths the walk happened to reach, not over the whole tree — raise it, or
	// index by mtime, only if a tree that large ever needs exact paging.
	collect := skip + limit
	if collect > maxScan {
		collect = maxScan
	}
	type hit struct {
		rel string
		mod time.Time
	}
	hits := make([]hit, 0)
	guard := newSandboxWalk(basePath)

	walkErr := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		// Without this the Stop button does nothing: a walk over a home
		// directory runs for a minute with no way out.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if name := d.Name(); path != basePath && (strings.HasPrefix(name, ".") || IgnoredDirs[name]) {
				return filepath.SkipDir
			}
			if guard.refuses(path) {
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
		// A name is a smaller leak than a body, and still a leak: `**/*.json`
		// from a root above <DataRoot> otherwise lists the credential files.
		if guard.refuses(path) {
			return nil
		}
		info, infoErr := d.Info()
		mod := time.Time{}
		if infoErr == nil {
			mod = info.ModTime()
		}
		hits = append(hits, hit{rel: rel, mod: mod})
		if len(hits) >= collect {
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

	// Sort before paging: "newest first" has to mean newest of everything the
	// walk saw, or page two would be a different ordering than page one.
	sort.Slice(hits, func(i, j int) bool { return hits[i].mod.After(hits[j].mod) })
	capped := len(hits) >= collect
	if skip < len(hits) {
		hits = hits[skip:]
	} else {
		hits = nil
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	paths := make([]string, 0, len(hits))
	for _, h := range hits {
		paths = append(paths, h.rel)
	}

	output := strings.Join(paths, "\n")
	if output == "" {
		output = "(no files matched)"
		if skip > 0 {
			output = "(no files past offset " + strconv.Itoa(skip) + ")"
		}
	}
	output, truncated := limitLines(output, defaultToolOutputLineLimit)
	if capped {
		output += "\n... (result cap reached — continue with offset=" + strconv.Itoa(skip+limit) + ")"
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
	return s.Execute(ctx, Input{
		"args":   callArgs,
		"limit":  args["limit"],
		"offset": args["offset"],
	})
}
