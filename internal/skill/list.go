package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

type listSkill struct {
	root string
}

func (*listSkill) Name() string { return "list" }

func (*listSkill) Description() string {
	return "List files in a sandbox subpath"
}

func (*listSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path to list, defaults to root.",
			},
		},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "list",
			Description: "List filenames in a sandbox folder.",
			Parameters:  payload,
		},
	}
}

func (s *listSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		return newToolOutput("list", "list", "", start, false, fmt.Errorf("list skill unavailable")), fmt.Errorf("list skill unavailable")
	}

	args := stringSlice(input["args"])
	requestPath := "."
	if len(args) > 0 {
		requestPath = strings.Join(args, " ")
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("list", "list "+requestPath, "", start, false, err), err
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return newToolOutput("list", "list "+requestPath, "", start, false, err), err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	output, truncated := limitLines(strings.Join(names, "\n"), defaultToolOutputLineLimit)
	command := "list"
	if requestPath != "" && requestPath != "." {
		command = "list " + requestPath
	}
	return newToolOutput("list", command, output, start, truncated, nil), nil
}

func (s *listSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	requestPath := "."
	if rawPath, ok := args["path"].(string); ok {
		requestPath = strings.TrimSpace(rawPath)
		if requestPath == "" {
			requestPath = "."
		}
	}
	params := []string{}
	if requestPath != "." {
		params = []string{requestPath}
	}
	return s.Execute(ctx, Input{"args": params})
}

func resolveSandboxPath(root string, requestPath string) (string, error) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		requestPath = "."
	}
	if filepath.IsAbs(requestPath) {
		return "", fmt.Errorf("absolute path is not allowed")
	}

	candidate := filepath.Join(safeRoot, requestPath)
	normalized := filepath.Clean(candidate)
	safeTarget, err := filepath.Abs(normalized)
	if err != nil {
		return "", err
	}

	// A lexical prefix check is not containment: a symlink sitting inside the
	// root and pointing at C:\Users or /etc passes it untouched. Compare the
	// link-resolved forms instead, but still hand back the lexical path so
	// callers and their output keep showing the path the user asked for.
	if !withinRoot(evalExistingSymlinks(safeTarget), evalExistingSymlinks(safeRoot)) {
		return "", fmt.Errorf("path is outside sandbox root")
	}
	return safeTarget, nil
}

// evalExistingSymlinks resolves symlinks on the deepest prefix of path that
// actually exists and re-attaches the rest. The leaf is often missing — write
// and edit create it — and EvalSymlinks fails outright on a missing path.
func evalExistingSymlinks(path string) string {
	rest := ""
	for {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Join(path, rest)
		}
		rest = filepath.Join(filepath.Base(path), rest)
		path = parent
	}
}

// withinRoot compares case-insensitively on Windows: NTFS is case-insensitive,
// so rejecting C:\Work under root c:\work is a false positive, not safety.
func withinRoot(target, root string) bool {
	if runtime.GOOS == "windows" {
		target, root = strings.ToLower(target), strings.ToLower(root)
	}
	sep := string(filepath.Separator)
	return target == root || strings.HasPrefix(target+sep, root+sep)
}
