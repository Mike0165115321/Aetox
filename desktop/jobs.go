package main

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
)

// A job is one unit of work with a result somebody could judge: a chat turn,
// or one `task` handed to a delegate. `tool_runs` already records every call;
// what it cannot say is where one piece of work stopped and the next began, or
// whether any of it was any good.
//
// Both gaps matter for the same reason. "This job has happened before" is the
// question repeat-detection asks, and it is only answerable against a unit —
// a single `read` recurs constantly and means nothing. "This way of doing it
// works" is the question every later layer asks, and `tool_runs.ok` cannot
// answer it: a confidently wrong OCR returns ok=1.
//
// Nothing here scores anything on its own. outcome starts `unknown` and only
// moves when a human says so (the thumbs under a reply) or does something that
// means it (pressing "answer again"). Inferring a score from tool errors was
// considered and dropped: a turn can fail every tool and still end with the
// right answer, and a learning layer fed guesses learns the guesses.
const (
	maxStoredRequest = 2 << 10
	maxStoredAnswer  = 4 << 10
	// maxStoredToolSeq bounds the shape string. A turn that called 400 tools
	// is not more identifiable for having all 400 named, and the column is
	// indexed — the head is the shape.
	maxStoredToolSeq = 1 << 10
)

// Outcome values. Deliberately three, not a number: v1 asks a human a yes/no
// question, and a scale would invite averaging ratings nobody actually gave.
const (
	outcomeUnknown = "unknown"
	outcomeGood    = "good"
	outcomeBad     = "bad"
)

// toolRunRow is one row of the window of `tool_runs` a turn just wrote.
type toolRunRow struct {
	id        int64
	ref       string
	parentRef string
	agent     string
	tool      string
	args      string
	output    string
	ok        bool
	duration  int64
}

// maxToolRunID is the marker taken before a turn runs, so the rows it writes
// can be identified afterwards without a second capture path alongside
// recordToolRun. Reading the store back is what makes a delegate's job rows
// free: the child's calls are already there, stamped with the `task` call that
// caused them.
func (a *App) maxToolRunID(conv *conversation) int64 {
	db, err := a.database()
	if err != nil {
		return 0
	}
	var max sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(id) FROM tool_runs WHERE session_id = ?`, conv.id).Scan(&max); err != nil {
		return 0
	}
	return max.Int64
}

// recordJobs writes the job rows for one finished turn: the main agent's, plus
// one for every delegate that ran inside it.
//
// Called after appendTurn, on the turn's own goroutine. Failures only log —
// same rule as recordToolRun: a chat turn must not break because the history
// could not be written.
func (a *App) recordJobs(conv *conversation, messageID int64, request, answer string, sinceToolRun int64, elapsed time.Duration) {
	if !learningEnabled() {
		return
	}
	// The turn's own session, the same one appendTurn wrote with — these rows
	// describe the same turn, and two readers of "which session" that can
	// disagree is the shape this change exists to end.
	sessionID := conv.id
	db, err := a.database()
	if err != nil || sessionID == "" {
		return
	}
	rows := a.toolRunsSince(conv, sinceToolRun)

	now := time.Now().Format(time.RFC3339)
	// The main agent's own calls are the ones with no parent. A `task` call is
	// one of them and stays in the sequence: "this turn delegated" is part of
	// the shape of the work, not noise to filter out.
	var mine []toolRunRow
	byParent := map[string][]toolRunRow{}
	for _, r := range rows {
		if r.parentRef == "" {
			mine = append(mine, r)
			continue
		}
		byParent[r.parentRef] = append(byParent[r.parentRef], r)
	}

	insertJob(db, jobInsert{
		sessionID: sessionID,
		messageID: messageID,
		// A direct chat's turns are the chair's own work (§85): scoped to the
		// chair so the office roster counts them and what gets learned from
		// them lands in the chair's file — and with no parent_ref, so the
		// received-work feed (which filters on it) correctly shows briefs
		// only, not conversations.
		agent:    a.cur().chair,
		request:  request,
		answer:   answer,
		runs:     mine,
		duration: elapsed.Milliseconds(),
		time:     now,
	})

	// A delegate's job is reconstructed rather than captured: its brief and its
	// result are the `task` call's own args and output, and its tool sequence is
	// every row stamped with that call's ref. Keeping this derived means there is
	// one recording path, not two that can disagree about what a job is.
	for _, parent := range mine {
		if parent.tool != "task" {
			continue
		}
		kids := byParent[parent.ref]
		if len(kids) == 0 && parent.output == "" {
			continue
		}
		insertJob(db, jobInsert{
			sessionID: sessionID,
			agent:     delegateName(parent, kids),
			parentRef: parent.ref,
			request:   parent.args,
			answer:    parent.output,
			runs:      kids,
			duration:  parent.duration,
			time:      now,
		})
	}

	// The turn's rows are in; now read the store back for patterns. Per turn
	// rather than per session because that is what the floor was priced for
	// (db.go: a GROUP BY, not a model call), and because a lesson that waits
	// for a restart arrives after the mistake has been repeated all day.
	a.summarizeFailures()

	// And the other reader of the same floor: the summarizer raises a problem
	// from a repeated crash, this drafts a skill fix from a repeated bad rating.
	// Cheap detect here, model call only in the background if something is
	// flagged — and only ever a proposal, never a write.
	a.maybeTuneSkills()
}

// delegateName is which sub-agent ran. The tool runs carry it (the relay in
// internal/subagent stamps every child run with the profile name); the `task`
// call's own arguments are the fallback for a delegation that called no tools
// at all, which is a real outcome and still a job worth scoring.
func delegateName(parent toolRunRow, kids []toolRunRow) string {
	for _, k := range kids {
		if k.agent != "" {
			return k.agent
		}
	}
	if name := jsonStringField(parent.args, "agent"); name != "" {
		return name
	}
	return "task"
}

// jsonStringField pulls one string field out of a stored tool-call argument
// blob. The blob is whatever the model sent, so anything unparseable, absent
// or of another type is simply "" — this is a best-effort label, never a
// decision, and a job row is worth writing with an imperfect one.
func jsonStringField(raw, key string) string {
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

type jobInsert struct {
	sessionID string
	messageID int64
	agent     string
	parentRef string
	request   string
	answer    string
	runs      []toolRunRow
	duration  int64
	time      string
}

func insertJob(db *sql.DB, j jobInsert) {
	seq, failed := toolShape(j.runs)
	request, _ := clampStored(j.request, maxStoredRequest)
	answer, _ := clampStored(j.answer, maxStoredAnswer)
	_, err := db.Exec(
		`INSERT INTO jobs(
			session_id, message_id, agent, parent_ref, request, answer,
			tool_seq, tool_count, failed_tools, duration_ms, outcome, outcome_source, time)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.sessionID, j.messageID, j.agent, j.parentRef, request, answer,
		seq, len(j.runs), failed, j.duration, outcomeUnknown, "", j.time,
	)
	if err != nil {
		debuglog.Msg("jobs: insert failed: %v", err)
	}
}

// toolShape renders the sequence of tools a job ran, and counts the ones that
// failed. The separator is ">" because the order is the point: read>ocr>write
// and write>ocr>read are different jobs that a set of names could not tell
// apart.
func toolShape(runs []toolRunRow) (string, int) {
	var b strings.Builder
	failed := 0
	for i, r := range runs {
		if !r.ok {
			failed++
		}
		if b.Len() >= maxStoredToolSeq {
			continue
		}
		if i > 0 {
			b.WriteString(">")
		}
		b.WriteString(r.tool)
	}
	seq, _ := clampStored(b.String(), maxStoredToolSeq)
	return seq, failed
}

func (a *App) toolRunsSince(conv *conversation, mark int64) []toolRunRow {
	db, err := a.database()
	if err != nil {
		return nil
	}
	var out []toolRunRow
	_ = eachRow(db, "jobs: tool_runs", `
		SELECT id, ref, parent_ref, agent, tool, args, output, ok, duration_ms
		   FROM tool_runs WHERE session_id = ? AND id > ? ORDER BY id`,
		[]any{conv.id, mark},
		func(rows *sql.Rows) error {
			var r toolRunRow
			var ok int
			if err := rows.Scan(&r.id, &r.ref, &r.parentRef, &r.agent, &r.tool, &r.args, &r.output, &ok, &r.duration); err != nil {
				return err
			}
			r.ok = ok != 0
			out = append(out, r)
			return nil
		})
	return out
}

// RateTurn records what the user thought of one reply. Bound to the frontend:
// the thumbs under a bubble address the job by the message they sit under,
// which is the only id the UI has and the only one it should need.
//
// An empty or unknown verdict clears the rating rather than being rejected —
// pressing the lit thumb again is how a user takes back a rating, and a store
// that could not represent "never mind" would keep a verdict they withdrew.
func (a *App) RateTurn(messageID int64, verdict string) {
	switch verdict {
	case outcomeGood, outcomeBad:
		a.setJobOutcome(messageID, verdict, "thumb")
	default:
		a.setJobOutcome(messageID, outcomeUnknown, "")
	}
}

// markTurnRedone is the rating nobody has to give. Asking for another answer
// says the first one missed, and it is the only negative signal that arrives
// without the user doing anything they would not have done anyway — which
// makes it the one most likely to actually accumulate.
//
// It never overwrites a verdict the user typed: a reply they marked good and
// then regenerated (to get a second good one) stays good.
func (a *App) markTurnRedone(messageID int64) {
	db, err := a.database()
	if err != nil || messageID == 0 {
		return
	}
	_, err = db.Exec(
		`UPDATE jobs SET outcome = ?, outcome_source = ?
		  WHERE message_id = ? AND outcome_source <> 'thumb'`,
		outcomeBad, "redo", messageID)
	if err != nil {
		debuglog.Msg("jobs: redo signal failed: %v", err)
	}
}

// newestJobFor is the attempt currently on screen. A bubble that was answered
// again holds more than one job row, and every one of them is worth keeping —
// but a rating is about the answer the user is looking at, which is the last.
const newestJobFor = `(SELECT id FROM jobs WHERE message_id = ? AND agent = '' ORDER BY id DESC LIMIT 1)`

func (a *App) setJobOutcome(messageID int64, outcome, source string) {
	db, err := a.database()
	if err != nil || messageID == 0 {
		return
	}
	if _, err := db.Exec(
		`UPDATE jobs SET outcome = ?, outcome_source = ? WHERE id = `+newestJobFor,
		outcome, source, messageID); err != nil {
		debuglog.Msg("jobs: rating failed: %v", err)
	}
}

// TurnRating reports the verdict already stored for a reply, so a reopened
// session shows the thumb the user pressed instead of a blank pair.
func (a *App) TurnRating(messageID int64) string {
	db, err := a.database()
	if err != nil || messageID == 0 {
		return outcomeUnknown
	}
	var outcome string
	if err := db.QueryRow(`SELECT outcome FROM jobs WHERE id = `+newestJobFor, messageID).
		Scan(&outcome); err != nil {
		return outcomeUnknown
	}
	return outcome
}

// lastAgentMessageID is the rowid of the reply currently at the bottom of the
// session — the one "answer again" replaces.
func (a *App) lastAgentMessageID(conv *conversation) int64 {
	sessionID := conv.id
	db, err := a.database()
	if err != nil || sessionID == "" {
		return 0
	}
	var id sql.NullInt64
	if db.QueryRow(
		`SELECT MAX(id) FROM messages WHERE session_id = ? AND role = 'agent'`,
		sessionID).Scan(&id) != nil {
		return 0
	}
	return id.Int64
}
