package subagent

// What a declared run has to be true of, all of it through the real tools: a
// phase that was promised and left empty stays visible, an invented phase is
// refused, and every delegate's spend is stamped with who spent it.

import (
	"context"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// The whole argument for runs in one test: the checking round is declared before
// the findings exist, so a run that stops after finding shows the gap instead of
// looking finished.
func TestADeclaredPhaseNobodyFillsStaysVisible(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")

	plan := f.call(t, "call_1", "task", map[string]any{"action": "plan",
		"name":  "ตรวจ SKILL.md ให้ตรงกับโค้ด",
		"brief": "กางข้อกล่าวอ้างออกทีละบรรทัด แล้วให้อีกชุดหักล้างก่อนรายงาน",
		"phases": []any{
			map[string]any{"title": "รอบตรวจ", "agents": 2},
			map[string]any{"title": "รอบหักล้าง", "agents": 2},
		},
	})
	if !plan.Success {
		t.Fatalf("task_plan refused a good declaration: %s", plan.Content)
	}

	start := f.callTask(t, "call_2", map[string]any{
		"description": "ตรวจข้อแรก", "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore",
		"phase": "รอบตรวจ",
	})
	f.call(t, "call_3", "task", map[string]any{"action": "collect", "task_id": taskIDOf(t, start)})

	runs := f.delegations.Runs()
	if len(runs) != 1 {
		t.Fatalf("Runs() = %d runs, want the one that was declared", len(runs))
	}
	run := runs[0]
	if run.Name == "" || run.Brief == "" {
		t.Errorf("the run lost what the user reads: %+v", run)
	}
	if len(run.Phases) != 2 {
		t.Fatalf("phases = %+v, want both declared stages", run.Phases)
	}
	if got := run.Phases[0]; got.Done != 1 || got.Planned != 2 {
		t.Errorf("the worked phase = %+v, want 1 of the 2 it planned", got)
	}
	// The point. Nobody did the second round, and the run says so rather than
	// dropping a stage it never saw work in.
	if got := run.Phases[1]; got.Title != "รอบหักล้าง" || got.Done != 0 || got.Planned != 2 {
		t.Errorf("the promised phase = %+v, want 0 of 2 still showing", got)
	}
	if run.Running {
		t.Errorf("the run still reads as running with nothing in flight: %+v", run)
	}
}

// A phase the declaration did not name is refused, and the refusal names what
// was declared — otherwise the model spends a round guessing at the vocabulary.
func TestAnUndeclaredPhaseIsRefusedByName(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	f.call(t, "call_1", "task", map[string]any{"action": "plan",
		"name":   "ตรวจเอกสาร",
		"phases": []any{"รอบตรวจ", "รอบหักล้าง"},
	})

	out := f.callTask(t, "call_2", map[string]any{
		"description": "x", "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore",
		"phase": "สรุป",
	})
	if out.Success {
		t.Fatal("a phase nobody declared was accepted, so the declaration promises nothing")
	}
	for _, want := range []string{"สรุป", "รอบตรวจ", "รอบหักล้าง"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("the refusal does not name %q: %s", want, out.Content)
		}
	}
}

// A delegate started without a phase is what every delegation was before runs
// existed, and what the user's own @doc still is. It must keep working, and it
// must not be counted into a run it never joined.
func TestALooseDelegateStillRunsBesideADeclaredRun(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	f.call(t, "call_1", "task", map[string]any{"action": "plan",
		"name": "ตรวจเอกสาร", "phases": []any{"รอบตรวจ"},
	})

	start := f.callTask(t, "call_2", map[string]any{
		"description": "งานเดี่ยว", "prompt": "รายงานไฟล์ในโฟลเดอร์", "agent": "explore",
	})
	if !start.Success {
		t.Fatalf("a delegate with no phase was refused: %s", start.Content)
	}
	out := f.call(t, "call_3", "task", map[string]any{"action": "collect", "task_id": taskIDOf(t, start)})
	if !out.Success {
		t.Fatalf("the loose delegate failed: %s", out.Content)
	}

	runs := f.delegations.Runs()
	if len(runs) != 1 || len(runs[0].Phases) != 1 {
		t.Fatalf("runs = %+v", runs)
	}
	if got := runs[0].Phases[0]; got.Done != 0 || got.Running != 0 {
		t.Errorf("the loose delegate was counted into a run it never joined: %+v", got)
	}
	for _, task := range f.delegations.Snapshot() {
		if task.Run != "" || task.Phase != "" {
			t.Errorf("a delegate started without a phase carries one: %+v", task)
		}
	}
}

// Per-delegate spend, which is the number the card needs and the session's own
// stats cannot give. The parent's reporter must still see every round — the
// stamp adds an answer, it does not move the tokens.
func TestADelegatesSpendIsStampedWithWhoSpentIt(t *testing.T) {
	isolate(t)
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	delegations := NewDelegations()
	var relayed []model.Usage
	for _, tool := range NewTaskTools(TaskOptions{
		Provider:     usageProvider{},
		Model:        "usage-fake-model",
		Registry:     registry,
		Delegations:  delegations,
		ApprovalMode: safety.ApprovalFullAccess,
		OnUsage:      func(u model.Usage) { relayed = append(relayed, u) },
	}) {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}

	ctx := turn.WithCallID(context.Background(), "call_1")
	dispatcher := skill.NewDispatcher(registry)
	if _, _, err := dispatcher.ExecuteTool(ctx, "task", map[string]any{"action": "plan",
		"name": "audit", "phases": []any{"check"},
	}); err != nil {
		t.Fatalf("declare failed: %v", err)
	}
	start, _, err := dispatcher.ExecuteTool(ctx, "task", map[string]any{
		"description": "x", "prompt": "report the files", "agent": "explore", "phase": "check",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if _, _, err = dispatcher.ExecuteTool(ctx, "task", map[string]any{"action": "collect",
		"task_id": taskIDOf(t, start),
	}); err != nil {
		t.Fatalf("collect failed: %v", err)
	}

	snapshot := delegations.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot = %+v, want the one delegation", snapshot)
	}
	if snapshot[0].Tokens <= 0 {
		t.Errorf("the delegate's own spend was never recorded: %+v", snapshot[0])
	}
	if snapshot[0].Model != "usage-fake-model" {
		t.Errorf("model = %q, want what the delegate actually ran on", snapshot[0].Model)
	}
	if len(relayed) == 0 {
		t.Error("stamping the spend stopped it reaching the session's stats")
	}
	// And the run adds its delegates up, which is the number on the card's head.
	runs := delegations.Runs()
	if len(runs) != 1 || runs[0].Tokens != snapshot[0].Tokens {
		t.Errorf("run total = %+v, want the one delegate's %d", runs, snapshot[0].Tokens)
	}
	if len(runs) == 1 && runs[0].Phases[0].Tokens != snapshot[0].Tokens {
		t.Errorf("phase total = %+v, want the delegate's spend", runs[0].Phases[0])
	}
}

// A run with nothing in it is paperwork, and a run whose phases collide cannot
// be addressed. Both are refused as readable results rather than errors.
func TestTaskPlanRefusesWhatCannotBeAddressed(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"no name", map[string]any{"action": "plan", "phases": []any{"a"}}, "name"},
		{"no phases", map[string]any{"action": "plan", "name": "x", "phases": []any{}}, "just a delegate"},
		{"two of the same", map[string]any{"action": "plan", "name": "x", "phases": []any{"a", "a"}}, "could not say which"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := f.call(t, "call_"+tc.name, "task", tc.args)
			if out.Success {
				t.Fatalf("accepted %v", tc.args)
			}
			if !strings.Contains(out.Content, tc.want) {
				t.Errorf("refusal = %q, want it to say %q", out.Content, tc.want)
			}
		})
	}
}

// taskSchema is what the model is actually offered for `task` right now, read
// back the way a request assembles it.
func (f *taskFixture) taskSchema(t *testing.T) string {
	t.Helper()
	for _, def := range skill.NewDispatcher(f.registry).ToolDefinitions() {
		if def.Function.Name == "task" {
			return string(def.Function.Parameters)
		}
	}
	t.Fatal("task is not in the tool block")
	return ""
}

// The phase parameter exists only while a run does, and carries that run's own
// titles — a delegate cannot be told to name a phase that was never declared,
// and cannot be offered the parameter when there is nothing to put in it.
func TestThePhaseParameterFollowsTheDeclaration(t *testing.T) {
	f := newTaskFixture(t, "aetox-tools:test")
	if schema := f.taskSchema(t); strings.Contains(schema, `"phase":`) {
		t.Errorf("phase is offered with no run declared: %s", schema)
	}

	f.call(t, "call_1", "task", map[string]any{"action": "plan",
		"name": "ตรวจเอกสาร", "phases": []any{"รอบตรวจ", "รอบหักล้าง"},
	})
	schema := f.taskSchema(t)
	for _, want := range []string{`"phase":`, "รอบตรวจ", "รอบหักล้าง"} {
		if !strings.Contains(schema, want) {
			t.Errorf("the schema does not carry %q once a run is declared: %s", want, schema)
		}
	}
}
