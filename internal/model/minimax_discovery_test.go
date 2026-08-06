package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The model picker must be filled from the provider's own catalog, not from a
// list typed into this repository.
//
// A hardcoded list is wrong the week after it is written — new models appear,
// old ones retire, and the user's account may not even carry the same set. The
// catalog holds exactly one model name per provider, and it is a cold-start
// fallback for when discovery cannot run at all, not the source the picker
// reads.
//
// MiniMax is the case worth pinning: it serves an OpenAI-shaped GET /v1/models,
// and its base URL already ends in /v1, so a discovery path that appended the
// wrong suffix would 404 and fall back to the single fallback name — a picker
// with one entry that looks deliberate.
func TestMiniMaxModelNamesComeFromTheProvider(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "MiniMax-M3", "object": "model", "owned_by": "minimax"},
				{"id": "MiniMax-M2.7", "object": "model", "owned_by": "minimax"},
				{"id": "MiniMax-M2.5", "object": "model", "owned_by": "minimax"},
			},
		})
	}))
	defer server.Close()

	models, err := ModelChoicesWithEndpointAndAPIKey("minimax", server.URL, "test-key")
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	if gotPath != "/models" {
		t.Errorf("asked for %q, want /models", gotPath)
	}
	if !strings.Contains(gotAuth, "test-key") {
		t.Errorf("no API key on the discovery request (Authorization=%q)", gotAuth)
	}

	want := []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.5"}
	if len(models) != len(want) {
		t.Fatalf("got %v, want the three the server offered", models)
	}
	for _, name := range want {
		found := false
		for _, got := range models {
			if got == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q missing from the discovered list %v", name, models)
		}
	}
}

// The catalog's own name for MiniMax must be one this table recognises. A
// fallback that lands on "unknown model" would leave a cold start with no
// thinking control at all, which is the honest answer for a name nobody has
// checked — and the wrong one for the provider's flagship.
func TestMiniMaxFallbackModelIsAKnownOne(t *testing.T) {
	fallback := DefaultModel("minimax")
	if fallback == "" {
		t.Fatal("minimax has no fallback model; a cold start with no network cannot pick one")
	}
	caps := ResolveThinkingCapabilities("minimax", fallback)
	if !caps.Supported {
		t.Fatalf("catalog falls back to %q, which this table does not recognise (%s)", fallback, caps.Source)
	}
}
