package skill

// The templates have to be templates.
//
// aetox-slide-templates ships markup that gets copied verbatim into somebody's deck,
// which makes it the same kind of file as the skeleton in aetox-slides and it
// gets the same treatment: checked by internal/deck, the parser the exporter
// itself runs, rather than by a regex that agrees with whatever the document
// happens to say. A broken template is not advice that ages badly, it is a
// slide that opens as source code, and every deck built from it that day is
// wrong the same way.
//
// The contract checks below are the ones that decide whether an exported deck
// comes out right, and every one of them is a rule some real deck broke:
// viewport units are laid out twice and differently (aetox-slides), a slide
// stacked at opacity:0 for a script to reveal exports as a blank rectangle, and
// a chart from a CDN prints empty.
//
// A test-only import of internal/deck, like the skeleton test's, so nothing
// here changes the production graph.

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/deck"
)

func templateFiles(t *testing.T) map[string]string {
	t.Helper()
	const dir = bundledSkillRoot + "/aetox-slide-templates/slides"
	entries, err := fs.ReadDir(bundledSkillFS, dir)
	if err != nil {
		t.Fatalf("aetox-slide-templates ships no slides folder: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		raw, readErr := fs.ReadFile(bundledSkillFS, path.Join(dir, e.Name()))
		if readErr != nil {
			t.Fatalf("reading %s: %v", e.Name(), readErr)
		}
		out[e.Name()] = string(raw)
	}
	if len(out) == 0 {
		t.Fatal("aetox-slide-templates ships no slide templates at all")
	}
	return out
}

// Every template is a slide the room and the exporter can both see. Without
// this a template is a file that looks like markup and routes to the editor.
func TestEverySlideTemplateIsAReadableSlide(t *testing.T) {
	for name, body := range templateFiles(t) {
		if got := deck.Count([]byte(body)); got != 1 {
			t.Errorf("%s: the exporter reads %d slides in it, want exactly 1 — a template is one slide to paste", name, got)
		}
		if !deck.Is([]byte(body)) {
			t.Errorf("%s: would not be routed to the slides room", name)
		}
	}
}

// The rules that decide whether the deck survives export. Each of these shipped
// broken somewhere before it was written down.
func TestSlideTemplatesObeyTheDeckContract(t *testing.T) {
	banned := map[string]string{
		"100vw":        "sized in viewport units, so the room and the exporter lay it out differently",
		"100vh":        "sized in viewport units, so the room and the exporter lay it out differently",
		"<script":      "carries script, and the deck is one file the exporter prints without running a deck runtime",
		"cdn.":         "fetches something at render time, which prints blank in the off-screen export",
		"https://cdn":  "fetches something at render time, which prints blank in the off-screen export",
		"<link":        "links a stylesheet, and the deck is one self-contained file",
		"chart.js":     "draws with a library fetched at render time, which prints blank",
		"opacity:0":    "starts hidden waiting for something to reveal it, which exports as a blank rectangle",
		"position:absolute;inset:0;opacity": "is stacked and hidden for a runtime to switch, which exports as one slide and blanks",
	}
	for name, body := range templateFiles(t) {
		flat := strings.ToLower(strings.ReplaceAll(body, " ", ""))
		for needle, why := range banned {
			if strings.Contains(flat, strings.ReplaceAll(needle, " ", "")) {
				t.Errorf("%s: contains %q, which means it %s", name, needle, why)
			}
		}
	}
}

// The skeleton owns the slide box. A template that redefines .slide silently
// wins over it, and the 1280x720 page stops being declared in one place.
func TestSlideTemplatesDoNotRedefineTheSlideBox(t *testing.T) {
	for name, body := range templateFiles(t) {
		flat := strings.ReplaceAll(body, " ", "")
		for _, bad := range []string{".slide{", ".slide,"} {
			if strings.Contains(flat, bad) {
				t.Errorf("%s: redefines %q — the skeleton in aetox-slides declares the slide box, once", name, bad)
			}
		}
	}
}

// The index in SKILL.md and the folder have to agree, in both directions. A
// layout listed and not shipped is a door onto nothing; one shipped and not
// listed is invisible, because the model reads the table rather than the
// folder.
func TestTheTemplateIndexMatchesTheFolder(t *testing.T) {
	var body string
	for _, b := range bundledSkills() {
		if b.Name == "aetox-slide-templates" {
			body = b.body
		}
	}
	if body == "" {
		t.Fatal("aetox-slide-templates is not bundled")
	}
	for name := range templateFiles(t) {
		if !strings.Contains(body, "slides/"+name) {
			t.Errorf("slides/%s ships but SKILL.md never names it, so nothing will open it", name)
		}
	}
	for _, ref := range strings.Split(body, "`slides/") {
		end := strings.Index(ref, "`")
		if end <= 0 || !strings.HasSuffix(ref[:end], ".html") {
			continue
		}
		if _, ok := templateFiles(t)[ref[:end]]; !ok {
			t.Errorf("SKILL.md points at slides/%s, which is not shipped", ref[:end])
		}
	}
}
