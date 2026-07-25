package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/rtk"
)

// isolateAuditLog keeps shellSkill.Execute's unconditional audit.WriteShell
// call (internal/audit) from writing into the real, machine-wide audit log
// every time this test suite runs.
func isolateAuditLog(t *testing.T) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
}

func TestShellSkillRunsCommand(t *testing.T) {
	isolateAuditLog(t)
	s := &shellSkill{root: t.TempDir()}
	out, err := s.Execute(context.Background(), Input{"args": []string{"echo", "hello-shell"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "hello-shell") {
		t.Errorf("Content = %q, want to contain %q", out.Content, "hello-shell")
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
}

func TestShellSkillRunsInSandboxRoot(t *testing.T) {
	isolateAuditLog(t)
	root := t.TempDir()
	s := &shellSkill{root: root}
	// "cd" with no output check needed: a failing command in a bad dir would
	// surface as an error, so a successful run against an arbitrary command
	// confirms cmd.Dir was set to a valid, existing directory.
	if _, err := s.Execute(context.Background(), Input{"args": []string{"echo", "ok"}}); err != nil {
		t.Fatalf("Execute in sandbox root: unexpected error: %v", err)
	}
}

func TestShellSkillCommandFailureReturnsError(t *testing.T) {
	isolateAuditLog(t)
	s := &shellSkill{root: t.TempDir()}
	out, err := s.Execute(context.Background(), Input{"args": []string{"this-command-does-not-exist-xyz"}})
	if err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
	if out.Success {
		t.Error("Success = true, want false on command failure")
	}
}

// The buffer must stop growing mid-command, not after it exits: limitLines
// only runs once the process is done, which is too late for a runaway
// producer to avoid eating RAM for the whole tool timeout.
func TestCappedWriterStopsGrowing(t *testing.T) {
	w := &cappedWriter{}
	chunk := make([]byte, 64*1024)
	for range 40 { // 2.5 MiB offered, well past the 1 MiB cap
		if n, err := w.Write(chunk); n != len(chunk) || err != nil {
			t.Fatalf("Write = (%d, %v), want (%d, nil) — a short write aborts the command", n, err, len(chunk))
		}
	}
	if w.buf.Len() > toolOutputByteCap {
		t.Errorf("buffered %d bytes, want at most %d", w.buf.Len(), toolOutputByteCap)
	}
	if !w.dropped {
		t.Error("dropped = false, want true so the output is reported as truncated")
	}
}

// Quoted arguments are the normal case for a coding agent — `git commit -m
// "msg"`, `python -c "..."`, `grep "a b"`. On Windows they used to arrive
// mangled as \"msg\", because os/exec escapes for the C runtime convention
// that cmd.exe does not follow (proc.ShellCommand). Nothing caught it until
// a process-tree test tried to run a quoted path.
func TestShellSkillPreservesQuotedArguments(t *testing.T) {
	isolateAuditLog(t)
	s := &shellSkill{root: t.TempDir()}

	for _, tc := range []struct{ command, want string }{
		{`echo "hello world"`, "hello world"},
		{`echo "a b" "c d"`, "a b c d"},
	} {
		out, err := s.Execute(context.Background(), Input{"args": []string{tc.command}})
		if err != nil {
			t.Fatalf("Execute(%s): unexpected error: %v", tc.command, err)
		}
		if strings.Contains(out.Content, `\"`) {
			t.Errorf("Execute(%s) = %q, want no backslash-escaped quotes", tc.command, out.Content)
		}
		// cmd's echo keeps the quotes, sh's drops them; either is fine, an
		// injected backslash is not.
		if got := strings.ReplaceAll(out.Content, `"`, ""); got != tc.want {
			t.Errorf("Execute(%s) = %q, want %q ignoring quotes", tc.command, got, tc.want)
		}
	}
}

const heartbeatEnv = "AETOX_TEST_HEARTBEAT"

// TestHeartbeatHelper is not a test. It is the grandchild process that
// TestShellSkillCancelKillsGrandchild starts through the shell, and it only
// does anything when that parent points heartbeatEnv at a file.
func TestHeartbeatHelper(t *testing.T) {
	path := os.Getenv(heartbeatEnv)
	if path == "" {
		t.Skip("helper process, driven by TestShellSkillCancelKillsGrandchild")
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_ = os.WriteFile(path, []byte(time.Now().Format(time.RFC3339Nano)), 0o600)
		time.Sleep(150 * time.Millisecond)
	}
}

// The real check behind proc.KillOnCancel: exec.CommandContext kills the
// shell Aetox spawned and nothing the shell itself started, so pressing Stop
// during `npm install` used to leave node running. A unit test can't see
// that — only a grandchild that keeps touching a file after the cancel can.
func TestShellSkillCancelKillsGrandchild(t *testing.T) {
	isolateAuditLog(t)
	root := t.TempDir()
	beat := filepath.Join(root, "beat.txt")
	t.Setenv(heartbeatEnv, beat)

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test binary to use as the grandchild: %v", err)
	}

	s := &shellSkill{root: root}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var out Output
	go func() {
		defer close(done)
		// Plain shell quoting, not strconv.Quote — that escapes backslashes
		// Go-literal style and hands cmd.exe a path it cannot resolve.
		out, _ = s.Execute(ctx, Input{"args": []string{`"` + self + `"`, "-test.run", "TestHeartbeatHelper"}})
	}()

	// Don't cancel until the grandchild is provably alive, or the test passes
	// for the wrong reason.
	if !waitUntil(10*time.Second, func() bool { _, err := os.Stat(beat); return err == nil }) {
		cancel()
		<-done
		t.Fatalf("grandchild never produced a heartbeat — the test never got to the thing it checks. shell said: %q / %q", out.Content, out.Stderr)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Execute never returned after cancel — WaitDelay is not releasing the output pipe")
	}

	// A straggler write can land while the kill is in flight; sample after it.
	time.Sleep(time.Second)
	before := beatStamp(t, beat)
	time.Sleep(1500 * time.Millisecond)
	if after := beatStamp(t, beat); after != before {
		t.Errorf("heartbeat still advancing after cancel (%q → %q): the grandchild outlived the shell", before, after)
	}
}

func waitUntil(limit time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func beatStamp(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading heartbeat: %v", err)
	}
	return string(data)
}

// resolveSandboxPath gained two EvalSymlinks walks per call (the symlink
// containment fix). It runs at most twice per tool call — never inside
// grep/fs-find's WalkDir — so this exists to keep that assumption honest.
func BenchmarkResolveSandboxPath(b *testing.B) {
	root := b.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0o755); err != nil {
		b.Fatalf("setup: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := resolveSandboxPath(root, "a/b/c/file.txt"); err != nil {
			b.Fatalf("resolveSandboxPath: %v", err)
		}
	}
}

func TestShellSkillMissingArgs(t *testing.T) {
	isolateAuditLog(t)
	s := &shellSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": nil}); err == nil {
		t.Fatal("expected usage error for empty command, got nil")
	}
}

// TestShellSkillRewritesToRTKWhenAvailable exercises the actual integration
// seam (ARCHITECTURE.md §13): shell.go substituting execLine with rtk's
// rewritten command, not internal/rtk's own Rewrite logic (already covered by
// internal/rtk/rtk_test.go).
func TestShellSkillRewritesToRTKWhenAvailable(t *testing.T) {
	// Guard on the rewrite actually existing, not merely on the binary being
	// resolvable: Rewrite legitimately returns ok=false when rtk has no
	// equivalent for a command, and the old `rtk.Available()` guard turned
	// that normal outcome into a red build (CI, 2026-07-25).
	if _, ok := rtk.Rewrite(context.Background(), "git status"); !ok {
		t.Skip("rtk has no rewrite for `git status` here (not installed, or no equivalent)")
	}
	isolateAuditLog(t)
	root := initGitRepo(t)
	s := &shellSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"git", "status"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false, output: %q", out.Content)
	}
	// rtk's own compact git-status filter collapses a clean tree to one short
	// line ("clean — nothing to commit"), unlike plain git's multi-line
	// porcelain-style output ("On branch ..."). This shape difference is what
	// proves the command was actually substituted, not just that Rewrite()
	// works in isolation.
	if strings.Contains(out.Content, "On branch") {
		t.Errorf("expected RTK-rewritten compact output, got plain git output: %q", out.Content)
	}
	// The recorded command must stay the ORIGINAL, never the rtk-substituted
	// one (ARCHITECTURE.md §13: approval/audit always see the real command).
	if !strings.Contains(out.Command, "git status") || strings.Contains(out.Command, "rtk") {
		t.Errorf("Command = %q, want to contain original \"git status\" and not \"rtk\"", out.Command)
	}
}
