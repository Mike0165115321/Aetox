package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func silentProvider(t *testing.T, name string, handler http.HandlerFunc) *OpenAICompatibleProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	requireKey := false
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: name, Model: "grok-code", BaseURL: server.URL, RequireAPIKey: &requireKey,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	return provider
}

// The exact answer that killed a 350-second turn: a 200, a stream, and not one
// frame in it. The engine replays this rather than losing the turn, and it can
// only do that if the error says what it is by identity — matching the prose
// would break the first time the sentence is reworded or translated (§27).
func TestASilentStreamIsIdentifiableAsOne(t *testing.T) {
	provider := silentProvider(t, "opencode-go", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})

	_, err := provider.StreamComplete(context.Background(),
		Request{Messages: []Message{{Role: RoleUser, Content: "ping"}}}, nil, nil)
	if err == nil {
		t.Fatal("a stream with nothing in it was reported as an answer")
	}
	if !IsEmptyCompletion(err) {
		t.Errorf("error = %v, want it to carry ErrEmptyCompletion so the engine can replay it", err)
	}
	// And it has to say enough to tell the two bugs apart next time. "0 stream
	// frames" is a gateway that opened a stream and closed it; frames that
	// carried nothing would be the model. The old message said neither, which
	// is why one line in the debug log was the whole of the evidence.
	if !strings.Contains(err.Error(), "0 stream frames") {
		t.Errorf("error = %v, want it to state how many frames arrived", err)
	}
	if !strings.Contains(err.Error(), "opencode-go") {
		t.Errorf("error = %v, want the provider named", err)
	}
}

// The other shape, and it is a different bug wearing the same words: frames
// arrived, the model finished normally, and every one of them was empty.
func TestAStreamOfEmptyFramesSaysSo(t *testing.T) {
	provider := silentProvider(t, "opencode-go", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, frame := range []string{
			`{"choices":[{"delta":{"content":""}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		} {
			_, _ = w.Write([]byte("data: " + frame + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})

	_, err := provider.StreamComplete(context.Background(),
		Request{Messages: []Message{{Role: RoleUser, Content: "ping"}}}, nil, nil)
	if err == nil {
		t.Fatal("a stream of empty deltas was reported as an answer")
	}
	if !strings.Contains(err.Error(), "2 stream frames") {
		t.Errorf("error = %v, want the frame count that separates this from a dead gateway", err)
	}
	if !strings.Contains(err.Error(), `finish_reason="stop"`) {
		t.Errorf("error = %v, want the finish reason the provider stated", err)
	}
}

// The exemption that must survive all of the above. A truncation at the token
// limit is a normal thing to ask for — the desktop's "test connection" button
// pings with a tiny max_tokens precisely because it wants no answer, only proof
// that the endpoint is alive.
func TestATruncationIsStillNotASilence(t *testing.T) {
	provider := silentProvider(t, "openai", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"grok-code","choices":[
			{"message":{"role":"assistant","content":""},"finish_reason":"length"}]}`))
	})

	if _, err := provider.Complete(context.Background(),
		Request{Messages: []Message{{Role: RoleUser, Content: "ping"}}}); err != nil {
		t.Fatalf("a truncation was reported as a broken provider: %v", err)
	}
}

// Every runtime states it the same way, so the engine's one rule reaches all of
// them. Before this, ollama and the Responses API each wrote their own sentence
// for the same condition and neither could be recognised — the retry would have
// covered the OpenAI-compatible rows and quietly missed the rest.
func TestEveryRuntimeStatesTheSilenceTheSameWay(t *testing.T) {
	for _, tc := range []struct{ name, provider, finishReason string }{
		{"chat completions", "opencode-go", "stop"},
		{"ollama", "ollama", "stop"},
		{"responses", "codex", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := errEmptyCompletion(tc.provider, tc.finishReason, "", "", 0)
			if err == nil {
				t.Fatal("an answer with nothing in it was accepted")
			}
			if !IsEmptyCompletion(err) {
				t.Errorf("error = %v, want ErrEmptyCompletion", err)
			}
			if !strings.HasPrefix(err.Error(), tc.provider+" ") {
				t.Errorf("error = %v, want it to open with the provider's name", err)
			}
		})
	}
}
