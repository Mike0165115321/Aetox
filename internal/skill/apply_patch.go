package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// applyPatchSkill applies several edits — across several files — as one call.
//
// The alternative is one edit tool call per change, and every extra call means
// another round trip that re-sends the whole conversation. A refactor touching
// six files costs six of those; here it costs one.
//
// Nothing is written until every edit has been checked: a patch that half
// applies leaves the tree in a state neither the user nor the model predicted,
// and the model's next move would be based on a false picture of the code.
type applyPatchSkill struct {
	root         string
	outputSubdir func() string
}

func (*applyPatchSkill) Name() string { return "apply_patch" }

func (*applyPatchSkill) Description() string {
	return "Apply several exact-string edits across files in one atomic call"
}

func (*applyPatchSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"edits": map[string]any{
				"type":        "array",
				"description": "Edits to apply together, in order. Every one must match, or none are written.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Relative file path",
						},
						"old_string": map[string]any{
							"type":        "string",
							"description": "Exact text to replace. Must appear exactly once in the file (after earlier edits in this same call).",
						},
						"new_string": map[string]any{
							"type":        "string",
							"description": "Replacement text",
						},
					},
					"required":             []string{"path", "old_string", "new_string"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"edits"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "apply_patch",
			Description: "Apply several edits across one or more files in a single atomic call — either all of them " +
				"apply or none do. Prefer this over repeated edit calls when a change touches more than one place. " +
				"read prefixes every line with its number and a tab — strip that prefix before matching, it is not in the file.",
			Parameters: payload,
		},
	}
}

type patchEdit struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (s *applyPatchSkill) Execute(ctx context.Context, input Input) (Output, error) {
	return s.ExecuteTool(ctx, map[string]any{"edits": input["edits"]})
}

func (s *applyPatchSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("apply_patch skill unavailable")
		return newToolOutput("apply_patch", "apply_patch", "", start, false, err), err
	}

	edits, err := parsePatchEdits(args["edits"])
	if err != nil {
		return newToolOutput("apply_patch", "apply_patch", "", start, false, err), err
	}
	command := fmt.Sprintf("apply_patch (%d edits)", len(edits))

	// Stage every file in memory first. Later edits see earlier ones, so two
	// changes to the same file in one patch behave the way they read.
	staged := map[string]string{}    // resolved path -> new content
	requested := map[string]string{} // resolved path -> the path the model used
	var added, removed int

	for i, e := range edits {
		e.Path = PlacedPath(s.root, s.outputSubdir, e.Path)
		targetPath, resolveErr := resolveSandboxPath(s.root, e.Path)
		if resolveErr != nil {
			return newToolOutput("apply_patch", command, "", start, false, resolveErr), resolveErr
		}
		content, staged_ok := staged[targetPath]
		if !staged_ok {
			data, readErr := os.ReadFile(targetPath)
			if readErr != nil {
				err := fmt.Errorf("edit %d (%s): %w", i+1, e.Path, readErr)
				return newToolOutput("apply_patch", command, "", start, false, err), err
			}
			content = string(data)
		}

		// Same resolution as edit, and it matters more here: one invisible `\r`
		// used to cost the whole batch, since a patch that cannot apply in full
		// writes nothing at all (see lineendings.go).
		matchString, count := resolveOldString(content, e.OldString)
		newString := newlinesLike(content, e.NewString)
		switch count {
		case 1:
			// unique match, safe to replace
		case 0:
			err := fmt.Errorf("edit %d (%s): old_string not found; nothing was written — %s", i+1, e.Path, whyNoMatch(content, e.OldString))
			return newToolOutput("apply_patch", command, "", start, false, err), err
		default:
			err := fmt.Errorf("edit %d (%s): old_string matches %d times; nothing was written — add surrounding lines to make it unique", i+1, e.Path, count)
			return newToolOutput("apply_patch", command, "", start, false, err), err
		}

		a, r := LineDelta(e.OldString, e.NewString)
		added += a
		removed += r
		staged[targetPath] = strings.Replace(content, matchString, newString, 1)
		requested[targetPath] = e.Path
	}

	// Every edit checked out; now write. A failure here is a real disk problem,
	// not a bad patch, and is reported with what did land.
	written := make([]string, 0, len(staged))
	for targetPath, content := range staged {
		if writeErr := os.WriteFile(targetPath, []byte(content), 0o644); writeErr != nil {
			err := fmt.Errorf("wrote %d of %d files, then failed on %s: %w",
				len(written), len(staged), requested[targetPath], writeErr)
			return newToolOutput("apply_patch", command, strings.Join(written, "\n"), start, false, err), err
		}
		written = append(written, requested[targetPath])
	}

	out := newToolOutput("apply_patch", command,
		fmt.Sprintf("applied %d edits to %d files: %s", len(edits), len(written), strings.Join(written, ", ")),
		start, false, nil)
	out.LinesAdded, out.LinesRemoved = added, removed
	return out, nil
}

// parsePatchEdits accepts what a model actually sends: the tool layer hands
// over []any of map[string]any, while a hand-written call may pass the typed
// slice directly.
func parsePatchEdits(raw any) ([]patchEdit, error) {
	switch v := raw.(type) {
	case nil:
		return nil, errors.New("edits is required")
	case []patchEdit:
		if len(v) == 0 {
			return nil, errors.New("edits is empty")
		}
		return v, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.New("edits must be an array of {path, old_string, new_string}")
	}
	var edits []patchEdit
	if err := json.Unmarshal(encoded, &edits); err != nil {
		return nil, errors.New("edits must be an array of {path, old_string, new_string}")
	}
	if len(edits) == 0 {
		return nil, errors.New("edits is empty")
	}
	for i, e := range edits {
		if strings.TrimSpace(e.Path) == "" {
			return nil, fmt.Errorf("edit %d: path is required", i+1)
		}
		if e.OldString == "" {
			return nil, fmt.Errorf("edit %d (%s): old_string is required — use write to create a file", i+1, e.Path)
		}
	}
	return edits, nil
}
