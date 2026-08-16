package subagent

import (
	"context"
	"fmt"
	"sort"
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
//   - A delegate outlives the turn that started it, and only the user ends it
//     early. See the rule on Delegations below.

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
	// model is which model this delegate actually ran on, which is its profile's
	// when it names one and the session's otherwise. Recorded rather than
	// recomputed: the profile can be edited while the work is in flight, and a
	// tray that answered from the file would relabel a finished run.
	model string
	// run and phase are the declared job this delegation belongs to, both empty
	// for a loose delegate (see run.go). Written at start and never again.
	run     string
	phase   string
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
	// tokensUsed is this delegate's own spend, summed as its rounds come back.
	//
	// It is counted here as well as reported upward because the two answer
	// different questions and only one of them was ever answerable: the parent's
	// reporter is told "the user spent this", which is true and is why a
	// delegate's tokens have always landed in the session's stats untouched
	// (task.go). Nothing was told WHO spent it, so "this run cost 712k across
	// eight workers" had no source. Stamping it here changes no total.
	tokensUsed int
	pending    *pendingAsk // non-nil while a question is waiting for an answer
	// parked is non-nil for the whole time the delegate's goroutine is inside
	// ask() — from before the question is even collectable to the moment an
	// answer (or cancellation) frees it. It exists for Snapshot alone: pending
	// only learns of a question when a collector comes asking, and the tray has
	// to show "stuck, needs you" before anyone has thought to collect.
	parked *pendingAsk
	// collected: somebody has redeemed the finished result at least once. The
	// tray hides a collected row — the work is in the conversation now — and
	// only collect can know this, because collecting IS the one door out.
	collected bool

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

// spend records one round's tokens against this delegate. Called from the
// delegate's own usage reporter, which runs on its goroutine.
func (r *runningTask) spend(n int) {
	if n <= 0 {
		return
	}
	r.mu.Lock()
	r.tokensUsed += n
	r.mu.Unlock()
}

func (r *runningTask) tokens() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokensUsed
}

// ask parks the delegate until somebody answers, which may be a later turn than
// the one that started it: a question is not lost when the reply it would have
// arrived in has already been given. The context is the delegate's own, so Stop
// unsticks a question nobody was ever going to answer.
func (r *runningTask) ask(ctx context.Context, question string) (string, error) {
	p := &pendingAsk{question: question, asked: time.Now(), answer: make(chan string, 1)}
	r.setParked(p)
	defer r.setParked(nil)
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

func (r *runningTask) setParked(p *pendingAsk) {
	r.mu.Lock()
	r.parked = p
	r.mu.Unlock()
}

func (r *runningTask) parkedAsk() *pendingAsk {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.parked
}

// takeAsk claims the outstanding question so exactly one answer reaches it.
func (r *runningTask) takeAsk() *pendingAsk {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.pending
	r.pending = nil
	return p
}

func (r *runningTask) markCollected() {
	r.mu.Lock()
	r.collected = true
	r.mu.Unlock()
}

func (r *runningTask) wasCollected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.collected
}

func (r *runningTask) finished() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// Delegations is this session's register of delegations — running, parked on a
// question, and finished. One instance is shared by the `task`, `task_result`
// and `task_answer` tools, because they are three halves of one mechanism and
// sharing state beats looking each other up.
//
// It is exported so the host can own it, and the host has to own it for one
// reason: Stop. A delegate's life is the session's, not the turn's (see start),
// which means the turn's context no longer ends one — and "the user pressed
// Stop" is a fact only the host has. Everything else about a delegation stays
// in here.
type Delegations struct {
	mu      sync.Mutex
	tasks   map[string]*runningTask
	counter atomic.Uint64

	// The declared jobs delegations can belong to (run.go). openRun is the one a
	// delegate joins by naming a phase of it; runOrder keeps declaration order,
	// which is the order the tray reads them back in.
	runs       map[string]*Run
	runOrder   []string
	openRun    string
	runCounter atomic.Uint64
}

// NewDelegations builds an empty register. A host that keeps the returned value
// can stop what is running; one that does not gets the same behaviour minus
// that door, which is what every test and the CLI want (see TaskOptions).
func NewDelegations() *Delegations {
	return &Delegations{tasks: map[string]*runningTask{}, runs: map[string]*Run{}}
}

// delegation is what start needs to know about a job before it runs it. A struct
// rather than five parameters because three of them are strings that mean
// entirely different things, and a call site that swapped two would compile.
type delegation struct {
	profile string
	label   string
	model   string
	// run and phase are empty for a loose delegate, which is every delegation
	// that was started without a declared job around it.
	run   string
	phase string
}

// start registers a delegation and launches work in the background. work must
// close nothing and return the output; the register publishes it and closes done.
//
// The delegate's context is its own — context.Background, not the caller's —
// and that is the whole of the background promise. shell_background settled the
// same question for commands first, in the same words: "a dev server that dies
// with the answer that started it is not a background command". Neither is an
// agent. A turn that ends while a delegate is still reading files used to throw
// the entire run away at the moment of the reply, so a model that had not
// thought to collect yet lost minutes of real work and the user watching it
// happen had no way to keep it.
//
// What still ends one early is StopAll, and only StopAll: the user pressing
// Stop is a statement about the work, not about the turn the work happened to
// start in. Nothing outlives the process — a goroutine cannot.
func (r *Delegations) start(spec delegation, work func(context.Context, *runningTask) skill.Output) (*runningTask, error) {
	r.mu.Lock()
	inFlight := 0
	for _, t := range r.tasks {
		if !t.finished() {
			inFlight++
		}
	}
	if inFlight >= maxConcurrent {
		r.mu.Unlock()
		return nil, fmt.Errorf("%d sub-agents are already running (the limit is %d) — collect one with task(action=collect) before starting another", inFlight, maxConcurrent)
	}
	id := "task_" + strconv.FormatUint(r.counter.Add(1), 10)
	childCtx, cancel := context.WithCancel(context.Background())
	task := &runningTask{
		id: id, profile: spec.profile, label: spec.label, model: spec.model,
		run: spec.run, phase: spec.phase,
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
func (r *Delegations) collect(ctx context.Context, id string) (*runningTask, *pendingAsk, error) {
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
			task.markCollected()
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
		task.markCollected()
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
func (r *Delegations) answer(id, text string) error {
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
			return fmt.Errorf("sub-agent %s has already finished — collect it with task(action=collect)", id)
		}
		return fmt.Errorf("sub-agent %s is not waiting for an answer; it is still working", id)
	}
	p.answer <- text // buffered: never blocks, even on a delegate already cancelled
	return nil
}

// waiting lists the delegates parked on a question, for the message a wrong id
// gets back.
func (r *Delegations) waiting() []string {
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
func (r *Delegations) running() []*runningTask {
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

// TaskInfo is one delegation as the host's tray sees it: who is working, on
// what, since when, how far along, and whether anyone needs to do anything.
//
// Read from the register rather than reconstructed from tool events, because
// the register is the authority and the events cannot answer the one question
// the tray exists for: the `task` tool call completes the instant the handle is
// returned — the moment the work *starts* — so a UI watching events sees every
// delegation as finished from birth.
type TaskInfo struct {
	ID      string    `json:"id"`
	Profile string    `json:"profile"`
	Label   string    `json:"label"`
	Started time.Time `json:"started"`
	// Model is what this delegate ran on. Worth a column only because a profile
	// may name its own, so a row can differ from the session's model without the
	// user having chosen anything.
	Model string `json:"model,omitempty"`
	// Tokens is this delegate's own spend. The session's total already includes
	// it; this says whose it was.
	Tokens int `json:"tokens"`
	// Run and Phase are the declared job this belongs to, both empty for a loose
	// delegate (run.go).
	Run   string `json:"run,omitempty"`
	Phase string `json:"phase,omitempty"`
	// ToolCalls so far — live while running, final once done.
	ToolCalls int `json:"toolCalls"`
	// Running means the work is still going. Waiting narrows it: parked on a
	// question, going nowhere until somebody answers.
	Running  bool   `json:"running"`
	Waiting  bool   `json:"waiting"`
	Question string `json:"question,omitempty"`
	// OK is the outcome, meaningful only once Running is false.
	OK bool `json:"ok"`
	// ElapsedMs is how long the delegation actually took, set once Running is
	// false and 0 before that (the clock is still running; read it from Started).
	//
	// It exists because nothing else can answer it. The `task` tool call returns
	// the instant the work starts, so its own duration is the spawn, not the job
	// — a UI reading that number tells the user a delegate finished in 8s while
	// it is on its twenty-seventh tool call.
	ElapsedMs int64 `json:"elapsedMs,omitempty"`
	// Collected: the finished result has been redeemed at least once — the
	// work is in a conversation now, so a tray can stop offering it.
	Collected bool `json:"collected"`
}

// Snapshot lists every delegation this session has had, newest first.
//
// Everything on it is safe to read concurrently: fields written after done
// closes are guarded by the done check, and the live ones (calls, the pending
// ask) take the task's own lock.
func (r *Delegations) Snapshot() []TaskInfo {
	r.mu.Lock()
	tasks := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		tasks = append(tasks, t)
	}
	r.mu.Unlock()

	out := make([]TaskInfo, 0, len(tasks))
	for _, t := range tasks {
		info := TaskInfo{
			ID: t.id, Profile: t.profile, Label: t.label,
			Model: t.model, Run: t.run, Phase: t.phase,
			Started: t.started, ToolCalls: t.calls(), Tokens: t.tokens(),
			Running: !t.finished(),
		}
		if info.Running {
			if p := t.parkedAsk(); p != nil {
				info.Waiting = true
				info.Question = p.question
			}
		} else {
			info.OK = t.output.Success
			info.ElapsedMs = t.output.DurationMs
			info.Collected = t.wasCollected()
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out
}

// StopAll ends every delegation still running, and reports how many it found.
//
// This is the Stop button reaching the work, and it is the only thing that ends
// a delegate early now that a turn ending does not (see start). The host calls
// it; nothing in this package does, because "the user pressed Stop" is not a
// fact a tool can observe.
//
// Cancelling is enough — it does not wait. Each delegate notices through its own
// context, unparks whatever it was blocked on (a question nobody answered, a
// tool mid-call) and winds its own loop up; a Stop that blocked until the
// slowest one had finished would be a Stop that visibly does not stop.
func (r *Delegations) StopAll() int {
	r.mu.Lock()
	stopping := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		if !t.finished() {
			stopping = append(stopping, t)
		}
	}
	r.mu.Unlock()

	for _, t := range stopping {
		t.cancel()
	}
	return len(stopping)
}
