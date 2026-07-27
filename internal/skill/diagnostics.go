package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/lsp"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// diagnosticsSkill asks the file's own language server whether the change that
// was just made is valid. Without it the model edits and moves on blind, and
// learns the file is broken several turns later — from the user, after more
// work has been stacked on top of it.
type diagnosticsSkill struct {
	root         string
	outputSubdir func() string
}

// diagnosticsTimeout is generous on purpose: gopls indexes the workspace on
// first use, so the first call in a session is slow and every one after is
// milliseconds. Giving up early would make the feature look broken exactly
// once per session — the time a user forms their opinion of it.
const diagnosticsTimeout = 20 * time.Second

func (*diagnosticsSkill) Name() string { return "diagnostics" }

func (*diagnosticsSkill) Description() string {
	return "Report compiler/linter errors for a file from its language server"
}

func (*diagnosticsSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path of a file, or of a folder to check every supported file inside it.",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "diagnostics",
			Description: "Check for compile/type errors using the language server (gopls, tsserver, ...). " +
				"Give it a file, or a folder to check every supported file inside it — \".\" checks the whole project. " +
				"Call it after editing source files to confirm the change is valid before moving on. " +
				"Returns '(no problems)' when everything checked is clean, and says so when no server is installed for that language.",
			Parameters: payload,
		},
	}
}

func (s *diagnosticsSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		start := time.Now()
		err := errors.New("usage: diagnostics <path>")
		return newToolOutput("diagnostics", "diagnostics", "", start, false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"path": strings.Join(args, " ")})
}

func (s *diagnosticsSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		err := errors.New("path is required")
		return newToolOutput("diagnostics", "diagnostics", "", start, false, err), err
	}
	path = PlacedPath(s.root, s.outputSubdir, path)
	command := "diagnostics " + path

	// Resolved for the sandbox check only — the server is handed the absolute
	// path, and the model keeps seeing the path it asked about.
	resolved, err := resolveSandboxPath(s.root, path)
	if err != nil {
		return newToolOutput("diagnostics", command, "", start, false, err), err
	}
	if info, statErr := os.Stat(resolved); statErr == nil && info.IsDir() {
		return s.diagnoseTree(ctx, path, resolved, command, start)
	}
	// Three distinct answers, never collapsed into one: no server exists for
	// this language, a server exists but could not be obtained, or the file
	// was really checked. Reporting an unchecked file as clean would be worse
	// than reporting nothing at all.
	if !lsp.Configured(path) {
		return newToolOutput("diagnostics", command,
			"(no language server exists for this file type — not checked)", start, false, nil), nil
	}
	if !lsp.Available(ctx, path) {
		return newToolOutput("diagnostics", command,
			"(language server for this file type is not installed and could not be installed — NOT checked)", start, false, nil), nil
	}

	diags, err := lsp.Shared(s.root).Diagnose(ctx, path, diagnosticsTimeout)
	if err != nil {
		return newToolOutput("diagnostics", command, "", start, false, err), err
	}
	if len(diags) == 0 {
		return newToolOutput("diagnostics", command, "(no problems)", start, false, nil), nil
	}
	lines := make([]string, 0, len(diags))
	for _, d := range diags {
		lines = append(lines, d.String())
	}
	out, truncated := limitLines(strings.Join(lines, "\n"), defaultToolOutputLineLimit)
	return newToolOutput("diagnostics", command, out, start, truncated, nil), nil
}

// maxDiagnosticsFiles bounds a folder check. Each file is a round trip to the
// language server, and the tool has 60 seconds before the turn abandons it
// (internal/turn) — a whole-repo sweep that returns nothing is worse than a
// partial one that says how far it got.
const maxDiagnosticsFiles = 40

// diagnoseTree answers "is anything in here broken", which is the question a
// model actually has after an edit that touched four files. Before this it
// could only ask about one file at a time, so it either asked four times or —
// far more often — asked about one and assumed the rest.
func (s *diagnosticsSkill) diagnoseTree(ctx context.Context, shown, absDir, command string, start time.Time) (Output, error) {
	var files []string
	walkErr := filepath.WalkDir(absDir, func(path string, d os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// The same exclusions grep and glob use: node_modules holds more
			// TypeScript than the project does, and none of it is the answer.
			if name := d.Name(); path != absDir && (strings.HasPrefix(name, ".") || IgnoredDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		if lsp.Configured(path) {
			files = append(files, path)
		}
		return nil
	})
	if walkErr != nil && ctx.Err() != nil {
		return newToolOutput("diagnostics", command, "", start, false, walkErr), walkErr
	}
	if len(files) == 0 {
		return newToolOutput("diagnostics", command,
			"(no files with a language server under "+shown+" — nothing checked)", start, false, nil), nil
	}
	// Sorted so two runs over an unchanged tree report in the same order, and
	// so the truncation below cuts predictably rather than by walk order.
	sort.Strings(files)
	capped := false
	if len(files) > maxDiagnosticsFiles {
		files = files[:maxDiagnosticsFiles]
		capped = true
	}

	var (
		lines    []string
		checked  int
		skipped  int
		problems int
	)
	for _, file := range files {
		if ctx.Err() != nil {
			break
		}
		if !lsp.Available(ctx, file) {
			skipped++
			continue
		}
		diags, err := lsp.Shared(s.root).Diagnose(ctx, file, diagnosticsTimeout)
		if err != nil {
			// One unreachable server must not lose the findings from every
			// other file already checked.
			skipped++
			continue
		}
		checked++
		if len(diags) == 0 {
			continue
		}
		rel := file
		if r, relErr := filepath.Rel(absDir, file); relErr == nil {
			rel = filepath.ToSlash(filepath.Join(shown, r))
		}
		lines = append(lines, rel+":")
		for _, d := range diags {
			lines = append(lines, "  "+d.String())
			problems++
		}
	}

	summary := fmt.Sprintf("%d file(s) checked, %d problem(s)", checked, problems)
	if skipped > 0 {
		summary += fmt.Sprintf(", %d skipped (no server available)", skipped)
	}
	if capped {
		summary += fmt.Sprintf(" — stopped at the first %d files, check a subfolder for the rest", maxDiagnosticsFiles)
	}
	if len(lines) == 0 {
		return newToolOutput("diagnostics", command, "(no problems) — "+summary, start, capped, nil), nil
	}
	out, truncated := limitLines(strings.Join(lines, "\n"), defaultToolOutputLineLimit)
	return newToolOutput("diagnostics", command, out+"\n\n"+summary, start, truncated || capped, nil), nil
}
