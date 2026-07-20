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
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// SessionMessage is one chat bubble, as the UI shows it.
type SessionMessage struct {
	Role string `json:"role"` // "user" | "agent"
	Text string `json:"text"`
	Time string `json:"time"`
}

// SessionMeta is one row in the sidebar's history list. Snippet is only set
// on search results.
type SessionMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"` // RFC3339
	Snippet   string `json:"snippet,omitempty"`
}

// projectKey isolates each project's history: readable base name + short hash
// of the full path (so two folders named "app" don't collide).
func projectKey(sandboxRoot string) string {
	root := strings.TrimSpace(sandboxRoot)
	sum := sha1.Sum([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Base(root) + "-" + hex.EncodeToString(sum[:4])
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
func (a *App) appendTurn(userMsg, agentMsg SessionMessage) {
	db, err := a.database()
	if err != nil || a.sessionID == "" {
		return
	}
	tx, err := db.Begin()
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Format(time.RFC3339)
	_, _ = tx.Exec(`
		INSERT INTO sessions(id, project_key, title, created_at, updated_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET updated_at = excluded.updated_at`,
		a.sessionID, projectKey(a.cfg.SandboxRoot), sessionTitleFrom(userMsg.Text), now, now)
	for _, m := range []SessionMessage{userMsg, agentMsg} {
		_, _ = tx.Exec(`INSERT INTO messages(session_id, role, text, time) VALUES(?,?,?,?)`,
			a.sessionID, m.Role, m.Text, m.Time)
	}
	_ = tx.Commit()
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
	rows, err := db.Query(`
		SELECT s.id, s.title, s.updated_at,
		       snippet(messages_fts, 0, '', '', '…', 10)
		FROM messages_fts
		JOIN messages m ON m.id = messages_fts.rowid
		JOIN sessions s ON s.id = m.session_id
		WHERE messages_fts MATCH ? AND s.project_key = ?
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

// LoadSession switches to a stored session: the UI gets the transcript back,
// and the agent's context is rebuilt from it so the conversation continues
// with memory intact.
func (a *App) LoadSession(id string) ([]SessionMessage, error) {
	db, err := a.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT m.role, m.text, m.time
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
		if rows.Scan(&m.Role, &m.Text, &m.Time) == nil {
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
