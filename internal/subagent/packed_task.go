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

// Description is the one place the rules of delegating are written, and every
// clause in it is load-bearing enough to have a test: the brief is self-
// contained, small work is cheaper done here, and a list is ONE delegate looping.
func (d *delegationTool) Description() string {
	return "Sub-agents: hand a self-contained job to one, collect it, answer one that got stuck, " +
		"and declare a run when the work takes more than one wave of them. See `action`. " +
		"They have NO access to this conversation, so `prompt` must carry everything they need. " +
		"Starting one RETURNS IMMEDIATELY with a task id — do other useful work, then collect. " +
		"WHEN TO USE: work that would otherwise pour a lot into this conversation — hunting through many " +
		"files for something you cannot name yet, or one mechanical change repeated across many places. " +
		"WHEN NOT TO: anything you can already name. Reading a file, one grep, one edit — do those yourself. " +
		"A delegate costs a second system prompt and its own tool list on every round. " +
		"REPEATED WORK IS ONE JOB: hand the whole list to ONE of them and let it loop — twelve items is one " +
		"task with twelve items in its prompt, never twelve tasks, because each one pays for its own context. " +
		"Available: " + strings.Join(profileNames(d.start.available()), ", ") + "."
}

// actionLines are written per action and assembled from the permitted set, so a
// narrowed tool never advertises a call this caller would be refused.
func (d *delegationTool) actionLines() string {
	lines := map[string]string{
		"start": "`start` (prompt, description, agent) — hand over a job. Default: a call with no action starts one. " +
			"Returns a task id and does NOT wait.",
		"collect": "`collect` (task_id) — redeem an id, waiting only if it is not finished. Several ids, comma separated, " +
			"cost the time of the slowest rather than the sum. One that got stuck comes back as a QUESTION instead " +
			"of an answer. Collect everything you start: a result nobody reads is work nobody used.",
		"answer": "`answer` (task_id, answer) — reply to that question. It resumes with everything it had already done " +
			"in hand, so answering costs far less than starting it over.",
		"plan": "`plan` (name, brief, phases) — declare a run BEFORE starting any of it: the stages this piece of work " +
			"goes through, in order, including the ones that have not happened yet. Starts nothing. Every start after " +
			"it names one of those phases. A phase declared and left empty sits at zero on the user's screen for the " +
			"whole run, which is what makes a promised checking round hard to skip quietly.",
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
			// The failure this clause exists for: a user who names the worker has
			// already written the brief, and paraphrasing it is where "ask doc how a
			// good document is put together" became "write a manual about…". The
			// worker only ever sees the paraphrase, so it cannot check it.
			"description": "start: the complete brief — paths, names, constraints. When the user asked for this " +
				"worker themselves, their own words ARE the brief: carry them through as written and add context " +
				"around them. A word you are unsure of crosses over exactly as typed; the worker can ask, and a " +
				"guess cannot be un-asked.",
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
				"description": "start: which phase of the run \"" + run.name + "\" this job belongs to. Name one every time.",
			}
		}
	}
	if slices.Contains(allowed, "collect") || slices.Contains(allowed, "answer") {
		properties["task_id"] = map[string]any{
			"type":        "string",
			"description": "collect/answer: the id start returned, e.g. \"task_1\". For several, separate with commas.",
		}
	}
	if slices.Contains(allowed, "answer") {
		properties["answer"] = map[string]any{
			"type": "string",
			"description": "answer: the decision, in full. The sub-agent cannot see this conversation, so " +
				"\"the first one\" means nothing to it — name the choice.",
		}
	}
	if slices.Contains(allowed, "plan") {
		properties["name"] = map[string]any{
			"type":        "string",
			"description": "plan: a few words naming the run, in the user's language. The headline above the group.",
		}
		properties["brief"] = map[string]any{
			"type":        "string",
			"description": "plan: one sentence for the user saying what this run does.",
		}
		properties["phases"] = map[string]any{
			"type":        "array",
			"description": "plan: the stages, in order. `title` is named exactly as written; `agents` is how many jobs you expect there, left out when you do not know.",
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
