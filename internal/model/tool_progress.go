package model

import "time"

// toolProgressInterval paces the "still writing" updates. Fast enough to read
// as a live counter rather than a stalled one, slow enough that an 800-line
// file is a couple of hundred IPC messages instead of a thousand — the model
// emits lines far faster than a screen is worth repainting.
// Var (not const) so tests can shrink it.
var toolProgressInterval = 200 * time.Millisecond

// nowFunc is time.Now, indirected so a test can hold the clock still.
//
// The rule here is "at most one update per interval". A test that proved it by
// racing 500 iterations against a real 200ms window was not testing the rule —
// it was testing how busy the machine is, and it failed on a loaded one while
// passing on an idle one. With the clock in the test's hand the rule is checked
// exactly, and its boundary can be checked too.
var nowFunc = time.Now

// toolProgressTracker decides what Request.OnToolCallProgress hears while a
// tool call is still being written. Every wire format accumulates the call
// differently — OpenAI streams argument fragments keyed by index, Anthropic
// streams input_json deltas per content block, Ollama hands over whole calls —
// but what a UI needs out of them is the same three facts, so the deciding
// lives here once instead of in each provider.
//
// A nil tracker, or one with no hook, is inert: most providers and every
// non-streaming call take that path.
type toolProgressTracker struct {
	onProgress func(id, name, subject string, lines int)
	state      map[int]*toolProgressState
}

type toolProgressState struct {
	started   bool      // an update has gone out for this call
	subject   string    // resolved once, then reused
	named     bool      // subject was resolvable
	sentNamed bool      // an update carrying the subject has gone out
	lines     int       // last count reported
	at        time.Time // when that report went out, for pacing
}

func newToolProgressTracker(onProgress func(id, name, subject string, lines int)) *toolProgressTracker {
	return &toolProgressTracker{
		onProgress: onProgress,
		state:      map[int]*toolProgressState{},
	}
}

// report is called with everything known about call `key` so far. It draws the
// row on the first sighting, keeps the line count climbing while content
// streams, and relabels the row when the naming argument finally arrives —
// argument order is the model's choice, so a write's "content" can precede its
// "path" and the row has to open unnamed rather than stay invisible.
func (t *toolProgressTracker) report(key int, id, name, args string) {
	if t == nil || t.onProgress == nil || name == "" {
		return
	}
	st := t.state[key]
	if st == nil {
		st = &toolProgressState{}
		t.state[key] = st
	}

	// Too soon to say anything new — return before measuring anything. Both
	// helpers below scan the arguments accumulated so far, which grow with the
	// file being written, so measuring on every fragment is what made a large
	// write quadratic. Pacing first bounds the work by wall-clock rate instead
	// of by how many fragments the provider chose to send.
	now := nowFunc()
	if st.started && now.Sub(st.at) < toolProgressInterval {
		return
	}

	if !st.named {
		if resolved, ok := SubjectFromPartialArgs(args); ok {
			st.subject, st.named = resolved, true
		}
	}
	lines := ContentLinesSoFar(args)

	// Nothing changed: the count stood still and the name is already on screen.
	if st.started && lines == st.lines && (!st.named || st.sentNamed) {
		return
	}

	st.started = true
	st.sentNamed = st.sentNamed || st.named
	st.lines = lines
	st.at = now
	t.onProgress(id, name, st.subject, lines)
}

// flush is report with the pacing window forced open, for the moment a call has
// finished arriving.
//
// Argument order is the model's choice, and when it puts `content` first the
// `path` lands in the very last fragment — inside the pacing window that the
// update before it just opened. Pacing then swallows the one update that would
// have named the row, and it stays unnamed for the rest of the turn: caught live
// on a 9KB write whose row never learned its filename. Nothing else is forced;
// report's own "nothing changed" check still applies, so a call that was already
// named sends nothing here.
func (t *toolProgressTracker) flush(key int, id, name, args string) {
	if t == nil || t.onProgress == nil {
		return
	}
	if st := t.state[key]; st != nil {
		st.at = time.Time{}
	}
	t.report(key, id, name, args)
}
