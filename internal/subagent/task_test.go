package subagent

// The main agent calling `task` for real, on Aetox's own model for both sides
// (§45). This is the test the rehearsal in spawn_demo_test.go was standing in for.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// taskFixture wires a session the way bootstrapFromConfig does: a registry with
// `task` in it, a main agent on the built-in provider, and the executor between.
type taskFixture struct {
	root     string
	registry *skill.Registry
	exec     *turn.Executor

	// Delegates emit from their own goroutines, so the collectors are guarded —
	// the same reason desktop/app.go locks its tool history (§44.11).
	mu     sync.Mutex
	events []turn.ToolEvent
	usage  []model.Usage
}

func (f *taskFixture) toolEvents() []turn.ToolEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]turn.ToolEvent(nil), f.events...)
}

func newTaskFixture(t *testing.T, mainModel string) *taskFixture {
	t.Helper()
	isolate(t)
	f := &taskFixture{root: t.TempDir()}
	for name, body := range map[string]string{
		"hay.txt":  "line one\nthe needle is here\nline three\n",
		"notes.md": "# notes\n",
	} {
		if err := os.WriteFile(filepath.Join(f.root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	f.registry = skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: f.root})
	provider := model.NewNoopProvider(mainModel)
	onToolAction := func(ev turn.ToolEvent) {
		f.mu.Lock()
		f.events = append(f.events, ev)
		f.mu.Unlock()
	}

	for _, tool := range NewTaskTools(TaskOptions{
		Provider:     provider,
		Model:        mainModel,
		Registry:     f.registry,
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: onToolAction,
		OnUsage: func(u model.Usage) {
			f.mu.Lock()
			f.usage = append(f.usage, u)
			f.mu.Unlock()
		},
	}) {
		if err := f.registry.Register(tool, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}

	mainAgent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     provider,
		Model:        mainModel,
		SystemPrompt: "You are Aetox, a concise assistant.",
	})
	f.exec = turn.NewExecutor(turn.ExecutorOptions{
		Agent:        mainAgent,
		Dispatcher:   skill.NewDispatcher(f.registry),
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: onToolAction,
	})
	return f
}

// callTask starts a delegation the way the executor does — through the
// dispatcher, with a call id on the context, so ToolEvent.Parent gets stamped for
// real. It returns as soon as the sub-agent is running.
func (f *taskFixture) callTask(t *testing.T, callID string, args map[string]any) skill.Output {
	t.Helper()
	return f.call(t, callID, "task", args)
}

// collect redeems a handle. Blocks only if the delegate has not finished.
func (f *taskFixture) collect(t *testing.T, ids string) skill.Output {
	t.Helper()
	return f.call(t, "call_collect", "task_result", map[string]any{"task_id": ids})
}

func (f *taskFixture) call(t *testing.T, callID, tool string, args map[string]any) skill.Output {
	t.Helper()
	ctx := turn.WithCallID(context.Background(), callID)
	out, handled, err := skill.NewDispatcher(f.registry).ExecuteTool(ctx, tool, args)
	if !handled {
		t.Fatalf("%s is not registered", tool)
	}
	if err != nil {
		t.Fatalf("%s returned an error instead of a failed result: %v", tool, err)
	}
	return out
}

// taskIDOf pulls the handle out of what `task` told the model.
func taskIDOf(t *testing.T, out skill.Output) string {
	t.Helper()
	for _, word := range strings.Fields(strings.ReplaceAll(out.Content, "\"", " ")) {
		if strings.HasPrefix(word, "task_") {
			return strings.Trim(word, ".,\"")
		}
	}
	t.Fatalf("no task id in %q", out.Content)
	return ""
}

// The whole point, end to end: a delegate runs, does real work, and the parent
// gets back its text plus a receipt — not its transcript.
func TestTaskRunsADelegateAndReturnsOnlyItsResult(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	start := f.callTask(t, "call_parent_1", map[string]any{
		"description": "list the sandbox",
		"prompt":      "หาไฟล์ทั้งหมดในโฟลเดอร์นี้แล้วรายงานชื่อไฟล์",
		"agent":       "explore",
	})
	t.Logf("task said: %s", start.Content)
	if !start.Success {
		t.Fatalf("start failed: %s", start.Stderr)
	}
	// Starting must NOT return the answer — that is the whole point (§44.11): the
	// model gets a handle back and its turn carries on.
	if strings.Contains(start.Content, "hay.txt") {
		t.Errorf("task blocked and returned the result: %q", start.Content)
	}
	if !strings.Contains(start.Content, "task_result") {
		t.Errorf("task did not tell the model how to collect: %q", start.Content)
	}

	out := f.collect(t, taskIDOf(t, start))
	t.Logf("collected:\n%s", out.Content)
	if !out.Success {
		t.Fatalf("collect failed: %s", out.Stderr)
	}
	// The delegate's real tool result reached the parent…
	if !strings.Contains(out.Content, "hay.txt") {
		t.Errorf("the delegate's finding is missing from the result: %q", out.Content)
	}
	// …with a receipt naming the profile and counting the work.
	if !strings.Contains(out.Content, "[task explore:") || !strings.Contains(out.Content, "tool calls") {
		t.Errorf("no receipt line: %q", out.Content)
	}

	// Every event the delegate caused is stamped with the parent call's id, and
	// the parent's own events are not.
	var stamped, unstamped int
	for _, ev := range f.toolEvents() {
		if ev.Parent == "call_parent_1" {
			stamped++
		} else if ev.Parent == "" {
			unstamped++
		} else {
			t.Errorf("event stamped with an unexpected parent %q", ev.Parent)
		}
	}
	t.Logf("events: %d stamped as the delegate's, %d the parent's own", stamped, unstamped)
	if stamped == 0 {
		t.Error("no delegate events were stamped — the UI could not nest them")
	}

	// Usage is asserted separately: Aetox's own provider reports none, and
	// fabricating token counts in it would put invented spend on the Usage page.
}

// usageProvider is the one fake §45 still allows: a provider *edge case* — here,
// a provider that fills Response.Usage, which the built-in one deliberately does
// not. Everything else about the path is real.
type usageProvider struct{}

func (usageProvider) Name() string              { return "usage-fake" }
func (usageProvider) SupportsToolCalling() bool { return true }
func (usageProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	return model.Response{Text: "done", Usage: &model.Usage{PromptTokens: 120, CompletionTokens: 8}}, nil
}

// A delegate's tokens are the user's tokens — they have to land in the same stats
// as the main agent's, or delegation becomes a way to spend money invisibly.
func TestTaskDelegateUsageReachesTheParent(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root})
	var usage []model.Usage
	var mu sync.Mutex
	for _, tool := range NewTaskTools(TaskOptions{
		Provider:     usageProvider{},
		Model:        "usage-fake-model",
		Registry:     registry,
		ApprovalMode: safety.ApprovalFullAccess,
		OnUsage: func(u model.Usage) {
			mu.Lock()
			usage = append(usage, u)
			mu.Unlock()
		},
	}) {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}

	ctx := turn.WithCallID(context.Background(), "call_1")
	dispatcher := skill.NewDispatcher(registry)
	start, _, err := dispatcher.ExecuteTool(ctx, "task", map[string]any{
		"description": "x", "prompt": "report the files", "agent": "explore",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	// Collect, so the delegate has certainly finished before usage is inspected.
	out, _, err := dispatcher.ExecuteTool(ctx, "task_result", map[string]any{
		"task_id": taskIDOf(t, start),
	})
	if err != nil || !out.Success {
		t.Fatalf("task failed: %v %q", err, out.Content)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(usage) == 0 {
		t.Fatal("the delegate's usage never reached the parent's reporter")
	}
	t.Logf("usage relayed to the parent: %+v", usage)
	if usage[0].PromptTokens != 120 {
		t.Errorf("relayed usage = %+v, want the delegate's real numbers", usage[0])
	}
}

// A delegate must not be handed what its profile excludes, and `task` least of
// all — that is what keeps the depth at 1 without a counter.
func TestTaskDelegateCannotRecurseOrMutate(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	profile, _ := Load("explore")
	child := FilterRegistry(f.registry, profile)
	for _, name := range []string{"task", "write", "edit", "shell", "help", "ask_user", "todo_write"} {
		if _, ok := child.Get(name); ok {
			t.Errorf("%q reached the delegate's registry", name)
		}
	}
	if _, ok := f.registry.Get("task"); !ok {
		t.Error("task is missing from the parent's own registry")
	}

	// general inherits everything the parent has, still minus task.
	general, _ := Load("general")
	generalRegistry := FilterRegistry(f.registry, general)
	if _, ok := generalRegistry.Get("write"); !ok {
		t.Error("general lost write, which it inherits")
	}
	if _, ok := generalRegistry.Get("task"); ok {
		t.Error("general could spawn its own children")
	}
}

// Bad input comes back as a failed result the model can read and retry, not as an
// error that surfaces to the user as a crash.
func TestTaskRefusesBadInputAsAResult(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	empty := f.callTask(t, "call_1", map[string]any{"description": "x", "prompt": "   "})
	if empty.Success || !strings.Contains(empty.Content, "brief") {
		t.Errorf("empty prompt: success=%v content=%q", empty.Success, empty.Content)
	}

	unknown := f.callTask(t, "call_2", map[string]any{"description": "x", "prompt": "do it", "agent": "nope"})
	if unknown.Success || !strings.Contains(unknown.Content, "no sub-agent named") {
		t.Errorf("unknown profile: success=%v content=%q", unknown.Success, unknown.Content)
	}
	// The refusal names what IS available, so the model's retry can be right.
	if !strings.Contains(unknown.Content, "explore") {
		t.Errorf("the refusal does not list the available profiles: %q", unknown.Content)
	}

	// A traversal attempt is just an unknown name, not a file read.
	escape := f.callTask(t, "call_3", map[string]any{"description": "x", "prompt": "do it", "agent": "../../etc/passwd"})
	if escape.Success {
		t.Error("a path-shaped profile name was accepted")
	}
}

// No `agent` argument means the cheapest delegate, not a failure.
func TestTaskDefaultsToExplore(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	start := f.callTask(t, "call_1", map[string]any{"description": "look around", "prompt": "รายงานไฟล์ในโฟลเดอร์"})
	if !start.Success {
		t.Fatalf("default spawn failed: %s", start.Stderr)
	}
	if !strings.Contains(start.Content, "explore") {
		t.Errorf("default profile is not explore: %q", start.Content)
	}
	if out := f.collect(t, taskIDOf(t, start)); !strings.Contains(out.Content, "[task explore:") {
		t.Errorf("the receipt does not name the default profile: %q", out.Content)
	}
}

// The tool the model sees has to describe what it can pick, or it will guess.
func TestTaskDefinitionListsTheProfiles(t *testing.T) {
	isolate(t)
	def := toolDefOf(t, NewTaskTools(TaskOptions{}), "task")
	schema := string(def.Function.Parameters)
	for _, want := range []string{"explore", "general", "\"required\"", "prompt", "description"} {
		if !strings.Contains(schema, want) {
			t.Errorf("schema is missing %q: %s", want, schema)
		}
	}
	if !strings.Contains(def.Function.Description, "NO access to this conversation") {
		t.Errorf("the description does not warn that the brief must be self-contained: %q", def.Function.Description)
	}
}

// Stop must reach a running delegate: the parent's cancel is the only brake on a
// loop with no human watching it.
func TestTaskStopsWhenTheTurnIsCancelled(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	dispatcher := skill.NewDispatcher(f.registry)

	// The delegate's context descends from the turn's, so cancelling the turn must
	// reach a delegate that is already running.
	turnCtx, cancel := context.WithCancel(turn.WithCallID(context.Background(), "call_1"))
	start, _, err := dispatcher.ExecuteTool(turnCtx, "task", map[string]any{
		"description": "x", "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore",
	})
	if err != nil || !start.Success {
		t.Fatalf("start failed: %v %q", err, start.Content)
	}
	cancel()

	// Collected on a live context, so what is asserted is the delegate stopping —
	// not the collector being cancelled out from under it.
	out, _, err := dispatcher.ExecuteTool(turn.WithCallID(context.Background(), "call_2"), "task_result",
		map[string]any{"task_id": taskIDOf(t, start)})
	t.Logf("after Stop: success=%v content=%q err=%v", out.Success, out.Content, err)
	if out.Success {
		t.Error("a cancelled delegate reported success")
	}
	if !strings.Contains(out.Content, "stopped") {
		t.Errorf("the model was not told the delegate was stopped: %q", out.Content)
	}
}

// The executor must not abandon a delegate at the 60s per-tool deadline — waiting
// is the work. Asserted on the exemption table rather than by sleeping a minute.
func TestTaskIsExemptFromTheToolDeadline(t *testing.T) {
	if !turn.HasNoDeadline("task") {
		t.Error("task is subject to the per-tool deadline — a real delegate would be killed mid-run")
	}
	if !turn.HasNoDeadline("ask_user") {
		t.Error("ask_user lost its exemption")
	}
	if turn.HasNoDeadline("grep") {
		t.Error("an ordinary tool must keep its deadline")
	}
}

// The receipt is what keeps a model from delegating everything: it judges the
// delegation after the fact, because how big a job is cannot be read off a brief.
func TestReceiptCallsOutAPointlessDelegation(t *testing.T) {
	small := receiptFor("explore", 1, 400*time.Millisecond)
	if !strings.Contains(small, "[task explore: 1 tool calls") {
		t.Errorf("receipt lost its numbers: %q", small)
	}
	if !strings.Contains(small, "Do work this size yourself") {
		t.Errorf("a one-call delegation was not called out: %q", small)
	}

	// Zero calls is the same lesson — the delegate answered from its brief alone.
	if !strings.Contains(receiptFor("general", 0, time.Second), "yourself") {
		t.Error("a no-call delegation was not called out")
	}

	// Real work gets a plain receipt: no lecture where none is due.
	big := receiptFor("explore", 7, 12*time.Second)
	if strings.Contains(big, "NOTE") {
		t.Errorf("a real delegation was lectured: %q", big)
	}
	if !strings.Contains(big, "7 tool calls, 12.0s") {
		t.Errorf("receipt lost its numbers: %q", big)
	}
}

// The description is the only pre-flight guard there is, so it has to state the
// rule and the reason, not hint at them.
func TestTaskDescriptionSaysWhenNotToDelegate(t *testing.T) {
	isolate(t)
	d := namedTool(t, NewTaskTools(TaskOptions{}), "task").Description()
	for _, want := range []string{"WHEN TO USE", "WHEN NOT TO", "yourself", "second system prompt"} {
		if !strings.Contains(d, want) {
			t.Errorf("the description is missing %q: %s", want, d)
		}
	}
}

// namedTool / toolDefOf keep the delegation pair's tests from caring which slot
// each tool came back in.
func namedTool(t *testing.T, tools []skill.Skill, name string) skill.Skill {
	t.Helper()
	for _, tool := range tools {
		if tool.Name() == name {
			return tool
		}
	}
	t.Fatalf("no tool named %q in the pair", name)
	return nil
}

func toolDefOf(t *testing.T, tools []skill.Skill, name string) model.ToolDefinition {
	t.Helper()
	definer, ok := namedTool(t, tools, name).(interface {
		ToolDefinition() model.ToolDefinition
	})
	if !ok {
		t.Fatalf("%q has no tool definition", name)
	}
	return definer.ToolDefinition()
}

// The point of not waiting: three delegates started before the first collect run
// at the same time, so the wall clock is the slowest one rather than the sum.
func TestDelegatesStartedTogetherRunConcurrently(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	started := time.Now()
	ids := make([]string, 0, 3)
	for i := range 3 {
		out := f.callTask(t, "call_"+strconv.Itoa(i), map[string]any{
			"description": "look " + strconv.Itoa(i), "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore",
		})
		if !out.Success {
			t.Fatalf("start %d failed: %s", i, out.Stderr)
		}
		ids = append(ids, taskIDOf(t, out))
	}
	startingTook := time.Since(started)
	t.Logf("starting three took %v (they are running now)", startingTook)

	// One collect, three results — the model pays one round trip for the batch.
	out := f.collect(t, strings.Join(ids, ","))
	t.Logf("batch result:\n%s", out.Content)
	if !out.Success {
		t.Fatalf("batch collect failed: %s", out.Content)
	}
	for _, id := range ids {
		if !strings.Contains(out.Content, id) {
			t.Errorf("%s is missing from the batch: %q", id, out.Content)
		}
	}
	if strings.Count(out.Content, "hay.txt") != 3 {
		t.Errorf("expected three real results, got: %q", out.Content)
	}
	// Each delegate's events carry its own parent id, so three timelines stay three.
	parents := map[string]bool{}
	for _, ev := range f.toolEvents() {
		if ev.Parent != "" {
			parents[ev.Parent] = true
		}
	}
	if len(parents) != 3 {
		t.Errorf("events carry %d parent ids, want 3: %v", len(parents), parents)
	}
}

// A model in a loop must not be able to melt the machine or the provider's rate
// limit, so there is a ceiling on delegates in flight.
func TestConcurrencyIsCapped(t *testing.T) {
	isolate(t)
	r := newRunner()
	block := make(chan struct{})
	defer close(block)

	for i := range maxConcurrent {
		if _, err := r.start(context.Background(), "explore", "held", func(context.Context, *runningTask) skill.Output {
			<-block
			return skill.Output{Success: true}
		}); err != nil {
			t.Fatalf("start %d refused early: %v", i, err)
		}
	}
	_, err := r.start(context.Background(), "explore", "one too many", func(context.Context, *runningTask) skill.Output {
		return skill.Output{}
	})
	if err == nil {
		t.Fatal("the cap did not hold")
	}
	t.Logf("refusal the model reads: %v", err)
	if !strings.Contains(err.Error(), "task_result") {
		t.Errorf("the refusal does not say how to make room: %v", err)
	}
}

// Collecting an id that was never handed out has to be a correctable mistake: say
// what is actually outstanding.
func TestCollectingAnUnknownIDNamesWhatIsRunning(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	start := f.callTask(t, "call_1", map[string]any{
		"description": "x", "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore",
	})
	realID := taskIDOf(t, start)

	out := f.collect(t, "task_999")
	t.Logf("unknown id: %q", out.Content)
	if out.Success {
		t.Error("collecting a made-up id succeeded")
	}
	// Either it names the outstanding one, or that one already finished — both are
	// honest; what matters is that the message is not a dead end.
	if !strings.Contains(out.Content, "task_999") {
		t.Errorf("the refusal does not name the id asked for: %q", out.Content)
	}

	// The real one still collects afterwards: a wrong guess costs nothing.
	if again := f.collect(t, realID); !again.Success {
		t.Errorf("the real delegate was lost after a bad collect: %q", again.Content)
	}
	// And collecting the same one twice returns the same answer rather than failing.
	if twice := f.collect(t, realID); !twice.Success {
		t.Errorf("collecting twice failed: %q", twice.Content)
	}
}

// Repetitive work is one delegate looping, not one delegate per item — the model
// has to be told, because fanning out is now possible and is much more expensive.
func TestTaskDescriptionPrefersOneLoopingDelegate(t *testing.T) {
	isolate(t)
	d := namedTool(t, NewTaskTools(TaskOptions{}), "task").Description()
	for _, want := range []string{"REPEATED WORK IS ONE JOB", "never twelve tasks", "own context"} {
		if !strings.Contains(d, want) {
			t.Errorf("the description does not steer repeated work into one delegate (%q): %s", want, d)
		}
	}
}

// general is the looper, so its brief has to say a list is one job and its cap has
// to be big enough for one.
func TestGeneralIsTheLooper(t *testing.T) {
	isolate(t)
	p, ok := Load("general")
	if !ok {
		t.Fatal("general profile missing")
	}
	if p.MaxToolCalls() <= defaultSteps {
		t.Errorf("general's cap is %d, no bigger than the default %d — a loop over a list needs room", p.MaxToolCalls(), defaultSteps)
	}
	for _, want := range []string{"A list is one job", "one after another", "where you stopped"} {
		if !strings.Contains(p.Prompt, want) {
			t.Errorf("general's brief is missing %q", want)
		}
	}
}

// Both ways a delegate's loop can end without it choosing to must reach the parent
// as something it can act on — not as cognitive's internal sentence, and not as
// Thai prose written for a human to read.
func TestALoopThatEndsItselfIsAnActionableFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider model.Provider
		want     []string
	}{
		{
			// Identical calls trip the doom-loop guard before the step cap.
			name: "doom loop", provider: &loopingProvider{},
			want: []string{"repeating the same tool call", "too vague"},
		},
		{
			// Varied calls get all the way to the step cap.
			name: "step cap exhausted", provider: &variedLoopProvider{},
			want: []string{"ran out of room", "smaller batches"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
			// A provider that never stops calling tools is the only way to reach either
			// ending; that is a provider *edge case*, which §45 still allows a stub for.
			for _, tool := range NewTaskTools(TaskOptions{
				Provider:     tc.provider,
				Model:        "loop-fake",
				Registry:     registry,
				ApprovalMode: safety.ApprovalFullAccess,
			}) {
				if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
					t.Fatalf("register %s: %v", tool.Name(), err)
				}
			}

			ctx := turn.WithCallID(context.Background(), "call_1")
			dispatcher := skill.NewDispatcher(registry)
			start, _, err := dispatcher.ExecuteTool(ctx, "task", map[string]any{
				"description": "endless", "prompt": "keep going forever", "agent": "explore",
			})
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			out, _, err := dispatcher.ExecuteTool(ctx, "task_result", map[string]any{"task_id": taskIDOf(t, start)})
			if err != nil {
				t.Fatalf("collect: %v", err)
			}
			t.Logf("%s: success=%v content=%q", tc.name, out.Success, out.Content)

			if out.Success {
				t.Error("a delegate that never finished reported success")
			}
			for _, want := range tc.want {
				if !strings.Contains(out.Content, want) {
					t.Errorf("the parent was not told %q: %q", want, out.Content)
				}
			}
			// Neither of cognitive's own endings may reach the parent as written.
			if strings.Contains(out.Content, cognitive.ToolLoopExhausted) ||
				strings.Contains(out.Content, cognitive.DoomLoopStopPrefix) {
				t.Errorf("an internal stop message leaked to the parent: %q", out.Content)
			}
		})
	}
}

// loopingProvider always asks for the same tool call, so the doom-loop guard fires.
type loopingProvider struct{}

func (*loopingProvider) Name() string              { return "loop-fake" }
func (*loopingProvider) SupportsToolCalling() bool { return true }
func (*loopingProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	return model.Response{ToolCalls: []model.ToolCall{{
		ID:       "loop_1",
		Type:     "function",
		Function: model.FunctionCall{Name: "list", Arguments: `{"path":"."}`},
	}}}, nil
}

// variedLoopProvider never repeats itself, so it slips past the doom-loop guard
// and runs until the profile's step cap stops it.
type variedLoopProvider struct{ n int }

func (*variedLoopProvider) Name() string              { return "loop-fake" }
func (*variedLoopProvider) SupportsToolCalling() bool { return true }
func (p *variedLoopProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	p.n++
	return model.Response{ToolCalls: []model.ToolCall{{
		ID:       "loop_" + strconv.Itoa(p.n),
		Type:     "function",
		Function: model.FunctionCall{Name: "grep", Arguments: `{"pattern":"x` + strconv.Itoa(p.n) + `","path":"."}`},
	}}}, nil
}
