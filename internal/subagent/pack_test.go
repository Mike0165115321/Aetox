package subagent

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// The shipping label: four fields the resolver never reads, which exist so a
// store, an update check and an install screen have something stable to hold.
// The local id stays the folder name, which is why none of these touch it.
func TestParseReadsTheShippingLabel(t *testing.T) {
	p := parse("taxdoc", `---
description: เขียนเอกสารภาษี
publisher: Mike
package: mike/tax-doc
version: 1.2.0
requires-app: 1.4.0
---
brief`)
	if p.Name != "taxdoc" {
		t.Fatalf("Name = %q — the local id is the folder, never a field in the file", p.Name)
	}
	if p.Publisher != "Mike" || p.Package != "mike/tax-doc" || p.Version != "1.2.0" || p.RequiresApp != "1.4.0" {
		t.Fatalf("label = %+v", struct{ Pub, Pkg, Ver, Req string }{p.Publisher, p.Package, p.Version, p.RequiresApp})
	}
}

// A file written before these fields existed is not a broken file. The whole
// point of frontmatter being additive is that it stays additive.
func TestParseWithoutTheShippingLabelIsHealthy(t *testing.T) {
	p := parse("doc", "---\ndescription: d\n---\nbrief")
	if p.Invalid != "" {
		t.Fatalf("Invalid = %q", p.Invalid)
	}
	if p.Publisher != "" || p.Package != "" || p.Version != "" || p.RequiresApp != "" {
		t.Fatalf("empty fields were invented: %+v", p)
	}
}

// A worker is an overlay, not a directory: what the user wrote, over what
// shipped under the same name. An exporter handed only one of the two would
// ship a half worker and call it whole.
func TestPackageSourcesOverlaysUserOverBundled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	home := filepath.Join(root, "agents", "github")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, config.AgentDefinitionFile), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	sources := PackageSources("github")
	if len(sources) != 2 {
		t.Fatalf("got %d sources, want the user's folder and the shipped copy", len(sources))
	}
	first, err := fs.ReadFile(sources[0], config.AgentDefinitionFile)
	if err != nil {
		t.Fatalf("read first source: %v", err)
	}
	if string(first) != "mine" {
		t.Fatalf("first source is not the user's own: %q", first)
	}
	if _, err := fs.Stat(sources[1], "skills"); err != nil {
		t.Fatalf("shipped source carries no skills folder: %v", err)
	}
}

// A name nobody ships and nobody wrote is no sources at all, rather than an
// empty folder the caller has to defend against.
func TestPackageSourcesForANameThatIsNotHere(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if got := PackageSources("นักบินอวกาศ"); len(got) != 0 {
		t.Fatalf("got %d sources for a worker that does not exist", len(got))
	}
}
