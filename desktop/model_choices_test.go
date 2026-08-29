package main

import (
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/model"
)

// The picker's middle answer: what this machine already knows a provider
// serves, for the moment the provider itself cannot be asked.
//
// Owner, 28 ส.ค.: "ทำไมเจ้านี้ขึ้นแค่ โมเดลตัวเดียวครับ" — the Alibaba card
// listed one model. Nothing was wrong with Alibaba; the key was issued for a
// different region than the base URL it was being sent to, so live discovery
// answered 401 and the chain fell to the single hard-coded FallbackModel.
// model-catalog.json in the same data root described 54 models for that
// provider, and the price column two lines to the right was already reading it.
func TestTheModelPickerFallsBackToTheCatalogBeforeTheHardCodedName(t *testing.T) {
	isolateUserDirs(t)
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{cfg: config.Config{}, dbDir: t.TempDir()}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	root, err := config.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := model.SaveModelCatalog(root, &model.ModelCatalog{
		Fetched: time.Now(),
		Models: map[string]model.ModelFacts{
			"alibaba/qwen3.8-max": {
				Price: model.ModelPrice{Input: 1.2, Output: 6}, Context: 262_144,
				ToolCall: true, Output: []string{"text"}, Released: "2026-08-01",
			},
			"alibaba/qwen-flash": {
				Price: model.ModelPrice{Input: 0.05, Output: 0.4}, Context: 1_000_000,
				ToolCall: true, Output: []string{"text"}, Released: "2026-07-01",
			},
			// On the shelf, not in the menu: a speech recognizer cannot hold a
			// conversation, and offering one is how a picker wastes a turn.
			"alibaba/qwen3-asr-flash": {
				Price: model.ModelPrice{Input: 0.035, Output: 0.035}, Context: 53_248,
				ToolCall: false, Output: []string{"text"}, Released: "2026-07-01",
			},
			// Another company's row in the same table.
			"deepseek/deepseek-v4-flash": {
				Price: model.ModelPrice{Input: 0.14, Output: 0.28}, Context: 1_000_000,
				ToolCall: true, Output: []string{"text"}, Released: "2026-07-01",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	got := a.catalogModelChoices("alibaba")

	if len(got) < 2 {
		t.Fatalf("the picker still has a shelf of %d: %v", len(got), got)
	}
	if !contains(got, "qwen3.8-max") || !contains(got, "qwen-flash") {
		t.Errorf("the catalog's chat models are missing: %v", got)
	}
	if contains(got, "qwen3-asr-flash") {
		t.Errorf("a speech recognizer is being offered as a chat model: %v", got)
	}
	if contains(got, "deepseek-v4-flash") {
		t.Errorf("another provider's model leaked into the list: %v", got)
	}
	// The name Aetox ships as known-good survives even when the catalog does
	// not carry it — a menu built from a third party must not be able to drop
	// the one entry we vouch for.
	for _, name := range model.ModelChoices("alibaba") {
		if !contains(got, name) {
			t.Errorf("the static fallback %q fell out of the menu: %v", name, got)
		}
	}
	// One order, not "the catalog's, plus one".
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("the list is not sorted: %v", got)
			break
		}
	}
}

// A machine that has never fetched the catalog must behave exactly as it did
// before: the chain falls through to the static name rather than to nothing.
func TestTheModelPickerIsUnchangedWithoutACatalog(t *testing.T) {
	isolateUserDirs(t)
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{cfg: config.Config{}, dbDir: t.TempDir()}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})

	if got := a.catalogModelChoices("alibaba"); got != nil {
		t.Errorf("with no catalog on disk the picker answered %v", got)
	}
}

