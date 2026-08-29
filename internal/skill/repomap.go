package skill

// The repo_map tool: internal/repomap's parse → rank → fit core behind the
// sandbox. Everything about WHERE it may look is decided here — the library
// never widens a path — and everything about HOW the map is built stays in the
// library, so ระดับ 5 can lift it without dragging the sandbox along
// (docs/aider-study/EXECUTION.md).
//
// CategoryCode on purpose (category.go): the assistant desk holds no project
// root, and a map of "wherever the session happens to stand" would be junk the
// model trusts. The desk wall keeps it off that desk; the open-sandbox refusal
// below is the second fence, in the tool itself, for the day some desk carries
// it unfocused anyway.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/repomap"
)

type repoMapSkill struct {
	root string
	open bool
}

func (*repoMapSkill) Name() string { return "repo_map" }

func (*repoMapSkill) Description() string {
	return "Ranked map of the project: files by incoming references, with their symbols"
}

func (*repoMapSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative folder (default: whole project)",
			},
		},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "repo_map",
			// "Read this before whole files" is judgment and lives in
			// Guidance(); the block carries what it IS (block standard, §99).
			Description: "Project map: files ranked by incoming references, with symbols and line numbers",
			Parameters:  payload,
		},
	}
}

func (*repoMapSkill) Guidance(map[string]any) string {
	return "The first files in the map are the ones the rest of the project leans on — " +
		"start reading there, and open only the line ranges the map points at " +
		"(read has offset/limit) instead of whole files. The map fits a fixed budget: " +
		"when it says files were cut, map a subfolder with path to see deeper."
}

func (s *repoMapSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("repo_map skill unavailable")
		return newToolOutput("repo_map", "repo_map", "", start, false, err), err
	}
	sub := "."
	if args := stringSlice(input["args"]); len(args) > 0 {
		sub = strings.TrimSpace(strings.Join(args, " "))
	}
	command := "repo_map"
	if sub != "." && sub != "" {
		command += " " + sub
	}
	// Unfocused, the root is the user's whole machine, and a map of a machine
	// is not a smaller answer — it is a wrong one, ranked confidently. Refusing
	// is the honest output, and it names the fix.
	if s.open {
		err := errors.New("no project is focused: repo_map maps a project folder, not the whole machine — focus a project, or pass a folder to another tool")
		return newToolOutput("repo_map", command, "", start, false, err), err
	}
	base, err := resolveSandboxPath(s.root, sub)
	if err != nil {
		return newToolOutput("repo_map", command, "", start, false, err), err
	}
	// A hard time budget, because the first real project this tool met proved
	// the failure mode: a Python venv the ignore list did not know took the
	// walk to 105 seconds, the loop's 60-second marker fired, and the model
	// called again — paying twice for a map that arrives too late to be one.
	// On the deadline Build renders what it parsed, marked partial.
	mapCtx, cancel := context.WithTimeout(ctx, RepoMapTimeBudget)
	defer cancel()
	out, err := repomap.Build(mapCtx, repomap.Options{Root: base, Ignore: RepoMapIgnores()})
	if err != nil {
		return newToolOutput("repo_map", command, "", start, false, err), err
	}
	return newToolOutput("repo_map", command, out, start, false, nil), nil
}

// Exported, both of them, because the desktop's node view calls the same
// analysis directly (repomap.Graph) and MUST walk under the same rules — the
// sync the owner asked for is these two names having exactly one definition.
const RepoMapTimeBudget = 15 * time.Second

// RepoMapIgnores is IgnoredDirs plus the dependency trees IgnoredDirs has
// never needed to name: grep pays per byte SEARCHED and a venv mostly misses,
// but the map pays per file PARSED and a venv is thirty thousand .py files of
// somebody else's code that no ranking should ever surface. Additions here,
// not in IgnoredDirs, so the sweep of what grep may see stays one decision in
// one place.
func RepoMapIgnores() map[string]bool {
	extra := map[string]bool{"venv": true, "env": true, "site-packages": true, "third_party": true}
	for name := range IgnoredDirs {
		extra[name] = true
	}
	return extra
}

func (s *repoMapSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	var callArgs []string
	if path, ok := args["path"].(string); ok && strings.TrimSpace(path) != "" {
		callArgs = append(callArgs, strings.TrimSpace(path))
	}
	return s.Execute(ctx, Input{"args": callArgs})
}
