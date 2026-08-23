package cognitive

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// What opencode-go actually did at 13:13:45 on 23 ส.ค.: answered a round in 1.4
// seconds with a stream that carried no frame at all. Wrapped exactly the way
// the provider layer wraps it, because the engine recognises this failure by
// identity (model.ErrEmptyCompletion) — a test that invented its own error
// would keep passing against a provider that stopped tagging it.
func answeredWithNothing() error {
	return fmt.Errorf("opencode-go %w (finish_reason=%q, 0 stream frames)", model.ErrEmptyCompletion, "")
}

// The turn the owner lost after 350 seconds and eighteen tool calls.
//
// Six rounds of web_fetch and web_search had already come back and been folded
// into the context. The seventh round returned nothing — no text, no reasoning,
// no tool call — and the provider layer stated that as an error, which the tool
// loop had no case for. So a whole afternoon's research died on one blank
// stream, one round short of the answer.
func TestAnEmptyAnswerMidToolLoopIsAskedAgain(t *testing.T) {
	callsTool := func(model.Request) (model.Response, error) {
		return model.Response{ToolCalls: []model.ToolCall{{
			ID: "1", Type: "function",
			Function: model.FunctionCall{Name: "read", Arguments: `{"path":"a.md"}`},
		}}}, nil
	}
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		callsTool,
		func(model.Request) (model.Response, error) { return model.Response{}, answeredWithNothing() },
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "ตารางราคาครบแล้วครับ"}, nil
		},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "grok-code", SystemPrompt: "sys", MaxChars: 1_000_000})

	reply, err := oneTurn(agent, "ค้นราคาโมเดลให้หน่อย")
	if err != nil {
		t.Fatalf("one silent round still throws away the whole turn: %v", err)
	}
	if reply != "ตารางราคาครบแล้วครับ" {
		t.Errorf("reply = %q, want the answer the replay produced", reply)
	}
	if len(provider.seen) != 3 {
		t.Fatalf("provider was called %d times, want 3 (the tool round, the silence, the replay)", len(provider.seen))
	}
	// The work already done has to survive the replay. Losing the tool results
	// here would be the same bug wearing a different face: the turn continues,
	// but the model is asked to answer a question whose research has vanished.
	var toolResults int
	for _, msg := range provider.seen[2].Messages {
		if msg.Role == model.RoleTool {
			toolResults++
		}
	}
	if toolResults == 0 {
		t.Error("the replay carried no tool results — the round's work was dropped along with the silence")
	}
	// And the silent round must leave nothing behind: it was never an answer,
	// so it cannot appear in the history the replay is built from.
	if len(provider.seen[1].Messages) != len(provider.seen[2].Messages) {
		t.Errorf("the replay sent %d messages against the silent round's %d — the dead round left residue",
			len(provider.seen[2].Messages), len(provider.seen[1].Messages))
	}
}

// Bounded, and then changed. Asking a fourth time is not a strategy, so the
// ladder's last rung is a different request rather than a louder one.
func TestAProviderThatKeepsAnsweringNothingIsNudgedOnceThenStops(t *testing.T) {
	silent := func(model.Request) (model.Response, error) { return model.Response{}, answeredWithNothing() }
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		silent, silent, silent, silent, silent, silent,
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "grok-code", SystemPrompt: "sys", MaxChars: 1_000_000})

	_, err := oneTurn(agent, "ค้นราคาโมเดลให้หน่อย")
	if err == nil {
		t.Fatal("a provider that said nothing four times running reported success")
	}
	// Recognisable by identity downstream too: the desktop decides whether to
	// draw a retry button, and matching the prose would break the first time
	// this sentence is reworded.
	if !model.IsEmptyCompletion(err) {
		t.Errorf("error = %v, want it to still carry model.ErrEmptyCompletion", err)
	}
	want := 1 + maxEmptyCompletionReplays + 1 // the ask, its replays, the nudged ask
	if len(provider.seen) != want {
		t.Fatalf("provider was called %d times, want %d (the ask, %d replays, one nudged ask)",
			len(provider.seen), want, maxEmptyCompletionReplays)
	}
	last := provider.seen[len(provider.seen)-1].Messages
	if !strings.Contains(last[len(last)-1].Content, "previous reply was empty") {
		t.Errorf("the last attempt asked the same question again; want the nudge, got %q",
			last[len(last)-1].Content)
	}
}

// The replays must not eat the identical-request budget of the failure next
// door. A socket that keeps dying and a gateway that keeps saying nothing are
// different problems, and a turn that meets one still deserves the full answer
// to the other.
func TestASilenceAndADropDoNotShareOneBudget(t *testing.T) {
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, answeredWithNothing() },
		func(model.Request) (model.Response, error) { return model.Response{}, droppedMidAnswer() },
		func(model.Request) (model.Response, error) { return model.Response{}, answeredWithNothing() },
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "รอดมาได้"}, nil
		},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "grok-code", SystemPrompt: "sys", MaxChars: 1_000_000})

	reply, err := oneTurn(agent, "ถามอะไรก็ได้")
	if err != nil {
		t.Fatalf("two different failures spent one budget between them: %v", err)
	}
	if reply != "รอดมาได้" {
		t.Errorf("reply = %q, want the answer that came after both were absorbed", reply)
	}
}

// The route with no tools, which is not a side road: it is what a provider
// without tool calling uses, what compaction uses, and what the tool loop falls
// back to.
//
// It had a recovery for this all along — recoverEmptyReply, the nudge that has
// been in the file since the small-Ollama days. It just could not reach it: the
// recovery reads an empty Response.Text, and every OpenAI-compatible provider
// states the same condition as an error instead, which returned three lines
// earlier. Dead code guarding a live failure.
func TestTheToolLessRouteRecoversFromASilentProvider(t *testing.T) {
	silent := func(model.Request) (model.Response, error) { return model.Response{}, answeredWithNothing() }
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		silent, silent, silent,
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "ตอบหลังโดนสะกิด"}, nil
		},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "grok-code", SystemPrompt: "sys", MaxChars: 1_000_000})

	// No tools and no executor is what sends RespondWithTools down Respond.
	reply, _, err := agent.RespondWithTools(context.Background(), nil, "ถามอะไรก็ได้", nil, nil, turn.TurnOptions{})
	if err != nil {
		t.Fatalf("the tool-less route still ends red on a silence it knows how to answer: %v", err)
	}
	if reply != "ตอบหลังโดนสะกิด" {
		t.Errorf("reply = %q, want the nudged answer", reply)
	}
}

// The other half of the bound, and the older lesson: a provider that ANSWERED
// and said no is not a silence. Replaying it spends the same money on the same
// refusal and hides what the user was told.
func TestARefusalIsNotASilence(t *testing.T) {
	refused := model.ErrMissingAPIKey
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, refused },
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "ไม่ควรมาถึงตรงนี้"}, nil
		},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "grok-code", SystemPrompt: "sys", MaxChars: 1_000_000})

	if _, err := oneTurn(agent, "ถามอะไรก็ได้"); err == nil {
		t.Fatal("a rejected key was reported as a working turn")
	}
	if len(provider.seen) != 1 {
		t.Errorf("provider was called %d times, want 1: a refusal is an answer", len(provider.seen))
	}
}
