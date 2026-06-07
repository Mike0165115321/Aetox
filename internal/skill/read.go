package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"aetox-cli/internal/model"
)

type readSkill struct {
	root string
}

func (*readSkill) Name() string { return "read" }

func (*readSkill) Description() string {
	return "Read a text file under sandbox root"
}

func (*readSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative file path to read",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "read",
			Description: "Read a text file in sandbox root.",
			Parameters:  payload,
		},
	}
}

func (s *readSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("read skill unavailable")
		return newToolOutput("read", "read", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: read <path>")
		return newToolOutput("read", "read", "", start, false, err), err
	}

	requestPath := strings.TrimSpace(strings.Join(args, " "))
	if requestPath == "" {
		err := errors.New("usage: read <path>")
		return newToolOutput("read", "read", "", start, false, err), err
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("read", "read "+requestPath, "", start, false, err), err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return newToolOutput("read", "read "+requestPath, "", start, false, err), err
	}
	if info.IsDir() {
		err = errors.New("read target is a directory")
		return newToolOutput("read", "read "+requestPath, "", start, false, err), err
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return newToolOutput("read", "read "+requestPath, "", start, false, err), err
	}
	defer func() {
		_ = file.Close()
	}()

	const maxBytes = 16384
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return newToolOutput("read", "read "+requestPath, "", start, false, err), err
	}
	if bytes.Contains(data, []byte{0}) {
		return newToolOutput("read", "read "+requestPath, "(binary file)", start, false, nil), nil
	}

	truncated := false
	if len(data) > maxBytes {
		data = data[:maxBytes]
		truncated = true
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		content = "(empty file)"
	}
	if truncated {
		content += "\n... (truncated)"
	}
	return newToolOutput("read", "read "+requestPath, content, start, truncated, nil), nil
}

func (s *readSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		err := errors.New("path is required")
		return newToolOutput("read", "read", "", time.Now(), false, err), err
	}
	return s.Execute(ctx, Input{"args": []string{path}})
}
