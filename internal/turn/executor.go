package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aetox-cli/internal/command"
	"aetox-cli/internal/model"
	"aetox-cli/internal/safety"
	"aetox-cli/internal/skill"
	"aetox-cli/internal/think"
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

type InferredToolCandidate struct {
	Name           string
	Args           map[string]any
	MissingMessage string
}

type ApprovalPromptFunc func(context.Context, string, string) (bool, error)

type Executor struct {
	agent          Agent
	dispatcher     Dispatcher
	commandSet     map[string]struct{}
	approve        ApprovalPromptFunc
	approvalMode   safety.ApprovalMode
	summaryTimeout time.Duration
	summaryLimit   int
	turnOptions    TurnOptions
	statusReporter func(string)
}

type ExecutorOptions struct {
	Agent          Agent
	Dispatcher     Dispatcher
	CommandSet     map[string]struct{}
	Approve        ApprovalPromptFunc
	ApprovalMode   safety.ApprovalMode
	SummaryTimeout time.Duration
	SummaryLimit   int
	TurnOptions    TurnOptions
	StatusReporter func(string)
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
		summaryTimeout: timeout,
		summaryLimit:   limit,
		turnOptions:    opts.TurnOptions,
		statusReporter: opts.StatusReporter,
	}
}

func (e *Executor) reportStatus(msg string) {
	if e.statusReporter != nil {
		e.statusReporter(msg)
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
	e.reportStatus("กำลังวิเคราะห์คำขอ...")
	parsed := e.normalizeIntent(line, intent)
	e.reportStatus("กำลังค้นหาเครื่องมือ...")
	candidates := e.inferToolCandidates(parsed.Raw)
	if e.shouldExecuteInferredBeforeAgent(parsed, candidates) {
		if result, handled, err := e.executeInferredToolCandidatesLoop(ctx, parsed.Raw, candidates); handled {
			return result, err
		}
	}

	if e.shouldUseInferredToolPath(parsed, candidates) {
		e.reportStatus("กำลังเตรียมเครื่องมือ...")
		if e.agent == nil || !e.agent.SupportsToolCalling() {
			if result, handled, err := e.executeInferredToolCandidatesLoop(ctx, parsed.Raw, candidates); handled {
				return result, err
			}
		} else {
			toolResult, handled, toolUsed, toolSucceeded, err := e.executeAgentToolLoop(ctx, parsed, onChunk)
			if handled {
				if (!toolUsed || !toolSucceeded) && len(candidates) > 0 {
					if fallback, fallbackHandled, fallbackErr := e.executeInferredToolCandidatesLoop(ctx, parsed.Raw, candidates); fallbackHandled {
						return fallback, fallbackErr
					}
				}
				return toolResult, err
			}

			if len(candidates) > 0 {
				if fallback, handled, fallbackErr := e.executeInferredToolCandidatesLoop(ctx, parsed.Raw, candidates); handled {
					return fallback, fallbackErr
				}
			}
		}
	}

	if parsed.Kind == command.KindConversation {
		e.reportStatus(e.conversationThinkingStatus())
		reply, streamed, err := e.agent.RespondStream(ctx, parsed.Raw, asStreamHandler(onChunk), e.turnOptions)
		return Result{
			Reply:    reply,
			Streamed: streamed,
			Status:   TurnStatusDone,
		}, err
	}

	e.reportStatus("กำลังรันเครื่องมือ...")
	return e.executeSkillTurn(ctx, line, parsed, onToolComplete)
}

func (e *Executor) shouldExecuteInferredBeforeAgent(parsed command.Intent, candidates []InferredToolCandidate) bool {
	if len(candidates) == 0 || parsed.IsSlash {
		return false
	}
	for _, candidate := range candidates {
		switch candidate.Name {
		case "write", "read", "delete", "github_repo_summary", "plugin_install":
			return true
		}
	}
	return false
}

func (e *Executor) shouldUseInferredToolPath(parsed command.Intent, candidates []InferredToolCandidate) bool {
	if len(candidates) == 0 {
		return false
	}
	if parsed.IsSlash {
		return false
	}
	if parsed.Kind == command.KindConversation {
		return true
	}
	if parsed.Kind != command.KindSkill {
		return false
	}

	if strings.Contains(strings.ToLower(parsed.Raw), " and ") || strings.Contains(parsed.Raw, ",") {
		return true
	}
	if strings.Contains(strings.ToLower(parsed.Raw), " then ") {
		return true
	}

	for _, candidate := range candidates {
		if candidate.Name == "list" {
			lower := strings.ToLower(parsed.Raw)
			if strings.Contains(lower, " directory ") || strings.Contains(lower, " folder ") || strings.Contains(lower, " path ") {
				return true
			}
		}
	}

	return false
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
	if assessment.Risk == safety.RiskHigh {
		approved, confirmErr := e.approveOrDeny(ctx, toolCommand, assessment.Reason)
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
) (Result, bool, bool, bool, error) {
	if e.agent == nil || !e.agent.SupportsToolCalling() {
		return Result{}, false, false, false, nil
	}
	if e.dispatcher == nil {
		return Result{}, false, false, false, nil
	}

	toolDefs := e.dispatcher.ToolDefinitions()
	if len(toolDefs) == 0 {
		return Result{}, false, false, false, nil
	}

	toolSucceeded := false
	reply, usedTools, err := e.agent.RespondWithTools(ctx, toolDefs, intent.Raw, func(ctx context.Context, call model.ToolCall) (string, error) {
		receipt, success, execErr := e.executeToolCallWithOutcome(ctx, call)
		if success {
			toolSucceeded = true
		}
		return receipt, execErr
	}, e.turnOptions)
	if err != nil {
		return Result{}, false, false, toolSucceeded, err
	}
	if onChunk != nil {
		if strings.TrimSpace(reply) != "" {
			onChunk(reply)
		}
	}
	return Result{
		Reply:    reply,
		Streamed: false,
		Status:   TurnStatusDone,
	}, true, usedTools, toolSucceeded, nil
}

func (e *Executor) executeInferredToolCandidatesLoop(
	ctx context.Context,
	rawInput string,
	candidates []InferredToolCandidate,
) (Result, bool, error) {
	if len(candidates) == 0 {
		return Result{}, false, nil
	}

	lines := []string{}
	status := TurnStatusDone
	var firstErr error
	for idx, candidate := range candidates {
		lineResult, handled, err := e.executeInferredTool(ctx, rawInput, candidate)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if idx == 0 {
				if lineResult.Status != "" {
					status = lineResult.Status
				} else {
					status = TurnStatusError
				}
			} else {
				status = TurnStatusError
			}
			lines = append(lines, lineResult.Reply)
			return Result{
				Reply:    strings.Join(lines, "\n"),
				Streamed: false,
				Status:   status,
			}, true, err
		}

		if !handled {
			continue
		}
		lines = append(lines, lineResult.Reply)
		if lineResult.Status == TurnStatusBlocked || lineResult.Status == TurnStatusError {
			return Result{
				Reply:    strings.Join(lines, "\n"),
				Streamed: false,
				Status:   lineResult.Status,
			}, true, err
		}
	}

	if len(lines) == 0 {
		return Result{}, false, nil
	}

	return Result{
		Reply:    strings.Join(lines, "\n"),
		Streamed: false,
		Status:   status,
	}, true, firstErr
}

func (e *Executor) executeInferredTool(
	ctx context.Context,
	rawInput string,
	candidate InferredToolCandidate,
) (Result, bool, error) {
	if strings.TrimSpace(candidate.MissingMessage) != "" {
		return Result{
			Reply:    candidate.MissingMessage,
			Streamed: false,
			Status:   TurnStatusBlocked,
		}, true, nil
	}

	name := strings.ToLower(strings.TrimSpace(candidate.Name))
	args := candidate.Args
	if args == nil {
		args = map[string]any{}
	}

	assessment := safety.AssessCommand(name, toolCallToArgs(name, args))
	if assessment.Risk == safety.RiskHigh {
		commandLine := name
		if path, ok := args["path"].(string); ok {
			path = strings.TrimSpace(path)
			if path != "" {
				commandLine += " " + path
			}
		}
		approved, confirmErr := e.approveOrDeny(ctx, commandLine, assessment.Reason)
		if confirmErr != nil {
			return Result{}, true, confirmErr
		}
		if !approved {
			blocked := e.newToolResultForTurn(name, commandLine, "execution blocked by user")
			summary, summarizeErr := e.summarizeToolExecution(ctx, rawInput, blocked, TurnStatusBlocked, nil)
			if summarizeErr != nil {
				return Result{
					Reply:    e.fallbackToolSummary(blocked, TurnStatusBlocked, nil),
					Streamed: false,
					Status:   TurnStatusBlocked,
				}, true, nil
			}
			return Result{
				Reply:    summary,
				Streamed: false,
				Status:   TurnStatusBlocked,
			}, true, nil
		}
	}

	output, handled, err := e.dispatcher.ExecuteTool(ctx, name, args)
	if !handled {
		return Result{
			Reply:    fmt.Sprintf("tool %q is not exposed", name),
			Streamed: false,
			Status:   TurnStatusError,
		}, true, fmt.Errorf("tool %q is not exposed to agent", name)
	}
	if err != nil || !output.Success || errors.Is(ctx.Err(), context.Canceled) {
		executionStatus := TurnStatusError
		if shouldUseDeterministicToolSummary(name) {
			return Result{
				Reply:    e.fallbackToolSummary(output, executionStatus, err),
				Streamed: false,
				Status:   executionStatus,
			}, true, nil
		}
		summary, summarizeErr := e.summarizeToolExecution(ctx, rawInput, output, executionStatus, err)
		if summarizeErr != nil {
			return Result{
				Reply:    e.fallbackToolSummary(output, executionStatus, err),
				Streamed: false,
				Status:   executionStatus,
			}, true, nil
		}
		return Result{
			Reply:    summary,
			Streamed: false,
			Status:   executionStatus,
		}, true, nil
	}

	if shouldUseDeterministicToolSummary(name) {
		return Result{
			Reply:    e.fallbackToolSummary(output, TurnStatusDone, nil),
			Streamed: false,
			Status:   TurnStatusDone,
		}, true, nil
	}
	summary, summarizeErr := e.summarizeToolExecution(ctx, rawInput, output, TurnStatusDone, nil)
	if summarizeErr != nil {
		return Result{
			Reply:    e.fallbackToolSummary(output, TurnStatusDone, nil),
			Streamed: false,
			Status:   TurnStatusDone,
		}, true, nil
	}
	return Result{
		Reply:    summary,
		Streamed: false,
		Status:   TurnStatusDone,
	}, true, nil
}

func (e *Executor) executeToolCall(ctx context.Context, call model.ToolCall) (string, error) {
	receipt, _, err := e.executeToolCallWithOutcome(ctx, call)
	return receipt, err
}

func (e *Executor) executeToolCallWithOutcome(ctx context.Context, call model.ToolCall) (string, bool, error) {
	args, parseErr := model.ParseToolArguments(call.Function.Arguments)
	if parseErr != nil {
		return "", false, parseErr
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
	if assessment.Risk == safety.RiskHigh {
		commandLine := name
		for _, rawArg := range toolCallToArgs(name, args) {
			if rawArg == "" {
				continue
			}
			commandLine += " " + rawArg
		}
		ok, confirmErr := e.approveOrDeny(ctx, commandLine, assessment.Reason)
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
	case "read", "delete":
		if raw, ok := args["path"].(string); ok {
			return []string{strings.TrimSpace(raw)}
		}
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
