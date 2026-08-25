package main

// Chat-session persistence in the local SQLite store (see db.go), separated
// per project via project_key — the sidebar only ever lists the history of
// the project that's open. Turns are written incrementally as they happen, so
// nothing is lost on crash. Loading a session also restores the agent's
// context (RestoreHistory) so the AI remembers the conversation.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/mode"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/subagent"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// SessionMessage is one chat bubble, as the UI shows it.
type SessionMessage struct {
	// ID is the store's rowid, and it is how a reply is addressed after the
	// fact: the rating under a bubble, and the job rows that bubble produced,
	// both key on it. 0 on a message that has not been persisted yet.
	ID   int64  `json:"id,omitempty"`
	Role string `json:"role"` // "user" | "agent"
	Text string `json:"text"`
	Time string `json:"time"`
	// Rating is the verdict already stored for this reply ("good"/"bad"/
	// "unknown"), so a reopened session shows the thumb the user pressed
	// instead of a blank pair. Agent messages only.
	Rating string `json:"rating,omitempty"`
	// Reasoning is the model's thinking for this reply, kept so the panel
	// survives turn completion and session reloads (collapsed by default).
	Reasoning string `json:"reasoning,omitempty"`
	// ThinkSecs is how long the model streamed thinking (first→last reasoning
	// chunk), for the "คิดเป็นเวลา Xs" label. 0 when the turn had no thinking.
	ThinkSecs int `json:"thinkSecs,omitempty"`
	// Variants holds every answer this bubble has had, oldest first, when the
	// user asked the question more than once. Text/Reasoning/ThinkSecs above
	// always mirror Variants[Active] — the live answer — so every reader that
	// predates variants keeps working unchanged.
	//
	// Empty for the overwhelming majority of messages, which were answered once.
	Variants []SessionVariant `json:"variants,omitempty"`
	Active   int              `json:"active,omitempty"`
	// Parts is the turn as it happened — prose, thinking segments and tool
	// calls in order (turn.TurnPart). Text is the concatenation of its prose,
	// so a reader that only knows about Text is unaffected; a reader that draws
	// Parts gets narration in the place it was said and the work in between.
	//
	// Nil on a user message, and on every agent message written before the
	// sequence existed.
	Parts []turn.TurnPart `json:"parts,omitempty"`
	// ErrorText is why this turn stopped, raw, when it did not finish — the
	// quota that ran out, the connection that dropped, or `context canceled` for
	// the Stop button. Non-empty IS "this turn failed"; there is no second flag
	// to disagree with it.
	//
	// It carries the failure across a reload. What the user sees is composed on
	// the other side, where the language lives: Text (whatever streamed before
	// the wall) with the localized ending under it, and the ลองใหม่ chip fed by
	// the question above.
	ErrorText string `json:"errorText,omitempty"`
}

// SessionVariant is one of the answers a question received. Stored as JSON in
// messages.variants rather than as extra rows, so the transcript stays one row
// per bubble and nothing else in this file has to learn what a variant is.
type SessionVariant struct {
	Text      string `json:"text"`
	Reasoning string `json:"reasoning,omitempty"`
	ThinkSecs int    `json:"thinkSecs,omitempty"`
	// Parts is the sequence THIS answer produced. Each attempt does its own
	// work — a second try may read different files — so a variant that carried
	// only its text left the bubble showing one answer beside another answer's
	// tool calls the moment the user flipped between them.
	Parts []turn.TurnPart `json:"parts,omitempty"`
}

// SessionMeta is one row in the sidebar's history list. Snippet is only set
// on search results. ProjectKey/ProjectName are only set by the cross-project
// (global) queries — the per-project ones would just repeat the active project.
type SessionMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"` // RFC3339
	// Mode is the desk this conversation was held at, "" for the sessions that
	// predate desks. Every list carries it, including the cross-project ones,
	// because the sidebar is where a person picks a chat back up and "which
	// desk was this?" is the first thing they need to know — and filtering the
	// list to one desk is impossible from a list that does not say.
	Mode string `json:"mode,omitempty"`
	// Agent is who the conversation was held with (§85): "" for the main
	// assistant, a chair's name for a direct chat in the office. Carried for
	// the same reason Mode is — the row has to say who you were talking to
	// before you click it.
	Agent   string `json:"agent,omitempty"`
	Snippet string `json:"snippet,omitempty"`
	// Space is the โปรเจกต์ the conversation was held inside (§90), "" for the
	// chats held outside every one. Only search results carry it filled in:
	// the plain lists have already dropped those rows, so a value here always
	// means "found by searching, and it lives somewhere else".
	Space       string `json:"space,omitempty"`
	ProjectKey  string `json:"projectKey,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
}

// heldOutsideAProject is the condition every general history list carries.
//
// A chat held inside a โปรเจกต์ belongs to that project's own list and to
// nothing else — mixed into the sidebar it reads as a chat that is in two
// places, which is what the room was built to stop. Search is deliberately
// exempt: a person who types a word is looking for a conversation, not for a
// tidy list, and the row they get back says which project it came from.
const heldOutsideAProject = "space = ''"

// andClause appends an optional condition to one that is already there.
func andClause(clause string) string {
	if strings.TrimSpace(clause) == "" {
		return ""
	}
	return " AND " + clause
}

// projectKey isolates each project's history: readable base name + short hash
// of the full path (so two folders named "app" don't collide).
//
// The definition moved to config.ProjectKey when the agent's per-project memory
// started keying on the same thing. Two implementations of it would let a
// project's history and its memory disagree about what that project is called,
// silently — this wrapper is here so the eight call sites below keep reading
// the way they always did.
func projectKey(sandboxRoot string) string {
	return config.ProjectKey(sandboxRoot)
}

// isUnfocusedKey reports whether a stored session belongs to the "no project
// open" bucket — the one that never gets a projects-table row.
//
// Two keys, not one: the current unfocused root, and the home directory, which
// was the unfocused root until 2026-07-26 (§19.1 amendment). Every chat held
// without a project open before then is filed under the old key, and losing
// the check would answer real, still-present history with "โฟลเดอร์อาจถูกย้าย
// หรือลบไปแล้ว" — the transcripts are fine, only the bucket was renamed.
func isUnfocusedKey(key string) bool {
	if key == "" {
		return false
	}
	if root := unfocusedRoot(); root != "" && key == projectKey(root) {
		return true
	}
	home, err := os.UserHomeDir()
	return err == nil && home != "" && key == projectKey(home)
}

// newSessionID is a chat's permanent name: the moment it was created, to the
// millisecond, which reads as a date in the database and sorts as one too.
//
// Two chats created inside the same millisecond used to get the same name.
// That was unreachable in practice while the app held one conversation and
// every door out of a running turn refused — you cannot click twice in a
// millisecond — and it stopped being unreachable the moment opening a chat
// became something code does beside a running turn. The collision is not
// cosmetic: two conversations land on one key in the live map, and openTurn's
// ON CONFLICT folds their rows into one session in the store. A test that
// opened a second chat programmatically hit it on the first run.
//
// The clock is still the name; the counter only breaks ties, and only within
// the one millisecond that produced them.
var (
	sessionIDMu   sync.Mutex
	lastSessionID string
	sessionIDSeq  int
)

func newSessionID() string {
	stamp := time.Now().Format("20060102-150405.000")
	sessionIDMu.Lock()
	defer sessionIDMu.Unlock()
	if stamp != lastSessionID {
		lastSessionID, sessionIDSeq = stamp, 0
		return stamp
	}
	sessionIDSeq++
	return fmt.Sprintf("%s-%d", stamp, sessionIDSeq)
}

func sessionTitleFrom(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return "(ว่าง)"
	}
	if r := []rune(t); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return t
}

// appendTurn persists one user/agent exchange into the current session.
// The session row is created on the first turn (title = first user message).
//
// Returns the agent message's rowid, or 0 if nothing was written. That id is
// how a rating finds its job later: the thumbs live under a bubble, and the
// bubble is the only thing the UI can name.
// openTurn writes the user's message the moment it is sent, before the model
// has said anything.
//
// It used to be written with the answer, in one transaction at the end of the
// turn — so a window that reloaded while the assistant was still working lost
// the question too, and the session row was never created at all. What the user
// saw was a conversation that had been on screen a second ago and now did not
// exist: "อ้าวแชทหาย … ยังคุยไม่เสร็จเลย". The turn that was interrupted is the
// exact turn most worth keeping, because it is the one the user has not
// finished with.
//
// The pair is no longer atomic, and that is the point. A question with no
// answer under it is the honest record of what happened; a conversation that
// vanishes is not. Returns false when there is nothing to write into, so the
// caller can carry on rather than treat it as a failed turn.
func (a *App) openTurn(conv *conversation, userMsg SessionMessage) bool {
	db, err := a.database()
	sessionID := conv.id
	if err != nil || sessionID == "" {
		return false
	}
	now := time.Now().Format(time.RFC3339)
	// The session row is created here now, which is also where the desk, the
	// agent and the project are recorded. Same rule as before: a session is
	// born with all three and the ON CONFLICT branch touches none of them.
	if _, err := db.Exec(`
		INSERT INTO sessions(id, project_key, title, created_at, updated_at, mode, agent, space, stance,
		                     provider, model, wire_format, think_level, approval_mode)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			updated_at = excluded.updated_at,
			-- The dials, unlike the four coordinates above, are meant to be
			-- turned mid-conversation, so the row follows them: what is
			-- recorded is what actually answered, updated by the act of
			-- answering. mode/agent/space/stance are deliberately absent — a
			-- session is born at a desk and stays there (§83).
			provider = excluded.provider,
			model = excluded.model,
			wire_format = excluded.wire_format,
			think_level = excluded.think_level,
			approval_mode = excluded.approval_mode`,
		sessionID, projectKey(conv.cfg.SandboxRoot), sessionTitleFrom(userMsg.Text), now, now,
		conv.desk.DeskName(), conv.chair, conv.space, conv.stance.String(),
		conv.cfg.ModelProvider, conv.cfg.ModelName, conv.cfg.ModelWireFormat,
		conv.cfg.ThinkLevel, conv.cfg.ApprovalMode); err != nil {
		return false
	}
	if _, err := db.Exec(
		`INSERT INTO messages(session_id, role, text, time, reasoning, think_secs, parts) VALUES(?,?,?,?,?,?,?)`,
		sessionID, userMsg.Role, userMsg.Text, userMsg.Time, userMsg.Reasoning, userMsg.ThinkSecs, encodeParts(userMsg.Parts)); err != nil {
		return false
	}
	conv.turnOpened = true
	return true
}

func (a *App) appendTurn(conv *conversation, userMsg, agentMsg SessionMessage) int64 {
	db, err := a.database()
	// The turn's own session, handed in when the turn began — never "the chat
	// on screen" read now, which is wherever the user has moved to since.
	sessionID := conv.id
	if err != nil || sessionID == "" {
		return 0
	}
	tx, err := db.Begin()
	if err != nil {
		return 0
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Format(time.RFC3339)
	// The row is created here, on the first turn, so this is where the desk —
	// and, since §85, the agent — is recorded; the ON CONFLICT branch
	// deliberately touches neither. A session is born at a desk, with whoever
	// it was opened with, and stays there (§83); an UPDATE of either column
	// would be the mid-session switch the whole design refuses.
	_, _ = tx.Exec(`
		INSERT INTO sessions(id, project_key, title, created_at, updated_at, mode, agent, space, stance,
		                     provider, model, wire_format, think_level, approval_mode)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			updated_at = excluded.updated_at,
			-- The dials, unlike the four coordinates above, are meant to be
			-- turned mid-conversation, so the row follows them: what is
			-- recorded is what actually answered, updated by the act of
			-- answering. mode/agent/space/stance are deliberately absent — a
			-- session is born at a desk and stays there (§83).
			provider = excluded.provider,
			model = excluded.model,
			wire_format = excluded.wire_format,
			think_level = excluded.think_level,
			approval_mode = excluded.approval_mode`,
		sessionID, projectKey(conv.cfg.SandboxRoot), sessionTitleFrom(userMsg.Text), now, now,
		conv.desk.DeskName(), conv.chair, conv.space, conv.stance.String(),
		conv.cfg.ModelProvider, conv.cfg.ModelName, conv.cfg.ModelWireFormat,
		conv.cfg.ThinkLevel, conv.cfg.ApprovalMode)
	// The question, unless openTurn already wrote it when it was asked —
	// writing it twice would double every user message in the transcript.
	//
	// The flag is THIS conversation's, read off the `conv` this function was
	// already handed — never off `a.cur()`, which is wherever the user has
	// moved to since. It was the cursor for as long as only one turn could
	// exist ("only one turn is ever in flight", the old comment said), and the
	// day two could, a turn ending off screen consulted the OPEN chat's flag:
	// false there, so the question went in a second time — the first doubled
	// user bubble arrived within minutes of two chats genuinely overlapping
	// (measured live, 22 ส.ค., session D of the multichat test). The other
	// direction is quieter and worse: an open flag on the wrong conversation
	// makes a turn skip storing its question entirely.
	pending := []SessionMessage{userMsg, agentMsg}
	if conv.turnOpened {
		pending = []SessionMessage{agentMsg}
	}
	conv.turnOpened = false
	var agentID int64
	for _, m := range pending {
		res, execErr := tx.Exec(`INSERT INTO messages(session_id, role, text, time, reasoning, think_secs, parts) VALUES(?,?,?,?,?,?,?)`,
			sessionID, m.Role, m.Text, m.Time, m.Reasoning, m.ThinkSecs, encodeParts(m.Parts))
		if execErr == nil {
			agentID, _ = res.LastInsertId() // the agent's row is the last one written
		}
	}
	if tx.Commit() != nil {
		return 0
	}
	return agentID
}

// appendFailedTurn closes a turn that never produced an answer, writing the
// agent row openTurn's question has been waiting for.
//
// It is appendTurn's other ending, not a special case beside it: the same one
// row, in the same session, keeping the same strict user/agent pairing every
// reader downstream assumes (regenerate, answer-again, edit-and-resend, the
// context rebuild). What differs is that the row carries error_text, and that
// its Text is whatever streamed before the wall — often nothing, sometimes half
// an answer the user watched arrive and would rather not lose.
//
// The FTS triggers index it like any other row, which is right: "the turn where
// the quota ran out" is a thing a person searches for.
func (a *App) appendFailedTurn(conv *conversation, agentMsg SessionMessage, cause error) int64 {
	db, err := a.database()
	sessionID := conv.id
	if err != nil || sessionID == "" || cause == nil {
		return 0
	}
	// The flag is lowered here as well as in appendTurn, because this is now the
	// other way a turn can end. Left standing it would tell the NEXT turn's
	// appendTurn that its question was already stored.
	conv.turnOpened = false
	// Never empty, whatever the error stringifies to: an empty column means "this
	// turn succeeded", and a failure recorded as a success is worse than one
	// recorded with a vague reason.
	reason := strings.TrimSpace(cause.Error())
	if reason == "" {
		reason = "unknown error"
	}
	res, execErr := db.Exec(
		`INSERT INTO messages(session_id, role, text, time, reasoning, think_secs, parts, error_text)
		 VALUES(?,?,?,?,?,?,?,?)`,
		sessionID, "agent", agentMsg.Text, agentMsg.Time,
		agentMsg.Reasoning, agentMsg.ThinkSecs, encodeParts(agentMsg.Parts), reason)
	if execErr != nil {
		return 0
	}
	// Same as any other turn: the conversation was just worked in, so it belongs
	// at the top of the history list. A turn that failed is still the most recent
	// thing that happened in it — arguably the one the user will come back to.
	_, _ = db.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), sessionID)
	id, _ := res.LastInsertId()
	return id
}

// startNewSession opens a fresh chat and puts it on screen.
//
// It used to empty the chat that was open: new id, transcript to nil, project
// and stance back to their defaults, ClearContext on the agent. That was the
// only way to say "new" when the app held one conversation — and it is also why
// a turn in flight had to be refused, because emptying the chat it was running
// in is exactly what it sounds like. A new chat is a new conversation now, and
// whatever was open keeps whatever it was doing.
//
// The desk (and its chair) come across, because those are where the WINDOW is
// rather than what the last chat held: pressing "new chat" at a desk opens a
// chat at that desk. Everything else starts at its default — no project, ลงมือ,
// and a memory that has never held anything.
func (a *App) startNewSession() {
	prev := a.cur()
	conv := newConversation()
	conv.id = newSessionID()
	conv.desk, conv.chair = prev.desk, prev.chair
	a.applyConfig(conv, a.cfg)
	a.showConversation(conv)
}

// ListSessions returns this project's chat history, newest first.
func (a *App) ListSessions() []SessionMeta {
	return a.ListSessionsAt("")
}

// ListSessionsAt is ListSessions filtered to one desk — the history list behind
// a single button (COMPANY.md §2).
//
// An empty desk means every desk rather than the legacy one, because that is
// the question the combined list asks. Sessions from before modes existed hold
// ” in the column and so appear only here, which is right: they were held at
// no desk, and filing them under one would be inventing a fact about them.
func (a *App) ListSessionsAt(desk string) []SessionMeta {
	out := []SessionMeta{}
	db, err := a.database()
	if err != nil {
		return out
	}
	query := `
		SELECT id, title, updated_at, mode, agent FROM sessions
		WHERE project_key = ? AND ` + heldOutsideAProject + ` ORDER BY updated_at DESC LIMIT 200`
	args := []any{projectKey(a.cur().cfg.SandboxRoot)}
	if desk = strings.TrimSpace(desk); desk != "" {
		query = `
		SELECT id, title, updated_at, mode, agent FROM sessions
		WHERE project_key = ? AND ` + heldOutsideAProject + ` AND mode = ? ORDER BY updated_at DESC LIMIT 200`
		args = append(args, desk)
	}
	out, _ = queryAll(db, "sessions", query, args, func(rows *sql.Rows) (SessionMeta, error) {
		var m SessionMeta
		err := rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent)
		return m, err
	})
	return out
}

// SessionMode reports which desk a session is at, so the UI can label it and
// light the room you are standing in. "" is both "no such session" and "the
// full desk", which is fine here: the label for the second is nothing, and so
// is the label for the first.
//
// The live session answers from the engine, not from the table. A session row
// is only written on its first turn, so the one you are looking at right now
// usually has no row at all — and reading "" back for it would tell the
// sidebar no room is lit while the engine is standing in one.
func (a *App) SessionMode(id string) string {
	db, err := a.database()
	if err != nil {
		return a.liveDesk(id)
	}
	var desk string
	if db.QueryRow(`SELECT mode FROM sessions WHERE id = ?`, id).Scan(&desk) != nil {
		return a.liveDesk(id)
	}
	return desk
}

// liveDesk is the desk of the session being written to right now, and "" for
// any other id — a stored session with no row is genuinely unknown, and
// answering with wherever the window happens to be would be inventing one.
func (a *App) liveDesk(id string) string {
	if id != "" && id == a.cur().id {
		return a.cur().desk.DeskName()
	}
	return ""
}

// SessionAgent reports who a stored session was held with (§85): "" for the
// main assistant — which is also the answer for a session that does not
// exist, and both label as nothing, same shape as SessionMode above.
func (a *App) SessionAgent(id string) string {
	db, err := a.database()
	if err != nil {
		return a.liveChair(id)
	}
	var chair string
	if db.QueryRow(`SELECT agent FROM sessions WHERE id = ?`, id).Scan(&chair) != nil {
		return a.liveChair(id)
	}
	return chair
}

// liveChair is liveDesk's other half: who the live session is being held with,
// before its first turn puts a row in the table.
func (a *App) liveChair(id string) string {
	if id != "" && id == a.cur().id {
		return a.cur().chair
	}
	return ""
}

// SearchSessions full-text searches this project's history (FTS5 trigram —
// works for Thai and English substrings alike).
func (a *App) SearchSessions(query string) []SessionMeta {
	out := []SessionMeta{}
	q := strings.TrimSpace(query)
	db, err := a.database()
	if err != nil || q == "" {
		return out
	}
	// Quote the query so FTS operators in user input can't break the MATCH.
	match := `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	// snippet() must stay inside a MATERIALIZED CTE: flattened into the outer
	// GROUP BY join, modernc.org/sqlite rejects it with "unable to use function
	// snippet in the requested context".
	out, _ = queryAll(db, "session search", `
		WITH f AS MATERIALIZED (
		  SELECT rowid AS mid, snippet(messages_fts, 0, '', '', '…', 10) AS snip
		  FROM messages_fts WHERE messages_fts MATCH ?
		)
		SELECT s.id, s.title, s.updated_at, s.mode, s.agent, MIN(f.snip)
		FROM f
		JOIN messages m ON m.id = f.mid
		JOIN sessions s ON s.id = m.session_id
		WHERE s.project_key = ?
		GROUP BY s.id
		ORDER BY s.updated_at DESC LIMIT 50`,
		[]any{match, projectKey(a.cur().cfg.SandboxRoot)},
		func(rows *sql.Rows) (SessionMeta, error) {
			var m SessionMeta
			err := rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.Snippet)
			return m, err
		})
	return out
}

// ProjectMeta is one row in the sidebar's project switcher.
type ProjectMeta struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	RootPath string `json:"rootPath"`
	OpenedAt string `json:"openedAt"`
	Snippet  string `json:"snippet,omitempty"` // most recent session title, if any
}

// touchProject records/refreshes a project's "last opened" time so it shows
// up in the sidebar's project switcher, even before it has any chat sessions.
func (a *App) touchProject(root string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	db, err := a.database()
	if err != nil {
		return
	}
	_, _ = db.Exec(`
		INSERT INTO projects(project_key, name, root_path, opened_at)
		VALUES(?,?,?,?)
		ON CONFLICT(project_key) DO UPDATE SET root_path = excluded.root_path, opened_at = excluded.opened_at`,
		projectKey(root), filepath.Base(filepath.Clean(root)), root, time.Now().Format(time.RFC3339))
}

// RecentProjects lists every project ever opened, newest first, each paired
// with its most recent session title (if any) for the sidebar subtitle.
func (a *App) RecentProjects() []ProjectMeta {
	out := []ProjectMeta{}
	db, err := a.database()
	if err != nil {
		return out
	}
	out, _ = queryAll(db, "projects", `
		SELECT p.project_key, p.name, p.root_path, p.opened_at,
		       COALESCE((SELECT s.title FROM sessions s
		                 WHERE s.project_key = p.project_key
		                 ORDER BY s.updated_at DESC LIMIT 1), '')
		FROM projects p
		ORDER BY p.opened_at DESC LIMIT 50`, nil,
		func(rows *sql.Rows) (ProjectMeta, error) {
			var m ProjectMeta
			err := rows.Scan(&m.Key, &m.Name, &m.RootPath, &m.OpenedAt, &m.Snippet)
			return m, err
		})
	return out
}

// ListAllSessions returns chat history across every project, newest first —
// the sidebar's global history layer, independent of which project is active.
// DeskFilter scopes a history list to one door's desks (§86).
//
// The frontend supplies it rather than the engine deriving it, because a door
// is a UI preference and not engine state — shell.svelte.ts is explicit that
// "the engine never needs to know which door you walked in through". What the
// engine stores is sessions.mode, a desk, and that is all this filters on. The
// door→desk mapping stays in exactly one place, up there, and this stays a
// question about desks.
//
// Exclude is what makes the storefront's list safe as desks are added: the
// โค้ด window asks for its own desk by name, and ผู้ช่วย asks for *everything
// else*. A new desk, a user-written one, or a legacy session held at no desk
// at all lands in the storefront without anyone having to remember to list it —
// which is the same rule shellForDesk already applies on the other side.
type DeskFilter struct {
	Desks   []string `json:"desks"`
	Exclude bool     `json:"exclude"`
}

// where renders the filter as a SQL fragment plus its arguments, or ("", nil)
// when it selects everything.
func (f DeskFilter) where(column string) (string, []any) {
	var desks []string
	for _, d := range f.Desks {
		if d = strings.TrimSpace(d); d != "" {
			desks = append(desks, d)
		}
	}
	if len(desks) == 0 {
		// An exclude-nothing filter is every session; an include-nothing filter
		// would be none, which is never what a door means — a door with no
		// desks listed is a bug upstream, and emptying the user's history to
		// report it is the wrong way round.
		return "", nil
	}
	args := make([]any, 0, len(desks))
	placeholders := make([]string, 0, len(desks))
	for _, d := range desks {
		args = append(args, d)
		placeholders = append(placeholders, "?")
	}
	op := "IN"
	if f.Exclude {
		op = "NOT IN"
	}
	return column + " " + op + " (" + strings.Join(placeholders, ",") + ")", args
}

// ListSessionsForDoor is ListAllSessions scoped to one door's desks.
//
// The scoping is in SQL and not in the caller, because LIMIT is applied after
// WHERE and that ordering is the whole point: filtering a fetched page in the
// frontend meant 200 rows were taken across every door and then thrown away, so
// 200 coding sessions in a row would hand the storefront an empty list while its
// history sat in the table. A defect that only appears once someone has used the
// app enough, which is the worst time to find one.
func (a *App) ListSessionsForDoor(filter DeskFilter) []SessionMeta {
	out := []SessionMeta{}
	db, err := a.database()
	if err != nil {
		return out
	}
	clause, args := filter.where("s.mode")
	clause = "WHERE s." + heldOutsideAProject + andClause(clause)
	out, _ = queryAll(db, "sessions for door", `
		SELECT s.id, s.title, s.updated_at, s.mode, s.agent, s.project_key, COALESCE(p.name, s.project_key)
		FROM sessions s LEFT JOIN projects p ON p.project_key = s.project_key
		`+clause+`
		ORDER BY s.updated_at DESC LIMIT 200`, args,
		func(rows *sql.Rows) (SessionMeta, error) {
			var m SessionMeta
			err := rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.ProjectKey, &m.ProjectName)
			return m, err
		})
	return out
}

// SearchSessionsForDoor is SearchAllSessions scoped to one door's desks. Its
// LIMIT is 50, so the starvation ListSessionsForDoor describes arrives four
// times sooner here.
func (a *App) SearchSessionsForDoor(query string, filter DeskFilter) []SessionMeta {
	out := []SessionMeta{}
	q := strings.TrimSpace(query)
	db, err := a.database()
	if err != nil || q == "" {
		return out
	}
	match := `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	clause, deskArgs := filter.where("s.mode")
	if clause != "" {
		clause = "WHERE " + clause
	}
	args := append([]any{match}, deskArgs...)
	out, _ = queryAll(db, "session search for door", `
		WITH f AS MATERIALIZED (
		  SELECT rowid AS mid, snippet(messages_fts, 0, '', '', '…', 10) AS snip
		  FROM messages_fts WHERE messages_fts MATCH ?
		)
		SELECT s.id, s.title, s.updated_at, s.mode, s.agent, s.space, s.project_key, COALESCE(p.name, s.project_key), MIN(f.snip)
		FROM f
		JOIN messages m ON m.id = f.mid
		JOIN sessions s ON s.id = m.session_id
		LEFT JOIN projects p ON p.project_key = s.project_key
		`+clause+`
		GROUP BY s.id
		ORDER BY s.updated_at DESC LIMIT 50`, args,
		func(rows *sql.Rows) (SessionMeta, error) {
			var m SessionMeta
			err := rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.Space, &m.ProjectKey, &m.ProjectName, &m.Snippet)
			return m, err
		})
	return out
}

func (a *App) ListAllSessions() []SessionMeta {
	out := []SessionMeta{}
	db, err := a.database()
	if err != nil {
		return out
	}
	out, _ = queryAll(db, "all sessions", `
		SELECT s.id, s.title, s.updated_at, s.mode, s.agent, s.project_key, COALESCE(p.name, s.project_key)
		FROM sessions s LEFT JOIN projects p ON p.project_key = s.project_key
		ORDER BY s.updated_at DESC LIMIT 200`, nil,
		func(rows *sql.Rows) (SessionMeta, error) {
			var m SessionMeta
			err := rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.ProjectKey, &m.ProjectName)
			return m, err
		})
	return out
}

// SearchAllSessions full-text searches chat history across every project.
func (a *App) SearchAllSessions(query string) []SessionMeta {
	out := []SessionMeta{}
	q := strings.TrimSpace(query)
	db, err := a.database()
	if err != nil || q == "" {
		return out
	}
	match := `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	out, _ = queryAll(db, "search all sessions", `
		WITH f AS MATERIALIZED (
		  SELECT rowid AS mid, snippet(messages_fts, 0, '', '', '…', 10) AS snip
		  FROM messages_fts WHERE messages_fts MATCH ?
		)
		SELECT s.id, s.title, s.updated_at, s.mode, s.agent, s.project_key, COALESCE(p.name, s.project_key), MIN(f.snip)
		FROM f
		JOIN messages m ON m.id = f.mid
		JOIN sessions s ON s.id = m.session_id
		LEFT JOIN projects p ON p.project_key = s.project_key
		GROUP BY s.id
		ORDER BY s.updated_at DESC LIMIT 50`, []any{match},
		func(rows *sql.Rows) (SessionMeta, error) {
			var m SessionMeta
			err := rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.ProjectKey, &m.ProjectName, &m.Snippet)
			return m, err
		})
	return out
}

// LoadSessionAnyProject loads a session from the global (cross-project) history,
// switching the active project first if the session belongs to a different one
// than whatever's currently open.
func (a *App) LoadSessionAnyProject(id string) ([]SessionMessage, error) {
	db, err := a.database()
	if err != nil {
		return nil, err
	}
	var key, rootPath string
	err = db.QueryRow(`
		SELECT s.project_key, COALESCE(p.root_path, '')
		FROM sessions s LEFT JOIN projects p ON p.project_key = s.project_key
		WHERE s.id = ?`, id).Scan(&key, &rootPath)
	if err != nil {
		return nil, fmt.Errorf("ไม่พบเซสชันนี้")
	}
	if key != projectKey(a.cur().cfg.SandboxRoot) {
		// Only this branch needs the gate, and only this branch ever did. It
		// re-roots the whole engine (reload/focusNone) — the sandbox, the
		// workspace folders, the shell backend — and those belong to the
		// machine rather than to a conversation, so a turn running ANYWHERE
		// would find the ground moved under it mid-tool-call.
		//
		// It used to sit at the top of the function, which made every row in
		// the history list unclickable while anything was working: the sidebar
		// sends every click through this door, and the overwhelming majority of
		// them are chats in the project already open, where nothing is re-rooted
		// and nothing was ever at risk. That is the refusal the owner hit again
		// after the switch was supposed to be open (20 ส.ค.: "ตอนมันทำงานผมกด
		// ไปเซสชั่นอื่นไม่ได้เลย") — the door below had opened and this one was
		// still shut in front of it.
		//
		// Before the re-root, not after: a refusal that fires afterwards has
		// already moved the machine the turn is running on.
		if err := a.guardSessionSwitch(); err != nil {
			return nil, err
		}
		// Sessions chatted "ไม่โฟกัสโปรเจกต์" live in the unfocused bucket,
		// which never gets a projects-table row — switch back to unfocused
		// mode for those instead of treating them as an orphaned project.
		//
		// And a key that has NO projects row is the same case wearing a name
		// we no longer recognize, not a project that lost its folder: every
		// real project gets its row from touchProject the moment it is opened.
		// The keys this actually catches are the buckets history has already
		// stranded — chats filed under the home key before the unfocused root
		// moved (§19.1), and chats filed under "." by a launch with a stripped
		// environment (tool_runs #533 — the launch path is fixed, the sessions
		// it stamped are still in the DB, and one of them was the owner's own
		// เงินบาท chat, unclickable the same day the run was logged). A key
		// carries no rights, so refusing protects nothing; it only orphans a
		// real transcript. Reopen it as what it is: a chat held outside every
		// project. LoadSession reads the messages by the session's own key, so
		// the odd bucket name still finds its rows.
		if isUnfocusedKey(key) || rootPath == "" {
			a.focusNone()
			return a.LoadSession(id)
		}
		a.reload(config.ConfigOptions{RootPath: rootPath, ApprovalMode: string(safety.ApprovalFullAccess)})
		a.projectFocused = true
		a.touchProject(a.cur().cfg.SandboxRoot)
	}
	return a.LoadSession(id)
}

// LoadSession switches to a stored session: the UI gets the transcript back,
// and the agent's context is rebuilt from it so the conversation continues
// with memory intact.
func (a *App) LoadSession(id string) ([]SessionMessage, error) {
	// No gate. There was one, and it refused every attempt to open another chat
	// while a turn was running, for a reason that was true when it was written:
	// this function rewrote THE agent context, so a turn in flight would have
	// finished on a conversation it never started in and its answer would have
	// followed the user into whatever chat they had opened.
	//
	// It rewrites nothing now. A conversation this process is still holding an
	// engine for is opened by attaching to it; one it is not is built fresh,
	// beside the running turn rather than on top of it (desktop/conversation.go).
	//
	// What "attaching" means is narrow, and the narrowness is the point: the
	// ENGINE is reused, the rows are not. What the model actually saw — the tool
	// calls, their results, the summaries a compaction left behind — lives in
	// that agent's context and none of it survives a trip through the text
	// transcript, so rebuilding it would throw away the difference between
	// opening a chat and coming back to one. The transcript is a different
	// question with a different home: it is rows, the store holds them, and
	// reading it from anywhere else would make `conv.transcript` a second
	// answer to what was said.
	live := a.convs.find(id)
	db, err := a.database()
	if err != nil {
		return nil, err
	}
	// Back to the desk — and, for a direct chat, the chair — this conversation
	// was held at, before its history is restored. A session reopened at
	// another desk would answer the next message with tools it never had; one
	// reopened without its chair would answer as somebody else. A legacy
	// session ('','') reopens at the full desk, exactly what it always ran with.
	//
	// A desk or chair whose file has since been deleted refuses rather than
	// falls back: the transcript is intact, but reproducing what that session
	// could do is impossible without the file, and quietly reopening it wider
	// — or as the main assistant wearing the chair's history — is worse than
	// the error. Putting the file back is the way in.
	//
	// The space comes back the same way and for the same reason, with one
	// difference: a project whose folder has since been deleted does not refuse
	// the session. The folder holds context the assistant may read, not the
	// tools it may use — reopening the conversation without it is a session
	// that knows less, not a session that can do more, so it is restored as a
	// chat held outside every project and the transcript stays reachable.
	//
	// The session's own project_key comes back here too, and it — not the key of
	// wherever the engine happens to be rooted — is what the messages are read
	// with. The two are normally the same, and where they are not, the caller has
	// already decided this session may be opened.
	//
	// The one place they legitimately differ is the unfocused bucket, which moved
	// from ~ to ~/aetox on 2026-07-26 (§19.1 amendment). isUnfocusedKey has always
	// accepted both keys, so every chat held with no project open before that date
	// got past the first gate — and then died on this query, which knew only the
	// current key. focusNone() roots the engine at ~/aetox by design; it is not
	// going to root it back at the home directory, so matching the message filter
	// to the running root can never find those rows. The session says which bucket
	// it is in; that is the answer, and it was two lines away the whole time.
	// Built beside whatever is on screen, never by rewriting it: the chat the
	// user is leaving may have a turn in it, and that turn's engine is that
	// object.
	conv := live
	if conv == nil {
		conv = newConversation()
		conv.id = id
		// Born from the template, then corrected by whatever the row recorded.
		// Both steps matter: a session from before the columns existed keeps the
		// app's default, and one that recorded its own dials gets them back.
		conv.cfg = a.cfg
	}
	var desk, chair, space, key, stance string
	var provider, modelName, wireFormat, thinkLevel, approval string
	if db.QueryRow(`SELECT mode, agent, space, project_key, stance,
	                       provider, model, wire_format, think_level, approval_mode
	                FROM sessions WHERE id = ?`, id).
		Scan(&desk, &chair, &space, &key, &stance,
			&provider, &modelName, &wireFormat, &thinkLevel, &approval) == nil {
		// Before setStation, which re-bootstraps when the desk changed: set here
		// and the engine that comes out of it already knows how this session was
		// being run, instead of being built at ลงมือ and corrected a line later.
		//
		// Normalized rather than trusted. The column can hold a stance a later
		// build wrote and this one does not implement, and NormalizeStance
		// answers ลงมือ for it — a reopened conversation must never come back
		// silently carrying nothing.
		m, seat, err := resolveStation(desk, chair)
		if err != nil {
			return nil, err
		}
		rememberDesk(m.DeskName())
		// Only a conversation being built takes its coordinates from the row. A
		// live one already IS at that desk — it has been running there — and
		// writing them again would be re-deciding a question that was settled
		// when it opened.
		if live == nil {
			// Normalized rather than trusted. The column can hold a stance a
			// later build wrote and this one does not implement, and
			// NormalizeStance answers ลงมือ for it — a reopened conversation
			// must never come back silently carrying nothing.
			conv.stance = mode.NormalizeStance(stance)
			conv.desk, conv.chair = m, seat
			conv.space = a.resolvedSpace(space)
			// The dials this chat was left on, restored the same way and for
			// the same reason as the four coordinates above: a conversation's
			// engine is a derivative that gets thrown away and rebuilt, so
			// anything that must survive that lives on the row, not in the
			// object. Holding them only in memory is what made the model
			// follow the user between chats after §155 — the field was in the
			// right place with the wrong lifetime (§157).
			//
			// Empty is "never recorded", which is every session from before the
			// column existed: those keep the app's default, which is what they
			// have always opened on.
			if provider != "" && provider != conv.cfg.ModelProvider {
				// The provider carries its own key and its own address. Set the
				// name alone and the engine is pointed at one provider holding
				// another's credentials, which comes out as "missing model API
				// key" and a silent fall back to the built-in engine — which is
				// what the owner got the first time this shipped (20 ส.ค.).
				// Same three lines SwitchProvider uses, for the same reason.
				conv.cfg.ModelProvider = provider
				conv.cfg.ModelBaseURL = resolveBaseURLForProvider(provider)
				conv.cfg.ModelAPIKey = resolveAPIKeyForProvider(provider)
			}
			if modelName != "" {
				conv.cfg.ModelName = modelName
			}
			if wireFormat != "" {
				conv.cfg.ModelWireFormat = wireFormat
			}
			if thinkLevel != "" {
				conv.cfg.ThinkLevel = thinkLevel
			}
			if approval != "" {
				conv.cfg.ApprovalMode = approval
			}
		}
	}
	if live == nil {
		// One bootstrap, for this conversation, from ITS config — the template
		// with the row's own dials laid over it. Built from a.cfg instead and
		// the restore above would be overwritten one line later by the model
		// somebody chose in another chat, which is the bug this whole section
		// exists to end.
		a.applyConfig(conv, conv.cfg)
		a.explainARecordedModelThatWillNotStart(conv, provider, modelName)
	}
	if key == "" {
		key = projectKey(a.cur().cfg.SandboxRoot)
	}
	messages, err := a.readTranscript(id, key)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("ไม่พบเซสชันนี้ในโปรเจกต์ปัจจุบัน")
	}

	conv.transcript = messages
	if live == nil && conv.agent != nil {
		// A freshly built engine has an empty context; the history it is given
		// is this session's and nothing else's. ClearContext used to be needed
		// here because the agent being restored into was the previous chat's.
		//
		// Never for a live one. Its memory is ahead of the transcript, not
		// behind it, and replaying rows over the top would be the one act this
		// whole change exists to stop.
		conv.agent.RestoreHistory(transcriptToModelMessages(messages))
	}
	a.showConversation(conv)
	return messages, nil
}

// readTranscript reads one session's messages, nothing else. The rows are read
// with the session's own project key — see LoadSession on why that key, not
// the engine's current one, is the filter.
//
// m.id comes back so a reopened session can still be rated: the thumbs
// address a job by the bubble they sit under, and without the row id an
// answer stops being ratable the moment the session is closed — which is
// exactly when a user knows whether it was any good.
func (a *App) readTranscript(id, key string) ([]SessionMessage, error) {
	db, err := a.database()
	if err != nil {
		return nil, err
	}
	// a.TurnRating below issues its own QueryRow while this cursor is still
	// open, which the unbounded default pool serves from a second connection.
	// eachRow holds no lock across the callback, so the re-entrancy is safe.
	messages := []SessionMessage{}
	err = eachRow(db, "transcript", `
		SELECT m.id, m.role, m.text, m.time, m.reasoning, m.think_secs, m.variants, m.variant_active, m.parts, m.error_text
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE m.session_id = ? AND s.project_key = ?
		ORDER BY m.id`,
		[]any{id, key},
		func(rows *sql.Rows) error {
			var m SessionMessage
			var variants, parts string
			if err := rows.Scan(&m.ID, &m.Role, &m.Text, &m.Time, &m.Reasoning, &m.ThinkSecs, &variants, &m.Active, &parts, &m.ErrorText); err != nil {
				return err
			}
			m.Variants = decodeVariants(variants)
			m.Parts = decodeParts(parts)
			m.Rating = a.TurnRating(m.ID)
			messages = append(messages, m)
			return nil
		})
	if err != nil {
		// A transcript that broke halfway is not a shorter conversation. This
		// caller has somewhere to put the error, so it gets it.
		return nil, err
	}
	return messages, nil
}

// SessionTranscript reads one session's messages without touching the engine:
// no station switch, no context rebuild, a.cur().id stays where it is.
//
// It exists for the window that just reloaded. The Go side outlives a webview
// reload, so the engine is still holding the session — and, mid-turn, still
// working in it. Putting the conversation back on that window's screen through
// LoadSession meant rebuilding an agent context the engine never lost (lossy:
// tool calls are not in the transcript), and, during a turn, rewriting the
// memory the turn was thinking with. Reading is not switching; this one reads.
func (a *App) SessionTranscript(id string) ([]SessionMessage, error) {
	db, err := a.database()
	if err != nil {
		return nil, err
	}
	var key string
	// A session not yet in the store (opened, never spoken to) has no rows to
	// read; the empty list — not an error — is exactly that fact.
	if db.QueryRow(`SELECT project_key FROM sessions WHERE id = ?`, id).Scan(&key) != nil {
		return []SessionMessage{}, nil
	}
	return a.readTranscript(id, key)
}

// NewSession starts a blank session at the desk the app is already at, and
// returns its id.
func (a *App) NewSession() (string, error) {
	a.startNewSession()
	return a.cur().id, nil
}

// NewSessionAt starts a blank session at the named desk — the five buttons'
// entry point (COMPANY.md §2). An empty name is the full desk, which is what
// NewSession has always given.
//
// An unknown name is an error rather than a quiet fall back to the full desk:
// falling back would hand a session every tool on the machine because a picker
// sent a stale name, and the one thing a desk must never do is widen by
// accident.
//
// Separate from NewSession rather than replacing it because a bound method's
// signature is a contract the running frontend already calls. This is the
// desk-aware door; the old one keeps working for everything that has not been
// pointed at a desk yet.
func (a *App) NewSessionAt(desk string) (string, error) {
	// Cleared first, then switched: setStation re-bootstraps, and a re-bootstrap
	// carries the outgoing agent's context into the new one. Emptying the
	// conversation before that means the new desk starts on nothing, which is
	// the entire promise of opening a session at one.
	a.startNewSession()
	if err := a.setStation(desk, ""); err != nil {
		return "", err
	}
	return a.cur().id, nil
}

// NewChairSession starts a blank session talking directly to one of the
// office's agents (§85), and returns its id. The desk is implied: a chair
// only exists in the office.
func (a *App) NewChairSession(chair string) (string, error) {
	a.startNewSession()
	if err := a.setStation(mode.Office, chair); err != nil {
		return "", err
	}
	return a.cur().id, nil
}

// setStation points the engine at a desk and, optionally, one of the office's
// chairs — the single writer of both fields, because they only mean anything
// as a pair: a chair on any desk but the office is a state the product says
// cannot exist (§85), and two writers is how it would come to exist anyway.
// Everything the pair decides — the dispatcher's cut, the system prompt, the
// memory scope — is built once at bootstrap from these values, so changing
// either means bootstrapping again; same path a project switch takes.
//
// A no-op when nothing changed, which is the common case: opening one
// assistant chat after another rebuilds nothing.
//
// Refusals are loud and name the file that is missing. Falling back would
// either widen a session (a stale desk name landing on the full desk) or
// impersonate someone (a deleted chair answered by the main assistant).
func (a *App) setStation(desk, chair string) error {
	desk, chair = strings.TrimSpace(desk), strings.TrimSpace(chair)
	if desk == a.cur().desk.DeskName() && chair == a.cur().chair {
		return nil
	}
	if chair != "" {
		if desk != mode.Office {
			return fmt.Errorf("เอเจนนั่งได้เฉพาะในออฟฟิศ — โต๊ะ %q มีเอเจนไม่ได้", desk)
		}
		p, ok := subagent.Load(chair)
		if !ok {
			return fmt.Errorf("ไม่รู้จักเอเจน %q — ไฟล์โปรไฟล์ของเอเจนนี้อาจถูกลบไปแล้ว", chair)
		}
		if p.Desk != mode.Office {
			return fmt.Errorf("%q ไม่ได้เป็นเอเจนของออฟฟิศ — คุยตรงได้เฉพาะโปรไฟล์ที่ประกาศ desk: specialized", chair)
		}
	}
	m, ok := mode.Load(desk)
	if !ok {
		return fmt.Errorf("ไม่รู้จักโต๊ะ %q — ไฟล์ของโต๊ะนี้อาจถูกลบไปแล้ว", desk)
	}
	a.cur().desk, a.cur().chair = m, chair
	a.applyConfig(a.cur(), a.cfg)
	rememberDesk(m.DeskName())
	return nil
}

// resolveStation is setStation's judgement without its effects: it answers
// whether this desk and chair are a pair that may exist, and what desk that is.
//
// Split out because a conversation being BUILT needs the same answer before it
// has an engine to rebuild — LoadSession opening a stored session at its own
// desk, where mutating the chat on screen first (which is what setStation does)
// would move the window to a desk that then turns out to be invalid.
func resolveStation(desk, chair string) (*mode.Mode, string, error) {
	desk, chair = strings.TrimSpace(desk), strings.TrimSpace(chair)
	if chair != "" {
		if desk != mode.Office {
			return nil, "", fmt.Errorf("เอเจนนั่งได้เฉพาะในออฟฟิศ — โต๊ะ %q มีเอเจนไม่ได้", desk)
		}
		p, ok := subagent.Load(chair)
		if !ok {
			return nil, "", fmt.Errorf("ไม่รู้จักเอเจน %q — ไฟล์โปรไฟล์ของเอเจนนี้อาจถูกลบไปแล้ว", chair)
		}
		if p.Desk != mode.Office {
			return nil, "", fmt.Errorf("%q ไม่ได้เป็นเอเจนของออฟฟิศ — คุยตรงได้เฉพาะโปรไฟล์ที่ประกาศ desk: specialized", chair)
		}
	}
	m, ok := mode.Load(desk)
	if !ok {
		return nil, "", fmt.Errorf("ไม่รู้จักโต๊ะ %q — ไฟล์ของโต๊ะนี้อาจถูกลบไปแล้ว", desk)
	}
	return m, chair, nil
}

// rememberDesk records where the window is, so the next launch opens there
// (COMPANY.md §2). Written here because setStation is the only writer of
// a.cur().desk — anywhere else would be a second answer to "where am I".
//
// The legacy full desk ("") is skipped: it is also the value that means
// "nothing remembered", and opening a pre-desk conversation to read it back is
// not the user saying that is where they want to start.
//
// Failing to remember is not worth an error. The window is already at the
// desk; all that is lost is the next launch starting somewhere else.
func rememberDesk(desk string) {
	if desk == "" {
		return
	}
	pref, _, err := config.LoadModelPreference()
	if err != nil || pref.LastDesk == desk {
		return
	}
	pref.LastDesk = desk
	_ = config.SaveModelPreference(pref)
}

// CurrentSessionID reports which session the engine is writing to, so the
// sidebar can highlight the active row.
func (a *App) CurrentSessionID() string {
	return a.cur().id
}

// DeleteSession permanently removes one session and its messages (any
// project; the messages_ad trigger cleans the FTS index). Deleting the
// session currently open also resets the transcript and agent memory.
func (a *App) DeleteSession(id string) error {
	// Only the chat the turn is writing into is protected — deleting any other
	// row touches nothing the turn holds. Without this, the delete's DELETE and
	// the turn's closing INSERT race, and appendTurn's ON CONFLICT re-creates
	// the session row the user just removed: a conversation that comes back
	// from the dead with one answer and no question.
	if a.turnRunningIn(id) {
		return errTurnBusy
	}
	db, err := a.database()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, id); err != nil {
		return err
	}
	// The session's tool runs go with it. A deleted conversation that left its
	// tool arguments and outputs behind would be the opposite of what the
	// delete button promises — and those rows hold file contents and page text,
	// not just labels.
	if _, err := tx.Exec(`DELETE FROM tool_runs WHERE session_id = ?`, id); err != nil {
		return err
	}
	// The job rows go too. They hold the question and the answer, so a session
	// the user deleted would otherwise survive in the one table built to be read
	// back later — and "delete this conversation" has to mean it everywhere, not
	// everywhere the feature existed when the button was written.
	if _, err := tx.Exec(`DELETE FROM jobs WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// The session's attachments go with it — they are inputs the user handed
	// this conversation. What does NOT go is `output/<session>`: a deleted
	// conversation must never take the work with it (COMPANY.md §6.7). The
	// report an employee wrote does not disappear because you deleted the email
	// thread that asked for it, and the ผลงาน page is the one place a file is
	// deleted, by the user.
	//
	// Session ids are our own timestamp format, but this id came over the JS
	// binding — refuse anything that could step out of the attachments folder.
	// Roots we can't see from here (another project's) are left for
	// sweepAttachments the next time that project opens.
	if id != "" && id == filepath.Base(id) && !strings.Contains(id, "..") {
		for _, root := range []string{a.cur().cfg.SandboxRoot, unfocusedRoot()} {
			if strings.TrimSpace(root) != "" {
				_ = os.RemoveAll(filepath.Join(root, attachmentsDir, id))
			}
		}
	}
	if a.cur().id == id {
		a.startNewSession()
	}
	return nil
}

// sweepAttachments removes what no session owns from <root>/.aetox-attachments:
// legacy flat files (attachments predating per-session folders — the shared
// pile any later chat could list and read), and per-session folders whose
// session row is gone (deleted while another project was open, or a crash
// between attach and first message). Runs in the background on every root
// change; a missing folder is the common case and returns immediately.
func (a *App) sweepAttachments(root string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	dir := filepath.Join(root, attachmentsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	db, err := a.database()
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			_ = os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		id := e.Name()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, id).Scan(&n); err != nil || n > 0 {
			continue
		}
		// ponytail: age guard instead of cross-window bookkeeping — a session
		// has no row until its first message, so a fresh chat in another window
		// (or this one) must not have its attachments swept out from under it.
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, id))
	}
}

// transcriptToModelMessages rebuilds the model's memory from what is on screen.
//
// A turn that failed is left out of it, both halves. The transcript keeps it —
// the user asked, the app could not answer, and that is what happened — but the
// model was never told the answer it gave, because it never gave one. Replaying
// the question with an empty reply underneath would teach it that it had ignored
// the user, and replaying the question alone would leave it dangling in a memory
// the very next message is about to ask again.
//
// This is also what RetryFailedTurn's promise rests on: it rebuilds from here
// before re-sending, so "remove the question the failed attempt left behind"
// costs nothing and cannot be forgotten at the call site.
func transcriptToModelMessages(messages []SessionMessage) []model.Message {
	out := make([]model.Message, 0, len(messages))
	for i, m := range messages {
		if failedPairAt(messages, i) {
			continue
		}
		role := model.RoleUser
		if m.Role == "agent" {
			role = model.RoleAssistant
		}
		out = append(out, model.Message{Role: role, Content: m.Text})
	}
	return out
}

// failedPairAt reports whether index i is part of a turn that never completed —
// the agent row carrying the error, or the question directly above it.
//
// The transcript is strictly user/agent pairs, so "the question of a failed
// turn" is always the row before its answer. Written as a lookup rather than a
// flag on the user row because the failure belongs to the turn, and storing it
// twice would create two columns that can disagree.
func failedPairAt(messages []SessionMessage, i int) bool {
	if messages[i].Role == "agent" {
		return messages[i].ErrorText != ""
	}
	next := i + 1
	return next < len(messages) && messages[next].Role == "agent" && messages[next].ErrorText != ""
}

// explainARecordedModelThatWillNotStart names the chat's own model when it is
// the reason the engine came up on the fallback.
//
// A conversation can record a model that stops existing under it. The owner's
// deepseek slot points at OpenRouter, so a chat that answered with a
// deepseek-native model before that move asks for a name the endpoint has never
// heard of; the same happens to a local runtime that is not running, a key that
// was revoked, a provider whose catalog moved on.
//
// What the window would say without this is the generic "เชื่อมต่อไม่ได้ ตอนนี้
// ใช้โมเดลในตัวของ Aetox แทน" — true, and useless, because it does not say that
// the model being asked for is one THIS CHAT remembers rather than one the user
// just picked. That is the sentence they need to act on, and the owner's call
// on 20 ส.ค. was explicitly for the loud version: silence is what made the last
// bug so hard to find.
//
// Nothing is auto-corrected. The chat keeps the model it recorded, the banner
// says what is answering instead, and choosing a different one is one click in
// the composer — a choice the user makes, not one made for them and written
// down (which is the mistake this same file made earlier the same day).
func (a *App) explainARecordedModelThatWillNotStart(conv *conversation, provider, modelName string) {
	if conv == nil || conv.modelErr == nil || modelName == "" {
		return
	}
	named := modelName
	if provider != "" {
		named = provider + " / " + modelName
	}
	conv.modelErr = fmt.Errorf(
		"แชทนี้เคยตอบด้วย %s ซึ่งตอนนี้เปิดไม่ได้ (%v) — เลือกโมเดลใหม่ให้แชทนี้ได้ที่ช่องพิมพ์",
		named, conv.modelErr)
}
