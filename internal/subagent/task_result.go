package subagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// task_result: the other half of delegation (see runner.go). It redeems a handle
// `task` handed out — waiting only if the delegate is not finished yet, which by
// then is time the model chose to spend rather than time it was forced to.

type taskResultTool struct{ runner *Delegations }

func (t *taskResultTool) Name() string { return "task_result" }
func (t *taskResultTool) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	started := time.Now()
	raw := strings.TrimSpace(stringArg(args, "task_id"))
	if raw == "" {
		return t.fail(started, "task_id is required — it is the id task returned when you started the sub-agent")
	}

	ids := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			ids = append(ids, part)
		}
	}

	// One id: the result is the result, so hand it back as its own outcome — a
	// failed delegation stays a failed tool call the model can react to.
	if len(ids) == 1 {
		task, ask, err := t.runner.collect(ctx, ids[0])
		if err != nil {
			return t.fail(started, err.Error())
		}
		if ask != nil {
			return t.question(started, task, ask), nil
		}
		out := task.output
		out.DurationMs = time.Since(task.startedAt()).Milliseconds()
		return out, nil
	}

	// Several: report each under its id, and succeed if any did. A batch that
	// failed as a whole is still one message the model can read, not an error.
	var b strings.Builder
	anyOK := false
	for _, id := range ids {
		task, ask, err := t.runner.collect(ctx, id)
		if err != nil {
			fmt.Fprintf(&b, "--- %s ---\n%s\n", id, err.Error())
			continue
		}
		if ask != nil {
			anyOK = true // a question is not a failure; it is work waiting on one answer
			fmt.Fprintf(&b, "--- %s (%s) ---\n%s\n", id, task.profile, askedText(id, ask))
			continue
		}
		if task.output.Success {
			anyOK = true
		}
		fmt.Fprintf(&b, "--- %s (%s) ---\n%s\n", id, task.profile, strings.TrimSpace(task.output.Content))
	}
	content := strings.TrimRight(b.String(), "\n")
	return skill.Output{
		Name:       t.Name(),
		Command:    "task_result " + raw,
		Content:    content,
		RawOutput:  content,
		Success:    anyOK,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

// question is what the parent gets when it collects a delegate that is stuck.
// Success is true on purpose: nothing failed, and a red row in the timeline for
// "your delegate needs a decision" tells the user the wrong story. What makes it
// unmistakable is the text, which names the one tool that unsticks it.
func (t *taskResultTool) question(started time.Time, task *runningTask, ask *pendingAsk) skill.Output {
	content := askedText(task.id, ask)
	return skill.Output{
		Name:       t.Name(),
		Command:    "task_result " + task.id,
		Content:    content,
		RawOutput:  content,
		Success:    true,
		DurationMs: time.Since(started).Milliseconds(),
	}
}

func askedText(id string, ask *pendingAsk) string {
	return fmt.Sprintf(
		"NOT FINISHED — sub-agent %s is waiting for a decision from you and has stopped until it gets one:\n\n%s\n\n"+
			"Answer with task(action=answer, task_id=%q, answer=...), then collect again. "+
			"It resumes with everything it has already done still in hand, so answering costs far less than starting it over. "+
			"If the question cannot be answered, answer saying so and what to do instead — leaving it unanswered throws the whole run away.",
		id, strings.TrimSpace(ask.question), id)
}

func (t *taskResultTool) fail(started time.Time, reason string) (skill.Output, error) {
	// Naming what is still outstanding turns a wrong id into a correctable
	// mistake rather than a dead end.
	if outstanding := t.runner.running(); len(outstanding) > 0 {
		ids := make([]string, 0, len(outstanding))
		for _, task := range outstanding {
			ids = append(ids, fmt.Sprintf("%s (%s, %d tool calls so far)", task.id, task.profile, task.calls()))
		}
		reason += "\nstill running: " + strings.Join(ids, ", ")
	}
	return skill.Output{
		Name:       t.Name(),
		Command:    "task_result",
		Content:    reason,
		RawOutput:  reason,
		Stderr:     reason,
		Success:    false,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}
