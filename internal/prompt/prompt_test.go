package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/learned"
)

// The sandbox root must NOT reach the prompt: it is a machine-specific path
// carrying the user's account name, it would be sent to whichever provider is
// configured on every request, and relative paths reach it anyway — so it was
// cost without a use. A folder the user *added* is the one exception, because
// its full path is the only name it has (TestBuildNamesTheFoldersTheUserAdded).
func TestBuildIncludesIdentityAndEnvironment(t *testing.T) {
	got := Build(SurfaceCLI, Scope{Root: "/tmp/proj"})
	if !strings.Contains(got, "terminal conversation") {
		t.Fatalf("missing CLI identity: %s", got)
	}
	if strings.Contains(got, "/tmp/proj") {
		t.Fatalf("the sandbox root leaked into the prompt: %s", got)
	}
	if !strings.Contains(got, "a bare path is relative to it") {
		t.Fatalf("missing environment layer: %s", got)
	}
}

func TestBuildDesktopIdentity(t *testing.T) {
	got := Build(SurfaceDesktop, Scope{Root: "/tmp/proj"})
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

	text, loaded := BuildWithReport(SurfaceCLI, Scope{Root: dir})
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

	text, loaded := BuildWithReport(SurfaceCLI, Scope{Root: t.TempDir()})
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
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
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
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
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
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
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
	open := Build(SurfaceDesktop, Scope{Root: t.TempDir(), Open: true})
	for _, want := range []string{"any path on this machine", "Credential stores", "output folder"} {
		if !strings.Contains(open, want) {
			t.Errorf("open-sandbox prompt is missing %q:\n%s", want, open)
		}
	}
	if strings.Contains(open, "absolute paths are rejected") {
		t.Errorf("open-sandbox prompt still claims absolute paths are rejected:\n%s", open)
	}

	closed := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	if !strings.Contains(closed, "confined to the project folder") {
		t.Errorf("closed-sandbox prompt lost its wall sentence:\n%s", closed)
	}
	if strings.Contains(closed, "any path on this machine") {
		t.Errorf("closed-sandbox prompt leaked the open-mode text:\n%s", closed)
	}
	// A refusal the model cannot act on turns into "I can't", which is the
	// failure this whole feature exists to end. The wall sentence has to carry
	// the way out with it.
	if !strings.Contains(closed, "ask the user to add that folder") {
		t.Errorf("closed-sandbox prompt states the wall without the remedy:\n%s", closed)
	}
}

// A folder the user added is only usable if the model is told it exists — the
// tools would accept it, but nothing else in the prompt names it, and a model
// that has not been told a folder is reachable never tries it.
func TestBuildNamesTheFoldersTheUserAdded(t *testing.T) {
	other := t.TempDir()
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir(), Extra: []string{other}})
	if !strings.Contains(got, other) {
		t.Errorf("prompt does not name the added folder %q:\n%s", other, got)
	}
	// Same rights as the project, stated outright: a model that guesses it has
	// read-only access to an added folder will refuse edits the user allowed.
	if !strings.Contains(got, "same rights as the project folder") {
		t.Errorf("prompt leaves the rights of an added folder to guesswork:\n%s", got)
	}
	if strings.Contains(got, "confined to the project folder") {
		t.Errorf("prompt still claims the project folder is the whole workspace:\n%s", got)
	}
}

// List-shaped work must be steered into one script rather than one tool call
// per item — a 200-item list as 200 calls exhausts a small-context model
// before the list ends, and that failure arrives silently, as a half-done job.
func TestBuildTeachesBatchWorkAsOneScript(t *testing.T) {
	got := Build(SurfaceCLI, Scope{Root: ""})
	if !strings.Contains(got, "same operation over many items") {
		t.Fatalf("missing batch-work guidance: %s", got)
	}
	if !strings.Contains(got, "per-item work, not batch work") {
		t.Fatalf("batch guidance lost its boundary — without it, per-file judgment edits get scripted too: %s", got)
	}
}

// Learned memory is folded in, and where it sits is the policy: after what the
// user told the agent, before what this project requires. Models weight later
// context more heavily, so the order is the whole precedence mechanism — there
// is no sentence in the prompt claiming a ranking that could drift from it.
func TestLearnedMemorySitsBetweenTheUsersRulesAndTheProjects(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)

	identityDir := filepath.Join(dataRoot, "identity")
	if err := os.MkdirAll(identityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(identityDir, "context.md"), "IDENTITY-MARKER")
	if err := learned.Apply(learned.MainScope, learned.OpAdd, "", "MEMORY-MARKER"); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	projectRoot := t.TempDir()
	mustWrite(t, filepath.Join(projectRoot, "AETOX.md"), "PROJECT-MARKER")

	text, loaded := BuildWithReport(SurfaceDesktop, Scope{Root: projectRoot})
	identity := strings.Index(text, "IDENTITY-MARKER")
	memory := strings.Index(text, "MEMORY-MARKER")
	project := strings.Index(text, "PROJECT-MARKER")
	if identity < 0 || memory < 0 || project < 0 {
		t.Fatalf("a layer is missing:\n%s", text)
	}
	if !(identity < memory && memory < project) {
		t.Errorf("layer order is identity(%d) < memory(%d) < project(%d)", identity, memory, project)
	}
	if loaded.MemoryPath == "" {
		t.Error("the report should name the memory file it folded in")
	}
}

// A delegate's memory belongs to that delegate. Carrying every sub-agent's
// accumulated knowledge in the main prompt is exactly the cost this
// architecture exists not to pay.
func TestTheMainPromptCarriesNoDelegatesMemory(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if err := learned.Apply("explore", learned.OpAdd, "", "DELEGATE-ONLY-MARKER"); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	text, loaded := BuildWithReport(SurfaceDesktop, Scope{Root: t.TempDir()})
	if strings.Contains(text, "DELEGATE-ONLY-MARKER") {
		t.Errorf("a sub-agent's memory reached the main prompt:\n%s", text)
	}
	if loaded.MemoryPath != "" {
		t.Errorf("nothing was learned in the main scope, got %q", loaded.MemoryPath)
	}
}

// An agent that has learned nothing must produce byte-for-byte the prompt it
// produced before this existed: the common case cannot pay for the feature,
// and prefix caching keys on the leading bytes.
func TestNothingLearnedChangesNothing(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	root := t.TempDir()
	before := Build(SurfaceDesktop, Scope{Root: root})
	if strings.Contains(before, "What you have learned") {
		t.Errorf("an empty memory should add no layer:\n%s", before)
	}
}
