package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// `ask_main`: how a sub-agent gets a decision out of the main agent without
// throwing away what it has already done.
//
// The shape is forced by the loop, not chosen. The main agent's tool loop is
// synchronous — the only place a message can enter its head is as the result of
// a tool it called itself. So a delegate cannot interrupt; it can only be found
// waiting. It calls `ask_main`, which parks its goroutine *inside the tool call*,
// and the next `task_result` finds a question instead of an answer. The parent
// replies with `task_answer`, that text becomes the return value of the parked
// call, and the delegate carries on in the same loop with its whole context
// intact — every file it read, every dead end it already ruled out.
//
// Parking rather than returning is the entire point. A delegate that ended its
// run to ask would have to be re-briefed and started fresh, which is the work
// paid for twice: ten files read, then read again because the answer to "which
// folder?" arrived after they were forgotten.
//
// Every delegate gets this tool regardless of its profile's `tools:` allowlist.
// Asking is not a capability that touches the machine — it is the escape hatch
// from a brief that turned out to be wrong, and a profile that cannot ask is
// exactly the state this replaces (a delegate guessing, or stopping).

// pendingAsk is one unanswered question. answer is buffered so the parent's
// task_answer never blocks on a delegate that has meanwhile been cancelled.
type pendingAsk struct {
	question string
	asked    time.Time
	answer   chan string
}

// askMainTool is bound to the one delegation it belongs to, so nothing has to
// be looked up and no id can be got wrong.
type askMainTool struct{ task *runningTask }

func (*askMainTool) Name() string { return "ask_main" }

func (*askMainTool) Description() string {
	return "ถามเมนเอเจนเมื่อติดปัญหาหรือต้องตัดสินใจ แล้วรอคำตอบเพื่อทำงานต่อ"
}

func (*askMainTool) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type": "string",
				"description": "What you need decided, in one or two sentences. State the options you see and " +
					"what you will do with each answer — the main agent cannot see your work, only this question.",
			},
		},
		"required":             []string{"question"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "ask_main",
			// The description has to fight two failure modes at once: a delegate
			// that never asks and guesses wrong, and one that asks about
			// everything and turns a delegation into a conversation.
			Description: "Ask the main agent for a decision and WAIT for the answer, then keep going with " +
				"everything you have already done still in hand. Use it when you are genuinely blocked: the brief " +
				"is ambiguous in a way that changes the work, or you have hit a problem the brief did not " +
				"anticipate and the wrong choice would waste the run. Do NOT use it to check in, to report " +
				"progress, or for anything you can decide yourself the way a careful colleague would — every " +
				"question costs the main agent a round trip. Ask once, with the options spelled out, not " +
				"repeatedly in small pieces.",
			Parameters: payload,
		},
	}
}

// Execute exists because skill.Skill requires it; a delegate has no command line.
func (*askMainTool) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	return skill.Output{}, fmt.Errorf("ask_main is called by a sub-agent, not from the command line")
}

func (a *askMainTool) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	started := time.Now()
	question := strings.TrimSpace(stringArg(args, "question"))
	if question == "" {
		return askFailure(started, "question is required — say what you need decided and what you will do with each answer")
	}

	answer, err := a.task.ask(ctx, question)
	if err != nil {
		// The turn was stopped, or the parent went away. Reported as a failed tool
		// result rather than an error so the delegate's own loop can wind up.
		return askFailure(started, "no answer came back: "+err.Error())
	}
	content := "main agent: " + answer
	return skill.Output{
		Name:       "ask_main",
		Command:    "ask_main " + truncate(question, 60),
		Content:    content,
		RawOutput:  content,
		Success:    true,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

func askFailure(started time.Time, reason string) (skill.Output, error) {
	return skill.Output{
		Name:       "ask_main",
		Command:    "ask_main",
		Content:    reason,
		RawOutput:  reason,
		Stderr:     reason,
		Success:    false,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

// ---------------------------------------------------------------------------
// The parent's half: task_answer
// ---------------------------------------------------------------------------

type taskAnswerTool struct{ runner *runner }

func (t *taskAnswerTool) Name() string { return "task_answer" }

func (t *taskAnswerTool) Description() string {
	return "Answer a question a sub-agent asked, so it can carry on. " +
		"Use it when task_result came back saying a sub-agent is waiting for a decision. " +
		"The sub-agent resumes with everything it had already done still in hand, so answering is far " +
		"cheaper than starting it again. Collect it with task_result afterwards."
}

func (t *taskAnswerTool) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The id of the sub-agent that asked, e.g. \"task_1\".",
			},
			"answer": map[string]any{
				"type": "string",
				"description": "The decision, and anything it needs to act on it. The sub-agent cannot see " +
					"this conversation, so an answer like \"the first one\" means nothing to it — name the choice.",
			},
		},
		"required":             []string{"task_id", "answer"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  payload,
		},
	}
}

func (t *taskAnswerTool) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	return skill.Output{}, fmt.Errorf("task_answer is called by the model, not from the command line")
}

func (t *taskAnswerTool) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	started := time.Now()
	id := strings.TrimSpace(stringArg(args, "task_id"))
	answer := strings.TrimSpace(stringArg(args, "answer"))
	if id == "" {
		return t.fail(started, "task_id is required — it is the id of the sub-agent that asked")
	}
	if answer == "" {
		return t.fail(started, "answer is required — a sub-agent waiting on an empty answer is a sub-agent still stuck")
	}
	if err := t.runner.answer(id, answer); err != nil {
		return t.fail(started, err.Error())
	}
	msg := fmt.Sprintf("answered %s — it is running again. Collect it with task_result when you are ready.", id)
	return skill.Output{
		Name:       t.Name(),
		Command:    "task_answer " + id,
		Content:    msg,
		RawOutput:  msg,
		Success:    true,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

func (t *taskAnswerTool) fail(started time.Time, reason string) (skill.Output, error) {
	if waiting := t.runner.waiting(); len(waiting) > 0 {
		reason += "\nwaiting for an answer: " + strings.Join(waiting, ", ")
	}
	return skill.Output{
		Name:       t.Name(),
		Command:    "task_answer",
		Content:    reason,
		RawOutput:  reason,
		Stderr:     reason,
		Success:    false,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}
