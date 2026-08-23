package model

import (
	"errors"
	"strings"
	"testing"
)

func truncatedWrite() ToolCall {
	return ToolCall{
		ID:   "call_write_1",
		Type: "function",
		Function: FunctionCall{
			Name:      "write",
			Arguments: `{"path": "report.html", "content": "<!DOCTYPE html>\n<html lang=\"th`,
		},
	}
}

func wholeSearch() ToolCall {
	return ToolCall{
		ID:       "call_search_1",
		Type:     "function",
		Function: FunctionCall{Name: "web_search", Arguments: `{"query":"Gemini 3.1 pricing"}`},
	}
}

// The case that sent this work: the provider cut the call and reported an
// ordinary stop. Nothing about the response says "length", so the old check saw
// a normal round and handed unrunnable JSON to the executor.
func TestToolCallTruncated_SilentProviderIsStillCaught(t *testing.T) {
	cut, atLimit := ToolCallTruncated(Response{
		FinishReason: "stop",
		ToolCalls:    []ToolCall{truncatedWrite()},
	}, 8192)
	if !cut {
		t.Fatal("a call whose arguments stop mid-string must be refused whatever the provider calls it")
	}
	if atLimit {
		t.Fatal("nothing here proves the ceiling was reached; the wording must not claim it as fact")
	}
}

func TestToolCallTruncated_FinishReasonLength(t *testing.T) {
	cut, atLimit := ToolCallTruncated(Response{
		FinishReason: FinishReasonLength,
		ToolCalls:    []ToolCall{truncatedWrite()},
	}, 8192)
	if !cut || !atLimit {
		t.Fatalf("the provider said so outright: cut=%v atLimit=%v", cut, atLimit)
	}
}

// Our own two numbers, needing nothing from the provider's judgement.
func TestToolCallTruncated_CompletionTokensReachCeiling(t *testing.T) {
	cut, atLimit := ToolCallTruncated(Response{
		FinishReason: "stop",
		Usage:        &Usage{CompletionTokens: 8192},
		ToolCalls:    []ToolCall{wholeSearch()},
	}, 8192)
	if !cut || !atLimit {
		t.Fatalf("spending the whole ceiling is the limit being hit: cut=%v atLimit=%v", cut, atLimit)
	}
}

// A duplicated argument is valid JSON meaning the wrong thing. Sending that
// model off to shorten its output is a fix for a problem it does not have.
func TestToolCallTruncated_DuplicateArgumentIsNotTruncation(t *testing.T) {
	cut, _ := ToolCallTruncated(Response{
		FinishReason: "stop",
		ToolCalls: []ToolCall{{
			ID:       "call_dupe",
			Function: FunctionCall{Name: "web_search", Arguments: `{"query":"Kimi K3","query":"Mistral Large 3"}`},
		}},
	}, 8192)
	if cut {
		t.Fatal("the duplicate-argument refusal owns this case, and words it from its own cause")
	}
}

func TestToolCallTruncated_HealthyRoundIsLeftAlone(t *testing.T) {
	cut, atLimit := ToolCallTruncated(Response{
		FinishReason: "tool_calls",
		Usage:        &Usage{CompletionTokens: 120},
		ToolCalls:    []ToolCall{wholeSearch()},
	}, 8192)
	if cut || atLimit {
		t.Fatalf("an ordinary round must run: cut=%v atLimit=%v", cut, atLimit)
	}
}

func TestToolCallUnfinished_SeparatesTheCutCallFromItsNeighbours(t *testing.T) {
	if ToolCallUnfinished(wholeSearch()) {
		t.Fatal("a call that arrived whole was not the one truncated")
	}
	if !ToolCallUnfinished(truncatedWrite()) {
		t.Fatal("the call that stops mid-string is the one that was cut")
	}
}

// The refusal is read by a model deciding what to do next, so it has to carry
// the number and the way out, not only the ban.
func TestTruncatedToolCallRefusal_CarriesLimitAndRemedy(t *testing.T) {
	msg := TruncatedToolCallRefusal("write", 8192, true, nil)
	for _, want := range []string{"NOT executed", "write", "8192", "truncated", "edit"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("refusal is missing %q: %s", want, msg)
		}
	}
}

// The executor side knows no ceiling. It must still name truncation and the
// same remedy rather than inventing a second explanation.
func TestTruncatedToolCallRefusal_WithoutCeilingStillNamesTheCause(t *testing.T) {
	msg := TruncatedToolCallRefusal("write", 0, false, errors.New("unexpected end of JSON input"))
	for _, want := range []string{"truncated", "unexpected end of JSON input", "edit"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("refusal is missing %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "0-token") {
		t.Fatalf("an unknown ceiling must not be printed as a number: %s", msg)
	}
}

// The calls that arrived whole are not told to write less. They were never too
// long, and obeying that advice would shrink work that was already correct.
func TestUnfinishedRoundRefusal_TellsIntactCallsToReissue(t *testing.T) {
	msg := UnfinishedRoundRefusal("web_search", 8192)
	if !strings.Contains(msg, "reissue") || !strings.Contains(msg, "web_search") {
		t.Fatalf("an intact call needs a reissue instruction naming it: %s", msg)
	}
	if strings.Contains(msg, "shorter") {
		t.Fatalf("nothing about this call was too long: %s", msg)
	}
}

// The fragment must not survive into the history. It is re-sent on every later
// request, and opencode-go 400s on it — the guard refuses the call and the
// turn dies anyway, one round later, of the evidence left behind.
func TestSanitizeToolCallArguments_ReplacesUnparseableArgumentsInPlace(t *testing.T) {
	calls := []ToolCall{wholeSearch(), truncatedWrite()}
	safe := SanitizeToolCallArguments(calls)

	if len(safe) != 2 {
		t.Fatalf("every call must survive, ids are paired with results: got %d", len(safe))
	}
	if safe[0].Function.Arguments != calls[0].Function.Arguments {
		t.Error("a call that arrived whole must be left exactly as it was")
	}
	if safe[1].Function.Arguments != "{}" {
		t.Errorf("the cut call must be made valid, got %q", safe[1].Function.Arguments)
	}
	if safe[1].ID != "call_write_1" || safe[1].Function.Name != "write" {
		t.Error("id and name carry the pairing and must not be touched")
	}
	for _, call := range safe {
		if _, err := ParseToolArguments(call.Function.Arguments); err != nil {
			t.Errorf("%s still does not parse: %v", call.ID, err)
		}
	}
	// The caller's slice is shared with the response it came from.
	if calls[1].Function.Arguments == "{}" {
		t.Error("sanitizing must not rewrite the response in place")
	}
}

// A duplicated argument is valid JSON. No gateway rejects it, and blanking it
// would erase the evidence the duplicate-argument refusal is written from.
func TestSanitizeToolCallArguments_LeavesValidJSONAlone(t *testing.T) {
	dupe := ToolCall{
		ID:       "call_dupe",
		Function: FunctionCall{Name: "web_search", Arguments: `{"query":"a","query":"b"}`},
	}
	safe := SanitizeToolCallArguments([]ToolCall{dupe})
	if safe[0].Function.Arguments != dupe.Function.Arguments {
		t.Errorf("valid JSON must pass through untouched, got %q", safe[0].Function.Arguments)
	}
}
