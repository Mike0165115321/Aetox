package skill

// Aetox's own programs, put where a shell can find them by name.
//
// internal/capability downloads ffmpeg, poppler and tesseract into
// <DataRoot>/tools/ and deliberately never touches the machine's PATH — that
// rule is what keeps the install unelevated and leaves every other program on
// the computer as it was, and it is not in question here. What it costs is
// invisible from inside this process, because Aetox's own tools all resolve
// through bundledBinary and never notice: they are handed an absolute path.
//
// The agent's shell is the one caller that cannot be handed one. It runs a
// command line the model wrote, and a model that has been told this machine
// can read video writes `ffmpeg`. Measured on the owner's machine
// (shell-audit.log, 2026-09-01T01:14:09): the video agent ran a bare `ffmpeg
// -ss 0 -i … -c copy` trim, got `exit status 1`, and spent every command after
// it carrying `"$env:APPDATA\Aetox\tools\ffmpeg-gpl\bin\ffmpeg.exe"` by hand.
// It recovered, which is why this was a tax rather than a failure — a wasted
// round, then a longer command line on each of the seven after it.
//
// So the tools go on the PATH of the shell Aetox starts, and nowhere else.
// That is a different act from the one capability.go refuses: this process
// hands its own child an environment, which is the ordinary way to tell a
// program where something is, and nothing outside this command can see it.
//
// APPENDED, never prepended, which is the same rule shellEnv() states one file
// over: a default arriving later must not overwrite one the user already has.
// Someone with their own ffmpeg keeps running their own ffmpeg. The only thing
// this changes is a name that used to resolve to nothing.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// shellToolDirs is which of Aetox's folders a shell may resolve names out of,
// most capable first. Explicit rather than derived from capability.Manifest(),
// because the manifest answers a different question — what to download and how
// to know it arrived — and three of its entries answer this one with "no":
//
//   - kinocut is an embeddable Python. On PATH it would answer to `python` for
//     an agent that meant the machine's Python, and it has no pip, no site,
//     and a `._pth` that exists to keep its sys.path pointed at one library.
//     A `python` that is not a python is worse than no python.
//   - whisper's folder holds main.exe, command.exe, bench.exe and stream.exe
//     beside whisper-cli.exe. Those four names belong to nobody.
//   - hyperframes ships node.exe with no npm beside it, and half a runtime
//     invites a command that cannot finish.
//
// Each of those is still reachable — by the absolute path its own caller
// already resolves — so nothing loses a capability here. They are only kept
// out of the namespace where a bare word decides what runs.
//
// ffmpeg-gpl before ffmpeg for the reason capability.go gives at length: they
// hold the same three names, and only the GPL build carries libx264, so a
// shell that found the LGPL one first would fail every encode with "Unknown
// encoder" while a working copy sat one folder away.
var shellToolDirs = []string{
	filepath.Join("ffmpeg-gpl", "bin"),
	filepath.Join("ffmpeg", "bin"),
	filepath.Join("poppler", "bin"),
	"tesseract",
}

// toolPATHDirs is those folders as they exist on this machine, absolute, in
// the order a shell should search them. Empty on a machine that has installed
// none of them, and on every non-Windows one — capability.Manifest() is
// Windows-only, so there is nothing here for a Linux shell to find and its
// package manager has already answered the question this solves.
//
// bundledRoots() rather than capability.Component.Root(), so the copy an
// installer up to v1.4.0 left beside aetox.exe is found too. That is the same
// pair of addresses bundledBinary searches, in the same order.
func toolPATHDirs() []string {
	var dirs []string
	for _, base := range bundledRoots() {
		for _, rel := range shellToolDirs {
			dir := filepath.Join(base, rel)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}

// withToolPATH extends cmd's PATH with those folders.
//
// Native shells only. A command bound for WSL runs in a filesystem where
// `C:\Users\…\aetox\tools` names nothing, and appending it there would put a
// dead entry on the PATH of every command the user runs in their distro. That
// is proc.IsNative's stated purpose (backend.go) and rtk already skips itself
// on the same test, for the same reason.
//
// The existing entry is found case-insensitively and rewritten in place:
// Windows spells it `Path`, and appending a second `PATH=` would leave two
// spellings of one variable in the block and let the wrong one win.
func withToolPATH(b proc.Backend, cmd *exec.Cmd) *exec.Cmd {
	if cmd == nil || !proc.IsNative(b) {
		return cmd
	}
	dirs := toolPATHDirs()
	if len(dirs) == 0 {
		return cmd
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	added := strings.Join(dirs, string(os.PathListSeparator))
	for i, kv := range env {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.EqualFold(name, "PATH") {
			continue
		}
		if value == "" {
			env[i] = name + "=" + added
		} else {
			env[i] = name + "=" + value + string(os.PathListSeparator) + added
		}
		cmd.Env = env
		return cmd
	}
	cmd.Env = append(env, "PATH="+added)
	return cmd
}
