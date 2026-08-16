package proc

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// These run against a real distro and skip without one. Everything they check
// is an assumption this backend is built on that no unit test can confirm:
// whether wsl.exe really takes a Windows path for --cd, whether os/exec's
// escaping survives the trip into Linux argv, and whether the profile is
// loaded. Each is the kind of thing that looks obviously true until the first
// command of a session fails on it.

func liveDistro(t *testing.T) string {
	t.Helper()
	distros := Distros()
	if len(distros) == 0 {
		t.Skip("no WSL distro on this machine")
	}
	return distros[0]
}

func runInWSL(t *testing.T, distro, line, dir string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := WSL(distro).Command(ctx, line, dir).Output()
	if err != nil {
		t.Fatalf("running %q in %s: %v", line, distro, err)
	}
	return strings.TrimSpace(string(out))
}

func TestWSLRunsTheCommandLine(t *testing.T) {
	distro := liveDistro(t)
	if got := runInWSL(t, distro, "uname -s", ""); got != "Linux" {
		t.Errorf("uname -s = %q, want Linux — the command did not run in the distro", got)
	}
}

// --cd is given the Windows directory and is trusted to translate it. If that
// stopped being true, every relative path in every command would resolve
// against the distro's home directory instead of the workspace, silently.
func TestWSLWorkingDirectoryIsTheWindowsOne(t *testing.T) {
	distro := liveDistro(t)
	dir := t.TempDir()
	marker := "aetox-cwd-probe"
	if err := os.WriteFile(dir+string(os.PathSeparator)+marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := runInWSL(t, distro, "ls "+marker, dir); got != marker {
		t.Errorf("ls in the working directory = %q, want %q — --cd did not land in the Windows folder", got, marker)
	}
	// And the same directory named the way the guest sees it, which is what
	// GuestPath has to agree with for the sandbox guard to be checking the
	// same folder the command is standing in.
	guest, ok := WSL(distro).(*wslBackend).GuestPath(dir)
	if !ok {
		t.Fatalf("GuestPath(%q) refused the temp dir", dir)
	}
	if got := runInWSL(t, distro, "pwd", dir); !strings.EqualFold(got, guest) {
		t.Errorf("pwd = %q but GuestPath said %q", got, guest)
	}
}

// The cmd.exe bug this backend must not repeat: quoted arguments arriving
// mangled. wsl.exe follows the argv convention os/exec escapes for, so the line
// should reach bash byte for byte — including the quotes and the spaces inside
// them (see shell_windows.go for what happens when a shell does not).
func TestWSLPassesQuotedArgumentsThrough(t *testing.T) {
	distro := liveDistro(t)
	if got := runInWSL(t, distro, `echo "hello   world"`, ""); got != "hello   world" {
		t.Errorf(`echo "hello   world" = %q — quoting was corrupted on the way in`, got)
	}
	if got := runInWSL(t, distro, `python3 -c "print('a b')" 2>/dev/null || echo skipped`, ""); got != "a b" && got != "skipped" {
		t.Errorf("embedded quotes reached the interpreter as %q", got)
	}
}

// The command line must reach bash with its `$` still in it.
//
// It did not, for as long as the invocation said `--`: that spelling hands the
// line to the distro's default shell first, and that shell expands every `$…`
// against its own environment before bash is started (Command's comment has the
// measurement). The damage was silent — `awk '{print $1}'` quietly printed
// whole lines instead of a column, and `$?` reported a failed command as 0 —
// which is why this is a test and not a comment: nothing about the output of a
// wrong `awk` looks wrong.
//
// Each case below is one shape of "the value does not exist yet when the outer
// shell reads the line": a field variable, an exit status, a variable the line
// assigns itself, and a `$` that was never a variable at all.
func TestWSLDoesNotExpandTheCommandLineBeforeBashReaches(t *testing.T) {
	distro := liveDistro(t)
	for _, tc := range []struct{ line, want string }{
		{`printf 'alpha 1\n' | awk '{print $1}'`, "alpha"},
		{`printf 'a 1\nb 2\n' | awk '{sum+=$2} END {print sum}'`, "3"},
		{`false; echo rc=$?`, "rc=1"},
		{`X=5; echo val=$X`, "val=5"},
		{`echo 'price is $5'`, "price is $5"},
	} {
		if got := runInWSL(t, distro, tc.line, ""); got != tc.want {
			t.Errorf("%s = %q, want %q — the command line was expanded before bash read it", tc.line, got, tc.want)
		}
	}
	// The same fault with no shell of ours in the way, so a failure above cannot
	// be mistaken for something bash did: /bin/echo has no expansion of its own,
	// so whatever it prints is exactly what was handed to the distro.
	if got := runInWSL(t, distro, `/bin/echo 'literal $NOSUCHVAR here'`, ""); got != "literal $NOSUCHVAR here" {
		t.Errorf("/bin/echo received %q — something expanded the argument before the program ran", got)
	}
}

// bash -lc, not -c: without the login shell the toolchain a version manager put
// on PATH is not there, and "run it in WSL" reaches a distro that cannot find
// the project's node or python.
func TestWSLLoadsTheLoginProfile(t *testing.T) {
	distro := liveDistro(t)
	if got := runInWSL(t, distro, `shopt -q login_shell && echo login || echo plain`, ""); got != "login" {
		t.Errorf("shell reports %q, want login — the profile will not be loaded", got)
	}
}

// The load-bearing assumption behind reusing KillOnCancel unchanged.
//
// Cancelling kills wsl.exe with taskkill, and a Windows kill cannot reach into
// the WSL2 VM — the Job Object of job_windows.go cannot either. The expectation
// was that Stop, a tool timeout and app exit would each leave the real work
// running inside the distro. Measured, they do not: wsl.exe is the session's
// relay, and losing it tears the session down, children included (2026-08-08 —
// a plain child and a `cmd & wait` child were both reaped; only a process that
// had deliberately left the session with setsid survived, which is exactly what
// survives taskkill /T on the native backend too).
//
// So there is no WSL-shaped hole to work around, and this test is what says so.
// If a future WSL stops reaping the session, every background command starts
// leaking processes and the only symptom is a machine that slowly gets busier —
// the kind of thing never traced back to here. It fails here instead.
func TestWSLCancellationReachesIntoTheDistro(t *testing.T) {
	distro := liveDistro(t)

	probe := func(line string) *exec.Cmd {
		return WSL(distro).Command(context.Background(), line, "")
	}
	alive := func() bool {
		// sleep 99[3]1 matches `sleep 9931` and not itself, so the shell running
		// this very check is not counted as the thing being checked for. Without
		// the bracket the probe reports success forever.
		out, _ := probe("pgrep -f 'sleep 99[3]1' >/dev/null && echo yes || echo no").Output()
		return strings.TrimSpace(string(out)) == "yes"
	}
	t.Cleanup(func() { _ = probe("pkill -f 'sleep 9931' || true").Run() })

	ctx, cancel := context.WithCancel(context.Background())
	cmd := WSL(distro).Command(ctx, "sleep 9931", "")
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("starting the probe: %v", err)
	}

	if !waitFor(30*time.Second, alive) {
		cancel()
		t.Skip("the probe never started inside the distro")
	}
	cancel()
	_ = cmd.Wait()
	if !waitFor(20*time.Second, func() bool { return !alive() }) {
		t.Error("the command is still running inside the distro after cancellation — " +
			"WSL no longer reaps the session, so background commands now need a guest-side kill")
	}
}

// Setting a variable on a child process is the universal way to pass it
// something, and across this boundary it silently does nothing: WSL forwards
// only the names WSLENV lists and drops the rest without a word (measured
// 2026-08-08 — the variable arrives empty, exit status 0, no diagnostic
// anywhere). Hooks are handed their whole payload this way, so a hook under WSL
// would read an empty $AETOX_TOOL and conclude the tool had no name.
func TestWSLExportMakesVariablesReadable(t *testing.T) {
	distro := liveDistro(t)
	backend := WSL(distro)

	read := func(exported bool) string {
		cmd := backend.Command(context.Background(), `printf %s "$AETOX_PROBE"`, "")
		cmd.Env = append(cmd.Env, "AETOX_PROBE=carried")
		if exported {
			backend.Export(cmd, "AETOX_PROBE")
		}
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("reading the variable back: %v", err)
		}
		return strings.TrimSpace(string(out))
	}

	if got := read(false); got != "" {
		t.Logf("WSL now forwards the environment without WSLENV (got %q) — Export could be simplified", got)
	}
	if got := read(true); got != "carried" {
		t.Errorf("an exported variable arrived as %q, want \"carried\" — hooks would run with an empty payload", got)
	}
}

// The variable is shared with the rest of the machine's WSL setup, so a session
// that overwrites it breaks tools the user configured long before Aetox.
func TestWSLExportKeepsWhatWSLENVAlreadyHeld(t *testing.T) {
	backend := WSL("mikedev")
	cmd := backend.Command(context.Background(), "true", "")
	cmd.Env = append(cmd.Env, "WSLENV=EXISTING/p:AETOX_PROBE")
	backend.Export(cmd, "AETOX_PROBE", "AETOX_OTHER")

	var wslenv string
	for _, entry := range cmd.Env {
		if value, ok := strings.CutPrefix(entry, "WSLENV="); ok {
			if wslenv != "" {
				t.Fatalf("WSLENV appears twice in the environment: %q and %q", wslenv, value)
			}
			wslenv = value
		}
	}
	if want := "EXISTING/p:AETOX_PROBE:AETOX_OTHER"; wslenv != want {
		t.Errorf("WSLENV = %q, want %q", wslenv, want)
	}
}

func waitFor(limit time.Duration, done func() bool) bool {
	for deadline := time.Now().Add(limit); time.Now().Before(deadline); {
		if done() {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return done()
}
