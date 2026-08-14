package main

// The ผลงาน gallery's range and its ordering (COMPANY.md §2).
//
// TestListArtifactsFindsWhatASessionProduced in desk_test.go covers the happy
// path — a file written, a file found. What this file is about is the two
// things that only show up once there are a lot of files: which end of the
// history you get, and how much of it.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bootGalleryApp is bootDeskApp with the home directory isolated too.
//
// artifactRoots() starts at unfocusedRoot(), which is <home>/aetox — read from
// USERPROFILE/HOME rather than from the data root the test harness already
// redirects. Left alone, every assertion here counts the developer's own
// artifacts as well as the ones it wrote, so these tests pass or fail by what
// happens to be in a folder they never touched.
func bootGalleryApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home) // Windows
	t.Setenv("HOME", home)        // everywhere else
	return bootDeskApp(t, "assistant")
}

// writeArtifact puts one file in a session folder and stamps its mtime, which
// is what the gallery sorts and ranges by.
func writeArtifact(t *testing.T, a *App, session, name string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(a.cfg.SandboxRoot, "output", session)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	return path
}

// The bug this range was built to fix, pinned so it cannot come back.
//
// maxArtifacts used to be applied on the way in, three times, all of them
// before the sort. Session folders are named for their timestamp and os.ReadDir
// returns names in order, so the sweep ran oldest-first and stopped at the cap:
// past 500 files the gallery showed the 500 *oldest* artifacts under a heading
// promising the newest, and today's work was not on the page at all.
func TestTheGalleryShowsTheNewestFilesNotTheFirstItWalked(t *testing.T) {
	a := bootGalleryApp(t)

	// Comfortably past maxArtifacts, oldest sessions written first so the
	// directory order is the wrong order.
	const old = 520
	for i := 0; i < old; i++ {
		writeArtifact(t, a, fmt.Sprintf("20200101-%06d.000", i), "old.txt", 300*24*time.Hour)
	}
	today := writeArtifact(t, a, "20991231-000000.000", "today.txt", time.Hour)

	page := a.ListArtifactsIn(RangeAll)
	if len(page.Files) == 0 {
		t.Fatal("the gallery found nothing at all")
	}
	if page.Files[0].Path != today {
		t.Errorf("newest first is the whole promise: got %q, want %q", page.Files[0].Path, today)
	}
	// Nothing is dropped. The old 500 would have cut this to 500; the bound
	// that replaced it is a safety valve, not a page size, and what keeps the
	// first paint cheap lives in the window instead.
	if page.Total != old+1 || len(page.Files) != old+1 {
		t.Errorf("every file in range must come back: %d files, Total %d, want %d",
			len(page.Files), page.Total, old+1)
	}
}

// The range is what bounds the page now. A week means a week.
func TestARangeShowsOnlyWhatFallsInsideIt(t *testing.T) {
	a := bootGalleryApp(t)
	recent := writeArtifact(t, a, "s-recent", "recent.txt", 2*24*time.Hour)
	writeArtifact(t, a, "s-mid", "mid.txt", 14*24*time.Hour)
	writeArtifact(t, a, "s-old", "old.txt", 200*24*time.Hour)

	week := a.ListArtifactsIn(RangeWeek)
	if week.Range != RangeWeek {
		t.Fatalf("a week with files in it must serve the week, got %q", week.Range)
	}
	if len(week.Files) != 1 || week.Files[0].Path != recent {
		t.Errorf("the week should hold exactly the recent file, got %d: %+v", len(week.Files), week.Files)
	}
	if got := len(a.ListArtifactsIn(RangeMonth).Files); got != 2 {
		t.Errorf("the month should hold the recent and the mid file, got %d", got)
	}
	if got := len(a.ListArtifactsIn(RangeAll).Files); got != 3 {
		t.Errorf("everything means everything, got %d", got)
	}
}

// Opening the page on a quiet Monday and being shown nothing is
// indistinguishable from the feature being broken.
func TestAnEmptyWeekWidensAndSaysSo(t *testing.T) {
	a := bootGalleryApp(t)
	writeArtifact(t, a, "s-old", "old.txt", 20*24*time.Hour)

	page := a.ListArtifactsIn(RangeWeek)
	if len(page.Files) != 1 {
		t.Fatalf("an empty week must fall through to a range that has something, got %d", len(page.Files))
	}
	if page.Range != RangeMonth {
		t.Errorf("the served range must say which one answered, got %q, want %q", page.Range, RangeMonth)
	}
}

// Nothing anywhere is its own answer, and it is not an error.
func TestNoArtifactsAtAllIsAnEmptyPageNotAWiderOne(t *testing.T) {
	a := bootGalleryApp(t)
	page := a.ListArtifactsIn(RangeWeek)
	if len(page.Files) != 0 {
		t.Errorf("expected nothing, got %d", len(page.Files))
	}
	if page.Range != RangeAll {
		t.Errorf("having widened all the way and still found nothing, the range is all, got %q", page.Range)
	}
}

// A file whose mtime will not parse is kept. It is a real file, and hiding it
// because its clock is odd is the worse failure of the two.
func TestAFileWithAnUnreadableTimeIsStillShown(t *testing.T) {
	a := bootGalleryApp(t)
	writeArtifact(t, a, "s-recent", "recent.txt", time.Hour)
	all := a.ListArtifactsIn(RangeAll)
	if len(all.Files) != 1 {
		t.Fatalf("setup: expected one file, got %d", len(all.Files))
	}
	// within() is the half that decides, so it is asked directly with a value
	// os.Stat could never produce but a future format change might.
	broken := []Artifact{{Path: "x", Modified: "not-a-time"}}
	if got := within(broken, RangeWeek); len(got) != 1 {
		t.Error("a file with an unparseable timestamp must be kept, not filtered away")
	}
}

// The old binding keeps working: it is the whole gallery, as it always was.
func TestListArtifactsStillMeansEverything(t *testing.T) {
	a := bootGalleryApp(t)
	writeArtifact(t, a, "s-old", "old.txt", 200*24*time.Hour)
	if got := len(a.ListArtifacts()); got != 1 {
		t.Errorf("ListArtifacts must still return every artifact, got %d", got)
	}
}

// A page bigger than the text-excerpt budget still renders as a page.
//
// The two budgets used to be one constant, so an .html file crossed from
// "rendered" to "shown as source" at 24 KB — which is a size, not a reason, and
// nothing on the card said so. Reported with two landing pages side by side:
// 22 KB drawn as a page, 49 KB drawn as a wall of markup.
func TestALargeHTMLPageStillPreviewsAsAPage(t *testing.T) {
	a := bootGalleryApp(t)
	dir := filepath.Join(a.cfg.SandboxRoot, "output", "s1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Comfortably over previewTextBytes and comfortably under previewHTMLBytes,
	// with a marker at the very end so a truncated read cannot pass.
	body := strings.Repeat("<p>filler</p>\n", 4000)
	page := "<!DOCTYPE html><html><body>" + body + "<i id=\"tail\">END</i></body></html>"
	if len(page) <= previewTextBytes {
		t.Fatalf("test is stale: %d bytes no longer exceeds the excerpt budget", len(page))
	}
	path := filepath.Join(dir, "big.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := a.ArtifactPreview(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "html" {
		t.Fatalf("a %d-byte page came back as %q, not html", len(page), got.Kind)
	}
	if !strings.Contains(got.Text, `id="tail"`) {
		t.Error("the page was truncated — a document cut in half renders as a broken one")
	}
}

// And past the render ceiling it goes back to being quoted, not dropped.
func TestAnEnormousHTMLPageFallsBackToAnExcerpt(t *testing.T) {
	a := bootGalleryApp(t)
	dir := filepath.Join(a.cfg.SandboxRoot, "output", "s1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	huge := "<!DOCTYPE html><html><body>" + strings.Repeat("<p>x</p>\n", previewHTMLBytes/4) + "</body></html>"
	path := filepath.Join(dir, "huge.html")
	if err := os.WriteFile(path, []byte(huge), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := a.ArtifactPreview(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "text" {
		t.Errorf("past the ceiling the card quotes the source, got kind %q", got.Kind)
	}
	// previewTextRunes plus clipRunes' own ellipsis, which is how an excerpt
	// says out loud that it is one.
	if n := len([]rune(got.Text)); n > previewTextRunes+1 {
		t.Errorf("the fallback must still be an excerpt, got %d runes", n)
	}
	if !strings.HasSuffix(got.Text, "…") {
		t.Error("a clipped excerpt must show that it was clipped")
	}
}
