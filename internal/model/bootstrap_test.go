package model

import (
	"strings"
	"testing"
)

// A local runtime that is not running resolves to an empty model name, which
// used to surface as "model name is required" — an error that sent people
// looking for a text field instead of at their LM Studio server. The bootstrap
// must fall back (the app stays usable) AND say which endpoint went unanswered.
func TestBootstrapUnreachableLocalRuntimeNamesTheEndpoint(t *testing.T) {
	res := BootstrapProvider(BootstrapOptions{Provider: "lmstudio", Model: ""})

	if res.Provider == nil {
		t.Fatal("expected the aetox fallback provider, got none")
	}
	if res.Warning == "" {
		t.Error("fallback bootstrap reported no warning — the UI has nothing to show")
	}
	if res.Error == nil {
		t.Fatal("expected an error explaining why lmstudio was not used")
	}
	msg := res.Error.Error()
	if strings.Contains(msg, ErrMissingModel.Error()) {
		t.Errorf("still reporting the misleading generic error: %s", msg)
	}
	for _, want := range []string{"lmstudio", DefaultBaseURL("lmstudio")} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

// A provider with a real catalog fallback keeps the plain error: an empty
// model there is not a runtime-not-running story.
func TestBootstrapKeepsGenericErrorForCatalogProviders(t *testing.T) {
	err := clarifyBootstrapError(BootstrapOptions{Provider: "anthropic"}, ErrMissingModel)
	if err != ErrMissingModel {
		t.Errorf("expected ErrMissingModel untouched, got %v", err)
	}
}
