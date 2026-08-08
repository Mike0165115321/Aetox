package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/learned"
)

// The reader over the learning floor.
//
// tool_runs and jobs record everything the agent does; nothing read them back.
// The floor answered "what happened" and no one was asking — 16 identical
// shell refusals sat in the store while the same mistake was made a 17th time
// (found 2026-08-07, and the fix arrived from the user noticing, not from the
// system that had already seen all 16).
//
// This pass asks the one question repeat data can answer without a model call:
// which failures keep happening for the same reason? A cluster of them becomes
// a memory proposal in pending_changes — the same door the agent's own
// proposals go through, so the approval UI, the audit trail and the "nothing
// writes itself" guarantee all apply unchanged. The summarizer differs from
// the agent in exactly one column: source says 'summarizer', because "the
// system noticed this pattern" and "the model decided this" must never be the
// same kind of row.
//
// Deterministic on purpose. A GROUP BY costs nothing and cannot hallucinate;
// what it cannot do is phrase a lesson better than the error text — but every
// refusal in this codebase is already written to carry its remedy, so quoting
// it IS the lesson. The model-driven half of the learning plan (summarize into
// better rules) can build on these rows later; it does not have to exist for
// the floor to start paying rent.

// summarizeMinRepeats is how many same-shaped failures make a pattern. Two is
// a coincidence a human would not want a card for; three is the same wall hit
// three separate times.
const summarizeMinRepeats = 3

// summarizeMaxProposals bounds one pass. The approval queue is a place the
// user reads, and ten cards at once reads as noise to dismiss, not lessons to
// consider — the strongest clusters go first and the rest wait for the next
// turn, which costs nothing because the pass runs after every one.
const summarizeMaxProposals = 3

type failureCluster struct {
	scope string
	tool  string
	head  string
	ids   []int64
	// lastArgs is the most recent failing call, for the reason column — the
	// card should show a concrete example, not only the abstraction.
	lastArgs string
}

// summarizeFailures drafts memory proposals from failures that keep happening
// for the same reason. Called after recordJobs on the turn's own goroutine;
// failures only log, same rule as every other writer on this path.
func (a *App) summarizeFailures() {
	if !learningEnabled() {
		return
	}
	db, err := a.database()
	if err != nil {
		return
	}

	rows, err := db.Query(
		`SELECT id, agent, tool, args, error FROM tool_runs
		  WHERE ok = 0 AND error <> '' ORDER BY id`)
	if err != nil {
		debuglog.Msg("summarize: reading failures: %v", err)
		return
	}
	clusters := map[string]*failureCluster{}
	for rows.Next() {
		var id int64
		var agent, tool, args, errText string
		if rows.Scan(&id, &agent, &tool, &args, &errText) != nil {
			continue
		}
		head := failureHead(errText)
		if head == "" {
			continue
		}
		key := agent + "\x00" + tool + "\x00" + strings.ToLower(head)
		c, ok := clusters[key]
		if !ok {
			c = &failureCluster{scope: agent, tool: tool, head: head}
			clusters[key] = c
		}
		c.ids = append(c.ids, id)
		c.lastArgs = args
	}
	rows.Close()

	repeated := make([]*failureCluster, 0, len(clusters))
	for _, c := range clusters {
		if len(c.ids) >= summarizeMinRepeats {
			repeated = append(repeated, c)
		}
	}
	// Biggest cluster first; the key breaks ties so two passes over the same
	// store propose in the same order.
	sort.Slice(repeated, func(i, j int) bool {
		if len(repeated[i].ids) != len(repeated[j].ids) {
			return len(repeated[i].ids) > len(repeated[j].ids)
		}
		return repeated[i].head < repeated[j].head
	})

	proposed := 0
	for _, c := range repeated {
		if proposed >= summarizeMaxProposals {
			break
		}
		if a.proposeFailureLesson(c) {
			proposed++
		}
	}
	if proposed > 0 {
		a.emitLearningChanged()
	}
}

// proposeFailureLesson queues one cluster as a pending memory line. Reports
// whether a new row was actually written — a duplicate is not a proposal.
func (a *App) proposeFailureLesson(c *failureCluster) bool {
	db, err := a.database()
	if err != nil {
		return false
	}
	target, err := learned.FileFor(c.scope)
	if err != nil {
		// A scope that no longer resolves (a deleted delegate) has nowhere for
		// the lesson to land; its history stays queryable, just not proposable.
		return false
	}

	// The body is the line the model would carry, and it is deliberately
	// stable: no counts, no timestamps. Stability is what makes the dedup
	// below hold across passes — a body that grew with the cluster would be
	// proposed again on every turn forever, which is the exact loop the kept
	// rejected rows exist to prevent.
	body := fmt.Sprintf("เครื่องมือ %s เคยล้มซ้ำ ๆ ด้วยเหตุเดียวกัน: \"%s\" — เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่ครั้งแรก", c.tool, c.head)

	// Any earlier decision about this exact lesson stands. Pending: it is
	// already on the card. Approved: it is already in the file. Rejected: the
	// user said no, and a system that asks again every turn is not proposing,
	// it is nagging.
	var existing int64
	err = db.QueryRow(
		`SELECT id FROM pending_changes WHERE kind = 'memory' AND scope = ? AND body = ? LIMIT 1`,
		c.scope, body).Scan(&existing)
	if err == nil {
		return false
	}

	if learned.Full(c.scope, len(body)) {
		// Nothing can apply this until the user makes room; a card that cannot
		// be approved is a bug report about the queue, not a lesson.
		return false
	}

	reason := fmt.Sprintf("เกิด %d ครั้ง", len(c.ids))
	if example := exampleArgs(c.lastArgs); example != "" {
		reason += " — ตัวอย่างล่าสุดที่ล้ม: " + example
	}

	_, err = db.Exec(
		`INSERT INTO pending_changes(kind, scope, target, op, before, body, reason, evidence, source, state, created_at)
		 VALUES('memory', ?, ?, 'add', '', ?, ?, ?, 'summarizer', ?, ?)`,
		c.scope, target, body, reason, evidenceFor(c.ids), statePending,
		time.Now().Format(time.RFC3339))
	if err != nil {
		debuglog.Msg("summarize: proposing failed: %v", err)
		return false
	}
	return true
}

// evidenceFor names the rows a proposal was drawn from, capped — a cluster of
// two hundred failures is not better evidenced for listing all two hundred.
func evidenceFor(ids []int64) string {
	const maxListed = 20
	listed := ids
	if len(listed) > maxListed {
		listed = listed[:maxListed]
	}
	parts := make([]string, len(listed))
	for i, id := range listed {
		parts[i] = fmt.Sprint(id)
	}
	out := "tool_runs:" + strings.Join(parts, ",")
	if extra := len(ids) - maxListed; extra > 0 {
		out += fmt.Sprintf(" +%d", extra)
	}
	return out
}

// failureHead reduces an error to the part that repeats. The variable text —
// the path in parentheses, the filename in quotes — is what distinguishes one
// occurrence from the next, so it is exactly the part that must go for the
// occurrences to land in one cluster.
func failureHead(raw string) string {
	line := raw
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = dropBracketed(line)
	line = dropQuoted(line, '"')
	line = dropQuoted(line, '\'')
	line = strings.Join(strings.Fields(line), " ")
	if r := []rune(line); len(r) > 160 {
		line = string(r[:160])
	}
	return strings.Trim(strings.TrimSpace(line), " :,-—")
}

// dropBracketed removes (...) spans, nested included.
func dropBracketed(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dropQuoted removes text between pairs of quote. An unpaired quote keeps its
// tail: cutting to the end would delete the very text that repeats.
func dropQuoted(s string, quote rune) string {
	var b strings.Builder
	var inQuote bool
	var pending strings.Builder
	for _, r := range s {
		switch {
		case r == quote && !inQuote:
			inQuote = true
			pending.Reset()
		case r == quote && inQuote:
			inQuote = false
		case inQuote:
			pending.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	if inQuote {
		b.WriteRune(quote)
		b.WriteString(pending.String())
	}
	return b.String()
}

// exampleArgs renders one failing call small enough for a card.
func exampleArgs(args string) string {
	args = strings.Join(strings.Fields(args), " ")
	if r := []rune(args); len(r) > 120 {
		args = string(r[:120]) + "…"
	}
	return args
}
