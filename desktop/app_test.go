package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/safety"
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

	a := &App{cfg: config.Config{SandboxRoot: root}, projectFocused: true}
	got := a.GitChangedFiles()
	if len(got) != 1 || got[0].Path != "new.txt" || got[0].Status != "U" {
		t.Errorf("GitChangedFiles() = %+v, want one untracked entry for new.txt", got)
	}
}

func TestGitChangedFilesEmptyWhenUnfocused(t *testing.T) {
	a := &App{cfg: config.Config{SandboxRoot: t.TempDir()}} // projectFocused: false
	if got := a.GitChangedFiles(); len(got) != 0 {
		t.Errorf("GitChangedFiles() unfocused = %v, want empty", got)
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

	a := &App{cfg: config.Config{SandboxRoot: root}, projectFocused: true}
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

// Regression test: resolveConfig used to merge every ModelPreference field
// (provider, model, baseURL, thinkLevel, API key) back onto Config except
// ApprovalMode, so persistModelPreference's own saved value was silently
// discarded on the next startup/OpenProjectFolder call — see
// desktop/app.go's resolveConfig.
func TestResolveConfigLoadsApprovalModeFromPreference(t *testing.T) {
	t.Setenv("AppData", t.TempDir())
	pref := config.ModelPreference{ApprovalMode: string(safety.ApprovalFullAccess)}
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// opts explicitly asks for a *different* mode — the saved preference must win.
	cfg := resolveConfig(config.ConfigOptions{ApprovalMode: string(safety.ApprovalAsk)})
	if cfg.ApprovalMode != string(safety.ApprovalFullAccess) {
		t.Errorf("ApprovalMode = %q, want %q (saved preference should override the passed-in default)", cfg.ApprovalMode, safety.ApprovalFullAccess)
	}
}

func TestResolveConfigKeepsOptsApprovalModeWithNoSavedPreference(t *testing.T) {
	t.Setenv("AppData", t.TempDir()) // empty dir — no preference file exists

	cfg := resolveConfig(config.ConfigOptions{ApprovalMode: string(safety.ApprovalUnsafeOnly)})
	if cfg.ApprovalMode != string(safety.ApprovalUnsafeOnly) {
		t.Errorf("ApprovalMode = %q, want %q (opts value should stand when nothing is saved)", cfg.ApprovalMode, safety.ApprovalUnsafeOnly)
	}
}

func TestSaveChatImageCopiesIntoSandbox(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(src, []byte("fake png bytes"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	a := &App{cfg: config.Config{SandboxRoot: root}}

	rel, err := a.SaveChatImage(src)
	if err != nil {
		t.Fatalf("SaveChatImage: unexpected error: %v", err)
	}
	if filepath.Ext(rel) != ".png" {
		t.Errorf("SaveChatImage relPath = %q, want a .png extension preserved", rel)
	}

	full := filepath.Join(root, filepath.FromSlash(rel))
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("copied file not found at %q: %v", full, err)
	}
	if string(got) != "fake png bytes" {
		t.Errorf("copied content = %q, want %q", got, "fake png bytes")
	}
}

func TestSaveChatImageRejectsOversized(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "huge.png")
	f, err := os.Create(src)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := f.Truncate(21 << 20); err != nil { // 21MB > the 20MB cap
		t.Fatalf("setup: %v", err)
	}
	f.Close()

	a := &App{cfg: config.Config{SandboxRoot: root}}
	if _, err := a.SaveChatImage(src); err == nil {
		t.Error("expected an error for an oversized image, got nil")
	}
}

func TestSaveChatImageNoProjectOpen(t *testing.T) {
	a := &App{}
	if _, err := a.SaveChatImage("whatever.png"); err == nil {
		t.Error("expected an error with no project open, got nil")
	}
}
