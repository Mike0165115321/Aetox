package subagent

// The main agent calling `task` for real, on Aetox's own model for both sides
// (§45). This is the test the rehearsal in spawn_demo_test.go was standing in for.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/command"
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
	// delegations is the register the three tools share, held so a test can be
	// the host and press Stop — the one thing that ends a delegate early now
	// that a turn ending does not.
	delegations *Delegations

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

	f.delegations = NewDelegations()
	for _, tool := range NewTaskTools(TaskOptions{
		Provider:     provider,
		Model:        mainModel,
		Registry:     f.registry,
		Delegations:  f.delegations,
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
	return f.call(t, "call_collect", "task", map[string]any{"action": "collect", "task_id": ids})
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
	if !strings.Contains(start.Content, "action=collect") {
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

// A delegate that gets stuck asks the main agent instead of guessing, and keeps
// everything it had already done while it waits. The three things that make it
// worth having, each of which fails differently if broken:
//
//  1. collecting a stuck delegate returns the question and does NOT block —
//     otherwise parent and child wait on each other until the user hits Stop;
//  2. the answer reaches the delegate and it carries on in the same run — a
//     restarted delegate would have no memory of having asked;
//  3. collecting again after the answer yields the finished work.
func TestDelegateAsksTheMainAgentAndResumesWithItsWorkIntact(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	start := f.callTask(t, "call_parent_1", map[string]any{
		"description": "list the sandbox",
		"prompt":      "askmain: หาไฟล์ในโฟลเดอร์นี้แล้วรายงาน",
		"agent":       "explore",
	})
	if !start.Success {
		t.Fatalf("start failed: %s", start.Stderr)
	}
	id := taskIDOf(t, start)

	// 1. The question comes back instead of an answer, and comes back fast.
	deadline := time.Now().Add(20 * time.Second)
	asked := f.collect(t, id)
	if time.Now().After(deadline) {
		t.Fatal("collecting a delegate that is waiting on a question blocked — parent and child were deadlocked")
	}
	t.Logf("collected while stuck:\n%s", asked.Content)
	if !strings.Contains(asked.Content, "list the sandbox root, or stop here?") {
		t.Fatalf("the delegate's question did not reach the parent: %q", asked.Content)
	}
	if !strings.Contains(asked.Content, "action=answer") {
		t.Errorf("the parent was not told how to unstick it: %q", asked.Content)
	}
	if strings.Contains(asked.Content, "[task explore:") {
		t.Errorf("a question was dressed up as a finished result: %q", asked.Content)
	}

	// Asking again before answering must repeat the question, not hang.
	if again := f.collect(t, id); !strings.Contains(again.Content, "or stop here?") {
		t.Errorf("collecting twice without answering lost the question: %q", again.Content)
	}

	// 2 + 3. Answer, then collect the finished work.
	ans := f.call(t, "call_parent_2", "task", map[string]any{"action": "answer",
		"task_id": id, "answer": "go ahead and list it",
	})
	if !ans.Success {
		t.Fatalf("task_answer failed: %s", ans.Stderr)
	}
	done := f.collect(t, id)
	t.Logf("collected after answering:\n%s", done.Content)
	if !done.Success {
		t.Fatalf("collect after answering failed: %s", done.Stderr)
	}
	if !strings.Contains(done.Content, "hay.txt") {
		t.Errorf("the delegate did not carry on with the work after being answered: %q", done.Content)
	}
	if !strings.Contains(done.Content, "go ahead and list it") {
		t.Errorf("the delegate lost the answer — it was restarted rather than resumed: %q", done.Content)
	}
	if !strings.Contains(done.Content, "[task explore:") {
		t.Errorf("no receipt on the finished result: %q", done.Content)
	}
}

// `aetox-subagent:test` is the delegation bench a developer picks in the app to
// exercise this by hand with no API key. It is only worth having if it drives
// the whole path *by itself*, so this runs it through the real executor — no
// tool calls staged by the test — and asserts the four rounds happened in order
// on both sides. If the bench stops working, the manual check silently stops
// checking anything, which is the failure mode worth a test.
func TestSubagentBenchModelDrivesTheWholeDelegationItself(t *testing.T) {
	f := newTaskFixture(t, "aetox-subagent:test")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	line := "ทดสอบซับเอเจนหน่อย"
	result, err := f.exec.Execute(ctx, line, command.Intent{Raw: line, Kind: command.KindConversation}, nil, nil, nil)
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	var mainCalls []string
	var subCalls []string
	for _, ev := range f.toolEvents() {
		if ev.Action != "call" {
			continue
		}
		if ev.Parent == "" {
			mainCalls = append(mainCalls, ev.Name)
		} else {
			subCalls = append(subCalls, ev.Name)
		}
	}
	t.Logf("main: %v", mainCalls)
	t.Logf("sub : %v", subCalls)
	t.Logf("reply:\n%s", result.Reply)

	// Four rounds of delegation, and packed (§99) all four are spelled `task` in
	// the timeline — start, collect, answer, collect. The actions they carry are
	// what the noop script's own ids assert; here the shape is the point.
	want := []string{"task", "task", "task", "task"}
	if len(mainCalls) != len(want) {
		t.Fatalf("main agent ran %v, want %v", mainCalls, want)
	}
	for i, name := range want {
		if mainCalls[i] != name {
			t.Fatalf("main agent ran %v, want %v", mainCalls, want)
		}
	}
	// The delegate's own run is the part the bench exists to show: one tool call
	// under a sub-agent block demonstrates nothing about how a delegation reads.
	// It asks, then does a real job — and ends with a file on disk.
	for _, want := range []string{"ask_main", "list", "write", "read", "grep"} {
		if !contains(subCalls, want) {
			t.Errorf("the delegate never ran %s; it ran %v", want, subCalls)
		}
	}
	if subCalls[0] != "ask_main" {
		t.Errorf("the delegate worked before asking, so the bench never shows a parked run: %v", subCalls)
	}
	if _, err := os.Stat(filepath.Join(f.root, "subagent-demo.md")); err != nil {
		t.Errorf("the delegate's file is not on disk: %v", err)
	}
	// The delegate's finding has to survive all four rounds back to the reply.
	if !strings.Contains(result.Reply, "hay.txt") {
		t.Errorf("the collected work never reached the final answer:\n%s", result.Reply)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// Answering something that is not waiting is a model that has lost track of
// which delegate it was talking to. Saying so beats accepting it and stranding
// the delegate that really is stuck.
func TestTaskAnswerRefusesWhatIsNotWaiting(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	missing := f.call(t, "call_parent_1", "task", map[string]any{"action": "answer",
		"task_id": "task_99", "answer": "yes",
	})
	if missing.Success {
		t.Error("answering an unknown id succeeded")
	}

	start := f.callTask(t, "call_parent_2", map[string]any{
		"description": "list", "prompt": "หาไฟล์ในโฟลเดอร์นี้", "agent": "explore",
	})
	id := taskIDOf(t, start)
	f.collect(t, id) // runs to completion without asking anything

	finished := f.call(t, "call_parent_3", "task", map[string]any{"action": "answer",
		"task_id": id, "answer": "yes",
	})
	if finished.Success {
		t.Error("answering a finished delegate succeeded")
	}
	if !strings.Contains(finished.Content, "already finished") {
		t.Errorf("unhelpful reason: %q", finished.Content)
	}
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
	out, _, err := dispatcher.ExecuteTool(ctx, "task", map[string]any{"action": "collect",
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
	child := FilterRegistry(f.registry, profile, nil)
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
	generalRegistry := FilterRegistry(f.registry, general, nil)
	if _, ok := generalRegistry.Get("write"); !ok {
		t.Error("general lost write, which it inherits")
	}
	if _, ok := generalRegistry.Get("task"); ok {
		t.Error("general could spawn its own children")
	}
	// …and minus the two its profile denies, because nobody is watching this loop
	// (§44.14). Both halves have to hold: gone from the registry so the model is
	// never offered them, and denied at the permission layer so a discovered skill
	// registering under the same name later cannot walk back in.
	for _, name := range []string{"plugin_install", "delete"} {
		if _, ok := generalRegistry.Get(name); ok {
			t.Errorf("%q was handed to an unattended delegate", name)
		}
	}
	denied := map[string]bool{}
	for _, rule := range general.DenyRules() {
		if rule.Action == safety.PermissionDeny {
			denied[rule.Tool] = true
		}
	}
	if !denied["plugin_install"] || !denied["delete"] {
		t.Errorf("general's denials never reached the permission layer: %v", denied)
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

// The background promise, stated as a test: the turn that started a delegate
// ends, and the delegate's work is still there to collect afterwards.
//
// This is the assertion that used to run the other way round. A turn ending
// took every uncollected delegate with it, so a model that ran out of turn
// before it thought to collect threw away the whole run — see the rule on
// Delegations.start.
func TestADelegateOutlivesTheTurnThatStartedIt(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	dispatcher := skill.NewDispatcher(f.registry)

	turnCtx, endTurn := context.WithCancel(turn.WithCallID(context.Background(), "call_1"))
	start, _, err := dispatcher.ExecuteTool(turnCtx, "task", map[string]any{
		"description": "x", "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore",
	})
	if err != nil || !start.Success {
		t.Fatalf("start failed: %v %q", err, start.Content)
	}
	endTurn() // the reply went out; this turn is over

	// A later turn, collecting by the same id.
	out, _, err := dispatcher.ExecuteTool(turn.WithCallID(context.Background(), "call_2"), "task",
		map[string]any{"action": "collect", "task_id": taskIDOf(t, start)})
	t.Logf("collected in a later turn: success=%v content=%q err=%v", out.Success, out.Content, err)
	if err != nil {
		t.Fatalf("collect failed: %v", err)
	}
	if !out.Success {
		t.Errorf("the turn ending killed the delegate: %q", out.Content)
	}
	if strings.Contains(out.Content, "stopped") {
		t.Errorf("the delegate reported itself stopped by a turn that merely ended: %q", out.Content)
	}
}

// Stop must reach a running delegate. It is the only brake left on a loop with
// no human watching it, and the user pressing it has said something about the
// work rather than about the turn.
//
// Asserted on a delegate parked on a question, because that is the one state a
// test can hold it in without racing its own completion — and it is also the
// worst case: a parked goroutine is waiting on a channel nobody will ever write
// to, so if Stop cannot free it the app leaks one per abandoned question.
func TestStopEndsARunningDelegate(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	dispatcher := skill.NewDispatcher(f.registry)

	before := runtime.NumGoroutine()
	start, _, err := dispatcher.ExecuteTool(turn.WithCallID(context.Background(), "call_1"), "task",
		map[string]any{"description": "x", "prompt": "askmain: หาไฟล์ในโฟลเดอร์นี้", "agent": "explore"})
	if err != nil || !start.Success {
		t.Fatalf("start failed: %v %q", err, start.Content)
	}
	id := taskIDOf(t, start)

	// Let it get as far as the question, then Stop without answering.
	asked := f.collect(t, id)
	if !strings.Contains(asked.Content, "waiting for a decision") {
		t.Fatalf("the delegate never parked: %q", asked.Content)
	}
	if stopped := f.delegations.StopAll(); stopped != 1 {
		t.Errorf("StopAll reported %d delegates stopped, want 1", stopped)
	}

	out, _, err := dispatcher.ExecuteTool(turn.WithCallID(context.Background(), "call_2"), "task",
		map[string]any{"action": "collect", "task_id": id})
	t.Logf("after Stop: success=%v err=%v content=%q", out.Success, err, out.Content)
	if out.Success {
		t.Error("a delegate stopped mid-question reported success")
	}
	if !strings.Contains(out.Content, "stopped") {
		t.Errorf("the model was not told the delegate was stopped: %q", out.Content)
	}

	// The parked goroutine has to be gone, not merely unreachable.
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if leaked := runtime.NumGoroutine() - before; leaked > 2 {
		t.Errorf("%d goroutines still running after Stop — a parked delegate was never freed", leaked)
	}
}

// Three delegates parked on questions at the same time, answered in the reverse
// order they asked. Each has to resume with *its own* answer.
//
// One shared runner holds every delegation, and the naive version of parking is
// one question slot: two parked delegates would then either overwrite each
// other's question or hand one delegate the other's answer, and both failures
// look like "the sub-agent did the wrong thing" rather than like a bug here.
func TestSeveralDelegatesParkAtOnceAndEachGetsItsOwnAnswer(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		start := f.callTask(t, "call_parent_"+strconv.Itoa(i), map[string]any{
			"description": "job " + strconv.Itoa(i),
			"prompt":      "askmain: หาไฟล์ในโฟลเดอร์นี้ (งานที่ " + strconv.Itoa(i) + ")",
			"agent":       "explore",
		})
		if !start.Success {
			t.Fatalf("start %d failed: %s", i, start.Stderr)
		}
		ids = append(ids, taskIDOf(t, start))
	}

	// All three ask before any is answered, and collecting each returns its own
	// question rather than blocking behind the others.
	for i, id := range ids {
		out := f.collect(t, id)
		if !strings.Contains(out.Content, "waiting for a decision") {
			t.Fatalf("delegate %d (%s) did not come back asking: %q", i, id, out.Content)
		}
		if !strings.Contains(out.Content, id) {
			t.Errorf("the question for %s names a different task: %q", id, out.Content)
		}
	}

	// Answered backwards, each answer distinct — a delegate that resumed on
	// somebody else's answer shows up in its own final text.
	for i := len(ids) - 1; i >= 0; i-- {
		ans := f.call(t, "call_answer_"+strconv.Itoa(i), "task", map[string]any{"action": "answer",
			"task_id": ids[i], "answer": "answer-for-" + strconv.Itoa(i),
		})
		if !ans.Success {
			t.Fatalf("answering %s failed: %s", ids[i], ans.Stderr)
		}
	}

	for i, id := range ids {
		done := f.collect(t, id)
		if !done.Success {
			t.Fatalf("collect %s failed: %s", id, done.Stderr)
		}
		want := "answer-for-" + strconv.Itoa(i)
		if !strings.Contains(done.Content, want) {
			t.Errorf("delegate %d resumed on the wrong answer — wanted %q in:\n%s", i, want, done.Content)
		}
		for j := range ids {
			if j == i {
				continue
			}
			if strings.Contains(done.Content, "answer-for-"+strconv.Itoa(j)) {
				t.Errorf("delegate %d also saw delegate %d's answer:\n%s", i, j, done.Content)
			}
		}
	}
}

// A question outlives the turn it was asked in. The delegate is parked inside
// its own tool call holding everything it had already worked out, and the reply
// going out without an answer is not a reason to throw that away — the next turn
// can still answer it and collect the finished work.
func TestAParkedQuestionSurvivesTheTurn(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	dispatcher := skill.NewDispatcher(f.registry)

	turnCtx, endTurn := context.WithCancel(turn.WithCallID(context.Background(), "call_1"))
	start, _, err := dispatcher.ExecuteTool(turnCtx, "task", map[string]any{
		"description": "x", "prompt": "askmain: หาไฟล์ในโฟลเดอร์นี้", "agent": "explore",
	})
	if err != nil || !start.Success {
		t.Fatalf("start failed: %v %q", err, start.Content)
	}
	id := taskIDOf(t, start)
	if asked := f.collect(t, id); !strings.Contains(asked.Content, "waiting for a decision") {
		t.Fatalf("the delegate never parked: %q", asked.Content)
	}
	endTurn()

	// A later turn answers it, and the delegate carries on from where it stopped.
	answered, _, err := dispatcher.ExecuteTool(turn.WithCallID(context.Background(), "call_2"), "task",
		map[string]any{"action": "answer", "task_id": id, "answer": "yes — list the sandbox root"})
	if err != nil || !answered.Success {
		t.Fatalf("answering across the turn boundary failed: %v %q", err, answered.Content)
	}
	out := f.collect(t, id)
	t.Logf("collected after answering in a later turn: success=%v content=%q", out.Success, out.Content)
	if !out.Success {
		t.Errorf("the delegate did not resume: %q", out.Content)
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

// The tray reads the register, and the register has to tell running from done
// from stuck — the tool-event stream cannot (a `task` call completes the moment
// the work starts), which is the whole reason Snapshot exists (§105).
func TestSnapshotTellsRunningFromDoneFromStuck(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	dispatcher := skill.NewDispatcher(f.registry)

	// One delegate parked on a question, never collected: the tray must call it
	// waiting anyway — "stuck, needs you" cannot depend on somebody having
	// already come asking.
	stuck, _, err := dispatcher.ExecuteTool(turn.WithCallID(context.Background(), "call_1"), "task",
		map[string]any{"description": "stuck one", "prompt": "askmain: หาไฟล์ในโฟลเดอร์นี้", "agent": "explore"})
	if err != nil || !stuck.Success {
		t.Fatalf("start failed: %v %q", err, stuck.Content)
	}
	stuckID := taskIDOf(t, stuck)
	waitFor(t, func() bool {
		for _, info := range f.delegations.Snapshot() {
			if info.ID == stuckID && info.Waiting {
				return true
			}
		}
		return false
	}, "the parked delegate never showed as waiting")

	// And one run to completion.
	done, _, err := dispatcher.ExecuteTool(turn.WithCallID(context.Background(), "call_2"), "task",
		map[string]any{"description": "quick one", "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore"})
	if err != nil || !done.Success {
		t.Fatalf("start failed: %v %q", err, done.Content)
	}
	doneID := taskIDOf(t, done)
	f.collect(t, doneID)

	byID := map[string]TaskInfo{}
	for _, info := range f.delegations.Snapshot() {
		byID[info.ID] = info
	}
	if got := byID[stuckID]; !got.Running || !got.Waiting || got.Question == "" {
		t.Errorf("stuck delegate reads %+v — want running, waiting, with its question", got)
	}
	if got := byID[doneID]; got.Running || got.Waiting || !got.OK {
		t.Errorf("finished delegate reads %+v — want done and ok", got)
	}

	// Freed for the goroutine check other tests do; here just unstick it.
	f.delegations.StopAll()
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A model in a loop must not be able to melt the machine or the provider's rate
// limit, so there is a ceiling on delegates in flight.
func TestConcurrencyIsCapped(t *testing.T) {
	isolate(t)
	r := NewDelegations()
	block := make(chan struct{})
	defer close(block)

	for i := range maxConcurrent {
		if _, err := r.start(delegation{profile: "explore", label: "held"}, func(context.Context, *runningTask) skill.Output {
			<-block
			return skill.Output{Success: true}
		}); err != nil {
			t.Fatalf("start %d refused early: %v", i, err)
		}
	}
	_, err := r.start(delegation{profile: "explore", label: "one too many"}, func(context.Context, *runningTask) skill.Output {
		return skill.Output{}
	})
	if err == nil {
		t.Fatal("the cap did not hold")
	}
	t.Logf("refusal the model reads: %v", err)
	if !strings.Contains(err.Error(), "action=collect") {
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

// general is the looper, so its brief has to say a list is one job and nothing
// may stop it halfway down one.
func TestGeneralIsTheLooper(t *testing.T) {
	isolate(t)
	p, ok := Load("general")
	if !ok {
		t.Fatal("general profile missing")
	}
	// This used to be "its cap is bigger than the default" (steps: 48 against 24).
	// Since §110 nothing carries one, which is the same guarantee without a
	// number the next list can outgrow.
	if p.MaxToolCalls() > 0 {
		t.Errorf("general's cap is %d — a loop over a list must not have one", p.MaxToolCalls())
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
		agent    string
		steps    int
		want     []string
	}{
		{
			// Identical calls trip the doom-loop guard, which needs no ceiling —
			// it fires on repetition, whatever the cap is.
			name: "doom loop", provider: &loopingProvider{}, agent: "explore",
			want: []string{"repeating the same tool call", "too vague"},
		},
		{
			// Varied calls slip past that guard, so the only thing left to stop
			// them is a ceiling — and since §110 nothing carries one unless its
			// profile asks. So this runs a profile that asks.
			//
			// It used to run `explore`, back when every delegate inherited a
			// default of 24. §110 removed the default and updated the test one
			// function up (TestGeneralIsTheLooper) but not this one, so the loop
			// this case exists to bound became genuinely unbounded: the subtest
			// stopped failing and started *hanging*, taking `go test ./...` with
			// it. Found 2026-08-16, 25 minutes into a full run.
			name: "step cap exhausted", provider: &variedLoopProvider{},
			agent: "capped", steps: 3,
			want: []string{"ran out of room", "smaller batches"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			if tc.steps > 0 {
				// In the agents' home, not the helpers': the helpers' home is
				// closed — a file put there is reported and not read — and a
				// user file named after a bundled helper does not shadow it
				// either. An agent is the one profile a person can still write,
				// which makes it the only place a `steps:` ceiling can come
				// from now, and therefore the honest place to test one.
				writeProfile(t, AgentsDir, tc.agent, fmt.Sprintf(
					"---\ndescription: capped runner\ntools: read\nsteps: %d\n---\nRun until stopped.\n", tc.steps))
			}
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

			// A deadline, because the failure this case is guarding against does
			// not look like a failure. A delegate with nothing to stop it does
			// not return a bad answer — it returns nothing, forever, and a test
			// that waits forever reads as "still running" until the whole suite
			// times out. collect honours this ctx, so a regression fails here in
			// seconds with a sentence saying what happened.
			ctx, cancel := context.WithTimeout(turn.WithCallID(context.Background(), "call_1"), 30*time.Second)
			defer cancel()
			dispatcher := skill.NewDispatcher(registry)
			start, _, err := dispatcher.ExecuteTool(ctx, "task", map[string]any{
				"description": "endless", "prompt": "keep going forever", "agent": tc.agent,
			})
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			out, _, err := dispatcher.ExecuteTool(ctx, "task", map[string]any{"action": "collect", "task_id": taskIDOf(t, start)})
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("the delegate never stopped: nothing bounded its loop, so %q ran until the deadline "+
						"instead of ending with something the parent could act on", tc.name)
				}
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
//
// `read`, and a different file each round, for two reasons that are both about
// staying reachable: read is carried by the desk an agent runs under (grep is
// not, and a profile asking only for grep is cut to nothing before the loop
// starts), and a missing file is a tool *error*, not a stop — which is the point,
// since what is being tested is the ending the loop reaches on its own.
type variedLoopProvider struct{ n int }

func (*variedLoopProvider) Name() string              { return "loop-fake" }
func (*variedLoopProvider) SupportsToolCalling() bool { return true }
func (p *variedLoopProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	p.n++
	return model.Response{ToolCalls: []model.ToolCall{{
		ID:       "loop_" + strconv.Itoa(p.n),
		Type:     "function",
		Function: model.FunctionCall{Name: "read", Arguments: `{"path":"f` + strconv.Itoa(p.n) + `.txt"}`},
	}}}, nil
}

// A delegate calling ONE tool proves the wiring; it does not prove the child's
// tool loop. This walks a chain — list → glob → grep — and checks the three
// things that are only true if the loop really ran:
//
//  1. every call happened, inside the delegate, stamped as the delegate's;
//  2. the parent still gets only the answer + receipt, never the tool log (§44.6)
//     — a three-call delegate is exactly where leaking it would cost the most;
//  3. the receipt counts all of them, so the "you could have done that here"
//     nudge stays off work that genuinely needed delegating.
func TestDelegateRunsAChainOfToolsAndOnlyTheAnswerComesBack(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	start := f.callTask(t, "call_chain", map[string]any{
		"description": "walk the sandbox",
		"prompt":      "toolchain: สำรวจโฟลเดอร์นี้ด้วยเครื่องมือทุกตัวที่มี แล้วรายงานผลของแต่ละตัว",
		"agent":       "explore",
	})
	if !start.Success {
		t.Fatalf("start failed: %s", start.Stderr)
	}
	out := f.collect(t, taskIDOf(t, start))
	t.Logf("collected:\n%s", out.Content)
	if !out.Success {
		t.Fatalf("collect failed: %s", out.Stderr)
	}

	// 1. Every probe ran inside the delegate, not in the parent's own timeline.
	inDelegate := map[string]bool{}
	for _, ev := range f.toolEvents() {
		if ev.Action == "call" && ev.Parent == "call_chain" {
			inDelegate[ev.Name] = true
		}
	}
	for _, want := range []string{"list", "glob", "grep"} {
		if !inDelegate[want] {
			t.Errorf("%q never ran inside the delegate — its loop stopped early: %v", want, inDelegate)
		}
	}

	// 2. The parent sees the answer, not the transcript. The delegate's raw tool
	//    envelopes carry "duration_ms"; one appearing here means the log leaked.
	if strings.Contains(out.Content, "duration_ms") {
		t.Errorf("the delegate's tool log reached the parent: %q", out.Content)
	}
	for _, want := range []string{"list", "glob", "grep"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("the delegate's report is missing %q: %q", want, out.Content)
		}
	}

	// 3. The receipt counts the whole chain, and does not scold a real delegation.
	if !strings.Contains(out.Content, "[task explore:") {
		t.Fatalf("no receipt line: %q", out.Content)
	}
	if strings.Contains(out.Content, "small enough to have done here") {
		t.Errorf("a three-call delegate was called pointless: %q", out.Content)
	}
	calls := 0
	if _, err := fmt.Sscanf(out.Content[strings.Index(out.Content, "[task explore:"):], "[task explore: %d tool calls", &calls); err != nil {
		t.Fatalf("cannot read the call count out of the receipt: %v", err)
	}
	if calls < len(inDelegate) {
		t.Errorf("receipt counted %d calls, but %d ran", calls, len(inDelegate))
	}
}

// A brief bigger than the child's context used to be accepted and then silently
// cut from the tail by memory.Context — the delegate would work from half a brief
// and report on it with no idea anything was missing. It is refused with the
// numbers instead, which the parent can act on by splitting the job.
func TestABriefTooBigForTheChildIsRefusedNotTruncated(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root})
	const budget = 4000
	for _, tool := range NewTaskTools(TaskOptions{
		Provider:     model.NewNoopProvider("aetox-tools:test"),
		Model:        "aetox-tools:test",
		Registry:     registry,
		ApprovalMode: safety.ApprovalFullAccess,
		MaxChars:     budget,
	}) {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	run := func(brief string) skill.Output {
		t.Helper()
		out, handled, err := skill.NewDispatcher(registry).ExecuteTool(
			turn.WithCallID(context.Background(), "call_1"), "task",
			map[string]any{"description": "big", "prompt": brief, "agent": "explore"})
		if !handled || err != nil {
			t.Fatalf("dispatch: handled=%v err=%v", handled, err)
		}
		return out
	}

	over := run(strings.Repeat("x", budget+1))
	if over.Success {
		t.Fatal("an oversize brief was accepted — the delegate would have run on a truncated one")
	}
	// The refusal has to carry the numbers, or the model cannot tell how much to cut.
	for _, want := range []string{"too long", strconv.Itoa(budget), "Split"} {
		if !strings.Contains(over.Content, want) {
			t.Errorf("the refusal is missing %q: %q", want, over.Content)
		}
	}

	// And it must not be trigger-happy: a brief that fits still runs.
	if fits := run("รายงานไฟล์ในโฟลเดอร์นี้"); !fits.Success {
		t.Errorf("a normal brief was refused: %q", fits.Content)
	}
}

// One door: a delegate is described the way the session it came from is.
//
// The two ways onto the same worker had drifted apart — a direct chat mounted
// the whole prompt, a delegated run mounted the brief and nothing else, so an
// agent asked by the assistant did not know where a bare filename lands and an
// agent asked by the user did. The host now lends the assembler it used on
// itself; these assert that the tool actually leans on it.
func TestADelegateIsMountedOnTheHostsAssembledPrompt(t *testing.T) {
	isolate(t)
	var got string
	tools := NewTaskTools(TaskOptions{
		Provider:     model.NewNoopProvider("m"),
		Model:        "m",
		Registry:     skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()}),
		ApprovalMode: safety.ApprovalFullAccess,
		BuildPrompt: func(direction string) string {
			got = direction
			return "FRAME\n" + direction
		},
	})
	registry := skill.NewRegistry()
	for _, tool := range tools {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	out, _, err := skill.NewDispatcher(registry).ExecuteTool(context.Background(), "task",
		map[string]any{"agent": "general", "description": "x", "prompt": "find the needle"})
	if err != nil {
		t.Fatalf("task: %v (%s)", err, out.Content)
	}

	// What the host was handed is the worker's own brief, whole — the assembler
	// puts the frame around it, so anything that pre-chewed it here would be a
	// second opinion about what a brief is.
	p, ok := Load("general")
	if !ok {
		t.Fatal("the general profile is missing")
	}
	if want := PromptFor(p); got != want {
		t.Errorf("the host was handed %d chars, want the profile's own %d", len(got), len(want))
	}
}

// The budget has to be measured against what the delegate actually carries. It
// used to be checked against the bare brief, which was the wrong number the
// moment the frame arrived — a brief could pass the check and the assembled
// prompt still not fit.
func TestTheBriefBudgetCountsTheAssembledPrompt(t *testing.T) {
	isolate(t)
	tools := NewTaskTools(TaskOptions{
		Provider:     model.NewNoopProvider("m"),
		Model:        "m",
		Registry:     skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()}),
		ApprovalMode: safety.ApprovalFullAccess,
		MaxChars:     4000,
		BuildPrompt: func(direction string) string {
			return strings.Repeat("frame ", 1000) + direction // 6k of frame alone
		},
	})
	registry := skill.NewRegistry()
	for _, tool := range tools {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	out, _, err := skill.NewDispatcher(registry).ExecuteTool(context.Background(), "task",
		map[string]any{"agent": "general", "description": "x", "prompt": "short"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// A refusal is a tool result, not a Go error: the model has to read it and
	// split the work, which it cannot do if the call itself blew up.
	if !strings.Contains(out.Content, "too long") {
		t.Fatalf("a job that cannot fit was accepted: %s", out.Content)
	}
	// And the number it names is the assembled prompt, not the bare brief —
	// the frame alone is already past the budget.
	if !strings.Contains(out.Content, "7611-character") {
		t.Errorf("the refusal counted something other than the assembled prompt: %s", out.Content)
	}
}
