package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func scriptApp(t *testing.T) *App {
	t.Helper()
	return seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, newConversation())
}

// The tag decides whether the button was offered at all, so one that is not a
// runnable *source* tag reaching here means the two ends disagree — and the
// answer is to refuse rather than to guess an interpreter. `bash` is the
// interesting member: it is runnable, but its text is a command, and writing a
// command into a file and handing it to an interpreter is not what it meant.
func TestRunChatScriptRefusesAnythingThatIsNotSource(t *testing.T) {
	app := scriptApp(t)
	for _, lang := range []string{"json", "", "bash", "powershell", "ts"} {
		if _, err := app.RunChatScript(lang, "print(1)"); err == nil {
			t.Errorf("lang %q was accepted; only source tags with an interpreter may run", lang)
		}
	}
}

// Reported as a result and not as an error: "Python is not installed" is the
// answer to "why didn't it work", and it belongs in the panel under the block
// with everything else the run has to say.
//
// Reached by emptying PATH rather than by injecting a fake language, because
// the table is internal/runlang's now — and a machine where nothing is on PATH
// is exactly the state this message exists for.
func TestRunChatScriptReportsAMissingInterpreterAsOutput(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	res, err := scriptApp(t).RunChatScript("python", "print(1)")
	if err != nil {
		t.Fatalf("a missing interpreter must not be a Go error: %v", err)
	}
	if res.Success {
		t.Error("a run that never happened is not a success")
	}
	// Every candidate is named, because "python is not installed" on a machine
	// where the user has `py` is an answer that sends them looking in the wrong
	// place.
	for _, want := range []string{"python", "py", "python3"} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("the output does not name %q among what was looked for: %q", want, res.Output)
		}
	}
}

// The button is drawn from this, so it is the answer that decides whether a
// user ever sees one. Shell tags cannot be missing; source tags are offered
// only where this machine can actually run them.
func TestRunnableLanguagesAnswersForThisMachine(t *testing.T) {
	got := (&App{}).RunnableLanguages()

	if got["bash"] != "shell" {
		t.Errorf("bash = %q, want \"shell\" — every machine has a shell", got["bash"])
	}
	if _, offered := got["json"]; offered {
		t.Error("json is offered; running markup means nothing")
	}
	if _, offered := got["ts"]; offered {
		t.Error("ts is offered, but no runtime here can run a .ts file")
	}

	// This suite runs where the Go toolchain does, by definition.
	if got["go"] != "script" {
		t.Errorf("go = %q, want \"script\" on a machine running `go test`", got["go"])
	}
}

// The whole reason the file lives inside the workspace: a focused session's
// guardCommandPaths (skill/shell_sandbox.go) refuses a command line naming
// anything outside it, so a script in Aetox's data root run by absolute path
// would be refused — correctly. Relative, inside the root, is a path the guard
// reads as a file in the folder the shell is already standing in.
func TestWriteScriptStaysInsideTheWorkspaceAndIsNamedRelatively(t *testing.T) {
	app := scriptApp(t)

	rel, remove, err := app.writeScript("print(1)", ".py")
	if err != nil {
		t.Fatalf("writeScript: %v", err)
	}
	t.Cleanup(remove)

	if filepath.IsAbs(rel) {
		t.Errorf("the script is named by an absolute path (%q), which the sandbox guard refuses", rel)
	}
	if strings.Contains(rel, "..") {
		t.Errorf("the path climbs out of the workspace: %q", rel)
	}
	if !strings.HasPrefix(rel, runScriptDir+"/") {
		t.Errorf("path = %q, want it under %q", rel, runScriptDir)
	}
	if _, err := os.Stat(filepath.Join(app.cur().cfg.SandboxRoot, filepath.FromSlash(rel))); err != nil {
		t.Errorf("the file is not where the command will look for it: %v", err)
	}
}

// Clicking Run twice on one block reuses a single file instead of littering,
// and two different blocks can never write over each other while an interpreter
// is reading one of them.
func TestWriteScriptIsStablePerSourceAndDistinctBetweenThem(t *testing.T) {
	app := scriptApp(t)

	first, removeFirst, err := app.writeScript("print(1)", ".py")
	if err != nil {
		t.Fatalf("writeScript: %v", err)
	}
	t.Cleanup(removeFirst)

	again, _, err := app.writeScript("print(1)", ".py")
	if err != nil {
		t.Fatalf("writeScript: %v", err)
	}
	if first != again {
		t.Errorf("the same source wrote two files: %q and %q", first, again)
	}

	other, removeOther, err := app.writeScript("print(2)", ".py")
	if err != nil {
		t.Fatalf("writeScript: %v", err)
	}
	t.Cleanup(removeOther)
	if other == first {
		t.Error("two different scripts share one file, so a run can overwrite a running one")
	}

	if got := filepath.Ext(first); got != ".py" {
		t.Errorf("extension = %q, want .py — the interpreter reads it", got)
	}
	body, err := os.ReadFile(filepath.Join(app.cur().cfg.SandboxRoot, filepath.FromSlash(first)))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	// Some interpreters treat a last line with no terminator as truncated.
	if !strings.HasSuffix(string(body), "\n") {
		t.Errorf("the script has no trailing newline: %q", string(body))
	}
}

// The file is scaffolding, not a record: the source is in the chat above the
// output, and a folder inside somebody's project that grows a file per click is
// litter in their git status.
func TestWriteScriptCleanupRemovesTheFile(t *testing.T) {
	app := scriptApp(t)

	rel, remove, err := app.writeScript("print(1)", ".py")
	if err != nil {
		t.Fatalf("writeScript: %v", err)
	}
	full := filepath.Join(app.cur().cfg.SandboxRoot, filepath.FromSlash(rel))
	remove()
	if _, err := os.Stat(full); !os.IsNotExist(err) {
		t.Errorf("the script survived its run: %v", err)
	}
}

// A session with no workspace has nowhere the guard would accept, and guessing
// one is how a scratch file ends up somewhere nobody looks.
func TestWriteScriptRefusesWithNoWorkspace(t *testing.T) {
	app := &App{}
	if _, _, err := app.writeScript("print(1)", ".py"); err == nil {
		t.Error("a script was written with no sandbox root")
	}
}
