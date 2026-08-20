package main

// seed puts a conversation on the App's screen, for tests that used to build
// one by setting App fields directly.
//
// Those fields moved into `conversation` on 2026-08-19 (desktop/conversation.go)
// because none of them was ever a property of the app. The tests that set them
// were saying "an app whose open chat is this" all along; this says it in the
// words the code now uses, and nothing about what they assert changed.
func seed(a *App, conv *conversation) *App {
	// The chat inherits the app's config unless the test gave it one of its own.
	// That is what production does — a conversation is built from the template
	// in App.cfg (applyConfig) — and it keeps every test that sets `cfg:` on the
	// App literal saying what it has always said, now that the readers ask the
	// conversation rather than the app (DECISIONS §155).
	if conv.cfg.SandboxRoot == "" && conv.cfg.ModelProvider == "" {
		conv.cfg = a.cfg
	}
	a.convs = newConversations()
	a.convs.show(conv)
	return a
}
