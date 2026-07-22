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

type Agent interface {
	Respond(context.Context, string, TurnOptions) (string, error)
	RespondStream(context.Context, string, func(string) error, TurnOptions) (string, bool, error)
	RespondWithTools(context.Context, []model.ToolDefinition, string, func(context.Context, model.ToolCall) (string, error), TurnOptions) (string, bool, error)
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
	onToolAction   func(action, detail string)
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
	OnToolAction   func(action, detail string)
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

func (e *Executor) reportToolCall(name, args string) {
	if e.onToolAction != nil {
		e.onToolAction("call", name+" "+truncate(args, 40))
	}
}

func (e *Executor) reportToolResult(name, status string) {
	if e.onToolAction != nil {
		e.onToolAction("result", name+": "+status)
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
		if result, handled, err := e.executeAgentToolLoop(ctx, parsed, onChunk); handled {
			return result, err
		}
	}

	debuglog.Msg("path: conversation (streaming chat)")
	reply, streamed, err := e.agent.RespondStream(ctx, parsed.Raw, asStreamHandler(onChunk), e.turnOptions)
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
		e.reportToolCall(call.Function.Name, call.Function.Arguments)
		receipt, success, execErr := e.executeToolCallWithOutcome(ctx, call)
		if success {
			e.reportToolResult(call.Function.Name, "สำเร็จ")
		} else if execErr != nil {
			e.reportToolResult(call.Function.Name, execErr.Error())
		} else {
			e.reportToolResult(call.Function.Name, "ไม่สำเร็จ")
		}
		return receipt, execErr
	}, e.turnOptions)
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

func (e *Executor) executeToolCallWithOutcome(ctx context.Context, call model.ToolCall) (string, bool, error) {
	args, parseErr := model.ParseToolArguments(call.Function.Arguments)
	if parseErr != nil {
		// if JSON is truncated (common with large write content), try to salvage
		if strings.TrimSpace(call.Function.Name) == "write" {
			salvaged := salvageWriteArgs(call.Function.Arguments)
			if salvaged != nil {
				args = salvaged
			} else {
				return "", false, parseErr
			}
		} else {
			return "", false, parseErr
		}
	}

	name := strings.TrimSpace(call.Function.Name)
	if name == "" {
		return "", false, errors.New("tool call has empty function name")
	}
	output, handled, execErr := e.executeTool(ctx, name, args)
	if !handled {
		return e.modelToolReceipt(name, args, output, execErr), false, execErr
	}
	if execErr != nil {
		return e.modelToolReceipt(name, args, output, execErr), false, execErr
	}
	success := output.Success
	return e.modelToolReceipt(name, args, output, nil), success, nil
}

func (e *Executor) modelToolReceipt(name string, args map[string]any, output skill.Output, execErr error) string {
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
		if filtered, ok := rtk.Filter(filter, result); ok {
			result = filtered
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

	output, handled, err := e.dispatcher.ExecuteTool(ctx, name, args)
	if !handled {
		return output, false, fmt.Errorf("tool %q is not exposed to agent", name)
	}
	return output, true, err
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

func salvageWriteArgs(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	// try to extract {"path": "...", "content": "..."} from truncated JSON
	pathStart := strings.Index(raw, `"path"`)
	contentStart := strings.Index(raw, `"content"`)
	if pathStart < 0 || contentStart < 0 {
		return nil
	}
	// find path value
	pathValStart := strings.Index(raw[pathStart:], `"`) + pathStart
	pathValStart = strings.Index(raw[pathValStart+1:], `"`) + pathValStart + 2
	pathValEnd := strings.Index(raw[pathValStart:], `"`) + pathValStart
	if pathValEnd <= pathValStart {
		return nil
	}
	path := raw[pathValStart:pathValEnd]

	// find content value
	contValStart := strings.Index(raw[contentStart:], `"`) + contentStart
	contValStart = strings.Index(raw[contValStart+1:], `"`) + contValStart + 2
	content := raw[contValStart:]
	// remove trailing garbage
	if idx := strings.LastIndex(content, `"`); idx > 0 {
		content = content[:idx]
	}
	content = strings.ReplaceAll(content, `\n`, "\n")
	content = strings.ReplaceAll(content, `\"`, `"`)
	content = strings.ReplaceAll(content, `\\`, `\`)

	return map[string]any{
		"path":    path,
		"content": content,
	}
}
