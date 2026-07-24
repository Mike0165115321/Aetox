package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSkillExecute(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi there"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"hello.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Content != "hi there" {
		t.Errorf("Content = %q, want %q", out.Content, "hi there")
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
}

func TestReadSkillMissingFile(t *testing.T) {
	s := &readSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"does-not-exist.txt"}})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestReadSkillRejectsEscape(t *testing.T) {
	s := &readSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"../outside.txt"}})
	if err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestReadSkillEmptyFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), []byte(""), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"empty.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Content != "(empty file)" {
		t.Errorf("Content = %q, want %q", out.Content, "(empty file)")
	}
}

func TestReadSkillDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}
	_, err := s.Execute(context.Background(), Input{"args": []string{"subdir"}})
	if err == nil {
		t.Fatal("expected error reading a directory, got nil")
	}
}

// The old flat 16KB ceiling hid the tail of any real source file. Paging must
// hand back the requested window and say where to resume.
func TestReadSkillPagesByLine(t *testing.T) {
	root := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i+1)
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"big.txt"}, "offset": 10, "limit": 3})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.HasPrefix(out.Content, "line-10\nline-11\nline-12") {
		t.Errorf("Content = %q, want to start at line-10 and hold 3 lines", out.Content)
	}
	if !out.Truncated || !strings.Contains(out.Content, "offset=13") {
		t.Errorf("Content = %q, truncated = %v, want a resume hint at offset=13", out.Content, out.Truncated)
	}

	// The last page ends cleanly — no truncation marker, nothing hidden.
	out, err = s.Execute(context.Background(), Input{"args": []string{"big.txt"}, "offset": 98, "limit": 3})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Truncated || !strings.HasSuffix(out.Content, "line-100") {
		t.Errorf("Content = %q, truncated = %v, want the file's tail with no truncation", out.Content, out.Truncated)
	}

	// A file far past the old 16KB cap must come back whole by default.
	fat := strings.Repeat(strings.Repeat("x", 79)+"\n", 800) // 800 lines, ~64KB
	if err := os.WriteFile(filepath.Join(root, "fat.txt"), []byte(fat), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	out, err = s.Execute(context.Background(), Input{"args": []string{"fat.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Truncated || len(out.Content) < len(fat)-1 {
		t.Errorf("50KB file: len(Content) = %d, truncated = %v, want the whole file", len(out.Content), out.Truncated)
	}
}

// JSON hands numbers over as float64; a model that quotes them must work too.
func TestReadSkillExecuteToolOffsetTypes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	for _, offset := range []any{float64(2), "2", 2} {
		out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "a.txt", "offset": offset, "limit": float64(1)})
		if err != nil {
			t.Fatalf("ExecuteTool(offset=%v): unexpected error: %v", offset, err)
		}
		if !strings.HasPrefix(out.Content, "two") {
			t.Errorf("ExecuteTool(offset=%v) = %q, want to start at %q", offset, out.Content, "two")
		}
	}
}

func TestReadSkillExecuteTool(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Error("ExecuteTool with no path: expected error, got nil")
	}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "a.txt"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "content") {
		t.Errorf("Content = %q, want to contain %q", out.Content, "content")
	}
}
