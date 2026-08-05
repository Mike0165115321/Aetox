package main

// The bindings behind three of the five buttons (COMPANY.md §2): the desk
// picker, the office roster, and the work the office has received.
//
// Nothing here holds state. A desk is a file on disk, a chair is a file on
// disk, and the received-work feed is a query over `jobs` — the table §82
// already writes on every delegation. The office page was specified as "a
// roster plus a feed, no new state, no inbox" (§84), and this file is what
// that comes to in code: three readers over things that already exist.

import (
	"strings"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// ListModes returns every desk a session can be opened at — bundled first,
// then the user's, with a user file shadowing a bundled one of the same name
// rather than appearing twice.
func (a *App) ListModes() []mode.Mode {
	return mode.List()
}

// Chair is one seat in the office, as the roster shows it: the job description
// from the profile, plus the tools it *actually* gets and what it has been
// doing.
//
// Tools is computed rather than copied out of the file on purpose. A chair's
// frontmatter is a request; what it gets is that request intersected with the
// office's own manifest (§84), so a chair that asks for `shell` is listed
// without one. Showing the request would make the ceiling invisible exactly
// where a person is looking to check it.
type Chair struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
	Builtin     bool     `json:"builtin"`
	Overrides   bool     `json:"overrides,omitempty"`
	Path        string   `json:"path,omitempty"`
	// Jobs and LastUsed are what this chair has actually done, from `jobs`.
	// Zero means it has never been handed anything — the honest state for a
	// fresh install, and not something to dress up as activity.
	Jobs     int    `json:"jobs"`
	LastUsed string `json:"lastUsed,omitempty"` // RFC3339, "" when never
}

// ListChairs returns the office roster: every sub-agent profile that declares
// the office as its desk, in the order profiles list.
//
// Hiring is dropping one more .md in <DataRoot>/subagents with `desk:
// specialized` — there is no registration step here to forget, which is what
// makes "the company grows by adding chairs" true rather than aspirational.
func (a *App) ListChairs() []Chair {
	ceiling, _ := mode.Load(mode.Office)
	used := a.chairActivity()

	chairs := subagent.Chairs(mode.Office)
	out := make([]Chair, 0, len(chairs))
	for _, p := range chairs {
		c := Chair{
			Name:        p.Name,
			Description: p.Description,
			Builtin:     p.Builtin,
			Overrides:   p.Overrides,
			Path:        p.Path,
		}
		// The child's registry is the answer to "what can this chair do", so it
		// is what gets asked — rather than a second reading of the same rules
		// that could drift from the one the delegate actually runs on.
		if child := subagent.FilterRegistry(a.registry, p, ceiling); child != nil {
			c.Tools = child.Names()
		}
		if act, ok := used[p.Name]; ok {
			c.Jobs, c.LastUsed = act.count, act.last
		}
		out = append(out, c)
	}
	return out
}

type chairActivity struct {
	count int
	last  string
}

// chairActivity counts what each chair has been handed. One query for the
// whole roster rather than one per chair: the roster is drawn on every visit
// to the office page, and a per-row query is how a page that opens instantly
// with three chairs stops opening instantly with thirty.
func (a *App) chairActivity() map[string]chairActivity {
	out := map[string]chairActivity{}
	db, err := a.database()
	if err != nil {
		return out
	}
	rows, err := db.Query(`
		SELECT agent, COUNT(*), MAX(time) FROM jobs
		WHERE agent <> '' GROUP BY agent`)
	if err != nil {
		debuglog.Msg("chairs: activity query failed: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name, last string
		var count int
		if rows.Scan(&name, &count, &last) == nil {
			out[name] = chairActivity{count: count, last: last}
		}
	}
	return out
}

// ReceivedJob is one job the office was handed, for the feed under the roster.
type ReceivedJob struct {
	ID        int64  `json:"id"`
	Chair     string `json:"chair"`
	SessionID string `json:"sessionId"` // the caller's session — what the file landed under
	Request   string `json:"request"`   // the brief, as the `task` call carried it
	Answer    string `json:"answer"`
	ToolSeq   string `json:"toolSeq,omitempty"`
	ToolCount int    `json:"toolCount"`
	Duration  int64  `json:"durationMs"`
	Outcome   string `json:"outcome"`
	Time      string `json:"time"`
}

// ListReceivedJobs returns the work the office has taken in, newest first.
//
// A query, not a queue. Every delegation already writes a `jobs` row carrying
// who ran it and which call started it (`agent` + `parent_ref`, §82), so the
// feed is a reading of what happened — there is no inbox to keep in sync, and
// nothing here can disagree with the record the learning layer reads.
//
// Scoped to profiles that are chairs today: a job run by a profile that has
// since stopped being one is history about a delegate, not about the office.
func (a *App) ListReceivedJobs(limit int) []ReceivedJob {
	out := []ReceivedJob{}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	db, err := a.database()
	if err != nil {
		return out
	}
	chairs := subagent.Chairs(mode.Office)
	if len(chairs) == 0 {
		return out
	}
	names := make([]any, 0, len(chairs)+1)
	for _, c := range chairs {
		names = append(names, c.Name)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	names = append(names, limit)
	rows, err := db.Query(`
		SELECT id, agent, session_id, request, answer, tool_seq, tool_count, duration_ms, outcome, time
		FROM jobs
		WHERE parent_ref <> '' AND agent IN (`+placeholders+`)
		ORDER BY id DESC LIMIT ?`, names...)
	if err != nil {
		debuglog.Msg("office: received-jobs query failed: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var j ReceivedJob
		if rows.Scan(&j.ID, &j.Chair, &j.SessionID, &j.Request, &j.Answer,
			&j.ToolSeq, &j.ToolCount, &j.Duration, &j.Outcome, &j.Time) == nil {
			out = append(out, j)
		}
	}
	return out
}
