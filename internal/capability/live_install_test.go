package capability

// The one test that actually downloads. Off by default and gated on an
// environment variable, the same shape internal/model/live_all_providers_test.go
// uses, because an ordinary `go test ./...` must not pull 150MB across
// somebody's connection.
//
//	AETOX_LIVE_CAPABILITY=1 go test ./internal/capability/ -run Live -v
//
// What it is for: every other test in this package works on archives it built
// itself, so all of them would still pass with a pin pointing at the wrong
// release, a SubPath that stopped matching after a version bump, or an archive
// whose layout upstream quietly rearranged. Those are the failures that reach a
// user, and the only way to see them is to fetch the real bytes.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLiveInstallEveryComponent(t *testing.T) {
	if os.Getenv("AETOX_LIVE_CAPABILITY") == "" {
		t.Skip("set AETOX_LIVE_CAPABILITY=1 to download and unpack the real manifest")
	}
	if runtime.GOOS != "windows" {
		t.Skip("manifest is win64-only; see Manifest()")
	}
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	comps := Missing()
	if len(comps) == 0 {
		t.Fatal("nothing missing against a fresh data root — Installed() is answering wrong")
	}
	if err := Install(context.Background(), comps, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	for _, c := range comps {
		if !c.Installed() {
			t.Errorf("%s: install reported success but Installed() says no", c.ID)
		}
	}
	if left := Missing(); len(left) != 0 {
		t.Errorf("still missing after a full install: %v", left)
	}
}

// Downloading Tesseract is not the same as being able to read Thai with it: the
// tree we unpack is not a registered install, so tesseract.exe cannot find its
// own tessdata and answers "Failed loading language" — a message about
// languages, not about a missing folder, which is exactly why this is worth a
// test rather than an assumption. See tesseractEnv in internal/skill/image_ocr.go.
func TestLiveTesseractCanLoadThai(t *testing.T) {
	if os.Getenv("AETOX_LIVE_CAPABILITY") == "" {
		t.Skip("set AETOX_LIVE_CAPABILITY=1 to download and unpack the real manifest")
	}
	if runtime.GOOS != "windows" {
		t.Skip("manifest is win64-only; see Manifest()")
	}
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)

	var tess Component
	for _, c := range Manifest() {
		if c.ID == "tesseract" {
			tess = c
		}
	}
	if tess.ID == "" {
		t.Fatal("no tesseract in the manifest")
	}
	if err := Install(context.Background(), []Component{tess}, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	dir := filepath.Join(root, tess.Dest)
	binary := filepath.Join(dir, tess.Probe)
	cmd := exec.Command(binary, "--list-langs")
	cmd.Env = append(os.Environ(), "TESSDATA_PREFIX="+filepath.Join(dir, "tessdata"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--list-langs: %v\n%s", err, out)
	}
	langs := string(out)
	for _, want := range []string{"tha", "eng"} {
		if !strings.Contains(langs, want) {
			t.Errorf("%q is not in the language list:\n%s", want, langs)
		}
	}
}
