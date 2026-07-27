package stt

// Where model files live, and whose they are.
//
// Two kinds of directory, deliberately never mixed:
//
//   - **Managed** — <DataRoot>/models. Aetox downloads here, may replace or
//     delete here, and the settings UI offers a delete button here. One
//     directory, ours, predictable.
//   - **External** — model stores that belong to other tools (Ollama, LM
//     Studio, the HuggingFace cache) or any folder the user points at. Aetox
//     reads them and never writes, replaces or deletes anything inside. A user
//     who already has 40GB of models should not be made to download again, but
//     Aetox does not get to own someone else's directory either.
//
// Owner asked for exactly this split — "ตำแหน่งโมเดลของระบบเราด้วย แบบแยกมา
// จะได้ไม่สับสน" (ARCHITECTURE.md §33).

import (
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	Label string
	Dir   string
	// Managed is true only for the directory Aetox itself downloads into.
	// Everything else is read-only to Aetox, no matter who put it there.
	Managed bool
}

// Stores lists every directory to look for models in, managed first. Only
// directories that exist are returned — an empty list means nothing is
// installed anywhere, which is a real state the UI has to render.
func Stores(opts Options) []Store {
	var stores []Store
	if dir, err := ModelDir(opts); err == nil && dir != "" {
		stores = append(stores, Store{Label: "Aetox", Dir: dir, Managed: true})
	}
	if dir := shippedModelDir(); dir != "" {
		stores = append(stores, Store{Label: "มากับ Aetox", Dir: dir})
	}
	for _, dir := range opts.ExtraModelDirs {
		if dir = strings.TrimSpace(dir); dir != "" {
			stores = append(stores, Store{Label: "โฟลเดอร์ที่คุณตั้งไว้", Dir: dir})
		}
	}

	seen := make(map[string]bool, len(stores))
	out := make([]Store, 0, len(stores))
	for _, s := range stores {
		key := strings.ToLower(filepath.Clean(s.Dir))
		if seen[key] {
			continue
		}
		seen[key] = true
		// The managed directory is listed even before it exists — it is where
		// the next download goes, and the UI needs somewhere to point at.
		if !s.Managed && !isDir(s.Dir) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// Ollama's, LM Studio's and the HuggingFace cache's model folders were scanned
// here too, on the reasoning that someone who already has 40GB of models should
// not have to download again. That reasoning was about LLMs and does not carry
// to speech:
//
//   - Ollama stores content-addressed blobs (models/blobs/sha256-…) with no
//     file names at all, so a name glob can never match one.
//   - LM Studio keeps .gguf under publisher/repo — a different format for a
//     different runtime, which whisper.cpp cannot load.
//   - The HuggingFace cache does hold real ggml file names, but nested under
//     models--org--repo/snapshots/<sha>/, and InstalledModels globs one level
//     deep by design.
//
// Checked against a machine with Ollama actually installed: zero matches in any
// of the three. They were folders the picker promised to search and never
// could. Options.ExtraModelDirs remains for a user who keeps models elsewhere.

// shippedModelDir is <install dir>/models — the starter model the Windows
// installer unpacks so a fresh install can transcribe without the user
// downloading anything first.
//
// Not Managed: the installer put it there and the uninstaller takes it away, so
// a delete button in the settings UI would be lying about who owns it. It also
// cannot live in <DataRoot>/models — that is a per-user path, and a
// per-machine install has no user to write it for.
func shippedModelDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "models")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
