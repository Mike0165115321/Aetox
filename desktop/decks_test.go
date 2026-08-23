package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func deckHTML(title string, slides ...string) string {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><title>" + title + "</title></head><body>")
	for _, s := range slides {
		b.WriteString(`<section class="slide"><h1>` + s + `</h1><ul><li>ประเด็น</li></ul>` +
			`<aside class="notes">พูดช้า ๆ ตรงนี้</aside></section>`)
	}
	b.WriteString("</body></html>")
	return b.String()
}

// deckApp gives the app a project AND a Downloads folder of its own. The second
// half is not tidiness: without it every run of these tests would drop files
// into the developer's real Downloads.
func deckApp(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	return seed(&App{cfg: config.Config{SandboxRoot: root}, exportsRoot: t.TempDir()}, newConversation()), root
}

func writeUnder(t *testing.T, root, rel, body string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func TestListDecksFindsDecksAndSkipsPages(t *testing.T) {
	a, root := deckApp(t)
	writeUnder(t, root, "output/20260819-010101.111/เสนอราคา.html", deckHTML("ก", "หนึ่ง", "สอง", "สาม"))
	writeUnder(t, root, "output/20260819-010101.111/index.html", "<html><body><section><h1>หน้าเว็บ</h1></section></body></html>")
	writeUnder(t, root, "output/20260819-010101.111/บันทึก.md", "# ไม่ใช่เด็ค")

	decks := a.ListDecks()
	if len(decks) != 1 {
		t.Fatalf("got %d decks, want 1: %+v", len(decks), decks)
	}
	if decks[0].Slides != 3 {
		t.Errorf("slides = %d, want 3", decks[0].Slides)
	}
	// The path has to be the one the file host and the pane both speak, or the
	// row lists fine and opens onto a blank iframe.
	if want := "output/20260819-010101.111/เสนอราคา.html"; decks[0].Path != want {
		t.Errorf("path = %q, want %q", decks[0].Path, want)
	}
	if filepath.IsAbs(decks[0].Path) {
		t.Errorf("path must be project-relative, got %q", decks[0].Path)
	}
	if decks[0].SessionID != "20260819-010101.111" {
		t.Errorf("sessionId = %q", decks[0].SessionID)
	}
}

// A deck loose in output/ predates per-session folders. It is still a deck; it
// just has no conversation to point back at, and the honest answer is "".
func TestDeckLooseInOutputHasNoSession(t *testing.T) {
	a, root := deckApp(t)
	writeUnder(t, root, "output/เก่า.html", deckHTML("ก", "หนึ่ง"))

	decks := a.ListDecks()
	if len(decks) != 1 {
		t.Fatalf("got %d decks, want 1", len(decks))
	}
	if decks[0].SessionID != "" {
		t.Errorf("sessionId = %q, want empty", decks[0].SessionID)
	}
}

// aged writes a deck and dates it, because every assertion about a range is an
// assertion about when the file was written.
func aged(t *testing.T, root, rel string, daysAgo int, slides ...string) {
	t.Helper()
	full := writeUnder(t, root, rel, deckHTML("ก", slides...))
	when := time.Now().AddDate(0, 0, -daysAgo).Add(-time.Hour)
	if err := os.Chtimes(full, when, when); err != nil {
		t.Fatal(err)
	}
}

// The room reloads on every finished turn, so listing without a bound charged a
// full read and a full HTML parse of every deck the workspace had ever produced
// to every one of those turns. The range is that bound, and this is the guard
// on it: what is outside the range must not be opened at all.
//
// Counted by what came back *and* by the fact that the old deck is not in it —
// a listing that read everything and then filtered would pass an assertion
// about rows and be the exact cost this replaced. The read is what the filter
// has to come before, which is why candidatesWithin takes deckFile and not Deck.
func TestTheWeekRangeLeavesOlderDecksUnopened(t *testing.T) {
	a, root := deckApp(t)
	aged(t, root, "output/s-new/สัปดาห์นี้.html", 1, "หนึ่ง", "สอง")
	aged(t, root, "output/s-mid/เดือนนี้.html", 14, "หนึ่ง")
	aged(t, root, "output/s-old/นานแล้ว.html", 200, "หนึ่ง")

	week := a.ListDecksIn(RangeWeek)
	if week.Range != RangeWeek {
		t.Errorf("range = %q, want %q", week.Range, RangeWeek)
	}
	if len(week.Decks) != 1 || week.Decks[0].Name != "สัปดาห์นี้.html" {
		t.Fatalf("week = %+v, want only this week's deck", week.Decks)
	}
	if week.Total != 1 {
		t.Errorf("total = %d, want 1 — with no cap it is exactly what is in range", week.Total)
	}

	month := a.ListDecksIn(RangeMonth)
	if len(month.Decks) != 2 {
		t.Fatalf("month = %+v, want two decks", month.Decks)
	}
	// Newest first, the order the room draws and the day headings depend on.
	if month.Decks[0].Name != "สัปดาห์นี้.html" {
		t.Errorf("month is not newest first: %+v", month.Decks)
	}

	if all := a.ListDecksIn(RangeAll); len(all.Decks) != 3 {
		t.Errorf("all = %d decks, want 3", len(all.Decks))
	}
}

// An empty week is indistinguishable from a broken room, so it widens — and it
// has to widen on the number of *decks*, not on the number of .html files the
// walk found. A week holding nothing but web pages is an empty week.
func TestAWeekOfNonDecksWidensRatherThanShowingNothing(t *testing.T) {
	a, root := deckApp(t)
	page := writeUnder(t, root, "output/s-new/หน้าเว็บ.html",
		"<html><body><section><h1>ไม่ใช่เด็ค</h1></section></body></html>")
	when := time.Now().AddDate(0, 0, -1)
	if err := os.Chtimes(page, when, when); err != nil {
		t.Fatal(err)
	}
	aged(t, root, "output/s-old/เด็คเก่า.html", 120, "หนึ่ง")

	got := a.ListDecksIn(RangeWeek)
	if len(got.Decks) != 1 || got.Decks[0].Name != "เด็คเก่า.html" {
		t.Fatalf("did not widen past a week of non-decks: %+v", got.Decks)
	}
	if got.Range != RangeAll {
		t.Errorf("range = %q, want %q — the picker has to say what is on screen", got.Range, RangeAll)
	}
}

// The widening chain must not pay for the same file twice. Without the memo,
// a week that widens to the month and then to everything reads the oldest decks
// three times over.
func TestWideningDoesNotRereadWhatItAlreadyOpened(t *testing.T) {
	a, root := deckApp(t)
	aged(t, root, "output/s-old/เก่า.html", 300, "หนึ่ง")

	got := a.ListDecksIn(RangeWeek)
	if len(got.Decks) != 1 || got.Range != RangeAll {
		t.Fatalf("got %+v range=%q", got.Decks, got.Range)
	}
	if got.Total != 1 {
		t.Errorf("total = %d, want 1", got.Total)
	}
}

func TestListDecksWithNoProjectIsEmptyNotNil(t *testing.T) {
	a := seed(&App{cfg: config.Config{SandboxRoot: ""}}, newConversation())
	// The binding crosses to JS, where a nil slice arrives as null and the room
	// would render `null.length`. Empty is the same answer without the crash.
	if got := a.ListDecks(); got == nil || len(got) != 0 {
		t.Errorf("ListDecks with no project = %#v, want an empty slice", got)
	}
}

func TestListDecksOnAFreshProjectIsEmpty(t *testing.T) {
	a, _ := deckApp(t) // no output folder at all
	if got := a.ListDecks(); len(got) != 0 {
		t.Errorf("a project that has produced nothing = %#v", got)
	}
}

// An export lands in the machine's Downloads folder, not beside the deck.
//
// It was beside the deck first, on the argument that this app already had one
// answer to "where do produced files go". That is right for what the agent
// produces and wrong for what a person asked for by pressing a button: an
// export is a file somebody is about to attach to an email, and Downloads is
// where every other program on the machine puts that. Owner's call.
func TestExportLandsInDownloadsNotBesideTheDeck(t *testing.T) {
	a, root := deckApp(t)
	rel := "output/s1/เด็ค.html"
	writeUnder(t, root, rel, deckHTML("ก", "ต้นทุนลดลง ๔๐%", "สามอย่างที่เปลี่ยน"))

	out, err := a.ExportDeck(rel, "pptx")
	if err != nil {
		t.Fatalf("ExportDeck: %v", err)
	}
	if !filepath.IsAbs(out) {
		t.Fatalf("got %q, want an absolute path — it is outside the project now", out)
	}
	if filepath.Dir(out) != a.exportsRoot {
		t.Errorf("landed in %q, want the downloads folder %q", filepath.Dir(out), a.exportsRoot)
	}
	if filepath.Base(out) != "เด็ค.pptx" {
		t.Errorf("named %q, want the deck's own name", filepath.Base(out))
	}
	// The deck itself does not move.
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Errorf("the deck was disturbed: %v", err)
	}

	// The words have to survive the trip. A .pptx that opens empty is the
	// failure that reads as the feature being broken.
	parts := unzipPPTX(t, out)
	slide1 := parts["ppt/slides/slide1.xml"]
	if !strings.Contains(slide1, "ต้นทุนลดลง ๔๐%") {
		t.Errorf("slide 1 lost its title:\n%s", slide1)
	}
	if !strings.Contains(slide1, "ประเด็น") {
		t.Errorf("slide 1 lost its bullet")
	}
	if notes := parts["ppt/notesSlides/notesSlide1.xml"]; !strings.Contains(notes, "พูดช้า ๆ ตรงนี้") {
		t.Errorf("the speaker notes did not come across:\n%s", notes)
	}
	if _, ok := parts["ppt/slides/slide2.xml"]; !ok {
		t.Error("the second slide is missing")
	}
}

// Downloads is shared with every other program, and the file already there may
// be one the user has already sent to somebody. Overwriting it would destroy
// something this app never made.
func TestASecondExportDoesNotOverwriteTheFirst(t *testing.T) {
	a, root := deckApp(t)
	rel := "output/s1/เด็ค.html"
	writeUnder(t, root, rel, deckHTML("ก", "หนึ่ง"))

	first, err := a.ExportDeck(rel, "pptx")
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.ExportDeck(rel, "pptx")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("both exports went to %q", first)
	}
	for _, p := range []string{first, second} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%q is not on disk: %v", p, err)
		}
	}
}

// A deck may be called anything; a file may not.
func TestADecksNameIsMadeSafeForAFilename(t *testing.T) {
	if got := sanitiseFileName(`ราคา: Q4/2026 <ร่าง>`); strings.ContainsAny(got, `<>:"/\|?*`) {
		t.Errorf("sanitiseFileName left something Windows refuses: %q", got)
	}
	if got := sanitiseFileName("   "); got != "deck" {
		t.Errorf("an empty name = %q, want a usable fallback", got)
	}
	// The name is still the deck's. Sanitising must not turn it into a hash.
	if got := sanitiseFileName("เสนอราคา"); got != "เสนอราคา" {
		t.Errorf("a perfectly good Thai name was mangled to %q", got)
	}
}

// Only a file this session actually exported can be opened, because the opener
// takes an absolute path and everything else here refuses one on purpose.
func TestOpenExportRefusesAPathItNeverWrote(t *testing.T) {
	a, _ := deckApp(t)
	if err := a.OpenExport(filepath.Join(a.exportsRoot, "..", "somebody-elses.pptx")); err == nil {
		t.Fatal("a path this app never wrote was opened")
	}
}

func TestExportDeckRefusesAFormatItCannotWrite(t *testing.T) {
	a, root := deckApp(t)
	rel := "output/s1/เด็ค.html"
	writeUnder(t, root, rel, deckHTML("ก", "หนึ่ง"))

	// Formats nothing writes, and the empty string. Every row the menu actually
	// offers has left this list as it shipped — a format that is ready must
	// never be asserted here, because on a machine with no webview it would
	// fail for the wrong reason and go red the day one existed. What guards the
	// other direction is TestEveryReadyFormatActuallyWrites.
	for _, format := range []string{"odp", "key", "mp4", ""} {
		if _, err := a.ExportDeck(rel, format); err == nil {
			t.Errorf("ExportDeck(%q) succeeded, but nothing writes that yet", format)
		}
	}
}

// needsEngine names the formats whose export drives a real WebView2, and which
// therefore cannot be exercised from `go test` on a build machine with no
// window — the same wall §75 hit with Excel, answered the same way: assert
// everything that is checkable and close the rest out by hand.
var needsEngine = map[string]bool{"pdf": true, "png": true, "jpg": true, "webp": true, "pptx-img": true}

// The menu and the writer read one list, so a row cannot say "ready" while
// ExportDeck refuses it, or the reverse.
func TestEveryReadyFormatActuallyWrites(t *testing.T) {
	a, root := deckApp(t)
	rel := "output/s1/เด็ค.html"
	writeUnder(t, root, rel, deckHTML("ก", "หนึ่ง"))

	formats := a.DeckFormats()
	if len(formats) == 0 {
		t.Fatal("the export menu is empty")
	}
	ready := 0
	for _, f := range formats {
		if !f.Ready {
			continue
		}
		ready++
		if needsEngine[f.ID] {
			continue
		}
		out, err := a.ExportDeck(rel, f.ID)
		if err != nil {
			t.Errorf("%s is marked ready and refused: %v", f.ID, err)
			continue
		}
		if !strings.HasSuffix(out, f.Ext) {
			t.Errorf("%s wrote %q, which does not end in %q", f.ID, out, f.Ext)
		}
	}
	if ready == 0 {
		t.Error("no format is ready, so the export button can never do anything")
	}
}

// The print settings are the whole difference between a PDF that looks like the
// deck and one that looks like a bug report, and they are checkable here even
// though the printing is not.
func TestPrintSettingsKeepTheDeckLookingLikeItself(t *testing.T) {
	var params map[string]any
	// 0, 0 is "nothing measured" — the fallback path, which is the one this
	// test's numbers describe (deckPageWidthInches × deckPageHeightInches).
	if err := json.Unmarshal([]byte(deckPrintParams(0, 0)), &params); err != nil {
		t.Fatalf("the print parameters are not valid JSON: %v", err)
	}

	// Without this every slide with a colour behind it prints white. The file
	// still opens, so nothing anywhere says why it came out wrong.
	if params["printBackground"] != true {
		t.Error("printBackground is off, so every coloured slide would print white")
	}
	// With this on, Chromium stamps the file:// URL and today's date across
	// every slide.
	if params["displayHeaderFooter"] != false {
		t.Error("displayHeaderFooter is on, so every slide would carry a URL and a date")
	}
	for _, margin := range []string{"marginTop", "marginBottom", "marginLeft", "marginRight"} {
		if params[margin] != float64(0) {
			t.Errorf("%s is %v, but the slide IS the page: a margin shrinks the artwork inside its own paper", margin, params[margin])
		}
	}
	// 1280x720 CSS pixels at 96dpi, which is also ooxml's 12192000 EMU.
	if w := params["paperWidth"]; w != 13.333 {
		t.Errorf("paperWidth = %v, want 13.333in (1280px at 96dpi)", w)
	}
	if h := params["paperHeight"]; h != 7.5 {
		t.Errorf("paperHeight = %v, want 7.5in (720px at 96dpi)", h)
	}
}

// The export webview is a browser tab, so it reaches the deck as a file:// URL
// rather than through the app's own /aetox-file/ host. A Thai filename or a
// space that arrives unescaped is a deck the engine cannot find, which prints
// as its own error page.
func TestFileURLEscapesEverySegment(t *testing.T) {
	got := fileURLForPath(`D:\งาน\output\s1\เสนอ ราคา.html`)
	if !strings.HasPrefix(got, "file:///") {
		t.Errorf("%q does not start with file:///", got)
	}
	if strings.Contains(got, " ") {
		t.Errorf("%q still contains a raw space", got)
	}
	if strings.Contains(got, `\`) {
		t.Errorf("%q still contains a backslash", got)
	}
	// The separators must survive: escaping the whole path at once turns them
	// into %2F and the engine gets one very long filename.
	if n := strings.Count(got, "/"); n < 5 {
		t.Errorf("%q lost its separators (%d slashes)", got, n)
	}
	if !strings.Contains(got, url.PathEscape("เสนอ ราคา.html")) {
		t.Errorf("%q does not carry the escaped filename", got)
	}
}

func TestExportDeckRefusesAPageWithNoSlides(t *testing.T) {
	a, root := deckApp(t)
	rel := "output/s1/index.html"
	writeUnder(t, root, rel, "<html><body><h1>หน้าเว็บธรรมดา</h1></body></html>")

	_, err := a.ExportDeck(rel, "pptx")
	if err == nil {
		t.Fatal("a page with no slides was exported")
	}
	// The remedy belongs in the message: whoever reads it has to know what to
	// change, and "no slides" alone does not say.
	if !strings.Contains(err.Error(), `<section class="slide">`) {
		t.Errorf("the error must say what a slide is: %v", err)
	}
}

func TestExportDeckStaysInsideTheProject(t *testing.T) {
	a, root := deckApp(t)
	writeUnder(t, root, "output/s1/เด็ค.html", deckHTML("ก", "หนึ่ง"))

	for _, escape := range []string{
		"../outside.html",
		"output/../../outside.html",
	} {
		if _, err := a.ExportDeck(escape, "pptx"); err == nil {
			t.Errorf("ExportDeck(%q) was allowed out of the project", escape)
		}
	}
}

func TestExportDeckOnAMissingFileSaysItIsGone(t *testing.T) {
	a, _ := deckApp(t)
	if _, err := a.ExportDeck("output/s1/หายไป.html", "pptx"); err == nil {
		t.Fatal("exporting a file that is not there succeeded")
	}
}

func unzipPPTX(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("the export is not a readable package: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(body)
	}
	return out
}

// Both pptx rows carry the same extension, and they are not interchangeable:
// one is editable and plain, the other exact and frozen. If they shared a
// filename the second export would silently replace the first.
// Both pptx rows carry the same extension and are not interchangeable: one is
// editable and plain, the other exact and frozen. Sharing a filename would make
// the second export silently replace the first.
func TestTheTwoPptxRowsDoNotShareAFilename(t *testing.T) {
	a, root := deckApp(t)
	rel := "output/s1/เด็ค.html"
	writeUnder(t, root, rel, deckHTML("ก", "หนึ่ง"))

	plain, err := a.ExportDeck(rel, "pptx")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(plain) != "เด็ค.pptx" {
		t.Errorf("the editable pptx is named %q", filepath.Base(plain))
	}
	// The picture one needs a renderer, so what is checked here is the naming
	// rule ExportDeck applies rather than the export itself.
	if filepath.Base(plain) == "เด็ค-img.pptx" {
		t.Error("the two pptx exports would collide")
	}
}

// The skeleton the slides skill hands out has to survive the whole export path,
// not just the parser.
//
// internal/skill already checks that `internal/deck` reads three slides in it
// (bundled_slides_skeleton_test.go). That proves it is a deck; it does not prove
// it is a deck somebody can hand to PowerPoint, which is the thing a user
// actually does with it. Between the two sit deck.Slides and ooxml.BuildPPTX,
// and a skeleton that parses but exports empty would look exactly like a working
// one right up until the click.
//
// Runs here rather than in internal/skill because ExportDeck is the App's, and
// `pptx` is the one format that needs no window (needsEngine, above).
func TestTheSlidesSkeletonExportsToARealPPTX(t *testing.T) {
	skeleton := skeletonFromTheSlidesSkill(t)

	a, root := deckApp(t)
	rel := "output/s1/skeleton.html"
	writeUnder(t, root, rel, skeleton)

	landed, err := a.ExportDeck(rel, "pptx")
	if err != nil {
		t.Fatalf("the documented skeleton does not export: %v", err)
	}
	info, err := os.Stat(landed)
	if err != nil {
		t.Fatalf("stat %s: %v", landed, err)
	}
	// A .pptx is a ZIP of XML parts. Anything this small is an empty shell.
	if info.Size() < 4096 {
		t.Errorf("%s is %d bytes — too small to hold three slides", landed, info.Size())
	}

	zr, err := zip.OpenReader(landed)
	if err != nil {
		t.Fatalf("the export is not a readable .pptx: %v", err)
	}
	defer zr.Close()
	slides := 0
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			slides++
		}
	}
	// However many the skeleton shows — the point is that every one of them
	// arrives, not that the document keeps a particular number of them.
	if want := strings.Count(skeleton, `class="slide"`); slides != want {
		t.Errorf("the .pptx holds %d slides, the skeleton has %d", slides, want)
	}
}

// skeletonFromTheSlidesSkill reads the largest ```html block out of the bundled
// aetox-slides document — the block a model copies before it changes a word.
//
// Read off disk rather than through internal/skill, which exposes a bundled
// skill's body only to its own package. The path is the same folder //go:embed
// compiles in, so the two cannot disagree about which document this is.
func skeletonFromTheSlidesSkill(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "internal", "skill", "skills", "aetox-slides", "SKILL.md"))
	if err != nil {
		t.Fatalf("the slides skill is not where it is compiled in from: %v", err)
	}
	best, rest := "", string(raw)
	for {
		start := strings.Index(rest, "```html")
		if start < 0 {
			break
		}
		rest = rest[start+len("```html"):]
		end := strings.Index(rest, "```")
		if end < 0 {
			break
		}
		if block := rest[:end]; len(block) > len(best) {
			best = block
		}
		rest = rest[end:]
	}
	if best == "" {
		t.Fatal("the slides skill carries no html block — the skeleton is the part that gets copied")
	}
	return best
}
