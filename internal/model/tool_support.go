package model

import "strings"

// toolBlockRejectionPhrases are what a provider says when the request was
// refused for the TOOL BLOCK it carried — not for the account behind it, not
// for the size of the prompt, and not for the network under it. Lowercase;
// matched as substrings.
//
// Each entry keeps both halves of the refusal in one phrase — the thing being
// refused and the refusal itself — rather than checking for "tool" and
// "invalid" separately. Providers echo the request they rejected often enough
// that a body containing the word "tools" proves nothing on its own.
var toolBlockRejectionPhrases = []string{
	"does not support tools",    // Ollama's own words — see rejectsTools in ollama.go
	"does not support function", // the same refusal, singular, on OpenAI-compatible hosts
	"tools are not supported",
	"tool use is not supported",
	"tool calling is not supported",
	"function calling is not supported",
	"function calling is not enabled",
	"invalid schema for function",                   // OpenAI, when a tool's parameters do not validate
	"unrecognized request argument supplied: tools", // OpenAI, when the endpoint takes no tools at all
}

// IsToolBlockRejection reports that a request failed because of the tools sent
// with it, and that asking the same question WITHOUT them is a fix rather than
// a second bill.
//
// It exists to narrow a door that used to stand open. The tool loop's
// first-round failure path asked the model again, without tools, on *any*
// error — a quota wall, a rejected sign-in, a dropped Wi-Fi, the user's own
// Stop. Measured against a plan that had said "resets in 19 days", one question
// cost two full-context calls, three HTTP attempts each: six requests to a
// provider that had already given its final answer (2026-08-22).
//
// Ollama already answers this properly one layer down: it knows its own dialect,
// spots the refusal in the body and re-sends as plain chat inside the provider
// (ollama.go). That is where this belongs, and where it should end up for the
// rest. Until then this is the same door for the providers that do not have one
// — OpenAICompatibleProvider claims tool support for every row it serves, so a
// model that turns out not to have it has nothing else to catch it.
//
// Deliberately conservative, and the asymmetry now runs the right way. A false
// negative costs the user an honest error with a ลองใหม่ button next to it,
// which is what they should have been shown all along. A false positive costs
// one extra call — the old behaviour, on one error instead of on all of them.
func IsToolBlockRejection(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range toolBlockRejectionPhrases {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}
