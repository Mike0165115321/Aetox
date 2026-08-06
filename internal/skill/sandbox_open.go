package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/Mike0165115321/Aetox/internal/config"
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
	".aetox", // the skills folder; Aetox's *secrets* are handled by ownSecretFiles
	"AppData/Roaming/Microsoft/Credentials",
	"AppData/Local/Microsoft/Credentials",
	"AppData/Roaming/Microsoft/Protect",
	"AppData/Local/Google", // Chrome profiles: cookies, tokens, saved logins
	"AppData/Local/Microsoft/Edge",
	"AppData/Roaming/Mozilla",
	"AppData/Local/BraveSoftware",
}

// ownSecretFiles are the files inside Aetox's own data root that hold
// credentials, refused by name wherever that root happens to be.
//
// They need their own check because the list above is *home-relative* and the
// data root is not under the home folder on any platform: it comes from
// os.UserConfigDir (%APPDATA%\aetox on Windows, ~/.config/aetox on Linux,
// ~/Library/Application Support/aetox on macOS) and AETOX_DATA_ROOT can move it
// anywhere. The `.aetox` entry above was written believing it covered this and
// covered the skills folder instead — so the file holding the user's model API
// keys was readable by every file tool, in the mode that roams the machine, in
// a loop that also holds web_fetch and browser_read. That is the exact path in
// and out this whole denylist exists to break (2026-08-06).
//
// By file rather than by folder, deliberately. The rest of the data root —
// logs, memory, the agents' folders, the database — is Aetox explaining itself,
// and the assistant being able to read its own logs is worth keeping. Only the
// files whose interesting content is a credential are shut.
var ownSecretFiles = []string{
	"credentials.json",      // provider API keys
	"oauth.json",            // OAuth refresh tokens
	".env",                  // whatever the user put in it
	"model-preference.json", // held the keys before they were split out
	// The MCP config. It reads like plumbing — a name and a command — but its
	// `headers` and `environment` maps are where a server's API key goes: the
	// exa preset in Settings declares `x-api-key` outright, and a stdio server
	// takes its token through the environment. Left readable at first on the
	// reasoning that the agent should be able to debug its own MCP setup, which
	// was a judgement made by looking at one server that happened to have no
	// key in it. `${env:...}` (bootstrap.MCPServers) is the real fix — a
	// reference instead of a secret — and this is the belt for the users who
	// paste the key in anyway.
	"mcp-servers.json",
	// The in-app browser's profile: cookies, tokens and saved logins for
	// every site the user signed into through it. The list above already
	// refuses Chrome's, Edge's, Brave's and Firefox's profiles for exactly
	// this content — refusing four other browsers' while leaving our own open
	// is the same mistake the `.aetox` entry made, one directory over. A
	// folder, not a file; withinRoot covers both.
	"webview",
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
	if err := refuseOwnSecrets(target); err != nil {
		return err
	}
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

// refuseOwnSecrets rejects Aetox's own credential files. The data root goes
// through resolvedHomeDir for the same reason the home folder does: the target
// arrives symlink-resolved and 8.3-expanded, and comparing those raw lets a
// short-named path through.
func refuseOwnSecrets(target string) error {
	root, err := config.DataRoot()
	if err != nil || strings.TrimSpace(root) == "" {
		return nil
	}
	root = resolvedHomeDir(strings.TrimSpace(root))
	for _, name := range ownSecretFiles {
		if withinRoot(target, filepath.Join(root, name)) {
			return fmt.Errorf(
				"%s holds Aetox's own credentials and stays off-limits in every mode. %s",
				name, ownSecretHint(name))
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

// ownSecretHint says what to do instead of reading the file.
//
// A refusal that only says "no" gets relayed to the user as a limitation of the
// product. That happened the day these files were shut: asked whether MCP was
// connected, the assistant tried to read mcp-servers.json, was refused, and
// reported that it could not reach its own MCP configuration — while holding
// that server's tools in its tool block. The wall was right; the silence next
// to it was not.
//
// Each hint names where the answer really is, so the model can finish the job
// rather than describe the obstacle.
func ownSecretHint(name string) string {
	switch name {
	case "mcp-servers.json":
		return "You do not need it to answer questions about MCP: every tool bridged from a server " +
			"is already in your tool list and says which server it came from. To change which desks " +
			"and agents a server is switched on for, the user does that in Settings → MCP servers."
	case "credentials.json", "model-preference.json":
		return "The user manages providers and API keys in Settings."
	case "oauth.json":
		return "The user signs in and out from Settings."
	case ".env":
		return "The user edits this file themselves; you can tell them the path."
	case "webview":
		return "This is the in-app browser's profile — cookies and saved logins for sites the user " +
			"signed into. Use the browser tools to visit a page instead of reading the profile."
	}
	return "The user manages this from Settings."
}
