package model

import (
	"strings"
	"testing"
)

// A provider's /v1/models is a shelf, not a statement about what a model can
// do. NVIDIA's returns 103 bare {id, object, created, owned_by} records with
// one identical created stamp, so nothing in the response separates a chat
// model from an embedding model — and sorted alphabetically the first entry
// was 01-ai/yi-large, a model still listed and no longer deployed. A fresh key
// met it on its first turn as:
//
//	404 Function '23bd454d-…': Not found for account '…'
//
// ResolveDefaultModel takes the first entry of this list, so the order is not
// cosmetic; it is what a new user's first request runs against.
func TestUsableFirstFloatsWhatTheCatalogVouchesFor(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })

	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"nvidia/openai/gpt-oss-120b":   {ToolCall: true, TextOut: true, Context: 128_000},
		"nvidia/google/gemma-3-12b-it": {ToolCall: true, TextOut: true, Context: 131_072},
		// Described, and described as unable to do the job: an embedding model
		// must stay behind the chat models even though the catalog knows it.
		"nvidia/nvidia/nv-embed-v1": {ToolCall: false, TextOut: false},
	}})

	// As discovery returns it: alphabetical, everything the shelf holds.
	discovered := []string{
		"01-ai/yi-large",
		"google/gemma-3-12b-it",
		"nvidia/nv-embed-v1",
		"openai/gpt-oss-120b",
	}
	got := usableFirst("nvidia", discovered)

	if got[0] != "google/gemma-3-12b-it" {
		t.Errorf("first entry is %q; want the catalog-vouched chat model, because "+
			"ResolveDefaultModel runs the first turn against it", got[0])
	}
	if len(got) != len(discovered) {
		t.Fatalf("reordering dropped entries: %d in, %d out — this orders, it does not filter", len(discovered), len(got))
	}
	// Nothing hidden: the unknown id and the embedding model are both still
	// offered, just not first. The catalog describes 43 of NVIDIA's 103 ids and
	// has been measured wrong about 43 more, so it does not get to hide the
	// other 60.
	for _, want := range discovered {
		if !contains(got, want) {
			t.Errorf("%q vanished from the picker", want)
		}
	}
	// Within a group the discovered order survives, so the list stays stable
	// across runs rather than being map-order roulette.
	if idx(got, "gemma") > idx(got, "gpt-oss") {
		t.Errorf("vouched models lost their alphabetical order: %v", got)
	}
	if idx(got, "yi-large") > idx(got, "nv-embed") {
		t.Errorf("the remainder lost its alphabetical order: %v", got)
	}
}

// With nothing in the catalog — every unit test, and every local runtime whose
// model names no catalog has heard of — the list must come back untouched.
// Reordering that guessed would be worse than not reordering.
func TestUsableFirstIsANoOpWithoutACatalog(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(nil)

	discovered := []string{"ornith:9b", "qwen3:8b"}
	got := usableFirst("ollama", discovered)
	if len(got) != 2 || got[0] != "ornith:9b" || got[1] != "qwen3:8b" {
		t.Fatalf("want the discovered order untouched, got %v", got)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func idx(list []string, substr string) int {
	for i, s := range list {
		if strings.Contains(s, substr) {
			return i
		}
	}
	return -1
}
