package main

import "testing"

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
		"localhost:8014":       "https://localhost:8014",
		"localhost":            "https://localhost",
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
