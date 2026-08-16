package subagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// task_plan: declare a run before starting any of it (run.go).
//
// It starts nothing and waits for nothing. That is the point — everything this
// tool does happens on the user's screen, where a phase named "หักล้าง" sits at
// 0/8 until somebody actually does it. The model already had every mechanism it
// needed to fan out and then check its own findings; what it did not have was a
// way to say out loud, before the answer existed, that the checking round was
// part of the job.

type taskPlanTool struct{ runner *Delegations }

func (t *taskPlanTool) Name() string { return "task_plan" }
func (t *taskPlanTool) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	started := time.Now()
	name := strings.TrimSpace(stringArg(args, "name"))
	phases, err := parsePhases(args["phases"])
	if err != nil {
		return t.fail(started, err.Error())
	}
	run, err := t.runner.plan(name, stringArg(args, "brief"), phases)
	if err != nil {
		return t.fail(started, err.Error())
	}

	// What comes back is the phase list as the schema will now enforce it, and
	// the one thing the model has to do differently from here on. A confirmation
	// that only said "declared" would leave it to remember a parameter that did
	// not exist a moment ago.
	content := fmt.Sprintf("declared the run %q (%s) with phases: %s.\n"+
		"Every task you start for this run must pass `phase` with one of those titles, exactly as written. "+
		"Work you start without one runs on its own and does not appear in this run.",
		run.name, run.id, strings.Join(run.phaseTitles(), ", "))
	return skill.Output{
		Name:       t.Name(),
		Command:    "task_plan " + run.name,
		Content:    content,
		RawOutput:  content,
		Success:    true,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

// parsePhases reads the declaration out of whatever the provider handed over.
//
// Both shapes are accepted because both turn up in practice: objects, which is
// what the schema asks for, and bare strings, which is what a model writes when
// it has no count to give. Refusing the second would cost a round of the loop to
// teach a rule the result does not depend on.
func parsePhases(raw any) ([]RunPhase, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("phases must be a list of stages, in order")
	}
	out := make([]RunPhase, 0, len(list))
	for _, item := range list {
		switch v := item.(type) {
		case string:
			out = append(out, RunPhase{Title: v})
		case map[string]any:
			phase := RunPhase{Title: stringArg(v, "title")}
			// Numbers arrive as float64 through JSON and as int from a hand-written
			// call (the tests, and the desktop's own dispatch).
			switch n := v["agents"].(type) {
			case float64:
				phase.Planned = int(n)
			case int:
				phase.Planned = n
			}
			out = append(out, phase)
		default:
			return nil, fmt.Errorf("a phase must be a title, or an object with a title")
		}
	}
	return out, nil
}

func (t *taskPlanTool) fail(started time.Time, reason string) (skill.Output, error) {
	return skill.Output{
		Name:       t.Name(),
		Command:    "task_plan",
		Content:    reason,
		RawOutput:  reason,
		Stderr:     reason,
		Success:    false,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}
