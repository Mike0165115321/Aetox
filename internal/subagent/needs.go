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
	// OneOf holds the *other* ways this same requirement could have been met,
	// when the entry named alternatives. Empty for an ordinary need.
	//
	// The Need carrying them is the one closest to being fixed, not the first
	// one written — see closeness. An agent that needs "n8n or Windmill" and
	// has n8n connected but unplaced should be told about the click that
	// finishes it, not about the product it has never installed.
	OneOf []Need `json:"one_of,omitempty"`
}

// Fixable reports whether the roster's one-click fix can resolve this need.
// Only placement can be: Aetox can write a `for:` entry, and cannot connect an
// account on the user's behalf or install a server for them.
func (n Need) Fixable() bool { return n.Reason == ReasonUnplaced }

// alternatives splits one `needs:` entry into the things that would each
// satisfy it on their own.
//
// An entry may name more than one, separated by `|`:
//
//	needs: connection:n8n | mcp:windmill
//
// which reads "an automation engine, and either of these is one". Aetox
// supports more than one engine on purpose (§92.3), and without this the
// agent's declaration is a lie for every user who runs the other one: a
// Windmill user was told they were missing n8n, which they had never wanted.
//
// `|` rather than a second frontmatter key because the alternation belongs to
// the one requirement rather than to the agent — an agent may want "either of
// these" for one thing and "definitely this" for another, and a key cannot say
// which of its entries go together.
func alternatives(entry string) []string {
	var out []string
	for _, part := range strings.Split(entry, "|") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

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
		var missing []Need
		met := false
		for _, alt := range alternatives(entry) {
			need, unmet := unmetOne(p.Name, alt)
			if !unmet {
				// One is enough. An entry naming a single thing takes this
				// path too, which is why there is no second code path for the
				// ordinary case to drift away from.
				met = true
				break
			}
			missing = append(missing, need)
		}
		if met || len(missing) == 0 {
			continue
		}
		out = append(out, group(missing))
	}
	return out
}

// Requirement is one `needs:` entry with every way of satisfying it and where
// each one stands.
//
// UnmetNeeds answers "what is missing", which is the right question for a
// prompt and for a badge. It is the wrong question for the page the user fixes
// it on: an entry naming two engines showed up there as one line about n8n, as
// though n8n were required and Windmill did not exist. Options is the whole
// entry — met or not, each with its own state — so the screen can say "either
// of these, and here is where each one stands".
type Requirement struct {
	// Entry as the author wrote it, alternatives and all. Carried so a `needs:`
	// line nothing could be made of is still shown as the line it was.
	Entry string `json:"entry"`
	Met   bool   `json:"met"`
	// Every alternative, in the order written. A satisfied one has an empty
	// Reason — the same convention needState uses, so there is one meaning of
	// "fine" in this file rather than two.
	Options []Need `json:"options"`
}

// Requirements reports every `needs:` entry with each alternative's state.
//
// Same walk as UnmetNeeds over the same needState, so the two cannot disagree
// about whether something is satisfied — which they would within a release of
// each other if the page computed its own answer.
func Requirements(p Profile) []Requirement {
	out := make([]Requirement, 0, len(p.Needs))
	for _, entry := range p.Needs {
		req := Requirement{Entry: entry}
		for _, alt := range alternatives(entry) {
			need := needState(p.Name, alt)
			if need.ID == "" {
				continue
			}
			if need.Reason == "" {
				req.Met = true
			}
			req.Options = append(req.Options, need)
		}
		out = append(out, req)
	}
	return out
}

// unmetOne answers for one alternative. The bool is "this is missing", so the
// zero Need is never reported.
func unmetOne(agent, alt string) (Need, bool) {
	need := needState(agent, alt)
	return need, need.ID != "" && need.Reason != ""
}

// needState is the one place that answers "where does this alternative stand".
// An empty Reason means satisfied; an empty ID means the entry was not a thing
// at all (a blank between two `|`).
func needState(agent, alt string) Need {
	kind, id := parseNeed(alt)
	if id == "" {
		return Need{}
	}
	switch kind {
	case NeedConnection:
		return connectionState(agent, id)
	case NeedMCP:
		return mcpState(agent, id)
	default:
		// A kind nothing understands is reported rather than dropped: a typo in
		// a hand-written file must be visible on the card, or the agent looks
		// ready and is not.
		return Need{Kind: kind, ID: id, Label: alt, Reason: ReasonUnknown}
	}
}

// group folds the alternatives of one unsatisfied entry into a single Need.
//
// The one reported is the one nearest to working, and the rest ride along in
// OneOf. Reporting the first-written instead would routinely send the user to
// install something while an account they already have sits one toggle away.
func group(missing []Need) Need {
	best := 0
	for i, need := range missing {
		if closeness(need.Reason) < closeness(missing[best].Reason) {
			best = i
		}
	}
	head := missing[best]
	for i, need := range missing {
		if i != best {
			head.OneOf = append(head.OneOf, need)
		}
	}
	return head
}

// closeness ranks how much stands between a reason and being satisfied. Lower
// is nearer. Placement is a click Aetox itself can make (Need.Fixable), and
// installing something that does not exist on the machine is the far end.
func closeness(reason string) int {
	switch reason {
	case ReasonUnplaced:
		return 0
	case ReasonDisabled:
		return 1
	case ReasonUnconnected:
		return 2
	case ReasonMissing:
		return 3
	default: // ReasonUnknown — nothing in this build answers to the name
		return 4
	}
}

// connectionState and mcpState fill in Label whether or not the thing is
// satisfied. The label is what the screen calls it, and a satisfied option
// still has to be named — "n8n, connected" is the line the user came to read.
func connectionState(agent, id string) Need {
	status, ok := connect.StatusOf(id)
	if !ok {
		return Need{Kind: NeedConnection, ID: id, Label: id, Reason: ReasonUnknown}
	}
	need := Need{Kind: NeedConnection, ID: id, Label: status.Label}
	if !status.Connected {
		need.Reason = ReasonUnconnected
		return need
	}
	for _, held := range config.ConnectionsForAgent(agent, connect.IDs(), connect.AgentDefaults()) {
		if strings.EqualFold(held, id) {
			return need // satisfied: connected and pointed at this agent
		}
	}
	need.Reason = ReasonUnplaced
	return need
}

func mcpState(agent, name string) Need {
	need := Need{Kind: NeedMCP, ID: name, Label: name}
	servers, err := config.LoadMCPServers()
	if err != nil {
		need.Reason = ReasonMissing
		return need
	}
	found := false
	for _, s := range servers {
		if !strings.EqualFold(strings.TrimSpace(s.Name), name) {
			continue
		}
		found = true
		if s.Disabled {
			need.Reason = ReasonDisabled
			return need
		}
	}
	if !found {
		need.Reason = ReasonMissing
		return need
	}
	for _, held := range config.MCPServersForAgent(agent) {
		if strings.EqualFold(held, name) {
			return need // satisfied: enabled and pointed at this agent
		}
	}
	need.Reason = ReasonUnplaced
	return need
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
// The second version fixed that by writing the opposite move out in full: ask
// in one line, then do the part that still works, and never refuse work you
// could do. It read as the cure, but it was the same medicine — a rule that
// happened to point the better way. Neither version ever tried the third thing,
// which is saying what is true and letting the agent decide what to do about it.
//
// That is what this is now (owner's call, 2026-08-16: an agent is an identity
// that decides within its own remit, and legislating its moves is what makes it
// read as a tool). What an agent cannot work out for itself is the *fact* — that
// a server is not connected, and where the user switches it on — so the fact is
// all this carries. Whether to ask first, work around it, or say the job cannot
// be done at all is its call, made against the situation in front of it rather
// than against a sentence written here months earlier.
//
// The one line that stays is not a move but a standard: a result that needed the
// missing thing, handed over as if it did not, is the failure nobody downstream
// can catch. That is the same kind of thing as the research worker's "never
// report what you did not read" — a truth about the job, not a procedure for it.
func needsNotice(unmet []Need) string {
	if len(unmet) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n---\n# What is not set up\n\n")
	b.WriteString("Not set up yet:\n\n")
	for _, need := range unmet {
		fmt.Fprintf(&b, "  - %s (%s) — %s\n", need.Label, need.Kind, reasonText(need))
		// Alternatives are listed under the one they are alternatives to, and
		// said to be alternatives. An agent that reads two lines as two missing
		// things asks the user for both, and the second is one they may have
		// deliberately not chosen.
		for _, alt := range need.OneOf {
			fmt.Fprintf(&b, "    or %s (%s) — %s\n", alt.Label, alt.Kind, reasonText(alt))
		}
	}
	b.WriteString("\nYou still have your own tools and everything you know, and this is the whole of ")
	b.WriteString("what is missing.\n")
	b.WriteString("\nWhat cannot be done is answering as though you had it. ")
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
