package main

import "testing"

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
	ch := make(chan string, 1)
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
	ch := make(chan string, 1)
	tab := &browserTab{textCh: ch, textToken: "real-token"}

	h.onMessage("tab1", tab, `{"__aetox":"text","token":"real-token","url":"https://example.com/","text":"real content"}`, "https://example.com/")

	select {
	case got := <-ch:
		if got != "real content" {
			t.Errorf("channel received %q, want %q", got, "real content")
		}
	default:
		t.Fatal("channel received nothing, want the matching-token message delivered")
	}
}
