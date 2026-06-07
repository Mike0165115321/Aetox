package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"aetox-cli/internal/cognitive"
	"aetox-cli/internal/plan"
	"aetox-cli/internal/safety"
	"aetox-cli/internal/skill"
)

const (
	ansiReset       = "\x1b[0m"
	ansiEraseLine   = "\x1b[2K\r"
	ansiBrandDark   = "\x1b[38;5;31m"
	ansiBrandMid    = "\x1b[38;5;45m"
	ansiBrandLight  = "\x1b[38;5;87m"
	ansiBrandBright = "\x1b[38;5;117m"
	ansiText        = "\x1b[97m"
	ansiSubtle      = "\x1b[38;5;249m"

	toolSummaryTimeout      = 30 * time.Second
	toolSummaryPromptMaxLen = 4096

	toolStatusDone    = "done"
	toolStatusError   = "error"
	toolStatusBlocked = "blocked"
)

type App struct {
	agent           *cognitive.Agent
	console         Console
	showBanner      bool
	skillDispatcher skillDispatcher
	commandSet      map[string]struct{}
	autoApprove     bool
	modelSwitcher   modelSwitcher

	title       string
	version     string
	userInfo    string
	modelStatus string
	skillNames  []string
}

type modelSwitcher func(context.Context) (*cognitive.Agent, string, bool, error)

type skillDispatcher interface {
	Execute(ctx context.Context, input string) (skill.Output, bool, error)
}

type describeSkills interface {
	Snapshot() map[string]skill.Skill
}

type namedDispatcher interface {
	Names() []string
}

type Options struct {
	Agent       *cognitive.Agent
	Console     Console
	Dispatcher  skillDispatcher
	ShowBanner  bool
	AutoApprove bool

	Title       string
	Version     string
	UserInfo    string
	ModelStatus string
	ModelSwitch func(context.Context) (*cognitive.Agent, string, bool, error)
}

func NewApp(opts Options) (*App, error) {
	if opts.Agent == nil {
		return nil, errors.New("agent is required")
	}
	if opts.Console == nil {
		return nil, errors.New("console is required")
	}

	var skillNames []string
	if named, ok := opts.Dispatcher.(namedDispatcher); ok {
		skillNames = append(skillNames, named.Names()...)
		sort.Strings(skillNames)
	}

	return &App{
		agent:           opts.Agent,
		console:         opts.Console,
		skillDispatcher: opts.Dispatcher,
		commandSet:      buildCommandSetFromDispatcher(opts.Dispatcher),
		showBanner:      opts.ShowBanner,
		autoApprove:     opts.AutoApprove,
		modelSwitcher:   opts.ModelSwitch,
		title:           strings.TrimSpace(opts.Title),
		version:         strings.TrimSpace(opts.Version),
		userInfo:        strings.TrimSpace(opts.UserInfo),
		modelStatus:     strings.TrimSpace(opts.ModelStatus),
		skillNames:      skillNames,
	}, nil
}

func (a *App) RunOnce(ctx context.Context, message string) (string, error) {
	return a.runCommand(ctx, message)
}

func (a *App) RunInteractive(ctx context.Context) error {
	if a.showBanner {
		a.PrintBanner()
	}

	a.printSeparator()

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		a.console.Print("> ")

		line, err := a.readLineInteractive(sigCtx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				a.console.Println()
				return nil
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "/"))
			slashCommand, _ := skill.ParseCommand(line)
			if slashCommand == "" {
				continue
			}
			if line == "model" {
				if err := a.switchModel(sigCtx); err != nil {
					a.console.Errorf("Model switch failed: %v\n", err)
				}
				a.printSeparator()
				a.printStatusBar()
				continue
			}
			if line == "help" || line == "h" {
				a.showSlashHelp()
				a.printSeparator()
				a.printStatusBar()
				continue
			}
			if _, ok := a.commandSet[slashCommand]; !ok {
				switch slashCommand {
				case "exit", "quit", "bye", "logout", ":help", ":clear", ":exit", ":quit":
				default:
					a.console.Println("Unknown slash command. Use / for commands or type skill names directly.")
					a.showSlashHelp()
					a.printSeparator()
					a.printStatusBar()
					continue
				}
			}
		}

		switch line {
		case "exit", "quit", ":exit", ":quit", "bye", "logout":
			a.console.Println("bye")
			return nil
		case ":help":
			a.showHelp()
			continue
		case ":clear":
			a.agent.ClearContext()
			a.console.Println("context cleared")
			continue
		}

		select {
		case <-sigCtx.Done():
			a.console.Println()
			a.console.Println("bye")
			return nil
		default:
		}

		intent := plan.Build(line, skill.ParseCommand, a.commandSet)
		var stopThinking func()
		if intent.Kind == plan.KindConversation {
			stopThinking = a.startThinkingIndicator("กำลังคิด...", ansiBrandBright, ansiSubtle)
		} else if intent.Kind == plan.KindSkill {
			stopThinking = a.startThinkingIndicator("กำลังรัน...", ansiBrandBright, ansiSubtle)
		}

		streamed := false
		spinnerStopped := false
		var onChunk func(string)
		if intent.Kind == plan.KindConversation {
			onChunk = func(chunk string) {
				streamed = true
				if !spinnerStopped {
					spinnerStopped = true
					if stopThinking != nil {
						stopThinking()
						stopThinking = nil
					}
					a.console.Print(ansiBrandBright + "Aetox: " + ansiReset)
				}
				a.console.Print(chunk)
			}
		}

		reply, wasStreamed, err := a.runCommandWithStream(sigCtx, line, onChunk, stopThinking)
		streamed = streamed || wasStreamed
		if streamed {
			a.console.Println()
		}
		if stopThinking != nil {
			stopThinking()
			stopThinking = nil
		}

		if err != nil {
			if errors.Is(err, context.Canceled) {
				if strings.TrimSpace(reply) != "" {
					a.console.Println(reply)
				} else {
					a.console.Println(a.fallbackToolSummary(a.newToolResultForApp("tool", line, "execution canceled"), toolStatusError, err))
				}
			} else {
				a.console.Errorf("command failed: %v\n", err)
			}
			if errors.Is(sigCtx.Err(), context.Canceled) {
				a.console.Println("bye")
				return nil
			}
			a.printSeparator()
			a.printStatusBar()
			continue
		}

		if !streamed && strings.TrimSpace(reply) != "" {
			a.console.Println(ansiBrandBright + "Aetox: " + ansiReset + reply)
		}
		a.printSeparator()
		a.printStatusBar()
	}
}

func (a *App) showSlashHelp() {
	a.console.Println("Slash commands:")
	a.console.Println("  /model    switch model/provider")
	a.console.Println("  /help (/h) show available slash/skill commands")
	a.console.Println("  /exit, /quit, /bye, /logout  leave session")
	a.console.Println("  :help     show quick conversation help")
	a.console.Println("  :clear    reset conversation context")
	a.console.Println("")
	a.console.Println("Skills:")
	a.printAvailableSkills()
	a.console.Println("")
	a.console.Println("Flow contract:")
	a.console.Println("  - conversation: stream immediately")
	a.console.Println("  - skill: execute first, then return one final summary")
	a.console.Println("  - status: executed (done) | executed (error) | executed (blocked)")
}

func (a *App) runCommand(ctx context.Context, line string) (string, error) {
	reply, _, err := a.runCommandWithStream(ctx, line, nil, nil)
	return reply, err
}

func (a *App) summarizeToolExecution(ctx context.Context, originalInput string, result skill.Output, status string, execErr error) (string, error) {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	output = a.sanitizeAndTrimOutput(output)
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

	summaryCtx, cancel := context.WithTimeout(ctx, toolSummaryTimeout)
	defer cancel()
	summary, err := a.agent.Respond(summaryCtx, summaryPrompt)
	if err != nil {
		return "", err
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", errors.New("empty summary response")
	}
	if len(summary) > toolSummaryPromptMaxLen {
		summary = summary[:toolSummaryPromptMaxLen] + "\n...(output truncated)"
	}
	return summary, nil
}

func (a *App) fallbackToolSummary(result skill.Output, status string, execErr error) string {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	if output == "" {
		output = "(no output)"
	}
	output = a.sanitizeAndTrimOutput(output)
	if output == "" {
		output = "(no output)"
	}
	if execErr != nil && result.Stderr == "" {
		output = fmt.Sprintf("%s\nError: %s", output, execErr.Error())
	}

	prefix := "executed (done)"
	switch status {
	case toolStatusError:
		prefix = "executed (error)"
	case toolStatusBlocked:
		prefix = "executed (blocked)"
	}
	command := strings.TrimSpace(result.Command)
	if command != "" {
		command = fmt.Sprintf("command: %s. ", command)
	}
	return fmt.Sprintf("%s (summary fallback). %s%s", prefix, command, output)
}

func (a *App) sanitizeAndTrimOutput(output string) string {
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

	if len(output) > toolSummaryPromptMaxLen {
		output = output[:toolSummaryPromptMaxLen] + "\n...(output truncated)"
	}
	return output
}

func (a *App) normalizeToolResult(result skill.Output) skill.Output {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	output = a.sanitizeAndTrimOutput(output)
	result.Content = output
	result.RawOutput = output
	return result
}

func (a *App) switchModel(ctx context.Context) error {
	if a.modelSwitcher == nil {
		return errors.New("model switch is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	newAgent, status, ok, err := a.modelSwitcher(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if newAgent == nil {
		return errors.New("model switch returned empty agent")
	}

	a.agent = newAgent
	if strings.TrimSpace(status) != "" {
		a.modelStatus = strings.TrimSpace(status)
	}
	return nil
}

func (a *App) runCommandWithStream(ctx context.Context, line string, onChunk func(string), onToolComplete func()) (string, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false, errors.New("empty input")
	}

	intent := plan.Build(line, skill.ParseCommand, a.commandSet)
	if intent.Kind == plan.KindConversation {
		return a.agent.RespondStream(ctx, intent.Raw, asStreamHandler(onChunk))
	}

	notifyToolComplete := func() {
		if onToolComplete == nil {
			return
		}
		onToolComplete()
		onToolComplete = nil
	}

	toolCommand := strings.TrimSpace(strings.Join(append([]string{intent.Command}, intent.Args...), " "))
	if toolCommand == "" {
		toolCommand = intent.Raw
	}

	assessment := a.assessCommand(intent.Command, intent.Args)
	if assessment.Risk == safety.RiskHigh {
		approved, confirmErr := a.confirmApproval(ctx, toolCommand, assessment.Reason)
		if confirmErr != nil {
			notifyToolComplete()
			if errors.Is(confirmErr, context.Canceled) {
				cancelled := a.newToolResultForApp("tool", toolCommand, "execution canceled during confirmation")
				summary, summarizeErr := a.summarizeToolExecution(ctx, line, cancelled, toolStatusError, confirmErr)
				if summarizeErr != nil {
					return a.fallbackToolSummary(cancelled, toolStatusError, confirmErr), false, nil
				}
				return summary, false, nil
			}
			return "", false, confirmErr
		}
		if !approved {
			notifyToolComplete()
			blocked := a.newToolResultForApp("tool", toolCommand, "execution blocked by user approval")
			summary, summarizeErr := a.summarizeToolExecution(ctx, line, blocked, toolStatusBlocked, nil)
			if summarizeErr != nil {
				return a.fallbackToolSummary(blocked, toolStatusBlocked, nil), false, nil
			}
			return summary, false, nil
		}
	}

	reply, handled, err := a.dispatchBySkill(ctx, intent.Raw)
	if !handled {
		notifyToolComplete()
		replyText, respondErr := a.agent.Respond(ctx, line)
		if respondErr != nil {
			return "", false, respondErr
		}
		return replyText, false, nil
	}

	if err != nil && errors.Is(err, context.Canceled) {
		reply = a.newToolResultForApp("tool", toolCommand, "execution canceled")
	}

	notifyToolComplete()
	reply = a.normalizeToolResult(reply)
	executionStatus := toolStatusDone
	if err != nil || !reply.Success || errors.Is(ctx.Err(), context.Canceled) {
		executionStatus = toolStatusError
	}

	summary, summarizeErr := a.summarizeToolExecution(ctx, line, reply, executionStatus, err)
	if summarizeErr != nil {
		return a.fallbackToolSummary(reply, executionStatus, err), false, nil
	}

	return summary, false, nil
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

func buildCommandSetFromDispatcher(dispatcher skillDispatcher) map[string]struct{} {
	if dispatcher == nil {
		return nil
	}
	named, ok := dispatcher.(namedDispatcher)
	if !ok {
		return nil
	}
	return plan.BuildCommandSet(named.Names())
}

func (a *App) dispatchBySkill(ctx context.Context, line string) (skill.Output, bool, error) {
	if a.skillDispatcher == nil {
		return skill.Output{}, false, nil
	}
	output, handled, err := a.skillDispatcher.Execute(ctx, line)
	if !handled || err != nil {
		return output, handled, err
	}
	return output, true, nil
}

func (a *App) requiresSkillApproval(_ context.Context, name string, args []string) bool {
	assessment := safety.AssessCommand(name, args)
	return assessment.Risk == safety.RiskHigh
}

func (a *App) assessCommand(name string, args []string) safety.Assessment {
	return safety.AssessCommand(name, args)
}

func (a *App) confirmApproval(ctx context.Context, name, reason string) (bool, error) {
	if a.autoApprove {
		return true, nil
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "may modify or read state"
	}
	prompt := fmt.Sprintf("Aetox: command `%s` is high-risk (%s), confirm? [y/N]: ", name, reason)
	a.console.Print(prompt)

	for {
		decision, err := a.awaitDecision(ctx)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(decision)) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		default:
			a.console.Println("please type y or n")
			continue
		}
	}
}

func (a *App) awaitDecision(ctx context.Context) (string, error) {
	decision := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		line, err := a.console.ReadLine()
		if err != nil {
			errCh <- err
			return
		}
		decision <- line
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case line := <-decision:
		return line, nil
	}
}

func (a *App) startThinkingIndicator(message, color, fallbackColor string) func() {
	frames := []string{"|", "/", "-", "\\"}
	_ = color
	_ = fallbackColor

	stopped := make(chan struct{})
	finished := make(chan struct{})
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "working"
	}

	go func() {
		defer close(finished)
		ticker := time.NewTicker(140 * time.Millisecond)
		defer ticker.Stop()

		i := 0
		for {
			select {
			case <-stopped:
				return
			default:
			}
			a.console.Print(ansiEraseLine + ansiSubtle + msg + " " + frames[i%len(frames)] + ansiReset)
			i++
			select {
			case <-ticker.C:
			case <-stopped:
				return
			}
		}
	}()

	return func() {
		select {
		case <-stopped:
			return
		default:
			close(stopped)
			<-finished
			a.console.Print(ansiEraseLine)
		}
	}
}

func (a *App) newToolResultForApp(name, command, detail string) skill.Output {
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

func (a *App) showHelp() {
	a.console.Println("Tips:")
	a.console.Println("  - ask in natural language")
	a.console.Println("  - /model    switch model/provider")
	a.console.Println("  - :clear    reset conversation context")
	a.console.Println("  - exit      leave terminal chat")
	a.console.Println("  - :help     quick command tips")
	a.console.Println("  - example: list")
	a.console.Println("")
	a.console.Println("Flow contract:")
	a.console.Println("  - conversation input: stream immediately")
	a.console.Println("  - skill command: execute fully, then return summary")
	a.console.Println("  - tool status: executed (done) | executed (error) | executed (blocked)")
	a.console.Println("")
	a.console.Println("Approval policy:")
	a.console.Println("  - high-risk commands require confirmation")
	a.console.Println("  - safe-only actions for v1: git status|log|branch|diff|show, fs pwd|ls|find|cat, shell safe subset")
	a.console.Println("  - safe shell commands are executed immediately; high-risk commands require confirmation")
	a.console.Println("  - use --yes to auto-approve command safety prompts")
}

func (a *App) PrintBanner() {
	a.console.Println("")
	a.console.Println("")
	a.console.Println(ansiBrandDark + "      █████╗ ███████╗████████╗ ██████╗ ██╗  ██╗" + ansiReset)
	a.console.Println(ansiBrandMid + "     ██╔══██╗██╔════╝╚══██╔══╝██╔═══██╗██║  ██║" + ansiReset)
	a.console.Println(ansiBrandLight + "     ███████║█████╗     ██║   ██║   ██║╚██╗██╔╝" + ansiReset)
	a.console.Println(ansiBrandBright + "     ██╔══██║██╔══╝     ██║   ██║   ██║ ╚███╔╝ " + ansiReset)
	a.console.Println(ansiBrandMid + "     ██║  ██║███████╗   ██║   ╚██████╔╝  ╚███╔╝ " + ansiReset)
	a.console.Println(ansiBrandDark + "     ╚═╝  ╚═╝╚══════╝   ╚═╝    ╚═════╝    ╚═╝  " + ansiReset)
	a.console.Println("")
	a.console.Println("")
	a.console.Println(ansiBrandBright + "         Aetox " + ansiText + "CLI" + ansiReset)
	a.console.Println("")
	a.console.Println(ansiSubtle + "  User: " + ansiText + a.userInfoLine() + ansiReset)
	a.console.Println(ansiSubtle + "  Model: " + ansiText + a.modelStatus + ansiReset)
	a.console.Println("")
	a.console.Println(ansiReset)
}
func (a *App) userInfoLine() string {
	if a.userInfo == "" {
		return "local user"
	}
	return a.userInfo
}

func (a *App) getModelStatusLine() string {
	if a.modelStatus == "" {
		return "noop (local)"
	}
	return a.modelStatus
}

func (a *App) printSeparator() {
	a.console.Println(strings.Repeat("═", 92))
}

func (a *App) printStatusBar() {
	left := "Aetox CLI"
	right := a.getModelStatusLine()
	padding := 100 - utf8.RuneCountInString(left) - utf8.RuneCountInString(right)
	if padding < 1 {
		padding = 1
	}
	a.console.Println(ansiSubtle + left + ansiReset + strings.Repeat(" ", padding) + ansiText + right + ansiReset)
}

func (a *App) showSkillPalette(ctx context.Context) error {
	a.showSlashHelp()
	_, handled, err := a.dispatchBySkill(ctx, "help")
	if err != nil {
		a.console.Println("command failed: " + err.Error())
		return nil
	}
	if handled {
		a.console.Println("")
	}

	if len(a.skillNames) == 0 {
		a.console.Println("No extra skills registered.")
	}

	return nil
}

func (a *App) printAvailableSkills() {
	describe := map[string]skill.Skill{}
	if source, ok := a.skillDispatcher.(describeSkills); ok {
		describe = source.Snapshot()
	} else {
		for _, name := range a.skillNames {
			// keep compatibility if dispatcher only exposes names
			describe[name] = nil
		}
	}

	names := make([]string, 0, len(describe))
	for name := range describe {
		if name == "" {
			continue
		}
		names = append(names, strings.ToLower(name))
	}
	sort.Strings(names)

	if len(names) == 0 {
		a.console.Println("  (none)")
		return
	}

	for _, name := range names {
		desc := "no description"
		if describe[name] != nil {
			desc = strings.TrimSpace(describe[name].Description())
			if desc == "" {
				desc = "no description"
			}
		}
		a.console.Println(fmt.Sprintf("  %-8s %s", "/"+name, desc))
	}
}

