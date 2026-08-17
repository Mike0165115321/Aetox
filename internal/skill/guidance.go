package skill

import "strings"

// Three things ride in a tool block today, and only one of them has to.
//
// A tool definition carries three separable things, and they were bundled
// because nothing ever asked them to be separate:
//
//	Existence  — that this capability is here at all.       Needed always.
//	Signature  — what to pass to call it.                   Needed when calling.
//	Judgment   — when to reach for it, what it costs, the   Needed ONCE.
//	             hazards, what it does NOT do.
//
// Measured on `browser` the day this landed: 10 tokens of existence, ~70 of
// signature, ~686 of judgment. The heaviest layer by an order of magnitude is
// the one the model needs a single time, and it was being re-sent with every
// message for the life of the conversation.
//
// Guidance is that third layer, moved. A tool that implements it keeps its
// judgment OUT of ToolDefinition and hands it over on the first call of the
// session instead — attached to a result the caller was going to receive
// anyway, so the saving costs no extra round trip. That is the difference from
// tool-search designs (Anthropic's own `defer_loading`, the MCP progressive-
// loading patterns): those spend a lookup to fetch a schema; this spends
// nothing, because the first real call is the delivery.
//
// The cost of that trade, stated plainly: the FIRST call of a session is made
// without judgment. Signature has to be enough to get it right — which is why
// the standard puts parameter names in the block and prose out of it — and a
// first call that goes wrong gets the guidance back as its error, which is a
// loop that closes itself in one round trip rather than one that never closes.
//
// What this deliberately does not touch: which tools exist for this caller.
// Every capability stays visible in the block, by name, at all times. Rights
// still come only from the list the user can see, and nothing here is a hidden
// door — the bytes move, the grant does not.
type Guided interface {
	// Guidance is what somebody needs to know once before using this well.
	// Returned verbatim to the model with the first result, and never again.
	//
	// It takes the call because a packed tool is several acts wearing one name,
	// and their judgment is not shared: a session that opens a page and reads it
	// has no use for what `dialog` defaults to. Keyed per action, a nine-action
	// browser sends the two paragraphs that session earned instead of all nine.
	// A plain tool ignores the argument.
	//
	// Write what generalizes, not a list of cases: the conditions under which
	// this is the right act, what it costs, and the failure that does not look
	// like a failure. Return "" when there is nothing to say once that the
	// signature does not already say every time.
	//
	// A rule that prevents HARM does not belong here — see guidanceKey.
	Guidance(args map[string]any) string
}

// guidanceFor returns what to teach about one call, or "" when it teaches
// nothing.
func guidanceFor(s Skill, args map[string]any) string {
	g, ok := s.(Guided)
	if !ok {
		return ""
	}
	return g.Guidance(args)
}

// guidanceKey is what "already taught" is remembered against: the tool, plus
// the action for a packed one.
//
// Per action rather than per tool because the packed tools are both the biggest
// spenders and the ones whose acts have least in common. What that leaves
// behind is the judgment SHARED by every act — how a ref goes stale, whose tabs
// are whose — which stays in the block, because it is closer to how you call
// the thing than to when you should want to.
//
// And one carve-out that the first migration turned up: a rule that exists to
// prevent harm stays in the block too, however much it reads like judgment.
// Guidance rides in the message stream, so a conversation long enough to be
// summarised can lose it — silently. That is an acceptable price for "read
// before you photograph" and not for "never type a password into a page".
func guidanceKey(tool string, args map[string]any) string {
	if len(PackedCalls(tool)) == 0 {
		return tool
	}
	action, _ := args["action"].(string)
	if action = strings.ToLower(strings.TrimSpace(action)); action == "" {
		return tool
	}
	return tool + "." + action
}
