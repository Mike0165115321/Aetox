package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The sandbox root must NOT reach the prompt: it is a machine-specific path
// carrying the user's account name, it would be sent to whichever provider is
// configured on every request, and no file tool accepts an absolute path
// anyway — so it was cost without a use.
func TestBuildIncludesIdentityAndEnvironment(t *testing.T) {
	got := Build(SurfaceCLI, "/tmp/proj", false)
	if !strings.Contains(got, "terminal conversation") {
		t.Fatalf("missing CLI identity: %s", got)
	}
	if strings.Contains(got, "/tmp/proj") {
		t.Fatalf("the sandbox root leaked into the prompt: %s", got)
	}
	if !strings.Contains(got, "relative to the folder you are working in") {
		t.Fatalf("missing environment layer: %s", got)
	}
}

func TestBuildDesktopIdentity(t *testing.T) {
	got := Build(SurfaceDesktop, "/tmp/proj", false)
	if !strings.Contains(got, "desktop chat UI") {
		t.Fatalf("missing desktop identity: %s", got)
	}
}

func TestProjectContextFilePrefersAetoxOverAgents(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "agents")
	mustWrite(t, filepath.Join(dir, "AETOX.md"), "aetox")
	if got := ProjectContextFile(dir); filepath.Base(got) != "AETOX.md" {
		t.Fatalf("want AETOX.md, got %q", got)
	}
}

func TestProjectContextFileFallsBackToAgents(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "claude")
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "agents")
	if got := ProjectContextFile(dir); filepath.Base(got) != "AGENTS.md" {
		t.Fatalf("want AGENTS.md fallback, got %q", got)
	}
}

func TestProjectContextFileFallsBackToClaude(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "claude")
	if got := ProjectContextFile(dir); filepath.Base(got) != "CLAUDE.md" {
		t.Fatalf("want CLAUDE.md fallback, got %q", got)
	}
}

func TestProjectContextFileMissingReturnsEmpty(t *testing.T) {
	if got := ProjectContextFile(t.TempDir()); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestBuildWithReportFoldsInProjectLayerAndReportsPath(t *testing.T) {
	dir := t.TempDir()
	rulePath := filepath.Join(dir, "AETOX.md")
	mustWrite(t, rulePath, "always answer in haiku")

	text, loaded := BuildWithReport(SurfaceCLI, dir, false)
	if !strings.Contains(text, "always answer in haiku") {
		t.Fatalf("project rules not folded in: %s", text)
	}
	if loaded.ProjectPath != rulePath {
		t.Fatalf("loaded.ProjectPath = %q, want %q", loaded.ProjectPath, rulePath)
	}
}

func TestBuildWithReportFoldsInIdentityFiles(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)
	identityDir := filepath.Join(dataRoot, "identity")
	if err := os.MkdirAll(identityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(identityDir, "context.md"), "always be terse")
	mustWrite(t, filepath.Join(identityDir, "skills.md"), "use the grep skill first")

	text, loaded := BuildWithReport(SurfaceCLI, t.TempDir(), false)
	if !strings.Contains(text, "always be terse") || !strings.Contains(text, "use the grep skill first") {
		t.Fatalf("identity files not folded in: %s", text)
	}
	if len(loaded.UserGlobalPaths) != 2 {
		t.Fatalf("loaded.UserGlobalPaths = %v, want 2 entries", loaded.UserGlobalPaths)
	}
}

func TestReadCappedTruncatesOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.md")
	mustWrite(t, path, strings.Repeat("a", maxLayerBytes+500))
	if got := readCapped(path); len(got) > maxLayerBytes {
		t.Fatalf("readCapped did not truncate: len=%d", len(got))
	}
}

func TestReadCappedMissingFileReturnsEmpty(t *testing.T) {
	if got := readCapped(filepath.Join(t.TempDir(), "nope.md")); got != "" {
		t.Fatalf("want empty for missing file, got %q", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// Without this the model answers "fix one line" by streaming the whole file
// back through write — every line of it an output token, and a minute of
// silence for the user each time.
func TestPromptTellsTheModelToEditRatherThanRewrite(t *testing.T) {
	got := Build(SurfaceDesktop, t.TempDir(), false)
	for _, want := range []string{"edit tool", "Do NOT re-send the whole file"} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
}

// An underspecified "create a file" used to fork two bad ways: invent a
// deliverable nobody asked for, or refuse and report what cannot be done.
// The prompt must point at the third option — one question first. ask_user's
// own description never fires here (an empty brief does not read as blocked),
// so the guidance has to come from the prompt.
func TestBuildTellsTheModelToAskWhenTheBriefIsEmpty(t *testing.T) {
	got := Build(SurfaceDesktop, t.TempDir(), false)
	for _, want := range []string{"ask ONE question", "ask_user"} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
}

// "สร้างสไลด์ … อยากได้เป็นไฟล์ HTML" was answered with a .pptx anyway: the
// model mapped "slides" to slides_write and never weighed the format the user
// had named. The prompt teaches the principle — a tool's usual mapping is a
// default, and defaults lose to what the user said — rather than a case rule
// (owner, 2026-08-04: "สอนให้มันฉลาดและเลือกถามได้ ไม่ใช่กำหนดตรงๆ").
func TestPromptTeachesThatDefaultsLoseToTheUsersWords(t *testing.T) {
	got := Build(SurfaceDesktop, t.TempDir(), false)
	for _, want := range []string{"a default, not a decision", "the choice is theirs"} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
	// The case rule the principle replaced must not creep back in.
	for _, reject := range []string{`"slides"`, ".pptx"} {
		if strings.Contains(got, reject) {
			t.Errorf("system prompt hardcodes the case %q instead of the principle:\n%s", reject, got)
		}
	}
}

// The prompt must describe the same wall the tools enforce. In the unfocused
// desktop the sandbox is open — telling the model "absolute paths are
// rejected" there makes it answer "I can't search this machine" while holding
// tools that can, which is the exact bug that motivated the mode.
func TestBuildOpenSandboxSwapsTheEnvironmentLayer(t *testing.T) {
	open := Build(SurfaceDesktop, t.TempDir(), true)
	for _, want := range []string{"any path on this machine", "Credential stores", "output folder"} {
		if !strings.Contains(open, want) {
			t.Errorf("open-sandbox prompt is missing %q:\n%s", want, open)
		}
	}
	if strings.Contains(open, "absolute paths are rejected") {
		t.Errorf("open-sandbox prompt still claims absolute paths are rejected:\n%s", open)
	}

	closed := Build(SurfaceDesktop, t.TempDir(), false)
	if !strings.Contains(closed, "absolute paths are rejected") {
		t.Errorf("closed-sandbox prompt lost its wall sentence:\n%s", closed)
	}
	if strings.Contains(closed, "any path on this machine") {
		t.Errorf("closed-sandbox prompt leaked the open-mode text:\n%s", closed)
	}
}

// List-shaped work must be steered into one script rather than one tool call
// per item — a 200-item list as 200 calls exhausts a small-context model
// before the list ends, and that failure arrives silently, as a half-done job.
func TestBuildTeachesBatchWorkAsOneScript(t *testing.T) {
	got := Build(SurfaceCLI, "", false)
	if !strings.Contains(got, "same operation over many items") {
		t.Fatalf("missing batch-work guidance: %s", got)
	}
	if !strings.Contains(got, "per-item work, not batch work") {
		t.Fatalf("batch guidance lost its boundary — without it, per-file judgment edits get scripted too: %s", got)
	}
}
