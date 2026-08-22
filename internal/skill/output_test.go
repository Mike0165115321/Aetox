package skill

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNewToolOutputSuccess(t *testing.T) {
	out := newToolOutput("read", "read a.txt", "hi", time.Now(), false, nil)
	if !out.Success || out.Stderr != "" {
		t.Errorf("Success=%v Stderr=%q, want success with no stderr", out.Success, out.Stderr)
	}
	if out.Content != "hi" || out.RawOutput != "hi" {
		t.Errorf("Content=%q RawOutput=%q, want both %q", out.Content, out.RawOutput, "hi")
	}
}

func TestNewToolOutputEmptyContentFilled(t *testing.T) {
	out := newToolOutput("x", "x", "", time.Now(), false, nil)
	if out.Content != "(no output)" {
		t.Errorf("Content = %q, want %q", out.Content, "(no output)")
	}
}

func TestNewToolOutputError(t *testing.T) {
	out := newToolOutput("x", "x", "", time.Now(), false, errors.New("boom"))
	if out.Success {
		t.Error("Success = true, want false on error")
	}
	if out.Stderr != "boom" {
		t.Errorf("Stderr = %q, want %q", out.Stderr, "boom")
	}
}

func TestLimitLinesUnderLimit(t *testing.T) {
	content := "a\nb\nc"
	got, truncated := limitLines(content, 10)
	if truncated || got != content {
		t.Errorf("limitLines under limit = (%q, %v), want (%q, false)", got, truncated, content)
	}
}

func TestLimitLinesOverLimit(t *testing.T) {
	content := "1\n2\n3\n4\n5"
	got, truncated := limitLines(content, 3)
	if !truncated {
		t.Fatal("expected truncated = true")
	}
	if !strings.HasPrefix(got, "1\n2\n3") {
		t.Errorf("limitLines(3) = %q, want the first 3 lines kept", got)
	}
	// Both numbers, not just the word. "(truncated)" alone cannot tell a result
	// that lost its last line from one that lost two thirds of itself, and the
	// nineteen tools that share this helper all used to say only that.
	if !strings.Contains(got, "3 of 5 lines") {
		t.Errorf("limitLines(3) = %q, want it to say how many of how many", got)
	}
}

// A command's verdict is in its last few lines, so the cut that keeps only the
// head throws away the part you ran the command for. `go test ./...` opens with
// a wall of `ok` and closes with FAIL and a count; a build opens with chatter
// and closes with the error.
func TestKeepingEndsSavesTheVerdict(t *testing.T) {
	lines := make([]string, 600)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	lines[599] = "FAIL: 3 tests failed"
	got, truncated := limitLinesKeepingEnds(strings.Join(lines, "\n"), 100)

	if !truncated {
		t.Fatal("600 lines into a 100-line ceiling must report truncation")
	}
	if !strings.Contains(got, "line 1\n") {
		t.Error("the head is gone; a reader needs to see what the command was doing")
	}
	// The whole point of this function.
	if !strings.Contains(got, "FAIL: 3 tests failed") {
		t.Errorf("the last line was dropped — that is the verdict, got tail:\n%s", got[len(got)-120:])
	}
	if !strings.Contains(got, "500 lines from the middle") {
		t.Errorf("the marker must say how much of the middle went, got:\n%s", got)
	}
	// Half and half, so neither end is guessed at.
	if !strings.Contains(got, "the last 50 of 600 follow") {
		t.Errorf("expected an even head/tail split reported honestly, got:\n%s", got)
	}
}

func TestKeepingEndsLeavesShortOutputAlone(t *testing.T) {
	content := "one\ntwo\nthree"
	got, truncated := limitLinesKeepingEnds(content, 100)
	if truncated || got != content {
		t.Errorf("output inside the ceiling must come back untouched, got %q", got)
	}
}
