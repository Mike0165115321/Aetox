package skill

import (
	"strings"
	"testing"
)

// A description long enough to wrap is written as a block scalar, and that is
// not an exotic shape — it is what every generator emits and what published
// skills arrive in.
//
// Read one line at a time, `description: >-` is a value of ">-" and the wrapped
// text is a run of lines with no colon in them, so the description became the
// literal string ">-". The skill installed fine, listed fine, and told the model
// nothing about itself. Nothing failed; there was simply no sentence there.
//
// Found on the owner's machine 2026-08-20, in `senior-architect-agent`, by
// noticing a blank line in `skills_list` — which is the only way it could have
// been found.
func TestAFoldedDescriptionSurvives(t *testing.T) {
	doc := "---\nname: senior-architect-agent\ndescription: >-\n" +
		"  Cognitive framework that transforms AI agents into senior architects,\n" +
		"  evidence-first system understanding, and architecture reasoning\n" +
		"  before action.\n---\nbody here\n"

	fields, body, err := ParseFrontmatter(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := "Cognitive framework that transforms AI agents into senior architects, " +
		"evidence-first system understanding, and architecture reasoning before action."
	if fields["description"] != want {
		t.Errorf("description = %q,\nwant %q", fields["description"], want)
	}
	if fields["name"] != "senior-architect-agent" {
		t.Errorf("name = %q", fields["name"])
	}
	if body != "body here" {
		t.Errorf("body = %q", body)
	}
}

// `|` keeps its line breaks. Both indicators take a trailing -/+ for how many
// newlines to leave at the end, which TrimSpace settles either way.
func TestALiteralBlockKeepsItsLines(t *testing.T) {
	doc := "---\nname: x\ndescription: |\n  first\n  second\n---\n"
	fields, _, err := ParseFrontmatter(doc)
	if err != nil {
		t.Fatal(err)
	}
	if fields["description"] != "first\nsecond" {
		t.Errorf("description = %q, want the two lines kept apart", fields["description"])
	}
	for _, indicator := range []string{">", ">+", "|-", "|+"} {
		f, _, err := ParseFrontmatter("---\nname: x\ndescription: " + indicator + "\n  text\n---\n")
		if err != nil {
			t.Fatal(err)
		}
		if f["description"] != "text" {
			t.Errorf("%q: description = %q", indicator, f["description"])
		}
	}
}

// A blank line inside a folded scalar is a paragraph break and stays one. The
// rest of the single newlines become spaces, which is what folding means.
func TestAFoldedBlockKeepsItsParagraphs(t *testing.T) {
	doc := "---\ndescription: >-\n  one\n  two\n\n  three\n---\n"
	fields, _, err := ParseFrontmatter(doc)
	if err != nil {
		t.Fatal(err)
	}
	if fields["description"] != "one two\nthree" {
		t.Errorf("description = %q", fields["description"])
	}
}

// The block ends where the indentation does. Without that, a block scalar would
// swallow every key after it and a profile would lose its model, its tools and
// its desk to a long description.
func TestABlockStopsAtTheNextKey(t *testing.T) {
	doc := "---\nname: x\ndescription: >-\n  wrapped text\n  more of it\nmodel: haiku\ntools: read, write\n---\n"
	fields, _, err := ParseFrontmatter(doc)
	if err != nil {
		t.Fatal(err)
	}
	if fields["description"] != "wrapped text more of it" {
		t.Errorf("description = %q", fields["description"])
	}
	if fields["model"] != "haiku" || fields["tools"] != "read, write" {
		t.Errorf("the keys after the block were swallowed: %+v", fields)
	}
}

// The ordinary one-line shape every file in this repo is written in, unchanged.
func TestAPlainValueIsUntouched(t *testing.T) {
	doc := "---\nname: aetox-slides\ndescription: \"quoted, with a colon: here\"\n---\nbody\n"
	fields, body, err := ParseFrontmatter(doc)
	if err != nil {
		t.Fatal(err)
	}
	if fields["description"] != "quoted, with a colon: here" {
		t.Errorf("description = %q", fields["description"])
	}
	if !strings.Contains(body, "body") {
		t.Errorf("body = %q", body)
	}
}
