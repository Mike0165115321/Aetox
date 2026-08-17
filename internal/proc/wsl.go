package proc

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// The WSL backend runs the agent's commands inside a Windows Subsystem for
// Linux distro instead of cmd.exe, while Aetox itself stays the one Windows
// process it has always been.
//
// That split is the whole design. The alternative — shipping a Linux build and
// running the app inside the distro — means two binaries, two ~/.aetox trees
// (skills, credentials, learned memory, desk history), two update paths, and a
// desktop shell that cannot follow because it is Wails on Windows. Nothing
// about the agent needs to live in Linux. One thing does: the command line,
// because that is where the project's toolchain is. So only that crosses.
//
// What does not cross is worth stating, because it is the part that makes this
// cheap: read, write, grep, glob, edit and apply_patch are Go reading files,
// and \\wsl.localhost has let Windows read a distro's filesystem since 21H2.
// They go on being Windows code whichever backend is selected.
//
// They do not, however, go on being *untouched* by the choice, and that
// sentence stood here claiming they did until 2026-08-17 (§126). One more thing
// crosses than the command line: the spelling. A session that runs its commands
// in a distro writes its paths the way that distro writes them — into the file
// tools as much as into the shell — and a `/mnt/d/project` handed to Go on
// Windows is not an absolute path at all. So the file tools translate too, in
// the one place containment is decided (skill.resolveSandboxPath, through
// HostPath). The seam to watch is the reverse trip: what a file tool *answers*
// with is still a Windows path, and a Windows path is not usable in the shell
// this same session runs. GuestPath is the translation for that direction.

// wslBackend is one distro, addressed by name.
type wslBackend struct {
	// distro is empty when the caller asked for "the default distro". It is
	// resolved to a real name before anything uses it, because two of the four
	// Backend methods cannot answer without one: Name has nothing to show the
	// user, and HostPath cannot build the \\wsl.localhost\<name> form that
	// makes the distro's own filesystem checkable. A backend that silently
	// degrades on those is worse than one that resolves a name once.
	distro    string
	nameOnce  sync.Once
	nameCache string

	homeOnce  sync.Once
	homeCache string

	// mountRoot is where this distro mounts the Windows drives — /mnt by
	// default, but /etc/wsl.conf can move it, and people who came from Git Bash
	// habitually move it to /. Resolved lazily and once: it decides whether
	// `/d/project` is a path on drive D or a directory named d, and getting
	// that wrong means the sandbox guard checks the wrong file.
	mountOnce sync.Once
	mount     string
}

// WSL returns a backend that runs commands in the named distro. An empty name
// selects the machine's default distro.
func WSL(distro string) Backend { return &wslBackend{distro: strings.TrimSpace(distro)} }

// Distro is the resolved name, which is the one the rest of the app should show
// and store: "" is an instruction, not an answer, and a setting saved as ""
// silently follows the user's default around when they change it.
func (w *wslBackend) Distro() string {
	w.nameOnce.Do(func() {
		w.nameCache = w.distro
		if w.nameCache == "" {
			// wsl.exe -l -q lists the default distro first, and that is the only
			// place the default is available without parsing the -v table's
			// asterisk out of a column-aligned, locale-translated header.
			if all := Distros(); len(all) > 0 {
				w.nameCache = all[0]
			}
		}
	})
	return w.nameCache
}

func (w *wslBackend) Name() string {
	if distro := w.Distro(); distro != "" {
		return "bash (WSL: " + distro + ")"
	}
	return "bash (WSL)"
}

// Note says where the command starts, which is the one thing "bash (WSL: x)"
// does not tell a model that reads it.
//
// It was empty on the reasoning that "bash" is instruction enough, and that is
// true of the syntax and false of the standpoint. A model told its commands run
// in bash-in-WSL still believes it is sitting on the Windows side looking in, so
// it writes the line that would be right from there. Measured on 2026-08-17,
// mid-session, after the user switched this workspace to a distro:
//
//	wsl -d mikedev -- bash -lc 'find /mnt/d/Project/… -name ".env"'
//
// which is a correct Windows command, sent to a shell already inside mikedev.
// `wsl` is not a Linux program, so it died with `command not found` — and the
// agent read that as "this machine has no WSL", told the user it could not reach
// the distro or their D: drive at all, and handed the job back. It had been
// reading files in that distro for fifteen minutes.
//
// The second sentence is the same fact pointed at paths, and it is needed for
// the same reason: every file tool answers with the Windows spelling of a file
// (D:\project\api.py), the prompt rightly tells the model to repeat the path a
// tool gave it, and repeated into this shell that path opens nothing. Naming the
// mount form once costs a line and saves the round trip that discovers it.
//
// One fact, two sentences, and not a syntax table: this is a standpoint, not a
// case list. What generalizes is "you are already there", and everything a model
// would otherwise do to get there follows from it.
func (w *wslBackend) Note() string {
	return "The command already starts inside the distro, so run it directly — reaching for wsl.exe from here " +
		"is a program Linux does not have. Windows drives are mounted, so a file this machine calls D:\\project " +
		"is /mnt/d/project to you: translate a Windows path before using it in a command."
}

func (w *wslBackend) POSIX() bool { return true }

// Command builds the wsl.exe invocation.
//
// Unlike cmd.exe (see shell_windows.go, and the corruption it caused), wsl.exe
// parses its command line by the ordinary CommandLineToArgvW rules that
// os/exec's escaping is written for. What it then does with those arguments is
// the part that had to be measured rather than assumed.
//
// -e, not --. Both spellings look like "run exactly this", and only one is:
// `--` hands the rest to the distro's **default shell**, which reads it as a
// command line of its own before bash ever sees it, and that shell expands
// every `$…` in it first. `-e` (--exec) starts the program directly with the
// argv given, which is what this call has always meant.
//
// Measured 2026-08-17, on a plain `/bin/echo` with no shell of ours involved:
// `-- /bin/echo 'A=$X B=$HOME'` prints `A= B=/home/mikedev`. The expansion is
// real, it happens on the Linux side, and single quotes do not stop it — by the
// time bash reads the line the `$` is already gone. What that cost, before the
// flag changed:
//
//   - `awk '{print $1}'` became `awk '{print }'`, which prints the whole line.
//     No error, no clue: a column of data silently became every column.
//   - `awk '{sum+=$2}'` became `{sum+=}` — a syntax error blamed on the model.
//   - `false; echo rc=$?` reported `rc=0`, and `set -o pipefail` reported a
//     failed pipeline as 0. A failure read as success is the one direction a
//     wrapper must never be wrong in (§111.2 says the same about PowerShell's
//     exit codes; this is that fault arriving through the other backend).
//   - Every shell variable the line set itself — a `for` loop's, a `VAR=x`
//     prefix — read back empty.
//
// So the failure was not "WSL is broken", it was that a command line was being
// read twice by two different shells, and the fix is to stop the first read.
// Everything the old spelling did right — the working directory, quoting with
// spaces in it, UTF-8, pipes — is unchanged under `-e`; only the extra shell is
// gone. wsl_live_test.go holds the line: it asserts `$1` and `$?` survive,
// which is the assertion that would have caught this on the day it shipped.
//
// --cd takes the Windows directory as-is and translates it, which is exactly
// the translation this file would otherwise have to do for the one path that
// matters most. cmd.Dir is set as well so that a wsl.exe that fails to start
// fails in the right place, and so the process looks like every other child.
//
// bash -lc, not -c: a login shell is what loads the profile, and the profile is
// where nvm, pyenv, rbenv, asdf and every language manager put the toolchain on
// PATH. Without it "run this in WSL" reaches a distro that cannot find node.
//
// WSL_UTF8 is not cosmetic. wsl.exe writes its own diagnostics — "There is no
// distribution with the supplied name" and friends — in UTF-16LE unless it is
// set, which reaches the model as interleaved NUL bytes. The failure a user
// most needs to read is the one this makes readable.
func (w *wslBackend) Command(ctx context.Context, line, dir string) *exec.Cmd {
	args := make([]string, 0, 8)
	if w.distro != "" {
		args = append(args, "-d", w.distro)
	}
	if dir != "" {
		args = append(args, "--cd", dir)
	}
	args = append(args, "-e", "bash", "-lc", line)

	cmd := exec.CommandContext(ctx, "wsl.exe", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	HideConsole(cmd)
	KillOnCancel(cmd)
	return cmd
}

func (w *wslBackend) HostPath(p string) (string, bool) {
	if rest, ok := afterTilde(p); ok {
		home := w.guestHome()
		if home == "" {
			// The distro would not say where home is. Refusing beats resolving
			// the tilde to this machine's Windows profile, which is a different
			// folder that a containment check could wrongly find acceptable.
			return "", false
		}
		p = strings.TrimSuffix(home, "/") + rest
	}
	return hostFromGuest(w.Distro(), w.mountRoot(), p)
}

// afterTilde splits a leading `~` off a guest path. Only the whole segment
// counts: `~foo` is another user's home to a shell and a filename to everything
// else, and neither reading makes it this user's home directory.
func afterTilde(p string) (rest string, ok bool) {
	if p == "~" {
		return "", true
	}
	if after, found := strings.CutPrefix(p, "~/"); found {
		return "/" + after, true
	}
	return "", false
}

// guestHome asks the distro where the user's home is, once per backend.
//
// This is the one translation that cannot be done by string arithmetic: the
// home directory depends on the distro's default user, which lives in the
// Windows registry under a per-distro GUID and is the sort of thing that would
// be wrong on somebody's machine forever. Asking costs a distro start, but only
// on the first command in a session that actually writes a `~` — most sessions
// never pay it.
func (w *wslBackend) guestHome() string {
	w.homeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		out, err := w.Command(ctx, `printf %s "$HOME"`, "").Output()
		if err != nil {
			return
		}
		if home := strings.TrimSpace(string(out)); strings.HasPrefix(home, "/") {
			w.homeCache = home
		}
	})
	return w.homeCache
}

// Export adds the named variables to WSLENV, which is the only way a Windows
// environment variable reaches the Linux side. WSL does not forward the
// environment wholesale — it forwards exactly the names WSLENV lists, and drops
// everything else without a word.
//
// The names are appended to whatever WSLENV already holds rather than
// replacing it, because the variable is shared with the rest of the machine's
// WSL setup, and a session that overwrites it breaks tools the user configured
// long before Aetox. No `/p` flag on any entry: these carry payloads and
// receipts, not paths, and path translation would rewrite text that merely
// looked like one.
func (w *wslBackend) Export(cmd *exec.Cmd, names ...string) {
	if cmd == nil || len(names) == 0 {
		return
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	existing := ""
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "WSLENV="); ok {
			existing = value
			continue
		}
		kept = append(kept, entry)
	}
	seen := make(map[string]bool)
	shared := make([]string, 0, len(names)+4)
	for _, name := range append(strings.Split(existing, ":"), names...) {
		// The name may carry a flag suffix ("PATH/p"); the flag is part of the
		// entry and only the name decides whether it is already listed.
		bare, _, _ := strings.Cut(name, "/")
		if bare == "" || seen[bare] {
			continue
		}
		seen[bare] = true
		shared = append(shared, name)
	}
	cmd.Env = append(kept, "WSLENV="+strings.Join(shared, ":"))
}

// GuestPath is HostPath's inverse: the Linux spelling of a Windows path.
//
// What it is for is a tool's *receipt*. Every file tool here is Go on Windows
// and answers in Windows paths, the prompt rightly tells the model to repeat
// the path a tool handed it rather than assemble one, and a `D:\…` repeated
// into this shell opens nothing. So the one receipt that names an absolute
// path (skill.onDiskNote) carries this spelling beside it.
func (w *wslBackend) GuestPath(p string) (string, bool) {
	return guestFromHost(w.Distro(), w.mountRoot(), p)
}

func (w *wslBackend) mountRoot() string {
	w.mountOnce.Do(func() { w.mount = detectMountRoot(w.Distro()) })
	return w.mount
}

// hostFromGuest maps a path as a command line bound for WSL spells it onto the
// Windows path naming the same file.
//
// Three cases, and the third is the one that makes containment hold rather than
// leak: a path inside the distro's own filesystem is not "unknown", it is
// \\wsl.localhost\<distro>\... — a real place Windows can stat, which lands
// outside any Windows workspace root and is therefore refused by the ordinary
// rule instead of by a special case. /tmp, /etc and /home stop being holes
// without anything here having to know they are sensitive.
func hostFromGuest(distro, mountRoot, p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return p, true
	}
	// The model mixing spellings is routine, and a Windows path handed to this
	// function is already the answer. Translating it would corrupt it.
	if isWindowsPath(p) {
		return p, true
	}
	// Relative: the caller resolves it against the working directory, which is
	// the same directory in both worlds.
	if !strings.HasPrefix(p, "/") {
		return p, true
	}
	if rest, ok := underMountRoot(mountRoot, p); ok {
		drive, tail := splitFirstSegment(rest)
		if len(drive) == 1 && isDriveLetter(drive[0]) {
			host := strings.ToUpper(drive) + `:\`
			if tail != "" {
				host += strings.ReplaceAll(tail, "/", `\`)
			}
			return host, true
		}
		// Under the mount root but not a drive — /mnt/wsl and /mnt/wslg are the
		// real examples. Not a Windows path; falls through to the UNC form.
	}
	if distro == "" {
		// The UNC form needs a name, and "the default distro" has none. Refusing
		// is the honest answer: guessing a distro here would check containment
		// against a filesystem that might not be the one the command runs in.
		return "", false
	}
	unc := `\\wsl.localhost\` + distro
	if trimmed := strings.TrimPrefix(p, "/"); trimmed != "" {
		unc += `\` + strings.ReplaceAll(trimmed, "/", `\`)
	}
	return unc, true
}

// guestFromHost is hostFromGuest read backwards, with one asymmetry: a UNC path
// into a *different* distro has no spelling inside this one, so it is refused
// rather than silently pointed at this distro's own copy of that path.
func guestFromHost(distro, mountRoot, p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return p, true
	}
	if strings.HasPrefix(p, "/") {
		return p, true // already guest-spelled
	}
	slashed := strings.ReplaceAll(p, `\`, "/")
	if host, ok := strings.CutPrefix(slashed, "//wsl.localhost/"); ok {
		return uncGuestPath(distro, host)
	}
	if host, ok := strings.CutPrefix(slashed, "//wsl$/"); ok {
		return uncGuestPath(distro, host)
	}
	if len(slashed) >= 2 && isDriveLetter(slashed[0]) && slashed[1] == ':' {
		guest := strings.TrimSuffix(mountRoot, "/") + "/" + strings.ToLower(slashed[:1])
		if tail := strings.TrimPrefix(slashed[2:], "/"); tail != "" {
			guest += "/" + tail
		}
		return guest, true
	}
	// Relative, or a UNC to a file server: relative is the same in both worlds,
	// and a network share genuinely has no path inside the distro.
	if strings.HasPrefix(slashed, "//") {
		return "", false
	}
	return strings.ReplaceAll(p, `\`, "/"), true
}

func uncGuestPath(distro, rest string) (string, bool) {
	name, tail := splitFirstSegment(rest)
	if name == "" || !strings.EqualFold(name, distro) {
		return "", false
	}
	return "/" + tail, true
}

// underMountRoot reports whether a guest path lives under the drive mount point
// and returns what follows it. Written against the string rather than with a
// path helper because a mount root of "/" — which is what people who want
// /c/Users set — makes every absolute path a match, and the prefix arithmetic
// for that case is exactly what a Join-based version gets wrong.
func underMountRoot(mountRoot, p string) (string, bool) {
	root := strings.TrimSuffix(mountRoot, "/")
	if root == "" {
		return strings.TrimPrefix(p, "/"), true
	}
	if p == root {
		return "", true
	}
	if rest, ok := strings.CutPrefix(p, root+"/"); ok {
		return rest, true
	}
	return "", false
}

func splitFirstSegment(p string) (head, tail string) {
	p = strings.TrimPrefix(p, "/")
	if at := strings.IndexByte(p, '/'); at >= 0 {
		return p[:at], p[at+1:]
	}
	return p, ""
}

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isWindowsPath(p string) bool {
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//wsl") {
		return true
	}
	return len(p) >= 3 && isDriveLetter(p[0]) && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// detectMountRoot reads the distro's own /etc/wsl.conf through \\wsl.localhost
// rather than by running a command in it. Reading a file costs nothing and
// cannot fail slowly; starting a distro to ask it where it mounts C: costs the
// distro's whole boot, on the first command of a session, for a value that is
// /mnt on almost every machine.
func detectMountRoot(distro string) string {
	const fallback = "/mnt"
	if runtime.GOOS != "windows" || distro == "" {
		return fallback
	}
	file, err := os.Open(`\\wsl.localhost\` + distro + `\etc\wsl.conf`)
	if err != nil {
		return fallback
	}
	defer file.Close()
	if root := parseAutomountRoot(file); root != "" {
		return root
	}
	return fallback
}

// parseAutomountRoot pulls `root=` out of wsl.conf's [automount] section. A
// deliberately small INI reader: wsl.conf has no continuations, no quoting
// beyond the optional pair this strips, and a general parser here would be more
// code than the thing it parses.
func parseAutomountRoot(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	section := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section != "automount" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "root") {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if value == "" {
			continue
		}
		return value
	}
	return ""
}

// systemDistros are WSL installations that exist to serve another product and
// have no shell a person would work in. Docker Desktop's two are the only ones
// in the wild; listing them is a smaller lie than a heuristic that guesses from
// the name and one day hides someone's distro called "docker-dev".
var systemDistros = map[string]bool{
	"docker-desktop":      true,
	"docker-desktop-data": true,
}

// Distros lists the distros a user could sensibly pick, best-effort and in the
// order wsl.exe reports them (which puts the default first). An empty result
// means the picker offers nothing but the native shell — no WSL, no wsl.exe, or
// nothing installed but Docker's.
func Distros() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	cmd := exec.Command("wsl.exe", "-l", "-q")
	// Without this the list comes back UTF-16LE and every name reads as a
	// letter followed by a NUL.
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	HideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseDistroList(string(out))
}

func parseDistroList(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		// The NUL trim is the belt to WSL_UTF8's braces: an older wsl.exe that
		// does not honour the variable would otherwise produce one entry per
		// character.
		name := strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(line), "\x00", ""))
		if name == "" || systemDistros[strings.ToLower(name)] {
			continue
		}
		names = append(names, name)
	}
	return names
}
