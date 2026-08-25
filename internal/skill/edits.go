package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
)

// editsSkill applies several edits — across several files — as one call.
//
// The alternative is one edit tool call per change, and every extra call means
// another round trip that re-sends the whole conversation. A refactor touching
// six files costs six of those; here it costs one.
//
// Nothing is written until every edit has been checked: a call that half
// applies leaves the tree in a state neither the user nor the model predicted,
// and the model's next move would be based on a false picture of the code.
type editsSkill struct {
	root         string
	outputSubdir func() string
	// files is the shared record a write checks and every toucher updates.
	// Nil is supported and means no guard (filestate.go).
	files *FileState
}

func (*editsSkill) Name() string { return "edits" }

func (*editsSkill) Description() string {
	return "Apply several exact-string edits across files in one atomic call"
}

func (*editsSkill) ToolDefinition() model.ToolDefinition {
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
						"find": map[string]any{
							"type":        "string",
							"description": "Exact text to find. Must appear exactly once in the file (after earlier edits in this same call).",
						},
						"replace": map[string]any{
							"type":        "string",
							"description": "Text to put in its place",
						},
					},
					// path is no longer required per edit: the call may name it
					// once at the top level instead. A strict provider rejects
					// what this list demands, so leaving it here would refuse
					// the very shape the top-level default exists to accept.
					"required":             []string{"find", "replace"},
					"additionalProperties": false,
				},
			},
			// Named once for the whole call, for the commonest shape there is:
			// several edits to one file. Without it in the schema a provider
			// that enforces additionalProperties refuses the call outright, so
			// the forgiving read in parseEditItems would never be reached.
			"path": map[string]any{
				"type":        "string",
				"description": "Default file for edits that omit one",
			},
		},
		"required":             []string{"edits"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "edits",
			Description: "Apply several edits across one or more files in a single atomic call — either all of them " +
				"apply or none do. Prefer this over repeated edit calls when a change touches more than one place.",
			Parameters: payload,
		},
	}
}

type editItem struct {
	Path    string `json:"path"`
	Find    string `json:"find"`
	Replace string `json:"replace"`
}

func (s *editsSkill) Execute(ctx context.Context, input Input) (Output, error) {
	return s.ExecuteTool(ctx, map[string]any{"edits": input["edits"]})
}

func (s *editsSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("edits skill unavailable")
		return newToolOutput("edits", "edits", "", start, false, err), err
	}

	// The top-level path is a default, not a second way to say the same thing:
	// several edits to ONE file is the commonest shape there is, and a model
	// that has just named the file does not expect to name it again on every
	// item. It sent `{"path": "...", "edits": [{find, replace}, ...]}`
	// and got "edit 1: path is required" — a refusal about shape, for a call
	// whose meaning was never in doubt (owner's log, 24 ส.ค. 03:16).
	edits, err := parseEditItems(args["edits"], stringArg(args["path"]))
	if err != nil {
		return newToolOutput("edits", "edits", "", start, false, err), err
	}
	command := fmt.Sprintf("edits (%d edits)", len(edits))

	// Stage every file in memory first. Later edits see earlier ones, so two
	// changes to the same file in one call behave the way they read.
	staged := map[string]string{}    // resolved path -> new content
	original := map[string]string{}  // resolved path -> what was there before this call
	requested := map[string]string{} // resolved path -> the path the model used
	order := []string{}              // resolved paths, first-touched first
	var added, removed int

	for i, e := range edits {
		e.Path = PlacedPath(s.root, s.outputSubdir, e.Path)
		targetPath, resolveErr := resolveSandboxPath(s.root, e.Path)
		if resolveErr != nil {
			return newToolOutput("edits", command, "", start, false, resolveErr), resolveErr
		}
		content, staged_ok := staged[targetPath]
		if !staged_ok {
			data, readErr := os.ReadFile(targetPath)
			if readErr != nil {
				err := fmt.Errorf("edit %d (%s): %w", i+1, e.Path, readErr)
				return newToolOutput("edits", command, "", start, false, err), err
			}
			content = string(data)
			// Kept for the diff: a second edit to the same file must be
			// measured against what was on disk when the call started, not
			// against the first edit's result. `order` exists for the same
			// reason a diff is worth showing at all — a map would hand the
			// reader the files in a different sequence every run.
			original[targetPath] = content
			order = append(order, targetPath)
		}

		// Same resolution as edit, and it matters more here: one invisible `\r`
		// used to cost the whole batch, since a call that cannot apply in full
		// writes nothing at all (see lineendings.go).
		matchString, count := resolveFindText(content, e.Find)
		replaceText := newlinesLike(content, e.Replace)
		switch count {
		case 1:
			// unique match, safe to replace
		case 0:
			err := fmt.Errorf("edit %d (%s): find text not found; nothing was written — %s", i+1, e.Path, whyNoMatch(content, e.Find))
			return newToolOutput("edits", command, "", start, false, err), err
		default:
			err := fmt.Errorf("edit %d (%s): find text matches %d times; nothing was written — add surrounding lines to make it unique", i+1, e.Path, count)
			return newToolOutput("edits", command, "", start, false, err), err
		}

		a, r := LineDelta(e.Find, e.Replace)
		added += a
		removed += r
		staged[targetPath] = strings.Replace(content, matchString, replaceText, 1)
		requested[targetPath] = e.Path
	}

	// Every edit checked out; now write. A failure here is a real disk problem,
	// not a bad set of edits, and is reported with what did land.
	written := make([]string, 0, len(staged))
	for targetPath, content := range staged {
		if writeErr := os.WriteFile(targetPath, []byte(content), 0o644); writeErr != nil {
			err := fmt.Errorf("wrote %d of %d files, then failed on %s: %w",
				len(written), len(staged), requested[targetPath], writeErr)
			return newToolOutput("edits", command, strings.Join(written, "\n"), start, false, err), err
		}
		s.files.Note(targetPath)
		written = append(written, requested[targetPath])
	}

	out := newToolOutput("edits", command,
		fmt.Sprintf("applied %d edits to %d files: %s", len(edits), len(written), strings.Join(written, ", ")),
		start, false, nil)
	out.LinesAdded, out.LinesRemoved = added, removed
	// One diff for the whole call, each file named — this is the only writer
	// that can change several at once, so it is the only one whose hunks need
	// to say which file they are in.
	diffs := make([]string, 0, len(order))
	for _, targetPath := range order {
		diffs = append(diffs, FileDiff(requested[targetPath], original[targetPath], staged[targetPath]))
	}
	out.Diff = JoinDiffs(diffs)
	return out, nil
}

// parseEditItems accepts what a model actually sends: the tool layer hands
// over []any of map[string]any, while a hand-written call may pass the typed
// slice directly.
// fallbackPath fills in for edits that name no file of their own. Empty when
// the call named none either, which is still the error it always was.
func parseEditItems(raw any, fallbackPath string) ([]editItem, error) {
	switch v := raw.(type) {
	case nil:
		return nil, errors.New("edits is required")
	case []editItem:
		if len(v) == 0 {
			return nil, errors.New("edits is empty")
		}
		return v, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.New("edits must be an array of {path, find, replace}")
	}
	var edits []editItem
	if err := json.Unmarshal(encoded, &edits); err != nil {
		return nil, errors.New("edits must be an array of {path, find, replace}")
	}
	if len(edits) == 0 {
		return nil, errors.New("edits is empty")
	}
	fallbackPath = strings.TrimSpace(fallbackPath)
	for i := range edits {
		if strings.TrimSpace(edits[i].Path) == "" {
			edits[i].Path = fallbackPath
		}
		e := edits[i]
		if strings.TrimSpace(e.Path) == "" {
			return nil, fmt.Errorf("edit %d: path is required — name the file on the edit, or once at the top level for all of them", i+1)
		}
		if e.Find == "" {
			return nil, fmt.Errorf("edit %d (%s): find text is required — use write to create a file", i+1, e.Path)
		}
	}
	return edits, nil
}

// Guidance is what edits used to say in the tool block on every request.
//
// The line-number prefix belongs here rather than there by the standard in
// guidance.go: it is a thing to watch out for, not part of what the tool is or
// what to pass it. `edit` says the same thing in its own block entry, which is
// where a model that only ever edits one file at a time meets it.
func (*editsSkill) Guidance(map[string]any) string {
	return "read prefixes every line with its number and a tab — strip that prefix before matching, it is not in the file.\n" +
		"Every edit must match exactly once, and if any one of them does not, NOTHING is written: the call is atomic on purpose, so a half-applied change cannot exist. When one fails, the report names which edit and why.\n" +
		"Several edits to one file is the ordinary case: name the file once at the top level and leave `path` off the edits."
}
