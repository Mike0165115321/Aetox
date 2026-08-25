package lsp

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/proc"
)

// Runtime install, deliberately NOT installer-time — the same judgment already
// made for rtk (internal/rtk/install.go, owner's explicit choice 2026-07-23):
// this runs lazily, in-process, the first time something actually needs the
// server, and fails open to "no diagnostics" if anything goes wrong.
//
// The reason matters more here than it did for rtk. A language server is only
// useful for a language the user actually writes, and installing all of them up
// front means several hundred megabytes of npm and Go toolchain output on a
// machine that may only ever open Python files. Installing gopls also requires
// a Go toolchain, and typescript-language-server requires npm — neither is
// guaranteed to exist, so an installer that tried this would have to either
// fail loudly or ship both toolchains.
//
// So: nothing is installed until a file of that language is checked, and then
// only if the toolchain that can install it is already present.

// installers maps a server binary to the command that installs it, and the
// toolchain that command needs.
var installers = map[string]struct {
	requires string // binary that must exist for the install to be possible
	command  string
	args     []string
}{
	"gopls": {
		requires: "go",
		command:  "go",
		args:     []string{"install", "golang.org/x/tools/gopls@latest"},
	},
	"typescript-language-server": {
		requires: "npm",
		command:  "npm",
		args:     []string{"install", "-g", "typescript-language-server", "typescript"},
	},
	"svelteserver": {
		requires: "npm",
		command:  "npm",
		args:     []string{"install", "-g", "svelte-language-server"},
	},
}

// installTimeout caps one attempt. `go install gopls` compiles a sizeable
// package and npm fetches a dependency tree, so a small timeout would abandon
// installs that were about to succeed; a large one still cannot hang a session
// forever.
const installTimeout = 5 * time.Minute

var (
	installMu    sync.Mutex
	installTried = map[string]bool{} // one attempt per binary per process
)

// ensureInstalled returns true when the server binary is available, installing
// it once if the toolchain to do so is present. Never returns an error: every
// failure path means the same thing to the caller — no diagnostics for this
// language, which is exactly how Aetox behaved before any of this existed.
func ensureInstalled(ctx context.Context, binary string) bool {
	if _, err := exec.LookPath(binary); err == nil {
		return true
	}
	spec, known := installers[binary]
	if !known {
		return false
	}

	installMu.Lock()
	if installTried[binary] {
		installMu.Unlock()
		// Already attempted this process. Re-check PATH rather than trusting
		// the earlier result: a user may have installed it themselves since.
		_, err := exec.LookPath(binary)
		return err == nil
	}
	installTried[binary] = true
	installMu.Unlock()

	// Never during `go test`: a test run must not spend minutes compiling a
	// language server or reaching npm. The rtk installer learned this the hard
	// way on CI's first green-field run.
	if testing.Testing() {
		return false
	}
	if _, err := exec.LookPath(spec.requires); err != nil {
		debuglog.Msg("lsp: cannot install %s — %s is not on PATH", binary, spec.requires)
		return false
	}

	debuglog.Msg("lsp: installing %s via %s %v", binary, spec.command, spec.args)
	runCtx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, spec.command, spec.args...)
	proc.HideConsole(cmd)
	// `npm install -g` forks node; on the 5-minute timeout CommandContext would
	// kill only npm itself and leave node downloading for the rest of the
	// session. This is the one exec site where that window is minutes long.
	proc.KillOnCancel(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		debuglog.Msg("lsp: install %s failed: %v — %s", binary, err, truncate(string(out), 400))
		return false
	}

	_, err := exec.LookPath(binary)
	if err != nil {
		// Installed somewhere PATH does not reach. Worth saying out loud in the
		// log: the difference between "install failed" and "install succeeded
		// but you need a new shell" is the whole fix.
		debuglog.Msg("lsp: installed %s but it is still not on PATH — restart may be needed", binary)
	}
	return err == nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
