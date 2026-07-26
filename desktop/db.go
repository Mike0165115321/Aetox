package main

// Local store: one SQLite file (<UserConfigDir>/aetox/aetox.db) holds every
// project's chat history — nothing ever leaves the machine. FTS5 with the
// trigram tokenizer gives substring full-text search that works for Thai
// (no word boundaries needed) as well as English. Driver is modernc.org/sqlite
// (pure Go, no CGO), which bundles FTS5.
//
// Schema grows here: future tables (agent memories with embedding BLOBs, tool
// runs, …) belong in this same file.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mike0165115321/Aetox/internal/config"
	_ "modernc.org/sqlite"
)

const dbSchema = `
CREATE TABLE IF NOT EXISTS sessions (
  id          TEXT PRIMARY KEY,
  project_key TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_key, updated_at DESC);

CREATE TABLE IF NOT EXISTS projects (
  project_key TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  root_path   TEXT NOT NULL,
  opened_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_projects_opened ON projects(opened_at DESC);

CREATE TABLE IF NOT EXISTS messages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  role       TEXT NOT NULL,
  text       TEXT NOT NULL,
  time       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, id);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
  text, content='messages', content_rowid='id', tokenize='trigram'
);
CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.id, old.text);
END;

CREATE TABLE IF NOT EXISTS token_usage (
  id                   INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id           TEXT NOT NULL DEFAULT '',
  model                TEXT NOT NULL,
  prompt_tokens        INTEGER NOT NULL,
  completion_tokens    INTEGER NOT NULL,
  cached_prompt_tokens INTEGER,
  time                 TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_token_usage_time ON token_usage(time);
`

// addedColumns are columns introduced after a table shipped. The schema above
// is all CREATE TABLE IF NOT EXISTS, which is a no-op on a database that
// already has the table — so a new column reaches existing installs only from
// here. Without this the INSERT would fail on every turn, and since usage
// writes only log their errors, the stats page would quietly stop growing.
//
// cached_prompt_tokens is nullable on purpose: NULL means "this provider does
// no cache accounting" (Ollama, LM Studio, and every row written before the
// column existed), which is not the same as a measured zero hits. SUM skips
// NULLs and COUNT counts only non-NULLs, so both questions stay answerable.
var addedColumns = []struct{ table, column, definition string }{
	{"token_usage", "cached_prompt_tokens", "INTEGER"},
}

// applyAddedColumns adds any missing column in addedColumns. Safe to run on
// every open: existing columns are detected first, so nothing is attempted
// twice and no data is touched.
func applyAddedColumns(db *sql.DB) error {
	for _, c := range addedColumns {
		has, err := hasColumn(db, c.table, c.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := db.Exec("ALTER TABLE " + c.table + " ADD COLUMN " + c.column + " " + c.definition); err != nil {
			return fmt.Errorf("add %s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("SELECT 1 FROM pragma_table_info(?) WHERE name = ?", table, column)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

// database opens (once) the app-wide SQLite store.
func (a *App) database() (*sql.DB, error) {
	a.dbInit.Do(func() {
		dir := a.dbDir
		if dir == "" {
			var err error
			dir, err = config.DataRoot()
			if err != nil {
				a.dbErr = err
				return
			}
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			a.dbErr = err
			return
		}
		dsn := "file:" + filepath.ToSlash(filepath.Join(dir, "aetox.db")) +
			"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			a.dbErr = err
			return
		}
		if _, err := db.Exec(dbSchema); err != nil {
			a.dbErr = err
			_ = db.Close()
			return
		}
		if err := applyAddedColumns(db); err != nil {
			a.dbErr = err
			_ = db.Close()
			return
		}
		a.db = db
	})
	return a.db, a.dbErr
}
