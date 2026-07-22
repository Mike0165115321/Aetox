package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepSkillFindsMatchesWithLineNumbers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc TargetFunc() {}\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "TargetFunc"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
	if !strings.Contains(out.Content, "a.go:3:func TargetFunc() {}") {
		t.Errorf("Content = %q, want match with path:line:text", out.Content)
	}
}

func TestGrepSkillNoMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("nothing here"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "absent"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "(no matches)") {
		t.Errorf("Content = %q, want (no matches)", out.Content)
	}
}

func TestGrepSkillScopedToSubdir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "inner.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "path": "sub"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if strings.Contains(out.Content, "top.txt") {
		t.Errorf("Content = %q, should not include files outside sub/", out.Content)
	}
	if !strings.Contains(out.Content, "sub/inner.txt:1:needle") {
		t.Errorf("Content = %q, want sub/inner.txt match", out.Content)
	}
}

func TestGrepSkillSkipsDotDirsAndBinary(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte("needle\x00"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "(no matches)") {
		t.Errorf("Content = %q, want dot-dir and binary skipped", out.Content)
	}
}

func TestGrepSkillExecuteCLIPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"needle", "."}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt:1:needle") {
		t.Errorf("Content = %q, want a.txt:1:needle", out.Content)
	}
}

func TestGrepSkillCapsResults(t *testing.T) {
	root := t.TempDir()
	lines := strings.Repeat("needle\n", 250)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(lines), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !out.Truncated {
		t.Error("Truncated = false, want true at result cap")
	}
	if !strings.Contains(out.Content, "(max results reached)") {
		t.Errorf("Content missing max-results marker: %q", out.Content[len(out.Content)-100:])
	}
}

func TestGrepSkillTruncatesLongLines(t *testing.T) {
	root := t.TempDir()
	long := "needle " + strings.Repeat("x", 500)
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(long), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	for _, line := range strings.Split(out.Content, "\n") {
		if len(line) > 250 {
			t.Errorf("line length %d exceeds truncation cap: %q...", len(line), line[:80])
		}
	}
	if !strings.Contains(out.Content, "...") {
		t.Errorf("Content = %q, want truncated line marker", out.Content)
	}
}

func TestGrepSkillCaseInsensitiveFlag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("NeeDLe"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "(?i)needle"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt:1:NeeDLe") {
		t.Errorf("Content = %q, want case-insensitive match", out.Content)
	}
}

func TestGrepSkillSingleFileTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "only.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "path": "only.txt"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "only.txt:1:needle") || strings.Contains(out.Content, "other.txt") {
		t.Errorf("Content = %q, want only.txt match only", out.Content)
	}
}

func TestGrepSkillRejectsBadRegex(t *testing.T) {
	s := &grepSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "("}); err == nil {
		t.Fatal("expected regex compile error, got nil")
	}
}

func TestGrepSkillRejectsEscape(t *testing.T) {
	s := &grepSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "x", "path": "../outside"}); err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestGrepSkillExecuteToolMissingPattern(t *testing.T) {
	s := &grepSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected error for missing pattern, got nil")
	}
}
