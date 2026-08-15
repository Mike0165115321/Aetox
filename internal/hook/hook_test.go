package hook

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// script writes a platform-appropriate one-liner. Every test below runs a real
// child process, because the whole feature is "run the user's command" and a
// mocked runner would test the mock. The Windows variant is PowerShell, the
// shell hooks actually run in since §111 — these tests were the first "user
// hook written in cmd dialect" to break, exactly as that entry predicted.
func script(t *testing.T, body, psBody string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return psBody
	}
	return body
}

func runnerFor(t *testing.T, hooks ...Hook) *Runner {
	t.Helper()
	return NewRunner(Config{Hooks: hooks}, t.TempDir())
}

func TestPreToolUseBlocksOnNonZeroExit(t *testing.T) {
	r := runnerFor(t, Hook{
		Event:    PreToolUse,
		Matcher:  "shell",
		Blocking: true,
		Command:  script(t, `echo "no shell today" && exit 1`, `echo "no shell today"; exit 1`),
	})

	d := r.Run(context.Background(), PreToolUse, "shell", map[string]any{"command": "rm -rf /"}, nil)
	if !d.Blocked {
		t.Fatal("a blocking hook that exited non-zero did not block the call")
	}
	if !strings.Contains(d.Reason, "no shell today") {
		t.Errorf("Reason = %q — the model needs to read why, or it just calls again", d.Reason)
	}
}

// The default. A hook is usually a formatter or a notifier, and one that starts
// silently refusing work because it returned 1 is worse than no hook at all.
func TestNonBlockingHookFailureDoesNotStopTheTool(t *testing.T) {
	r := runnerFor(t, Hook{
		Event:   PreToolUse,
		Matcher: "*",
		Command: script(t, `exit 3`, `exit 3`),
	})

	if d := r.Run(context.Background(), PreToolUse, "write", nil, nil); d.Blocked {
		t.Error("a non-blocking hook blocked the call")
	}
}

func TestMatcherSelectsTools(t *testing.T) {
	r := runnerFor(t, Hook{
		Event:    PreToolUse,
		Matcher:  "github_*",
		Blocking: true,
		Command:  script(t, `exit 1`, `exit 1`),
	})

	if d := r.Run(context.Background(), PreToolUse, "github_search", nil, nil); !d.Blocked {
		t.Error("github_search did not match github_*")
	}
	if d := r.Run(context.Background(), PreToolUse, "read", nil, nil); d.Blocked {
		t.Error("read matched github_* — the glob is too loose")
	}
}

// A hook has to be able to see what it is deciding about, or every hook is
// "block all shell" and nothing finer.
func TestHookReceivesTheCallOnStdinAndInTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "seen.txt")
	// Both channels at once: the env var, and stdin.
	body := `printf '%s|' "$AETOX_TOOL" > ` + out + ` && cat >> ` + out
	psBody := `Set-Content -NoNewline -Path "` + out + `" -Value "$env:AETOX_TOOL|"; [Console]::In.ReadToEnd() | Add-Content -Path "` + out + `"`
	r := NewRunner(Config{Hooks: []Hook{{
		Event: PreToolUse, Matcher: "*", Command: script(t, body, psBody),
	}}}, dir)

	r.Run(context.Background(), PreToolUse, "write", map[string]any{"path": "a.go"}, nil)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the hook did not run: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "write") {
		t.Errorf("AETOX_TOOL did not reach the hook: %q", got)
	}
	if !strings.Contains(got, `"path"`) || !strings.Contains(got, "a.go") {
		t.Errorf("the call arguments did not reach the hook on stdin: %q", got)
	}
	// And what arrived is parseable, not a Go map printed with %v.
	if i := strings.Index(got, "{"); i >= 0 {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(got[i:]), &parsed); err != nil {
			t.Errorf("stdin payload is not valid JSON (%v): %q", err, got[i:])
		}
	}
}

// PostToolUse must fire on failure too: "tell me when a command fails" is the
// same hook point as "run my formatter after a write".
func TestPostToolUseSeesTheOutcome(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "ok.txt")
	body := `printf '%s' "$AETOX_TOOL_OK" > ` + out
	psBody := `Set-Content -NoNewline -Path "` + out + `" -Value "$env:AETOX_TOOL_OK"`
	r := NewRunner(Config{Hooks: []Hook{{
		Event: PostToolUse, Matcher: "*", Command: script(t, body, psBody),
	}}}, dir)

	r.Run(context.Background(), PostToolUse, "shell", nil, &Result{OK: false, Output: "boom"})

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the post hook did not run: %v", err)
	}
	if strings.TrimSpace(string(data)) != "0" {
		t.Errorf("AETOX_TOOL_OK = %q, want 0 for a failed call", string(data))
	}
}

// A PostToolUse hook cannot block: the tool has already run, and pretending
// otherwise would be a lie about what the model is reading.
func TestPostToolUseCannotBlock(t *testing.T) {
	r := runnerFor(t, Hook{
		Event: PostToolUse, Matcher: "*", Blocking: true,
		Command: script(t, `exit 1`, `exit 1`),
	})
	if d := r.Run(context.Background(), PostToolUse, "write", nil, &Result{OK: true}); d.Blocked {
		t.Error("a PostToolUse hook blocked something that had already happened")
	}
}

func TestAnySkipsWorkWhenNothingIsConfigured(t *testing.T) {
	var nilRunner *Runner
	if nilRunner.Any(PreToolUse) {
		t.Error("a nil runner claims to have hooks")
	}
	if d := nilRunner.Run(context.Background(), PreToolUse, "shell", nil, nil); d.Blocked {
		t.Error("a nil runner blocked a call")
	}
	r := runnerFor(t, Hook{Event: PostToolUse, Matcher: "*", Command: "echo hi"})
	if r.Any(PreToolUse) {
		t.Error("Any(PreToolUse) is true with only a PostToolUse hook configured")
	}
	if !r.Any(PostToolUse) {
		t.Error("Any(PostToolUse) is false with one configured")
	}
}

func TestLoadDefaultsAndMissingFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.json")
	if cfg, err := Load(missing); err != nil || len(cfg.Hooks) != 0 {
		t.Errorf("a missing hooks file must be empty and not an error: %v %+v", err, cfg)
	}

	path := filepath.Join(dir, "hooks.json")
	// No event and no matcher — the shape someone writes without reading docs.
	if err := os.WriteFile(path, []byte(`{"hooks":[{"command":"echo hi"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Hooks) != 1 {
		t.Fatalf("got %d hooks", len(cfg.Hooks))
	}
	// A hook with no event is far more likely to be a guard than a notifier.
	if cfg.Hooks[0].Event != PreToolUse {
		t.Errorf("Event = %q, want the PreToolUse default", cfg.Hooks[0].Event)
	}
	if cfg.Hooks[0].Matcher != "*" {
		t.Errorf("Matcher = %q, want the everything default", cfg.Hooks[0].Matcher)
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*", "anything", true},
		{"", "anything", true},
		{"shell", "shell", true},
		{"shell", "shell_output", false},
		{"shell*", "shell_output", true},
		{"github_*", "github_search", true},
		{"github_*", "git", false},
		{"*_output", "shell_output", true},
		{"?ead", "read", true},
		{"?ead", "spread", false},
	}
	for _, c := range cases {
		if got := globMatch(c.pattern, c.name); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}
