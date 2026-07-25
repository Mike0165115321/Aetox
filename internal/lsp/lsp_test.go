package lsp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPathURIRoundTrip(t *testing.T) {
	// gopls rejects file://E:\a\b outright, and a drive letter without the
	// leading slash is read as a hostname — both fail silently as "no
	// diagnostics", which is indistinguishable from "your code is fine".
	for _, p := range []string{`E:\Aetox\Aetox\main.go`, "/home/u/main.go"} {
		uri := pathToURI(p)
		if !strings.HasPrefix(uri, "file:///") {
			t.Errorf("pathToURI(%q) = %q, want a file:/// URI", p, uri)
		}
		if got, want := uriToPath(uri), filepath.ToSlash(mustAbs(t, p)); got != want {
			t.Errorf("round trip of %q gave %q, want %q", p, got, want)
		}
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// "no server for this language" and "a server exists but is not here" must
// stay distinguishable — collapsing them lets an unchecked file be reported as
// a clean one, the most damaging wrong answer this tool can give.
func TestConfiguredIsNotAvailable(t *testing.T) {
	if !Configured("a.rs") {
		t.Error("rust is in the server table, Configured should say so")
	}
	if Configured("notes.txt") {
		t.Error("plain text has no server configured")
	}
	// Available may legitimately be either, depending on the machine — what it
	// must never do is panic or block, and under `go test` it never installs.
	_ = Available(context.Background(), "a.rs")
}

func TestUnsupportedAndMissingServersAreSilent(t *testing.T) {
	c := New(t.TempDir())
	defer c.Close()
	// A language with no server configured, and one configured but (almost
	// certainly) not installed. Both must be quiet: diagnostics are advice on
	// top of an edit that already succeeded.
	for _, name := range []string{"notes.txt", "main.rs"} {
		diags, err := c.Diagnose(context.Background(), name, time.Second)
		if err != nil {
			t.Errorf("%s returned an error: %v", name, err)
		}
		if len(diags) != 0 {
			t.Errorf("%s returned diagnostics: %v", name, diags)
		}
	}
}

// The real thing, against a real server. Skipped where gopls is absent so the
// suite stays green on a machine that never installed it.
func TestDiagnoseReportsRealErrorsFromGopls(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module scratch\n\ngo 1.21\n")
	write("broken.go", "package main\n\nfunc main() {\n\tx := 1\n}\n") // x declared and not used

	c := New(root)
	defer c.Close()

	diags, err := c.Diagnose(context.Background(), "broken.go", 20*time.Second)
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("gopls reported nothing for a file with an unused variable")
	}
	found := false
	for _, d := range diags {
		if d.Line == 4 && strings.Contains(strings.ToLower(d.Message), "declared and not used") {
			found = true
		}
		if d.Line < 1 || d.Column < 1 {
			t.Errorf("positions must be 1-based, got %+v", d)
		}
	}
	if !found {
		t.Errorf("expected the unused-variable error on line 4, got: %v", diags)
	}
}
