package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/rtk"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/think"
)

type TurnStatus string

const (
	TurnStatusDone    TurnStatus = "done"
	TurnStatusError   TurnStatus = "error"
	TurnStatusBlocked TurnStatus = "blocked"

	defaultToolSummaryTimeout      = 30 * time.Second
	defaultToolSummaryPromptMaxLen = 4096
)

// A single tool call that runs longer than this is reported back to the model
// as abnormally slow so it can retry with a narrower scope instead of hanging
// the turn. Applies to tool execution only, not conversation. Var (not const)
// so tests can shrink it.
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
func HasNoDeadline(name string) bool { return noDeadlineTools[strings.ToLower(strings.TrimSpace(name))] }

type Agent interface {
	Respond(context.Context, string, TurnOptions) (string, error)
	// RespondEphemeral answers a one-shot prompt without writing anything into
	// conversation history — for meta work like tool-run summaries.
	RespondEphemeral(context.Context, string, TurnOptions) (string, error)
	RespondStream(context.Context, string, func(string) error, func(string) error, TurnOptions) (string, bool, error)
	RespondWithTools(context.Context, []model.ToolDefinition, string, func(context.Context, model.ToolCall) (string, error), func(string) error, TurnOptions) (string, bool, error)
	SupportsToolCalling() bool
}

type TurnOptions struct {
	ThinkLevel think.Level
}

type Dispatcher interface {
	Execute(context.Context, string) (skill.Output, bool, error)
	ToolDefinitions() []model.ToolDefinition
	ExecuteTool(context.Context, string, map[string]any) (skill.Output, bool, error)
}

type ApprovalPromptFunc func(context.Context, string, string) (bool, error)

type Executor struct {
	agent          Agent
	dispatcher     Dispatcher
	commandSet     map[string]struct{}
	approve        ApprovalPromptFunc
	approvalMode   safety.ApprovalMode
	permissions    safety.PermissionConfig
	summaryTimeout time.Duration
	summaryLimit   int
	turnOptions    TurnOptions
	statusReporter func(string)
	onToolAction   func(ToolEvent)
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
}

type Result struct {
	Reply    string
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
		limit = defaultToolSummaryPromptMaxLen
	}
	mode := opts.ApprovalMode
	if mode == "" {
		mode = safety.ApprovalAsk
	}
	return &Executor{
		agent:          opts.Agent,
		dispatcher:     opts.Dispatcher,
		commandSet:     opts.CommandSet,
		approve:        opts.Approve,
		approvalMode:   mode,
		permissions:    opts.Permissions,
		summaryTimeout: timeout,
		summaryLimit:   limit,
		turnOptions:    opts.TurnOptions,
		statusReporter: opts.StatusReporter,
		onToolAction:   opts.OnToolAction,
	}
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
	Action string `json:"action"` // "call" | "result"
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
	OK      bool   `json:"ok"`              // result only
	Error   string `json:"error,omitempty"` // result only, when !OK
	// Added/Removed are the line counts of a write or edit, zero elsewhere.
	Added   int `json:"added,omitempty"`
	Removed int `json:"removed,omitempty"`
}

// Label is what a timeline row reads, e.g. "write internal/skill/edit.go".
func (ev ToolEvent) Label() string {
	return strings.TrimSpace(ev.Name + " " + ev.Subject)
}

func (e *Executor) reportToolCall(ref, name, args string) {
	if e.onToolAction != nil {
		e.onToolAction(ToolEvent{Action: "call", Ref: ref, Name: name, Subject: toolCallSubject(args)})
	}
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

func (e *Executor) reportToolResult(ev ToolEvent) {
	if e.onToolAction != nil {
		ev.Action = "result"
		e.onToolAction(ev)
	}
}

func (e *Executor) conversationThinkingStatus() string {
	if e.turnOptions.ThinkLevel == think.LevelNoThinking {
		return "กำลังประมวลผลคำตอบ..."
	}
	return "กำลังคิดคำตอบ..."
}

func (e *Executor) Execute(
	ctx context.Context,
	line string,
	intent command.Intent,
	onChunk func(string),
	onReasoningChunk func(string),
	onToolComplete func(),
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

	// Explicit command (grammar-recognized skill token, e.g. "read foo.txt",
	// "/time") → direct dispatch. Everything else is the model's call — there
	// is deliberately no keyword/regex guessing between the user and the model
	// (ARCHITECTURE.md §17).
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
		if result, handled, err := e.executeAgentToolLoop(ctx, parsed, onChunk, onReasoningChunk); handled {
			return result, err
		}
	}

	debuglog.Msg("path: conversation (streaming chat)")
	reply, streamed, err := e.agent.RespondStream(ctx, parsed.Raw, asStreamHandler(onChunk), asStreamHandler(onReasoningChunk), e.turnOptions)
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

	reply, usedTools, err := e.agent.RespondWithTools(ctx, toolDefs, intent.Raw, func(ctx context.Context, call model.ToolCall) (string, error) {
		e.reportToolCall(call.ID, call.Function.Name, call.Function.Arguments)
		// A tool that spawns anything needs to know which call it is, so the work
		// it causes can be traced back to this row (`task` stamps ToolEvent.Parent
		// with it). Nothing else reads it, and a tool that ignores it is unaffected.
		ctx = WithCallID(ctx, call.ID)
		receipt, output, success, execErr := e.executeToolCallWithOutcome(ctx, call)
		ev := ToolEvent{
			Ref:     call.ID,
			Name:    call.Function.Name,
			Subject: toolCallSubject(call.Function.Arguments),
			OK:      success,
			Added:   output.LinesAdded,
			Removed: output.LinesRemoved,
		}
		if !success {
			if execErr != nil {
				ev.Error = execErr.Error()
			} else {
				ev.Error = "ไม่สำเร็จ"
			}
		}
		e.reportToolResult(ev)
		return receipt, execErr
	}, asStreamHandler(onReasoningChunk), e.turnOptions)
	if err != nil {
		return Result{}, false, err
	}
	debuglog.Info("agent tool loop", fmt.Sprintf("usedTools=%v", usedTools))
	if onChunk != nil {
		if strings.TrimSpace(reply) != "" {
			onChunk(reply)
		}
	}
	return Result{
		Reply:    reply,
		Streamed: false,
		Status:   TurnStatusDone,
	}, true, nil
}

// The skill.Output is returned alongside the receipt because the receipt is
// written for the model, and the UI needs facts the model has no use for —
// today the write/edit line counts.
func (e *Executor) executeToolCallWithOutcome(ctx context.Context, call model.ToolCall) (string, skill.Output, bool, error) {
	args, parseErr := model.ParseToolArguments(call.Function.Arguments)
	if parseErr != nil {
		// No salvage: writing half a file and reporting success is worse than
		// failing loudly. Truncated JSON usually means the output token limit
		// cut the call short — say so, so the model fixes the size, not the path.
		return "", skill.Output{}, false, fmt.Errorf(
			"tool call NOT executed: arguments are not valid JSON (%v) — likely truncated by the output token limit; retry with shorter content or split the work into smaller calls",
			parseErr)
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

	assessment := safety.AssessCommand(name, toolCallToArgs(name, args))
	commandLine := name
	for _, rawArg := range toolCallToArgs(name, args) {
		if rawArg == "" {
			continue
		}
		commandLine += " " + rawArg
	}
	ok, confirmErr := e.resolveApproval(ctx, name, toolCallToArgs(name, args), commandLine, assessment)
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

	// Run under a per-tool deadline. Tools that honor ctx (http, mcp) stop on
	// cancel; those that don't (grep/list walk the FS ignoring ctx) keep running
	// in this goroutine after we return — its result is just discarded.
	// ponytail: leaks the stray goroutine's CPU until it finishes on its own;
	// plumb ctx into the FS-walking tools if that leak ever bites.
	// Interactive tools wait on a human — no deadline, ctx cancel is the brake.
	if noDeadlineTools[strings.ToLower(name)] {
		output, handled, err := e.dispatcher.ExecuteTool(ctx, name, args)
		if !handled {
			return output, false, fmt.Errorf("tool %q is not exposed to agent", name)
		}
		return output, true, err
	}
	toolCtx, cancel := context.WithTimeout(ctx, toolExecutionTimeout)
	defer cancel()

	type toolExecResult struct {
		output  skill.Output
		handled bool
		err     error
	}
	done := make(chan toolExecResult, 1)
	go func() {
		output, handled, err := e.dispatcher.ExecuteTool(toolCtx, name, args)
		done <- toolExecResult{output, handled, err}
	}()

	select {
	case r := <-done:
		if !r.handled {
			return r.output, false, fmt.Errorf("tool %q is not exposed to agent", name)
		}
		return r.output, true, r.err
	case <-ctx.Done():
		// Parent turn canceled (user hit stop) — propagate, not a slowness report.
		return skill.Output{Name: name, Content: "tool execution canceled", RawOutput: "tool execution canceled", Success: false, Stderr: ctx.Err().Error()}, true, ctx.Err()
	case <-time.After(toolExecutionTimeout):
		cancel() // nudge ctx-aware tools to stop
		msg := fmt.Sprintf("tool %q is abnormally slow: still running after %s and was abandoned. Retry with a narrower scope (a more specific path or pattern) or a different approach.", name, toolExecutionTimeout)
		return skill.Output{Name: name, Content: msg, RawOutput: msg, Success: false, Stderr: msg}, true, errors.New(msg)
	}
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
func (e *Executor) resolveApproval(ctx context.Context, toolName string, args []string, commandLine string, assessment safety.Assessment) (bool, error) {
	if action, matched := e.permissions.Resolve(toolName, args); matched {
		switch action {
		case safety.PermissionAllow:
			return true, nil
		case safety.PermissionDeny:
			return false, nil
		}
	} else if !safety.ShouldPrompt(e.approvalMode, assessment) {
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

func asStreamHandler(callback func(string)) func(string) error {
	if callback == nil {
		return nil
	}
	return func(chunk string) error {
		callback(chunk)
		return nil
	}
}
