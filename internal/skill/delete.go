package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
)

type deleteSkill struct {
	root         string
	outputSubdir func() string
	// files is the shared record a write checks and every toucher updates.
	// Nil is supported and means no guard (filestate.go).
	files *FileState
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
				"description": "Relative path of the file or folder to delete",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "Required to delete a folder, and it takes everything inside. Naming a folder without this is refused, because \"delete\" of a path that turned out to be a directory is the one mistake with no undo at the tool level.",
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
			Description: "Delete a file in sandbox root, or a folder and its contents with recursive. Requires user approval through Aetox safety policy.",
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

	requestPath = PlacedPath(s.root, s.outputSubdir, requestPath)
	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
	}
	recursive, _ := input["recursive"].(bool)
	if info.IsDir() {
		if !recursive {
			// Named rather than assumed: a model that thought it was deleting
			// one generated file and passed a folder path takes the whole
			// folder with it, and there is no smaller mistake to make here.
			err = errors.New("delete target is a directory — pass recursive to remove it and everything inside")
			return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
		}
		// The sandbox root itself is not a thing to delete: it is the project.
		if root, rootErr := resolveSandboxPath(s.root, "."); rootErr == nil && targetPath == root {
			err = errors.New("refusing to delete the sandbox root")
			return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
		}
		if err := os.RemoveAll(targetPath); err != nil {
			return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
		}
		s.files.Forget(targetPath)
		output := "delete done: " + requestPath + "/ (folder and contents)"
		return newToolOutput("delete", "delete "+requestPath, output, start, false, nil), nil
	}
	if err := os.Remove(targetPath); err != nil {
		return newToolOutput("delete", "delete "+requestPath, "", start, false, err), err
	}
	// Forgotten, not noted: writing this name again is a creation, and holding
	// the dead file's stamp would refuse it (filestate.go).
	s.files.Forget(targetPath)

	// Relative, matching edit and write — see write.go for why.
	output := "delete done: " + requestPath
	return newToolOutput("delete", "delete "+requestPath, output, start, false, nil), nil
}

func (s *deleteSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		err := errors.New("path is required")
		return newToolOutput("delete", "delete", "", time.Now(), false, err), err
	}
	return s.Execute(ctx, Input{"args": []string{path}, "recursive": args["recursive"]})
}
