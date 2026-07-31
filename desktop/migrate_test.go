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
	a := &App{cfg: config.Config{}, dbDir: t.TempDir()}
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

	a := &App{cfg: config.Config{}, dbDir: dir}
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

	a := &App{cfg: config.Config{}, dbDir: dir}
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
