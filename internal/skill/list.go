package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	if safeTarget != safeRoot && !strings.HasPrefix(safeTarget+string(filepath.Separator), safeRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside sandbox root")
	}
	return safeTarget, nil
}
