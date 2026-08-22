package skill

// Some tools need a program Aetox ships alongside itself rather than one the
// machine already has. They arrive as plain archives with no installer of
// their own, so nothing puts them on PATH — poppler for pdf_read, ffmpeg for
// video_ocr and audio_transcribe.
//
// Two addresses hold them, and both are checked. internal/capability now
// downloads them into <DataRoot>/tools/<name>/, which a normally-launched
// Aetox can write to; releases up to v1.4.0 had the elevated NSIS installer
// unpack them next to aetox.exe instead. Someone upgrading already has the
// second kind and must not be asked to fetch 150MB they already own
// (docs/architecture/capability-install-2026-08-21.md).
//
// Tesseract does not go through here — it arrives with a real installer, so it
// lands in Program Files rather than in our tree. That is not the same as
// being findable: the silent install leaves PATH alone, so image_ocr.go's
// resolveTesseract has to know that address itself.
//
// Editing the machine's PATH is the alternative, and a much worse thing to get
// wrong on someone else's computer.

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// bundledBinary resolves one of those programs: the copy this Aetox
// downloaded, then the copy an older installer left next to the executable,
// then the bare name so PATH resolves it.
//
// Bundled wins over PATH deliberately — the shipped copy is the version that
// was pinned and tested, and a stale or broken one already on the machine
// should not quietly become the one that runs. The PATH fallback is what keeps
// a dev build, a package-manager install, or a non-Windows machine working at
// all.
//
// DataRoot wins over next-to-the-executable, because it is the address the
// current pin writes to: after a version bump, the downloaded copy is the one
// that was tested against this build, and the one beside the exe is whatever
// the installer of some earlier release happened to leave there.
//
// Falling back to the bare name (not a guessed absolute path) is load-bearing:
// it is what makes a genuinely missing binary come back as exec.ErrNotFound,
// which every caller turns into its own install instructions.
func bundledBinary(dir, name string) string {
	exeName := name
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	for _, base := range bundledRoots() {
		candidate := filepath.Join(base, dir, "bin", exeName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return name
}

// bundledRoots is the two directories that can hold a bundled program, most
// current first. Either may be missing — a fresh install has nothing in
// DataRoot yet, and a portable zip has no installer-written tree beside it.
func bundledRoots() []string {
	var roots []string
	if root, err := config.DataRoot(); err == nil {
		roots = append(roots, filepath.Join(root, "tools"))
	}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	return roots
}
