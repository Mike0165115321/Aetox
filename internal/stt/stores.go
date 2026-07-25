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
	for _, dir := range opts.ExtraModelDirs {
		if dir = strings.TrimSpace(dir); dir != "" {
			stores = append(stores, Store{Label: "โฟลเดอร์ที่คุณตั้งไว้", Dir: dir})
		}
	}
	stores = append(stores, KnownExternalStores()...)

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

// KnownExternalStores returns the model directories other local-AI tools use on
// this machine, whether or not they exist — callers filter. Aetox looks here so
// a user who does not know where their models live does not have to find out.
func KnownExternalStores() []Store {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	join := func(parts ...string) string {
		if home == "" {
			return ""
		}
		return filepath.Join(append([]string{home}, parts...)...)
	}
	stores := []Store{
		{Label: "Ollama", Dir: envOr("OLLAMA_MODELS", join(".ollama", "models"))},
		{Label: "LM Studio", Dir: join(".lmstudio", "models")},
		{Label: "LM Studio", Dir: join(".cache", "lm-studio", "models")},
		{Label: "Hugging Face cache", Dir: envOr("HF_HOME", join(".cache", "huggingface", "hub"))},
	}
	out := make([]Store, 0, len(stores))
	for _, s := range stores {
		if strings.TrimSpace(s.Dir) != "" {
			out = append(out, s)
		}
	}
	return out
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
