package model

import (
	"context"
	"strings"
	"testing"
)

// The built-in provider is the one place in the engine that speaks the user's
// language, because it is an onboarding screen rather than a model (§40).
func TestNoopOnboardingFollowsUILocale(t *testing.T) {
	ask := func(locale string) string {
		p := NewNoopProvider("aetox-review")
		p.Locale = locale
		resp, err := p.Complete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
		})
		if err != nil {
			t.Fatalf("Complete(%q): %v", locale, err)
		}
		return resp.Text
	}

	english := ask("en")
	if !strings.Contains(english, "isn't connected to a real model") {
		t.Errorf("locale en should answer in English, got:\n%s", english)
	}
	if strings.ContainsAny(english, "ก-๙") {
		t.Errorf("the English reply still contains Thai:\n%s", english)
	}

	// Thai is the fallback for empty and for anything unrecognized, because a
	// fresh install runs in Thai.
	for _, locale := range []string{"", "th", "th-TH", "xx"} {
		if got := ask(locale); got != noopOnboardingReply {
			t.Errorf("locale %q should fall back to the Thai reply, got:\n%s", locale, got)
		}
	}
}

// The locale must survive the path the app actually uses, not just a direct
// field set — factory and bootstrap both have to carry it.
func TestLocaleReachesTheNoopProviderThroughBootstrap(t *testing.T) {
	result := BootstrapProvider(BootstrapOptions{Provider: "aetox", Model: "aetox-review", Locale: "en"})
	if result.Provider == nil {
		t.Fatalf("bootstrap returned no provider: %v", result.Error)
	}
	noop, ok := result.Provider.(*NoopProvider)
	if !ok {
		t.Fatalf("expected the built-in provider, got %T", result.Provider)
	}
	if noop.Locale != "en" {
		t.Errorf("Locale = %q, want it carried through BootstrapOptions → ProviderOptions", noop.Locale)
	}
}

// Every real provider ignores it: the locale must not leak into a model request.
func TestLocaleIsIgnoredByRealProviders(t *testing.T) {
	p, err := NewProvider(ProviderOptions{Provider: "ollama", Model: "llama3", Locale: "en"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, isNoop := p.(*NoopProvider); isNoop {
		t.Fatal("ollama must not resolve to the built-in provider")
	}
}
