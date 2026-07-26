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
//
// ponytail: it counts delegates, not writers — four `general` delegates editing
// the same file at once would interleave, and nothing here stops them. The only
// guard today is prose: `task`'s description says to hand a list to ONE delegate,
// and general.md's prompt says the same. That matches how delegation is meant to
// be used, so it holds until it doesn't. If two delegates ever really do need to
// write at the same time, the fix is a worktree each, not a lock here.
const maxConcurrent = 4

// runningTask is one delegation in flight or finished. Fields after done is
// closed are read-only, so a collector needs no lock for them.
type runningTask struct {
	id      string
	profile string
	label   string
	started time.Time
	// ctx is the delegate's own, so a collector can tell "stopped" from "still
	// working" the instant Stop is pressed rather than when the goroutine
	// happens to notice.
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// asks carries a question from the delegate to whoever collects next.
	// Buffered by one, which is all that can ever be outstanding: the delegate
	// is parked inside the tool call until it is answered, so it cannot ask
	// twice (see ask.go).
	asks chan *pendingAsk

	mu        sync.Mutex
	toolCalls int
	pending   *pendingAsk // non-nil while a question is waiting for an answer

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

// ask parks the delegate until the parent answers. The context is the
// delegate's own, which descends from the turn's — so Stop unsticks a question
// nobody was ever going to answer, and no goroutine outlives its reply.
func (r *runningTask) ask(ctx context.Context, question string) (string, error) {
	p := &pendingAsk{question: question, asked: time.Now(), answer: make(chan string, 1)}
	select {
	case r.asks <- p:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	select {
	case answer := <-p.answer:
		return answer, nil
	case <-ctx.Done():
		// The asker is gone, so the question is void. Clearing it matters:
		// otherwise a collector keeps being handed a question about a delegate
		// that no longer exists, and answering it fails — a dead end costing a
		// round each time round.
		r.takeAsk()
		return "", ctx.Err()
	}
}

// currentAsk is the question already handed to a collector and still unanswered.
// Collect consults it before waiting, so a parent that collects twice without
// answering is told the same question again instead of blocking on a delegate
// that is itself blocked on the parent.
func (r *runningTask) currentAsk() *pendingAsk {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending
}

func (r *runningTask) setAsk(p *pendingAsk) {
	r.mu.Lock()
	r.pending = p
	r.mu.Unlock()
}

// takeAsk claims the outstanding question so exactly one answer reaches it.
func (r *runningTask) takeAsk() *pendingAsk {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.pending
	r.pending = nil
	return p
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
		started: time.Now(), ctx: childCtx, cancel: cancel, done: make(chan struct{}),
		asks: make(chan *pendingAsk, 1),
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

// collect waits for one delegation and returns it, or the question it is stuck
// on. A finished task keeps its result, so collecting twice is not an error — a
// model that loses track of what it already read gets the same answer rather
// than a failure.
//
// The pending-question check comes first and is what keeps a turn from wedging:
// a delegate waiting on `ask_main` is blocked on the parent, so a parent that
// blocked on it would leave both parked until Stop. Collecting an asking
// delegate returns the question — again, if it is asked again — never a wait.
func (r *runner) collect(ctx context.Context, id string) (*runningTask, *pendingAsk, error) {
	r.mu.Lock()
	task, ok := r.tasks[id]
	r.mu.Unlock()

	// The caller appends what is outstanding — saying it here too gave the model
	// the same list twice in one message.
	if !ok {
		return nil, nil, fmt.Errorf("no sub-agent has id %q", id)
	}
	// A delegate that is finished — or stopped, which it has not necessarily
	// noticed yet — outranks a question it asked earlier. Stop frees a parked
	// goroutine but cannot un-ask its question, so without this the parent is
	// told "it is waiting for a decision" about a delegate that is already dead,
	// and told it again on every collect. The task's own context is what makes
	// this exact rather than a race against the goroutine waking up.
	if task.finished() || task.ctx.Err() != nil {
		select {
		case <-task.done:
			return task, nil, nil
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	if p := task.currentAsk(); p != nil {
		return task, p, nil
	}
	select {
	case <-task.done:
		return task, nil, nil
	case p := <-task.asks:
		task.setAsk(p)
		return task, p, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

// answer releases a parked delegate. Refusing an unasked task is deliberate: a
// model answering a question nobody asked has lost track of which delegate it
// was talking to, and silently accepting it would strand the real one.
func (r *runner) answer(id, text string) error {
	r.mu.Lock()
	task, ok := r.tasks[id]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("no sub-agent has id %q", id)
	}
	// Drain a question that was posted but never collected, so answering works
	// even if the model somehow knew what to say without collecting first.
	if task.currentAsk() == nil {
		select {
		case p := <-task.asks:
			task.setAsk(p)
		default:
		}
	}
	p := task.takeAsk()
	if p == nil {
		if task.finished() {
			return fmt.Errorf("sub-agent %s has already finished — collect it with task_result", id)
		}
		return fmt.Errorf("sub-agent %s is not waiting for an answer; it is still working", id)
	}
	p.answer <- text // buffered: never blocks, even on a delegate already cancelled
	return nil
}

// waiting lists the delegates parked on a question, for the message a wrong id
// gets back.
func (r *runner) waiting() []string {
	r.mu.Lock()
	tasks := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		tasks = append(tasks, t)
	}
	r.mu.Unlock()

	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if p := t.currentAsk(); p != nil {
			out = append(out, fmt.Sprintf("%s (%s): %s", t.id, t.profile, truncate(p.question, 80)))
		}
	}
	return out
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
