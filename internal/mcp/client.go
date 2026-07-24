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
}

// Client wraps a single MCP server connection. Connect is lazy: the subprocess
// is not started until the first Tools/CallTool call, so servers that are
// configured but never used don't slow startup. Safe for concurrent use.
type Client struct {
	cfg Server

	mu        sync.Mutex
	session   *mcpsdk.ClientSession
	status    Status
	lastErr   error
	toolCount int // tools seen on the last successful Tools(); 0 until then
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
		cmd := exec.Command(c.cfg.Command[0], c.cfg.Command[1:]...)
		cmd.Dir = c.cfg.Cwd
		// The production desktop exe is a GUI app: without this, a console child
		// (npx→cmd.exe on Windows) pops a visible Windows Terminal window on spawn.
		proc.HideConsole(cmd)
		if len(c.cfg.Environment) > 0 {
			env := os.Environ()
			for k, v := range c.cfg.Environment {
				env = append(env, k+"="+v)
			}
			cmd.Env = env
		}
		// ponytail: relies on CommandTransport.Close (stdin-close then SIGTERM to the
		// direct child) for cleanup. A server that forks (npx→node, uvx→python) can
		// still orphan grandchildren. Upgrade path: set cmd.SysProcAttr for a process
		// group (Setpgid on unix, CREATE_NEW_PROCESS_GROUP on windows) and kill the
		// group on Close — do it in the local-server hardening pass, not the skeleton.
		transport = &mcpsdk.CommandTransport{Command: cmd}
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "aetox", Version: "0"}, nil)

	// Bound the initialize handshake so a process that starts but never speaks
	// MCP can't hang the caller (and, via lazy connect, startup) indefinitely.
	connectCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		c.status = StatusFailed
		c.lastErr = err
		return nil, err
	}
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

// Close terminates the subprocess if connected and resets to idle so a later
// call can reconnect. Safe to call when never connected.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		c.status = StatusIdle
		return nil
	}
	err := c.session.Close()
	c.session = nil
	c.status = StatusIdle
	c.lastErr = nil
	c.toolCount = 0
	return err
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
