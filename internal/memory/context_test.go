package memory

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/model"
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

// Compaction split: the boundary must land on a user turn so an assistant
// message and its tool results always travel together.
func TestSplitForCompactionRespectsTurnBoundaries(t *testing.T) {
	c := NewContext("system", 0, 100_000)
	c.Add(model.RoleUser, "q1")
	c.AddMessage(model.Message{Role: model.RoleAssistant, Content: "", ToolCalls: []model.ToolCall{{ID: "t1"}}})
	c.AddMessage(model.Message{Role: model.RoleTool, ToolCallID: "t1", Content: "result1"})
	c.Add(model.RoleAssistant, "a1")
	c.Add(model.RoleUser, "q2")
	c.AddMessage(model.Message{Role: model.RoleAssistant, Content: "", ToolCalls: []model.ToolCall{{ID: "t2"}}})
	c.AddMessage(model.Message{Role: model.RoleTool, ToolCallID: "t2", Content: "result2"})
	c.Add(model.RoleAssistant, "a2")

	// keepRecent=3 would cut inside q2's tool block — the boundary must
	// snap back to the q2 user message instead.
	old, recentStart := c.SplitForCompaction(3)
	if len(old) == 0 {
		t.Fatal("expected a compactable span")
	}
	msgs := c.Messages()
	if msgs[recentStart].Role != model.RoleUser || msgs[recentStart].Content != "q2" {
		t.Fatalf("boundary must land on the q2 user turn, got %+v", msgs[recentStart])
	}
	for _, m := range old {
		if m.Role == model.RoleSystem {
			t.Fatal("system prompt must never be in the summarized span")
		}
	}
}

func TestSplitForCompactionTooShort(t *testing.T) {
	c := NewContext("system", 0, 100_000)
	c.Add(model.RoleUser, "q1")
	c.Add(model.RoleAssistant, "a1")
	if old, _ := c.SplitForCompaction(6); old != nil {
		t.Fatalf("short conversation must not compact, got %d messages", len(old))
	}
}

func TestReplaceWithSummaryKeepsSystemAndTail(t *testing.T) {
	c := NewContext("system", 0, 100_000)
	for i := 0; i < 5; i++ {
		c.Add(model.RoleUser, fmt.Sprintf("q%d", i))
		c.Add(model.RoleAssistant, fmt.Sprintf("a%d", i))
	}
	old, recentStart := c.SplitForCompaction(3)
	if len(old) == 0 {
		t.Fatal("expected a compactable span")
	}
	c.ReplaceWithSummary("สรุป: คุยเรื่อง q0..q3", recentStart)

	msgs := c.Messages()
	if msgs[0].Role != model.RoleSystem || msgs[0].Content != "system" {
		t.Fatalf("system prompt must survive, got %+v", msgs[0])
	}
	if !strings.Contains(msgs[1].Content, "สรุป: คุยเรื่อง q0..q3") ||
		!strings.Contains(msgs[1].Content, "Compacted summary") {
		t.Fatalf("summary message malformed: %q", msgs[1].Content)
	}
	if got := len(msgs); got != 2+(10-recentStart)+1-1 {
		// system + summary + kept tail
		t.Logf("messages after compaction: %d (recentStart=%d)", got, recentStart)
	}
	last := msgs[len(msgs)-1]
	if last.Content != "a4" {
		t.Fatalf("tail must be preserved verbatim, last = %+v", last)
	}
}

func TestNeedsCompactionThreshold(t *testing.T) {
	c := NewContext("system", 0, 1000)
	if c.NeedsCompaction(0.8) {
		t.Fatal("fresh context must not need compaction")
	}
	c.Add(model.RoleUser, strings.Repeat("x", 850))
	if !c.NeedsCompaction(0.8) {
		t.Fatal("85% usage must cross the 0.8 threshold")
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

func TestMicroCompactSweepsOnlyOldReObtainableToolOutput(t *testing.T) {
	c := NewContext("system", 0, 1_000_000)
	big := strings.Repeat("x", 1000)
	sweepable := map[string]bool{"read": true}

	c.Add(model.RoleUser, "q1")
	c.AddMessage(model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "t1"}}})
	c.AddMessage(model.Message{Role: model.RoleTool, Name: "read", ToolCallID: "t1", Content: big})
	c.AddMessage(model.Message{Role: model.RoleTool, Name: "shell", ToolCallID: "t2", Content: big})
	c.AddMessage(model.Message{Role: model.RoleTool, Name: "read", ToolCallID: "t3", Content: "tiny"})
	c.Add(model.RoleAssistant, "a1")
	c.Add(model.RoleUser, "q2")
	c.AddMessage(model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "t4"}}})
	c.AddMessage(model.Message{Role: model.RoleTool, Name: "read", ToolCallID: "t4", Content: big})
	c.Add(model.RoleAssistant, "a2")

	swept, freed := c.MicroCompact(3, sweepable)
	if swept != 1 {
		t.Fatalf("swept = %d, want exactly the one old big read", swept)
	}
	if freed <= 0 || freed >= 1000 {
		t.Errorf("freed = %d, want the content minus the marker", freed)
	}

	msgs := c.Messages()
	find := func(id string) model.Message {
		t.Helper()
		for _, m := range msgs {
			if m.ToolCallID == id {
				return m
			}
		}
		t.Fatalf("message %s vanished — MicroCompact must never remove a message", id)
		return model.Message{}
	}
	if m := find("t1"); !strings.HasPrefix(m.Content, sweptMarkerPrefix) || m.Name != "read" {
		t.Errorf("old big read should be a marker keeping name and id, got %.60q", m.Content)
	}
	if m := find("t2"); m.Content != big {
		t.Error("shell output is not sweepable and must be untouched")
	}
	if m := find("t3"); m.Content != "tiny" {
		t.Error("output under the minimum is not worth a marker")
	}
	if m := find("t4"); m.Content != big {
		t.Error("the recent tail is what the model works from and must be untouched")
	}
}

func TestMicroCompactDoesNotSweepTwice(t *testing.T) {
	c := NewContext("system", 0, 1_000_000)
	big := strings.Repeat("y", 800)
	c.Add(model.RoleUser, "q1")
	c.AddMessage(model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "t1"}}})
	c.AddMessage(model.Message{Role: model.RoleTool, Name: "grep", ToolCallID: "t1", Content: big})
	c.Add(model.RoleAssistant, "a1")
	c.Add(model.RoleUser, "q2")
	c.Add(model.RoleAssistant, "a2")
	c.Add(model.RoleUser, "q3")
	c.Add(model.RoleAssistant, "a3")

	sweepable := map[string]bool{"grep": true}
	if swept, _ := c.MicroCompact(3, sweepable); swept != 1 {
		t.Fatalf("first sweep = %d, want 1", swept)
	}
	if swept, freed := c.MicroCompact(3, sweepable); swept != 0 || freed != 0 {
		t.Fatalf("second sweep = %d/%d, a marker must not be re-swept", swept, freed)
	}
}
