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
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
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
	// Icon is the mark the roster draws, always filled in: the profile's own
	// `icon:` when it names one, otherwise derived from what this agent
	// produces. Resolved here rather than in the page because the page would
	// have to know which tool means which mark, and that is a fact about the
	// engine's tools, not about a card.
	Icon string `json:"icon"`
}

// chairIcon is the face an agent wears when its profile does not choose one.
//
// Derived from the deliverable rather than from the name: what a person is
// looking for on this page is who makes the thing they need, and the writers
// are the one part of an office agent's tool list that differs between them
// (the ceiling gives everyone else the same set). An agent with no writer at
// all gets the generic mark — honest, and the case where the user is most
// likely to want to choose one themselves.
func chairIcon(p subagent.Profile, tools []string) string {
	if named := strings.TrimSpace(p.Icon); named != "" {
		return named
	}
	for _, tool := range tools {
		switch tool {
		case "doc_write":
			return "fileText"
		case "sheet_write":
			return "chartColumn"
		}
	}
	return "bot"
}

// ListChairs returns the office roster: every sub-agent profile that declares
// the office as its desk, in the order profiles list.
//
// Hiring is dropping one more .md in <DataRoot>/agents — there is no
// registration step here to forget, which is what makes "the company grows by
// adding chairs" true rather than aspirational.
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
		if child := subagent.FilterRegistry(a.cur().registry, p, ceiling); child != nil {
			c.Tools = child.Names()
		}
		c.Icon = chairIcon(p, c.Tools)
		if act, ok := used[p.Name]; ok {
			c.Jobs, c.LastUsed = act.count, act.last
		}
		out = append(out, c)
	}
	return out
}

// ChairStarters returns how one office agent opens a conversation — the
// question at the top of an empty chat with it, and the cards under it, read
// out of that agent's own folder (subagent/starters.go).
//
// Its own binding rather than a field on Chair, because the two are wanted at
// different moments and one of them is expensive: the roster is a page of
// job counts and tool lists, and an empty chat needs none of that. A worker
// with no STARTERS.md comes back empty, and the window falls back to the four
// it draws for any colleague.
//
// The language is a parameter and not a.cfg.UILocale on purpose. The window is
// the one that knows which language it is currently drawing; reading the
// engine's copy would race the moment the user switches, and would show one
// language's cards under the other language's chrome for exactly as long as it
// took the preference to be written.
func (a *App) ChairStarters(name, locale string) subagent.StarterSet {
	return subagent.Starters(name, locale)
}

// SaveChairStarters writes an agent's opening from the settings page.
//
// The same language parameter as the read above, and for the same reason: the
// window knows which language it is drawing, and STARTERS.md is the author's
// own while STARTERS.<lang>.md is a translation of it. Editing in Thai writes
// the base file; editing in English writes the English one beside it, which is
// what the reader resolves and therefore what the writer must produce.
//
// A set with no headline and no cards deletes the user's file — see
// subagent.SaveStarters for why that is the honest inverse rather than writing
// an empty one.
func (a *App) SaveChairStarters(name, locale string, set subagent.StarterSet) error {
	return subagent.SaveStarters(name, locale, set)
}

// ChairStartersFile is the filename the editor is currently writing, so the
// panel can say which of the two it means. A name, not a path: the folder is
// one click away behind the skills button and the whole path in a label is a
// line nobody reads.
func (a *App) ChairStartersFile(locale string) string {
	return config.AgentStartersName(locale)
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
	_ = eachRow(db, "chairs: activity", `
		SELECT agent, COUNT(*), MAX(time) FROM jobs
		WHERE agent <> '' GROUP BY agent`, nil,
		func(rows *sql.Rows) error {
			var name, last string
			var count int
			if err := rows.Scan(&name, &count, &last); err != nil {
				return err
			}
			out[name] = chairActivity{count: count, last: last}
			return nil
		})
	return out
}

// ReceivedJob is one job the office was handed, for the feed under the roster.
type ReceivedJob struct {
	ID        int64  `json:"id"`
	Chair     string `json:"chair"`
	SessionID string `json:"sessionId"` // the caller's session — what the file landed under
	Request   string `json:"request"`   // the brief, as the `task` call carried it
	// Brief is the one line a person reads: the `description` the caller wrote
	// for this job, pulled out of Request.
	//
	// Request is the tool call's arguments verbatim — a JSON object with the
	// brief inside it — because that is what the record has to keep. Putting it
	// on screen unread turned the feed into five rows of `{"agent":"doc",
	// "description":…,"prompt":…}`, which is the machine's copy shown to the
	// person it was never for. Derived here rather than in the page: the page
	// would have to learn the shape of a tool call to undo it.
	Brief     string `json:"brief"`
	Answer    string `json:"answer"`
	ToolSeq   string `json:"toolSeq,omitempty"`
	ToolCount int    `json:"toolCount"`
	Duration  int64  `json:"durationMs"`
	Outcome   string `json:"outcome"`
	Time      string `json:"time"`
}

// briefDescription pulls `"description": "…"` out of a tool call's arguments
// when the JSON itself will not parse. Stored requests are clamped to a maximum
// length (jobs.go), so the longest and most interesting ones arrive cut off in
// the middle — exactly the rows a feed is for, and the ones a strict decode
// would give up on.
var briefDescription = regexp.MustCompile(`"description"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// briefOf is the line a person reads, out of the arguments a machine sent.
//
// Three answers in order of how much they can be trusted: the decoded
// description, the same field scraped out of a truncated object, and — for a
// row that is not a JSON object at all, which is every job written before `task`
// carried arguments — the text itself. Never an empty string: a feed row with no
// line is worse than a raw one, because it says nothing at all happened.
func briefOf(request string) string {
	trimmed := strings.TrimSpace(request)
	if !strings.HasPrefix(trimmed, "{") {
		return trimmed
	}
	var args struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	if json.Unmarshal([]byte(trimmed), &args) == nil {
		if d := strings.TrimSpace(args.Description); d != "" {
			return d
		}
		if p := strings.TrimSpace(args.Prompt); p != "" {
			return p
		}
	}
	if m := briefDescription.FindStringSubmatch(trimmed); len(m) == 2 {
		var unquoted string
		if json.Unmarshal([]byte(`"`+m[1]+`"`), &unquoted) == nil && strings.TrimSpace(unquoted) != "" {
			return strings.TrimSpace(unquoted)
		}
	}
	return trimmed
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
	out, _ = queryAll(db, "office: received-jobs", `
		SELECT id, agent, session_id, request, answer, tool_seq, tool_count, duration_ms, outcome, time
		FROM jobs
		WHERE parent_ref <> '' AND agent IN (`+placeholders+`)
		ORDER BY id DESC LIMIT ?`, names,
		func(rows *sql.Rows) (ReceivedJob, error) {
			var j ReceivedJob
			err := rows.Scan(&j.ID, &j.Chair, &j.SessionID, &j.Request, &j.Answer,
				&j.ToolSeq, &j.ToolCount, &j.Duration, &j.Outcome, &j.Time)
			j.Brief = briefOf(j.Request)
			return j, err
		})
	return out
}
