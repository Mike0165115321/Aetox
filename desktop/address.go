package main

// Is this text a place, or a question?
//
// Every browser answers it, and until now Aetox answered it twice: normalizeUrl
// in workbench.svelte.ts for the address bar, and normalizeWorkbenchURL here for
// the agent. Both ended the same way — anything without a scheme got `https://`
// stamped on the front — so typing **ยูทูป** produced `https://ยูทูป`, which the
// engine punycoded to `xn--o3cit6gb` and DNS refused. Owner, 17 ส.ค.: *"อนนี้มัน
// ไม่ฉลาดพอจะค้นหรอ หรือเราแบบไม่เหมือน Google"*.
//
// It was never about intelligence. The address bar had one job where every
// browser's has two, and nothing in it had ever been asked to tell an address
// from a search.
//
// The classification lives here once. **The policy does not**, because the two
// callers genuinely differ and that difference is the design:
//
//   - The address bar searches. Someone who types a word into an address bar
//     wants to search; every browser they have ever used does this.
//   - The agent's `open` refuses and names `web_search`. The agent already has a
//     search tool, and an `open` that quietly searched would teach it that
//     `open` is a search box — it would keep reaching for the wrong tool and
//     keep getting away with it.
//
// One question, one answer; two callers, two policies. That is not the "second
// place answering the same question" this project treats as debt, and the test
// for the difference is that changing how we classify text should change both
// callers, while changing what the address bar does with a search should change
// neither the agent nor this file.

import (
	"fmt"
	"net/url"
	"strings"
)

// searchURLTemplate is the ONE place the address bar's search engine is
// decided. %s takes the query, already escaped.
//
// Google rather than a privacy-first default, on one ground that outweighs the
// rest here: this is a Thai-first product, and Thai-language queries come back
// materially better. Surfacing it as a setting is a frontend job and deliberately
// not done in the same change as the plumbing — but when it is, this constant is
// the value it replaces, and nothing else has to move.
const searchURLTemplate = "https://www.google.com/search?q=%s"

// Address is what a line of text turned out to be. Exactly one of URL and Query
// is ever set.
type Address struct {
	// URL is set when the text named a place.
	URL string `json:"url"`
	// Fallback is the same place under the other scheme, and it is set only
	// when WE chose the scheme — never when the user typed one. See guess.
	Fallback string `json:"fallback"`
	// Query is set when it did not.
	Query string `json:"query"`
	// SearchURL is where Query would be searched for, filled only alongside it.
	// The agent ignores this field; it exists so the choice of engine stays in
	// one Go constant instead of being rebuilt in TypeScript.
	SearchURL string `json:"searchUrl"`
}

// guess names a place under the scheme it is most likely to be served on, and
// keeps the other one as what to try if the first fails.
//
// A guess is what it is called because that is what it is: text with no scheme
// does not say http or https, and every browser has to decide. Chrome and Edge
// decide the same way — upgrade a typed address to https, then fall back to
// http when the upgrade cannot connect — and the fallback is the half that
// makes the guess safe to make at all. Without it, one wrong guess is a dead
// page and the user has no way to know which half we got wrong.
//
// The fallback is only ever attached to a scheme WE supplied. Text that already
// said https:// gets no http twin, because silently retrying someone's explicit
// https over http is a downgrade, and a browser that does that is worse than
// one that shows the error.
func guess(s, first, other string) Address {
	return Address{URL: first + "://" + s, Fallback: other + "://" + s}
}

// ResolveAddress is the Wails binding: the address bar asks this on Enter.
func (a *App) ResolveAddress(input string) Address { return resolveAddress(input) }

// resolveAddress decides between a place and a question.
//
// The rules are the boring part and every browser agrees on them. What matters
// is which way the doubt falls, and it falls toward *search*: a mistaken search
// shows the user a results page they can act on, while a mistaken navigation
// shows them ERR_NAME_NOT_RESOLVED and a punycode hostname, which is where this
// whole thing started.
func resolveAddress(input string) Address {
	s := strings.TrimSpace(input)
	if s == "" {
		return Address{}
	}

	switch {
	case driveLetterRe.MatchString(s): // E:\site\index.html
		return Address{URL: "file:///" + strings.ReplaceAll(s, `\`, "/")}
	case urlSchemeRe.MatchString(s), bareSchemeRe.MatchString(s):
		return Address{URL: s}
	case strings.HasPrefix(s, "/"), strings.HasPrefix(s, "./"), strings.HasPrefix(s, "../"):
		return Address{URL: s} // a path; the caller resolves it against its own root
	}

	// A space is the strongest signal there is, and it comes before every other
	// rule: "example.com is down" is a sentence about a domain, not a domain.
	if strings.ContainsAny(s, " \t") {
		return searchFor(s)
	}

	host, hasPort := hostOf(s)

	// This machine, and the machines on this desk. Chrome and Edge both cut
	// loopback and IP literals out of the https upgrade, and the reason is not
	// politeness about local development: there is no certificate authority
	// that can vouch for `localhost`, so https there is either a self-signed
	// warning page or nothing listening at all.
	//
	// This is the case the whole file was rewritten for. Someone starts XAMPP,
	// types `localhost`, and Apache is serving plain http on port 80 — the one
	// combination the old rule turned into `https://localhost` and a dead pane.
	// Aetox was the only browser on their machine that could not open their own
	// server.
	if isLocal(host) {
		return guess(s, "http", "https")
	}
	// A dot with something on both sides of the last one. "example.com" is a
	// place; "ยูทูป" is not; "3.14" is not, because a bare number is arithmetic
	// far more often than it is a hostname — and it is not an address either,
	// since an IPv4 literal has four parts and that has two.
	if dot := strings.LastIndexByte(host, '.'); dot > 0 && dot < len(host)-1 && !isAllDigits(strings.ReplaceAll(host, ".", "")) {
		return guess(s, "https", "http")
	}
	// A port is what makes a one-word host an address: `dev:8080` is a machine,
	// `dev` is a search. Http first, because a single-label name with a port is
	// something on this network far more often than it is a public site.
	if hasPort {
		return guess(s, "http", "https")
	}
	return searchFor(s)
}

// hostOf strips a scheme-less address down to its host, and says whether an
// explicit port came off with the rest.
func hostOf(s string) (host string, hasPort bool) {
	host = s
	if slash := strings.IndexByte(host, '/'); slash >= 0 {
		host = host[:slash] // example.com/a/b — judge the host, keep the path
	}
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		host = host[at+1:] // user:pass@host
	}
	// An IPv6 literal is bracketed precisely so its own colons cannot be read
	// as a port, so the search for one starts after the bracket.
	from := 0
	if b := strings.LastIndexByte(host, ']'); b >= 0 {
		from = b + 1
	}
	if colon := strings.LastIndexByte(host[from:], ':'); colon >= 0 {
		if port := host[from+colon+1:]; port != "" && isAllDigits(port) {
			return host[:from+colon], true
		}
	}
	return host, false
}

// isLocal reports whether a host can only mean this machine or the network it
// is sitting on — the hosts no public certificate can be issued for.
func isLocal(host string) bool {
	h := strings.ToLower(host)
	switch {
	case h == "localhost", strings.HasSuffix(h, ".localhost"): // RFC 6761
		return true
	case h == "[::1]", h == "::1":
		return true
	}
	return isIPv4(h)
}

// isIPv4 reports whether host is four dotted decimal parts, each 0-255.
//
// Four parts and not "some digits and dots": Windows and every resolver will
// happily read `127.1` as an address, but a person typing two numbers into an
// address bar means arithmetic, and "3.14 is a search" is a rule this file
// already had and is not about to lose.
func isIPv4(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 3 || !isAllDigits(p) {
			return false
		}
		n := 0
		for _, r := range p {
			n = n*10 + int(r-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}

func searchFor(q string) Address {
	return Address{Query: q, SearchURL: fmt.Sprintf(searchURLTemplate, url.QueryEscape(q))}
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
