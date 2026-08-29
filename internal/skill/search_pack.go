package skill

// One tool for one thing: finding where something is.
//
// It was three - `list`, `glob`, `grep` - and they are three names for one act
// performed at three depths: what is in this folder, which files are named like
// this, which files contain this. That is a search, and a search is one tool
// (github_pack.go, and the reasoning in packed.go).
//
// **`read` is deliberately not one of them**, and the line is the same one
// `plugin_install` sits on the far side of in the github pack: these three hand
// back where something IS, and `read` hands back what it SAYS. It is also the
// most-called tool in this repository's history and an act of its own, not a
// fourth depth of looking.
//
// The three were chosen so the pack does not cross a line some gate already
// draws. All three only read, so:
//
//   - `planKeeps` (internal/mode/stance.go) holds all three, so วางแผน keeps the
//     pack whole rather than narrowing it.
//   - `parallelToolCalls` (internal/cognitive/agent.go) allows all three, so the
//     pack is parallel-safe under one name.
//   - `safety.AssessCommand` judges all three as reads.
//
// A pack straddling one of those would have to be split at the gate rather than
// at the object. This one needs no gate to change except the name it is keyed
// by, and Unpack hands each of them the name it always had.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
)

type searchSkill struct {
	root         string
	outputSubdir func() string
	// actions this caller may use, nil for all of them. See shellSkill.
	actions []string
}

func (*searchSkill) Name() string { return "search" }

func (*searchSkill) Description() string {
	return "ค้นหาในโปรเจกต์, ดูว่าโฟลเดอร์มีอะไร หาไฟล์จากชื่อ และค้นข้อความในไฟล์"
}

func (s *searchSkill) allowedActions() []string {
	p := packs["search"]
	if s == nil || len(s.actions) == 0 {
		return append([]string(nil), p.actions...)
	}
	return s.actions
}

func (s *searchSkill) Actions() []string { return packs["search"].permissions() }

func (s *searchSkill) Narrow(named []string) Skill {
	narrowed := *s
	narrowed.actions = packs["search"].narrow(named)
	return &narrowed
}

// inner resolves an action to the tool that does it, refusing an unknown one and
// an unpermitted one in different sentences - the first is a mistake in the
// call, the second is an answer about this session (github_pack.go).
func (s *searchSkill) inner(action string) (Tool, error) {
	p := packs["search"]
	if _, known := p.names[action]; !known {
		return nil, fmt.Errorf("unknown search action %q, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	if !slices.Contains(s.allowedActions(), action) {
		return nil, fmt.Errorf("search %s is not available here, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	switch action {
	case "list":
		return &listSkill{root: s.root, outputSubdir: s.outputSubdir}, nil
	case "glob":
		return &globSkill{root: s.root}, nil
	case "grep":
		return &grepSkill{root: s.root, outputSubdir: s.outputSubdir}, nil
	}
	// Unreachable while the switch covers packs["search"].actions, which the
	// pack test holds it to.
	return nil, fmt.Errorf("search action %q has no implementation", action)
}

func (s *searchSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	// The order a session uses them, and the description keeps it: a grep with
	// no idea which directory it is standing in pays for the whole tree.
	lines := map[string]string{
		"list": "`list` (path?), what is in a folder. Directories end in a slash. Defaults to the root.",
		"glob": "`glob` (pattern, path?), files by NAME, e.g. **/*.go or src/**/*.{ts,svelte}. Newest first.",
		"grep": "`grep` (pattern, path?, glob?, type?, show?, context?, multiline?), files by CONTENT. Go/RE2 regex, no backreferences or lookaround. Returns paths; show=content returns path:line:text.",
	}
	var actions strings.Builder
	for _, a := range allowed {
		actions.WriteString(lines[a] + "\n")
	}

	// One `path` and one `pattern` across three acts, because they mean the same
	// kind of thing in each. What differs is said once in the parameter, not in
	// three parameters that would each be absent two thirds of the time.
	properties := map[string]any{
		"action": map[string]any{
			"type": "string", "enum": allowed,
			"description": "What to do",
		},
		"path": map[string]any{
			"type":        "string",
			"description": "Where to look: the folder for list, the directory to search from for glob and grep. Defaults to the root.",
		},
		"pattern": map[string]any{
			"type":        "string",
			"description": "action=glob: a path pattern, ** matches any number of directories. action=grep: a Go regular expression, prefix (?i) for case-insensitive.",
		},
		"limit":  map[string]any{"type": "integer", "description": "Cap on entries returned."},
		"offset": map[string]any{"type": "integer", "description": "Skip this many entries first."},
	}
	// grep's own five, present only when grep is: a narrowed pack must not bill
	// for parameters none of its actions read.
	if slices.Contains(allowed, "grep") {
		properties["glob"] = map[string]any{
			"type":        "string",
			"description": "action=grep: only files whose name matches, e.g. *.go or *.{ts,svelte}",
		}
		properties["type"] = map[string]any{
			"type":        "string",
			"description": "action=grep: only this language's files: go, ts, py, rust and 20 more; an unknown name lists them.",
		}
		properties["show"] = map[string]any{
			"type": "string", "enum": []string{"content", "files_with_matches", "count"},
			"description": "action=grep: files_with_matches (default) paths only; content the matching lines; count a per-file tally.",
		}
		properties["context"] = map[string]any{
			"type": "integer", "description": "action=grep: lines around each match, max 50. Selects show=content.",
		}
		properties["multiline"] = map[string]any{
			"type": "boolean", "description": "action=grep: let the pattern cross line boundaries.",
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"action"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "search",
			Description: "Find where something is in the workspace. Actions:\n" +
				actions.String() +
				"Reading a file you have already found is `read`. Dependency and build directories are never searched.",
			Parameters: payload,
		},
	}
}

// Guidance is the inner tool's, for the action actually called.
//
// The pack has no judgment of its own and must not invent any: what a session
// needs to know about paging a grep is grep's, unchanged, and a session that
// only ever lists a folder should never be sent it. guidanceKey already keys on
// the action for exactly this (guidance.go).
func (s *searchSkill) Guidance(args map[string]any) string {
	inner, err := s.inner(actionOf(args))
	if err != nil {
		return ""
	}
	return guidanceFor(inner, args)
}

// Execute is the door code: `search <action> <arg...>`, handed to whichever of
// the three owns that action with the action word taken off the front. Kept for
// the reason github_pack.go keeps its own: packing changed what the model is
// offered, not what a person can type.
func (s *searchSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := fmt.Errorf("usage: search <%s> ...", strings.Join(s.allowedActions(), "|"))
		return newToolOutput("search", "search", "", start, false, err), err
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	inner, err := s.inner(action)
	if err != nil {
		return newToolOutput("search", "search", "", start, false, err), err
	}
	rest := Input{"args": args[1:]}
	// Everything a tool call would have carried by name, so both doors reach the
	// same code with the same options.
	for _, key := range []string{"glob", "type", "show", "context", "limit", "offset", "multiline"} {
		if v, ok := input[key]; ok {
			rest[key] = v
		}
	}
	return inner.Execute(ctx, rest)
}

func (s *searchSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	action := actionOf(args)
	if action == "" {
		err := errors.New("action is required, one of: " + strings.Join(s.allowedActions(), ", "))
		return newToolOutput("search", "search", "", start, false, err), err
	}
	inner, err := s.inner(action)
	if err != nil {
		return newToolOutput("search", "search", "", start, false, err), err
	}
	return inner.ExecuteTool(ctx, args)
}

// actionOf reads the action word out of a call.
func actionOf(args map[string]any) string {
	raw, _ := args["action"].(string)
	return strings.ToLower(strings.TrimSpace(raw))
}
