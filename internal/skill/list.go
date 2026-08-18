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
	"github.com/Mike0165115321/Aetox/internal/proc"
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

// onDiskNote is the "(on disk: …)" clause the four writing tools append when
// placement moved a file somewhere the caller did not name.
//
// One copy because it was four, all identical, and a receipt that has to be
// right about two filesystems is not the thing to keep four copies of.
//
// It names both spellings when the session's shell has its own, because the
// clause has two readers and they need different answers. The user asks "where
// is it on my machine" and the Windows path is that answer — it is what goes in
// Explorer, and it is why this clause exists at all. The model's next move is
// often to *use* the file, in a shell that has never heard of drive letters,
// where that same path opens nothing (§126.6). Saying which is which costs one
// clause on a receipt that is already rare, and picking one reader to be wrong
// for costs a turn every time.
//
// Native sessions never reach the translation, so the ordinary case is exactly
// the string it always was.
func onDiskNote(root, target string) string {
	backend := sandboxShellFor(root)
	if proc.IsNative(backend) {
		return " (on disk: " + target + ")"
	}
	guest, ok := backend.GuestPath(target)
	if !ok || guest == target {
		return " (on disk: " + target + ")"
	}
	return " (on disk: " + target + ", and " + guest + " in this shell)"
}

// searchBaseExists is the check that stands between "I searched and found
// nothing" and "I never searched".
//
// A walk over a directory that is not there opens no files, matches no lines
// and completes without error, so grep and glob both reported an empty result:
// the exact same sentence they use for a folder they read every byte of. The
// two are not the same answer and must never share one, because only one of
// them is evidence.
//
// Found 2026-08-17, and the transcript is the argument for this being an error
// rather than a gentler empty result. Asked whether a project held an admin
// password, grep answered "(no matches)" and glob answered "(no files
// matched)" for a folder full of both — the path had been silently resolved
// somewhere else — and the agent reported to the user that the project does
// not contain one. A failure that reads as a successful search is the single
// direction a tool must never be wrong in: everything downstream, the model and
// the person reading it, treats the empty page as a fact about their code.
//
// The resolved path is named beside the requested one on purpose. "no such
// folder" says the search failed; "/mnt/d/Project resolved to
// C:\Users\ASUS\aetox\mnt\d\Project" says why, and is the difference between
// the model rewriting the path and the model concluding the folder is empty.
//
// Only for a base the caller actually named. A path this package derived for
// itself — glob's literal prefix, taken out of a pattern the user guessed at —
// is not a claim about the filesystem, and failing on it would turn "that
// pattern matches nothing here" into a broken tool.
func searchBaseExists(requested, resolved string) error {
	if _, err := os.Stat(resolved); err != nil {
		if requested == "" || requested == "." || requested == resolved {
			return fmt.Errorf("%s cannot be searched: %w", resolved, err)
		}
		return fmt.Errorf("%s cannot be searched (it resolved to %s): %w", requested, resolved, err)
	}
	return nil
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
//
// What a path is *spelled in* does change the answer, and that is a different
// thing from how it was spelled. A session whose shell is a WSL distro names
// its files the way that distro does, and those names have to be brought into
// the host's spelling before any of the above means anything — otherwise
// `/mnt/d/project` is not an absolute path to Go on Windows, gets joined onto
// the root, and the tool answers about a folder nobody asked for (hostSpelling).
func resolveSandboxPath(root string, requestPath string) (string, error) {
	return resolveWithin(root, requestPath, true)
}

// WorkspacePath is resolveSandboxPath for the window rather than the agent: the
// same one gate, asked quietly.
//
// It exists because the desktop had grown its own copy of this rule, and the
// copy was worse in the way a second copy always is — it answered a question
// the original had already solved. `filepath.Join(root, "D:\\Mike\\report.docx")`
// is `<root>\D:\Mike\report.docx`: a path that cannot exist, that passes a
// prefix check because it really is under the root, and that Stat then fails to
// find. The window read that as *the file is gone* and hid the button to open
// it, about a file sitting on the user's disk the whole time.
//
// Quiet is the whole difference from the agent's door. A tool refused a path
// mid-turn can offer to add the folder and carry on; a pane working out whether
// to draw a button must not put a permission dialog on screen for a question
// nobody asked out loud. Refused stays refused here, and the caller reports it
// as what it is — not knowing — rather than as absence.
func WorkspacePath(root string, requestPath string) (string, error) {
	return resolveWithin(root, requestPath, false)
}

func resolveWithin(root string, requestPath string, mayWiden bool) (string, error) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		requestPath = "."
	}
	if requestPath, err = hostSpelling(safeRoot, requestPath); err != nil {
		return "", err
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
		if err := refuseResolved(resolvedTarget); err != nil {
			return "", err
		}
		return safeTarget, nil
	}

	// Outside the project root, so it is only reachable if the user widened the
	// workspace — by adding this folder, or by working with no project focused
	// at all. Either way the credential stores stay shut (sandbox_open.go).
	policy := sandboxPolicyFor(safeRoot)
	if !policy.open && !policy.covers(resolvedTarget) && !(mayWiden && widened(safeRoot, policy, resolvedTarget)) {
		return "", fmt.Errorf("path is outside the folders this session can use — the user has to add it first")
	}
	if err := refuseResolved(resolvedTarget); err != nil {
		return "", err
	}
	return safeTarget, nil
}

// widened offers the refusal above to the host as a question — "this needs
// D:\shared-lib, add it to the project?" — and reports whether the workspace
// really did grow to cover the target.
//
// The host's yes is not what decides it. Whatever the answer, the verdict comes
// from re-reading the policy: the workspace is what the folder list says it is,
// and a host that answered true without putting anything on the list gets the
// same refusal it would have got in silence. That is the difference between a
// door and a hole — the door still leads through the list, it just stops making
// the user walk around to reach it.
//
// Left after `covers` on purpose, so the ordinary case costs nothing: a session
// with no door, or a path already inside the workspace, never reaches this.
func widened(safeRoot string, policy sandboxPolicy, resolvedTarget string) bool {
	if policy.ask == nil {
		return false
	}
	if !policy.ask(resolvedTarget) {
		return false
	}
	return sandboxPolicyFor(safeRoot).covers(resolvedTarget)
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
