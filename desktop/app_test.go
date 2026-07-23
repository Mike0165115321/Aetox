package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
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

// The context meter must add up: slices (minus free) sum to usedTokens, free
// fills the remainder, and an unconfigured context max falls back to the
// agent's char budget instead of reporting 0 (which would hide the meter).
func TestGetContextBreakdownSumsAndFallsBackToAgentBudget(t *testing.T) {
	agent := cognitive.NewAgent(cognitive.AgentConfig{
		SystemPrompt: "you are a test system prompt",
	})
	agent.RestoreHistory([]model.Message{
		{Role: model.RoleUser, Content: "hello there"},
		{Role: model.RoleAssistant, Content: "hi, how can I help?"},
	})
	a := &App{agent: agent}

	got := a.GetContextBreakdown()
	if got.MaxTokens <= 0 {
		t.Fatalf("MaxTokens = %d, want agent char-budget fallback > 0", got.MaxTokens)
	}
	// With a known model the meter must show the model's real window, not the
	// engine's internal char budget (the "32k for a 1M model" bug).
	a.cfg.ModelProvider = "deepseek"
	a.cfg.ModelName = "deepseek-v4-flash"
	if got := a.GetContextBreakdown(); got.MaxTokens != 1_000_000 {
		t.Errorf("deepseek-v4-flash MaxTokens = %d, want 1000000", got.MaxTokens)
	}
	a.cfg.ModelContextTokens = 42_000 // explicit user override wins over catalog
	if got := a.GetContextBreakdown(); got.MaxTokens != 42_000 {
		t.Errorf("override MaxTokens = %d, want 42000", got.MaxTokens)
	}
	a.cfg = config.Config{}
	sum, free := 0, 0
	for _, s := range got.Slices {
		if s.Key == "free" {
			free = s.Tokens
			continue
		}
		sum += s.Tokens
		if s.Tokens < 0 {
			t.Errorf("slice %s has negative tokens %d", s.Key, s.Tokens)
		}
	}
	if sum != got.UsedTokens {
		t.Errorf("slices sum to %d, want UsedTokens %d", sum, got.UsedTokens)
	}
	if want := got.MaxTokens - got.UsedTokens; free != want {
		t.Errorf("free = %d, want %d", free, want)
	}
	if got.UsedTokens <= 0 {
		t.Error("expected non-zero usage from system prompt + history")
	}
}

// browser_open must not stamp https:// onto URLs that already have a scheme —
// the old ^https?://-only check turned file:///C:/x.html into
// https://file:///C:/x.html, a permanently blank tab.
func TestNormalizeWorkbenchURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"file:///C:/Users/x/index.html", "file:///C:/Users/x/index.html"},
		{"FILE:///C:/a.html", "FILE:///C:/a.html"},
		{`C:\Users\x\a.html`, "file:///C:/Users/x/a.html"},
		{"E:/site/index.html", "file:///E:/site/index.html"},
		{"https://example.com", "https://example.com"},
		{"http://example.com", "http://example.com"},
		{"about:blank", "about:blank"},
		{"example.com", "https://example.com"},
		{"localhost:5173", "https://localhost:5173"},
	}
	for _, c := range cases {
		if got := normalizeWorkbenchURL(c.in); got != c.want {
			t.Errorf("normalizeWorkbenchURL(%q) = %q, want %q", c.in, got, c.want)
		}
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
