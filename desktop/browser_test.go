package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		name, a, b string
		want       bool
	}{
		{"identical", "https://example.com/page", "https://example.com/other", true},
		{"different host", "https://example.com/", "https://evil.com/", false},
		{"different scheme", "http://example.com/", "https://example.com/", false},
		{"page claims different site (spoof attempt)", "https://evil.com/", "https://accounts.google.com/login", false},
		{"empty source", "", "https://example.com/", false},
		{"malformed source", "not a url", "https://example.com/", false},
		// file pages have no host — scheme match is the whole check there
		{"file page, same path", "file:///C:/Users/x/page.html", "file:///C:/Users/x/page.html", true},
		{"file page, different local path", "file:///C:/a.html", "file:///E:/other.html", true},
		{"file page claims a website", "file:///C:/a.html", "https://accounts.google.com/", false},
		{"website claims a file path", "https://evil.com/", "file:///C:/a.html", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sameOrigin(c.a, c.b); got != c.want {
				t.Errorf("sameOrigin(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestNewMessageTokenUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		tok := newMessageToken()
		if tok == "" {
			t.Fatal("newMessageToken() returned empty string")
		}
		if seen[tok] {
			t.Fatalf("newMessageToken() returned duplicate: %q", tok)
		}
		seen[tok] = true
	}
}

func TestOnMessageRejectsSpoofedMetaURL(t *testing.T) {
	h := &browserHost{app: &App{}}
	tab := &browserTab{}
	// Page at evil.com claims to be accounts.google.com — must be rejected.
	h.onMessage("tab1", tab, `{"__aetox":"meta","title":"Google","url":"https://accounts.google.com/login"}`, "https://evil.com/")

	title, url := tab.meta()
	if title != "" || url != "" {
		t.Errorf("tab.meta() = (%q, %q), want unchanged/empty after a spoofed meta message", title, url)
	}
}

func TestOnMessageAcceptsGenuineMeta(t *testing.T) {
	h := &browserHost{app: &App{}}
	tab := &browserTab{}
	h.onMessage("tab1", tab, `{"__aetox":"meta","title":"Example","url":"https://example.com/page"}`, "https://example.com/page")

	title, url := tab.meta()
	if title != "Example" || url != "https://example.com/page" {
		t.Errorf("tab.meta() = (%q, %q), want (%q, %q)", title, url, "Example", "https://example.com/page")
	}
}

func TestOnMessageRejectsTextWithoutPendingRequest(t *testing.T) {
	h := &browserHost{app: &App{}}
	tab := &browserTab{} // textCh is nil: nothing is waiting
	// Must not panic (sending on a nil channel would block/panic if reached).
	h.onMessage("tab1", tab, `{"__aetox":"text","token":"whatever","url":"https://example.com/","text":"unsolicited"}`, "https://example.com/")
}

func TestOnMessageRejectsTextWithWrongToken(t *testing.T) {
	h := &browserHost{app: &App{}}
	ch := make(chan browserSnapshot, 1)
	tab := &browserTab{textCh: ch, textToken: "real-token"}

	h.onMessage("tab1", tab, `{"__aetox":"text","token":"forged-token","url":"https://example.com/","text":"fake"}`, "https://example.com/")

	select {
	case got := <-ch:
		t.Fatalf("channel received %q, want no delivery for a wrong-token message", got)
	default:
	}
}

func TestOnMessageAcceptsTextWithMatchingToken(t *testing.T) {
	h := &browserHost{app: &App{}}
	ch := make(chan browserSnapshot, 1)
	tab := &browserTab{textCh: ch, textToken: "real-token"}

	h.onMessage("tab1", tab, `{"__aetox":"text","token":"real-token","url":"https://example.com/","text":"real content"}`, "https://example.com/")

	select {
	case got := <-ch:
		if got.Text != "real content" {
			t.Errorf("channel received %q, want %q", got.Text, "real content")
		}
	default:
		t.Fatal("channel received nothing, want the matching-token message delivered")
	}
}

// The ref reaches the script. Spelled aetoxFind since 2026-08-22: the search
// runs over every root the page has rather than `document` alone, because
// textScript now hands out refs for nodes inside shadow roots and same-origin
// frames. What is asserted is unchanged — this call acts on that ref and no
// other.
func TestClickScriptEmbedsRef(t *testing.T) {
	js := clickScript("tok", 42)
	if !strings.Contains(js, `aetoxFind(42)`) {
		t.Errorf("clickScript(42) = %q, want it to target ref 42", js)
	}
}

// typeScript embeds arbitrary user/page-adjacent text into a JS string via
// json.Marshal — this is the one thing here worth a real test, since getting
// that escaping wrong (quotes, backslashes, newlines) would either break the
// generated script or, worse, let attacker-controlled text break out of the
// string literal into executable JS.
func TestTypeScriptEscapesTextSafely(t *testing.T) {
	cases := []string{
		`hello`,
		`it's a "quoted" string`,
		`backslash \ and newline` + "\n" + `continues`,
		`</script><script>alert(1)</script>`,
		``,
	}
	for _, text := range cases {
		js := typeScript("tok", 7, text, false)
		wantEncoded, err := json.Marshal(text)
		if err != nil {
			t.Fatalf("json.Marshal(%q): %v", text, err)
		}
		if !strings.Contains(js, string(wantEncoded)) {
			t.Errorf("typeScript(7, %q) does not contain the expected JSON-escaped literal %s\ngot: %s", text, wantEncoded, js)
		}
		if !strings.Contains(js, `aetoxFind(7)`) {
			t.Errorf("typeScript(7, %q) = %q, want it to target ref 7", text, js)
		}
	}
}

func TestTypeScriptSelectAndEnterVariants(t *testing.T) {
	js := typeScript("tok", 3, "Thailand", false)
	if !strings.Contains(js, `el.tagName==="SELECT"`) || !strings.Contains(js, "HTMLSelectElement") {
		t.Errorf("typeScript must handle select elements via the native value setter, got: %s", js)
	}
	if strings.Contains(js, "requestSubmit") {
		t.Errorf("enter=false must not emit the Enter/submit snippet, got: %s", js)
	}

	js = typeScript("tok", 3, "query", true)
	for _, want := range []string{`new KeyboardEvent("keydown"`, "requestSubmit", "notHandled"} {
		if !strings.Contains(js, want) {
			t.Errorf("typeScript enter=true missing %q, got: %s", want, js)
		}
	}
}

func TestTextScriptListsSelectOptions(t *testing.T) {
	js := textScript("tok", "")
	if !strings.Contains(js, "[options: ") {
		t.Errorf("textScript should surface select options so the model knows what browser_type can choose, got: %s", js)
	}
}

// fakeBackend is a hostBackend that queues commands without ever touching a
// real webview, so the bookkeeping around them can be tested on any platform.
// It exists because that bookkeeping used to need a live HWND to reach.
type fakeBackend struct {
	mu   sync.Mutex
	cmds []func()
}

func (b *fakeBackend) start() error { return nil }

func (b *fakeBackend) do(fn func()) {
	b.mu.Lock()
	b.cmds = append(b.cmds, fn)
	b.mu.Unlock()
}

// drain runs the queued commands in order, the way a real host's pump does.
func (b *fakeBackend) drain() {
	for {
		b.mu.Lock()
		if len(b.cmds) == 0 {
			b.mu.Unlock()
			return
		}
		fn := b.cmds[0]
		b.cmds = b.cmds[1:]
		b.mu.Unlock()
		fn()
	}
}

func (b *fakeBackend) openTab(id, url string, x, y, w, h int, cb tabCallbacks) tabView {
	return &fakeView{}
}

// fakeView records what the portable layer asked a tab to do.
type fakeView struct {
	lastJS      string
	visible     []bool
	zoom        float64
	bounds      [4]int
	destroyed   bool
	devToolsHit bool
	shot        shotResult
	// calls records every engine call, so a test can assert the settings a deck
	// depends on (backgrounds on, no header) without a webview.
	calls [][2]string
	reply engineReply
	// behind is what the deck export asks for and nothing else does: laid out
	// and painting, with the app's own webview drawn over it.
	behind bool
}

func (v *fakeView) navigate(url string)  { v.lastJS = "navigate:" + url }
func (v *fakeView) eval(js string)       { v.lastJS = js }
func (v *fakeView) setZoom(f float64)    { v.zoom = f }
func (v *fakeView) openDevTools()        { v.devToolsHit = true }
func (v *fakeView) destroy()             { v.destroyed = true }
func (v *fakeView) setVisible(show bool) { v.visible = append(v.visible, show) }
func (v *fakeView) capture() <-chan shotResult {
	out := make(chan shotResult, 1)
	out <- v.shot
	return out
}

func (v *fakeView) sendBehind() { v.behind = true }

func (v *fakeView) callEngine(method, paramsJSON string) <-chan engineReply {
	v.calls = append(v.calls, [2]string{method, paramsJSON})
	out := make(chan engineReply, 1)
	out <- v.reply
	return out
}
func (v *fakeView) setBounds(x, y, w, h int) {
	v.bounds = [4]int{x, y, w, h}
}

// A tab is only registered at the END of open()'s own queued command, so any
// lookup done on the caller's goroutine finds nil for every call made right
// after BrowserOpen — which silently dropped the bounds correction the frontend
// sends once the address bar has appeared, leaving the tab's window covering
// the toolbar. withTab must therefore resolve the tab inside the queue.
func TestOnTabResolvesAfterAQueuedOpen(t *testing.T) {
	b := &fakeBackend{}
	h := &browserHost{backend: b, tabs: map[string]*browserTab{}, views: map[string]tabView{}}
	registered := &fakeView{}

	// Stands in for open(): registers the tab only when its command runs.
	b.do(func() {
		h.mu.Lock()
		h.tabs["web-1"] = &browserTab{}
		h.views["web-1"] = registered
		h.mu.Unlock()
	})
	// Stands in for BrowserSetBounds, issued before that command has run.
	var got tabView
	h.onTab("web-1", func(v tabView, _ *browserTab) { got = v })

	b.drain()

	if got != registered {
		t.Fatalf("command dropped: onTab never reached the registered tab")
	}
}

// navCompleted carries three rules that only ever ran against a live WebView2
// before the platform seam existed: record the outcome before waking waiters,
// do not surface a tab the UI has hidden, and re-assert the emulation zoom that
// a cross-origin navigation resets.
func TestNavCompletedRaisesReassertsZoomAndAsksForMeta(t *testing.T) {
	h := &browserHost{app: &App{}, tabs: map[string]*browserTab{}}
	tab := &browserTab{navDone: make(chan struct{})}
	tab.zoom = 0.5
	view := &fakeView{}

	h.navCompleted(tab, view, true)

	if !tab.navLoaded() {
		t.Error("navOK not recorded")
	}
	select {
	case <-tab.navDone:
	default:
		t.Error("navDone still open — a waiter would hang")
	}
	if len(view.visible) != 1 || !view.visible[0] {
		t.Errorf("tab not raised: setVisible calls = %v", view.visible)
	}
	if view.zoom != 0.5 {
		t.Errorf("zoom re-asserted as %v, want 0.5 — a cross-site navigation resets it", view.zoom)
	}
	if !strings.Contains(view.lastJS, "__aetox") {
		t.Errorf("did not ask the page for its title/url, last js = %q", view.lastJS)
	}
}

// A tab the user has switched away from must stay down when its page finishes
// loading, or it pops over the UI on every background navigation.
func TestNavCompletedLeavesAHiddenTabHidden(t *testing.T) {
	h := &browserHost{app: &App{}, tabs: map[string]*browserTab{}}
	tab := &browserTab{navDone: make(chan struct{}), hidden: true}
	view := &fakeView{}

	h.navCompleted(tab, view, true)

	for _, shown := range view.visible {
		if shown {
			t.Fatal("raised a tab the UI had hidden")
		}
	}
}

// browser_open used to report success on ANY completed navigation, so a
// file:// path that does not exist came back with a green tick and the model
// went on working against Chrome's own File-not-found page.
func TestAwaitNavigationFailsWhenThePageDidNotLoad(t *testing.T) {
	tab := &browserTab{navDone: make(chan struct{})}
	tab.setNavOK(false)
	close(tab.navDone)

	err := tab.awaitNavigation(context.Background(), time.Second)
	if err == nil {
		t.Fatal("expected an error for a navigation that failed, got nil")
	}
	// Must not be reported as "still loading" — that sends the caller back to
	// waiting instead of fixing the URL.
	if strings.Contains(err.Error(), "did not finish loading") {
		t.Errorf("error = %q, want it to name a load failure, not a timeout", err)
	}
}

func TestAwaitNavigationPassesWhenThePageLoaded(t *testing.T) {
	tab := &browserTab{navDone: make(chan struct{})}
	tab.setNavOK(true)
	close(tab.navDone)

	if err := tab.awaitNavigation(context.Background(), time.Second); err != nil {
		t.Fatalf("awaitNavigation on a loaded page: %v", err)
	}
}

func TestAwaitNavigationTimesOutWhileStillLoading(t *testing.T) {
	tab := &browserTab{navDone: make(chan struct{})} // never completes
	if err := tab.awaitNavigation(context.Background(), 10*time.Millisecond); err == nil {
		t.Fatal("expected a timeout error while the page is still loading, got nil")
	}
}

func TestOnTabSkipsAnUnknownTab(t *testing.T) {
	b := &fakeBackend{}
	h := &browserHost{backend: b, tabs: map[string]*browserTab{}, views: map[string]tabView{}}
	ran := false
	h.onTab("gone", func(tabView, *browserTab) { ran = true })
	b.drain()
	if ran {
		t.Fatal("ran against a tab that does not exist")
	}
}

// A tab whose openTab failed is registered without a view, and onTab must treat
// that as no tab rather than handing fn a nil interface to call through.
func TestOnTabSkipsATabWithNoView(t *testing.T) {
	b := &fakeBackend{}
	h := &browserHost{backend: b, tabs: map[string]*browserTab{"web-1": {}}, views: map[string]tabView{}}
	ran := false
	h.onTab("web-1", func(tabView, *browserTab) { ran = true })
	b.drain()
	if ran {
		t.Fatal("ran against a tab that never got a webview")
	}
	if h.live("web-1") {
		t.Error("live() calls a viewless tab alive")
	}
}

// A reused tab has to be awaitable twice, and the second wait must report the
// second navigation — not the first one's verdict, still sitting in a latch
// that was closed and never reopened.
//
// This is the half of tab reuse that cannot be seen by looking: the agent
// navigates its one tab to a new page, the wait returns instantly because the
// latch is already closed, and the tools that follow read the OLD page while
// reporting the new URL. Before armNavigation existed there was no second
// wait at all, because there was no second navigation — every open made a
// fresh tab (the "เปิดใหม่ ๆ รัว ๆ" the owner watched happen on 2026-08-10).
func TestAReusedTabIsAwaitedAgainRatherThanAnsweringWithTheLastResult(t *testing.T) {
	tab := &browserTab{}
	h := &browserHost{app: &App{}, tabs: map[string]*browserTab{}}

	// First navigation lands, and fails.
	h.navCompleted(tab, &fakeView{}, false)
	if err := tab.awaitNavigation(context.Background(), time.Second); err == nil {
		t.Fatal("a failed first navigation was reported as loaded")
	}

	// Re-armed for the next one: the wait must now block rather than hand back
	// the failure above.
	tab.armNavigation()
	done := make(chan error, 1)
	go func() { done <- tab.awaitNavigation(context.Background(), 2*time.Second) }()
	select {
	case err := <-done:
		t.Fatalf("the second wait returned before the second navigation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	h.navCompleted(tab, &fakeView{}, true)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the second navigation succeeded and the wait said %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("the second wait never woke — a reused tab hangs the turn")
	}
}
