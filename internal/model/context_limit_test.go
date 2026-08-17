package model

import (
	"errors"
	"fmt"
	"testing"
)

// The messages are quoted the way a provider actually sends them, wrapped the
// way Aetox's provider layer actually wraps them, because both halves are what
// the detector sees in production. A test written against a bare phrase would
// pass while the real thing arrived as "codex request failed with status 400:
// {...}" and matched nothing.
func TestIsContextLengthErrorRecognizesWhatProvidersActuallySay(t *testing.T) {
	tooLong := []error{
		fmt.Errorf("openai request failed with status 400: {\"error\":{\"message\":\"This model's maximum context length is 400000 tokens. However, your messages resulted in 431204 tokens. Please reduce the length of the messages.\",\"code\":\"context_length_exceeded\"}}"),
		fmt.Errorf("codex request failed with status 400: {\"error\":{\"message\":\"Your input exceeds the context window of this model.\",\"code\":\"context_length_exceeded\"}}"),
		fmt.Errorf("anthropic request failed with status 400: {\"type\":\"invalid_request_error\",\"message\":\"prompt is too long: 219398 tokens > 200000 maximum\"}"),
		errors.New("gemini request failed with status 400: The input token count exceeds the maximum number of tokens allowed"),
	}
	for _, err := range tooLong {
		if !IsContextLengthError(err) {
			t.Errorf("not recognized as a context-length failure:\n  %v", err)
		}
	}

	// The three failures that look adjacent and must not be treated the same.
	// A rate limit needs waiting, a quota needs money, and a truncated answer
	// needs a shorter reply — summarizing the history fixes none of them, and
	// doing it anyway spends a model call to make the user's context worse.
	other := []error{
		errors.New("openai request failed with status 429: Rate limit reached for gpt-5.5 in organization org-x on tokens per min (TPM)"),
		errors.New("anthropic request failed with status 429: {\"type\":\"rate_limit_error\",\"message\":\"Number of request tokens has exceeded your per-minute rate limit\"}"),
		errors.New("deepseek request failed with status 402: Insufficient Balance"),
		errors.New("openai request failed with status 400: max_tokens is too large: 200000 > 128000"),
		errors.New("Post \"https://api.openai.com/v1/responses\": dial tcp: lookup api.openai.com: no such host"),
		nil,
	}
	for _, err := range other {
		if IsContextLengthError(err) {
			t.Errorf("wrongly read as a context-length failure, which would trigger a pointless compaction:\n  %v", err)
		}
	}
}
