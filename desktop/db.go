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
	"github.com/Mike0165115321/Aetox/internal/turn"
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

// toolRunsFTSSchema (schema version 3) makes the work history searchable the
// same way the chat history already is: trigram FTS5, so Thai substrings match
// without word boundaries. Indexing the stored (clamped) args/output — not the
// full originals — is deliberate: the clamp bounds the index the same way it
// bounds the table, and anything worth finding again ("that OCR that read the
// wrong amount", "the fetch that hit a login page") shows in the first
// kilobytes. session_search reads this; nothing else does.
const toolRunsFTSSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS tool_runs_fts USING fts5(
  tool, args, output, content='tool_runs', content_rowid='id', tokenize='trigram'
);
CREATE TRIGGER IF NOT EXISTS tool_runs_ai AFTER INSERT ON tool_runs BEGIN
  INSERT INTO tool_runs_fts(rowid, tool, args, output) VALUES (new.id, new.tool, new.args, new.output);
END;
CREATE TRIGGER IF NOT EXISTS tool_runs_ad AFTER DELETE ON tool_runs BEGIN
  INSERT INTO tool_runs_fts(tool_runs_fts, rowid, tool, args, output) VALUES ('delete', old.id, old.tool, old.args, old.output);
END;
`

// learningSchema (schema version 7) is the floor the learning layers stand on.
//
// It was written as 6 and became 7 the same day, because `project_folders`
// reached that number first. A migration must always take the next free one: a
// step numbered below a version some database has already passed never runs on
// it, so the tables would be missing on exactly the machines that had been kept
// most up to date.
//
// `tool_runs` remembers what the agent did. Neither it nor `messages` says
// whether any of it was any good: `tool_runs.ok` means "the tool did not
// error", which a confidently wrong OCR passes as easily as a correct one. So
// nothing in the store could answer "which way of doing this job works" — and
// a system that cannot answer that cannot improve itself, only change itself.
//
// `jobs` is one row per unit of work — a chat turn, or one `task` handed to a
// delegate. Three columns carry the weight:
//
//   - tool_seq ("read>image_ocr>sheet_write") is the shape of the work,
//     matchable with a plain GROUP BY. Finding "this job has happened five
//     times" costs a query, not a model call, which is what makes repeat
//     detection affordable enough to run on every turn.
//   - outcome is the score, and outcome_source records where it came from, so
//     a rating the user actually gave is never confused with one inferred
//     from behaviour.
//   - agent is the scope: "" for the main agent, else the profile that ran.
//     Every later layer reads it, because what a delegate learned belongs to
//     that delegate and must not leak into the main agent's prompt.
//
// request/answer are clamped like tool_runs' args/output, and for the same
// reason: this is the user's disk.
//
// `pending_changes` is the door. Anything the agent proposes to learn — a
// memory line, a skill, a prompt revision — lands here and does nothing until
// a human approves it. Rows are NOT deleted when approved: the row is the only
// record of why a learned artifact exists and what it replaced, which is the
// difference between an agent that shows its work and one that quietly
// rewrites itself. `before` makes an approval reversible; `evidence` names the
// job rows the proposal was drawn from.
const learningSchema = `
CREATE TABLE IF NOT EXISTS jobs (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id     TEXT NOT NULL DEFAULT '',
  message_id     INTEGER NOT NULL DEFAULT 0,
  agent          TEXT NOT NULL DEFAULT '',
  parent_ref     TEXT NOT NULL DEFAULT '',
  request        TEXT NOT NULL DEFAULT '',
  answer         TEXT NOT NULL DEFAULT '',
  tool_seq       TEXT NOT NULL DEFAULT '',
  tool_count     INTEGER NOT NULL DEFAULT 0,
  failed_tools   INTEGER NOT NULL DEFAULT 0,
  duration_ms    INTEGER NOT NULL DEFAULT 0,
  outcome        TEXT NOT NULL DEFAULT 'unknown',
  outcome_source TEXT NOT NULL DEFAULT '',
  time           TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_session ON jobs(session_id, id);
CREATE INDEX IF NOT EXISTS idx_jobs_shape ON jobs(agent, tool_seq, outcome);
CREATE INDEX IF NOT EXISTS idx_jobs_message ON jobs(message_id);

CREATE TABLE IF NOT EXISTS pending_changes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  kind       TEXT NOT NULL,
  scope      TEXT NOT NULL DEFAULT '',
  target     TEXT NOT NULL DEFAULT '',
  op         TEXT NOT NULL DEFAULT 'add',
  before     TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  reason     TEXT NOT NULL DEFAULT '',
  evidence   TEXT NOT NULL DEFAULT '',
  source     TEXT NOT NULL DEFAULT '',
  state      TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL,
  decided_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pending_state ON pending_changes(state, id);
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
	{
		version: 3,
		name:    "tool_runs_fts",
		apply: func(tx *sql.Tx) error {
			if _, err := tx.Exec(toolRunsFTSSchema); err != nil {
				return err
			}
			// Databases that lived at version 2 already hold rows the new
			// triggers never saw; index them now or they would be the one
			// permanently unsearchable stretch of history.
			_, err := tx.Exec(`INSERT INTO tool_runs_fts(rowid, tool, args, output)
				SELECT id, tool, args, output FROM tool_runs`)
			return err
		},
	},
	{
		version: 4,
		name:    "message_variants",
		apply: func(tx *sql.Tx) error {
			for _, stmt := range []string{
				// A regenerated answer is an alternate for the SAME bubble, not a
				// second bubble: `text` stays the live one, so FTS, session titles
				// and the context rebuild all keep reading the column they always
				// did, and none of them learn what a variant is. The alternates
				// ride alongside as JSON (see storedVariant), which costs one
				// column instead of a second table and a grouping key.
				`ALTER TABLE messages ADD COLUMN variants TEXT NOT NULL DEFAULT ''`,
				// The turn as it actually happened — prose, thinking segments and
				// tool calls in order (turn.TurnPart). `text` is the concatenation
				// of its prose, so every older reader is unaffected; this is what
				// lets a reopened session show the work rather than only the
				// conclusion, which no amount of columns on `text` could.
				`ALTER TABLE messages ADD COLUMN parts TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE messages ADD COLUMN variant_active INTEGER NOT NULL DEFAULT 0`,
				// Reasoning and its clock were on SessionMessage from the start and
				// written to nothing: appendTurn inserted role/text/time only, so
				// reopening a session dropped every "คิดเป็นเวลา Xs" panel it had.
				// They are stored per variant too, inside the JSON above.
				`ALTER TABLE messages ADD COLUMN reasoning TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE messages ADD COLUMN think_secs INTEGER NOT NULL DEFAULT 0`,
			} {
				if _, err := tx.Exec(stmt); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version: 5,
		name:    "fts_update_triggers",
		apply: func(tx *sql.Tx) error {
			for _, stmt := range []string{
				// An external-content FTS5 table is not a view: nothing updates it
				// but a trigger, and version 4 shipped `text` as a column that gets
				// UPDATEd (storeVariants, when the user asks for another answer).
				// The insert/delete pair alone leaves the index holding a row the
				// table no longer has, and SQLite then reports the whole database
				// as "malformed" on the delete that tries to reconcile them — in
				// the exact place DeleteSession promises to remove a session.
				//
				// Written as delete-then-insert rather than an FTS 'update'
				// command because that is the documented shape for external
				// content, and because the old value has to come from OLD.
				`DROP TRIGGER IF EXISTS messages_au`,
				`CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
				   INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.id, old.text);
				   INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
				 END`,
				// tool_runs has no UPDATE anywhere today. The trigger goes in
				// anyway: the cost is one unfired trigger, and the alternative is
				// the same silent corruption the first time somebody adds one.
				`DROP TRIGGER IF EXISTS tool_runs_au`,
				`CREATE TRIGGER tool_runs_au AFTER UPDATE ON tool_runs BEGIN
				   INSERT INTO tool_runs_fts(tool_runs_fts, rowid, tool, args, output) VALUES ('delete', old.id, old.tool, old.args, old.output);
				   INSERT INTO tool_runs_fts(rowid, tool, args, output) VALUES (new.id, new.tool, new.args, new.output);
				 END`,
				// Repair, not just prevention. Anyone who pressed "ตอบใหม่" on
				// v0.8.6 already has a desynced index, and it will not heal on its
				// own — rebuild puts it back from the table it shadows.
				`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`,
				`INSERT INTO tool_runs_fts(tool_runs_fts) VALUES('rebuild')`,
			} {
				if _, err := tx.Exec(stmt); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version: 6,
		name:    "project_folders",
		apply: func(tx *sql.Tx) error {
			// The folders a user added to a project, and the whole of what the
			// sandbox gate is widened by (skill.RegistryOptions.ExtraRoots).
			// Stored per project rather than globally because "this project's
			// bug comes from that library" is a fact about one project, and a
			// global list would quietly widen every other project too.
			//
			// No enabled/disabled column, no read-only flag: a folder is on the
			// list or it is not. Any second dimension here becomes a permission
			// the user has to reason about somewhere other than the list they
			// can see.
			_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS project_folders (
  project_key TEXT NOT NULL,
  path        TEXT NOT NULL,
  added_at    TEXT NOT NULL,
  PRIMARY KEY (project_key, path)
);`)
			return err
		},
	},
	{
		version: 7,
		name:    "learning_floor",
		apply: func(tx *sql.Tx) error {
			_, err := tx.Exec(learningSchema)
			return err
		},
	},
	{
		version: 8,
		name:    "session_mode",
		apply: func(tx *sql.Tx) error {
			// Which desk this session was opened at (ARCHITECTURE.md §83) —
			// assistant, coding, specialized, or '' for every session that
			// predates modes. '' means "the full desk", so an upgraded install
			// reopens its history with exactly the tools it had; no backfill
			// could honestly claim to know which mode an old session was.
			//
			// On sessions rather than messages because the mode is decided at
			// creation and never changes — the entire value of a mode is that
			// the context never contained the other desks' tools, and a column
			// that could vary per message would say switching is a thing.
			_, err := tx.Exec(`ALTER TABLE sessions ADD COLUMN mode TEXT NOT NULL DEFAULT ''`)
			return err
		},
	},
	{
		version: 9,
		name:    "session_agent",
		apply: func(tx *sql.Tx) error {
			// The session's second coordinate (§85): which agent the user is
			// talking to directly, '' for every session held with the main
			// assistant — which is all of them until the office's direct chat
			// existed, so the default is also the truth about old rows.
			//
			// A chair session is mode='specialized' + agent='<chair>'. Same
			// column name as jobs.agent on purpose: one spelling for "whose
			// work is this" across the store.
			_, err := tx.Exec(`ALTER TABLE sessions ADD COLUMN agent TEXT NOT NULL DEFAULT ''`)
			return err
		},
	},
	{
		version: 10,
		name:    "session_space",
		apply: func(tx *sql.Tx) error {
			// The session's third coordinate (COMPANY.md §84): which โปรเจกต์
			// at the storefront door it was held inside, '' for every chat held
			// outside one — which is all of them until this shipped, so the
			// default is also the truth about old rows.
			//
			// The name of the folder, not a key: `<DataRoot>/project/<name>` is
			// the only record that a project exists (desktop/spaces.go), so a
			// number here would be a second identity for something the disk
			// already names. A renamed folder therefore orphans its chats
			// rather than following them, which is the honest outcome — the
			// column records where a conversation was held, and renaming a
			// folder afterwards does not change where it was held.
			_, err := tx.Exec(`ALTER TABLE sessions ADD COLUMN space TEXT NOT NULL DEFAULT ''`)
			return err
		},
	},
	{
		version: 11,
		name:    "tool_run_error_kind",
		apply: func(tx *sql.Tx) error {
			// Where a failure came from, which the error text cannot say
			// (turn.ErrorFromProgram). '' means an error this codebase wrote —
			// the conservative default, and what every reader assumed before the
			// column existed.
			if _, err := tx.Exec(
				`ALTER TABLE tool_runs ADD COLUMN error_kind TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
			// Backfill, and the only place in this file that reads an error's
			// text to decide what it is. Rows written before the column existed
			// have no other evidence left, and leaving them unmarked is not
			// neutral: the summarizer would keep proposing them as lessons
			// forever, which is the very thing this column exists to stop. New
			// rows are classified from the error value and never from its
			// spelling — the pattern here must not grow into a rule anywhere
			// else.
			//
			// `exit status <n>` is Go's own formatting of *exec.ExitError, fixed
			// in the standard library, which is what makes it safe to match on
			// once against rows that can no longer be re-derived.
			_, err := tx.Exec(
				`UPDATE tool_runs SET error_kind = ?
				  WHERE ok = 0 AND error GLOB 'exit status [0-9]*'`, turn.ErrorFromProgram)
			return err
		},
	},
	{
		version: 12,
		name:    "message_error_text",
		apply: func(tx *sql.Tx) error {
			// Why a turn stopped, on the answer it stopped in the middle of.
			//
			// Until now a turn that failed was persisted as half a turn: openTurn
			// had already written the question, and SendMessage returned before
			// appendTurn, so the answer — and the fact that there had been an
			// error at all — existed only in the window's memory. Reload, and the
			// red box and its ลองใหม่ button were gone, leaving a question sitting
			// alone with no reply and nothing saying why.
			//
			// One column rather than a `failed` flag beside it: non-empty IS
			// failed, so there is no pair of columns that can disagree about
			// whether a turn worked. Empty is every message ever written before
			// this, and every one that succeeds after — the default needs no
			// backfill because "no error" is what those rows have always meant.
			//
			// The raw error string, not a sentence for the user. Which words a
			// person reads is the frontend's business (cockpit.sendError, and
			// turnStopped for a cancel), and a Thai sentence frozen into the
			// database would be the wrong language the day somebody switches.
			_, err := tx.Exec(
				`ALTER TABLE messages ADD COLUMN error_text TEXT NOT NULL DEFAULT ''`)
			return err
		},
	},
	{
		version: 13,
		name:    "session_stance",
		apply: func(tx *sql.Tx) error {
			// The session's fourth coordinate (DECISIONS.md §106): how the turn
			// runs, after mode (v8) = which desk, agent (v9) = whose chat, and
			// space (v10) = which โปรเจกต์.
			//
			// '' is ลงมือ — the default stance, today's behaviour, and what
			// every row written before this column meant. mode.StanceAct is the
			// empty string for exactly this reason, so the column default and
			// the zero value in Go say the same thing without either side
			// translating.
			//
			// On the session rather than on each message, because a stance is
			// state the user set and left set, not a property of one turn. What
			// a *message* still cannot say is which stance produced it — §106.4
			// asks for that too, and it is a separate column on `messages` that
			// this build has not needed yet: ลงมือ and คู่คิด are told apart by
			// whether the answer touched anything, and the ambiguity §106
			// warns about arrives with วางแผน, which refuses to write files
			// while looking exactly like a desk that could.
			_, err := tx.Exec(
				`ALTER TABLE sessions ADD COLUMN stance TEXT NOT NULL DEFAULT ''`)
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
