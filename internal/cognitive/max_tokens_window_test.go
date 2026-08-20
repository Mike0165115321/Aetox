package cognitive

import (
	"testing"

	"github.com/Mike0165115321/Aetox/internal/memory"
	"github.com/Mike0165115321/Aetox/internal/model"
)

type fakeProv struct{ model.Provider; name string }

func (f fakeProv) Name() string { return f.name }

// The 400 that started this, kept as a case: ThaiLLM's THaLLE-8B serves a
// 16,384-token window, and on 2026-08-20 a real turn with 9,791 tokens of input
// was rejected outright for asking 8,192 tokens back when only 6,593 remained.
// max_tokens is checked against what is LEFT in the window, so a per-provider
// floor alone is a 400 waiting for any small model.
//
// The rows below the first one are the other half of the guard: a clamp that
// fixed the small window by quietly shrinking every big one would be its own
// regression, so anthropic, xai and openai are here to stay untouched.
func TestMaxTokensNeverExceedsTheRoomLeft(t *testing.T) {
	cases := []struct {
		provider, mdl string
		promptTokens  int
		wantAtMost    int
		note          string
	}{
		// The exact turn that 400'd: window 16384, input 9791.
		{"thaillm", "THaLLE-0.2-ThaiLLM-8B-fa", 9791, 16384 - 9791, "the reported failure"},
		{"thaillm", "THaLLE-0.2-ThaiLLM-8B-fa", 200, 16384 - 200, "fresh chat, small model"},
		{"thaillm", "THaLLE-0.2-ThaiLLM-8B-fa", 16000, 8192, "already over the window"},
		// Big windows must be untouched.
		{"anthropic", "claude-haiku-4-5", 5000, 32000, "big window, unchanged"},
		{"xai", "grok-4.6", 5000, 32000, "500k window, unchanged"},
		{"openai", "gpt-4o-mini", 1000, 16384, "openai floor kept"},
		{"ollama", "whatever", 1000, 8192, "unknown window, floor kept"},
	}
	for _, c := range cases {
		a := &Agent{
			provider:  fakeProv{name: c.provider},
			model:     c.mdl,
			context:   memory.NewContext("sys", 50, 100000),
			lastUsage: model.Usage{PromptTokens: c.promptTokens},
		}
		got := a.toolLoopMaxTokens()
		ok := "ok"
		if got > c.wantAtMost {
			ok = "TOO BIG -> would 400"
			t.Errorf("%s/%s input=%d: max_tokens=%d exceeds room %d",
				c.provider, c.mdl, c.promptTokens, got, c.wantAtMost)
		}
		t.Logf("  %-10s %-28s input=%-6d -> max_tokens=%-6d (room %d) %s | %s",
			c.provider, c.mdl, c.promptTokens, got, c.wantAtMost, ok, c.note)
	}
}
