package subagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/cognitive"
	"github.com/Mikedev115/Aetox/internal/command"
	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/learned"
	"github.com/Mikedev115/Aetox/internal/mode"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/think"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// The `task` tool: the one way a sub-agent ever runs (ARCHITECTURE.md §44.4).
//
// It cannot live in internal/skill — turn imports skill, and this needs turn and
// cognitive — so the host registers it at bootstrap, the same way the desktop
// injects ask_user/todo_write. Everything it needs from the session arrives in
// TaskOptions; it holds no global state.
//
// One call does NOT wait: it picks a profile, builds the child's registry from
// it, starts a full turn in the background and returns a handle. `task_result`
// redeems the handle later. See runner.go for why that shape and not another.

// defaultProfile is what `task` spawns when the model names nothing: the
// read-only searcher, because that is both the cheapest delegate and the one a
// model reaches for most.
const defaultProfile = "explore"

// TaskOptions is everything the host has to lend a delegate. Fields the session
// owns (provider, permissions, the live registry) are passed rather than looked
// up, so a re-bootstrap replaces the tool along with everything else instead of
// leaving it pointed at a dead engine.
type TaskOptions struct {
	Provider model.Provider
	Model    string
	Registry *skill.Registry // the session's registry; the child gets a filtered copy
	// Desk is the mode this session was opened at, and it decides two things:
	// the ceiling a delegate runs under, and which chairs this desk may hand a
	// job to (COMPANY.md §3). Nil is the pre-modes full desk — no ceiling, every
	// profile reachable — which is what every session before §83 had.
	//
	// The registry above stays whole on purpose. The desk filter lives in the
	// session's dispatcher, so what the *model* is offered is already trimmed;
	// keeping the registry complete is what lets a cross-desk dispatch hand a
	// chair a tool its caller cannot see — the file crosses the counter, the
	// tool never does (§84).
	Desk *mode.Mode
	// NoAgents and NoHelpers are this session's reach, one switch per kind
	// (COMPANY.md §4). They are the whole of it: both false is the full reach,
	// both true builds no tool at all, and either one alone builds a tool that
	// carries one roster and describes one act.
	//
	// Two switches because they are two acts. An เอเจน is a colleague who takes
	// a whole job off your hands and can be talked to directly by the user; a
	// ซับเอเจน is your own hands in a second context, so a step of YOUR work
	// stays out of THIS context. They were one switch until 2026-08-20, and the
	// fusion leaked everywhere it touched — the tool introduced itself to the
	// model as "Sub-agents" while offering colleagues, a failed call with nobody
	// resolved was counted as a ซับเอเจน on screen, and one switch on the เอเจน
	// settings page silently greyed out every ซับเอเจน on the other page. Owner's
	// call that day: แยกชัดเจน, หลังบ้านถึงหน้าบ้าน.
	//
	// The two do NOT ship the same way, and that asymmetry lives in
	// config.Config rather than here: handing a whole job to a colleague is off
	// until asked for, while the assistant's own hands are on from the start
	// (owner, 20 ส.ค.). Nothing in this package encodes that — a library that
	// decided a product's defaults would be deciding them for every other caller
	// too.
	//
	// Negative, so the zero value is the full reach: a library that does nothing
	// until you opt in is a library that gets called wrong. The one translation
	// between the product's defaults and these lives in internal/bootstrap, at
	// the boundary, where it can be read.
	NoAgents bool
	// NoHelpers is NoAgents' twin. A chair chat sets NoAgents rather than this
	// one: a chair is itself a colleague, so handing a whole job to another one
	// is two peers arguing about whose job it was, one level below the person
	// who asked — while hands are exactly what it runs short of. See
	// delegationTool.forChair.
	NoHelpers bool
	// WorkersOff names individual workers the assistant may not hand work to,
	// either kind. Named by worker rather than split per kind because a name is
	// unique across both homes (subagent.Conflicts refuses a collision), so the
	// kind is always derivable and a second list would be a second place to get
	// it wrong.
	//
	// They are NOT disabled. The user can still open a chat with one and still
	// write @name at it, because that is the user's own act. What this narrows
	// is the assistant's reach, and the copy on the switch has to say so — "ปิด
	// doc" would tell somebody their agent is gone when it is standing right
	// there.
	WorkersOff   []string
	Permissions  safety.PermissionConfig
	ApprovalMode safety.ApprovalMode
	Approve      turn.ApprovalPromptFunc
	// OnToolAction is the parent's live tool feed. Every event a delegate causes
	// is stamped with the `task` call's own id before it goes down this channel,
	// so the UI can tell whose work it is.
	OnToolAction func(turn.ToolEvent)
	// OnToolRun is the parent's run recorder. Same stamping as OnToolAction,
	// plus the delegate's profile name — "which sub-agent is bad at what" is the
	// question the stored record exists to answer, and it needs both.
	OnToolRun func(turn.ToolRun)
	// OnUsage is the parent's usage reporter — a delegate's tokens are the user's
	// tokens, so they land in the same stats with no extra plumbing.
	OnUsage    func(model.Usage)
	MaxChars   int
	ThinkLevel think.Level
	// Proposer is the approval door a delegate's `memory` tool writes to. Nil
	// means delegates get no memory tool at all rather than a broken one — a
	// front end with no store (the CLI) should not hand out a tool whose whole
	// promise is that something survives the session.
	Proposer learned.Proposer
	// BuildPrompt assembles a delegate's system prompt the way the session's own
	// was assembled — identity, environment, the machine's rules, the user's own
	// layers, this project's rules — with the worker's brief as its direction.
	//
	// It exists because the two doors onto the same worker had drifted apart and
	// nobody had decided they should: a direct chat mounted 15.1k of prompt, a
	// delegated run mounted 4.4k of brief and nothing else, so the same agent did
	// not know where a bare filename lands depending on who asked it. Owner's
	// call, 2026-08-08: one door. Standards are unenforceable across two.
	//
	// A function rather than the surface and the scope, because there must be one
	// assembler (internal/prompt) with one caller (internal/bootstrap). Handing
	// this package the pieces would make it a second place that knows how a
	// prompt is put together, which is the shape the drift grew in.
	//
	// Nil mounts the brief alone. That is for a host with no prompt of its own —
	// and for the tests, which assert on the brief rather than on the frame
	// around it.
	BuildPrompt func(direction string) string
	// Delegations is the register the three tools share. Passed in rather than
	// made here so the host can keep a handle on it — a delegate now outlives
	// the turn that started it (runner.go), which leaves Stop as the one thing
	// that ends one early, and Stop is a fact only the host has.
	//
	// Nil builds a private one, which is the right answer for a host with no
	// Stop button to wire: the CLI, whose Ctrl+C takes the whole process, and
	// every test.
	Delegations *Delegations
	// EnsureServers brings up the MCP servers a worker needs, for the ones the
	// startup connect deliberately skipped: a server no desk carries waits for
	// the agent that does (mcp.Server.Deferred).
	//
	// Called here because this is where such an agent starts, so the connect is
	// paid for by the job that wanted it instead of by every launch. Nil means
	// the host connects everything up front, which is what every host did before
	// deferral existed and stays correct.
	EnsureServers func(context.Context, []string) []error
}

type taskTool struct {
	opts   TaskOptions
	runner *Delegations
}

// Dispatcher is the `task` tool seen by a host that wants to run a worker
// itself — the door a user's own `@name` goes through (see mention.go).
//
// An interface rather than an exported type because the host already holds the
// tool: it is in the session's registry under "task", and the host asks whether
// that registration can also be spoken to directly. Nothing new is constructed,
// so a dispatch from the composer and a dispatch from the model are the same
// machinery by construction rather than by agreement.
type Dispatcher interface {
	Direct(ctx context.Context, agent, brief string) (Reply, error)
	Answer(ctx context.Context, taskID, answer string) (Reply, error)
}

// Reply is one exchange with a worker, and Pending is the whole reason it is a
// struct: a worker that stopped to ask something is not finished, and the id of
// the run it stopped in the middle of is the only thing that can restart it
// where it left off.
//
// Without it the host has a question on screen and nothing to answer it with —
// the user types their answer, a fresh run starts, and everything the worker had
// already read and decided is thrown away. That was the state this door shipped
// in for an hour, and it made `ask_main` worse than useless: a worker that asked
// a good question cost more than one that guessed.
type Reply struct {
	Output skill.Output
	// Pending is the task waiting on an answer, or "" when the work is done.
	Pending string
	// Agent is who this was, for a host that wants to say so on screen.
	Agent string
}

// NewTaskTools builds delegation: one tool named `task`, four actions inside it
// (packed_task.go), all sharing one runner because they are four parts of one
// mechanism. Register it into the same registry passed in opts.Registry;
// FilterRegistry drops it from every child, so depth stays 1 structurally rather
// than by a counter — and packed, that is now one name to drop instead of four.
//
// Still a slice: the host registers what it is handed without knowing how many
// there are, and a signature that changed every time this family did would make
// packing a change to bootstrap.
func NewTaskTools(opts TaskOptions) []skill.Skill {
	// Nothing, rather than a tool that refuses. A `task` that exists and says no
	// would still cost its 710 tokens in every request to say it, which is the
	// opposite of what the switches are for.
	//
	// Both, because either one alone still leaves an act to perform. Off is not
	// all-or-nothing any more: with one kind switched off the tool is built with
	// that roster missing, which is cheaper than the whole tool — 629 tokens for
	// เอเจน alone, 599 for ซับเอเจน alone, against 710 for the pair (measured
	// 2026-08-20).
	if opts.NoAgents && opts.NoHelpers {
		return nil
	}
	shared := opts.Delegations
	if shared == nil {
		shared = NewDelegations()
	}
	return []skill.Skill{&delegationTool{
		start:   &taskTool{opts: opts, runner: shared},
		collect: &taskResultTool{runner: shared},
		answer:  &taskAnswerTool{runner: shared},
		plan:    &taskPlanTool{runner: shared},
	}}
}

func (t *taskTool) Name() string { return "task" }

// available is the roster this desk may actually start: its own delegates,
// plus the chairs at any desk it is allowed to hand work to. A profile listed
// but refused would spend a round of the loop teaching the model a rule the
// list could have expressed.
// reach answers the one question the roster and a dispatch both ask: may this
// session hand work to this worker, and if not, why not.
//
// One function because it was nearly two, and the two would not have agreed.
// available() hid a switched-off worker from the roster while begin() resolved
// the name with Load() and ran it — so the desk ceiling was a real gate and the
// user's switch was a suggestion, in the same list, looking identical. Somebody
// would eventually have relied on the wrong one.
//
// It returns the ceiling because begin needs it. Getting the answer and the
// thing the answer implies from the same call is what keeps them one question.
func (t *taskTool) reach(p Profile) (*mode.Mode, error) {
	if p.Invalid != "" {
		// A sick file is the settings page's to explain, never a roster entry.
		return nil, fmt.Errorf("the profile for %q cannot be read: %s", p.Name, p.Invalid)
	}
	if t.switchedOff(p.Name) {
		return nil, fmt.Errorf("%s is switched off for the assistant. The worker is not disabled — open a chat with it, or write @%s — but handing it work from here is turned off in settings", p.Name, p.Name)
	}
	// One check, and the roster, the enum, the guidance and this dispatch all
	// read it: available() is the only way a profile reaches any of them.
	//
	// The two refusals are written apart because they are refusing two different
	// things, and the model's next move differs. Turned away from a colleague it
	// should say so to the person — the work really does belong with that
	// worker, and the person is the one who can send it there. Turned away from
	// a helper there is nobody to name: the step is its own to take, here.
	if p.Desk != "" && t.opts.NoAgents {
		return nil, fmt.Errorf("%s is an AGENT (เอเจน) — a colleague with a desk of its own — and this session does not hand whole jobs to one. Tell the person you are talking to that it belongs with %s. What you can hand out is a step of your own work, to a helper (ซับเอเจน)", p.Name, p.Name)
	}
	if p.Desk == "" && t.opts.NoHelpers {
		return nil, fmt.Errorf("%s is a HELPER (ซับเอเจน) — your own hands in a second context — and this session does not use them. Do the step here, in this conversation", p.Name)
	}
	return t.ceilingFor(p)
}

// switchedOff reports whether the user has taken this worker out of the
// assistant's reach. Case and stray spaces are forgiven: this list can be edited
// by hand in a config file.
func (t *taskTool) switchedOff(name string) bool {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, off := range t.opts.WorkersOff {
		if strings.ToLower(strings.TrimSpace(off)) == want {
			return true
		}
	}
	return false
}

func (t *taskTool) available() []Profile {
	all := List()
	out := make([]Profile, 0, len(all))
	for _, p := range all {
		if _, err := t.reach(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// ceilingFor answers which desk's manifest a job on p runs under, or why it
// cannot run from here at all.
//
// Three cases and one rule: work may cross desks only when it compresses to a
// single brief and comes back as a file (§84).
//
//   - An ordinary delegate (no `desk:`) is the caller doing its own work in a
//     second context, so it runs under the caller's ceiling.
//   - A chair at the caller's own desk is the same thing by another name.
//   - A chair at another desk is a dispatch: allowed only if this desk declares
//     that one in `dispatch:`, and then it runs on the *target* desk's
//     manifest, in its own context, with only the result crossing back.
func (t *taskTool) ceilingFor(p Profile) (*mode.Mode, error) {
	here := t.opts.Desk
	if p.Desk == "" || p.Desk == here.DeskName() {
		return here, nil
	}
	if !here.AllowsDispatch(p.Desk) {
		return nil, fmt.Errorf(
			"%s works at the %s desk, and this desk does not hand work to that one. Do it here, or tell the user which kind of session this belongs in",
			p.Name, p.Desk)
	}
	target, ok := mode.Load(p.Desk)
	if !ok {
		return nil, fmt.Errorf("%s says it works at the %q desk, and no such desk exists", p.Name, p.Desk)
	}
	return target, nil
}

func profileNames(profiles []Profile) []string {
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Name)
	}
	return out
}

// agentChoice describes the `agent` parameter, grouped into the two kinds the
// product actually has (COMPANY.md §4: เอเจน and ซับเอเจน).
//
// One flat list of names taught the model there was one kind of worker, so it
// told users so — "ซับเอเจน 6 ตัว", with an agent's job described as a chair's.
// It was not wrong; it was repeating what this schema said. The grouping is
// derived from the resolver's answer (Desk), never from a word in a profile's
// description — descriptions say what the job *is*, and the kind is decided by
// which home the file lives in.
//
// The Thai terms ride along because this list is the only place the model
// meets these workers, and a model that has to invent a word for them will.
//
// What this deliberately does NOT say is what an agent hands back. It used to
// promise "a finished file", and that one clause decided the answer before the
// user's request had even been read: told that agents return files, the caller
// had to write every brief as an order to produce one, so "ask doc how a good
// document is put together" left here as "write a manual about…" and came back
// as a manual nobody wanted. The agent's own rule against exactly that — "the
// mistake to avoid is answering a question with a file", in doc's AGENT.md —
// never got to apply, because by the time the brief arrived there was no
// question left in it.
//
// Owner's call, 12 ส.ค.: take it out rather than reword it. A specialist has
// its own instructions and its own tools and already knows what its work looks
// like; a caller told the return type in advance is a second answer to a
// question that has one, which is a debt in the system. What the caller needs
// from this list is who these people are — and §84's "returns as a file" is
// satisfied by any compressed result, a paragraph included, since its whole
// argument was about not shipping context between rooms.
// agentChoice is the roster as the tool block carries it: every worker, with
// enough of what it is FOR to be chosen between.
//
// It was 673 tokens — 43% of the whole `task` entry, more than the entire
// browser tool — because it carried each worker's full description plus two
// paragraphs explaining what the two kinds are. Owner's rule for the trim,
// 18 ส.ค.: *"ทำให้มันฉลาดเลือก ไม่ใช่เดาจากชื่อ"*. So the line is drawn at
// CHOOSING and not at naming. A bare enum of names would be cheaper still and
// would make every first delegation a guess, which is the saving nobody wanted.
//
// What makes the cut possible without losing the choice is that the profiles
// already mark the split themselves. A description reads "เอเจนดูแลงานเอกสาร —
// ตอบว่าเอกสารแบบไหนต้องมีอะไร …": before the dash is what this worker is FOR,
// after it is how it works. Existence and judgment, separated by whoever wrote
// the file. The block takes the first half; agentRoster hands the whole thing
// over with the first `start`.
//
// The @name sentence stays, short. One address for everybody (owner, 12 ส.ค.):
// the user writes `@doc …` and it reaches doc unedited, and the model reads that
// in the transcript and has to recognise it as the thing it does itself. Two
// names for one act is how a user gets told their own convention does not exist.
func agentChoice(profiles []Profile) string {
	var agents, helpers []string
	for _, p := range profiles {
		agents, helpers = appendKind(agents, helpers, p, ForClause(p.Description))
	}
	out := "Which worker. The user writes @name for the same thing."
	if len(agents) > 0 {
		out += "\nAGENTS (เอเจน), a colleague who takes a whole job: " + strings.Join(agents, " | ")
	}
	if len(helpers) > 0 {
		out += "\nHELPERS (ซับเอเจน), your own hands for one step of YOUR work: " + strings.Join(helpers, " | ")
	}
	return out
}

// agentRoster is that same list with every word of it, for the guidance sent
// with the first `start` of a session.
func agentRoster(profiles []Profile) string {
	var agents, helpers []string
	for _, p := range profiles {
		agents, helpers = appendKind(agents, helpers, p, p.Description)
	}
	out := "Who you can hand work to, in full."
	if len(agents) > 0 {
		out += "\nAGENTS (เอเจน) — a colleague who takes a whole job off your hands. Brief them like a coworker and use what comes back:\n  " +
			strings.Join(agents, "\n  ")
	}
	if len(helpers) > 0 {
		out += "\nHELPERS (ซับเอเจน) — your own hands for one step of YOUR work, in a second context so it stays out of this one:\n  " +
			strings.Join(helpers, "\n  ")
	}
	return out
}

// appendKind files one worker under its kind. Which home the profile lives in
// decides that, never a word inside its description — see homes_test.go.
func appendKind(agents, helpers []string, p Profile, text string) ([]string, []string) {
	line := p.Name + " — " + text
	if p.Desk != "" {
		return append(agents, line), helpers
	}
	return agents, append(helpers, line)
}

// ForClause is the half of a profile's description that says what the worker is
// FOR, which is the half needed to choose one. Profiles separate it with an em
// dash by convention; one that does not is carried whole rather than guessed at.
//
// Exported because the settings page shows the same half for the same reason,
// and two functions splitting one string the same way is how they stop doing it
// the same way.
func ForClause(description string) string {
	if before, _, found := strings.Cut(description, " — "); found {
		return strings.TrimSpace(before)
	}
	return strings.TrimSpace(description)
}
func (t *taskTool) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	return t.begin(ctx, args, nil)
}

// Direct runs one worker and waits for it, for a caller that is not the model:
// the user named this worker themselves, so there is no round of tool-calling to
// have, and nothing to gain from handing a handle back to someone who is only
// going to redeem it.
//
// The brief is the user's own text, unedited. That is the whole point of the
// door — the paraphrase step is what turned "ask doc how a good document is put
// together" into "write a manual about…", and a step that does not run cannot
// mistranslate. Everything else is the model's dispatch exactly: same profile,
// same registry cut, same ceiling, same prompt, same `ask_main`, so a worker
// cannot tell which door the job came through and there is no second behaviour
// to keep in step.
//
// A worker that stops to ask comes back as its question with Pending set; hand
// the user's next message to Answer and it carries on from where it stopped.
func (t *taskTool) Direct(ctx context.Context, agent, brief string) (Reply, error) {
	var task *runningTask
	out, err := t.begin(ctx, map[string]any{
		"agent": agent, "prompt": brief, "description": agent,
	}, &task)
	if err != nil || task == nil {
		return Reply{Output: out, Agent: agent}, err // a refusal, already shaped as a failed result
	}
	return t.redeem(ctx, task.id, agent, task.startedAt())
}

// Answer gives a waiting worker the decision it stopped for, and waits again.
//
// Through the runner, which is where `task_answer` puts it too — the worker is
// blocked on one channel and there is exactly one way to unblock it, whether the
// answer came from the model or from the person who asked the question.
func (t *taskTool) Answer(ctx context.Context, taskID, answer string) (Reply, error) {
	started := time.Now()
	if err := t.runner.answer(taskID, answer); err != nil {
		out, _ := t.fail(taskID, started, err.Error())
		return Reply{Output: out}, nil
	}
	return t.redeem(ctx, taskID, "", started)
}

// redeem waits on a run and shapes what came back for a person to read.
//
// One function for both doors in, because "what does an answer look like" must
// not have two answers: a finished run loses its receipt, and an unfinished one
// says what it is waiting for and that the next message is the way to say it.
func (t *taskTool) redeem(ctx context.Context, id, agent string, started time.Time) (Reply, error) {
	collected, ask, err := t.runner.collect(ctx, id)
	if err != nil {
		out, _ := t.fail(agent, started, err.Error())
		return Reply{Output: out, Agent: agent}, nil
	}
	if agent == "" {
		agent = collected.profile
	}
	if ask != nil {
		// Success, not failure: nothing went wrong. A red row for "your worker
		// needs a decision" tells the user the wrong story — the same reasoning
		// task_result.question is built on.
		question := strings.TrimSpace(ask.question) + "\n\n— " + agent + " กำลังรออยู่ ตอบในข้อความถัดไปได้เลย"
		return Reply{
			Output: skill.Output{
				Name: "task_result", Command: "@" + agent,
				Content: question, RawOutput: question, Success: true,
				DurationMs: time.Since(started).Milliseconds(),
			},
			Pending: id, Agent: agent,
		}, nil
	}
	// The receipt comes off. It is an argument aimed at a model that delegates
	// too eagerly ("that was one tool call, do work this size yourself"), and
	// nobody chose to delegate here — the user named the worker. Read by a
	// person, under an answer they asked for, it is a machine talking to itself.
	out := collected.output
	out.Content = withoutReceipt(out.Content)
	out.RawOutput = withoutReceipt(out.RawOutput)
	return Reply{Output: out, Agent: agent}, nil
}

// begin is the whole of a dispatch up to the moment it is running. `out`, when
// non-nil, receives the started task — the one thing a caller that intends to
// wait needs and the model's caller has no use for. An out-parameter rather
// than a third return value because every refusal below is a two-value
// `t.fail`, and rewriting eight of them to carry a nil task would be eight
// chances to change the wrong one.
func (t *taskTool) begin(ctx context.Context, args map[string]any, out **runningTask) (skill.Output, error) {
	started := time.Now()
	name := strings.TrimSpace(stringArg(args, "agent"))
	if name == "" {
		name = defaultProfile
	}
	brief := strings.TrimSpace(stringArg(args, "prompt"))
	label := strings.TrimSpace(stringArg(args, "description"))
	if label == "" {
		label = name
	}
	// Which declared job this belongs to, if any. A phase that was never declared
	// is refused rather than created: a run whose phases can grow while it runs
	// records what happened instead of promising what will, and the promise is
	// the only thing this mechanism adds (run.go).
	//
	// A missing phase is NOT refused. It means a loose delegate, which is what
	// every delegation was before runs existed and what the user's own `@doc`
	// still is — Direct comes through this same door and has no run to name.
	var runID, phase string
	if phase = strings.TrimSpace(stringArg(args, "phase")); phase != "" {
		run := t.runner.currentRun()
		if run == nil {
			return t.fail(label, started, fmt.Sprintf(
				"there is no run to put %q in — declare one with task(action=plan) first, or leave the phase out and this runs on its own", phase))
		}
		if !run.hasPhase(phase) {
			return t.fail(label, started, fmt.Sprintf(
				"the run %q has no phase called %q. It declared: %s. Use one of those, or leave the phase out",
				run.name, phase, strings.Join(run.phaseTitles(), ", ")))
		}
		runID = run.id
	}

	if brief == "" {
		return t.fail(label, started, "the prompt is empty — a sub-agent sees no conversation history, so the brief has to carry the whole job")
	}
	profile, ok := Load(name)
	if !ok {
		return t.fail(label, started, fmt.Sprintf("no sub-agent named %q; available: %s", name, strings.Join(profileNames(t.available()), ", ")))
	}
	// Every worker switched off, master switch still on. A config file can say
	// that even if the settings page will not let you, and a `task` with an
	// empty roster is a dead end the model would otherwise walk into and get a
	// list of nobody back.
	if len(t.available()) == 0 {
		return t.fail(label, started, "every worker this session can reach is switched off. Turn one back on in settings, or switch both kinds off so this tool stops being offered at all")
	}
	// Which desk this job runs at, decided before anything is built: a dispatch
	// this desk may not make is a refusal the model can read and act on, not a
	// job that starts and quietly comes back with half its tools missing.
	// reach, not ceilingFor: the desk's ceiling and the user's switch are the
	// same question asked of the same worker, and asking only half of it here is
	// what made one of them enforceable and the other decorative.
	ceiling, err := t.reach(profile)
	if err != nil {
		return t.fail(label, started, err.Error())
	}
	if t.opts.Provider == nil || t.opts.Registry == nil {
		return t.fail(label, started, "the engine is not ready to spawn a sub-agent")
	}
	// A brief the child cannot hold is not simply a smaller job. memory trims the
	// last message from its tail (memory.Context.truncateLastIfNeeded), so an
	// oversize brief arrives cut off at an arbitrary point and the delegate works
	// from half of it — confidently, because nothing tells it the rest existed.
	// Refuse with the numbers instead, so the parent can split the work.
	// Its own instructions plus whatever it has learned: both are in its system
	// prompt on every round, so both count against what a brief can be.
	childPrompt := PromptFor(profile)
	if t.opts.BuildPrompt != nil {
		childPrompt = t.opts.BuildPrompt(childPrompt)
	}
	// Measured against the assembled prompt, not the brief alone: the frame is
	// what the delegate actually carries on every round, so a budget checked
	// without it was checking the wrong number.
	if budget := t.opts.MaxChars; budget > 0 && len(brief)+len(childPrompt) > budget {
		return t.fail(label, started, fmt.Sprintf(
			"the brief is too long for sub-agent %s: %d characters on top of its %d-character instructions goes past the %d it can hold, and the end would be silently cut off. Split it into smaller jobs.",
			profile.Name, len(brief), len(childPrompt), budget))
	}

	defer debuglog.Block("task: " + profile.Name + " — " + truncate(label, 60))()

	// Before the registry is cut: a server placed on this agent alone was not
	// connected at startup, so its tools are not in the parent yet and the cut
	// would take a copy without them. Failures are not fatal here — the
	// missingAgentServers check below is what reports them, in the words the
	// caller can act on, and it does that for a server that failed to connect
	// exactly as it does for one that has not finished.
	if t.opts.EnsureServers != nil {
		if servers := config.MCPServersForAgent(profile.Name); len(servers) > 0 {
			for _, err := range t.opts.EnsureServers(ctx, servers) {
				debuglog.Msg("task %s: %v", profile.Name, err)
			}
		}
	}
	// The child's tool set is its profile's, under the ceiling of the desk the
	// job runs at, minus what every delegate is refused — `task` above all,
	// which is what keeps depth at 1.
	childRegistry := FilterRegistry(t.opts.Registry, profile, ceiling)
	if len(childRegistry.Names()) == 0 {
		return t.fail(label, started, fmt.Sprintf("sub-agent %q was left with no tools at all", profile.Name))
	}
	// MCP servers register into the parent in the background, so a dispatch in
	// the seconds after a launch or a settings change can copy a snapshot taken
	// before the connect landed. The agent would then accept the brief, work
	// without the tool it was given for the job, and hand back something that
	// looks like an answer — the failure mode that costs the most to diagnose,
	// because it only happens sometimes and leaves no trace.
	//
	// Refuse instead, naming the server and saying it is a retry. The model can
	// read that and either wait or tell the user, and the same job succeeds a
	// moment later.
	//
	// Marked FromWorld, because it says so itself: a connection still being made
	// is the machine's current state, and the sentence even ends by asking for a
	// retry. Unmarked, the same wait queued a problem card naming the server, and
	// a card about a connection that landed a second later teaches nobody.
	if missing := missingAgentServers(t.opts.Registry, profile); len(missing) > 0 {
		out, err := t.fail(label, started, fmt.Sprintf(
			"the MCP server(s) %s that %s works with have not finished connecting yet — start this job again in a moment",
			strings.Join(missing, ", "), profile.Name))
		out.FromWorld = true
		return out, err
	}

	childModel := t.opts.Model
	if profile.Model != "" {
		childModel = profile.Model
	}

	// Everything below the goroutine boundary is built here, on the calling
	// goroutine, so a configuration mistake is reported as a failed tool call
	// rather than surfacing minutes later out of a background run.
	parentRef := turn.CallID(ctx)
	child := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     t.opts.Provider,
		Model:        childModel,
		SystemPrompt: childPrompt,
		MaxChars:     t.opts.MaxChars,
		MaxToolCalls: profile.MaxToolCalls(),
	})
	// A delegate inherits the session's prohibitions and adds its own; it never
	// inherits a permission the session has that its profile does not.
	permissions := safety.PermissionConfig{
		Rules: append(append([]safety.PermissionRule{}, t.opts.Permissions.Rules...), profile.DenyRules()...),
	}

	task := t.runner.start(delegation{
		profile: profile.Name, label: label, model: childModel, run: runID, phase: phase,
	}, func(runCtx context.Context, self *runningTask) skill.Output {
		defer debuglog.Block("task: " + profile.Name + " — " + truncate(label, 60))()

		// The delegate's tokens go where they always went — the parent's reporter,
		// so the session's stats absorb them untouched — and are stamped with this
		// delegation on the way past. Set here rather than with the rest of the
		// child's configuration because the stamp needs `self`, which does not
		// exist until the register has admitted the job.
		child.SetUsageReporter(func(u model.Usage) {
			self.spend(u)
			if t.opts.OnUsage != nil {
				t.opts.OnUsage(u)
			}
		})

		// `ask_main` is bound to this delegation and cannot exist before it, so it
		// is the one tool registered inside the goroutine rather than with the
		// rest. Every profile gets it, allowlist or not — see ask.go.
		if regErr := childRegistry.Register(&askMainTool{task: self}, skill.SourceBuiltin); regErr != nil {
			debuglog.Msg("ask_main unavailable to %s: %v", profile.Name, regErr)
		}

		// `memory` is rebuilt per delegate rather than inherited (FilterRegistry
		// drops the parent's). The parent's instance is bound to the main agent's
		// scope, and a delegate holding it would write what it learned into the
		// prompt every other agent pays for — the exact leak the scope split
		// exists to prevent. Constructing it here with the profile's own name is
		// what makes the boundary structural: there is no scope a child could
		// name but its own.
		if t.opts.Proposer != nil && profile.AllowsTool("memory") {
			scoped := &learned.MemoryTool{Scope: profile.Name, Proposer: t.opts.Proposer}
			if regErr := childRegistry.Register(scoped, skill.SourceBuiltin); regErr != nil {
				debuglog.Msg("memory unavailable to %s: %v", profile.Name, regErr)
			}
		}

		// Every event the delegate causes is stamped with the `task` call's id, so
		// the UI can nest it instead of mixing it into the main agent's timeline.
		relay := func(ev turn.ToolEvent) {
			if ev.Action == "call" {
				self.countCall()
			}
			if t.opts.OnToolAction != nil {
				ev.Parent = parentRef
				// And which delegation, in the register's own namespace — the
				// tray joins these rows to their task on it (§105).
				ev.Task = self.id
				ev.Agent = profile.Name
				t.opts.OnToolAction(ev)
			}
		}
		relayRun := func(run turn.ToolRun) {
			if t.opts.OnToolRun == nil {
				return
			}
			run.Parent = parentRef
			run.Agent = profile.Name
			t.opts.OnToolRun(run)
		}
		exec := turn.NewExecutor(turn.ExecutorOptions{
			Agent:        child,
			Dispatcher:   skill.NewDispatcher(childRegistry),
			Approve:      t.opts.Approve,
			ApprovalMode: t.opts.ApprovalMode,
			Permissions:  permissions,
			OnToolAction: relay,
			OnToolRun:    relayRun,
			TurnOptions:  turn.TurnOptions{ThinkLevel: t.opts.ThinkLevel},
		})

		// An explicit Intent is load-bearing: without one the executor parses the
		// brief, and a brief that happens to start with a tool name ("read every
		// test file and…") would run as a single explicit tool call, not a turn.
		result, runErr := exec.Execute(runCtx, brief, command.Intent{Raw: brief, Kind: command.KindConversation}, nil, nil, nil)
		elapsed := time.Since(self.startedAt())

		// Cancellation is checked BEFORE runErr and independently of it: a stopped
		// turn can come back with runErr == nil carrying the empty-reply fallback,
		// and calling that a successful delegation is a lie the user cannot see —
		// they pressed Stop.
		if runCtx.Err() != nil {
			return failure(self.id, label, elapsed, "sub-agent stopped: "+runCtx.Err().Error())
		}
		if runErr != nil {
			return failure(self.id, label, elapsed, "sub-agent failed: "+runErr.Error())
		}

		reply := strings.TrimSpace(result.Reply)
		if reply == "" {
			reply = "(the sub-agent returned nothing)"
		}
		// A loop that ends without the delegate choosing to stop is not a result.
		// Both of cognitive's endings are replies rather than errors (the user has to
		// see them), and both are useless to a parent model as written — one is an
		// internal sentence, the other is Thai prose addressed to a human. Recognised
		// by exported sentinel, not by matching the words, and turned into the thing
		// the parent can actually do next.
		switch {
		case strings.Contains(reply, cognitive.ToolLoopExhausted):
			return failure(self.id, label, elapsed, fmt.Sprintf(
				"sub-agent %s ran out of room: it used all %d of its tool-call steps after %d calls and did not finish. "+
					"Split the work into smaller batches and start it again, or raise `steps:` in its profile.",
				profile.Name, profile.MaxToolCalls(), self.calls()))
		case strings.HasPrefix(reply, cognitive.DoomLoopStopPrefix):
			return failure(self.id, label, elapsed, fmt.Sprintf(
				"sub-agent %s was stopped after repeating the same tool call with no progress (%d calls). "+
					"Its brief was probably too vague to act on — say concretely what to look at and what the answer must contain, then start it again.",
				profile.Name, self.calls()))
		}
		// What the parent model sees: the result, plus one line of receipt. NOT the
		// delegate's tool log — that would put back the context cost delegating just
		// saved (§44.6). The user sees every step live in the UI instead.
		content := reply + "\n" + receiptFor(profile.Name, self.calls(), elapsed)
		return skill.Output{
			Name:       "task_result",
			Command:    "task " + profile.Name,
			Content:    content,
			RawOutput:  content,
			Success:    true,
			DurationMs: elapsed.Milliseconds(),
		}
	})
	if out != nil {
		*out = task
	}

	// Started or queued, said plainly. The model does the same thing either way
	// — go and do something else, then collect — but "it is running now" about a
	// delegate that has not begun is a small lie the model will reason from, and
	// the honest version is also the one that explains a collect that takes
	// longer than it expected.
	began := fmt.Sprintf("started sub-agent %s as %s — it is running now.", profile.Name, task.id)
	if task.isQueued() {
		began = fmt.Sprintf("queued sub-agent %s as %s — %d sub-agents are already running, so this one begins the moment a slot frees. Nothing is lost and you need do nothing.",
			profile.Name, task.id, maxConcurrent)
	}
	started_ := began + fmt.Sprintf(" Do other work, then call task(action=collect, task_id=%q) to collect it.", task.id)
	return skill.Output{
		Name:       t.Name(),
		Command:    "task " + profile.Name,
		Content:    started_,
		RawOutput:  started_,
		Success:    true,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

// failure is the shape a background run reports a refusal in: a failed result the
// collector hands to the model, never a Go error nobody is left to receive.
func failure(id, label string, elapsed time.Duration, reason string) skill.Output {
	return skill.Output{
		Name:       "task_result",
		Command:    "task " + label,
		Content:    reason,
		RawOutput:  reason,
		Stderr:     reason,
		Success:    false,
		DurationMs: elapsed.Milliseconds(),
	}
}

// receiptFor is the one line the parent model gets about how the delegation went.
//
// When the delegate did almost nothing, the receipt says so. Judging *afterwards*
// is the honest way round: how big a job turns out to be is not knowable from the
// brief, so a pre-flight heuristic would refuse real work and wave through
// pointless work. A model that reads "you could have done that here" mid-turn
// stops doing it for the rest of the conversation, which is the behaviour the
// description alone cannot enforce.
//
// ponytail: the threshold is tool calls, not seconds — one slow grep is still one
// call, and wall-clock says more about the disk than about the work. Revisit if a
// one-call delegate ever turns out to be worth it.
// withoutReceipt drops the trailing line receiptFor added, and only that line:
// matched by the shape receiptFor writes rather than by position, so a worker
// whose own answer happens to end in brackets keeps it.
func withoutReceipt(content string) string {
	at := strings.LastIndex(content, "\n")
	if at < 0 || !strings.HasPrefix(content[at+1:], "[task ") {
		return content
	}
	return strings.TrimRight(content[:at], "\n")
}

func receiptFor(name string, toolCalls int, elapsed time.Duration) string {
	receipt := fmt.Sprintf("[task %s: %d tool calls, %.1fs]", name, toolCalls, elapsed.Seconds())
	if toolCalls <= 1 {
		receipt += " NOTE: that was one tool call — small enough to have done here, and delegating it cost a whole second context. Do work this size yourself."
	}
	return receipt
}

// fail reports a refusal to the model as a normal unsuccessful tool result: it
// can read the reason and try something else, which is more useful than an error
// that reaches the user as a crash.
func (t *taskTool) fail(label string, started time.Time, reason string) (skill.Output, error) {
	return skill.Output{
		Name:       t.Name(),
		Command:    "task " + label,
		Content:    reason,
		RawOutput:  reason,
		Stderr:     reason,
		Success:    false,
		DurationMs: time.Since(started).Milliseconds(),
	}, nil
}

func stringArg(args map[string]any, key string) string {
	if raw, ok := args[key].(string); ok {
		return raw
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
