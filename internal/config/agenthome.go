package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// One agent is one folder (owner's call, 2026-08-06). Everything that belongs
// to a worker — what it is, what it has learned, and the tools it brings with
// it — sits under <DataRoot>/agents/<name>/ so the answer to "what is this
// agent" is a directory listing rather than a search across three trees.
//
//	<DataRoot>/agents/doc/AGENT.md    frontmatter + brief
//	<DataRoot>/agents/doc/MEMORY.md   what it learned doing the job
//	<DataRoot>/agents/doc/mcp.json    (planned) the servers it brings
//
// The paths live here rather than in internal/subagent because two packages
// need them and only one of them may import the other: subagent imports
// learned, so learned cannot ask subagent where an agent's folder is. A second
// copy of `filepath.Join(root, "agents", name)` in the package that could not
// import the first is exactly the kind of second answer this codebase treats
// as debt — so both ask config, which imports neither.
//
// AGENT.md is what makes a folder a *definition the user wrote*. A folder
// holding only MEMORY.md is a worker whose definition is bundled (the shipped
// agents, and the helpers, which have no on-disk definition at all) and which
// has learned something. That is why the resolver keys on AGENT.md and not on
// the folder: presence of the folder means "has state", presence of AGENT.md
// means "the user owns the definition".
const (
	// AgentDefinitionFile is the profile — frontmatter plus the brief.
	AgentDefinitionFile = "AGENT.md"
	// AgentMemoryFile is what this worker learned and the user approved.
	// Named MEMORY.md for the same reason the main agent's file is: a folder
	// handed to any agent runtime needs no explanation.
	AgentMemoryFile = "MEMORY.md"
)

// AgentsRoot returns <DataRoot>/agents — the team's home, one folder per
// worker. Not created here.
func AgentsRoot() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "agents"), nil
}

// AgentHome returns <DataRoot>/agents/<name> — everything belonging to one
// worker. Not created here.
//
// The name is refused if it could escape the home. It arrives from a filename,
// a tool call the model wrote, or a settings field, and this function turns it
// into a path — which makes it a trust boundary, not a formatting rule.
func AgentHome(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return "", errors.New("invalid agent name")
	}
	root, err := AgentsRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}

// AgentDefinitionPath and AgentMemoryPath are the two files inside a home.
func AgentDefinitionPath(name string) (string, error) {
	return agentFile(name, AgentDefinitionFile)
}

func AgentMemoryPath(name string) (string, error) {
	return agentFile(name, AgentMemoryFile)
}

func agentFile(name, file string) (string, error) {
	home, err := AgentHome(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, file), nil
}

// Keyed by data root rather than a bare sync.Once. A process has one root in
// production, so the two behave identically there — but AETOX_DATA_ROOT makes
// the root a per-caller value, and a plain Once would let the first test that
// touched an agent path decide that every later root was already migrated.
// A migration nothing can exercise is a migration nobody knows is broken.
var (
	migratedMu    sync.Mutex
	migratedRoots map[string]bool
)

// MigrateAgentHomes moves the pre-folder layout into the folders, once per
// process. Both readers call it before their first read — subagent's resolver
// and learned's path lookup — rather than a bootstrap wiring it, because there
// is no single entry point: the CLI, the desktop app and the tests all reach
// these files, and a migration that runs only where somebody remembered to
// call it is a migration that has already failed for somebody.
//
//	<DataRoot>/agents/<name>.md         → <DataRoot>/agents/<name>/AGENT.md
//	<DataRoot>/memory/agents/<name>.md  → <DataRoot>/agents/<name>/MEMORY.md
//
// Every failure is skipped rather than reported. A move that cannot happen
// leaves the old file exactly where it was, which is recoverable; refusing to
// start because one agent's folder is read-only is not. The old file is
// removed only after the new one is written, so an interrupted run costs a
// duplicate and never the content.
func MigrateAgentHomes() {
	dataRoot, err := DataRoot()
	if err != nil {
		return
	}
	migratedMu.Lock()
	defer migratedMu.Unlock()
	if migratedRoots[dataRoot] {
		return
	}
	if migratedRoots == nil {
		migratedRoots = map[string]bool{}
	}
	migratedRoots[dataRoot] = true

	moveInto(filepath.Join(dataRoot, "agents"), AgentDefinitionPath)
	moveInto(filepath.Join(dataRoot, "memory", "agents"), AgentMemoryPath)
}

// moveInto relocates every <dir>/<name>.md to wherever dest says it belongs.
func moveInto(dir string, dest func(string) (string, error)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no folder yet is the normal state, not an error
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		target, err := dest(name)
		if err != nil {
			continue
		}
		if _, err := os.Stat(target); err == nil {
			continue // already migrated, or the user wrote the new one by hand
		} else if !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			continue
		}
		os.Remove(filepath.Join(dir, e.Name()))
	}
}
