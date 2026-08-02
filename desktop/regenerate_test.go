package main

import (
	"testing"
)

// Reasoning and its clock were fields on SessionMessage from the day thinking
// panels shipped, and appendTurn wrote neither: reopening a session dropped
// every "คิดเป็นเวลา Xs" it had. Variants needed the same migration, so this is
// the test that stops it regressing again.
func TestAppendTurnPersistsReasoning(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "แบตผม 17", Time: "00:34"},
		SessionMessage{Role: "agent", Text: "ชาร์จไปเรื่อยๆ ครับ", Time: "00:34", Reasoning: "user is charging slowly", ThinkSecs: 4},
	)

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("loaded %d messages; want the pair that was written", len(messages))
	}
	if got := messages[1].Reasoning; got != "user is charging slowly" {
		t.Errorf("reasoning = %q; want it to survive the reload", got)
	}
	if got := messages[1].ThinkSecs; got != 4 {
		t.Errorf("thinkSecs = %d; want 4", got)
	}
}

// A regenerated answer must leave `text` holding the live one: the FTS index,
// the session title and LoadSession's context rebuild all read that column and
// none of them know variants exist.
func TestStoreVariantsKeepsTheLiveAnswerInText(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "ทำไมแบตขึ้นช้า", Time: "00:34"},
		SessionMessage{Role: "agent", Text: "คำตอบแรก", Time: "00:34"},
	)

	variants := []SessionVariant{
		{Text: "คำตอบแรก"},
		{Text: "คำตอบที่สอง", Reasoning: "second attempt", ThinkSecs: 2},
	}
	a.storeVariants(variants, 1)

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	reply := messages[1]
	if reply.Text != "คำตอบที่สอง" {
		t.Errorf("text = %q; want the live variant", reply.Text)
	}
	if reply.Reasoning != "second attempt" || reply.ThinkSecs != 2 {
		t.Errorf("reasoning/thinkSecs = %q/%d; want the live variant's", reply.Reasoning, reply.ThinkSecs)
	}
	if len(reply.Variants) != 2 {
		t.Fatalf("variants = %d; want both answers kept", len(reply.Variants))
	}
	if reply.Active != 1 {
		t.Errorf("active = %d; want 1", reply.Active)
	}
	if reply.Variants[0].Text != "คำตอบแรก" {
		t.Errorf("variants[0] = %q; want the answer that was replaced", reply.Variants[0].Text)
	}

	// The user message beside it must not have grown a variant list.
	if len(messages[0].Variants) != 0 {
		t.Error("the question was given variants; only answers have them")
	}
}

// Picking answer 1 after answer 2 was generated has to rewrite what the
// conversation continues from. Being replied to as if you had kept the other
// one is the bug the switcher would otherwise introduce.
func TestSwitchVariantRewritesTheLiveAnswer(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "ทำไมแบตขึ้นช้า", Time: "00:34"},
		SessionMessage{Role: "agent", Text: "คำตอบที่สอง", Time: "00:34"},
	)
	variants := []SessionVariant{{Text: "คำตอบแรก", ThinkSecs: 1}, {Text: "คำตอบที่สอง", ThinkSecs: 2}}
	a.transcript = []SessionMessage{
		{Role: "user", Text: "ทำไมแบตขึ้นช้า", Time: "00:34"},
		{Role: "agent", Text: "คำตอบที่สอง", Time: "00:34", Variants: variants, Active: 1},
	}
	a.storeVariants(variants, 1)

	result, err := a.SwitchVariant(0)
	if err != nil {
		t.Fatalf("SwitchVariant: %v", err)
	}
	if result.Text != "คำตอบแรก" || result.Active != 0 {
		t.Fatalf("result = %q/%d; want the first answer live", result.Text, result.Active)
	}
	if got := a.transcript[1].Text; got != "คำตอบแรก" {
		t.Errorf("transcript still says %q; the next turn would be answered against the wrong reply", got)
	}
	if got := a.transcript[1].ThinkSecs; got != 1 {
		t.Errorf("thinkSecs = %d; want the chosen variant's own clock", got)
	}

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if messages[1].Text != "คำตอบแรก" || messages[1].Active != 0 {
		t.Errorf("stored text/active = %q/%d; want the switch to have persisted", messages[1].Text, messages[1].Active)
	}
	if len(messages[1].Variants) != 2 {
		t.Error("switching lost the other answer; both must stay switchable")
	}
}

func TestSwitchVariantRefusesWhatItCannotSwitch(t *testing.T) {
	a := newTestApp(t, t.TempDir())

	if _, err := a.SwitchVariant(0); err == nil {
		t.Error("switching on an empty transcript was allowed")
	}

	a.transcript = []SessionMessage{
		{Role: "user", Text: "q"},
		{Role: "agent", Text: "answered once"},
	}
	if _, err := a.SwitchVariant(1); err == nil {
		t.Error("a bubble with a single answer offered a second one")
	}

	a.transcript[1].Variants = []SessionVariant{{Text: "a"}, {Text: "b"}}
	for _, index := range []int{-1, 2, 99} {
		if _, err := a.SwitchVariant(index); err == nil {
			t.Errorf("SwitchVariant(%d) was accepted; want it refused", index)
		}
	}
}

// An edited question deletes its old exchange rather than keeping it as a
// variant — but only that exchange.
func TestDropLastTurnRowsRemovesOnlyTheLastPair(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "คำถามแรก", Time: "00:30"},
		SessionMessage{Role: "agent", Text: "ตอบแรก", Time: "00:30"},
	)
	a.appendTurn(
		SessionMessage{Role: "user", Text: "คำถามสอง", Time: "00:34"},
		SessionMessage{Role: "agent", Text: "ตอบสอง", Time: "00:34"},
	)

	a.dropLastTurnRows()

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("%d messages left; want the first exchange only", len(messages))
	}
	if messages[0].Text != "คำถามแรก" || messages[1].Text != "ตอบแรก" {
		t.Errorf("left %q/%q; want the earlier exchange untouched", messages[0].Text, messages[1].Text)
	}
}

func TestLastQuestionNeedsACompletedExchange(t *testing.T) {
	a := newTestApp(t, t.TempDir())

	if _, ok := a.lastQuestion(); ok {
		t.Error("an empty transcript reported a question to re-run")
	}

	// A turn that failed leaves nothing in the transcript, so what is there
	// must still end in a matched pair before anything is re-run against it.
	a.transcript = []SessionMessage{{Role: "user", Text: "แบตผม 17"}}
	if _, ok := a.lastQuestion(); ok {
		t.Error("a lone user message was treated as a completed exchange")
	}

	a.transcript = append(a.transcript, SessionMessage{Role: "agent", Text: "ครับ"})
	question, ok := a.lastQuestion()
	if !ok || question != "แบตผม 17" {
		t.Errorf("lastQuestion = %q/%v; want the user's own words back", question, ok)
	}
}

func TestVariantEncodingRoundTrips(t *testing.T) {
	// One answer is not a set of alternates, and storing it as one would put a
	// JSON array on every row in every session for nothing.
	if got := encodeVariants([]SessionVariant{{Text: "only"}}); got != "" {
		t.Errorf("encodeVariants(single) = %q; want nothing stored", got)
	}

	variants := []SessionVariant{{Text: "หนึ่ง", ThinkSecs: 1}, {Text: "สอง", Reasoning: "why", ThinkSecs: 2}}
	decoded := decodeVariants(encodeVariants(variants))
	if len(decoded) != 2 {
		t.Fatalf("decoded %d variants; want 2", len(decoded))
	}
	if decoded[1].Text != "สอง" || decoded[1].Reasoning != "why" || decoded[1].ThinkSecs != 2 {
		t.Errorf("decoded[1] = %+v; want the Thai text and its reasoning intact", decoded[1])
	}

	// A row this build cannot read costs the switcher on one bubble, never the
	// ability to open the session.
	for _, broken := range []string{"", "   ", "not json", `{"text":"an object"}`, `[]`, `[{"text":"one"}]`} {
		if got := decodeVariants(broken); got != nil {
			t.Errorf("decodeVariants(%q) = %+v; want it ignored", broken, got)
		}
	}
}

// variantsOf has to invent the one-element list for a bubble answered once, or
// the first regenerate would drop the answer it is supposed to keep.
func TestVariantsOfPromotesASingleAnswer(t *testing.T) {
	got := variantsOf(SessionMessage{Role: "agent", Text: "คำตอบแรก", Reasoning: "r", ThinkSecs: 3})
	if len(got) != 1 || got[0].Text != "คำตอบแรก" || got[0].Reasoning != "r" || got[0].ThinkSecs != 3 {
		t.Fatalf("variantsOf = %+v; want the existing answer promoted intact", got)
	}

	existing := []SessionVariant{{Text: "a"}, {Text: "b"}}
	got = variantsOf(SessionMessage{Role: "agent", Text: "b", Variants: existing, Active: 1})
	if len(got) != 2 {
		t.Fatalf("variantsOf = %+v; want the stored list", got)
	}
	// A copy, not the slice itself: appending the new answer must not write
	// into the transcript's own backing array.
	got[0].Text = "mutated"
	if existing[0].Text != "a" {
		t.Error("variantsOf handed back the caller's own slice")
	}
}
