package skill

// One tool for one thing: somebody else's repository, read from here.
//
// It was four — `github_search`, `github_repo_summary`, `github_list_files`,
// `github_read_file` — and they were four names describing one act performed at
// four depths: find the repo, see what it is, see what is in it, read a file of
// it. That is a browse, and a browse is one tool (desktop/browser_tool.go, and
// the reasoning in packed.go).
//
// `plugin_install` lives in the same file as those four and is deliberately not
// in this pack. It reaches GitHub the same way, through the same connection and
// the same gate, and that is where the resemblance stops: the other four hand
// back something to read, and this one puts code on the user's machine. A tool
// that installs is not a depth of looking, and folding it in would have made
// "may read a repository" and "may install from one" the same grant — the
// distinction browser_tool.go was careful not to lose, for the same reason.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
)

type githubSkill struct {
	// actions this caller may use, nil for all of them. See shellSkill.
	actions []string
}

func (*githubSkill) Name() string { return "github" }

func (*githubSkill) Description() string {
	return "ค้นและอ่าน repository บน GitHub — หา repo, สรุปว่ามันคืออะไร, ดูว่ามีไฟล์อะไร และอ่านไฟล์"
}

func (s *githubSkill) allowedActions() []string {
	p := packs["github"]
	if s == nil || len(s.actions) == 0 {
		return append([]string(nil), p.actions...)
	}
	return s.actions
}

func (s *githubSkill) Actions() []string { return packs["github"].permissions() }

func (s *githubSkill) Narrow(named []string) Skill {
	narrowed := *s
	narrowed.actions = packs["github"].narrow(named)
	return &narrowed
}

func (s *githubSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	// The order in packs["github"] is the order a session uses them, and the
	// description says so out loud: a model that reads a file before finding
	// out which repository it is in wastes a call on a guessed path.
	lines := map[string]string{
		"search":       "`search` (query) — find repositories. GitHub repo search syntax, e.g. 'terminal ui language:go'. Returns name, stars, description and URL for each.",
		"repo_summary": "`repo_summary` (repo_url) — what one repository is, from its GitHub metadata.",
		"list_files":   "`list_files` (repo_url, path?) — the files and directories at a path, or at the root. Use it before read_file rather than guessing a path.",
		"read_file":    "`read_file` (repo_url, path, ref?) — one file's raw content. ref is a branch, tag or commit; without it, the repo's default branch.",
	}
	var actions strings.Builder
	for _, a := range allowed {
		actions.WriteString(lines[a] + "\n")
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        allowed,
				"description": "What to do",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "action=search: the search query",
			},
			"repo_url": map[string]any{
				"type":        "string",
				"description": "action=repo_summary/list_files/read_file: the repository URL (https://github.com/owner/repo)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "action=read_file: the file path inside the repo, e.g. README.md or src/main.go. action=list_files: the directory to list, default the repo root.",
			},
			"ref": map[string]any{
				"type":        "string",
				"description": "action=read_file: branch, tag, or commit (default: the repo's default branch)",
			},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "github",
			Description: "Read a repository on GitHub — anyone's, not just the user's. Actions:\n" +
				actions.String() + "\n" +
				"Public repositories need nothing but a URL. Reading code that is already on this machine is the file tools' job, not this one's — this is for repositories that are not checked out here.",
			Parameters: payload,
		},
	}
}

// Execute is the door code: `github <action> <arg...>`, handed to whichever of
// the four owns that action with the action word taken off the front.
//
// Kept because packing changed what the model is offered, not what the user can
// type — the four door codes in DOOR-CODE.md became four actions on one, and a
// person who typed them before should not find out about that by getting an
// error.
func (s *githubSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := fmt.Errorf("usage: github <%s> ...", strings.Join(s.allowedActions(), "|"))
		return newToolOutput("github", "github", "", start, false, err), err
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	inner, err := s.innerFor(action)
	if err != nil {
		return newToolOutput("github", "github", "", start, false, err), err
	}
	return inner.Execute(ctx, Input{"args": args[1:]})
}

func (s *githubSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	raw, _ := args["action"].(string)
	inner, err := s.innerFor(strings.ToLower(strings.TrimSpace(raw)))
	if err != nil {
		return newToolOutput("github", "github", "", start, false, err), err
	}
	return inner.ExecuteTool(ctx, args)
}

// innerFor resolves an action to the tool that does it, refusing an unknown one
// and an unpermitted one in different sentences — the first is a mistake in the
// call, the second is an answer about this session, and a model that cannot
// tell them apart retries the wrong one.
func (s *githubSkill) innerFor(action string) (Tool, error) {
	p := packs["github"]
	if _, known := p.names[action]; !known {
		return nil, fmt.Errorf("unknown github action %q — this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	if !slices.Contains(s.allowedActions(), action) {
		return nil, fmt.Errorf("github %s is not available here — this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	switch action {
	case "search":
		return &githubSearchSkill{}, nil
	case "repo_summary":
		return &githubRepoSummarySkill{}, nil
	case "list_files":
		return &githubListFilesSkill{}, nil
	case "read_file":
		return &githubReadFileSkill{}, nil
	}
	// Unreachable while the switch covers packs["github"].actions, which the
	// pack test holds it to.
	return nil, fmt.Errorf("github action %q has no implementation", action)
}
