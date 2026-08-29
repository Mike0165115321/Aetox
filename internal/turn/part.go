package turn

// A turn is a sequence, not a string.
//
// Every provider protocol says so — Anthropic and OpenAI both stream an
// assistant turn as ordered blocks: prose, a tool call, more prose, another
// tool call, a final paragraph. Aetox collapsed that into one Reply string and
// kept only the last block, which is why so much of the engine is spent
// deciding *which* text is "the answer": narration in front of a tool call is
// not, a round the loop nudges away is not, a finished reply demoted by an
// interjection is not. Fifteen distinct places had to answer that question, and
// none of them would exist if the turn were simply kept whole.
//
// TurnPart is the whole. Reply survives beside it as the concatenation of the
// text parts — every existing caller reads that and keeps working — but the
// parts are what the UI draws and what the store remembers, so narration lands
// where it was said instead of in a separate panel, and a streamed fragment
// belongs to a specific part rather than to a bubble that might later have to
// take it back.

// PartKind names what one piece of a turn is.
type PartKind string

const (
	// PartText is prose the model wrote — narration ahead of a tool call, or
	// the closing answer. The distinction that used to matter (which one is
	// "the reply") stops mattering once both are simply in the sequence.
	PartText PartKind = "text"
	// PartThinking is one reasoning segment, kept for its duration rather than
	// its content: the text streams live and is stored on the message, but how
	// long the model thought belongs in the sequence, between the parts it sat
	// between.
	PartThinking PartKind = "thinking"
	// PartTool is one completed tool call.
	PartTool PartKind = "tool"
)

// TurnPart is one piece of an assistant turn, in the order it happened.
type TurnPart struct {
	Kind PartKind `json:"kind"`
	// Text carries a PartText's prose. Empty for the other kinds.
	Text string `json:"text,omitempty"`
	// Demoted marks a PartText the model wrote as its whole answer, which an
	// interjection then re-placed (cognitive.Agent keeps the turn alive rather
	// than making the user wait out a reply they have already moved past). It
	// is prose the reader was already reading, so it stays prose wherever the
	// turn is drawn — the sequence is the only record that it was ever more
	// than narration, and without this field a reopened session cannot tell the
	// two apart.
	Demoted bool `json:"demoted,omitempty"`
	// Secs is how long a PartThinking segment streamed.
	Secs int `json:"secs,omitempty"`
	// Tool describes a PartTool. Nil for the other kinds.
	Tool *ToolPart `json:"tool,omitempty"`
}

// ToolPart is what a finished tool call looks like in the transcript — the same
// facts ToolEvent carries live, kept because a reopened session should show the
// work, not just its conclusion.
type ToolPart struct {
	Ref     string `json:"ref,omitempty"`
	Name    string `json:"name"`
	Subject string `json:"subject,omitempty"`
	// Agent and Brief are set only on a `task` call: which sub-agent the work
	// went to, and the brief it was given. AgentKind is which pile that worker
	// is in — "agent" or "helper", same values as ToolEvent.AgentKind — kept so
	// a reopened session still counts the two apart.
	Agent     string `json:"agent,omitempty"`
	Brief     string `json:"brief,omitempty"`
	AgentKind string `json:"agentKind,omitempty"`
	// Delegation is ToolEvent.Delegation written down: whether this `task` row
	// hired anybody, or was one of the four actions that do not. A reopened
	// session reads only the parts, so leaving it off the transcript would fix
	// the live turn and let the same wrong card come back on reload.
	Delegation *bool  `json:"delegation,omitempty"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	Secs       int    `json:"secs,omitempty"`
	Added      int    `json:"added,omitempty"`
	Removed    int    `json:"removed,omitempty"`
	// Count/Range are ToolEvent.Count and Range written down — see the comment
	// there. A reopened session rebuilds its rows from these parts alone, and
	// "read gate.py 1-60" is exactly the kind of fact worth still being true
	// tomorrow.
	Count int    `json:"count,omitempty"`
	Range string `json:"range,omitempty"`
	// Problems is ToolEvent.Problems written down — the "!N" a reopened
	// session still owes the reader about an edit that broke the file.
	Problems int `json:"problems,omitempty"`
	// Diff is ToolEvent.Diff written down, for the same reason Artifacts is one
	// field below: this is the part that survives a restart. A change you can
	// only inspect until the app closes is a change you have to take on trust
	// the next morning. Capped where it is built, so a turn of large writes
	// cannot grow the transcript without bound.
	Diff string `json:"diff,omitempty"`
	// Artifacts are the finished files this call made for the user, carried
	// here rather than only on the live ToolEvent because this is the part that
	// is *written down*. The chat draws an open button per artifact, and until
	// this field existed that button vanished the moment the app restarted —
	// the file was still on disk and the user had no way back to it. A new JSON
	// field inside the existing parts column, so no migration: older rows simply
	// have no artifacts, which is what they had anyway.
	Artifacts []string `json:"artifacts,omitempty"`
	// ProposalID is the queued change a `memory` call is waiting on, written
	// down for the same reason Artifacts is: the card belongs to the answer that
	// proposed it, and a session reopened tomorrow should still show it — either
	// still asking, or saying which way it went. Zero on every other tool.
	ProposalID int64 `json:"proposalId,omitempty"`
}

// partList accumulates a turn as it happens. Not safe for concurrent use, and
// does not need to be: the tool loop is one goroutine, and a delegate's steps
// are its own turn, not this one.
type partList struct {
	parts []TurnPart
}

// addText appends prose, merging into the previous part when that was also
// prose. Two text parts in a row mean nothing happened between them, and a
// reader should see one paragraph rather than an arbitrary seam.
func (p *partList) addText(text string) {
	if p == nil || text == "" {
		return
	}
	// A demoted answer is a closed block — it was complete when it was said, and
	// whatever the model writes after the interjection is a separate thought.
	// Merging into it would glue two answers into one paragraph run.
	if n := len(p.parts); n > 0 && p.parts[n-1].Kind == PartText && !p.parts[n-1].Demoted {
		p.parts[n-1].Text += "\n\n" + text
		return
	}
	p.parts = append(p.parts, TurnPart{Kind: PartText, Text: text})
}

// addAnswer appends prose the model meant to end the turn with, before an
// interjection kept the turn going. Never merged, in either direction: it is one
// finished answer, and the reader saw it as one.
func (p *partList) addAnswer(text string) {
	if p == nil || text == "" {
		return
	}
	p.parts = append(p.parts, TurnPart{Kind: PartText, Text: text, Demoted: true})
}

func (p *partList) addThinking(secs int) {
	if p == nil || secs <= 0 {
		return
	}
	p.parts = append(p.parts, TurnPart{Kind: PartThinking, Secs: secs})
}

func (p *partList) addTool(tool ToolPart) {
	if p == nil {
		return
	}
	p.parts = append(p.parts, TurnPart{Kind: PartTool, Tool: &tool})
}

// lastText is the most recent prose in the sequence, which for a normal turn is
// the closing answer. Used to tell "the reply already reached the sequence
// through OnRound" from "the loop wrote this reply itself" — comparing against
// the whole joined text would fail on every turn that had narration, and append
// the answer a second time.
func (p *partList) lastText() (string, bool) {
	if p == nil {
		return "", false
	}
	for i := len(p.parts) - 1; i >= 0; i-- {
		if p.parts[i].Kind == PartText {
			return p.parts[i].Text, true
		}
	}
	return "", false
}

func (p *partList) all() []TurnPart {
	if p == nil || len(p.parts) == 0 {
		return nil
	}
	return append([]TurnPart(nil), p.parts...)
}

// TextOf joins a part sequence back into the single string every caller that
// predates parts still expects — context, persistence, a sub-agent's return
// value. It is the definition of Reply, not an approximation of it.
func TextOf(parts []TurnPart) string {
	var out []string
	for _, part := range parts {
		if part.Kind == PartText && part.Text != "" {
			out = append(out, part.Text)
		}
	}
	return joinParagraphs(out)
}

func joinParagraphs(chunks []string) string {
	switch len(chunks) {
	case 0:
		return ""
	case 1:
		return chunks[0]
	}
	total := 0
	for _, c := range chunks {
		total += len(c) + 2
	}
	buf := make([]byte, 0, total)
	for i, c := range chunks {
		if i > 0 {
			buf = append(buf, '\n', '\n')
		}
		buf = append(buf, c...)
	}
	return string(buf)
}
