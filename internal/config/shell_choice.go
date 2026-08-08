package config

import (
	"strings"
	"sync"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

// ShellChoice is the live answer to "which shell do this project's commands run
// in", shared by both hosts so the CLI and the desktop cannot drift into
// resolving the same preference file two different ways.
//
// It exists to be a cache. The answer is read on every tool definition and
// every command, and reading it from disk each time would put a file open in
// front of every line the agent types. The cache is keyed by project folder, so
// focusing a different project re-resolves rather than carrying the previous
// project's shell into the new one — a workspace silently inheriting a setting
// that was never made for it is the failure this shape prevents.
//
// The zero value is ready to use and answers with the native shell until a
// preference says otherwise.
type ShellChoice struct {
	mu      sync.Mutex
	root    string
	backend proc.Backend
}

// For returns the shell for a project folder.
func (c *ShellChoice) For(root string) proc.Backend {
	if c == nil {
		return proc.Native()
	}
	root = strings.TrimSpace(root)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.backend != nil && strings.EqualFold(c.root, root) {
		return c.backend
	}
	pref, _, err := LoadModelPreference()
	if err != nil {
		// A preference file that cannot be read is not a reason to refuse to
		// run commands — it is a reason to run them where they have always run.
		debuglog.Msg("shell preference unreadable, using the native shell: %v", err)
		pref = ModelPreference{}
	}
	c.root, c.backend = root, pref.ShellFor(root)
	return c.backend
}

// Set records a choice for a project folder and makes For answer with it.
//
// The write is a load-modify-save of the whole preference file, like every
// other setting that lives there, so a shell picked in one surface is the shell
// the other surface finds.
func (c *ShellChoice) Set(root, setting string) error {
	pref, _, err := LoadModelPreference()
	if err != nil {
		return err
	}
	pref.SetShellFor(root, proc.ParseBackend(setting))
	if err := SaveModelPreference(pref); err != nil {
		return err
	}
	if c != nil {
		c.mu.Lock()
		c.root, c.backend = "", nil // re-resolve, rather than trust this to agree with the file
		c.mu.Unlock()
	}
	return nil
}
