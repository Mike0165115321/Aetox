package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/learned"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The approval door. Everything the agent proposes to learn stops here and
// does nothing until a human says yes.
//
// The rows are not deleted when a proposal is decided — approving sets a state
// and a timestamp. That is the whole audit trail: for any line sitting in a
// memory file there is a row saying when it was proposed, what reasoning was
// offered, what it replaced, and when it was let through. Without it "the
// agent learned this" is indistinguishable from "the agent changed itself",
// and the second one is the thing nobody should have to take on trust.
//
// `before` is kept for the same reason it is shown: it is what an undo would
// need. Nothing undoes an approval today — the memory files are plain
// markdown the user can edit — but a queue that threw away the previous value
// could never grow one.

// PendingChange is one proposal as the review UI receives it.
type PendingChange struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	Target    string `json:"target"`
	Op        string `json:"op"`
	Before    string `json:"before"`
	Body      string `json:"body"`
	Reason    string `json:"reason"`
	Evidence  string `json:"evidence"`
	Source    string `json:"source"`
	State     string `json:"state"`
	CreatedAt string `json:"createdAt"`
	DecidedAt string `json:"decidedAt"`
}

const (
	statePending  = "pending"
	stateApproved = "approved"
	stateRejected = "rejected"
	// stateReported is an issue the user carried out the door. Deliberately not
	// "approved": approving applies a change, and reporting applies nothing —
	// the button opens a prefilled form in the user's own browser and this app
	// writes nothing anywhere. The row leaves the waiting list and stays as the
	// record that this cluster was already sent.
	stateReported = "reported"
)

// The two questions this table holds, which are not the same question
// (docs/architecture/system-problems-vs-learning-2026-08-18.md).
//
// kindMemory asks "จำไว้ไหม", and yes writes a line into a .md file that shapes
// every later turn. kindIssue asks "แจ้งนักพัฒนาไหม", and nothing it does
// touches memory at all — a repeated failure is a fact about Aetox or about
// this machine, never a lesson about the user.
//
// They share a table because they need the same three properties, all of which
// were built here first: rows are never deleted, the body text is the dedup
// key, and a thing turned down once never comes back. What they must never
// share is the queue the user reads, which is why every query below says which
// kind it means.
const (
	kindMemory = "memory"
	kindIssue  = "issue"
)

// appProposer adapts the App to learned.Proposer. The indirection exists so
// the method Wails binds to the frontend is not the same one a tool calls:
// proposing is something the agent does, and the UI's half of this is approve
// and reject.
type appProposer struct{ app *App }

func (p appProposer) Propose(pr learned.Proposal) (learned.Result, error) {
	return p.app.proposeLearned(pr)
}

func (a *App) proposeLearned(p learned.Proposal) (learned.Result, error) {
	if !learningEnabled() {
		return learned.Result{}, fmt.Errorf("learning is switched off in settings")
	}
	db, err := a.database()
	if err != nil {
		return learned.Result{}, err
	}

	target := ""
	if path, err := learned.FileFor(p.Scope); err == nil {
		target = path
	}

	// An identical proposal already waiting is answered as a duplicate rather
	// than queued again. The agent cannot see the queue — approved memory
	// reaches it only at the next session — so a second attempt in the same
	// conversation is the expected behaviour of a model that does not know it
	// already asked, not a mistake worth an error.
	var existing int64
	err = db.QueryRow(
		`SELECT id FROM pending_changes
		  WHERE state = ? AND kind = ? AND scope = ? AND op = ? AND body = ? AND before = ?
		  LIMIT 1`,
		statePending, p.Kind, p.Scope, p.Op, p.Body, p.Before).Scan(&existing)
	if err == nil {
		return learned.Result{ID: existing, Duplicate: true}, nil
	}
	if err != sql.ErrNoRows {
		return learned.Result{}, err
	}

	res, err := db.Exec(
		`INSERT INTO pending_changes(kind, scope, target, op, before, body, reason, evidence, source, state, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		p.Kind, p.Scope, target, p.Op, p.Before, p.Body, p.Reason,
		"session:"+a.sessionID, "agent", statePending, time.Now().Format(time.RFC3339))
	if err != nil {
		return learned.Result{}, err
	}
	id, _ := res.LastInsertId()
	a.emitLearningChanged()
	return learned.Result{ID: id}, nil
}

// ListPendingChanges returns what is waiting for a decision, oldest first —
// the order they were learned in is the order they make sense read in.
//
// Lessons only. Before the split this returned everything pending, and what
// arrived was mostly the summarizer's failure clusters: seventeen of the
// twenty-two cards this queue had ever held were complaints about a tool, and
// one of the seventeen was a usable lesson.
func (a *App) ListPendingChanges() []PendingChange {
	return a.queryChanges(`WHERE state = ? AND kind = ? ORDER BY id`, statePending, kindMemory)
}

// ListDecidedChanges is the record of what was let through and what was turned
// down, newest first. This is where "why does it think that?" gets answered.
func (a *App) ListDecidedChanges(limit int) []PendingChange {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return a.queryChanges(
		`WHERE state <> ? AND kind = ? ORDER BY id DESC LIMIT `+fmt.Sprint(limit),
		statePending, kindMemory)
}

// ListSystemIssues is the other queue: things that failed the same way more
// than once, waiting for the user to decide whether the developer should hear
// about them.
//
// Newest first, the opposite of the lessons above, because these are read for
// a different reason. A lesson is read in the order it was learned; a problem
// is read to find out what is breaking now.
func (a *App) ListSystemIssues() []PendingChange {
	return a.queryChanges(`WHERE state = ? AND kind = ? ORDER BY id DESC`, statePending, kindIssue)
}

// ListDecidedIssues is what was already reported or waved off, newest first.
//
// Without it the page lied by omission. Nothing is ever deleted here — a
// decided row keeps its state and its timestamp, which is the whole audit
// trail — but a list that only draws what is pending makes a row the user just
// decided look destroyed, and "where did it go?" was the first thing asked of
// the shipped page.
func (a *App) ListDecidedIssues(limit int) []PendingChange {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return a.queryChanges(
		`WHERE state <> ? AND kind = ? ORDER BY id DESC LIMIT `+fmt.Sprint(limit),
		statePending, kindIssue)
}

// PendingChangeByID is one proposal, whatever state it is in — the card the
// chat draws under the answer that proposed it asks for exactly this.
//
// Deliberately not restricted to pending rows: the card has to be able to say
// "จำไว้แล้ว" as readily as it offers the two buttons, and a proposal decided
// on the Settings page an hour ago must not still be asking here. A row that no
// longer exists comes back as the zero value, and the card draws nothing.
func (a *App) PendingChangeByID(id int64) PendingChange {
	found := a.queryChanges(`WHERE id = ?`, id)
	if len(found) == 0 {
		return PendingChange{}
	}
	return found[0]
}

// PendingLearnedCount is the badge. Cheap enough to call on every render.
//
// The kind filter is what makes the badge true. Counting every pending row put
// a number on the gear that was three-quarters failure clusters — a mark that
// says "there is something here only you can decide" and is wrong three times
// in four is a mark the user stops believing, and the five real proposals
// underneath it go unread with the rest.
func (a *App) PendingLearnedCount() int {
	return a.countPending(kindMemory)
}

// PendingIssueCount is the same mark for the problems room. It lights the row
// in the settings nav and nothing else: a repeated failure is worth finding
// when the user goes looking, and is not worth interrupting them for.
func (a *App) PendingIssueCount() int {
	return a.countPending(kindIssue)
}

func (a *App) countPending(kind string) int {
	db, err := a.database()
	if err != nil {
		return 0
	}
	var n int
	if db.QueryRow(
		`SELECT COUNT(*) FROM pending_changes WHERE state = ? AND kind = ?`,
		statePending, kind).Scan(&n) != nil {
		return 0
	}
	return n
}

// ApprovePendingChange applies one proposal and marks it approved.
//
// The write happens first and the state moves only if it succeeded: a row
// marked approved whose change never landed would be a lie in the one place
// that exists to be trusted.
func (a *App) ApprovePendingChange(id int64) error {
	db, err := a.database()
	if err != nil {
		return err
	}
	var c PendingChange
	err = db.QueryRow(
		`SELECT id, kind, scope, op, before, body, state FROM pending_changes WHERE id = ?`, id).
		Scan(&c.ID, &c.Kind, &c.Scope, &c.Op, &c.Before, &c.Body, &c.State)
	if err != nil {
		return fmt.Errorf("no such proposal")
	}
	if c.State != statePending {
		return fmt.Errorf("this was already decided")
	}

	// An issue has no "apply" and must never grow one: the default branch is
	// what guarantees a failure cluster can never be written into a memory file,
	// whatever a future caller passes in.
	switch c.Kind {
	case kindMemory:
		if err := learned.Apply(c.Scope, c.Op, c.Before, c.Body); err != nil {
			return err
		}
	default:
		return fmt.Errorf("this build does not know how to apply a %q change", c.Kind)
	}

	if _, err := db.Exec(
		`UPDATE pending_changes SET state = ?, decided_at = ? WHERE id = ?`,
		stateApproved, time.Now().Format(time.RFC3339), id); err != nil {
		// The change is on disk; only the bookkeeping failed. Say so rather
		// than reporting a failure the user would try to fix by approving again.
		debuglog.Msg("pending_changes: approved %d but could not record it: %v", id, err)
	}
	a.emitLearningChanged()
	return nil
}

// RejectPendingChange turns one down. The row stays: what the agent proposed
// and was refused is the more interesting half of the record, and it is what a
// later pass would need to avoid proposing the same thing forever.
//
// Both queues end here. "ไม่เอา" on a lesson and "ไม่เป็นไร" on a problem are
// the same act — the user read it and said no — and one function keeps them
// one act in the record.
func (a *App) RejectPendingChange(id int64) error {
	db, err := a.database()
	if err != nil {
		return err
	}
	res, err := db.Exec(
		`UPDATE pending_changes SET state = ?, decided_at = ? WHERE id = ? AND state = ?`,
		stateRejected, time.Now().Format(time.RFC3339), id, statePending)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("this was already decided")
	}
	a.emitLearningChanged()
	return nil
}

// MarkIssueReported records that the user took one problem to the developer.
//
// Called after the browser has been handed the prefilled form, never before:
// the app's part is finished the moment the form opens, and what happens on
// GitHub is the user's — they may read the whole thing and close the tab. So
// this records "carried out the door", which is true either way, and not "a
// bug was filed", which this side cannot know.
//
// Only issues, and only pending ones. A lesson has no door to be carried out
// of, and the state guard is what stops a double click filing the same row
// twice.
func (a *App) MarkIssueReported(id int64) error {
	db, err := a.database()
	if err != nil {
		return err
	}
	res, err := db.Exec(
		`UPDATE pending_changes SET state = ?, decided_at = ?
		  WHERE id = ? AND state = ? AND kind = ?`,
		stateReported, time.Now().Format(time.RFC3339), id, statePending, kindIssue)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("this was already decided")
	}
	a.emitLearningChanged()
	return nil
}

func (a *App) queryChanges(where string, args ...any) []PendingChange {
	out := []PendingChange{}
	db, err := a.database()
	if err != nil {
		return out
	}
	out, _ = queryAll(db, "pending_changes", `
		SELECT id, kind, scope, target, op, before, body, reason, evidence, source, state, created_at, decided_at
		   FROM pending_changes `+where, args,
		func(rows *sql.Rows) (PendingChange, error) {
			var c PendingChange
			err := rows.Scan(&c.ID, &c.Kind, &c.Scope, &c.Target, &c.Op, &c.Before, &c.Body,
				&c.Reason, &c.Evidence, &c.Source, &c.State, &c.CreatedAt, &c.DecidedAt)
			return c, err
		})
	return out
}

// emitLearningChanged is the one signal for both queues: any row inserted or
// decided, of either kind, moves something a surface is drawing. The payload
// stays the lessons count because that is the number the gear wears; the
// problems room reads its own count when the signal arrives, rather than
// growing this into an object two listeners would have to agree about.
func (a *App) emitLearningChanged() {
	payload := a.PendingLearnedCount()
	if a.emit != nil {
		a.emit("learning:changed", payload)
		return
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "learning:changed", payload)
	}
}

// LearnedMemory returns what one scope currently holds, for the settings page
// that shows the user everything shaping the model. Empty scope is the main
// agent.
func (a *App) LearnedMemory(scope string) string {
	return learned.Read(strings.TrimSpace(scope))
}

// LearnedEntries is LearnedMemory as the list it actually is, one string per
// remembered line, so the settings page can put each on its own row.
//
// Never nil: Wails marshals a nil slice as JSON null, and the page maps over
// what comes back.
func (a *App) LearnedEntries(scope string) []string {
	out := learned.Entries(strings.TrimSpace(scope))
	if out == nil {
		return []string{}
	}
	return out
}

// LearnedScopes is which memories currently hold anything, so the review page
// can show all of them instead of only the main agent's. Empty string in the
// list is the main agent, spelled the way the rest of the system spells it.
//
// Never nil, for the same reason LearnedEntries is not.
func (a *App) LearnedScopes() []string {
	out := learned.Scopes()
	if out == nil {
		return []string{}
	}
	return out
}

// SaveLearnedEntry rewrites one remembered line in place; an empty text removes
// it. Addressed by the row's position, which is what the user is looking at —
// see learned.EditEntry for why not by substring.
//
// The file is the user's, and this is the user editing it by hand through a
// window instead of in an editor. So there is no approval step here and no
// proposal recorded: the approval queue exists to gate what the *agent* wants
// to write, and asking someone to approve their own edit would be asking them
// to approve themselves.
func (a *App) SaveLearnedEntry(scope string, index int, text string) error {
	if err := learned.EditEntry(strings.TrimSpace(scope), index, text); err != nil {
		return err
	}
	// Same event the approval path emits: anything showing memory is looking at
	// a file that just changed, and one signal beats each surface polling.
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "learning:changed", nil)
	}
	return nil
}

// OpenMemoryFolder reveals the memory directory in the file manager. The point
// of keeping this as plain markdown is that the user can take it elsewhere;
// that is only true if they can find it.
func (a *App) OpenMemoryFolder() error {
	dir, err := learned.Dir()
	if err != nil {
		return err
	}
	// Created on demand: the folder does not exist until the first approval, and
	// "open" failing on a fresh install would read as the feature being broken.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return openInFileManager(dir)
}

// learningEnabled is the single switch. Off means the agent stops recording
// jobs and cannot queue anything — not that proposals pile up unseen.
//
// Read from preferences on each use rather than cached: the switch has to take
// effect on the next turn, and a cached copy would need an invalidation path
// that exists for nothing else.
func learningEnabled() bool {
	pref, ok, _ := config.LoadModelPreference()
	if !ok {
		return true
	}
	return !pref.LearningDisabled
}

// SetLearningEnabled persists the switch.
func (a *App) SetLearningEnabled(on bool) error {
	pref, _, err := config.LoadModelPreference()
	if err != nil {
		return err
	}
	pref.LearningDisabled = !on
	return config.SaveModelPreference(pref)
}

// LearningEnabled reports the switch for the settings page.
func (a *App) LearningEnabled() bool { return learningEnabled() }
