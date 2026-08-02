package model

// The streaming half of the DSML story.
//
// parseDSMLToolCalls (dsml.go) is a backstop that runs on a finished response:
// by the time it lifts a leaked block out, the whole answer is in hand and the
// user has seen nothing. That is why the tool loop could afford to be strict —
// it simply never streamed content, so leaked markup could never reach a screen.
//
// Streaming the answer live gives that guarantee up, and gets it back here. The
// gate sits between the provider's content deltas and the UI, and withholds any
// tail that could still turn into an opening DSML marker. Prose flows; markup
// never does.
//
// It withholds rather than filters because a marker can be split across deltas —
// "<｜DS" in one frame and "ML｜invoke" in the next is the normal case, not the
// exotic one, and a per-chunk check would pass both halves through.

import (
	"strings"
	"unicode/utf8"
)

// dsmlMarkerRunes is the opening marker, position by position: `<`, a pipe,
// D, S, M, L, a pipe. It mirrors dsmlOpenRe's prefix — the regex continues with
// "tool_calls|invoke", but seven runes is already proof enough: no prose writes
// `<｜DSML｜`, and holding back the two candidate words would only widen the
// window for no gain.
var dsmlMarkerRunes = []func(rune) bool{
	func(r rune) bool { return r == '<' },
	isDSMLPipe,
	func(r rune) bool { return r == 'D' },
	func(r rune) bool { return r == 'S' },
	func(r rune) bool { return r == 'M' },
	func(r rune) bool { return r == 'L' },
	isDSMLPipe,
}

// isDSMLPipe accepts both the real token's fullwidth bar and the ASCII one
// dsml.go already tolerates.
func isDSMLPipe(r rune) bool { return r == '|' || r == '｜' }

// dsmlMarkerPrefix measures how much of s could be the start of an opening
// marker.
//
// It returns the byte length matched and whether the whole marker is present.
// (n > 0, false) means "s ends mid-marker — it may still become one", which is
// the case the gate exists for; (0, false) means s is ordinary text.
func dsmlMarkerPrefix(s string) (int, bool) {
	consumed := 0
	for i, matches := range dsmlMarkerRunes {
		if consumed >= len(s) {
			return consumed, false // ran out of input mid-marker
		}
		r, size := utf8.DecodeRuneInString(s[consumed:])
		// A rune split across two deltas decodes as RuneError with size 1. It
		// cannot be judged yet, so it is treated as "may still become a marker"
		// rather than released — the next delta completes it.
		if r == utf8.RuneError && size <= 1 && consumed+utf8.UTFMax > len(s) {
			return consumed, false
		}
		if !matches(r) {
			return 0, false
		}
		consumed += size
		if i == len(dsmlMarkerRunes)-1 {
			return consumed, true
		}
	}
	return consumed, false
}

// dsmlGate forwards a content stream with leaked tool-call markup held back.
//
// One gate per provider call: it latches shut on the first complete marker and
// stays shut, because everything after an opening marker belongs to the block,
// and a round that leaked markup is a round whose text the tool loop is about to
// discard anyway.
type dsmlGate struct {
	deliver StreamChunkHandler
	// held is the tail withheld so far: always a proper prefix of a marker, so
	// at most seven runes.
	held string
	shut bool
}

// newDSMLGate wraps a content handler. A nil handler yields a nil gate, and
// every method is nil-safe, so callers that do not want live content can pass
// the result straight through.
func newDSMLGate(deliver StreamChunkHandler) *dsmlGate {
	if deliver == nil {
		return nil
	}
	return &dsmlGate{deliver: deliver}
}

// write is the StreamChunkHandler the provider is given.
func (g *dsmlGate) write(chunk string) error {
	if g == nil || g.deliver == nil || g.shut {
		return nil
	}
	g.held += chunk
	for {
		start := strings.IndexByte(g.held, '<')
		if start < 0 {
			// Nothing here can open a marker.
			return g.flush(len(g.held))
		}
		n, complete := dsmlMarkerPrefix(g.held[start:])
		switch {
		case complete:
			// Markup begins at start. Release the prose in front of it and
			// close for the rest of this call.
			if err := g.flush(start); err != nil {
				return err
			}
			g.held = ""
			g.shut = true
			return nil
		case n > 0:
			// The tail is an unfinished marker — or the beginning of a
			// sentence that merely looks like one. Either way it waits for
			// the next delta.
			return g.flush(start)
		default:
			// An ordinary '<' (markdown, a comparison). Release it and keep
			// looking at what follows.
			if err := g.flush(start + 1); err != nil {
				return err
			}
		}
	}
}

// flush delivers the first n bytes of held and keeps the rest.
func (g *dsmlGate) flush(n int) error {
	if n <= 0 {
		return nil
	}
	out := g.held[:n]
	g.held = g.held[n:]
	return g.deliver(out)
}

// handler adapts the gate to the provider interface, returning nil when there
// is no gate — so "no live content wanted" stays expressible as a nil handler,
// which every provider already guards.
func (g *dsmlGate) handler() StreamChunkHandler {
	if g == nil {
		return nil
	}
	return g.write
}

// GateLeakedDSML wraps a live content handler so leaked tool-call markup can
// never reach it, and returns nil for a nil handler.
//
// This is what the tool loop passes to StreamComplete in place of the nil it
// used to pass: the answer streams, and a leaking model's markup stops here
// instead of on the user's screen.
func GateLeakedDSML(deliver StreamChunkHandler) StreamChunkHandler {
	return newDSMLGate(deliver).handler()
}
