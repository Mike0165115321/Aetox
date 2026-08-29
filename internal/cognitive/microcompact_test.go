package cognitive

import (
	"context"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/think"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// The trigger path, end to end: one giant agentic turn — a single user
// message, then a long run of big reads — must cross the 60% line and get
// SWEPT mid-turn, and the swept markers must reach the provider as ordinary
// tool messages the loop keeps working through. This is the test that found
// the boundary bug: the sweep used the summarizer's user-turn boundary, and a
// single-turn conversation has none, so the exact session micro-compact was
// built for was the one it silently skipped.
func TestMicroCompactFiresMidTurnAndTheLoopSurvives(t *testing.T) {
	const rounds = 12
	responses := make([]model.Response, 0, rounds+1)
	for i := 0; i < rounds; i++ {
		responses = append(responses, model.Response{
			ToolCalls: []model.ToolCall{{
				ID: "call_" + strings.Repeat("x", i+1), Type: "function",
				// A different path each round, or the doom-loop guard (rightly)
			// stops the turn at five identical calls before the sweep line.
			Function: model.FunctionCall{Name: "read", Arguments: `{"path":"big` + strings.Repeat("g", i) + `.txt"}`},
			}},
		})
	}
	responses = append(responses, model.Response{Text: "done"})
	provider := &toolLoopProvider{responses: responses}

	agent := NewAgent(AgentConfig{
		Provider: provider, Model: "test-model", MaxToolCalls: rounds + 2,
		// Small on purpose: 12 reads of ~4KB cross 60% of 40KB fast, the way a
		// real session crosses 60% of a model window slowly.
		MaxChars: 40_000,
	})
	big := strings.Repeat("line of file content\n", 200)
	_, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"summarize the repo",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			return big, nil, nil
		},
		nil,
		turn.TurnOptions{ThinkLevel: think.LevelMedium},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	swept := 0
	for _, m := range agent.ContextMessages() {
		if m.Role == model.RoleTool && strings.HasPrefix(m.Content, "[cleared to save context]") {
			swept++
		}
	}
	if swept == 0 {
		t.Fatal("a single giant turn crossed the sweep line and nothing was swept")
	}
	// The provider must have SEEN swept markers as normal tool messages —
	// role and id intact — in some later request of the same turn.
	sawMarker := false
	for _, req := range provider.requests {
		for _, m := range req.Messages {
			if m.Role == model.RoleTool && strings.HasPrefix(m.Content, "[cleared to save context]") {
				if m.ToolCallID == "" {
					t.Fatal("a swept tool message lost its call id — providers reject that")
				}
				sawMarker = true
			}
		}
	}
	if !sawMarker {
		t.Fatal("swept markers never reached the provider, so the sweep saved nothing")
	}
	items, chars, _ := agent.MaintenanceStats()
	if items != swept || chars <= 0 {
		t.Fatalf("MaintenanceStats says %d/%d, context says %d swept", items, chars, swept)
	}
}
