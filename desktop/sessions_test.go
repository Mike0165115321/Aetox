package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
)

func TestProjectKeyStableAndDistinct(t *testing.T) {
	// Built with filepath.Join rather than written out: projectKey ends in
	// filepath.Base, and a hardcoded `C:\projects\app` is one undivided name
	// off Windows, so the "app-" prefix check passed for the wrong reason
	// there and failed outright once these tests ran on Linux.
	root := filepath.Join(string(filepath.Separator), "projects", "app")
	other := filepath.Join(string(filepath.Separator), "other", "app")

	k1 := projectKey(root)
	k2 := projectKey(root)
	if k1 != k2 {
		t.Errorf("projectKey not stable: %q != %q", k1, k2)
	}
	k3 := projectKey(other)
	if k1 == k3 {
		t.Errorf("projectKey(%q) == projectKey(%q) = %q, want distinct keys for two folders both named \"app\"", root, other, k1)
	}
	if !strings.HasPrefix(k1, "app-") {
		t.Errorf("projectKey = %q, want prefix %q", k1, "app-")
	}
}

// Unfocused chats are filed under a key derived from the unfocused root, and
// that root changed from <home> to <home>/aetox on 2026-07-26. Both keys have
// to keep resolving, or every chat held before that date is reported as a
// project whose folder was moved or deleted — with the transcripts still sitting
// in the database.
func TestIsUnfocusedKeyAcceptsBothTheCurrentAndTheLegacyBucket(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory on this machine")
	}
	if !isUnfocusedKey(projectKey(unfocusedRoot())) {
		t.Error("the current unfocused bucket was not recognised")
	}
	if !isUnfocusedKey(projectKey(home)) {
		t.Error("the pre-2026-07-26 home-dir bucket was not recognised — old chats would read as a missing project")
	}
	if isUnfocusedKey(projectKey(filepath.Join(home, "some-project"))) {
		t.Error("a real project was mistaken for the unfocused bucket")
	}
	if isUnfocusedKey("") {
		t.Error("an empty key must not match any bucket")
	}
}

func TestSessionTitleFrom(t *testing.T) {
	if got := sessionTitleFrom("   "); got != "(ว่าง)" {
		t.Errorf("sessionTitleFrom(blank) = %q, want %q", got, "(ว่าง)")
	}
	if got := sessionTitleFrom("hello"); got != "hello" {
		t.Errorf("sessionTitleFrom(short) = %q, want %q", got, "hello")
	}
	long := strings.Repeat("a", 50)
	got := sessionTitleFrom(long)
	wantRunes := []rune(long)[:40]
	if got != string(wantRunes)+"…" {
		t.Errorf("sessionTitleFrom(long) = %q, want 40 runes + ellipsis", got)
	}
}

func TestSessionTitleFromThai(t *testing.T) {
	// Thai text is multi-byte per rune; truncation must count runes, not bytes.
	thai := strings.Repeat("ก", 50)
	got := sessionTitleFrom(thai)
	if r := []rune(got); len(r) != 41 || r[40] != '…' { // 40 chars + ellipsis
		t.Errorf("sessionTitleFrom(thai 50 runes) = %q (%d runes), want 40 runes + ellipsis", got, len(r))
	}
}

func TestNewSessionIDFormat(t *testing.T) {
	id := newSessionID()
	if len(id) != len("20060102-150405.000") {
		t.Errorf("newSessionID() = %q, unexpected length %d", id, len(id))
	}
}

func TestTranscriptToModelMessages(t *testing.T) {
	in := []SessionMessage{
		{Role: "user", Text: "hi"},
		{Role: "agent", Text: "hello"},
	}
	out := transcriptToModelMessages(in)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].Role != model.RoleUser || out[0].Content != "hi" {
		t.Errorf("out[0] = %+v, want Role=user Content=hi", out[0])
	}
	if out[1].Role != model.RoleAssistant || out[1].Content != "hello" {
		t.Errorf("out[1] = %+v, want Role=assistant Content=hello", out[1])
	}
}

// closeDBOnCleanup registers a.database()'s *sql.DB to close when the test
// ends. Without this, t.TempDir()'s own cleanup fails on Windows because the
// sqlite file is still open (file cannot be removed while locked).
func closeDBOnCleanup(t *testing.T, a *App) {
	t.Helper()
	t.Cleanup(func() {
		if db, err := a.database(); err == nil {
			_ = db.Close()
		}
	})
}

func newTestApp(t *testing.T, sandboxRoot string) *App {
	t.Helper()
	a := &App{
		dbDir: t.TempDir(),
		cfg:   config.Config{SandboxRoot: sandboxRoot},
	}
	closeDBOnCleanup(t, a)
	a.startNewSession()
	return a
}

func TestAppendAndListSessions(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "hello world", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "hi there", Time: "10:00"},
	)

	sessions := a.ListSessions()
	if len(sessions) != 1 {
		t.Fatalf("ListSessions() = %d entries, want 1", len(sessions))
	}
	if sessions[0].ID != a.sessionID {
		t.Errorf("ListSessions()[0].ID = %q, want current session %q", sessions[0].ID, a.sessionID)
	}
	if sessions[0].Title != "hello world" {
		t.Errorf("ListSessions()[0].Title = %q, want %q", sessions[0].Title, "hello world")
	}
}

func TestDeleteSessionRemovesRowAndResetsCurrent(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "delete me", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "ok", Time: "10:00"},
	)
	deleted := a.sessionID

	if err := a.DeleteSession(deleted); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if len(a.ListSessions()) != 0 {
		t.Fatalf("session still listed after delete")
	}
	if msgs, _ := a.LoadSessionAnyProject(deleted); len(msgs) != 0 {
		t.Fatalf("messages still present after delete")
	}
	if a.sessionID == deleted {
		t.Fatalf("current session id not reset after deleting the open session")
	}
}

func TestListSessionsIsolatedByProject(t *testing.T) {
	dbDir := t.TempDir()
	rootA := t.TempDir()
	rootB := t.TempDir()

	a := &App{dbDir: dbDir, cfg: config.Config{SandboxRoot: rootA}}
	closeDBOnCleanup(t, a)
	a.startNewSession()
	a.appendTurn(SessionMessage{Role: "user", Text: "project A message"}, SessionMessage{Role: "agent", Text: "ok"})

	b := &App{dbDir: dbDir, cfg: config.Config{SandboxRoot: rootB}}
	closeDBOnCleanup(t, b)
	b.startNewSession()
	b.appendTurn(SessionMessage{Role: "user", Text: "project B message"}, SessionMessage{Role: "agent", Text: "ok"})

	sessionsA := a.ListSessions()
	if len(sessionsA) != 1 || sessionsA[0].Title != "project A message" {
		t.Errorf("ListSessions() for project A = %+v, want only project A's session", sessionsA)
	}
	sessionsB := b.ListSessions()
	if len(sessionsB) != 1 || sessionsB[0].Title != "project B message" {
		t.Errorf("ListSessions() for project B = %+v, want only project B's session", sessionsB)
	}
}

func TestSearchSessionsFindsMatch(t *testing.T) {
	root := t.TempDir()
	a := newTestApp(t, root)
	a.appendTurn(
		SessionMessage{Role: "user", Text: "unicorn migration plan", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "sure, let's plan it", Time: "10:00"},
	)

	found := a.SearchSessions("unicorn")
	if len(found) != 1 {
		t.Fatalf("SearchSessions(unicorn) = %d results, want 1", len(found))
	}

	notFound := a.SearchSessions("nonexistent-term-xyz")
	if len(notFound) != 0 {
		t.Errorf("SearchSessions(nonexistent) = %d results, want 0", len(notFound))
	}
}

func TestSearchSessionsQuotesUserInput(t *testing.T) {
	// A raw double-quote or FTS operator in the query must not break MATCH
	// (the implementation quotes the query before sending it to FTS5).
	a := newTestApp(t, t.TempDir())
	a.appendTurn(SessionMessage{Role: "user", Text: "hello"}, SessionMessage{Role: "agent", Text: "hi"})
	// Must simply not error/crash — a malformed MATCH expression would make
	// db.Query return an error, which SearchSessions swallows into `out`.
	_ = a.SearchSessions(`weird "quote` + " AND OR")
}

func TestLoadSessionRoundTrip(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	want := SessionMessage{Role: "user", Text: "remember this", Time: "10:00"}
	a.appendTurn(want, SessionMessage{Role: "agent", Text: "ok", Time: "10:00"})
	id := a.sessionID

	// Simulate switching away, then loading the session back.
	a.startNewSession()
	messages, err := a.LoadSession(id)
	if err != nil {
		t.Fatalf("LoadSession: unexpected error: %v", err)
	}
	if len(messages) != 2 || messages[0].Text != "remember this" {
		t.Errorf("LoadSession() = %+v, want the persisted turn back", messages)
	}
	if a.sessionID != id {
		t.Errorf("sessionID after LoadSession = %q, want %q", a.sessionID, id)
	}
}

func TestLoadSessionUnknownID(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if _, err := a.LoadSession("does-not-exist"); err == nil {
		t.Error("LoadSession(unknown id): expected error, got nil")
	}
}

// The reason the scoping is in SQL and not in the caller.
//
// LIMIT is applied after WHERE. Fetching a page across every door and filtering
// it in the frontend meant 200 rows were taken from the whole table and half
// thrown away — so a long enough run of coding sessions handed the storefront
// an empty list while its own history sat in the table, untouched and
// unreachable. Nothing errors; the column is just blank. It only appears once
// someone has used the app enough to have 200 sessions, which is the worst
// moment to find a bug of any kind.
//
// 250 workshop sessions, newer than every storefront one, is that state.
func TestADoorsHistoryIsNotEatenByTheOtherDoorsPage(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	key := projectKey(a.cfg.SandboxRoot)
	insert := func(id, mode, updated string) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO sessions(id, project_key, title, mode, created_at, updated_at) VALUES(?,?,?,?,?,?)`,
			id, key, "chat "+id, mode, updated, updated); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	// Oldest first, so every coding session sorts above them all.
	insert("shop-1", "assistant", "2026-01-01T00:00:01+07:00")
	insert("shop-2", "", "2026-01-01T00:00:02+07:00") // legacy: held at no desk
	for i := 0; i < 250; i++ {
		insert(fmt.Sprintf("code-%03d", i), "coding", fmt.Sprintf("2026-02-01T00:%02d:%02d+07:00", i/60, i%60))
	}

	storefront := a.ListSessionsForDoor(DeskFilter{Desks: []string{"coding"}, Exclude: true})
	var ids []string
	for _, s := range storefront {
		ids = append(ids, s.ID)
	}
	if !slices.Contains(ids, "shop-1") || !slices.Contains(ids, "shop-2") {
		t.Fatalf("the storefront's history was eaten by the workshop's page: got %v", ids)
	}
	// A session held at no desk is at home in the storefront — the regression
	// that filtering by desk name (rather than excluding the other door's)
	// caused once before.
	for _, s := range storefront {
		if s.Mode == "coding" {
			t.Errorf("a workshop session reached the storefront's list: %+v", s)
		}
	}

	workshop := a.ListSessionsForDoor(DeskFilter{Desks: []string{"coding"}})
	if len(workshop) != 200 {
		t.Errorf("workshop got %d sessions, want its own full page of 200", len(workshop))
	}
	for _, s := range workshop {
		if s.Mode != "coding" {
			t.Errorf("a storefront session reached the workshop's list: %+v", s)
		}
	}

	// An empty filter is every session, never none: a door that names no desks
	// is a bug upstream, and emptying the user's history to report it is the
	// wrong way round.
	if all := a.ListSessionsForDoor(DeskFilter{}); len(all) == 0 {
		t.Error("an empty filter returned nothing instead of everything")
	}
}
