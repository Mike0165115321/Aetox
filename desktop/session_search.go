package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// sessionSearchSkill lets the model search past conversations and past tool
// work by content — the same FTS5 indexes the history panel uses, exposed as
// a tool. Before this, everything outside the current context window was
// effectively forgotten: the user saying "แบบคราวที่แล้ว" got a clarifying
// question about work that is sitting, searchable, in aetox.db. A lookup here
// costs no model tokens beyond the results themselves.
//
// Desktop-only by nature: the history database belongs to the desktop app.
type sessionSearchSkill struct{ app *App }

func (*sessionSearchSkill) Name() string { return "session_search" }

func (*sessionSearchSkill) Description() string {
	return "ค้นประวัติแชทและงานที่เคยทำในเซสชันก่อน ๆ ด้วยคำค้น (ไทย/อังกฤษ)"
}

func (*sessionSearchSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Text to find, at least 3 characters. Thai and English both work; substrings match.",
			},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "session_search",
			Description: "Search past sessions on this machine: chat history and completed tool work (full-text, " +
				"Thai and English). Use when the user refers to earlier work — \"เหมือนคราวที่แล้ว\", \"the file " +
				"we made last week\" — instead of asking them to repeat what the machine already remembers. " +
				"Results show the session, date and matching text.",
			Parameters: payload,
		},
	}
}

func (s *sessionSearchSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	query, _ := input["args"].(string)
	return s.ExecuteTool(ctx, map[string]any{"query": query})
}

// Caps on what one search returns to the model. History can hold thousands of
// matches for a common word; the model needs the newest few, clamped, not a
// transcript dump that costs more context than it saves.
const (
	maxChatHits    = 8
	maxWorkHits    = 6
	maxHitChars    = 300
	minQueryLength = 3 // trigram FTS cannot match anything shorter
)

func (s *sessionSearchSkill) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	start := time.Now()
	fail := func(err error) (skill.Output, error) {
		return skill.Output{
			Name:       "session_search",
			Content:    err.Error(),
			Command:    "session_search",
			Success:    false,
			Stderr:     err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		}, err
	}

	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < minQueryLength {
		return fail(fmt.Errorf("query must be at least %d characters — the index matches substrings, so a distinctive fragment beats a single word", minQueryLength))
	}

	db, err := s.app.database()
	if err != nil {
		return fail(fmt.Errorf("history database unavailable: %w", err))
	}

	// The FTS query is the user text as one quoted phrase — same idiom as the
	// history panel (SearchAllSessions): no MATCH operators, no syntax errors
	// from an innocent apostrophe.
	match := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`

	var b strings.Builder
	chatHits := s.searchChat(db, match, &b)
	workHits := s.searchWork(db, match, &b)

	if chatHits == 0 && workHits == 0 {
		return skill.Output{
			Name:       "session_search",
			Content:    fmt.Sprintf("No history matches %q. Shorter or different words may match — the search is substring-based.", query),
			Command:    "session_search " + query,
			Success:    true,
			DurationMs: time.Since(start).Milliseconds(),
		}, nil
	}

	return skill.Output{
		Name:       "session_search",
		Content:    strings.TrimRight(b.String(), "\n"),
		RawOutput:  b.String(),
		Command:    "session_search " + query,
		Success:    true,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// searchChat appends matching chat messages, newest first, and reports how
// many it wrote. Query failures write nothing — a missing table (ancient
// database) must degrade to "no chat matches", not break the tool.
func (s *sessionSearchSkill) searchChat(db *sql.DB, match string, b *strings.Builder) int {
	n := 0
	_ = eachRow(db, "session_search: chat", `
		SELECT m.role, m.text, m.time, s.id, COALESCE(s.title, '')
		FROM messages_fts f
		JOIN messages m ON m.id = f.rowid
		JOIN sessions s ON s.id = m.session_id
		WHERE messages_fts MATCH ?
		ORDER BY m.id DESC LIMIT ?`, []any{match, maxChatHits},
		func(rows *sql.Rows) error {
			var role, text, when, sessionID, title string
			if err := rows.Scan(&role, &text, &when, &sessionID, &title); err != nil {
				return err
			}
			if n == 0 {
				b.WriteString("## Chat history\n")
			}
			n++
			fmt.Fprintf(b, "[%s] %s%s — %s: %s\n",
				datePart(when), sessionLabel(title), s.currentMark(sessionID), role, clampHit(text))
			return nil
		})
	if n > 0 {
		b.WriteString("\n")
	}
	return n
}

// searchWork appends matching tool runs — what was actually done, not said.
func (s *sessionSearchSkill) searchWork(db *sql.DB, match string, b *strings.Builder) int {
	n := 0
	_ = eachRow(db, "session_search: work", `
		SELECT r.tool, r.args, r.output, r.ok, r.time, r.session_id, COALESCE(s.title, '')
		FROM tool_runs_fts f
		JOIN tool_runs r ON r.id = f.rowid
		LEFT JOIN sessions s ON s.id = r.session_id
		WHERE tool_runs_fts MATCH ?
		ORDER BY r.id DESC LIMIT ?`, []any{match, maxWorkHits},
		func(rows *sql.Rows) error {
			var tool, toolArgs, output, when, sessionID, title string
			var ok int
			if err := rows.Scan(&tool, &toolArgs, &output, &ok, &when, &sessionID, &title); err != nil {
				return err
			}
			if n == 0 {
				b.WriteString("## Tool work\n")
			}
			n++
			status := "ok"
			if ok == 0 {
				status = "failed"
			}
			fmt.Fprintf(b, "[%s] %s%s — %s (%s) args: %s",
				datePart(when), sessionLabel(title), s.currentMark(sessionID), tool, status, clampHit(toolArgs))
			if output != "" {
				fmt.Fprintf(b, " → %s", clampHit(output))
			}
			b.WriteString("\n")
			return nil
		})
	if n > 0 {
		b.WriteString("\n")
	}
	return n
}

func (s *sessionSearchSkill) currentMark(sessionID string) string {
	if s.app != nil && sessionID != "" && sessionID == s.app.cur().id {
		return " (this session)"
	}
	return ""
}

func sessionLabel(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "(untitled)"
	}
	return `"` + clampTo(title, 60) + `"`
}

func datePart(rfc3339 string) string {
	if len(rfc3339) >= 10 {
		return rfc3339[:10]
	}
	return rfc3339
}

func clampHit(s string) string {
	return clampTo(strings.Join(strings.Fields(s), " "), maxHitChars)
}

// clampTo cuts on a rune boundary — half a Thai character is worse than one
// character less (same rule as clampStored in tool_runs.go).
func clampTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
