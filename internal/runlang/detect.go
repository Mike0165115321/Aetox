package runlang

import (
	"os"
	"os/exec"
)

// Interpreter picks the first of l's candidates this machine actually has.
// Reported as "not found" rather than guessed at, because the caller's next
// move — offer a button, or say plainly that nothing here can run this — is
// exactly the difference.
func (l Language) Interpreter() (Interpreter, bool) {
	for _, i := range l.Interpreters {
		if installed(i.Name) {
			return i, true
		}
	}
	return Interpreter{}, false
}

// Runnable is every fence tag this machine can run right now, mapped to its
// kind. It is what the chat asks once at startup so the Run button appears
// only where clicking it would do something.
//
// Shell tags are always in: the machine's shell is the one interpreter that
// cannot be missing. Script tags are in only if their interpreter is here,
// which is the whole point — a machine without Python gets no Run button on a
// Python block, instead of a button that returns a Store advert.
func Runnable() map[string]Kind {
	out := make(map[string]Kind)
	for _, l := range languages {
		if l.Kind == Script {
			if _, ok := l.Interpreter(); !ok {
				continue
			}
		}
		for _, tag := range l.Tags {
			out[tag] = l.Kind
		}
	}
	return out
}

// installed reports whether name resolves to a program that can actually run.
//
// exec.LookPath alone answers yes too often on Windows. A clean install puts
// Microsoft Store "app execution aliases" for python.exe and python3.exe on
// PATH before any real Python exists, and they are not programs: they are
// zero-byte reparse points that print an advert for the Store and exit 9009.
// LookPath finds them, so "is Python installed" answered yes on a machine with
// no Python at all, and the honest message the caller had ready never showed.
//
// Measured 2026-08-16: Go sees both stubs as size 0 with an irregular mode,
// where every real interpreter on this machine is a regular file with a size.
// That is the whole test, and it deliberately names no Windows path and checks
// no GOOS — a zero-byte program is not a program on any operating system.
func installed(name string) bool {
	path, err := exec.LookPath(name)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Size() > 0
}
