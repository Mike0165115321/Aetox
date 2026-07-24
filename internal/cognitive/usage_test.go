package cognitive

import (
	"context"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// The usage reporter must fire once per API response — including every round
// of a tool loop, not just the final one — with the provider's real numbers.
// This is the contract the desktop's Usage stats page depends on.
func TestUsageReporterFiresPerToolLoopRound(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{
				ToolCalls: []model.ToolCall{{
					ID: "call_1", Type: "function",
					Function: model.FunctionCall{Name: "read", Arguments: `{"path":"a.txt"}`},
				}},
				Usage: &model.Usage{PromptTokens: 11, CompletionTokens: 3},
			},
			{
				Text:  "done",
				Usage: &model.Usage{PromptTokens: 29, CompletionTokens: 7},
			},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxToolCalls: 4})

	var got []model.Usage
	agent.SetUsageReporter(func(u model.Usage) { got = append(got, u) })

	_, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"read a.txt",
		func(_ context.Context, _ model.ToolCall) (string, error) { return "ok", nil },
		turn.TurnOptions{ThinkLevel: think.LevelMedium},
	)
	if err != nil {
		t.Fatalf("RespondWithTools: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("reporter fired %d times, want 2 (one per API round): %+v", len(got), got)
	}
	if got[0].PromptTokens != 11 || got[0].CompletionTokens != 3 ||
		got[1].PromptTokens != 29 || got[1].CompletionTokens != 7 {
		t.Fatalf("reporter received wrong numbers: %+v", got)
	}
}

// Respond (the non-tool path) reports too.
func TestUsageReporterFiresOnPlainRespond(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{{Text: "hi", Usage: &model.Usage{PromptTokens: 5, CompletionTokens: 2}}},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	var got []model.Usage
	agent.SetUsageReporter(func(u model.Usage) { got = append(got, u) })

	if _, err := agent.Respond(context.Background(), "hello", turn.TurnOptions{ThinkLevel: think.LevelLow}); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if len(got) != 1 || got[0].PromptTokens != 5 || got[0].CompletionTokens != 2 {
		t.Fatalf("reporter = %+v, want one report of 5/2", got)
	}
}
