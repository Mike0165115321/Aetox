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
	"sort"
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

// grepTypes maps a language name to the extensions it means, the way
// ripgrep's --type does and the way Claude Code's Grep exposes it.
//
// Why a name and not just `glob`: glob answers "files spelled like this", which
// is only the same question when the caller already knows how the repository
// spells them. TypeScript is .ts and .tsx; C++ is six extensions; a model that
// has not read the tree writes `*.ts` and silently misses every component.
// A name is one token, cannot be spelled wrong without being told, and the two
// compose — `type=ts glob=*store*` is both filters, not the later one winning.
//
// Deliberately short. This is not ripgrep's 800-entry table; it is the
// languages this project's own map parses (internal/repomap/parse.go) plus the
// configuration and text files a code search actually asks for. An unknown
// name is refused by Execute rather than matching nothing, because a filter
// that quietly matches nothing reports "(no matches)" — the answer that looks
// like knowledge and is not.
var grepTypes = map[string][]string{
	"go":     {".go"},
	"ts":     {".ts", ".tsx", ".mts", ".cts"},
	"js":     {".js", ".jsx", ".mjs", ".cjs"},
	"svelte": {".svelte"},
	"vue":    {".vue"},
	"py":     {".py", ".pyi"},
	"rust":   {".rs"},
	"java":   {".java"},
	"kotlin": {".kt", ".kts"},
	"cs":     {".cs"},
	"c":      {".c", ".h"},
	"cpp":    {".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"},
	"php":    {".php"},
	"ruby":   {".rb"},
	"swift":  {".swift"},
	"dart":   {".dart"},
	"sql":    {".sql"},
	"sh":     {".sh", ".bash", ".zsh"},
	"ps1":    {".ps1", ".psm1"},
	"html":   {".html", ".htm"},
	"css":    {".css", ".scss", ".sass", ".less"},
	"json":   {".json", ".jsonc"},
	"yaml":   {".yaml", ".yml"},
	"toml":   {".toml"},
	"md":     {".md", ".markdown"},
}

// GrepTypeNames lists every accepted type name, sorted, so the schema and the
// refusal message read from the table rather than from a second copy of it.
// Exported for the tool-block tests, which assert what the model is told.
func GrepTypeNames() []string {
	names := make([]string, 0, len(grepTypes))
	for name := range grepTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// matchesType reports whether a file belongs to a named type. An empty name
// means no type filter and matches everything.
func matchesType(name, fileName string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	for _, want := range grepTypes[name] {
		if ext == want {
			return true
		}
	}
	return false
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
				"description": "Go regular expression. Prefix (?i) for case-insensitive.",
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
				"description": "Lines around each match, max 50. Selects show=content.",
			},
			"show": map[string]any{
				"type":        "string",
				"enum":        []string{"content", "files_with_matches", "count"},
				"description": "files_with_matches (default) paths only; content the matching lines; count a per-file tally.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Cap on entries returned.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Skip this many entries first.",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "Only this language's files: go, ts, py, rust and 20 more; an unknown name lists them.",
			},
			"multiline": map[string]any{
				"type":        "boolean",
				"description": "Let the pattern cross line boundaries.",
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
				"Returns the paths of matching files; show=content returns path:line:text instead.",
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
		"Build an edit from what read returns, not from a grep line.\n" +
		"Dependency and build directories are never searched, so a repo-wide search is the repository and not its node_modules. " +
		"In content mode a context line reads path-line-text and a match reads path:line:text, which is how you tell them apart.\n" +
		"limit and offset page the result: a capped result names the offset to continue from.\n" +
		"type=go is how you say \"only this language\" across a whole repository without knowing its layout, and it composes " +
		"with glob rather than replacing it. The names are one per language, not one per extension: ts covers .tsx too.\n" +
		"multiline hands the pattern the whole file at once: . crosses newlines, ^ and $ still mean line edges, and CRLF is " +
		"folded first so a pattern written with a newline works on any checkout. It is the only way to match a struct body or " +
		"a two-line call. Keep it tight: a greedy .* now runs to the end of the file, and one match can be hundreds of lines " +
		"that all count as one hit."
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
	fileType := strings.ToLower(strings.TrimSpace(stringArg(input["type"])))
	multiline := boolArg(input["multiline"])
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
	if fileType != "" && grepTypes[fileType] == nil {
		// Named, not silent. A type nobody recognises would otherwise match no
		// file and answer "(no matches)", which reads as "this string is not in
		// your repository" — a wrong answer with a tick beside it.
		err := errors.New("unknown type " + fileType + "; known types: " + strings.Join(GrepTypeNames(), ", "))
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
	if fileType != "" {
		command += " --type " + fileType
	}
	if multiline {
		command += " --multiline"
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

	// (?s) so . crosses a newline and (?m) so ^ and $ still mean line edges —
	// together they are ripgrep's -U --multiline-dotall, which is the behaviour
	// a caller asking for multiline has in mind. Prefixed rather than wrapped:
	// a pattern that turns a flag back off later still wins, as it should.
	compiled := pattern
	if multiline {
		compiled = "(?sm)" + pattern
	}
	re, err := regexp.Compile(compiled)
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
		if !matchesType(fileType, d.Name()) {
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
		// files_with_matches has its answer for this file after one hit; count
		// and content need every one.
		for _, sp := range grepSpans(re, string(data), lines, multiline, mode == grepModeFiles) {
			matches++
			hits++
			if mode != grepModeContent {
				continue
			}
			if matches <= skip {
				continue // paging past a page already returned
			}
			from, to := sp.from-ctxLines, sp.to+ctxLines
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
				if j >= sp.from && j <= sp.to {
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

	out := newToolOutput("grep", command, output, start, truncated, nil)
	// Matches in content mode, files otherwise — the same unit the mode pages
	// over, so the row's count and the tool's own cap always agree.
	if mode == grepModeContent {
		out.ResultCount = matches
	} else {
		out.ResultCount = len(perFile)
	}
	return out, nil
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

// grepSpan is one hit as a range of line indexes, 0-based and inclusive. A
// line-by-line search always produces from == to; a multiline one may not, and
// that is the whole reason a hit is a range rather than a line number.
type grepSpan struct{ from, to int }

// grepSpans finds every hit in one file.
//
// Two searches behind one shape. Line by line is what grep has always done and
// is byte-for-byte what it was. Multiline hands the pattern the whole file, with
// CRLF folded to LF first: a pattern written with \n in mind must not fail on a
// checked-out Windows file (lineendings.go makes the same promise for edit), and
// folding cannot move a line number, since only the \r is dropped.
//
// firstOnly stops at one hit, for the mode that only needs to know whether the
// file matched at all.
func grepSpans(re *regexp.Regexp, data string, lines []string, multiline, firstOnly bool) []grepSpan {
	var spans []grepSpan
	if !multiline {
		for i, line := range lines {
			if !re.MatchString(line) {
				continue
			}
			spans = append(spans, grepSpan{i, i})
			if firstOnly {
				break
			}
		}
		return spans
	}

	text := strings.ReplaceAll(data, "\r\n", "\n")
	locs := re.FindAllStringIndex(text, -1)
	if firstOnly && len(locs) > 1 {
		locs = locs[:1]
	}
	for _, loc := range locs {
		// The end offset is exclusive, so a match ending exactly on a newline
		// belongs to the line before it, not to the empty one after.
		end := loc[1]
		if end > loc[0] {
			end--
		}
		spans = append(spans, grepSpan{
			from: strings.Count(text[:loc[0]], "\n"),
			to:   strings.Count(text[:end], "\n"),
		})
	}
	return spans
}

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
		"args":      callArgs,
		"glob":      args["glob"],
		"context":   args["context"],
		"show":      args["show"],
		"limit":     args["limit"],
		"offset":    args["offset"],
		"type":      args["type"],
		"multiline": args["multiline"],
	})
}
