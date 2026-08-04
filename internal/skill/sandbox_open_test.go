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
	setSandboxOpen(root, true)
	t.Cleanup(func() { setSandboxOpen(root, false) })

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
	setSandboxOpen(root, true)
	t.Cleanup(func() { setSandboxOpen(root, false) })

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
	setSandboxOpen(root, true)
	t.Cleanup(func() { setSandboxOpen(root, false) })

	for _, path := range []string{
		filepath.Join(home, ".ssh", "id_rsa"), // absolute spelling
		"../.ssh/id_rsa",                      // climbing spelling of the same file
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
