// Package mcp connects Aetox to external Model Context Protocol servers.
//
// Two transports: local/stdio (a subprocess speaking MCP over stdin/stdout,
// e.g. npx/uvx-based servers) and remote streamable HTTP (URL + optional
// static headers). OAuth stays deferred until a real need appears — see
// MCP-SUPPORT-PLAN.md.
//
// The transport, JSON-RPC framing, and initialize handshake come from the
// official github.com/modelcontextprotocol/go-sdk; this package owns only
// config, connection lifecycle, and (elsewhere) the skill.Tool adapter.
package mcp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Mike0165115321/Aetox/internal/proc"
)

// Status reports where a server's connection stands. Connection failures are
// surfaced as StatusFailed rather than thrown, so a misconfigured server just
// drops out of the tool list instead of breaking the agent loop.
type Status string

const (
	StatusIdle      Status = "idle"      // configured, not connected yet (lazy)
	StatusConnected Status = "connected" // handshake succeeded, tools usable
	StatusFailed    Status = "failed"    // connect failed; see Client.Err
)

const defaultTimeout = 30 * time.Second

// Server is one configured MCP server. A non-empty URL selects the remote
// streamable-HTTP transport; otherwise Command is spawned as a local stdio
// subprocess. Fields mirror the config schema in MCP-SUPPORT-PLAN.md §4.
type Server struct {
	Name        string            // stable id; used as the tool-name prefix
	Command     []string          // local: argv0 + args
	Cwd         string            // local: working dir; caller resolves against sandbox root
	Environment map[string]string // local: merged over os.Environ()
	URL         string            // remote: streamable HTTP endpoint
	Headers     map[string]string // remote: static headers (e.g. Authorization)
	Timeout     time.Duration     // connect timeout; default 30s
	Disabled    bool              // configured but switched off; Manager skips it
	// Deferred means no desk carries this server — only named agents do — so
	// it is left out of the startup connect and brought up when one of them is
	// actually started.
	//
	// `for:` decided who *sees* a server's tools and never decided whether to
	// connect it, so a server placed on one agent was still spawned on every
	// launch by every user who had it configured, whether or not that agent was
	// ever spoken to. For a 90-tool remote server that is a handshake and a full
	// tool listing bought on the chance somebody might.
	//
	// Only agent-only servers qualify. A desk's server has to be there before
	// the first message, because a desk is what the user is already sitting at.
	Deferred bool
	// Tools, when non-empty, is the only tools accepted from this server —
	// everything else it offers is dropped before it ever reaches a registry.
	//
	// This exists for the server that is two products in one box. n8n-mcp is
	// the case that forced it: 7 of its tools answer a question Aetox cannot
	// (n8n publishes no node schemas), and 16 write workflows — which Aetox
	// already does, through its own tools, its own permission rules and its own
	// tool_runs log. Taking the whole server would put a second way to write in
	// front of the model, and the one it happened to pick would decide whether
	// the user's own record of what was done to their instance had the entry.
	//
	// Why an allowlist and not `deny:` on the profile, which already works: a
	// denial list is written against somebody else's catalogue, and the day
	// that server ships a seventeenth write tool it arrives switched on and
	// nothing says so. An allowlist is wrong loudly instead — see SkillTools,
	// which refuses to let a list that matches nothing pass as a server with
	// nothing to offer.
	//
	// Empty means take everything, so nothing changes for a server nobody has
	// written a list for, which is all of them today.
	Tools []string
}

// Client wraps a single MCP server connection. Connect is lazy: the subprocess
// is not started until the first Tools/CallTool call, so servers that are
// configured but never used don't slow startup. Safe for concurrent use.
type Client struct {
	cfg Server

	mu      sync.Mutex
	session *mcpsdk.ClientSession
	status  Status
	lastErr error
	// procCancel and procPID belong to a local (stdio) server's subprocess and
	// are zero for an HTTP one. They are the client's own rather than the
	// caller's: ensure() holds whichever per-call context asked for a tool, and
	// binding the server to that would kill it when the first call returned.
	procCancel context.CancelFunc
	procPID    int
	toolCount  int // tools seen on the last successful Tools(); 0 until then
}

// New builds a Client for cfg without connecting.
func New(cfg Server) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{cfg: cfg, status: StatusIdle}
}

// Name returns the server's configured id.
func (c *Client) Name() string { return c.cfg.Name }

// Deferred reports whether this server waits for the agent that needs it. See
// Server.Deferred.
func (c *Client) Deferred() bool { return c.cfg.Deferred }

// Command returns the server's configured argv (for status display).
func (c *Client) Command() []string { return c.cfg.Command }

// Status reports the current connection state.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Err returns the last connection error, if the client is in StatusFailed.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

// ensure connects on first use and caches the session. A prior failure is
// sticky — we don't respawn on every call, which would let a broken server
// stall each tool invocation. Close resets that so a reconfigured server can
// reconnect.
func (c *Client) ensure(ctx context.Context) (*mcpsdk.ClientSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session != nil {
		return c.session, nil
	}
	if c.status == StatusFailed {
		return nil, c.lastErr
	}
	// Both stay nil for an HTTP server, which has no process to tear down.
	var procCancel context.CancelFunc
	var localCmd *exec.Cmd

	var transport mcpsdk.Transport
	switch {
	case c.cfg.URL != "":
		transport = &mcpsdk.StreamableClientTransport{
			Endpoint:   c.cfg.URL,
			HTTPClient: headerHTTPClient(c.cfg.Headers),
		}
	case len(c.cfg.Command) == 0 || c.cfg.Command[0] == "":
		c.status = StatusFailed
		c.lastErr = errors.New("mcp: server " + c.cfg.Name + " has no command or url")
		return nil, c.lastErr
	default:
		// The server lives as long as this Client does, so it gets a context of
		// its own — and a cancellable one, because that is the only kind
		// os/exec watches. proc.KillOnCancel installs cmd.Cancel, and with a
		// context that can never be done the command starts fine and the kill
		// silently never happens.
		//
		// It also has to be exec.CommandContext and not exec.Command: Start
		// refuses a command that carries a Cancel it was not built with, and the
		// Start here is inside the SDK's Connect, where the refusal would
		// surface as every local server failing to connect.
		procCtx, cancelProc := context.WithCancel(context.Background())
		cmd := exec.CommandContext(procCtx, c.cfg.Command[0], c.cfg.Command[1:]...)
		cmd.Dir = c.cfg.Cwd
		// The production desktop exe is a GUI app: without this, a console child
		// (npx→cmd.exe on Windows) pops a visible Windows Terminal window on spawn.
		proc.HideConsole(cmd)
		// CommandTransport.Close closes stdin, then SIGTERMs, then kills — all of
		// it aimed at the direct child. A server that forks (npx→node,
		// uvx→python) left its grandchildren running, and every mid-life
		// teardown hits this: Settings' Test button, adding or removing a server,
		// a project switch. Close() below does the polite shutdown first and
		// sweeps the tree after.
		proc.KillOnCancel(cmd)
		procCancel, localCmd = cancelProc, cmd
		if len(c.cfg.Environment) > 0 {
			env := os.Environ()
			for k, v := range c.cfg.Environment {
				env = append(env, k+"="+v)
			}
			cmd.Env = env
		}
		transport = &mcpsdk.CommandTransport{Command: cmd}
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "aetox", Version: "0"}, nil)

	// Bound the initialize handshake so a process that starts but never speaks
	// MCP can't hang the caller (and, via lazy connect, startup) indefinitely.
	connectCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		// A server that half-started still forked whatever it forked, and
		// nothing else holds a handle on it from here.
		if procCancel != nil {
			procCancel()
		}
		c.status = StatusFailed
		c.lastErr = err
		return nil, err
	}
	// Connect is where the SDK calls Start, so this is the first moment there is
	// a pid to remember. Close needs it after Wait has already reaped the
	// process, by which point cmd.Process is no longer a safe thing to read.
	if localCmd != nil && localCmd.Process != nil {
		c.procPID = localCmd.Process.Pid
	}
	c.procCancel = procCancel
	c.session = session
	c.status = StatusConnected
	return session, nil
}

// Tools lists the server's tools, connecting lazily. On connect failure it
// returns the error; callers treat that as "this server contributes no tools".
func (c *Client) Tools(ctx context.Context) ([]*mcpsdk.Tool, error) {
	session, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	var tools []*mcpsdk.Tool
	for tool, iterErr := range session.Tools(ctx, nil) {
		if iterErr != nil {
			return nil, iterErr
		}
		tools = append(tools, tool)
	}
	c.mu.Lock()
	c.toolCount = len(tools)
	c.mu.Unlock()
	return tools, nil
}

// ToolCount reports how many tools the server exposed on the last successful
// Tools() enumeration (0 before the first one, or after Close).
func (c *Client) ToolCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.toolCount
}

// CallTool invokes one tool on the server, connecting lazily.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*mcpsdk.CallToolResult, error) {
	session, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	return session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
}

// Resources lists what the server offers to read — files, records, documents
// it is the authority on. Empty is the normal case: a server that only exposes
// tools declares no resources, and asking one that does not support them at all
// is an error, not a fact worth reporting.
func (c *Client) Resources(ctx context.Context) ([]*mcpsdk.Resource, error) {
	session, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	var out []*mcpsdk.Resource
	for resource, iterErr := range session.Resources(ctx, nil) {
		if iterErr != nil {
			return nil, iterErr
		}
		out = append(out, resource)
	}
	return out, nil
}

// ReadResource fetches one resource's contents by URI.
func (c *Client) ReadResource(ctx context.Context, uri string) (*mcpsdk.ReadResourceResult, error) {
	session, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	return session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: uri})
}

// Prompts lists the server's prompt templates — the workflows its author
// thought were worth naming.
func (c *Client) Prompts(ctx context.Context) ([]*mcpsdk.Prompt, error) {
	session, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	var out []*mcpsdk.Prompt
	for prompt, iterErr := range session.Prompts(ctx, nil) {
		if iterErr != nil {
			return nil, iterErr
		}
		out = append(out, prompt)
	}
	return out, nil
}

// GetPrompt renders one prompt template with arguments filled in.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*mcpsdk.GetPromptResult, error) {
	session, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	return session.GetPrompt(ctx, &mcpsdk.GetPromptParams{Name: name, Arguments: args})
}

// Close terminates the subprocess if connected and resets to idle so a later
// call can reconnect. Safe to call when never connected.
//
// The order is the whole point. session.Close runs the shutdown the protocol
// prescribes — close stdin, wait, SIGTERM, wait, kill — and every step of it
// reaches only the process the SDK started. Then the sweep, because a server
// that forked (npx→node, uvx→python) has left descendants that no signal in
// that ladder was ever addressed to.
//
// KillTree rather than procCancel for the sweep: the SDK's Close calls Wait,
// and once Wait returns, os/exec has retired the goroutine that would have
// invoked cmd.Cancel — cancelling here would compile, run, and do nothing.
// procCancel is still called, for the release of the context itself.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		c.status = StatusIdle
		c.releaseProcess()
		return nil
	}
	err := c.session.Close()
	c.releaseProcess()
	c.session = nil
	c.status = StatusIdle
	c.lastErr = nil
	c.toolCount = 0
	return err
}

// releaseProcess sweeps a local server's tree and drops what named it. Caller
// holds c.mu.
func (c *Client) releaseProcess() {
	if c.procPID != 0 {
		proc.KillTree(c.procPID)
		c.procPID = 0
	}
	if c.procCancel != nil {
		c.procCancel()
		c.procCancel = nil
	}
}

// headerHTTPClient returns an http.Client that stamps the given static
// headers onto every request (Authorization tokens etc.), or the default
// client when there are none.
func headerHTTPClient(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return nil // transport falls back to http.DefaultClient
	}
	return &http.Client{Transport: headerRoundTripper{headers: headers}}
}

type headerRoundTripper struct {
	headers map[string]string
}

func (h headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Per http.RoundTripper's contract, don't mutate the caller's request.
	req = req.Clone(req.Context())
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(req)
}
