package cognitive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// scriptedProvider answers call N with step N, so a test can make one round
// fail and the next succeed. toolLoopProvider cannot: it returns responses, and
// what matters here is the error.
type scriptedProvider struct {
	steps []func(model.Request) (model.Response, error)
	seen  []model.Request
}

func (p *scriptedProvider) Name() string              { return "scripted" }
func (p *scriptedProvider) SupportsToolCalling() bool { return true }
func (p *scriptedProvider) SupportsReasoning() bool   { return false }

func (p *scriptedProvider) Complete(_ context.Context, req model.Request) (model.Response, error) {
	p.seen = append(p.seen, req)
	if n := len(p.seen); n <= len(p.steps) {
		return p.steps[n-1](req)
	}
	return model.Response{Text: "done"}, nil
}

func longHistory(turns int) []model.Message {
	out := make([]model.Message, 0, turns*2)
	for i := 0; i < turns; i++ {
		out = append(out,
			model.Message{Role: model.RoleUser, Content: fmt.Sprintf("q%d %s", i, strings.Repeat("คำถามยาว ", 20))},
			model.Message{Role: model.RoleAssistant, Content: fmt.Sprintf("a%d %s", i, strings.Repeat("คำตอบยาว ", 20))},
		)
	}
	return out
}

func oneTurn(a *Agent, msg string) (string, error) {
	reply, _, err := a.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		msg,
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil,
		turn.TurnOptions{},
	)
	return reply, err
}

// The char budget is a guess and the provider is the fact.
//
// Aetox sizes its retained history in BYTES (bootstrap.ContextChars: window
// tokens x 4) against a limit the provider counts in TOKENS, and the ratio
// between them is not constant — 3.45 to 5.46 bytes per token across three real
// sessions on one machine, depending on how much of the turn was Thai prose and
// how much was English tool output. At the low end a budget reading
// comfortably-under is already past what the model will take.
//
// Before this, nothing in the engine reacted: the turn died, the history that
// caused it stayed, and every retry the user typed by hand failed the same way.
func TestAProviderSayingTheHistoryIsTooLongGetsAShorterHistory(t *testing.T) {
	tooLong := errors.New(`codex request failed with status 400: {"error":{"message":"Your input exceeds the context window of this model.","code":"context_length_exceeded"}}`)
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, tooLong },
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "COMPACT-SUMMARY: they were tracing a context meter"}, nil
		},
		func(model.Request) (model.Response, error) { return model.Response{Text: "final answer"}, nil },
	}}
	// A budget far larger than this history, so the byte-driven path cannot be
	// what compacts: only the provider's refusal can.
	agent := NewAgent(AgentConfig{Provider: provider, Model: "gpt-5.6-luna", SystemPrompt: "sys", MaxChars: 10_000_000})
	agent.RestoreHistory(longHistory(10))

	reply, err := oneTurn(agent, "แล้วสรุปยังไง")
	if err != nil {
		t.Fatalf("the turn died on a failure that had a fix: %v", err)
	}
	if reply != "final answer" {
		t.Fatalf("reply = %q, want the retry's answer", reply)
	}
	if len(provider.seen) != 3 {
		t.Fatalf("made %d calls, want 3 (refused turn, summarizer, retry)", len(provider.seen))
	}

	// The middle call is the summarizer: no tools, and it is the compaction
	// prompt rather than the conversation's own system prompt.
	if summarizer := provider.seen[1]; len(summarizer.Tools) != 0 {
		t.Errorf("the summarization request carried %d tools; it is not a turn", len(summarizer.Tools))
	}
	// And the retry is smaller than what was refused. Retrying the same bytes
	// would just collect the same refusal.
	refused, retried := provider.seen[0], provider.seen[2]
	if len(retried.Messages) >= len(refused.Messages) {
		t.Errorf("retried with %d messages against the refused %d; nothing was actually given up",
			len(retried.Messages), len(refused.Messages))
	}
}

// Two attempts, then a sentence. A single message too big for the window is not
// something summarizing can fix, and spinning on it turns a clear failure into a
// slow one.
func TestAnOverflowWithNothingLeftToSummarizeSaysSoInsteadOfSpinning(t *testing.T) {
	tooLong := errors.New(`anthropic request failed with status 400: {"message":"prompt is too long: 219398 tokens > 200000 maximum"}`)
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, tooLong },
	}}
	// No history at all, so SplitForCompaction has nothing to hand over.
	agent := NewAgent(AgentConfig{Provider: provider, Model: "claude-opus-5", SystemPrompt: "sys", MaxChars: 10_000_000})

	_, err := oneTurn(agent, "สวัสดี")
	if err == nil {
		t.Fatal("want an error when the prompt cannot be made to fit")
	}
	// The user has to be able to act on this: which model, and that shortening
	// is no longer available.
	for _, want := range []string{"context window", "claude-opus-5", "summarize"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not say what is wrong (%q missing): %v", want, err)
		}
	}
	if len(provider.seen) != 1 {
		t.Errorf("made %d calls; with nothing to summarize there is nothing to retry", len(provider.seen))
	}
}

// A rate limit, a spent balance and a truncated reply all arrive as errors too,
// and summarizing helps none of them. Guarding the wiring and not just the
// classifier: this is the path that would spend a model call and shorten the
// user's history for a failure that had nothing to do with length.
func TestAFailureThatIsNotAboutLengthNeverCostsTheHistory(t *testing.T) {
	rateLimited := errors.New("openai request failed with status 429: Rate limit reached for gpt-5.5 on tokens per min (TPM)")
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, rateLimited },
		func(model.Request) (model.Response, error) { return model.Response{Text: "recovered"}, nil },
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "gpt-5.5", SystemPrompt: "sys", MaxChars: 10_000_000})
	agent.RestoreHistory(longHistory(10))
	before := len(agent.ContextMessages())

	_, _ = oneTurn(agent, "ถามใหม่")

	if got := len(agent.ContextMessages()); got < before {
		t.Errorf("history shrank from %d to %d messages on a rate limit; a 429 is not a size problem", before, got)
	}
	for i, req := range provider.seen {
		if len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "compacting") {
			t.Errorf("call %d was a summarization triggered by a rate limit", i+1)
		}
	}
}
