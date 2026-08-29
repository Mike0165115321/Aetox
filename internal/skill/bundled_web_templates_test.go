package skill

// The web templates have to be web templates.
//
// Same treatment as the slide ones (bundled_templates_test.go) and for the same
// reason: these are files somebody copies verbatim, so a broken one is not
// advice that ages badly, it is every page built from it that day being wrong
// the same way.
//
// The rules checked here are the opposite of the slide contract in the one
// place that matters, and getting that backwards is the specific mistake this
// file exists to catch. A deck is a fixed 1280x720 box and must never be sized
// in viewport units; a web page is resized, zoomed and read on a phone, so a
// fixed pixel width is the defect instead.

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

func webTemplateFiles(t *testing.T) map[string]string {
	t.Helper()
	const dir = bundledSkillRoot + "/aetox-web-templates/sections"
	entries, err := fs.ReadDir(bundledSkillFS, dir)
	if err != nil {
		t.Fatalf("aetox-web-templates ships no sections folder: %v", err)
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
		t.Fatal("aetox-web-templates ships no sections at all")
	}
	return out
}

// One file, no build, no network. The page has to open from a USB stick.
func TestWebTemplatesAreSelfContained(t *testing.T) {
	banned := map[string]string{
		"<link":           "links a stylesheet, and the page is one self-contained file",
		"cdn.":            "fetches something at load time, so the page breaks offline",
		"https://unpkg":   "fetches something at load time, so the page breaks offline",
		"https://cdnjs":   "fetches something at load time, so the page breaks offline",
		"tailwindcss.com": "pulls a framework off the network",
		"src=\"http":      "pulls an asset off the network instead of shipping it beside the file",
		"@import":         "pulls another stylesheet in",
		"require(":        "expects a bundler",
		"node_modules":    "expects a build step",
	}
	for name, body := range webTemplateFiles(t) {
		flat := strings.ToLower(body)
		for needle, why := range banned {
			if strings.Contains(flat, needle) {
				t.Errorf("%s: contains %q, which means it %s", name, needle, why)
			}
		}
	}
}

// Responsive by construction, which is where the web contract is the exact
// opposite of the slide one. A hard pixel width on a layout box is the defect.
func TestWebTemplatesAreResponsive(t *testing.T) {
	// The whole set has to reach for the fluid primitives rather than a ladder
	// of breakpoints. Checked across the folder rather than per file, because a
	// footer honestly needs neither.
	var fluid, autofit int
	for _, body := range webTemplateFiles(t) {
		if strings.Contains(body, "clamp(") {
			fluid++
		}
		if strings.Contains(body, "auto-fit") {
			autofit++
		}
	}
	if fluid < 10 {
		t.Errorf("only %d templates use clamp() for type or space; the set is meant to be fluid rather than stepped", fluid)
	}
	if autofit < 4 {
		t.Errorf("only %d templates use an auto-fit grid; column counts should follow the content, not a breakpoint list", autofit)
	}

	// And no template may pin a layout box to a fixed width.
	//
	// `max-width` and `min-width` are the opposite of the defect and must not be
	// caught: a max-width is a cap the page still shrinks below, which is how
	// the docs page keeps its measure, and a min-width is what makes a table
	// scroll inside its own box rather than stretch the document. So the match
	// is anchored to the start of a declaration rather than done on substrings,
	// which is what flagged `max-width:1280px` the first time this ran.
	for name, body := range webTemplateFiles(t) {
		flat := strings.ReplaceAll(body, " ", "")
		for _, size := range []string{"1280px", "1440px", "960px", "720px"} {
			for _, prop := range []string{"width:", "height:"} {
				for _, anchor := range []string{"{" + prop, ";" + prop, "\n" + prop} {
					if strings.Contains(flat, anchor+size) {
						t.Errorf("%s: pins a box with %s%s — that is the slide contract, and a web page is resized", name, prop, size)
					}
				}
			}
		}
	}
}

// The parts a screen reader navigates by, and the parts a keyboard needs.
func TestWebTemplatesKeepTheirSemantics(t *testing.T) {
	want := map[string]string{
		"nav.html":          "<nav",
		"footer.html":       "<footer",
		"testimonial.html":  "<blockquote",
		"how-it-works.html": "<ol",
		"data-table.html":   "scope=",
		"faq.html":          "<details",
		"form-card.html":    "<label",
		"newsletter.html":   "<label",
		"article.html":      "datetime=",
		"docs-page.html":    "aria-label",
		"page-shell.html":   "w-skip",
	}
	files := webTemplateFiles(t)
	for name, needle := range want {
		body, ok := files[name]
		if !ok {
			t.Errorf("%s is named in this test but is not shipped", name)
			continue
		}
		if !strings.Contains(body, needle) {
			t.Errorf("%s no longer carries %q, which is what makes it navigable rather than merely styled", name, needle)
		}
	}
}

// Two templates pasted into one page must not reach into each other, which is
// the failure aetox-frontend-design warns about by name: a class selector and
// an element selector fighting over the same padding.
func TestWebTemplateClassesAreNamespaced(t *testing.T) {
	for name, body := range webTemplateFiles(t) {
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, ".") || !strings.Contains(trimmed, "{") {
				continue
			}
			selector := strings.TrimSpace(strings.SplitN(trimmed, "{", 2)[0])
			if !strings.HasPrefix(selector, ".w-") {
				t.Errorf("%s: selector %q does not start with the w- namespace, so it can collide with another template on the same page", name, selector)
			}
		}
	}
}

// The index and the folder agree, both ways. A section listed and not shipped
// is a door onto nothing; one shipped and not listed is invisible, because the
// model reads the table rather than the folder.
func TestTheWebTemplateIndexMatchesTheFolder(t *testing.T) {
	var body string
	for _, b := range bundledSkills() {
		if b.Name == "aetox-web-templates" {
			body = b.body
		}
	}
	if body == "" {
		t.Fatal("aetox-web-templates is not bundled")
	}
	files := webTemplateFiles(t)
	for name := range files {
		if !strings.Contains(body, "sections/"+name) {
			t.Errorf("sections/%s ships but SKILL.md never names it, so nothing will open it", name)
		}
	}
	for _, ref := range strings.Split(body, "`sections/")[1:] {
		end := strings.Index(ref, "`")
		if end <= 0 || !strings.HasSuffix(ref[:end], ".html") {
			continue
		}
		if _, ok := files[ref[:end]]; !ok {
			t.Errorf("SKILL.md points at sections/%s, which is not shipped", ref[:end])
		}
	}
}
