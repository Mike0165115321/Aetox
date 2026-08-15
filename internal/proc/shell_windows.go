package proc

import (
	"context"
	"os/exec"
	"sync"
)

// powershell resolves, once per process, which PowerShell this machine runs
// agent commands with: pwsh (7+) when it is installed, Windows PowerShell 5.1
// otherwise. The bool reports the fallback, because 5.1 is missing the one
// operator every model reaches for (`&&`) and the tool description has to say
// so (ShellNote).
//
// cmd.exe is gone from this list on purpose. It was never chosen for the
// model's sake — it was simply what "the Windows shell" had always meant — and
// it is the dialect models have seen least: the first command of nearly every
// session died on a PowerShell-ism or an `ls -la` before the second tried
// again in cmd. PowerShell is the dialect models actually write on Windows,
// pwsh the more fluently of the two (`&&` works), so the order is pwsh, then
// the 5.1 every Windows ships with (owner's call, 2026-08-15; OpenCode landed
// on the same order independently, with cmd.exe as a last resort this machine
// can never reach because 5.1 is always there).
//
// Resolved once and cached: which shells are installed does not change while
// the app runs, and re-walking PATH on every command would buy nothing.
var powershell = sync.OnceValues(func() (path string, legacy bool) {
	if p, err := exec.LookPath("pwsh.exe"); err == nil {
		return p, false
	}
	if p, err := exec.LookPath("powershell.exe"); err == nil {
		return p, true
	}
	// LookPath failing on powershell.exe means a machine broken beyond this
	// function's concern; naming the program anyway lets exec report the real
	// error instead of this file inventing one.
	return "powershell.exe", true
})

// utf8Prologue makes the shell speak UTF-8 to the programs it runs. Spawned
// the way Aetox spawns it — no console, output into a pipe — PowerShell
// decodes native commands' output with the OEM codepage (874 on a Thai
// Windows), so `git log` with a Thai commit message arrived as mojibake
// before this line existed. Measured on this machine: both 5.1 and pwsh 7.5
// mangle identically without it, and both are clean with it (owner's call,
// 2026-08-15: "กรองเป็น UTF-8 ในตัวเลย"). $OutputEncoding is the other
// direction — bytes piped INTO a native command — set to match. try/catch
// because an encoding the host refuses to set must degrade to yesterday's
// behaviour, never fail the user's actual command.
const utf8Prologue = "try { [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new(); $OutputEncoding = [Console]::OutputEncoding } catch {}\n"

// exitEpilogue makes the process exit code mean what the command's author
// meant. Measured on this machine (5.1 and 7.5 both): `-Command` collapses a
// native command's exit code to 1, so `git`'s 128 and a linter's 2 would
// arrive indistinguishable — and the obvious fix, appending
// `; exit $LASTEXITCODE`, is a trap, because after a failed *cmdlet* no native
// command ever ran, $LASTEXITCODE is null, and `exit $null` is exit 0: a
// failure reported as success, which is the one direction a wrapper must
// never be wrong in. So: read $? first (the last statement's own verdict, the
// same one bash's $? gives), and only then prefer the native code that
// explains it.
//
// Joined with a newline, not ";" — a command line ending in a `# comment`
// would swallow a same-line epilogue whole, and every command after it would
// exit 0.
const exitEpilogue = "\nif (-not $?) { if ($null -ne $LASTEXITCODE -and $LASTEXITCODE -ne 0) { exit $LASTEXITCODE } else { exit 1 } }"

// ShellCommand builds the PowerShell invocation for an agent-supplied command
// line.
//
// Plain arguments, no SysProcAttr.CmdLine: that hack existed because cmd.exe
// does not follow the CommandLineToArgvW convention os/exec escapes for, and
// quoted arguments arrived corrupted (the 2026-07-25 bug this file used to
// document). PowerShell is a .NET program and parses its command line by
// exactly that convention, so the ordinary escaping is the correct one again.
func ShellCommand(ctx context.Context, line string) *exec.Cmd {
	shell, _ := powershell()
	cmd := exec.CommandContext(ctx, shell, "-NoProfile", "-Command", utf8Prologue+line+exitEpilogue)
	// Here rather than left to callers, so the shell path cannot flash a
	// console window even if a new caller forgets.
	HideConsole(cmd)
	return cmd
}

// ShellName is what the model is told it is writing for. It lives next to the
// invocation it describes so the two cannot drift: a tool description that
// named the wrong shell would be worse than one that named none. The version
// is part of the name because it is part of the dialect — "PowerShell 7"
// licenses `&&`, "Windows PowerShell 5.1" is the reason ShellNote exists.
func ShellName() string {
	if _, legacy := powershell(); legacy {
		return "Windows PowerShell 5.1"
	}
	return "PowerShell 7 (pwsh)"
}

// ShellNote is the one constraint of the resolved shell that its name cannot
// carry and the model cannot infer: 5.1 predates the pipeline chain
// operators, and a model that was only told "PowerShell" will chain with `&&`
// by habit and lose the round. Empty for pwsh, where the name really is the
// whole instruction — this is deliberately not a syntax table (see the shell
// tool's description for why), just the single fact that breaks the default.
func ShellNote() string {
	if _, legacy := powershell(); legacy {
		return "This shell has no `&&` or `||`: chain dependent commands as `cmd1; if ($?) { cmd2 }`."
	}
	return ""
}
