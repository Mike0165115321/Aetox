package subagent

// The run bench driven end to end, the way a person clicking aetox-run:test
// drives it: the built-in model on both sides, the real executor, the real
// tools. It is the only test that proves the script and the mechanism agree —
// a bench that half works is worse than none, because it is watched rather than
// asserted on, and a phase that silently never fills looks like the feature.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/cognitive"
	"github.com/Mikedev115/Aetox/internal/command"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/turn"
)

func TestTheRunBenchFillsEveryPhaseItDeclares(t *testing.T) {
	f := newTaskFixture(t, "aetox-run:test")

	// A deadline rather than a bare context: a script that stops advancing
	// hangs the whole package otherwise, which is how this kind of bug reaches
	// CI as a timeout nobody can read.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const ask = "ทดสอบชุดงาน"
	result, err := f.exec.Execute(ctx, ask, command.Intent{Raw: ask, Kind: command.KindConversation}, nil, nil, nil)
	if err != nil {
		t.Fatalf("the bench did not finish: %v", err)
	}
	t.Logf("final reply:\n%s", result.Reply)

	runs := f.delegations.Runs()
	if len(runs) != 1 {
		t.Fatalf("Runs() = %d, want the one the bench declares", len(runs))
	}
	run := runs[0]
	if len(run.Phases) != 3 {
		t.Fatalf("phases = %+v, want the three the bench declares", run.Phases)
	}
	for _, phase := range run.Phases {
		t.Logf("%-24s planned=%d done=%d failed=%d running=%d", phase.Title, phase.Planned, phase.Done, phase.Failed, phase.Running)
		if phase.Planned == 0 {
			t.Errorf("phase %q declared no count, so the card has no denominator to draw", phase.Title)
		}
		// Every phase the bench promises has to fill, or the demo teaches the
		// opposite of what the mechanism is for.
		if phase.Done != phase.Planned {
			t.Errorf("phase %q finished %d of the %d it planned", phase.Title, phase.Done, phase.Planned)
		}
		if phase.Failed != 0 {
			t.Errorf("phase %q had %d failed delegate(s); the bench must not demo a broken run", phase.Title, phase.Failed)
		}
	}
	if run.Running {
		t.Error("the bench ended with work still in flight")
	}

	// The wave is the point: three delegates started before the first collect,
	// which is what puts three rows on the card at the same elapsed time.
	tasks := f.delegations.Snapshot()
	if len(tasks) != 6 {
		t.Errorf("the bench ran %d delegates, want the 6 it declares", len(tasks))
	}
	for _, task := range tasks {
		if task.Run != run.ID || task.Phase == "" {
			t.Errorf("a bench delegate landed outside the run: %+v", task)
		}
		if task.Model != "aetox-run:test" {
			t.Errorf("delegate model = %q, want the session's own", task.Model)
		}
	}
}

// The bench's delegates must be read-only. Six of them work at once in the
// user's real sandbox when this is clicked, and a demo that has six writers on
// one path is demonstrating interleaving.
func TestTheRunBenchDelegatesTouchNothing(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root})
	provider := model.NewNoopProvider("aetox-run:test")
	delegations := NewDelegations()

	// One slice, two writers: a delegate reports on its own goroutine
	// (Delegations.start) while the main agent reports on this one, which is the
	// arrangement the bench exists to exercise. Unguarded it is a data race the
	// Linux job catches and Windows does not (found in CI, 16 ส.ค.).
	var mu sync.Mutex
	var events []turn.ToolEvent
	record := func(ev turn.ToolEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}

	for _, tool := range NewTaskTools(TaskOptions{
		Provider: provider, Model: "aetox-run:test", Registry: registry,
		Delegations: delegations, ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: record,
	}) {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	exec := turn.NewExecutor(turn.ExecutorOptions{
		Agent: cognitive.NewAgent(cognitive.AgentConfig{
			Provider: provider, Model: "aetox-run:test", SystemPrompt: "bench",
		}),
		Dispatcher:   skill.NewDispatcher(registry),
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: record,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	const ask = "ทดสอบชุดงาน"
	if _, err := exec.Execute(ctx, ask, command.Intent{Raw: ask, Kind: command.KindConversation}, nil, nil, nil); err != nil {
		t.Fatalf("the bench did not finish: %v", err)
	}

	// Execute has returned and every delegate with it, but the lock is what says
	// so to the race detector rather than the ordering being obvious to a reader.
	mu.Lock()
	defer mu.Unlock()
	for _, ev := range events {
		switch ev.Name {
		case "write", "edit", "edits", "delete", "shell":
			t.Errorf("the bench ran %q — its delegates work in the user's own sandbox", ev.Name)
		}
	}
}
