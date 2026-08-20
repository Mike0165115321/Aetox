package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// Delegation as ONE name in the tool block, four rights inside it (§99).
//
// It was four names until 2026-08-16, and the fourth is why it stopped being
// four: declaring a run (run.go) needed a tool, and there was no room for one —
// the block sat at 10,004 of its 10,100 tokens with 22% of it already spent on
// this family. Owner's call: pack it, the same answer §99 gave shell.
//
// Outside is `task`. Inside, every gate below the block still judges the old
// per-action name through skill.Unpack — the approval prompt, the permission
// rules a user wrote, the turn's deadline (executor.go stretches it for exactly
// this tool). The act did not change; only the spelling of the call did.
//
// This file owns everything the MODEL reads about delegating. The four
// implementations underneath it own what happens; none of them describes itself
// any more, because a tool that is one name outside cannot have four
// descriptions and still have one answer to "what is this".
type delegationTool struct {
	start   *taskTool
	collect *taskResultTool
	answer  *taskAnswerTool
	plan    *taskPlanTool
	// actions this caller may use, nil for all of them. Set only by Narrow.
	actions []string
}

func (*delegationTool) Name() string { return "task" }

// allowedActions is the whole set unless Narrow said otherwise.
func (d *delegationTool) allowedActions() []string {
	if len(d.actions) == 0 {
		out := make([]string, 0, 4)
		for _, c := range skill.PackedCalls("task") {
			out = append(out, c.Action)
		}
		return out
	}
	return d.actions
}

func (*delegationTool) Actions() []string { return skill.PackedActions("task") }

// Narrow hands back a copy offering only the named actions. A caller that names
// none gets the tool whole — it did not ask for a narrower tool, it asked for
// this one (packed.go's silence rule).
func (d *delegationTool) Narrow(named []string) skill.Skill {
	want := make(map[string]bool, len(named))
	for _, n := range named {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var allowed []string
	for _, c := range skill.PackedCalls("task") {
		if want[c.Permission] {
			allowed = append(allowed, c.Action)
		}
	}
	if len(allowed) == 0 {
		return d
	}
	narrowed := *d
	narrowed.actions = allowed
	return &narrowed
}

// forChair is this tool as a chair chat gets it (§151): the same mechanism,
// with a roster of helpers alone and no `task_plan`.
//
// Helpers alone because a chair is itself a colleague, and one colleague handing
// a whole job to another is two peers deciding whose job it was, one level below
// the person who asked. A helper is the other thing entirely — the caller's own
// work in a second context — and is exactly what a chair runs short of: reading
// twenty sources to build a deck spends the context the deck was to be built in.
//
// Expressed as NoAgents, the same switch the user's own settings page turns,
// because it is the same question — may this session hand a whole job to a
// colleague — and a chair answering it through a mechanism of its own would be
// a second answer to drift from the first.
//
// No `task_plan` for the reason `todo_write` is also absent from a chair chat: a
// run declared here would draw a second panel over the one the person is already
// watching. Starting, collecting and answering are the whole of the mechanism
// without it.
func (d *delegationTool) forChair() skill.Skill {
	start := *d.start
	start.opts.NoAgents = true
	withRoster := *d
	withRoster.start = &start
	return withRoster.Narrow([]string{"task", "task_result", "task_answer"})
}

// Description is the one place the rules of delegating are written, and every
// clause in it is load-bearing enough to have a test: the brief is self-
// contained, small work is cheaper done here, and a list is ONE delegate looping.
func (d *delegationTool) Description() string {
	// WHEN TO USE / WHEN NOT TO stays in the block, and it is the only judgment
	// in this tool that does. Everything else here is advice about a delegation
	// already decided on, which the first result carries in time. Whether to
	// delegate AT ALL is decided before any call exists, so guidance attached to
	// the first `start` would reach only a model that already chose to start
	// one — the same shape as the `wait` trigger.
	//
	// Compressed rather than moved, because the failure it prevents is expensive
	// and does not correct itself: delegating a single grep costs a whole second
	// context to find that out.
	//
	// REPEATED WORK IS ONE JOB moved to Guidance. It shapes a delegation that is
	// already happening, and one over-fanned wave is visible in the result.
	return "Sub-agents: hand a self-contained job to one, collect it, answer one that got stuck, " +
		"and declare a run when the work takes more than one wave. See `action`. " +
		"WHEN TO USE: work that would otherwise pour a lot into this conversation — a hunt through many " +
		"files for something you cannot name yet, one mechanical change repeated across many places. " +
		"WHEN NOT TO: anything you can already name. One read, one grep, one edit — do those yourself; " +
		"a delegate costs a second system prompt and its own tool list on every round."
}

// actionLines are written per action and assembled from the permitted set, so a
// narrowed tool never advertises a call this caller would be refused.
func (d *delegationTool) actionLines() string {
	// Signatures. Everything these used to explain about when and why now lives
	// in Guidance and is sent once, with the first call to that action — see
	// internal/skill/guidance.go and task_guidance.go.
	lines := map[string]string{
		"start":   "`start` (prompt, description, agent) — hand over a job. The default action. Returns a task id and does NOT wait.",
		"collect": "`collect` (task_id) — redeem an id, or several comma separated.",
		"answer":  "`answer` (task_id, answer) — reply to one that came back as a question.",
		"plan":    "`plan` (name, brief, phases) — declare a run before starting any of it. Starts nothing.",
	}
	var b strings.Builder
	for _, a := range d.allowedActions() {
		b.WriteString(lines[a] + "\n")
	}
	return b.String()
}

func (d *delegationTool) ToolDefinition() model.ToolDefinition {
	allowed := d.allowedActions()
	profiles := d.start.available()
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}

	properties := map[string]any{
		"action": map[string]any{
			"type":        "string",
			"enum":        allowed,
			"description": "What to do. Omit to start a job.\n" + d.actionLines(),
		},
	}
	if slices.Contains(allowed, "start") {
		properties["description"] = map[string]any{
			"type":        "string",
			"description": "start: a few words naming the job, for the user's timeline.",
		}
		properties["prompt"] = map[string]any{
			"type": "string",
			// The rest of what this used to say — why a paraphrase is dangerous —
			// moved to Guidance. It is advice about writing a brief, read at the
			// moment of writing one, and the first result carries it in time.
			"description": "start: the complete brief — paths, names, constraints. The worker cannot see this conversation.",
		}
		properties["agent"] = map[string]any{
			"type":        "string",
			"enum":        names,
			"description": agentChoice(profiles),
		}
		if run := d.start.runner.currentRun(); run != nil {
			properties["phase"] = map[string]any{
				"type":        "string",
				"enum":        run.phaseTitles(),
				"description": "start: which phase of the run \"" + run.name + "\" this job belongs to.",
			}
		}
	}
	if slices.Contains(allowed, "collect") || slices.Contains(allowed, "answer") {
		properties["task_id"] = map[string]any{
			"type":        "string",
			"description": "collect/answer: the id start returned, e.g. \"task_1\".",
		}
	}
	if slices.Contains(allowed, "answer") {
		properties["answer"] = map[string]any{
			"type":        "string",
			"description": "answer: the decision, in full.",
		}
	}
	if slices.Contains(allowed, "plan") {
		properties["name"] = map[string]any{
			"type":        "string",
			"description": "plan: a few words naming the run, in the user's language.",
		}
		properties["brief"] = map[string]any{
			"type":        "string",
			"description": "plan: one sentence for the user saying what this run does.",
		}
		properties["phases"] = map[string]any{
			"type":        "array",
			"description": "plan: the stages, in order.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":  map[string]any{"type": "string"},
					"agents": map[string]any{"type": "integer"},
				},
				"required":             []string{"title"},
				"additionalProperties": false,
			},
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        d.Name(),
			Description: d.Description(),
			Parameters:  payload,
		},
	}
}

// Execute exists because skill.Skill requires it; delegation is model-only —
// there is no grammar for it and no reason to type it by hand.
func (*delegationTool) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	return skill.Output{}, fmt.Errorf("task is called by the model, not from the command line")
}

func (d *delegationTool) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	action := strings.ToLower(strings.TrimSpace(stringArg(args, "action")))
	if action == "" {
		action = "start" // the pack's fallback, spelled out here too: see packed.go
	}
	// A typo is refused rather than run as the default. Naming what this caller
	// may use is what turns a wrong word into a correctable mistake — and for a
	// narrowed tool it is the only place the smaller set is said out loud.
	if !slices.Contains(d.allowedActions(), action) {
		reason := fmt.Sprintf("task has no action %q here; available: %s",
			action, strings.Join(d.allowedActions(), ", "))
		return skill.Output{
			Name: "task", Command: "task", Content: reason, RawOutput: reason,
			Stderr: reason, Success: false,
		}, nil
	}
	switch action {
	case "collect":
		return d.collect.ExecuteTool(ctx, args)
	case "answer":
		return d.answer.ExecuteTool(ctx, args)
	case "plan":
		return d.plan.ExecuteTool(ctx, args)
	default:
		return d.start.ExecuteTool(ctx, args)
	}
}

// Direct and Answer are the host's door onto the same machinery — a user's own
// `@name` (mention.go), which is not a tool call at all. Forwarded rather than
// reimplemented so a worker cannot tell which door the job came through.
func (d *delegationTool) Direct(ctx context.Context, agent, brief string) (Reply, error) {
	return d.start.Direct(ctx, agent, brief)
}

func (d *delegationTool) Answer(ctx context.Context, taskID, answer string) (Reply, error) {
	return d.start.Answer(ctx, taskID, answer)
}
