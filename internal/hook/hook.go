// Package hook runs the user's own commands around a tool call.
//
// This is the Claude Code shape, not opencode's. opencode has a plugin runtime:
// a JS module registers `tool.execute.before`/`after` functions and the host
// calls them. Aetox has nowhere to run one — `plugin_install` is still the
// half-finished loader of ARCHITECTURE.md §6.5 — so building those callback
// points would be an extension point with nothing to plug into.
//
// A hook here is a **shell command in a config file**. No plugin, no loader, no
// sandbox for third-party code, and the thing the user actually wanted:
//
//	before a shell command runs, check it against my own rules and refuse it
//	after any file is written, run my formatter
//	whenever the agent edits, tell me in Slack
//
// The pattern is already in the codebase, hardcoded three times over — rtk
// rewriting a command before it runs, safety refusing one, rtk compacting the
// output after. This is the same idea made into something a user can configure
// without a Go build.
package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

// Event is when a hook fires.
type Event string

const (
	// PreToolUse runs before the tool does, and can refuse it.
	PreToolUse Event = "PreToolUse"
	// PostToolUse runs after, and cannot change what already happened.
	PostToolUse Event = "PostToolUse"
)

// Timeout bounds one hook. Deliberately short and not configurable: a hook sits
// between the model and its tool, so a slow one is felt on every single call.
// A user who needs longer than this wants a background job, not a hook.
const Timeout = 10 * time.Second

// Hook is one configured command.
type Hook struct {
	// Event is PreToolUse or PostToolUse. Empty means PreToolUse, because a
	// hook someone wrote without reading the docs is far more likely to be a
	// guard than a notifier.
	Event Event `json:"event"`
	// Matcher is a glob against the tool name: "shell", "write", "*", or
	// "github_*". Empty matches everything.
	Matcher string `json:"matcher"`
	// Command is run through the platform shell, so a user can write what they
	// would type. It receives the call as JSON on stdin and as environment
	// variables — see Run.
	Command string `json:"command"`
	// Blocking makes a non-zero exit refuse the tool call. Only meaningful on
	// PreToolUse. Off by default: a hook that silently starts blocking work
	// because a formatter returned 1 is worse than no hook at all.
	Blocking bool `json:"blocking"`
}

// Config is the whole hooks file.
type Config struct {
	Hooks []Hook `json:"hooks"`
}

// Decision is what a PreToolUse pass concluded.
type Decision struct {
	// Blocked is true when a blocking hook exited non-zero.
	Blocked bool
	// Reason is what that hook printed, which is shown to the model so it can
	// do something else rather than repeat the call. A hook that blocks without
	// printing anything is a wall with no sign on it, so there is a fallback.
	Reason string
}

// Load reads the hooks file. A missing file is not an error: almost nobody has
// hooks, and startup must not care.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	// Normalize once here rather than at every comparison.
	for i := range cfg.Hooks {
		if cfg.Hooks[i].Event == "" {
			cfg.Hooks[i].Event = PreToolUse
		}
		if strings.TrimSpace(cfg.Hooks[i].Matcher) == "" {
			cfg.Hooks[i].Matcher = "*"
		}
	}
	return cfg, nil
}

// Runner holds the loaded hooks. The zero value is valid and does nothing,
// which is what every caller gets when no hooks file exists.
type Runner struct {
	hooks   []Hook
	workDir string
	// backend is the shell hooks run in, and it is deliberately the same one
	// the shell tool runs in rather than a setting of its own.
	//
	// A PreToolUse hook exists to inspect what the agent is about to do, and a
	// hook that inspects a bash command line from cmd.exe is inspecting a
	// string it cannot evaluate — it would pass commands it should stop and
	// stop commands it should pass, quietly, in the one place built to be a
	// safety gate. A PostToolUse hook has the same problem in reverse: it
	// checks work done by a toolchain it cannot see.
	//
	// Nil means the native shell, which is every existing configuration.
	backend func() proc.Backend
}

func NewRunner(cfg Config, workDir string) *Runner {
	return &Runner{hooks: cfg.Hooks, workDir: workDir}
}

// WithBackend points the runner's hooks at the shell the workspace is set to.
// Separate from NewRunner so that every existing caller keeps compiling and
// keeps its old behaviour, which is the native shell.
func (r *Runner) WithBackend(backend func() proc.Backend) *Runner {
	if r != nil {
		r.backend = backend
	}
	return r
}

func (r *Runner) shell() proc.Backend {
	if r == nil || r.backend == nil {
		return proc.Native()
	}
	if backend := r.backend(); backend != nil {
		return backend
	}
	return proc.Native()
}

// Any reports whether anything is configured at all, so the hot path can skip
// building the payload for the overwhelmingly common case of no hooks.
func (r *Runner) Any(event Event) bool {
	if r == nil {
		return false
	}
	for _, h := range r.hooks {
		if h.Event == event && strings.TrimSpace(h.Command) != "" {
			return true
		}
	}
	return false
}

// Run fires every hook matching this event and tool.
//
// The call is handed over two ways at once, because the two kinds of hook want
// different things: a one-line shell guard wants `$AETOX_TOOL` in an `if`, and
// a real script wants the arguments, which only JSON can carry faithfully.
//
//	stdin           the whole call as JSON: {"event","tool","args"}
//	AETOX_TOOL      the tool name
//	AETOX_EVENT     PreToolUse or PostToolUse
//	AETOX_TOOL_ARGS the same JSON as on stdin, for a shell that cannot read it
//
// PostToolUse also gets AETOX_TOOL_OK ("1"/"0") and AETOX_TOOL_OUTPUT.
func (r *Runner) Run(ctx context.Context, event Event, tool string, args map[string]any, result *Result) Decision {
	if r == nil {
		return Decision{}
	}
	var payload []byte
	for _, h := range r.hooks {
		if h.Event != event || !globMatch(h.Matcher, strings.ToLower(strings.TrimSpace(tool))) {
			continue
		}
		command := strings.TrimSpace(h.Command)
		if command == "" {
			continue
		}
		if payload == nil {
			payload = marshalCall(event, tool, args)
		}
		out, err := r.exec(ctx, command, payload, event, tool, result)
		if err == nil {
			continue
		}
		debuglog.Msg("hook %q on %s(%s) failed: %v", command, event, tool, err)
		// A failing non-blocking hook is the user's problem to see in the log,
		// never a reason to stop the agent doing what it was asked.
		if !h.Blocking || event != PreToolUse {
			continue
		}
		reason := strings.TrimSpace(out)
		if reason == "" {
			reason = "a PreToolUse hook refused this call and printed no reason"
		}
		return Decision{Blocked: true, Reason: reason}
	}
	return Decision{}
}

// Result describes a finished tool call, for PostToolUse.
type Result struct {
	OK     bool
	Output string
}

func (r *Runner) exec(ctx context.Context, command string, payload []byte, event Event, tool string, result *Result) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	backend := r.shell()
	cmd := backend.Command(ctx, command, r.workDir)
	cmd.Stdin = bytes.NewReader(payload)
	// Extended, not replaced: the backend may have put its own variables on the
	// command, and starting again from os.Environ() would drop them.
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	names := []string{"AETOX_EVENT", "AETOX_TOOL", "AETOX_TOOL_ARGS"}
	cmd.Env = append(cmd.Env,
		"AETOX_EVENT="+string(event),
		"AETOX_TOOL="+tool,
		"AETOX_TOOL_ARGS="+string(payload),
	)
	if result != nil {
		ok := "0"
		if result.OK {
			ok = "1"
		}
		// Capped: a hook is passed the receipt, not a megabyte of build log,
		// and some platforms have a hard limit on one variable's size.
		cmd.Env = append(cmd.Env, "AETOX_TOOL_OK="+ok, "AETOX_TOOL_OUTPUT="+truncate(result.Output, 8<<10))
		names = append(names, "AETOX_TOOL_OK", "AETOX_TOOL_OUTPUT")
	}
	// Setting them is not enough to make them readable — see Backend.Export.
	backend.Export(cmd, names...)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return truncate(strings.TrimSpace(buf.String()), 4<<10), err
}

func marshalCall(event Event, tool string, args map[string]any) []byte {
	payload, err := json.Marshal(map[string]any{
		"event": string(event),
		"tool":  tool,
		"args":  args,
	})
	if err != nil {
		return []byte(`{}`)
	}
	return payload
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// globMatch is the same "*" and "?" matching safety.PermissionConfig uses for
// tool names, kept identical on purpose: a user who has written one matcher
// should not have to learn a second syntax for the other file.
func globMatch(pattern, s string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" || pattern == "*" {
		return true
	}
	return matchGlob(pattern, s)
}

// matchGlob is a small iterative matcher — no regexp compile per call, because
// this runs for every hook on every tool call.
func matchGlob(pattern, s string) bool {
	var pi, si, star, mark int
	star = -1
	for si < len(s) {
		switch {
		case pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == s[si]):
			pi++
			si++
		case pi < len(pattern) && pattern[pi] == '*':
			star, mark = pi, si
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}
