package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/snapshot"
)

// undoApp is the App reduced to what undo touches: a real git project and a
// real store. Nothing is stubbed — the whole claim of this feature is about
// what happens to files on disk.
func undoApp(t *testing.T) *App {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on this machine")
	}
	isolateUserDirs(t)

	work := t.TempDir()
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "t"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	store, err := snapshot.New(work)
	if err != nil {
		t.Fatalf("snapshot.New: %v", err)
	}
	return &App{ctx: context.Background(), snapshots: store}
}

func TestUndoLastTurnPutsFilesBack(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	if err := os.WriteFile(filepath.Join(root, "code.go"), []byte("good\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.captureSnapshot(app.cur()) // what SendMessage does before a turn runs
	if err := os.WriteFile(filepath.Join(root, "code.go"), []byte("the agent broke it\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The UI asks this first, to decide whether to offer undo at all.
	if pending := app.PendingUndo(); len(pending) != 1 || pending[0] != "code.go" {
		t.Errorf("PendingUndo = %v, want code.go", pending)
	}

	result, err := app.UndoLastTurn()
	if err != nil {
		t.Fatalf("UndoLastTurn: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0] != "code.go" {
		t.Fatalf("Files = %v (%s), want code.go", result.Files, result.Reason)
	}
	data, _ := os.ReadFile(filepath.Join(root, "code.go"))
	if string(data) != "good\n" {
		t.Errorf("code.go = %q, want the pre-turn content", string(data))
	}
}

// Pressing undo twice must not walk further back. The second press has nothing
// to undo, because the restore is itself the state now.
func TestUndoTwiceDoesNotStepFurtherBack(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.captureSnapshot(app.cur())
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := app.UndoLastTurn(); err != nil {
		t.Fatalf("first undo: %v", err)
	}
	again, err := app.UndoLastTurn()
	if err != nil {
		t.Fatalf("second undo: %v", err)
	}
	if len(again.Files) != 0 {
		t.Errorf("the second undo changed %v — it must be a no-op", again.Files)
	}
	data, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(data) != "first\n" {
		t.Errorf("a.txt = %q, want it to have stayed at the first undo's result", string(data))
	}
}

// No git repository is the common case for a scratch folder, and it must read
// as an explanation rather than an error.
func TestUndoWithoutASnapshotStoreExplainsItself(t *testing.T) {
	app := &App{ctx: context.Background()}

	result, err := app.UndoLastTurn()
	if err != nil {
		t.Fatalf("UndoLastTurn must not fail without a store: %v", err)
	}
	if result.Reason == "" {
		t.Error("want a reason the user can read")
	}
	if got := app.PendingUndo(); got == nil {
		t.Error("PendingUndo returned nil — §34, a nil slice crashes the frontend")
	}
}

func TestUndoBeforeAnyTurnHasNothingToDo(t *testing.T) {
	app := undoApp(t)

	result, err := app.UndoLastTurn()
	if err != nil {
		t.Fatalf("UndoLastTurn: %v", err)
	}
	if len(result.Files) != 0 || result.Reason == "" {
		t.Errorf("result = %+v, want nothing undone and a reason", result)
	}
}
