package skill

// The guard in front of a whole-file write.
//
// Owner, 24 ส.ค.: *"write ทับทั้งไฟล์ ไม่เช็คอะไรเลย อันตรายมาก"*. `os.WriteFile`
// truncates, nothing in this program locks, and the last writer wins — so the
// case that matters is the one where two writers share a tree: the agent, and
// the person typing in the editor beside it.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeApp is the write skill wired to a real folder and a real record.
func writeSkillIn(t *testing.T) (*writeSkill, *FileState, string) {
	t.Helper()
	root := t.TempDir()
	files := NewFileState()
	return &writeSkill{root: root, files: files}, files, root
}

// The whole point, in one test: the agent read a file, the person changed it,
// and the agent's whole-file write is refused instead of winning.
func TestWriteRefusesAFileThatMovedSinceItWasRead(t *testing.T) {
	skill, files, root := writeSkillIn(t)
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("what the agent read\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files.Note(path)

	// The person types, and the editor saves. mtime granularity is coarse on
	// some filesystems, so the content differs in length as well — which is what
	// the size half of the stamp is for.
	if err := os.WriteFile(path, []byte("what I was actually writing, at length\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := skill.ExecuteTool(context.Background(), map[string]any{
		"path": "notes.md", "content": "the agent's whole-file rewrite\n",
	})
	if err == nil {
		t.Fatalf("want a refusal, got success: %+v", out)
	}
	// The message has to name the act, not the state: the model is who has to
	// resolve it, and a refusal it cannot act on becomes a retry loop.
	if got := err.Error(); !contains(got, "Read it again") {
		t.Errorf("refusal does not say what to do: %q", got)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "what I was actually writing, at length\n" {
		t.Errorf("the file was overwritten anyway: %q", string(data))
	}
}

// Writing a file nobody here has looked at is what `write` is FOR. A guard that
// demanded a prior read would refuse the ordinary case.
func TestWriteAllowsAFileThisAppHasNeverSeen(t *testing.T) {
	skill, _, root := writeSkillIn(t)
	if err := os.WriteFile(filepath.Join(root, "theirs.md"), []byte("was here first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := skill.ExecuteTool(context.Background(), map[string]any{
		"path": "theirs.md", "content": "replaced\n",
	}); err != nil {
		t.Fatalf("a blind overwrite is the tool's job: %v", err)
	}
}

// The agent's own second write must not refuse itself. Every writer updates the
// record, so the state it left behind is the state it finds.
func TestWriteTwiceInARowIsFine(t *testing.T) {
	skill, _, root := writeSkillIn(t)
	for i, content := range []string{"one\n", "two\n", "three\n"} {
		if _, err := skill.ExecuteTool(context.Background(), map[string]any{
			"path": "draft.md", "content": content,
		}); err != nil {
			t.Fatalf("write %d refused itself: %v", i+1, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(root, "draft.md"))
	if string(data) != "three\n" {
		t.Errorf("draft.md = %q", string(data))
	}
}

// A read is what clears the refusal, which is exactly what the message tells
// the model to do. If it did not, the guard would be a dead end.
func TestReadingAgainClearsTheRefusal(t *testing.T) {
	skill, files, root := writeSkillIn(t)
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files.Note(path)
	if err := os.WriteFile(path, []byte("changed underneath, and longer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !files.ChangedSinceSeen(path) {
		t.Fatal("the change was not noticed at all")
	}

	// What `read` does on its way past.
	files.Note(path)

	if files.ChangedSinceSeen(path) {
		t.Error("reading it again left the file still reading as stale")
	}
	if _, err := skill.ExecuteTool(context.Background(), map[string]any{
		"path": "notes.md", "content": "now written on purpose\n",
	}); err != nil {
		t.Fatalf("write after a fresh read: %v", err)
	}
}

// A deleted file is forgotten, not remembered as it was: writing the name again
// is a creation, and refusing that would be refusing the obvious thing.
func TestADeletedFileIsForgotten(t *testing.T) {
	files := NewFileState()
	root := t.TempDir()
	path := filepath.Join(root, "gone.md")
	if err := os.WriteFile(path, []byte("here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files.Note(path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	files.Forget(path)

	if files.ChangedSinceSeen(path) {
		t.Error("a name nobody holds must not read as stale")
	}
}

// Nil is a supported configuration — the CLI has one writer and nobody typing
// beside it — and must be a no-op rather than a crash.
func TestTheGuardIsOptional(t *testing.T) {
	var files *FileState
	files.Note("anything")
	files.Forget("anything")
	if files.ChangedSinceSeen("anything") {
		t.Error("a record that does not exist cannot report a change")
	}
	if err := files.guardStale("a", "a"); err != nil {
		t.Errorf("nil guard refused: %v", err)
	}
}

// Size alone is not enough: a file rewritten to the same length by somebody
// else is still somebody else's work.
func TestASameLengthRewriteIsStillAChange(t *testing.T) {
	files := NewFileState()
	root := t.TempDir()
	path := filepath.Join(root, "same.md")
	if err := os.WriteFile(path, []byte("aaaa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files.Note(path)

	// Ahead of the stamp by more than any filesystem's tick, because what is
	// being checked is the comparison and not the clock.
	later := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(path, []byte("bbbb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	if !files.ChangedSinceSeen(path) {
		t.Error("a same-length rewrite went unnoticed")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
