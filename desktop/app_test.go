package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/cognitive"
	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// isolateUserDirs points every "where does this user's data live" lookup at a
// temp directory, on every platform.
//
// Setting APPDATA/LOCALAPPDATA alone only isolates Windows. os.UserConfigDir
// reads XDG_CONFIG_HOME on Linux and $HOME/Library/Application Support on
// macOS, so on those platforms these tests were reading and writing the real
// ~/.config/aetox — which is how a "missing file returns nil" test started
// failing the moment another package's tests ran first and left a file there.
func isolateUserDirs(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("APPDATA", base)         // windows: os.UserConfigDir
	t.Setenv("LOCALAPPDATA", base)    // windows: the legacy preference path
	t.Setenv("USERPROFILE", base)     // windows: os.UserHomeDir
	t.Setenv("XDG_CONFIG_HOME", base) // linux: os.UserConfigDir
	t.Setenv("HOME", base)            // linux + macos: os.UserHomeDir, and macOS' config dir
	// And the one that actually decides where this app keeps things.
	//
	// config.DataRoot reads AETOX_DATA_ROOT *before* it asks the OS for a config
	// dir, and TestMain sets it once for the whole package. So every line above
	// was ignored by everything that goes through DataRoot, and each caller here
	// was sharing one directory while reading like it had its own — two MCP
	// tests asserting on an exact server count saw each other's servers and
	// failed under -shuffle, whichever of them ran second.
	//
	// Set here rather than at the call sites for the reason TestMain gives for
	// itself: the trap is invisible where it bites, so the fix belongs in the
	// function whose name is the promise being broken. t.Setenv restores to
	// TestMain's root, never to the developer's real one.
	t.Setenv("AETOX_DATA_ROOT", base)
	return base
}

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
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())

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
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
	if _, err := a.ReadFile(filepath.Join("..", "escape.txt")); err == nil {
		t.Error("expected error escaping sandbox root, got nil")
	}
}

// The context meter must add up: slices (minus free) sum to usedTokens, and
// free fills the remainder of a window we actually know.
//
// This test used to assert the opposite of its last third. It required that an
// unknown window fall back to the agent's char budget over four, "instead of
// reporting 0 (which would hide the meter)" — and that sentence is the whole
// bug, written down and defended. Those are two different facts: the budget is
// how much history Aetox keeps before summarizing, the window is how much the
// model accepts. Codex resolved to unknown, so every Codex user was shown a
// 32,000-token window (memory.defaultMaxChars / 4) on models the catalog puts
// at 1,050,000, while their own token_usage held rounds at 43,434.
//
// Hiding the meter was never the alternative. Dropping the DENOMINATOR is: the
// request's size is known and gets shown, the window is not and does not.
func TestGetContextBreakdownReportsAnUnknownWindowAsUnknown(t *testing.T) {
	agent := cognitive.NewAgent(cognitive.AgentConfig{
		SystemPrompt: "you are a test system prompt",
	})
	agent.RestoreHistory([]model.Message{
		{Role: model.RoleUser, Content: "hello there"},
		{Role: model.RoleAssistant, Content: "hi, how can I help?"},
	})
	a := seed(&App{}, &conversation{agent: agent})

	// No provider configured: nobody knows the window, so nobody claims one.
	unknown := a.GetContextBreakdown()
	if unknown.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d on an unknown model; a number here is invented", unknown.MaxTokens)
	}
	if unknown.UsedTokens <= 0 {
		t.Error("expected non-zero usage from system prompt + history")
	}
	for _, s := range unknown.Slices {
		if s.Key == "free" {
			t.Errorf("free = %d with no window to subtract from; that is a claim about a number nobody has", s.Tokens)
		}
	}

	// With a known model the meter must show the model's real window, not the
	// engine's internal char budget (the "32k for a 1M model" bug).
	a.cur().cfg.ModelProvider = "deepseek"
	a.cur().cfg.ModelName = "deepseek-v4-flash"
	if got := a.GetContextBreakdown(); got.MaxTokens != 1_000_000 {
		t.Errorf("deepseek-v4-flash MaxTokens = %d, want 1000000", got.MaxTokens)
	}
	// The door the whole fix is about: same models, subscription instead of
	// API key, and until 2026-08-18 this line read 32000.
	a.cur().cfg.ModelProvider = "codex"
	a.cur().cfg.ModelName = "gpt-5.5"
	if got := a.GetContextBreakdown(); got.MaxTokens != 400_000 {
		t.Errorf("codex/gpt-5.5 MaxTokens = %d, want OpenAI's 400000", got.MaxTokens)
	}
	a.cur().cfg.ModelContextTokens = 42_000 // explicit user override wins over catalog
	got := a.GetContextBreakdown()
	if got.MaxTokens != 42_000 {
		t.Errorf("override MaxTokens = %d, want 42000", got.MaxTokens)
	}

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
}

// Once a round has been measured, the gap between the provider's count and the
// chars/4 estimate belongs to the conversation alone. The old split scaled all
// three slices by real/used, so the tools bar climbed round after round while
// not one byte of the tool list changed — a meter steadily "proving" that the
// tool block eats the window. And the cache column that was always in the same
// row must come out with it, so a mostly-cached prompt stops presenting as
// fully paid.
func TestGetContextBreakdownPinsFixedSlicesAndReportsCache(t *testing.T) {
	agent := cognitive.NewAgent(cognitive.AgentConfig{
		SystemPrompt: "you are a test system prompt",
	})
	agent.RestoreHistory([]model.Message{
		{Role: model.RoleUser, Content: "hello there"},
		{Role: model.RoleAssistant, Content: "hi, how can I help?"},
	})
	a := seed(&App{dbDir: t.TempDir()}, &conversation{agent: agent, id: "s1"})
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})

	est := a.GetContextBreakdown()
	slices := func(b ContextBreakdown) map[string]int {
		out := map[string]int{}
		for _, s := range b.Slices {
			out[s.Key] = s.Tokens
		}
		return out
	}
	estSlices := slices(est)

	// The provider counted far more than the estimate (Thai text, real
	// tokenizer), and most of it hit the prompt cache.
	a.recordTokenUsage(a.cur(), model.Usage{PromptTokens: 40_000, CachedPromptTokens: 36_000, CacheReported: true, CompletionTokens: 5})

	got := a.GetContextBreakdown()
	if !got.Measured {
		t.Fatal("a session with a recorded round must report Measured")
	}
	if got.UsedTokens != 40_000 {
		t.Errorf("UsedTokens = %d, want the provider's 40000", got.UsedTokens)
	}
	if got.CachedTokens != 36_000 {
		t.Errorf("CachedTokens = %d, want 36000", got.CachedTokens)
	}
	gotSlices := slices(got)
	for _, key := range []string{"system", "tools"} {
		if gotSlices[key] != estSlices[key] {
			t.Errorf("%s slice moved from %d to %d; fixed blocks must not absorb the tokenizer gap", key, estSlices[key], gotSlices[key])
		}
	}
	if want := 40_000 - estSlices["system"] - estSlices["tools"]; gotSlices["messages"] != want {
		t.Errorf("messages = %d, want the whole gap %d", gotSlices["messages"], want)
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
		got, query := normalizeWorkbenchURL(c.in, "", nil)
		if got != c.want {
			t.Errorf("normalizeWorkbenchURL(%q) = %q, want %q", c.in, got, c.want)
		}
		if query != "" {
			t.Errorf("normalizeWorkbenchURL(%q) read an address as a search for %q", c.in, query)
		}
	}
}

// A source file is a download, not a page. WebView2 aborts the navigation and
// the tab reports "not found, or unreachable" — which sends the model looking
// for a path bug when the file was right there.
func TestUnrenderableFile(t *testing.T) {
	refused := []string{
		"file:///C:/proj/test-hello.ts",
		"file:///C:/proj/main.go",
		"file:///C:/proj/style.scss",
	}
	for _, url := range refused {
		why := unrenderableFile(url)
		if why == "" {
			t.Errorf("unrenderableFile(%q) = \"\", want a reason the browser cannot show it", url)
			continue
		}
		if !strings.Contains(why, "read") {
			t.Errorf("the refusal must name what to use instead, got %q", why)
		}
	}

	allowed := []string{
		"file:///C:/proj/index.html",
		"file:///C:/proj/diagram.svg",
		"file:///C:/proj/report.pdf",
		"file:///C:/proj/notes.txt",
		"file:///C:/proj/README", // no extension: let the browser decide
		"https://example.com/app.ts",
		"about:blank",
	}
	for _, url := range allowed {
		if why := unrenderableFile(url); why != "" {
			t.Errorf("unrenderableFile(%q) = %q, want it opened", url, why)
		}
	}
}

// write reports where a file landed as a sandbox-relative path and never the
// absolute one, so browser_open has to accept that same path — otherwise the
// model cannot show the user what it just built without splicing in the
// sandbox root by hand.
func TestNormalizeWorkbenchURLResolvesSandboxRelativePaths(t *testing.T) {
	root := t.TempDir()
	rel := "output/s1/index.html"
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("<h1>a</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	want := "file:///" + strings.ReplaceAll(full, `\`, "/")
	if got, _ := normalizeWorkbenchURL(rel, root, nil); got != want {
		t.Errorf("normalizeWorkbenchURL(%q, root) = %q, want %q", rel, got, want)
	}

	// A bare domain must still reach the web, not be mistaken for a local file.
	if got, _ := normalizeWorkbenchURL("example.com", root, nil); got != "https://example.com" {
		t.Errorf("bare domain = %q, want https://example.com", got)
	}
}

// The path the model asks to open is the path it asked write for — "index.html"
// — but write steers new files into the session output folder, so that name
// resolves to nothing at the root. Resolving only against the root found no
// file, fell through to the bare-domain case, and the workbench went off to do
// a DNS lookup for a host called index.html.
func TestNormalizeWorkbenchURLFindsWhatWriteSteeredIntoTheOutputFolder(t *testing.T) {
	root := t.TempDir()
	subdir := "output/20260726-063257.366"
	outputSubdir := func() string { return subdir }

	full := filepath.Join(root, filepath.FromSlash(subdir), "index.html")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("<h1>a</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	want := "file:///" + strings.ReplaceAll(full, `\`, "/")
	if got, _ := normalizeWorkbenchURL("index.html", root, outputSubdir); got != want {
		t.Errorf("normalizeWorkbenchURL(index.html) = %q, want %q", got, want)
	}
	// The placed path spelled out in full must work too — that is what write
	// reported back, and a model may repeat it verbatim.
	if got, _ := normalizeWorkbenchURL(subdir+"/index.html", root, outputSubdir); got != want {
		t.Errorf("normalizeWorkbenchURL(placed path) = %q, want %q", got, want)
	}
	// Still a domain, not a file, when nothing of that name exists.
	if got, _ := normalizeWorkbenchURL("example.com", root, outputSubdir); got != "https://example.com" {
		t.Errorf("bare domain = %q, want https://example.com", got)
	}
}

// The unfocused sandbox root and the session output folder are two halves of
// one absolute path — <home>/aetox plus output/<session>. Changing either half
// alone either doubles the folder name (<home>/aetox/aetox/output/...) or moves
// every artifact the app has written so far, so they are checked together.
func TestUnfocusedRootAndOutputSubdirComposeToTheSessionFolder(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory on this machine")
	}
	a := seed(&App{}, &conversation{id: "20260726-063257.366"})
	got := filepath.Join(unfocusedRoot(), filepath.FromSlash(a.outputSubdir()))
	if want := filepath.Join(home, "aetox", "output", "20260726-063257.366"); got != want {
		t.Errorf("unfocused output path = %q, want %q", got, want)
	}

	// Focused, there is no output folder at all: the project is the destination.
	a.projectFocused = true
	if sub := a.outputSubdir(); sub != "" {
		t.Errorf("focused on a project, outputSubdir = %q, want empty", sub)
	}
}

// Unfocused mode roams the machine; focusing a project restores the wall.
// End-to-end through applyConfig because OpenSandbox rides the bootstrap —
// a regression in the wiring, not just in resolveSandboxPath, must fail this.
func TestUnfocusedEngineRoamsFocusedEngineStaysWalled(t *testing.T) {
	isolateUserDirs(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "stray.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}

	listOutside := func(a *App) (skill.Output, error) {
		t.Helper()
		s, ok := a.cur().registry.Get("list")
		if !ok {
			t.Fatal("no list skill in the registry")
		}
		tool, ok := s.(interface {
			ExecuteTool(context.Context, map[string]any) (skill.Output, error)
		})
		if !ok {
			t.Fatal("list skill lost ExecuteTool")
		}
		return tool.ExecuteTool(context.Background(), map[string]any{"path": outside})
	}

	a := &App{} // zero value: projectFocused=false, the unfocused startup state
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-grid",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})
	out, err := listOutside(a)
	if err != nil || !strings.Contains(out.Content, "stray.pdf") {
		t.Fatalf("unfocused engine could not list an outside folder: err=%v content=%q", err, out.Content)
	}

	a.projectFocused = true
	a.applyConfig(a.cur(), a.cfg)
	if _, err := listOutside(a); err == nil {
		t.Fatal("focused engine listed a folder outside the project")
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
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
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
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
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
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
	if _, err := a.ReadFile("sub"); err == nil {
		t.Error("expected error reading a directory, got nil")
	}
}

func TestCommandHistoryOrderAndCap(t *testing.T) {
	a := &App{}
	for i := 0; i < maxToolHistory+5; i++ {
		a.recordToolAction(a.cur(), turn.ToolEvent{Action: "call", Name: "action-" + string(rune('a'+i%26))})
	}
	// "result" events must be ignored.
	a.recordToolAction(a.cur(), turn.ToolEvent{Action: "result", Name: "should-not-appear"})

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
	if hist[0] != a.cur().toolHistory[len(a.cur().toolHistory)-1] {
		t.Errorf("CommandHistory()[0] = %q, want most recent action %q", hist[0], a.cur().toolHistory[len(a.cur().toolHistory)-1])
	}
}

func TestGitChangedFilesOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	a := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, newConversation())
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

	a := seed(&App{cfg: config.Config{SandboxRoot: root}, projectFocused: true}, newConversation())
	got := a.GitChangedFiles()
	if len(got) != 1 || got[0].Path != "new.txt" || got[0].Status != "U" {
		t.Errorf("GitChangedFiles() = %+v, want one untracked entry for new.txt", got)
	}
}

func TestGitChangedFilesEmptyWhenUnfocused(t *testing.T) {
	a := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, newConversation()) // projectFocused: false
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

	a := seed(&App{cfg: config.Config{SandboxRoot: root}, projectFocused: true}, newConversation())
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
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
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
	// Both roots have to be empty, not just the new one: with only
	// AETOX_DATA_ROOT set, resolveConfig still found the legacy preference path
	// under the developer's real config dir off Windows.
	isolateUserDirs(t)
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // empty dir — no preference file exists

	cfg := resolveConfig(config.ConfigOptions{ApprovalMode: string(safety.ApprovalUnsafeOnly)})
	if cfg.ApprovalMode != string(safety.ApprovalUnsafeOnly) {
		t.Errorf("ApprovalMode = %q, want %q (opts value should stand when nothing is saved)", cfg.ApprovalMode, safety.ApprovalUnsafeOnly)
	}
}

func TestProviderWireFormatsListsAltForDeepSeek(t *testing.T) {
	a := &App{}
	got := a.ProviderWireFormats("deepseek")
	want := []string{"anthropic", "openai-compatible"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ProviderWireFormats(deepseek) = %v, want %v", got, want)
	}
}

// openai grew a second format on 2026-08-22, and it became the default:
// /chat/completions rejects function tools sent with reasoning_effort, which
// is every turn Aetox takes. /chat/completions stays reachable as the alt.
func TestProviderWireFormatsListsAltForOpenAI(t *testing.T) {
	a := &App{}
	got := a.ProviderWireFormats("openai")
	want := []string{"responses", "openai-compatible"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ProviderWireFormats(openai) = %v, want %v", got, want)
	}
}

func TestProviderWireFormatsEmptyForSingleFormatProvider(t *testing.T) {
	a := &App{}
	if got := a.ProviderWireFormats("groq"); len(got) != 0 {
		t.Fatalf("ProviderWireFormats(groq) = %v, want empty (groq has only one wire format)", got)
	}
}

// resolveConfig must round-trip a saved wire-format preference the same way
// it already does for provider/model/base URL.
func TestResolveConfigLoadsWireFormatFromPreference(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	pref := config.ModelPreference{ModelProvider: "deepseek", ModelWireFormat: "openai-compatible"}
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg := resolveConfig(config.ConfigOptions{})
	if cfg.ModelWireFormat != "openai-compatible" {
		t.Errorf("ModelWireFormat = %q, want %q (saved preference should load)", cfg.ModelWireFormat, "openai-compatible")
	}
}

func TestEffectiveWireFormatFallsBackToProviderDefault(t *testing.T) {
	// Nothing explicitly chosen yet — the UI must still highlight the
	// provider's real default (deepseek's is "anthropic"), not blank.
	if got := effectiveWireFormat("deepseek", ""); got != "anthropic" {
		t.Errorf("effectiveWireFormat(deepseek, \"\") = %q, want %q", got, "anthropic")
	}
	if got := effectiveWireFormat("deepseek", "openai-compatible"); got != "openai-compatible" {
		t.Errorf("effectiveWireFormat(deepseek, explicit) = %q, want the explicit value unchanged", got)
	}
}

func TestEnabledProvidersDefaultsToActiveProviderFromCfg(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // no saved preference — untouched install
	a := seed(&App{cfg: config.Config{ModelProvider: "deepseek"}}, newConversation())
	got := a.EnabledProviders()
	if len(got) != 1 || got[0] != "deepseek" {
		t.Fatalf("EnabledProviders() = %v, want [deepseek] (default to the active provider)", got)
	}
}

func TestSetProviderEnabledAddsAndRemoves(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{cfg: config.Config{ModelProvider: "deepseek"}}, newConversation())

	got, err := a.SetProviderEnabled("openai", true)
	if err != nil {
		t.Fatalf("enable openai: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("after enabling openai = %v, want 2 entries (deepseek + openai)", got)
	}

	got, err = a.SetProviderEnabled("deepseek", false)
	if err != nil {
		t.Fatalf("disable deepseek: %v", err)
	}
	if len(got) != 1 || got[0] != "openai" {
		t.Fatalf("after disabling deepseek = %v, want [openai]", got)
	}

	if _, err := a.SetProviderEnabled("openai", false); err == nil {
		t.Fatal("disabling the last remaining provider must be refused, got nil error")
	}
	// Refused removal must not have mutated the persisted state.
	if got := a.EnabledProviders(); len(got) != 1 || got[0] != "openai" {
		t.Fatalf("EnabledProviders() after refused removal = %v, want [openai] unchanged", got)
	}
}

func TestSetProviderEnabledUnknownProviderErrors(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{cfg: config.Config{ModelProvider: "deepseek"}}, newConversation())
	if _, err := a.SetProviderEnabled("not-a-real-provider-xyz", true); err == nil {
		t.Fatal("expected an error for an unrecognized provider name")
	}
}

func TestSaveChatImageCopiesIntoSandbox(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(src, []byte("fake png bytes"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
	a.startNewSession()

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

	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
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

// A pasted screenshot has no file behind it, which is why the picker-only
// attach routes could not take one — Ctrl+V did nothing at all. The bytes come
// over as a data URL and have to land in the same per-session folder, on a
// relative path with the right extension, or the skills that open attachments
// by path cannot read what the user just pasted.
func TestSaveChatImageDataWritesPastedBytes(t *testing.T) {
	root := t.TempDir()
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
	a.startNewSession()

	// base64("fake png bytes")
	rel, err := a.SaveChatImageData("data:image/png;base64,ZmFrZSBwbmcgYnl0ZXM=")
	if err != nil {
		t.Fatalf("SaveChatImageData: unexpected error: %v", err)
	}
	if filepath.Ext(rel) != ".png" {
		t.Errorf("relPath = %q, want a .png extension", rel)
	}
	if strings.Contains(rel, "..") || filepath.IsAbs(rel) {
		t.Errorf("relPath = %q, want a path inside the sandbox", rel)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("pasted image not found: %v", err)
	}
	if string(got) != "fake png bytes" {
		t.Errorf("written content = %q, want %q", got, "fake png bytes")
	}
}

// A JPEG screenshot is the common case on Windows' own snipping tools, so the
// extension has to follow the clipboard's type rather than always being .png:
// the path is handed to skills that pick their reader by extension.
func TestSaveChatImageDataKeepsTheClipboardsType(t *testing.T) {
	a := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, newConversation())
	a.startNewSession()

	rel, err := a.SaveChatImageData("data:image/jpeg;base64,ZmFrZQ==")
	if err != nil {
		t.Fatalf("SaveChatImageData: %v", err)
	}
	if filepath.Ext(rel) != ".jpg" {
		t.Errorf("relPath = %q, want .jpg for an image/jpeg paste", rel)
	}
}

// Refusing loudly is the point: an unknown type written with a guessed
// extension is a file that fails later, somewhere the user cannot connect to
// the paste. Text on the clipboard must not come through here at all.
func TestSaveChatImageDataRefusesWhatIsNotAPastableImage(t *testing.T) {
	a := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, newConversation())
	a.startNewSession()

	for _, bad := range []string{
		"", "hello", "data:text/plain;base64,aGVsbG8=",
		"data:image/tiff;base64,aGVsbG8=", "data:image/svg+xml;base64,aGVsbG8=",
		"data:image/png;base64,", "data:image/png;base64,!!!not-base64!!!",
		"data:image/png,ZmFrZQ==", // not base64-encoded at all
	} {
		if _, err := a.SaveChatImageData(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// A model/provider switch re-bootstraps the engine — the new agent must
// inherit the old agent's REAL context (including tool messages and their
// results), not just the text transcript. Regression for the switch dropping
// tool history.
func TestApplyConfigInheritsPriorAgentContext(t *testing.T) {
	root := t.TempDir()
	a := &App{}
	cfg := config.Config{SandboxRoot: root, ModelProvider: "aetox", ModelName: "aetox-grid"}
	a.applyConfig(a.cur(), cfg)
	if a.cur().agent == nil {
		t.Fatal("setup: agent must bootstrap")
	}

	a.cur().agent.RestoreHistory([]model.Message{
		{Role: model.RoleUser, Content: "read the config file"},
		{Role: model.RoleAssistant, Content: "reading", ToolCalls: []model.ToolCall{{
			ID: "call_1", Type: "function",
			Function: model.FunctionCall{Name: "read", Arguments: `{"path":"config.json"}`},
		}}},
		{Role: model.RoleTool, Name: "read", ToolCallID: "call_1", Content: "config contents: xyz"},
		{Role: model.RoleAssistant, Content: "the config says xyz"},
	})

	// Switch models — a fresh agent is built and must inherit everything.
	cfg.ModelName = "aetox-think:test"
	a.applyConfig(a.cur(), cfg)

	sawToolResult := false
	for _, m := range a.cur().agent.ContextMessages() {
		if m.Role == model.RoleTool && m.ToolCallID == "call_1" {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Fatal("tool result must survive the model switch (prior context inheritance)")
	}
}

// A fresh install — no preference file — must land on Aetox's own engine with
// its real model name showing, not a blank. aetox used to be excluded from the
// catalog-default fill, which left the composer with no model name at all on
// first run (ARCHITECTURE.md §43).
func TestFreshInstallDefaultsToTheGuideModel(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // no model-preference.json anywhere
	cfg := resolveConfig(config.ConfigOptions{RootPath: t.TempDir()})

	if cfg.ModelProvider != "aetox" {
		t.Errorf("fresh install provider = %q, want aetox (its own engine, no key needed)", cfg.ModelProvider)
	}
	if cfg.ModelName != "aetox-grid" {
		t.Errorf("fresh install model = %q, want aetox-grid — a blank name shows nothing in the picker", cfg.ModelName)
	}
}

// Opening a project must not change the model this window is running on.
// reload() used to re-resolve from model-preference.json — a global file every
// other Aetox window and the CLI also write — so a project switch adopted
// someone else's model mid-session (see desktop/app.go reload).
func TestReloadKeepsTheRunningModelWhenTheGlobalPreferenceChanges(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{}
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-grid",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	// Another instance writes the shared preference file out from under us.
	if err := config.SaveModelPreference(config.ModelPreference{
		ModelProvider: "aetox",
		ModelName:     "aetox-think:test",
		ApprovalMode:  string(safety.ApprovalAsk),
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	root := t.TempDir()
	a.reload(config.ConfigOptions{RootPath: root, ApprovalMode: string(safety.ApprovalFullAccess)})

	if a.cur().cfg.ModelName != "aetox-grid" {
		t.Errorf("model after project switch = %q, want aetox-grid (the running model must survive)", a.cur().cfg.ModelName)
	}
	if a.cur().cfg.ApprovalMode != string(safety.ApprovalFullAccess) {
		t.Errorf("approval mode after project switch = %q, want %q", a.cur().cfg.ApprovalMode, safety.ApprovalFullAccess)
	}
	if a.cur().cfg.SandboxRoot != root {
		t.Errorf("sandbox root after project switch = %q, want %q (the root is the one thing reload changes)", a.cur().cfg.SandboxRoot, root)
	}
}

// The other half of the same branch: startup has no running model, so it must
// still load the one the user saved last time.
func TestReloadLoadsTheSavedModelOnFirstBootstrap(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if err := config.SaveModelPreference(config.ModelPreference{
		ModelProvider: "aetox",
		ModelName:     "aetox-think:test",
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	a := &App{} // nothing bootstrapped yet — this is App.startup
	a.reload(config.ConfigOptions{RootPath: t.TempDir()})

	if a.cur().cfg.ModelName != "aetox-think:test" {
		t.Errorf("model on first bootstrap = %q, want the saved aetox-think:test", a.cur().cfg.ModelName)
	}
}

// The Interject binding: what the composer calls when the user types under a
// running turn. It must hand the text to the agent and nothing else — no second
// turn, no waiting on a reply.
func TestInterjectHandsTheTextToTheRunningAgent(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // no user presets to expand against
	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider: model.NewNoopProvider("aetox-tools:test"),
		Model:    "aetox-tools:test",
	})
	a := seed(&App{}, &conversation{agent: agent})

	if err := a.Interject("ใส่สีน้ำเงินด้วยนะ"); err != nil {
		t.Fatalf("Interject: %v", err)
	}
	// Blank text is not a message — it would cost a round asking the model to
	// respond to nothing.
	if err := a.Interject("   "); err != nil {
		t.Fatalf("blank Interject returned an error: %v", err)
	}
	got := agent.DrainInterjections()
	if len(got) != 1 || got[0] != "ใส่สีน้ำเงินด้วยนะ" {
		t.Fatalf("agent received %v", got)
	}

	// No engine yet is a readable error, not a panic — the composer is usable
	// before a provider is configured.
	if err := (seed(&App{}, &conversation{modelStatus: "no key"})).Interject("hello"); err == nil {
		t.Error("Interject with no agent should say the core is not ready")
	}
}

// Stop has to mean stop, including whatever was typed under the turn being
// stopped. The loop checks ctx before it drains, so a cancelled turn returns with
// the message still pending — if CancelTurn left it there, SendMessage would hand
// it back as a straggler and the composer would send the very thing the user just
// cancelled.
func TestCancelTurnDropsWhatWasTypedUnderIt(t *testing.T) {
	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider: model.NewNoopProvider("aetox-tools:test"),
		Model:    "aetox-tools:test",
	})
	a := seed(&App{}, &conversation{agent: agent})
	agent.Interject("เดี๋ยว เปลี่ยนใจ")

	a.CancelTurn() // no turn in flight: must still clear, and must not panic

	if left := agent.DrainInterjections(); len(left) != 0 {
		t.Errorf("Stop left %v pending — it would be sent as a fresh turn", left)
	}
}

// Picking a local runtime whose server is not up must not read as success.
// BootstrapProvider swaps in the built-in aetox provider so the window stays
// alive, which left a non-nil chat — the only thing every Switch* method
// checked — so the picker showed "lmstudio / —" with no error anywhere while
// the engine answered as Aetox. The fallback stays; the warning now travels.
func TestSwitchToUnreachableProviderReportsTheFallback(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{}
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "lmstudio",
		ModelBaseURL:  "http://127.0.0.1:1/v1", // nothing ever listens on port 1
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	info, err := a.modelSwitchResult()
	if err != nil {
		t.Fatalf("the aetox fallback must keep the app usable: %v", err)
	}
	if info.Warning == "" {
		t.Fatal("no warning — the UI would show lmstudio as if it were connected")
	}
	if !strings.Contains(info.Warning, "lmstudio") {
		t.Errorf("warning %q does not name the provider that failed", info.Warning)
	}
}

// The same path on a reachable provider must stay quiet, or the banner cries
// wolf on every switch.
func TestSwitchToWorkingProviderReportsNoWarning(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{}
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	info, err := a.modelSwitchResult()
	if err != nil {
		t.Fatalf("switch to the built-in provider failed: %v", err)
	}
	if info.Warning != "" {
		t.Errorf("warning on a healthy switch: %q", info.Warning)
	}
}

// LM Studio's server port is user-configurable, so the catalog default is a
// guess, not a fact — and the base URL used to be read-only in Settings with no
// way to say otherwise. The override has to reach every path that dials the
// provider, not just the field that displays it.
func TestCustomBaseURLIsWhatEveryProviderPathDials(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{}
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	const custom = "http://127.0.0.1:54998/v1"
	if _, err := a.SetProviderBaseURL("lmstudio", custom); err != nil {
		t.Fatalf("SetProviderBaseURL: %v", err)
	}
	if got := a.ProviderBaseURL("lmstudio"); got != custom {
		t.Errorf("ProviderBaseURL = %q, want %q", got, custom)
	}
	if !a.ProviderBaseURLIsCustom("lmstudio") {
		t.Error("override not reported as custom — the UI would offer no reset")
	}
	if got := resolveBaseURLForProvider("lmstudio"); got != custom {
		t.Errorf("resolveBaseURLForProvider = %q — discovery and the connection test dial the wrong host", got)
	}
	// The single-slot predecessor stored only the active provider's URL, so
	// visiting another provider wiped it. Per-provider means it survives.
	if _, err := a.SwitchProvider("aetox"); err != nil {
		t.Fatalf("SwitchProvider: %v", err)
	}
	if got := a.ProviderBaseURL("lmstudio"); got != custom {
		t.Errorf("after visiting another provider, lmstudio URL = %q, want %q", got, custom)
	}

	// Empty is the reset, and it has to actually clear rather than store "".
	if _, err := a.SetProviderBaseURL("lmstudio", ""); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got, want := a.ProviderBaseURL("lmstudio"), model.DefaultBaseURL("lmstudio"); got != want {
		t.Errorf("after reset = %q, want the catalog default %q", got, want)
	}
	if a.ProviderBaseURLIsCustom("lmstudio") {
		t.Error("still reported as custom after reset")
	}
}

// The value becomes an outbound request target, so garbage must be refused at
// the boundary rather than saved and re-surfacing later as a provider outage.
func TestSetProviderBaseURLRejectsWhatCannotBeDialed(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{}
	for _, bad := range []string{"localhost:1234", "file:///etc/passwd", "ftp://x/y", "://nope"} {
		if _, err := a.SetProviderBaseURL("lmstudio", bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if got, want := a.ProviderBaseURL("lmstudio"), model.DefaultBaseURL("lmstudio"); got != want {
		t.Errorf("a rejected value still landed: %q", got)
	}
	if _, err := a.SetProviderBaseURL("nonsense-provider", "http://x/v1"); err == nil {
		t.Error("accepted an unknown provider")
	}
}

// A file the agent produced can be gone by the time anyone opens its card: the
// agent can delete files (it did, testing `delete` on its own output), and
// session output folders age out. The card is history and stays; the offer to
// open it must not outlive the file.
func TestAMissingFileIsReportedAsGoneNotAsAnOSError(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	root := t.TempDir()
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())

	if err := os.WriteFile(filepath.Join(root, "made.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := a.FileStillThere("made.txt"); got != FileHere {
		t.Fatalf("a file that exists reported %q, want %q", got, FileHere)
	}

	if err := os.Remove(filepath.Join(root, "made.txt")); err != nil {
		t.Fatal(err)
	}
	if got := a.FileStillThere("made.txt"); got != FileGone {
		t.Errorf("a deleted file reported %q, want %q", got, FileGone)
	}
	// The gap between asking and clicking: the answer has to be the one the UI
	// can translate, not "GetFileAttributesEx …: The system cannot find the
	// file specified", which reads to a user as a crash.
	err := a.OpenFileExternally("made.txt")
	if !errors.Is(err, errFileGone) {
		t.Errorf("opening a deleted file returned %v, want the named gone error", err)
	}
}

// A directory and a path outside the sandbox are both "nothing here to open" —
// but neither of them is the file having been deleted, and saying so is the
// whole of §133. Only a real miss may take the offer to open away.
func TestNotAllowedToLookIsNotTheSameAsGone(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	root := t.TempDir()
	a := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"a directory":           "sub",
		"a path outside":        "../outside.txt",
		"an absolute path away": filepath.Join(t.TempDir(), "elsewhere.docx"),
	} {
		if got := a.FileStillThere(path); got != FileUnknown {
			t.Errorf("%s reported %q, want %q — the pane draws %q as \"deleted\"",
				name, got, FileUnknown, FileGone)
		}
	}
	if got := (&App{}).FileStillThere("anything.txt"); got != FileUnknown {
		t.Errorf("with no project open the answer is %q, want %q", got, FileUnknown)
	}
}

// The bug this whole change exists for, end to end.
//
// An unfocused session has the machine for a workspace, so the agent can write
// a document anywhere the user asked for — and did: D:\Mike\...\ภาคผนวก.docx,
// 2026-08-18. The window then said the file had been deleted and hid the button
// to open it, because its own copy of the containment rule joined that absolute
// path onto the root and Stat'd `<root>\D:\Mike\...`, which of course was not
// there.
func TestAnAbsolutePathTheAgentCanWriteIsOneTheWindowCanSee(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	root := t.TempDir()
	// Somewhere that is emphatically not under the root, the way D: is not
	// under C:\Users\ASUS\aetox.
	elsewhere := t.TempDir()
	doc := filepath.Join(elsewhere, "ภาคผนวก.docx")
	if err := os.WriteFile(doc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The OS door is stubbed, and the stub is the point: this test used to open
	// a real File Explorer window on whoever ran it, every run, and land it at
	// Documents because Start() outlived the temp directory. What it means to
	// ask is "did the binding get as far as opening?", which is what `opened`
	// records.
	var opened string
	a := seed(&App{cfg: config.Config{SandboxRoot: root}, openDir: func(p string) error {
		opened = p
		return nil
	}}, newConversation())
	// Focused: the folder was never added, so the honest answer is that the
	// window cannot say — never that the file is gone.
	skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root})
	if got := a.FileStillThere(doc); got != FileUnknown {
		t.Errorf("focused: %q, want %q", got, FileUnknown)
	}

	// Unfocused: the machine is the workspace, which is exactly the session
	// that produced the file. The window must see what the engine could write.
	skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root, OpenSandbox: true})
	if got := a.FileStillThere(doc); got != FileHere {
		t.Errorf("unfocused: %q, want %q — the engine wrote this file", got, FileHere)
	}
	if err := a.OpenFileExternally(doc); errors.Is(err, errFileGone) {
		t.Error("opening it reports the file as gone, and it is right there")
	}
	if opened == "" {
		t.Error("the file manager was never asked to open anything")
	}
}

// The frontend caches what GetModelInfo reports and seeds the next launch's
// first paint from it — so a raw "" from the pre-startup window painted an
// empty approval dropdown on every launch after it (found 2026-08-07, the
// screenshot with a blank "ระดับการอนุมัติ" box). The report is normalized, never raw.
func TestGetModelInfoNeverReportsAnEmptyApproval(t *testing.T) {
	a := &App{} // the pre-startup state: no config built yet
	if got := a.GetModelInfo().ApprovalMode; got != "ask" {
		t.Fatalf("ApprovalMode = %q before startup, want the normalized default %q", got, "ask")
	}
}

// An installer's "run now" spawns the app with a stripped environment and no
// USERPROFILE. unfocusedRoot answering "" then flows through config.Load into
// os.Getwd() — for an installed app that is C:\Program Files, where the
// session's first write dies on Access denied (tool_runs #533, 2026-08-08).
func TestUnfocusedRootSurvivesAMissingHome(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOME", "")
	root := unfocusedRoot()
	if strings.TrimSpace(root) == "" {
		t.Fatal("unfocusedRoot returned \"\" with no home — config.Load turns that into the process cwd")
	}
}
