//go:build !windows

package update

import "errors"

// Self-update ships with the Windows build only, because Windows is the only
// platform Aetox ships on (release.yml). When the Linux/macOS port lands,
// these grow real bodies — the portable rename trick works as-is on both; the
// macOS bundle needs the Sparkle-shaped care channel.go describes.

func relaunchAfterExit(string) error {
	return errors.New("self-update is Windows-only for now")
}

func handOffToInstaller(string) error {
	return errors.New("self-update is Windows-only for now")
}
