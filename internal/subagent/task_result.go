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

// task_result: the other half of delegation (see runner.go). It redeems a handle
// `task` handed out — waiting only if the delegate is not finished yet, which by
// then is time the model chose to spend rather than time it was forced to.

type taskResultTool struct{ runner *runner }

func (t *taskResultTool) Name() string { return "task_result" }

func (t *taskResultTool) Description() string {
	return "Collect a sub-agent started with task, by its task id. Waits only if it has not finished. " +
		"Pass several ids to collect several at once — sub-agents started before the first collect run " +
		"concurrently, so collecting three costs the time of the slowest, not the sum. " +
		"Every task you start must be collected: an uncollected sub-agent's work is thrown away when the turn ends."
}

func (t *taskResultTool) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The id task returned, e.g. \"task_1\". For several, separate with commas.",
			},
		},
		"required":             []string{"task_id"},
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

// Execute exists because skill.Skill requires it; this tool is model-only.
func (t *taskResultTool) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	return skill.Output{}, fmt.Errorf("task_result is called by the model, not from the command line")
}

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
		task, err := t.runner.collect(ctx, ids[0])
		if err != nil {
			return t.fail(started, err.Error())
		}
		out := task.output
		out.DurationMs = time.Since(task.started).Milliseconds()
		return out, nil
	}

	// Several: report each under its id, and succeed if any did. A batch that
	// failed as a whole is still one message the model can read, not an error.
	var b strings.Builder
	anyOK := false
	for _, id := range ids {
		task, err := t.runner.collect(ctx, id)
		if err != nil {
			fmt.Fprintf(&b, "--- %s ---\n%s\n", id, err.Error())
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
