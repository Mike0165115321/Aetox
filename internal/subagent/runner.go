package subagent

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// Delegation is asynchronous: `task` starts a sub-agent and returns immediately,
// `task_result` collects it later. Owner's requirement, and the reason is the
// whole point of delegating — *"ตอนส่งงานไปให้ซับเอเจนมันจะต้องไม่เสียเวลารอนะ มันต้องไปทำ
// อย่างอื่นได้ระหว่างรอ ไม่ใช่ให้มันโยนงานนะแต่มันต้องรู้ว่าต้องทำอะไร"* (§44.11).
//
// The shape falls out of the tool loop we already have: it is synchronous, one
// round at a time, so the way to stop waiting is not to make the loop
// asynchronous — it is to let one tool return a handle and another one redeem it.
// The model stays in charge of what happens in between, which is what makes this
// "go do something else" rather than "fire and forget".
//
// Two consequences worth naming:
//
//   - N delegates started before the first collect run concurrently, so wall
//     clock is the slowest one rather than the sum. Parallelism arrives as a
//     property of the tool pair, not as a second mechanism (§44.9).
//   - A delegate never outlives its turn: its context descends from the turn's,
//     so Stop kills every outstanding one and nothing can leak past the reply.

// maxConcurrent caps how many delegates one turn may have in flight. Four is a
// desktop-sized number: enough for a real fan-out, few enough that a model in a
// loop cannot melt the machine or the provider's rate limit.
//
// ponytail: a fixed number, not a setting. Turn it into one when somebody has a
// measured reason to want a different value.
const maxConcurrent = 4

// runningTask is one delegation in flight or finished. Fields after done is
// closed are read-only, so a collector needs no lock for them.
type runningTask struct {
	id      string
	profile string
	label   string
	started time.Time
	cancel  context.CancelFunc
	done    chan struct{}

	mu        sync.Mutex
	toolCalls int

	// Written exactly once, before done is closed.
	output skill.Output
}

func (r *runningTask) countCall() {
	r.mu.Lock()
	r.toolCalls++
	r.mu.Unlock()
}

func (r *runningTask) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.toolCalls
}

func (r *runningTask) finished() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// runner tracks this session's delegations. One instance is shared by the `task`
// and `task_result` tools — they are two halves of one mechanism, so they share
// state rather than looking each other up.
type runner struct {
	mu      sync.Mutex
	tasks   map[string]*runningTask
	counter atomic.Uint64
}

func newRunner() *runner { return &runner{tasks: map[string]*runningTask{}} }

// start registers a delegation and launches work in the background. work must
// close nothing and return the output; the runner publishes it and closes done.
func (r *runner) start(ctx context.Context, profile, label string, work func(context.Context, *runningTask) skill.Output) (*runningTask, error) {
	r.mu.Lock()
	inFlight := 0
	for _, t := range r.tasks {
		if !t.finished() {
			inFlight++
		}
	}
	if inFlight >= maxConcurrent {
		r.mu.Unlock()
		return nil, fmt.Errorf("%d sub-agents are already running (the limit is %d) — collect one with task_result before starting another", inFlight, maxConcurrent)
	}
	id := "task_" + strconv.FormatUint(r.counter.Add(1), 10)
	// The delegate's context descends from the turn's: Stop cancels every
	// outstanding delegate, and none can outlive the reply it was meant to serve.
	childCtx, cancel := context.WithCancel(ctx)
	task := &runningTask{
		id: id, profile: profile, label: label,
		started: time.Now(), cancel: cancel, done: make(chan struct{}),
	}
	r.tasks[id] = task
	r.mu.Unlock()

	go func() {
		defer close(task.done)
		defer cancel()
		task.output = work(childCtx, task)
	}()
	return task, nil
}

// collect waits for one delegation and returns it. A finished task keeps its
// result, so collecting twice is not an error — a model that loses track of what
// it already read gets the same answer rather than a failure.
func (r *runner) collect(ctx context.Context, id string) (*runningTask, error) {
	r.mu.Lock()
	task, ok := r.tasks[id]
	r.mu.Unlock()

	// The caller appends what is outstanding — saying it here too gave the model
	// the same list twice in one message.
	if !ok {
		return nil, fmt.Errorf("no sub-agent has id %q", id)
	}
	select {
	case <-task.done:
		return task, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// running lists what has not been collected yet, for a status line the model can
// read when it wonders what it is still owed.
func (r *runner) running() []*runningTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		if !t.finished() {
			out = append(out, t)
		}
	}
	return out
}
