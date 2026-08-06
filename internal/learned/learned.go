// Package learned holds what the agent has worked out for itself and had
// approved — durable facts that should survive the session that discovered
// them ("this machine's scanner writes to D:\Scans", "receipts from this shop
// put the total above the date").
//
// Three decisions shape the whole package:
//
//   - **It is plain markdown in one folder, not a table.** The store could
//     hold this more cheaply, but then it would only ever be Aetox's. A folder
//     of .md files can be copied into Claude Code, Codex or anything else that
//     reads context files, and it can be edited by hand in any editor. Nothing
//     the agent learns is allowed to be trapped in a format only we read.
//   - **It is written per scope, and read per scope.** MEMORY.md belongs to
//     the main agent; agents/<name>.md belongs to that one sub-agent and is
//     folded only into that sub-agent's prompt. This is the capability
//     boundary (ARCHITECTURE.md §44) applied to knowledge: what a delegate
//     learned about doing its job is not something the main agent has to carry
//     the cost of knowing. It is also the difference between a system whose
//     context stays flat as it learns and one whose prompt grows forever.
//   - **Nothing here writes itself.** Every change arrives as a proposal and
//     is applied only after a human approves it (desktop/pending.go). The tool
//     the model calls does not touch the disk.
//
// Reading is deliberately dumb — file contents, capped. The prompt is built at
// bootstrap only (internal/prompt), so an approval lands on disk immediately
// and reaches the model at the next session. That lag is a feature: a system
// prompt that changes mid-conversation invalidates the provider's prefix
// cache, which is the cost this project pays most attention to.
package learned

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// MaxBytes is the ceiling on one scope's file.
//
// A ceiling exists because this text is in the system prompt of every request
// that scope makes: unbounded memory is an unbounded bill, paid on every turn
// forever, and the cheap models Aetox targets are the ones that can least
// afford it. When it is reached the write is refused with the reason, so the
// agent can consolidate — merging two facts into one is work it can do, and
// silently dropping the newest fact would teach it that writing memory works
// when it did not.
const MaxBytes = 8 << 10

// MainScope is the main agent's own memory. Empty rather than "main" because
// that is how the rest of the codebase already spells "not a delegate"
// (tool_runs.agent, jobs.agent), and a second spelling would need a
// translation layer that could disagree with itself.
const MainScope = ""

// modePrefix marks a scope that belongs to a desk rather than to a delegate
// (ARCHITECTURE.md §83). A prefix rather than a second field because a scope
// is a string in three places already — the tool that proposes, the
// pending_changes row that holds it, and the file it lands in — and adding a
// dimension to it would mean teaching all three what a desk is.
//
// ':' cannot appear in a delegate's scope (validScope refuses it, and a
// profile name is a filename), so the two namespaces cannot collide.
const modePrefix = "mode:"

// ModeScope is the memory scope of the desk named name: what working at that
// desk taught the agent, folded into that desk's prompt and no other's.
//
// This is the second axis of §44's capability boundary. MEMORY.md stays the
// cross-desk truths — who the user is, what this machine is like — because
// those are true wherever they are sitting; what coding work taught the agent
// about this repository must never cost the assistant desk a token.
func ModeScope(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return MainScope
	}
	return modePrefix + name
}

// SplitModeScope reports the desk a scope belongs to, and whether it is one at
// all. Callers that render a scope to a person need it — "coding" reads as a
// desk, "mode:coding" reads as an implementation detail leaking out.
func SplitModeScope(scope string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(scope), modePrefix)
	return rest, ok && rest != ""
}

// Dir is <DataRoot>/memory — separate from <DataRoot>/identity on purpose.
// Identity is what the user tells the agent about itself; this is what the
// agent worked out. Keeping them in one folder would make "delete everything
// it thinks it learned" impossible to offer without also deleting the user's
// own instructions.
func Dir() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "memory"), nil
}

// FileFor returns the path holding one scope's memory. Named MEMORY.md at the
// top for the main agent because that filename already means this to every
// other agent runtime — a folder handed to one of them needs no explanation.
//
// A delegate's memory is the exception to "everything the agent worked out
// lives under <DataRoot>/memory": it sits in that worker's own folder
// (config.AgentHome) instead, because one agent is one folder (owner's call,
// 2026-08-06) and its memory is part of what it is. The main agent's and each
// desk's stay here — neither has a folder of its own to be part of.
func FileFor(scope string) (string, error) {
	scope = strings.TrimSpace(scope)
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if scope == MainScope {
		return filepath.Join(dir, "MEMORY.md"), nil
	}
	if desk, ok := SplitModeScope(scope); ok {
		if !validScope(desk) {
			return "", fmt.Errorf("invalid memory scope %q", scope)
		}
		return filepath.Join(dir, "modes", desk+".md"), nil
	}
	if !validScope(scope) {
		return "", fmt.Errorf("invalid memory scope %q", scope)
	}
	// Before the first read, from whichever entry point got here first — a
	// memory lookup can precede any profile resolution.
	config.MigrateAgentHomes()
	return config.AgentMemoryPath(scope)
}

// Read returns one scope's memory as it stands on disk, or "" when there is
// none — which is the normal state, not an error. Truncated at MaxBytes so a
// hand-edited file cannot cost more than the quota the tool enforces.
func Read(scope string) string {
	path, err := FileFor(scope)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > MaxBytes {
		data = data[:MaxBytes]
	}
	return strings.TrimSpace(stripHeader(string(data)))
}

// Ops a proposal can carry. Substring-matched rather than line-numbered
// (Hermes' shape, and it is the right one): the agent has the text in its
// prompt, and a line number is a coordinate that goes stale the moment
// anything above it changes.
const (
	OpAdd     = "add"
	OpReplace = "replace"
	OpRemove  = "remove"
)

// Apply writes an approved change to disk. Called from the approval path, never
// from a tool.
func Apply(scope, op, before, body string) error {
	path, err := FileFor(scope)
	if err != nil {
		return err
	}
	body = strings.TrimSpace(body)
	before = strings.TrimSpace(before)

	existing, _ := os.ReadFile(path)
	lines := splitEntries(string(existing))

	switch op {
	case OpAdd:
		if body == "" {
			return fmt.Errorf("nothing to remember")
		}
		lines = append(lines, body)
	case OpReplace:
		idx := findEntry(lines, before)
		if idx < 0 {
			return fmt.Errorf("no remembered line contains %q", before)
		}
		if body == "" {
			return fmt.Errorf("replacement is empty — use remove to forget a line")
		}
		lines[idx] = body
	case OpRemove:
		idx := findEntry(lines, before)
		if idx < 0 {
			return fmt.Errorf("no remembered line contains %q", before)
		}
		lines = append(lines[:idx], lines[idx+1:]...)
	default:
		return fmt.Errorf("unknown memory operation %q", op)
	}

	rendered := render(scope, lines)
	if len(rendered) > MaxBytes {
		return fmt.Errorf(
			"this scope's memory is full (%d bytes, limit %d) — merge or drop an existing line first",
			len(rendered), MaxBytes)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(rendered), 0o644)
}

// Full reports whether a scope has room for one more entry of the given size,
// so the tool can refuse a proposal at the moment the agent writes it rather
// than at approval time — a queue full of proposals that cannot be applied is
// worse than a refusal the agent can act on.
func Full(scope string, addBytes int) bool {
	return len(Read(scope))+addBytes+len(header(scope)) > MaxBytes
}

// header is the first thing in the file. It exists for the human who opens
// this folder in six months, or drops it into another agent: a bare list of
// assertions with no provenance reads as configuration, and someone will edit
// it expecting the agent to obey rather than to have learned.
func header(scope string) string {
	who := "the main agent"
	switch desk, isDesk := SplitModeScope(scope); {
	case isDesk:
		who = "the agent working at the " + desk + " desk"
	case scope != MainScope:
		who = "the " + scope + " sub-agent"
	}
	return "# Learned by " + who + "\n\n" +
		"Written by Aetox from its own completed work, and approved by you before it landed here.\n" +
		"Plain markdown on purpose — this folder works in any agent that reads .md context files.\n" +
		"Edit or delete any line; nothing here is load-bearing for the app.\n\n"
}

// stripHeader removes the explanatory block so it is not folded into the
// prompt. It explains the file to a person; the model is told what the layer is
// by the layer's own title.
func stripHeader(raw string) string {
	const marker = "\n\n"
	if !strings.HasPrefix(raw, "# Learned by ") {
		return raw
	}
	if idx := strings.Index(raw, "\n- "); idx >= 0 {
		return raw[idx:]
	}
	if idx := strings.LastIndex(raw, marker); idx >= 0 {
		return raw[idx:]
	}
	return raw
}

func render(scope string, lines []string) string {
	var b strings.Builder
	b.WriteString(header(scope))
	for _, l := range lines {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}

// splitEntries reads the bullet list back out of a file, ignoring the header
// and any prose a user added around it. Anything that is not a bullet is
// dropped on the next write — the file's contract is a list, and quietly
// keeping stray prose would mean two formats to reason about forever.
func splitEntries(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			if entry := strings.TrimSpace(trimmed[2:]); entry != "" {
				out = append(out, entry)
			}
		}
	}
	return out
}

func findEntry(lines []string, needle string) int {
	if needle == "" {
		return -1
	}
	for i, l := range lines {
		if strings.Contains(l, needle) {
			return i
		}
	}
	return -1
}

// validScope rejects anything that would escape the memory directory. A scope
// arrives from a profile name, which arrives from a tool call the model wrote.
func validScope(scope string) bool {
	return scope != "." && scope != ".." &&
		!strings.ContainsAny(scope, `\/:*?"<>|`) &&
		!strings.ContainsAny(scope, " \t\n")
}
