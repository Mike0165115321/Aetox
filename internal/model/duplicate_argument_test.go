package model

import (
	"errors"
	"testing"
)

// The call the owner's log caught at 13:13:43 on 23 ส.ค. The model wanted two
// prices and wrote them into one object, so encoding/json kept the last and
// dropped the first without a word: Mistral was searched, Kimi never was, and
// the answer that followed still listed Kimi as outstanding. Nothing failed,
// which is what made it invisible.
func TestAnArgumentWrittenTwiceIsRefusedRatherThanCollapsed(t *testing.T) {
	raw := `{"query":"Kimi K3 Moonshot API pricing per million tokens","query":"Mistral Large 3 API pricing per million tokens"}`

	args, err := ParseToolArguments(raw)
	if err == nil {
		t.Fatalf("a call that would have run a different job than it was given was accepted: %v", args)
	}
	var duplicate *DuplicateArgumentError
	if !errors.As(err, &duplicate) {
		t.Fatalf("error = %v, want a DuplicateArgumentError the executor can word for the model", err)
	}
	// Named, because "one of your arguments" is not something a model can act
	// on and the whole point is that it re-issues the call correctly.
	if duplicate.Key != "query" {
		t.Errorf("key = %q, want the argument that was written twice", duplicate.Key)
	}
}

// The guard reads a tool's own parameters and nothing deeper. A model passing a
// JSON document through to a file is carrying somebody else's text, and
// refusing to write it because of what is inside would be this codebase
// deciding what a valid document looks like.
func TestARepeatInsideAValueIsNotTheToolsBusiness(t *testing.T) {
	raw := `{"path":"config.json","content":"{\"a\":1,\"a\":2}"}`

	args, err := ParseToolArguments(raw)
	if err != nil {
		t.Fatalf("a file whose CONTENT repeats a key was refused: %v", err)
	}
	if args["path"] != "config.json" {
		t.Errorf("path = %v, want the call to have gone through untouched", args["path"])
	}
}

// Nested objects are still walked past whole, so a key that appears once at the
// top level and again inside one of its own values is not a collision.
func TestANestedKeyDoesNotLookLikeARepeat(t *testing.T) {
	raw := `{"name":"outer","options":{"name":"inner","deep":{"name":"deeper"}},"limit":3}`

	args, err := ParseToolArguments(raw)
	if err != nil {
		t.Fatalf("a nested key was mistaken for a repeated argument: %v", err)
	}
	if len(args) != 3 {
		t.Errorf("args = %v, want all three top-level arguments", args)
	}
}

// The ordinary cases keep working, empty arguments included: a tool that takes
// none is called with "" or "{}" and both have always meant the same thing.
func TestOrdinaryArgumentsStillParse(t *testing.T) {
	for _, raw := range []string{"", "{}", `{"query":"one thing only"}`} {
		if _, err := ParseToolArguments(raw); err != nil {
			t.Errorf("ParseToolArguments(%q) = %v, want it accepted", raw, err)
		}
	}
}
