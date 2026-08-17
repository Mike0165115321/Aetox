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

	// Thai is what "nothing chosen" means — a fresh install runs in Thai — so
	// an empty locale and Thai itself both land on the Thai reply.
	for _, locale := range []string{"", "th", "th-TH"} {
		if got := ask(locale); got != noopOnboarding["th"] {
			t.Errorf("locale %q should answer in Thai, got:\n%s", locale, got)
		}
	}

	// An unrecognized language is NOT the same as nothing chosen: someone is
	// there, and they picked something that is not Thai. English is the better
	// guess for them than the language they did not pick.
	if got := ask("xx"); got != noopOnboarding["en"] {
		t.Errorf("an unknown locale should fall back to English, got:\n%s", got)
	}
}

// A third language must be a data change and nothing else. This is the whole
// point of the phrase map that replaced pick(th, en): the file used to encode
// "there are exactly two" in the shape of every canned string, so a third could
// not be written without first editing every one of them.
//
// It reaches past the exported surface deliberately: no real phrase in this
// package carries a third language yet, so there is nothing Complete could be
// asked. The shape is what is being pinned — a branch on "en" must not come
// back, whichever language is written next.
func TestAThirdLanguageIsDataNotCode(t *testing.T) {
	greeting := phrase{"th": "สวัสดี", "en": "hello", "ja": "こんにちは"}

	for _, tc := range []struct{ locale, want string }{
		{"ja", "こんにちは"},
		{"ja-JP", "こんにちは"}, // a tag with a region resolves like its language
		{"JA", "こんにちは"},    // and case is not a language
		{"en", "hello"},
		{"th", "สวัสดี"},
		{"", "สวัสดี"},  // nothing chosen: a fresh install runs in Thai
		{"fr", "hello"}, // never translated: English, which they can more likely read than Thai
	} {
		if got := inLocale(tc.locale, greeting); got != tc.want {
			t.Errorf("inLocale(%q) = %q, want %q", tc.locale, got, tc.want)
		}
	}

	// The whole tag wins over its language half, because pt-BR is a language of
	// its own here and not a flavour of pt. Reading it as pt would hand a
	// Brazilian the European translation and look like it worked.
	regional := phrase{"th": "ไทย", "en": "English", "pt": "português europeu", "pt-BR": "português brasileiro"}
	for _, tc := range []struct{ locale, want string }{
		{"pt-BR", "português brasileiro"},
		{"pt-PT", "português europeu"}, // no entry of its own: the language half
		{"pt", "português europeu"},
	} {
		if got := inLocale(tc.locale, regional); got != tc.want {
			t.Errorf("inLocale(%q) = %q, want %q", tc.locale, got, tc.want)
		}
	}

	// And the provider reads it through the same resolver, so a locale the
	// engine was handed cannot mean one thing here and another there.
	p := NewNoopProvider("aetox-grid")
	p.Locale = "ja"
	if got := p.say(greeting); got != "こんにちは" {
		t.Errorf("say under locale ja = %q, want the Japanese entry", got)
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

	// The retired names are walked alongside the current one on purpose: they
	// still resolve, so they still have to answer in the right language.
	for _, modelName := range []string{
		"aetox-grid", "aetox-render:test", "aetox-think:test",
		"aetox-image:test", "aetox-markdown:test",
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

	// Same for the delegation bench: its first move is a `task` call, and the
	// brief inside it is what the user reads in the timeline.
	sub := NewNoopProvider("aetox-subagent:test")
	sub.Locale = "en"
	subResp, err := sub.Complete(context.Background(), Request{
		Model:    "aetox-subagent:test",
		Messages: []Message{{Role: RoleUser, Content: "go"}},
		Tools:    []ToolDefinition{{Type: "function", Function: ToolFunction{Name: "task"}}},
	})
	if err != nil {
		t.Fatalf("subagent model: %v", err)
	}
	if len(subResp.ToolCalls) == 0 {
		t.Fatal("subagent model must open by starting a delegate")
	}
	if thai(subResp.ToolCalls[0].Function.Arguments) {
		t.Errorf("subagent model briefs its delegate in Thai under locale en:\n%s", subResp.ToolCalls[0].Function.Arguments)
	}

	// And Thai stays the default for everyone.
	for _, modelName := range []string{"aetox-grid", "aetox-render:test"} {
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

// The guide is answered by the engine on the normal reply path (§42), so every
// question the UI can show must resolve to an answer — in every language it is
// offered in, on any built-in model.
func TestGuideQuestionsAllAnswer(t *testing.T) {
	thai := func(s string) bool {
		for _, r := range s {
			if r >= 0x0E00 && r <= 0x0E7F {
				return true
			}
		}
		return false
	}
	// Han, to tell a real Chinese answer from the English one falling through.
	// The fallback is deliberate and right for an untranslated bench script; it
	// would be a silent hole in the guide, which is product copy a new user
	// reads before anything else works.
	han := func(s string) bool {
		for _, r := range s {
			if r >= 0x4E00 && r <= 0x9FFF {
				return true
			}
		}
		return false
	}

	for _, locale := range []string{"th", "en", "zh"} {
		topics := GuideTopics(locale)
		if len(topics) < 6 {
			t.Fatalf("locale %q: only %d guide topics", locale, len(topics))
		}
		for _, topic := range topics {
			if topic.ID == "" || topic.Question == "" {
				t.Errorf("locale %q: incomplete topic %+v", locale, topic)
			}
			p := NewNoopProvider("aetox-grid")
			p.Locale = locale
			resp, err := p.Complete(context.Background(), Request{
				Messages: []Message{{Role: RoleUser, Content: topic.Question}},
			})
			if err != nil {
				t.Fatalf("%s/%s: %v", locale, topic.ID, err)
			}
			if resp.Text == noopOnboarding["th"] || resp.Text == noopOnboarding["en"] {
				t.Errorf("%s/%s fell through to the onboarding reply — the question did not match",
					locale, topic.ID)
			}
			if len(resp.Text) < 120 {
				t.Errorf("%s/%s answer is too short to be an answer: %q", locale, topic.ID, resp.Text)
			}
			if locale == "en" && thai(resp.Text) {
				t.Errorf("en/%s answered with Thai:\n%s", topic.ID, resp.Text)
			}
			if locale == "zh" && (!han(resp.Text) || thai(resp.Text)) {
				t.Errorf("zh/%s did not answer in Chinese:\n%s", topic.ID, resp.Text)
			}
		}
	}

	// A question asked before a language switch must still resolve afterwards.
	p := NewNoopProvider("aetox-grid")
	p.Locale = "en"
	thaiQuestion := GuideTopics("th")[0].Question
	resp, err := p.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: thaiQuestion}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text == noopOnboarding["en"] {
		t.Error("a Thai question must still match after switching to English")
	}

	// And it answers the same on whichever built-in model is selected — the
	// user clicked an option Aetox offered, not a test scenario.
	tools := NewNoopProvider("aetox-tools:test")
	toolsResp, err := tools.Complete(context.Background(), Request{
		Model:    "aetox-tools:test",
		Messages: []Message{{Role: RoleUser, Content: GuideTopics("th")[0].Question}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(toolsResp.ToolCalls) != 0 {
		t.Error("a guide question must not be hijacked by the tools test script")
	}
}
