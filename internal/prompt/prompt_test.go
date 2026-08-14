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
	if !strings.Contains(got, "You are Aetox") {
		t.Fatalf("missing identity: %s", got)
	}
	if strings.Contains(got, "/tmp/proj") {
		t.Fatalf("the sandbox root leaked into the prompt: %s", got)
	}
	if !strings.Contains(got, "a bare path is relative to it") {
		t.Fatalf("missing environment layer: %s", got)
	}
}

// Identity says who is speaking and nothing else. It named the surface until
// 2026-08-11, which made it one of four places answering "where does my answer
// end up" — and it named two languages, which is this build's first user rather
// than a fact about Aetox.
func TestIdentityIsWhoIsSpeakingAndNothingElse(t *testing.T) {
	for _, s := range []Surface{SurfaceCLI, SurfaceDesktop} {
		if got := identity(); !strings.Contains(got, "Speak the user's language") {
			t.Errorf("identity lost its language rule: %s", got)
		} else if strings.Contains(got, "Thai") || strings.Contains(got, "English") {
			t.Errorf("identity names particular languages instead of stating the rule: %s", got)
		} else if strings.Contains(got, "terminal") || strings.Contains(got, "chat UI") {
			t.Errorf("identity is answering the surface question again: %s", got)
		}
		_ = s
	}
}

// One layer owns "where does what I write end up", and it must answer for the
// terminal too — that half was never stated at all, only inferable from the two
// words "a terminal conversation", while the layers that spell out rendering
// are desktop-only.
func TestSurfaceLayerAnswersForBothSurfaces(t *testing.T) {
	desktop := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	if !strings.Contains(desktop, "rendered as markdown in a chat panel") {
		t.Errorf("desktop prompt does not say what happens to the answer:\n%s", desktop)
	}

	cli := Build(SurfaceCLI, Scope{Root: t.TempDir()})
	if !strings.Contains(cli, "Markdown is not rendered and SVG is not drawn") {
		t.Errorf("terminal prompt still leaves the model to infer that nothing renders:\n%s", cli)
	}
	// The craft layers are desktop-only and must not follow it there.
	for _, leak := range []string{"viewBox", "var(--surface-panel)"} {
		if strings.Contains(cli, leak) {
			t.Errorf("terminal prompt carries drawing guidance it cannot use (%q):\n%s", leak, cli)
		}
	}
}

// drawing() and panel() teach the craft; they must not re-declare the surface,
// which is surfaceLayer's job now.
func TestDrawingAndPanelDoNotRedeclareTheSurface(t *testing.T) {
	for name, text := range map[string]string{"drawing": drawing(), "panel": panel()} {
		for _, opener := range []string{"is rendered as markdown", "drawn in the app's own document"} {
			if strings.Contains(text, opener) {
				t.Errorf("%s() re-opens the surface question with %q:\n%s", name, opener, text)
			}
		}
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

	text, loaded := BuildWithReport(SurfaceCLI, Scope{Root: dir}, Desk{})
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

	text, loaded := BuildWithReport(SurfaceCLI, Scope{Root: t.TempDir()}, Desk{})
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

// A long explanation used to leave as a `task` to the document writer and come
// back a .docx — `doc_write` announced itself as the way to hand back writing,
// and nothing said otherwise. The owner's complaint (2026-08-06) was the folder
// of one-off documents that produced. The prompt must say what the main agent's
// own long-form writing is: a .md file it writes itself.
func TestPromptMakesMarkdownTheDefaultForLongFormWriting(t *testing.T) {
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	for _, want := range []string{"long-form writing", ".md", "Markdown is the default"} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
	// The writers stay available — the rule is about which request they answer,
	// not a ban on a tool name (§ the same principle as the defaults test).
	if strings.Contains(got, "doc_write") {
		t.Errorf("system prompt names a tool instead of stating what the writers are for:\n%s", got)
	}
}

// The prompt must describe the same wall the tools enforce. In the unfocused
// desktop the sandbox is open — telling the model "absolute paths are
// rejected" there makes it answer "I can't search this machine" while holding
// tools that can, which is the exact bug that motivated the mode.
func TestBuildOpenSandboxSwapsTheEnvironmentLayer(t *testing.T) {
	open := Build(SurfaceDesktop, Scope{Root: t.TempDir(), Open: true})
	for _, want := range []string{"any absolute path", "Credential stores", "output folder"} {
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

// The one fact an open workspace owes the model, and the only one: the working
// folder is Aetox's own. A model that reads it as the user's home sends a bare
// `Downloads` and gets nothing, which is how 2026-08-11 started.
//
// Everything else it already knows. Two attempts at saying more were deleted —
// a list of the user's folders by name, then the same paragraph moved into the
// tool's not-found error — and this test is what stops a third.
func TestOpenSandboxSaysTheWorkingFolderIsNotTheUsersHome(t *testing.T) {
	root := t.TempDir()
	got := Build(SurfaceDesktop, Scope{Root: root, Open: true})

	if !strings.Contains(got, "not the user's home") {
		t.Errorf("open-sandbox prompt lets the working folder read as the user's home:\n%s", got)
	}

	// No machine-specific path, in any scope. The one exception is a folder the
	// user added, which has no other name (TestBuildNamesTheFoldersTheUserAdded).
	for name, scope := range map[string]Scope{
		"open":    {Root: root, Open: true},
		"focused": {Root: root},
	} {
		text := Build(SurfaceDesktop, scope)
		if strings.Contains(text, root) {
			t.Errorf("%s prompt names the root, spending a machine-specific path on every request:\n%s", name, text)
		}
		if home, err := os.UserHomeDir(); err == nil && strings.Contains(text, strings.TrimSpace(home)) {
			t.Errorf("%s prompt names the home folder, which the model can read off any path a tool returns:\n%s", name, text)
		}
	}

	// A folder of the user's, named in the prompt, is a case hardcoded into
	// every request — wrong on any machine where they moved it, and paid for
	// forever whether or not it ever comes up.
	for _, folder := range []string{"Downloads", "Documents", "Desktop", "Pictures"} {
		if strings.Contains(got, folder) {
			t.Errorf("open-sandbox prompt hardcodes the folder name %q:\n%s", folder, got)
		}
	}
}

// The sentence that closes shell as an escape route is true in a focused
// project and false with the machine open — and appended to all three scopes it
// was the instruction that ended the 2026-08-11 session: after one mistyped
// relative path the model never called shell again, holding the one tool that
// would have found the folder in a line.
func TestShellEscapeIsShutOnlyWhereItIsActuallyShut(t *testing.T) {
	const shut = "reaching for shell after another tool refused a path"

	open := Build(SurfaceDesktop, Scope{Root: t.TempDir(), Open: true})
	if strings.Contains(open, shut) {
		t.Errorf("open-sandbox prompt tells the model not to use shell on a machine shell can reach:\n%s", open)
	}

	for name, scope := range map[string]Scope{
		"focused":       {Root: t.TempDir()},
		"focused+extra": {Root: t.TempDir(), Extra: []string{t.TempDir()}},
	} {
		if got := Build(SurfaceDesktop, scope); !strings.Contains(got, shut) {
			t.Errorf("%s prompt lost the shell-is-walled-in sentence, which is true there:\n%s", name, got)
		}
	}

	// The half that holds everywhere must not have moved with it.
	if !strings.Contains(open, "Write paths out literally in shell commands") {
		t.Errorf("open-sandbox prompt dropped the literal-paths rule, which the command scanner still enforces:\n%s", open)
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

	text, loaded := BuildWithReport(SurfaceDesktop, Scope{Root: projectRoot}, Desk{})
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
	text, loaded := BuildWithReport(SurfaceDesktop, Scope{Root: t.TempDir()}, Desk{})
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

// Skills are already behind a door — skills_list returns them on request and
// their bodies are never sent (§71) — and until this section the prompt did not
// mention them at all. Whether the model ever knocked was left to the one tool
// description that names them, which is the same shape of mismatch as the
// sandbox one above: the system can do something the model has no reason to
// believe it can.
func TestPromptTeachesThatTheToolListIsNotTheWholeInventory(t *testing.T) {
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	for _, want := range []string{
		"not everything this machine can do",
		"skills_list",
		// The asymmetry is what makes the lookup worth a round.
		"costs one cheap round",
		"hides its own mistake",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
}

// The trigger is a state the model can recognize from the inside — about to
// refuse, about to build from nothing — not a list of topics. A topic list
// answers the failures somebody remembered and nothing else, and it would have
// to be edited every time anything else moves behind a door.
func TestTheCapabilityLessonNamesNoTopics(t *testing.T) {
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	for _, reject := range []string{
		"when the user mentions", "when the user asks about", "if the user says",
	} {
		if strings.Contains(strings.ToLower(got), reject) {
			t.Errorf("prompt hardcodes a trigger phrase %q instead of the state:\n%s", reject, got)
		}
	}
}

// A brief can be complete and still assume something that is not there. Asked
// to migrate a project from one UI library to another with no project in the
// workspace, the model ran list, two session_searches, two globs and three
// shell commands before asking where it was — eight rounds to reach a question
// the user answers in a word.
//
// The lesson is about the state, not about projects: an empty result twice is
// evidence, and the thing that is missing is the thing to ask about.
func TestPromptTeachesThatAnEmptySearchIsAnAnswer(t *testing.T) {
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	for _, want := range []string{
		"rest on something that is not here",
		"come back empty",
		"ask where it is",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
	// The case it generalizes from must not be written down as the rule.
	for _, reject := range []string{"package.json", "components.json", "Radix", "git repo"} {
		if strings.Contains(got, reject) {
			t.Errorf("prompt hardcodes the case %q instead of the principle:\n%s", reject, got)
		}
	}
}

// A wrong sum is the one mistake that never announces itself: it arrives in the
// same confident sentence as a right one, and neither side finds out. calc can
// settle it, and a tool cannot ask to be used — so this layer is what decides
// whether the capability fires at all.
func TestPromptTeachesThatANumberIsWorkedOutNotRecalled(t *testing.T) {
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	for _, want := range []string{
		"calc",
		// The line is at long work, not at any arithmetic: a tool call to prove
		// 20% of 500 is ceremony, and the user said so.
		"Short arithmetic is yours to do",
		// And "long" is countable, because difficulty is the one thing a model
		// cannot judge about its own arithmetic — 47 × 93 and 4.7 × 9.3 feel
		// the same from the inside, and so does getting one of them wrong.
		"not when it feels hard",
		"a wrong sum feels exactly like a right one",
		// Why it beats a number in prose — the user can check the arithmetic.
		"shown the script beside the result",
		// And where the line is: real data, real libraries, a file — that is
		// shell, and shell costs the user's machine.
		"write plus shell",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
}

// The third capability that was already working and never used: the answer is
// drawn in the app's own document, so a styled <div> lays out normally and the
// app's CSS variables resolve against the user's live theme inside it. Without
// a layer saying so it ships and never fires — twice now the same lesson.
func TestPromptTeachesThatTheAnswerCanLayThingsOut(t *testing.T) {
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	for _, want := range []string{
		// The condition, not a list of occasions to decorate.
		"several things of the same kind",
		// Why a panel is the app's surface rather than something pasted on it.
		"var(--surface-panel)",
		"whichever theme the user is running",
		// The two hazards: what the sanitizer removes, and the one thing it
		// cannot catch — a width chosen for a window the model cannot see.
		"<style> element and a <script> are both removed",
		"minmax(0, 1fr)",
		// A panel is a way of saying something, not decoration.
		"single fact is decoration",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
}

// The chat renders an answer as markdown through DOMPurify, which passes SVG
// and strips scripts and handlers — so a drawing in an answer has always been
// possible, and never happened, because nothing said so. This layer says so.
//
// Written as the one condition where a picture beats prose (several things and
// how they relate) rather than as a list of occasions to draw: a list produces
// drawings on the listed occasions and paragraphs everywhere else, including
// the places three boxes would have ended the conversation.
func TestPromptTeachesWhenAPictureBeatsAParagraph(t *testing.T) {
	got := Build(SurfaceDesktop, Scope{Root: t.TempDir()})
	for _, want := range []string{
		"inline <svg>",
		"how several things relate",
		"currentColor",
		// Sized in viewBox units and set to full width: the model cannot know
		// how wide the panel is, and unsized <text> renders at 16px whatever
		// the scale and overflows the drawing.
		"width=\"100%\"",
		"font-size",
		// What the renderer removes without saying so. A legend laid out in
		// <foreignObject> leaves a hole the size of the legend and no error,
		// and the model has no way to find that out on its own.
		"<foreignObject>",
		"sanitizer, not a browser",
		// Markdown owns the answer the drawing sits in: a blank line inside one
		// hands the rest of it to the parser, a fence shows it as source.
		"no blank line inside it",
		"fenced block",
		// A drawing is not a decoration.
		"one fact, one number",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, got)
		}
	}
	// The kinds of thing worth drawing are examples inside the condition. The
	// named subjects of any one drawing must not become the trigger.
	for _, reject := range []string{"flowchart", "architecture diagram", "org chart", "mermaid"} {
		if strings.Contains(got, reject) {
			t.Errorf("prompt hardcodes the case %q instead of the principle:\n%s", reject, got)
		}
	}
}

// Both visual layers open with "your answer is rendered as markdown and the
// markup in it is drawn" — true of the desktop chat, false of a terminal. A
// CLI told its terminal draws SVG hands the user a page of path coordinates
// where the picture was meant to be.
func TestCLIPromptDoesNotTeachDrawing(t *testing.T) {
	got := Build(SurfaceCLI, Scope{Root: t.TempDir()})
	for _, leaked := range []string{"inline <svg>", "var(--surface-panel)"} {
		if strings.Contains(got, leaked) {
			t.Errorf("the CLI prompt teaches a renderer the terminal does not have: %q", leaked)
		}
	}
}

// A layer that names a tool must be able to ask whether the desk has it. The
// assistant desk carries no `diagnostics` — that is a `code` tool and the desk
// declares `agent, web, media, files, shell` — yet fileEditing told every desk
// to call it after changing a source file. The desk aimed at people who have
// never opened a terminal was being sent, on every code edit, after a tool it
// was never given.
func TestADeskIsNotToldToCallAToolItDoesNotCarry(t *testing.T) {
	without := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()}, Desk{
		Name:      "assistant",
		Direction: "This session is assistant work.",
		Carries:   func(name string) bool { return name != "diagnostics" },
	})
	if strings.Contains(without, "call diagnostics") {
		t.Errorf("a desk without diagnostics is still told to call it:\n%s", without)
	}
	with := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()}, Desk{
		Name:      "coding",
		Direction: "This session is coding work.",
		Carries:   func(string) bool { return true },
	})
	if !strings.Contains(with, "call diagnostics") {
		t.Errorf("a desk that carries diagnostics lost the step that verifies its edits:\n%s", with)
	}
}

// The tool loop has always executed every call in a reply, and nothing ever
// told the model it could send more than one. That silence is what turns four
// independent file reads into four round trips, and a round trip re-sends the
// whole conversation — the owner's own usage database showed 1,102 DeepSeek
// calls averaging 26.5K re-sent input tokens each.
//
// Unlike its neighbours this layer is not gated on a tool: it is about how calls
// are sent, not about any one of them. A stance that has taken the writing tools
// away still reads and greps, which is the shape that saves the most.
func TestEveryDeskIsToldItCanSendSeveralToolCallsAtOnce(t *testing.T) {
	for _, desk := range []Desk{
		{Name: "coding", Direction: "This session is coding work.", Carries: func(string) bool { return true }},
		// A stance holding only the reading tools — no shell, no write.
		{Name: "plan", Direction: "This session is planning.", Carries: func(name string) bool {
			return name != "shell" && name != "write" && name != "edit"
		}},
	} {
		got := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()}, desk)
		if !strings.Contains(got, "several tool calls in one reply") {
			t.Errorf("desk %q is never told it can batch calls:\n%s", desk.Name, got)
		}
		// Told to parallelize without the dependency test, a model batches the
		// read of a file against the write it is about to base on that read.
		if !strings.Contains(got, "The test is dependency, not similarity") {
			t.Errorf("desk %q got the encouragement without the rule that makes it safe:\n%s", desk.Name, got)
		}
	}
}

// A session with no tools at all reads tool instruction as a description of
// moves it cannot make, which is why the whole block is skipped rather than
// gated line by line. The new layer has to sit inside that skip like the rest.
func TestAToolLessDeskIsNotToldHowToSendToolCalls(t *testing.T) {
	got := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()}, Desk{
		Name:      "chat",
		Direction: "This session is conversation.",
		ToolLess:  true,
		Carries:   func(string) bool { return false },
	})
	if strings.Contains(got, "several tool calls in one reply") {
		t.Errorf("a desk with no tools is told how to batch them:\n%s", got)
	}
}

// The other half of the same mistake. The coding desk declares no `dispatch:`,
// and internal/subagent.available filters the office's agents out of the list
// its `task` tool advertises — so "hand the job to the agent whose craft it is"
// described a move with nobody on the other end. What survives at every desk is
// the lesson underneath it: length is not a request for a .docx.
func TestADeskThatCannotDelegateIsNotToldToHandWorkOver(t *testing.T) {
	cannot := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()}, Desk{
		Name:      "coding",
		Direction: "This session is coding work.",
		Carries:   func(string) bool { return true },
	})
	if strings.Contains(cannot, "hand the job to the agent") {
		t.Errorf("a desk with no dispatch is told to hand work to an agent it cannot reach:\n%s", cannot)
	}
	if !strings.Contains(cannot, "Length alone is not that request") {
		t.Errorf("the lesson that holds at every desk was gated away with the mechanism:\n%s", cannot)
	}
	can := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()}, Desk{
		Name:      "assistant",
		Direction: "This session is assistant work.",
		Carries:   func(string) bool { return true },
		Delegates: true,
	})
	if !strings.Contains(can, "hand the job to the agent") {
		t.Errorf("the desk that can delegate lost the instruction to:\n%s", can)
	}
}
