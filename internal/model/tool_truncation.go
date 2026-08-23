package model

import (
	"errors"
	"strconv"
	"strings"
)

// A tool call the model never finished writing, and the one wording that says so.
//
// The shape is always the same: the arguments are bigger than the tokens left
// in the round, the JSON stops mid-string, and what arrives cannot be run.
// Running it anyway is the worst outcome on offer, a file written to half its
// length and reported as done, so it is refused wherever it is caught.
//
// It is caught in two places because two different things know about it. The
// tool loop holds the round's ceiling and sees the whole response, and refuses
// before a tool is reached (cognitive.Agent). The executor holds only the
// broken arguments, and catches whatever reaches it by another road
// (turn.Executor). Both speak from here: a model that reads two explanations
// of one failure fixes it two ways, and only one of them is the way out.

// ToolCallTruncated reports whether this response's tool calls were cut off,
// and whether the output limit can be stated as fact rather than inferred.
//
// Detection deliberately does not rest on the provider being honest. It used
// to: finish_reason "length" was the only signal, and Google's endpoint has
// been seen cutting a call short while reporting an ordinary stop (owner's
// log, 23 ส.ค. 2026 — a whole HTML document attempted in one write, 139
// seconds of streaming thrown away). What that missed fell through to the
// executor, which knows less and could only guess out loud.
//
// So two signals stand in front of it, neither needing anything from the
// provider's judgement: completion tokens reaching the ceiling we ourselves
// set, and arguments that will not parse, which is the symptom itself.
//
// A duplicated argument is not this failure. That JSON is valid and means the
// wrong thing (DuplicateArgumentError), and its remedy — two calls instead of
// one — is not "write less".
func ToolCallTruncated(response Response, ceiling int) (cut, atLimit bool) {
	spent := 0
	if response.Usage != nil {
		spent = response.Usage.CompletionTokens
	}
	if response.FinishReason == FinishReasonLength || (ceiling > 0 && spent >= ceiling) {
		return len(response.ToolCalls) > 0, true
	}
	for _, call := range response.ToolCalls {
		if ToolCallUnfinished(call) {
			return true, false
		}
	}
	return false, false
}

// ToolCallUnfinished reports whether this one call is the one that was cut.
//
// Truncation lands on the last call of a round; the ones before it arrived
// whole. Asking per call is what lets a refusal tell each of them the truth,
// rather than sending a complete call away to shorten arguments that were
// never too long.
func ToolCallUnfinished(call ToolCall) bool {
	_, err := ParseToolArguments(call.Function.Arguments)
	if err == nil {
		return false
	}
	var duplicate *DuplicateArgumentError
	return !errors.As(err, &duplicate)
}

// SanitizeToolCallArguments makes a round's tool calls safe to keep.
//
// A refused call is still recorded: the assistant message that carried it goes
// into the history, and the history is re-sent on every later request. So the
// broken fragment does not stay in one round, it rides along for the rest of
// the conversation — and a gateway that validates what it is handed rejects
// the whole thing. Measured: one second after the truncation guard fired,
// opencode-go answered 400 "tool_calls[].function.arguments for function
// 'write' must be a JSON object string (or an object), got invalid JSON",
// ending a turn that had already run 173 seconds (owner's log, 12:36:28 on
// 23 ส.ค. 2026). The guard had done its job and the turn died anyway, of the
// evidence it left behind.
//
// An empty object replaces the fragment. Not a deletion: the call id has to
// survive, because the wire formats pair every call with its result and a
// missing half is its own 400. What the model needs to know is in that result,
// which is where it belongs — the arguments were never going to be read again.
func SanitizeToolCallArguments(calls []ToolCall) []ToolCall {
	var out []ToolCall
	for i, call := range calls {
		if !ToolCallUnfinished(call) {
			continue
		}
		if out == nil {
			out = make([]ToolCall, len(calls))
			copy(out, calls)
		}
		out[i].Function.Arguments = "{}"
	}
	if out == nil {
		return calls // nothing broken; keep the original slice
	}
	return out
}

// TruncatedToolCallRefusal is the sentence handed back in place of the call
// that was cut off.
//
// ceiling is the round's max_tokens where the caller knows it and 0 where it
// does not, and atLimit is whether hitting it is established rather than
// inferred. The remedy does not vary, which is the point of one function: it
// names the way out (skeleton first, extend after) rather than only the ban.
func TruncatedToolCallRefusal(toolName string, ceiling int, atLimit bool, parseErr error) string {
	subject := "your tool call arguments"
	if name := strings.TrimSpace(toolName); name != "" {
		subject = "your " + name + " arguments"
	}
	limit := "the output token limit"
	if ceiling > 0 {
		limit = "this round's " + strconv.Itoa(ceiling) + "-token output limit"
	}
	var cause string
	switch {
	case atLimit:
		cause = subject + " were truncated at " + limit
	case parseErr != nil:
		cause = subject + " are not valid JSON (" + parseErr.Error() +
			"), which is what a truncated call leaves behind when " + limit + " cuts it short"
	default:
		cause = subject + " are not valid JSON, which is what a truncated call leaves behind when " +
			limit + " cuts it short"
	}
	return "tool call NOT executed: " + cause +
		". Produce a shorter version, or split the work into several smaller tool calls (e.g. write a skeleton file first, then extend it with edit)."
}

// UnfinishedRoundRefusal is for the calls that were NOT the one cut off.
//
// They still need an answer — every tool call id in the assistant message
// requires a result, or the next request is a 400 — and the truncation wording
// would be false for them. Reissuing is the whole instruction, with the
// ceiling attached so a round that keeps hitting it does not reissue its way
// into the same wall.
func UnfinishedRoundRefusal(toolName string, ceiling int) string {
	subject := "this call"
	if name := strings.TrimSpace(toolName); name != "" {
		subject = "your " + name + " call"
	}
	limit := "its output token limit"
	if ceiling > 0 {
		limit = "its " + strconv.Itoa(ceiling) + "-token output limit"
	}
	return "tool call NOT executed: the round was cut off at " + limit +
		" before it finished, so nothing in it ran. " + subject +
		" arrived intact, so reissue it and keep the round's total output inside the limit."
}
