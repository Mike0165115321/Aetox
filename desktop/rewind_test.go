package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// turn runs what a turn does to the project, as far as rewinding is concerned:
// a point is taken, then the file changes.
func runTurn(t *testing.T, app *App, label, name, body string) {
	t.Helper()
	app.captureSnapshot(app.cur(), label)
	root := app.snapshots.WorkTree()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileBody(t *testing.T, app *App, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(app.snapshots.WorkTree(), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// The headline: three turns, and the user can go back to the state before the
// first of them rather than only before the last.
func TestRewindReachesPastTheLastTurn(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runTurn(t, app, "first ask", "notes.md", "after one\n")
	runTurn(t, app, "second ask", "notes.md", "after two\n")
	runTurn(t, app, "third ask", "notes.md", "after three\n")

	points := app.RestorePoints()
	if len(points) != 3 {
		t.Fatalf("RestorePoints() = %d points, want 3: %+v", len(points), points)
	}
	// Newest first, so the list reads the way a person scrolls back.
	if points[0].Label != "third ask" || points[2].Label != "first ask" {
		t.Errorf("points are not newest-first: %+v", points)
	}

	result, err := app.RewindTo(points[2].ID)
	if err != nil {
		t.Fatalf("RewindTo: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0] != "notes.md" {
		t.Fatalf("Files = %v (%s), want notes.md", result.Files, result.Reason)
	}
	if got := fileBody(t, app, "notes.md"); got != "original\n" {
		t.Errorf("notes.md = %q, want the state before the first turn", got)
	}
}

// Every turn leaves a point, and the point remembers what was asked for. A list
// of times and tree hashes is a list nobody can pick from.
func TestRestorePointsCarryWhatWasAsked(t *testing.T) {
	app := undoApp(t)
	app.cur().cfg.SandboxRoot = app.snapshots.WorkTree()
	runTurn(t, app, "  แก้ bug   ที่ parser  ", "a.txt", "x")

	points := app.RestorePoints()
	if len(points) != 1 {
		t.Fatalf("want one point, got %+v", points)
	}
	if points[0].Label != "แก้ bug ที่ parser" {
		t.Errorf("Label = %q, want the message with its whitespace collapsed", points[0].Label)
	}
	if points[0].At == "" || points[0].ID == "" {
		t.Errorf("a point with no time or no tree is not one: %+v", points[0])
	}
}

// A long message is cut to something one row can hold, and cut on a rune
// boundary — half a Thai character is worse than one character less.
func TestRestoreLabelIsClampedOnARuneBoundary(t *testing.T) {
	long := strings.Repeat("ก", 200)
	got := clampRestoreLabel(long)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a long label was not marked as cut: %q", got)
	}
	if !strings.HasPrefix(got, "ก") || strings.Contains(got, "�") {
		t.Errorf("the cut landed inside a character: %q", got)
	}
}

// The state a rewind leaves behind goes on the list, which is the way back from
// a rewind somebody regrets.
func TestRewindLeavesAWayBack(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTurn(t, app, "one", "notes.md", "the work\n")

	before := app.RestorePoints()
	if _, err := app.RewindTo(before[0].ID); err != nil {
		t.Fatalf("RewindTo: %v", err)
	}
	after := app.RestorePoints()
	if len(after) <= len(before) {
		t.Fatalf("the state before the rewind was not kept: %+v", after)
	}
	if _, err := app.RewindTo(after[0].ID); err != nil {
		t.Fatalf("RewindTo the state we came from: %v", err)
	}
	if got := fileBody(t, app, "notes.md"); got != "the work\n" {
		t.Errorf("notes.md = %q, want the work back", got)
	}
}

// A file the person typed in survives a rewind that goes back past their
// typing, and does not survive one that stops short of it — the rule undo
// already had, now measured against the point actually being gone back to.
func TestRewindSparesWhatWasTypedAfterThePointItGoesTo(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	for _, name := range []string{"agent.md", "mine.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("original\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Turn one changes agent.md; during it, the person saves mine.md.
	app.captureSnapshot(app.cur(), "one")
	if err := os.WriteFile(filepath.Join(root, "agent.md"), []byte("agent wrote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.WriteFile("mine.md", "what I was writing\n"); err != nil {
		t.Fatal(err)
	}
	// Turn two changes agent.md again and nobody types.
	runTurn(t, app, "two", "agent.md", "agent wrote again\n")

	points := app.RestorePoints() // [two, one]
	result, err := app.RewindTo(points[1].ID)
	if err != nil {
		t.Fatalf("RewindTo: %v", err)
	}
	if got := fileBody(t, app, "mine.md"); got != "what I was writing\n" {
		t.Errorf("mine.md = %q — a rewind past the save must leave it alone", got)
	}
	if !contains(result.Kept, "mine.md") {
		t.Errorf("Kept = %v, want it to say which file was left alone", result.Kept)
	}
	if got := fileBody(t, app, "agent.md"); got != "original\n" {
		t.Errorf("agent.md = %q, want the state before turn one", got)
	}
}

// An id from a list that has moved on is told so, rather than doing nothing and
// looking like a control that does not work.
func TestRewindToAnUnknownPointSaysSo(t *testing.T) {
	app := undoApp(t)
	app.cur().cfg.SandboxRoot = app.snapshots.WorkTree()
	runTurn(t, app, "one", "a.txt", "x")

	result, err := app.RewindTo("0000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("RewindTo: %v", err)
	}
	if len(result.Files) != 0 {
		t.Errorf("an unknown point restored files: %v", result.Files)
	}
	if !strings.Contains(result.Reason, "no longer on this chat's list") {
		t.Errorf("Reason = %q, want it to say the point is gone", result.Reason)
	}
}

// PendingRestore is the same plan the rewind will carry out, so a row can say
// what it will do before the user commits to it.
func TestPendingRestoreMatchesWhatRewindWouldDo(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTurn(t, app, "one", "notes.md", "changed\n")

	points := app.RestorePoints()
	pending := app.PendingRestore(points[0].ID)
	if len(pending) != 1 || pending[0] != "notes.md" {
		t.Fatalf("PendingRestore = %v, want notes.md", pending)
	}
	result, err := app.RewindTo(points[0].ID)
	if err != nil {
		t.Fatalf("RewindTo: %v", err)
	}
	if len(result.Files) != len(pending) || result.Files[0] != pending[0] {
		t.Errorf("the offer was %v and the act was %v", pending, result.Files)
	}
	if got := app.PendingRestore("nope"); got == nil {
		t.Error("PendingRestore returned nil — §34, a nil slice crashes the frontend")
	}
}

// A turn that changed nothing does not add a row: two rows that do the same
// nothing are two rows nobody can choose between.
func TestATurnThatChangedNothingAddsNoPoint(t *testing.T) {
	app := undoApp(t)
	app.cur().cfg.SandboxRoot = app.snapshots.WorkTree()
	runTurn(t, app, "one", "a.txt", "x")
	app.captureSnapshot(app.cur(), "two") // nothing changed since
	app.captureSnapshot(app.cur(), "three")

	if got := len(app.RestorePoints()); got != 2 {
		t.Errorf("RestorePoints() = %d, want 2 — the identical trees should collapse: %+v",
			got, app.RestorePoints())
	}
}

// The list is capped, and trimming has to move the remembered saves with it or
// a save recorded against a point that is gone stops protecting its file.
func TestTheListIsCappedAndSavesMoveWithIt(t *testing.T) {
	app := undoApp(t)
	root := app.snapshots.WorkTree()
	app.cur().cfg.SandboxRoot = root
	if err := os.WriteFile(filepath.Join(root, "mine.md"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := range maxRestorePoints + 5 {
		runTurn(t, app, "turn", "a.txt", strings.Repeat("x", i+1))
	}
	if got := len(app.RestorePoints()); got != maxRestorePoints {
		t.Fatalf("RestorePoints() = %d, want the cap of %d", got, maxRestorePoints)
	}

	// A save now, then one more turn, then a rewind past the save.
	if err := app.WriteFile("mine.md", "what I was writing\n"); err != nil {
		t.Fatal(err)
	}
	runTurn(t, app, "after the save", "a.txt", "later")

	points := app.RestorePoints()
	if _, err := app.RewindTo(points[1].ID); err != nil {
		t.Fatalf("RewindTo: %v", err)
	}
	if got := fileBody(t, app, "mine.md"); got != "what I was writing\n" {
		t.Errorf("mine.md = %q — trimming the list lost the save's place in it", got)
	}
}
