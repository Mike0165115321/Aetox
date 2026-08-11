package skill

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// focusedProject sets up a project with a folder next door that is NOT part
// of the workspace — the shape every escape below is trying to break out of.
func focusedProject(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root, outside = filepath.Join(base, "project"), filepath.Join(base, "outside")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

// Every one of these used to run. `shell` set its working directory and handed
// the line to the OS, so the wall the UI describes was real for read/grep/write
// and imaginary for the tool that can do what all three do.
func TestShellRefusesEveryWayOutOfTheWorkspace(t *testing.T) {
	root, outside := focusedProject(t)
	secret := filepath.Join(outside, "secret.txt")

	cases := []struct {
		name    string
		command string
	}{
		{"an absolute path outside", "type " + secret},
		{"a climbing relative path", "type " + filepath.Join("..", "outside", "secret.txt")},
		{"a redirect target outside", "echo hi > " + filepath.Join(outside, "out.txt")},
		{"the second half of a compound command", "git status && type " + secret},
		{"a piped-to command's argument", "echo x | findstr /f:" + secret},
		{"a glob rooted outside", "del " + filepath.Join(outside, "*")},
		{"a path attached to a flag", "grep --file=" + secret + " pattern"},
		{"a quoted path outside", `type "` + secret + `"`},
	}
	for _, tc := range cases {
		if err := guardCommandPaths(root, tc.command, nativeGate()); err == nil {
			t.Errorf("%s: command was allowed out of the workspace: %s", tc.name, tc.command)
		}
	}
}

// A path assembled while the command runs cannot be checked before it runs.
// Passing it through unchecked would be a permission granted by the parser's
// limitations, so it stops the command instead — the difference between a gap
// that is known and one that is quiet.
func TestShellRefusesCommandsItCannotRead(t *testing.T) {
	root, _ := focusedProject(t)

	cases := []struct{ name, command string }{
		{"command substitution", "type $(echo somewhere)"},
		{"backtick substitution", "type `echo somewhere`"},
		{"brace expansion", "type ${SOMEWHERE}/file"},
		{"an encoded command", "powershell -EncodedCommand ZQBjAGgAbwA="},
		{"invoke-expression", "powershell Invoke-Expression $payload"},
		{"a variable nobody can resolve", `type %SOMEWHERE%\file.txt`},
		{"a dollar variable heading a path", `type $SOMEWHERE\file.txt`},
	}
	for _, tc := range cases {
		err := guardCommandPaths(root, tc.command, nativeGate())
		if err == nil {
			t.Errorf("%s: unreadable command was allowed: %s", tc.name, tc.command)
			continue
		}
		if !strings.Contains(err.Error(), "builds a path while it runs") {
			t.Errorf("%s: refused for the wrong reason (%v)", tc.name, err)
		}
	}
}

// The variables that name a folder are expanded before the check, or the
// oldest escape in the book walks straight through: nothing in the literal text
// of "%USERPROFILE%\.ssh\id_rsa" looks like an absolute path.
func TestShellExpandsHomeVariablesBeforeChecking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{
		`type %USERPROFILE%\.ssh\id_rsa`,
		`cat $HOME/.ssh/id_rsa`,
		`cat $env:USERPROFILE\.ssh\id_rsa`,
		`cat ~/.ssh/id_rsa`,
	} {
		if err := guardCommandPaths(root, command, nativeGate()); err == nil {
			t.Errorf("a home-variable path reached a credential store: %s", command)
		}
	}
}

// The gate is worth nothing if it is turned off, and it gets turned off the
// first time it refuses `go test`. These are the commands an agent actually
// runs, and none of them names anything outside the project.
func TestShellDoesNotRefuseOrdinaryCommands(t *testing.T) {
	root, _ := focusedProject(t)

	commands := []string{
		"go test ./...",
		"go build -o bin/app.exe ./cmd/aetox",
		"npm run build",
		"npm test -- --watch",
		"git status --porcelain",
		"git log --oneline -5",
		// The reason variables are only expanded at the head of a token: this
		// is a format string, not a path built out of a variable named H.
		"git log --pretty=format:%H%n",
		// An unknown variable with no path separator after it names no folder,
		// whatever it holds — this is an expression, and refusing it refused
		// every PowerShell command that starts with one.
		"$PSVersionTable.PSVersion.ToString()",
		"powershell -Command $Host.Version",
		"echo %ERRORLEVEL%",
		"grep -rn pattern internal/",
		"cat src/main.go",
		"NODE_ENV=test npm run check",
		"echo hello > out.txt",
		"go test ./internal/... 2>&1",
		"ping -n 2 127.0.0.1 >nul",
		"echo done | more",
	}
	for _, command := range commands {
		if err := guardCommandPaths(root, command, nativeGate()); err != nil {
			t.Errorf("refused an ordinary command %q: %v", command, err)
		}
	}
}

// A toolchain lives outside the project by definition, and PATH resolution
// reaches the same file — so refusing the absolute spelling would forbid the
// spelling and permit the act. This is the boundary's stated edge: program
// position is not contained, because running code is what the tool is for and
// no path check can contain it. Arguments after it are, which is the half that
// decides what the command touches.
func TestShellDoesNotContainProgramPosition(t *testing.T) {
	root, outside := focusedProject(t)
	tool := filepath.Join(outside, "tool.exe")

	for _, program := range []string{tool, filepath.Join("..", "outside", "tool.exe")} {
		if err := guardCommandPaths(root, program+" --version", nativeGate()); err != nil {
			t.Errorf("refused a program named by path (%s): %v", program, err)
		}
	}
	if err := guardCommandPaths(root, tool+" "+filepath.Join(outside, "secret.txt"), nativeGate()); err == nil {
		t.Error("an outside path in argument position rode in behind the program name")
	}
	// And the separator resets it: after `&&` the next token is a program
	// again, but the tokens after THAT are arguments and stay checked.
	if err := guardCommandPaths(root, "git status && "+tool+" "+filepath.Join(outside, "secret.txt"), nativeGate()); err == nil {
		t.Error("a second command's argument escaped the check")
	}
}

// Same gate as every file tool, so the folders the user added have to work here
// too — otherwise "add the folder" would fix read and grep and leave the agent
// unable to build or test the thing it was let in to look at.
func TestShellFollowsTheWorkspaceFolderList(t *testing.T) {
	root, outside := focusedProject(t)
	command := "go build " + filepath.Join(outside, "main.go")

	if err := guardCommandPaths(root, command, nativeGate()); err == nil {
		t.Fatal("the folder was reachable before it was added")
	}
	setSandboxPolicy(root, false, []string{outside})
	t.Cleanup(func() { setSandboxPolicy(root, false, nil) })
	if err := guardCommandPaths(root, command, nativeGate()); err != nil {
		t.Errorf("an added folder is still refused by shell: %v", err)
	}
}

// Unfocused mode roams the machine, and shell should roam with it — but the
// credential cabinet does not open for any mode (sandbox_open.go).
func TestShellInOpenModeRoamsButNotIntoCredentialStores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "aetox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	setSandboxPolicy(root, true, nil)
	t.Cleanup(func() { setSandboxPolicy(root, false, nil) })

	if err := guardCommandPaths(root, "type "+filepath.Join(home, "notes.txt"), nativeGate()); err != nil {
		t.Errorf("open mode refused an ordinary path: %v", err)
	}
	if err := guardCommandPaths(root, "type "+filepath.Join(home, ".aws", "credentials"), nativeGate()); err == nil {
		t.Error("open mode handed shell a credential store path")
	}
}

// A variable holding a path is unreadable to the scanner, so a focused project
// refuses the command — there is a wall it might have jumped. The open machine
// has no wall, and refusing it there breaks nearly every real PowerShell
// script, which is what it did on 2026-08-11: counting a folder's files failed
// because the folder was in a variable. This is the fix's whole point, and the
// contrast with the focused case is the proof it turns on the workspace and not
// on the shell.
func TestVariablePathIsRefusedOnlyWhereThereIsAWall(t *testing.T) {
	command := `$d='C:\Users\ASUS\Downloads'; Get-ChildItem -Path $d`

	focused, _ := focusedProject(t)
	if err := guardCommandPaths(focused, command, nativeGate()); err == nil {
		t.Error("a focused project let a variable-built path through — the wall it guards is gone")
	}

	home := t.TempDir()
	open := filepath.Join(home, "aetox")
	if err := os.MkdirAll(open, 0o755); err != nil {
		t.Fatal(err)
	}
	setSandboxPolicy(open, true, nil)
	t.Cleanup(func() { setSandboxPolicy(open, false, nil) })
	if err := guardCommandPaths(open, command, nativeGate()); err != nil {
		t.Errorf("open mode refused a variable-built path, guarding a wall that is not there: %v", err)
	}
}

// The residual the open-mode relaxation is willing to pay, pinned so a later
// change cannot widen it by accident: a credential path written LITERALLY is
// still refused in open mode, because that check lives on the resolved path in
// resolveSandboxPath, not in the token scanner. Only a path assembled entirely
// from runtime values — which the scanner never could read — slips, and that is
// the same defence-in-depth line OpenCode draws.
func TestOpenModeStillRefusesLiteralCredentialPathsEvenBesideAVariable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "aetox")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	setSandboxPolicy(root, true, nil)
	t.Cleanup(func() { setSandboxPolicy(root, false, nil) })

	// A variable elsewhere in the line must not become a skeleton key for the
	// literal credential path beside it.
	command := `$x=1; type ` + filepath.Join(home, ".ssh", "id_rsa")
	if err := guardCommandPaths(root, command, nativeGate()); err == nil {
		t.Error("a variable in the line let a literal credential path through in open mode")
	}
}

// An interpreter one-liner is a single token to any tokenizer and a filesystem
// path to the OS. Tokenizing alone leaves the whole family open — python, node,
// sed, powershell -Command — so paths are also recognised by shape anywhere in
// the line, quoted or not.
func TestShellFindsPathsHiddenInsideQuotedCode(t *testing.T) {
	root, outside := focusedProject(t)
	secret := filepath.ToSlash(filepath.Join(outside, "secret.txt"))

	cases := []string{
		`python -c "print(open('` + secret + `').read())"`,
		`node -e "console.log(require('fs').readFileSync('` + secret + `','utf8'))"`,
		`powershell -Command "Get-Content '` + secret + `'"`,
		`sed -n 1p "` + secret + `"`,
	}
	for _, command := range cases {
		if err := guardCommandPaths(root, command, nativeGate()); err == nil {
			t.Errorf("a path hidden inside code walked out: %s", command)
		}
	}

	// And the scan must not start refusing ordinary lines that merely contain
	// a colon or a slash.
	for _, command := range []string{
		"npm install https://github.com/someone/repo.git",
		"go test ./...",
		"curl https://example.com/api -o out.json",
		`python -c "print('hello')"`,
	} {
		if err := guardCommandPaths(root, command, nativeGate()); err != nil {
			t.Errorf("the path scan refused an ordinary command %q: %v", command, err)
		}
	}
}

// git had its own smaller gate — a denylist of options someone thought of,
// which is why `--output=` was not on it. Routing its argv through this one
// means git stops needing anyone to think of the next option.
func TestGitAnswersToTheSameGate(t *testing.T) {
	root, outside := focusedProject(t)

	refuse := [][]string{
		{"--output=" + filepath.Join(outside, "leak.patch")},
		{"--", filepath.Join(outside, "secret.txt")},
		{filepath.Join("..", "outside", "secret.txt")},
	}
	for _, args := range refuse {
		if err := guardArgs(root, args); err == nil {
			t.Errorf("git args reached outside the workspace: %v", args)
		}
	}

	allow := [][]string{
		{"--oneline", "-5"},
		{"--pretty=format:%H%n"},
		{"HEAD~1", "--", "internal/skill"},
		{"--since", "2 weeks ago"},
		{"--porcelain"},
	}
	for _, args := range allow {
		if err := guardArgs(root, args); err != nil {
			t.Errorf("refused ordinary git args %v: %v", args, err)
		}
	}
}

// cmd spells its switches with a leading forward slash, so reading those as
// drive-relative paths refuses half the commands anyone writes on Windows. A
// leading BACKSLASH is a real path from the current drive's root, and stays
// closed.
func TestShellTellsWindowsSwitchesFromDriveRelativePaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd switch syntax is a Windows question")
	}
	root, _ := focusedProject(t)

	for _, command := range []string{"dir /s /b", "for /L %i in (1,0,2) do @echo tick", "ping /?"} {
		if err := guardCommandPaths(root, command, nativeGate()); err != nil {
			t.Errorf("refused a cmd switch as if it were a path: %q → %v", command, err)
		}
	}
	if err := guardCommandPaths(root, `type \Users\someone\.ssh\id_rsa`, nativeGate()); err == nil {
		t.Error("a drive-relative path walked out of the workspace")
	}
}
