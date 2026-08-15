package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// widenProject is a focused project with a folder next door that is not in the
// workspace — the shape every case below asks the door about.
func widenProject(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root, outside = filepath.Join(base, "project"), filepath.Join(base, "shared-lib")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "api.go"), []byte("package api"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

// The door's whole purpose: a refused path becomes a question, and a yes that
// really does put the folder on the list lets the SAME call through — no retry,
// no new turn, no trip through a menu.
func TestWidenLetsTheParkedCallThrough(t *testing.T) {
	root, outside := widenProject(t)
	probe := filepath.Join(outside, "api.go")

	asked := 0
	setSandboxPolicy(root, false, nil, func(target string) bool {
		asked++
		// What a host does: put the folder on the list, then answer.
		SetWorkspaceFolders(root, []string{filepath.Dir(target)})
		return true
	})
	t.Cleanup(func() { setSandboxPolicy(root, false, nil, nil) })

	if _, err := resolveSandboxPath(root, probe); err != nil {
		t.Fatalf("the path was still refused after the folder was added: %v", err)
	}
	if asked != 1 {
		t.Errorf("asked %d times, want 1", asked)
	}

	// And the folder is on the list now, so the next path in the same folder is
	// not a second question. A command naming three files must not raise three
	// cards.
	if _, err := resolveSandboxPath(root, filepath.Join(outside, "other.go")); err != nil {
		t.Fatalf("a sibling in the added folder was refused: %v", err)
	}
	if asked != 1 {
		t.Errorf("asked %d times after the folder was on the list, want 1 — the door re-asked for a folder already granted", asked)
	}
}

// The answer is a hint; the folder list is the permission. A host that says yes
// without widening anything must not get a pass — otherwise the door stops
// leading through the list and becomes a way around it, which is the one thing
// it must never be.
func TestWidenIgnoresAYesThatGrantsNothing(t *testing.T) {
	root, outside := widenProject(t)

	setSandboxPolicy(root, false, nil, func(string) bool { return true })
	t.Cleanup(func() { setSandboxPolicy(root, false, nil, nil) })

	if _, err := resolveSandboxPath(root, filepath.Join(outside, "api.go")); err == nil {
		t.Fatal("a bare yes opened the wall with nothing on the folder list")
	}
}

// A no leaves the wall exactly where it was, and says the same thing it always
// said — the model has to be able to report a refusal, not a new kind of error.
func TestWidenRefusalKeepsTheOriginalWording(t *testing.T) {
	root, outside := widenProject(t)

	setSandboxPolicy(root, false, nil, func(string) bool { return false })
	t.Cleanup(func() { setSandboxPolicy(root, false, nil, nil) })

	_, err := resolveSandboxPath(root, filepath.Join(outside, "api.go"))
	if err == nil {
		t.Fatal("a refused card still let the path through")
	}
	if !strings.Contains(err.Error(), "outside the folders this session can use") {
		t.Errorf("refusal reads differently after a no: %v", err)
	}
}

// The credential stores are refused after the door, on the resolved path, so a
// yes cannot reach one. This is the ordering opencode uses to keep a saved
// "always allow" from overriding a configured deny, and it is why the door can
// be as generous as it is.
func TestWidenCannotOpenACredentialStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	// A host that asks nothing and grants everything — the worst case the
	// ordering has to survive.
	setSandboxPolicy(root, false, nil, func(target string) bool {
		SetWorkspaceFolders(root, []string{filepath.Dir(target)})
		return true
	})
	t.Cleanup(func() { setSandboxPolicy(root, false, nil, nil) })

	_, err := resolveSandboxPath(root, filepath.Join(ssh, "id_rsa"))
	if err == nil {
		t.Fatal("the door opened a credential store")
	}
	if !strings.Contains(err.Error(), "credential store") {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}

// No door is the ordinary configuration — the CLI, every test, every headless
// host. Nothing about the refusal may change for them.
func TestNoDoorMeansTheFlatRefusal(t *testing.T) {
	root, outside := widenProject(t)

	NewDefaultRegistry(RegistryOptions{SandboxRoot: root})
	if _, err := resolveSandboxPath(root, filepath.Join(outside, "api.go")); err == nil {
		t.Fatal("a registry with no AskWorkspace let an outside path through")
	}
}

// The door is never consulted for a path already inside the workspace. It costs
// a user-facing card, so a session that asks about paths it can already reach is
// a session nobody will read the cards of.
func TestWidenIsNotAskedAboutPathsAlreadyInside(t *testing.T) {
	root, outside := widenProject(t)

	asked := 0
	setSandboxPolicy(root, false, []string{outside}, func(string) bool {
		asked++
		return false
	})
	t.Cleanup(func() { setSandboxPolicy(root, false, nil, nil) })

	for _, path := range []string{
		filepath.Join(root, "main.go"),   // the project itself
		".",                              // the root, by the tautology shortcut
		filepath.Join(outside, "api.go"), // an added folder
		"../shared-lib/api.go",           // the same file, climbing
	} {
		if _, err := resolveSandboxPath(root, path); err != nil {
			t.Errorf("%s refused: %v", path, err)
		}
	}
	if asked != 0 {
		t.Errorf("the door was opened %d times for paths already in the workspace, want 0", asked)
	}
}

// Open mode has no wall, so it has no door: the whole machine is the workspace
// and there is nothing to ask about.
func TestWidenIsNotAskedInOpenMode(t *testing.T) {
	root, outside := widenProject(t)

	asked := 0
	setSandboxPolicy(root, true, nil, func(string) bool {
		asked++
		return false
	})
	t.Cleanup(func() { setSandboxPolicy(root, false, nil, nil) })

	if _, err := resolveSandboxPath(root, filepath.Join(outside, "api.go")); err != nil {
		t.Fatalf("open mode refused an outside path: %v", err)
	}
	if asked != 0 {
		t.Errorf("the door was opened %d times in open mode, want 0", asked)
	}
}

// SetWorkspaceFolders is the mid-turn writer, and it must touch only the folder
// list: a session's mode and its door are decided when the engine is built, and
// nothing happening inside a tool call may change either.
func TestSetWorkspaceFoldersLeavesTheRestOfThePolicyAlone(t *testing.T) {
	root, outside := widenProject(t)

	asked := 0
	setSandboxPolicy(root, true, nil, func(string) bool { asked++; return false })
	t.Cleanup(func() { setSandboxPolicy(root, false, nil, nil) })

	SetWorkspaceFolders(root, []string{outside})

	policy := sandboxPolicyFor(mustAbs(root))
	if !policy.open {
		t.Error("open was cleared by a folder-list write")
	}
	if policy.ask == nil {
		t.Error("the door was dropped by a folder-list write")
	}
	if !policy.covers(evalExistingSymlinks(mustAbs(outside))) {
		t.Error("the folder that was just written is not covered")
	}
}

// The shell answers through the same gate, so the door has to be there too —
// otherwise adding a folder would fix read and grep and leave the agent unable
// to build or test the thing it was let in to look at.
func TestShellReachesTheSameDoor(t *testing.T) {
	root, outside := widenProject(t)
	command := "go build " + filepath.Join(outside, "main.go")

	asked := 0
	setSandboxPolicy(root, false, nil, func(target string) bool {
		asked++
		SetWorkspaceFolders(root, []string{filepath.Dir(target)})
		return true
	})
	t.Cleanup(func() { setSandboxPolicy(root, false, nil, nil) })

	if err := guardCommandPaths(root, command, nativeGate()); err != nil {
		t.Fatalf("shell refused after the folder was added: %v", err)
	}
	if asked != 1 {
		t.Errorf("shell asked %d times, want 1", asked)
	}
}
