package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"aetox-cli/internal/cognitive"
	"aetox-cli/internal/command"
	"aetox-cli/internal/skill"
	"aetox-cli/internal/turn"
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
)

type App struct {
	agent           *cognitive.Agent
	console         Console
	showBanner      bool
	skillDispatcher skillDispatcher
	commandSet      map[string]struct{}
	autoApprove     bool
	turnExecutor    *turn.Executor
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

	commandSet := buildCommandSetFromDispatcher(opts.Dispatcher)
	a := &App{
		agent:           opts.Agent,
		console:         opts.Console,
		skillDispatcher: opts.Dispatcher,
		commandSet:      commandSet,
		showBanner:      opts.ShowBanner,
		autoApprove:     opts.AutoApprove,
		modelSwitcher:   opts.ModelSwitch,
		title:           strings.TrimSpace(opts.Title),
		version:         strings.TrimSpace(opts.Version),
		userInfo:        strings.TrimSpace(opts.UserInfo),
		modelStatus:     strings.TrimSpace(opts.ModelStatus),
		skillNames:      skillNames,
	}
	a.turnExecutor = turn.NewExecutor(turn.ExecutorOptions{
		Agent:      a.agent,
		Dispatcher: a.skillDispatcher,
		CommandSet: a.commandSet,
		Approve:    a.confirmApproval,
	})
	return a, nil
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

		intent := a.parseInputIntent(line)
		if intent.IsMeta {
			switch intent.Command {
			case "model":
				if err := a.switchModel(sigCtx); err != nil {
					a.console.Errorf("Model switch failed: %v\n", err)
				}
				a.printSeparator()
				a.printStatusBar()
				continue
			case "help", "h":
				a.showSlashHelp()
				a.printSeparator()
				a.printStatusBar()
				continue
			case ":help":
				a.showHelp()
				continue
			case ":clear":
				a.agent.ClearContext()
				a.console.Println("context cleared")
				continue
			case "exit", "quit", "bye", "logout", ":exit", ":quit":
				a.console.Println("bye")
				return nil
			}
		}

		if intent.IsSlash && intent.Command == "" {
			continue
		}

		if intent.IsSlash && intent.Kind == command.KindConversation && intent.Command != "" && !intent.IsMeta {
			a.console.Println("Unknown slash command. Use / for commands or type skill names directly.")
			a.showSlashHelp()
			a.printSeparator()
			a.printStatusBar()
			continue
		}

		select {
		case <-sigCtx.Done():
			a.console.Println()
			a.console.Println("bye")
			return nil
		default:
		}

		var stopThinking func()
		if intent.Kind == command.KindConversation {
			stopThinking = a.startThinkingIndicator("กำลังคิด...", ansiBrandBright, ansiSubtle)
		} else if intent.Kind == command.KindSkill {
			stopThinking = a.startThinkingIndicator("กำลังรัน...", ansiBrandBright, ansiSubtle)
		}

		streamed := false
		spinnerStopped := false
		var onChunk func(string)
		if intent.Kind == command.KindConversation {
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

		turnResult, err := a.turnExecutor.Execute(sigCtx, line, intent, onChunk, stopThinking)
		reply := strings.TrimSpace(turnResult.Reply)
		streamed = streamed || turnResult.Streamed
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
					a.console.Println("execution canceled")
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
	result, err := a.turnExecutor.Execute(ctx, line, a.parseInputIntent(line), nil, nil)
	return result.Reply, err
}

func (a *App) parseInputIntent(line string) command.Intent {
	return command.Parse(line, command.ParseTokens, a.commandSet)
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

func buildCommandSetFromDispatcher(dispatcher skillDispatcher) map[string]struct{} {
	if dispatcher == nil {
		return nil
	}
	named, ok := dispatcher.(namedDispatcher)
	if !ok {
		return nil
	}
	return command.BuildCommandSet(named.Names())
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
