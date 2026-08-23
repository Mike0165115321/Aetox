package skill

import (
	"strings"
	"testing"
)

// The bundled slides skill has to keep saying what the code actually does.
//
// A skill is prose, and prose drifts silently. What is pinned here is only the
// handful of facts inside it that are copies of decisions living in code — a
// copy that stops matching teaches the model something false with the same
// confidence as something true.
//
//   - `<section class="slide">` is internal/deck's contract (slideTag), the
//     desk's routing rule (isDeck), and what desktop/deck_flatten.go's
//     flattenScript searches for before an export.
//   - 1280x720 is the box desktop/deck_render.go builds the export webview at.
//   - The formats are desktop/decks.go's deckFormats.
//   - An entrance that rests at opacity:0 prints at opacity:0. Not because
//     nothing runs before the export — desktop/deck_reveal.go scrolls every
//     slide into view, settles animations to their last frame and pins what it
//     finds — but because pinning keeps whatever it found, and a trigger that
//     missed leaves the resting state to be pinned.
//
// Nothing about how a deck should LOOK is pinned. That was tried and it was the
// wrong shape of test: asserting the document still says `linear-gradient(145deg`
// guards a sentence, not a behaviour, and freezes taste the model is better at
// than the instruction is (the deck the owner named as the house standard was
// written with no skill open at all — six tool calls, one `write`).
func TestTheSlidesSkillKeepsSayingWhatTheCodeDoes(t *testing.T) {
	body := slidesSkillBody(t)

	for _, fact := range []string{
		`<section class="slide">`, // the contract
		"1280", "720",             // the export's box
		".pptx", ".pdf", // what the room's export bar writes
		"CDN",                  // the export does not wait for a third-party host
		"@keyframes",           // where the hidden half of an entrance belongs
		"forwards",             // an entrance that finishes on its own
		"IntersectionObserver", // ...and one the export's walk fires for you
	} {
		if !strings.Contains(body, fact) {
			t.Errorf("the skill no longer mentions %q", fact)
		}
	}

	// Half of making a deck lives in another skill, and progressive loading
	// means a skill the model is never pointed at is a skill it never opens.
	// aetox-slides is the door every deck goes through, so it is the only place
	// that pointer can be made — the link ran the other way alone until
	// 2026-08-23, and the 25 layouts and 15 deck structures in
	// aetox-design-system/data were never read on the way to writing a deck.
	if !strings.Contains(body, "aetox-design-system") {
		t.Error("the slides skill no longer names aetox-design-system — the layout tables become unreachable from the one skill a deck always opens")
	}

	// The room's own controls exist; a deck that brings a second set is the
	// thing this document was written for.
	if !strings.Contains(body, "navigation") {
		t.Error("the skill no longer says the room's controls are already there")
	}

	// A skill that names a file it does not ship is checked for every bundled
	// skill at once, in TestNoBundledSkillNamesAFileItDoesNotShip.
}

// The house look is a reference the owner chose, so its one durable artefact —
// the palette, and the instruction to go and get pictures — is checked for
// existence and nothing more. Deleting it should be a decision; rewording it
// should not need a test run.
func TestTheSlidesSkillStillOffersTheHouseLook(t *testing.T) {
	body := slidesSkillBody(t)
	for _, want := range []string{"--accent", "pictures"} {
		if !strings.Contains(body, want) {
			t.Errorf("the house look no longer offers %q", want)
		}
	}
}

func slidesSkillBody(t *testing.T) string {
	t.Helper()
	for _, b := range bundledSkills() {
		if b.Name == "aetox-slides" {
			return b.body
		}
	}
	t.Fatal("aetox-slides is not bundled — the skill folder or its frontmatter is wrong")
	return ""
}
