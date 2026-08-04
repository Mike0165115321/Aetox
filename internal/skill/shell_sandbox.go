package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
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
// **Two things it deliberately does not contain**, both stated so they are not
// mistaken for oversights. Program position: a command's first token names code
// to run, and running code is the tool's purpose — `go` resolved through PATH
// reaches a binary outside the workspace exactly as surely as its absolute
// path does, so checking the spelling would forbid the spelling and permit the
// act. And what a program does once started: a script inside the workspace can
// open any file the user can. Both need an OS boundary, not a better parser.
//
// That last rule is where this deliberately parts company with OpenCode, whose
// shell tool is otherwise the better-engineered version of this idea (real
// tree-sitter grammars for bash and PowerShell, provider paths, cygpath). It
// checks paths only for arguments of a known verb list (`FILES`), and skips any
// token whose value is dynamic. Both are quiet holes: `findstr`, `python -c`
// and `Get-Content` are not on anyone's verb list, and a token skipped because
// it was hard to read is a permission granted by the parser's limitations. Here
// every token is checked whatever the verb, and a token that cannot be read
// stops the command.

// guardCommandPaths refuses a command line that names anything outside the
// workspace. The returned error is written for the model: it says which path
// was refused and what the user can do about it, because a refusal that does
// not carry the remedy just becomes "I can't".
func guardCommandPaths(root, commandLine string) error {
	targets, opaque := commandTargets(root, commandLine)
	return guardTargets(root, targets, opaque)
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
func guardArgs(root string, args []string) error {
	var targets []string
	for _, arg := range args {
		if reason := unreadable(arg); reason != "" {
			return guardTargets(root, nil, reason)
		}
		target, opaque := tokenTarget(root, arg)
		if opaque != "" {
			return guardTargets(root, nil, opaque)
		}
		if target != "" {
			targets = append(targets, target)
		}
	}
	return guardTargets(root, targets, "")
}

func guardTargets(root string, targets []string, opaque string) error {
	if opaque != "" {
		return fmt.Errorf("this command builds a path while it runs (%s), so it cannot be checked "+
			"against the folders this session may use — write the path out literally, or run the "+
			"step that needs it as its own command", opaque)
	}
	for _, target := range targets {
		if _, err := resolveSandboxPath(root, target); err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
	}
	return nil
}

// commandTargets returns every path-shaped token in the command line, expanded
// the way the shell would expand it, plus a description of the first construct
// it could not read (empty when it read everything).
func commandTargets(root, commandLine string) (targets []string, opaque string) {
	// Against the whole line, before tokenizing: `$(` straddles a token break
	// (the tokenizer treats `(` as a separator), so a per-token scan would
	// never see the two characters together and command substitution would be
	// the one unreadable construct that reads as fine.
	if reason := unreadable(commandLine); reason != "" {
		return nil, reason
	}
	// Tokenizing alone leaves one family wide open: a path that never appears
	// as a token because it lives inside a quoted argument. `python -c
	// "open('C:/Users/me/.ssh/id_rsa').read()"` is one token to any tokenizer
	// and an absolute path to the operating system, and the same is true of
	// `node -e`, `sed` scripts and `powershell -Command`. Reading inside those
	// strings properly would mean an interpreter per language; recognising the
	// shape of a path in them costs a scan and closes the family.
	segments := shellSegments(commandLine)
	// The scan reads the raw line, so it also finds the program names the loop
	// below deliberately skips. Dropping them here keeps the two halves saying
	// the same thing. Only the scan's results are filtered: a program path that
	// appears AGAIN as an argument is still caught, by the loop, as an argument.
	programs := make(map[string]bool, len(segments))
	for _, segment := range segments {
		if len(segment) > 0 {
			programs[literalPathPrefix(expandTilde(segment[0]))] = true
		}
	}
	for _, found := range embeddedPaths(commandLine) {
		if !programs[found] {
			targets = append(targets, found)
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
				return nil, "Invoke-Expression"
			}
			target, reason := tokenTarget(root, token)
			if reason != "" {
				return nil, reason
			}
			if target != "" {
				targets = append(targets, target)
			}
		}
	}
	return targets, ""
}

// tokenTarget reduces one already-split token to the path it names, or "" when
// it names none. Shared by the command-line and argv paths so both apply the
// same rules to a token — the flag prefix, the variable expansion, the glob.
func tokenTarget(root, token string) (target string, opaque string) {
	token = stripFlagPrefix(token)
	if token == "" || isNullDevice(token) {
		return "", ""
	}
	expanded, reason := expandToken(root, token)
	if reason != "" {
		return "", reason
	}
	// A glob names a directory plus a pattern; the directory is the part that
	// can leave the workspace, and it is the part a wildcard cannot hide.
	// `del D:\Other\*` is a path question about D:\Other.
	return literalPathPrefix(expanded), ""
}

// embeddedPathPatterns find a path by its shape anywhere in the line, quoted or
// not. Each requires a boundary in front so the shape has to start where a path
// could start:
//
//   - a drive letter keeps "https://host" out, whose ":" is preceded by a letter
//   - the ".." form excludes a leading "." so `go test ./...` is not a climb
//   - the bare "/" form (non-Windows only — on Windows a leading "/" is a cmd
//     switch) excludes a preceding ":" so a URL's "//" is not an absolute path
var embeddedPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|[^A-Za-z0-9_])([A-Za-z]:[\\/][^\s"'` + "`" + `<>|]*)`),
	regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])(\.\.[\\/][^\s"'` + "`" + `<>|]*)`),
	regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(~[\\/][^\s"'` + "`" + `<>|]*)`),
}

var unixEmbeddedPath = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.:/])(/[^\s"'` + "`" + `<>|]*)`)

func embeddedPaths(line string) []string {
	patterns := embeddedPathPatterns
	if runtime.GOOS != "windows" {
		patterns = append(append([]*regexp.Regexp{}, patterns...), unixEmbeddedPath)
	}
	var out []string
	for _, re := range patterns {
		for _, match := range re.FindAllStringSubmatch(line, -1) {
			candidate := strings.Trim(match[1], `"'`+"`"+`,;:)]}`)
			if candidate == "" {
				continue
			}
			// Same reductions a token gets, minus the flag prefix — this text
			// was never a flag, it was found inside one.
			if literal := literalPathPrefix(expandTilde(candidate)); literal != "" {
				out = append(out, literal)
			}
		}
	}
	return out
}

// isNullDevice matches the two spellings of "throw this output away". Neither
// is a place on disk, and NUL in particular is a reserved Windows device that
// symlink resolution reports as living outside every directory — so left alone
// it refuses `ping -n 2 127.0.0.1 >nul` as a sandbox escape.
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
// shells accept. Only these are expanded: an unknown variable in path position
// is refused instead (expandToken), because guessing it is empty would turn
// "%SOMEWHERE%\file" into a harmless-looking relative path and let it through.
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
func expandToken(root, token string) (string, string) {
	head, rest, ok := splitLeadingVar(token)
	if !ok {
		return expandTilde(token), ""
	}
	value, known := lookupPathVar(root, head)
	if !known {
		return "", "the variable " + head + " in " + token
	}
	return filepath.Join(value, filepath.FromSlash(strings.TrimLeft(rest, `/\`))), ""
}

// splitLeadingVar recognises %NAME%, $env:NAME, ${env:NAME} and $NAME at the
// start of a token and returns the name plus what follows it.
func splitLeadingVar(token string) (name, rest string, ok bool) {
	switch {
	case strings.HasPrefix(token, "%"):
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
func lookupPathVar(root, name string) (string, bool) {
	upper := strings.ToUpper(name)
	if _, listed := homeVars[upper]; !listed {
		return "", false
	}
	if upper == "PWD" {
		return root, true
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

func expandTilde(token string) string {
	if token != "~" && !strings.HasPrefix(token, "~/") && !strings.HasPrefix(token, `~\`) {
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
func literalPathPrefix(token string) string {
	if idx := strings.IndexAny(token, "*?["); idx >= 0 {
		if idx == 0 {
			return ""
		}
		token = token[:idx]
	}
	// On Windows a leading backslash means "root of the current drive", which
	// filepath.IsAbs does not consider absolute — so `type \Users\me\.ssh\id_rsa`
	// would otherwise be joined onto the sandbox root and read as a path inside
	// it. A leading FORWARD slash is left alone on purpose: in cmd that is a
	// switch, not a path (`dir /s`, `for /L`), and treating it as drive-relative
	// refuses half the commands anyone writes on Windows.
	if runtime.GOOS == "windows" && len(token) > 0 && token[0] == '\\' && !strings.HasPrefix(token, `\\`) {
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
func shellSegments(line string) [][]string {
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
	// On Windows `\` is the path separator, so treating it as an escape would
	// eat every backslash in every path — the opposite of the goal.
	escapes := runtime.GOOS != "windows"

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
