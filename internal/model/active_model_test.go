package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The default model must be the one the server is actually serving. The
// discovery list is sorted for the picker, so reading a default off its first
// entry answers "which model is loaded?" with "whichever sorts first" — on LM
// Studio that addresses a model that is not in memory, which either fails or
// forces a slow just-in-time load the user never asked for.
func TestDefaultModelPrefersTheLoadedOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v0/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": "google/gemma-3-4b", "type": "llm", "state": "not-loaded"},
				{"id": "text-embedding-nomic", "type": "embeddings", "state": "loaded"},
				{"id": "qwen/qwen3-8b", "type": "llm", "state": "loaded"},
			}})
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": "google/gemma-3-4b"}, {"id": "qwen/qwen3-8b"},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	got := ResolveDefaultModel("lmstudio", server.URL+"/v1", "")
	if got != "qwen/qwen3-8b" {
		t.Fatalf("default model = %q, want the loaded one (%q) — alphabetical order would say %q",
			got, "qwen/qwen3-8b", "google/gemma-3-4b")
	}
}

// An embedding model reports "loaded" too and cannot answer a chat request.
func TestDefaultModelSkipsLoadedEmbeddingModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v0/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": "text-embedding-nomic", "type": "embeddings", "state": "loaded"},
			}})
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "google/gemma-3-4b"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if got := ResolveDefaultModel("lmstudio", server.URL+"/v1", ""); got != "google/gemma-3-4b" {
		t.Fatalf("default model = %q, want the discovery fallback — no chat model is loaded", got)
	}
}

// Nothing loaded is not an error: fall back to the list rather than returning
// nothing and leaving the app with no model at all.
func TestDefaultModelFallsBackWhenNothingIsLoaded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v0/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": "google/gemma-3-4b", "type": "llm", "state": "not-loaded"},
			}})
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "google/gemma-3-4b"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if got := ResolveDefaultModel("lmstudio", server.URL+"/v1", ""); got != "google/gemma-3-4b" {
		t.Fatalf("default model = %q, want the discovery fallback", got)
	}
}

// Ollama answers the same question through /api/ps — the models resident right
// now. Same rule: what is running beats what sorts first.
func TestDefaultModelUsesOllamaResidentModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "qwen3:8b", "model": "qwen3:8b"},
			}})
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "ornith:9b"}, {"name": "qwen3:8b"},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if got := ResolveDefaultModel("ollama", server.URL, ""); got != "qwen3:8b" {
		t.Fatalf("default model = %q, want the resident model qwen3:8b (ornith:9b sorts first)", got)
	}
}

// An idle Ollama has nothing resident; that must not blank out the model.
func TestDefaultModelFallsBackWhenOllamaIsIdle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{}})
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "ornith:9b"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if got := ResolveDefaultModel("ollama", server.URL, ""); got != "ornith:9b" {
		t.Fatalf("default model = %q, want the discovery fallback ornith:9b", got)
	}
}

// A hosted provider has no "currently loaded model" to ask about, so it must
// never get the local runtime probe. It *is* asked for its model list — that is
// deliberate, and the catalog name is only what survives when the list does not
// answer.
func TestDefaultModelIgnoresActiveProbeForHostedProviders(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if got := ResolveDefaultModel("deepseek", server.URL, "k"); got == "" {
		t.Fatal("hosted provider lost its catalog default when discovery failed")
	}
	for _, path := range paths {
		if strings.Contains(path, "/api/ps") || strings.Contains(path, "/api/v0/models") {
			t.Errorf("probed a hosted provider with the local-runtime endpoint %q", path)
		}
	}
}
