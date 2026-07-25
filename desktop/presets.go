package main

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

// ListPromptPresets reports every prompt preset for the Settings page —
// the ones bundled with Aetox plus anything in <DataRoot>/prompts/*.md.
func (a *App) ListPromptPresets() []command.Preset {
	return jsonSlice(command.ListPresets())
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
