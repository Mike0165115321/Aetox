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
	// files is the shared record a write checks and every toucher updates.
	// Nil is supported and means no guard (filestate.go).
	files *FileState
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
				"description": "Exact text to replace, unique in the file. Unused by mode=append.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement text, or the text to add in mode=append. Empty deletes old_string.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace every occurrence instead of requiring exactly one.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"replace", "append"},
				"description": "replace (default), or append to add new_string at the end of the file.",
			},
		},
		"required":             []string{"path", "new_string"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "edit",
			Description: "Change part of an existing file: replace an exact string, or append to its end.",
			Parameters:  payload,
		},
	}
}

// Guidance carries what the signature above deliberately does not. The block
// entry was already over the standard before append was added, so the way to
// pay for a new parameter was to move the prose that was never needed twice.
func (*editSkill) Guidance(map[string]any) string {
	return "edit changes part of a file; write re-sends all of it. On anything large edit is the cheaper act, " +
		"and the only one that survives an output token limit: a write whose content does not fit the round " +
		"is cut mid-JSON and cannot run at all.\n" +
		"mode=append carries on a file that was cut off. Send only what follows the last byte already on disk. " +
		"No separator is added for you, so a file cut mid-word is continued mid-word. An append carries at " +
		"most 300 lines, the same cap write has, so a long continuation is several appends. Replacing is " +
		"not capped: one substitution cannot be split in half.\n" +
		"read prefixes every line with its number and a tab. Strip that prefix before matching: it is not in the file. " +
		"Line endings are matched for you, so a failed match is about the text, not invisible characters, and " +
		"re-reading will show the same bytes again."
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

	// Appending is the same skill because it is the same act: changing part of
	// a file without re-sending the rest. It earns its place on the truncation
	// path — a write cut off at the output ceiling leaves a file that needs
	// carrying on, and the alternative was making the model quote the tail
	// back as old_string, which is the very content the limit already proved
	// it cannot afford to repeat.
	appendMode := strings.EqualFold(strings.TrimSpace(stringArg(input["mode"])), "append")
	switch {
	case appendMode && oldString != "":
		err := errors.New("mode=append adds to the end of the file, so it takes no old_string; drop it, or use the default replace mode")
		return newToolOutput("edit", command, "", start, false, err), err
	case appendMode && newString == "":
		err := errors.New("new_string is empty; mode=append needs the text to add")
		return newToolOutput("edit", command, "", start, false, err), err
	case !appendMode && oldString == "":
		err := errors.New("old_string is empty; use mode=append to add to the end of the file, or write to create one")
		return newToolOutput("edit", command, "", start, false, err), err
	case !appendMode && oldString == newString:
		err := errors.New("old_string and new_string are identical")
		return newToolOutput("edit", command, "", start, false, err), err
	}

	// Append is capped, replace is not, and the difference is whether the act
	// can be split at all. An append is a file being written a piece at a time,
	// so capping it only decides how many pieces; refusing it costs nothing but
	// one more call. A replace is one substitution — a 400-line block swapped
	// for another — and there is no honest way to cut that in half. Capping it
	// would refuse correct work and offer no way to do it.
	//
	// The hole that leaves is narrow: replace cannot create a file, only change
	// one that exists, and a replace too large for the round is refused by the
	// truncation guard with the file untouched. What the cap is protecting
	// against is spending a whole round's output on something that cannot run,
	// and append is the door that would otherwise take the traffic write no
	// longer accepts.
	if appendMode {
		if err := checkContentLineCap("new_string", newString); err != nil {
			return newToolOutput("edit", command, "", start, false, err), err
		}
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

	// Appended verbatim, with only the file's own line endings applied. No
	// separator is inserted: the caller is continuing a file from the exact
	// byte it stopped at, and a newline we add on its behalf lands inside the
	// content rather than between two of its parts.
	if appendMode {
		addition := newlinesLike(content, newString)
		updated := content + addition
		if err := os.WriteFile(targetPath, []byte(updated), 0o644); err != nil {
			return newToolOutput("edit", command, "", start, false, err), err
		}
		// This app has now seen the file, so a later whole-file write can be
		// held to it (filestate.go). edit needs no guard of its own: an
		// old_string aimed at text somebody has changed simply will not match.
		s.files.Note(targetPath)
		out := newToolOutput("edit", command, "edit done: appended to "+requestPath, start, false, nil)
		out.LinesAdded, _ = LineDelta("", addition)
		out.Diff = UnifiedDiff(content, updated)
		return out, nil
	}
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
	s.files.Note(targetPath)

	result := "edit done: " + requestPath
	if replacements > 1 {
		result = fmt.Sprintf("edit done: %s (%d occurrences)", requestPath, replacements)
	}
	out := newToolOutput("edit", command, result, start, false, nil)
	// The two strings are the whole change, once per occurrence replaced.
	out.LinesAdded, out.LinesRemoved = LineDelta(oldString, newString)
	out.LinesAdded *= replacements
	out.LinesRemoved *= replacements
	// The file before against the file after — not the two strings. A
	// replace_all that changed eight call sites is eight hunks in the file and
	// one pair of strings, and it is the eight the reader is owed.
	out.Diff = UnifiedDiff(content, updated)
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
	mode, _ := args["mode"].(string)
	if !pathOK || strings.TrimSpace(path) == "" {
		err := errors.New("path is required")
		return newToolOutput("edit", "edit", "", time.Now(), false, err), err
	}
	// old_string stopped being unconditionally required when append arrived,
	// so the check moved into Execute where the mode is known. Both entry
	// points now refuse the same combinations, worded from the same place.
	if !oldOK {
		oldString = ""
	}
	return s.Execute(ctx, Input{
		"args":        []string{path, oldString, newString},
		"replace_all": args["replace_all"],
		"mode":        mode,
	})
}
