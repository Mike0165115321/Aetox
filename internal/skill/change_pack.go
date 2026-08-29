package skill

// One tool for one thing: changing what is on disk.
//
// It was four - `write`, `edit`, `edits`, `delete` - and they are four names for
// one act at four sizes: put a whole file there, change part of one, change
// several places at once, take one away. Packed for the reason packed.go gives,
// and split from `search` (search_pack.go) for a reason worth stating rather
// than discovering:
//
// **A pack must not straddle a line some gate already draws.** Every act here
// writes, so all four fall on the same side of all three:
//
//   - `planKeeps` (internal/mode/stance.go) holds none of them, so วางแผน drops
//     the pack whole - which is the right answer, not a rough one.
//   - `parallelToolCalls` (internal/cognitive/agent.go) allows none of them: a
//     write and the read after it are the one pair whose order is the answer.
//   - `safety.AssessCommand` judges write and edit as their own risks, and
//     Unpack hands it the name it has always judged.
//
// `append` is an action rather than a `mode` parameter, and it is the same move
// the browser made with `scroll`: it rides on `edit`'s permission because it is
// editing, but a model choosing between five verbs makes fewer mistakes than
// one choosing a verb and then remembering a flag. The `mode` parameter is gone
// from the block with it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
)

type changeSkill struct {
	root         string
	outputSubdir func() string
	files        *FileState
	// actions this caller may use, nil for all of them. See shellSkill.
	actions []string
}

func (*changeSkill) Name() string { return "change" }

func (*changeSkill) Description() string {
	return "แก้ไฟล์ในโปรเจกต์, เขียนไฟล์ใหม่ แก้บางส่วน ต่อท้าย แก้หลายที่พร้อมกัน และลบ"
}

func (s *changeSkill) allowedActions() []string {
	p := packs["change"]
	if s == nil || len(s.actions) == 0 {
		return append([]string(nil), p.actions...)
	}
	return s.actions
}

func (s *changeSkill) Actions() []string { return packs["change"].permissions() }

func (s *changeSkill) Narrow(named []string) Skill {
	narrowed := *s
	narrowed.actions = packs["change"].narrow(named)
	return &narrowed
}

// inner resolves an action to the tool that does it, refusing an unknown one and
// an unpermitted one in different sentences (github_pack.go).
func (s *changeSkill) inner(action string) (Tool, error) {
	p := packs["change"]
	if _, known := p.names[action]; !known {
		return nil, fmt.Errorf("unknown change action %q, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	if !slices.Contains(s.allowedActions(), action) {
		return nil, fmt.Errorf("change %s is not available here, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	switch action {
	case "write":
		return &writeSkill{root: s.root, outputSubdir: s.outputSubdir, files: s.files}, nil
	case "edit", "append":
		return &editSkill{root: s.root, outputSubdir: s.outputSubdir, files: s.files}, nil
	case "batch":
		return &editsSkill{root: s.root, outputSubdir: s.outputSubdir, files: s.files}, nil
	case "delete":
		return &deleteSkill{root: s.root, outputSubdir: s.outputSubdir, files: s.files}, nil
	}
	// Unreachable while the switch covers packs["change"].actions, which the
	// pack test holds it to.
	return nil, fmt.Errorf("change action %q has no implementation", action)
}

func (s *changeSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	lines := map[string]string{
		"write":  "`write` (path, content), a whole file, at most 300 lines per call. Over that nothing is written: send 300 and carry on with append.",
		"edit":   "`edit` (path, find, replace, all?), replace an exact string. find must be unique in the file unless all=true. Empty replace deletes what matched.",
		"append": "`append` (path, replace), add text to the end of a file. Carries on a file that write had to cut; no separator is added, so a file cut mid-word continues mid-word. 300 lines per call, same as write.",
		"batch":  "`batch` (edits, path?), several edits across one or more files, all applied or none. Prefer it over repeated edit calls when one change touches several places.",
		"delete": "`delete` (path, recursive?), remove a file, or a folder and everything in it with recursive.",
	}
	var actions strings.Builder
	for _, a := range allowed {
		actions.WriteString(lines[a] + "\n")
	}

	properties := map[string]any{
		"action": map[string]any{
			"type": "string", "enum": allowed,
			"description": "What to do",
		},
		"path": map[string]any{
			"type":        "string",
			"description": "The file to change. For batch it is the default for edits that name none. A relative path may land in a per-session output folder; the result names where it really went, so use that for later reads and opens.",
		},
	}
	if slices.Contains(allowed, "write") {
		properties["content"] = map[string]any{
			"type": "string", "description": "action=write: the file's whole content.",
		}
	}
	if slices.Contains(allowed, "edit") || slices.Contains(allowed, "append") {
		properties["find"] = map[string]any{
			"type":        "string",
			"description": "action=edit: the exact text to find, unique in the file. Unused by append.",
		}
		properties["replace"] = map[string]any{
			"type":        "string",
			"description": "action=edit: what to put in its place, empty to delete it. action=append: the text to add at the end.",
		}
		properties["all"] = map[string]any{
			"type": "boolean", "description": "action=edit: replace every occurrence instead of requiring exactly one.",
		}
	}
	if slices.Contains(allowed, "batch") {
		properties["edits"] = map[string]any{
			"type":        "array",
			"description": "action=batch: the edits, in order. Every one must match or none are written.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Relative file path, or omit to use the call's path."},
					"find":    map[string]any{"type": "string", "description": "Exact text to find, unique after the earlier edits in this call."},
					"replace": map[string]any{"type": "string", "description": "Text to put in its place"},
				},
				"required":             []string{"find", "replace"},
				"additionalProperties": false,
			},
		}
	}
	if slices.Contains(allowed, "delete") {
		properties["recursive"] = map[string]any{
			"type":        "boolean",
			"description": "action=delete: required to remove a folder, and it takes everything inside. A folder named without it is refused.",
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"action", "path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "change",
			// Capability only: no when-to-pick-me language and no routing to
			// sibling tools (owner, 2026-08-04). The registry already lists
			// every option, and which one answers the request is the model's
			// call - a line here saying "reading is `read`" is paid for on
			// every request to repeat what the block beside it already says.
			Description: "Change what is on disk. Actions:\n" + actions.String(),
			Parameters:  payload,
		},
	}
}

// Guidance is the inner tool's, for the action actually called - the pack has
// none of its own and must not invent any (search_pack.go).
func (s *changeSkill) Guidance(args map[string]any) string {
	action := actionOf(args)
	inner, err := s.inner(action)
	if err != nil {
		return ""
	}
	return guidanceFor(inner, s.innerArgs(action, args))
}

// innerArgs is the call as the tool underneath expects it.
//
// One rewrite, and it is what lets `append` be an action: the edit tool has
// taken a `mode` since it learned to append, and this is the one place that
// knows the action word means that flag. Everything else passes through
// untouched, so a pack is a name and not a translation layer.
func (s *changeSkill) innerArgs(action string, args map[string]any) map[string]any {
	if action != "append" {
		return args
	}
	out := make(map[string]any, len(args)+1)
	for k, v := range args {
		out[k] = v
	}
	out["mode"] = "append"
	return out
}

// Execute is the door code: `change <action> <arg...>`, handed to whichever of
// the four owns that action (github_pack.go).
func (s *changeSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := fmt.Errorf("usage: change <%s> ...", strings.Join(s.allowedActions(), "|"))
		return newToolOutput("change", "change", "", start, false, err), err
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	inner, err := s.inner(action)
	if err != nil {
		return newToolOutput("change", "change", "", start, false, err), err
	}
	rest := Input{"args": args[1:]}
	if action == "append" {
		rest["mode"] = "append"
	}
	return inner.Execute(ctx, rest)
}

func (s *changeSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	action := actionOf(args)
	if action == "" {
		err := errors.New("action is required, one of: " + strings.Join(s.allowedActions(), ", "))
		return newToolOutput("change", "change", "", start, false, err), err
	}
	inner, err := s.inner(action)
	if err != nil {
		return newToolOutput("change", "change", "", start, false, err), err
	}
	out, err := inner.ExecuteTool(ctx, s.innerArgs(action, args))
	if err == nil && out.Success {
		if note := reviewNote(action, args); note != "" {
			out.Content = strings.TrimRight(out.Content, "\n") + "\n\n" + note
			out.RawOutput = out.Content
		}
	}
	return out, err
}

// reviewNote rides back with a change that reached across files.
//
// The failure it answers was measured rather than guessed: across three long
// sessions the assistant verified its own work every time — a compile check, a
// grep for leftovers, the diagnostics that ride back on every edit — and never
// once asked anybody else to look. That is not carelessness. Checking your own
// change is cheap and right there, and nothing in front of the model said the
// other thing was different in kind.
//
// It is: the context that wrote a change is the one least able to see what is
// wrong with it, and no amount of self-checking fixes that. A second reader is
// not a more careful version of the first, it is a different one.
//
// **Only a batch that touched more than one file.** A typo fixed in place needs
// no second reader and a note on every edit is a note nobody reads by the third
// one. Reaching across files is the cheapest honest signal that a change has a
// shape somebody could be wrong about — and it is a property of the call, so
// this needs no memory of what it has already said.
//
// It states a fact and names a door. It does not instruct: a tool result that
// tells the model what to do next is a tool result arguing with the system
// prompt, and the model is the one holding the context to judge from.
func reviewNote(action string, args map[string]any) string {
	if action != "batch" {
		return ""
	}
	raw, _ := args["edits"].([]any)
	paths := map[string]bool{}
	fallback := strings.TrimSpace(stringArg(args["path"]))
	for _, item := range raw {
		edit, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := strings.TrimSpace(stringArg(edit["path"]))
		if path == "" {
			path = fallback
		}
		if path != "" {
			paths[path] = true
		}
	}
	if len(paths) < 2 {
		return ""
	}
	return "[note] That change reached across " + strconv.Itoa(len(paths)) +
		" files, and nothing has read it except the context that wrote it. " +
		"`task` with agent=reviewer reads a change without having made it, and changes nothing itself."
}
