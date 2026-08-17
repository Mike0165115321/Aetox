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
	// Query is set when it did not.
	Query string `json:"query"`
	// SearchURL is where Query would be searched for, filled only alongside it.
	// The agent ignores this field; it exists so the choice of engine stays in
	// one Go constant instead of being rebuilt in TypeScript.
	SearchURL string `json:"searchUrl"`
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

	host := s
	if slash := strings.IndexByte(host, '/'); slash >= 0 {
		host = host[:slash] // example.com/a/b — judge the host, keep the path
	}
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		host = host[at+1:] // user:pass@host
	}
	if colon := strings.LastIndexByte(host, ':'); colon >= 0 {
		if port := host[colon+1:]; port != "" && isAllDigits(port) {
			return Address{URL: "https://" + s} // localhost:8014, example.com:8443
		}
	}
	if host == "localhost" {
		return Address{URL: "https://" + s}
	}
	// A dot with something on both sides of the last one. "example.com" is a
	// place; "ยูทูป" is not; "3.14" is not, because a bare number is arithmetic
	// far more often than it is a hostname.
	if dot := strings.LastIndexByte(host, '.'); dot > 0 && dot < len(host)-1 && !isAllDigits(strings.ReplaceAll(host, ".", "")) {
		return Address{URL: "https://" + s}
	}
	return searchFor(s)
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
