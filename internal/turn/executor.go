package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mikedev115/Aetox/internal/command"
	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/hook"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/rtk"
	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/statereport"
	"github.com/Mikedev115/Aetox/internal/think"
)

type TurnStatus string

const (
	TurnStatusDone    TurnStatus = "done"
	TurnStatusError   TurnStatus = "error"
	TurnStatusBlocked TurnStatus = "blocked"

	defaultToolSummaryTimeout = 30 * time.Second

	// The floor under OutputBackstop, and the whole of it when nothing knows
	// this model's context window.
	//
	// It is 4096 because that is what this cap has always been, and that number
	// was not arbitrary: memory.defaultMaxChars is 128000, and 128000/32 is
	// 4000. It was one thirty-second of the history budget, correctly, back when
	// the budget was one fixed number. Then the budget started scaling with the
	// model (bootstrap.ContextChars) and this did not, so on a 1M-token model it
	// had quietly become one *thousandth* of the room available — and 14.2% of
	// 3,335 recorded tool runs on the owner's machine were arriving at the model
	// cut in half with no way to ask for the rest.
	defaultOutputBackstop = 4096

	// outputBackstopFraction keeps the original relationship: a single tool
	// result may take at most this fraction of the history it has to share.
	outputBackstopFraction = 32
)

// A single tool call that runs longer than this hands the turn back to the
// model instead of hanging it. The call is NOT abandoned — it keeps running and
// the model is told to call the same tool with the same arguments to look in on
// it (see dispatchWithDeadline). Applies to tool execution only, not
// conversation. Var (not const) so tests can shrink it.
var toolExecutionTimeout = 60 * time.Second

// noDeadlineTools are exempt from the slow-tool guard above, for two different
// reasons that both end the same way: waiting IS the work.
//
//   - ask_user blocks on a human answering.
//   - task starts a nested agent loop, and task_result waits for one. A delegate
//     doing real work takes minutes; its own step cap (internal/subagent) is what
//     bounds it, and by the time the parent collects, the waiting is time the
//     model chose to spend rather than time it was forced to.
//
// Ctx cancel — Ctrl+C in the CLI, the Stop button in the desktop — remains the
// brake for both, and it propagates into a sub-agent's loop unchanged.
var noDeadlineTools = map[string]bool{"ask_user": true, "task": true, "task_result": true}

// HasNoDeadline reports whether the named tool is exempt from the per-tool
// deadline. Exported so the tools that depend on the exemption can assert it
// without sleeping a minute to find out (internal/subagent).
func HasNoDeadline(name string) bool {
	return noDeadlineTools[strings.ToLower(strings.TrimSpace(name))]
}

// maxToolExecutionTimeout bounds what a tool call may ask for. Ten minutes is
// long enough for a full test suite or a cold build and short enough that a
// command which will never finish still ends the turn rather than hanging it.
const maxToolExecutionTimeout = 10 * time.Minute

// toolCallDeadline is the per-call deadline, which only two tools can move,
// and both for the same reason: the call itself is the only thing that knows
// how long its work is about to take. A 60 second cap is right for a tool that
// reads a file, wrong for `shell` running `go test ./...`, and wrong again for
// `shell_output` that was asked to wait_for a server's listening line — a wait
// the turn cuts short at 60s degrades back into the polling loop it exists to
// replace. Anything absent, unparseable, negative or over the ceiling falls
// back to the default rather than failing the call — a bad timeout is not
// worth refusing to run over.
//
// JSON numbers decode to float64, so an integer schema still arrives as one.
func toolCallDeadline(name string, args map[string]any) time.Duration {
	var key string
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "shell":
		key = "timeout_seconds"
	case "shell_output":
		// Only a read that waits gets to stretch the turn. A plain read
		// returns at once, and a wait_timeout without a wait_for is noise.
		if target, _ := args["wait_for"].(string); strings.TrimSpace(target) == "" {
			return toolExecutionTimeout
		}
		key = "wait_timeout_seconds"
	default:
		return toolExecutionTimeout
	}
	var seconds float64
	switch v := args[key].(type) {
	case float64:
		seconds = v
	case int:
		seconds = float64(v)
	default:
		return toolExecutionTimeout
	}
	if seconds <= 0 {
		return toolExecutionTimeout
	}
	if d := time.Duration(seconds) * time.Second; d > toolExecutionTimeout {
		if d > maxToolExecutionTimeout {
			return maxToolExecutionTimeout
		}
		return d
	}
	// A request to shorten the default is honored too — a model that expects a
	// command to be instant has told us something useful about when to give up.
	return time.Duration(seconds) * time.Second
}

type Agent interface {
	Respond(context.Context, string, TurnOptions) (string, error)
	// RespondEphemeral answers a one-shot prompt without writing anything into
	// conversation history — for meta work like tool-run summaries.
	RespondEphemeral(context.Context, string, TurnOptions) (string, error)
	RespondStream(context.Context, string, func(string) error, func(string) error, TurnOptions) (string, bool, error)
	// The tool callback returns images alongside its text so a tool can hand
	// back a picture instead of a description of one — today only `read` on an
	// image file. Nil for every other tool and every other call.
	RespondWithTools(context.Context, []model.ToolDefinition, string, func(context.Context, model.ToolCall) (string, []model.Image, error), func(string) error, TurnOptions) (string, bool, error)
	SupportsToolCalling() bool
}

type TurnOptions struct {
	ThinkLevel think.Level
	// Images ride with this turn's user message and this turn's only — they are
	// set per Execute call, never on the executor, so an attachment cannot leak
	// into the next question. Empty unless the caller both had an image and
	// established that the model can see one (model.ResolveVision).
	Images []model.Image
	// Documents ride with the same message under the same rule, set only when
	// model.ResolveDocuments says this endpoint takes a file part.
	Documents []model.Document
	// OnRound, if set, hears about each completed round of the tool loop — the
	// text the model wrote alongside that round's tool calls, and whether the
	// round ended the turn. The executor uses it to interleave narration and
	// thinking into the live timeline (§59). Carried here rather than as one
	// more RespondWithTools parameter so the Agent interface, and every fake
	// implementing it, stays untouched.
	OnRound func(RoundEvent)
	// OnContent, if set, receives the model's answer text as it is generated,
	// for a live preview only. It is NOT the delivery channel: the reply still
	// arrives exactly once, through onChunk, when the turn ends.
	//
	// The distinction is the whole design. A tool loop writes text every round
	// and only the last round's text is the answer — the others are narration,
	// a draft the loop is about to nudge away, or a finished reply demoted to
	// narration because the user typed under it. A preview can be thrown away;
	// a delivery cannot. So this streams freely and OnContentReset erases it
	// the instant the round turns out not to be the answer.
	OnContent func(chunk string)
	// OnContentReset discards whatever OnContent has shown so far. Called after
	// every provider call whose text is not going to be surfaced as the reply,
	// and once more immediately before the reply is delivered — so the preview
	// is always empty when the real answer lands, and it cannot be doubled.
	OnContentReset func()
}

// RoundEvent is one completed round of the model⇄tool loop.
type RoundEvent struct {
	// Text is the round's assistant text — narration when the round also called
	// tools, the reply itself when Final.
	Text string
	// Final marks the round that ended the turn. Its Text is the reply the
	// caller will surface, so the UI must not also show it as narration.
	Final bool
	// Demoted marks a round the model meant to end the turn with, kept alive by
	// something the user typed while it was writing (cognitive.Agent's
	// interjection drain). Its Text is a finished answer, not narration — the
	// only reason it is not Final is that the conversation moved past it, and
	// treating the two the same sent a whole markdown reply down the channel
	// built for a one-line "reading the config first".
	Demoted bool
}

type Dispatcher interface {
	Execute(context.Context, string) (skill.Output, bool, error)
	ToolDefinitions() []model.ToolDefinition
	ExecuteTool(context.Context, string, map[string]any) (skill.Output, bool, error)
}

type ApprovalPromptFunc func(context.Context, string, string) (bool, error)

type Executor struct {
	agent      Agent
	dispatcher Dispatcher
	commandSet map[string]struct{}
	approve    ApprovalPromptFunc
	// approvalMode is live rather than a snapshot: the desktop's Settings
	// dropdown can change it while a turn is running, and a user who just
	// clicked "full access" *because* the prompts were in the way means this
	// prompt too. It used to be a plain field fixed at construction, and
	// switching mid-turn changed nothing until the next one — the engine was
	// rebuilt around the executor the running turn still held.
	approvalMode   atomic.Pointer[safety.ApprovalMode]
	permissions    safety.PermissionConfig
	summaryTimeout time.Duration
	summaryLimit   int
	turnOptions    TurnOptions
	statusReporter func(string)
	onToolAction   func(ToolEvent)
	onToolRun      func(ToolRun)
	delegateKind   func(string) string
	hooks          *hook.Runner

	// pending holds tool calls that outran their deadline and were left running.
	pendingMu sync.Mutex
	pending   map[string]*pendingCall
}

type ExecutorOptions struct {
	Agent          Agent
	Dispatcher     Dispatcher
	CommandSet     map[string]struct{}
	Approve        ApprovalPromptFunc
	ApprovalMode   safety.ApprovalMode
	Permissions    safety.PermissionConfig
	SummaryTimeout time.Duration
	SummaryLimit   int
	TurnOptions    TurnOptions
	StatusReporter func(string)
	OnToolAction   func(ToolEvent)
	// OnToolRun, if set, receives one record per finished tool call — the whole
	// call, not the timeline's summary of it (see ToolRun). Nil records nothing,
	// which is what the CLI does.
	OnToolRun func(ToolRun)
	// DelegateKind, if set, names which pile a `task` call's worker belongs to —
	// subagent.KindAgent or subagent.KindHelper — given the profile name the
	// model passed (empty means the default profile). Injected by the host
	// because the profile registry lives in internal/subagent, which imports
	// this package. Nil leaves AgentKind empty and the UI counts the delegation
	// in the helper pile, which is every delegation's home before the split.
	DelegateKind func(agent string) string
	// Hooks are the user's own commands run around every tool call. Nil is the
	// normal case — almost nobody configures one — and *hook.Runner is written
	// so that nil does nothing rather than needing a guard at each call site.
	Hooks *hook.Runner
}

type Result struct {
	Reply string
	// Parts is the turn as it actually happened — prose, thinking segments and
	// tool calls in order (see part.go). Reply is the concatenation of its text
	// parts, kept so every caller that predates this keeps working; new readers
	// should draw Parts, because Reply alone cannot say where the work went.
	//
	// Nil for the paths that produce a single answer and no sequence: an
	// explicit skill command, and the plain conversational turn.
	Parts    []TurnPart
	Streamed bool
	Status   TurnStatus
}

func NewExecutor(opts ExecutorOptions) *Executor {
	timeout := opts.SummaryTimeout
	if timeout <= 0 {
		timeout = defaultToolSummaryTimeout
	}
	limit := opts.SummaryLimit
	if limit <= 0 {
		// Asked of the agent rather than passed in by each caller. There are
		// four places that build an executor, and a number that has to be
		// remembered at four call sites is a number that will be forgotten at
		// one — it was, within an hour of this being written, and the result
		// was every tool result still cut at the floor while the code that was
		// supposed to raise it sat there looking correct.
		limit = OutputBackstop(historyCharsOf(opts.Agent))
	}
	mode := opts.ApprovalMode
	if mode == "" {
		mode = safety.ApprovalAsk
	}
	e := &Executor{
		agent:          opts.Agent,
		dispatcher:     opts.Dispatcher,
		commandSet:     opts.CommandSet,
		approve:        opts.Approve,
		permissions:    opts.Permissions,
		summaryTimeout: timeout,
		summaryLimit:   limit,
		turnOptions:    opts.TurnOptions,
		statusReporter: opts.StatusReporter,
		onToolAction:   opts.OnToolAction,
		onToolRun:      opts.OnToolRun,
		delegateKind:   opts.DelegateKind,
		hooks:          opts.Hooks,
	}
	e.SetApprovalMode(mode)
	return e
}

// SetApprovalMode swaps the mode a running executor enforces. Safe from another
// goroutine, which is the point: the desktop's Settings dropdown is on the UI
// thread and the turn it has to affect is already running on another one.
func (e *Executor) SetApprovalMode(mode safety.ApprovalMode) {
	if mode == "" {
		mode = safety.ApprovalAsk
	}
	e.approvalMode.Store(&mode)
}

func (e *Executor) currentApprovalMode() safety.ApprovalMode {
	if mode := e.approvalMode.Load(); mode != nil {
		return *mode
	}
	return safety.ApprovalAsk
}

func (e *Executor) reportStatus(msg string) {
	if e.statusReporter != nil {
		e.statusReporter(msg)
	}
}

func (e *Executor) stopSpinner() {
	if e.statusReporter != nil {
		e.statusReporter("")
	}
}

// ToolEvent is one entry in a UI's live tool timeline. It replaced a pair of
// formatted strings ("write {\"path\":\"internal/skil", "write: สำเร็จ"): the
// frontend had to parse a *localized* Thai word to tell success from failure,
// and anything the UI wanted to show beyond the name had nowhere to travel.
// New per-call facts belong here as fields, not as more text to re-parse.
type ToolEvent struct {
	Action string `json:"action"` // "call" | "result" | "note" | "thinking" | "said"
	Name   string `json:"name"`
	// Ref is the provider's tool-call id, and it is what the UI keys a timeline
	// row on. The label used to serve as identity, which forced the streaming
	// path to stay silent until it could name the call — a model that streams a
	// write's content before its path then showed nothing at all for the whole
	// file. With a stable ref the label is free to fill itself in later. Empty
	// for providers that send no id; the UI falls back to matching on the
	// label, as before.
	Ref string `json:"ref,omitempty"`
	// Parent is the Ref of the tool call that caused this one — set only on
	// events from inside a sub-agent, where it carries the id of the `task` call
	// that spawned it. Empty for everything the main agent does itself. Without
	// it a delegate's tool calls arrive on the same channel as the main agent's
	// and are indistinguishable from them (ARCHITECTURE.md §44.5).
	Parent string `json:"parent,omitempty"`
	// Subject is the one argument worth reading in a list: the path a write
	// touches, the URL a fetch opens. Empty when the tool takes nothing nameable.
	Subject string `json:"subject,omitempty"`
	// Act is the action *inside* a packed tool — `browser`'s open/read/click,
	// `task`'s start/collect. Packing (§99) put a dozen capabilities behind one
	// name, and Name has said "browser" for every one of them ever since: a UI
	// holding a call could tell that the browser was busy and not one thing more.
	//
	// Written as the generic fact rather than as a browser field, because the
	// question "which action of the packed tool is this" is asked by every pack
	// and answered the same way in all of them. Empty for a tool that is not
	// packed, and for arguments that will not parse.
	Act string `json:"act,omitempty"`
	// Tab is the browser tab this call is working, and it is the one field here
	// the executor does not fill in: `turn` has never heard of a browser and
	// should not start now. The host stamps it on the way past
	// (desktop/app.go recordToolAction), because the host is the side that owns
	// the tabs.
	//
	// It exists for ไฟบอกสถานะ (§174): "the agent is working" and "the agent is
	// working *here*" are different sentences, and a panel that can only say the
	// first has to light all five tab chips to say it. Empty for every tool that
	// is not the browser, and for a browser call made before any tab exists.
	Tab string `json:"tab,omitempty"`
	// Git is the touched file's one-letter git state (M/A/U/D/R), host-stamped
	// on file-tool results the same way Tab is stamped on browser events —
	// `turn` has never heard of git either. Empty for a clean tracked file, a
	// session outside a repository, and every tool that touches no file: the
	// badge exists to mark the noteworthy, and clean is the ground state.
	Git   string `json:"git,omitempty"`
	OK    bool   `json:"ok"`              // result only
	Error string `json:"error,omitempty"` // result only, when !OK
	// Added/Removed are the line counts of a write or edit, zero elsewhere.
	Added   int `json:"added,omitempty"`
	Removed int `json:"removed,omitempty"`
	// Count/Range are the reading tools' readout (skill.Output.ResultCount and
	// ResultRange): how much came back in the tool's own unit, and for read the
	// 1-based line span it actually returned — the difference between a row
	// saying "read gate.py" and one saying "read gate.py 1-60 of it". Result
	// events only; zero/empty where the tool has no honest number.
	Count int    `json:"count,omitempty"`
	Range string `json:"range,omitempty"`
	// Problems is the after-edit self-check's number (skill.Output.Problems):
	// how many errors the language server sees in a file this call changed.
	// The row wears it as a red "!N" — the one mark that must not wait to be
	// discovered inside the folded result.
	Problems int `json:"problems,omitempty"`
	// Diff is what those counts are counting: git-style unified hunks for the
	// change this call made (internal/skill/hunk.go), empty on every tool that
	// writes no file. The โค้ด desk draws it under the row, folded shut.
	//
	// It rides the result event rather than being fetched later on demand,
	// because "later" is a different file: the next call in the same turn may
	// touch these lines again, and a row expanded tomorrow must still show what
	// THIS call did. Asking git at expand time would answer a question nobody
	// asked — the working tree's state, with the whole turn mixed together.
	Diff string `json:"diff,omitempty"`
	// Artifacts carries skill.Output.Artifacts through to the UI: finished
	// files this call made for the user, which the chat shows as cards with an
	// open button instead of leaving them to be hunted for in the file tree.
	// Empty for every tool whose output is text.
	Artifacts []string `json:"artifacts,omitempty"`
	// ProposalID carries skill.Output.ProposalID the same way: the queued change
	// a `memory` call is waiting on a decision for, which the chat draws under
	// the answer as the proposal itself, with the yes and the no on it. Zero for
	// every other tool.
	ProposalID int64 `json:"proposalId,omitempty"`
	// Text is the model's own words on a "note" event — narration it wrote
	// alongside a round's tool calls, which used to go into context and nowhere
	// else — and on a "said" event, where it is a whole answer an interjection
	// re-placed. Empty on every other action.
	Text string `json:"text,omitempty"`
	// Secs is how long a "thinking" segment streamed (whole seconds, min 1).
	// Set only on "thinking" events.
	Secs int `json:"secs,omitempty"`
	// Agent and Brief describe a delegation, and are set only on the `task` call
	// that opens one. They are what lets the UI show a sub-agent as a sub-agent —
	// titled with who is doing the work and what it was asked — instead of one
	// more row reading "task". Without them the delegate's name lives only in
	// prose inside the tool result, which is not a place a UI can read.
	Agent string `json:"agent,omitempty"`
	Brief string `json:"brief,omitempty"`
	// Task is the delegation's own id ("task_1"), stamped on every event from
	// inside a sub-agent. Parent already says which `task` CALL caused it, but
	// that is the provider's tool-call id — a different namespace from the id
	// the register hands out, so a UI holding both could not tell that they
	// describe the same delegation. The tray needs exactly that join: rows from
	// the event stream, state from the register (§105).
	Task string `json:"task,omitempty"`
	// AgentKind says which pile that worker belongs to — "agent" (เอเจน, a
	// chair with a desk) or "helper" (ซับเอเจน). The UI counts the two
	// apart, and it cannot decide this itself: the answer lives in which home
	// the profile file sits, which only the resolver the host injects can see
	// (ExecutorOptions.DelegateKind). Empty when no resolver is wired or the
	// profile cannot run — counted as a helper, the pre-split reading.
	AgentKind string `json:"agentKind,omitempty"`
	// Delegation says whether this row hired anybody.
	//
	// It has to travel as its own fact because the label cannot carry it:
	// delegation is packed under one tool name (§99), so `task collect` — the
	// agent sitting and waiting on a delegate it started earlier — reads
	// exactly like the delegation it is waiting for. delegationOf has always
	// known the difference; it just had nowhere to put the answer, and the UI
	// re-derived it from the label and drew the waiting next to the working as
	// two sub-agents.
	//
	// A pointer, because there are three answers and not two. nil is "nobody
	// has said", which is the honest state of a row born from the streaming
	// announcement — that fires while the model is still writing the
	// arguments, and the action is in the arguments. Only a caller holding
	// them ever fills this in.
	Delegation *bool `json:"delegation,omitempty"`
}

// Label is what a timeline row reads, e.g. "write internal/skill/edit.go".
func (ev ToolEvent) Label() string {
	return strings.TrimSpace(ev.Name + " " + ev.Subject)
}

// ToolRun is one finished tool call, recorded whole.
//
// It exists because ToolEvent cannot be widened into this without cost:
// ToolEvent is drawn on screen and crosses the Wails bridge on every call, so
// it deliberately keeps one Subject and throws the arguments away. A run
// record is written to disk and read later by something asking "what did this
// tool actually get, and what actually came back" — a question the timeline
// never asks and the learning pass can ask nothing else.
//
// Output is the tool's own output *before* the receipt is built for the model:
// the receipt is shaped for a reader (trimmed, rtk-filtered, wrapped in
// status), and a record of what the tool returned should not be a record of
// how we chose to phrase it.
type ToolRun struct {
	// Ref is the provider's tool-call id, matching ToolEvent.Ref.
	Ref string
	// Parent is the `task` call that caused this run — stamped by the
	// delegation relay, empty for the main agent's own calls.
	Parent string
	// Agent names the sub-agent profile that ran it; empty means the main agent.
	Agent string
	Name  string
	// Args is the raw arguments JSON exactly as the model sent it, unparsed:
	// a malformed call is itself a thing worth having recorded.
	Args   string
	Output string
	OK     bool
	Error  string
	// ErrorKind says where Error came from, because the text cannot. Empty for
	// an error this codebase wrote; ErrorFromProgram when a program the tool ran
	// returned nonzero. See classifyToolError.
	ErrorKind string
	Duration  time.Duration
}

// ErrorFromProgram marks a failure that is not the tool's: the tool started a
// program, the program ran to completion, and it exited nonzero.
//
// That is routinely the correct outcome of a tool working perfectly — a test
// suite reporting failures, a grep finding nothing (exit 1), a timeout firing
// (124). It is recorded as a tool error because the model must see it, and the
// two readings were indistinguishable for as long as the only thing carried
// forward was the string: "exit status 1" is Go's spelling of an exit code, not
// a sentence anybody here wrote to be read.
//
// The learning floor is where the difference bites. Its reader assumes every
// failure it clusters carries its own remedy — true of this codebase's
// refusals, which are written that way on purpose, and false of an exit code,
// which carries nothing. Without this mark the summarizer cannot tell them
// apart, and proposes "avoid whatever hits exit status 1" as a lesson (found
// 2026-08-09: five of the six clusters over the threshold on a real machine
// were exit codes, the sixth was a real refusal).
const ErrorFromProgram = "exit"

// ErrorFromWorld marks a failure that is a report about the machine's current
// state, not about anyone's behaviour: the server was not running, the key had
// expired, the network was out. Same reader, same bite as ErrorFromProgram —
// the summarizer must not turn "n8n was down tonight" into a permanent memory
// line teaching the agent to avoid n8n (three such cards reached the approval
// queue on 2026-08-12). Unlike an exit status, nothing about the error's type
// can reveal this — only its author knows — so the author says it, by writing
// the error through internal/statereport.
const ErrorFromWorld = "state"

// ErrorFromCancel marks a tool that died because the turn ended — Stop
// pressed, almost always. It is nobody's failure at all: not the tool's, not
// a program's, not the machine's, and it carries exactly nothing to learn.
//
// The problems page is where the absence bit (2026-08-25): one Stop killed
// three parallel web_fetch calls in the same second, and the summarizer —
// which reads every unmarked failure as a sentence with a remedy in it —
// raised "เครื่องมือ web_fetch ล้มซ้ำด้วยเหตุเดียวกัน: context canceled" as a
// problem worth reporting to a developer. Every card in that queue spends the
// user's attention; a card about their own Stop spends it on nothing.
const ErrorFromCancel = "canceled"

// classifyToolError asks the error what it is rather than reading its text.
// Two cases answer definitively: an author's own statereport mark (checked
// first among the authored kinds — an explicit statement outranks inference),
// and *exec.ExitError, which exists only when a process was started and
// returned a status. Everything else stays unmarked, which keeps the default
// the conservative one: unmarked errors are still read as lessons.
func classifyToolError(err error) string {
	// Before everything, because it outranks everything: whatever the tool
	// was doing when the turn was stopped, the stop is why it ended, and no
	// mark an author put on the error changes that. errors.Is first, per this
	// function's own rule — with a suffix fallback because not every tool
	// wraps ctx.Err() with %w, and Go's one spelling of the sentinel is what
	// such a tool flattens into its string ("...: context canceled").
	if err != nil && (errors.Is(err, context.Canceled) ||
		strings.HasSuffix(err.Error(), context.Canceled.Error())) {
		return ErrorFromCancel
	}
	if statereport.Is(err) {
		return ErrorFromWorld
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return ErrorFromProgram
	}
	// A *fs.PathError is the filesystem answering, never a sentence anyone here
	// wrote: the file was not there, the permission was refused, the path was
	// not a directory. That is ErrorFromWorld by the definition above — a report
	// about the machine's current state — and it arrives from every file tool at
	// once, which is why it is recognised here rather than marked at each os.Stat.
	//
	// It cannot swallow a refusal. The sandbox denylist and every other rule this
	// codebase enforces are built with errors.New/fmt.Errorf and stay unmarked,
	// so "you asked for a path inside a credential store" is still offered as a
	// lesson while "that file is not there" no longer is.
	//
	// Found 2026-08-18 on the problems page: a sub-agent asked to produce a file
	// it had no tool to write, then read it back, and three GetFileAttributesEx
	// failures were queued as if the agent had broken a rule with a remedy to
	// quote. The remedy a missing file carries is nothing.
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return ErrorFromWorld
	}
	return ""
}

// classifyToolOutcome is classifyToolError plus the case it cannot see: a tool
// that reported failure through Success:false and a nil error.
//
// The error value is asked first and its answer is final, so a tool that both
// returns a marked error and sets the flag cannot end up with the two disagreeing.
func classifyToolOutcome(out skill.Output, err error) string {
	if kind := classifyToolError(err); kind != "" {
		return kind
	}
	if err == nil && !out.Success && out.FromWorld {
		return ErrorFromWorld
	}
	return ""
}

func (e *Executor) reportToolCall(ref, name, args string) {
	if e.onToolAction != nil {
		agent, brief, isTask := delegationOf(name, args)
		e.onToolAction(ToolEvent{
			Action: "call", Ref: ref, Name: name, Subject: toolCallSubject(args),
			Act:   packedActionOf(args),
			Agent: agent, Brief: brief, AgentKind: e.kindOf(isTask, agent),
			Delegation: &isTask,
			// The REQUESTED range, on the call event: a read's offset/limit are
			// in the arguments, so "40-60" can sit on the row the whole time it
			// runs instead of appearing only when the result closes it. The
			// result then overwrites with what actually came back, which may be
			// shorter — the file ended, or the page cap bit.
			Range: requestedReadRange(name, args),
		})
	}
}

// requestedReadRange derives the line span a read call is asking for from its
// own arguments — "40-60" for {offset:40, limit:21}, "40-" when only the start
// is known, "" for any other tool or a whole-file read (a range of everything
// says nothing).
func requestedReadRange(name, args string) string {
	if !strings.EqualFold(strings.TrimSpace(name), "read") {
		return ""
	}
	var parsed struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	}
	if json.Unmarshal([]byte(args), &parsed) != nil {
		return ""
	}
	first := parsed.Offset
	if first < 1 {
		if parsed.Limit <= 0 {
			return ""
		}
		first = 1
	}
	if parsed.Limit <= 0 {
		return fmt.Sprintf("%d-", first)
	}
	return fmt.Sprintf("%d-%d", first, first+parsed.Limit-1)
}

// kindOf asks the injected resolver which pile a delegation's worker is in.
// Only a `task` call has a kind — for everything else the resolver is not even
// consulted, so a tool that happens to take an `agent` argument cannot be
// mistaken for a delegation.
func (e *Executor) kindOf(isTask bool, agent string) string {
	if !isTask || e.delegateKind == nil {
		return ""
	}
	return e.delegateKind(agent)
}

// delegationOf reports which sub-agent a `task` call is handing work to, and the
// brief it is handing over. isTask distinguishes "not a delegation at all" from
// "a delegation whose agent was left to default" — the two used to look the
// same (an empty agent), and the kind stamp needs the difference.
//
// This is the one place `turn` names a specific tool, and it earns it: the
// alternative is a UI that reads a delegate's identity out of English prose in a
// tool result. Nothing is dispatched on the name — it decides a label, and a
// mismatch costs a plain "task" row rather than a broken turn. The default when
// `agent` is omitted is deliberately not repeated here; an unnamed delegate
// reports an empty agent and the UI falls back to the row's own label, which
// keeps the profile registry in one package.
func delegationOf(name, args string) (agent, brief string, isTask bool) {
	if !strings.EqualFold(strings.TrimSpace(name), "task") {
		return "", "", false
	}
	parsed, err := model.ParseToolArguments(args)
	if err != nil {
		return "", "", true
	}
	// Only STARTING one is a delegation. Since delegation was packed under this
	// single name (§99), collecting, answering and declaring a run arrive here as
	// `task` too — and a collect reported as a delegation is a card for a worker
	// that was never hired, drawn next to the real one it just redeemed.
	if action, _ := parsed["action"].(string); action != "" && !strings.EqualFold(strings.TrimSpace(action), "start") {
		return "", "", false
	}
	agent, _ = parsed["agent"].(string)
	brief, _ = parsed["prompt"].(string)
	return strings.TrimSpace(agent), strings.TrimSpace(brief), true
}

// toolCallSubject picks the one argument worth reading in a timeline row — the
// path a write touches, the URL a fetch opens. The raw JSON truncated at 40
// characters used to cut mid-key ("write {\"path\":\"internal/skil"), which is
// the least useful 40 characters available. Falls back to the old behaviour
// when the arguments are not JSON or carry nothing nameable.
func toolCallSubject(args string) string {
	parsed, err := model.ParseToolArguments(args)
	if err != nil {
		return truncate(args, 40)
	}
	// One definition, shared with the streaming path — see model.SubjectFromArgs
	// for why the two must not drift apart by even a truncation.
	return model.SubjectFromArgs(parsed)
}

// packedActionOf reads the `action` a packed tool was called with.
//
// One key, spelled once. Every pack in the product takes its sub-call under
// this name (internal/skill/packed.go builds the enum for all of them), so
// this reads the packing convention rather than any particular tool — which is
// why it lives here beside toolCallSubject and not next to the browser.
//
// Lower-cased, because the model sends what it likes and a UI comparing
// against "click" should not be defeated by "Click". Empty when the arguments
// will not parse or carry no action, which is the honest answer for an
// unpacked tool and for a call whose JSON arrived broken.
// unpackedName is the name a RECORD of this call should carry: the per-action
// permission name for a packed tool, the tool's own for everything else.
//
// The same answer skill.Unpack gives the gates (executor.go's approval and
// deadline paths), from the raw argument string a record has.
//
// It exists because tool_runs was keyed on what the MODEL called, and packing
// made that one word stand for several acts: every read, write, edit and delete
// through `change` would have been filed as "change", and the log this project
// makes its decisions from would have stopped being able to tell them apart.
// The ToolEvent beside it already carries Act for the live timeline; this is
// the same fact for the record that outlives the session.
func unpackedName(name, args string) string {
	parsed, err := model.ParseToolArguments(args)
	if err != nil {
		return name
	}
	return skill.Unpack(name, parsed)
}

func packedActionOf(args string) string {
	parsed, err := model.ParseToolArguments(args)
	if err != nil {
		return ""
	}
	action, _ := parsed["action"].(string)
	return strings.ToLower(strings.TrimSpace(action))
}

func (e *Executor) reportToolResult(ev ToolEvent) {
	if e.onToolAction != nil {
		ev.Action = "result"
		e.onToolAction(ev)
	}
}

func (e *Executor) reportToolRun(run ToolRun) {
	if e.onToolRun != nil {
		e.onToolRun(run)
	}
}

func (e *Executor) conversationThinkingStatus() string {
	if e.turnOptions.ThinkLevel == think.LevelNoThinking {
		return "กำลังประมวลผลคำตอบ..."
	}
	return "กำลังคิดคำตอบ..."
}

// ExecuteWithAttachments is Execute for a turn that carries attachments —
// pictures the model can look at, documents it can read, or both. Execute
// itself stays at six parameters: every caller but the desktop's chat path has
// nothing to attach, and two more nil arguments at each of them would be noise.
func (e *Executor) ExecuteWithAttachments(
	ctx context.Context,
	line string,
	intent command.Intent,
	onChunk func(string),
	onReasoningChunk func(string),
	onToolComplete func(),
	images []model.Image,
	documents []model.Document,
) (Result, error) {
	// A copy, not a write to e.turnOptions: the executor outlives the turn, and
	// a field set here would still be set on the next question.
	turnOptions := e.turnOptions
	turnOptions.Images = images
	turnOptions.Documents = documents
	return e.execute(ctx, line, intent, onChunk, onReasoningChunk, onToolComplete, turnOptions)
}

func (e *Executor) Execute(
	ctx context.Context,
	line string,
	intent command.Intent,
	onChunk func(string),
	onReasoningChunk func(string),
	onToolComplete func(),
) (Result, error) {
	return e.execute(ctx, line, intent, onChunk, onReasoningChunk, onToolComplete, e.turnOptions)
}

func (e *Executor) execute(
	ctx context.Context,
	line string,
	intent command.Intent,
	onChunk func(string),
	onReasoningChunk func(string),
	onToolComplete func(),
	turnOptions TurnOptions,
) (Result, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Result{}, errors.New("empty input")
	}

	defer debuglog.Block("Turn: " + truncate(line, 120))()

	e.reportStatus("กำลังวิเคราะห์คำขอ...")
	parsed := e.normalizeIntent(line, intent)
	debuglog.Info("parsed.Kind", kindName(parsed.Kind))
	debuglog.Info("parsed.Command", parsed.Command)
	debuglog.Info("parsed.IsSlash", fmt.Sprintf("%v", parsed.IsSlash))
	debuglog.Info("parsed.IsMeta", fmt.Sprintf("%v", parsed.IsMeta))

	// Explicit command (a slashed skill token, e.g. "/read foo.txt", "/time")
	// → direct dispatch. Everything else is the model's call — there is
	// deliberately no keyword/regex guessing between the user and the model
	// (ARCHITECTURE.md §17). The slash is what makes that true rather than
	// merely stated: until §201 a bare first word matching a skill name came
	// down this path too, which took whole sentences away from the model.
	if parsed.Kind == command.KindSkill {
		debuglog.Msg("path: executeSkillTurn (explicit skill command)")
		e.reportStatus("กำลังรันเครื่องมือ...")
		return e.executeSkillTurn(ctx, line, parsed, onToolComplete)
	}

	e.reportStatus(e.conversationThinkingStatus())
	agentCanUseTools := e.agent != nil && e.agent.SupportsToolCalling() &&
		e.dispatcher != nil && len(e.dispatcher.ToolDefinitions()) > 0
	if agentCanUseTools {
		debuglog.Msg("path: executeAgentToolLoop (model-driven tool calling)")
		if result, handled, err := e.executeAgentToolLoop(ctx, parsed, onChunk, onReasoningChunk, turnOptions); handled {
			return result, err
		}
	}

	debuglog.Msg("path: conversation (streaming chat)")
	// The live text goes to the preview channel; onChunk gets the finished
	// answer once, exactly as the tool loop above delivers it.
	//
	// This path used to hand onChunk the token stream, which broke the one
	// contract TurnOptions.OnContent documents ("a preview is NOT a delivery:
	// the reply arrives exactly once, through onChunk"). A front end is
	// entitled to read a delivery as the whole answer — the desktop's handler
	// *replaces* the bubble with whatever it is given, because on every other
	// path that is the finished reply. Fed one word at a time, the bubble
	// became each word in turn and finished holding whichever token happened to
	// be last: a multi-paragraph markdown answer rendered as a stray "12" out
	// of the middle of a table.
	//
	// It went unseen because only a model that cannot call tools reaches this
	// path. Every real provider takes the loop above, so the failure was
	// invisible except on Aetox's own test models — the surface §45 says is
	// supposed to be the one that catches things like this.
	reply, streamed, err := e.agent.RespondStream(ctx, parsed.Raw,
		asStreamHandler(turnOptions.OnContent), asStreamHandler(onReasoningChunk), turnOptions)
	if onChunk != nil {
		// Erase the preview first, for the reason the tool path does: whatever
		// it holds is the same answer arriving a moment earlier, and the
		// delivery below is the authority.
		if turnOptions.OnContentReset != nil {
			turnOptions.OnContentReset()
		}
		if strings.TrimSpace(reply) != "" {
			onChunk(reply)
		}
	}
	return Result{
		Reply:    reply,
		Streamed: streamed,
		Status:   TurnStatusDone,
	}, err
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func kindName(k command.Kind) string {
	switch k {
	case command.KindConversation:
		return "conversation"
	case command.KindSkill:
		return "skill"
	default:
		return fmt.Sprintf("unknown(%d)", k)
	}
}

func (e *Executor) executeSkillTurn(
	ctx context.Context,
	line string,
	intent command.Intent,
	onToolComplete func(),
) (Result, error) {
	notifyToolComplete := func() {
		if onToolComplete == nil {
			return
		}
		onToolComplete()
		onToolComplete = nil
	}

	toolCommand := strings.TrimSpace(strings.Join(append([]string{intent.Command}, intent.Args...), " "))
	if toolCommand == "" {
		toolCommand = strings.TrimSpace(intent.Raw)
	}

	assessment := safety.AssessCommand(intent.Command, intent.Args)
	approved, confirmErr := e.resolveApproval(ctx, intent.Command, intent.Args, toolCommand, assessment)
	if confirmErr != nil {
		notifyToolComplete()
		if errors.Is(confirmErr, context.Canceled) {
			cancelled := e.newToolResultForTurn("tool", toolCommand, "execution canceled during confirmation")
			summary, summarizeErr := e.summarizeToolExecution(ctx, line, cancelled, TurnStatusError, confirmErr)
			if summarizeErr != nil {
				return Result{
					Reply:    e.fallbackToolSummary(cancelled, TurnStatusError, confirmErr),
					Streamed: false,
					Status:   TurnStatusError,
				}, nil
			}
			return Result{
				Reply:    summary,
				Streamed: false,
				Status:   TurnStatusError,
			}, nil
		}
		return Result{}, confirmErr
	}
	if !approved {
		notifyToolComplete()
		blocked := e.newToolResultForTurn("tool", toolCommand, "execution blocked by user approval")
		summary, summarizeErr := e.summarizeToolExecution(ctx, line, blocked, TurnStatusBlocked, nil)
		if summarizeErr != nil {
			return Result{
				Reply:    e.fallbackToolSummary(blocked, TurnStatusBlocked, nil),
				Streamed: false,
				Status:   TurnStatusBlocked,
			}, nil
		}
		return Result{
			Reply:    summary,
			Streamed: false,
			Status:   TurnStatusBlocked,
		}, nil
	}

	reply, handled, err := e.dispatchBySkill(ctx, intent.Raw)
	if !handled {
		notifyToolComplete()
		replyText, respondErr := e.agent.Respond(ctx, line, e.turnOptions)
		if respondErr != nil {
			return Result{}, respondErr
		}
		return Result{
			Reply:    replyText,
			Streamed: false,
			Status:   TurnStatusDone,
		}, nil
	}

	if err != nil && errors.Is(err, context.Canceled) {
		reply = e.newToolResultForTurn("tool", toolCommand, "execution canceled")
	}

	notifyToolComplete()
	reply = e.normalizeToolResult(reply)

	executionStatus := TurnStatusDone
	if err != nil || !reply.Success || errors.Is(ctx.Err(), context.Canceled) {
		executionStatus = TurnStatusError
	}

	if shouldUseDeterministicToolSummary(intent.Command) {
		return Result{
			Reply:    e.fallbackToolSummary(reply, executionStatus, err),
			Streamed: false,
			Status:   executionStatus,
		}, nil
	}

	summary, summarizeErr := e.summarizeToolExecution(ctx, line, reply, executionStatus, err)
	if summarizeErr != nil {
		return Result{
			Reply:    e.fallbackToolSummary(reply, executionStatus, err),
			Streamed: false,
			Status:   executionStatus,
		}, nil
	}

	return Result{
		Reply:    summary,
		Streamed: false,
		Status:   executionStatus,
	}, nil
}

func (e *Executor) executeAgentToolLoop(
	ctx context.Context,
	intent command.Intent,
	onChunk func(string),
	onReasoningChunk func(string),
	turnOptions TurnOptions,
) (Result, bool, error) {
	if e.agent == nil || !e.agent.SupportsToolCalling() {
		return Result{}, false, nil
	}
	if e.dispatcher == nil {
		return Result{}, false, nil
	}

	toolDefs := e.dispatcher.ToolDefinitions()
	if len(toolDefs) == 0 {
		return Result{}, false, nil
	}

	debuglog.Info("sending tools", fmt.Sprintf("%d definitions", len(toolDefs)))
	for _, td := range toolDefs {
		debuglog.Msg("tool: %s", td.Function.Name)
	}

	// The turn is assembled as it happens, in order, instead of being reduced to
	// its last sentence. Narration, thinking segments and tool calls all land in
	// `parts` at the moment they occur — which is the whole reason the caller can
	// later draw the turn the way it actually went.
	parts := &partList{}

	// Interleave the loop's own story into the timeline (§59): a "thinking"
	// segment for each round that streamed reasoning, and a "note" for the
	// narration the model wrote alongside its tool calls — text that used to go
	// into context and nowhere else (measured 2026-07-28: 28% of tool rounds
	// carry it).
	opts := turnOptions
	reasoning := onReasoningChunk
	seg := &thinkSegments{}
	if onReasoningChunk != nil {
		reasoning = func(chunk string) {
			seg.observe()
			onReasoningChunk(chunk)
		}
	}
	// Always set now, not only when someone is listening to tool events: the
	// sequence is the turn's own record, and a caller that draws no live
	// timeline still wants it back.
	opts.OnRound = func(r RoundEvent) {
		secs, streamed := seg.flush()
		if streamed {
			parts.addThinking(secs)
			if e.onToolAction != nil {
				e.onToolAction(ToolEvent{Action: "thinking", Secs: secs})
			}
		}
		text := strings.TrimSpace(r.Text)
		if text == "" {
			return
		}
		// An answer an interjection re-placed is still an answer: it is written
		// down as one and handed over as one. Sending it along the "note" route
		// below is what the owner saw break — the preview was erased and the
		// same prose came back as a narration row, which is drawn as raw text
		// at --fs-xs in --text-muted. A reply with headings and a table lost its
		// markdown, its size and its colour in one step, and after the turn
		// ended it was behind the "used N tools" toggle, because a text part
		// that is not the last one is not the bubble's body.
		if r.Demoted {
			parts.addAnswer(text)
			if turnOptions.OnContentReset != nil {
				turnOptions.OnContentReset()
			}
			if e.onToolAction != nil {
				e.onToolAction(ToolEvent{Action: "said", Text: text})
			}
			return
		}
		// Both narration and the closing answer are text parts — the difference
		// between them was only ever "which one gets shown", and in a sequence
		// both do.
		parts.addText(text)
		if r.Final {
			return
		}
		// This round's prose is now in the record. The live preview hands it
		// over rather than showing the same sentence twice while the tools run.
		if turnOptions.OnContentReset != nil {
			turnOptions.OnContentReset()
		}
		if e.onToolAction != nil {
			e.onToolAction(ToolEvent{Action: "note", Text: text})
		}
	}

	reply, usedTools, err := e.agent.RespondWithTools(ctx, toolDefs, intent.Raw, func(ctx context.Context, call model.ToolCall) (string, []model.Image, error) {
		e.reportToolCall(call.ID, call.Function.Name, call.Function.Arguments)
		// A tool that spawns anything needs to know which call it is, so the work
		// it causes can be traced back to this row (`task` stamps ToolEvent.Parent
		// with it). Nothing else reads it, and a tool that ignores it is unaffected.
		ctx = WithCallID(ctx, call.ID)
		startedAt := time.Now()
		receipt, output, success, execErr := e.executeToolCallWithOutcome(ctx, call)
		elapsed := time.Since(startedAt)
		ev := ToolEvent{
			Ref:     call.ID,
			Name:    call.Function.Name,
			Subject: toolCallSubject(call.Function.Arguments),
			// Read from the arguments again rather than carried down from the
			// call event, because the two are built in different functions and a
			// result whose action disagreed with its call's would be worse than
			// one that had none: the UI matches the pair up and would draw the
			// close of an action it never saw open.
			Act:        packedActionOf(call.Function.Arguments),
			OK:         success,
			Added:      output.LinesAdded,
			Removed:    output.LinesRemoved,
			Count:      output.ResultCount,
			Range:      output.ResultRange,
			Problems:   output.Problems,
			Diff:       output.Diff,
			Artifacts:  output.Artifacts,
			ProposalID: output.ProposalID,
		}
		if !success {
			if execErr != nil {
				ev.Error = execErr.Error()
			} else {
				ev.Error = failureReason(output)
			}
		}
		e.reportToolResult(ev)
		e.reportToolRun(ToolRun{
			Ref: call.ID,
			// The act, not the packed name it was called by: a record read back
			// months later has to be able to say whether it was a write or a
			// delete. See unpackedName.
			Name: unpackedName(call.Function.Name, call.Function.Arguments),
			Args: call.Function.Arguments,
			// Classified from the error value, which only exists here — by the
			// time ev.Error is a string the answer is unrecoverable.
			Output:    toolRunOutput(output),
			OK:        success,
			Error:     ev.Error,
			ErrorKind: classifyToolOutcome(output, execErr),
			Duration:  elapsed,
		})
		// The work goes into the sequence where it happened — between the
		// narration that announced it and whatever the model said next.
		// Agent/Brief/AgentKind are written down too: the live event named the
		// delegate, but a reopened session reads only the parts, and a `task`
		// row that forgot who it hired drew a generic "sub-agent" block.
		agent, brief, isTask := delegationOf(call.Function.Name, call.Function.Arguments)
		parts.addTool(ToolPart{
			Ref:        call.ID,
			Name:       call.Function.Name,
			Subject:    ev.Subject,
			Agent:      agent,
			Brief:      brief,
			AgentKind:  e.kindOf(isTask, agent),
			Delegation: &isTask,
			OK:         success,
			Error:      ev.Error,
			Secs:       int(elapsed.Round(time.Second) / time.Second),
			Added:      output.LinesAdded,
			Removed:    output.LinesRemoved,
			Count:      output.ResultCount,
			Range:      output.ResultRange,
			Problems:   output.Problems,
			Diff:       output.Diff,
			Artifacts:  output.Artifacts,
			ProposalID: output.ProposalID,
		})
		return receipt, output.Images, execErr
	}, asStreamHandler(reasoning), opts)
	if err != nil {
		// handled=true: this path RAN and this is its outcome. Reporting false
		// here conflated "the tool loop failed" with the guard clauses above,
		// which mean "the tool loop does not apply" — and execute() reacts to
		// the latter by falling through to RespondStream. A failed turn was
		// therefore re-sent as a second, non-tool completion that added the
		// user's message to the context a second time and discarded this error
		// in favour of whatever the retry produced. One DNS failure cost three
		// provider calls and left the question in the model's memory twice.
		//
		// The sequence comes back with it, and that is the whole difference
		// between a stopped turn and a deleted one. `Result{}` here threw away
		// every part the loop had recorded — each tool call, each line of
		// narration between them — so a Stop pressed after twenty minutes of
		// work reached the caller as an error and nothing else. desktop's
		// appendFailedTurn then stored an empty row, and reopening the chat
		// showed a question with no answer under it: the record of the work was
		// gone from the one place it was supposed to survive.
		//
		// Assembled exactly as the success path assembles it, ten lines down.
		// What is NOT here is the round that was in flight when the wall came:
		// its prose never completed a round, so it never became a part. The
		// window still holds that half-sentence (its live preview is the only
		// copy), which is why the frontend keeps its own and this keeps the
		// rest.
		//
		// Reply is the last thing the model *said*, not `reply`. The loop
		// returns a cancelled tool's own output in that variable, which on a
		// Stop is the string "context canceled" — and a bubble reading
		// "context canceled" over "หยุดการทำงานแล้ว" is the app blaming itself
		// for obeying. The last text part is always the model's own sentence.
		// It goes in Reply *and* stays in Parts on purpose: the frontend draws
		// the final text part as the bubble and skips it in the timeline
		// (stepsFromParts), so leaving Reply empty would drop that sentence
		// from both.
		partial := ""
		if last, ok := parts.lastText(); ok {
			partial = last
		}
		return Result{Reply: partial, Parts: parts.all(), Status: TurnStatusError}, true, err
	}
	debuglog.Info("agent tool loop", fmt.Sprintf("usedTools=%v", usedTools))
	// A reply the loop produced itself rather than taking from a round — the
	// doom-loop stop, ToolLoopExhausted, a cancelled tool's output — never went
	// through OnRound, so the sequence has not heard of it. Record it, or the
	// turn would be drawn without the only thing the user is actually told.
	if trimmed := strings.TrimSpace(reply); trimmed != "" {
		if last, ok := parts.lastText(); !ok || last != trimmed {
			parts.addText(trimmed)
		}
	}
	assembled := parts.all()
	if onChunk != nil {
		// Erase the live preview first. Whatever it holds is at best the same
		// answer arriving a moment earlier, and at worst a draft the loop
		// discarded — either way the delivery below is the authority.
		if opts.OnContentReset != nil {
			opts.OnContentReset()
		}
		if strings.TrimSpace(reply) != "" {
			onChunk(reply)
		}
	}
	return Result{
		Reply:    reply,
		Parts:    assembled,
		Streamed: false,
		Status:   TurnStatusDone,
	}, true, nil
}

// toolRunOutput picks what a tool actually returned. RawOutput is the tool's
// own bytes and Content is the rendered-for-a-human version; modelToolReceipt
// prefers Raw for the same reason, and a record that disagreed with what the
// model was shown would be worse than useless.
func toolRunOutput(output skill.Output) string {
	if raw := strings.TrimSpace(output.RawOutput); raw != "" {
		return raw
	}
	return strings.TrimSpace(output.Content)
}

// failureReason is what a tool that failed *without* returning a Go error has
// to say for itself.
//
// A tool can fail two ways. It can return an error, and then the error is the
// reason. Or it can return `Success: false` with the reason written into its
// output — which is what every tool does whose refusal is meant to be read and
// acted on rather than to arrive as a crash. `subagent.task` is the clearest
// case and says so in its own comment: "no sub-agent named X", "the brief is too
// long", "the MCP server has not finished connecting" are all refusals a model
// can do something about, so they come back as unsuccessful results.
//
// That whole family used to be recorded as the word "ไม่สำเร็จ" and nothing
// else, and the word went to three readers at once: the timeline the user reads,
// the `error` column of tool_runs, and the summarizer that clusters that column
// into memory proposals. So a tool could fail ten times carrying ten copies of a
// precise sentence and produce a card reading *repeatedly failed with
// "ไม่สำเร็จ" — avoid this pattern*: a lesson with its cause deleted, drawn from
// evidence that was one field away the whole time (2026-08-12; see the summarizer's
// own note about `exit status 1`, which is this same shape one layer down).
//
// Stderr before Content, which is the precedence fallbackToolSummary already
// uses. First line and capped, because this is a label beside a tool call and a
// value that gets grouped on — not a transcript. The whole of it is kept anyway,
// in the run's output.
func failureReason(output skill.Output) string {
	for _, candidate := range []string{output.Stderr, output.Content} {
		line := strings.TrimSpace(candidate)
		if at := strings.IndexByte(line, '\n'); at >= 0 {
			line = strings.TrimSpace(line[:at])
		}
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > failureReasonMax {
			line = strings.TrimRight(string(r[:failureReasonMax]), " ,;:-—") + "…"
		}
		return line
	}
	// A tool that failed and said nothing anywhere. The placeholder stays for
	// exactly that case, and now means what it says rather than standing in for
	// a sentence nobody looked for.
	return "ไม่สำเร็จ"
}

// failureReasonMax caps the label. Long enough for the refusals this codebase
// writes, which are one or two sentences carrying their own remedy.
const failureReasonMax = 300

// The skill.Output is returned alongside the receipt because the receipt is
// written for the model, and the UI needs facts the model has no use for —
// today the write/edit line counts.
func (e *Executor) executeToolCallWithOutcome(ctx context.Context, call model.ToolCall) (string, skill.Output, bool, error) {
	args, parseErr := model.ParseToolArguments(call.Function.Arguments)
	if parseErr != nil {
		// The same refusal, worded from the actual cause. One argument written
		// twice is valid JSON that means something the model did not intend,
		// and telling it about the token limit would send it to fix a size that
		// is not the problem.
		var duplicate *model.DuplicateArgumentError
		if errors.As(parseErr, &duplicate) {
			return "", skill.Output{}, false, fmt.Errorf(
				"tool call NOT executed: you wrote %q twice in one call, and only the last value would have survived — the first was about to be dropped with nothing said. If you meant to do two things, make two separate tool calls.",
				duplicate.Key)
		}
		// No salvage: writing half a file and reporting success is worse than
		// failing loudly. Truncated JSON means the output token limit cut the
		// call short, so the wording comes from the one place that says it
		// (model.TruncatedToolCallRefusal) rather than being written a second
		// time here, in different words, for the same model to read.
		//
		// The ceiling is passed as 0 because this side does not know it: the
		// round's max_tokens belongs to the tool loop, which now catches this
		// case ahead of here and can state the number. What still arrives here
		// came by another road, and gets the same remedy without it.
		return "", skill.Output{}, false, errors.New(
			model.TruncatedToolCallRefusal(call.Function.Name, 0, false, parseErr))
	}

	name := strings.TrimSpace(call.Function.Name)
	if name == "" {
		return "", skill.Output{}, false, errors.New("tool call has empty function name")
	}
	output, handled, execErr := e.executeTool(ctx, name, args)
	if !handled || execErr != nil {
		return e.modelToolReceipt(ctx, name, args, output, execErr), output, false, execErr
	}
	return e.modelToolReceipt(ctx, name, args, output, nil), output, output.Success, nil
}

// ctx is threaded in for the rtk pass below: it shells out, and a subprocess
// running during a turn has to be reachable by the user's Stop like every
// other one.
func (e *Executor) modelToolReceipt(ctx context.Context, name string, args map[string]any, output skill.Output, execErr error) string {
	status := string(TurnStatusDone)
	success := output.Success && execErr == nil
	if !success {
		status = string(TurnStatusError)
	}

	result := strings.TrimSpace(output.RawOutput)
	if result == "" {
		result = strings.TrimSpace(output.Content)
	}
	// Optional token-savings pass (ARCHITECTURE.md §13): shrinks raw output
	// before it's wrapped into the receipt sent back to the model. Purely
	// additive — falls through to the untouched result if rtk isn't
	// installed or this tool call has no matching filter.
	if filter := rtk.FilterForTool(name, args); filter != "" {
		if filtered, ok := rtk.Filter(ctx, filter, result); ok {
			result = rtk.StripBanner(filtered)
		}
	}
	result = e.sanitizeAndTrimOutput(result)

	stderr := strings.TrimSpace(output.Stderr)
	if stderr == "" && execErr != nil {
		stderr = execErr.Error()
	}

	receipt := map[string]any{
		"tool":        strings.TrimSpace(name),
		"status":      status,
		"success":     success,
		"command":     strings.TrimSpace(output.Command),
		"output":      result,
		"stderr":      strings.TrimSpace(stderr),
		"duration_ms": output.DurationMs,
	}
	if path, ok := args["path"].(string); ok && strings.TrimSpace(path) != "" {
		receipt["path"] = strings.TrimSpace(path)
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return result
	}
	return string(payload)
}

func (e *Executor) executeTool(ctx context.Context, name string, args map[string]any) (skill.Output, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return skill.Output{}, false, errors.New("tool call has empty function name")
	}

	if e.dispatcher == nil {
		return skill.Output{}, false, errors.New("tool dispatcher is not available")
	}

	// What the gates below are really judging.
	//
	// A packed tool is one name in the block and several acts inside it
	// (skill.Unpack), and every reader from here down was written against the
	// names those acts used to have: safety keys on the literal "shell" and
	// answers RiskHigh to a call it can find no command in, and a permission
	// rule the user wrote names the tool they were asked about. Judging an
	// output read as "shell" would put an approval prompt in front of looking at
	// a log — and train the user to click through the one that matters.
	//
	// Only the judging half. What the model called is still `name`, and that is
	// what the hooks, the dispatcher and the result carry, because that is what
	// actually happened.
	judged := skill.Unpack(name, args)
	judgedArgs := toolCallToArgs(judged, args)
	assessment := safety.AssessCommand(judged, judgedArgs)
	commandLine := judged
	for _, rawArg := range judgedArgs {
		if rawArg == "" {
			continue
		}
		commandLine += " " + rawArg
	}
	ok, confirmErr := e.resolveApproval(ctx, judged, judgedArgs, commandLine, assessment)
	if confirmErr != nil {
		return skill.Output{}, true, confirmErr
	}
	if !ok {
		return skill.Output{
			Name:       name,
			Content:    "tool execution blocked by user",
			RawOutput:  "tool execution blocked by user",
			Success:    false,
			Stderr:     "tool execution blocked by user",
			DurationMs: 0,
		}, true, nil
	}

	// The user's own PreToolUse commands, after approval and before the tool.
	// After, deliberately: a hook is a rule the user wrote, and running it on a
	// call the user is about to refuse anyway would fire their formatter and
	// their notifier for work that never happens.
	if e.hooks.Any(hook.PreToolUse) {
		if decision := e.hooks.Run(ctx, hook.PreToolUse, name, args, nil); decision.Blocked {
			// Reported as a normal tool result rather than an error, so the
			// model reads the reason and tries something else instead of
			// repeating the call into the same wall.
			msg := "blocked by a PreToolUse hook: " + decision.Reason
			return skill.Output{
				Name: name, Content: msg, RawOutput: msg,
				Success: false, Stderr: msg,
			}, true, nil
		}
	}

	output, handled, err := e.dispatchWithDeadline(ctx, name, args)
	// PostToolUse fires on whatever happened, success or failure — "run my
	// formatter after a write" and "tell me when a command fails" are the same
	// hook point, and a hook that only saw the happy path would be useless for
	// the second. It cannot change the result: the tool has already run, and
	// pretending otherwise would be a lie about what the model is reading.
	if handled && e.hooks.Any(hook.PostToolUse) {
		e.hooks.Run(ctx, hook.PostToolUse, name, args, &hook.Result{
			OK:     err == nil && output.Success,
			Output: output.Content,
		})
	}
	return output, handled, err
}

// pendingCall is one tool call still running after its deadline. Fields other
// than started are written once, before done is closed, so a second caller
// needs no lock to read them.
type pendingCall struct {
	started time.Time
	done    chan struct{}
	output  skill.Output
	handled bool
	err     error
}

// notExposed is the refusal for a tool nothing answered under this caller's
// exposure — the name is unknown, or held back from this agent or stance.
//
// It says what to do next, because the bare sentence taught the wrong lesson:
// an agent told only "not exposed" went detective (31 ส.ค., session 192150 —
// ~40 calls listing DataRoot, reading the renderer's package.json and this
// repo's source, hunting a way to run the tool by hand). The blocked step is
// the user's to unblock, and one question reaches them in one call.
func notExposed(name string) error {
	return fmt.Errorf("tool %q is not exposed to agent here — either no such tool, or this seat does not hold it. "+
		"Do not hunt for another way to run it; name the blocked step and ask in one call (ask_user, or ask_main from a subagent), or finish what the tools you do hold can do", name)
}

func (e *Executor) dispatchWithDeadline(ctx context.Context, name string, args map[string]any) (skill.Output, bool, error) {
	// Interactive tools wait on a human — no deadline, ctx cancel is the brake.
	if noDeadlineTools[strings.ToLower(name)] {
		output, handled, err := e.dispatcher.ExecuteTool(ctx, name, args)
		if !handled {
			return output, false, notExposed(name)
		}
		return output, true, err
	}

	// The deadline bounds how long the *turn* waits, not how long the tool runs.
	// It used to be both: an overrun was cancelled and reported as "abnormally
	// slow, retry with a narrower scope", so a transcription that needed 90
	// seconds got thrown away at 60 and started again from zero — twice the work
	// and still no answer. Now the call is left running and the model is told to
	// look in on it, which is the same shape as shell's run_in_background /
	// shell_output pair and as task / task_result.
	// Unpacked for the same reason the approval gate is: the one call that gets
	// to stretch a turn is an output read that was asked to wait, and after the
	// packing it arrives named `shell`. Asked under that name it would take the
	// plain shell branch, read `timeout_seconds` off a call that has none, and
	// cut a wait_for off at sixty seconds — degrading it back into the polling
	// loop it exists to replace.
	deadline := toolCallDeadline(skill.Unpack(name, args), args)
	key := callKey(name, args)
	call := e.beginCall(ctx, key, name, args)

	select {
	case <-call.done:
		e.forget(key)
		if !call.handled {
			return call.output, false, notExposed(name)
		}
		return call.output, true, call.err
	case <-ctx.Done():
		// Parent turn canceled (user hit stop) — propagate, not a status report.
		// The entry is dropped with the turn: the executor outlives it (one per
		// session), and a parked entry whose work is now dying would answer the
		// NEXT turn's identical call — the user asking for the same thing again
		// — with this turn's "canceled" instead of running it.
		e.forget(key)
		return skill.Output{Name: name, Content: "tool execution canceled", RawOutput: "tool execution canceled", Success: false, Stderr: ctx.Err().Error()}, true, ctx.Err()
	case <-time.After(deadline):
		return stillRunning(name, call), true, nil
	}
}

// stillRunning is what the model reads when a call outran the turn's patience.
// Success is true on purpose: nothing failed, and a red row for "still working"
// tells the user the wrong story — the same call task_result makes for a
// delegate that is waiting rather than finished. What makes it unmistakable is
// the text, which names the one thing that collects it.
func stillRunning(name string, call *pendingCall) skill.Output {
	msg := fmt.Sprintf(
		"STILL RUNNING — %q has been working for %s. It was NOT cancelled and NOT restarted; it is still going in the background. "+
			"Do NOT start the same work again, and do not retry it with different arguments. "+
			"To look in on it, call %s again with exactly the same arguments: you get its real result the moment it lands, or this line again if it is still working. "+
			"It is given up on only after %s in total. In the meantime you can run other tools, or tell the user what you are waiting for.",
		name, time.Since(call.started).Round(time.Second), name, maxToolExecutionTimeout)
	return skill.Output{
		Name:       name,
		Content:    msg,
		RawOutput:  msg,
		Success:    true,
		DurationMs: time.Since(call.started).Milliseconds(),
	}
}

// beginCall returns the call already in flight for this exact tool+arguments,
// starting one only if there is none. That lookup is the whole mechanism: the
// model's "call it again to check" costs a status read instead of a second run.
func (e *Executor) beginCall(ctx context.Context, key, name string, args map[string]any) *pendingCall {
	e.pendingMu.Lock()
	defer e.pendingMu.Unlock()
	if call, ok := e.pending[key]; ok {
		// A cancelled call is an absence, not a result. Its ctx died with a turn
		// that is over — Stop, or the turn's own end cancelling what it parked —
		// and replaying its "canceled" to a later identical call reports a
		// cancellation nobody asked for and runs nothing. Only finished-and-
		// cancelled entries are dropped; a live call is still the dedup this
		// lookup exists for.
		stale := false
		select {
		case <-call.done:
			stale = errors.Is(call.err, context.Canceled)
		default:
		}
		if !stale {
			return call
		}
		delete(e.pending, key)
	}
	call := &pendingCall{started: time.Now(), done: make(chan struct{})}
	if e.pending == nil {
		e.pending = map[string]*pendingCall{}
	}
	e.pending[key] = call

	// The work outlives this call but never the turn: ctx is the turn's, so Stop
	// still kills it, and maxToolExecutionTimeout is the ceiling for a tool that
	// is never going to finish. Tools that ignore ctx (grep/list walk the FS)
	// run to their own end either way — the difference is that their result is
	// now collected instead of discarded.
	bg, cancel := context.WithTimeout(ctx, maxToolExecutionTimeout)
	go func() {
		defer cancel()
		defer close(call.done)
		call.output, call.handled, call.err = e.dispatcher.ExecuteTool(bg, name, args)
	}()
	return call
}

// forget drops a call once its result has reached the model, so the next
// identical call runs fresh instead of replaying an answer from before.
//
// ponytail: a parked call the model never checks back on keeps its entry for
// the life of the session — one small struct per slow tool. Sweep it if a
// session ever collects enough of them for anyone to notice.
func (e *Executor) forget(key string) {
	e.pendingMu.Lock()
	delete(e.pending, key)
	e.pendingMu.Unlock()
}

// callKey identifies "the same call again". encoding/json sorts map keys, so
// the same arguments key the same way however the model happened to order them.
func callKey(name string, args map[string]any) string {
	raw, _ := json.Marshal(args)
	return strings.ToLower(strings.TrimSpace(name)) + "\x00" + string(raw)
}

func toolCallToArgs(name string, args map[string]any) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "write":
		path := ""
		content := ""
		if raw, ok := args["path"].(string); ok {
			path = strings.TrimSpace(raw)
		}
		if raw, ok := args["content"].(string); ok {
			content = strings.TrimSpace(raw)
		}
		result := make([]string, 0, 2)
		if path != "" {
			result = append(result, path)
		}
		if content != "" {
			result = append(result, content)
		}
		return result
	case "list":
		if raw, ok := args["path"].(string); ok {
			return []string{strings.TrimSpace(raw)}
		}
	case "read", "delete", "edit":
		if raw, ok := args["path"].(string); ok {
			return []string{strings.TrimSpace(raw)}
		}
	case "sheet_write", "doc_write":
		// Path only, and these strings are what the approval prompt shows: the
		// question is "overwrite quarterly.xlsx?", not several thousand
		// characters of rows. For the two writers the rest is the whole document
		// as JSON; for deck_export it is only a format name, and the file the
		// user is being asked about is still the path.
		if raw, ok := args["path"].(string); ok {
			return []string{strings.TrimSpace(raw)}
		}
	case "grep":
		result := make([]string, 0, 2)
		if raw, ok := args["pattern"].(string); ok && strings.TrimSpace(raw) != "" {
			result = append(result, strings.TrimSpace(raw))
		}
		if raw, ok := args["path"].(string); ok && strings.TrimSpace(raw) != "" {
			result = append(result, strings.TrimSpace(raw))
		}
		return result
	case "github_repo_summary", "plugin_install":
		if raw, ok := args["repo_url"].(string); ok {
			return []string{strings.TrimSpace(raw)}
		}
	case "shell", "desk_terminal":
		// desk_terminal shares this case because it runs the same thing in the
		// same shell — the only difference is that the user watches. Splitting
		// it off would mean an approval prompt that reads differently for the
		// visible version of an identical command.
		//
		// Tokenized, because this is what safety.AssessCommand reads: it judges
		// the command word against isShellHighRisk and takes the rest as its
		// flags. Handing it the whole line as one element makes every call look
		// like an unrecognized command, and handing it nothing makes every call
		// read as "shell with empty command" — high risk, every time, which
		// trains the user to click through the prompt that matters most.
		if raw, ok := args["command"].(string); ok {
			return strings.Fields(raw)
		}
	case "git":
		// action first, then its arguments — the same shape the text path
		// produces, so one permission rule matches whether the human typed the
		// command or the model called the tool.
		result := make([]string, 0, 4)
		if raw, ok := args["action"].(string); ok && strings.TrimSpace(raw) != "" {
			result = append(result, strings.TrimSpace(raw))
		}
		if raw, ok := args["args"].([]any); ok {
			for _, item := range raw {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					result = append(result, strings.TrimSpace(s))
				}
			}
		}
		return result
	}
	return nil
}

func (e *Executor) approveOrDeny(ctx context.Context, name, reason string) (bool, error) {
	if e.approve == nil {
		return true, nil
	}
	return e.approve(ctx, name, reason)
}

// resolveApproval decides whether a tool call is allowed to run, checking
// user-configured PermissionConfig rules before falling back to the coarse
// ApprovalMode gate. A matching "allow"/"deny" rule short-circuits without
// prompting; "ask" (or no matching rule under a mode that requires it) goes
// through the normal approveOrDeny prompt.
//
// The one subtlety is which "ask" wins. An ask the *user* wrote is their
// decision and beats the mode outright — that is what writing it was for. An
// ask the *app* generated (safety.PermissionRule.Default: the per-server rule
// bootstrap attaches so MCP tools never auto-run before anyone has decided
// anything) is only an opening position, and it must step aside for the mode
// the user then chose. It did not: full access, whose card reads "รับทุกอย่าง
// โดยไม่ถาม", still opened a dialog on every single MCP call, because a matched
// rule skipped the mode check entirely (2026-08-06).
//
// That is also the permissions rule this codebase already settled: rights come
// from the user's visible list, and there is no second, quieter tier deciding
// on their behalf.
func (e *Executor) resolveApproval(ctx context.Context, toolName string, args []string, commandLine string, assessment safety.Assessment) (bool, error) {
	rule, matched := e.permissions.ResolveRule(toolName, args)
	appDefaultAsk := matched && rule.Default && rule.Action == safety.PermissionAsk
	if matched && !appDefaultAsk {
		switch rule.Action {
		case safety.PermissionAllow:
			return true, nil
		case safety.PermissionDeny:
			return false, nil
		}
	} else if !safety.ShouldPrompt(e.currentApprovalMode(), assessment) {
		return true, nil
	}
	e.stopSpinner()
	return e.approveOrDeny(ctx, commandLine, assessment.Reason)
}

func (e *Executor) normalizeIntent(line string, intent command.Intent) command.Intent {
	if intent.Raw != "" {
		return intent
	}
	return command.Parse(line, command.ParseTokens, e.commandSet)
}

func (e *Executor) dispatchBySkill(ctx context.Context, line string) (skill.Output, bool, error) {
	if e.dispatcher == nil {
		return skill.Output{}, false, nil
	}
	output, handled, err := e.dispatcher.Execute(ctx, line)
	if !handled || err != nil {
		return output, handled, err
	}
	return output, true, nil
}

// thinkSegments turns the reasoning-chunk stream into per-round durations for
// the timeline's "thinking" events: observe() on every chunk, flush() at each
// round boundary. Chunks and flushes arrive on different call paths, hence the
// lock.
type thinkSegments struct {
	mu    sync.Mutex
	seen  bool
	start time.Time
	last  time.Time
}

func (s *thinkSegments) observe() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !s.seen {
		s.seen, s.start = true, now
	}
	s.last = now
}

func (s *thinkSegments) flush() (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.seen {
		return 0, false
	}
	s.seen = false
	secs := int(s.last.Sub(s.start).Seconds())
	if secs < 1 {
		secs = 1
	}
	return secs, true
}

func asStreamHandler(callback func(string)) func(string) error {
	if callback == nil {
		return nil
	}
	return func(chunk string) error {
		callback(chunk)
		return nil
	}
}

// OutputBackstop is the largest a single tool result may be before the executor
// cuts it, derived from the history budget that result has to share.
//
// **A backstop, not a working limit.** Every tool that can produce a lot of
// output already bounds itself, at a size chosen for what that tool's job needs:
// `read` stops at 256KB or 2000 lines and hands back an offset to continue,
// `shell` stops at 220 lines, the browser's `read` at 60,000 characters,
// `skill_view` at 32KB with the byte count said out loud. Those are considered
// decisions. This number exists for the results that had no such thought behind
// them — a third-party MCP server that answers a scrape with 147KB, measured, on
// the owner's machine, 50 times in 54 calls.
//
// The reason it may not simply be removed is written in cognitive/agent.go's own
// comment about maxOverflowCompactions: compaction summarizes an *accumulated*
// history, and "summarizing cannot fix" a single oversized message. So the job
// here is precisely to keep any one result below the size at which it becomes
// the thing compaction cannot rescue.
//
// One thirty-second of the budget, which is the ratio the old fixed 4096 already
// was against the old fixed budget. Nothing about the shape changed; it only
// started scaling with the thing it was always a fraction of.
func OutputBackstop(historyChars int) int {
	if override := backstopOverride(); override > 0 {
		return override
	}
	if historyChars <= 0 {
		return defaultOutputBackstop
	}
	if limit := historyChars / outputBackstopFraction; limit > defaultOutputBackstop {
		return limit
	}
	return defaultOutputBackstop
}

// backstopOverride is AETOX_MAX_TOOL_OUTPUT, in characters, read once.
//
// An escape hatch for whoever is debugging, not the dial an ordinary user is
// meant to reach for — the same standing as AETOX_DATA_ROOT and
// AETOX_DISABLE_UPDATE_CHECK, which is why it is an environment variable at all.
// This is a desktop app; the day a real user needs to move this number, it
// belongs in Settings, and copying Claude Code's BASH_MAX_OUTPUT_LENGTH into a
// window nobody launches from a shell would be taking the shape of somebody
// else's answer without their question.
//
// Both of the implementations worth comparing against are asked for a knob like
// this because their limits are fixed constants. Aetox's scales with the model's
// window, so the reason people ask has mostly been removed — which is why this
// stays an escape hatch rather than growing a settings page nobody has asked for.
//
// Read once: it sits on the path of every tool result, and an environment
// variable that changes mid-process is not a thing worth supporting.
var backstopOverride = sync.OnceValue(readBackstopOverride)

func readBackstopOverride() int {
	raw := strings.TrimSpace(os.Getenv("AETOX_MAX_TOOL_OUTPUT"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		// A misspelled number is ignored rather than obeyed. Obeying a zero
		// would silently empty every tool result in the session, which is a
		// worse answer to a typo than carrying on with the default.
		debuglog.Msg("AETOX_MAX_TOOL_OUTPUT=%q is not a positive number; ignoring it", raw)
		return 0
	}
	return n
}

// historyCharsOf asks an agent for the budget its results have to share.
//
// A type assertion rather than a method on the Agent interface: every fake in
// every test implements that interface, and widening it would make them all
// fail to compile over a number none of them care about. An agent that cannot
// answer returns zero, which is the floor, which is what it had before.
func historyCharsOf(a Agent) int {
	budget, ok := a.(interface{ HistoryChars() int })
	if !ok {
		return 0
	}
	return budget.HistoryChars()
}
