package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/runlang"
	"github.com/Mike0165115321/Aetox/internal/safety"
)

// The Run button on a source block, end to end through the real shell skill.
//
// Live because the two things most likely to break it are facts about the
// machine rather than about this code: whether an interpreter is on PATH, and
// whether a focused workspace's guardCommandPaths accepts the command line the
// script is named by. Both are exactly what the unit tests cannot see.
func liveScriptApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // never touch the real preference file
	a := &App{}
	a.applyConfig(config.Config{
		SandboxRoot:  t.TempDir(),
		ApprovalMode: string(safety.ApprovalFullAccess),
	})
	return a
}

func TestLiveRunChatScriptPython(t *testing.T) {
	if os.Getenv("AETOX_LIVE_RUN") != "1" {
		t.Skip("set AETOX_LIVE_RUN=1 to run a real interpreter")
	}
	if runlang.Runnable()["python"] != runlang.Script {
		t.Skip("no Python on PATH")
	}
	app := liveScriptApp(t)

	// Multi-line, indented, and with quotes in it — the three things that make
	// "just paste it at the prompt" the wrong answer for source.
	res, err := app.RunChatScript("python", "def f(x):\n    return x ** 2\n\nprint(\"f(4) =\", f(4))\n")
	if err != nil {
		t.Fatalf("RunChatScript: %v", err)
	}
	t.Logf("success=%v output=%q", res.Success, res.Output)
	if !res.Success {
		t.Fatalf("the script did not run: %+v", res)
	}
	if !strings.Contains(res.Output, "f(4) = 16") {
		t.Errorf("output does not carry the script's own answer: %q", res.Output)
	}
}

// JavaScript, end to end. Worth its own live test rather than trusting the
// Python one: node is the runtime that behaves best on Windows — it writes
// UTF-8 whatever the machine's codepage says, where Python had to be told
// (proc.shellEnv) — and this is the assertion that would catch it if that ever
// stopped being true.
func TestLiveRunChatScriptJavaScript(t *testing.T) {
	if os.Getenv("AETOX_LIVE_RUN") != "1" {
		t.Skip("set AETOX_LIVE_RUN=1 to run a real interpreter")
	}
	if runlang.Runnable()["js"] != runlang.Script {
		t.Skip("no node on PATH")
	}
	app := liveScriptApp(t)

	res, err := app.RunChatScript("js", "const f = x => x ** 2\nconsole.log('f(4) =', f(4), '∞ ทดสอบ')\n")
	if err != nil {
		t.Fatalf("RunChatScript: %v", err)
	}
	t.Logf("success=%v output=%q", res.Success, res.Output)
	if !res.Success {
		t.Fatalf("the script did not run: %+v", res)
	}
	if !strings.Contains(res.Output, "f(4) = 16") {
		t.Errorf("output does not carry the script's own answer: %q", res.Output)
	}
	if !strings.Contains(res.Output, "∞ ทดสอบ") {
		t.Errorf("non-ASCII output arrived as mojibake: %q", res.Output)
	}
}

// Go is the entry that proves an interpreter can need arguments of its own:
// `go x.go` is not a command and `go run x.go` is. It also runs where the user
// has no Go project at all — checked live, because "works inside this repo" is
// not the claim.
func TestLiveRunChatScriptGo(t *testing.T) {
	if os.Getenv("AETOX_LIVE_RUN") != "1" {
		t.Skip("set AETOX_LIVE_RUN=1 to run a real interpreter")
	}
	if runlang.Runnable()["go"] != runlang.Script {
		t.Skip("no go on PATH")
	}
	app := liveScriptApp(t)

	res, err := app.RunChatScript("go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"f(4) =\", 4*4) }\n")
	if err != nil {
		t.Fatalf("RunChatScript: %v", err)
	}
	t.Logf("success=%v output=%q", res.Success, res.Output)
	if !res.Success {
		t.Fatalf("the script did not run: %+v", res)
	}
	if !strings.Contains(res.Output, "f(4) = 16") {
		t.Errorf("output does not carry the script's own answer: %q", res.Output)
	}
}

// A failing script is a result the user reads, not a Go error — the traceback
// is the answer to "why didn't it work".
func TestLiveRunChatScriptPythonReportsATraceback(t *testing.T) {
	if os.Getenv("AETOX_LIVE_RUN") != "1" {
		t.Skip("set AETOX_LIVE_RUN=1 to run a real interpreter")
	}
	if runlang.Runnable()["python"] != runlang.Script {
		t.Skip("no Python on PATH")
	}
	app := liveScriptApp(t)

	res, err := app.RunChatScript("python", "raise ValueError('nope')\n")
	if err != nil {
		t.Fatalf("a failing script must not be a Go error: %v", err)
	}
	if res.Success {
		t.Error("a script that raised is not a success")
	}
	if !strings.Contains(res.Output, "ValueError") {
		t.Errorf("the traceback did not come back: %q", res.Output)
	}
}
