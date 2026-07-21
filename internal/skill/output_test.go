package skill

import (
	"errors"
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
	if !strings.HasPrefix(got, "1\n2\n3") || !strings.HasSuffix(got, "(truncated)") {
		t.Errorf("limitLines(3) = %q, want first 3 lines + truncation marker", got)
	}
}
