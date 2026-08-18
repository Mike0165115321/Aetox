package learned

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// Proposal is one change the agent wants made to what it has learned. It is
// not applied here — Propose queues it, and a human decides.
type Proposal struct {
	Kind   string // "memory" today; "skill" and "prompt" queue through the same door
	Scope  string
	Op     string
	Before string
	Body   string
	Reason string
}

// Result is what queuing produced, so the tool can tell the model the truth
// about what happened rather than always claiming success.
type Result struct {
	ID        int64
	Duplicate bool // an identical proposal was already waiting
}

// Proposer is the door. Implemented by the desktop app against the
// pending_changes table; kept as an interface so this package never learns
// what a database is, and so a test can watch what the tool proposed without
// one.
type Proposer interface {
	Propose(Proposal) (Result, error)
}

// MemoryTool is the model-facing half. One instance per scope: the main
// agent's registry gets a MainScope one, and every delegate gets one bound to
// its own profile name, which is what makes "an agent learns only inside its
// own scope" true by construction rather than by the model's cooperation.
type MemoryTool struct {
	Scope    string
	Proposer Proposer
	// Desk is the second scope a main-agent session can write to: the desk it
	// was opened at (§83). Empty everywhere else — a delegate has one scope and
	// a pre-modes session has one file — and when it is empty this tool's
	// definition is byte-for-byte what it was before desks existed, because the
	// tool block is in every request and the common case must not pay for a
	// feature it cannot use.
	//
	// Two scopes rather than one because the split is the point: "the user
	// prefers X" is true at every desk and belongs in MEMORY.md, while "this
	// repository's tests need Y first" is worth carrying only where that work
	// happens. Which of the two a fact is, only the model writing it knows.
	Desk string
	// Project is the third destination: the ROOT PATH of the folder this
	// session is focused on, empty when none is (the unfocused session, the
	// pre-project hosts, every delegate). The path rather than a key so there
	// is one spelling of "which project" in the system — ProjectScope does the
	// rest.
	//
	// The desk above cannot stand in for it. โต๊ะโค้ด is the same desk in every
	// repository, so a decision made in one would arrive as advice in the next;
	// this is the axis where "we settled on X here, because Y" can be kept
	// without being carried anywhere it is not true.
	Project string
}

func (*MemoryTool) Name() string { return "memory" }

// Destinations the `where` parameter can name, in the order they are offered.
const (
	whereEverywhere = "everywhere"
	whereDesk       = "this-desk"
	whereProject    = "this-project"
)

func (t *MemoryTool) whereOptions() []string {
	out := []string{whereEverywhere}
	if t.Desk != "" {
		out = append(out, whereDesk)
	}
	if t.Project != "" {
		out = append(out, whereProject)
	}
	return out
}

// whereDescription says what each destination is FOR, in one sentence each.
// The names alone would not settle it: "this-desk" and "this-project" both
// sound like "not everywhere", and the whole value of the split is that the
// model picks between them correctly without being given a list of examples
// that only covers the cases somebody happened to think of.
func (t *MemoryTool) whereDescription() string {
	b := strings.Builder{}
	b.WriteString("everywhere (default) for a fact that is true whatever you are working on — this user, " +
		"this machine, how they like things done.")
	if t.Desk != "" {
		b.WriteString(" this-desk for something only " + t.Desk +
			" work needs, which then costs nothing anywhere else.")
	}
	if t.Project != "" {
		b.WriteString(" this-project for something that is true only in " + filepath.Base(t.Project) +
			" — what was decided here and why, a convention this codebase holds to. " +
			"It follows nobody into another project.")
	}
	return b.String()
}

func (*MemoryTool) Description() string {
	return "Remember something durable about this user or machine, or revise something already remembered. " +
		"The user approves it before it takes effect."
}

// ToolDefinition states what belongs in memory as a principle rather than a
// list of triggers: the failure this guards against is a memory that fills
// with restatements of the current task, and no enumeration of forbidden
// topics would prevent that. What separates a fact worth keeping from noise is
// whether it will still be true, and still change what the agent does, on a
// day nobody remembers this conversation.
//
// It was written entirely against that failure, and against the opposite one
// it was silent — so the opposite one is what shipped. Owner, 18 ส.ค.: the
// agent remembers only when told to remember, and a user saying who they are
// and what they are building goes straight past this tool. Two clauses did it,
// both aimed at the agent's own guesses and neither saying so. "Something you
// worked out" is a source test that the user's own sentence fails. "Anything
// you have not actually seen borne out" is an evidence test, and the agent
// waits for corroboration of a fact whose only possible source has just
// spoken.
//
// What generalises, and what the text below now says: the bar is whether a
// line will still be true and still matter, never where it came from. When the
// user states something about themselves, that IS the evidence — there is no
// later observation that would confirm it further. The cost sentence is what
// holds the other side; it always was.
func (t *MemoryTool) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"op": map[string]any{
				"type":        "string",
				"enum":        []string{OpAdd, OpReplace, OpRemove},
				"description": "add a new line, replace an existing one, or remove one. Defaults to add.",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "The fact, in one sentence. Required for add and replace.",
			},
			"old": map[string]any{
				"type":        "string",
				"description": "Distinctive words from the line being replaced or removed — enough to match it uniquely.",
			},
			"why": map[string]any{
				"type":        "string",
				"description": "What in this session showed you this. The user reads it when deciding whether to keep it.",
			},
		},
		"additionalProperties": false,
	}
	// The parameter appears only for a session that has somewhere else to put a
	// line, and lists only the destinations that session actually has. The tool
	// block rides in every request, so an option nobody can use is a bill with
	// no benefit — the same rule the desk half has followed since it shipped.
	if where := t.whereOptions(); len(where) > 1 {
		schema["properties"].(map[string]any)["where"] = map[string]any{
			"type":        "string",
			"enum":        where,
			"description": t.whereDescription(),
		}
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "memory",
			Description: "Keep what you learn about this user across sessions, so the next one starts knowing it. " +
				"Worth keeping: what they tell you about themselves — who they are, what they are building, how " +
				"they want to be worked with — and anything about their machine or their setup that will " +
				"still be true next month and would change what you do: a convention they hold to, where " +
				"something lives, a step that turned out to be necessary here. " +
				"A fact the user states about themselves is already the evidence for it, so propose it when it " +
				"is said rather than waiting to be told to. " +
				"Not worth keeping: anything about the task in front of you, anything you could look up or " +
				"search for when you need it, and a conclusion of your own you have not seen borne out. " +
				"A remembered line costs context on every request this agent ever makes again, so a wrong or " +
				"idle one is paid for forever. Nothing here takes effect until the user approves it, and it " +
				"reaches you at the start of the next session, not this one.",
			Parameters: payload,
		},
	}
}

// Execute exists because skill.Skill requires it. Memory is model-only: a
// person editing what the agent learned does it in the file or the review
// list, both of which show them what is already there.
func (t *MemoryTool) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	return skill.Output{}, fmt.Errorf("memory is called by the model; edit the memory folder directly to change it by hand")
}

func (t *MemoryTool) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	start := time.Now()
	fail := func(err error) (skill.Output, error) {
		return skill.Output{
			Name:       "memory",
			Content:    err.Error(),
			Command:    "memory",
			Success:    false,
			Stderr:     err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		}, err
	}
	// id travels with the receipt so the chat can draw the proposal where it was
	// made, with the decision on it. A duplicate carries the id of the row
	// already waiting: the second attempt is the same proposal, and the card
	// under this answer should be about that one rather than about nothing.
	ok := func(msg, command string, id int64) (skill.Output, error) {
		return skill.Output{
			Name:       "memory",
			Content:    msg,
			Command:    command,
			Success:    true,
			ProposalID: id,
			DurationMs: time.Since(start).Milliseconds(),
		}, nil
	}

	if t.Proposer == nil {
		return fail(fmt.Errorf("memory is not available in this session"))
	}

	op := strings.TrimSpace(stringArg(args, "op"))
	if op == "" {
		op = OpAdd
	}
	text := strings.TrimSpace(stringArg(args, "text"))
	old := strings.TrimSpace(stringArg(args, "old"))
	why := strings.TrimSpace(stringArg(args, "why"))
	// Anything other than one of the words that asks for a narrower file means
	// the shared one, including the word being absent: a model that guesses at
	// this parameter should land on the scope that was the only one before it
	// existed, and a narrow line kept too widely is a smaller mistake than a
	// wide one kept where nobody will ever read it again.
	scope := t.Scope
	switch strings.TrimSpace(stringArg(args, "where")) {
	case whereDesk:
		if t.Desk != "" {
			scope = ModeScope(t.Desk)
		}
	case whereProject:
		if t.Project != "" {
			scope = ProjectScope(t.Project)
		}
	}

	switch op {
	case OpAdd:
		if text == "" {
			return fail(fmt.Errorf("text is required to remember something"))
		}
	case OpReplace:
		if text == "" || old == "" {
			return fail(fmt.Errorf("replace needs both old (what to find) and text (what it becomes)"))
		}
	case OpRemove:
		if old == "" {
			return fail(fmt.Errorf("remove needs old — distinctive words from the line to forget"))
		}
	default:
		return fail(fmt.Errorf("unknown op %q — use add, replace or remove", op))
	}

	// Refused here rather than at approval: a proposal that cannot be applied
	// would sit in the user's review list looking like progress, and the agent
	// would never learn that it needs to consolidate.
	if op == OpAdd && Full(scope, len(text)+2) {
		return fail(fmt.Errorf(
			"this agent's memory is full (limit %d bytes) — replace or remove a line that has stopped being useful before adding another",
			MaxBytes))
	}

	// Same door, same reason, and the case that made it necessary is the
	// ordinary one rather than a rare one: the agent proposes a line, the user
	// corrects it two messages later, and the agent revises the line it just
	// proposed. But a proposal is not memory — nothing is, until somebody
	// approves it — so `old` names a line that is not in any file, and the
	// revision queued as a card that could never be approved. Its only exit was
	// ไม่เอา, which records the user refusing a line they had just asked for.
	//
	// What the model needs told is which of the two things is true, because the
	// answer changes what it should do next: the line is not there, so revising
	// it is not the move — adding it is.
	if op == OpReplace || op == OpRemove {
		if !Has(scope, old) {
			if op == OpRemove {
				return fail(fmt.Errorf("nothing remembered contains %q, so there is nothing to forget", old))
			}
			return fail(fmt.Errorf(
				"nothing remembered contains %q — a proposal you made earlier is not memory until the user approves it. Use add if this should be remembered on its own",
				old))
		}
	}

	res, err := t.Proposer.Propose(Proposal{
		Kind:   "memory",
		Scope:  scope,
		Op:     op,
		Before: old,
		Body:   text,
		Reason: why,
	})
	if err != nil {
		return fail(fmt.Errorf("could not queue this for approval: %w", err))
	}
	if res.Duplicate {
		return ok("Already waiting for the user to approve — not queued twice.", "memory "+op, res.ID)
	}
	return ok(
		"Queued for the user to approve. It does not affect this session; once approved it is there from the next one on.",
		"memory "+op, res.ID)
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	s, _ := args[key].(string)
	return s
}
