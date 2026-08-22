package model

import (
	"strings"
	"testing"
)

// The body below is verbatim from a real turn on 2026-08-20, with the account
// id shortened. Two different models produced two different function ids and
// the same refusal, which is what established that it is the account and not
// the model — the fact the raw message hides.
const nvidiaEntitlement404 = `{"status":404,"title":"Not Found","detail":"Function 'ee47df99-c92b-4dc9-b3a7-f3fb0f087b73': Not found for account 'LYikgj4…'"}`

func TestAccountAccessErrorNamesTheRealFix(t *testing.T) {
	err := accountAccessError("nvidia", 404, []byte(nvidiaEntitlement404), nvidiaEntitlement404)
	if err == nil {
		t.Fatal("a refusal-by-entitlement was read as an ordinary 404 — the user is sent back to the model picker for a problem no model can solve")
	}
	msg := err.Error()
	// Try another model first, support second. The order matters: on the key
	// this was measured with, a priced model answered fine minutes after two
	// free ones were refused, so "your account is locked" would have been a
	// wrong instruction for a wall the user could walk around.
	for _, want := range []string{"try another model", "help@build.nvidia.com", "nvidia"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention %q, so it does not say what to do: %s", want, msg)
		}
	}
	if strings.Index(msg, "try another model") > strings.Index(msg, "help@build.nvidia.com") {
		t.Errorf("support is offered before the fix that usually works: %s", msg)
	}
	// The raw body survives in the message. A user forwarding this to NVIDIA
	// support needs the function id, and support asks for it by name.
	if !strings.Contains(msg, "ee47df99") {
		t.Errorf("the function id was dropped; support asks for it: %s", msg)
	}
}

// The common 404 must stay the common 404. Rewriting every not-found into an
// account lecture would be worse than the raw message it replaced.
func TestAnOrdinaryNotFoundIsLeftAlone(t *testing.T) {
	body := `{"error":{"message":"The model 'gpt-9' does not exist","code":"model_not_found"}}`
	if err := accountAccessError("openai", 404, []byte(body), body); err != nil {
		t.Fatalf("a missing model was rewritten as an account problem: %v", err)
	}
}
