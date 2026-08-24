package main

// ไฟล์ที่แก้, read back off tool_runs.
//
// The behaviours worth pinning are the ones that decide whether a person can
// trust the list: that a call which failed left nothing behind, that the last
// thing done to a file is what the row says, that a path the model typed
// resolves to where the file actually landed, and that a deleted file is still
// reported rather than quietly dropped.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/turn"
)

// recordFailedRun is recordRun's other half: a call the engine ran and the tool
// refused. It changed nothing, so nothing may appear.
func recordFailedRun(t *testing.T, a *App, tool, args string) {
	t.Helper()
	a.recordToolRun(a.cur(), turn.ToolRun{Name: tool, Args: args, OK: false, Error: "denied"})
}

func TestSessionEditsListsWhatTheRoomWrote(t *testing.T) {
	a := bootDeskApp(t, "coding")
	root := a.cur().cfg.SandboxRoot
	touch(t, filepath.Join(root, "internal"), "app.go")
	touch(t, root, "README.md")

	recordRun(t, a, "edit", `{"path":"internal/app.go","find":"a","replace":"b"}`)
	recordRun(t, a, "write", `{"path":"README.md","content":"x"}`)

	page := a.SessionEdits(a.cur().id)
	if page.Total != 2 || len(page.Files) != 2 {
		t.Fatalf("want 2 files, got total=%d files=%+v", page.Total, page.Files)
	}
	// Newest first, same as แหล่งที่มา: what you are looking for is nearly
	// always what was just done.
	if page.Files[0].Label != "README.md" || page.Files[0].Status != "W" {
		t.Errorf("first row wrong: %+v", page.Files[0])
	}
	if page.Files[1].Label != "app.go" || page.Files[1].Status != "M" {
		t.Errorf("second row wrong: %+v", page.Files[1])
	}
	for _, f := range page.Files {
		if f.Gone {
			t.Errorf("%s is on disk and must not read as gone", f.Label)
		}
	}
}

// The list is what the room DID. A tool that refused did nothing, and a row for
// it would be the one mistake this list cannot afford to make.
func TestSessionEditsIgnoresCallsThatFailed(t *testing.T) {
	a := bootDeskApp(t, "coding")
	touch(t, a.cur().cfg.SandboxRoot, "kept.md")

	recordFailedRun(t, a, "write", `{"path":"refused.md","content":"x"}`)
	recordRun(t, a, "write", `{"path":"kept.md","content":"x"}`)

	page := a.SessionEdits(a.cur().id)
	if len(page.Files) != 1 || page.Files[0].Label != "kept.md" {
		t.Fatalf("a refused write must leave nothing behind: %+v", page.Files)
	}
}

// Reading a file is the other list's business (sources.go). One row that cannot
// say whether the room trusted a file or changed it is worse than two lists.
func TestSessionEditsLeavesReadsToTheOtherList(t *testing.T) {
	a := bootDeskApp(t, "coding")
	notes := touch(t, t.TempDir(), "notes.md")

	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, notes))
	recordRun(t, a, "grep", `{"pattern":"TODO"}`)

	if page := a.SessionEdits(a.cur().id); len(page.Files) != 0 {
		t.Fatalf("reads are not edits: %+v", page.Files)
	}
}

// One call, several files, every one of them changed by it. Reading only a
// top-level "path" would drop the lot.
func TestSessionEditsCountsEveryPathInOnePatch(t *testing.T) {
	a := bootDeskApp(t, "coding")
	root := a.cur().cfg.SandboxRoot
	touch(t, root, "one.go")
	touch(t, root, "two.go")

	recordRun(t, a, "edits", `{"edits":[
		{"path":"one.go","find":"a","replace":"b"},
		{"path":"two.go","find":"c","replace":"d"}]}`)

	page := a.SessionEdits(a.cur().id)
	if len(page.Files) != 2 {
		t.Fatalf("want a row per file in the call, got %+v", page.Files)
	}
}

// A file written eleven times is one row, and the row says the LAST thing done
// to it: written and then deleted reads as deleted, which is the truth about it.
func TestSessionEditsKeepsOnlyTheLastThingDoneToAFile(t *testing.T) {
	a := bootDeskApp(t, "coding")

	recordRun(t, a, "write", `{"path":"draft.md","content":"x"}`)
	recordRun(t, a, "edit", `{"path":"draft.md","find":"x","replace":"y"}`)
	recordRun(t, a, "delete", `{"path":"draft.md"}`)

	page := a.SessionEdits(a.cur().id)
	if len(page.Files) != 1 {
		t.Fatalf("one file is one row, got %+v", page.Files)
	}
	if page.Files[0].Status != "D" {
		t.Errorf("the last action is what the row says, got %q", page.Files[0].Status)
	}
	// Reported, not dropped: deleting a file is the loudest thing this room can
	// do to one, and it is exactly what somebody opens the panel to confirm.
	if !page.Files[0].Gone {
		t.Error("a deleted file must not claim to still be there")
	}
}

// The model types relative paths, and a relative `write` in an unfocused chat
// lands in output/<session>. Resolving against this process's working directory
// instead answers about a folder nobody in the conversation ever mentioned —
// which is how every local file used to vanish off แหล่งที่มา as well.
func TestSessionEditsResolvesAPathToWhereTheFileLanded(t *testing.T) {
	a := bootDeskApp(t, "coding")
	id := a.cur().id
	touch(t, filepath.Join(a.cur().cfg.SandboxRoot, "output", id), "post.md")

	recordRun(t, a, "write", `{"path":"post.md","content":"x"}`)

	page := a.SessionEdits(id)
	if len(page.Files) != 1 {
		t.Fatalf("want one row, got %+v", page.Files)
	}
	if want := "output/" + id + "/post.md"; page.Files[0].Path != want {
		t.Errorf("path = %q, want %q", page.Files[0].Path, want)
	}
	if page.Files[0].Gone {
		t.Error("the file is on disk at the placed path")
	}
}

// Two rows both reading `code.html` is the failure a list of shortened names
// exists to prevent, and the whole group has to say where it is: one bare name
// beside one qualified one still leaves the reader working out which is which.
func TestSessionEditsNamesTheFolderWhenTwoRowsCollide(t *testing.T) {
	a := bootDeskApp(t, "coding")
	root := a.cur().cfg.SandboxRoot
	touch(t, filepath.Join(root, "site"), "code.html")
	touch(t, filepath.Join(root, "docs"), "code.html")
	touch(t, root, "alone.md")

	recordRun(t, a, "write", `{"path":"site/code.html","content":"x"}`)
	recordRun(t, a, "write", `{"path":"docs/code.html","content":"x"}`)
	recordRun(t, a, "write", `{"path":"alone.md","content":"x"}`)

	byPath := map[string]EditedFile{}
	for _, f := range a.SessionEdits(a.cur().id).Files {
		byPath[f.Path] = f
	}
	one, two := byPath["site/code.html"], byPath["docs/code.html"]
	if one.Dir == "" || two.Dir == "" {
		t.Fatalf("both colliding rows must say where they are: %+v %+v", one, two)
	}
	if one.Dir == two.Dir {
		t.Errorf("folders that do not distinguish them are no help: %q", one.Dir)
	}
	if byPath["alone.md"].Dir != "" {
		t.Errorf("uncontested name should carry no folder, got %q", byPath["alone.md"].Dir)
	}
}

// A file the agent wrote and the user then deleted is still what the agent did.
// The row stays and says so; the frontend is what refuses to make it a door.
func TestSessionEditsReportsAFileThatIsNoLongerThere(t *testing.T) {
	a := bootDeskApp(t, "coding")
	gone := filepath.Join(a.cur().cfg.SandboxRoot, "vanished.md")
	touch(t, a.cur().cfg.SandboxRoot, "vanished.md")

	recordRun(t, a, "write", `{"path":"vanished.md","content":"x"}`)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	page := a.SessionEdits(a.cur().id)
	if len(page.Files) != 1 || !page.Files[0].Gone {
		t.Fatalf("want one row that admits the file is gone, got %+v", page.Files)
	}
}

// An id nobody has, and a call with arguments nobody can parse: both are
// "nothing to report", and neither may be a nil slice — §34.
func TestSessionEditsAnswersEmptyRatherThanNil(t *testing.T) {
	a := bootDeskApp(t, "coding")
	recordRun(t, a, "write", `{not json`)

	for _, id := range []string{"", "no-such-session", a.cur().id} {
		page := a.SessionEdits(id)
		if page.Files == nil {
			t.Errorf("SessionEdits(%q) returned a nil slice — marshals to JSON null", id)
		}
		if len(page.Files) != 0 || page.Total != 0 {
			t.Errorf("SessionEdits(%q) = %+v, want nothing", id, page)
		}
	}
}

// changedPaths reads the paths out of whatever the app announced. captureEvents
// and `emitted` are workbench_desk_test.go's — one recorder for the package, or
// two tests would be asserting against two different ideas of an event.
func changedPaths(t *testing.T, events []emitted) []string {
	t.Helper()
	out := []string{}
	for _, ev := range events {
		if ev.Name != "workbench:files-changed" || len(ev.Data) == 0 {
			continue
		}
		payload, ok := ev.Data[0].(sessionEvent[[]string])
		if !ok {
			t.Fatalf("workbench:files-changed carried %T, not a session event", ev.Data[0])
		}
		out = append(out, payload.Data...)
	}
	return out
}

// A pane showing a file the agent just rewrote has to hear about it. Nothing
// told it before: the tab read the file when it was opened and never again, so
// the user watched an old version of a document being described as new work.
func TestNotifyFilesChangedNamesWhatAWriteTouched(t *testing.T) {
	a := bootDeskApp(t, "coding")
	events := captureEvents(a)
	touch(t, a.cur().cfg.SandboxRoot, "post.md")

	a.notifyFilesChanged(a.cur(), turn.ToolRun{Name: "write", Args: `{"path":"post.md","content":"x"}`, OK: true})

	if got := changedPaths(t, *events); len(got) != 1 || got[0] != "post.md" {
		t.Fatalf("want the written path announced, got %+v", got)
	}
}

// The same parse the panel uses, so the two cannot disagree about what a call
// changed — several files in one atomic call included.
func TestNotifyFilesChangedCoversEveryPathInAPatch(t *testing.T) {
	a := bootDeskApp(t, "coding")
	events := captureEvents(a)
	root := a.cur().cfg.SandboxRoot
	touch(t, root, "one.go")
	touch(t, root, "two.go")

	a.notifyFilesChanged(a.cur(), turn.ToolRun{Name: "edits", OK: true, Args: `{"edits":[
		{"path":"one.go","find":"a","replace":"b"},
		{"path":"two.go","find":"c","replace":"d"}]}`})

	if got := changedPaths(t, *events); len(got) != 2 {
		t.Fatalf("want both files announced, got %+v", got)
	}
}

// A call that read, and a call that was refused. Neither changed a byte, and a
// pane that re-reads on either is a pane that flickers for no reason — worse,
// one that says "this file just changed" when it did not.
func TestNotifyFilesChangedStaysQuietWhenNothingChanged(t *testing.T) {
	a := bootDeskApp(t, "coding")
	events := captureEvents(a)
	notes := touch(t, a.cur().cfg.SandboxRoot, "notes.md")

	a.notifyFilesChanged(a.cur(), turn.ToolRun{Name: "read", Args: fmt.Sprintf(`{"path":%q}`, notes), OK: true})
	a.notifyFilesChanged(a.cur(), turn.ToolRun{Name: "write", Args: `{"path":"refused.md","content":"x"}`, OK: false})

	if got := changedPaths(t, *events); len(got) != 0 {
		t.Fatalf("want silence, got %+v", got)
	}
}

// The path announced is the path on disk, not the one the model typed — the
// window matches it against an open tab, and "post.md" matches nothing when the
// tab was opened at output/<session>/post.md.
func TestNotifyFilesChangedAnnouncesThePlacedPath(t *testing.T) {
	a := bootDeskApp(t, "coding")
	events := captureEvents(a)
	id := a.cur().id
	touch(t, filepath.Join(a.cur().cfg.SandboxRoot, "output", id), "post.md")

	a.notifyFilesChanged(a.cur(), turn.ToolRun{Name: "write", Args: `{"path":"post.md","content":"x"}`, OK: true})

	got := changedPaths(t, *events)
	if want := "output/" + id + "/post.md"; len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v, want [%s]", got, want)
	}
}
