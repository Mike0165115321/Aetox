package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/lsp"
	"github.com/Mikedev115/Aetox/internal/model"
)

// symbolSkill answers "what is this, and where does it come from" using the
// language server that is already running for diagnostics.
//
// Without it the model answers that question by searching: grep the name, open
// the files that matched, guess which definition is the one in scope. That
// works and it is expensive, and it gets subtly wrong answers on any codebase
// with two types of the same name — which is most of them, once vendored code
// and test doubles are counted.
//
// It takes a name, not a line and column. A model reads code, not coordinates;
// asking it to count characters invites an off-by-one that returns confidently
// wrong information about the token next door.
type symbolSkill struct {
	root         string
	outputSubdir func() string
}

func (*symbolSkill) Name() string { return "symbol" }

func (*symbolSkill) Description() string {
	return "ถามชนิดและที่ประกาศของชื่อในไฟล์ผ่าน language server"
}

func (*symbolSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path of a file where the name is used",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "The identifier to look up, exactly as written, a function, type, method, variable or field.",
			},
		},
		"required":             []string{"path", "name"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "symbol",
			Description: "Ask the language server what an identifier is: its type or signature, its doc comment, and the file and line where it is declared. " +
				"Faster and more exact than grepping for a definition, and it picks the one actually in scope rather than the first name that matches. " +
				"Needs a server for that language, the same ones diagnostics uses.",
			Parameters: payload,
		},
	}
}

func (s *symbolSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) < 2 {
		err := errors.New("usage: symbol <path> <name>")
		return newToolOutput("symbol", "symbol", "", time.Now(), false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"path": args[0], "name": strings.Join(args[1:], " ")})
}

func (s *symbolSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	path, _ := args["path"].(string)
	name, _ := args["name"].(string)
	path, name = strings.TrimSpace(path), strings.TrimSpace(name)
	if path == "" || name == "" {
		err := errors.New("path and name are required")
		return newToolOutput("symbol", "symbol", "", start, false, err), err
	}
	path = PlacedPath(s.root, s.outputSubdir, path)
	command := "symbol " + path + " " + name

	if _, err := resolveSandboxPath(s.root, path); err != nil {
		return newToolOutput("symbol", command, "", start, false, err), err
	}
	// The same three distinct answers diagnostics gives, for the same reason:
	// "no server for this language" and "the server could not be run" must not
	// be reported as "this symbol does not exist".
	if !lsp.Configured(path) {
		return newToolOutput("symbol", command,
			"(no language server exists for this file type, not checked)", start, false, nil), nil
	}
	if !lsp.Available(ctx, path) {
		return newToolOutput("symbol", command,
			"(language server for this file type is not installed and could not be installed, NOT checked)", start, false, nil), nil
	}

	info, err := lsp.Shared(s.root).Symbol(ctx, path, name, diagnosticsTimeout)
	if err != nil {
		return newToolOutput("symbol", command, "", start, false, err), err
	}
	if info == nil {
		return newToolOutput("symbol", command, "(nothing known about this symbol)", start, false, nil), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s, used at %s:%d\n", name, path, info.Occurrence)
	if info.DefPath != "" {
		fmt.Fprintf(&b, "declared at %s:%d\n", relativeToRoot(s.root, info.DefPath), info.DefLine)
	}
	if info.Hover != "" {
		b.WriteString("\n" + info.Hover)
	}
	out, truncated := limitLines(strings.TrimRight(b.String(), "\n"), defaultToolOutputLineLimit)
	return newToolOutput("symbol", command, out, start, truncated, nil), nil
}

// relativeToRoot shortens a definition path that lies inside the project, and
// leaves one that does not — a declaration in the Go standard library or in a
// module cache is outside the sandbox, and its full path is the useful form.
func relativeToRoot(root, path string) string {
	base, err := resolveSandboxPath(root, ".")
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.ToSlash(rel)
}
