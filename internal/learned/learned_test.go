package learned

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points the data root at a temp dir and returns both halves the tests
// assert against: the memory folder, and the agents' home next to it.
func isolate(t *testing.T) (memoryDir, agentsDir string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	return filepath.Join(root, "memory"), filepath.Join(root, "agents")
}

// The portability promise: plain markdown, in a folder, under names another
// agent runtime already understands.
//
// Two homes since 2026-08-06, and the split is the point. What has no folder of
// its own — the main agent, each desk — stays under memory/. A delegate's goes
// inside that worker's own folder, because one agent is one folder and its
// memory is part of what it is.
func TestMemoryIsPlainMarkdownAtAPredictablePath(t *testing.T) {
	dir, agents := isolate(t)

	main, err := FileFor(MainScope)
	if err != nil {
		t.Fatalf("FileFor(main): %v", err)
	}
	if want := filepath.Join(dir, "MEMORY.md"); main != want {
		t.Errorf("main memory at %q, want %q", main, want)
	}

	desk, err := FileFor(ModeScope("coding"))
	if err != nil {
		t.Fatalf("FileFor(mode:coding): %v", err)
	}
	if want := filepath.Join(dir, "modes", "coding.md"); desk != want {
		t.Errorf("desk memory at %q, want %q", desk, want)
	}

	child, err := FileFor("explore")
	if err != nil {
		t.Fatalf("FileFor(explore): %v", err)
	}
	if want := filepath.Join(agents, "explore", "MEMORY.md"); child != want {
		t.Errorf("delegate memory at %q, want %q", child, want)
	}
}

// The pre-folder layout must not strand what an agent already learned. Both
// halves move, and the old files are gone afterwards so nothing reads a stale
// copy — the whole point of one agent being one folder.
func TestOldMemoryFilesMoveIntoTheAgentsFolder(t *testing.T) {
	dir, agents := isolate(t)

	old := filepath.Join(dir, "agents", "doc.md")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(old, []byte("จำได้ว่าเจ้านายชอบตารางสั้น"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := Read("doc"); got != "จำได้ว่าเจ้านายชอบตารางสั้น" {
		t.Fatalf("Read(doc) = %q — the old memory did not survive the move", got)
	}
	if _, err := os.Stat(filepath.Join(agents, "doc", "MEMORY.md")); err != nil {
		t.Fatalf("memory is not in the agent's folder: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("the pre-folder file is still there — two copies, and only one of them is read")
	}
}

// A scope reaches this from a profile name, which reaches it from a tool call
// the model wrote — so it is a trust boundary, not a formatting rule.
func TestScopeCannotEscapeTheMemoryFolder(t *testing.T) {
	isolate(t)
	for _, bad := range []string{"..", "../../etc", `..\..\Windows`, "a/b"} {
		if _, err := FileFor(bad); err == nil {
			t.Errorf("scope %q was accepted; it must be refused", bad)
		}
	}
}

func TestApplyAddsReplacesAndRemovesLines(t *testing.T) {
	isolate(t)

	if err := Apply(MainScope, OpAdd, "", "สแกนเนอร์ของเครื่องนี้เขียนไฟล์ลง D:\\Scans"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := Apply(MainScope, OpAdd, "", "ใบเสร็จร้านนี้วางยอดรวมไว้เหนือวันที่"); err != nil {
		t.Fatalf("add second: %v", err)
	}
	got := Read(MainScope)
	if !strings.Contains(got, "D:\\Scans") || !strings.Contains(got, "เหนือวันที่") {
		t.Fatalf("both lines should be readable back:\n%s", got)
	}

	// Substring match, because the agent has the text in its prompt and a line
	// number would go stale the moment anything above it moved.
	if err := Apply(MainScope, OpReplace, "สแกนเนอร์", "สแกนเนอร์เขียนลง E:\\Scans แล้ว"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if got := Read(MainScope); strings.Contains(got, "D:\\Scans") || !strings.Contains(got, "E:\\Scans") {
		t.Fatalf("replace did not take:\n%s", got)
	}

	if err := Apply(MainScope, OpRemove, "เหนือวันที่", ""); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := Read(MainScope); strings.Contains(got, "เหนือวันที่") {
		t.Fatalf("remove did not take:\n%s", got)
	}
}

// The settings page edits by row position, and this is the case that forced
// it: a real memory file collects lines that differ only at the very end. Ask
// for the fourth by substring and findEntry hands back the first.
func TestEditEntryHitsTheRowTheUserIsLookingAt(t *testing.T) {
	isolate(t)
	for _, status := range []string{"1", "2", "124", "255"} {
		if err := Apply(MainScope, OpAdd, "",
			`เครื่องมือ shell เคยล้มซ้ำ ๆ ด้วยเหตุเดียวกัน: "exit status `+status+`"`); err != nil {
			t.Fatalf("add %s: %v", status, err)
		}
	}
	entries := Entries(MainScope)
	if len(entries) != 4 {
		t.Fatalf("Entries = %d lines, want 4: %v", len(entries), entries)
	}

	// Row 3 is "exit status 124". A substring edit for "exit status" would take
	// row 0 instead — which is the whole reason EditEntry counts.
	if err := EditEntry(MainScope, 3, "เครื่องมือ shell timeout บ่อยในโปรเจกต์นี้"); err != nil {
		t.Fatalf("edit row 3: %v", err)
	}
	after := Entries(MainScope)
	if after[3] != "เครื่องมือ shell timeout บ่อยในโปรเจกต์นี้" {
		t.Errorf("row 3 = %q, want the edited text", after[3])
	}
	if !strings.Contains(after[0], "exit status 1") {
		t.Errorf("row 0 was rewritten instead: %q", after[0])
	}
	if len(after) != 4 {
		t.Fatalf("editing changed the line count: %v", after)
	}

	// Empty text is how a row is forgotten — the button beside it has nothing
	// else to send.
	if err := EditEntry(MainScope, 0, "  "); err != nil {
		t.Fatalf("delete row 0: %v", err)
	}
	left := Entries(MainScope)
	if len(left) != 3 || strings.Contains(strings.Join(left, "\n"), "exit status 1\"") {
		t.Errorf("row 0 should be gone, got %v", left)
	}

	// A row that moved under the user — a second window, or an approval that
	// landed while this page sat open — is refused rather than applied to
	// whatever now sits at that index.
	if err := EditEntry(MainScope, 9, "x"); err == nil {
		t.Error("editing past the end must fail, not write to the wrong row")
	}
	if err := EditEntry(MainScope, -1, "x"); err == nil {
		t.Error("a negative index must fail")
	}
}

// The header is the file explaining itself to whoever opens the folder. An edit
// through the window must leave it there — it is not one of the entries.
func TestEditEntryKeepsTheFileExplainingItself(t *testing.T) {
	isolate(t)
	if err := Apply(MainScope, OpAdd, "", "จำอันนี้ไว้"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := EditEntry(MainScope, 0, "จำอันนี้ไว้แทน"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	path, err := FileFor(MainScope)
	if err != nil {
		t.Fatalf("FileFor: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.HasPrefix(string(raw), "# Learned by ") {
		t.Errorf("the header did not survive an edit:\n%s", raw)
	}
}

// This used to assert the opposite, on the grounds that "an error the agent can
// act on beats a silent no-op". The error was right and the caller was wrong:
// Apply runs at approval time, where the agent is not present and the only
// person who could act on it holds one button that means "do not learn this".
// The refusal moved to the tool (TestARevisionOfSomethingUnrememberedIsRefused);
// what is left here is the race the design invites, because the memory files
// are plain markdown the user is told to edit.
func TestApprovingAStaleRevisionStillReachesTheStateApproved(t *testing.T) {
	isolate(t)
	if err := Apply(MainScope, OpAdd, "", "ผู้ใช้ใช้ Windows"); err != nil {
		t.Fatalf("add: %v", err)
	}

	// The line it names is gone, but what the user approved is that memory
	// should say this — and now it does.
	if err := Apply(MainScope, OpReplace, "ไม่มีบรรทัดนี้", "ผู้ใช้เป็นนักพัฒนา Aetox"); err != nil {
		t.Fatalf("a replace whose target is gone should still land: %v", err)
	}
	if got := Entries(MainScope); len(got) != 2 || got[1] != "ผู้ใช้เป็นนักพัฒนา Aetox" {
		t.Fatalf("the approved line is not in memory: %q", got)
	}

	// Twice is once. Approving the same card from two open windows must not
	// leave the fact in the file twice.
	if err := Apply(MainScope, OpReplace, "ไม่มีบรรทัดนี้", "ผู้ใช้เป็นนักพัฒนา Aetox"); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if got := Entries(MainScope); len(got) != 2 {
		t.Errorf("a re-applied replace duplicated the line: %q", got)
	}

	// Forgetting what is already forgotten is the state the user asked for.
	if err := Apply(MainScope, OpRemove, "ไม่มีบรรทัดนี้", ""); err != nil {
		t.Errorf("removing a line that is already gone should be a no-op: %v", err)
	}
	if got := Entries(MainScope); len(got) != 2 {
		t.Errorf("a no-op remove changed the file: %q", got)
	}
}

func TestHasAnswersTheSameQuestionApplyAsks(t *testing.T) {
	isolate(t)
	if err := Apply(MainScope, OpAdd, "", "สแกนเนอร์เขียนลง E:\\Scans"); err != nil {
		t.Fatalf("add: %v", err)
	}
	for _, c := range []struct {
		needle string
		want   bool
	}{
		{"สแกนเนอร์", true},
		{"E:\\Scans", true},
		{"ไม่มีบรรทัดนี้", false},
		{"", false},
		{"   ", false},
	} {
		if got := Has(MainScope, c.needle); got != c.want {
			t.Errorf("Has(%q) = %v, want %v", c.needle, got, c.want)
		}
	}
	if Has(MainScope+"-unwritten", "อะไรก็ได้") {
		t.Error("a scope with no file cannot contain anything")
	}
}

// The written file has to explain itself: someone opens this folder in six
// months, or drops it into another agent, and a bare list of assertions reads
// as configuration to obey rather than as something that was learned.
func TestTheFileExplainsItselfButThePromptDoesNotPayForIt(t *testing.T) {
	isolate(t)
	if err := Apply("explore", OpAdd, "", "โปรเจกต์นี้เก็บ fixture ไว้ที่ testdata/"); err != nil {
		t.Fatalf("add: %v", err)
	}
	path, _ := FileFor("explore")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(raw), "approved by you") {
		t.Errorf("the file should say where its contents came from:\n%s", raw)
	}
	if !strings.Contains(string(raw), "explore") {
		t.Errorf("the file should name whose memory it is:\n%s", raw)
	}
	if got := Read("explore"); strings.Contains(got, "approved by you") {
		t.Errorf("the explanation is for a person, not for the prompt:\n%s", got)
	}
}

// A quota exists because this text rides in a system prompt forever. Being
// told is what lets the agent consolidate; being ignored would teach it that
// writing memory works when it did not.
func TestMemoryIsCappedAndSaysSo(t *testing.T) {
	isolate(t)
	line := strings.Repeat("ก", 500)
	var lastErr error
	for i := 0; i < 40; i++ {
		if err := Apply(MainScope, OpAdd, "", line+string(rune('a'+i))); err != nil {
			lastErr = err
			break
		}
	}
	if lastErr == nil {
		t.Fatal("memory should have filled up and refused a write")
	}
	if !strings.Contains(lastErr.Error(), "full") {
		t.Errorf("the refusal should say what happened, got %q", lastErr)
	}
	// Full is what the tool consults before proposing, so it has to agree with
	// Apply about the write that was actually refused — not about some smaller
	// one that would still fit.
	if !Full(MainScope, len(line)) {
		t.Error("Full should agree with Apply that another line of this size does not fit")
	}
}

// Reading a scope nothing has been written to is the normal state on every
// fresh install, not an error to handle.
func TestUnwrittenScopeReadsEmpty(t *testing.T) {
	isolate(t)
	if got := Read(MainScope); got != "" {
		t.Errorf("want empty, got %q", got)
	}
	if got := Read("explore"); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

// A desk's memory is its own file, beside the delegates' and under the same
// rules — one more scope value, not a second mechanism (ARCHITECTURE.md §83).
// The three namespaces have to stay apart on disk, because a desk and a
// sub-agent can legitimately share a name.
func TestModeScopeIsItsOwnFileBesideTheOthers(t *testing.T) {
	isolate(t)

	if err := Apply(ModeScope("coding"), OpAdd, "", "this repo runs its tests with a script"); err != nil {
		t.Fatalf("write desk memory: %v", err)
	}
	if err := Apply(MainScope, OpAdd, "", "the user works in Thai"); err != nil {
		t.Fatalf("write shared memory: %v", err)
	}
	if err := Apply("coding", OpAdd, "", "a delegate that happens to share the name"); err != nil {
		t.Fatalf("write delegate memory: %v", err)
	}

	desk, err := FileFor(ModeScope("coding"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(desk), "memory/modes/coding.md") {
		t.Errorf("a desk's memory landed at %q, want memory/modes/coding.md", desk)
	}
	agent, err := FileFor("coding")
	if err != nil {
		t.Fatal(err)
	}
	if agent == desk {
		t.Fatalf("a desk and a sub-agent of the same name share one file: %q", desk)
	}

	if got := Read(ModeScope("coding")); !strings.Contains(got, "tests with a script") {
		t.Errorf("the desk scope read back %q", got)
	}
	if got := Read(ModeScope("coding")); strings.Contains(got, "works in Thai") {
		t.Error("the shared memory leaked into the desk's own file")
	}
	if got := Read(MainScope); strings.Contains(got, "tests with a script") {
		t.Error("what one desk learned reached the file every desk pays for")
	}
	// The header explains the file to whoever opens the folder, and says which
	// desk it belongs to rather than calling it a sub-agent.
	raw, err := os.ReadFile(desk)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "coding desk") {
		t.Errorf("the desk file does not say whose it is:\n%s", raw)
	}
}

// The third axis, and the reason it exists: a desk is the same desk in every
// repository, so what one project settled must not arrive as advice in the
// next one (owner, 16 ส.ค.). Two projects, two files, and neither reaches the
// shared memory every session pays for.
func TestAProjectsMemoryStaysInThatProject(t *testing.T) {
	isolate(t)

	here := filepath.Join(t.TempDir(), "Aetox")
	there := filepath.Join(t.TempDir(), "Tennis")
	if err := Apply(ProjectScope(here), OpAdd, "", "statereport marks an error as coming from the world"); err != nil {
		t.Fatalf("write project memory: %v", err)
	}
	if err := Apply(ProjectScope(there), OpAdd, "", "the coach list is keyed by phone"); err != nil {
		t.Fatalf("write the other project's memory: %v", err)
	}

	path, err := FileFor(ProjectScope(here))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(path), "memory/projects/Aetox-") {
		t.Errorf("a project's memory landed at %q, want memory/projects/<key>.md", path)
	}
	if got := Read(ProjectScope(here)); !strings.Contains(got, "statereport") {
		t.Errorf("the project scope read back %q", got)
	}
	if got := Read(ProjectScope(here)); strings.Contains(got, "keyed by phone") {
		t.Error("one project's memory reached another's file")
	}
	if got := Read(MainScope); got != "" {
		t.Errorf("a project decision reached the memory every session carries: %q", got)
	}
	// Reopening the same folder spelled differently is the same project — the
	// key is path-cleaned and case-folded, and a memory that missed on a
	// trailing slash would be a memory the user never sees again.
	if ProjectScope(here+string(filepath.Separator)) != ProjectScope(here) {
		t.Error("a trailing separator produced a different project")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "project") {
		t.Errorf("the project file does not say whose it is:\n%s", raw)
	}
}

// Folder names on this platform have spaces in them, and validScope refuses a
// scope with one. Unsanitized, "My App" would be a project that silently could
// never have a memory at all.
func TestAProjectWhoseNameIsNotFilenameSafeStillGetsAFile(t *testing.T) {
	isolate(t)

	root := filepath.Join(t.TempDir(), "My App")
	if err := Apply(ProjectScope(root), OpAdd, "", "จำอันนี้ไว้"); err != nil {
		t.Fatalf("write: %v", err)
	}
	path, err := FileFor(ProjectScope(root))
	if err != nil {
		t.Fatalf("FileFor: %v", err)
	}
	if strings.ContainsAny(filepath.Base(path), ` \/:*?"<>|`) {
		t.Errorf("unsafe characters survived into the filename %q", path)
	}
	if got := Read(ProjectScope(root)); !strings.Contains(got, "จำอันนี้ไว้") {
		t.Errorf("read back %q", got)
	}
	// Two folders whose names flatten to the same thing are still two projects:
	// the hash half is what makes the key unique, and it is untouched.
	other := filepath.Join(t.TempDir(), "My-App")
	if ProjectScope(root) == ProjectScope(other) {
		t.Error("two different folders collapsed into one project")
	}
}

// No project open is a real state, not an error. It must land where it always
// landed rather than inventing a project named after the home directory.
func TestNoProjectMeansTheSharedMemory(t *testing.T) {
	if got := ProjectScope("   "); got != MainScope {
		t.Errorf("ProjectScope(empty) = %q, want the shared scope", got)
	}
}

// The review page can only show a memory it knows exists. Before Scopes it
// listed the main agent's file and nothing else, so a line approved into a desk
// or a project was visible only by opening the folder.
func TestScopesListsEveryMemoryThatHoldsSomething(t *testing.T) {
	isolate(t)
	root := filepath.Join(t.TempDir(), "Aetox")

	if got := Scopes(); len(got) != 0 {
		t.Fatalf("a machine that has learned nothing listed %v", got)
	}
	for _, w := range []struct{ scope, line string }{
		{MainScope, "the user works in Thai"},
		{ModeScope("coding"), "the owner reads the diff first"},
		{ProjectScope(root), "we settled on PowerShell here"},
	} {
		if err := Apply(w.scope, OpAdd, "", w.line); err != nil {
			t.Fatalf("write %q: %v", w.scope, err)
		}
	}

	got := Scopes()
	if len(got) != 3 || got[0] != MainScope {
		t.Fatalf("Scopes() = %v, want the shared one first and all three present", got)
	}
	var hasDesk, hasProject bool
	for _, s := range got {
		if _, ok := SplitModeScope(s); ok {
			hasDesk = true
		}
		if _, ok := SplitProjectScope(s); ok {
			hasProject = true
		}
	}
	if !hasDesk || !hasProject {
		t.Errorf("Scopes() = %v, want a desk and a project in it", got)
	}
	// A delegate's memory lives in that worker's own folder and has its own
	// page. Listing it here would put it on a page that cannot address it.
	if err := Apply("doc", OpAdd, "", "a delegate learned something"); err != nil {
		t.Fatal(err)
	}
	for _, s := range Scopes() {
		if s == "doc" {
			t.Error("a delegate's memory was listed with the main agent's")
		}
	}
}

// An unknown scope shape must not be able to walk out of the memory folder —
// the desk name arrives from a database column.
func TestModeScopeRefusesPathShapedDesks(t *testing.T) {
	isolate(t)
	for _, desk := range []string{"..", "a/b", `a\b`, "a b"} {
		if _, err := FileFor(ModeScope(desk)); err == nil {
			t.Errorf("FileFor(mode:%s) accepted a path-shaped desk", desk)
		}
	}
}
