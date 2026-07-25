package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

type writeSkill struct {
	root         string
	outputSubdir func() string
}

// placed decides where a new file actually lands. Without a project focused,
// the sandbox root is the user's home directory, so "write index.html" dropped
// a stray file next to Documents and Downloads — and the next chat that wrote
// index.html silently overwrote it. A relative path is therefore steered into
// the session's own output folder; an absolute one is an explicit destination
// and is left exactly where the caller asked for it.
//
// The returned path is what gets echoed back to the model, so a later read or
// edit of the same relative path finds the file where it was actually written.
func (s *writeSkill) placed(requestPath string) string {
	if s.outputSubdir == nil || filepath.IsAbs(requestPath) {
		return requestPath
	}
	subdir := strings.TrimSpace(s.outputSubdir())
	if subdir == "" {
		return requestPath
	}
	return filepath.ToSlash(filepath.Join(subdir, requestPath))
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
	requestPath = s.placed(requestPath)

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	if err := ensureWriteDir(targetPath); err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	// Read the outgoing version before clobbering it, so the timeline can say
	// whether this was a new file or a rewrite of N lines. A failed read means
	// "no file there" — a brand new file, nothing removed.
	previous, _ := os.ReadFile(targetPath)

	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	// Echo the path the caller asked for, like edit does — the resolved
	// absolute path is noise in context and nudges the model into repeating
	// the sandbox root back at the user (see internal/prompt environment()).
	output := "write done: " + requestPath
	out := newToolOutput("write", "write "+requestPath, output, start, false, nil)
	out.LinesAdded, out.LinesRemoved = LineDelta(string(previous), content)
	return out, nil
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
