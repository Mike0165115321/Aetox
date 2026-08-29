package main

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/turn"
)

// The point of session_search in one test: what was said and what was done in
// an earlier session are both findable by content, and each hit says where
// and when it happened.
func TestSessionSearchFindsChatAndWork(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "ช่วยจัดไฟล์สลิปโอนเงินหน่อย", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "จัดสลิปเสร็จแล้ว อยู่ในโฟลเดอร์ receipts", Time: "10:01"},
	)
	a.recordToolRun(a.cur(), turn.ToolRun{
		Name:     "image_ocr",
		Args:     `{"path":"สลิปโอนเงิน-0731.png"}`,
		Output:   "ยอดโอน 1,500 บาท",
		OK:       true,
		Duration: 120 * time.Millisecond,
	})

	search := &sessionSearchSkill{app: a}
	out, err := search.ExecuteTool(context.Background(), map[string]any{"query": "สลิปโอนเงิน"})
	if err != nil {
		t.Fatalf("session_search: %v", err)
	}
	if !strings.Contains(out.Content, "Chat history") || !strings.Contains(out.Content, "จัดไฟล์สลิป") {
		t.Fatalf("chat hit missing: %q", out.Content)
	}
	if !strings.Contains(out.Content, "Tool work") || !strings.Contains(out.Content, "image_ocr") {
		t.Fatalf("work hit missing: %q", out.Content)
	}
	// Both hits happened in the open session and must say so — the model
	// otherwise treats them as an earlier session's work and re-reports it
	// to the user as history.
	if !strings.Contains(out.Content, "(this session)") {
		t.Fatalf("current-session hits not marked: %q", out.Content)
	}
}

func TestSessionSearchNoMatchesSaysSoPlainly(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	search := &sessionSearchSkill{app: a}
	out, err := search.ExecuteTool(context.Background(), map[string]any{"query": "ไม่มีทางเจอ"})
	if err != nil {
		t.Fatalf("no matches must not be an error — it is an answer: %v", err)
	}
	if !strings.Contains(out.Content, "No history matches") {
		t.Fatalf("want a plain no-match answer, got %q", out.Content)
	}
}

// Trigram FTS physically cannot match under 3 characters — the tool must say
// so instead of returning an empty result the model reads as "not there".
func TestSessionSearchRejectsTooShortQuery(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	search := &sessionSearchSkill{app: a}
	if _, err := search.ExecuteTool(context.Background(), map[string]any{"query": "ab"}); err == nil {
		t.Fatalf("2-char query must error, not silently match nothing")
	}
}

// A database that lived at schema version 2 already holds tool_runs rows the
// v3 triggers never saw. The migration backfills them; without that, history
// recorded before the upgrade would be the one permanently unsearchable part.
func TestMigrationBackfillsToolRunsFTS(t *testing.T) {
	dir := t.TempDir()
	dsn := "file:" + filepath.ToSlash(filepath.Join(dir, "aetox.db")) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	// Build a version-2 database by hand: schema steps 1 and 2, one row, no FTS.
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(baselineSchema); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := db.Exec(toolRunsSchema); err != nil {
		t.Fatalf("tool_runs: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO tool_runs(session_id, tool, args, output, ok, time)
		 VALUES('old-session', 'web_fetch', 'ราคาทองวันนี้', 'บาทละ 45,000', 1, '2026-07-01T10:00:00Z')`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Opening through the app migrates to v3, which must index the old row.
	a := seed(&App{dbDir: dir}, newConversation())
	closeDBOnCleanup(t, a)
	migrated, err := a.database()
	if err != nil {
		t.Fatalf("migrate to v3: %v", err)
	}
	var hits int
	if err := migrated.QueryRow(
		`SELECT COUNT(*) FROM tool_runs_fts WHERE tool_runs_fts MATCH '"ราคาทอง"'`,
	).Scan(&hits); err != nil {
		t.Fatalf("fts query: %v", err)
	}
	if hits != 1 {
		t.Fatalf("pre-upgrade tool run not searchable after migration: %d hits", hits)
	}
}

// Deleting a session must delete its work from the search index too — the
// history panel's delete promises the data is gone, and a searchable ghost
// row would make that promise false.
func TestDeleteSessionRemovesToolRunsFromIndex(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "seed", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "ok", Time: "10:00"},
	)
	a.recordToolRun(a.cur(), turn.ToolRun{Name: "grep", Args: "needle-xyzzy", OK: true})

	db, err := a.database()
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	var before int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tool_runs_fts WHERE tool_runs_fts MATCH '"needle-xyzzy"'`).Scan(&before); err != nil || before != 1 {
		t.Fatalf("precondition: run not indexed (%d, %v)", before, err)
	}

	if err := a.DeleteSession(a.cur().id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	var after int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tool_runs_fts WHERE tool_runs_fts MATCH '"needle-xyzzy"'`).Scan(&after); err != nil {
		t.Fatalf("fts query after delete: %v", err)
	}
	if after != 0 {
		t.Fatalf("deleted session's work still searchable: %d hits", after)
	}
}

// clampHit flattens whitespace and cuts on a rune boundary — half a Thai
// character stored as invalid UTF-8 is worse than one character less.
func TestClampHitRuneSafety(t *testing.T) {
	long := strings.Repeat("ก", 200)
	got := clampHit(long)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("long hit not truncated: %d chars", len(got))
	}
	if !strings.Contains(fmt.Sprintf("%q", got), "ก") || strings.Contains(got, "�") {
		t.Fatalf("truncation broke UTF-8: %q", got)
	}
}

// A chat hit must carry the DAY, not the clock. messages.time is "15:04" (the
// chat UI's format) while tool_runs.time is RFC3339, and running datePart over
// both printed "[01:41]" where a date belonged — on the one tool whose whole
// job is answering "เหมือนคราวที่แล้ว".
func TestChatStampCarriesTheDay(t *testing.T) {
	cases := []struct {
		name    string
		opened  string
		message string
		want    string
	}{
		{"clock plus session day", "2026-08-29T01:41:22+07:00", "01:41", "2026-08-29 01:41"},
		{"full stamp is left alone", "2026-08-29T01:41:22+07:00", "2026-08-28T23:10:00+07:00", "2026-08-28"},
		{"no session date falls back to the clock", "", "01:41", "01:41"},
		{"no message time is still a day", "2026-08-29T01:41:22+07:00", "", "2026-08-29"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chatStamp(c.opened, c.message); got != c.want {
				t.Errorf("chatStamp(%q, %q) = %q, want %q", c.opened, c.message, got, c.want)
			}
		})
	}
}
