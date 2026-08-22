package capability

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The pins are the whole security story of this package: a URL that can be
// rebuilt in place, or a digest of the wrong length, turns "verified download"
// into a phrase rather than a check. Cheap to assert, and it fails at desk
// rather than on a stranger's machine.
func TestManifestPinsAreWellFormed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("manifest is win64-only; see Manifest()")
	}
	seen := map[string]bool{}
	for _, c := range Manifest() {
		if seen[c.ID] {
			t.Errorf("%s: duplicate id", c.ID)
		}
		seen[c.ID] = true

		if !strings.HasPrefix(c.URL, "https://") {
			t.Errorf("%s: URL is not https: %q", c.ID, c.URL)
		}
		if len(c.SHA256) != 64 {
			t.Errorf("%s: SHA256 is %d chars, want 64", c.ID, len(c.SHA256))
		}
		if strings.ToLower(c.SHA256) != c.SHA256 {
			t.Errorf("%s: SHA256 must be lower-case — the comparison is EqualFold, but a mixed-case pin hides which form is canonical", c.ID)
		}
		if c.Dest == "" || c.Probe == "" {
			t.Errorf("%s: Dest and Probe are both required", c.ID)
		}
		if c.ApproxBytes <= 0 {
			t.Errorf("%s: ApproxBytes is what the button says out loud", c.ID)
		}
		switch c.Kind {
		case KindZip:
			if c.SubPath == "" {
				t.Errorf("%s: a zip needs a SubPath", c.ID)
			}
			// Strip must not exceed SubPath's depth, or every entry is
			// dropped and the unpack silently produces nothing.
			if depth := len(strings.Split(strings.Trim(c.SubPath, "/"), "/")); c.Strip > depth {
				t.Errorf("%s: Strip %d is deeper than SubPath %q", c.ID, c.Strip, c.SubPath)
			}
		case KindFile:
			if c.SubPath != "" || c.Strip != 0 {
				t.Errorf("%s: SubPath/Strip mean nothing for a plain file", c.ID)
			}
		default:
			t.Errorf("%s: unknown kind %q", c.ID, c.Kind)
		}
	}
}

// Traversal in a zip has two shapes on Windows, and only one of them reaches
// safeJoin.
//
// A forward-slash "../" is resolved by path.Clean before the SubPath filter
// runs, so the entry stops matching the prefix and is skipped: never written,
// never an error. A BACKSLASH one survives Clean untouched (path treats "\\" as
// an ordinary character), passes the prefix test, and only becomes a separator
// later in filepath.FromSlash — on Windows, and only there. That is the case
// safeJoin exists for, and the reason it is not dead code.
func TestUnzipRefusesEscapingEntry(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "evil.txt")

	forward := writeZip(t, map[string]string{
		"pkg/lib/bin/ok.txt":    "fine",
		"pkg/lib/../../../evil": "no",
	})
	c := Component{Kind: KindZip, SubPath: "pkg/lib", Strip: 2}
	if err := c.unzip(forward, root); err != nil {
		t.Fatalf("unzip: %v", err)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("a ../ entry was written outside the destination")
	}
	if !isFile(filepath.Join(root, "bin", "ok.txt")) {
		t.Error("the legitimate entry beside it was lost")
	}

	if runtime.GOOS != "windows" {
		t.Skip(`a backslash is not a separator here, so the entry stays inside root`)
	}
	back := writeZip(t, map[string]string{
		`pkg/lib/bin/..\..\..\evil.txt`: "no",
	})
	err := c.unzip(back, t.TempDir())
	if err == nil {
		t.Fatal("a backslash traversal entry was accepted")
	}
	if !strings.Contains(err.Error(), "นอกโฟลเดอร์ปลายทาง") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestUnzipStripsAndSelectsSubtree(t *testing.T) {
	root := t.TempDir()
	archive := writeZip(t, map[string]string{
		"pkg-1.2/Library/bin/tool.exe":   "T",
		"pkg-1.2/Library/share/data.txt": "D",
		"pkg-1.2/include/header.h":       "H",
		"other/bin/nope.exe":             "N",
	})
	c := Component{Kind: KindZip, SubPath: "pkg-1.2/Library", Strip: 2}
	if err := c.unzip(archive, root); err != nil {
		t.Fatalf("unzip: %v", err)
	}
	for _, want := range []string{
		filepath.Join("bin", "tool.exe"),
		filepath.Join("share", "data.txt"),
	} {
		if !isFile(filepath.Join(root, want)) {
			t.Errorf("missing %s", want)
		}
	}
	// Outside SubPath, so it must not have been written anywhere.
	for _, unwanted := range []string{"include", "other", "pkg-1.2"} {
		if _, err := os.Stat(filepath.Join(root, unwanted)); err == nil {
			t.Errorf("%s was extracted but is outside SubPath", unwanted)
		}
	}
}

// The archive tools.yml builds is already the tree image_ocr wants, so its
// SubPath is ".". path.Clean drops a leading "./" from every entry name, which
// means the obvious prefix of "./" matches nothing and the unpack quietly
// produces an empty folder — caught here rather than as a missing tesseract.exe
// on someone's first OCR.
func TestUnzipTakesWholeArchiveWhenSubPathIsDot(t *testing.T) {
	root := t.TempDir()
	archive := writeZip(t, map[string]string{
		"tesseract.exe":            "T",
		"tessdata/tha.traineddata": "D",
	})
	c := Component{Kind: KindZip, SubPath: ".", Strip: 0}
	if err := c.unzip(archive, root); err != nil {
		t.Fatalf("unzip: %v", err)
	}
	for _, want := range []string{"tesseract.exe", filepath.Join("tessdata", "tha.traineddata")} {
		if !isFile(filepath.Join(root, want)) {
			t.Errorf("missing %s", want)
		}
	}
}

// A stale SubPath after a version bump would otherwise surface one step later
// as a missing Probe, which reads like a failed download.
func TestUnzipSaysWhenSubPathMatchesNothing(t *testing.T) {
	archive := writeZip(t, map[string]string{"pkg-9.9/bin/tool.exe": "T"})
	c := Component{Kind: KindZip, SubPath: "pkg-1.2/Library", Strip: 2}
	err := c.unzip(archive, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "ไม่พบ") {
		t.Fatalf("want a not-found-in-archive error, got %v", err)
	}
}

// Decision 4 of the architecture note: someone upgrading from a release whose
// NSIS installer did the fetching already owns these files, next to the
// executable rather than under DataRoot, and must not be asked for 150MB again.
func TestInstalledAcceptsEitherAddress(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)

	c := Component{
		ID:     "poppler",
		Dest:   filepath.Join("tools", "poppler"),
		Probe:  filepath.Join("bin", "pdftotext.exe"),
		Marker: "poppler-26.02.0.ok",
	}
	if c.Installed() {
		t.Fatal("reported installed with nothing on disk")
	}

	root, err := c.Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if got, want := root, filepath.Join(dataRoot, "tools", "poppler"); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}

	// Probe without marker is an older pin's tree, not this one's.
	touch(t, filepath.Join(root, c.Probe))
	if c.Installed() {
		t.Error("a tree with no version marker was accepted")
	}
	// Marker without probe is a marker that outlived its files.
	os.Remove(filepath.Join(root, c.Probe))
	touch(t, filepath.Join(root, c.Marker))
	if c.Installed() {
		t.Error("a marker with no files behind it was accepted")
	}
	touch(t, filepath.Join(root, c.Probe))
	if !c.Installed() {
		t.Error("a complete tree under DataRoot was not accepted")
	}

	// And the legacy address, which is what an upgrade actually has.
	os.RemoveAll(root)
	legacy := c.legacyRoot()
	if legacy == "" {
		t.Skip("os.Executable unavailable")
	}
	touch(t, filepath.Join(legacy, c.Probe))
	touch(t, filepath.Join(legacy, c.Marker))
	t.Cleanup(func() { os.RemoveAll(legacy) })
	if !c.Installed() {
		t.Error("files left by an older installer beside the exe were not accepted")
	}
}

// One row per capability, not per download. Speech is the case that matters:
// the engine and its model are two pinned files and neither transcribes
// anything alone, so offering them as separate ticks would let someone choose a
// combination that cannot work.
func TestStatusesGroupByCapabilityAndPriceOnlyTheMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("manifest is win64-only; see Manifest()")
	}
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	rows := Statuses()
	seen := map[string]int{}
	for _, r := range rows {
		seen[r.Capability]++
	}
	for cap, n := range seen {
		if n != 1 {
			t.Errorf("capability %q drew %d rows, want 1", cap, n)
		}
	}

	var speech Status
	for _, r := range rows {
		if r.Capability == "speech" {
			speech = r
		}
	}
	if speech.Capability == "" {
		t.Fatal("no speech row")
	}
	if speech.Installed {
		t.Error("speech reported installed against an empty data root")
	}
	// Whatever the pins are, the row must quote the sum of its parts rather
	// than any one of them.
	var want int64
	for _, c := range Manifest() {
		if c.Capability == "speech" {
			want += c.ApproxBytes
		}
	}
	if speech.ApproxBytes != want {
		t.Errorf("speech priced at %d, want the sum of its parts %d", speech.ApproxBytes, want)
	}
}

func TestMissingForSelectsWholeCapabilities(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("manifest is win64-only; see Manifest()")
	}
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	ids := func(comps []Component) []string {
		out := make([]string, 0, len(comps))
		for _, c := range comps {
			out = append(out, c.ID)
		}
		sort.Strings(out)
		return out
	}

	// Ticking one capability takes every part of it and nothing else.
	if got, want := ids(MissingFor([]string{"speech"})), []string{"speech-model", "whisper"}; !slices.Equal(got, want) {
		t.Errorf("MissingFor(speech) = %v, want %v", got, want)
	}
	// An untouched screen sends an empty list, which must install nothing —
	// not, by a nil-versus-empty slip, everything.
	if got := MissingFor([]string{}); len(got) != 0 {
		t.Errorf("MissingFor(empty) = %v, want nothing", ids(got))
	}
	// A name from a window left open across an update matches nothing.
	if got := MissingFor([]string{"telepathy"}); len(got) != 0 {
		t.Errorf("MissingFor(unknown) = %v, want nothing", ids(got))
	}
	if len(MissingFor(nil)) != len(Manifest()) {
		t.Error("MissingFor(nil) must mean all of them")
	}
}

func writeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
