package main

import "github.com/UserExistsError/conpty"

// startPTY opens a ConPTY running shellPath, sized and rooted where the caller
// asked. *conpty.ConPty already satisfies ptySession as it stands — Read,
// Write, Close and Resize(cols, rows) error — so there is no adapter here, and
// the Unix side is the one that needs a wrapper (terminal_unix.go).
//
// The explicit nil return on error is not ceremony: handing conpty.Start's
// (*ConPty, error) pair straight back would wrap a nil *ConPty in a non-nil
// ptySession, and every later nil check upstream would silently be wrong.
func startPTY(shellPath string, cols, rows int, dir string) (ptySession, error) {
	pty, err := conpty.Start(
		shellPath,
		conpty.ConPtyDimensions(cols, rows),
		conpty.ConPtyWorkDir(dir),
	)
	if err != nil {
		return nil, err
	}
	return pty, nil
}

// shellCandidates is what the terminal's "+" picker offers, best first.
// TerminalShells drops whichever of these the machine does not actually have.
func shellCandidates() []ShellProfile {
	return []ShellProfile{
		{Name: "PowerShell 7", Path: "pwsh.exe"},
		{Name: "Windows PowerShell", Path: "powershell.exe"},
		{Name: "Command Prompt", Path: "cmd.exe"},
		{Name: "Git Bash", Path: "bash.exe"},
		{Name: "WSL", Path: "wsl.exe"},
	}
}
