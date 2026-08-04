package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// The workspace is the whole answer to "which folders may this session touch",
// and it is the only answer: one value, written in one place (RegisterDefaults)
// and read in one place (resolveSandboxPath). Every file tool goes through that
// function, so there is no second gate to keep in sync and no tool that quietly
// knows better.
//
// It has exactly two ingredients, both chosen by the user and nothing else:
//
//   - Extra: folders the user added to a focused project. A problem in one
//     project often comes from another one, and the fix for "go look over
//     there" is the user naming "over there" — not the agent widening its own
//     reach, and not a rule buried in code that decides on their behalf.
//   - Open: the desktop's "ไม่โฟกัสโปรเจกต์" mode (2026-08-04), where no project
//     is focused and the machine itself is the workspace. Picking that mode IS
//     the user's choice of scope.
//
// Extra folders get exactly the rights the project root has — read and write,
// no prompt. A second, quieter tier of rights ("you may look but not touch")
// would be a rule the user never agreed to and cannot see, and it is the kind
// of thing that turns into debt the moment someone hits it. The folder list is
// the permission; if a folder should not be written to, it does not belong on
// the list.
//
// The one thing the user's choice does not reach is credentialStores below.
//
// Keyed by root rather than stored per skill because ~20 skills hold a root
// string and all of them answer through resolveSandboxPath; one map here beats
// threading state through every struct. RegisterDefaults records the policy on
// every registry build, so re-focusing a project or dropping a folder narrows
// the workspace in the same call that re-roots the engine.
type sandboxPolicy struct {
	// open lifts the wall entirely: any path on the machine except the
	// credential stores.
	open bool
	// extra are absolute, symlink-resolved folders that count as part of the
	// workspace alongside the root.
	extra []string
}

var policies sync.Map // map[string]sandboxPolicy, keyed by rootKey(abs root)

// setSandboxPolicy records what root may reach. Called on every registry build,
// zero values included: a project that just lost its extra folders has to stop
// reaching them in the same call, not on the next restart.
func setSandboxPolicy(root string, open bool, extra []string) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return
	}
	policy := sandboxPolicy{open: open, extra: resolveExtraRoots(extra)}
	if !policy.open && len(policy.extra) == 0 {
		policies.Delete(rootKey(safeRoot))
		return
	}
	policies.Store(rootKey(safeRoot), policy)
}

func sandboxPolicyFor(safeRoot string) sandboxPolicy {
	if stored, ok := policies.Load(rootKey(safeRoot)); ok {
		return stored.(sandboxPolicy)
	}
	return sandboxPolicy{}
}

// resolveExtraRoots normalizes the user's folder list once, at policy time,
// rather than on every path check: absolute, symlink-resolved (the same form
// resolveSandboxPath compares targets in), blanks dropped, duplicates dropped.
func resolveExtraRoots(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		abs, err := filepath.Abs(entry)
		if err != nil {
			continue
		}
		resolved := evalExistingSymlinks(abs)
		key := rootKey(resolved)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, resolved)
	}
	return out
}

// covers reports whether an already-resolved target sits in one of the extra
// folders.
func (p sandboxPolicy) covers(resolvedTarget string) bool {
	for _, extra := range p.extra {
		if withinRoot(resolvedTarget, extra) {
			return true
		}
	}
	return false
}

// rootKey mirrors withinRoot's case rule: NTFS is case-insensitive, so
// C:\Users\x and c:\users\x must be the same key.
func rootKey(p string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// credentialStores are home-relative paths every file tool refuses, in every
// mode, whatever the user put on the folder list. This is the exact threat that
// narrowed the unfocused root to <home>/aetox in the first place (2026-07-26,
// §19.1): web_fetch, web_search and browser_read sit in the same tool loop as
// read/grep, so a fetched page can carry an instruction in and the loop can
// carry a key out — and the desktop runs full-access, meaning no approval
// prompt would interrupt it.
//
// This is the one line that does not follow the user's folder choice, and it is
// deliberately not a preference: a user who adds ~/.ssh to a project has almost
// certainly added their home folder and not thought about what is under it, and
// "the agent read my SSH key because I dragged in a folder" is not a trade
// anyone makes on purpose. It costs nothing else — no other folder is affected,
// and the refusal says why so the model can tell the user.
//
// A denylist, not a heuristic: short, literal, and each entry is a place whose
// only interesting content is a credential.
var credentialStores = []string{
	".ssh", ".aws", ".gnupg", ".azure", ".kube",
	".netrc", ".git-credentials", ".config/gh",
	".aetox", // Aetox's own data root: model API keys live here
	"AppData/Roaming/Microsoft/Credentials",
	"AppData/Local/Microsoft/Credentials",
	"AppData/Roaming/Microsoft/Protect",
	"AppData/Local/Google", // Chrome profiles: cookies, tokens, saved logins
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
			return fmt.Errorf("path is inside a credential store (%s) and stays off-limits in every mode", sub)
		}
	}
	return nil
}

// CredentialStoreAt names the credential store a folder sits inside, or "" if
// it sits outside all of them. The host calls it when the user picks a folder
// to add, so the refusal arrives at the moment of choosing rather than later as
// a tool error on one file — a folder that is accepted into the list and then
// refuses everything in it is the app disagreeing with itself in front of the
// user.
func CredentialStoreAt(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	home = resolvedHomeDir(strings.TrimSpace(home))
	resolved := evalExistingSymlinks(abs)
	for _, sub := range credentialStores {
		if withinRoot(resolved, filepath.Join(home, filepath.FromSlash(sub))) {
			return sub
		}
	}
	return ""
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
