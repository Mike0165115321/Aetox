package subagent

// What a worker's own opening has to survive.
//
// The file is hand-edited markdown in somebody's agent folder, so most of what
// is pinned here is the parser refusing to be brittle: a typo costs its own
// line, prose is not a card, and a heading is a question rather than a bullet.
// The rest is the resolution rule — home first, then language — which is the
// part that decides whose voice a user hears when they have taken a shipped
// worker over.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func TestParseStartersReadsHeadingAndCards(t *testing.T) {
	set := parseStarters(`# จะให้ทำเอกสารอะไรดี?

- ร่างรายงาน | ช่วยร่างเอกสารเรื่องนี้: | pencil
- ตรวจเอกสาร | ช่วยตรวจเอกสารนี้ว่าขาดอะไรบ้าง
`)

	if set.Headline != "จะให้ทำเอกสารอะไรดี?" {
		t.Fatalf("headline = %q", set.Headline)
	}
	if len(set.Cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(set.Cards))
	}
	if set.Cards[0].Title != "ร่างรายงาน" || set.Cards[0].Icon != "pencil" {
		t.Errorf("first card = %+v", set.Cards[0])
	}
	// The half-sentence: a prompt ending in a colon is the author saying "the
	// user types the rest", and the space after it cannot survive being written
	// in a file, so the parser owes it back.
	if set.Cards[0].Prompt != "ช่วยร่างเอกสารเรื่องนี้: " {
		t.Errorf("prompt = %q — trailing space not restored", set.Cards[0].Prompt)
	}
	// A finished sentence is left exactly as written.
	if set.Cards[1].Prompt != "ช่วยตรวจเอกสารนี้ว่าขาดอะไรบ้าง" {
		t.Errorf("prompt = %q", set.Cards[1].Prompt)
	}
	if set.Cards[1].Icon != "" {
		t.Errorf("icon = %q, want empty — the window picks one", set.Cards[1].Icon)
	}
}

// A line the parser cannot read must cost that line and nothing else. The file
// is edited by hand; refusing the whole opening over one bad bullet is how a
// worker goes silent because of a typo in its fourth card.
func TestParseStartersSkipsWhatItCannotRead(t *testing.T) {
	set := parseStarters(`# หัวข้อ

<!-- คอมเมนต์ -->
บรรทัดนี้เป็นคำอธิบายเฉย ๆ ไม่มีตัวคั่น
- | ไม่มีหัวข้อ
- ไม่มีประโยค |
- ใบที่ใช้ได้ | ประโยคของมัน
`)

	if len(set.Cards) != 1 {
		t.Fatalf("cards = %+v, want only the one readable line", set.Cards)
	}
	if set.Cards[0].Title != "ใบที่ใช้ได้" {
		t.Errorf("card = %+v", set.Cards[0])
	}
}

// The window owns its grid, but the file is not the app's to trust: a worker
// must not be able to hand back three hundred cards. Dropped from the end,
// which is the order the author would have chosen to lose them in.
func TestParseStartersCapsThePool(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= maxStarters+3; i++ {
		fmt.Fprintf(&b, "- %d | ก\n", i)
	}
	set := parseStarters(b.String())
	if len(set.Cards) != maxStarters {
		t.Fatalf("cards = %d, want %d", len(set.Cards), maxStarters)
	}
	if set.Cards[maxStarters-1].Title != fmt.Sprint(maxStarters) {
		t.Errorf("dropped from the wrong end: last card = %+v", set.Cards[maxStarters-1])
	}
}

// A pool is worth having only if it can hold more than the window draws — the
// cap and the hand were the same number once, and that is exactly what stopped
// a worker from ever having a fifth card.
func TestPoolCeilingIsBiggerThanAHand(t *testing.T) {
	const windowHand = 4 // STARTER_SLOTS in starters.ts
	if maxStarters <= windowHand {
		t.Fatalf("maxStarters = %d, which is no deeper than the %d the window draws", maxStarters, windowHand)
	}
}

// The settings form has to stop where the reader stops.
//
// The form and this package answer the same question — how many cards an agent
// may hold — from two files in two languages, and the frontend cannot import a
// Go constant. Left to discipline, the day they disagree is the day a user
// fills in a card, presses save, and watches it not come back: parseStarters
// drops the overflow silently, because a file the app did not write is not
// something to fail an app launch over. So the disagreement is caught here
// instead, where it costs a red test rather than a card nobody can explain.
func TestSettingsFormStopsWhereTheReaderStops(t *testing.T) {
	const form = "../../desktop/frontend/src/lib/Settings.svelte"
	raw, err := os.ReadFile(form)
	if err != nil {
		t.Fatalf("cannot read the opening editor: %v", err)
	}
	want := fmt.Sprintf("const STARTER_MAX = %d", maxStarters)
	if !strings.Contains(string(raw), want) {
		t.Fatalf("%s does not say %q — the form and the reader disagree on how many cards an agent may hold, "+
			"and the extras will be dropped on read without telling anyone", form, want)
	}
}

// Every shipped worker opens with something, in both languages the app ships
// in. A worker is not required to have an opening — but one that has a Thai
// file and no English one would fall back to Thai in front of an English user,
// silently, and these six are ours to keep straight. (`research` was missing
// from this list while it shipped both files, so nothing was checking them.)
func TestBundledAgentsShipStartersInBothLanguages(t *testing.T) {
	// What a pool owes is enough to fill the grid, not an exact number — the
	// window deals four out of it (STARTER_SLOTS in starters.ts) and a worker
	// holding three would deal a widow onto the second row.
	const windowHand = 4

	for _, name := range []string{"automation", "doc", "github", "research", "sheet"} {
		thCards := 0
		for _, locale := range []string{"th", "en"} {
			raw, ok := bundledStarters(name)(locale)
			if !ok {
				t.Errorf("%s: no %s", name, config.AgentStartersName(locale))
				continue
			}
			set := parseStarters(string(raw))
			if set.Headline == "" {
				t.Errorf("%s (%s): no question above the cards", name, locale)
			}
			if len(set.Cards) < windowHand {
				t.Errorf("%s (%s): %d cards, too few to fill a hand of %d", name, locale, len(set.Cards), windowHand)
			}
			// A translation that dropped a card would deal a different pool to
			// an English user than to a Thai one.
			if locale == "th" {
				thCards = len(set.Cards)
			} else if len(set.Cards) != thCards {
				t.Errorf("%s: th holds %d cards, en holds %d", name, thCards, len(set.Cards))
			}
		}
	}
}

// A worker nobody wrote an opening for is the normal case, not a failure.
func TestStartersEmptyForAWorkerWithNoFile(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	set := Starters("explore", "th")
	if len(set.Cards) != 0 || set.Headline != "" {
		t.Fatalf("got %+v, want nothing", set)
	}
	// And nil is never handed to the window — an absent opening is an empty
	// list, not a null the page has to guard.
	if set.Cards == nil {
		t.Error("Cards is nil; JSON would say null")
	}
}

// The rule that decides whose voice you hear. A user who took a shipped worker
// over gets THEIR file in every language: falling back to the bundled English
// of a definition they replaced would answer as an agent that no longer exists.
func TestStartersHomeWinsBeforeLanguage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	home := filepath.Join(root, "agents", "doc")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "# คำถามของผม\n\n- ของผม | ประโยคของผม\n"
	if err := os.WriteFile(filepath.Join(home, config.AgentStartersFile), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	// English asked for, and the bundled doc agent HAS an English file — but
	// the user's home answered first, so the untranslated user file wins.
	set := Starters("doc", "en")
	if set.Headline != "คำถามของผม" {
		t.Fatalf("headline = %q — the bundled file answered over the user's", set.Headline)
	}

	// A worker the user never touched still comes from the package, translated.
	// Pinned as "the two languages answered differently" rather than as one
	// exact sentence: the wording of a shipped card is editorial and changes,
	// and a test that fails on a reworded headline is a test that punishes
	// improving the product.
	th, en := Starters("sheet", "th"), Starters("sheet", "en")
	if th.Headline == "" || en.Headline == "" {
		t.Fatalf("sheet lost a headline: th=%q en=%q", th.Headline, en.Headline)
	}
	if th.Headline == en.Headline {
		t.Fatalf("sheet answered %q in both languages — the English file did not win", en.Headline)
	}
}

// A locale is turned into a filename, so it is a trust boundary rather than a
// formatting rule — same reason AgentHome guards the name.
func TestStartersLocaleCannotEscapeTheHome(t *testing.T) {
	for _, locale := range []string{"../../../etc", "th/../..", `..\..`, "en.md"} {
		if got := config.AgentStartersName(locale); got != config.AgentStartersFile {
			t.Errorf("locale %q → %q, want the base file", locale, got)
		}
	}
}

// What the settings page writes has to be what the chat window reads back —
// the form is a second door onto the same file, not a second format.
func TestSaveStartersRoundTrips(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	want := StarterSet{
		Headline: "จะให้ปิดบัญชีอะไรดี?",
		Cards: []Starter{
			{Title: "กระทบยอดธนาคาร", Prompt: "ช่วยกระทบยอดเดือนนี้:", Icon: "chartColumn"},
			{Title: "ตรวจใบกำกับ", Prompt: "ตรวจใบกำกับภาษีชุดนี้ให้หน่อย"},
		},
	}
	if err := SaveStarters("bookkeeper", "th", want); err != nil {
		t.Fatal(err)
	}

	got := Starters("bookkeeper", "th")
	if got.Headline != want.Headline {
		t.Errorf("headline = %q, want %q", got.Headline, want.Headline)
	}
	if len(got.Cards) != 2 {
		t.Fatalf("got %d cards, want 2", len(got.Cards))
	}
	if got.Cards[0].Icon != "chartColumn" {
		t.Errorf("icon = %q", got.Cards[0].Icon)
	}
	// The half-sentence rule survives the trip: the file cannot hold the
	// trailing space, and inviteCompletion puts it back on read.
	if got.Cards[0].Prompt != "ช่วยกระทบยอดเดือนนี้: " {
		t.Errorf("prompt = %q, want the trailing space back", got.Cards[0].Prompt)
	}
	if got.Cards[1].Icon != "" {
		t.Errorf("icon = %q, want none written", got.Cards[1].Icon)
	}
}

// It writes a file somebody would have been happy to write by hand — the whole
// reason the format is markdown rather than a format with a spec.
func TestSaveStartersWritesPlainMarkdown(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)

	if err := SaveStarters("bookkeeper", "th", StarterSet{
		Headline: "ถามอะไรดี?",
		Cards:    []Starter{{Title: "ก", Prompt: "ข"}},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "agents", "bookkeeper", config.AgentStartersFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "# ถามอะไรดี?\n\n- ก | ข\n" {
		t.Errorf("wrote %q", raw)
	}
}

// Clearing the form is "I do not want my own opening", which is an absent file
// — not a file that is present and empty. With the file gone the shipped
// opening answers again, which is the thing being asked for.
func TestSaveStartersEmptyRemovesTheFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	path := filepath.Join(root, "agents", "doc", config.AgentStartersFile)

	if err := SaveStarters("doc", "th", StarterSet{Headline: "ของผม", Cards: []Starter{{Title: "ก", Prompt: "ข"}}}); err != nil {
		t.Fatal(err)
	}
	if Starters("doc", "th").Headline != "ของผม" {
		t.Fatal("the user's file did not take over")
	}

	if err := SaveStarters("doc", "th", StarterSet{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	// And the bundled opening is heard again rather than nothing.
	if Starters("doc", "th").Headline == "" {
		t.Error("no opening at all; the shipped one should answer")
	}
	// Removing what is already gone is the state being asked for, not an error.
	if err := SaveStarters("doc", "th", StarterSet{}); err != nil {
		t.Errorf("second clear: %v", err)
	}
}

// `|` is the field separator, so one inside a card would come back as a
// different card than the one saved — the icon field holding half a sentence.
// Refused by name, because that is something the user can act on.
func TestSaveStartersRefusesTheSeparator(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	err := SaveStarters("bookkeeper", "th", StarterSet{
		Cards: []Starter{{Title: "กำไร | ขาดทุน", Prompt: "ดูให้หน่อย"}},
	})
	if err == nil {
		t.Fatal("saved a card that cannot be read back")
	}
	if !strings.Contains(err.Error(), "กำไร") {
		t.Errorf("error does not name the card: %v", err)
	}
}

// Writing and reading agree on the ceiling. A form that let one more card
// through than the reader will return would save a card the agent never opens
// with, and the user would have no way of knowing which one vanished.
func TestSaveStartersCapsAtThePoolCeiling(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)

	var cards []Starter
	for i := 1; i <= maxStarters+3; i++ {
		cards = append(cards, Starter{Title: fmt.Sprint(i), Prompt: fmt.Sprintf("p%d", i)})
	}
	if err := SaveStarters("bookkeeper", "th", StarterSet{Cards: cards}); err != nil {
		t.Fatal(err)
	}
	if got := Starters("bookkeeper", "th"); len(got.Cards) != maxStarters {
		t.Fatalf("got %d cards, want %d", len(got.Cards), maxStarters)
	}
}
