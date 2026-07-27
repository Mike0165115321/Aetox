package stt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The picker lists these folders to the user as "where Aetox looks". Every one
// of them has to be somewhere a whisper model can actually be found — Ollama's
// and LM Studio's were listed for a while and could never match, because
// neither stores models under a plain ggml-*.bin name.
func TestOnlyScansFoldersAWhisperModelCanBeFoundIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("AETOX_DATA_ROOT", filepath.Join(home, "data"))
	t.Setenv("OLLAMA_MODELS", filepath.Join(home, ".ollama", "models"))

	// Folders that exist and hold something, but nothing our glob can match —
	// exactly the state a real machine with Ollama installed is in.
	for _, d := range []string{
		filepath.Join(home, ".ollama", "models", "blobs"),
		filepath.Join(home, ".lmstudio", "models"),
		filepath.Join(home, ".cache", "huggingface", "hub"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	for _, s := range Stores(Options{}) {
		lower := strings.ToLower(s.Dir)
		for _, banned := range []string{".ollama", ".lmstudio", "huggingface"} {
			if strings.Contains(lower, banned) {
				t.Errorf("%s (%s) is scanned, but a whisper model cannot be found there — see stores.go", s.Label, s.Dir)
			}
		}
	}
}

// A folder the user names themselves is still honoured: that is the escape
// hatch for someone who keeps models somewhere of their own.
func TestExtraModelDirsAreStillScanned(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	mine := t.TempDir()
	found := false
	for _, s := range Stores(Options{ExtraModelDirs: []string{mine}}) {
		if s.Dir == mine {
			found = true
		}
	}
	if !found {
		t.Errorf("a user-specified model folder was dropped: %s", mine)
	}
}
