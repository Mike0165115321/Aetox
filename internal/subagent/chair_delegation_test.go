package subagent

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// A chair chat may use hands. It may not hand a whole job to another colleague,
// and a chair reached by `task` still gets neither (§151).
//
// Both halves in one test on purpose: the value of the grant is entirely in
// where it stops, and a test that only proved the tool arrives would pass just
// as well on the version that hands over the main agent's full roster.
func TestAChairChatGetsHandsButNotColleagues(t *testing.T) {
	isolate(t)

	parent := skill.NewRegistry()
	for _, tool := range NewTaskTools(TaskOptions{}) {
		if err := parent.Register(tool, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	p, ok := Load("doc")
	if !ok {
		t.Fatal("the bundled doc profile did not load")
	}
	desk, _ := mode.Load(p.Desk)

	// Handed a job by `task`, it is a leaf and stays one: depth 1 is still
	// enforced by absence rather than by a counter.
	if _, has := FilterRegistry(parent, p, desk).Get("task"); has {
		t.Error("a delegate carries task — a leaf that can start its own delegate is unbounded depth")
	}

	chair := AttendedRegistry(parent, p, desk)
	tool, has := chair.Get("task")
	if !has {
		t.Fatal("a chair chat has no task at all — the person sitting in it is the root of this tree, as the main agent is of its own")
	}
	packed, isDelegation := tool.(*delegationTool)
	if !isDelegation {
		t.Fatalf("the chair's task is a %T, not the delegation tool", tool)
	}

	var colleagues, hands []string
	for _, worker := range packed.start.available() {
		if worker.Desk != "" {
			colleagues = append(colleagues, worker.Name)
			continue
		}
		hands = append(hands, worker.Name)
	}
	if len(colleagues) > 0 {
		t.Errorf("the chair is offered colleagues %v — one peer handing another a whole job is a decision for the person who asked", colleagues)
	}
	if len(hands) == 0 {
		t.Error("the chair is offered nobody at all, which is the same as not having the tool")
	}

	// task_plan stays out for the reason todo_write does: the run panel belongs
	// to the conversation the person is watching, and nothing draws a second.
	for _, action := range packed.allowedActions() {
		if strings.Contains(strings.ToLower(action), "plan") {
			t.Errorf("the chair carries the %q action, which would draw a second run panel", action)
		}
	}
}

// The refusal has to name what to do instead, because the model reaches for a
// colleague exactly when the work genuinely belongs to one — and the useful
// answer is to say so to the person, not to fall silent.
func TestRefusingAColleagueTellsTheChairWhatToDoInstead(t *testing.T) {
	isolate(t)

	tool := &taskTool{opts: TaskOptions{NoAgents: true, Desk: nil}}
	research, ok := Load("research")
	if !ok {
		t.Fatal("the bundled research profile did not load")
	}
	_, err := tool.reach(research)
	if err == nil {
		t.Fatal("a helpers-only roster reached a colleague")
	}
	if !strings.Contains(err.Error(), research.Name) {
		t.Errorf("the refusal does not name the worker it is about: %v", err)
	}
	for _, want := range []string{"helper", "person"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Errorf("the refusal does not say what to do instead — missing %q: %v", want, err)
		}
	}

	// The same tool without the narrowing still reaches it, so the test above is
	// measuring the switch rather than some other refusal on the way.
	open := &taskTool{opts: TaskOptions{Desk: nil}}
	if _, err := open.reach(research); err != nil {
		t.Errorf("research is unreachable even without the narrowing, so this proves nothing: %v", err)
	}
}
