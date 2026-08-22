package model

import (
	"errors"
	"fmt"
	"testing"
)

// Wrapped the way Aetox's provider layer actually wraps them, for the same
// reason context_limit_test.go is: the detector never sees a bare phrase, it
// sees "ollama request failed with status 400: {...}" with the phrase inside.
func TestIsToolBlockRejectionRecognizesARefusedToolBlock(t *testing.T) {
	refused := []error{
		errors.New("ollama request failed with status 400: registry.ollama.ai/library/gemma3 does not support tools"),
		fmt.Errorf("openai request failed with status 400: {\"error\":{\"message\":\"Invalid schema for function 'desk_terminal': 'command' is not of type 'object'\"}}"),
		fmt.Errorf("groq request failed with status 400: {\"error\":{\"message\":\"tool calling is not supported for this model\"}}"),
		errors.New("gemini request failed with status 400: Function calling is not enabled for models/gemma-3-27b-it"),
	}
	for _, err := range refused {
		if !IsToolBlockRejection(err) {
			t.Errorf("not recognized as a refused tool block:\n  %v", err)
		}
	}
}

// Everything a second, tool-less call cannot fix. Each of these used to reach
// the retry anyway, and each of them answers the retry exactly as it answered
// the first call — one more full-context request, one more wait, same wall.
//
// The Stop button is in this list on purpose. A cancelled turn asking the
// provider one more question is the opposite of what the user just pressed.
func TestIsToolBlockRejectionLeavesTheOtherFailuresAlone(t *testing.T) {
	notAboutTools := []error{
		errors.New("codex: the free plan's limit is used up. It resets in 19 days."),
		errors.New("codex plan limit reached. It resets on its own schedule. (429: usage_limit_reached)"),
		errors.New("z.ai says this account is out of credits, so waiting will not help. (429: insufficient balance)"),
		errors.New("deepseek rejected the sign-in. Sign in again. (401: invalid api key)"),
		errors.New("openai refused this account. The plan may not include it. (403: model_not_found)"),
		errors.New("context canceled"),
		errors.New("Post \"https://api.deepseek.com/chat/completions\": dial tcp: lookup api.deepseek.com: no such host"),
		errors.New("anthropic request failed with status 400: prompt is too long: 219398 tokens > 200000 maximum"),
		errors.New("ollama request failed with status 500: llama runner process has terminated"),
	}
	for _, err := range notAboutTools {
		if IsToolBlockRejection(err) {
			t.Errorf("would be asked a second time, and would fail the same way:\n  %v", err)
		}
	}
}

// A provider that echoes the request it rejected sends the word "tools" back
// inside the body. That is not a refusal of the tool block, it is a copy of
// what was sent — which is why every phrase in the list carries the refusal and
// the thing refused together.
func TestIsToolBlockRejectionIsNotFooledByAnEchoedRequest(t *testing.T) {
	echoed := errors.New(`openrouter request failed with status 400: {"error":{"message":"Invalid value for 'temperature'","request":{"model":"x","tools":[{"type":"function"}]}}}`)
	if IsToolBlockRejection(echoed) {
		t.Error("matched a request echo rather than a refusal")
	}
}

func TestIsToolBlockRejectionOnNil(t *testing.T) {
	if IsToolBlockRejection(nil) {
		t.Error("nil is not a refusal")
	}
}
