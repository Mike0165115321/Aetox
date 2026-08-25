package skill

import (
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/deck"
)

// The skeleton in the slides skill has to actually be a deck.
//
// It is the block a model copies verbatim before it changes a word, so it is
// the one piece of that document where "mentions the right things" is not
// enough — a broken skeleton is not advice that ages badly, it is a deck that
// opens as source code, and every deck built from it that day is wrong the same
// way.
//
// Checked by `internal/deck`, which is the parser the exporter itself runs
// (deck.Count is what desktop/decks.go counts slides with). So this asks the
// real question — "would the room and the exporter see slides here?" — rather
// than a regex that agrees with whatever the document happens to say.
//
// A test-only import. internal/skill does not depend on internal/deck in the
// build, and internal/deck does not depend on this package, so nothing here
// changes the production graph.
func TestTheSkeletonInTheSlidesSkillIsAReadableDeck(t *testing.T) {
	var body string
	for _, b := range bundledSkills() {
		if b.Name == "aetox-slides" {
			body = b.body
		}
	}
	if body == "" {
		t.Fatal("aetox-slides is not bundled")
	}

	skeleton := longestHTMLBlock(body)
	if skeleton == "" {
		t.Fatal("the skill no longer carries an html code block — the skeleton is the part that gets copied")
	}

	// More than one, because one slide proves nothing about paging — and the
	// exact number is the skeleton's business, not this test's.
	if got := deck.Count([]byte(skeleton)); got < 2 {
		t.Errorf("the exporter reads %d slides in the skeleton, want at least 2", got)
	}

	// The desk's own routing rule, in the shape the frontend applies it
	// (isDeck in stores/workbench.svelte.ts): without the marker on a section
	// or a div, this file opens in the editor as source and never reaches the
	// slides room at all.
	if !deck.Is([]byte(skeleton)) {
		t.Error("the skeleton would not be routed to the slides room")
	}

	// The contract the skill spends its first section on, checked against the
	// skeleton rather than against the prose: a fixed 1280x720 page, and no
	// navigation of its own.
	for _, want := range []string{"width:1280px", "height:720px"} {
		if !strings.Contains(strings.ReplaceAll(skeleton, " ", ""), want) {
			t.Errorf("the skeleton no longer fixes the slide box: missing %q", want)
		}
	}
	for _, banned := range []string{"<script", "addEventListener", "IntersectionObserver", "100vh"} {
		if strings.Contains(skeleton, banned) {
			t.Errorf("the skeleton contains %q — the room does the moving, and a fixed slide is not sized in viewport units", banned)
		}
	}
}

// longestHTMLBlock returns the body of the largest ```html fence in the
// document. Largest rather than first, because the sections above the skeleton
// quote fragments of CSS and HTML to explain a single technique, and those are
// deliberately not whole documents.
func longestHTMLBlock(body string) string {
	const fence = "```html"
	best := ""
	rest := body
	for {
		start := strings.Index(rest, fence)
		if start < 0 {
			return best
		}
		rest = rest[start+len(fence):]
		end := strings.Index(rest, "```")
		if end < 0 {
			return best
		}
		if block := rest[:end]; len(block) > len(best) {
			best = block
		}
		rest = rest[end:]
	}
}
