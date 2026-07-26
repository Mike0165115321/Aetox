package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

type writeSkill struct {
	root         string
	outputSubdir func() string
}

// placed decides where a new file actually lands. Without a project focused,
// the sandbox root is the user's home directory, so "write index.html" dropped
// a stray file next to Documents and Downloads — and the next chat that wrote
// index.html silently overwrote it. A relative path is therefore steered into
// the session's own output folder; an absolute one is an explicit destination
// and is left exactly where the caller asked for it.
//
// The returned path is what gets echoed back to the model, so a later read or
// edit of the same relative path finds the file where it was actually written.
func (s *writeSkill) placed(requestPath string) string {
	if s.outputSubdir == nil || filepath.IsAbs(requestPath) {
		return requestPath
	}
	subdir := strings.TrimSpace(s.outputSubdir())
	if subdir == "" {
		return requestPath
	}
	return filepath.ToSlash(filepath.Join(subdir, requestPath))
}

// PlacedPath is the read side of the same rule, and the single definition of
// it: write steers a new relative file into the session output folder, so
// anything resolving the path the model originally asked for has to look there
// too, or the model loses the file it just created and burns turns hunting for
// it. Exported because the rule reaches past this package — browser_open shows
// the user what write just produced, and a second copy of the rule there is a
// second chance for the two to disagree.
//
// The literal path always wins: the fallback only fires when nothing resolves
// there, so a real file is never shadowed by a same-named artifact in the
// output folder. Unfocused sessions are the only ones with a subdir at all
// (see App.outputSubdir), so a focused project never takes this path.
func PlacedPath(root string, outputSubdir func() string, requestPath string) string {
	if outputSubdir == nil || requestPath == "" || filepath.IsAbs(requestPath) {
		return requestPath
	}
	subdir := strings.TrimSpace(outputSubdir())
	if subdir == "" || existsInSandbox(root, requestPath) {
		return requestPath
	}
	// Report the original path when the fallback misses too: the error the
	// caller gets should name the path they actually asked for.
	if candidate := filepath.ToSlash(filepath.Join(subdir, requestPath)); existsInSandbox(root, candidate) {
		return candidate
	}
	return requestPath
}

func existsInSandbox(root, requestPath string) bool {
	target, err := resolveSandboxPath(root, requestPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(target)
	return err == nil
}

func (*writeSkill) Name() string { return "write" }

func (*writeSkill) Description() string {
	return "Create or overwrite a file under sandbox root"
}

func (*writeSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative destination path",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "File content",
			},
		},
		"required":             []string{"path", "content"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "write",
			// The placement rule is stated here rather than in the system
			// prompt because the destination changes per chat session and the
			// prompt is built once at bootstrap — a tool description travels
			// with every request and cannot go stale. Without it the model
			// assumed the file was at the sandbox root and told the user so.
			Description: "Write content to a file. A relative path may be placed in a per-session output folder rather than at the sandbox root — the result reports where the file actually landed, so use that path when telling the user where it is, and when reading, editing or opening it later.",
			Parameters:  payload,
		},
	}
}

func (s *writeSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("write skill unavailable")
		return newToolOutput("write", "write", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) < 2 {
		err := errors.New("usage: write <path> <content>")
		return newToolOutput("write", "write", "", start, false, err), err
	}

	requestPath := strings.TrimSpace(args[0])
	content := strings.Join(args[1:], " ")
	if requestPath == "" {
		err := errors.New("usage: write <path> <content>")
		return newToolOutput("write", "write "+strings.TrimSpace(strings.Join(args, " ")), "", start, false, err), err
	}
	original := requestPath
	requestPath = s.placed(requestPath)

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	if err := ensureWriteDir(targetPath); err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	// Read the outgoing version before clobbering it, so the timeline can say
	// whether this was a new file or a rewrite of N lines. A failed read means
	// "no file there" — a brand new file, nothing removed.
	previous, _ := os.ReadFile(targetPath)

	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	// Echo the path the caller asked for, like edit does — the resolved
	// absolute path is noise in context and nudges the model into repeating
	// the sandbox root back at the user (see internal/prompt environment()).
	//
	// Unless placement moved the file. Then "where is it on my machine" cannot
	// be answered from anything in context: the prompt has the root, the
	// receipt has a relative path, and the model has to compose the two. A
	// model that composes the root with the name it typed drops the output
	// folder in the middle and sends the user to a file that is not there —
	// which is exactly what happened. So the one case that needs it gets the
	// on-disk path handed over instead of computed.
	output := "write done: " + requestPath
	if requestPath != original {
		output += " (on disk: " + targetPath + ")"
	}
	out := newToolOutput("write", "write "+requestPath, output, start, false, nil)
	out.LinesAdded, out.LinesRemoved = LineDelta(string(previous), content)
	return out, nil
}

func (s *writeSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	if s == nil {
		err := errors.New("write skill unavailable")
		return newToolOutput("write", "write", "", time.Now(), false, err), err
	}

	path, pathOK := args["path"].(string)
	content, contentOK := args["content"].(string)
	if !pathOK || strings.TrimSpace(path) == "" {
		err := errors.New("path is required")
		return newToolOutput("write", "write", "", time.Now(), false, err), err
	}
	if !contentOK {
		content = ""
	}
	return s.Execute(ctx, Input{"args": []string{path, content}})
}

func ensureWriteDir(targetPath string) error {
	dir := filepath.Dir(targetPath)
	if dir == "." {
		return nil
	}
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return errors.New("parent path is not a directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}
