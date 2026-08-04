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
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/safety"
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
	ID          string `json:"id"`
	Title       string `json:"title"`
	UpdatedAt   string `json:"updatedAt"` // RFC3339
	Snippet     string `json:"snippet,omitempty"`
	ProjectKey  string `json:"projectKey,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
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
	_, _ = tx.Exec(`
		INSERT INTO sessions(id, project_key, title, created_at, updated_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at`,
		a.sessionID, projectKey(a.cfg.SandboxRoot), sessionTitleFrom(userMsg.Text), now, now)
	var agentID int64
	for _, m := range []SessionMessage{userMsg, agentMsg} {
		res, execErr := tx.Exec(`INSERT INTO messages(session_id, role, text, time, reasoning, think_secs, parts) VALUES(?,?,?,?,?,?,?)`,
			a.sessionID, m.Role, m.Text, m.Time, m.Reasoning, m.ThinkSecs, encodeParts(m.Parts))
		if execErr == nil {
			agentID, _ = res.LastInsertId() // the agent's row is the second and last
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
	if a.agent != nil {
		a.agent.ClearContext()
	}
}

// ListSessions returns this project's chat history, newest first.
func (a *App) ListSessions() []SessionMeta {
	out := []SessionMeta{}
	db, err := a.database()
	if err != nil {
		return out
	}
	rows, err := db.Query(`
		SELECT id, title, updated_at FROM sessions
		WHERE project_key = ? ORDER BY updated_at DESC LIMIT 200`,
		projectKey(a.cfg.SandboxRoot))
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMeta
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt) == nil {
			out = append(out, m)
		}
	}
	return out
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
		SELECT s.id, s.title, s.updated_at, MIN(f.snip)
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
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Snippet) == nil {
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
func (a *App) ListAllSessions() []SessionMeta {
	out := []SessionMeta{}
	db, err := a.database()
	if err != nil {
		return out
	}
	rows, err := db.Query(`
		SELECT s.id, s.title, s.updated_at, s.project_key, COALESCE(p.name, s.project_key)
		FROM sessions s LEFT JOIN projects p ON p.project_key = s.project_key
		ORDER BY s.updated_at DESC LIMIT 200`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMeta
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.ProjectKey, &m.ProjectName) == nil {
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
		SELECT s.id, s.title, s.updated_at, s.project_key, COALESCE(p.name, s.project_key), MIN(f.snip)
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
		if rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.ProjectKey, &m.ProjectName, &m.Snippet) == nil {
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
		if isUnfocusedKey(key) {
			a.focusNone()
			return a.LoadSession(id)
		}
		if rootPath == "" {
			return nil, fmt.Errorf("ไม่พบโปรเจกต์ของเซสชันนี้ (โฟลเดอร์อาจถูกย้ายหรือลบไปแล้ว)")
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
		id, projectKey(a.cfg.SandboxRoot))
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

// NewSession starts a blank session and returns its id.
func (a *App) NewSession() string {
	a.startNewSession()
	return a.sessionID
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
	// The session's attachments go with it. Session ids are our own timestamp
	// format, but this id came over the JS binding — refuse anything that could
	// step out of the attachments folder. Roots we can't see from here (another
	// project's) are left for sweepAttachments the next time that project opens.
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
