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

// editMaxFileBytes is generous enough that no source file, lockfile or config
// ever hits it, and small enough that four in-memory copies stay harmless.
const editMaxFileBytes = 16 << 20 // 16 MiB

type editSkill struct {
	root         string
	outputSubdir func() string
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
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace every occurrence instead of requiring exactly one. This is how you rename a symbol throughout a file in one call.",
			},
		},
		"required":             []string{"path", "old_string", "new_string"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "edit",
			Description: "Replace an exact string in an existing file. Safer than write for changing part of a file. " +
				"The text must appear exactly once unless replace_all is set. " +
				"read prefixes every line with its number and a tab — strip that prefix before matching, it is not in the file.",
			Parameters: payload,
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

	requestPath := PlacedPath(s.root, s.outputSubdir, strings.TrimSpace(args[0]))
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

	// Exact search-and-replace needs the whole file, and between `data`, the
	// string conversion, the Replace result and the write-back that is four
	// copies live at once — a few hundred MB of generated log or lockfile is
	// enough to take a desktop app down. Refuse early and say what to do
	// instead, rather than OOM mid-edit.
	if info, statErr := os.Stat(targetPath); statErr == nil && info.Size() > editMaxFileBytes {
		err = fmt.Errorf("file is %d MB, too large to edit safely (limit %d MB) — narrow the change with shell tools instead", info.Size()>>20, int64(editMaxFileBytes)>>20)
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

	replaceAll, _ := input["replace_all"].(bool)

	content := string(data)
	// What the file actually holds for what the caller asked for, and the
	// replacement in the file's own line endings. See lineendings.go: on the
	// reference platform every checked-out file is CRLF and a model cannot see
	// a `\r`, so an exact-only match failed every multi-line edit and told the
	// model to re-read, which showed it the same invisible character again.
	matchString, count := resolveOldString(content, oldString)
	newString = newlinesLike(content, newString)
	switch {
	case count == 0:
		err = fmt.Errorf("old_string not found in file — %s", whyNoMatch(content, oldString))
		return newToolOutput("edit", command, "", start, false, err), err
	case count > 1 && !replaceAll:
		// Still the default, and still the safer one: a model that meant to
		// change one call site and matched eight has made a mistake worth
		// stopping, and replace_all is how it says it meant all eight.
		err = fmt.Errorf("old_string matches %d times; add surrounding lines to make it unique, or set replace_all to change all %d", count, count)
		return newToolOutput("edit", command, "", start, false, err), err
	}

	replacements := 1
	if replaceAll {
		replacements = count
	}
	updated := strings.Replace(content, matchString, newString, replacements)
	if err := os.WriteFile(targetPath, []byte(updated), 0o644); err != nil {
		return newToolOutput("edit", command, "", start, false, err), err
	}

	result := "edit done: " + requestPath
	if replacements > 1 {
		result = fmt.Sprintf("edit done: %s (%d occurrences)", requestPath, replacements)
	}
	out := newToolOutput("edit", command, result, start, false, nil)
	// The two strings are the whole change, once per occurrence replaced.
	out.LinesAdded, out.LinesRemoved = LineDelta(oldString, newString)
	out.LinesAdded *= replacements
	out.LinesRemoved *= replacements
	return out, nil
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
	return s.Execute(ctx, Input{
		"args":        []string{path, oldString, newString},
		"replace_all": args["replace_all"],
	})
}
