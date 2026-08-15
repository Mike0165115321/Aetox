package skill

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/proc"
)

// The containment story for a workspace whose commands run in a WSL distro.
//
// Everything here used to be decided by runtime.GOOS, and every case below is
// one the Windows branch got wrong for a bash command line — not by refusing
// too much, which would at least be visible, but by finding no path at all in
// `/mnt/d/elsewhere` and letting it through. The guard cannot be right about a
// shell it is not told about.
//
// Drive letters are the whole subject, so these are Windows-only.

func wslGate(t *testing.T) shellGate {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("translating drive letters is a Windows question")
	}
	// A name no machine has: HostPath resolves it without starting anything,
	// and a real distro would make the test depend on which one is installed.
	return gateFor(proc.WSL("aetox-guard-test"))
}

// guestSpelling is how the distro names a Windows path, worked out here rather
// than borrowed from proc so the guard is checked against an independent
// answer instead of against the same code twice.
func guestSpelling(t *testing.T, host string) string {
	t.Helper()
	abs, err := filepath.Abs(host)
	if err != nil {
		t.Fatal(err)
	}
	if len(abs) < 2 || abs[1] != ':' {
		t.Fatalf("expected a drive-letter path, got %q", abs)
	}
	return "/mnt/" + strings.ToLower(abs[:1]) + strings.ReplaceAll(abs[2:], `\`, "/")
}

func TestShellUnderWSLRefusesGuestPathsOutsideTheWorkspace(t *testing.T) {
	gate := wslGate(t)
	root, outside := focusedProject(t)
	secret := guestSpelling(t, filepath.Join(outside, "secret.txt"))

	cases := []struct {
		name    string
		command string
	}{
		{"an absolute path outside", "cat " + secret},
		{"a redirect target outside", "echo hi > " + guestSpelling(t, filepath.Join(outside, "out.txt"))},
		{"the second half of a compound command", "git status && cat " + secret},
		{"a piped-to command's argument", "echo x | grep -f " + secret},
		{"a glob rooted outside", "rm " + guestSpelling(t, outside) + "/*"},
		{"a path attached to a flag", "grep --file=" + secret + " pattern"},
		{"a quoted path outside", `cat "` + secret + `"`},
		{"a climbing relative path", "cat ../outside/secret.txt"},
	}
	for _, tc := range cases {
		if err := guardCommandPaths(root, tc.command, gate); err == nil {
			t.Errorf("%s: command was allowed out of the workspace: %s", tc.name, tc.command)
		}
	}
}

// The distro's own filesystem is not a blind spot to be listed and blocked. It
// has a name Windows knows — \\wsl.localhost\<distro>\... — which lands outside
// every workspace root, so /etc and /home are refused by the ordinary rule
// without this file ever having heard of them.
func TestShellUnderWSLTreatsTheDistrosOwnFilesystemAsOutside(t *testing.T) {
	gate := wslGate(t)
	root, _ := focusedProject(t)

	for _, command := range []string{
		"cat /etc/passwd",
		"cat /home/someone/.ssh/id_rsa",
		"cp secret.txt /tmp/stash",
		"ls /root",
	} {
		if err := guardCommandPaths(root, command, gate); err == nil {
			t.Errorf("a path in the distro's own filesystem was allowed: %s", command)
		}
	}
}

// A path inside the workspace, spelled the way the shell that will run it
// spells it. If this fails the feature is unusable rather than unsafe: every
// command the model writes for the distro gets refused.
func TestShellUnderWSLAllowsGuestPathsInsideTheWorkspace(t *testing.T) {
	gate := wslGate(t)
	root, _ := focusedProject(t)
	inside := guestSpelling(t, root)

	for _, command := range []string{
		"cat " + inside + "/main.go",
		"go build -o " + inside + "/bin/app ./cmd/aetox",
		"ls " + inside,
	} {
		if err := guardCommandPaths(root, command, gate); err != nil {
			t.Errorf("refused a path inside the workspace %q: %v", command, err)
		}
	}
}

// The gate is worth nothing if it is turned off, and it gets turned off the
// first time it refuses `go test`. Same standard the native side is held to,
// with the commands a bash user actually writes.
func TestShellUnderWSLDoesNotRefuseOrdinaryCommands(t *testing.T) {
	gate := wslGate(t)
	root, _ := focusedProject(t)

	commands := []string{
		"ls -la",
		"go test ./...",
		"npm run build",
		"git log --oneline -5",
		"git log --pretty=format:%H%n",
		"grep -rn pattern internal/",
		"cat src/main.go",
		"NODE_ENV=test npm run check",
		"echo hello > out.txt",
		"go test ./internal/... 2>&1",
		"curl https://example.com/api -o out.json",
		"echo done | head -20",
		"cat /dev/null",
		// %VAR% is not a variable to bash, it is a filename; expanding it would
		// send the check off to a Windows folder the command never touches.
		"cat %USERPROFILE%",
	}
	for _, command := range commands {
		if err := guardCommandPaths(root, command, gate); err != nil {
			t.Errorf("refused an ordinary command %q: %v", command, err)
		}
	}
}

// The embedded-path scan only looks for bare `/…` when the shell is a POSIX
// one. On Windows it was switched off, which is exactly the machine a WSL
// command line runs on — so without the gate this family walked straight out.
func TestShellUnderWSLFindsGuestPathsHiddenInsideQuotedCode(t *testing.T) {
	gate := wslGate(t)
	root, outside := focusedProject(t)
	secret := guestSpelling(t, filepath.Join(outside, "secret.txt"))

	for _, command := range []string{
		`python3 -c "print(open('` + secret + `').read())"`,
		`node -e "console.log(require('fs').readFileSync('` + secret + `','utf8'))"`,
		`sed -n 1p "` + secret + `"`,
		`awk '{print}' /etc/shadow`,
	} {
		if err := guardCommandPaths(root, command, gate); err == nil {
			t.Errorf("a path hidden inside code walked out: %s", command)
		}
	}
}

// ~ is the shell's home, and the shell's home is in the distro. Expanding it to
// this machine's Windows profile would check a folder the command will not
// touch — which lets it through when the real target is outside, and refuses it
// when the real target is the project.
func TestShellUnderWSLDoesNotExpandTildeToTheWindowsProfile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the two homes are only different on Windows")
	}
	root, _ := focusedProject(t)
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	// A stub for the distro's answer, because the real one costs a distro
	// start and this is a question about which home gets asked, not about WSL.
	guestHome := "/home/someone"
	gate := shellGate{posix: true, toHost: func(p string) (string, bool) {
		if after, ok := strings.CutPrefix(p, "~"); ok {
			p = guestHome + after
		}
		if strings.HasPrefix(p, "/") {
			return `\\wsl.localhost\d\` + strings.ReplaceAll(strings.TrimPrefix(p, "/"), "/", `\`), true
		}
		return p, true
	}}

	for _, command := range []string{"cat ~/.ssh/id_rsa", "cat $HOME/.ssh/id_rsa"} {
		err := guardCommandPaths(root, command, gate)
		if err == nil {
			t.Errorf("%q was allowed — the tilde resolved to somewhere reachable", command)
			continue
		}
		// The refusal has to be about the distro's home, not about a Windows
		// folder that happens to also be outside the project.
		if strings.Contains(err.Error(), home) {
			t.Errorf("%q was checked against the Windows profile %q: %v", command, home, err)
		}
	}
}

// End to end, through the tool the model actually calls: the selected backend
// reaching a real distro, the guard letting a workspace path through, and the
// output coming back. Each test above checks one link; this is the only one
// that checks they are joined.
func TestShellSkillRunsInTheSelectedDistro(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("choosing between a native shell and WSL is a Windows question")
	}
	distros := proc.Distros()
	if len(distros) == 0 {
		t.Skip("no WSL distro on this machine")
	}
	isolateAuditLog(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte("in the workspace\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := proc.WSL(distros[0])
	s := &shellSkill{root: root, backend: func() proc.Backend { return backend }}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	out, err := s.Execute(ctx, Input{"args": []string{"uname -s && cat marker.txt"}})
	if err != nil {
		t.Fatalf("running a command in %s: %v", distros[0], err)
	}
	if !strings.Contains(out.Content, "Linux") {
		t.Errorf("the output did not come from the distro:\n%s", out.Content)
	}
	// Named relatively, which is the spelling that has to work because it is
	// the one the model writes most.
	if !strings.Contains(out.Content, "in the workspace") {
		t.Errorf("the command did not run in the workspace folder:\n%s", out.Content)
	}

	// And the wall still stands from inside the distro: a guest path pointing
	// out of the workspace is refused before anything runs.
	if _, err := s.Execute(ctx, Input{"args": []string{"cat /etc/hostname"}}); err == nil {
		t.Error("a command reached outside the workspace through the distro's own filesystem")
	}
}

// The one fact the model is given about the shell it is writing for. Baked in
// at build time it was always true; read from a setting it is only true if it
// is read every time the definition is built.
func TestShellToolDescriptionNamesTheSelectedShell(t *testing.T) {
	selected := proc.Backend(proc.Native())
	s := &shellSkill{backend: func() proc.Backend { return selected }}

	if desc := s.ToolDefinition().Function.Description; !strings.Contains(desc, proc.ShellName()) {
		t.Errorf("the description does not name the native shell:\n%s", desc)
	}
	selected = proc.WSL("mikedev")
	desc := s.ToolDefinition().Function.Description
	if !strings.Contains(desc, "mikedev") {
		t.Errorf("after switching to WSL the description still does not say so:\n%s", desc)
	}
	if strings.Contains(desc, proc.ShellName()) {
		t.Errorf("the description still names the native shell (%s) after switching to WSL:\n%s", proc.ShellName(), desc)
	}
}
