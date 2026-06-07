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

type writeSkill struct {
	root string
}

func (*writeSkill) Name() string { return "write" }

func (*writeSkill) Description() string {
	return "Create or overwrite a file under sandbox root"
}

func (*writeSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative destination path",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "File content",
			},
		},
		"required":             []string{"path", "content"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "write",
			Description: "Write content to a file in sandbox root.",
			Parameters:  payload,
		},
	}
}

func (s *writeSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("write skill unavailable")
		return newToolOutput("write", "write", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) < 2 {
		err := errors.New("usage: write <path> <content>")
		return newToolOutput("write", "write", "", start, false, err), err
	}

	requestPath := strings.TrimSpace(args[0])
	content := strings.Join(args[1:], " ")
	if requestPath == "" {
		err := errors.New("usage: write <path> <content>")
		return newToolOutput("write", "write "+strings.TrimSpace(strings.Join(args, " ")), "", start, false, err), err
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	if err := ensureWriteDir(targetPath); err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	output := "write done: " + filepath.ToSlash(targetPath)
	return newToolOutput("write", "write "+requestPath, output, start, false, nil), nil
}

func (s *writeSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	if s == nil {
		err := errors.New("write skill unavailable")
		return newToolOutput("write", "write", "", time.Now(), false, err), err
	}

	path, pathOK := args["path"].(string)
	content, contentOK := args["content"].(string)
	if !pathOK || strings.TrimSpace(path) == "" {
		err := errors.New("path is required")
		return newToolOutput("write", "write", "", time.Now(), false, err), err
	}
	if !contentOK {
		content = ""
	}
	return s.Execute(ctx, Input{"args": []string{path, content}})
}

func ensureWriteDir(targetPath string) error {
	dir := filepath.Dir(targetPath)
	if dir == "." {
		return nil
	}
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return errors.New("parent path is not a directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}
