package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

type editSkill struct {
	root string
}

func (*editSkill) Name() string { return "edit" }

func (*editSkill) Description() string {
	return "Replace an exact string in a file under sandbox root"
}

func (*editSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative file path to edit",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact text to replace. Must appear exactly once in the file; include surrounding lines to make it unique.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement text. Empty string deletes old_string.",
			},
		},
		"required":             []string{"path", "old_string", "new_string"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "edit",
			Description: "Replace an exact, unique string in an existing file. Safer than write for changing part of a file.",
			Parameters:  payload,
		},
	}
}

func (s *editSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("edit skill unavailable")
		return newToolOutput("edit", "edit", "", start, false, err), err
	}

	// raw []string on purpose: stringSlice trims and drops empty items, which
	// would corrupt whitespace-significant old_string/new_string.
	args, _ := input["args"].([]string)
	if len(args) != 3 {
		err := errors.New("usage: edit <path> <old_string> <new_string>")
		return newToolOutput("edit", "edit", "", start, false, err), err
	}

	requestPath := strings.TrimSpace(args[0])
	oldString := args[1]
	newString := args[2]
	command := "edit " + requestPath

	if requestPath == "" {
		err := errors.New("usage: edit <path> <old_string> <new_string>")
		return newToolOutput("edit", command, "", start, false, err), err
	}
	if oldString == "" {
		err := errors.New("old_string is empty; use write to create a file")
		return newToolOutput("edit", command, "", start, false, err), err
	}
	if oldString == newString {
		err := errors.New("old_string and new_string are identical")
		return newToolOutput("edit", command, "", start, false, err), err
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("edit", command, "", start, false, err), err
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return newToolOutput("edit", command, "", start, false, err), err
	}
	if bytes.Contains(data, []byte{0}) {
		err = errors.New("edit target is a binary file")
		return newToolOutput("edit", command, "", start, false, err), err
	}

	content := string(data)
	switch count := strings.Count(content, oldString); count {
	case 0:
		err = errors.New("old_string not found in file; re-read the file and match the text exactly")
		return newToolOutput("edit", command, "", start, false, err), err
	case 1:
		// unique match, safe to replace
	default:
		err = fmt.Errorf("old_string matches %d times; add surrounding lines to make it unique", count)
		return newToolOutput("edit", command, "", start, false, err), err
	}

	updated := strings.Replace(content, oldString, newString, 1)
	if err := os.WriteFile(targetPath, []byte(updated), 0o644); err != nil {
		return newToolOutput("edit", command, "", start, false, err), err
	}

	return newToolOutput("edit", command, "edit done: "+requestPath, start, false, nil), nil
}

func (s *editSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	if s == nil {
		err := errors.New("edit skill unavailable")
		return newToolOutput("edit", "edit", "", time.Now(), false, err), err
	}

	path, pathOK := args["path"].(string)
	oldString, oldOK := args["old_string"].(string)
	newString, _ := args["new_string"].(string)
	if !pathOK || strings.TrimSpace(path) == "" {
		err := errors.New("path is required")
		return newToolOutput("edit", "edit", "", time.Now(), false, err), err
	}
	if !oldOK || oldString == "" {
		err := errors.New("old_string is required")
		return newToolOutput("edit", "edit "+path, "", time.Now(), false, err), err
	}
	return s.Execute(ctx, Input{"args": []string{path, oldString, newString}})
}
