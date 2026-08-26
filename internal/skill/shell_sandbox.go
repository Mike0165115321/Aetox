package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// The shell answers to the same gate every other file tool answers to.
//
// Until now it did not. `shell` set cmd.Dir to the sandbox root and handed the
// command line to the OS untouched, so `type D:\Other\file` walked straight out
// of a focused project — and the desktop runs full-access, so no prompt stood in
// the way either. The wall the UI describes was real for read/write/grep and
// imaginary for the one tool that can do everything they can. Owner: *"ปิดให้หมด
// สิครับ shell สำคัญมาก"*.
//
// What this does: find every path the command line names, apply the
// substitutions a shell would apply, and put each one through
// resolveSandboxPath — the same function, not a copy of its rules. A path
// outside the workspace refuses the command before it runs.
//
// **What it is and is not.** This is containment, not isolation. It reads the
// command as written; it cannot follow a path that only exists once the command
// is running. Claude Code draws the same line and ships both halves: permission
// rules evaluated on the command string, plus an OS sandbox (Seatbelt,
// bubblewrap) that "holds regardless of what the model chose to run". The OS
// half does not exist on native Windows, which is where Aetox runs, so this is
// the half that can be built today. Everything below is written to make the
// gap small and, where it cannot be closed, loud: a construct this cannot read
// refuses the command rather than passing it through unchecked.
//
// With one distinction, added once the cost of not making it was measured: a
// path the command *states* and a path this file *guesses* from shape are not
// the same claim, and only the first refuses on its own authority. The guess —
// a bare `/…` found inside quoted text, which is also how POSIX spells a regex
// — has to produce evidence first. evidenceForAGuess is where that is argued.
//
// **Two things it deliberately does not contain**, both stated so they are not
// mistaken for oversights. Program position: a command's first token names code
// to run, and running code is the tool's purpose — `go` resolved through PATH
// reaches a binary outside the workspace exactly as surely as its absolute
// path does, so checking the spelling would forbid the spelling and permit the
// act. And what a program does once started: a script inside the workspace can
// open any file the user can. Both need an OS boundary, not a better parser.
//
// That last rule is where this deliberately parts company with OpenCode, whose
// shell tokenizer is otherwise the better-engineered version of this idea (real
// tree-sitter grammars for bash and PowerShell, provider paths, cygpath) — but
// which does not gate on what it finds at all. Read against the source
// (source read at commit 76ced54): `tool/bash.ts` puts the
// raw command string through the permission rules and nothing else, and the
// path scan is an advisory warning about tokens outside the workspace. Its
// containment is the prompt, defaulting to `ask` when no rule matches; the
// filesystem is not walled at all. Here every token is checked whatever the
// verb, and a token that cannot be read stops the command — inside a focused
// project, where there is a wall to keep it behind.
//
// In the unfocused desktop there is no wall (sandbox_open.go's Open policy: the
// machine is the workspace), and there this rejoins OpenCode exactly. An
// unreadable token cannot escape a boundary that is not there, so guardTargets
// stops refusing it — the choice OpenCode can afford everywhere because its real
// containment is an OS sandbox, and the one native Windows cannot, which is why
// the focused project keeps the hard refusal. The credential-store lock, which
// outlives open mode, is enforced on resolved literal paths in
// resolveSandboxPath, not by this parser.

// shellGate is what this file needs to know about the shell a command line is
// bound for. It exists because the two facts below used to be read off
// runtime.GOOS, which was the same thing only for as long as a machine had one
// shell. A Windows machine running the agent's commands in a WSL distro breaks
// the identity in both directions at once: the line is bash on an OS where the
// native shell is cmd, and the paths in it are Linux ones naming Windows files.
// Reading GOOS there does not merely mis-parse — it silently stops containing,
// because `/mnt/d/elsewhere` matches no pattern the Windows branch looks for.
type shellGate struct {
	// posix decides how the line reads: whether `\` escapes or separates
	// directories, whether a bare `/…` opens a path or a cmd switch.
	posix bool
	// toHost turns a path spelled the way this shell spells it into the path
	// Windows knows the same file by, so the containment check compares two
	// spellings of one place rather than one spelling of two.
	toHost func(string) (string, bool)
}

func gateFor(backend proc.Backend) shellGate {
	if backend == nil {
		return nativeGate()
	}
	return shellGate{posix: backend.POSIX(), toHost: backend.HostPath}
}

// nativeGate is the gate for a command this process runs itself rather than
// handing to a selectable shell — git, and every test that predates backends.
func nativeGate() shellGate { return gateFor(proc.Native()) }

// guardCommandPaths refuses a command line that names anything outside the
// workspace. The returned error is written for the model: it says which path
// was refused and what the user can do about it, because a refusal that does
// not carry the remedy just becomes "I can't".
func guardCommandPaths(root, commandLine string, gate shellGate) error {
	targets, guesses, opaque := commandTargets(root, commandLine, gate)
	return guardTargets(root, targets, guesses, opaque, gate)
}

// openWorkspace reports whether root's session may reach the whole machine —
// the unfocused desktop (sandbox_open.go's Open policy). It is the one condition
// under which guardTargets stops hard-refusing a construct it cannot read: with
// no wall to guard, an unreadable token is not an escape from one.
//
// Keyed the way resolveSandboxPath keys it — abs of the trimmed root — so the
// two agree about which root a policy belongs to. A root that cannot be made
// absolute answers false: the safe direction is to keep the wall.
func openWorkspace(root string) bool {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return false
	}
	return sandboxPolicyFor(safeRoot).open
}

// guardArgs is guardCommandPaths for a caller that already holds its arguments
// as a list — `git`, which builds an argv rather than a command line, so there
// is nothing to tokenize and nothing to guess about where one argument ends.
//
// It exists so git answers to this gate instead of the second, smaller denylist
// it grew of its own (validateGitReadArgs). Two gates with different rules is
// the shape that drifts: that one blocks `-C` and `--git-dir` by name and lets
// `git diff --output=D:\elsewhere` through, because nobody thought of it. This
// one does not need to think of it.
//
// The native gate, always: git is run by this process through exec.Command, so
// its arguments are read by nothing and its paths are Windows paths whatever
// shell the user picked for the shell tool.
func guardArgs(root string, args []string) error {
	gate := nativeGate()
	var targets []string
	for _, arg := range args {
		if reason := unreadable(arg); reason != "" {
			return guardTargets(root, nil, nil, reason, gate)
		}
		target, opaque := tokenTarget(root, arg, gate)
		if opaque != "" {
			return guardTargets(root, nil, nil, opaque, gate)
		}
		if target != "" {
			targets = append(targets, target)
		}
	}
	// No guesses: every argument here is one the caller means as an argument.
	// The guess list exists for text found by shape inside a command line, and
	// an argv has no such text.
	return guardTargets(root, targets, nil, "", gate)
}

// guardTargets refuses the command when any target leaves the workspace.
//
// targets are the paths the command says it uses: a token the shell would pass
// as an argument, or a path written in a spelling only a filesystem could mean.
// guesses are the ones this file inferred from shape alone — a `/…` found
// inside quoted text — and they are held to a different standard, stated in
// evidenceForAGuess.
func guardTargets(root string, targets, guesses []string, opaque string, gate shellGate) error {
	// A construct this scanner cannot read — a variable holding a path, a
	// $(sub-command) — stops the command, but only where there is a wall for it
	// to have jumped. In the open workspace there is none: resolveSandboxPath
	// lets any absolute path through, so an unreadable token is not a way around
	// a boundary, because there is no boundary. Refusing it there guards nothing
	// and breaks almost every real PowerShell script, which is built on
	// variables — the same "this mode is useless" that unfocusedRoot's own note
	// records for file tools, one enforcement path over (owner's call,
	// 2026-08-11, after confirming this is exactly how OpenCode's scanner
	// behaves: its path check is advisory and skips dynamic tokens, because its
	// real containment is an OS sandbox — which native Windows has not got, so
	// a focused project keeps this hard refusal as its only wall).
	//
	// The one thing open mode still protects — the credential stores — does not
	// live here. It is enforced on the resolved literal path inside
	// resolveSandboxPath, so `cat C:\Users\x\.ssh\id_rsa` is still refused; only
	// a path assembled entirely at runtime slips, which an opaque token was
	// never able to check anyway. That residual is stated, not hidden: it is the
	// price of a mode that works, and it is the same defence-in-depth line
	// OpenCode draws.
	if opaque != "" && !openWorkspace(root) {
		return fmt.Errorf("this command builds a path while it runs (%s), so it cannot be checked "+
			"against the folders this session may use, write the path out literally, or run the "+
			"step that needs it as its own command", opaque)
	}
	for _, target := range targets {
		// Translated here, at the one point where a path stops being text in a
		// command line and becomes a place on disk — so every producer above
		// (tokens, embedded paths, expanded variables) is translated exactly
		// once, and none of them has to know which shell it was written for.
		host, ok := gate.toHost(target)
		if !ok {
			return fmt.Errorf("%s: this path is inside the shell's own filesystem and has no name "+
				"this machine can check it by, so it cannot be shown to be inside the folders this "+
				"session may use, work inside the project folder instead", target)
		}
		// The refusal names the path as the model wrote it, not as it was
		// translated: being told that `\\wsl.localhost\mikedev\etc\passwd` was
		// refused, having written `/etc/passwd`, reads as a different failure.
		if _, err := resolveSandboxPath(root, host); err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
	}
	// The same checks, and one extra step before a guess is allowed to refuse
	// anything. Run second so a command that names a real path outside the
	// workspace is refused for that, and never spends a filesystem lookup on a
	// guess it was going to be refused for anyway.
	for _, guess := range guesses {
		host, ok := gate.toHost(guess)
		if !ok {
			if !evidenceForAGuess(guess, gate) {
				continue
			}
			return fmt.Errorf("%s: this path is inside the shell's own filesystem and has no name "+
				"this machine can check it by, so it cannot be shown to be inside the folders this "+
				"session may use, work inside the project folder instead", guess)
		}
		if _, err := resolveSandboxPath(root, host); err != nil {
			if !evidenceForAGuess(guess, gate) {
				continue
			}
			return fmt.Errorf("%s: %w", guess, err)
		}
	}
	return nil
}

// evidenceForAGuess asks whether a `/…` found by shape is a path at all.
//
// The scan that produces these exists to close a real family — `python3 -c
// "open('/etc/passwd')"` is one token to any tokenizer and an absolute path to
// the operating system — and it pays for that with the other thing a `/` is:
// POSIX's most common delimiter. `sed 's/^/n=/'`, `sed 's/ /_/g'` and `sed -E
// 's/(a)-(b)/\2\1/'` all contain something the pattern reads as an absolute
// path, and all three were refused (owner, 17 ส.ค., after the WSL check: a
// wall that stops ordinary work is still a wall).
//
// The line drawn here: **a token the command passes is checked as written; a
// guess must produce evidence before it may refuse.** The evidence is the
// cheapest fact that separates the two — whether the first segment names
// something that is actually there. `/etc`, `/home`, `/mnt`, `/tmp` exist on
// every distro, so every real escape keeps its refusal; `/n=`, `/_` and `/\2\1`
// exist nowhere, so the delimiters stop costing the user a command.
//
// Failure is closed in both directions that matter. A filesystem this process
// cannot reach — a distro that is not installed, a name that never resolves —
// answers "keep the refusal", because "I could not look" must never read as "it
// is not there". And this only ever *removes* a refusal the shape produced: a
// path that resolves inside the workspace was never going to be refused, and a
// literal argument never reaches this function at all.
//
// The residual, stated rather than hidden: text that is genuinely
// indistinguishable from a real path stays refused. `grep -E '^/usr'` names
// /usr, which exists, and nothing here can tell a pattern that matches a path
// from a path.
func evidenceForAGuess(guess string, gate shellGate) bool {
	if !strings.HasPrefix(guess, "/") {
		// Only the bare-slash family is guessed, and only it is answered here.
		return true
	}
	segment, _ := firstGuestSegment(guess)
	if segment == "" {
		return true
	}
	// Reachability first, and asked of the root rather than of the segment: a
	// filesystem that answers nothing would otherwise report every segment as
	// missing, and turn "cannot look" into "let it through" for the whole scan.
	root, ok := gate.toHost("/")
	if !ok {
		return true
	}
	if _, err := os.Stat(root); err != nil {
		return true
	}
	host, ok := gate.toHost("/" + segment)
	if !ok {
		return true
	}
	if _, err := os.Stat(host); err != nil {
		return false
	}
	return true
}

func firstGuestSegment(p string) (head, tail string) {
	p = strings.TrimPrefix(p, "/")
	if at := strings.IndexByte(p, '/'); at >= 0 {
		return p[:at], p[at+1:]
	}
	return p, ""
}

// commandTargets returns every path-shaped token in the command line, expanded
// the way the shell would expand it, plus a description of the first construct
// it could not read (empty when it read everything).
//
// Two lists, because they are not the same claim. targets is what the command
// says: a token the shell would hand to the program, or a path in a spelling
// nothing but a filesystem uses. guesses is what a bare `/…` inside quoted text
// might be — the family evidenceForAGuess exists for.
func commandTargets(root, commandLine string, gate shellGate) (targets, guesses []string, opaque string) {
	// Against the whole line, before tokenizing: `$(` straddles a token break
	// (the tokenizer treats `(` as a separator), so a per-token scan would
	// never see the two characters together and command substitution would be
	// the one unreadable construct that reads as fine.
	if reason := unreadable(commandLine); reason != "" {
		return nil, nil, reason
	}
	// Tokenizing alone leaves one family wide open: a path that never appears
	// as a token because it lives inside a quoted argument. `python -c
	// "open('C:/Users/me/.ssh/id_rsa').read()"` is one token to any tokenizer
	// and an absolute path to the operating system, and the same is true of
	// `node -e`, `sed` scripts and `powershell -Command`. Reading inside those
	// strings properly would mean an interpreter per language; recognising the
	// shape of a path in them costs a scan and closes the family.
	segments := shellSegments(commandLine, gate)
	// The scan reads the raw line, so it also finds the program names the loop
	// below deliberately skips. Dropping them here keeps the two halves saying
	// the same thing. Only the scan's results are filtered: a program path that
	// appears AGAIN as an argument is still caught, by the loop, as an argument.
	programs := make(map[string]bool, len(segments))
	for _, segment := range segments {
		if len(segment) > 0 {
			programs[literalPathPrefix(expandTilde(segment[0], gate), gate)] = true
		}
	}
	spelled, guessed := embeddedPaths(commandLine, gate)
	for _, found := range spelled {
		if !programs[found] {
			targets = append(targets, found)
		}
	}
	for _, found := range guessed {
		if !programs[found] {
			guesses = append(guesses, found)
		}
	}
	for _, segment := range segments {
		// Token 0 is the program, and a program is not a file the command
		// touches — it is a name the OS resolves, usually through PATH, to
		// something that was never inside the workspace anyway. `go`, `npm` and
		// `git` all live outside it; refusing `C:\Program Files\Go\bin\go.exe`
		// while allowing `go` would forbid the spelling and permit the act.
		// Nothing is lost: putting a secret in program position executes it, it
		// does not read it, and every argument after this IS checked.
		for _, token := range segment[min(1, len(segment)):] {
			if strings.EqualFold(token, "iex") {
				// Only ever as a whole word: three letters, and refusing every
				// command that contains them somewhere would be absurd.
				return nil, nil, "Invoke-Expression"
			}
			target, reason := tokenTarget(root, token, gate)
			if reason != "" {
				return nil, nil, reason
			}
			if target != "" {
				targets = append(targets, target)
			}
		}
	}
	return targets, guesses, ""
}

// tokenTarget reduces one already-split token to the path it names, or "" when
// it names none. Shared by the command-line and argv paths so both apply the
// same rules to a token — the flag prefix, the variable expansion, the glob.
func tokenTarget(root, token string, gate shellGate) (target string, opaque string) {
	token = stripFlagPrefix(token)
	if token == "" || isNullDevice(token) {
		return "", ""
	}
	expanded, reason := expandToken(root, token, gate)
	if reason != "" {
		return "", reason
	}
	// A glob names a directory plus a pattern; the directory is the part that
	// can leave the workspace, and it is the part a wildcard cannot hide.
	// `del D:\Other\*` is a path question about D:\Other.
	return literalPathPrefix(expanded, gate), ""
}

// embeddedPathPatterns find a path by its shape anywhere in the line, quoted or
// not. Each requires a boundary in front so the shape has to start where a path
// could start:
//
//   - a drive letter keeps "https://host" out, whose ":" is preceded by a letter
//   - the ".." form excludes a leading "." so `go test ./...` is not a climb
//   - the bare "/" form (POSIX shells only — to cmd a leading "/" is a switch)
//     excludes a preceding ":" so a URL's "//" is not an absolute path
var embeddedPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|[^A-Za-z0-9_])([A-Za-z]:[\\/][^\s"'` + "`" + `<>|]*)`),
	regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])(\.\.[\\/][^\s"'` + "`" + `<>|]*)`),
	regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(~[\\/][^\s"'` + "`" + `<>|]*)`),
}

// unixEmbeddedPath is the bare "/" form, and it carries one exclusion the other
// three do not need: a `/` straight after a glob or regex metacharacter.
//
// An absolute path cannot start there. `^`, `$`, `*`, `+`, `?`, `(`, `)`, `[`,
// `]`, `{` and `}` are how POSIX writes a pattern, and the `/` that follows one
// is the pattern's next character — `grep -E '^/usr'` names no file. Nothing is
// let out by this: to reach /etc/passwd the literal text has to be there, and
// putting a metacharacter immediately in front of it makes it a different,
// relative path (`*/etc/passwd`) rather than a hidden absolute one. The two
// constructs where such a character really can precede a path, `${…}` and
// `$(…)`, never reach this scan — unreadable() stops the command first.
//
// `\` is deliberately not in the set: to a POSIX shell `\/` is an escaped
// slash, so `cat \/etc/passwd` really does open /etc/passwd.
var unixEmbeddedPath = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.:/^$*+?()\[\]{}])(/[^\s"'` + "`" + `<>|]*)`)

// embeddedPaths returns the paths written into the line, and separately the
// ones only guessed from a bare `/`.
//
// The split is the whole point. A drive letter, a `..` climb and a `~` are
// shapes nothing but a path uses, so finding one is finding a path. A `/` is
// also how POSIX writes a regex, a field separator and a substitution, so
// finding one is finding a maybe — and a maybe answers to evidenceForAGuess
// before it refuses anybody's `sed`.
func embeddedPaths(line string, gate shellGate) (spelled, guessed []string) {
	reduce := func(candidate string) string {
		candidate = strings.Trim(candidate, `"'`+"`"+`,;:)]}`)
		if candidate == "" || isNullDevice(candidate) {
			return ""
		}
		// Same reductions a token gets, minus the flag prefix — this text
		// was never a flag, it was found inside one.
		return literalPathPrefix(expandTilde(candidate, gate), gate)
	}
	for _, re := range embeddedPathPatterns {
		for _, match := range re.FindAllStringSubmatch(line, -1) {
			if literal := reduce(match[1]); literal != "" {
				spelled = append(spelled, literal)
			}
		}
	}
	// Keyed on the shell, not the OS: this is the pattern that finds
	// `/mnt/d/elsewhere` inside a quoted argument, and a WSL command line on
	// Windows is exactly where that path shape turns up.
	if !gate.posix {
		return spelled, nil
	}
	for _, match := range unixEmbeddedPath.FindAllStringSubmatchIndex(line, -1) {
		start, end := match[2], match[3]
		if !slashCouldOpenAPath(line, start) {
			continue
		}
		literal := reduce(line[start:end])
		// A lone `/` found by shape is a delimiter, not the root of the
		// filesystem. Dropped here and nowhere else: a `/` the shell would pass
		// as an argument arrives as a token, so `rm -rf /` is refused by the
		// token loop exactly as before — and so, unavoidably, is `tr '/' '-'`,
		// since after quote-stripping the two are the same argument.
		if literal == "" || literal == "/" {
			continue
		}
		guessed = append(guessed, literal)
	}
	return spelled, guessed
}

// slashCouldOpenAPath rejects the one position where a `/` provably is not the
// start of one: straight after a `#!`.
//
// A shebang names an interpreter, and program position is the thing this file
// has always declined to contain (see the top of this file: refusing the
// spelling would forbid the spelling and permit the act). Without this,
// `echo '#!/usr/bin/env bash' > script.sh` and every heredoc that writes a
// script are refused for naming /usr/bin/env — a path the command does not open
// and the file, once run, resolves for itself.
func slashCouldOpenAPath(line string, at int) bool {
	return at < 2 || line[at-2:at] != "#!"
}

// isNullDevice matches the two spellings of "throw this output away". Neither
// is a place on disk, and NUL in particular is a reserved Windows device that
// symlink resolution reports as living outside every directory — so left alone
// it refuses `ping -n 2 127.0.0.1 >nul` as a sandbox escape.
//
// Asked by the embedded-path scan as well as by the token loop, because the
// scan is where `/dev/null` turns up: as a bare absolute path it is only looked
// for under a POSIX shell, so on Windows this never came up until a WSL command
// line arrived and `cat /dev/null` started being refused as an escape into the
// distro's filesystem.
func isNullDevice(token string) bool {
	switch strings.ToLower(strings.Trim(token, `"'`)) {
	case "nul", "/dev/null", `\dev\null`:
		return true
	}
	return false
}

// unreadableConstructs make a command's paths unknowable before it runs. Kept
// short and literal for the same reason credentialStores is: a fuzzy heuristic
// here would refuse ordinary commands, and the whole value of the list is that
// a reader can tell at a glance what it costs.
//
// Command substitution and eval are the honest cases — the path genuinely does
// not exist yet. The encoded-command forms are the dishonest ones: nothing that
// needs base64 to say what it runs is being written for a human to read.
var unreadableConstructs = []struct {
	needle string
	what   string
}{
	{"$(", "$(...) command substitution"},
	{"`", "backtick command substitution"},
	{"${", "${...} expansion"},
	{"-encodedcommand", "-EncodedCommand"},
	{"-enc ", "-enc"},
	{"frombase64string", "FromBase64String"},
	{"invoke-expression", "Invoke-Expression"},
}

func unreadable(line string) string {
	lowered := strings.ToLower(line)
	for _, c := range unreadableConstructs {
		if strings.Contains(lowered, c.needle) {
			return c.what
		}
	}
	return ""
}

// stripFlagPrefix turns "--out=D:\x" and cmd's "/f:D:\x" into "D:\x". Without
// it a path attached to a flag reads as a flag and sails through: the token
// does not start with a drive letter, so nothing about it looks absolute.
func stripFlagPrefix(token string) string {
	if rest, ok := afterFlagSeparator(token, '='); ok {
		return rest
	}
	// The ":" form only for tokens that open with a flag marker — otherwise
	// "C:\Users\me" is a flag named C carrying the path "\Users\me", and the
	// drive letter is exactly what makes it absolute.
	if strings.HasPrefix(token, "-") || strings.HasPrefix(token, "/") {
		if rest, ok := afterFlagSeparator(token, ':'); ok {
			return rest
		}
	}
	return token
}

func afterFlagSeparator(token string, sep byte) (string, bool) {
	at := strings.IndexByte(token, sep)
	if at <= 0 {
		return "", false
	}
	// A flag name ("--out", "/f") or a leading assignment ("NODE_ENV=test"),
	// never something that merely contains the separator — a path can hold one.
	for _, r := range token[:at] {
		if r == '-' || r == '_' || r == '/' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return "", false
	}
	return token[at+1:], true
}

// homeVars are the variable names that name a folder, in every spelling the two
// shells accept. Only these are expanded: an unknown variable with a path
// separator after it is refused instead (expandToken), because guessing it is
// empty would turn "%SOMEWHERE%\file" into a harmless-looking relative path and
// let it through.
var homeVars = map[string]string{
	"USERPROFILE": "", "HOME": "", "HOMEPATH": "", "HOMEDRIVE": "",
	"APPDATA": "", "LOCALAPPDATA": "", "TEMP": "", "TMP": "", "PWD": "",
}

// expandToken applies the substitutions a shell makes before a path reaches the
// filesystem: `~`, `%VAR%` (cmd), `$VAR` and `$env:VAR` (PowerShell/sh).
//
// A variable only matters when it is the head of the token, because that is the
// only position where it decides which folder the path is in. Anywhere else it
// is text — `git log --pretty=format:%H%n` must not be mistaken for a path
// built out of a variable named H.
func expandToken(root, token string, gate shellGate) (string, string) {
	head, rest, ok := splitLeadingVar(token, gate)
	if !ok {
		return expandTilde(token, gate), ""
	}
	value, known := lookupPathVar(root, head, gate)
	if !known {
		// Whether an unknown variable matters is decided by what follows it.
		// With no separator after the name the token cannot name a folder,
		// whatever the variable holds — `$PSVersionTable.PSVersion.ToString()`
		// is an expression, and refusing it blocked every PowerShell command
		// that starts with one (found 2026-08-07). `$V\file` and `$V=...\...`
		// still stop the command: there a path IS being built on a value this
		// check cannot see.
		if !strings.ContainsAny(rest, `/\`) {
			return "", ""
		}
		return "", "the variable " + head + " in " + token
	}
	return joinExpanded(value, rest, gate), ""
}

// joinExpanded puts a token back together after its head was substituted.
//
// filepath.Join would be wrong under a POSIX shell for the same reason the rest
// of this file's GOOS checks were: it produces backslashes, and the result goes
// on to be read as a Linux path. Worse, it would flatten the "~" that HOME
// resolves to below into a relative-looking `~\x` that nothing downstream
// recognises as the home directory any more.
func joinExpanded(value, rest string, gate shellGate) string {
	rest = strings.TrimLeft(rest, `/\`)
	if !gate.posix {
		return filepath.Join(value, filepath.FromSlash(rest))
	}
	if rest == "" {
		return value
	}
	return strings.TrimSuffix(value, "/") + "/" + rest
}

// splitLeadingVar recognises %NAME%, $env:NAME, ${env:NAME} and $NAME at the
// start of a token and returns the name plus what follows it.
func splitLeadingVar(token string, gate shellGate) (name, rest string, ok bool) {
	switch {
	case strings.HasPrefix(token, "%"):
		// %NAME% is cmd and PowerShell; to a POSIX shell those are literal
		// percent signs, and reading them as a variable would resolve a token
		// the shell never resolves — checking a path the command never touches
		// while missing the one it does.
		if gate.posix {
			return "", "", false
		}
		if end := strings.IndexByte(token[1:], '%'); end > 0 {
			return token[1 : 1+end], token[end+2:], true
		}
	case strings.HasPrefix(strings.ToLower(token), "$env:"):
		name, rest := splitVarName(token[5:])
		return name, rest, name != ""
	case strings.HasPrefix(token, "$"):
		name, rest := splitVarName(token[1:])
		return name, rest, name != ""
	}
	return "", "", false
}

func splitVarName(s string) (string, string) {
	for i, r := range s {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return s[:i], s[i:]
	}
	return s, ""
}

// lookupPathVar answers only for the names in homeVars, and answers from the
// real environment so the check sees the same folder the shell would.
func lookupPathVar(root, name string, gate shellGate) (string, bool) {
	upper := strings.ToUpper(name)
	if _, listed := homeVars[upper]; !listed {
		return "", false
	}
	if upper == "PWD" {
		// The one variable whose value is the same folder in both worlds,
		// because it is the folder the command was started in.
		return root, true
	}
	if gate.posix {
		// This process's environment is the Windows one, and under a POSIX
		// shell somewhere else these names hold that shell's values, not these.
		// $HOME is answered with the tilde so the shell's own translation
		// resolves it (see the WSL backend's HostPath) — and the rest are
		// refused rather than answered with a Windows folder that would put the
		// containment check on the wrong side of the wall in either direction.
		if upper == "HOME" {
			return "~", true
		}
		return "", false
	}
	if value := os.Getenv(upper); strings.TrimSpace(value) != "" {
		return value, true
	}
	// HOME on Windows and USERPROFILE on Unix are commonly unset; both name the
	// same folder, and os.UserHomeDir knows it per platform.
	if upper == "HOME" || upper == "USERPROFILE" {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			return home, true
		}
	}
	// Listed but empty in this environment. Refusing beats treating it as "",
	// which would silently rewrite the path to something else entirely.
	return "", false
}

func expandTilde(token string, gate shellGate) string {
	if token != "~" && !strings.HasPrefix(token, "~/") && !strings.HasPrefix(token, `~\`) {
		return token
	}
	// Under a POSIX shell the tilde is the *shell's* home, which on WSL is a
	// Linux path, not this process's Windows one. Left as written so the
	// backend's own translation answers it — expanding it here would check
	// C:\Users\<me> while the command reads /home/<me>.
	if gate.posix {
		return token
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return token
	}
	if token == "~" {
		return home
	}
	return filepath.Join(home, filepath.FromSlash(token[2:]))
}

// literalPathPrefix reduces a wildcard token to the literal path in front of it, which
// is the part containment is about. A token that is nothing but a pattern
// ("*.go") has no directory in it and needs no check.
func literalPathPrefix(token string, gate shellGate) string {
	if idx := strings.IndexAny(token, "*?["); idx >= 0 {
		if idx == 0 {
			return ""
		}
		token = token[:idx]
	}
	// In a Windows shell a leading backslash means "root of the current drive",
	// which filepath.IsAbs does not consider absolute — so `type
	// \Users\me\.ssh\id_rsa` would otherwise be joined onto the sandbox root and
	// read as a path inside it. A leading FORWARD slash is left alone on
	// purpose: in cmd that is a switch, not a path (`dir /s`, `for /L`), and
	// treating it as drive-relative refuses half the commands anyone writes on
	// Windows.
	//
	// Not applied to a POSIX shell even when this process is on Windows: there
	// a leading backslash is an escape character, and the absolute path is the
	// one that starts with the forward slash — which is the pattern above.
	if !gate.posix && runtime.GOOS == "windows" && len(token) > 0 && token[0] == '\\' && !strings.HasPrefix(token, `\\`) {
		if vol := filepath.VolumeName(mustAbs(token)); vol != "" {
			token = vol + token
		}
	}
	return strings.TrimSpace(token)
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// shellSegments splits a command line into its separate commands, each as a
// list of tokens, for the one purpose of finding the paths in it: quotes hold a
// token together, whitespace ends one, and a command separator starts a new
// segment. Redirections stay inside their segment, because `> D:\out.txt`
// leaves the workspace exactly as surely as `cat D:\secret` enters another —
// and unlike the program name, a redirect target is a file being written.
//
// Not a shell parser and not trying to be. It errs toward producing MORE tokens
// than a shell would, because an extra token costs a containment check that
// passes, while a missing one costs a path nobody looked at.
func shellSegments(line string, gate shellGate) [][]string {
	var segments [][]string
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	// A separator means the next token is a program name again, so segments
	// have to break where the shell breaks them — otherwise `git status && cat
	// D:\secret` would treat `cat` as an argument and `D:\secret` as safe by
	// association.
	breakSegment := func() {
		flush()
		if len(tokens) > 0 {
			segments = append(segments, tokens)
			tokens = nil
		}
	}
	var quote rune
	// To cmd `\` is the path separator, so treating it as an escape would eat
	// every backslash in every path — the opposite of the goal. To bash it
	// really is an escape, and that stays true when the bash in question is a
	// WSL distro on a Windows machine.
	escapes := gate.posix

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			if escapes && r == '\\' && i+1 < len(runes) {
				i++
				current.WriteRune(runes[i])
				continue
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\r':
			flush()
		case r == '\n' || strings.ContainsRune("|&;()", r):
			breakSegment()
		case r == '<' || r == '>':
			// Ends the token, keeps the segment: what follows is a file this
			// command reads or writes, not a new program.
			flush()
		default:
			current.WriteRune(r)
		}
	}
	breakSegment()
	return segments
}
