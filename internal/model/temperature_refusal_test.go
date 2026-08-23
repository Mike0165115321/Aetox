package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The exact 400 opencode-go answered on a real turn, 2026-08-23:
//
//	Error from provider (Console Go): Upstream request failed:
//	[invalid_request_error] invalid temperature: only 1 is allowed for this model
//
// It ended the turn before a token was generated, on a provider whose key and
// subscription were both good — the request shape was the only thing wrong.
func TestTemperatureRefusalIsReplayedWithoutIt(t *testing.T) {
	const refusal = `{"type":"error","error":{"type":"invalid_request_error","message":"invalid temperature: only 1 is allowed for this model"}}`

	// The catalog must know nothing about this model. TestMain seeds one for
	// the package, and it states outright that gpt-5.6-luna refuses a
	// temperature — so with it installed the field is dropped before the first
	// attempt and the replay this test exists to check never runs. Which is the
	// catalog working, and the wrong world for this test to be in.
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(nil)

	for _, tc := range []struct {
		name   string
		stream bool
	}{{"complete", false}, {"stream", true}} {
		t.Run(tc.name, func(t *testing.T) {
			refusedTemperature.Delete("opencode-go/gpt-5.6-luna")
			var sent []float64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var got struct {
					Temperature float64 `json:"temperature"`
				}
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &got)
				sent = append(sent, got.Temperature)
				if got.Temperature != 0 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(refusal))
					return
				}
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
			}))
			defer srv.Close()

			p, perr := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
				Provider: "opencode-go", Model: "gpt-5.6-luna", APIKey: "k", BaseURL: srv.URL})
			if perr != nil {
				t.Fatalf("provider: %v", perr)
			}
			req := Request{Model: "gpt-5.6-luna", Temperature: 0.2,
				Messages: []Message{{Role: RoleUser, Content: "hi"}}}

			var resp Response
			var err error
			if tc.stream {
				resp, err = p.StreamComplete(context.Background(), req, func(string) error { return nil }, func(string) error { return nil })
			} else {
				resp, err = p.Complete(context.Background(), req)
			}
			if err != nil {
				t.Fatalf("turn died on a refusal that should have been replayed: %v", err)
			}
			if resp.Text != "ok" {
				t.Errorf("content = %q, want %q", resp.Text, "ok")
			}
			// Two attempts: the one that was refused and the one without the field.
			if len(sent) != 2 {
				t.Fatalf("attempts = %d, want 2 (%v)", len(sent), sent)
			}
			if sent[0] != 0.2 {
				t.Errorf("first attempt sent temperature %v, want 0.2", sent[0])
			}
			if sent[1] != 0 {
				t.Errorf("replay still carried temperature %v, want it dropped", sent[1])
			}

			// The refusal is remembered, so the next turn on this model pays
			// one round trip instead of two.
			sent = nil
			if tc.stream {
				_, err = p.StreamComplete(context.Background(), req,
					func(string) error { return nil }, func(string) error { return nil })
			} else {
				_, err = p.Complete(context.Background(), req)
			}
			if err != nil {
				t.Fatalf("second turn: %v", err)
			}
			if len(sent) != 1 {
				t.Errorf("second turn made %d attempts, want 1 — the refusal was not remembered", len(sent))
			}
		})
	}
}

// A 400 that is about anything else must still end the turn. Replaying an
// account or model-id error would only spend the user's money twice.
func TestOtherBadRequestsAreNotReplayed(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
	}))
	defer srv.Close()

	p, perr := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "opencode", Model: "nope", APIKey: "k", BaseURL: srv.URL})
	if perr != nil {
		t.Fatalf("provider: %v", perr)
	}
	_, err := p.Complete(context.Background(), Request{Model: "nope", Temperature: 0.2,
		Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil {
		t.Fatal("a 400 about the model was swallowed")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 — an unrelated 400 was replayed", attempts)
	}
}

// The catalog states this outright for 1,454 of the models it describes,
// gpt-5.6-luna — the one that actually 400d — among them. Reading it means the
// first attempt is already right and the replay above never has to fire.
func TestCatalogKnownRefusalCostsNoFailedAttempt(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"opencode-go/gpt-5.6-luna": {Context: 400000, ToolCall: true, NoTemperature: true},
		"opencode-go/kimi-k2.5":    {Context: 262144, ToolCall: true},
	}})

	for _, tc := range []struct {
		model string
		want  float64
	}{
		{"gpt-5.6-luna", 0}, // catalog says it refuses; never offered
		{"kimi-k2.5", 0.2},  // catalog is silent; sent as normal
	} {
		refusedTemperature.Delete("opencode-go/" + tc.model)
		var sent []float64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var got struct {
				Temperature float64 `json:"temperature"`
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &got)
			sent = append(sent, got.Temperature)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		}))

		p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider: "opencode-go", Model: tc.model, APIKey: "k", BaseURL: srv.URL})
		if err != nil {
			t.Fatalf("provider: %v", err)
		}
		if _, err := p.Complete(context.Background(), Request{Model: tc.model, Temperature: 0.2,
			Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
			t.Fatalf("%s: %v", tc.model, err)
		}
		srv.Close()

		if len(sent) != 1 {
			t.Errorf("%s made %d attempts, want 1", tc.model, len(sent))
		}
		if sent[0] != tc.want {
			t.Errorf("%s first attempt sent temperature %v, want %v", tc.model, sent[0], tc.want)
		}
	}
}

// A model the catalog has never described must still be able to teach us, and
// a catalog that is wrong about one must not be the last word. This is the
// nvidia lesson applied: the endpoint outranks the table.
func TestMeasuredRefusalStillWinsOverASilentCatalog(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{}})
	refusedTemperature.Delete("opencode-go/unknown-to-any-table")

	var sent []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got struct {
			Temperature float64 `json:"temperature"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		sent = append(sent, got.Temperature)
		if got.Temperature != 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid temperature: only 1 is allowed for this model"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "opencode-go", Model: "unknown-to-any-table", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	req := Request{Model: "unknown-to-any-table", Temperature: 0.2,
		Messages: []Message{{Role: RoleUser, Content: "hi"}}}
	if _, err := p.Complete(context.Background(), req); err != nil {
		t.Fatalf("turn died on a refusal the catalog could not warn about: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("attempts = %d, want 2", len(sent))
	}
	sent = nil
	if _, err := p.Complete(context.Background(), req); err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if len(sent) != 1 || sent[0] != 0 {
		t.Errorf("second turn attempts=%d sent=%v; the measurement was not remembered", len(sent), sent)
	}
}
