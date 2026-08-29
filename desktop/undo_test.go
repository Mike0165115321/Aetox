package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Mikedev115/Aetox/internal/snapshot"
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

	app.captureSnapshot(app.cur(), "") // what SendMessage does before a turn runs
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
	app.captureSnapshot(app.cur(), "")
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

// The half §157 left open: an undo used to put back EVERY file that differed
// from the snapshot, and the snapshot store is the whole work tree that every
// chat shares. So pressing undo to reject one answer also threw away whatever
// the person had typed while the turn ran (owner, 24 ส.ค.).
func TestUndoLeavesTheUserOwnSavesAlone(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	for _, name := range []string{"code.go", "mine.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	app.captureSnapshot(app.cur(), "")
	// The agent changes one file...
	if err := os.WriteFile(filepath.Join(root, "code.go"), []byte("the agent broke it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// ...while the person types in another and the editor saves it.
	if err := app.WriteFile("mine.md", "what I was writing\n"); err != nil {
		t.Fatal(err)
	}

	// The chip must promise exactly what the button will do.
	if pending := app.PendingUndo(); len(pending) != 1 || pending[0] != "code.go" {
		t.Errorf("PendingUndo = %v, want only code.go", pending)
	}

	result, err := app.UndoLastTurn()
	if err != nil {
		t.Fatalf("UndoLastTurn: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0] != "code.go" {
		t.Errorf("Files = %v, want only code.go", result.Files)
	}
	// Named, not merely skipped: "I left one of your files alone" is only useful
	// if it says which one.
	if len(result.Kept) != 1 || result.Kept[0] != "mine.md" {
		t.Errorf("Kept = %v, want mine.md", result.Kept)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "mine.md")); string(data) != "what I was writing\n" {
		t.Errorf("mine.md = %q — the user's own work was put back over", string(data))
	}
	if data, _ := os.ReadFile(filepath.Join(root, "code.go")); string(data) != "before\n" {
		t.Errorf("code.go = %q, want the pre-turn content", string(data))
	}
}

// Everything that moved was the person's own typing. "Nothing changed" would be
// false, and to somebody who had just been typing it would read as "your work
// is gone".
func TestUndoSaysSoWhenOnlyTheUserFilesMoved(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	if err := os.WriteFile(filepath.Join(root, "mine.md"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.captureSnapshot(app.cur(), "")
	if err := app.WriteFile("mine.md", "what I was writing\n"); err != nil {
		t.Fatal(err)
	}

	result, err := app.UndoLastTurn()
	if err != nil {
		t.Fatalf("UndoLastTurn: %v", err)
	}
	if len(result.Files) != 0 || result.Reason == "" {
		t.Fatalf("want nothing restored and a reason, got %+v", result)
	}
	if len(result.Kept) != 1 || result.Kept[0] != "mine.md" {
		t.Errorf("Kept = %v, want mine.md", result.Kept)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "mine.md")); string(data) != "what I was writing\n" {
		t.Errorf("mine.md = %q — untouched was the whole promise", string(data))
	}
}

// The list describes one turn. A save from two turns ago is not a reason to
// spare a file the turn just rewrote — by then the user has seen it and moved on.
func TestUserSavesDoNotOutliveTheirTurn(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.captureSnapshot(app.cur(), "")
	if err := app.WriteFile("notes.md", "mine\n"); err != nil {
		t.Fatal(err)
	}
	// A second turn begins, and nobody has typed since.
	app.captureSnapshot(app.cur(), "")
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("the agent rewrote it\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := app.UndoLastTurn()
	if err != nil {
		t.Fatalf("UndoLastTurn: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0] != "notes.md" {
		t.Fatalf("Files = %v (%s), want notes.md", result.Files, result.Reason)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "notes.md")); string(data) != "mine\n" {
		t.Errorf("notes.md = %q, want the state at the start of THIS turn", string(data))
	}
}

// A save is a fact about the tree, and the chat whose undo might eat it is very
// often not the chat being looked at — that is what background work means.
func TestUserSaveIsRememberedByEveryLiveChat(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	if err := os.WriteFile(filepath.Join(root, "mine.md"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The chat working in the background takes its snapshot. Shown first,
	// because that is what files it under its id — a chat left running when the
	// window moves on is one that was on screen a moment ago.
	background := app.cur()
	background.id = "20260824-000000.001"
	app.convs.show(background)
	app.captureSnapshot(background, "")
	// ...the window moves to another chat, and the person saves from there.
	other := newConversation()
	other.id = "20260824-000000.002"
	app.convs.show(other)
	app.cur().cfg.SandboxRoot = root
	if err := app.WriteFile("mine.md", "mine\n"); err != nil {
		t.Fatal(err)
	}

	app.snapshotMu.Lock()
	saves := make([]string, 0, len(background.userSaves))
	for _, saved := range background.userSaves {
		saves = append(saves, saved.Path)
	}
	app.snapshotMu.Unlock()
	if len(saves) != 1 || saves[0] != "mine.md" {
		t.Fatalf("the background chat did not hear about the save: %v", saves)
	}
}
