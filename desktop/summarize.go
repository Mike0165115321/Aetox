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
//
// That assumption held for the failures this codebase authors and for nothing
// else, which took a user noticing to find (2026-08-09). `exit status 1` is
// Go's spelling of an exit code — it reached the same column by the same path,
// carrying no remedy because nobody wrote it to carry one, and the reader had
// no way to tell it apart from a sentence that did. On a real machine five of
// the six clusters over the threshold were exit codes, and every one of them
// became a memory line instructing the agent to avoid a pattern nothing had
// named. The fix is upstream, at turn.ErrorFromProgram: the run now records
// where its failure came from, and this pass reads only the failures that are
// the tool's own. A nonzero exit is a program reporting a result — a test suite
// with failures, a grep with no match — and a tool that ran it correctly has
// nothing to be taught.
//
// The second time the assumption broke was the same shape again (2026-08-12).
// A tool that refuses by returning an unsuccessful result rather than an error —
// which is every refusal a model is meant to read and act on — reached the
// `error` column as the bare word "ไม่สำเร็จ", so ten precise sentences arrived
// here as one content-free cluster and became a card telling the agent to avoid
// a pattern that word does not name. Fixed at the same kind of place, upstream:
// turn.failureReason records what the tool actually said. Note what that does to
// this pass — clusters now group by real cause, so what used to be one cluster
// of ten may be several below the threshold, which is the honest count.
//
// The third break was the split this comment used to predict (2026-08-13): a
// message this codebase wrote may be a **refusal** (the agent broke a rule, and
// the remedy is to behave differently) or a **state report** (the world is not
// in the shape the tool needed — a server not running, a key expired). Only the
// first is a lesson; the second is about a machine that will be different
// tomorrow, and three cards teaching "avoid whatever hits an n8n that is down
// tonight" reached the queue before the rows could tell them apart. Same fix at
// the same place as the first two: the author of the error says what it is
// (internal/statereport), the run records it (ErrorFromWorld), and the
// error_kind filter below drops it unread. Unmarked stays unfiltered — a
// failure of unknown origin is still offered, and the user still says no.

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

	// error_kind = '' is "a failure this codebase authored" — the only kind that
	// carries a remedy to quote. A program's exit status is excluded here rather
	// than filtered out later, so a cluster of them never reaches the point of
	// competing with a real lesson for one of the three proposal slots.
	rows, err := db.Query(
		`SELECT id, agent, tool, args, error FROM tool_runs
		  WHERE ok = 0 AND error <> '' AND error_kind = '' ORDER BY id`)
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
		key := agent + "\x00" + tool + "\x00" + clusterKey(head)
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

	// One card per tool per pass. Three slots filled by one tool's three ways of
	// failing is a queue that reads as a complaint about that tool, and it is
	// how a single noisy tool ends up owning a memory file: every cluster over
	// the threshold on the machine that prompted this change was `shell`. The
	// rest are not dropped — the pass runs after every turn, and the next one
	// takes them.
	proposed := 0
	perTool := map[string]bool{}
	for _, c := range repeated {
		if proposed >= summarizeMaxProposals {
			break
		}
		if perTool[c.tool] {
			continue
		}
		if a.proposeFailureLesson(c) {
			perTool[c.tool] = true
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
//
// What comes back is the whole remaining sentence, because this is the text
// that becomes the lesson. It used to be cut to 160 runes here, which is the
// length that makes a stable grouping key and has nothing to do with where a
// sentence ends: the one genuinely useful lesson on a real machine was stored
// as "…write the path out literally, or run the step that" — the remedy cut
// off mid-clause, which is the half that was worth learning. Grouping now
// takes its own shortened form (clusterKey) and the body keeps the sentence.
func failureHead(raw string) string {
	line := raw
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = dropBracketed(line)
	line = dropQuoted(line, '"')
	line = dropQuoted(line, '\'')
	line = strings.Join(strings.Fields(line), " ")
	// Removing a bracketed span leaves the space that stood in front of it
	// stranded against the punctuation that followed — "while it runs , so".
	// Cosmetic, but this string is read by a person on a card and by a model in
	// every request afterwards.
	for _, mark := range []string{",", ".", ";", ":"} {
		line = strings.ReplaceAll(line, " "+mark, mark)
	}
	line = truncateWords(line, 400)
	return strings.Trim(strings.TrimSpace(line), " :,-—")
}

// clusterKey is the grouping form of a head: short enough that two occurrences
// diverging late still land together, lowercased so casing cannot split a
// cluster. Deliberately not what gets stored — a key may be lossy, a lesson
// may not.
func clusterKey(head string) string {
	if r := []rune(head); len(r) > 160 {
		head = string(r[:160])
	}
	return strings.ToLower(head)
}

// truncateWords caps a string without cutting a word in half. A hard cap still
// exists because an error is not required to be short, and a memory line is
// paid for on every request that loads the file.
func truncateWords(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if at := strings.LastIndexByte(cut, ' '); at > max/2 {
		cut = cut[:at]
	}
	return strings.TrimRight(cut, " ,;:-—") + "…"
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
