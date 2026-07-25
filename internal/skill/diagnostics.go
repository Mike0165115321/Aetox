package skill

import (
	"context"
	"encoding/json"
	"errors"
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
				"description": "Relative path of the file to check",
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
			Description: "Check a file for compile/type errors using its language server (gopls, tsserver, ...). " +
				"Call it after editing source files to confirm the change is valid before moving on. " +
				"Returns '(no problems)' when the file is clean and nothing when no server is installed for that language.",
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
	path = placedFallback(s.root, s.outputSubdir, path)
	command := "diagnostics " + path

	// Resolved for the sandbox check only — the server is handed the absolute
	// path, and the model keeps seeing the path it asked about.
	if _, err := resolveSandboxPath(s.root, path); err != nil {
		return newToolOutput("diagnostics", command, "", start, false, err), err
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
