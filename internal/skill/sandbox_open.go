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
// and it is the only answer: one value, read in one place (resolveSandboxPath).
// Every file tool goes through that function, so there is no second gate to keep
// in sync and no tool that quietly knows better.
//
// It is written by RegisterDefaults on every registry build, plus one narrow
// second writer — SetWorkspaceFolders, which updates the folder list of a
// session already running and touches nothing else. Two writers is worth
// stating plainly: the alternative was rebuilding the engine mid-turn to record
// a folder the user just approved, which throws away the turn they are waiting
// on. Both writers take the list from the same place (the host's stored list),
// so they cannot disagree about it.
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
	// ask is the door in the wall: the host's offer to put the folder a refused
	// path lives in onto the list, right there, instead of ending the work. See
	// WidenFunc.
	ask WidenFunc
}

// WidenFunc asks the user whether to add the folder holding target to this
// project, and reports whether they said yes.
//
// It exists because the wall had no door. A path outside the workspace was a
// dead end: the agent knew the folder it needed, and the user had to go find it
// again by hand — open the project menu, click "เพิ่มโฟลเดอร์", walk an OS
// dialog to a path already written in the transcript — and then ask for the same
// thing a second time. The app knew the answer and made the person fetch it.
// Claude Code and opencode both work across projects as ordinary daily work; a
// refusal with no way forward is not us being safer than them, it is us being
// narrower at the thing they are useful at.
//
// Two rules make this a door rather than a hole, and both live at the call site
// in resolveSandboxPath rather than here:
//
//   - The answer is a hint, never a permission. Whatever this returns, the
//     verdict is re-read from the policy afterwards — the workspace is what the
//     folder list says it is, and a host that answers true without widening
//     anything gets the refusal it would have got anyway.
//   - The credential stores are unaffected. They are refused after this point,
//     on the same resolved path, so a yes here cannot reach one — the same
//     ordering opencode uses to keep a saved "always allow" from overriding a
//     configured deny.
//
// Nil is the ordinary value: the CLI, every test and every host that draws no
// UI leave it unset and get the flat refusal, unchanged.
type WidenFunc func(target string) bool

var policies sync.Map // map[string]sandboxPolicy, keyed by rootKey(abs root)

// setSandboxPolicy records what root may reach. Called on every registry build,
// zero values included: a project that just lost its extra folders has to stop
// reaching them in the same call, not on the next restart.
func setSandboxPolicy(root string, open bool, extra []string, ask WidenFunc) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return
	}
	policy := sandboxPolicy{open: open, extra: resolveExtraRoots(extra), ask: ask}
	if !policy.open && len(policy.extra) == 0 && policy.ask == nil {
		policies.Delete(rootKey(safeRoot))
		return
	}
	policies.Store(rootKey(safeRoot), policy)
}

// SetWorkspaceFolders replaces the folder list for a root that is already
// running, leaving the rest of its policy alone.
//
// This is the one write that does not come from a registry build, and it is
// narrow on purpose. A folder accepted through the door above is accepted in the
// middle of a tool call, on an engine that is mid-turn; rebuilding that engine
// to record one folder would throw away the turn the user is waiting on. The
// folder reaches the running call through here, and the engine picks the same
// list up at the end of the turn through the ordinary path — the host holds the
// list either way, so the two never disagree about what it contains.
//
// `open` is still written only by RegisterDefaults: which mode a session runs in
// is decided when the session is built, and nothing mid-turn may change it.
func SetWorkspaceFolders(root string, extra []string) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return
	}
	current := sandboxPolicyFor(safeRoot)
	setSandboxPolicy(root, current.open, extra, current.ask)
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

// refuseCredentialStore rejects a target inside any credential store, resolving
// it first. Callers that already hold a resolved path call refuseResolved
// directly rather than paying for the walk twice.
func refuseCredentialStore(target string) error {
	return refuseResolved(evalExistingSymlinks(target))
}

// refuseResolved is the denylist itself, and the only copy of it: one gate, two
// rhythms of caller. resolveSandboxPath resolves a single path per tool call and
// hands it here; a walking tool resolves its base once and derives every entry
// from it (sandboxWalk). Both arrive already symlink-resolved, so nothing below
// resolves again — a per-entry EvalSymlinks is the exact cost that kept this
// check out of the walkers in the first place, and it is what let a walk started
// above a refused file read it (2026-08-16).
//
// A missing home dir fails open on purpose: it means there is no home to
// protect, not that everything is suspect.
//
// The home side goes through the same canonicalization as the target, because
// the two arrive in different spellings: the target was symlink-resolved,
// which on Windows also expands 8.3 short names (RUNNER~1 → runneradmin),
// while os.UserHomeDir returns USERPROFILE verbatim. Compare those raw and a
// short-named home lets every credential read through — caught by CI, whose
// runner's TEMP really is spelled C:\Users\RUNNER~1\....
func refuseResolved(target string) error {
	if err := refuseOwnSecrets(target); err != nil {
		return err
	}
	if err := refuseAgentKnowledge(target); err != nil {
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

// refuseAgentKnowledge shuts <DataRoot>/agents/<name>/skills to every file
// tool, in every mode.
//
// A worker's skills folder is its specialist knowledge — the required fields of
// a tax invoice, the columns of a payroll workbook — and the whole reason it
// sits in that worker's own folder rather than on the shared shelf is that the
// others must not have it. Without this the separation held only because
// nothing pointed across: the folders are inside the data root, which is
// readable on purpose, so a worker that guessed a sibling's path was in.
//
// The door is not closed, it is moved: a worker's own skills reach it as tools
// in its own registry (internal/subagent/skills.go), which is exactly how
// ~/.aetox/skills is reachable through `skills_list` and `skill_view` while
// being refused to `read`. Knowledge travels through the skill door or not at
// all.
//
// Only `skills`. AGENT.md and MEMORY.md stay readable, because the assistant
// explaining its own team — and writing a new AGENT.md when the user asks for a
// teammate — are things this product is built to do.
func refuseAgentKnowledge(target string) error {
	root, err := config.AgentsRoot()
	if err != nil || strings.TrimSpace(root) == "" {
		return nil
	}
	// Both sides in the same spelling, or the comparison is between two spellings
	// of one place and quietly decides they are different places.
	//
	// Only the root was resolved once, which held on a developer's machine and
	// failed on a Windows CI runner, where the temp path arrives in its 8.3 short
	// form (`RUNNER~1`) while the resolved root is the long one. filepath.Rel then
	// returns `..\..\…`, the guard reads that as "outside the agents root", and
	// a worker's private knowledge is readable. The test caught it before the
	// release did; the same mismatch is reachable off CI wherever the data root
	// is behind a symlink or a junction. The target's half of that resolution now
	// happens once at the door (refuseCredentialStore / sandboxWalk) instead of
	// here, so a walk does not pay for it per file.
	rel, err := filepath.Rel(resolvedHomeDir(strings.TrimSpace(root)), target)
	if err != nil {
		return nil // a different volume: not under the agents root at all
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 || parts[0] == ".." || parts[0] == "." {
		return nil
	}
	if !strings.EqualFold(parts[1], config.AgentSkillsDir) {
		return nil
	}
	return fmt.Errorf(
		"%s is %s's own knowledge and is not reachable through file tools — an agent's skills are handed to that agent as tools, and to nobody else",
		filepath.ToSlash(rel), parts[0])
}

// sandboxWalk is the denylist for a tool that walks instead of naming a path.
//
// grep, glob and `fs find` resolve one base path through resolveSandboxPath and
// then hand every file under it to os.Open. That check answered "may this
// session reach the base", and nothing asked the question again per entry — so a
// walk started at a folder the session legitimately holds read straight through
// the refusals underneath it. With no project focused the workspace is the
// machine, which put credentials.json and every worker's private skills folder
// one `grep` away from a session that was refused both by name (2026-08-16).
//
// The reason it was not simply wired to refuseCredentialStore is cost: that door
// resolves symlinks, ~1ms a call on Windows, and a walk visits thousands of
// entries. So the resolution happens once here, on the base, and every entry is
// placed under it by string arithmetic. Same list, same verdicts, no syscalls.
//
// A refused entry is skipped in silence. These paths are not part of the
// workspace the question was asked about, and a walk that annotated every
// credential store it declined to read would be publishing their locations into
// a transcript to no purpose. Skipping a directory prunes it whole.
type sandboxWalk struct {
	base     string
	resolved string
}

func newSandboxWalk(basePath string) sandboxWalk {
	return sandboxWalk{base: basePath, resolved: evalExistingSymlinks(basePath)}
}

// refuses reports whether the walk must leave path alone. A path it cannot place
// under the base is refused rather than allowed: not being able to say where
// something is is not a reason to read it.
func (w sandboxWalk) refuses(path string) bool {
	rel, err := filepath.Rel(w.base, path)
	if err != nil {
		return true
	}
	return refuseResolved(filepath.Join(w.resolved, rel)) != nil
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
