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
		p := NewNoopProvider("aetox-grid")
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
	result := BootstrapProvider(BootstrapOptions{Provider: "aetox", Model: "aetox-grid", Locale: "en"})
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

// Every built-in model is a surface a user can pick from the model list, so
// every one of them has to answer in the UI's language — not just the default
// one (§41). This walks the whole family rather than spot-checking.
func TestEveryBuiltinModelSpeaksEnglishWhenAsked(t *testing.T) {
	thai := func(s string) bool {
		for _, r := range s {
			if r >= 0x0E00 && r <= 0x0E7F {
				return true
			}
		}
		return false
	}

	for _, modelName := range []string{
		"aetox-grid", "aetox-image:test", "aetox-think:test", "aetox-markdown:test",
	} {
		p := NewNoopProvider(modelName)
		p.Locale = "en"
		resp, err := p.Complete(context.Background(), Request{
			Model:    modelName,
			Messages: []Message{{Role: RoleUser, Content: "hello"}},
		})
		if err != nil {
			t.Fatalf("%s: %v", modelName, err)
		}
		if thai(resp.Text) {
			t.Errorf("%s still answers with Thai under locale en:\n%s", modelName, resp.Text)
		}
		if thai(resp.ReasoningContent) {
			t.Errorf("%s streams Thai reasoning under locale en:\n%s", modelName, resp.ReasoningContent)
		}
	}

	// The tools model answers with tool calls rather than text — its arguments
	// are what reaches the user, through the todo panel and the ask_user cards.
	tools := NewNoopProvider("aetox-tools:test")
	tools.Locale = "en"
	resp, err := tools.Complete(context.Background(), Request{
		Model:    "aetox-tools:test",
		Messages: []Message{{Role: RoleUser, Content: "go"}},
	})
	if err != nil {
		t.Fatalf("tools model: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Fatal("tools model must open with a tool call")
	}
	if thai(resp.ToolCalls[0].Function.Arguments) {
		t.Errorf("tools model sends Thai todo items under locale en:\n%s", resp.ToolCalls[0].Function.Arguments)
	}

	// And Thai stays the default for everyone.
	for _, modelName := range []string{"aetox-grid", "aetox-markdown:test"} {
		p := NewNoopProvider(modelName)
		resp, err := p.Complete(context.Background(), Request{
			Model:    modelName,
			Messages: []Message{{Role: RoleUser, Content: "สวัสดี"}},
		})
		if err != nil {
			t.Fatalf("%s: %v", modelName, err)
		}
		if !thai(resp.Text) {
			t.Errorf("%s should default to Thai, got:\n%s", modelName, resp.Text)
		}
	}
}
