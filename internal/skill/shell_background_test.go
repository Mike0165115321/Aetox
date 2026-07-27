package skill

import (
	"context"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// backgroundShellTools builds the three tools the way RegisterDefaults does —
// sharing one registry of jobs, which is the whole point of the arrangement.
func backgroundShellTools(t *testing.T) (*shellSkill, *shellOutputSkill, *shellKillSkill) {
	t.Helper()
	isolateAuditLog(t)
	shells := newBackgroundShells()
	return &shellSkill{root: t.TempDir(), shells: shells},
		&shellOutputSkill{shells: shells},
		&shellKillSkill{shells: shells}
}

var backgroundIDRe = regexp.MustCompile(`\bbg_\d+\b`)

func startBackground(t *testing.T, s *shellSkill, command string) string {
	t.Helper()
	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"command":           command,
		"run_in_background": true,
	})
	if err != nil {
		t.Fatalf("start %q: %v", command, err)
	}
	id := backgroundIDRe.FindString(out.Content)
	if id == "" {
		t.Fatalf("no handle in %q — the model has no way to reach the command it just started", out.Content)
	}
	return id
}

// outputBody strips the status header. The header repeats the command line, so
// asserting against the whole result matches words the command never printed —
// which is what made the first version of these tests lie.
func outputBody(content string) string {
	if _, body, ok := strings.Cut(content, "\n\n"); ok {
		return body
	}
	return content
}

// waitForOutput polls the way the model does: shell_output is not a promise
// that anything has been printed yet, so "nothing yet, ask again" is a normal
// answer and the test has to tolerate it exactly as the model must.
func waitForOutput(t *testing.T, o *shellOutputSkill, id, want string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var seen strings.Builder
	for time.Now().Before(deadline) {
		out, err := o.ExecuteTool(context.Background(), map[string]any{"shell_id": id})
		if err != nil {
			t.Fatalf("shell_output(%s): %v", id, err)
		}
		body := outputBody(out.Content)
		seen.WriteString(body)
		if strings.Contains(body, want) {
			return body
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("never saw %q from %s; collected:\n%s", want, id, seen.String())
	return ""
}

// The command shell was previously unable to run at all: one that keeps
// printing and never exits. Foreground it would run out the 60s clock and be
// killed with nothing to show for it.
func TestBackgroundShellRunsPastTheTurn(t *testing.T) {
	s, o, k := backgroundShellTools(t)

	command := `for /L %i in (1,0,2) do @(echo tick & ping -n 2 127.0.0.1 >nul)`
	if runtime.GOOS != "windows" {
		command = `while true; do echo tick; sleep 1; done`
	}
	id := startBackground(t, s, command)

	waitForOutput(t, o, id, "tick")

	// Still running is the point — a foreground call would have had to wait for
	// an exit that never comes.
	out, err := o.ExecuteTool(context.Background(), map[string]any{"shell_id": id})
	if err != nil {
		t.Fatalf("shell_output: %v", err)
	}
	if !strings.Contains(out.Content, "running for") {
		t.Errorf("status = %q, want it reported as still running", firstLineOf(out.Content))
	}

	killed, err := k.ExecuteTool(context.Background(), map[string]any{"shell_id": id})
	if err != nil {
		t.Fatalf("shell_kill: %v", err)
	}
	if !killed.Success {
		t.Errorf("kill reported failure: %q", killed.Content)
	}

	// And the kill is real: after it, the job stops calling itself running.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := o.ExecuteTool(context.Background(), map[string]any{"shell_id": id})
		if strings.Contains(out.Content, "killed after") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("the command was never reported as killed")
}

// Output is consumed: the second read returns what arrived after the first, not
// the whole log again. Without this a model polling a chatty server pays for
// every line once per poll.
func TestBackgroundShellOutputIsIncremental(t *testing.T) {
	s, o, _ := backgroundShellTools(t)

	command := `echo first & ping -n 2 127.0.0.1 >nul & echo second`
	if runtime.GOOS != "windows" {
		command = `echo first; sleep 1; echo second`
	}
	id := startBackground(t, s, command)

	waitForOutput(t, o, id, "first")
	second := waitForOutput(t, o, id, "second")
	if strings.Contains(second, "first") {
		t.Errorf("the second read replayed the first read's output:\n%s", second)
	}
}

func TestBackgroundShellFilterKeepsOnlyMatchingLines(t *testing.T) {
	s, o, _ := backgroundShellTools(t)

	command := `echo keep-me & echo drop-this`
	if runtime.GOOS != "windows" {
		command = `echo keep-me; echo drop-this`
	}
	id := startBackground(t, s, command)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		out, err := o.ExecuteTool(context.Background(), map[string]any{"shell_id": id, "filter": "keep"})
		if err != nil {
			t.Fatalf("shell_output: %v", err)
		}
		body := outputBody(out.Content)
		if strings.Contains(body, "keep-me") {
			if strings.Contains(body, "drop-this") {
				t.Errorf("filter let a non-matching line through:\n%s", body)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("never saw the matching line")
}

// A handle the model invented, or one from a previous session, must come back
// as a readable error that says what IS running — otherwise a mistyped id
// leaves a real process unreachable.
func TestBackgroundShellUnknownHandle(t *testing.T) {
	_, o, k := backgroundShellTools(t)

	_, err := o.ExecuteTool(context.Background(), map[string]any{"shell_id": "bg_999"})
	if err == nil || !strings.Contains(err.Error(), "nothing is running") {
		t.Errorf("shell_output on an unknown handle: err = %v, want it to say what is running", err)
	}
	_, err = k.ExecuteTool(context.Background(), map[string]any{"shell_id": "bg_999"})
	if err == nil {
		t.Error("shell_kill on an unknown handle must fail rather than report a kill that never happened")
	}
}

func TestBackgroundShellCapsConcurrentJobs(t *testing.T) {
	s, _, _ := backgroundShellTools(t)

	command := `ping -n 60 127.0.0.1 >nul`
	if runtime.GOOS != "windows" {
		command = `sleep 60`
	}
	for i := 0; i < maxBackgroundShells; i++ {
		startBackground(t, s, command)
	}
	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"command":           command,
		"run_in_background": true,
	})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("the %dth background command was accepted: err = %v", maxBackgroundShells+1, err)
	}
	// Leaving them running would outlive the test; the job object handles the
	// app, but a test should clean up after itself.
	for _, id := range s.shells.running() {
		if job, ok := s.shells.get(id); ok {
			job.cancel()
		}
	}
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
