package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

type listSkill struct {
	root         string
	outputSubdir func() string
}

func (*listSkill) Name() string { return "list" }

func (*listSkill) Description() string {
	return "List files in a sandbox subpath"
}

func (*listSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path to list, defaults to root.",
			},
		},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "list",
			Description: "List the entries of a sandbox folder. Directories end in \"/\"; everything else is a file.",
			Parameters:  payload,
		},
	}
}

func (s *listSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		return newToolOutput("list", "list", "", start, false, fmt.Errorf("list skill unavailable")), fmt.Errorf("list skill unavailable")
	}

	args := stringSlice(input["args"])
	requestPath := "."
	if len(args) > 0 {
		requestPath = PlacedPath(s.root, s.outputSubdir, strings.Join(args, " "))
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("list", "list "+requestPath, "", start, false, err), err
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return newToolOutput("list", "list "+requestPath, "", start, false, err), err
	}

	// A trailing "/" on directories, the way ls -F and every file listing a
	// model has ever read marks them. Without it "sub" and "sub.txt" are the
	// same kind of thing on the page, and the only way to find out was to call
	// list again and see whether it errored.
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name()+"/")
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	output, truncated := limitLines(strings.Join(names, "\n"), defaultToolOutputLineLimit)
	command := "list"
	if requestPath != "" && requestPath != "." {
		command = "list " + requestPath
	}
	return newToolOutput("list", command, output, start, truncated, nil), nil
}

func (s *listSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	requestPath := "."
	if rawPath, ok := args["path"].(string); ok {
		requestPath = strings.TrimSpace(rawPath)
		if requestPath == "" {
			requestPath = "."
		}
	}
	params := []string{}
	if requestPath != "." {
		params = []string{requestPath}
	}
	return s.Execute(ctx, Input{"args": params})
}

func resolveSandboxPath(root string, requestPath string) (string, error) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		requestPath = "."
	}
	if filepath.IsAbs(requestPath) {
		return "", fmt.Errorf("absolute path is not allowed")
	}

	candidate := filepath.Join(safeRoot, requestPath)
	normalized := filepath.Clean(candidate)
	safeTarget, err := filepath.Abs(normalized)
	if err != nil {
		return "", err
	}

	// A lexical prefix check is not containment: a symlink sitting inside the
	// root and pointing at C:\Users or /etc passes it untouched. Compare the
	// link-resolved forms instead, but still hand back the lexical path so
	// callers and their output keep showing the path the user asked for.
	//
	// ponytail: two EvalSymlinks walks per call — measured 981µs vs 1.8µs for
	// the old lexical check on Windows (Defender scans every component open).
	// Called at most twice per tool call and never inside grep/fs-find's
	// WalkDir, so ~2ms sits under operations that already cost 10ms+. Cache
	// the root's resolution per skill instance if that stops being true.
	if !withinRoot(evalExistingSymlinks(safeTarget), evalExistingSymlinks(safeRoot)) {
		return "", fmt.Errorf("path is outside sandbox root")
	}
	return safeTarget, nil
}

// evalExistingSymlinks resolves symlinks on the deepest prefix of path that
// actually exists and re-attaches the rest. The leaf is often missing — write
// and edit create it — and EvalSymlinks fails outright on a missing path.
func evalExistingSymlinks(path string) string {
	rest := ""
	for {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Join(path, rest)
		}
		rest = filepath.Join(filepath.Base(path), rest)
		path = parent
	}
}

// withinRoot compares case-insensitively on Windows: NTFS is case-insensitive,
// so rejecting C:\Work under root c:\work is a false positive, not safety.
func withinRoot(target, root string) bool {
	if runtime.GOOS == "windows" {
		target, root = strings.ToLower(target), strings.ToLower(root)
	}
	sep := string(filepath.Separator)
	return target == root || strings.HasPrefix(target+sep, root+sep)
}
