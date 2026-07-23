package main

import (
	"encoding/json"
	"strings"
	"testing"
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

func TestClickScriptEmbedsRef(t *testing.T) {
	js := clickScript(42)
	if !strings.Contains(js, `[data-aetox-ref="42"]`) {
		t.Errorf("clickScript(42) = %q, want it to target [data-aetox-ref=\"42\"]", js)
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
		js := typeScript(7, text)
		wantEncoded, err := json.Marshal(text)
		if err != nil {
			t.Fatalf("json.Marshal(%q): %v", text, err)
		}
		if !strings.Contains(js, string(wantEncoded)) {
			t.Errorf("typeScript(7, %q) does not contain the expected JSON-escaped literal %s\ngot: %s", text, wantEncoded, js)
		}
		if !strings.Contains(js, `[data-aetox-ref="7"]`) {
			t.Errorf("typeScript(7, %q) = %q, want it to target [data-aetox-ref=\"7\"]", text, js)
		}
	}
}
