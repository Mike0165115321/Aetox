// Package model_test holds the one test that cannot live inside package model:
// it needs internal/config to read the key store, and config imports model.
package model_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/provider"
)

// A real tool-calling round trip against every provider the user has a key for.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveEveryConfiguredProvider -v -count=1
//
// It exists because a catalog row that compiles proves nothing. Every field in
// one is a claim about somebody else's server — that the base URL is the one
// serving chat, that the fallback model id is still served, that the provider
// speaks the tools protocol at all — and the only thing that can check a claim
// like that is the server. Two of those claims were already wrong when this was
// written, and neither showed up in a green suite.
//
// Keys come from config.LoadCredentials(), never from an environment variable
// and never from a test argument: LoadCredentials registers every key it reads
// with debuglog.Redact, so a key that passes through here cannot reach a log
// line. Same reasoning as liveProviderKey in live_provider_test.go, pointed at
// the store the desktop actually writes now.
//
// Providers with no key are skipped, not failed, so the covered set grows as
// keys are added instead of demanding all of them at once.
func TestLiveEveryConfiguredProvider(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run live provider tests")
	}
	// There are two stores, not one, and which is live depends on how the app
	// was started: wails-dev.bat sets AETOX_DATA_ROOT=<desktop>/.aetox-data,
	// while the built exe launched on its own falls through to the platform
	// config dir. Guessing one costs an afternoon — this test first reported
	// "no key" for a key that had just been saved, because it was looking in
	// the dev folder while the app was writing to %APPDATA%. So try both and
	// use whichever actually holds keys.
	//
	// The env var is AETOX_LIVE_DATA_ROOT rather than AETOX_DATA_ROOT because
	// TestMain points the latter at a throwaway directory for this whole
	// package on purpose, so a unit test's result never depends on what the
	// developer happens to be signed into. A live test wants the opposite.
	candidates := []string{strings.TrimSpace(os.Getenv("AETOX_LIVE_DATA_ROOT"))}
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		candidates = append(candidates, filepath.Join(configDir, "aetox"))
	}
	if dev, err := filepath.Abs(filepath.Join("..", "..", "desktop", ".aetox-data")); err == nil {
		candidates = append(candidates, dev)
	}

	restore := os.Getenv("AETOX_DATA_ROOT")
	defer func() { _ = os.Setenv("AETOX_DATA_ROOT", restore) }()

	var creds config.Credentials
	var root string
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if err := os.Setenv("AETOX_DATA_ROOT", candidate); err != nil {
			t.Fatalf("setenv: %v", err)
		}
		found, err := config.LoadCredentials()
		if err != nil {
			t.Logf("  (store at %s unreadable: %v)", candidate, err)
			continue
		}
		if len(found.ModelAPIKeys) > len(creds.ModelAPIKeys) {
			creds, root = found, candidate
		}
	}
	if len(creds.ModelAPIKeys) == 0 {
		t.Fatalf("no keys in any credentials store (looked in %s) — add them in Settings first",
			strings.Join(candidates, ", "))
	}
	t.Logf("credentials store: %s (%d providers)", root, len(creds.ModelAPIKeys))

	// One tool, shaped the way Aetox shapes its own: the model has to choose it
	// AND fill an argument, which is the behaviour the agent loop depends on and
	// the one a chat-only model fails.
	weather := model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "get_weather",
			Description: "Get the current weather for a city.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {"city": {"type": "string", "description": "City name"}},
				"required": ["city"]
			}`),
		},
	}

	for _, name := range provider.SupportedProviders() {
		spec, _ := provider.Lookup(name)
		if !spec.RequiresAPIKey || !spec.AcceptsAPIKey {
			continue // local runtimes and sign-in providers are not this test's job
		}
		key := strings.TrimSpace(creds.ModelAPIKeys[name])
		if key == "" {
			t.Logf("  %-11s SKIP  ยังไม่มีคีย์ — %s", name, spec.APIKeyURL)
			continue
		}

		t.Run(name, func(t *testing.T) {
			// The model list is asked of the provider before it is used, so a
			// fallback id that has quietly died reads as its own failure rather
			// than as a confusing 404 out of the chat call.
			// An empty fallback is an answer, not an omission: rows whose
			// endpoint serves /v1/models to anyone deliberately write no name
			// down (modelscope, nvidia, ollama-cloud — see their catalog
			// entries). For those the live list is not a check on the row, it
			// IS the row, so take the first served id and carry on.
			modelID := spec.ModelDefaults.FallbackModel
			if live, err := model.DiscoverOpenAICompatibleModels(name, spec.BaseURL, key); err != nil {
				// Not a failure of its own: a provider may refuse to list models
				// for reasons that have nothing to do with the row being right.
				// xAI answers /v1/models with 403 until the team has credits.
				t.Logf("  %-11s (model list unavailable: %v)", name, err)
			} else if len(live) > 0 {
				if strings.TrimSpace(modelID) == "" {
					modelID = live[0]
					t.Logf("  %-11s (no fallback by design — asking the endpoint: %s)", name, modelID)
				} else {
					found := false
					for _, m := range live {
						if sameModelID(m, modelID) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("fallbackModel %q is NOT in the live list (%d served, first: %s) — "+
							"the catalog row points at something nobody serves", modelID, len(live), live[0])
						modelID = live[0]
					}
				}
			}
			// Only reachable when the row writes no name AND the list could not
			// be read — a provider that gates /v1/models behind a key it just
			// rejected, say. Nothing left to try, and guessing would report a
			// 404 as if the row were wrong.
			if strings.TrimSpace(modelID) == "" {
				t.Skipf("  %-11s SKIP  no fallback in the row and no live list to ask", name)
			}

			p, err := model.NewProvider(model.ProviderOptions{
				Provider: name,
				Model:    modelID,
				APIKey:   key,
				Timeout:  90 * time.Second,
			})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			resp, err := p.Complete(ctx, model.Request{
				Model:      modelID,
				Messages:   []model.Message{{Role: model.RoleUser, Content: "What is the weather in Chiang Mai? Use the tool."}},
				Tools:      []model.ToolDefinition{weather},
				ToolChoice: "auto",
				MaxTokens:  1024,
			})
			if err != nil {
				t.Fatalf("%s / %s: %v", name, modelID, err)
			}
			if len(resp.ToolCalls) == 0 {
				t.Errorf("%s / %s: answered without calling the tool, though the catalog claims "+
					"ToolCalling=%v. content: %.160q", name, modelID, spec.Capabilities.ToolCalling, resp.Text)
				return
			}
			call := resp.ToolCalls[0]
			in, out := 0, 0
			if resp.Usage != nil {
				in, out = resp.Usage.PromptTokens, resp.Usage.CompletionTokens
			}
			t.Logf("  %-11s OK    %s -> %s(%s)  in=%d out=%d",
				name, modelID, call.Function.Name, call.Function.Arguments, in, out)
		})
	}
}

// sameModelID compares a served id with a catalog id across the one difference
// providers actually have in how they spell the same model: Gemini lists
// "models/gemini-2.5-flash" where the catalog, the picker and every request
// body say "gemini-2.5-flash".
//
// Written after a naive strings.EqualFold reported the gemini row as pointing
// at a model nobody serves, while the model sat in the list one prefix away.
// A live test that cries wolf about a correct row is worse than no live test:
// the next real one gets read as another prefix bug.
func sameModelID(served, want string) bool {
	trim := func(s string) string {
		return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "models/"))
	}
	return trim(served) == trim(want)
}
