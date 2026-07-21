package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func TestSafeSandboxPathAllowsInside(t *testing.T) {
	root := t.TempDir()
	got, err := safeSandboxPath(root, filepath.Join("sub", "file.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, "sub", "file.txt")
	if got != want {
		t.Errorf("safeSandboxPath = %q, want %q", got, want)
	}
}

func TestSafeSandboxPathRejectsEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sandbox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := safeSandboxPath(root, filepath.Join("..", "outside.txt")); err == nil {
		t.Error("expected error escaping sandbox root, got nil")
	}
}

func TestReadWriteFileRoundTrip(t *testing.T) {
	root := t.TempDir()
	a := &App{cfg: config.Config{SandboxRoot: root}}

	if err := a.WriteFile("note.txt", "hello desktop"); err != nil {
		t.Fatalf("WriteFile: unexpected error: %v", err)
	}
	got, err := a.ReadFile("note.txt")
	if err != nil {
		t.Fatalf("ReadFile: unexpected error: %v", err)
	}
	if got != "hello desktop" {
		t.Errorf("ReadFile = %q, want %q", got, "hello desktop")
	}
}

func TestReadFileRejectsEscape(t *testing.T) {
	root := t.TempDir()
	a := &App{cfg: config.Config{SandboxRoot: root}}
	if _, err := a.ReadFile(filepath.Join("..", "escape.txt")); err == nil {
		t.Error("expected error escaping sandbox root, got nil")
	}
}

func TestReadFileNoProjectOpen(t *testing.T) {
	a := &App{}
	if _, err := a.ReadFile("x.txt"); err == nil {
		t.Error("expected error with no project open, got nil")
	}
}

func TestRelativizePathInsideProject(t *testing.T) {
	root := t.TempDir()
	a := &App{cfg: config.Config{SandboxRoot: root}}
	abs := filepath.Join(root, "sub", "file.txt")
	got, err := a.RelativizePath(abs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "sub/file.txt"; got != want {
		t.Errorf("RelativizePath = %q, want %q", got, want)
	}
}

func TestRelativizePathRejectsOutside(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sandbox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	a := &App{cfg: config.Config{SandboxRoot: root}}
	outside := filepath.Join(filepath.Dir(root), "elsewhere.txt")
	if _, err := a.RelativizePath(outside); err == nil {
		t.Error("expected error for path outside project root, got nil")
	}
}

func TestReadFileRejectsDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	a := &App{cfg: config.Config{SandboxRoot: root}}
	if _, err := a.ReadFile("sub"); err == nil {
		t.Error("expected error reading a directory, got nil")
	}
}

func TestCommandHistoryOrderAndCap(t *testing.T) {
	a := &App{}
	for i := 0; i < maxToolHistory+5; i++ {
		a.recordToolAction("call", "action-"+string(rune('a'+i%26)))
	}
	// "result" events must be ignored.
	a.recordToolAction("result", "should-not-appear")

	hist := a.CommandHistory()
	if len(hist) != maxToolHistory {
		t.Fatalf("len(CommandHistory()) = %d, want %d (capped)", len(hist), maxToolHistory)
	}
	for _, h := range hist {
		if h == "should-not-appear" {
			t.Error("CommandHistory() contains a \"result\" event, want only \"call\" events")
		}
	}
	// Most recent action recorded must come first.
	if hist[0] != a.toolHistory[len(a.toolHistory)-1] {
		t.Errorf("CommandHistory()[0] = %q, want most recent action %q", hist[0], a.toolHistory[len(a.toolHistory)-1])
	}
}

func TestGitChangedFilesOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	a := &App{cfg: config.Config{SandboxRoot: t.TempDir()}}
	got := a.GitChangedFiles()
	if len(got) != 0 {
		t.Errorf("GitChangedFiles() outside a repo = %v, want empty", got)
	}
}

func TestGitChangedFilesDetectsUntracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	a := &App{cfg: config.Config{SandboxRoot: root}}
	got := a.GitChangedFiles()
	if len(got) != 1 || got[0].Path != "new.txt" || got[0].Status != "U" {
		t.Errorf("GitChangedFiles() = %+v, want one untracked entry for new.txt", got)
	}
}

func TestProjectTreeListsFilesAndSkipsIgnored(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "junk.js"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	a := &App{cfg: config.Config{SandboxRoot: root}}
	tree := a.ProjectTree()

	foundMain, foundIgnored := false, false
	for _, n := range tree {
		if n.Path == "main.go" {
			foundMain = true
		}
		if n.Label == "node_modules" {
			foundIgnored = true
		}
	}
	if !foundMain {
		t.Error("ProjectTree() missing main.go")
	}
	if foundIgnored {
		t.Error("ProjectTree() should skip node_modules (treeIgnore)")
	}
}

func TestProjectTreeEmptyRoot(t *testing.T) {
	a := &App{}
	if got := a.ProjectTree(); len(got) != 0 {
		t.Errorf("ProjectTree() with no sandbox root = %v, want empty", got)
	}
}
