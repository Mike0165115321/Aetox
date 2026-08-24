package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Channel is how this copy of Aetox got onto the machine. It is the whole
// reason this package exists.
//
// The tempting design — download the new exe and overwrite ourselves — is the
// one Aetox deliberately does not use. It needs a signing key and a signed
// update feed to be safe, it needs elevation because the Windows installer
// puts us under Program Files, and on macOS replacing your own bundle without
// notarization breaks Gatekeeper. opencode solves this by asking *how it was
// installed* and then handing the upgrade back to whoever installed it
// (`scoop install`, `brew upgrade`, `npm i -g`) instead of touching its own
// files. That design is why it runs on eight install methods without owning an
// updater at all, and it is what this package copies.
//
// The practical test for adding a channel is not "does this platform exist"
// but "can we say something more useful here than 'open the releases page'".
// Anything we cannot beat that answer for stays ChannelUnknown, which is
// always correct and never wrong.
type Channel string

const (
	// ChannelScoop — Scoop already knows how to upgrade us, and its manifest
	// (scoop/aetox.json) already carries checkver+autoupdate. Aetox must never
	// write into a Scoop app directory; it tells the user the one command.
	ChannelScoop Channel = "scoop"

	// ChannelInstaller — the NSIS build, under Program Files. Upgradable by
	// re-running the installer for the new release, which already taskkills the
	// running app for us (desktop/build/windows/installer/project.nsi).
	ChannelInstaller Channel = "installer"

	// ChannelPortable — the zip, unpacked wherever the user chose. Writable,
	// but it is their folder and their layout; we point at the new zip.
	ChannelPortable Channel = "portable"

	// ChannelStore — the MSIX package from the Microsoft Store, which lives in
	// Program Files\WindowsApps. Windows owns updates here and the package's
	// own files are not ours to replace, so this channel's whole job is to make
	// Check stop: no GitHub call, no download offer, no command to run. It has
	// to exist as its own case rather than fall into ChannelInstaller, which is
	// where the path alone would put it — that would have told a Store user to
	// go and re-run an NSIS installer that would then install a second copy.
	ChannelStore Channel = "store"

	// ChannelUnknown — everything else, including every non-Windows install
	// until those platforms actually ship. Resolves to "open the releases
	// page", which is the honest answer when we cannot do better.
	//
	// When the Linux/macOS port lands (ARCHITECTURE.md §48), the cases to add
	// here are Homebrew (path under a Cellar), AppImage (an .AppImage suffix),
	// and a system package (under /usr/bin with a dpkg/rpm owner). They are
	// deliberately not guessed at now: untested detection that names the wrong
	// upgrade command is worse than the fallback it replaced.
	ChannelUnknown Channel = "unknown"
)

// Detect works out how this build was installed from where its own executable
// sits. Cheap, read-only, and wrong-answers-safe: every path that is not
// positively identified falls through to ChannelUnknown.
func Detect() Channel {
	exe, err := os.Executable()
	if err != nil {
		return ChannelUnknown
	}
	return detectFrom(exe, filepath.EvalSymlinks, runtime.GOOS, os.Getenv)
}

// detectFrom takes the executable path, the symlink resolver, the OS and the
// environment as parameters for one reason: Scoop is the channel this package
// most needs to get right, it hinges on a symlink farm, and a test cannot
// conjure one on Windows without privileges. Every input being injectable
// means the whole table runs on all three CI platforms.
func detectFrom(exe string, resolve func(string) (string, error), goos string, env func(string) string) Channel {
	if goos != "windows" {
		return ChannelUnknown
	}
	// Scoop installs are a symlink farm (current/ → the versioned dir), so the
	// un-resolved path can hide the marker the resolved one shows — and the
	// reverse for someone who symlinked a portable copy onto PATH. Check both.
	candidates := []string{exe}
	if resolved, err := resolve(exe); err == nil && resolved != exe {
		candidates = append(candidates, resolved)
	}
	for _, p := range candidates {
		if c := classifyWindows(p, env); c != ChannelUnknown {
			return c
		}
	}
	// A Windows build that is none of the above is the portable zip: it is the
	// only remaining way to end up with a loose aetox.exe.
	return ChannelPortable
}

// classifyWindows takes its environment as a function rather than reading it
// directly, so the table test runs on all three CI platforms instead of only
// the Windows runner. What it classifies are Windows-shaped path *strings* —
// nothing here needs the path to exist, or the OS to be Windows.
func classifyWindows(exe string, env func(string) string) Channel {
	p := norm(exe)
	if strings.Contains(p, "/scoop/apps/") {
		return ChannelScoop
	}
	// Before the Program Files check below, not after: an MSIX package installs
	// to Program Files\WindowsApps, so it satisfies that check too and would be
	// classified as the NSIS installer by whichever test ran first. The more
	// specific place has to win.
	//
	// Matched on the path segment rather than an env var because Windows
	// publishes no variable for it — and the folder is ACL'd so tightly that
	// even reading its name from anywhere else is a privilege question.
	if strings.Contains(p, "/windowsapps/") {
		return ChannelStore
	}
	// Both installer scopes: the release build is machine-wide (Program Files),
	// but project.nsi also supports WAILS_INSTALL_SCOPE=user, which lands in
	// %LOCALAPPDATA%\Programs. Either one is "re-run the installer".
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		if under(p, env(key)) {
			return ChannelInstaller
		}
	}
	if local := env("LOCALAPPDATA"); local != "" {
		if under(p, local+`\Programs`) {
			return ChannelInstaller
		}
	}
	return ChannelUnknown
}

// norm lowercases and forward-slashes a Windows path. Not filepath.ToSlash:
// that is a no-op off Windows, which would leave the backslashes in place and
// silently break every comparison below on the Linux and macOS runners.
func norm(p string) string {
	return strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
}

// under reports whether exe (already normalised) sits inside dir. Prefix match
// on a trailing separator, so C:/Program FilesX never counts as inside
// C:/Program Files.
func under(exe, dir string) bool {
	if dir == "" {
		return false
	}
	prefix := norm(dir)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.HasPrefix(exe, prefix)
}

// UpgradeHint is what the user should actually do about a new release on this
// channel. Empty means "no command to give" — the UI offers the releases page.
func UpgradeHint(c Channel) string {
	if c == ChannelScoop {
		return "scoop update aetox"
	}
	return ""
}
