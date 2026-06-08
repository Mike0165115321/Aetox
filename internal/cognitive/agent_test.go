package cognitive

import (
	"context"
	"strings"
	"testing"

	"aetox-cli/internal/model"
	"aetox-cli/internal/think"
	"aetox-cli/internal/turn"
)

func TestRespondWithToolsContinuesAfterToolCall(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{
				ToolCalls: []model.ToolCall{
					{
						ID:   "call_read_1",
						Type: "function",
						Function: model.FunctionCall{
							Name:      "read",
							Arguments: `{"path":"note.txt"}`,
						},
					},
				},
			},
			{
				Text: "read note.txt: alpha",
			},
		},
	}
	agent := NewAgent(AgentConfig{
		Provider:     provider,
		Model:        "test-model",
		MaxToolCalls: 4,
	})

	reply, usedTools, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"read note.txt",
		func(_ context.Context, call model.ToolCall) (string, error) {
			if call.Function.Name != "read" {
				t.Fatalf("unexpected tool call: %s", call.Function.Name)
			}
			return `{"tool":"read","status":"done","output":"alpha"}`, nil
		},
		turn.TurnOptions{ThinkLevel: think.LevelMedium},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !usedTools {
		t.Fatal("expected tool usage")
	}
	if reply != "read note.txt: alpha" {
		t.Fatalf("expected final model reply, got %q", reply)
	}
	if provider.calls != 2 {
		t.Fatalf("expected provider to be called twice, got %d", provider.calls)
	}
	second := provider.requests[1]
	if len(second.Messages) < 4 {
		t.Fatalf("expected second request to include tool transcript, got %d messages", len(second.Messages))
	}
	var sawAssistantToolCall, sawToolResult bool
	for _, msg := range second.Messages {
		if msg.Role == model.RoleAssistant && len(msg.ToolCalls) == 1 && msg.ToolCalls[0].ID == "call_read_1" {
			sawAssistantToolCall = true
		}
		if msg.Role == model.RoleTool && msg.ToolCallID == "call_read_1" && strings.Contains(msg.Content, "alpha") {
			sawToolResult = true
		}
	}
	if !sawAssistantToolCall {
		t.Fatal("expected assistant tool call message in transcript")
	}
	if !sawToolResult {
		t.Fatal("expected tool result message in transcript")
	}
}

func TestRespondAttachesReasoningOnlyWhenProviderSupportsIt(t *testing.T) {
	supported := &toolLoopProvider{}
	agent := NewAgent(AgentConfig{
		Provider: supported,
		Model:    "test-model",
	})
	if _, err := agent.Respond(context.Background(), "hello", turn.TurnOptions{ThinkLevel: think.LevelHigh}); err != nil {
		t.Fatalf("respond failed: %v", err)
	}
	if len(supported.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(supported.requests))
	}
	if supported.requests[0].Reasoning == nil || supported.requests[0].Reasoning.Effort != "high" {
		t.Fatalf("expected high reasoning config, got %+v", supported.requests[0].Reasoning)
	}

	unsupported := &plainProvider{}
	agent = NewAgent(AgentConfig{
		Provider: unsupported,
		Model:    "test-model",
	})
	if _, err := agent.Respond(context.Background(), "hello", turn.TurnOptions{ThinkLevel: think.LevelLow}); err != nil {
		t.Fatalf("respond failed: %v", err)
	}
	if len(unsupported.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(unsupported.requests))
	}
	if unsupported.requests[0].Reasoning != nil {
		t.Fatalf("expected no reasoning config, got %+v", unsupported.requests[0].Reasoning)
	}
	profile := agent.ResolveThinkProfile(think.LevelLow)
	if !profile.Downgraded {
		t.Fatalf("expected downgraded profile, got %+v", profile)
	}
}

type toolLoopProvider struct {
	responses []model.Response
	requests  []model.Request
	calls     int
}

func (p *toolLoopProvider) Name() string { return "tool-loop-test" }

func (p *toolLoopProvider) SupportsToolCalling() bool { return true }

func (p *toolLoopProvider) SupportsReasoning() bool { return true }

func (p *toolLoopProvider) Complete(_ context.Context, req model.Request) (model.Response, error) {
	p.requests = append(p.requests, req)
	p.calls++
	if len(p.responses) == 0 {
		return model.Response{Text: "done"}, nil
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

type plainProvider struct {
	requests []model.Request
}

func (p *plainProvider) Name() string { return "plain" }

func (p *plainProvider) Complete(_ context.Context, req model.Request) (model.Response, error) {
	p.requests = append(p.requests, req)
	return model.Response{Text: "ok"}, nil
}
