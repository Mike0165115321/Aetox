package skill

// One tool for one thing: asking the code about itself.
//
// It was three - `diagnostics`, `symbol`, `repo_map` - and they are three names
// for one act at three scales: is this file broken, what is this name and who
// uses it, what shape is this project. None of them changes anything; all three
// answer a question the model would otherwise answer by reading files and
// guessing.
//
// Named `codebase` rather than `code` on purpose. `code` is already a
// *category* (category.go), the word a desk manifest writes in `categories:` to
// mean the developer group - and a tool wearing the same word would make
// `tools: code` and `categories: code` two different grants spelled identically.
// One word, two meanings, in the file where a user writes both.
//
// Gates, the same check every pack here is held to (search_pack.go):
//
//   - `planKeeps` (internal/mode/stance.go) holds all three, so วางแผน keeps
//     the pack whole - which is the point: a plan is built by looking.
//   - `parallelToolCalls` (internal/cognitive/agent.go) allows none of them, so
//     the pack does not straddle that line either. They start language servers
//     and walk whole trees; several at once is not a faster turn.
//
// **`rename` is deliberately not one of them.** It uses the same language
// server and answers the same kind of question, and it WRITES - which puts it
// on the other side of both gates above. A pack holding it would have cost
// วางแผน all three of these, which is exactly the failure the split between
// `search` and `change` exists to avoid.

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

type codebaseSkill struct {
	root         string
	outputSubdir func() string
	// open reports an unfocused session, where a project map has no project to
	// stand in. Carried rather than resolved here for the reason repoMapSkill
	// carries it: the host decides what the session is, not the tool.
	open bool
	// actions this caller may use, nil for all of them. See shellSkill.
	actions []string
}

func (*codebaseSkill) Name() string { return "codebase" }

func (*codebaseSkill) Description() string {
	return "ถามตัวโค้ดเกี่ยวกับตัวเอง, ไฟล์นี้พังไหม ชื่อนี้คืออะไรใครใช้บ้าง และโปรเจกต์นี้รูปร่างแบบไหน"
}

func (s *codebaseSkill) allowedActions() []string {
	p := packs["codebase"]
	if s == nil || len(s.actions) == 0 {
		return append([]string(nil), p.actions...)
	}
	return s.actions
}

func (s *codebaseSkill) Actions() []string { return packs["codebase"].permissions() }

func (s *codebaseSkill) Narrow(named []string) Skill {
	narrowed := *s
	narrowed.actions = packs["codebase"].narrow(named)
	return &narrowed
}

func (s *codebaseSkill) inner(action string) (Tool, error) {
	p := packs["codebase"]
	if _, known := p.names[action]; !known {
		return nil, fmt.Errorf("unknown codebase action %q, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	if !slices.Contains(s.allowedActions(), action) {
		return nil, fmt.Errorf("codebase %s is not available here, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	switch action {
	case "errors":
		return &diagnosticsSkill{root: s.root, outputSubdir: s.outputSubdir}, nil
	case "symbol":
		return &symbolSkill{root: s.root, outputSubdir: s.outputSubdir}, nil
	case "map":
		return &repoMapSkill{root: s.root, open: s.open}, nil
	}
	return nil, fmt.Errorf("codebase action %q has no implementation", action)
}

func (s *codebaseSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	lines := map[string]string{
		"errors": "`errors` (path), compile and type errors from the language server (gopls, tsserver, ...). A file, or a folder to check everything supported inside it; \".\" is the whole project. '(no problems)' means clean, and it says so when no server is installed for that language.",
		"symbol": "`symbol` (path, name), what an identifier is: signature, doc, where it is declared, and every place that references it. Exact where a search guesses.",
		"map":    "`map` (path?), the project's shape: files ranked by incoming references, with their symbols and line numbers.",
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
			"description": "The file for errors and symbol; the folder for map, which defaults to the whole project.",
		},
	}
	if slices.Contains(allowed, "symbol") {
		properties["name"] = map[string]any{
			"type":        "string",
			"description": "action=symbol: the identifier to look up, exactly as written.",
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
			Name: "codebase",
			Description: "Ask the code about itself. Actions:\n" +
				actions.String(),
			Parameters: payload,
		},
	}
}

func (s *codebaseSkill) Guidance(args map[string]any) string {
	inner, err := s.inner(actionOf(args))
	if err != nil {
		return ""
	}
	return guidanceFor(inner, args)
}

func (s *codebaseSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := fmt.Errorf("usage: codebase <%s> ...", strings.Join(s.allowedActions(), "|"))
		return newToolOutput("codebase", "codebase", "", start, false, err), err
	}
	inner, err := s.inner(strings.ToLower(strings.TrimSpace(args[0])))
	if err != nil {
		return newToolOutput("codebase", "codebase", "", start, false, err), err
	}
	return inner.Execute(ctx, Input{"args": args[1:]})
}

func (s *codebaseSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	action := actionOf(args)
	if action == "" {
		err := errors.New("action is required, one of: " + strings.Join(s.allowedActions(), ", "))
		return newToolOutput("codebase", "codebase", "", start, false, err), err
	}
	inner, err := s.inner(action)
	if err != nil {
		return newToolOutput("codebase", "codebase", "", start, false, err), err
	}
	return inner.ExecuteTool(ctx, args)
}
