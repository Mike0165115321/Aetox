package snapshot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newProject builds a real git repository, because every claim this package
// makes is a claim about what git does. A fake would test the fake.
func newProject(t *testing.T) *Store {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on this machine")
	}
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	work := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	store, err := New(work)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func write(t *testing.T, store *Store, rel, body string) {
	t.Helper()
	full := filepath.Join(store.workTree, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, store *Store, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(store.workTree, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// The shape of a real undo: take a snapshot, let the agent make a mess, put one
// file back and leave the rest of its work alone.
func TestRestoreUndoesOneFileAndLeavesTheOthers(t *testing.T) {
	store := newProject(t)
	write(t, store, "keep.go", "original keep\n")
	write(t, store, "break.go", "original break\n")

	before, err := store.Capture(context.Background())
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	write(t, store, "keep.go", "a good edit\n")
	write(t, store, "break.go", "a bad edit\n")

	restored, err := store.Restore(context.Background(), before, []string{"break.go"})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(restored) != 1 || restored[0] != "break.go" {
		t.Errorf("restored = %v, want just break.go", restored)
	}
	if got := read(t, store, "break.go"); got != "original break\n" {
		t.Errorf("break.go = %q, want it back to how it was", got)
	}
	if got := read(t, store, "keep.go"); got != "a good edit\n" {
		t.Errorf("keep.go = %q — undoing one file must not undo the others", got)
	}
}

// "Restore to before" has to include "before it existed", or a file the agent
// created can never be undone.
func TestRestoreDeletesAFileThatDidNotExistYet(t *testing.T) {
	store := newProject(t)
	write(t, store, "existing.txt", "here all along\n")
	before, err := store.Capture(context.Background())
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	write(t, store, "invented.txt", "the agent made this up\n")
	if _, err := store.Restore(context.Background(), before, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if _, err := os.Stat(filepath.Join(store.workTree, "invented.txt")); !os.IsNotExist(err) {
		t.Error("a file created after the snapshot survived the undo")
	}
	if got := read(t, store, "existing.txt"); got != "here all along\n" {
		t.Errorf("existing.txt = %q, want it untouched", got)
	}
}

// An unchanged tree must snapshot to the same id, which is how a caller answers
// "did this turn change anything" without diffing.
func TestCaptureIsStableWhenNothingChanged(t *testing.T) {
	store := newProject(t)
	write(t, store, "a.txt", "same\n")

	first, err := store.Capture(context.Background())
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	second, err := store.Capture(context.Background())
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if first != second {
		t.Errorf("ids differ for an unchanged tree: %s vs %s", first, second)
	}

	write(t, store, "a.txt", "different\n")
	third, err := store.Capture(context.Background())
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if third == first {
		t.Error("the id did not change after the file did")
	}
}

func TestChangedListsWhatMoved(t *testing.T) {
	store := newProject(t)
	write(t, store, "a.txt", "one\n")
	write(t, store, "b.txt", "two\n")
	before, _ := store.Capture(context.Background())

	write(t, store, "b.txt", "two, edited\n")
	after, _ := store.Capture(context.Background())

	changed, err := store.Changed(context.Background(), before, after)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if len(changed) != 1 || changed[0] != "b.txt" {
		t.Errorf("Changed = %v, want just b.txt", changed)
	}
}

// The promise that makes this safe to run on every turn: the user's own
// repository must not notice. No commits, no staged changes, no stash.
func TestSnapshotsAreInvisibleToTheProjectRepo(t *testing.T) {
	store := newProject(t)
	write(t, store, "a.txt", "content\n")
	if _, err := store.Capture(context.Background()); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = store.workTree
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v: %s", err, out)
	}
	// a.txt is untracked in the user's repo and must have stayed that way —
	// "?? a.txt", never "A  a.txt".
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "??") {
		t.Errorf("the snapshot changed the user's index:\n%s", out)
	}

	log := exec.Command("git", "log", "--oneline")
	log.Dir = store.workTree
	if out, _ := log.CombinedOutput(); strings.Contains(string(out), "snapshot") {
		t.Errorf("a snapshot reached the user's history:\n%s", out)
	}
}

// .gitignore is git's own answer to "what is not the user's code", and a
// snapshot has no business disagreeing with it.
func TestSnapshotHonoursGitignore(t *testing.T) {
	store := newProject(t)
	write(t, store, ".gitignore", "build/\n")
	write(t, store, "build/artifact.bin", "generated\n")
	write(t, store, "src.go", "package main\n")

	id, err := store.Capture(context.Background())
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	files, err := store.filesIn(context.Background(), id)
	if err != nil {
		t.Fatalf("filesIn: %v", err)
	}
	if files["build/artifact.bin"] {
		t.Error("ignored build output was snapshotted")
	}
	if !files["src.go"] {
		t.Error("real source was not snapshotted")
	}
}

// A project that is not a repository, or a machine with no git, must degrade to
// "no undo" rather than to an error the user has to understand.
func TestNewRefusesAnEmptyPath(t *testing.T) {
	if _, err := New("   "); err != ErrUnavailable {
		t.Errorf("New(\"\") = %v, want ErrUnavailable", err)
	}
}
