package cognitive

import (
	"context"
	"errors"
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
		func(_ context.Context, call model.ToolCall) (string, []model.Image, error) {
			if call.Function.Name != "read" {
				t.Fatalf("unexpected tool call: %s", call.Function.Name)
			}
			return `{"tool":"read","status":"done","output":"alpha"}`, nil, nil
		},
		nil,
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
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			executed++
			return "should never run", nil, nil
		},
		nil,
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
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			executed++
			return "same failure", nil, nil
		},
		nil,
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
		{"deepseek", "deepseek-chat", 8192},       // V3-era API max — larger values 400
		{"deepseek", "deepseek-v4-flash", 65536},  // V4 allows up to 384K output; big enough for a whole file in one call
		{"anthropic", "claude-sonnet-4-5", 32000}, // OUTPUT_TOKEN_MAX ceiling
		{"openai", "gpt-4o", 16384},               // gpt-4o floor
		{"openrouter", "vendor/model", 8192},      // mixed routed models — conservative
		{"tool-loop-test", "m", 8192},             // unknown provider falls back safe
	}
	for _, tc := range cases {
		provider := &toolLoopProvider{name: tc.provider, responses: []model.Response{{Text: "done"}}}
		agent := NewAgent(AgentConfig{Provider: provider, Model: tc.modelName})

		if _, _, err := agent.RespondWithTools(
			context.Background(),
			[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
			"hello",
			func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
			nil,
			turn.TurnOptions{},
		); err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.provider, err)
		}
		if got := provider.requests[0].MaxTokens; got != tc.want {
			t.Errorf("%s: tool loop MaxTokens = %d, want %d", tc.provider, got, tc.want)
		}
	}
}

// finish_reason "length" with no tool calls used to be handed over as the
// reply, on the reasoning that half an answer is better than none. It is, but
// it is not better than the whole one — and the user was reading answers that
// stopped in the middle of a word with no sign anything was missing and nothing
// anywhere asking for the rest (owner, 23 ส.ค.: *"โมเดลทำงานนานเกินจนระบบตัดอ่ะ
// ... รับไว้แล้วส่งไปใหม่ไม่ให้มันขาดตอน"*).
//
// The pieces are joined with nothing between them, because the limit cuts
// mid-token: "ตารางราคาจะเป็นแบบ" + "นี้ครับ" is one word, and a space or a
// newline inserted here would split it.
func TestATruncatedAnswerIsCarriedOnAndJoined(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{Text: "ครึ่งแรกของคำตอบที่ยังเขียนไม่", FinishReason: model.FinishReasonLength},
			{Text: "จบครับ"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	reply, usedTools, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"long question",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedTools {
		t.Fatal("no tools were called")
	}
	if reply != "ครึ่งแรกของคำตอบที่ยังเขียนไม่จบครับ" {
		t.Fatalf("reply = %q, want both halves joined with nothing between them", reply)
	}
	if provider.calls != 2 {
		t.Fatalf("provider was called %d times, want 2 (the cut answer and its continuation)", provider.calls)
	}
	// The model has to be able to see what it already wrote, or it starts over.
	second := provider.requests[1].Messages
	if second[len(second)-2].Content != "ครึ่งแรกของคำตอบที่ยังเขียนไม่" {
		t.Errorf("the continuation was asked without the half already written: %q",
			second[len(second)-2].Content)
	}
	if !strings.Contains(second[len(second)-1].Content, "cut off mid-answer") {
		t.Errorf("the continuation carried no instruction to carry on: %q", second[len(second)-1].Content)
	}
	// And history must hold each piece once. Storing the stitched version
	// alongside the piece it was stitched from would say the first half twice,
	// and the model reads that as having repeated itself.
	var stored []string
	for _, msg := range agent.context.Messages() {
		if msg.Role == model.RoleAssistant {
			stored = append(stored, msg.Content)
		}
	}
	if len(stored) != 2 || stored[0] != "ครึ่งแรกของคำตอบที่ยังเขียนไม่" || stored[1] != "จบครับ" {
		t.Errorf("history holds %q, want each piece exactly once", stored)
	}
}

// The half-answer must not outlive the answer it belongs to. A turn does not
// end at its first reply — a round that calls tools carries straight on, and an
// interjection keeps a finished one alive — so the pieces buffer has to be
// emptied the moment it is used. It was not, in the first draft of this: the
// continuation worked, and then the NEXT answer in the same turn arrived with
// the previous one's first half glued to its front.
func TestTheCarriedHalfDoesNotFollowTheNextAnswer(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{Text: "ครึ่งแรก", FinishReason: model.FinishReasonLength},
			{Text: "ครึ่งหลัง", ToolCalls: []model.ToolCall{{
				ID: "1", Type: "function",
				Function: model.FunctionCall{Name: "read", Arguments: `{"path":"a.md"}`},
			}}},
			{Text: "คำตอบสุดท้าย"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"long question",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			return "อ่านแล้ว", nil, nil
		},
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "คำตอบสุดท้าย" {
		t.Errorf("reply = %q, want the last answer alone, with no half of the first one on the front", reply)
	}
}

// Bounded, and the bound is not the same judgement as the retries next to it:
// a continuation is something that is WORKING, so the cap is only there to stop
// a model that answers every continuation with a fresh essay. Past it the user
// gets everything written so far rather than an error, because what is in hand
// is a real answer that happens to be unfinished.
func TestAnAnswerThatNeverStopsGrowingIsStillHandedOver(t *testing.T) {
	cut := model.Response{Text: "ยาว", FinishReason: model.FinishReasonLength}
	provider := &toolLoopProvider{responses: []model.Response{cut, cut, cut, cut, cut, cut}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"long question",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("an answer that kept growing was reported as a failure: %v", err)
	}
	if provider.calls != 1+maxAnswerContinuations {
		t.Fatalf("provider was called %d times, want %d (the answer plus its continuations)",
			provider.calls, 1+maxAnswerContinuations)
	}
	if reply != strings.Repeat("ยาว", 1+maxAnswerContinuations) {
		t.Errorf("reply = %q, want every piece that was written", reply)
	}
}

func TestRespondWithToolsLeakedDSMLNeverSurfacesRawMarkup(t *testing.T) {
	leak := model.Response{Text: "ขอโทษครับ\n<｜DSML｜invoke name=\"write\">\n" +
		"<｜DSML｜parameter name=\"file_path\" string=\"true\">phone.html</｜DSML｜parameter>\n" +
		"<｜DSML｜parameter name=\"content\" string=\"true\">"}
	// Always leaks: initial + maxDSMLNudges retries, then the fallback fires.
	provider := &toolLoopProvider{responses: []model.Response{leak, leak, leak}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash"})

	reply, usedTools, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "write", Parameters: []byte(`{"type":"object"}`)}}},
		"write phone.html",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedTools {
		t.Fatal("no real tool call was ever produced")
	}
	if strings.Contains(reply, "DSML") {
		t.Fatalf("raw markup must never surface to the user, got %q", reply)
	}
	if reply != dsmlLeakFallback {
		t.Fatalf("expected leak fallback, got %q", reply)
	}
	if len(provider.requests) != 1+maxDSMLNudges {
		t.Fatalf("expected %d requests (initial + %d nudges), got %d", 1+maxDSMLNudges, maxDSMLNudges, len(provider.requests))
	}
}

func TestRespondWithToolsStreamsReasoningWhenHandlerPresent(t *testing.T) {
	provider := &streamingToolLoopProvider{
		responses: []model.Response{{Text: "คำตอบสุดท้าย", ReasoningContent: "กำลังคิด"}},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash"})

	var reasoning []string
	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"hi",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		func(chunk string) error { reasoning = append(reasoning, chunk); return nil },
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.streamCalls == 0 || provider.completeCalls != 0 {
		t.Fatalf("tool loop must stream when a reasoning handler is present (stream=%d complete=%d)", provider.streamCalls, provider.completeCalls)
	}
	if len(reasoning) != 1 || reasoning[0] != "กำลังคิด" {
		t.Fatalf("reasoning must reach the handler live, got %#v", reasoning)
	}
	if reply != "คำตอบสุดท้าย" {
		t.Fatalf("unexpected reply %q", reply)
	}
}

func TestRespondWithToolsUsesCompleteWithoutReasoningHandler(t *testing.T) {
	provider := &streamingToolLoopProvider{responses: []model.Response{{Text: "done"}}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash"})

	if _, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"hi",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil, // no reasoning UI (e.g. CLI) — must stay on the non-streaming path
		turn.TurnOptions{},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.completeCalls == 0 || provider.streamCalls != 0 {
		t.Fatalf("no reasoning handler must keep Complete (stream=%d complete=%d)", provider.streamCalls, provider.completeCalls)
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
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			executed++
			return "content", nil, nil
		},
		nil,
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
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
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
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "still worked" {
		t.Fatalf("turn must proceed when compaction yields nothing, got %q", reply)
	}
}

func TestRespondWithToolsEmptyReplyNudgeKeepsTools(t *testing.T) {
	toolDefs := []model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read_image", Parameters: []byte(`{"type":"object"}`)}}}
	execTool := func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
		return "image says hi", nil, nil
	}

	provider := &toolLoopProvider{responses: []model.Response{{}, {Text: "recovered"}}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxToolCalls: 4})
	reply, _, err := agent.RespondWithTools(context.Background(), toolDefs, "อ่านภาพนี้", execTool, nil, turn.TurnOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "recovered" {
		t.Fatalf("expected nudged reply, got %q", reply)
	}
	if provider.calls != 2 {
		t.Fatalf("expected nudge round-trip, got %d calls", provider.calls)
	}
	nudgeReq := provider.requests[1]
	if len(nudgeReq.Tools) == 0 {
		t.Fatal("nudge round must keep tools so the model can use a skill instead of refusing")
	}
	last := nudgeReq.Messages[len(nudgeReq.Messages)-1]
	if last.Content != emptyReplyNudge {
		t.Fatalf("expected nudge as last message, got %q", last.Content)
	}

	provider = &toolLoopProvider{responses: []model.Response{{}, {}}}
	agent = NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxToolCalls: 4})
	reply, _, err = agent.RespondWithTools(context.Background(), toolDefs, "อ่านภาพนี้", execTool, nil, turn.TurnOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != emptyReplyFallback {
		t.Fatalf("expected fallback, got %q", reply)
	}
}

func TestRespondRecoversFromEmptyReply(t *testing.T) {
	provider := &toolLoopProvider{responses: []model.Response{{}, {Text: "recovered"}}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})
	reply, err := agent.Respond(context.Background(), "hello", turn.TurnOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "recovered" {
		t.Fatalf("expected nudged reply, got %q", reply)
	}
	if provider.calls != 2 {
		t.Fatalf("expected nudge round-trip, got %d calls", provider.calls)
	}
	second := provider.requests[1].Messages
	if second[len(second)-1].Content != emptyReplyNudge {
		t.Fatalf("expected nudge as last message, got %q", second[len(second)-1].Content)
	}
	for _, m := range agent.context.Messages() {
		if m.Content == emptyReplyNudge {
			t.Fatal("nudge must not persist in context")
		}
	}

	provider = &toolLoopProvider{responses: []model.Response{{}, {}}}
	agent = NewAgent(AgentConfig{Provider: provider, Model: "test-model"})
	reply, err = agent.Respond(context.Background(), "hello", turn.TurnOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != emptyReplyFallback {
		t.Fatalf("expected fallback, got %q", reply)
	}
}

type toolLoopProvider struct {
	name      string
	responses []model.Response
	requests  []model.Request
	calls     int
	// beforeReply runs as this call is being served, so a test can act at the
	// one moment that is otherwise unreachable: while the model is producing a
	// response. Interjection needs it — the user types mid-answer.
	beforeReply func(call int)
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
	if p.beforeReply != nil {
		p.beforeReply(p.calls)
	}
	if len(p.responses) == 0 {
		return model.Response{Text: "done"}, nil
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

// streamingToolLoopProvider records whether the loop used StreamComplete vs
// Complete, and emits reasoning through onReasoningChunk when streamed.
type streamingToolLoopProvider struct {
	responses     []model.Response
	streamCalls   int
	completeCalls int
}

func (p *streamingToolLoopProvider) Name() string              { return "deepseek" }
func (p *streamingToolLoopProvider) SupportsToolCalling() bool { return true }
func (p *streamingToolLoopProvider) SupportsReasoning() bool   { return true }

func (p *streamingToolLoopProvider) next() model.Response {
	if len(p.responses) == 0 {
		return model.Response{Text: "done"}
	}
	r := p.responses[0]
	p.responses = p.responses[1:]
	return r
}

func (p *streamingToolLoopProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	p.completeCalls++
	return p.next(), nil
}

func (p *streamingToolLoopProvider) StreamComplete(_ context.Context, _ model.Request, _ model.StreamChunkHandler, onReasoningChunk model.StreamChunkHandler) (model.Response, error) {
	p.streamCalls++
	resp := p.next()
	if onReasoningChunk != nil && resp.ReasoningContent != "" {
		if err := onReasoningChunk(resp.ReasoningContent); err != nil {
			return model.Response{}, err
		}
	}
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

// RespondEphemeral must never write into conversation history — the whole
// point is keeping summary prompts (with kilobytes of tool output) out of the
// session transcript.
func TestRespondEphemeralDoesNotTouchContext(t *testing.T) {
	provider := &toolLoopProvider{responses: []model.Response{{Text: "summary text"}}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})
	before := len(agent.ContextMessages())

	reply, err := agent.RespondEphemeral(context.Background(), "summarize this tool run", turn.TurnOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "summary text" {
		t.Fatalf("expected provider reply, got %q", reply)
	}
	if got := len(agent.ContextMessages()); got != before {
		t.Fatalf("ephemeral respond leaked into context: %d messages before, %d after", before, got)
	}
	// The prompt itself must still have reached the provider (as the last message).
	req := provider.requests[0]
	last := req.Messages[len(req.Messages)-1]
	if last.Role != model.RoleUser || !strings.Contains(last.Content, "summarize this tool run") {
		t.Fatalf("prompt did not reach provider, last message: %+v", last)
	}
}

// failFirstProvider refuses the tool block on the first Complete and answers
// normally after — the tool loop's first-call failure path falls back to a
// plain response.
//
// The refusal has to be a real one. The fallback is no longer reached by any
// first-round error, only by one that names the tools as the problem
// (model.IsToolBlockRejection), so an error that says "boom" now ends the turn
// here instead of falling through — which is the point of this test's own
// subject: what the fallback does to context WHEN it runs.
type failFirstProvider struct {
	calls    int
	requests []model.Request
}

func (p *failFirstProvider) Name() string              { return "fail-first" }
func (p *failFirstProvider) SupportsToolCalling() bool { return true }
func (p *failFirstProvider) Complete(_ context.Context, req model.Request) (model.Response, error) {
	p.calls++
	p.requests = append(p.requests, req)
	if p.calls == 1 {
		return model.Response{}, fmt.Errorf("registry.ollama.ai/library/gemma3: does not support tools")
	}
	return model.Response{Text: "fallback answer"}, nil
}

// Regression: the first-call-failure fallback used to call Respond(msg), which
// added the user message to context a second time.
func TestToolLoopFirstCallFailureDoesNotDuplicateUserMessage(t *testing.T) {
	provider := &failFirstProvider{}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	reply, usedTools, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"hello world",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedTools {
		t.Fatal("no tool ran — usedTools must be false")
	}
	if reply != "fallback answer" {
		t.Fatalf("expected fallback answer, got %q", reply)
	}
	userCount := 0
	for _, m := range agent.ContextMessages() {
		if m.Role == model.RoleUser && m.Content == "hello world" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("user message must appear exactly once in context, found %d", userCount)
	}
	// The fallback request must also carry exactly one copy.
	fallbackReq := provider.requests[len(provider.requests)-1]
	reqCount := 0
	for _, m := range fallbackReq.Messages {
		if m.Role == model.RoleUser && m.Content == "hello world" {
			reqCount++
		}
	}
	if reqCount != 1 {
		t.Fatalf("fallback request must carry the user message exactly once, found %d", reqCount)
	}
}

// alwaysFailsProvider is the provider on an account that has run out: every
// call is the same wall, because the wall is the answer.
type alwaysFailsProvider struct {
	calls int
	err   error
}

func (p *alwaysFailsProvider) Name() string              { return "codex" }
func (p *alwaysFailsProvider) SupportsToolCalling() bool { return true }
func (p *alwaysFailsProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	p.calls++
	return model.Response{}, p.err
}

// A provider that has stated its final answer is asked once, not twice.
//
// The measured case (2026-08-22): "codex: the free plan's limit is used up. It
// resets in 19 days." One question cost two full-context calls here, and the
// transport under each spends three attempts on a 429 — six requests to a
// backend that had already said no, plus six seconds of backoff the user waited
// through with "กำลังคิดคำตอบ..." on screen and nothing in the log to explain it.
func TestAWallIsNotAskedTwice(t *testing.T) {
	for _, wall := range []string{
		"codex: the free plan's limit is used up. It resets in 19 days.",
		"deepseek rejected the sign-in. Sign in again. (401: invalid api key)",
		"z.ai says this account is out of credits, so waiting will not help. (429: insufficient balance)",
		"Post \"https://api.openai.com/v1/responses\": dial tcp: lookup api.openai.com: no such host",
	} {
		provider := &alwaysFailsProvider{err: errors.New(wall)}
		agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

		reply, _, err := agent.RespondWithTools(
			context.Background(),
			[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
			"เช็คโค้ดให้ที",
			func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
			nil,
			turn.TurnOptions{},
		)
		if err == nil {
			t.Errorf("%.40s...: turn succeeded against a wall, replying %q", wall, reply)
		}
		if provider.calls != 1 {
			t.Errorf("%.40s...: asked %d times; the first answer was the whole answer", wall, provider.calls)
		}
	}
}

// blipThenSilence is the failure this cost the most on: nothing is wrong with
// the account or the request, the connection was gone for a moment, and the
// model has nothing to say when asked again.
type blipThenSilence struct {
	calls int
}

func (p *blipThenSilence) Name() string              { return "deepseek" }
func (p *blipThenSilence) SupportsToolCalling() bool { return true }
func (p *blipThenSilence) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	p.calls++
	if p.calls == 1 {
		return model.Response{}, errors.New("context deadline exceeded (Client.Timeout exceeded while awaiting headers)")
	}
	return model.Response{Text: ""}, nil
}

// A dropped connection is never reported as the model not being good enough.
//
// This is what the old blanket fallback did with a blip: it asked again without
// tools, got silence, asked a THIRD time, and returned emptyReplyFallback with a
// nil error — so the turn was stored as a success carrying "ลองแบ่งงานให้เล็กลง
// หรือเปลี่ยนโมเดล". No red bubble, no ลองใหม่ button, and the user's next move
// was to go pay for a bigger model to fix their Wi-Fi.
func TestABlipIsNotReportedAsTheModelsFault(t *testing.T) {
	provider := &blipThenSilence{}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash"})

	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"อ่านไฟล์ app.go ให้ที",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{},
	)
	if err == nil {
		t.Fatalf("a timeout came back as a successful turn, replying %q", reply)
	}
	if reply == emptyReplyFallback {
		t.Error("a network failure was dressed up as the model running out of room")
	}
	if provider.calls != 1 {
		t.Errorf("one timeout cost %d calls; the answer to a blip is a ลองใหม่ button, not a second bill", provider.calls)
	}
}

// Long tool loops must compact mid-turn (OpenCode checks per step) — without
// this, a single mega-loop overflows the char budget and old turns get dropped
// verbatim instead of summarized.
func TestToolLoopCompactsMidLoop(t *testing.T) {
	provider := &toolLoopProvider{responses: []model.Response{
		// round 1: a tool call whose result inflates the context past 80%
		{ToolCalls: []model.ToolCall{{ID: "c1", Type: "function",
			Function: model.FunctionCall{Name: "read", Arguments: `{"path":"big.txt"}`}}}},
		// (compaction summary request consumes the next scripted response)
		{Text: "summary of earlier turns"},
		// round 2: final text
		{Text: "done"},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "m", MaxChars: 5000})
	// Seed enough history that SplitForCompaction has something to fold
	// (it needs keepRecent+4 messages) without tripping the hard char trim.
	seed := make([]model.Message, 0, 8)
	for i := 0; i < 4; i++ {
		seed = append(seed,
			model.Message{Role: model.RoleUser, Content: strings.Repeat("old question ", 20)},
			model.Message{Role: model.RoleAssistant, Content: strings.Repeat("old answer ", 20)},
		)
	}
	agent.RestoreHistory(seed)

	_, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"read big.txt",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			return strings.Repeat("huge tool output ", 150), nil, nil // ~2550 chars → history ~4500 crosses 80% of 5000
		},
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sawCompaction := false
	for _, req := range provider.requests {
		if len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "compacting a long conversation") {
			sawCompaction = true
		}
	}
	if !sawCompaction {
		t.Fatal("expected a mid-loop compaction request to the provider")
	}
}

// Plain conversation replies use the same per-provider output ceiling as the
// tool loop — the old flat 768 truncated long answers mid-sentence.
func TestRespondUsesPerProviderOutputCeiling(t *testing.T) {
	provider := &toolLoopProvider{responses: []model.Response{{Text: "hi"}}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "m"})
	if _, err := agent.Respond(context.Background(), "hello", turn.TurnOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := provider.requests[0].MaxTokens
	if got == 768 {
		t.Fatal("conversation replies must not be capped at the old flat 768")
	}
	if want := agent.toolLoopMaxTokens(); got != want {
		t.Fatalf("MaxTokens = %d, want per-provider ceiling %d", got, want)
	}
}

// Typing while the agent is working must reach the model on its next round, not
// after the turn ends. The owner asked for this twice; the composer used to park
// the text in a queue and fire it as a fresh turn once the engine was free, which
// is the behaviour these two tests exist to prevent coming back.

// The common case: the user types while a tool is running.
func TestInterjectionReachesTheModelOnTheNextRound(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{ToolCalls: []model.ToolCall{{ID: "c1", Type: "function", Function: model.FunctionCall{Name: "read", Arguments: `{}`}}}},
			{Text: "final answer"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", SystemPrompt: "sys"})

	const typed = "จริง ๆ แล้วดูไฟล์อีกอันแทน"
	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"อ่านไฟล์นี้ให้หน่อย",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			// The tool is running — exactly when a user gets bored and types.
			agent.Interject(typed)
			return "file contents", nil, nil
		},
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "final answer" {
		t.Fatalf("reply = %q", reply)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected two rounds, got %d", len(provider.requests))
	}

	// Round 2 must carry it, and it must come after the tool result — the model
	// reads the new instruction knowing what the tool already returned.
	second := provider.requests[1].Messages
	typedAt, toolAt := -1, -1
	for i, m := range second {
		switch {
		case m.Role == model.RoleUser && strings.Contains(m.Content, typed):
			typedAt = i
		case m.Role == model.RoleTool:
			toolAt = i
		}
	}
	if typedAt < 0 {
		t.Fatalf("the interjection never reached the model: %+v", second)
	}
	if toolAt < 0 || typedAt < toolAt {
		t.Errorf("the interjection landed at %d, before the tool result at %d", typedAt, toolAt)
	}
	// Unmarked, this is indistinguishable from a message typed before the turn
	// started — and the model would have no reason to keep doing what it was
	// doing. The note says which of the two this is, and what to default to.
	if !strings.Contains(second[typedAt].Content, "WHILE you were working") {
		t.Errorf("the interjection arrived unmarked, so the model cannot tell it came mid-work: %q", second[typedAt].Content)
	}
}

// The case that would otherwise lose it: the model had already decided to stop,
// so there is no next round to fold it into. The turn has to keep going instead.
func TestInterjectionKeepsAFinishingTurnAlive(t *testing.T) {
	const typed = "เดี๋ยว อีกเรื่องนึง"
	var agent *Agent
	provider := &toolLoopProvider{
		responses: []model.Response{{Text: "first answer"}, {Text: "second answer"}},
		beforeReply: func(call int) {
			if call == 1 {
				// Typed while the model was writing an answer with no tool calls in
				// it — the loop is about to return.
				agent.Interject(typed)
			}
		},
	}
	agent = NewAgent(AgentConfig{Provider: provider, Model: "test-model", SystemPrompt: "sys"})

	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"ถามอะไรสักอย่าง",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "second answer" {
		t.Fatalf("the turn ended instead of picking the message up: reply = %q", reply)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected the turn to continue into a second round, got %d", len(provider.requests))
	}
	// What it already said stays in context, so the second round builds on it.
	var sawFirst, sawTyped bool
	for _, m := range provider.requests[1].Messages {
		if m.Role == model.RoleAssistant && strings.Contains(m.Content, "first answer") {
			sawFirst = true
		}
		if m.Role == model.RoleUser && strings.Contains(m.Content, typed) {
			sawTyped = true
		}
	}
	if !sawFirst {
		t.Error("the answer it had already given was dropped from context")
	}
	if !sawTyped {
		t.Error("the interjection never reached the second round")
	}
	// Nothing is left over for the host to pick up — it was delivered, not parked.
	if left := agent.DrainInterjections(); len(left) != 0 {
		t.Errorf("interjection was left in the buffer: %v", left)
	}
}

// The round that keeps the turn alive has to say WHAT it is handing over. Only
// here is it still knowable: a round with no tool calls is a round the model
// meant to stop on, and one step downstream a demoted answer and a line of
// narration are the same non-final text. The executor drew the first as the
// second, which cost the user a finished reply's markdown, its size and its
// place in the bubble.
func TestAnAnswerHeldOpenByAnInterjectionIsHandedOverAsAnAnswer(t *testing.T) {
	var agent *Agent
	provider := &toolLoopProvider{
		responses: []model.Response{{Text: "## สรุป\n\nเรียบร้อยครับ"}, {Text: "เช็คเพิ่มแล้วครับ"}},
		beforeReply: func(call int) {
			if call == 1 {
				agent.Interject("เช็คเรื่อง memory ด้วย")
			}
		},
	}
	agent = NewAgent(AgentConfig{Provider: provider, Model: "test-model", SystemPrompt: "sys"})

	var rounds []turn.RoundEvent
	if _, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"เช็คหน่อย",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{OnRound: func(r turn.RoundEvent) { rounds = append(rounds, r) }},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rounds) != 2 {
		t.Fatalf("OnRound calls = %d; want one per round", len(rounds))
	}
	if !rounds[0].Demoted {
		t.Errorf("the answer the user typed over went out as plain narration: %+v", rounds[0])
	}
	if rounds[0].Final {
		t.Errorf("a demoted answer is not the turn's reply: %+v", rounds[0])
	}
	if rounds[1].Demoted || !rounds[1].Final {
		t.Errorf("the answer that ended the turn = %+v; want the final one, undemoted", rounds[1])
	}
}

// Several messages typed under one turn all arrive, in the order they were sent —
// a user firing off three corrections must not have two of them dropped, or get
// them shuffled into an order that reverses their own decisions.
func TestInterjectionsArriveTogetherInOrder(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{ToolCalls: []model.ToolCall{{ID: "c1", Type: "function", Function: model.FunctionCall{Name: "read", Arguments: `{}`}}}},
			{Text: "final answer"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", SystemPrompt: "sys"})

	_, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"งานหลัก",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			agent.Interject("อันแรก")
			agent.Interject("   ") // blank: nothing to say, must not become a message
			agent.Interject("อันสอง")
			agent.Interject("อันสาม")
			return "file contents", nil, nil
		},
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []string
	for _, m := range provider.requests[1].Messages {
		if m.Role != model.RoleUser {
			continue
		}
		for _, want := range []string{"อันแรก", "อันสอง", "อันสาม"} {
			if strings.Contains(m.Content, want) {
				got = append(got, want)
			}
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected all three interjections, got %v", got)
	}
	for i, want := range []string{"อันแรก", "อันสอง", "อันสาม"} {
		if got[i] != want {
			t.Fatalf("order was scrambled: %v", got)
		}
	}
	// A blank interjection is not a message — it would cost a whole round asking
	// the model to respond to nothing.
	for _, m := range provider.requests[1].Messages {
		if m.Role == model.RoleUser && strings.HasSuffix(strings.TrimSpace(m.Content), "]") {
			t.Errorf("a blank interjection became a message: %q", m.Content)
		}
	}
	// Delivered means gone: a second drain must not hand them out again, or the
	// host would re-send messages the model has already answered.
	if left := agent.DrainInterjections(); len(left) != 0 {
		t.Errorf("interjections were delivered twice: %v", left)
	}
}

// A cancelled turn returns before the loop reaches its drain, so the message is
// still in the buffer — and the host has to be able to see it, because that is how
// Stop knows there is something to throw away (desktop App.CancelTurn) and how a
// straggler gets picked up at all.
func TestACancelledTurnLeavesTheInterjectionForTheHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &toolLoopProvider{
		responses: []model.Response{
			{ToolCalls: []model.ToolCall{{ID: "c1", Type: "function", Function: model.FunctionCall{Name: "read", Arguments: `{}`}}}},
			{Text: "never reached"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", SystemPrompt: "sys"})

	_, _, err := agent.RespondWithTools(
		ctx,
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"งานหลัก",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			agent.Interject("พิมพ์แล้วกด Stop")
			cancel() // the user's brake, hit right after typing
			return "file contents", nil, nil
		},
		nil,
		turn.TurnOptions{},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the turn to stop, got err=%v", err)
	}
	if len(provider.requests) != 1 {
		t.Errorf("the loop ran another round after Stop: %d requests", len(provider.requests))
	}
	left := agent.DrainInterjections()
	if len(left) != 1 || !strings.Contains(left[0], "Stop") {
		t.Fatalf("the message was swallowed instead of handed back: %v", left)
	}
	// And the note is NOT baked in by Interject — it is added where the message
	// joins the context, so what the host gets back is the user's own text, fit to
	// re-send as its own turn.
	if strings.Contains(left[0], "[system]") {
		t.Errorf("the mid-work note leaked into what the host takes back: %q", left[0])
	}
}

// Nothing typed, nothing to hand over — the host calls this after every turn, so
// the quiet path has to be the cheap one.
func TestDrainInterjectionsIsEmptyWhenNobodyTyped(t *testing.T) {
	agent := NewAgent(AgentConfig{Provider: &toolLoopProvider{}, Model: "test-model"})
	if left := agent.DrainInterjections(); left != nil {
		t.Errorf("expected nil, got %v", left)
	}
	var nilAgent *Agent
	nilAgent.Interject("must not panic")
	if left := nilAgent.DrainInterjections(); left != nil {
		t.Errorf("nil agent returned %v", left)
	}
}

// The note is a prompt, so it is asserted like the repo asserts its other prompts
// (see TestTaskDescriptionSaysWhenNotToDelegate): the point is not the wording but
// that all three dispositions survive an edit. Losing one silently collapses the
// feature back to "the model guesses", which is what the note exists to replace.
func TestTheMidWorkNoteLeavesTheChoiceToTheModel(t *testing.T) {
	for _, want := range []string{
		"WHILE you were working", // the fact it cannot infer
		"do it now",              // small enough to fold in
		"adjust course",          // a change to the job in hand
		"finish what you are already doing first", // separate or larger
		"Only drop the current work if",           // the one hard rule
		"acknowledge it",                          // the user has to see it landed
	} {
		if !strings.Contains(interjectionNote, want) {
			t.Errorf("the mid-work note no longer says %q:\n%s", want, interjectionNote)
		}
	}
	// It must not decide for the model — a single hard-coded disposition is the
	// §17 mistake this wording was rewritten to undo.
	for _, forbidden := range []string{"always treat", "ignore it until"} {
		if strings.Contains(strings.ToLower(interjectionNote), forbidden) {
			t.Errorf("the note pre-judges with %q", forbidden)
		}
	}
}

// §59: each round of the loop reports its assistant text through
// TurnOptions.OnRound — narration on a tool round, Final on the round that
// ends the turn — so the UI can interleave the model's own words into the
// timeline instead of discarding them.
func TestRespondWithToolsReportsEachRound(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{
				Text: "แวะดู config ก่อน",
				ToolCalls: []model.ToolCall{{
					ID:   "call_read_1",
					Type: "function",
					Function: model.FunctionCall{
						Name:      "read",
						Arguments: `{"path":"config.json"}`,
					},
				}},
			},
			{Text: "config เรียบร้อยดี"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxToolCalls: 4})

	var rounds []turn.RoundEvent
	_, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"ดู config ให้หน่อย",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			return "ok", nil, nil
		},
		nil,
		turn.TurnOptions{OnRound: func(r turn.RoundEvent) { rounds = append(rounds, r) }},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rounds) != 2 {
		t.Fatalf("OnRound calls = %d, want 2 (one per round)", len(rounds))
	}
	if rounds[0].Final || rounds[0].Text != "แวะดู config ก่อน" {
		t.Errorf("round 1 = %+v, want its narration, not Final", rounds[0])
	}
	if !rounds[1].Final || rounds[1].Text != "config เรียบร้อยดี" {
		t.Errorf("round 2 = %+v, want the reply marked Final", rounds[1])
	}
}

// The link the turn executor reaches this agent through. It is a type
// assertion on the far side (turn cannot import cognitive — cognitive imports
// turn), so nothing else would notice if this method were renamed away, and
// every tool result would quietly go back to being cut at the floor.
var _ interface{ HistoryChars() int } = (*Agent)(nil)

// The provider that started this: it cut the call and reported an ordinary
// stop, so finish_reason had nothing to say and the guard used to wave the
// round through to a parse error the model then "fixed" by rewriting paths.
//
// The second call in the round arrived whole, and is the reason the refusals
// are worded per call: telling web_search to produce something shorter would
// shrink a query that was never the problem.
func TestRespondWithToolsCatchesTruncationWithoutFinishReason(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{
				FinishReason: "stop", // the provider claims an ordinary end
				Usage:        &model.Usage{PromptTokens: 40000, CompletionTokens: 900},
				ToolCalls: []model.ToolCall{
					{
						ID:       "call_search_1",
						Type:     "function",
						Function: model.FunctionCall{Name: "web_search", Arguments: `{"query":"Gemini 3.1 pricing"}`},
					},
					{
						ID:   "call_write_1",
						Type: "function",
						Function: model.FunctionCall{
							Name:      "write",
							Arguments: `{"path": "report.html", "content": "<!DOCTYPE html>\n<html`, // cut mid-JSON
						},
					},
				},
			},
			{Text: "ok, wrote the skeleton first"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxToolCalls: 4})

	executed := 0
	reply, usedTools, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "write", Parameters: []byte(`{"type":"object"}`)}}},
		"research this and write it up",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			executed++
			return "should never run", nil, nil
		},
		nil,
		turn.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if executed != 0 {
		t.Fatalf("a round the model never finished writing must not run, ran %d times", executed)
	}
	if !usedTools || reply != "ok, wrote the skeleton first" {
		t.Fatalf("expected the turn to continue after the refusal, got %q (usedTools=%v)", reply, usedTools)
	}

	// Every id answered, or the next request is a 400 from the providers that
	// pair tool_use with tool_result.
	receipts := map[string]string{}
	for _, msg := range provider.requests[1].Messages {
		if msg.Role == model.RoleTool {
			receipts[msg.ToolCallID] = msg.Content
		}
	}
	if len(receipts) != 2 {
		t.Fatalf("every tool call needs a result, got %d: %v", len(receipts), receipts)
	}
	if !strings.Contains(receipts["call_write_1"], "truncated") {
		t.Fatalf("the cut call must be told it was cut: %q", receipts["call_write_1"])
	}
	if !strings.Contains(receipts["call_search_1"], "reissue") {
		t.Fatalf("the intact call must be told to reissue, not to shorten: %q", receipts["call_search_1"])
	}
	if strings.Contains(receipts["call_search_1"], "shorter") {
		t.Fatalf("nothing about the intact call was too long: %q", receipts["call_search_1"])
	}
}

// The 400 that ended a 173-second turn one second after the guard did its job:
// the refused call's arguments stayed in the history, and the next request
// carried them back to a provider that validates what it is handed.
func TestRespondWithToolsKeepsTruncatedArgumentsOutOfHistory(t *testing.T) {
	provider := &toolLoopProvider{
		responses: []model.Response{
			{
				FinishReason: model.FinishReasonLength,
				ToolCalls: []model.ToolCall{{
					ID:   "call_write_1",
					Type: "function",
					Function: model.FunctionCall{
						Name:      "write",
						Arguments: `{"path": "slides.html", "content": "<!DOCTYPE html>\n<html`,
					},
				}},
			},
			{Text: "split into two"},
		},
	}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model", MaxToolCalls: 4})

	if _, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "write", Parameters: []byte(`{"type":"object"}`)}}},
		"make me slides",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
			return "should never run", nil, nil
		},
		nil,
		turn.TurnOptions{},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var seen int
	for _, msg := range provider.requests[1].Messages {
		for _, call := range msg.ToolCalls {
			seen++
			if _, parseErr := model.ParseToolArguments(call.Function.Arguments); parseErr != nil {
				t.Fatalf("history still carries unparseable arguments (%s): %v", call.ID, parseErr)
			}
			if call.ID != "call_write_1" {
				t.Fatalf("the call id must survive so its result stays paired, got %q", call.ID)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("the assistant message must keep its one tool call, saw %d", seen)
	}
}
