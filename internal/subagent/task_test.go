package subagent

// The main agent calling `task` for real, on Aetox's own model for both sides
// (§45). This is the test the rehearsal in spawn_demo_test.go was standing in for.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	events   []turn.ToolEvent
	usage    []model.Usage
	exec     *turn.Executor
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
	onToolAction := func(ev turn.ToolEvent) { f.events = append(f.events, ev) }

	if err := f.registry.Register(NewTaskTool(TaskOptions{
		Provider:     provider,
		Model:        mainModel,
		Registry:     f.registry,
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: onToolAction,
		OnUsage:      func(u model.Usage) { f.usage = append(f.usage, u) },
	}), skill.SourceBuiltin); err != nil {
		t.Fatalf("register task: %v", err)
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

// callTask invokes the tool the way the executor does — through the dispatcher,
// with a call id on the context, so ToolEvent.Parent gets stamped for real.
func (f *taskFixture) callTask(t *testing.T, callID string, args map[string]any) skill.Output {
	t.Helper()
	ctx := turn.WithCallID(context.Background(), callID)
	out, handled, err := skill.NewDispatcher(f.registry).ExecuteTool(ctx, "task", args)
	if !handled {
		t.Fatal("task is not registered")
	}
	if err != nil {
		t.Fatalf("task returned an error instead of a failed result: %v", err)
	}
	return out
}

// The whole point, end to end: a delegate runs, does real work, and the parent
// gets back its text plus a receipt — not its transcript.
func TestTaskRunsADelegateAndReturnsOnlyItsResult(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	out := f.callTask(t, "call_parent_1", map[string]any{
		"description": "list the sandbox",
		"prompt":      "หาไฟล์ทั้งหมดในโฟลเดอร์นี้แล้วรายงานชื่อไฟล์",
		"agent":       "explore",
	})
	t.Logf("task output:\n%s", out.Content)

	if !out.Success {
		t.Fatalf("task failed: %s", out.Stderr)
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
	for _, ev := range f.events {
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
	if err := registry.Register(NewTaskTool(TaskOptions{
		Provider:     usageProvider{},
		Model:        "usage-fake-model",
		Registry:     registry,
		ApprovalMode: safety.ApprovalFullAccess,
		OnUsage:      func(u model.Usage) { usage = append(usage, u) },
	}), skill.SourceBuiltin); err != nil {
		t.Fatalf("register task: %v", err)
	}

	ctx := turn.WithCallID(context.Background(), "call_1")
	out, _, err := skill.NewDispatcher(registry).ExecuteTool(ctx, "task", map[string]any{
		"description": "x", "prompt": "report the files", "agent": "explore",
	})
	if err != nil || !out.Success {
		t.Fatalf("task failed: %v %q", err, out.Content)
	}
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
	out := f.callTask(t, "call_1", map[string]any{"description": "look around", "prompt": "รายงานไฟล์ในโฟลเดอร์"})
	if !out.Success {
		t.Fatalf("default spawn failed: %s", out.Stderr)
	}
	if !strings.Contains(out.Content, "[task explore:") {
		t.Errorf("default profile is not explore: %q", out.Content)
	}
}

// The tool the model sees has to describe what it can pick, or it will guess.
func TestTaskDefinitionListsTheProfiles(t *testing.T) {
	isolate(t)
	tool := NewTaskTool(TaskOptions{}).(interface {
		ToolDefinition() model.ToolDefinition
	})
	def := tool.ToolDefinition()
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
	ctx, cancel := context.WithCancel(turn.WithCallID(context.Background(), "call_1"))
	cancel()

	out, handled, err := skill.NewDispatcher(f.registry).ExecuteTool(ctx, "task", map[string]any{
		"description": "x", "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore",
	})
	if !handled {
		t.Fatal("task is not registered")
	}
	t.Logf("cancelled run: success=%v content=%q err=%v", out.Success, out.Content, err)
	if out.Success {
		t.Error("a cancelled delegate reported success")
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
	tool := NewTaskTool(TaskOptions{}).(skill.Skill)
	d := tool.Description()
	for _, want := range []string{"WHEN TO USE", "WHEN NOT TO", "yourself", "second system prompt"} {
		if !strings.Contains(d, want) {
			t.Errorf("the description is missing %q: %s", want, d)
		}
	}
}
