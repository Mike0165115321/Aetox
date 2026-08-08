package subagent

import (
	"fmt"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/connect"
)

// What an agent cannot do its job without.
//
// A specialist agent is a folder someone drops in, and the things it leans on —
// an external account, a tool server — live outside that folder by design:
// ownership sits on the connection and on the server, because only those can be
// written at the moment the user decides (the lesson Mode.MCP records). So the
// folder cannot *grant* itself anything, and nothing here tries to.
//
// What it can do is **say what it needs**. That is the whole of this file, and
// the distinction is the point:
//
//   - `needs:` is a declaration. It grants nothing, ever.
//   - `for:` on the connection or the server is what grants, and stays the only
//     thing that does.
//
// Keeping them apart is what stops the failure Mode.MCP warns about — a
// manifest compiled before the user's things exist, naming none of them and
// filtering everything off. A need that names something missing produces a
// sentence on screen; it can never produce a silently empty tool list.
//
// Two readers, one source. The roster draws unmet needs on the card so an agent
// that cannot work does not look ready, and PromptFor folds them into the
// agent's own prompt so that an agent asked to work anyway *says what is
// missing* instead of improvising around it.

// Need kinds, as written in `needs:`.
const (
	NeedConnection = "connection"
	NeedMCP        = "mcp"
)

// Reasons a need is unmet. Codes rather than sentences, because the roster
// renders them in the user's language and this package cannot.
const (
	// ReasonUnknown: nothing in this build answers to that id at all.
	ReasonUnknown = "unknown"
	// ReasonMissing: it exists as a kind, but the user has not added one.
	ReasonMissing = "missing"
	// ReasonUnconnected: the account exists but holds no credential.
	ReasonUnconnected = "unconnected"
	// ReasonDisabled: it is there and switched off.
	ReasonDisabled = "disabled"
	// ReasonUnplaced: it is there and working, and this agent is not on its
	// `for:` list — the one unmet reason that is a deliberate user choice, and
	// the only one a single click can fix.
	ReasonUnplaced = "unplaced"
)

// Need is one requirement this agent does not currently have.
type Need struct {
	Kind   string `json:"kind"`   // NeedConnection | NeedMCP
	ID     string `json:"id"`     // the connection id, or the server name
	Label  string `json:"label"`  // what to call it on screen
	Reason string `json:"reason"` // one of the Reason constants
}

// Fixable reports whether the roster's one-click fix can resolve this need.
// Only placement can be: Aetox can write a `for:` entry, and cannot connect an
// account on the user's behalf or install a server for them.
func (n Need) Fixable() bool { return n.Reason == ReasonUnplaced }

// parseNeed splits a `needs:` entry into its kind and id. An entry with no
// colon is a line the user wrote wrong; it is returned with an empty kind so
// the caller reports it rather than guessing which kind was meant.
func parseNeed(entry string) (kind, id string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", ""
	}
	before, after, found := strings.Cut(entry, ":")
	if !found {
		return "", entry
	}
	return strings.ToLower(strings.TrimSpace(before)), strings.TrimSpace(after)
}

// UnmetNeeds reports everything this agent declared and does not have, in the
// order it declared them. An agent with no `needs:` returns nothing, which is
// every agent that ships today.
func UnmetNeeds(p Profile) []Need {
	if len(p.Needs) == 0 {
		return nil
	}
	var out []Need
	for _, entry := range p.Needs {
		kind, id := parseNeed(entry)
		if id == "" {
			continue
		}
		switch kind {
		case NeedConnection:
			if need, ok := unmetConnection(p.Name, id); ok {
				out = append(out, need)
			}
		case NeedMCP:
			if need, ok := unmetMCP(p.Name, id); ok {
				out = append(out, need)
			}
		default:
			// A kind nothing understands is reported rather than dropped: a
			// typo in a hand-written file must be visible on the card, or the
			// agent looks ready and is not.
			out = append(out, Need{Kind: kind, ID: id, Label: entry, Reason: ReasonUnknown})
		}
	}
	return out
}

func unmetConnection(agent, id string) (Need, bool) {
	status, ok := connect.StatusOf(id)
	if !ok {
		return Need{Kind: NeedConnection, ID: id, Label: id, Reason: ReasonUnknown}, true
	}
	need := Need{Kind: NeedConnection, ID: id, Label: status.Label}
	if !status.Connected {
		need.Reason = ReasonUnconnected
		return need, true
	}
	for _, held := range config.ConnectionsForAgent(agent, connect.IDs()) {
		if strings.EqualFold(held, id) {
			return Need{}, false
		}
	}
	need.Reason = ReasonUnplaced
	return need, true
}

func unmetMCP(agent, name string) (Need, bool) {
	servers, err := config.LoadMCPServers()
	if err != nil {
		return Need{Kind: NeedMCP, ID: name, Label: name, Reason: ReasonMissing}, true
	}
	need := Need{Kind: NeedMCP, ID: name, Label: name}
	found := false
	for _, s := range servers {
		if !strings.EqualFold(strings.TrimSpace(s.Name), name) {
			continue
		}
		found = true
		if s.Disabled {
			need.Reason = ReasonDisabled
			return need, true
		}
	}
	if !found {
		need.Reason = ReasonMissing
		return need, true
	}
	for _, held := range config.MCPServersForAgent(agent) {
		if strings.EqualFold(held, name) {
			return Need{}, false
		}
	}
	need.Reason = ReasonUnplaced
	return need, true
}

// needsNotice is what an agent with unmet needs carries in its own prompt.
//
// It exists because a readiness badge on a card only helps someone looking at
// the card, and the main assistant that dispatches this worker never sees one.
//
// The first version of this text said "say what is missing, then stop", and
// that was wrong in a way worth recording: it turned a specialist into a door
// nobody could get through. The GitHub worker with no token still has its own
// file tools, its own four skill documents, and the whole of what it knows —
// and most of what anyone asks about its subject needs no account at all. An
// agent that answers "I have no key" to "what should a repository have in it"
// has failed at its job while appearing to be careful about it.
//
// So the instruction is the other shape, and it is three things in order: ask
// for what is missing, do the part that does not need it, and never pass off a
// result as though the missing thing had been there. Only the third is a hard
// line — it is the one failure nobody downstream can catch.
func needsNotice(unmet []Need) string {
	if len(unmet) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n---\n# Ask for what is missing, then do the rest\n\n")
	b.WriteString("Not set up yet:\n\n")
	for _, need := range unmet {
		fmt.Fprintf(&b, "  - %s (%s) — %s\n", need.Label, need.Kind, reasonText(need))
	}
	b.WriteString("\nSay so in one line and ask for it, naming where it is switched on. ")
	b.WriteString("Then do the part of the job that does not need it — you still have your own tools ")
	b.WriteString("and everything you know, and most questions about your subject need neither an ")
	b.WriteString("account nor a server. Refusing work you could actually do is its own kind of wrong answer.\n")
	b.WriteString("\nThe one thing never to do is answer as though you had it. ")
	b.WriteString("A result that needed the missing thing, handed over as if it did not, is the single ")
	b.WriteString("failure here that nobody downstream can catch.\n")
	return b.String()
}

func reasonText(need Need) string {
	switch need.Reason {
	case ReasonUnconnected:
		if need.Kind == NeedConnection {
			return "no account is connected. The user connects one at Settings → การเชื่อมต่อ"
		}
		return "not connected"
	case ReasonUnplaced:
		if need.Kind == NeedConnection {
			return "an account is connected, but this agent is not switched on for it (Settings → การเชื่อมต่อ)"
		}
		return "the server is running, but this agent is not switched on for it (Settings → MCP servers)"
	case ReasonDisabled:
		return "it is configured and switched off (Settings → MCP servers)"
	case ReasonMissing:
		return "no server by that name is configured (Settings → MCP servers)"
	default:
		return "this build has nothing by that name — the agent's own file may name it wrongly"
	}
}
