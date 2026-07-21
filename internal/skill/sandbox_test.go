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

func TestResolveSandboxPathRejectsAbsolute(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "abs.txt")
	if _, err := resolveSandboxPath(root, abs); err == nil {
		t.Errorf("resolveSandboxPath(absolute path): expected error, got nil")
	}
}

