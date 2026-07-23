package memory

import (
	"fmt"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// No message-count cap: a long conversation within the char budget keeps
// every message (the old default silently trimmed at 80).
func TestContextKeepsLongConversationWithinCharBudget(t *testing.T) {
	c := NewContext("system", 0, 1_000_000)
	for i := 0; i < 150; i++ {
		c.Add(model.RoleUser, fmt.Sprintf("q%d", i))
		c.Add(model.RoleAssistant, fmt.Sprintf("a%d", i))
	}
	if got := len(c.Messages()); got != 1+300 {
		t.Fatalf("messages = %d, want 301 (no turn cap)", got)
	}
}

// The char budget is still the real brake: exceeding it drops the oldest
// assistant+tool turns while keeping the system prompt and first user message.
func TestContextCharBudgetStillTrims(t *testing.T) {
	c := NewContext("system", 0, 400)
	c.Add(model.RoleUser, "first question")
	for i := 0; i < 30; i++ {
		c.Add(model.RoleAssistant, fmt.Sprintf("answer %d padded to be long enough", i))
	}
	msgs := c.Messages()
	if len(msgs) >= 32 {
		t.Fatalf("messages = %d, want trimmed below 32", len(msgs))
	}
	if msgs[0].Role != model.RoleSystem || msgs[0].Content != "system" {
		t.Fatalf("system prompt must survive trimming, got %+v", msgs[0])
	}
	if msgs[1].Role != model.RoleUser || msgs[1].Content != "first question" {
		t.Fatalf("first user message must survive trimming, got %+v", msgs[1])
	}
}

// An explicit positive maxTurns still works for callers that want a hard cap.
func TestContextExplicitTurnCapStillEnforced(t *testing.T) {
	c := NewContext("system", 10, 1_000_000)
	for i := 0; i < 40; i++ {
		c.Add(model.RoleUser, fmt.Sprintf("m%d", i))
	}
	if got := len(c.Messages()); got > 10 {
		t.Fatalf("messages = %d, want <= 10 with explicit cap", got)
	}
}
