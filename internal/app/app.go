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
				a.console.Println("เคลียร์บริบทแล้ว")
				continue
			case "exit", "quit", "bye", "logout", ":exit", ":quit":
				a.console.Println("bye")
				return nil
			}
		}

		if intent.IsSlash && intent.Command == "" {
			a.printSeparator()
			a.showSlashHelp()
			a.printSeparator()
			a.printStatusBar()
			continue
		}

		if intent.IsSlash && intent.Kind == command.KindConversation && intent.Command != "" && !intent.IsMeta {
			a.console.Println("ไม่รู้จักคำสั่ง /" + intent.Command)
			a.console.Println("พิมพ์ / เพื่อดูคำสั่งทั้งหมด")
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
			a.console.Println(ansiBrandBright + "Aetox: " + ansiReset + "กำลังทำงานเครื่องมือ...")
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
					a.console.Println("ยกเลิกการทำงาน")
				}
			} else {
				a.console.Errorf("คำสั่งล้มเหลว: %v\n", err)
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
	a.console.Println("  /model        เปลี่ยนโมเดล/provider")
	a.console.Println("  /help (/h)    แสดงความช่วยเหลือโดยย่อ")
	a.console.Println("  /exit         ออกจากโปรแกรม")
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
		reason = "อาจมีการเปลี่ยนแปลงหรืออ่านสถานะระบบ"
	}
	prompt := fmt.Sprintf("Aetox: คำสั่ง `%s` มีความเสี่ยงสูง (%s) ยืนยันหรือไม่? [y/N]: ", name, reason)
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
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	stopped := make(chan struct{})
	finished := make(chan struct{})

	baseMsg := strings.TrimRight(strings.TrimSpace(message), ".")
	if baseMsg == "" {
		baseMsg = "กำลังคิด"
	}

	go func() {
		defer close(finished)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		i := 0
		for {
			select {
			case <-stopped:
				return
			default:
			}

			dots := strings.Repeat(".", (i/3)%4)
			padding := strings.Repeat(" ", 3-(i/3)%4)

			a.console.Print(ansiEraseLine + color + frames[i%len(frames)] + " " + fallbackColor + baseMsg + dots + padding + ansiReset)
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
	a.console.Println("  - พิมพ์เป็นภาษาปกติ")
	a.console.Println("  - /model    เปลี่ยนโมเดล/โปรไฟเดอร์")
	a.console.Println("  - :clear    เคลียร์บริบทการสนทนา")
	a.console.Println("  - exit      ออกจากโหมดเทอร์มินัลแชต")
	a.console.Println("  - :help     แสดงเคล็ดลับการใช้คำสั่งสั้น")
	a.console.Println("  - ตัวอย่าง: list")
	a.console.Println("")
	a.console.Println("สัญญาการทำงาน:")
	a.console.Println("  - การสนทนาทั่วไป: แสดงผลตอบทันที")
	a.console.Println("  - คำสั่ง skill: รันเสร็จแล้วสรุปผล")
	a.console.Println("  - สถานะเครื่องมือ: executed (done) | executed (error) | executed (blocked)")
	a.console.Println("")
	a.console.Println("Approval policy:")
	a.console.Println("  - คำสั่งเสี่ยงสูง: ต้องยืนยันก่อนทำ")
	a.console.Println("  - v1 ปัจจุบันอนุมัติแต่คำสั่งปลอดภัย: git status|log|branch|diff|show, fs pwd|ls|find|cat, shell safe subset")
	a.console.Println("  - shell แบบปลอดภัยรันได้ทันที, คำสั่งเสี่ยงสูงต้องยืนยัน")
	a.console.Println("  - ใช้ --yes เพื่ออนุมัติ prompt ความปลอดภัยอัตโนมัติ")
}

func (a *App) PrintBanner() {
	a.console.Println("")
	a.console.Println("")
	a.console.Println(ansiBrandDark + "      █████╗ ███████╗████████╗ ██████╗ ██╗  ██╗" + ansiReset)
	a.console.Println(ansiBrandMid + "     ██╔══██╗██╔════╝╚══██╔══╝██╔═══██╗╚██╗██╔╝" + ansiReset)
	a.console.Println(ansiBrandLight + "     ███████║█████╗     ██║   ██║   ██║ ╚███╔╝ " + ansiReset)
	a.console.Println(ansiBrandBright + "     ██╔══██║██╔══╝     ██║   ██║   ██║ ██╔██╗ " + ansiReset)
	a.console.Println(ansiBrandMid + "     ██║  ██║███████╗   ██║   ╚██████╔╝██╔╝╚██╗" + ansiReset)
	a.console.Println(ansiBrandDark + "     ╚═╝  ╚═╝╚══════╝   ╚═╝    ╚═════╝ ╚═╝  ╚═╝" + ansiReset)
	a.console.Println("")
	a.console.Println("")
	a.console.Println(ansiBrandBright + "         Aetox " + ansiText + "CLI" + ansiReset)
	a.console.Println("")
	a.console.Println(ansiSubtle + "  User: " + ansiText + a.userInfoLine() + ansiReset)
	a.console.Println(ansiSubtle + "  Model: " + ansiText + a.getModelStatusLine() + ansiReset)
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
	status := strings.TrimSpace(a.modelStatus)
	if status == "" {
		status = "noop (local)"
	}
	return status
}

func (a *App) getContextStatusLine() string {
	if a.agent == nil {
		return ""
	}

	usage := a.agent.LastUsage()
	if usage.TotalTokenCount() > 0 {
		_, _, maxChars := a.agent.ContextUsage()
		maxTokens := 0
		if maxChars > 0 {
			maxTokens = (maxChars + 3) / 4
		}
		if maxTokens > 0 {
			return fmt.Sprintf("context %d/%d tokens", usage.TotalTokenCount(), maxTokens)
		}
		return fmt.Sprintf("context %d tokens", usage.TotalTokenCount())
	}

	_, usedChars, maxChars := a.agent.ContextUsage()
	if maxChars <= 0 {
		return ""
	}
	return fmt.Sprintf("context %d/%d tokens", (usedChars+3)/4, (maxChars+3)/4)
}

func (a *App) printSeparator() {
	a.console.Println(strings.Repeat("═", 92))
}

func (a *App) printStatusBar() {
	left := "Aetox CLI"
	right := a.getContextStatusLine()
	padding := 100 - utf8.RuneCountInString(left) - utf8.RuneCountInString(right)
	if padding < 1 {
		padding = 1
	}
	line := ansiSubtle + left + ansiReset
	if right != "" {
		line += strings.Repeat(" ", padding) + ansiText + right + ansiReset
	}
	a.console.Println(line)
}

func (a *App) showSkillPalette(ctx context.Context) error {
	a.showSlashHelp()
	_, handled, err := a.dispatchBySkill(ctx, "help")
	if err != nil {
		a.console.Println("คำสั่งล้มเหลว: " + err.Error())
		return nil
	}
	if handled {
		a.console.Println("")
	}

	if len(a.skillNames) == 0 {
		a.console.Println("ยังไม่มี skill เพิ่มเติมในระบบ.")
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
		a.console.Println("  (ไม่มี)")
		return
	}

	for _, name := range names {
		desc := "ไม่มีคำอธิบาย"
		if describe[name] != nil {
			desc = strings.TrimSpace(describe[name].Description())
			if desc == "" {
				desc = "ไม่มีคำอธิบาย"
			}
		}
		a.console.Println(fmt.Sprintf("  %-8s %s", "/"+name, desc))
	}
}
