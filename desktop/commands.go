package main

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

// ListCustomCommands reports the user's custom slash commands
// (<DataRoot>/commands/*.md) for the Settings page.
func (a *App) ListCustomCommands() []command.CustomCommand {
	return command.ListCustom()
}

// OpenCommandsFolder creates the commands directory if needed and reveals it
// in the OS file manager, so adding a command is "drop a .md file here".
func (a *App) OpenCommandsFolder() error {
	dir, err := command.CustomCommandsDir()
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
