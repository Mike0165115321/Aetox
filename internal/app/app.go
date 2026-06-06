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
	"unicode/utf8"

	"aetox-cli/internal/cognitive"
	"aetox-cli/internal/plan"
	"aetox-cli/internal/safety"
	"aetox-cli/internal/skill"
)

const (
	ansiReset      = "\x1b[0m"
	ansiBlueDark   = "\x1b[38;5;18m"
	ansiBlueMid    = "\x1b[38;5;33m"
	ansiBlueLight  = "\x1b[38;5;39m"
	ansiBlueBright = "\x1b[38;5;75m"
)

type App struct {
	agent           *cognitive.Agent
	console         Console
	showBanner      bool
	skillDispatcher skillDispatcher
	commandSet      map[string]struct{}
	autoApprove     bool

	title       string
	version     string
	userInfo    string
	modelStatus string
	skillNames  []string
}

type skillDispatcher interface {
	Execute(ctx context.Context, input string) (skill.Output, bool, error)
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

		line, err := a.console.ReadLine()
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
			if line == "" {
				if err := a.showSkillPalette(sigCtx); err != nil {
					a.console.Errorf("command failed: %v\n", err)
				}
				a.printSeparator()
				a.printShortcutBar()
				continue
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

		reply, err := a.runCommand(sigCtx, line)
		if err != nil {
			a.console.Errorf("Chat failed: %v\n", err)
			a.printSeparator()
			a.printShortcutBar()
			continue
		}

		a.console.Println(reply)
		a.printSeparator()
		a.printShortcutBar()
	}
}

func (a *App) runCommand(ctx context.Context, line string) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", errors.New("empty input")
	}

	intent := plan.Build(line, skill.ParseCommand, a.commandSet)
	if intent.Kind == plan.KindConversation {
		return a.agent.Respond(ctx, intent.Raw)
	}

	if a.requiresSkillApproval(ctx, intent.Command, intent.Args) {
		approved, confirmErr := a.confirmApproval(ctx, intent.Command)
		if confirmErr != nil {
			return "", confirmErr
		}
		if !approved {
			return "command blocked: approval required", nil
		}
	}

	reply, handled, err := a.dispatchBySkill(ctx, intent.Raw)
	if handled {
		return reply, nil
	}
	if err != nil {
		return "", err
	}
	return a.agent.Respond(ctx, intent.Raw)
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

func (a *App) dispatchBySkill(ctx context.Context, line string) (string, bool, error) {
	if a.skillDispatcher == nil {
		return "", false, nil
	}
	output, handled, err := a.skillDispatcher.Execute(ctx, line)
	if !handled || err != nil {
		return "", handled, err
	}
	return output.Content, true, nil
}

func (a *App) requiresSkillApproval(_ context.Context, name string, args []string) bool {
	assessment := safety.AssessCommand(name, args)
	return assessment.Risk == safety.RiskHigh
}

func (a *App) confirmApproval(ctx context.Context, name string) (bool, error) {
	if a.autoApprove {
		return true, nil
	}

	prompt := fmt.Sprintf("Aetox: command `%s` is high-risk, confirm? [y/N]: ", name)
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

func (a *App) showHelp() {
	a.console.Println("Tips:")
	a.console.Println("  - ask in natural language")
	a.console.Println("  - :clear    reset conversation context")
	a.console.Println("  - exit      leave terminal chat")
	a.console.Println("  - :help     quick command tips")
	a.console.Println("  - example: list")
	a.console.Println("  - safe shell commands are executed immediately; high-risk commands require confirmation")
	a.console.Println("  - use --yes to auto-approve command safety prompts")
}

func (a *App) PrintBanner() {
	a.console.Println(ansiBlueDark + "      " + "████╗ ███████╗████████╗ ██████╗ ██╗  ██╗     ██████╗██╗     ██╗" + ansiReset)
	a.console.Println(ansiBlueMid + "     " + "██╔══██╗██╔════╝╚══██╔══╝██╔═══██╗╚██╗██╔╝     ██╔════╝██║     ██║" + ansiReset)
	a.console.Println(ansiBlueLight + "     " + "███████║█████╗     ██║   ██║   ██║ ╚███╔╝      ██║     ██║     ██║" + ansiReset)
	a.console.Println(ansiBlueMid + "     " + "██╔══██║██╔══╝     ██║   ██║   ██║ ██╔██╗      ██║     ██║     ██║" + ansiReset)
	a.console.Println(ansiBlueBright + "     " + "██║  ██║███████╗   ██║   ╚██████╔╝██╔╝ ██╗     ╚██████╗███████╗██║" + ansiReset)
	a.console.Println(ansiBlueDark + "     " + "╚═╝  ╚═╝╚══════╝   ╚═╝    ╚═════╝ ╚═╝  ╚═╝      ╚═════╝╚══════╝╚═╝" + ansiReset)
	a.console.Println("")
	a.console.Println(ansiBlueBright + "             Aetox  " + ansiBlueLight + "CLI" + ansiReset)
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
	a.console.Println(strings.Repeat("─", 100))
}

func (a *App) printShortcutBar() {
	left := "? for shortcuts"
	right := a.getModelStatusLine()
	padding := 100 - utf8.RuneCountInString(left) - utf8.RuneCountInString(right)
	if padding < 1 {
		padding = 1
	}
	a.console.Println(left + strings.Repeat(" ", padding) + right)
}

func (a *App) showSkillPalette(ctx context.Context) error {
	output, handled, err := a.dispatchBySkill(ctx, "help")
	if err != nil {
		return err
	}
	if handled && output != "" {
		a.console.Println(output)
		return nil
	}

	if len(a.skillNames) == 0 {
		a.console.Println("No skills available.")
		return nil
	}
	a.console.Println("Available skills:")
	for _, name := range a.skillNames {
		a.console.Println("  /" + name)
	}
	return nil
}
