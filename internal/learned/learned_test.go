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

// Editing something that is not there is an error the agent can act on, not a
// silent no-op it would read as success.
func TestReplacingSomethingAbsentFails(t *testing.T) {
	isolate(t)
	if err := Apply(MainScope, OpReplace, "ไม่มีบรรทัดนี้", "x"); err == nil {
		t.Fatal("replacing a line that does not exist should fail")
	}
	if err := Apply(MainScope, OpRemove, "ไม่มีบรรทัดนี้", ""); err == nil {
		t.Fatal("removing a line that does not exist should fail")
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
