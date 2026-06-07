package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aetox-cli/internal/model"
)

type deleteSkill struct {
	root string
}

func (*deleteSkill) Name() string { return "delete" }

func (*deleteSkill) Description() string {
	return "Delete a file under sandbox root"
}

func (*deleteSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative file path to delete",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "delete",
			Description: "Delete a file in sandbox root. Requires user approval through Aetox safety policy.",
			Parameters:  payload,
		},
	}
}

func (s *deleteSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("delete skill unavailable")
		return newToolOutput("delete", "delete", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: delete <path>")
		return newToolOutput("delete", "delete", "", start, false, err), err
	}

	requestPath := strings.TrimSpace(strings.Join(args, " "))
	if requestPath == "" {
		err := errors.New("usage: delete <path>")
		return newToolOutput("delete", "delete", "", start, false, err), err
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
	}
	if info.IsDir() {
		err = errors.New("delete target is a directory")
		return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
	}
	if err := os.Remove(targetPath); err != nil {
		return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
	}

	output := "delete done: " + filepath.ToSlash(targetPath)
	return newToolOutput("delete", "delete "+requestPath, output, start, false, nil), nil
}

func (s *deleteSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		err := errors.New("path is required")
		return newToolOutput("delete", "delete", "", time.Now(), false, err), err
	}
	return s.Execute(ctx, Input{"args": []string{path}})
}
