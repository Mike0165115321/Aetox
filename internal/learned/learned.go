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
	"unicode"

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

// projectPrefix marks a scope that belongs to one folder on this machine — the
// third axis, alongside the desk and the delegate.
//
// It exists because the other two cannot answer for it. A desk is the same desk
// in every repository: what โต๊ะโค้ด learned in one project would follow the
// user into the next, which is right for "this machine has no Excel" and wrong
// for "we decided this package owns the retry". The user's own file for a
// project (AETOX.md, prompt.ProjectContextFileNames) is not this either — that
// is what the user tells the agent, written by hand and checked into the repo,
// while this is what the work itself settled and the user approved. Different
// author, different question.
//
// Owner, 16 ส.ค., correcting an answer that had folded the two together:
// *"เราต้องแยกสิ ความจำ การเรียนรู้ ... ที่ผมอยากให้เพิ่มตอนแรก คือ
// จำการตัดสินใจตอนทำโปรเจกต์"*.
const projectPrefix = "project:"

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

// ProjectScope is the memory scope of the folder at root: what working in that
// project settled, folded into sessions held there and no others.
//
// Takes the root path rather than a key so there is one way to name a project
// and no caller has to know how it is spelled. The key underneath is
// config.ProjectKey — the same one the store files that project's history
// under, which is what lets a person open the memory folder and match a file
// to a project without a lookup table.
//
// Empty root means no project is open, which is a real state (the unfocused
// session that roams the machine) and not an error. It answers with MainScope,
// so a caller that asks anyway lands where it would have landed before this
// existed rather than in a junk drawer named after the home directory.
func ProjectScope(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return MainScope
	}
	return projectPrefix + safeKey(config.ProjectKey(root))
}

// SplitProjectScope is SplitModeScope's counterpart: the key a scope belongs
// to, and whether it is a project scope at all.
func SplitProjectScope(scope string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(scope), projectPrefix)
	return rest, ok && rest != ""
}

// safeKey makes a project key usable as a filename. config.ProjectKey opens
// with the folder's own base name, and folder names on this platform routinely
// carry spaces — "My App-1a2b3c4d" would be refused by validScope and the
// project would silently have no memory at all. The hash half is what makes a
// key unique, so flattening the readable half cannot collide two projects: it
// can only make two different folders' files sort next to each other.
func safeKey(key string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune(`\/:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(key))
	cleaned = strings.Trim(cleaned, ".-")
	if cleaned == "" {
		return "project"
	}
	return cleaned
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
	// Under <DataRoot>/memory rather than inside the project itself (owner's
	// call, 16 ส.ค.): writing into somebody's repository means git noise they
	// did not ask for, a file that needs the sandbox's write right to exist at
	// all, and nothing at all for a project folder that is not a repository.
	// What the team should see goes in AETOX.md, which is theirs and is written
	// on purpose.
	if key, ok := SplitProjectScope(scope); ok {
		if !validScope(key) {
			return "", fmt.Errorf("invalid memory scope %q", scope)
		}
		return filepath.Join(dir, "projects", key+".md"), nil
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

// Scopes lists every scope under <DataRoot>/memory that currently holds
// something, main first, then the desks and the projects in the order the
// filesystem gives them.
//
// It exists because the review page could only ever show one file. That was
// true enough while the main agent's was the only one a session could write
// to; the moment a desk or a project could be the destination, approving a
// line meant sending it somewhere the app had no way to show — visible only by
// opening the folder, which is the "it exists and you cannot reach it" shape
// this project keeps having to remove.
//
// A delegate's memory is deliberately not here: it lives in that worker's own
// folder and has its own page (การตั้งค่า › เอเจน), which is where somebody
// looking for it would look.
func Scopes() []string {
	dir, err := Dir()
	if err != nil {
		return nil
	}
	var out []string
	if Read(MainScope) != "" {
		out = append(out, MainScope)
	}
	for _, sub := range []struct {
		folder string
		scope  func(string) string
	}{
		{"modes", func(name string) string { return modePrefix + name }},
		{"projects", func(name string) string { return projectPrefix + name }},
	} {
		entries, err := os.ReadDir(filepath.Join(dir, sub.folder))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
				continue
			}
			scope := sub.scope(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
			if Read(scope) != "" {
				out = append(out, scope)
			}
		}
	}
	return out
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

// Entries is Read's list form: the remembered lines, in file order, with their
// bullets off. The settings page shows one row per entry and addresses them by
// position, so it needs the same split the writer uses rather than a second
// parse of the same text.
func Entries(scope string) []string {
	path, err := FileFor(scope)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return splitEntries(string(data))
}

// EditEntry rewrites or removes one remembered line, addressed by its position
// in Entries. An empty body removes it.
//
// By index, deliberately, where Apply matches a substring. Apply is written for
// the agent, which has the text in its prompt and no index that survives an
// edit above it — the right shape there. It is the wrong shape here: findEntry
// takes the *first* line containing the needle, and a real memory file is full
// of lines that differ only at the end ("shell failed: exit status 1", "…status
// 2", "…status 124"). Editing the fourth from a list on screen would silently
// rewrite the first. A row the user is looking at has an exact position, so
// that is what gets sent.
func EditEntry(scope string, index int, body string) error {
	path, err := FileFor(scope)
	if err != nil {
		return err
	}
	existing, _ := os.ReadFile(path)
	lines := splitEntries(string(existing))
	if index < 0 || index >= len(lines) {
		// Two tabs open on the same page, or an approval that landed while this
		// one sat there. Refusing beats editing whatever now sits at that row.
		return fmt.Errorf("that line is no longer there — reopen the page and try again")
	}
	if body = strings.TrimSpace(body); body == "" {
		lines = append(lines[:index], lines[index+1:]...)
	} else {
		lines[index] = body
	}
	rendered := render(scope, lines)
	if len(rendered) > MaxBytes {
		return fmt.Errorf(
			"this scope's memory is full (%d bytes, limit %d) — shorten the line or remove another",
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
	desk, isDesk := SplitModeScope(scope)
	project, isProject := SplitProjectScope(scope)
	switch {
	case isDesk:
		who = "the agent working at the " + desk + " desk"
	case isProject:
		who = "the agent working in the " + project + " project"
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
