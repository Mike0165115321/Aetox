package main

// Doing the same thing again and not being told.
//
// These three cover one failure, watched end to end on 28 ส.ค. The agent was
// asked to fix a deck's opening slide and spent 83 tool calls across two turns
// on it, of which a large share were an edit → open → capture round that could
// not tell it anything. Three of the thirteen captures came back byte-for-byte
// identical to the one before; the same picture was opened in a second tab ten
// seconds after the first; and every round photographed the on-screen tab when
// the complaint had been about the export.
//
// None of that was a wrong answer from a tool. Each tool answered correctly and
// left out the one fact that would have ended the loop, so the tests are all of
// the same shape: the fact is now in the answer.

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The capture that says nothing.
//
// Two captures of an unchanged page used to be two identical pictures with
// nothing marking them as identical, and a model reading the second one reads
// it as "the edit did not land" rather than "nothing changed" — which is how
// one hero slide came to be rewritten four times over a picture that never
// moved.
func TestCaptureKnowsWhenThePageDidNotChange(t *testing.T) {
	tab := &browserTab{}
	first := sha256.Sum256([]byte("the deck, before the edit"))

	// Nothing to compare against yet, so the first capture of a tab is always
	// new. Answering "same" here would be worse than saying nothing.
	if _, _, same := tab.lastShot(first); same {
		t.Fatal("the very first capture reported itself as unchanged")
	}
	tab.rememberShot(first, "output/s1/work/page-1.png")

	where, inARow, same := tab.lastShot(first)
	if !same {
		t.Fatal("the identical picture came back reported as a new one, which is the whole bug")
	}
	if where != "output/s1/work/page-1.png" {
		t.Errorf("it points at %q, want the capture this one is identical to", where)
	}
	if inARow != 1 {
		t.Errorf("inARow = %d, want 1 on the first repeat", inARow)
	}

	if _, inARow, _ = tab.lastShot(first); inARow != 2 {
		t.Errorf("inARow = %d on the second repeat, want 2 — a run is a different fact from a repeat", inARow)
	}

	// A page that did change must never be reported as unchanged. This is the
	// direction that matters: a false "nothing changed" would stop an agent
	// verifying a fix that really had landed.
	second := sha256.Sum256([]byte("the deck, after the edit"))
	if _, _, same := tab.lastShot(second); same {
		t.Fatal("a different picture was reported as the same one")
	}

	// And remembering the new one ends the run rather than carrying it.
	tab.rememberShot(second, "output/s1/work/page-2.png")
	where, inARow, same = tab.lastShot(second)
	if !same || where != "output/s1/work/page-2.png" || inARow != 1 {
		t.Errorf("after a change: same=%v where=%q inARow=%d, want the new picture with a fresh run", same, where, inARow)
	}
}

// The second tab on a page the agent already had open.
//
// newTab minted one unconditionally, so asking for the same picture twice
// produced web-agent-2 and web-agent-3 on one file. The ability to open a
// second tab is not what was wrong and is not what is removed: only opening the
// SAME page twice is.
func TestOpenFindsTheTabAlreadyOnThatPage(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2", "web-3")

	got, found := app.agentTabOn("https://example.com/web-agent-2")
	if !found || got != "web-agent-2" {
		t.Errorf("agentTabOn() = %q, %v — want the tab that is already there", got, found)
	}

	// A file URL is the same file whichever way the drive letter was spelled.
	if _, found := app.agentTabOn("HTTPS://EXAMPLE.COM/web-agent-2"); !found {
		t.Error("the same address in another case read as a different page")
	}

	// A page nobody is on still gets a tab of its own — this guard must not
	// become the one-tab rule again by accident.
	if _, found := app.agentTabOn("https://example.com/somewhere-else"); found {
		t.Error("a page no tab is on was matched to one anyway")
	}

	// And never the user's tab, whatever it is showing. The ownership rule is
	// older than this guard and outranks it.
	if _, found := app.agentTabOn("https://example.com/web-3"); found {
		t.Error("a tab the user opened was offered for reuse")
	}
}

// Reusing a tab means going to it. Every browsing tool after `open` works
// whichever tab is current, so handing back web-agent-2 while web-agent-1 stays
// current would answer with one page and then read another — which is a worse
// failure than the duplicate tab this replaces.
func TestReusingAnOpenPageMovesToThatTab(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2")

	found, ok := app.agentTabOn("https://example.com/web-agent-2")
	if !ok {
		t.Fatal("the tab already on that page was not found")
	}
	if err := app.selectAgentTab(string(found)); err != nil {
		t.Fatalf("selectAgentTab(%q) = %v", found, err)
	}

	current, err := app.agentTab()
	if err != nil {
		t.Fatalf("agentTab() = %v", err)
	}
	if current != found {
		t.Errorf("after reuse the current tab is %q, want %q — read and capture would work the wrong page", current, found)
	}
}

// The answer that never said which tab.
//
// Owner, 28 ส.ค., reading a transcript back: *"เวลาเอเจนทำในแท็ปไหน มันควรระบุ
// แท็ปด้วย"*. Every browser action named the page and none named the tab, which
// was harmless while there was one of them. With the same file open in two, two
// tabs gave two identical answers.
func TestEveryBrowserActionSaysWhichTab(t *testing.T) {
	app := hostWithTabs(t, "web-agent-2", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2")
	browser := &browserSkill{app: app}

	// dialog stands in for the eight actions that carry no tab of their own:
	// they all leave through the same door, which is the only reason one stamp
	// covers them.
	out, err := browser.run(t.Context(), map[string]any{"action": "dialog", "accept": true})
	if err != nil {
		t.Fatalf("browser dialog = %v", err)
	}
	if !strings.Contains(out.Content, "web-agent-2") {
		t.Errorf("the answer does not say which tab it was about:\n%s", out.Content)
	}
	if out.RawOutput != out.Content {
		t.Errorf("RawOutput drifted from Content:\n%q\n%q", out.RawOutput, out.Content)
	}

	// And it is said once. `tabs` lists every id and marks the current one, so a
	// stamp there would be a fourth mention of the same fact in three lines.
	listed, err := browser.run(t.Context(), map[string]any{"action": "tabs", "act": "list"})
	if err != nil {
		t.Fatalf("browser tabs list = %v", err)
	}
	if strings.Contains(listed.Content, "[แท็บ ") {
		t.Errorf("tabs list was stamped on top of its own listing:\n%s", listed.Content)
	}
}

// The two shapes of naming a tab, and why there are two: the page half is
// dropped when the sentence already names the page, and the tab half never is.
func TestBrowserTabRefNamesTheTabWithOrWithoutThePage(t *testing.T) {
	withPage := browserTabRef("web-agent-2", "Deck (file:///deck.html)")
	if !strings.Contains(withPage, "web-agent-2") || !strings.Contains(withPage, "Deck") {
		t.Errorf("browserTabRef() = %q, want both the tab and the page", withPage)
	}

	bare := browserTabRef("web-agent-2", "")
	if !strings.Contains(bare, "web-agent-2") {
		t.Errorf("browserTabRef() with no page = %q, want the tab still named", bare)
	}
	if strings.Contains(bare, "อยู่ที่") {
		t.Errorf("browserTabRef() with no page = %q, want no claim about where it is", bare)
	}

	if got := browserTabRef("", "Deck"); got != "" {
		t.Errorf("browserTabRef() with no tab = %q, want nothing appended", got)
	}
}

// The capture that photographed the wrong renderer.
//
// A deck is exported by a window of its own at a fixed size, never by the tab
// it is being looked at in — so "it looks right on screen" and "it exports
// right" are two claims, and a capture only ever supports the first. The
// complaint that started the loop was about the export.
func TestCaptureOfADeckNamesTheRendererItIsNot(t *testing.T) {
	dir := t.TempDir()
	app := &App{}

	deckPath := filepath.Join(dir, "talk.html")
	if err := os.WriteFile(deckPath, []byte(`<html><body><section class="slide"><h1>Hi</h1></section></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	note := app.deckRendererNote(fileURLForPath(deckPath))
	if note == "" {
		t.Fatal("a capture of a deck said nothing about the renderer that exports it")
	}
	// The size is the whole reason the two can disagree, so it has to be in the
	// sentence rather than left as "a different size".
	if !strings.Contains(note, "1280") || !strings.Contains(note, "720") {
		t.Errorf("the note does not name the export's slide box: %q", note)
	}

	// Every other page is left alone. A note on every capture would be noise,
	// and noise is what stops notes being read.
	plain := filepath.Join(dir, "notes.html")
	if err := os.WriteFile(plain, []byte(`<html><body><p>just a page</p></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := app.deckRendererNote(fileURLForPath(plain)); got != "" {
		t.Errorf("an ordinary page got the deck note: %q", got)
	}
	if got := app.deckRendererNote("https://example.com/deck.html"); got != "" {
		t.Errorf("a remote page got the deck note: %q", got)
	}
	if got := app.deckRendererNote(fileURLForPath(filepath.Join(dir, "gone.html"))); got != "" {
		t.Errorf("a file that is not there got the deck note: %q", got)
	}
}
