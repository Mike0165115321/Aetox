package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/runlang"
)

// A fenced block tagged with a language is a *file*, not a command, which is
// why the chat's Run button used to be offered on shell blocks alone: pasting
// Python source at a shell prompt is not running it, it is a syntax error with
// the user's consent attached to it.
//
// The button is offered on source now (owner, 16 ส.ค.), and what makes that the
// same act rather than a second one is that nothing here runs anything. The
// source is written to a file and an ordinary command — `python <file>` — is
// handed to runThroughShell, the one door in run_block.go. So a script gets the
// sandbox working directory, the shell audit log, the background-shells
// registry and the timeout, exactly as the model's own calls do, and there is
// still exactly one way a command runs in Aetox.
//
// The consent argument is unchanged and is the reason there is no extra dialog:
// the code is fully visible in the block the user is looking at, and clicking
// Run on it is the same act as saving it and running it themselves. What a
// script can do that a shell one-liner cannot is nothing — both are this
// machine, with this user's rights.
//
// Which tags are runnable and what runs them is not decided here. That is
// internal/runlang's single question, so this file cannot drift from the
// renderer that decided to draw the button.

// runScriptDir is where the file goes, relative to the sandbox root, and the
// relative part is the design rather than a convenience.
//
// A focused session has a wall: guardCommandPaths (skill/shell_sandbox.go)
// refuses a command line that names anything outside the workspace, and it is
// right to — a command that reaches out of the project is exactly what it
// exists to stop. A script parked in Aetox's own data root and run by absolute
// path would have been refused by it, and correctly so. So the script is
// written inside the workspace and named relative to it, which is a path the
// guard reads as what it is: a file in the folder the shell is already
// standing in.
//
// Dotted and its own folder so it reads as tooling rather than as work, and so
// there is one line to add to .gitignore for anyone who wants it. It exists for
// the length of one run; see the cleanup in RunChatScript. The dot earns a
// second keep for Go: the go tool skips dot-directories, so a `package main`
// block sitting here for the length of a run is invisible to the user's own
// `go build ./...`.
const runScriptDir = ".aetox/run-blocks"

// RunChatScript runs a fenced source block the user clicked Run on. lang is the
// fence's own tag, which is what decided the button was offered at all.
func (a *App) RunChatScript(lang, source string) (RunBlockResult, error) {
	l, ok := runlang.Lookup(lang)
	// A Shell tag reaching here means the two ends disagree about what kind of
	// block this is, and the answer is to refuse rather than to write somebody's
	// shell command into a file and hand it to an interpreter.
	if !ok || l.Kind != runlang.Script {
		return RunBlockResult{}, fmt.Errorf("no interpreter for %q", lang)
	}
	if strings.TrimSpace(source) == "" {
		return RunBlockResult{}, fmt.Errorf("empty script")
	}

	// Reported as a result rather than as an error, because "Python is not
	// installed" is the answer to "why didn't it work" and belongs in the panel
	// under the block with everything else the run has to say. RunnableLanguages
	// means the button should not have been there at all, so reaching this is a
	// machine that changed under a chat that was already open.
	interpreter, ok := l.Interpreter()
	if !ok {
		return RunBlockResult{
			Output:  fmt.Sprintf("%s is not installed, or not on PATH (looked for: %s)", lang, strings.Join(l.Names(), ", ")),
			Success: false,
		}, nil
	}

	rel, remove, err := a.writeScript(source, l.Ext)
	if err != nil {
		return RunBlockResult{}, err
	}
	// Removed after the run, not kept: the source the user is reading is in the
	// chat above the output, so the file is scaffolding rather than a record,
	// and a folder inside somebody's project that grows a file per click is
	// litter in their git status.
	defer remove()

	return a.runThroughShell(interpreter.CommandLine(rel))
}

// writeScript puts the source under the sandbox root and hands back the path to
// name it by, which is relative for the reason written on runScriptDir.
//
// The file name is a hash of the source, so clicking Run twice on the same
// block reuses one name instead of leaving two files, and two different blocks
// can never write over each other while an interpreter is reading one of them.
func (a *App) writeScript(source, ext string) (rel string, remove func(), err error) {
	noop := func() {}
	root := strings.TrimSpace(a.cur().cfg.SandboxRoot)
	if root == "" {
		return "", noop, fmt.Errorf("no workspace to run the script in")
	}
	dir := filepath.Join(root, filepath.FromSlash(runScriptDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", noop, fmt.Errorf("could not make %s: %w", dir, err)
	}
	sum := sha256.Sum256([]byte(source))
	name := "block-" + hex.EncodeToString(sum[:8]) + ext
	path := filepath.Join(dir, name)
	// A trailing newline because some interpreters treat a file whose last line
	// has no terminator as truncated.
	if err := os.WriteFile(path, []byte(strings.TrimRight(source, "\r\n")+"\n"), 0o600); err != nil {
		return "", noop, fmt.Errorf("could not write %s: %w", path, err)
	}
	return runScriptDir + "/" + name, func() {
		if err := os.Remove(path); err != nil {
			debuglog.Msg("run script: leftover %s: %v", path, err)
		}
	}, nil
}
