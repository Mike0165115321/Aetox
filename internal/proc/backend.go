package proc

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// A Backend is the shell an agent-supplied command line actually runs in.
//
// Until now there was no such thing: ShellCommand and ShellName were package
// functions picked by build tag, so the shell was decided when the binary was
// compiled and every command in the process ran in it. That is the right shape
// for "Windows means cmd.exe, Linux means sh" and the wrong one the moment a
// Windows machine has a second, equally real shell sitting next to it — a WSL
// distro holding the project's toolchain. Choosing between them is a runtime
// question about this workspace, not a compile-time question about this OS.
//
// The interface is deliberately this small: only what actually differs
// between shells is on it, and everything else about running a command is not:
//
//   - Name, because the model is told which syntax to write (see the shell
//     tool's description) and being told the wrong one costs a wasted turn.
//   - Note, because one shell has a constraint its name cannot carry:
//     Windows PowerShell 5.1 is missing the `&&` every model chains with by
//     habit, and a model that is not told loses its first round to it.
//   - POSIX, because the sandbox guard reads the command line, and how a line
//     tokenizes — whether `\` escapes, whether a leading `/` opens a path or a
//     switch — is a property of the shell, not of GOOS.
//   - Command, because the invocation and the way a working directory is
//     expressed are the whole difference in spawning.
//   - HostPath, because containment is decided in Windows path terms while a
//     command bound for WSL names its files in Linux ones. Without a
//     translation the guard compares two spellings of different worlds and
//     refuses everything.
//
// Command owns HideConsole and cancellation rather than leaving them to
// callers, for the reason shell_windows.go already gives about HideConsole: a
// new call site that forgets one gets a flashing console window or an orphaned
// process tree, and neither failure points back here.
type Backend interface {
	// Name is what the model is told it is writing for.
	Name() string

	// Note is the one syntax constraint of this shell that its name cannot
	// carry, or "" when the name is instruction enough. It rides the tool
	// description beside Name, and it is a single fact, never a syntax table —
	// the shell tool's description says why.
	Note() string

	// POSIX reports whether the command line is read by a POSIX shell.
	POSIX() bool

	// Command builds the invocation for one command line, run in the host
	// directory dir. dir is always a path in the host OS's own spelling — the
	// backend translates it if its shell needs something else.
	Command(ctx context.Context, line, dir string) *exec.Cmd

	// HostPath turns a path spelled the way this shell spells it into the path
	// the host OS knows the same file by, so it can be checked against the
	// workspace. The bool is false when the path names something outside any
	// place the host can see, which is a refusal, not an error.
	//
	// A path already written in host spelling comes back unchanged: the model
	// mixing the two is a thing that happens, and a translation that mangled
	// `D:\x` on the way through would turn a checkable path into a nonsense
	// one.
	HostPath(p string) (string, bool)

	// GuestPath is HostPath read backwards: the name this shell knows a host
	// file by. The bool is false when there is no such name, which is a fact
	// about the two filesystems and not an error.
	//
	// It is on the interface for the direction the pair was missing. Everything
	// crossing into the workspace is translated by HostPath, so the tools can be
	// told where a command's files are; nothing translated what the tools
	// *answer* with, and their answers are host paths. A model handed
	// `D:\project\out.png` and told — correctly — to repeat the path a tool gave
	// it will put that path into the next command, where a shell that has never
	// heard of drive letters opens nothing (§126.6).
	//
	// Native is the identity both ways, which is what makes this cheap: on a
	// machine with one filesystem the pair collapses and every caller can stop
	// asking which world it is in.
	GuestPath(p string) (string, bool)

	// Export declares that the named variables, which the caller has already
	// put in cmd.Env, must be readable by the program the shell starts.
	//
	// A method rather than something callers do for themselves because it is
	// only not a no-op in one place, and that place is invisible: setting an
	// environment variable on a child process is the universal way to pass it
	// something, and it silently does nothing across the WSL boundary. A hook
	// reads $AETOX_TOOL and finds it empty, with no error anywhere to say the
	// variable was dropped rather than never set.
	Export(cmd *exec.Cmd, names ...string)
}

// The stored spelling of a chosen shell: "" or "native" for the machine's own,
// "wsl" for its default distro, "wsl:<name>" for a named one. One vocabulary,
// kept beside the backends it names, so the settings file, the slash command
// and the desktop picker cannot each invent their own.
const (
	nativeSetting = "native"
	wslSetting    = "wsl"
)

// ParseBackend turns a stored setting into the backend it names. Anything
// unrecognised is the native shell: a settings file hand-edited into nonsense
// should leave the app working, not leave it unable to run a command until
// somebody finds the typo.
func ParseBackend(setting string) Backend {
	setting = strings.TrimSpace(setting)
	if distro, ok := strings.CutPrefix(setting, wslSetting+":"); ok {
		return WSL(distro)
	}
	if strings.EqualFold(setting, wslSetting) {
		return WSL("")
	}
	return Native()
}

// FormatBackend is ParseBackend's inverse, and the only thing that should be
// written to a settings file.
//
// A WSL backend is written with its resolved distro name rather than the bare
// "wsl" a user may have picked: "the default distro" is an instruction whose
// meaning changes when they change their default, and a workspace that quietly
// follows that around is one that will someday run its commands somewhere else.
func FormatBackend(b Backend) string {
	wsl, ok := b.(*wslBackend)
	if !ok {
		return nativeSetting
	}
	if distro := wsl.Distro(); distro != "" {
		return wslSetting + ":" + distro
	}
	return wslSetting
}

// Label is what a person choosing a shell reads, and it is deliberately not
// Name.
//
// Name answers the model's question. "Windows PowerShell 5.1" in the shell
// tool's description is the instruction that decides whether the next command
// is written with `&&` or `;`, and naming the machine there would be a worse
// instruction than the one it replaced: Windows does not imply one shell, it
// is merely where they live. A person opening the picker is asking the
// opposite question — not which syntax, but which machine their command lands
// on — and a shell's name answers that one only for someone who already knew.
//
// So the two words split, and the reason to keep them split is that they drift
// apart on purpose: the moment a second Windows backend exists this has to
// become "Windows (PowerShell)" while Name stays exactly what ShellName says.
//
// A package function rather than a Backend method, for FormatBackend's reason:
// it is one more spelling of a backend, produced for one more audience, and
// keeping the spellings together is what stops each surface inventing its own.
func Label(b Backend) string {
	wsl, ok := b.(*wslBackend)
	if !ok {
		return nativeLabel()
	}
	if distro := wsl.Distro(); distro != "" {
		return "WSL: " + distro
	}
	return "WSL"
}

// nativeLabel names the machine rather than the shell sitting on it.
//
// A runtime switch rather than the build-tagged pair ShellName uses: those
// files are split because the invocation genuinely needs platform-only imports,
// and a string does not. One list read top to bottom is worth more here than
// symmetry with a split made for a different reason. An unlisted GOOS falls
// back to the shell's own name, which is wrong for a person but never blank.
func nativeLabel() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	}
	return ShellName()
}

// DefaultBackendFor is the shell a workspace should start out in when the user
// has never chosen one.
//
// A project living at \\wsl.localhost\<distro>\… is already inside a distro:
// its toolchain, its node_modules, its virtualenv and its line endings are all
// Linux, and the native shell can read the files but cannot run any of it. That
// case answers itself, and answering it is the difference between the feature
// working out of the box and the user having to know it exists. Everywhere else
// the answer is the machine's own shell, which is what it has always been.
func DefaultBackendFor(root string) Backend {
	slashed := strings.ReplaceAll(strings.TrimSpace(root), `\`, "/")
	for _, prefix := range []string{"//wsl.localhost/", "//wsl$/"} {
		if rest, ok := strings.CutPrefix(slashed, prefix); ok {
			if distro, _ := splitFirstSegment(rest); distro != "" {
				return WSL(distro)
			}
		}
	}
	return Native()
}

// IsNative reports whether b runs commands the way this process itself would.
//
// Asked by the one caller that legitimately cares which side of the boundary a
// command lands on: rtk, whose rewrites name a binary found on this machine's
// PATH and mean nothing inside somebody else's filesystem. A type assertion
// rather than a Backend method, because "are you the plain one" is a question
// about the set of backends, not a capability each backend should get to answer
// for itself.
func IsNative(b Backend) bool {
	_, ok := b.(nativeBackend)
	return ok
}

// Native is the machine's own shell: PowerShell on Windows (shell_windows.go
// picks which of the two), sh everywhere else. It stays the default
// everywhere, and it is the only backend on a machine with no WSL.
func Native() Backend { return nativeBackend{} }

type nativeBackend struct{}

func (nativeBackend) Name() string { return ShellName() }

func (nativeBackend) Note() string { return ShellNote() }

func (nativeBackend) POSIX() bool { return runtime.GOOS != "windows" }

func (nativeBackend) Command(ctx context.Context, line, dir string) *exec.Cmd {
	cmd := ShellCommand(ctx, line)
	cmd.Dir = dir
	// ShellCommand already hides the console on Windows — calling it again is
	// free, and doing it here is what lets a caller trust the interface rather
	// than the implementation it happened to get.
	HideConsole(cmd)
	KillOnCancel(cmd)
	return cmd
}

func (nativeBackend) HostPath(p string) (string, bool) { return p, true }

// GuestPath is the identity for the same reason HostPath is: this shell's files
// and this machine's files are the same files, under the same names.
func (nativeBackend) GuestPath(p string) (string, bool) { return p, true }

// Export is a no-op here: a child process on this machine inherits cmd.Env as
// it stands, which is what every caller already assumed.
func (nativeBackend) Export(*exec.Cmd, ...string) {}
