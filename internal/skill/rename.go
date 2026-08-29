package skill

// The workspace rename every IDE has and almost no agent does: one symbol,
// every declaration and use, changed together by the language server's own
// understanding — where a grep-and-replace of the same name would also hit
// the OTHER thing that happens to share it.
//
// The server proposes, this tool disposes: lsp.Rename returns the edit and
// touches nothing, because which files may change is the sandbox's law and
// the sandbox lives here. Every touched file passes resolveSandboxPath; an
// edit the server proposes OUTSIDE the sandbox (a rename that would reach
// into the module cache, say) refuses the whole call rather than applying
// half a rename — a symbol renamed in some of its homes is a broken build
// with extra steps.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Mikedev115/Aetox/internal/lsp"
	"github.com/Mikedev115/Aetox/internal/model"
)

type renameSkill struct {
	root         string
	outputSubdir func() string
	files        *FileState
}

func (*renameSkill) Name() string { return "rename" }

func (*renameSkill) Description() string {
	return "เปลี่ยนชื่อ symbol ทั้งโปรเจกต์ผ่าน language server"
}

func (*renameSkill) ToolDefinition() model.ToolDefinition {
	// Bare types, no per-parameter prose: path/name/new_name explain
	// themselves, the judgment lives in Guidance(), and the 80-token ceiling
	// (block standard) is exactly three self-evident parameters wide.
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":     map[string]any{"type": "string"},
			"name":     map[string]any{"type": "string"},
			"new_name": map[string]any{"type": "string"},
		},
		"required":             []string{"path", "name", "new_name"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "rename",
			Description: "Rename a symbol project-wide via the language server",
			Parameters:  payload,
		},
	}
}

func (*renameSkill) Guidance(map[string]any) string {
	return "Reach for this instead of edit all=true whenever the change IS a rename: " +
		"the server changes every real use and never a string or a comment that happens to match. " +
		"symbol (references) first tells you how big the blast radius is. " +
		"Needs that language's server installed, same as diagnostics."
}

func (s *renameSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) < 3 {
		err := errors.New("usage: rename <path> <name> <new_name>")
		return newToolOutput("rename", "rename", "", time.Now(), false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"path": args[0], "name": args[1], "new_name": args[2]})
}

func (s *renameSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	path, _ := args["path"].(string)
	name, _ := args["name"].(string)
	newName, _ := args["new_name"].(string)
	path, name, newName = strings.TrimSpace(path), strings.TrimSpace(name), strings.TrimSpace(newName)
	if path == "" || name == "" || newName == "" {
		err := errors.New("path, name and new_name are required")
		return newToolOutput("rename", "rename", "", start, false, err), err
	}
	path = PlacedPath(s.root, s.outputSubdir, path)
	command := fmt.Sprintf("rename %s %s -> %s", path, name, newName)

	if _, err := resolveSandboxPath(s.root, path); err != nil {
		return newToolOutput("rename", command, "", start, false, err), err
	}
	if !lsp.Configured(path) {
		return newToolOutput("rename", command,
			"(no language server exists for this file type, nothing renamed)", start, false, nil), nil
	}
	if !lsp.Available(ctx, path) {
		return newToolOutput("rename", command,
			"(language server for this file type is not installed and could not be installed, nothing renamed)", start, false, nil), nil
	}

	edits, err := lsp.Shared(s.root).Rename(ctx, path, name, newName, diagnosticsTimeout)
	if err != nil {
		return newToolOutput("rename", command, "", start, false, err), err
	}

	// Authorize every file BEFORE touching any. Half a rename is worse than
	// none: the build breaks either way, but half leaves the tree lying about
	// which name is current.
	type job struct {
		abs, rel string
		edits    []lsp.TextEdit
	}
	jobs := make([]job, 0, len(edits))
	for abs, es := range edits {
		rel := relativeToRoot(s.root, abs)
		if _, err := resolveSandboxPath(s.root, rel); err != nil {
			err := fmt.Errorf("the rename would change %s, which is outside this workspace; nothing was written", abs)
			return newToolOutput("rename", command, "", start, false, err), err
		}
		jobs = append(jobs, job{abs: abs, rel: rel, edits: es})
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].rel < jobs[j].rel })

	totalAdded, totalRemoved, occurrences := 0, 0, 0
	changed := make([]string, 0, len(jobs))
	var diffs []string
	for _, j := range jobs {
		before, err := os.ReadFile(j.abs)
		if err != nil {
			return newToolOutput("rename", command, "", start, false, err), err
		}
		after := applyTextEdits(string(before), j.edits)
		if after == string(before) {
			continue
		}
		if err := os.WriteFile(j.abs, []byte(after), 0o644); err != nil {
			err = fmt.Errorf("renamed %d of %d files, then failed on %s: %w", len(changed), len(jobs), j.rel, err)
			return newToolOutput("rename", command, strings.Join(changed, "\n"), start, false, err), err
		}
		s.files.Note(j.abs)
		occurrences += len(j.edits)
		a, r := LineDelta(string(before), after)
		totalAdded += a
		totalRemoved += r
		changed = append(changed, j.rel)
		diffs = append(diffs, FileDiff(j.rel, string(before), after))
	}
	if len(changed) == 0 {
		return newToolOutput("rename", command, "(the server's edit changed nothing)", start, false, nil), nil
	}
	out := newToolOutput("rename", command,
		fmt.Sprintf("renamed %s -> %s: %d occurrence(s) across %d file(s): %s",
			name, newName, occurrences, len(changed), strings.Join(changed, ", ")),
		start, false, nil)
	out.LinesAdded, out.LinesRemoved = totalAdded, totalRemoved
	out.Diff = JoinDiffs(diffs)
	// The self-check, on the file the rename was aimed from — one file stands
	// for the change; if the rename broke the build, it broke it here too.
	return appendFreshDiagnostics(ctx, s.root, path, out), nil
}

// applyTextEdits applies LSP edits to content. Positions arrive as 0-based
// lines and UTF-16 code units — converted here and nowhere else, because a
// byte-offset shortcut is invisible on ASCII identifiers and silently corrupts
// the first file with a Thai comment on the edited line.
func applyTextEdits(content string, edits []lsp.TextEdit) string {
	lines := strings.SplitAfter(content, "\n")
	offsets := make([]int, len(lines)+1)
	for i, l := range lines {
		offsets[i+1] = offsets[i] + len(l)
	}
	byteAt := func(line, u16 int) int {
		if line >= len(lines) {
			return len(content)
		}
		l := strings.TrimRight(lines[line], "\n")
		l = strings.TrimRight(l, "\r")
		units := utf16.Encode([]rune(l))
		if u16 > len(units) {
			u16 = len(units)
		}
		return offsets[line] + len(string(utf16.Decode(units[:u16])))
	}
	// Applied back to front so earlier offsets stay true as later text moves.
	sorted := append([]lsp.TextEdit(nil), edits...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].StartLine != sorted[j].StartLine {
			return sorted[i].StartLine > sorted[j].StartLine
		}
		return sorted[i].StartChar > sorted[j].StartChar
	})
	for _, e := range sorted {
		from := byteAt(e.StartLine, e.StartChar)
		to := byteAt(e.EndLine, e.EndChar)
		if from > len(content) || to > len(content) || from > to {
			continue
		}
		content = content[:from] + e.NewText + content[to:]
	}
	return content
}
