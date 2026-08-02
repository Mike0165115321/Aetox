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
	// went to, and the brief it was given.
	Agent   string `json:"agent,omitempty"`
	Brief   string `json:"brief,omitempty"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Secs    int    `json:"secs,omitempty"`
	Added   int    `json:"added,omitempty"`
	Removed int    `json:"removed,omitempty"`
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
	if n := len(p.parts); n > 0 && p.parts[n-1].Kind == PartText {
		p.parts[n-1].Text += "\n\n" + text
		return
	}
	p.parts = append(p.parts, TurnPart{Kind: PartText, Text: text})
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
