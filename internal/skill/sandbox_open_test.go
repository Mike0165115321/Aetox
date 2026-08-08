package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Open-sandbox mode (unfocused desktop): the machine is the workspace, so an
// absolute path — the thing the closed sandbox exists to reject — must work.
func TestOpenSandboxAcceptsAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	setSandboxPolicy(root, true, nil)
	t.Cleanup(func() { setSandboxPolicy(root, false, nil) })

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "stray.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSandboxPath(root, filepath.Join(outside, "stray.pdf"))
	if err != nil {
		t.Fatalf("open sandbox rejected an absolute path: %v", err)
	}
	if _, statErr := os.Stat(got); statErr != nil {
		t.Fatalf("resolved path %q does not point at the file: %v", got, statErr)
	}
}

// A relative path that climbs out gets the same treatment as an absolute one —
// otherwise "../Documents/x" and its absolute spelling would disagree.
func TestOpenSandboxAcceptsClimbingRelativePaths(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "aetox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "sibling.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	setSandboxPolicy(root, true, nil)
	t.Cleanup(func() { setSandboxPolicy(root, false, nil) })

	if _, err := resolveSandboxPath(root, "../sibling.txt"); err != nil {
		t.Fatalf("open sandbox rejected a climbing relative path: %v", err)
	}
}

// The closed sandbox — every focused project, and the whole CLI — must keep
// rejecting exactly what it always rejected. Open mode is opt-in per root.
func TestClosedSandboxStillRejectsAbsoluteAndClimbingPaths(t *testing.T) {
	root := t.TempDir()
	if _, err := resolveSandboxPath(root, t.TempDir()); err == nil {
		t.Fatal("closed sandbox accepted an absolute path")
	}
	if _, err := resolveSandboxPath(root, "../elsewhere"); err == nil {
		t.Fatal("closed sandbox accepted a climbing relative path")
	}
}

// Re-focusing a project after an unfocused session must close the wall again:
// RegisterDefaults records the mode on every build, false included.
func TestRegisterDefaultsClosesAPreviouslyOpenRoot(t *testing.T) {
	root := t.TempDir()
	NewDefaultRegistry(RegistryOptions{SandboxRoot: root, OpenSandbox: true})
	if _, err := resolveSandboxPath(root, t.TempDir()); err != nil {
		t.Fatalf("open registry root rejected an absolute path: %v", err)
	}
	NewDefaultRegistry(RegistryOptions{SandboxRoot: root, OpenSandbox: false})
	if _, err := resolveSandboxPath(root, t.TempDir()); err == nil {
		t.Fatal("root stayed open after a closed re-registration")
	}
}

// A folder the user added to a focused project is part of the workspace: the
// point of the feature is that a bug whose cause lives in another project can
// actually be looked at, so both spellings of that folder have to work while
// everything the user did NOT add stays refused.
func TestExtraRootIsReachableAndNothingElseIs(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	other := filepath.Join(base, "shared-lib")
	stranger := filepath.Join(base, "unrelated")
	for _, dir := range []string{root, other, stranger} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(other, "api.go"), []byte("package api"), 0o644); err != nil {
		t.Fatal(err)
	}
	setSandboxPolicy(root, false, []string{other})
	t.Cleanup(func() { setSandboxPolicy(root, false, nil) })

	for _, path := range []string{
		filepath.Join(other, "api.go"), // absolute spelling
		"../shared-lib/api.go",         // climbing spelling of the same file
		filepath.Join(other, "new.go"), // a file that does not exist yet: writes count too
	} {
		if _, err := resolveSandboxPath(root, path); err != nil {
			t.Errorf("added folder refused %s: %v", path, err)
		}
	}

	// Adding one folder widens the workspace by exactly that folder. A sibling
	// sitting right next to it is still outside.
	if _, err := resolveSandboxPath(root, filepath.Join(stranger, "secret.txt")); err == nil {
		t.Error("a folder the user never added was reachable")
	}
	if _, err := resolveSandboxPath(root, "../unrelated/secret.txt"); err == nil {
		t.Error("a folder the user never added was reachable by climbing")
	}
}

// Dropping a folder has to take effect in the same reload that dropped it —
// the registry is rebuilt on every focus change, and a stale entry would mean
// the UI says one thing while the tools do another.
func TestRegisterDefaultsDropsRemovedExtraRoots(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	other := filepath.Join(base, "shared-lib")
	for _, dir := range []string{root, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	probe := filepath.Join(other, "api.go")

	NewDefaultRegistry(RegistryOptions{SandboxRoot: root, ExtraRoots: []string{other}})
	if _, err := resolveSandboxPath(root, probe); err != nil {
		t.Fatalf("registry with an extra root refused it: %v", err)
	}
	NewDefaultRegistry(RegistryOptions{SandboxRoot: root})
	if _, err := resolveSandboxPath(root, probe); err == nil {
		t.Error("folder stayed reachable after it was removed from the list")
	}
}

// The credential cabinet does not answer to the folder list. Someone who adds
// their home folder to a project has not decided to hand over their SSH key,
// and the refusal has to hold for the added folder exactly as it does for the
// unfocused machine.
func TestExtraRootStillRefusesCredentialStores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// The whole home folder added as one entry — the realistic way this happens.
	setSandboxPolicy(root, false, []string{home})
	t.Cleanup(func() { setSandboxPolicy(root, false, nil) })

	if _, err := resolveSandboxPath(root, filepath.Join(home, ".ssh", "id_rsa")); err == nil {
		t.Error("an added folder handed out a credential store path")
	} else if !strings.Contains(err.Error(), "credential store") {
		t.Errorf("wrong refusal — the model should learn WHY: %v", err)
	}
	if _, err := resolveSandboxPath(root, filepath.Join(home, "notes.txt")); err != nil {
		t.Errorf("the rest of the added folder should stay reachable: %v", err)
	}
}

// The credential stores are the one thing an open sandbox still refuses: the
// 2026-07-26 narrowing existed because a fetched page can order a read of
// .ssh/.aws and the full-access loop would carry it out promptless. Opening
// the machine back up must not reopen those.
func TestOpenSandboxRefusesCredentialStores(t *testing.T) {
	home := t.TempDir()
	// os.UserHomeDir reads USERPROFILE on Windows and HOME everywhere else;
	// setting both pins the fake home on any platform.
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(home, "aetox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	setSandboxPolicy(root, true, nil)
	t.Cleanup(func() { setSandboxPolicy(root, false, nil) })

	for _, path := range []string{
		filepath.Join(home, ".ssh", "id_rsa"),                  // absolute spelling
		"../.ssh/id_rsa",                                       // climbing spelling of the same file
		filepath.Join(home, ".aetox", "model-preference.json"), // Aetox's own keys
	} {
		if _, err := resolveSandboxPath(root, path); err == nil {
			t.Errorf("open sandbox handed out a credential store path: %s", path)
		} else if !strings.Contains(err.Error(), "credential store") {
			t.Errorf("wrong refusal for %s — the model should learn WHY: %v", path, err)
		}
	}

	// The rest of the fake home stays reachable — the denylist is a cabinet
	// lock, not a second wall.
	if err := os.WriteFile(filepath.Join(home, "notes.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSandboxPath(root, filepath.Join(home, "notes.txt")); err != nil {
		t.Errorf("open sandbox refused an ordinary home file: %v", err)
	}
}

// The denylist used to name `.aetox` and believe it covered Aetox's own data
// root. It did not — on every platform the data root comes from
// os.UserConfigDir (%APPDATA%\aetox, ~/.config/aetox, ~/Library/Application
// Support/aetox) and `.aetox` is the skills folder. So the file holding the
// user's model API keys was readable by every file tool, in the mode that roams
// the machine, in a loop that also holds web_fetch and browser_read: a page
// could carry an instruction in and the loop could carry a key out, with no
// approval prompt on the desktop to interrupt it (found 2026-08-06).
// The data root is spelled the way the gate is promised it: symlink-resolved,
// which on Windows also expands 8.3 short names. resolveSandboxPath does that
// to every target before handing it over (list.go), so a test that skips the
// step is testing the gate through a door production does not use — and it
// passed anyway on any machine whose TEMP has no short name, which is every
// developer machine and not the CI runner, whose TEMP really is spelled
// C:\Users\RUNNER~1\.... That is the same mismatch a792bf6 fixed on the home
// side; this is the data-root side of it, and it kept two releases from
// building.
func TestAetoxOwnCredentialFilesAreRefused(t *testing.T) {
	root := evalExistingSymlinks(t.TempDir())
	t.Setenv("AETOX_DATA_ROOT", root)

	for _, name := range []string{"credentials.json", "oauth.json", ".env", "model-preference.json", "mcp-servers.json"} {
		if err := refuseCredentialStore(filepath.Join(root, name)); err == nil {
			t.Errorf("%s is readable — it holds credentials and must be refused in every mode", name)
		}
	}

	// The in-app browser's profile holds cookies and saved logins for every
	// site signed into through it. Chrome's, Edge's, Brave's and Firefox's are
	// already refused for exactly that; ours was not.
	if err := refuseCredentialStore(filepath.Join(root, "webview", "browser", "Default", "Cookies")); err == nil {
		t.Error("the in-app browser's profile is readable — four other browsers' are refused for the same content")
	}

	// By file, not by folder: the rest of the data root is Aetox explaining
	// itself, and being able to read its own logs and memory is worth keeping.
	for _, keep := range []string{
		filepath.Join(root, "logs", "aetox-20260806.log"),
		filepath.Join(root, "memory", "MEMORY.md"),
		filepath.Join(root, "agents", "doc", "AGENT.md"),
		filepath.Join(root, "aetox.db"),
	} {
		if err := refuseCredentialStore(keep); err != nil {
			t.Errorf("%s was refused: %v — only the credential files should be shut", keep, err)
		}
	}
}

// A refusal that only says "no" gets relayed to the user as a limitation of the
// product. Measured 2026-08-06: asked whether MCP was connected, the assistant
// tried to read mcp-servers.json, was refused, and told the user it could not
// reach its own MCP configuration — while holding that server's tools. The wall
// was right; the silence next to it was not.
func TestARefusedCredentialFileSaysWhereTheAnswerIs(t *testing.T) {
	// Resolved for the same reason as the test above.
	root := evalExistingSymlinks(t.TempDir())
	t.Setenv("AETOX_DATA_ROOT", root)

	err := refuseCredentialStore(filepath.Join(root, "mcp-servers.json"))
	if err == nil {
		t.Fatal("mcp-servers.json must stay refused")
	}
	// It must point at what the model can actually do instead, or the model has
	// nothing to offer but the obstacle.
	for _, want := range []string{"tool list", "Settings"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q, so it reads as a dead end:\n%v", want, err)
		}
	}

	// Every refused path says something actionable, not just this one.
	for _, name := range []string{"credentials.json", "oauth.json", ".env", "model-preference.json", "webview"} {
		err := refuseCredentialStore(filepath.Join(root, name))
		if err == nil {
			t.Errorf("%s must stay refused", name)
			continue
		}
		if !strings.Contains(err.Error(), "Settings") && !strings.Contains(err.Error(), "browser tools") && !strings.Contains(err.Error(), "path") {
			t.Errorf("%s refuses without saying what to do instead:\n%v", name, err)
		}
	}
}

// A worker's skills folder is its specialist knowledge, and the reason it sits
// in that worker's own folder is that the others must not have it. Before this
// the separation held only because nothing pointed across — the folders are
// inside the data root, which is readable on purpose.
func TestAnAgentsKnowledgeIsNotReachableThroughFileTools(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)

	skills := filepath.Join(dataRoot, "agents", "doc", "skills", "tax-invoice", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skills), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(skills, []byte("secret craft"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := refuseCredentialStore(skills); err == nil {
		t.Error("another worker's skill file was reachable")
	}
	if err := refuseCredentialStore(filepath.Dir(skills)); err == nil {
		t.Error("another worker's skills folder was reachable")
	}

	// The rest of a home stays open: the assistant explaining its own team, and
	// writing a new AGENT.md when the user asks for a teammate, both depend on
	// it and are things this product is built to do.
	for _, open := range []string{
		filepath.Join(dataRoot, "agents", "doc", "AGENT.md"),
		filepath.Join(dataRoot, "agents", "doc", "MEMORY.md"),
		filepath.Join(dataRoot, "agents", "doc"),
		filepath.Join(dataRoot, "agents"),
	} {
		if err := refuseCredentialStore(open); err != nil {
			t.Errorf("%s should stay readable: %v", filepath.Base(open), err)
		}
	}

	// A folder called "skills" that is not inside an agent's home is somebody
	// else's project and none of this rule's business.
	elsewhere := filepath.Join(t.TempDir(), "skills", "thing.md")
	if err := refuseCredentialStore(elsewhere); err != nil {
		t.Errorf("an unrelated skills folder was refused: %v", err)
	}
}

// The guard has to compare like with like.
//
// It resolved the agents root through evalExistingSymlinks and left the target
// raw, so two spellings of one path read as two different places: on a Windows
// CI runner the temp dir arrives as `RUNNER~1` while the resolved root is the
// long form, filepath.Rel answers `..\..\…`, and the refusal never fires. This
// stands the two forms up against each other on purpose.
func TestAgentKnowledgeIsRefusedThroughASymlinkedDataRoot(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	// The data root is named one way; the tool is handed the other.
	t.Setenv("AETOX_DATA_ROOT", link)

	skills := filepath.Join(real, "agents", "doc", "skills", "tax-invoice", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skills), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(skills, []byte("secret craft"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, target := range []string{
		skills,
		filepath.Join(link, "agents", "doc", "skills", "tax-invoice", "SKILL.md"),
	} {
		if err := refuseCredentialStore(target); err == nil {
			t.Errorf("reachable through %s — the two spellings were read as two places", target)
		}
	}
}
