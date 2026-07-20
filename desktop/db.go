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
	"os"
	"path/filepath"

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
`

// database opens (once) the app-wide SQLite store.
func (a *App) database() (*sql.DB, error) {
	a.dbInit.Do(func() {
		configDir, err := os.UserConfigDir()
		if err != nil || configDir == "" {
			configDir = os.Getenv("LOCALAPPDATA")
		}
		dir := filepath.Join(configDir, "aetox")
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
		a.db = db
	})
	return a.db, a.dbErr
}
