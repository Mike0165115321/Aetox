package turn

// The pending-call ledger and the Stop button.
//
// The executor lives for the whole session, and dispatchWithDeadline parks a
// call's entry so "ask again" collects a result instead of re-running the
// work. A cancelled turn broke that contract from the other side: its entry
// stayed parked holding "tool execution canceled", and the next turn's
// identical call — the user pressing Stop and then asking for the same thing
// again, the single most natural sequence there is — was answered from the
// corpse instead of being run.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// blockingDispatcher's tool runs until its ctx dies, then reports that death —
// the shape of a real shell command the Stop button kills.
type blockingDispatcher struct {
	executions atomic.Int64
}

func (d *blockingDispatcher) Execute(context.Context, string) (skill.Output, bool, error) {
	return skill.Output{}, false, nil
}

func (d *blockingDispatcher) ToolDefinitions() []model.ToolDefinition { return nil }

func (d *blockingDispatcher) ExecuteTool(ctx context.Context, name string, _ map[string]any) (skill.Output, bool, error) {
	if d.executions.Add(1) == 1 {
		<-ctx.Done()
		return skill.Output{Name: name, Success: false}, true, ctx.Err()
	}
	return skill.Output{Name: name, Content: "real result", RawOutput: "real result", Success: true}, true, nil
}

func TestCancelledCallDoesNotAnswerTheNextTurn(t *testing.T) {
	d := &blockingDispatcher{}
	e := NewExecutor(ExecutorOptions{Agent: &toolAwareAgent{}, Dispatcher: d})
	args := map[string]any{"args": []any{"go", "test", "./..."}}

	// Turn 1: the call blocks, the user presses Stop.
	turn1, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	out, _, err := e.dispatchWithDeadline(turn1, "shell", args)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("turn 1 = (%q, %v), want the cancellation", out.Content, err)
	}

	// The dying goroutine needs a moment to observe its ctx and finish.
	deadline := time.Now().Add(2 * time.Second)
	for d.executions.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	// Turn 2: the same call again must RUN, not replay the corpse.
	out, handled, err := e.dispatchWithDeadline(context.Background(), "shell", args)
	if err != nil || !handled {
		t.Fatalf("turn 2 = (%v, handled=%v), want a clean run", err, handled)
	}
	if out.Content != "real result" {
		t.Errorf("turn 2 content = %q, want the fresh execution's output — the stale 'canceled' means the ledger replayed a dead turn", out.Content)
	}
	if got := d.executions.Load(); got != 2 {
		t.Errorf("tool executed %d time(s), want 2 — the second call must actually run", got)
	}
}
