package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/safety"
)

// Replace:true means "this is the whole answer, draw exactly this" — the bubble
// takes it verbatim, because on every path that produces one it is the finished
// reply. Anything the model streams on the way is a preview (Replace:false) and
// may be thrown away.
//
// The conversation path used to hand the delivery channel the token stream, so
// every single word arrived claiming to be the whole answer and the bubble
// finished holding whichever token happened to be last. A multi-paragraph
// markdown reply rendered as a stray "12" out of the middle of a table.
//
// Only a model that cannot call tools reaches that path — every real provider
// takes the tool loop — so this is the failure that shows up on Aetox's own
// test models and nowhere else, which is exactly the case §45 says they exist
// to cover.

func chunkRecorder(t *testing.T, modelName string) (*App, *[]chatChunk) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	var seen []chatChunk
	a := seed(&App{ctx: context.Background(), dbDir: t.TempDir()}, &conversation{id: newSessionID()})
	a.emit = func(event string, data ...any) {
		if event != "agent:chunk" || len(data) == 0 {
			return
		}
		// Every agent:* event carries the conversation it came from now
		// (desktop/conversation.go). The recorder asserts that too: a chunk
		// with no home is the bug this file's sibling tests are about, one
		// layer down.
		ev, ok := data[0].(sessionEvent[chatChunk])
		if !ok {
			t.Errorf("agent:chunk payload is %T, want a stamped sessionEvent", data[0])
			return
		}
		if ev.SessionID == "" {
			t.Error("agent:chunk arrived with no session on it")
		}
		seen = append(seen, ev.Data)
	}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     modelName,
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})
	return a, &seen
}

func TestAConversationTurnDeliversTheWholeAnswerExactlyOnce(t *testing.T) {
	a, seen := chunkRecorder(t, "aetox-render:test")

	reply, err := a.SendMessage("เทสๆ")
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	var deliveries []chatChunk
	for _, c := range *seen {
		if c.Replace && strings.TrimSpace(c.Text) != "" {
			deliveries = append(deliveries, c)
		}
	}
	if len(deliveries) != 1 {
		t.Fatalf("want exactly one delivery, got %d — every one of them claims to be the whole answer", len(deliveries))
	}

	// The delivery is the whole reply, not a fragment of it. Asserted on pieces
	// from the start, middle and end, because the symptom was a single token
	// from the middle surviving as the entire bubble.
	for _, want := range []string{"## ทดสอบ Markdown", "| คอลัมน์ |", "picsum.photos"} {
		if !strings.Contains(deliveries[0].Text, want) {
			t.Errorf("the delivered answer is missing %q:\n%s", want, deliveries[0].Text)
		}
	}
	if deliveries[0].Text != reply.Text {
		t.Error("the bubble was drawn from something other than what the turn returned")
	}
	// Newlines are the difference between a document and one long line: the
	// provider streams word by word, and a delivery assembled out of that stream
	// would have lost every one of them.
	if strings.Count(deliveries[0].Text, "\n") < 5 {
		t.Errorf("the delivery lost its line structure:\n%q", deliveries[0].Text)
	}
}

// The same guarantee on the path that already held it, so a future change
// cannot fix one and break the other.
func TestAToolTurnAlsoDeliversTheWholeAnswerExactlyOnce(t *testing.T) {
	a, seen := chunkRecorder(t, "aetox-tools:test")

	reply, err := a.SendMessage("memory: ทดสอบการส่งคำตอบ")
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	deliveries := 0
	for _, c := range *seen {
		if c.Replace && strings.TrimSpace(c.Text) != "" {
			deliveries++
			if c.Text != reply.Text {
				t.Errorf("a delivery that is not the returned reply:\n%q", c.Text)
			}
		}
	}
	if deliveries != 1 {
		t.Fatalf("want exactly one delivery, got %d", deliveries)
	}
}
