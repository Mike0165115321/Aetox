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

	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"

	"sync"
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
	statusLineWidth = 140
)

type App struct {
	agent            *cognitive.Agent
	console          Console
	showBanner       bool
	skillDispatcher  skillDispatcher
	commandSet       map[string]struct{}
	approvalMode     safety.ApprovalMode
	permissions      safety.PermissionConfig
	onApprovalChange func(safety.ApprovalMode)
	turnExecutor     *turn.Executor
	modelSwitcher    modelSwitcher

	title              string
	version            string
	userInfo           string
	modelStatus        string
	modelContextTokens int
	thinkLevel         think.Level
	skillNames         []string

	statusReporter     func(string)
	lastPrintedTool    string
	toolActionListener func(action, detail string)
}

type ModelSwitchResult struct {
	Agent              *cognitive.Agent
	ModelStatus        string
	ModelContextTokens int
	ThinkLevel         think.Level
	Changed            bool
}

type modelSwitcher func(context.Context) (ModelSwitchResult, error)

type skillDispatcher interface {
	Execute(ctx context.Context, input string) (skill.Output, bool, error)
	ToolDefinitions() []model.ToolDefinition
	ExecuteTool(ctx context.Context, name string, args map[string]any) (skill.Output, bool, error)
}

type describeSkills interface {
	Snapshot() map[string]skill.Skill
}

type namedDispatcher interface {
	Names() []string
}

type Options struct {
	Agent            *cognitive.Agent
	Console          Console
	Dispatcher       skillDispatcher
	ShowBanner       bool
	ApprovalMode     safety.ApprovalMode
	Permissions      safety.PermissionConfig
	OnApprovalChange func(safety.ApprovalMode)
	// OnToolAction, if set, is notified of every tool call/result this session
	// runs (e.g. for a UI command-history panel). Nil means silent, as before.
	OnToolAction func(action, detail string)
	// StatusReporter, if set, receives human-readable turn-progress messages
	// ("กำลังคิดคำตอบ...", "กำลังรันเครื่องมือ...") as the turn executor moves
	// through phases, and a final "" when the turn completes. CLI wires its own
	// per-turn spinner via wireStatusReporter instead of this field; this is for
	// callers (e.g. the desktop app) that want one stable callback for the
	// session's lifetime, e.g. to relay it to a UI as a live status/typing indicator.
	StatusReporter func(string)

	Title              string
	Version            string
	UserInfo           string
	ModelStatus        string
	ModelContextTokens int
	ThinkLevel         think.Level
	ModelSwitch        func(context.Context) (ModelSwitchResult, error)
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
		agent:              opts.Agent,
		console:            opts.Console,
		skillDispatcher:    opts.Dispatcher,
		commandSet:         commandSet,
		showBanner:         opts.ShowBanner,
		approvalMode:       normalizeApprovalMode(opts.ApprovalMode),
		permissions:        opts.Permissions,
		onApprovalChange:   opts.OnApprovalChange,
		modelSwitcher:      opts.ModelSwitch,
		title:              strings.TrimSpace(opts.Title),
		version:            strings.TrimSpace(opts.Version),
		userInfo:           strings.TrimSpace(opts.UserInfo),
		modelStatus:        strings.TrimSpace(opts.ModelStatus),
		modelContextTokens: opts.ModelContextTokens,
		thinkLevel:         think.NormalizeLevel(string(opts.ThinkLevel)),
		skillNames:         skillNames,
		toolActionListener: opts.OnToolAction,
	}
	a.turnExecutor = turn.NewExecutor(turn.ExecutorOptions{
		Agent:          a.agent,
		Dispatcher:     a.skillDispatcher,
		CommandSet:     a.commandSet,
		Approve:        a.confirmApproval,
		ApprovalMode:   a.approvalMode,
		Permissions:    a.permissions,
		OnToolAction:   a.onToolAction,
		StatusReporter: opts.StatusReporter,
		TurnOptions: turn.TurnOptions{
			ThinkLevel: a.thinkLevel,
		},
	})
	return a, nil
}

func (a *App) wireStatusReporter() {
	if a.statusReporter == nil {
		return
	}
	a.turnExecutor = turn.NewExecutor(turn.ExecutorOptions{
		Agent:          a.agent,
		Dispatcher:     a.skillDispatcher,
		CommandSet:     a.commandSet,
		Approve:        a.confirmApproval,
		StatusReporter: a.statusReporter,
		ApprovalMode:   a.approvalMode,
		Permissions:    a.permissions,
		OnToolAction:   a.onToolAction,
		TurnOptions: turn.TurnOptions{
			ThinkLevel: a.thinkLevel,
		},
	})
}

func (a *App) RunOnce(ctx context.Context, message string) (string, error) {
	return a.runCommand(ctx, message)
}

// RunOnceStream is RunOnce with live callbacks for the reply text and (when
// the provider streams it) the model's reasoning/thinking tokens as they're
// produced — token-by-token for a plain conversational turn, or once with the
// full text for a turn that went through tool calls, since that path has no
// partial text to stream. Same callback either way: the desktop UI just
// appends whatever it's given to a growing bubble.
func (a *App) RunOnceStream(ctx context.Context, message string, onChunk func(string), onReasoningChunk func(string)) (string, error) {
	result, err := a.turnExecutor.Execute(ctx, message, a.parseInputIntent(message), onChunk, onReasoningChunk, nil)
	return result.Reply, err
}

func (a *App) onToolAction(action, detail string) {
	if a.toolActionListener != nil {
		a.toolActionListener(action, detail)
	}
}

func (a *App) RunInteractive(ctx context.Context) error {
	if a.showBanner {
		a.PrintBanner()
	}

	a.printSeparator()

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		a.printPromptLine()

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
			case "approval":
				a.handleApprovalCommand(line)
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

		a.statusReporter = nil
		thinkingMessage := a.thinkingStatusMessage(intent.Kind)
		var stopThinking func(string)
		if intent.Kind == command.KindConversation {
			stopThinking = a.startThinkingIndicator(thinkingMessage, ansiBrandBright, ansiSubtle)
			a.statusReporter = stopThinking
		} else if intent.Kind == command.KindSkill {
			a.console.Println(ansiBrandBright + "Aetox: " + ansiReset + thinkingMessage)
			stopThinking = a.startThinkingIndicator(thinkingMessage, ansiBrandBright, ansiSubtle)
			a.statusReporter = stopThinking
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
						stopThinking("")
						stopThinking = nil
					}
					a.console.Print(ansiBrandBright + "Aetox: " + ansiReset)
				}
				a.console.Print(chunk)
			}
		}

		onToolComplete := func() {
			if stopThinking != nil {
				stopThinking("")
				stopThinking = nil
			}
		}
		a.wireStatusReporter()
		// CLI has no reasoning-stream UI yet — nil is safe (StreamComplete
		// implementations skip the callback entirely when it's nil).
		turnResult, err := a.turnExecutor.Execute(sigCtx, line, intent, onChunk, nil, onToolComplete)
		reply := strings.TrimSpace(turnResult.Reply)
		streamed = streamed || turnResult.Streamed
		if streamed {
			a.console.Println()
		}
		if stopThinking != nil {
			stopThinking("")
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

func (a *App) thinkingStatusMessage(kind command.Kind) string {
	if kind == command.KindConversation {
		if a.thinkLevel == think.LevelNoThinking {
			return "กำลังตอบกลับ..."
		}
		return "กำลังคิด..."
	}

	if a.thinkLevel == think.LevelNoThinking {
		return "กำลังประมวลผลคำสั่ง..."
	}
	return "กำลังรัน..."
}

func (a *App) showSlashHelp() {
	a.console.Println("Slash commands:")
	a.console.Println("  /model        เปลี่ยนโมเดล/provider")
	a.console.Println("  /approval     แสดงหรือเปลี่ยนโหมดอนุมัติ (ถามก่อน/คำสั่งเสี่ยง/รันเต็มที่)")
	a.console.Println("  /help (/h)    แสดงความช่วยเหลือโดยย่อ")
	a.console.Println("  /exit         ออกจากโปรแกรม")
}

func (a *App) runCommand(ctx context.Context, line string) (string, error) {
	result, err := a.turnExecutor.Execute(ctx, line, a.parseInputIntent(line), nil, nil, nil)
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

	result, err := a.modelSwitcher(ctx)
	if err != nil {
		return err
	}
	if !result.Changed {
		return nil
	}
	if result.Agent == nil {
		return errors.New("model switch returned empty agent")
	}

	a.agent = result.Agent
	if strings.TrimSpace(result.ModelStatus) != "" {
		a.modelStatus = strings.TrimSpace(result.ModelStatus)
	}
	a.modelContextTokens = result.ModelContextTokens
	a.thinkLevel = think.NormalizeLevel(string(result.ThinkLevel))
	a.turnExecutor = turn.NewExecutor(turn.ExecutorOptions{
		Agent:        a.agent,
		Dispatcher:   a.skillDispatcher,
		CommandSet:   a.commandSet,
		Approve:      a.confirmApproval,
		ApprovalMode: a.approvalMode,
		Permissions:  a.permissions,
		TurnOptions: turn.TurnOptions{
			ThinkLevel: a.thinkLevel,
		},
	})
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

func normalizeApprovalMode(mode safety.ApprovalMode) safety.ApprovalMode {
	if mode == "" {
		return safety.ApprovalAsk
	}
	return mode
}

func (a *App) confirmApproval(ctx context.Context, name, reason string) (bool, error) {
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

func (a *App) handleApprovalCommand(line string) {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 2 || strings.ToLower(parts[0]) != "/approval" {
		a.pickApprovalMode()
		return
	}
	a.applyApprovalMode(parts[1])
}

func (a *App) pickApprovalMode() {
	modes := []struct {
		label string
		mode  safety.ApprovalMode
	}{
		{"ถามก่อน — ยืนยันทุกคำสั่งเสี่ยง", safety.ApprovalAsk},
		{"คำสั่งเสี่ยง — ถามเฉพาะ destructive", safety.ApprovalUnsafeOnly},
		{"รันเต็มที่ — ไม่อนุมัติใด ๆ", safety.ApprovalFullAccess},
	}

	currentLabel := approvalLabelThai(a.approvalMode)
	a.console.Println("เลือกโหมดอนุมัติ (ปัจจุบัน: " + currentLabel + "):")
	for i, m := range modes {
		a.console.Printf("  %d) %s\n", i+1, m.label)
	}
	a.console.Print("เลือก [1-3]: ")

	line, err := a.console.ReadLine()
	if err != nil {
		return
	}
	line = strings.TrimSpace(line)
	switch line {
	case "1":
		a.applyApprovalMode("ask")
	case "2":
		a.applyApprovalMode("unsafe-only")
	case "3":
		a.applyApprovalMode("full-access")
	default:
		a.console.Println("ยกเลิก")
	}
}

func (a *App) applyApprovalMode(modeArg string) {
	modeArg = strings.ToLower(strings.TrimSpace(modeArg))
	switch modeArg {
	case "ask", "unsafe-only", "full-access", "ถามก่อน", "ถาม", "คำสั่งเสี่ยง", "เสี่ยง", "รันเต็มที่", "เต็มที่", "ไม่ถาม":
		normalized := modeArg
		switch modeArg {
		case "ถามก่อน", "ถาม":
			normalized = "ask"
		case "คำสั่งเสี่ยง", "เสี่ยง":
			normalized = "unsafe-only"
		case "รันเต็มที่", "เต็มที่", "ไม่ถาม":
			normalized = "full-access"
		}
		a.approvalMode = safety.ApprovalMode(normalized)
		a.turnExecutor = turn.NewExecutor(turn.ExecutorOptions{
			Agent:        a.agent,
			Dispatcher:   a.skillDispatcher,
			CommandSet:   a.commandSet,
			Approve:      a.confirmApproval,
			ApprovalMode: a.approvalMode,
			Permissions:  a.permissions,
			TurnOptions: turn.TurnOptions{
				ThinkLevel: a.thinkLevel,
			},
		})
		if a.onApprovalChange != nil {
			a.onApprovalChange(a.approvalMode)
		}
		a.console.Println("เปลี่ยนโหมดอนุมัติเป็น: " + approvalLabelThai(a.approvalMode))
	default:
		a.console.Println("โหมดไม่ถูกต้อง ใช้: /approval ถามก่อน, /approval คำสั่งเสี่ยง, /approval รันเต็มที่")
		a.showApprovalStatus()
	}
}

func approvalLabelThai(mode safety.ApprovalMode) string {
	switch mode {
	case safety.ApprovalAsk:
		return "ถามก่อน"
	case safety.ApprovalUnsafeOnly:
		return "คำสั่งเสี่ยง"
	case safety.ApprovalFullAccess:
		return "รันเต็มที่"
	default:
		return string(mode)
	}
}

func (a *App) showApprovalStatus() {
	a.console.Println("อนุมัติ: " + approvalLabelThai(a.approvalMode))
	a.console.Println("ใช้ /approval ถามก่อน | /approval คำสั่งเสี่ยง | /approval รันเต็มที่ เพื่อเปลี่ยน")
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

func (a *App) startThinkingIndicator(message, color, fallbackColor string) func(string) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	stopped := make(chan struct{})
	finished := make(chan struct{})

	var mu sync.Mutex
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

			mu.Lock()
			msg := baseMsg
			mu.Unlock()

			dots := strings.Repeat(".", (i/3)%4)
			padding := strings.Repeat(" ", 3-(i/3)%4)

			a.console.Print(ansiEraseLine + color + frames[i%len(frames)] + " " + fallbackColor + msg + dots + padding + ansiReset)
			i++

			select {
			case <-ticker.C:
			case <-stopped:
				return
			}
		}
	}()

	return func(newMsg string) {
		if newMsg != "" {
			mu.Lock()
			baseMsg = strings.TrimRight(strings.TrimSpace(newMsg), ".")
			mu.Unlock()
			return
		}
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
	a.console.Println("  - /approval เปลี่ยนโหมดอนุมัติ (ถามก่อน/คำสั่งเสี่ยง/รันเต็มที่)")
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
	a.console.Println("  - โหมดอนุมัติมี 3 ระดับ: ถามก่อน, คำสั่งเสี่ยง, รันเต็มที่")
	a.console.Println("  - ถามก่อน: ยืนยันทุกคำสั่งเสี่ยง (ค่าเริ่มต้น)")
	a.console.Println("  - คำสั่งเสี่ยง: ถามเฉพาะคำสั่ง destructive, เปลี่ยน git, shell, หรือนอก workspace")
	a.console.Println("  - รันเต็มที่: ไม่อนุมัติใด ๆ ทั้งสิ้น")
	a.console.Println("  - เปลี่ยนด้วย /approval <mode>")
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
	a.console.Println(ansiSubtle + "  User:   " + ansiText + a.userInfoLine() + ansiReset)
	a.console.Println(ansiSubtle + "  Model:  " + ansiText + a.getModelStatusLine() + ansiReset)
	a.console.Println(ansiSubtle + "  อนุมัติ: " + ansiText + approvalLabelThai(a.approvalMode) + ansiSubtle + " (เปลี่ยนด้วย /approval)" + ansiReset)
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
	contextLimit := a.modelContextTokens
	if a.agent == nil {
		if contextLimit > 0 {
			return fmt.Sprintf("context 0/%d tokens", contextLimit)
		}
		return ""
	}

	_, usedChars, maxChars := a.agent.ContextUsage()
	usedTokens := (usedChars + 3) / 4
	if maxChars <= 0 && contextLimit <= 0 {
		return fmt.Sprintf("context %d tokens", usedTokens)
	}
	if contextLimit > 0 {
		return fmt.Sprintf("context %d/%d tokens", usedTokens, contextLimit)
	}
	if maxChars > 0 {
		return fmt.Sprintf("context %d tokens", usedTokens)
	}
	return fmt.Sprintf("context %d tokens", usedTokens)
}

func (a *App) printSeparator() {
	a.console.Println(strings.Repeat("═", 92))
}

func renderAlignedStatusLine(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if right == "" {
		return left
	}
	padding := statusLineWidth - utf8.RuneCountInString(left) - utf8.RuneCountInString(right)
	if padding < 1 {
		padding = 1
	}
	return left + strings.Repeat(" ", padding) + right
}

func (a *App) renderHeaderStatusLine() string {
	left := strings.TrimSpace(a.title)
	if left == "" {
		left = "Aetox CLI"
	}
	return renderAlignedStatusLine(left, a.getModelStatusLine())
}

func (a *App) renderPromptStatusLine() string {
	return renderAlignedStatusLine(">", a.getContextStatusLine())
}

func (a *App) printStatusBar() {
	line := ansiSubtle + strings.TrimSpace(a.title) + ansiReset
	if strings.TrimSpace(a.title) == "" {
		line = ansiSubtle + "Aetox CLI" + ansiReset
	}
	right := strings.TrimSpace(a.getModelStatusLine())
	approvalLabel := "อนุมัติ: " + approvalLabelThai(a.approvalMode)
	if right != "" {
		right = right + "  " + ansiSubtle + approvalLabel + ansiReset
	}
	if right != "" {
		plain := a.renderHeaderStatusLine()
		leftText := strings.TrimSpace(a.title)
		if leftText == "" {
			leftText = "Aetox CLI"
		}
		padding := strings.TrimPrefix(plain, leftText)
		line = ansiSubtle + leftText + ansiReset + padding[:len(padding)-len(right)] + right
	}
	a.console.Println(line)
}

func (a *App) printPromptLine() {
	right := strings.TrimSpace(a.getContextStatusLine())
	if right == "" {
		a.console.Print("> ")
		return
	}
	plain := a.renderPromptStatusLine()
	padding := strings.TrimPrefix(plain, ">")
	spacePad := padding[:len(padding)-len(right)]
	a.console.Print(ansiBrandBright + ">" + ansiReset + spacePad + ansiSubtle + right + ansiReset + "\r" + ansiBrandBright + "> " + ansiReset)
}

