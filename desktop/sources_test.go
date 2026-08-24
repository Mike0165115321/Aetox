package main

// แหล่งที่มา, read back off tool_runs.
//
// The behaviours worth pinning are the ones that make a list of names usable
// rather than merely present: that it names things the room *read* and not
// things it wrote, that no two rows can read the same, and that it never offers
// a row that opens nothing.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/turn"
)

// recordRun writes one tool call into the store the way a real turn does.
func recordRun(t *testing.T, a *App, tool, args string) {
	t.Helper()
	a.recordToolRun(a.cur(), turn.ToolRun{Name: tool, Args: args, OK: true})
}

// touch makes a real file, because a source that is not on disk is dropped —
// which is the point of half these tests and a trap for the other half.
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSessionSourcesListsWhatTheRoomRead(t *testing.T) {
	a := bootDeskApp(t, "coding")
	dir := t.TempDir()
	notes := touch(t, dir, "notes.md")

	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, notes))
	recordRun(t, a, "web_fetch", `{"url":"https://example.invalid/docs/api"}`)

	got := a.SessionSources(a.cur().id)
	if len(got) != 2 {
		t.Fatalf("want 2 sources, got %d: %+v", len(got), got)
	}
	// Newest first: what you are looking for is nearly always what you just did.
	if got[0].Kind != "url" || got[0].Label != "example.invalid/docs/api" {
		t.Errorf("first row wrong: %+v", got[0])
	}
	if got[1].Kind != "file" || got[1].Label != "notes.md" || got[1].Path != notes {
		t.Errorf("second row wrong: %+v", got[1])
	}
}

// A file this conversation produced is not a source for it, and the ผลงาน page
// already answers "what did it make". Two places answering one question is the
// debt this whole file was written to avoid.
func TestSessionSourcesIgnoresWritesAndSearches(t *testing.T) {
	a := bootDeskApp(t, "coding")
	dir := t.TempDir()
	made := touch(t, dir, "report.docx")

	recordRun(t, a, "write", fmt.Sprintf(`{"path":%q}`, made))
	recordRun(t, a, "edit", fmt.Sprintf(`{"path":%q}`, made))
	recordRun(t, a, "doc_write", fmt.Sprintf(`{"path":%q}`, made))
	recordRun(t, a, "glob", `{"pattern":"**/*.ts"}`)
	recordRun(t, a, "grep", `{"pattern":"func main"}`)

	if got := a.SessionSources(a.cur().id); len(got) != 0 {
		t.Fatalf("want nothing, got %+v", got)
	}
}

// The failure this list exists to prevent: two rows a person cannot tell apart.
// Both members of the group get the folder, not just the later one — a bare
// `code.html` sitting next to `src/code.html` still leaves the reader guessing
// which one the bare name is.
func TestSessionSourcesSeparatesFilesThatShareAName(t *testing.T) {
	a := bootDeskApp(t, "coding")
	root := t.TempDir()
	one := touch(t, filepath.Join(root, "site"), "code.html")
	two := touch(t, filepath.Join(root, "docs"), "code.html")
	alone := touch(t, root, "DESIGN.md")

	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, one))
	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, two))
	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, alone))

	byPath := map[string]Source{}
	for _, s := range a.SessionSources(a.cur().id) {
		byPath[s.Path] = s
	}
	if byPath[one].Dir == "" || byPath[two].Dir == "" {
		t.Errorf("both colliding rows need a folder: %+v / %+v", byPath[one], byPath[two])
	}
	if byPath[one].Dir == byPath[two].Dir {
		t.Errorf("folders that do not distinguish them are no help: %q", byPath[one].Dir)
	}
	// A name nothing collides with stays clean — the folder is a fix for a
	// problem, not decoration on every row.
	if byPath[alone].Dir != "" {
		t.Errorf("uncontested name should carry no folder, got %q", byPath[alone].Dir)
	}
}

// A URL's identity is at both ends. Cutting the tail leaves two deployments
// reading identically, which is the same failure as two `code.html` rows.
func TestSessionSourcesKeepsTheTailOfAURL(t *testing.T) {
	a := bootDeskApp(t, "coding")
	recordRun(t, a, "browser_open", `{"action":"open","url":"https://alm-x-impact-tennis-production.up.railway.app/health"}`)

	got := a.SessionSources(a.cur().id)
	if len(got) != 1 {
		t.Fatalf("want 1, got %+v", got)
	}
	if got[0].Label != "alm-x-impact-tennis-production.up.railway.app/health" {
		t.Errorf("label lost the part that identifies it: %q", got[0].Label)
	}
}

// Only `open` names something read. A click's coordinates arriving as a source
// would be nonsense the panel then invites you to open.
func TestSessionSourcesIgnoresBrowserActionsThatReadNothing(t *testing.T) {
	a := bootDeskApp(t, "coding")
	recordRun(t, a, "browser", `{"action":"click","url":"https://example.invalid/x"}`)

	if got := a.SessionSources(a.cur().id); len(got) != 0 {
		t.Fatalf("want nothing, got %+v", got)
	}
}

// The user moved it or deleted it. A row that opens nothing is worse than no
// row, and it is exactly the lie an index table would have told.
func TestSessionSourcesDropsFilesThatAreGone(t *testing.T) {
	a := bootDeskApp(t, "coding")
	dir := t.TempDir()
	gone := touch(t, dir, "temp.md")
	kept := touch(t, dir, "kept.md")

	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, gone))
	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, kept))
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	got := a.SessionSources(a.cur().id)
	if len(got) != 1 || got[0].Path != kept {
		t.Fatalf("want only the file that is still there, got %+v", got)
	}
}

// Read eleven times is one row, and its time answers "when did this room last
// touch it" rather than "when did it first".
func TestSessionSourcesCollapsesRepeatedReads(t *testing.T) {
	a := bootDeskApp(t, "coding")
	dir := t.TempDir()
	path := touch(t, dir, "notes.md")

	for i := 0; i < 11; i++ {
		recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, path))
	}

	if got := a.SessionSources(a.cur().id); len(got) != 1 {
		t.Fatalf("want 1 row, got %d", len(got))
	}
}

// A truncated list that will not say how truncated reads as complete.
func TestSessionSourceCountSeesPastTheCap(t *testing.T) {
	a := bootDeskApp(t, "coding")
	dir := t.TempDir()
	for i := 0; i < maxSources+7; i++ {
		path := touch(t, dir, fmt.Sprintf("f%02d.md", i))
		recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, path))
	}

	if got := len(a.SessionSources(a.cur().id)); got != maxSources {
		t.Errorf("list should stop at the cap, got %d", got)
	}
	if got := a.SessionSourceCount(a.cur().id); got != maxSources+7 {
		t.Errorf("count should see everything, got %d", got)
	}
}

// Args is the model's raw JSON, unparsed by design. A call nobody can read the
// arguments of read nothing anybody can name.
func TestSessionSourcesSkipsUnreadableArguments(t *testing.T) {
	a := bootDeskApp(t, "coding")
	recordRun(t, a, "read", `{"path": broken`)
	recordRun(t, a, "read", `{}`)
	recordRun(t, a, "read", `{"path":""}`)

	if got := a.SessionSources(a.cur().id); len(got) != 0 {
		t.Fatalf("want nothing, got %+v", got)
	}
}

// Another room's reads are not this room's sources.
func TestSessionSourcesStayInTheirOwnRoom(t *testing.T) {
	a := bootDeskApp(t, "coding")
	dir := t.TempDir()
	mine := touch(t, dir, "mine.md")
	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, mine))

	if got := a.SessionSources("some-other-session"); len(got) != 0 {
		t.Fatalf("want nothing for a room that read nothing, got %+v", got)
	}
	if got := a.SessionSources(""); len(got) != 0 {
		t.Fatalf("an empty session id is not a wildcard, got %+v", got)
	}
}

// The bug this list shipped with, and the reason it went unnoticed: every test
// above hands `read` an ABSOLUTE path, and absolute paths were the only ones
// that survived.
//
// The model types relative paths. `os.Stat` on one resolves against this
// process's working directory — wherever the app happened to be launched from —
// so a file the room really did read was dropped as missing, and แหล่งที่มา
// came back holding nothing but web pages for a session full of reading.
func TestSessionSourcesResolvesARelativePathAgainstTheRoot(t *testing.T) {
	a := bootDeskApp(t, "coding")
	root := a.cur().cfg.SandboxRoot
	touch(t, filepath.Join(root, "internal"), "app.go")

	recordRun(t, a, "read", `{"path":"internal/app.go"}`)

	got := a.SessionSources(a.cur().id)
	if len(got) != 1 {
		t.Fatalf("a file the room read must not vanish because the path was relative: %+v", got)
	}
	if got[0].Path != "internal/app.go" || got[0].Label != "app.go" {
		t.Errorf("row wrong: %+v", got[0])
	}
	// The count is built the same way, or it would promise rows the list cannot
	// show — and the two would disagree about which files are still there.
	if n := a.SessionSourceCount(a.cur().id); n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

// The second half of the same rule. A relative `write` in an unfocused chat
// lands in output/<session>, so the name in the record is not the name on disk —
// and the model reads that file back by the name it first typed.
func TestSessionSourcesFollowsAReadIntoTheOutputFolder(t *testing.T) {
	a := bootDeskApp(t, "coding")
	id := a.cur().id
	touch(t, filepath.Join(a.cur().cfg.SandboxRoot, "output", id), "post.md")

	recordRun(t, a, "read", `{"path":"post.md"}`)

	got := a.SessionSources(id)
	if len(got) != 1 {
		t.Fatalf("want the placed file, got %+v", got)
	}
	if want := "output/" + id + "/post.md"; got[0].Path != want {
		t.Errorf("path = %q, want %q — the row has to open what was actually read", got[0].Path, want)
	}
}

// The other direction, and the check that the resolution above did not quietly
// narrow the list: a file outside the project root keeps its row.
//
// It is reachable — the sandbox reopened the machine on purpose (§19.1 reversed
// 2026-08-04), and ReadFile resolves through the very same gate — so the row
// opens what it names. A path that gate really does refuse (a credential store)
// is dropped by the same call, and refused in skill's own tests rather than
// re-asserted here.
func TestSessionSourcesKeepsAFileOutsideTheProjectRoot(t *testing.T) {
	a := bootDeskApp(t, "coding")
	a.projectFocused = true
	outside := touch(t, t.TempDir(), "elsewhere.md")

	recordRun(t, a, "read", fmt.Sprintf(`{"path":%q}`, outside))

	got := a.SessionSources(a.cur().id)
	if len(got) != 1 || got[0].Path != outside {
		t.Fatalf("a file this session can open must keep its row, got %+v", got)
	}
}
