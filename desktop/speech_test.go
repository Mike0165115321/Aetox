package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// newSpeechTestApp bootstraps an App the way the desktop does, on an isolated
// data root with a fake model file in Aetox's own model folder.
func newSpeechTestApp(t *testing.T) (*App, string) {
	t.Helper()
	base := isolateUserDirs(t)
	dataRoot := filepath.Join(base, "data")
	t.Setenv("AETOX_DATA_ROOT", dataRoot)

	models := filepath.Join(dataRoot, "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	modelPath := filepath.Join(models, "ggml-base.bin")
	if err := os.WriteFile(modelPath, []byte("not really a model"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	a := &App{}
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-grid",
	})
	return a, modelPath
}

// The picker has to see models that are actually on disk, wherever they live.
func TestListSpeechModelsFindsTheManagedStore(t *testing.T) {
	a, modelPath := newSpeechTestApp(t)

	models := a.ListSpeechModels()
	if len(models) == 0 {
		t.Fatal("no models listed, want the one written into the managed store")
	}
	var found *SpeechModelInfo
	for i := range models {
		if strings.EqualFold(models[i].Path, modelPath) {
			found = &models[i]
		}
	}
	if found == nil {
		t.Fatalf("model %q not in the list: %+v", modelPath, models)
	}
	if !found.Managed {
		t.Error("a model in Aetox's own folder must be reported as managed")
	}
	if found.Active {
		t.Error("nothing is pinned yet, so no model should come back active")
	}
}

// The whole point: picking one has to stick, reach the engine, and survive a
// restart — which here means landing in the preference file resolveConfig reads.
func TestSetSpeechModelPinsPersistsAndClears(t *testing.T) {
	a, modelPath := newSpeechTestApp(t)

	if err := a.SetSpeechModel(modelPath); err != nil {
		t.Fatalf("SetSpeechModel: %v", err)
	}
	if a.cfg.SpeechModelPath != modelPath {
		t.Errorf("cfg.SpeechModelPath = %q, want %q", a.cfg.SpeechModelPath, modelPath)
	}

	pref, ok, err := config.LoadModelPreference()
	if err != nil || !ok {
		t.Fatalf("preference not saved: ok=%v err=%v", ok, err)
	}
	if pref.SpeechModelPath != modelPath {
		t.Errorf("saved preference = %q, want %q — the choice would not survive a restart", pref.SpeechModelPath, modelPath)
	}
	// The restart itself: resolveConfig is what startup builds its config from.
	if got := resolveConfig(config.ConfigOptions{RootPath: t.TempDir()}); got.SpeechModelPath != modelPath {
		t.Errorf("resolveConfig gave %q, want the saved %q", got.SpeechModelPath, modelPath)
	}

	active := 0
	for _, m := range a.ListSpeechModels() {
		if m.Active {
			active++
		}
	}
	if active != 1 {
		t.Errorf("%d models marked active, want exactly 1", active)
	}

	// Empty is a real choice — back to auto-discovery — not a no-op.
	if err := a.SetSpeechModel(""); err != nil {
		t.Fatalf("SetSpeechModel(\"\"): %v", err)
	}
	if a.cfg.SpeechModelPath != "" {
		t.Errorf("cfg.SpeechModelPath = %q, want empty after clearing", a.cfg.SpeechModelPath)
	}
	pref, _, _ = config.LoadModelPreference()
	if pref.SpeechModelPath != "" {
		t.Errorf("cleared choice was not written through: %q", pref.SpeechModelPath)
	}
}

// A model file that moved or was deleted must fail loudly at the moment of
// choosing, not silently later when a transcription is attempted.
func TestSetSpeechModelRejectsAMissingFile(t *testing.T) {
	a, _ := newSpeechTestApp(t)

	if err := a.SetSpeechModel(filepath.Join(t.TempDir(), "gone.bin")); err == nil {
		t.Fatal("expected an error for a model file that does not exist")
	}
	if a.cfg.SpeechModelPath != "" {
		t.Errorf("a rejected choice must not be stored, got %q", a.cfg.SpeechModelPath)
	}
}
