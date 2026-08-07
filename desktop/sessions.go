package main

// Chat-session persistence in the local SQLite store (see db.go), separated
// per project via project_key — the sidebar only ever lists the history of
// the project that's open. Turns are written incrementally as they happen, so
// nothing is lost on crash. Loading a session also restores the agent's
// context (RestoreHistory) so the AI remembers the conversation.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/subagent"
	"github.com/Mike0165115321/Aetox/internal/turn"
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
func projectKey(sandboxRoot string) string {
	root := strings.TrimSpace(sandboxRoot)
	sum := sha1.Sum([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Base(root) + "-" + hex.EncodeToString(sum[:4])
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

func newSessionID() string {
	return time.Now().Format("20060102-150405.000")
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
func (a *App) openTurn(userMsg SessionMessage) bool {
	db, err := a.database()
	if err != nil || a.sessionID == "" {
		return false
	}
	now := time.Now().Format(time.RFC3339)
	// The session row is created here now, which is also where the desk, the
	// agent and the project are recorded. Same rule as before: a session is
	// born with all three and the ON CONFLICT branch touches none of them.
	if _, err := db.Exec(`
		INSERT INTO sessions(id, project_key, title, created_at, updated_at, mode, agent, space)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at`,
		a.sessionID, projectKey(a.cfg.SandboxRoot), sessionTitleFrom(userMsg.Text), now, now,
		a.desk.DeskName(), a.chair, a.space); err != nil {
		return false
	}
	if _, err := db.Exec(
		`INSERT INTO messages(session_id, role, text, time, reasoning, think_secs, parts) VALUES(?,?,?,?,?,?,?)`,
		a.sessionID, userMsg.Role, userMsg.Text, userMsg.Time, userMsg.Reasoning, userMsg.ThinkSecs, encodeParts(userMsg.Parts)); err != nil {
		return false
	}
	a.turnOpened = true
	return true
}

func (a *App) appendTurn(userMsg, agentMsg SessionMessage) int64 {
	db, err := a.database()
	if err != nil || a.sessionID == "" {
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
		INSERT INTO sessions(id, project_key, title, created_at, updated_at, mode, agent, space)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at`,
		a.sessionID, projectKey(a.cfg.SandboxRoot), sessionTitleFrom(userMsg.Text), now, now, a.desk.DeskName(), a.chair, a.space)
	// The question, unless openTurn already wrote it when it was asked —
	// writing it twice would double every user message in the transcript. The
	// flag is the whole coupling between the two halves, and it is deliberately
	// on the App rather than passed in: only one turn is ever in flight.
	pending := []SessionMessage{userMsg, agentMsg}
	if a.turnOpened {
		pending = []SessionMessage{agentMsg}
	}
	a.turnOpened = false
	var agentID int64
	for _, m := range pending {
		res, execErr := tx.Exec(`INSERT INTO messages(session_id, role, text, time, reasoning, think_secs, parts) VALUES(?,?,?,?,?,?,?)`,
			a.sessionID, m.Role, m.Text, m.Time, m.Reasoning, m.ThinkSecs, encodeParts(m.Parts))
		if execErr == nil {
			agentID, _ = res.LastInsertId() // the agent's row is the last one written
		}
	}
	if tx.Commit() != nil {
		return 0
	}
	return agentID
}

// startNewSession begins a fresh transcript (and fresh agent memory). Nothing
// is written until the first message, so blank sessions never appear.
func (a *App) startNewSession() {
	a.sessionID = newSessionID()
	a.transcript = nil
	// No half-written turn carries into the next conversation: the flag means
	// "the question for the turn in flight is already stored", and a session
	// that has not started has no turn in flight.
	a.turnOpened = false
	// A new chat is in no project until something says otherwise. Left standing,
	// the last project a session was opened in would follow the user out of the
	// room: click ผู้ช่วย for a fresh chat and it would be filed under that
	// project, told about its files, and recorded on its row — a chat nobody
	// opened there. NewSessionInSpace sets this again after calling through
	// here, which is why clearing it first is safe as well as right.
	a.space = ""
	if a.agent != nil {
		a.agent.ClearContext()
	}
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
// '' in the column and so appear only here, which is right: they were held at
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
	args := []any{projectKey(a.cfg.SandboxRoot)}
	if desk = strings.TrimSpace(desk); desk != "" {
		query = `
		SELECT id, title, updated_at, mode, agent FROM sessions
		WHERE project_key = ? AND ` + heldOutsideAProject + ` AND mode = ? ORDER BY updated_at DESC LIMIT 200`
		args = append(args, desk)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMeta
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent) == nil {
			out = append(out, m)
		}
	}
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
	if id != "" && id == a.sessionID {
		return a.desk.DeskName()
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
	if id != "" && id == a.sessionID {
		return a.chair
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
	rows, err := db.Query(`
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
		match, projectKey(a.cfg.SandboxRoot))
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMeta
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.Snippet) == nil {
			out = append(out, m)
		}
	}
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
	rows, err := db.Query(`
		SELECT p.project_key, p.name, p.root_path, p.opened_at,
		       COALESCE((SELECT s.title FROM sessions s
		                 WHERE s.project_key = p.project_key
		                 ORDER BY s.updated_at DESC LIMIT 1), '')
		FROM projects p
		ORDER BY p.opened_at DESC LIMIT 50`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m ProjectMeta
		if rows.Scan(&m.Key, &m.Name, &m.RootPath, &m.OpenedAt, &m.Snippet) == nil {
			out = append(out, m)
		}
	}
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
	rows, err := db.Query(`
		SELECT s.id, s.title, s.updated_at, s.mode, s.agent, s.project_key, COALESCE(p.name, s.project_key)
		FROM sessions s LEFT JOIN projects p ON p.project_key = s.project_key
		`+clause+`
		ORDER BY s.updated_at DESC LIMIT 200`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMeta
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.ProjectKey, &m.ProjectName) == nil {
			out = append(out, m)
		}
	}
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
	rows, err := db.Query(`
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
		ORDER BY s.updated_at DESC LIMIT 50`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMeta
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.Space, &m.ProjectKey, &m.ProjectName, &m.Snippet) == nil {
			out = append(out, m)
		}
	}
	return out
}

func (a *App) ListAllSessions() []SessionMeta {
	out := []SessionMeta{}
	db, err := a.database()
	if err != nil {
		return out
	}
	rows, err := db.Query(`
		SELECT s.id, s.title, s.updated_at, s.mode, s.agent, s.project_key, COALESCE(p.name, s.project_key)
		FROM sessions s LEFT JOIN projects p ON p.project_key = s.project_key
		ORDER BY s.updated_at DESC LIMIT 200`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMeta
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.ProjectKey, &m.ProjectName) == nil {
			out = append(out, m)
		}
	}
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
	rows, err := db.Query(`
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
		ORDER BY s.updated_at DESC LIMIT 50`, match)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMeta
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent, &m.ProjectKey, &m.ProjectName, &m.Snippet) == nil {
			out = append(out, m)
		}
	}
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
	if key != projectKey(a.cfg.SandboxRoot) {
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
		a.touchProject(a.cfg.SandboxRoot)
	}
	return a.LoadSession(id)
}

// LoadSession switches to a stored session: the UI gets the transcript back,
// and the agent's context is rebuilt from it so the conversation continues
// with memory intact.
func (a *App) LoadSession(id string) ([]SessionMessage, error) {
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
	var desk, chair, space, key string
	if db.QueryRow(`SELECT mode, agent, space, project_key FROM sessions WHERE id = ?`, id).
		Scan(&desk, &chair, &space, &key) == nil {
		if err := a.setStation(desk, chair); err != nil {
			return nil, err
		}
		a.space = a.resolvedSpace(space)
		a.applyConfig(a.cfg)
	}
	if key == "" {
		key = projectKey(a.cfg.SandboxRoot)
	}
	// m.id comes back so a reopened session can still be rated: the thumbs
	// address a job by the bubble they sit under, and without the row id an
	// answer stops being ratable the moment the session is closed — which is
	// exactly when a user knows whether it was any good.
	rows, err := db.Query(`
		SELECT m.id, m.role, m.text, m.time, m.reasoning, m.think_secs, m.variants, m.variant_active, m.parts
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE m.session_id = ? AND s.project_key = ?
		ORDER BY m.id`,
		id, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := []SessionMessage{}
	for rows.Next() {
		var m SessionMessage
		var variants, parts string
		if rows.Scan(&m.ID, &m.Role, &m.Text, &m.Time, &m.Reasoning, &m.ThinkSecs, &variants, &m.Active, &parts) == nil {
			m.Variants = decodeVariants(variants)
			m.Parts = decodeParts(parts)
			m.Rating = a.TurnRating(m.ID)
			messages = append(messages, m)
		}
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("ไม่พบเซสชันนี้ในโปรเจกต์ปัจจุบัน")
	}

	a.sessionID = id
	a.transcript = messages
	if a.agent != nil {
		a.agent.ClearContext()
		a.agent.RestoreHistory(transcriptToModelMessages(messages))
	}
	return messages, nil
}

// NewSession starts a blank session at the desk the app is already at, and
// returns its id.
func (a *App) NewSession() string {
	a.startNewSession()
	return a.sessionID
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
	return a.sessionID, nil
}

// NewChairSession starts a blank session talking directly to one of the
// office's agents (§85), and returns its id. The desk is implied: a chair
// only exists in the office.
func (a *App) NewChairSession(chair string) (string, error) {
	a.startNewSession()
	if err := a.setStation(mode.Office, chair); err != nil {
		return "", err
	}
	return a.sessionID, nil
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
	if desk == a.desk.DeskName() && chair == a.chair {
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
		return fmt.Errorf("ไม่รู้จักโหมด %q — ไฟล์ของโหมดนี้อาจถูกลบไปแล้ว", desk)
	}
	a.desk, a.chair = m, chair
	a.applyConfig(a.cfg)
	rememberDesk(m.DeskName())
	return nil
}

// rememberDesk records where the window is, so the next launch opens there
// (COMPANY.md §2). Written here because setStation is the only writer of
// a.desk — anywhere else would be a second answer to "where am I".
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
	return a.sessionID
}

// DeleteSession permanently removes one session and its messages (any
// project; the messages_ad trigger cleans the FTS index). Deleting the
// session currently open also resets the transcript and agent memory.
func (a *App) DeleteSession(id string) error {
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
		for _, root := range []string{a.cfg.SandboxRoot, unfocusedRoot()} {
			if strings.TrimSpace(root) != "" {
				_ = os.RemoveAll(filepath.Join(root, attachmentsDir, id))
			}
		}
	}
	if a.sessionID == id {
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

func transcriptToModelMessages(messages []SessionMessage) []model.Message {
	out := make([]model.Message, 0, len(messages))
	for _, m := range messages {
		role := model.RoleUser
		if m.Role == "agent" {
			role = model.RoleAssistant
		}
		out = append(out, model.Message{Role: role, Content: m.Text})
	}
	return out
}
