package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
)

var errGrepLimitReached = errors.New("grep result limit reached")

// The three shapes a search can come back in. content is what grep always did;
// the other two exist because "which files mention this" is the commonest
// question asked of a code search and answering it with every matching line
// costs one or two orders of magnitude more tokens than answering it with a
// list of paths.
const (
	grepModeContent = "content"
	grepModeFiles   = "files_with_matches"
	grepModeCount   = "count"
)

// IgnoredDirs are directories no search should descend into: dependency trees
// and build output, which are not the user's code and are enormous. On this
// repo alone node_modules is 10,826 of 12,073 files — searching it means
// opening ten thousand irrelevant files and filling the result limit with
// matches from vendored code before ever reaching src/.
//
// Exported so the desktop file tree hides exactly the same set: two lists that
// disagree about what "noise" means is how a search starts contradicting the
// tree beside it.
var IgnoredDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, "bin": true, "obj": true, "__pycache__": true,
	".vs": true, ".idea": true,
	// Unfocused, the sandbox root is the user's home directory, and AppData is
	// the bulk of it — hundreds of thousands of files of machine state that no
	// one has ever asked the assistant to search.
	"AppData": true,
}

type grepSkill struct {
	root         string
	outputSubdir func() string
}

func (*grepSkill) Name() string { return "grep" }

func (*grepSkill) Description() string {
	return "Search file contents under sandbox root with a regular expression"
}

func (*grepSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Go regular expression to search for. Prefix with (?i) for case-insensitive.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Relative file or directory to search (default: whole sandbox)",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Only search files whose name matches this pattern, e.g. *.go or *.{ts,svelte}",
			},
			"context": map[string]any{
				"type":        "integer",
				"description": "Lines of surrounding context around each match (default 0, max 50). Selects show=content.",
			},
			"show": map[string]any{
				"type":        "string",
				"enum":        []string{"content", "files_with_matches", "count"},
				"description": "files_with_matches (default) paths only; content the matching lines; count a per-file tally.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Return at most this many entries, matches in content mode, files otherwise.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Skip this many entries first. With limit, pages past the result cap.",
			},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "grep",
			Description: "Search file contents by regex (Go/RE2 syntax: no backreferences or lookaround). " +
				"Returns the paths of matching files; show=content returns path:line:text instead, and path-line-text for context lines. " +
				"Dependency and build directories are never searched.",
			Parameters: payload,
		},
	}
}

// Guidance carries the judgment that used to be spread through the block, and
// the one thing the signature cannot say: which mode to want.
//
// The block entry states that files_with_matches is the default. It cannot
// afford to state WHY, or what the other one costs, and those are exactly what
// decides whether a search is cheap. Measured on this machine 2026-08-27,
// before the default moved: 189 of 210 grep calls took content at 3,801 bytes
// each against 646 for a path list, on a schema that already said the cheap
// modes cost a fraction as much. A description is read once and a default is
// obeyed every time, which is why the default moved and this is only the
// explanation.
func (*grepSkill) Guidance(map[string]any) string {
	return "grep answers \"where does this live\" with paths, which is the question most searches are really asking. " +
		"Pass show=content once you know which file you mean, or ask for context lines and content is selected for you. " +
		"Context goes up to 50 lines either side, which is a whole function seen in place: a wide window on one match " +
		"is very often cheaper and better than opening the file it lives in. Narrow the pattern, then widen the window.\n" +
		"A path list costs roughly a sixth of the same search in content, so mapping first and reading second is the cheap order.\n" +
		"The result cap is 200 entries and limit only tightens it. A capped result names the offset to resume from, " +
		"so a search that hit the ceiling is paged rather than re-invented as a narrower pattern.\n" +
		"Matching lines are clipped at 200 characters, so a hit in generated code shows where it is and not what it says. " +
		"Build an edit from what read returns, not from a grep line."
}

func (s *grepSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("grep skill unavailable")
		return newToolOutput("grep", "grep", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		err := errors.New("usage: grep <pattern> [path]")
		return newToolOutput("grep", "grep", "", start, false, err), err
	}

	pattern := args[0]
	searchPath := "."
	if len(args) > 1 {
		searchPath = PlacedPath(s.root, s.outputSubdir, strings.TrimSpace(strings.Join(args[1:], " ")))
	}
	// Named options rather than more positional args: the CLI form stays
	// "grep <pattern> [path]" while a tool call can ask for more.
	glob := strings.TrimSpace(stringArg(input["glob"]))
	ctxLines := intArg(input["context"])
	if ctxLines < 0 {
		ctxLines = 0
	}
	if ctxLines > maxGrepContext {
		ctxLines = maxGrepContext
	}
	mode := strings.ToLower(strings.TrimSpace(stringArg(input["show"])))
	if mode == "" {
		// files_with_matches, not content, and this is the one line of the file
		// worth arguing about.
		//
		// Measured on this machine 2026-08-27: 189 of 210 grep calls took the
		// old default and cost 3,801 bytes each; the 11 that asked for
		// files_with_matches cost 646. The cheap mode was documented in the
		// schema as costing "a fraction of the tokens" and was still chosen 5%
		// of the time, because a description never outranks a default. So the
		// default becomes the cheap one and the expensive one is asked for by
		// name, which is the shape Claude Code's Grep ships.
		//
		// `show: "content"` is untouched and always will be. A changed default
		// that quietly removes the setting it replaced is a different change
		// than this one.
		mode = grepModeFiles
		// Except when context lines were asked for. "Show me three lines
		// around each match" is a request for the lines, and a file list
		// answers it with a green tick and none of what was wanted — the
		// silent-success failure this package has already been bitten by
		// (see readImage's refusal in read.go). Claude Code documents the
		// same pair as "ignored otherwise"; a weaker model reads a document
		// once and a result every time, so this honours it instead.
		if ctxLines > 0 {
			mode = grepModeContent
		}
	}
	if mode != grepModeContent && mode != grepModeFiles && mode != grepModeCount {
		err := errors.New("show must be content, files_with_matches or count")
		return newToolOutput("grep", "grep "+pattern, "", start, false, err), err
	}
	skip := intArg(input["offset"])
	if skip < 0 {
		skip = 0
	}
	command := "grep " + pattern
	if searchPath != "." {
		command += " " + searchPath
	}
	if glob != "" {
		command += " --glob " + glob
	}
	if ctxLines > 0 {
		command += " -C " + strconv.Itoa(ctxLines)
	}
	// Name whatever was not the default, so the logged command and the shown
	// one stay readable now that the default is the file list.
	if mode != grepModeFiles {
		command += " --" + mode
	}
	if skip > 0 {
		command += " --offset " + strconv.Itoa(skip)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return newToolOutput("grep", command, "", start, false, err), err
	}

	basePath, err := resolveSandboxPath(s.root, searchPath)
	if err != nil {
		return newToolOutput("grep", command, "", start, false, err), err
	}
	if err := searchBaseExists(searchPath, basePath); err != nil {
		return newToolOutput("grep", command, "", start, false, err), err
	}
	root, err := resolveSandboxPath(s.root, ".")
	if err != nil {
		return newToolOutput("grep", command, "", start, false, err), err
	}

	const (
		maxResults   = 200
		maxFileBytes = 1 << 20
		maxLineLen   = 200
	)
	// limit only ever tightens the cap. A model asking for 10,000 matches
	// is not a reason to send it 10,000 matches.
	limit := maxResults
	if want := intArg(input["limit"]); want > 0 && want < limit {
		limit = want
	}
	results := make([]string, 0)
	// matches counts every hit the walk sees, offset included, so the caller can
	// be told there is more behind the cap. emitted counts what came back.
	matches, emitted := 0, 0
	type fileHit struct {
		display string
		count   int
	}
	var perFile []fileHit

	guard := newSandboxWalk(basePath)

	walkErr := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// ponytail: skips all dot-dirs (.git, .cache, ...), allowlist if a dot-dir ever matters
			name := d.Name()
			if path != basePath && (strings.HasPrefix(name, ".") || IgnoredDirs[name]) {
				return filepath.SkipDir
			}
			if guard.refuses(path) {
				return filepath.SkipDir
			}
			return nil
		}

		if glob != "" && !matchesGlob(glob, d.Name()) {
			return nil
		}
		// The base was checked once; this is every file under it. Without it a
		// grep rooted above <DataRoot> reads the credential files by content that
		// `read` refuses by name.
		if guard.refuses(path) {
			return nil
		}

		file, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxFileBytes))
		_ = file.Close()
		if readErr != nil || bytes.Contains(data, []byte{0}) {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		display := filepath.ToSlash(path)
		if relErr == nil {
			display = filepath.ToSlash(rel)
		}

		clip := func(s string) string {
			s = strings.TrimRight(s, "\r")
			if len(s) > maxLineLen {
				s = s[:maxLineLen] + "..."
			}
			return s
		}

		lines := strings.Split(string(data), "\n")
		lastShown := -1 // last line index already written out for this file
		hits := 0
		for i, line := range lines {
			if !re.MatchString(line) {
				continue
			}
			matches++
			hits++
			if mode != grepModeContent {
				// files_with_matches has its answer for this file after one
				// hit; count needs the tally, so it keeps going.
				if mode == grepModeFiles {
					break
				}
				continue
			}
			if matches <= skip {
				continue // paging past a page already returned
			}
			from, to := i-ctxLines, i+ctxLines
			if from < 0 {
				from = 0
			}
			if to > len(lines)-1 {
				to = len(lines) - 1
			}
			// ripgrep's conventions, because models already read them fluently:
			// "--" between separated groups, ":" on a match, "-" on context.
			if lastShown >= 0 && from > lastShown+1 {
				results = append(results, "--")
			}
			if from <= lastShown {
				from = lastShown + 1
			}
			for j := from; j <= to; j++ {
				sep := "-"
				if j == i {
					sep = ":"
				}
				results = append(results, display+sep+strconv.Itoa(j+1)+sep+clip(lines[j]))
			}
			lastShown = to
			emitted++
			if emitted >= limit {
				return errGrepLimitReached
			}
		}
		if hits > 0 {
			perFile = append(perFile, fileHit{display: display, count: hits})
			// The file modes page over files, so their cap counts files too —
			// otherwise a repo-wide search walks every file to build a list
			// the caller asked to have cut short.
			if mode != grepModeContent && len(perFile) >= skip+limit {
				return errGrepLimitReached
			}
		}
		return nil
	})
	if errors.Is(walkErr, errGrepLimitReached) {
		walkErr = nil
	}
	if walkErr != nil {
		return newToolOutput("grep", command, "", start, false, walkErr), walkErr
	}

	var capped bool
	switch mode {
	case grepModeContent:
		capped = emitted >= limit
	default:
		if skip < len(perFile) {
			perFile = perFile[skip:]
		} else {
			perFile = nil
		}
		capped = len(perFile) >= limit
		if len(perFile) > limit {
			perFile = perFile[:limit]
		}
		results = make([]string, 0, len(perFile))
		for _, f := range perFile {
			if mode == grepModeCount {
				results = append(results, f.display+":"+strconv.Itoa(f.count))
				continue
			}
			results = append(results, f.display)
		}
	}

	output := strings.Join(results, "\n")
	if output == "" {
		output = "(no matches)"
		if skip > 0 {
			output = "(no matches past offset " + strconv.Itoa(skip) + ")"
		}
	}
	output, truncated := limitLines(output, defaultToolOutputLineLimit)
	if capped {
		// Naming the next offset, not just the fact of the cap: the whole point
		// of paging is that the caller can continue without guessing.
		output += "\n... (result cap reached, continue with offset=" + strconv.Itoa(skip+limit) + ")"
		truncated = true
	}

	return newToolOutput("grep", command, output, start, truncated, nil), nil
}

// maxGrepContext caps how much surrounding code one match may drag in.
//
// Raised from 20 to 50 on 2026-08-27, and the measurement is the argument.
// 115 of 210 grep calls in this machine's history asked for context, so the
// model already reaches for grep as a reader more often than not. At 20 lines
// either side it cannot hold a function, and a search that nearly answers the
// question is answered the rest of the way by opening the whole file: 237 of
// ~406 reads followed another read rather than a search.
//
// The arithmetic the old number was defending is unchanged and still holds,
// because it was never this constant doing the defending. 200 matches at 50
// lines either side is 20,000 lines in theory and cannot happen:
// defaultToolOutputLineLimit trims the output to 220 lines whatever produced
// it, and `limit` tightens the match cap for a caller who wants a wide window
// on few hits. Which is the shape this is for -- one match, seen properly,
// instead of a read of the file it lives in.
const maxGrepContext = 50

// matchesGlob reports whether a file name satisfies a caller's glob. Supports
// the two forms that actually get typed: "*.go" and "*.{ts,svelte}". A leading
// "**/" is accepted and ignored — matching is on the file name — so a pattern
// copied from another tool does not silently match nothing.
func matchesGlob(glob, name string) bool {
	glob = strings.TrimPrefix(strings.TrimSpace(glob), "**/")
	if glob == "" {
		return true
	}
	open := strings.Index(glob, "{")
	close := strings.Index(glob, "}")
	if open >= 0 && close > open {
		prefix, suffix := glob[:open], glob[close+1:]
		for _, alt := range strings.Split(glob[open+1:close], ",") {
			if ok, _ := filepath.Match(prefix+strings.TrimSpace(alt)+suffix, name); ok {
				return true
			}
		}
		return false
	}
	ok, _ := filepath.Match(glob, name)
	return ok
}

func (s *grepSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || strings.TrimSpace(pattern) == "" {
		err := errors.New("pattern is required")
		return newToolOutput("grep", "grep", "", time.Now(), false, err), err
	}
	callArgs := []string{pattern}
	if path, ok := args["path"].(string); ok && strings.TrimSpace(path) != "" {
		callArgs = append(callArgs, strings.TrimSpace(path))
	}
	return s.Execute(ctx, Input{
		"args":    callArgs,
		"glob":    args["glob"],
		"context": args["context"],
		"show":    args["show"],
		"limit":   args["limit"],
		"offset":  args["offset"],
	})
}
