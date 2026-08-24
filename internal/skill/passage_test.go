package skill

// BM25 over one page (§178).
//
// What is worth pinning is not that the arithmetic is BM25 — that is a formula
// with a paper behind it. It is the four decisions around it that would each
// fail silently: Thai has to match at all, the answer has to come back in
// reading order, a miss has to say it missed, and the whole thing has to be
// fast enough that nobody notices it ran.

import (
	"strings"
	"testing"
	"time"
)

// Thai has no spaces. A whitespace tokenizer turns a Thai paragraph into one
// token that matches nothing, and the failure is silent: the page comes back
// looking like it simply had no answer. Trigrams are what desktop/db.go's FTS
// tables already use, for this reason.
func TestThaiMatchesAtAll(t *testing.T) {
	page := strings.Repeat("This page is mostly English filler about nothing.\n\n", 40) +
		"\n\nวิธีปิดการลองใหม่อัตโนมัติ ให้ตั้งค่า MaxAttempts เป็นศูนย์\n\n" +
		strings.Repeat("More English filler that does not answer anything.\n\n", 40)

	hits := selectPassages(splitPassages(page), "ปิดการลองใหม่", webFetchWindow)
	if len(hits) == 0 {
		t.Fatal("a Thai query found nothing on a page that answers it in Thai")
	}
	found := false
	for _, h := range hits {
		if strings.Contains(h.text, "MaxAttempts") {
			found = true
		}
	}
	if !found {
		t.Error("the Thai query did not surface the passage that answers it")
	}
}

func TestEnglishRanksTheAnsweringPassageFirst(t *testing.T) {
	page := strings.Repeat("Filler about unrelated subjects and general prose.\n\n", 30) +
		"\n\nThe retry option is called MaxAttempts and defaults to three.\n\n" +
		strings.Repeat("More filler about unrelated subjects and general prose.\n\n", 30)

	ps := splitPassages(page)
	scores := scorePassages(ps, "what is the retry option called")
	best, bestAt := 0.0, -1
	for i, sc := range scores {
		if sc > best {
			best, bestAt = sc, i
		}
	}
	if bestAt < 0 {
		t.Fatal("nothing scored above zero")
	}
	if !strings.Contains(ps[bestAt].text, "MaxAttempts") {
		t.Errorf("the top passage is not the one with the answer:\n%s", ps[bestAt].text)
	}
}

// Reading order, not score order. Ranked output hands the model paragraph 40
// before paragraph 3 and leaves it to reassemble the argument; document order
// gives it the page with the irrelevant parts removed, which is what it asked
// for.
func TestHitsComeBackInPageOrder(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 30; i++ {
		if i == 2 || i == 20 {
			b.WriteString("the retry option MaxAttempts is described here\n\n")
			continue
		}
		b.WriteString(strings.Repeat("filler sentence with nothing useful in it. ", 20) + "\n\n")
	}
	hits := selectPassages(splitPassages(b.String()), "retry MaxAttempts", webFetchWindow)
	if len(hits) < 2 {
		t.Fatalf("want both mentions, got %d", len(hits))
	}
	for i := 1; i < len(hits); i++ {
		if hits[i].at <= hits[i-1].at {
			t.Errorf("hits are not in page order: %d came after %d", hits[i].at, hits[i-1].at)
		}
	}
}

// A miss must say it missed. Handing back the first window silently is a page
// presented as a hit, and the caller then reports on a page that never
// mentioned what it asked about.
func TestAMissSaysSoInsteadOfPretending(t *testing.T) {
	page := strings.Repeat("This page is entirely about gardening in the spring. ", 300)
	shown, note := readFor(page, "kubernetes ingress controller", 0)
	if shown == "" {
		t.Fatal("a miss returned nothing at all; the page was still worth having")
	}
	if !strings.Contains(note, "nothing on this page mentions") {
		t.Errorf("a miss did not say it missed: %q", note)
	}
	if !strings.Contains(note, "not a match") {
		t.Errorf("the note does not rule out reading it as a hit: %q", note)
	}
}

// Every part carries the offset it came from, so a caller that wants the
// paragraphs around a hit asks for them rather than re-fetching and hunting.
func TestEachPartCarriesItsOffset(t *testing.T) {
	page := strings.Repeat("filler. ", 400) + "\n\nthe answer is MaxAttempts\n\n" + strings.Repeat("filler. ", 400)
	shown, note := readFor(page, "MaxAttempts", 0)
	if !strings.Contains(shown, "[at ") {
		t.Errorf("no offset on the returned part:\n%s", shown)
	}
	if !strings.Contains(note, "from:") {
		t.Errorf("the note does not say how to get a part in full: %q", note)
	}
}

// No `find` is the old behaviour exactly: the top of the page, positionally.
func TestWithoutFindNothingChanges(t *testing.T) {
	page := strings.Repeat("a", 20000)
	withFind, _ := readFor(page, "", 0)
	direct, _ := windowOf(page, 0)
	if withFind != direct {
		t.Error("an empty find took a different path from a plain fetch")
	}
}

// The whole argument for BM25 over a model is that it is free. A page the
// summarizer took 57 seconds on has to come back in a time nobody notices, or
// this was not worth building.
func TestScoringAFullPageIsFast(t *testing.T) {
	page := strings.Repeat("the quick brown fox jumps over the lazy dog near the retry option. ", 600)
	start := time.Now()
	if hits := selectPassages(splitPassages(page), "retry option", webFetchWindow); len(hits) == 0 {
		t.Fatal("found nothing on a page that says it repeatedly")
	}
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Errorf("scoring a %d-character page took %v", len(page), took)
	}
}
