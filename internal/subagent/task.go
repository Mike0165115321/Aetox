package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// The `task` tool: the one way a sub-agent ever runs (ARCHITECTURE.md §44.4).
//
// It cannot live in internal/skill — turn imports skill, and this needs turn and
// cognitive — so the host registers it at bootstrap, the same way the desktop
// injects ask_user/todo_write. Everything it needs from the session arrives in
// TaskOptions; it holds no global state.
//
// One call does NOT wait: it picks a profile, builds the child's registry from
// it, starts a full turn in the background and returns a handle. `task_result`
// redeems the handle later. See runner.go for why that shape and not another.

// defaultProfile is what `task` spawns when the model names nothing: the
// read-only searcher, because that is both the cheapest delegate and the one a
// model reaches for most.
const defaultProfile = "explore"

// TaskOptions is everything the host has to lend a delegate. Fields the session
// owns (provider, permissions, the live registry) are passed rather than looked
// up, so a re-bootstrap replaces the tool along with everything else instead of
// leaving it pointed at a dead engine.
type TaskOptions struct {
	Provider     model.Provider
	Model        string
	Registry     *skill.Registry // the session's registry; the child gets a filtered copy
	Permissions  safety.PermissionConfig
	ApprovalMode safety.ApprovalMode
	Approve      turn.ApprovalPromptFunc
	// OnToolAction is the parent's live tool feed. Every event a delegate causes
	// is stamped with the `task` call's own id before it goes down this channel,
	// so the UI can tell whose work it is.
	OnToolAction func(turn.ToolEvent)
	// OnUsage is the parent's usage reporter — a delegate's tokens are the user's
	// tokens, so they land in the same stats with no extra plumbing.
	OnUsage    func(model.Usage)
	MaxChars   int
	ThinkLevel think.Level
}

type taskTool struct {
	opts   TaskOptions
	runner *runner
}

// NewTaskTools builds the delegation pair — `task` to start one, `task_result` to
// collect it — sharing one runner, because they are two halves of one mechanism.
// Register both into the same registry passed in opts.Registry; FilterRegistry
// drops both from every child, so depth stays 1 structurally rather than by a
// counter.
func NewTaskTools(opts TaskOptions) []skill.Skill {
	shared := newRunner()
	return []skill.Skill{&taskTool{opts: opts, runner: shared}, &taskResultTool{runner: shared}}
}

func (t *taskTool) Name() string { return "task" }

// Description is also the only thing standing between a model and delegating
// everything, so it states the rule rather than hinting at it. The cost argument
// is the honest one: a delegate pays for a second system prompt and its own tool
// list, so anything you can already name is cheaper done here.
func (t *taskTool) Description() string {
	return "Start a self-contained job on a sub-agent. RETURNS IMMEDIATELY with a task id — it does NOT wait. " +
		"Do other useful work in the meantime (read the next file, start a second sub-agent), then call " +
		"task_result with the id to collect the answer. If you have nothing else to do, collect it straight away. " +
		"The sub-agent has NO access to this conversation, so `prompt` must carry everything it needs. " +
		"WHEN TO USE: the work would otherwise pour a lot into this conversation — hunting through many " +
		"files for something you cannot name yet, or the same mechanical change repeated across many places. " +
		"WHEN NOT TO: anything you can already name. Reading a file, one grep, one edit, a handful of known " +
		"paths — do those yourself. A delegate costs a second system prompt and its own tool list on every " +
		"round, so a small job is strictly more expensive delegated than done here. Available: " +
		strings.Join(profileNames(), ", ") + "."
}

func profileNames() []string {
	profiles := List()
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names
}

func (t *taskTool) ToolDefinition() model.ToolDefinition {
	profiles := List()
	names := make([]string, 0, len(profiles))
	described := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
		described = append(described, p.Name+" — "+p.Description)
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "A few words naming the job, for the user's timeline (e.g. \"find every caller of Resolve\").",
			},
			"prompt": map[string]any{
				"type": "string",
				"description": "The complete brief. The sub-agent sees no conversation history, " +
					"so include the paths, names and constraints it needs, and say what its answer must contain.",
			},
			"agent": map[string]any{
				"type":        "string",
				"enum":        names,
				"description": "Which sub-agent to use. " + strings.Join(described, " | "),
			},
		},
		"required":             []string{"description", "prompt"},
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

// Execute exists because skill.Skill requires it; `task` is model-only — there is
// no grammar for it and no reason to type it by hand.
func (t *taskTool) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	return skill.Output{}, fmt.Errorf("task is called by the model, not from the command line")
}

func (t *taskTool) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	started := time.Now()
	name := strings.TrimSpace(stringArg(args, "agent"))
	if name == "" {
		name = defaultProfile
	}
	brief := strings.TrimSpace(stringArg(args, "prompt"))
	label := strings.TrimSpace(stringArg(args, "description"))
	if label == "" {
		label = name
	}

	if brief == "" {
		return t.fail(label, started, "the prompt is empty — a sub-agent sees no conversation history, so the brief has to carry the whole job")
	}
	profile, ok := Load(name)
	if !ok {
		return t.fail(label, started, fmt.Sprintf("no sub-agent named %q; available: %s", name, strings.Join(profileNames(), ", ")))
	}
	if t.opts.Provider == nil || t.opts.Registry == nil {
		return t.fail(label, started, "the engine is not ready to spawn a sub-agent")
	}

	defer debuglog.Block("task: " + profile.Name + " — " + truncate(label, 60))()

	// The child's tool set is its profile's, minus what every delegate is refused
	// — `task` above all, which is what keeps depth at 1.
	childRegistry := FilterRegistry(t.opts.Registry, profile)
	if len(childRegistry.Names()) == 0 {
		return t.fail(label, started, fmt.Sprintf("sub-agent %q was left with no tools at all", profile.Name))
	}

	childModel := t.opts.Model
	if profile.Model != "" {
		childModel = profile.Model
	}

	// Everything below the goroutine boundary is built here, on the calling
	// goroutine, so a configuration mistake is reported as a failed tool call
	// rather than surfacing minutes later out of a background run.
	parentRef := turn.CallID(ctx)
	child := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     t.opts.Provider,
		Model:        childModel,
		SystemPrompt: profile.Prompt,
		MaxChars:     t.opts.MaxChars,
		MaxToolCalls: profile.MaxToolCalls(),
	})
	if t.opts.OnUsage != nil {
		child.SetUsageReporter(t.opts.OnUsage)
	}
	// A delegate inherits the session's prohibitions and adds its own; it never
	// inherits a permission the session has that its profile does not.
	permissions := safety.PermissionConfig{
		Rules: append(append([]safety.PermissionRule{}, t.opts.Permissions.Rules...), profile.DenyRules()...),
	}

	task, err := t.runner.start(ctx, profile.Name, label, func(runCtx context.Context, self *runningTask) skill.Output {
		defer debuglog.Block("task: " + profile.Name + " — " + truncate(label, 60))()

		// Every event the delegate causes is stamped with the `task` call's id, so
		// the UI can nest it instead of mixing it into the main agent's timeline.
		relay := func(ev turn.ToolEvent) {
			if ev.Action == "call" {
				self.countCall()
			}
			if t.opts.OnToolAction != nil {
				ev.Parent = parentRef
				t.opts.OnToolAction(ev)
			}
		}
		exec := turn.NewExecutor(turn.ExecutorOptions{
			Agent:        child,
			Dispatcher:   skill.NewDispatcher(childRegistry),
			Approve:      t.opts.Approve,
			ApprovalMode: t.opts.ApprovalMode,
			Permissions:  permissions,
			OnToolAction: relay,
			TurnOptions:  turn.TurnOptions{ThinkLevel: t.opts.ThinkLevel},
		})

		// An explicit Intent is load-bearing: without one the executor parses the
		// brief, and a brief that happens to start with a tool name ("read every
		// test file and…") would run as a single explicit tool call, not a turn.
		result, runErr := exec.Execute(runCtx, brief, command.Intent{Raw: brief, Kind: command.KindConversation}, nil, nil, nil)
		elapsed := time.Since(self.started)

		// Cancellation is checked BEFORE runErr and independently of it: a stopped
		// turn can come back with runErr == nil carrying the empty-reply fallback,
		// and calling that a successful delegation is a lie the user cannot see —
		// they pressed Stop.
		if runCtx.Err() != nil {
			return failure(self.id, label, elapsed, "sub-agent stopped: "+runCtx.Err().Error())
		}
		if runErr != nil {
			return failure(self.id, label, elapsed, "sub-agent failed: "+runErr.Error())
		}

		reply := strings.TrimSpace(result.Reply)
		if reply == "" {
			reply = "(the sub-agent returned nothing)"
		}
		// What the parent model sees: the result, plus one line of receipt. NOT the
		// delegate's tool log — that would put back the context cost delegating just
		// saved (§44.6). The user sees every step live in the UI instead.
		content := reply + "\n" + receiptFor(profile.Name, self.calls(), elapsed)
		return skill.Output{
			Name:       "task_result",
			Command:    "task " + profile.Name,
			Content:    content,
			RawOutput:  content,
			Success:    true,
			DurationMs: elapsed.Milliseconds(),
		}
	})
	if err != nil {
		return t.fail(label, started, err.Error())
	}

	started_ := fmt.Sprintf("started sub-agent %s as %s — it is running now. Do other work, then call task_result with task_id %q to collect it.",
		profile.Name, task.id, task.id)
	return skill.Output{
		Name:       t.Name(),
		Command:    "task " + profile.Name,
		Content:    started_,
		RawOutput:  started_,
		Success:    true,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

// failure is the shape a background run reports a refusal in: a failed result the
// collector hands to the model, never a Go error nobody is left to receive.
func failure(id, label string, elapsed time.Duration, reason string) skill.Output {
	return skill.Output{
		Name:       "task_result",
		Command:    "task " + label,
		Content:    reason,
		RawOutput:  reason,
		Stderr:     reason,
		Success:    false,
		DurationMs: elapsed.Milliseconds(),
	}
}

// receiptFor is the one line the parent model gets about how the delegation went.
//
// When the delegate did almost nothing, the receipt says so. Judging *afterwards*
// is the honest way round: how big a job turns out to be is not knowable from the
// brief, so a pre-flight heuristic would refuse real work and wave through
// pointless work. A model that reads "you could have done that here" mid-turn
// stops doing it for the rest of the conversation, which is the behaviour the
// description alone cannot enforce.
//
// ponytail: the threshold is tool calls, not seconds — one slow grep is still one
// call, and wall-clock says more about the disk than about the work. Revisit if a
// one-call delegate ever turns out to be worth it.
func receiptFor(name string, toolCalls int, elapsed time.Duration) string {
	receipt := fmt.Sprintf("[task %s: %d tool calls, %.1fs]", name, toolCalls, elapsed.Seconds())
	if toolCalls <= 1 {
		receipt += " NOTE: that was one tool call — small enough to have done here, and delegating it cost a whole second context. Do work this size yourself."
	}
	return receipt
}

// fail reports a refusal to the model as a normal unsuccessful tool result: it
// can read the reason and try something else, which is more useful than an error
// that reaches the user as a crash.
func (t *taskTool) fail(label string, started time.Time, reason string) (skill.Output, error) {
	return skill.Output{
		Name:       t.Name(),
		Command:    "task " + label,
		Content:    reason,
		RawOutput:  reason,
		Stderr:     reason,
		Success:    false,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

func stringArg(args map[string]any, key string) string {
	if raw, ok := args[key].(string); ok {
		return raw
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
