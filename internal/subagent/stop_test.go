package subagent

import (
	"context"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// The brake, and what it is allowed to reach.
//
// A delegate's life is the session's, not the turn's (runner.go), which is the
// right call and is also what left the app with no brake at all on the ordinary
// path: the main agent dispatches, answers, the turn ends and the composer's
// Stop goes away, while up to four sub-agents keep looping. StopAll was the
// whole vocabulary, so the only answer to one runaway delegate was to throw
// away the three that were fine.

// blocker starts a delegation that does nothing but wait to be cancelled, which
// is every delegation from the brake's point of view.
func blocker(t *testing.T, r *Delegations, label string) *runningTask {
	t.Helper()
	task, err := r.start(delegation{profile: "explore", label: label},
		func(ctx context.Context, self *runningTask) skill.Output {
			<-ctx.Done()
			return skill.Output{Success: false, Content: "cancelled"}
		})
	if err != nil {
		t.Fatalf("start %s: %v", label, err)
	}
	return task
}

func waitFinished(t *testing.T, task *runningTask) {
	t.Helper()
	select {
	case <-task.done:
	case <-time.After(2 * time.Second):
		t.Fatal("delegation did not end after it was stopped")
	}
}

func TestStopEndsOneDelegateAndLeavesTheRest(t *testing.T) {
	r := NewDelegations()
	doomed := blocker(t, r, "the one looping")
	spared := blocker(t, r, "the one that is fine")

	if !r.Stop(doomed.id) {
		t.Fatal("Stop reported nothing to stop, with a delegation running")
	}
	waitFinished(t, doomed)

	if spared.finished() {
		t.Fatal("stopping one delegate ended another — that is StopAll wearing a different name")
	}
	// The point of the whole change: the survivor is still working, so the user
	// who wanted one job dead did not pay for it with the other three.
	if spared.wasStopped() {
		t.Error("a delegate nobody stopped is marked stopped")
	}
}

func TestStoppedIsReportedApartFromFailure(t *testing.T) {
	r := NewDelegations()
	task := blocker(t, r, "work the user ends")
	r.Stop(task.id)
	waitFinished(t, task)

	var info TaskInfo
	for _, i := range r.Snapshot() {
		if i.ID == task.id {
			info = i
		}
	}
	if !info.Stopped {
		t.Fatal("a stopped delegation does not say so on its snapshot")
	}
	// It reports !OK too, and that is exactly why Stopped has to exist: read
	// through OK alone this is indistinguishable from a delegate that broke,
	// and the desktop layer would send the model to go and read the wreckage of
	// work the user just paid a click to end.
	if info.OK {
		t.Error("a cancelled delegation reported success")
	}
}

func TestStopIsFalseWhenThereIsNothingLeftToStop(t *testing.T) {
	r := NewDelegations()
	if r.Stop("task_nope") {
		t.Error("Stop claimed to end a delegation that never existed")
	}
	task, err := r.start(delegation{profile: "explore", label: "quick"},
		func(context.Context, *runningTask) skill.Output {
			return skill.Output{Success: true, Content: "done"}
		})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	<-task.done
	// Not an error, and the tray depends on that: it polls every two seconds,
	// so a row can finish between the paint and the click, and "it is already
	// over" is the outcome the user wanted anyway.
	if r.Stop(task.id) {
		t.Error("Stop claimed to end a delegation that had already finished")
	}
}

func TestStopRunEndsEveryWorkerInThatJobOnly(t *testing.T) {
	r := NewDelegations()
	inRun := []*runningTask{}
	for _, phase := range []string{"หา", "ตรวจ"} {
		task, err := r.start(delegation{profile: "explore", label: phase, run: "run_1", phase: phase},
			func(ctx context.Context, self *runningTask) skill.Output {
				<-ctx.Done()
				return skill.Output{}
			})
		if err != nil {
			t.Fatalf("start %s: %v", phase, err)
		}
		inRun = append(inRun, task)
	}
	loose := blocker(t, r, "not part of the job")

	if n := r.StopRun("run_1"); n != 2 {
		t.Fatalf("StopRun ended %d workers, want 2", n)
	}
	for _, task := range inRun {
		waitFinished(t, task)
	}
	if loose.finished() {
		t.Error("stopping a run reached a delegate that was never in it")
	}
}

func TestSpendKeepsTheHalvesApart(t *testing.T) {
	task := &runningTask{}
	task.spend(model.Usage{PromptTokens: 4011, CachedPromptTokens: 3968, CacheReported: true, CompletionTokens: 120})
	task.spend(model.Usage{PromptTokens: 30, CompletionTokens: 7})

	in, out, cached, reported := task.tokenSplit()
	if in != 4041 || out != 127 {
		t.Errorf("split is in=%d out=%d, want in=4041 out=127", in, out)
	}
	// The total keeps meaning what it always meant, because the session's stats
	// and the run cards both read it.
	if got := task.tokens(); got != 4011+120+30+7 {
		t.Errorf("total is %d, want %d", got, 4011+120+30+7)
	}
	if !reported || cached != 3968 {
		t.Errorf("cache is cached=%d reported=%v, want 3968/true", cached, reported)
	}
}

func TestSpendStaysSilentAboutCacheNobodyReported(t *testing.T) {
	// A local runtime accounts for no cache at all. Summing its silent zero
	// would turn "nobody said" into "nothing hit", and the tray would draw a
	// hit rate the provider never claimed.
	task := &runningTask{}
	task.spend(model.Usage{PromptTokens: 900, CompletionTokens: 40})
	if _, _, cached, reported := task.tokenSplit(); reported || cached != 0 {
		t.Errorf("cache reported as cached=%d reported=%v for a provider that said nothing", cached, reported)
	}
}
