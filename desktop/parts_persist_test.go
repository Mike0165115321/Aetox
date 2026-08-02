package main

import (
	"testing"

	"github.com/Mike0165115321/Aetox/internal/turn"
)

// The tool timeline used to be unstorable: `messages` had one text column, so
// reopening a session showed the sentence a turn ended on and nothing about the
// work that produced it. The sequence is what made it storable.
func TestATurnsSequenceSurvivesAReload(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	parts := []turn.TurnPart{
		{Kind: turn.PartText, Text: "กำลังอ่านไฟล์ให้ครับ"},
		{Kind: turn.PartThinking, Secs: 4},
		{Kind: turn.PartTool, Tool: &turn.ToolPart{
			Ref: "call_1", Name: "read", Subject: "note.txt", OK: true, Secs: 2,
		}},
		{Kind: turn.PartText, Text: "เจอแล้วครับ อยู่บรรทัดที่ 12"},
	}
	a.appendTurn(
		SessionMessage{Role: "user", Text: "อ่านไฟล์ให้หน่อย", Time: "00:34"},
		SessionMessage{Role: "agent", Text: "เจอแล้วครับ อยู่บรรทัดที่ 12", Time: "00:34", Parts: parts},
	)

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	reply := messages[1]
	if len(reply.Parts) != 4 {
		t.Fatalf("loaded %d parts; want the whole sequence", len(reply.Parts))
	}
	if reply.Parts[0].Kind != turn.PartText || reply.Parts[0].Text != "กำลังอ่านไฟล์ให้ครับ" {
		t.Errorf("parts[0] = %+v; want the narration first", reply.Parts[0])
	}
	if reply.Parts[1].Kind != turn.PartThinking || reply.Parts[1].Secs != 4 {
		t.Errorf("parts[1] = %+v; want the thinking segment", reply.Parts[1])
	}
	tool := reply.Parts[2].Tool
	if tool == nil || tool.Name != "read" || tool.Subject != "note.txt" || !tool.OK {
		t.Errorf("parts[2].tool = %+v; want the read call with its outcome", tool)
	}
	if reply.Parts[3].Text != "เจอแล้วครับ อยู่บรรทัดที่ 12" {
		t.Errorf("parts[3] = %+v; want the closing answer last", reply.Parts[3])
	}

	// Text still means what it always did, so search, session titles and the
	// context rebuild are untouched by any of this.
	if reply.Text != "เจอแล้วครับ อยู่บรรทัดที่ 12" {
		t.Errorf("text = %q; want the answer unchanged", reply.Text)
	}
	// And a user message has no sequence to load.
	if len(messages[0].Parts) != 0 {
		t.Error("the question was given a sequence; only answers have one")
	}
}

// Every message already on a user's disk was written before the sequence
// existed. Those must open as they always have, not fail to open.
func TestAMessageWithNoSequenceStillLoads(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "ถาม", Time: "00:34"},
		SessionMessage{Role: "agent", Text: "ตอบ", Time: "00:34"},
	)

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if messages[1].Parts != nil {
		t.Errorf("parts = %+v; want nil so the bubble falls back to plain text", messages[1].Parts)
	}
	if messages[1].Text != "ตอบ" {
		t.Errorf("text = %q", messages[1].Text)
	}
}

func TestPartsEncodingRoundTrips(t *testing.T) {
	if got := encodeParts(nil); got != "" {
		t.Errorf("encodeParts(nil) = %q; want nothing stored", got)
	}
	if got := decodeParts(""); got != nil {
		t.Errorf("decodeParts(\"\") = %+v; want nil", got)
	}
	// A row this build cannot read costs the sequence on one bubble, never the
	// ability to open the session.
	for _, broken := range []string{"   ", "not json", `{"kind":"text"}`} {
		if got := decodeParts(broken); got != nil {
			t.Errorf("decodeParts(%q) = %+v; want it ignored", broken, got)
		}
	}
}

// Answering again produces different work, so the record of that work has to
// move with it — otherwise the bubble would show the new answer beside the old
// answer's tool calls.
func TestRegeneratingReplacesTheStoredSequence(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "ถาม", Time: "00:34"},
		SessionMessage{Role: "agent", Text: "ตอบแรก", Time: "00:34", Parts: []turn.TurnPart{
			{Kind: turn.PartTool, Tool: &turn.ToolPart{Name: "read", OK: true}},
			{Kind: turn.PartText, Text: "ตอบแรก"},
		}},
	)

	a.storeParts([]turn.TurnPart{
		{Kind: turn.PartTool, Tool: &turn.ToolPart{Name: "grep", OK: true}},
		{Kind: turn.PartText, Text: "ตอบใหม่"},
	})

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	parts := messages[1].Parts
	if len(parts) != 2 || parts[0].Tool == nil || parts[0].Tool.Name != "grep" {
		t.Fatalf("stored sequence = %+v; want the re-run's own work", parts)
	}
}
