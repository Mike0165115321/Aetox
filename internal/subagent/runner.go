package subagent

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
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
//
// It is a limit on how many run AT ONCE, not on how many you may ask for. The
// fifth waits (see start); it used to be refused, and the refusal is what the
// owner saw on 30 ส.ค.: six workers dispatched in one round, four running and
// two red cards reading "4 sub-agents are already running". Every one of those
// six was a real job the model had already decided on, and the two that lost the
// race were not the two that mattered least — they were the two the scheduler
// happened to reach last. A cap that turns a queue into a failure makes the
// model's fan-out a lottery, and asks it to hand-manage a scheduler it cannot
// see. **"เราต้องสร้างระบบแบบ รอโมเดลด้วย"** (owner, 30 ส.ค.).
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
	run   string
	phase string
	// asked is when the delegation was requested, which is not when it began:
	// with four already running it waits its turn first. Write-once.
	asked time.Time
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

	mu sync.Mutex
	// started is when the work actually began — after the wait for a slot, not
	// before it. Under the lock because it is written once the delegate is
	// admitted and read live by the tray, and because the difference is the
	// whole point: a delegate that waited 40s for a slot and then worked for 5
	// must not report 45, or every duration in the app inherits the queue.
	//
	// Set to `asked` until then, so nothing reads a zero time and the tray can
	// sort a queued row among the rest.
	started time.Time
	// queued: registered, and still waiting for one of the maxConcurrent slots.
	// It has an id, it is in the register, it is collectable and Stop reaches
	// it — it simply has not started.
	queued    bool
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
	// The same spend, split the way the bill is: input the provider had to
	// read, output it wrote. One number cannot answer what a person watching a
	// delegate reach its seventy-second tool call is actually asking, which is
	// WHICH HALF is growing โ€” a transcript re-sent every round and a model
	// writing at length cost the same and mean opposite things, and only one of
	// them is worth stopping.
	//
	// The split was always in hand and thrown away one line before it arrived
	// (task.go handed spend a Usage and kept only the total), so this costs a
	// wider parameter and nothing else.
	tokensIn  int
	tokensOut int
	// cachedIn is the part of tokensIn the provider served from its own prompt
	// cache; cacheReported is whether it accounts for that at all. A local
	// runtime reports neither, and drawing that as a 0% hit rate would be the
	// tray claiming something the provider never said (model.Usage).
	cachedIn      int
	cacheReported bool
	// stopped: the user ended this delegate on purpose. Kept apart from the
	// failure it would otherwise be indistinguishable from, because "I stopped
	// it" and "it broke" are different sentences and only the second is worth
	// sending the model back to look at (see taskState, desktop layer).
	stopped bool
	pending *pendingAsk // non-nil while a question is waiting for an answer
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

// startedAt is when the work began, or when it was asked for while it is still
// waiting for a slot.
func (r *runningTask) startedAt() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started
}

// isQueued reports whether this delegation is still waiting for a slot.
func (r *runningTask) isQueued() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.queued
}

// beginRun marks the moment a waiting delegation was admitted. The clock every
// duration is measured from starts here.
func (r *runningTask) beginRun() {
	r.mu.Lock()
	r.queued = false
	r.started = time.Now()
	r.mu.Unlock()
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

// spend records one round's usage against this delegate. Called from the
// delegate's own usage reporter, which runs on its goroutine.
//
// It takes the whole Usage rather than a total because the caller already holds
// one โ€” the total was simply the only part that used to survive the trip. A
// round that reports nothing at all is still ignored; a round that reports only
// one half is not, because half a bill is a fact and a zero is not.
func (r *runningTask) spend(u model.Usage) {
	in, out := atLeastZero(u.PromptTokens), atLeastZero(u.CompletionTokens)
	total := u.TotalTokenCount()
	if in == 0 && out == 0 && total <= 0 {
		return
	}
	r.mu.Lock()
	r.tokensUsed += total
	r.tokensIn += in
	r.tokensOut += out
	// Only from a provider that accounts for it. Summing a 0 from one that does
	// not would turn "unknown" into "nothing hit", which reads as a real answer.
	if u.CacheReported {
		r.cacheReported = true
		r.cachedIn += atLeastZero(u.CachedPromptTokens)
	}
	r.mu.Unlock()
}

// atLeastZero floors a reported count. A provider sending a negative is not a
// case worth a branch at every call site, but it must never subtract from a
// running total the user is reading.
func atLeastZero(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func (r *runningTask) tokens() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokensUsed
}

// tokenSplit reports the same spend as input and output, with the cached share
// of the input when the provider says there is one.
func (r *runningTask) tokenSplit() (in, out, cached int, reported bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokensIn, r.tokensOut, r.cachedIn, r.cacheReported
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

// markStopped records that the user ended this delegate, before the cancel that
// actually ends it. The order matters: the goroutine may notice the cancelled
// context and finish within microseconds, and a snapshot taken in between would
// otherwise draw a delegate the user just stopped as one that failed on its own.
func (r *runningTask) markStopped() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

func (r *runningTask) wasStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
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

	// slots is the concurrency limit as a thing you wait on rather than a number
	// you are refused by: maxConcurrent tokens, taken before a delegate's work
	// begins and returned when it ends. Go queues blocked senders in arrival
	// order, so once the slots are full the waiting delegates start in the order
	// they began waiting — the model's own ordering survives, which is the part
	// a refusal destroyed. (Delegates dispatched in the same breath race each
	// other to the wait, which is not an order anybody promised: what matters is
	// that all of them run.)
	//
	// Held by the delegate's own goroutine for exactly as long as it works, so a
	// slot cannot be leaked by an early return: the release is deferred beside
	// the close of `done`.
	slots chan struct{}

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
	return &Delegations{
		tasks: map[string]*runningTask{},
		runs:  map[string]*Run{},
		slots: make(chan struct{}, maxConcurrent),
	}
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
// Nothing here refuses. A job asked for while maxConcurrent are already working
// waits for a slot and starts in the order it was asked, however many are ahead
// of it.
//
// There was a ceiling on outstanding delegations for one round of this change,
// on the argument that a model asking for a seventeenth job without collecting
// one is looping rather than fanning out. That argument is true and it is not
// this function's to act on. It is the same bargain §110 and §163 already
// settled twice: **the bound is a number on screen with a hand on the switch,
// never a refusal** — a refusal picks which of the user's jobs dies by whichever
// one asked last, which is not a decision anybody made. What makes it honest
// here is that both halves finally exist for a queue too: the tray draws what is
// waiting, and StopQueued clears the whole line in one press.
func (r *Delegations) start(spec delegation, work func(context.Context, *runningTask) skill.Output) *runningTask {
	now := time.Now()
	r.mu.Lock()
	id := "task_" + strconv.FormatUint(r.counter.Add(1), 10)
	childCtx, cancel := context.WithCancel(context.Background())
	task := &runningTask{
		id: id, profile: spec.profile, label: spec.label, model: spec.model,
		run: spec.run, phase: spec.phase,
		asked: now, started: now, queued: true,
		ctx: childCtx, cancel: cancel, done: make(chan struct{}),
		asks: make(chan *pendingAsk, 1),
	}
	r.tasks[id] = task
	r.mu.Unlock()

	go func() {
		// Order matters and is the reverse of how it reads: cancel, then close,
		// then hand the slot back. Releasing before `done` closes would admit the
		// next delegate while this one still counted as working, and the count is
		// what the tray and the model are both reading.
		defer r.release(task)
		defer close(task.done)
		defer cancel()

		// The wait, and the one place a delegation can end without ever having
		// run: Stop reaches a queued delegate through the same context every
		// other part of this uses, so a user who changes their mind about a
		// fan-out does not have to wait for it to start before stopping it.
		select {
		case r.slots <- struct{}{}:
		case <-childCtx.Done():
			task.output = failure(task.id, spec.label, 0,
				"sub-agent stopped while it was waiting for a free slot")
			return
		}
		task.beginRun()
		task.output = work(childCtx, task)
	}()
	return task
}

// release hands a slot back, and only for a delegate that took one. A delegation
// stopped while queued never held one, and returning a token it did not take
// would raise the concurrency limit by one for the rest of the session.
func (r *Delegations) release(task *runningTask) {
	if task.isQueued() {
		return
	}
	select {
	case <-r.slots:
	default:
	}
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
	// TokensIn and TokensOut split that spend into what the model read and what
	// it wrote, live while the delegate runs. Two numbers rather than one
	// because they are two different problems: input that keeps climbing is a
	// transcript being re-sent, output that keeps climbing is a model that will
	// not stop writing, and a person deciding whether to press the brake is
	// deciding between exactly those.
	//
	// TokensIn counts cached input too, the same as model.Usage.PromptTokens.
	// CachedIn is the part of it that was served from the provider's cache, and
	// CacheReported says whether the provider accounts for that at all: without
	// it, a local runtime that reports nothing is indistinguishable from one
	// that reported a genuine zero, and the tray must not draw the second.
	TokensIn      int  `json:"tokensIn"`
	TokensOut     int  `json:"tokensOut"`
	CachedIn      int  `json:"cachedIn"`
	CacheReported bool `json:"cacheReported"`
	// Run and Phase are the declared job this belongs to, both empty for a loose
	// delegate (run.go).
	Run   string `json:"run,omitempty"`
	Phase string `json:"phase,omitempty"`
	// ToolCalls so far — live while running, final once done.
	ToolCalls int `json:"toolCalls"`
	// Running means the work is still going. Waiting and Queued each narrow it,
	// and they are opposite kinds of stuck: Waiting is parked on a question and
	// goes nowhere until somebody answers, Queued has not begun at all because
	// maxConcurrent delegates are already working. Only one of the two is
	// anybody's to act on, which is why the tray must be able to tell them apart
	// rather than drawing both as a spinner.
	Running  bool   `json:"running"`
	Waiting  bool   `json:"waiting"`
	Queued   bool   `json:"queued"`
	Question string `json:"question,omitempty"`
	// OK is the outcome, meaningful only once Running is false.
	OK bool `json:"ok"`
	// Stopped: the user ended this one. It is not OK and it is not a failure
	// either, and the tray and the auto-collect both have to tell the
	// difference โ€” work somebody stopped on purpose must not come back as a
	// report the model is asked to go and read.
	Stopped bool `json:"stopped"`
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
		// Named apart from the `out` slice this loop is filling, which the
		// obvious names for these would have shadowed.
		tokIn, tokOut, tokCached, tokReported := t.tokenSplit()
		info := TaskInfo{
			ID: t.id, Profile: t.profile, Label: t.label,
			Model: t.model, Run: t.run, Phase: t.phase,
			Started: t.startedAt(), ToolCalls: t.calls(), Tokens: t.tokens(),
			TokensIn: tokIn, TokensOut: tokOut, CachedIn: tokCached, CacheReported: tokReported,
			Running: !t.finished(),
			// Read before the running check rather than inside the finished
			// branch: a delegate is marked stopped the instant the user asks
			// for it and stays Running until its goroutine unwinds, so a row
			// that only learned this once it had ended would keep drawing a
			// live spinner over work that is already on its way out.
			Stopped: t.wasStopped(),
		}
		if info.Running {
			info.Queued = t.isQueued()
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
// This is the Stop button reaching the work, now that a turn ending does not
// (see start). Stop and StopRun below narrow the same act to one delegate and
// to one declared job. The host calls all three; nothing in this package does,
// because "the user pressed Stop" is not a fact a tool can observe.
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
		stop(t)
	}
	return len(stopping)
}

// StopQueued ends every delegation that has not started yet, and leaves the ones
// already working alone. It reports how many it found.
//
// The brake the queue needs, and the reason there is no ceiling above it. With
// nothing refusing a fan-out, a model that has asked for two hundred jobs leaves
// a line two hundred long, and a line you can only cancel one row at a time is
// not one anybody can actually stop — the user would be clicking while the queue
// drains into the bill. One press, the whole line, and the four in flight keep
// going: work already begun has been paid for, and throwing it away is a
// different decision (StopAll) taken with a different button.
func (r *Delegations) StopQueued() int {
	r.mu.Lock()
	stopping := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		if t.isQueued() && !t.finished() {
			stopping = append(stopping, t)
		}
	}
	r.mu.Unlock()

	for _, t := range stopping {
		stop(t)
	}
	return len(stopping)
}

// Stop ends one delegation and reports whether there was anything to end.
//
// StopAll was the whole vocabulary for a long time, and that made the brake
// coarser than the work it brakes: delegates run concurrently, so "one of the
// four is looping and the other three are fine" is an ordinary state, and the
// only answer to it was to throw all four away. Naming one is not a second
// mechanism, it is the same cancel reached by id.
//
// False means no such delegation, or one that had already finished. Neither is
// an error: the tray polls, so the row a person clicks can finish between the
// paint and the click, and "it is already over" is the outcome they were after.
func (r *Delegations) Stop(id string) bool {
	r.mu.Lock()
	task, ok := r.tasks[id]
	r.mu.Unlock()
	if !ok || task.finished() {
		return false
	}
	stop(task)
	return true
}

// StopRun ends every delegation inside one declared job, and reports how many.
// A run is one piece of work in the user's head however many workers it spread
// across, so stopping it a worker at a time, while the phases still running
// start more, would be a brake the job outruns.
func (r *Delegations) StopRun(runID string) int {
	r.mu.Lock()
	stopping := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		if t.run == runID && !t.finished() {
			stopping = append(stopping, t)
		}
	}
	r.mu.Unlock()

	for _, t := range stopping {
		stop(t)
	}
	return len(stopping)
}

// stop is the one place a delegate is ended, so the mark and the cancel cannot
// drift apart. Marked first, deliberately: cancel can be noticed and the
// goroutine finished before this returns, and a snapshot taken in that window
// would draw work the user just stopped as work that failed on its own.
func stop(t *runningTask) {
	t.markStopped()
	t.cancel()
}
