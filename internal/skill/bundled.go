package skill

// Some tools need a program Aetox ships alongside itself rather than one the
// machine already has. The Windows installer unpacks those into the install
// directory, because they arrive as plain archives with no installer of their
// own and so nothing puts them on PATH — poppler for pdf_read, ffmpeg for
// video_ocr and audio_transcribe.
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
)

// bundledBinary resolves one of those programs: the copy shipped next to
// Aetox's own executable first, then the bare name so PATH resolves it.
//
// Bundled wins over PATH deliberately — the shipped copy is the version that
// was pinned and tested, and a stale or broken one already on the machine
// should not quietly become the one that runs. The PATH fallback is what keeps
// a dev build, a package-manager install, or a non-Windows machine working at
// all.
//
// Falling back to the bare name (not a guessed absolute path) is load-bearing:
// it is what makes a genuinely missing binary come back as exec.ErrNotFound,
// which every caller turns into its own install instructions.
func bundledBinary(dir, name string) string {
	exeName := name
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		bundled := filepath.Join(filepath.Dir(exe), dir, "bin", exeName)
		if info, statErr := os.Stat(bundled); statErr == nil && !info.IsDir() {
			return bundled
		}
	}
	return name
}
