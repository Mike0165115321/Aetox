package model

import (
	"os"
	"path/filepath"
	"testing"
)

// The bug this function was written for, pinned.
//
// Capabilities moved to the fetched catalog — thinking depths, vision,
// documents, tool calling — and neither entry point installed one at startup.
// The desktop installed a catalog only when its usage panel happened to run,
// and the CLI never installed one at all, so `aetox --model deepseek-v4-flash`
// reported no thinking level for a model that has three. The owner found it
// with `go test ./...`, on a package this session had not been running.
//
// The lesson is not "add a TestMain". It is that a capability answered from
// data has to have that data installed before anything asks, and that has to be
// the first thing an entry point does rather than a side effect of a screen.
func TestInstalledCatalogIsWhatCapabilitiesRead(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })

	dir := t.TempDir()
	SetModelCatalog(nil)

	// With nothing installed, a model that reasons reports no dial. That is the
	// honest answer and the state a user must never be left in.
	if caps := ResolveThinkingCapabilities("deepseek", "deepseek-v4-flash"); caps.Supported {
		t.Fatalf("a dial appeared with no catalog installed: %+v", caps)
	}

	if err := SaveModelCatalog(dir, &ModelCatalog{
		Source: "test",
		Models: map[string]ModelFacts{
			"deepseek/deepseek-v4-flash": {
				Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true,
				ReasoningLevels: []string{"low", "high", "max"},
				Input:           []string{"text"}, Output: []string{"text"},
			},
		},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	InstallCachedCatalog(dir)

	caps := ResolveThinkingCapabilities("deepseek", "deepseek-v4-flash")
	if !caps.Supported {
		t.Fatalf("the cached table was installed and the dial is still missing: %+v", caps)
	}
	if NormalizeThinkingLevel("deepseek", "deepseek-v4-flash", "HIGH") != "high" {
		t.Error("a level the model offers did not survive normalization")
	}
}

// A first run has no cache, and that is not a failure: the install is silent,
// leaves whatever was there alone, and never reaches the network. Startup must
// not wait on models.dev.
func TestInstallCachedCatalogIsSilentWithNothingToRead(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })

	marker := &ModelCatalog{Source: "already here", Models: map[string]ModelFacts{
		"deepseek/deepseek-v4-flash": {Context: 1, Output: []string{"text"}},
	}}
	SetModelCatalog(marker)

	InstallCachedCatalog(filepath.Join(t.TempDir(), "does-not-exist"))
	if installedCatalog != marker {
		t.Error("a missing cache replaced the table that was already installed")
	}

	InstallCachedCatalog("")
	if installedCatalog != marker {
		t.Error("an empty data root replaced the installed table")
	}

	// A cache that parses to nothing usable is the same case as no cache: it
	// must not clear what is working.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, modelCatalogFile), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	InstallCachedCatalog(dir)
	if installedCatalog != marker {
		t.Error("an empty cache file replaced the installed table")
	}
}
