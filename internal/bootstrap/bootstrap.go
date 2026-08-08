package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	aetoxapp "github.com/Mike0165115321/Aetox/internal/app"
	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/hook"
	"github.com/Mike0165115321/Aetox/internal/learned"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/prompt"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/stt"
	"github.com/Mike0165115321/Aetox/internal/subagent"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// deferredConnectTimeout bounds bringing up a server that waited for the agent
// that needs it (mcp.Server.Deferred). Generous, because it can be a cold `npx`
// fetching a package; bounded, because the user is looking at a door that has
// not opened yet.
const deferredConnectTimeout = 30 * time.Second

// Options are the parts of an engine a host supplies. Everything else comes
// from config.Config, so two hosts on the same config get the same engine.
//
// The callbacks are deliberately all optional except Approve: a host that draws
// nothing still gets a working engine, which is what a server needs. They are
// also deliberately NOT split into "desktop" and "headless" sets — an HTTP host
// streaming over SSE wants OnStatus and OnContentPreview for exactly the reason
// a window does.
// SpaceContext is where a โปรเจกต์ keeps the files every session inside it
// should know about, and which files those are right now. The list is names
// only — the host reads the folder, the assistant reads the files it needs with
// the tools it already has, and neither the prompt nor this struct ever carries
// their contents.
type SpaceContext struct {
	Path  string
	Files []string
}

type Options struct {
	// Surface selects the identity sentence in the system prompt
	// (internal/prompt.Build). Zero value means prompt.SurfaceDesktop, which is
	// what every graphical and headless host wants; only a terminal REPL is
	// SurfaceCLI.
	Surface prompt.Surface

	// Console is handed to app.NewApp, which requires a non-nil one. Nil means
	// app.NewStdIO(). A process with no terminal must pass DiscardConsole().
	Console aetoxapp.Console

	// Mode is the desk the session being built was opened at (ARCHITECTURE.md
	// §83). It decides three things and nothing else: which of the one shared
	// registry the dispatcher shows, what direction and which memory scope the
	// system prompt gains, and the ceiling a sub-agent runs under.
	//
	// Nil is the full desk — every session from before modes existed, plus every
	// host that has none (the CLI) — and it produces byte-for-byte the engine
	// this function produced before desks were a thing.
	//
	// It never grants anything: the safety gate, the approval rules and the
	// permission list are identical at every desk, and a narrower desk is not a
	// narrower gate (§83, and §84 does not amend it).
	Mode *mode.Mode

	// Chair mounts the engine as one of the office's agents instead of the main
	// assistant — the direct chat §85 added. Mode must be the office desk; the
	// chair's tools are its profile intersected with that ceiling
	// (subagent.FilterRegistry, the same cut its delegate runs get), its prompt
	// direction is subagent.PromptFor (profile + what it has learned), and its
	// `memory` tool writes the chair's own scope, never the main's.
	//
	// Nil is every session that is not a chair chat, and changes nothing.
	Chair *subagent.Profile

	// Approve is the human approval gate, used by the App and by every
	// sub-agent.
	//
	// REQUIRED. turn reads a nil Approve as "approved" — so accepting nil here
	// would silently disarm whatever approval mode the config asked for, and it
	// would fail open, quietly, on the host least likely to have a human
	// watching. Engine refuses instead.
	Approve turn.ApprovalPromptFunc

	// Manager is caller-owned: a host caches one across re-bootstraps so live
	// server connections survive a model switch. Nil is safe and means no MCP.
	// Only its permission rules are read here — connecting is the host's call,
	// because the right moment differs between a window and a one-shot process.
	Manager *mcp.Manager

	// ExtraSkills are host-owned tools (the desktop workbench's browser tools,
	// for instance), registered as skill.SourceWorkbench.
	ExtraSkills []skill.Skill

	// OutputSubdir answers "where does a brand-new file go", relative to
	// cfg.SandboxRoot. Read per tool call rather than baked in, so a host can
	// move it while running. Nil writes to the sandbox root.
	OutputSubdir func() string

	// OpenSandbox lifts the file-tool wall around cfg.SandboxRoot: any path on
	// the machine works except the credential stores (skill/sandbox_open.go),
	// and the system prompt says so. The desktop passes true exactly when no
	// project is focused; everything else leaves it false and keeps the
	// closed sandbox.
	OpenSandbox bool

	// ExtraRoots are folders the user added to the focused project, each with
	// the same rights as cfg.SandboxRoot. Both the tools and the system prompt
	// are built from this one list, so what the model is told it can reach and
	// what it can actually reach cannot drift apart.
	ExtraRoots []string

	// Space and SpaceContext name the โปรเจกต์ this session is being held
	// inside (COMPANY.md §84) and the folder that project keeps its context in.
	// Unlike ExtraRoots this widens nothing: the sandbox is untouched, and the
	// only thing they change is that the assistant is told the project exists.
	// Empty is a chat held outside every project, which is most of them.
	Space        string
	SpaceContext SpaceContext

	// Digest overrides the page summarizer web_fetch uses. Nil means
	// Digester(provider, cfg.ModelName) — the normal case. Present so a test
	// can pin it without standing up a provider.
	Digest skill.Digester

	OnToolAction     func(turn.ToolEvent)
	OnToolRun        func(turn.ToolRun)
	OnStatus         func(string)
	OnContentPreview func(string)
	OnContentReset   func()
	OnUsage          func(model.Usage)

	// Proposer is the approval door for anything an agent wants to learn. Only
	// the desktop supplies one — it owns the store that holds the queue — and a
	// nil one means delegates simply have no `memory` tool, which is the honest
	// state for a front end that cannot persist anything.
	Proposer learned.Proposer
}

// Result is a live engine. Every field is what the host needs to drive it or to
// show what it is made of.
type Result struct {
	App        *aetoxapp.App
	Agent      *cognitive.Agent
	Registry   *skill.Registry
	Dispatcher *skill.Dispatcher
	Provider   model.Provider

	// Permissions is the resolved rule list the engine is running under: the
	// MCP ask-rules first, the user's own rules last so an explicit choice
	// still wins. Exposed because a host that hands the engine to someone else
	// — an HTTP caller, an MCP client — needs to see, and narrow, what that
	// caller is allowed to do.
	Permissions safety.PermissionConfig

	// Status is the human-readable model line ("deepseek-v4-flash · think=high"),
	// already carrying any provider warning in parentheses. Set even when Engine
	// returns an error, so a host can show why it is not running.
	Status string

	// Fallback is non-nil when the engine came up on the built-in aetox provider
	// instead of the one the config asked for. The engine works; this says the
	// caller did not get what they requested, and why. Distinct from Engine's
	// error, which means there is no engine at all.
	Fallback error
}

// ErrNoApprover is returned when Options.Approve is nil. See the field comment:
// this fails closed on purpose.
var ErrNoApprover = errors.New("bootstrap: Options.Approve is required (a nil approver silently auto-approves every tool call)")

// Engine builds the provider, agent, skill registry, sub-agent tools and app
// from one config, and returns them ready to run.
func Engine(cfg config.Config, opts Options) (Result, error) {
	defer debuglog.Block("bootstrap.Engine")()

	if opts.Approve == nil {
		return Result{}, ErrNoApprover
	}

	// Which model this actually resolved to. The log recorded that a bootstrap
	// happened but never what came out of it, so "it switched models on its
	// own" had no evidence either way — the same reason startup logs a
	// checkpoint at all.
	debuglog.Info("resolved model", fmt.Sprintf("%s / %s (think=%s, approval=%s)",
		cfg.ModelProvider, cfg.ModelName, cfg.ThinkLevel, cfg.ApprovalMode))
	status := model.ResolveStatus(cfg.ModelProvider, cfg.ModelName, nil)

	providerDone := debuglog.Block("model.BootstrapProvider")
	bootstrapResult := model.BootstrapProvider(model.BootstrapOptions{
		Provider:   cfg.ModelProvider,
		Model:      cfg.ModelName,
		APIKey:     cfg.ModelAPIKey,
		BaseURL:    cfg.ModelBaseURL,
		Timeout:    modelTimeout(cfg),
		WireFormat: cfg.ModelWireFormat,
		Locale:     cfg.UILocale,
	})
	providerDone()
	if bootstrapResult.Provider == nil {
		return Result{Status: status + " (init failed: " + bootstrapResult.Error.Error() + ")"}, bootstrapResult.Error
	}
	if bootstrapResult.Warning != "" {
		status += " (" + bootstrapResult.Warning + ")"
	}

	maxChars := ContextChars(cfg)
	surface := opts.Surface
	if surface == "" {
		surface = prompt.SurfaceDesktop
	}
	// One scope value feeds both the prompt and the registry below — the model's
	// picture of the workspace and the gate that enforces it come from the same
	// place by construction.
	scope := prompt.Scope{
		Root: cfg.SandboxRoot, Open: opts.OpenSandbox, Extra: opts.ExtraRoots,
		Space: prompt.Space{
			Name:        opts.Space,
			ContextPath: opts.SpaceContext.Path,
			Files:       opts.SpaceContext.Files,
		},
	}
	// Built once, here, and never mutated for the life of the session — the
	// desk's direction and its memory are part of the prompt prefix the
	// provider caches, and a prompt that changed mid-conversation would spend
	// more on cache misses than either layer saves (internal/learned).
	desk := prompt.Desk{Name: opts.Mode.DeskName(), Direction: opts.Mode.Direction()}
	if opts.Chair != nil {
		// A chair chat runs on the chair's own prompt — profile plus what it
		// has learned, the same fold its delegate runs get (§85: one fold, two
		// doors, never drifting). The desk's direction is not stacked on top:
		// the office manifest briefs deliverable work in general, the chair's
		// file briefs *this job*, and an agent reading both would be serving
		// two masters where its delegate runs serve one.
		desk = prompt.Desk{Name: opts.Chair.Name, Direction: subagent.PromptFor(*opts.Chair)}
	}
	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     bootstrapResult.Provider,
		Model:        cfg.ModelName,
		SystemPrompt: prompt.BuildForDesk(surface, scope, desk),
		// Scale the retained-history budget to the model's real window
		// (0 → NewContext's 128k-char default). At 80% of this budget the
		// agent summarizes older turns into one message (cognitive
		// compactIfNeeded); the verbatim oldest-turn trim in enforceLimits
		// remains only as the hard floor when summarization fails.
		MaxChars: maxChars,
	})

	digest := opts.Digest
	if digest == nil {
		digest = Digester(bootstrapResult.Provider, cfg.ModelName)
	}
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{
		SandboxRoot:  cfg.SandboxRoot,
		OutputSubdir: opts.OutputSubdir,
		OpenSandbox:  scope.Open,
		ExtraRoots:   scope.Extra,
		// Empty ModelPath keeps the original behaviour — auto-discover — so a
		// user who never opens the picker sees no change.
		Speech: stt.Options{ModelPath: cfg.SpeechModelPath},
		Digest: digest,
		// The same question visionAttachments asks of a user's attachment,
		// asked once here for the files the model finds on its own. Re-asked on
		// every re-bootstrap, which is what happens when the model changes.
		Vision: model.ResolveVision(cfg.ModelProvider, cfg.ModelName),
	})
	for _, s := range opts.ExtraSkills {
		if err := registry.Register(s, skill.SourceWorkbench); err != nil {
			debuglog.Msg("skill registration skipped: %v", err)
		}
	}
	// Scans ~/.aetox/skills — a real filesystem walk, not a fixed-cost lookup;
	// timed because a large/slow-disk skills directory is a plausible,
	// easy-to-overlook source of startup latency.
	discoverDone := debuglog.Block("skill.RegisterDiscovered")
	for _, discErr := range skill.RegisterDiscovered(registry, skill.DefaultDiscoveryPaths()) {
		debuglog.Msg("skill discovery: %v", discErr)
	}
	discoverDone()
	// One registry, one desk's view of it. NewDispatcher moved from boot to
	// session-open for exactly this (§83): installing a tool still has a single
	// home, and what varies per session is only which of it is visible. The
	// filter reads the registry live, so an MCP server that connects mid-session
	// still appears on the desks that attached it and on no others.
	dispatcher := skill.NewDispatcherFor(registry, opts.Mode.Carries)
	if opts.Chair != nil {
		// A chair chat sees exactly what its delegate runs see: profile ∩ the
		// office ceiling, cut by the same FilterRegistry — a second reading of
		// the same rules could drift from the one the delegate actually runs
		// on. The cut is a snapshot, which is also what keeps `task` out: the
		// task tools are registered below, after this line, and a leaf stays a
		// leaf even when spoken to (§85). Note FilterRegistry's own caveat: an
		// MCP server that finishes connecting after this point reaches every
		// desk that attached it, but not a chair session already open.
		//
		// Which is why the deferred ones are brought up *before* the cut, not
		// after: a server only this agent carries was skipped at startup, and a
		// snapshot taken first would be a snapshot without it — the same
		// connect the delegate path does, on the other door, so opening a chat
		// and being handed a job give the worker the same tools.
		// Engine has no context of its own — it is called from a settings save
		// and a session open, neither of which cancels. A bound of its own,
		// then, so a server that never answers delays opening the chat by a
		// known amount instead of hanging the door.
		chairCtx, cancelChair := context.WithTimeout(context.Background(), deferredConnectTimeout)
		for _, err := range opts.Manager.RegisterFor(chairCtx, registry, config.MCPServersForAgent(opts.Chair.Name)) {
			debuglog.Msg("chair %s: %v", opts.Chair.Name, err)
		}
		cancelChair()
		child := subagent.FilterRegistry(registry, *opts.Chair, opts.Mode)
		// `memory` is rebuilt bound to the chair's own scope, exactly as
		// task.go does for a delegate and for the same reason: the parent's
		// instance writes the main agent's file, and there must be no scope an
		// agent can name but its own.
		if opts.Proposer != nil && opts.Chair.AllowsTool("memory") {
			scoped := &learned.MemoryTool{Scope: opts.Chair.Name, Proposer: opts.Proposer}
			if regErr := child.Register(scoped, skill.SourceBuiltin); regErr != nil {
				debuglog.Msg("memory unavailable to chair %s: %v", opts.Chair.Name, regErr)
			}
		}
		dispatcher = skill.NewDispatcher(child)
	}

	permissions, permErr := config.LoadPermissions()
	if permErr != nil {
		debuglog.Msg("permissions load failed: %v", permErr)
	}
	// The user's own commands around each tool call (ARCHITECTURE.md §57). A
	// broken hooks file is logged and ignored rather than fatal: a typo in an
	// optional convenience must never be the reason the app will not start.
	hookCfg, hookErr := config.LoadHooks()
	if hookErr != nil {
		debuglog.Msg("hooks load failed, running without them: %v", hookErr)
	}
	hooks := hook.NewRunner(hookCfg, cfg.SandboxRoot)
	// Prepend the default MCP "ask" rules so MCP tools never auto-run. These are
	// derived from configured server names WITHOUT connecting — so the safety
	// gate is in place synchronously here, even though the tools themselves are
	// registered later by a background connect, which is what used to block
	// startup ~5s on a cold `npx` resolve. A tool named "<server>_x" can't be
	// called before its "<server>_*" ask-rule exists, so nothing races.
	// User rules stay last (last-match-wins) so explicit choices still win.
	permissions.Rules = append(opts.Manager.PermissionRules(), permissions.Rules...)

	// `task`, `task_result` and `task_answer` — the only way a sub-agent runs
	// (§44.4, §44.11: starting one never blocks the turn). Registered here
	// rather than in skill.RegisterDefaults because they need turn+cognitive,
	// which skill cannot import. They hold the live provider/registry/
	// permissions, so a re-bootstrap replaces them along with everything else
	// instead of leaving them pointed at a dead engine. FilterRegistry drops
	// them from every child, which is what keeps delegation depth at 1.
	//
	// Registered BEFORE app.NewApp on purpose: NewApp snapshots the dispatcher's
	// names into the app's skill and command sets, so a host that registered
	// these afterwards would silently omit them from both.
	//
	// The mode the user actually picked (SwitchApprovalMode persists it), not a
	// hardcoded full-access: with approval hardwired, the Settings toggle was
	// cosmetic and ask/unsafe-only never reached the engine.
	approvalMode := safety.NormalizeApprovalMode(cfg.ApprovalMode)

	// Against this provider's own list, not just parsed. The desktop already
	// normalizes on every switch, so this changes nothing there; a host that
	// does not — the CLI's --think, a config file written by hand — used to
	// hand through a level the provider does not have, and the level a provider
	// lacks most often is "off". think.Resolve short-circuits "off" to "send no
	// effort", so on a model that cannot stop thinking (Kimi K3) asking for off
	// silently produced its default, which is the *most* thinking it has.
	//
	// Empty means the provider has no dial at all (ollama, lmstudio); the
	// original value rides through untouched because nothing will read it.
	thinkLevel := cfg.ThinkLevel
	if normalized := model.NormalizeThinkingLevel(cfg.ModelProvider, cfg.ModelName, thinkLevel); normalized != "" {
		thinkLevel = normalized
	}
	for _, tool := range subagent.NewTaskTools(subagent.TaskOptions{
		Provider: bootstrapResult.Provider,
		Model:    cfg.ModelName,
		Registry: registry,
		// The desk decides the ceiling a delegate runs under and which chairs
		// this desk may hand a job to. The registry stays whole: a cross-desk
		// dispatch runs on the target desk's manifest, so the tool it needs has
		// to still be in there to be filtered *in* (§84).
		Desk: opts.Mode,
		// The same assembler the session's own prompt came out of, three lines
		// up, with the delegate's brief where the desk's direction goes. One
		// door: a worker reached by `task` and a worker reached by opening a
		// chat now stand in the same room, described the same way.
		BuildPrompt: func(direction string) string {
			return prompt.BuildForDesk(surface, scope, prompt.Desk{
				Name:      opts.Mode.DeskName(),
				Direction: direction,
			})
		},
		// The deferred half of the MCP config: a server only this agent carries
		// was skipped at startup and comes up here, on the dispatch that needs
		// it. Reads the live manager, so a re-bootstrap hands the new tool a
		// closure over the manager that is actually connected.
		EnsureServers: func(ctx context.Context, names []string) []error {
			return opts.Manager.RegisterFor(ctx, registry, names)
		},
		Permissions:  permissions,
		ApprovalMode: approvalMode,
		Approve:      opts.Approve,
		OnToolAction: opts.OnToolAction,
		OnToolRun:    opts.OnToolRun,
		OnUsage:      opts.OnUsage,
		MaxChars:     maxChars,
		ThinkLevel:   think.NormalizeLevel(thinkLevel),
		Proposer:     opts.Proposer,
	}) {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			debuglog.Msg("%s registration skipped: %v", tool.Name(), err)
		}
	}

	console := opts.Console
	if console == nil {
		console = aetoxapp.NewStdIO()
	}
	chatApp, err := aetoxapp.NewApp(aetoxapp.Options{
		Agent:      agent,
		Console:    console,
		Dispatcher: dispatcher,
		// The level the user picked, on the main agent's turns too. Omitting it
		// left app.thinkLevel resolving an empty string, which think.Normalize
		// reads as "medium" — so every desktop turn went out at a depth nobody
		// chose, "off" included, while the sub-agent tools built ten lines above
		// honoured the setting correctly. The CLI passed it and was never
		// affected, which is why the picker looked fine everywhere it was tested.
		ThinkLevel:     think.NormalizeLevel(thinkLevel),
		ApprovalMode:   approvalMode,
		Permissions:    permissions,
		Hooks:          hooks,
		OnToolAction:   opts.OnToolAction,
		OnToolRun:      opts.OnToolRun,
		StatusReporter: opts.OnStatus,
		// The answer streams into the live bubble as the model writes it; the
		// reset is what keeps a round that turns out to be narration, a nudged
		// draft or a failure from being mistaken for the reply.
		OnContentPreview: opts.OnContentPreview,
		OnContentReset:   opts.OnContentReset,
		Approve:          opts.Approve,
	})
	if err != nil {
		return Result{Status: status + " (init failed: " + err.Error() + ")"}, err
	}
	// bootstrapResult.Error survives a successful return on purpose: the
	// engine is up, but on the aetox fallback rather than the provider the
	// caller asked for, and only this error says why.
	return Result{
		App:         chatApp,
		Agent:       agent,
		Registry:    registry,
		Dispatcher:  dispatcher,
		Provider:    bootstrapResult.Provider,
		Permissions: permissions,
		Status:      status,
		Fallback:    bootstrapResult.Error,
	}, nil
}

// modelTimeout honours the configured per-request timeout instead of the 30s
// that used to be hardcoded here. A local model on a cold load routinely takes
// longer than 30s to answer, and the setting existed already — this path just
// never read it.
func modelTimeout(cfg config.Config) time.Duration {
	if cfg.ModelTimeoutSec > 0 {
		return time.Duration(cfg.ModelTimeoutSec) * time.Second
	}
	return 30 * time.Second
}
