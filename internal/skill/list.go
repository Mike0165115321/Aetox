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
	"sync"
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

// resolveSandboxPath turns a requested path into an absolute one and decides,
// in this one place, whether the caller may have it. Every file tool answers
// through here; there is deliberately no second check anywhere else.
//
// The question it asks is always the same — does the target land inside the
// workspace? — and the workspace is the project root plus whatever folders the
// user added to it (sandboxPolicy). How the path was spelled does not change
// the answer: "D:\Other\api\main.go", "../api/main.go" and a symlink pointing
// at either are one target with one verdict. Absolute paths used to be refused
// outright, which was a spelling rule wearing a permission rule's clothes; once
// a workspace can hold a second folder it stops being expressible at all, since
// naming that folder in full is the only sane way to reach it.
func resolveSandboxPath(root string, requestPath string) (string, error) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		requestPath = "."
	}
	if !filepath.IsAbs(requestPath) {
		requestPath = filepath.Join(safeRoot, requestPath)
	}
	safeTarget, err := filepath.Abs(filepath.Clean(requestPath))
	if err != nil {
		return "", err
	}

	// The root asking whether it contains itself is a tautology, not a
	// filesystem question. Eight call sites pass `.` (shell's working directory,
	// grep and glob's walk base, delete's are-you-deleting-the-root guard); they
	// were paying two symlink walks to be told yes. 2.51ms → 1.6µs.
	if safeTarget == safeRoot {
		return safeTarget, nil
	}

	// A lexical prefix check is not containment: a symlink sitting inside the
	// root and pointing at C:\Users or /etc passes it untouched. Compare the
	// link-resolved forms instead, but still hand back the lexical path so
	// callers and their output keep showing the path the user asked for.
	//
	// ponytail: two EvalSymlinks walks per call — measured 981µs vs 1.8µs for
	// the old lexical check on Windows (Defender scans every component open).
	// Called at most twice per tool call and never inside grep/fs-find's
	// WalkDir, so ~2ms sits under operations that already cost 10ms+.
	//
	// Half of that was the root, resolved from scratch on every call to get the
	// same answer, so it is cached now — which is what this note used to say to
	// do "if that stops being true". Measured 2.51ms → 1.38ms per call.
	resolvedTarget := evalExistingSymlinks(safeTarget)
	if withinRoot(resolvedTarget, resolvedRoot(safeRoot)) {
		// Being inside the project root is not a reason to skip the credential
		// check. This branch used to return here and let refuseCredentialStore
		// guard only the outside-the-root path below, which read as "the user
		// chose this folder, so they chose what is in it" — true of source
		// files, false of ~/.ssh. Focus a home folder as the project (Aetox
		// invites exactly that: the assistant door works over the whole
		// machine) and the workspace *contains* every credential store the
		// denylist exists to refuse, reachable by a plain relative path.
		//
		// The folder being the root rather than an added one does not change
		// what is under it, and "the agent read my SSH key because I opened my
		// home directory" is the same trade nobody makes on purpose that
		// sandbox_open.go:135 already refuses one branch over (2026-08-13).
		if err := refuseCredentialStore(resolvedTarget); err != nil {
			return "", err
		}
		return safeTarget, nil
	}

	// Outside the project root, so it is only reachable if the user widened the
	// workspace — by adding this folder, or by working with no project focused
	// at all. Either way the credential stores stay shut (sandbox_open.go).
	policy := sandboxPolicyFor(safeRoot)
	if !policy.open && !policy.covers(resolvedTarget) {
		return "", fmt.Errorf("path is outside the folders this session can use — the user has to add it first")
	}
	if err := refuseCredentialStore(resolvedTarget); err != nil {
		return "", err
	}
	return safeTarget, nil
}

// rootResolutions caches evalExistingSymlinks per sandbox root. Roots are
// process-lifetime values — one per project, a handful per session — so this
// never needs eviction.
//
// Staleness fails CLOSED, which is why caching a security check is safe here.
// If a component of the root is repointed after the entry is warm, a target
// under the new location resolves somewhere the cached root is no longer a
// prefix of, and the call is rejected. The target side is still resolved live
// on every call; only the thing being compared against is remembered.
var rootResolutions sync.Map // map[string]string

func resolvedRoot(safeRoot string) string {
	if cached, ok := rootResolutions.Load(safeRoot); ok {
		return cached.(string)
	}
	resolved := evalExistingSymlinks(safeRoot)
	rootResolutions.Store(safeRoot, resolved)
	return resolved
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
