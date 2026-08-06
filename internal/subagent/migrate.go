package subagent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// Migrate moves the user's agent files out of the sub-agents' home, once.
//
// Until 2026-08-05 both kinds shared one folder and the `desk:` field said
// which was which. The homes split (see the package doc), and the old shared
// folder became the sub-agents' — so a chair file a user dropped there under
// the old contract would wake up sick under the new one, for following the
// rules as they stood. Files written under an old contract are the previous
// owner's word, and the move is this package keeping it.
//
// A plain move, visible on disk, because these files are the user's own. A
// name the agents' home already holds is not overwritten — the file stays put
// and resolve() reports the conflict, which is the honest state of two files
// claiming one name.
//
// It lands in the **folder** shape (config.AgentDefinitionPath) rather than the
// flat file it used to, so this migration and config.MigrateAgentHomes never
// have to run in a particular order. Writing a flat file here would depend on
// the folder migration running afterwards — and that one fires once per
// process, so a Migrate() called after it would leave a file nothing reads.
//
// Idempotent by construction: after a run, no file in the sub-agents' home
// declares a desk, so the next run finds nothing. Returns the names moved, for
// the caller's log line.
func Migrate() []string {
	var moved []string
	for _, path := range userHelperFiles() {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if parse(name, string(raw)).Desk == "" {
			continue
		}
		target, err := config.AgentDefinitionPath(name)
		if err != nil {
			continue
		}
		if _, err := os.Stat(target); err == nil {
			continue // both names exist: a conflict to report, not a file to clobber
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return moved
		}
		if err := os.Rename(path, target); err == nil {
			moved = append(moved, name)
		}
	}
	return moved
}
