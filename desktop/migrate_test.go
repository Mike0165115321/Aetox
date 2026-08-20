package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func schemaVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return v
}

// A fresh database must come up at the newest version, not at 0 with the
// tables present — otherwise the very first migration added later would run
// against a database that already has its result.
func TestFreshDatabaseLandsAtLatestSchemaVersion(t *testing.T) {
	a := seed(&App{cfg: config.Config{}, dbDir: t.TempDir()}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	if got, want := schemaVersion(t, db), latestSchemaVersion(); got != want {
		t.Fatalf("fresh database at version %d, want %d", got, want)
	}
}

// Every install in the wild is a database with the tables already present and
// user_version still 0 — versioning did not exist when they were written. Step
// 1 has to be a no-op over them that only moves the marker, and their history
// has to survive it.
func TestExistingUnversionedDatabaseMigratesWithoutLosingHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aetox.db")

	old, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := old.Exec(baselineSchema); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	if _, err := old.Exec(
		`INSERT INTO sessions(id, project_key, title, created_at, updated_at) VALUES(?,?,?,?,?)`,
		"20260101-000000.000", "proj", "งานเก่า", time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("legacy session: %v", err)
	}
	if _, err := old.Exec(
		`INSERT INTO messages(session_id, role, text, time) VALUES(?,?,?,?)`,
		"20260101-000000.000", "user", "ข้อความเก่า", time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("legacy message: %v", err)
	}
	if v := schemaVersion(t, old); v != 0 {
		t.Fatalf("legacy database should start at version 0, got %d", v)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	a := seed(&App{cfg: config.Config{}, dbDir: dir}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	if got, want := schemaVersion(t, db), latestSchemaVersion(); got != want {
		t.Fatalf("migrated database at version %d, want %d", got, want)
	}
	var text string
	if err := db.QueryRow(`SELECT text FROM messages WHERE session_id = ?`, "20260101-000000.000").Scan(&text); err != nil {
		t.Fatalf("read migrated message: %v", err)
	}
	if text != "ข้อความเก่า" {
		t.Fatalf("history changed by migration: got %q", text)
	}
	// The table the new version adds must exist afterwards.
	if _, err := db.Exec(`SELECT 1 FROM tool_runs LIMIT 1`); err != nil {
		t.Fatalf("tool_runs missing after migration: %v", err)
	}
	// v8: a session from before modes existed must reopen at the full desk —
	// mode '' — not at a guessed one. No backfill can know which mode an old
	// conversation was, so the honest answer is the one that changes nothing.
	var mode string
	if err := db.QueryRow(`SELECT mode FROM sessions WHERE id = ?`, "20260101-000000.000").Scan(&mode); err != nil {
		t.Fatalf("sessions.mode missing after migration: %v", err)
	}
	if mode != "" {
		t.Fatalf("a pre-mode session was assigned mode %q; '' is the full desk", mode)
	}
	// v9: and no agent — every old conversation was held with the main
	// assistant, which is what '' says.
	var agent string
	if err := db.QueryRow(`SELECT agent FROM sessions WHERE id = ?`, "20260101-000000.000").Scan(&agent); err != nil {
		t.Fatalf("sessions.agent missing after migration: %v", err)
	}
	if agent != "" {
		t.Fatalf("a pre-§85 session was assigned agent %q; '' is the main assistant", agent)
	}
}

// A database written by a newer build must be refused rather than used: this
// build cannot know what a future step changed, and writing into it would
// corrupt the user's only copy. The app still has to start — database() is
// allowed to return an error, and every caller treats that as "no history".
func TestDatabaseFromNewerBuildIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aetox.db")

	future, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := future.Exec(baselineSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := future.Exec("PRAGMA user_version = 9999"); err != nil {
		t.Fatalf("set future version: %v", err)
	}
	if err := future.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	a := seed(&App{cfg: config.Config{}, dbDir: dir}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	_, err = a.database()
	if err == nil {
		t.Fatal("a database from a newer build was accepted")
	}
	if !strings.Contains(err.Error(), "9999") {
		t.Fatalf("error should name the version found, got: %v", err)
	}

	// Refusing must not have rewritten anything.
	check, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = check.Close() }()
	if v := schemaVersion(t, check); v != 9999 {
		t.Fatalf("refused database was modified: version now %d", v)
	}
}

// v16: the summarizer's rows become system problems, and the decisions the
// user already made about them have to come along.
//
// The dedup key is (kind, scope, body), so a row left behind is not merely
// untidy — it stops matching, and every cluster the user already waved off is
// raised again the first time the pass runs on the new page. Sixteen decisions
// re-opened on day one, about outages from a week earlier.
//
// The body is rewritten in the same step because the sentence changed: it used
// to end with an instruction to the agent, which is what a lesson sounds like,
// and nothing on a problem card teaches anyone anything. This asserts the
// rewrite lands on exactly what issueBody produces today — the migration and
// the live code have to spell the identity the same way or the dedup misses by
// a space.
func TestSummarizerRowsBecomeIssuesAndKeepTheirDecisions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aetox.db")
	old, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := old.Exec(baselineSchema); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	// The queue arrived after the baseline, so a legacy database only has it if
	// its own step already ran — which is the state every install with rows in
	// it is actually in.
	if _, err := old.Exec(learningSchema); err != nil {
		t.Fatalf("legacy learning schema: %v", err)
	}
	const head = "ไม่พบโปรแกรม Tesseract ในเครื่อง"
	legacyBody := `เครื่องมือ image_ocr เคยล้มซ้ำ ๆ ด้วยเหตุเดียวกัน: "` + head +
		`" — เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่ครั้งแรก`
	now := time.Now().Format(time.RFC3339)
	if _, err := old.Exec(
		`INSERT INTO pending_changes(kind, scope, target, op, before, body, reason, evidence, source, state, created_at)
		 VALUES('memory','','C:\memory\MEMORY.md','add','',?,'เกิด 3 ครั้ง','tool_runs:1,2,3','summarizer','rejected',?)`,
		legacyBody, now); err != nil {
		t.Fatalf("legacy summarizer row: %v", err)
	}
	if _, err := old.Exec(
		`INSERT INTO pending_changes(kind, scope, target, op, before, body, reason, evidence, source, state, created_at)
		 VALUES('memory','','C:\memory\MEMORY.md','add','','ผู้ใช้เป็นผู้พัฒนาระบบ','ผู้ใช้บอกเอง','','agent','approved',?)`,
		now); err != nil {
		t.Fatalf("legacy agent row: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	a := seed(&App{cfg: config.Config{}, dbDir: dir}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}

	var kind, body, target, state string
	if err := db.QueryRow(
		`SELECT kind, body, target, state FROM pending_changes WHERE source = 'summarizer'`).
		Scan(&kind, &body, &target, &state); err != nil {
		t.Fatalf("read migrated row: %v", err)
	}
	if kind != kindIssue {
		t.Errorf("kind = %q, want %q — it would still be offered as something to remember", kind, kindIssue)
	}
	if want := issueBody("image_ocr", head); body != want {
		t.Errorf("body =\n%q\nwant\n%q\n— dedup matches on this exact string", body, want)
	}
	if target != "" {
		t.Errorf("target = %q, want empty: an issue lands in no memory file", target)
	}
	if state != "rejected" {
		t.Errorf("state = %q — the user already decided this one", state)
	}

	// The agent's own proposal is not the summarizer's and must not be moved.
	var agentKind, agentBody string
	if err := db.QueryRow(
		`SELECT kind, body FROM pending_changes WHERE source = 'agent'`).Scan(&agentKind, &agentBody); err != nil {
		t.Fatalf("read agent row: %v", err)
	}
	if agentKind != kindMemory || agentBody != "ผู้ใช้เป็นผู้พัฒนาระบบ" {
		t.Errorf("an agent proposal was rewritten: kind %q body %q", agentKind, agentBody)
	}
}

// Migrations are append-only and their versions must climb by one from 1: a
// duplicate or a gap makes "which steps has this database run" unanswerable.
func TestMigrationVersionsAreSequential(t *testing.T) {
	for i, m := range migrations {
		if m.version != i+1 {
			t.Fatalf("migration %d (%s) has version %d, want %d", i, m.name, m.version, i+1)
		}
		if strings.TrimSpace(m.name) == "" {
			t.Fatalf("migration %d has no name", m.version)
		}
	}
}
