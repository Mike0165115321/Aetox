package main

// Local store: one SQLite file (<UserConfigDir>/aetox/aetox.db) holds every
// project's chat history — nothing ever leaves the machine. FTS5 with the
// trigram tokenizer gives substring full-text search that works for Thai
// (no word boundaries needed) as well as English. Driver is modernc.org/sqlite
// (pure Go, no CGO), which bundles FTS5.
//
// Schema grows here: future tables (agent memories with embedding BLOBs, …)
// belong in this same file, as a new entry in `migrations`.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mike0165115321/Aetox/internal/config"
	_ "modernc.org/sqlite"
)

const baselineSchema = `
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

// toolRunsSchema (schema version 2) records one row per completed tool call —
// what the model sent, what came back, who ran it, how long it took.
//
// `messages` remembers the conversation; this remembers the *work*, which is
// what a later learning pass has to reason over ("receipt jobs where OCR
// returned under 3 lines"). ToolEvent could never answer that: it carries one
// Subject for the UI and throws the arguments away.
//
// args/output are stored truncated with the true byte length beside them (see
// recordToolRun): a single `read` of a large file or a `web_fetch` of a long
// page would otherwise grow aetox.db without bound on the user's machine, and
// "33 MB, local-first" is a promise about their disk too. output_sha256 is over
// the *whole* output, so two runs that truncate to the same prefix are still
// distinguishable.
//
// parent_ref is the `task` call that caused this run, empty for the main
// agent's own calls. The desktop command-history panel deliberately hides a
// delegate's calls (that panel is "what the main agent did"); the store keeps
// them, because "which sub-agent is bad at what" is unanswerable without them.
const toolRunsSchema = `
CREATE TABLE IF NOT EXISTS tool_runs (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id    TEXT NOT NULL DEFAULT '',
  ref           TEXT NOT NULL DEFAULT '',
  parent_ref    TEXT NOT NULL DEFAULT '',
  agent         TEXT NOT NULL DEFAULT '',
  tool          TEXT NOT NULL,
  args          TEXT NOT NULL DEFAULT '',
  args_bytes    INTEGER NOT NULL DEFAULT 0,
  output        TEXT NOT NULL DEFAULT '',
  output_bytes  INTEGER NOT NULL DEFAULT 0,
  output_sha256 TEXT NOT NULL DEFAULT '',
  ok            INTEGER NOT NULL DEFAULT 0,
  error         TEXT NOT NULL DEFAULT '',
  duration_ms   INTEGER NOT NULL DEFAULT 0,
  time          TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tool_runs_session ON tool_runs(session_id, id);
CREATE INDEX IF NOT EXISTS idx_tool_runs_tool ON tool_runs(tool, time);
`

// migration is one step from schema version N-1 to N. The version a database
// is on lives in SQLite's own `PRAGMA user_version` rather than a table of our
// own: it costs no row, no join and no bootstrap ordering problem, and it is
// written inside the same transaction as the step it describes, so a migration
// that fails halfway leaves the version behind rather than claiming work it
// did not do.
type migration struct {
	version int
	name    string
	apply   func(*sql.Tx) error
}

// migrations must only ever be appended to. Editing a shipped entry changes
// what a version *means* on machines that already ran it, which is exactly the
// drift `user_version` exists to prevent.
//
// Version 1 is the schema as it stood before versioning existed. It is written
// entirely in CREATE ... IF NOT EXISTS plus applyAddedColumns, so it is a no-op
// on every database already in the wild — those are at user_version 0 with the
// tables already present, and running step 1 over them changes nothing but the
// version marker.
var migrations = []migration{
	{
		version: 1,
		name:    "baseline",
		apply: func(tx *sql.Tx) error {
			if _, err := tx.Exec(baselineSchema); err != nil {
				return err
			}
			return applyAddedColumns(tx)
		},
	},
	{
		version: 2,
		name:    "tool_runs",
		apply: func(tx *sql.Tx) error {
			_, err := tx.Exec(toolRunsSchema)
			return err
		},
	},
}

// latestSchemaVersion is what this build understands.
func latestSchemaVersion() int {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].version
}

// sqlExecQuerier is the overlap between *sql.DB and *sql.Tx that
// applyAddedColumns needs — it ran against the database directly before
// migrations existed and now runs inside one.
type sqlExecQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
}

// migrate brings an open database up to latestSchemaVersion, one transaction
// per step.
//
// A database from a *newer* build is refused rather than used: this build does
// not know what a future step changed, and a wrong write into a user's only
// copy of their history is worse than the feature being unavailable until they
// upgrade. The app still starts — every caller of database() treats an error as
// "no history", not as a fatal.
func migrate(db *sql.DB) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if latest := latestSchemaVersion(); current > latest {
		return fmt.Errorf(
			"aetox.db is at schema version %d but this build knows only %d — it was written by a newer Aetox. Nothing was changed; upgrade to open this history",
			current, latest)
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("migration %d (%s): begin: %w", m.version, m.name, err)
		}
		if err := m.apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		// PRAGMA takes no bound parameter, hence the format — m.version is an
		// int from this file's own table, never user input.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): set version: %w", m.version, m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d (%s): commit: %w", m.version, m.name, err)
		}
	}
	return nil
}

// addedColumns are columns introduced after a table shipped. The baseline
// schema is all CREATE TABLE IF NOT EXISTS, which is a no-op on a database that
// already has the table — so a new column reaches existing installs only from
// here. Without this the INSERT would fail on every turn, and since usage
// writes only log their errors, the stats page would quietly stop growing.
//
// New work should add a migration instead; this stays because the databases it
// fixes are already out there, and step 1 has to keep doing what it did.
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
func applyAddedColumns(db sqlExecQuerier) error {
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

func hasColumn(db sqlExecQuerier, table, column string) (bool, error) {
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
		if err := migrate(db); err != nil {
			a.dbErr = err
			_ = db.Close()
			return
		}
		a.db = db
	})
	return a.db, a.dbErr
}
