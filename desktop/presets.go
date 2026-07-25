package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/proc"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ListPromptPresets reports every prompt preset for the Settings page —
// the ones bundled with Aetox plus anything in <DataRoot>/prompts/*.md.
func (a *App) ListPromptPresets() []command.Preset {
	return jsonSlice(command.ListPresets())
}

// SetUILocale records the language the UI is showing so Aetox's own built-in
// provider can answer in it — the only part of the engine that has any business
// with language (ARCHITECTURE.md §40). Persisted next to the other preferences
// and applied through the same path a model change takes, so there is no new
// machinery here at all.
func (a *App) SetUILocale(locale string) error {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "" || locale == a.cfg.UILocale {
		return nil // nothing to do; never re-bootstrap for no reason
	}
	cfg := a.cfg
	cfg.UILocale = locale
	a.applyConfig(cfg)
	return nil
}

// SavePromptPreset writes (or overwrites) a user preset. Saving under a
// bundled preset's name creates an override — see internal/command.
func (a *App) SavePromptPreset(name, body string) error {
	return command.SavePreset(name, body)
}

// DeletePromptPreset removes a user preset and its cover. Deleting an override
// restores the bundled preset it was hiding.
func (a *App) DeletePromptPreset(name string) error {
	return command.DeletePreset(name)
}

// PickPresetImage opens the native picker and, if the user chose a file,
// copies it in as that preset's cover. Returns the cover as a data URI so the
// card updates without re-reading the whole list.
func (a *App) PickPresetImage(name string) (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "เลือกรูปหน้าปกพรอมต์",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Images (*.png, *.jpg, *.jpeg, *.webp, *.gif, *.bmp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.gif;*.bmp"},
		},
	})
	if err != nil || strings.TrimSpace(path) == "" {
		return "", err
	}
	if err := command.SavePresetImage(name, path); err != nil {
		return "", err
	}
	for _, p := range command.ListPresets() {
		if p.Name == name {
			return p.Image, nil
		}
	}
	return "", nil
}

// RemovePresetImage drops a preset's cover, keeping the prompt.
func (a *App) RemovePresetImage(name string) error {
	return command.RemovePresetImage(name)
}

// OpenPromptsFolder creates the prompts directory if needed and reveals it in
// the OS file manager, so adding a preset is "drop a .md file here". Creating
// it on demand is why the folder does not need to exist at install time.
func (a *App) OpenPromptsFolder() error {
	dir, err := command.PresetsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	proc.HideConsole(cmd)
	return cmd.Start()
}
