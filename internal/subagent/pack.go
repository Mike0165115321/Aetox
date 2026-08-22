package subagent

// Where one worker's folder actually is, for whoever wants to pack it up.
//
// A worker on a running machine can be in two places at once: the folder the
// user wrote, and the copy that shipped inside the binary under the same name.
// Editing a shipped worker means shadowing it, file by file — that is what the
// profile resolver does with AGENT.md and what OwnSkills does with skills — so
// "what is this worker, right now" is an overlay rather than a directory.
//
// This returns that overlay, most-owned first, and nothing else. The packing
// itself lives in internal/agentpkg, which knows nothing about this package and
// must not: skill imports neither, subagent imports skill, and an exporter that
// reached back into the resolver would close that ring.

import (
	"io/fs"
	"os"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// PackageSources returns the folders that make up one worker, the user's own
// first. Empty when no such worker exists on this machine.
func PackageSources(name string) []fs.FS {
	var out []fs.FS
	if home, err := config.AgentHome(name); err == nil {
		if info, statErr := os.Stat(home); statErr == nil && info.IsDir() {
			out = append(out, os.DirFS(home))
		}
	}
	if sub, err := fs.Sub(bundledProfiles, bundledAgentDir+"/"+name); err == nil {
		// fs.Sub succeeds for a path that does not exist — it is the read that
		// fails, later, from inside a walk. Asking now keeps a name nobody ships
		// from becoming an empty source that a caller has to defend against.
		if _, statErr := fs.Stat(sub, config.AgentDefinitionFile); statErr == nil {
			out = append(out, sub)
		}
	}
	return out
}
