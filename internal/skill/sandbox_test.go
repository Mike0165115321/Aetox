package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSandboxPath(t *testing.T) {
	root := t.TempDir()

	got, err := resolveSandboxPath(root, "sub/file.txt")
	if err != nil {
		t.Fatalf("relative path under root: unexpected error: %v", err)
	}
	want := filepath.Join(root, "sub", "file.txt")
	if got != want {
		t.Errorf("resolveSandboxPath(sub/file.txt) = %q, want %q", got, want)
	}

	if got, err := resolveSandboxPath(root, ""); err != nil || got != root {
		t.Errorf("resolveSandboxPath(\"\") = %q, %v, want root %q, nil", got, err, root)
	}
	if got, err := resolveSandboxPath(root, "."); err != nil || got != root {
		t.Errorf("resolveSandboxPath(.) = %q, %v, want root %q, nil", got, err, root)
	}
}

func TestResolveSandboxPathRejectsEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sandbox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cases := []string{
		"../outside.txt",
		"../../etc/passwd",
		"sub/../../escape.txt",
	}
	for _, c := range cases {
		if _, err := resolveSandboxPath(root, c); err == nil {
			t.Errorf("resolveSandboxPath(%q): expected error escaping sandbox, got nil", c)
		}
	}
}

// A symlink inside the root pointing outside it passes a purely lexical
// prefix check — the escape the ../ cases above cannot catch.
func TestResolveSandboxPathRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "sandbox")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		// Skipping is fine on a dev machine without Developer Mode. On CI it
		// is not: a containment test that silently skips means nothing ever
		// checks the sandbox except the owner's laptop.
		if os.Getenv("CI") != "" {
			t.Fatalf("CI cannot create symlinks, so this containment test would not run: %v", err)
		}
		t.Skipf("symlinks unavailable (Windows needs Developer Mode): %v", err)
	}

	for _, escape := range []string{"link", "link/secret.txt", "link/new-file.txt"} {
		if _, err := resolveSandboxPath(root, escape); err == nil {
			t.Errorf("resolveSandboxPath(%q): expected error escaping sandbox via symlink, got nil", escape)
		}
	}

	// A symlink that stays inside the root is still fine.
	if err := os.Symlink(root, filepath.Join(root, "self")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := resolveSandboxPath(root, "self/ok.txt"); err != nil {
		t.Errorf("resolveSandboxPath(self/ok.txt): unexpected error for in-root symlink: %v", err)
	}
}

// The gate judges where a path lands, not how it was written. An absolute path
// used to be refused on sight, even when it pointed inside the root — a
// spelling rule that read like a permission rule. It cannot survive a workspace
// with a second folder in it (naming that folder in full is the only way to
// reach it), and a rule that holds for one workspace folder but not the others
// is the kind of surprise nobody can find later.
func TestResolveSandboxPathAcceptsAbsolutePathInsideRoot(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "abs.txt")
	got, err := resolveSandboxPath(root, abs)
	if err != nil {
		t.Fatalf("resolveSandboxPath(absolute path inside root): unexpected error: %v", err)
	}
	if got != abs {
		t.Errorf("resolveSandboxPath(%q) = %q, want the path unchanged", abs, got)
	}
}

// ...and an absolute path outside every workspace folder is still refused, which
// is the half that was doing real work.
func TestResolveSandboxPathRejectsAbsoluteOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	if _, err := resolveSandboxPath(root, filepath.Join(t.TempDir(), "elsewhere.txt")); err == nil {
		t.Error("resolveSandboxPath(absolute path outside the workspace): expected error, got nil")
	}
}
