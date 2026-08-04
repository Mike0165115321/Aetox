package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Open-sandbox mode (2026-08-04): the desktop's "ไม่โฟกัสโปรเจกต์" runs the
// engine rooted at <home>/aetox, and until now every file tool was walled into
// that folder — which is why "find a PDF on this machine" was answered with
// "I can't". The user's call: unfocused mode means the machine IS the
// workspace, so the wall opens; what keeps it defensible is that new files
// still land in the session output folder (outputSubdir, unchanged) and the
// credential stores below stay refused. A focused project keeps the closed
// sandbox exactly as before — that guarantee is load-bearing.
//
// Openness is keyed by root rather than stored per skill because ~20 skills
// hold a root string and all of them answer through resolveSandboxPath; one
// map here beats threading a flag through every struct. RegisterDefaults
// records the mode on every registry build, so re-focusing a project closes
// the root again in the same call that re-roots the engine.
var openRoots sync.Map // map[string]bool, keyed by rootKey(abs root)

func setSandboxOpen(root string, open bool) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return
	}
	if open {
		openRoots.Store(rootKey(safeRoot), true)
		return
	}
	openRoots.Delete(rootKey(safeRoot))
}

func sandboxIsOpen(safeRoot string) bool {
	_, ok := openRoots.Load(rootKey(safeRoot))
	return ok
}

// rootKey mirrors withinRoot's case rule: NTFS is case-insensitive, so
// C:\Users\x and c:\users\x must be the same key.
func rootKey(p string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// credentialStores are home-relative paths every file tool refuses even when
// the sandbox is open. This is the exact threat that narrowed the unfocused
// root to <home>/aetox in the first place (2026-07-26, §19.1): web_fetch,
// web_search and browser_read sit in the same tool loop as read/grep, so a
// fetched page can carry an instruction in and the loop can carry a key out —
// and unfocused runs full-access, meaning no approval prompt would interrupt
// it. Opening the sandbox back up is the user's call; reopening the key
// cabinets is not part of it.
//
// A denylist, not a heuristic: short, literal, and each entry is a place
// whose only interesting content is a credential.
var credentialStores = []string{
	".ssh", ".aws", ".gnupg", ".azure", ".kube",
	".netrc", ".git-credentials", ".config/gh",
	".aetox", // Aetox's own data root: model API keys live here
	"AppData/Roaming/Microsoft/Credentials",
	"AppData/Local/Microsoft/Credentials",
	"AppData/Roaming/Microsoft/Protect",
	"AppData/Local/Google",        // Chrome profiles: cookies, tokens, saved logins
	"AppData/Local/Microsoft/Edge",
	"AppData/Roaming/Mozilla",
	"AppData/Local/BraveSoftware",
}

// refuseCredentialStore rejects a (symlink-resolved) target inside any
// credential store. A missing home dir fails open on purpose: it means there
// is no home to protect, not that everything is suspect.
//
// The home side goes through the same canonicalization as the target, because
// the two arrive in different spellings: the target was symlink-resolved,
// which on Windows also expands 8.3 short names (RUNNER~1 → runneradmin),
// while os.UserHomeDir returns USERPROFILE verbatim. Compare those raw and a
// short-named home lets every credential read through — caught by CI, whose
// runner's TEMP really is spelled C:\Users\RUNNER~1\....
func refuseCredentialStore(target string) error {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	home = resolvedHomeDir(strings.TrimSpace(home))
	for _, sub := range credentialStores {
		if withinRoot(target, filepath.Join(home, filepath.FromSlash(sub))) {
			return fmt.Errorf("path is inside a credential store (%s) and stays off-limits even without a project", sub)
		}
	}
	return nil
}

// resolvedHomeDir caches evalExistingSymlinks per raw home value — the same
// trade rootResolutions makes, and safe the same way: keyed by the raw value,
// so a test re-pointing USERPROFILE gets a fresh entry, not a stale answer.
var homeResolutions sync.Map // map[string]string

func resolvedHomeDir(raw string) string {
	if cached, ok := homeResolutions.Load(raw); ok {
		return cached.(string)
	}
	resolved := evalExistingSymlinks(raw)
	homeResolutions.Store(raw, resolved)
	return resolved
}
