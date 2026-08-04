package learned

import (
	"context"
	"encoding/json"
	"fmt"
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
}

func (*MemoryTool) Name() string { return "memory" }

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
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "memory",
			Description: "Keep something you worked out across sessions, so the next one starts knowing it. " +
				"Worth keeping: a fact about this user, their machine, or how their work is set up that will " +
				"still be true next month and would change what you do — a convention they hold to, where " +
				"something lives, a step that turned out to be necessary here. " +
				"Not worth keeping: anything about the task in front of you, anything you could look up or " +
				"search for when you need it, and anything you have not actually seen borne out. " +
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
	ok := func(msg, command string) (skill.Output, error) {
		return skill.Output{
			Name:       "memory",
			Content:    msg,
			Command:    command,
			Success:    true,
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
	if op == OpAdd && Full(t.Scope, len(text)+2) {
		return fail(fmt.Errorf(
			"this agent's memory is full (limit %d bytes) — replace or remove a line that has stopped being useful before adding another",
			MaxBytes))
	}

	res, err := t.Proposer.Propose(Proposal{
		Kind:   "memory",
		Scope:  t.Scope,
		Op:     op,
		Before: old,
		Body:   text,
		Reason: why,
	})
	if err != nil {
		return fail(fmt.Errorf("could not queue this for approval: %w", err))
	}
	if res.Duplicate {
		return ok("Already waiting for the user to approve — not queued twice.", "memory "+op)
	}
	return ok(
		"Queued for the user to approve. It does not affect this session; once approved it is there from the next one on.",
		"memory "+op)
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	s, _ := args[key].(string)
	return s
}
