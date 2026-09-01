package main

import (
	"strings"
	"testing"
)

// The bug this exists for: the owner typed **ยูทูป** into the address bar and
// got https://ยูทูป, punycoded to xn--o3cit6gb, refused by DNS. The address bar
// had one job where every browser's has two.
func TestResolveAddressTellsAPlaceFromAQuestion(t *testing.T) {
	places := map[string]string{
		"https://example.com":  "https://example.com",
		"http://example.com":   "http://example.com",
		"about:blank":          "about:blank",
		"example.com":          "https://example.com",
		"example.com/a/b":      "https://example.com/a/b",
		"localhost:8014":       "http://localhost:8014",
		"localhost":            "http://localhost",
		"127.0.0.1":            "http://127.0.0.1",
		"127.0.0.1:8080/admin": "http://127.0.0.1:8080/admin",
		"192.168.1.5":          "http://192.168.1.5",
		"[::1]:8080":           "http://[::1]:8080",
		"dev:8080":             "http://dev:8080",
		"example.com:8443/x":   "https://example.com:8443/x",
		`C:\site\index.html`:   "file:///C:/site/index.html",
		"E:/site/index.html":   "file:///E:/site/index.html",
		"/usr/share/doc/a.htm": "/usr/share/doc/a.htm",
		"./notes.html":         "./notes.html",
	}
	for in, want := range places {
		got := resolveAddress(in)
		if got.URL != want {
			t.Errorf("resolveAddress(%q).URL = %q, want %q", in, got.URL, want)
		}
		if got.Query != "" {
			t.Errorf("resolveAddress(%q) read a place as a search for %q", in, got.Query)
		}
	}

	questions := []string{
		"ยูทูป",
		"youtube",
		"how to write a go test",
		"example.com is down", // a sentence about a domain is not a domain
		"3.14",                // arithmetic far more often than a hostname
		"weather tomorrow",
	}
	for _, in := range questions {
		got := resolveAddress(in)
		if got.Query != in {
			t.Errorf("resolveAddress(%q).Query = %q, want the text back", in, got.Query)
		}
		if got.URL != "" {
			t.Errorf("resolveAddress(%q) navigated to %q instead of searching", in, got.URL)
		}
		if got.SearchURL == "" {
			t.Errorf("resolveAddress(%q) has nowhere to search", in)
		}
	}
}

// The query has to survive the trip into a URL, or a Thai search becomes a
// broken one — which is the same class of failure this change exists to end.
func TestASearchURLCarriesTheQueryIntact(t *testing.T) {
	got := resolveAddress("ยูทูป")
	const want = "https://www.google.com/search?q=%E0%B8%A2%E0%B8%B9%E0%B8%97%E0%B8%B9%E0%B8%9B"
	if got.SearchURL != want {
		t.Errorf("SearchURL = %q, want %q", got.SearchURL, want)
	}
	if got := resolveAddress("a b&c=d"); got.SearchURL != "https://www.google.com/search?q=a+b%26c%3Dd" {
		t.Errorf("an unescaped query would break the search URL: %q", got.SearchURL)
	}
}

// Empty in, empty out — the address bar returns early on this, and neither a
// navigation nor a search for nothing is a sensible thing to do.
func TestResolveAddressOnNothing(t *testing.T) {
	got := resolveAddress("   ")
	if got.URL != "" || got.Query != "" || got.SearchURL != "" {
		t.Errorf("resolveAddress(blank) = %+v, want all empty", got)
	}
}

// The agent gets the other policy. It already has web_search, and an `open`
// that quietly searched would teach it that `open` is a search box — it would
// keep reaching for the wrong tool and keep getting away with it.
func TestTheAgentIsToldToSearchRatherThanSearchedFor(t *testing.T) {
	resolved, query := normalizeWorkbenchURL("ยูทูป", "", nil)
	if query != "ยูทูป" {
		t.Errorf("query = %q, want the text back so open can refuse it", query)
	}
	if resolved != "" {
		t.Errorf("open would have navigated to %q", resolved)
	}

	// And a real address is still just an address.
	resolved, query = normalizeWorkbenchURL("example.com", "", nil)
	if resolved != "https://example.com" || query != "" {
		t.Errorf("normalizeWorkbenchURL(example.com) = %q, %q", resolved, query)
	}
}

// Which scheme a scheme-less address gets, and what is tried when that guess is
// wrong. This is the Chrome/Edge behaviour, and it is two rules, not one:
// upgrade what can hold a certificate, and never upgrade what cannot.
//
// The bug it closes: a user starts XAMPP, types `localhost`, and Apache is
// serving plain http on port 80. Every other browser on their machine opens it.
// Aetox stamped https:// on the front, WebView2 found nothing on 443, and the
// pane stayed empty with nothing on screen saying why.
func TestALocalAddressIsNotUpgradedToHTTPS(t *testing.T) {
	local := []string{
		"localhost", "localhost/phpmyadmin", "localhost:8080", "myapp.localhost",
		"127.0.0.1", "127.0.0.1/x", "127.0.0.1:8080", "192.168.1.5:3000", "[::1]",
	}
	for _, in := range local {
		got := resolveAddress(in)
		if !strings.HasPrefix(got.URL, "http://") {
			t.Errorf("resolveAddress(%q).URL = %q, want http:// — nothing can hold a certificate for a local host", in, got.URL)
		}
		if !strings.HasPrefix(got.Fallback, "https://") {
			t.Errorf("resolveAddress(%q).Fallback = %q, want the https twin to try if http is not what is listening", in, got.Fallback)
		}
	}

	// A public name is still upgraded, and still has somewhere to fall back to:
	// plain-http sites on port 80 have not stopped existing.
	got := resolveAddress("example.com/a")
	if got.URL != "https://example.com/a" || got.Fallback != "http://example.com/a" {
		t.Errorf("resolveAddress(example.com/a) = %+v, want https first and http to fall back on", got)
	}
}

// A scheme the user typed is a decision, not a guess, and it is not ours to
// retry under another one. Retrying an explicit https:// over http:// would be
// a silent downgrade — the browser deciding, on its own, that the user did not
// mean the secure one.
func TestAnExplicitSchemeIsNeverGivenAFallback(t *testing.T) {
	for _, in := range []string{"https://example.com", "http://example.com", "about:blank", `C:\site\index.html`, "./notes.html"} {
		if got := resolveAddress(in); got.Fallback != "" {
			t.Errorf("resolveAddress(%q).Fallback = %q, want none — the scheme was not ours to choose", in, got.Fallback)
		}
	}
}

// An IPv4 literal is four parts. Two numbers with a dot between them are
// arithmetic, and the address bar has searched for those since the day it
// learned to tell a place from a question.
func TestAnAddressNeedsFourPartsToBeAnIPAddress(t *testing.T) {
	if got := resolveAddress("3.14"); got.Query != "3.14" {
		t.Errorf("resolveAddress(3.14) = %+v, want a search", got)
	}
	if got := resolveAddress("999.1.1.1"); got.Query != "999.1.1.1" {
		t.Errorf("resolveAddress(999.1.1.1) = %+v, want a search — 999 is not an octet", got)
	}
	if got := resolveAddress("10.0.0.1"); got.URL != "http://10.0.0.1" {
		t.Errorf("resolveAddress(10.0.0.1) = %+v, want the machine at that address", got)
	}
}
