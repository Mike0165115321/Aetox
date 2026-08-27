package skill

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// The file tools' half of the WSL story. shell_sandbox_wsl_test.go covers the
// command line; this covers `path:` — the argument every other tool takes.
//
// They were two different worlds until 2026-08-17. The shell knew which distro
// it was speaking to and translated; read, grep, glob and list did not, so a
// guest path handed to them was not absolute to Go on Windows, was joined onto
// the sandbox root, and pointed at a folder that has never existed. grep
// answered "(no matches)" for a project full of them, and the agent told the
// user their code did not contain what it had never looked at.

// selectShell points a root's file tools at a distro for the length of one test.
// A name no machine has, for wslGate's reason: HostPath answers for it without
// starting anything.
func selectShell(t *testing.T, root string, backend proc.Backend) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("translating drive letters is a Windows question")
	}
	setSandboxShell(root, func() proc.Backend { return backend })
	t.Cleanup(func() { setSandboxShell(root, nil) })
}

func TestFileToolsFindTheWorkspaceThroughItsGuestSpelling(t *testing.T) {
	root, _ := focusedProject(t)
	selectShell(t, root, proc.WSL("aetox-guard-test"))
	if err := os.WriteFile(filepath.Join(root, "config.py"), []byte("ADMIN_PASSWORD = \"hunter2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	guest := guestSpelling(t, root)

	// The exact call from the transcript that started this: a search of a real
	// folder, named the way the session's own shell names it.
	grep := &grepSkill{root: root}
	out, err := grep.ExecuteTool(context.Background(), map[string]any{"pattern": "ADMIN_PASSWORD", "path": guest, "show": grepModeContent})
	if err != nil {
		t.Fatalf("grep refused the workspace in its guest spelling (%s): %v", guest, err)
	}
	if !strings.Contains(out.Content, "ADMIN_PASSWORD") {
		t.Errorf("grep searched somewhere else — Content = %q", out.Content)
	}

	list := &listSkill{root: root}
	if out, err := list.ExecuteTool(context.Background(), map[string]any{"path": guest}); err != nil {
		t.Errorf("list refused the workspace in its guest spelling: %v", err)
	} else if !strings.Contains(out.Content, "config.py") {
		t.Errorf("list read a different folder — Content = %q", out.Content)
	}

	glob := &globSkill{root: root}
	if out, err := glob.ExecuteTool(context.Background(), map[string]any{"pattern": "**/*.py", "path": guest}); err != nil {
		t.Errorf("glob refused the workspace in its guest spelling: %v", err)
	} else if !strings.Contains(out.Content, "config.py") {
		t.Errorf("glob walked a different folder — Content = %q", out.Content)
	}
}

// Translating is not widening. The same wall, checked against the file the
// caller actually meant instead of against a mangled path that was refused by
// accident.
func TestGuestPathsOutsideTheWorkspaceAreStillRefused(t *testing.T) {
	root, outside := focusedProject(t)
	selectShell(t, root, proc.WSL("aetox-guard-test"))

	grep := &grepSkill{root: root}
	if _, err := grep.ExecuteTool(context.Background(), map[string]any{
		"pattern": "secret",
		"path":    guestSpelling(t, outside),
	}); err == nil {
		t.Error("a guest path outside the project was searched")
	}

	read := &readSkill{root: root}
	if _, err := read.ExecuteTool(context.Background(), map[string]any{
		"path": guestSpelling(t, filepath.Join(outside, "secret.txt")),
	}); err == nil {
		t.Error("a guest path outside the project was read")
	}

	// The distro's own filesystem is not the workspace either, and it is the
	// spelling that carries no drive letter to give it away.
	if _, err := read.ExecuteTool(context.Background(), map[string]any{"path": "/etc/passwd"}); err == nil {
		t.Error("a path inside the distro was read from a focused Windows project")
	}
}

// A session on the native shell must not start guessing that `/etc/passwd` is
// a distro path. Nothing about those workspaces changed.
func TestNativeSessionsTreatGuestPathsExactlyAsBefore(t *testing.T) {
	root, _ := focusedProject(t)
	if runtime.GOOS != "windows" {
		t.Skip("a leading / is an ordinary absolute path off Windows")
	}
	if err := os.MkdirAll(filepath.Join(root, "mnt", "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "mnt", "d", "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No shell selected: the old reading, where a leading slash is relative to
	// the root because Windows does not consider it absolute.
	got, err := resolveSandboxPath(root, "/mnt/d/a.txt")
	if err != nil {
		t.Fatalf("resolveSandboxPath: %v", err)
	}
	if want := filepath.Join(root, "mnt", "d", "a.txt"); got != want {
		t.Errorf("resolveSandboxPath = %q, want %q", got, want)
	}
}

// The denylist is home-relative, and a distro is a second home on this machine.
// Reachable by the natural spelling now that the file tools translate, so the
// list has to know that home by the name Windows gives it.
func TestGuestHomeDirNamesTheHomeADistroPathBelongsTo(t *testing.T) {
	cases := []struct {
		target, want string
	}{
		{`\\wsl.localhost\mikedev\home\mike\.ssh\id_rsa`, `\\wsl.localhost\mikedev\home\mike`},
		{`\\wsl.localhost\mikedev\root\.aws\credentials`, `\\wsl.localhost\mikedev\root`},
		{`\\WSL.LOCALHOST\mikedev\home\mike\.ssh`, `\\wsl.localhost\mikedev\home\mike`}, // one server, two spellings
		{`\\wsl$\mikedev\home\mike\.gnupg`, `\\wsl.localhost\mikedev\home\mike`},        // the older share name
		{`\\wsl.localhost\mikedev\etc\passwd`, ``},                                      // not a home, and not this check's business
		{`D:\project\home\mike\.ssh`, ``},                                               // an ordinary Windows folder that reads like one
	}
	for _, c := range cases {
		if got := guestHomeDir(c.target); got != c.want {
			t.Errorf("guestHomeDir(%q) = %q, want %q", c.target, got, c.want)
		}
	}
}

// Against a real distro, because everything above agrees with proc about how a
// path is spelled and none of it can tell whether that spelling is the one this
// machine's distro actually uses. The mount root is the part that has to be
// asked rather than assumed: /etc/wsl.conf can move it, and a workspace whose
// files sit somewhere the distro does not mount them is exactly the silence
// this whole change exists to break.
func TestAFileToolAndTheDistroAgreeOnWhereTheWorkspaceIs(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("choosing between a native shell and WSL is a Windows question")
	}
	distros := proc.Distros()
	if len(distros) == 0 {
		t.Skip("no WSL distro on this machine")
	}
	isolateAuditLog(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte("ADMIN_PASSWORD=hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := proc.WSL(distros[0])
	selectShell(t, root, backend)

	// Ask the distro itself where it is standing, and hand that answer — not
	// one this package computed — to the file tools.
	shell := &shellSkill{root: root}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := shell.Execute(ctx, Input{"args": []string{"pwd"}})
	if err != nil {
		t.Fatalf("asking %s where it is: %v", distros[0], err)
	}
	guest := strings.TrimSpace(out.Content)
	if !strings.HasPrefix(guest, "/") {
		t.Fatalf("pwd did not answer with a guest path: %q", guest)
	}

	grep := &grepSkill{root: root}
	found, err := grep.ExecuteTool(context.Background(), map[string]any{"pattern": "ADMIN_PASSWORD", "path": guest, "show": grepModeContent})
	if err != nil {
		t.Fatalf("grep refused %s, the folder the distro says the command is standing in: %v", guest, err)
	}
	if !strings.Contains(found.Content, "ADMIN_PASSWORD") {
		t.Errorf("grep searched somewhere other than %s — Content = %q", guest, found.Content)
	}
}

// The return trip. Every test above hands a guest path *in*; this is the one
// path a tool hands *out*, and it is the one the model is told to repeat rather
// than assemble. A receipt naming only `D:\…` sends the next command to a drive
// letter this shell has never heard of.
func TestAPlacedFileReceiptCarriesTheSpellingTheShellCanOpen(t *testing.T) {
	root := t.TempDir()
	selectShell(t, root, proc.WSL("aetox-guard-test"))
	placed := "out"
	s := &writeSkill{root: root, outputSubdir: func() string { return placed }}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "report.md", "content": "hello\n"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	// The Windows path stays: it is what the user opens in Explorer, and
	// answering "where is it on my machine" is why this clause exists at all.
	if !strings.Contains(out.Content, filepath.Join(root, placed, "report.md")) {
		t.Errorf("the receipt lost the path the user needs:\n%s", out.Content)
	}
	// And beside it, the name this session's shell knows the same file by.
	guest := guestSpelling(t, filepath.Join(root, placed, "report.md"))
	if !strings.Contains(out.Content, guest) {
		t.Errorf("the receipt does not say what to type in this shell (want %s):\n%s", guest, out.Content)
	}
}

// The same receipt on an ordinary machine is the string it always was. A second
// spelling where there is only one filesystem is noise in every context window.
func TestAPlacedFileReceiptStaysOneLineOnANativeShell(t *testing.T) {
	root := t.TempDir()
	placed := "out"
	s := &writeSkill{root: root, outputSubdir: func() string { return placed }}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "report.md", "content": "hello\n"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if strings.Contains(out.Content, "in this shell") {
		t.Errorf("a native session was told about a second spelling of its own paths:\n%s", out.Content)
	}
}

func TestCredentialStoresInsideADistroAreRefused(t *testing.T) {
	refused := []string{
		`\\wsl.localhost\mikedev\home\mike\.ssh\id_rsa`,
		`\\wsl.localhost\mikedev\home\mike\.aws\credentials`,
		`\\wsl.localhost\mikedev\root\.git-credentials`,
	}
	for _, target := range refused {
		if err := refuseResolved(target); err == nil {
			t.Errorf("refuseResolved(%q) = nil — a credential store inside the distro was readable", target)
		}
	}
	// And the rest of the distro is not swept up with it: the point of the
	// denylist is that it is short and literal.
	for _, target := range []string{
		`\\wsl.localhost\mikedev\home\mike\projects\api\main.go`,
		`\\wsl.localhost\mikedev\etc\hostname`,
	} {
		if err := refuseResolved(target); err != nil {
			t.Errorf("refuseResolved(%q) = %v — ordinary files inside the distro must stay readable", target, err)
		}
	}
}
