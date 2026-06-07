package turn

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aetox-cli/internal/command"
	"aetox-cli/internal/safety"
	"aetox-cli/internal/skill"
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
	Respond(context.Context, string) (string, error)
	RespondStream(context.Context, string, func(string) error) (string, bool, error)
}

type Dispatcher interface {
	Execute(context.Context, string) (skill.Output, bool, error)
}

type ApprovalPromptFunc func(context.Context, string, string) (bool, error)

type Executor struct {
	agent          Agent
	dispatcher     Dispatcher
	commandSet     map[string]struct{}
	approve        ApprovalPromptFunc
	summaryTimeout time.Duration
	summaryLimit   int
}

type ExecutorOptions struct {
	Agent          Agent
	Dispatcher     Dispatcher
	CommandSet     map[string]struct{}
	Approve        ApprovalPromptFunc
	SummaryTimeout time.Duration
	SummaryLimit   int
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
	return &Executor{
		agent:          opts.Agent,
		dispatcher:     opts.Dispatcher,
		commandSet:     opts.CommandSet,
		approve:        opts.Approve,
		summaryTimeout: timeout,
		summaryLimit:   limit,
	}
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
	parsed := e.normalizeIntent(line, intent)

	if parsed.Kind == command.KindConversation {
		reply, streamed, err := e.agent.RespondStream(ctx, parsed.Raw, asStreamHandler(onChunk))
		return Result{
			Reply:    reply,
			Streamed: streamed,
			Status:   TurnStatusDone,
		}, err
	}

	return e.executeSkillTurn(ctx, line, parsed, onToolComplete)
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
		replyText, respondErr := e.agent.Respond(ctx, line)
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

func (e *Executor) summarizeToolExecution(
	ctx context.Context,
	originalInput string,
	result skill.Output,
	status TurnStatus,
	execErr error,
) (string, error) {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	output = e.sanitizeAndTrimOutput(output)
	if output == "" {
		output = "(no output)"
	}

	commandLine := strings.TrimSpace(result.Command)
	if commandLine == "" {
		commandLine = result.Name
	}

	errorLine := ""
	if strings.TrimSpace(result.Stderr) != "" {
		errorLine = fmt.Sprintf("\nTool error: %s", result.Stderr)
	} else if execErr != nil {
		errorLine = fmt.Sprintf("\nTool error: %s", execErr.Error())
	}

	summaryPrompt := fmt.Sprintf(
		"Original user request: %q\n"+
			"Tool: %s\n"+
			"Command: %s\n"+
			"Execution status: %s\n"+
			"DurationMs: %d\n"+
			"Output:\n%s\n%s\n\n"+
			"Respond in the same language as the user and be concise.\n"+
			"Start with an explicit status phrase for executed (%s), then summarize key result and mention completion.",
		originalInput,
		result.Name,
		commandLine,
		status,
		result.DurationMs,
		output,
		errorLine,
		status,
	)

	summaryCtx, cancel := context.WithTimeout(ctx, e.summaryTimeout)
	defer cancel()
	summary, err := e.agent.Respond(summaryCtx, summaryPrompt)
	if err != nil {
		return "", err
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", errors.New("empty summary response")
	}
	if len(summary) > e.summaryLimit {
		summary = summary[:e.summaryLimit] + "\n...(output truncated)"
	}
	return summary, nil
}

func (e *Executor) fallbackToolSummary(result skill.Output, status TurnStatus, execErr error) string {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	if output == "" {
		output = "(no output)"
	}
	output = e.sanitizeAndTrimOutput(output)
	if output == "" {
		output = "(no output)"
	}
	if execErr != nil && result.Stderr == "" {
		output = fmt.Sprintf("%s\nError: %s", output, execErr.Error())
	}

	prefix := "executed (done)"
	switch status {
	case TurnStatusError:
		prefix = "executed (error)"
	case TurnStatusBlocked:
		prefix = "executed (blocked)"
	}
	commandText := strings.TrimSpace(result.Command)
	if commandText != "" {
		commandText = fmt.Sprintf("command: %s. ", commandText)
	}
	return fmt.Sprintf("%s (summary fallback). %s%s", prefix, commandText, output)
}

func (e *Executor) sanitizeAndTrimOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "(no output)"
	}

	redactionRules := map[string]*regexp.Regexp{
		"api key":  regexp.MustCompile(`(?i)(api key\s*[:=]\s*)[^\s]+`),
		"token":    regexp.MustCompile(`(?i)(token\s*[:=]\s*)[^\s]+`),
		"password": regexp.MustCompile(`(?i)(password\s*[:=]\s*)[^\s]+`),
	}
	for _, re := range redactionRules {
		output = re.ReplaceAllString(output, "$1[REDACTED]")
	}

	if len(output) > e.summaryLimit {
		output = output[:e.summaryLimit] + "\n...(output truncated)"
	}
	return output
}

func (e *Executor) normalizeToolResult(result skill.Output) skill.Output {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	output = e.sanitizeAndTrimOutput(output)
	result.Content = output
	result.RawOutput = output
	return result
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

func (e *Executor) newToolResultForTurn(name, command, detail string) skill.Output {
	if strings.TrimSpace(name) == "" {
		name = "tool"
	}
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "(no output)"
	}
	return skill.Output{
		Name:       name,
		Command:    command,
		Content:    detail,
		RawOutput:  detail,
		Success:    false,
		Stderr:     detail,
		DurationMs: 0,
	}
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
