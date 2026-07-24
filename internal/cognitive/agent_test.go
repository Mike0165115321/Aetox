package cognitive

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"
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

func TestRespondSetsDeepSeekThinkingToggle(t *testing.T) {
	provider := &deepSeekLikeProvider{}
	agent := NewAgent(AgentConfig{
		Provider: provider,
		Model:    "deepseek-v4-flash",
	})

	if _, err := agent.Respond(context.Background(), "hello", turn.TurnOptions{ThinkLevel: think.LevelNoThinking}); err != nil {
		t.Fatalf("respond failed: %v", err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(provider.requests))
	}
	if provider.requests[0].Thinking == nil || provider.requests[0].Thinking.Type != "disabled" {
		t.Fatalf("expected disabled thinking config, got %+v", provider.requests[0].Thinking)
	}
	if provider.requests[0].Reasoning != nil {
		t.Fatalf("expected no reasoning config in off mode, got %+v", provider.requests[0].Reasoning)
	}
}

func TestRespondWithToolsSkipsTruncatedToolCall(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{
				FinishReason: model.FinishReasonLength,
				ToolCalls: []model.ToolCall{
					{
						ID:   "call_write_1",
						Type: "function",
						Function: model.FunctionCall{
							Name:      "write",
							Arguments: `{"path": "landing.html", "content": "<!DOCTYPE html>\n<html`, // cut mid-JSON
						},
					},
				},
			},
			{Text: "ok, shorter version written"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxToolCalls: 4})

	executed := 0
	reply, usedTools, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "write", Parameters: []byte(`{"type":"object"}`)}}},
		"make me a landing page",
		func(_ context.Context, _ model.ToolCall) (string, error) {
			executed++
			return "should never run", nil
		},
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if executed != 0 {
		t.Fatalf("truncated tool call must not execute, ran %d times", executed)
	}
	if !usedTools || reply != "ok, shorter version written" {
		t.Fatalf("expected final reply after truncation receipt, got %q (usedTools=%v)", reply, usedTools)
	}
	second := provider.requests[1]
	var sawTruncationReceipt bool
	for _, msg := range second.Messages {
		if msg.Role == model.RoleTool && msg.ToolCallID == "call_write_1" && strings.Contains(msg.Content, "truncated") {
			sawTruncationReceipt = true
		}
	}
	if !sawTruncationReceipt {
		t.Fatal("expected a truncation receipt tool message in the transcript")
	}
}

func TestRespondWithToolsStopsDoomLoop(t *testing.T) {
	same := model.Response{
		ToolCalls: []model.ToolCall{
			{
				ID:       "call_x",
				Type:     "function",
				Function: model.FunctionCall{Name: "write", Arguments: `{"path":"a.html","content":"x"}`},
			},
		},
	}
	provider := &toolLoopProvider{
		responses: []model.Response{same, same, same, same, same, same, same},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	executed := 0
	warned := false
	reply, usedTools, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "write", Parameters: []byte(`{"type":"object"}`)}}},
		"loop forever",
		func(_ context.Context, _ model.ToolCall) (string, error) {
			executed++
			return "same failure", nil
		},
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !usedTools {
		t.Fatal("expected tool usage")
	}
	if executed != doomLoopStop-1 {
		t.Fatalf("expected %d executions before the brake, got %d", doomLoopStop-1, executed)
	}
	if !strings.Contains(reply, "ซ้ำ") {
		t.Fatalf("expected doom-loop stop message, got %q", reply)
	}
	for _, req := range provider.requests {
		for _, msg := range req.Messages {
			if msg.Role == model.RoleTool && strings.Contains(msg.Content, "[loop warning]") {
				warned = true
			}
		}
	}
	if !warned {
		t.Fatalf("expected a [loop warning] nudge at %d repeats", doomLoopWarn)
	}
}

func TestRespondWithToolsSendsPerProviderMaxTokens(t *testing.T) {
	cases := []struct {
		provider  string
		modelName string
		want      int
	}{
		{"deepseek", "deepseek-chat", 8192},          // V3-era API max — larger values 400
		{"deepseek", "deepseek-v4-flash", 32000},     // V4 allows up to 384K output
		{"anthropic", "claude-sonnet-4-5", 32000},    // OUTPUT_TOKEN_MAX ceiling
		{"openai", "gpt-4o", 16384},                  // gpt-4o floor
		{"openrouter", "vendor/model", 8192},         // mixed routed models — conservative
		{"tool-loop-test", "m", 8192},                // unknown provider falls back safe
	}
	for _, tc := range cases {
		provider := &toolLoopProvider{name: tc.provider, responses: []model.Response{{Text: "done"}}}
		agent := NewAgent(AgentConfig{Provider: provider, Model: tc.modelName})

		if _, _, err := agent.RespondWithTools(
			context.Background(),
			[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
			"hello",
			func(_ context.Context, _ model.ToolCall) (string, error) { return "", nil },
			turn.TurnOptions{},
		); err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.provider, err)
		}
		if got := provider.requests[0].MaxTokens; got != tc.want {
			t.Errorf("%s: tool loop MaxTokens = %d, want %d", tc.provider, got, tc.want)
		}
	}
}

func TestRespondWithToolsLengthWithoutToolCallsReturnsText(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{Text: "partial but usable answer", FinishReason: model.FinishReasonLength},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	reply, usedTools, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"long question",
		func(_ context.Context, _ model.ToolCall) (string, error) { return "", nil },
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedTools {
		t.Fatal("no tools were called")
	}
	if reply != "partial but usable answer" {
		t.Fatalf("length without tool calls must return the text as-is, got %q", reply)
	}
}

func TestRespondWithToolsDoomLoopResetsOnDifferentCall(t *testing.T) {
	callA := model.Response{ToolCalls: []model.ToolCall{{
		ID: "a", Type: "function",
		Function: model.FunctionCall{Name: "read", Arguments: `{"path":"a.txt"}`},
	}}}
	callB := model.Response{ToolCalls: []model.ToolCall{{
		ID: "b", Type: "function",
		Function: model.FunctionCall{Name: "read", Arguments: `{"path":"b.txt"}`},
	}}}
	provider := &toolLoopProvider{
		// a,a,b,a,a: never doomLoopStop consecutive repeats — must run through
		responses: []model.Response{callA, callA, callB, callA, callA, {Text: "all done"}},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	executed := 0
	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"read some files",
		func(_ context.Context, _ model.ToolCall) (string, error) {
			executed++
			return "content", nil
		},
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if executed != 5 {
		t.Fatalf("interleaved calls must all execute, got %d of 5", executed)
	}
	if reply != "all done" {
		t.Fatalf("expected normal completion, got %q", reply)
	}
}

func TestCompactionSummarizesOldTurnsBeforeTheTurn(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{Text: "COMPACT-SUMMARY: user is building a landing page in Go"},
			{Text: "final answer"},
		},
	}
	// budget 5000 bytes; ~4050 bytes of history crosses the 0.8 threshold
	// (Thai chars are 3 bytes each) without tripping the hard trim
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxChars: 5000, SystemPrompt: "sys"})
	history := make([]model.Message, 0, 10)
	for i := 0; i < 5; i++ {
		history = append(history,
			model.Message{Role: model.RoleUser, Content: fmt.Sprintf("q%d %s", i, strings.Repeat("คำถาม ", 25))},
			model.Message{Role: model.RoleAssistant, Content: fmt.Sprintf("a%d %s", i, strings.Repeat("คำตอบ ", 25))},
		)
	}
	agent.RestoreHistory(history)

	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"คำถามใหม่ล่าสุด",
		func(_ context.Context, _ model.ToolCall) (string, error) { return "", nil },
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "final answer" {
		t.Fatalf("expected the turn to complete after compaction, got %q", reply)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected compaction call + turn call, got %d requests", len(provider.requests))
	}
	compactReq := provider.requests[0]
	if len(compactReq.Tools) != 0 || !strings.Contains(compactReq.Messages[0].Content, "compacting") {
		t.Fatalf("first call must be the tool-less compaction request, got %+v", compactReq.Messages[0])
	}
	turnReq := provider.requests[1]
	var sawSummary, sawOldContent bool
	for _, m := range turnReq.Messages {
		if strings.Contains(m.Content, "COMPACT-SUMMARY") {
			sawSummary = true
		}
		if strings.Contains(m.Content, "q0 ") {
			sawOldContent = true
		}
	}
	if !sawSummary {
		t.Fatal("turn request must carry the summary message")
	}
	if sawOldContent {
		t.Fatal("oldest turns must be gone from the turn request")
	}
	if last := turnReq.Messages[len(turnReq.Messages)-1]; last.Content != "คำถามใหม่ล่าสุด" {
		t.Fatalf("fresh question must be last and untouched, got %q", last.Content)
	}
}

func TestCompactionFailureIsNonFatal(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{Text: ""},             // summarizer returns nothing usable
			{Text: "still worked"}, // the actual turn
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxChars: 5000, SystemPrompt: "sys"})
	history := make([]model.Message, 0, 10)
	for i := 0; i < 5; i++ {
		history = append(history,
			model.Message{Role: model.RoleUser, Content: strings.Repeat("คำถาม ", 26)},
			model.Message{Role: model.RoleAssistant, Content: strings.Repeat("คำตอบ ", 26)},
		)
	}
	agent.RestoreHistory(history)

	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"ถามต่อ",
		func(_ context.Context, _ model.ToolCall) (string, error) { return "", nil },
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "still worked" {
		t.Fatalf("turn must proceed when compaction yields nothing, got %q", reply)
	}
}

type toolLoopProvider struct {
	name      string
	responses []model.Response
	requests  []model.Request
	calls     int
}

func (p *toolLoopProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "tool-loop-test"
}

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

type deepSeekLikeProvider struct {
	requests []model.Request
}

func (p *deepSeekLikeProvider) Name() string { return "deepseek" }

func (p *deepSeekLikeProvider) SupportsReasoning() bool { return true }

func (p *deepSeekLikeProvider) Complete(_ context.Context, req model.Request) (model.Response, error) {
	p.requests = append(p.requests, req)
	return model.Response{Text: "ok"}, nil
}
