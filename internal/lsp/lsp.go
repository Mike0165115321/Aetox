// Package lsp is a minimal Language Server Protocol client: enough to ask a
// language server "is this file broken?" right after Aetox changed it.
//
// Why this exists: the model edits a file and moves on with no idea whether it
// still compiles. It finds out several turns later, from the user, having built
// more work on top of a broken file. A language server already knows within
// milliseconds — it just was never asked.
//
// Deliberately not a full client. No completion, no hover, no rename, no
// incremental sync: one document opened, one set of diagnostics collected. That
// is the whole feature, and the rest of LSP is a large surface to maintain for
// capabilities an agent that edits text files does not use.
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

// Diagnostic is one problem a server reported, flattened to what a model can
// act on: where, how bad, and what is wrong.
type Diagnostic struct {
	Path     string `json:"path"`
	Line     int    `json:"line"` // 1-based, as humans and every other tool count
	Column   int    `json:"column"`
	Severity string `json:"severity"` // error | warning | info | hint
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
}

func (d Diagnostic) String() string {
	head := fmt.Sprintf("%s:%d:%d: %s: %s", d.Path, d.Line, d.Column, d.Severity, d.Message)
	if d.Source != "" {
		head += " (" + d.Source + ")"
	}
	return head
}

// server is one language server binary and the arguments that put it on stdio.
type server struct {
	command string
	args    []string
	// languageID is what the protocol calls this file type in didOpen.
	languageID string
}

// servers maps a file extension to the server that understands it. Only
// entries whose binary is actually installed are ever used — a missing server
// means "no diagnostics", never an error, because a user who has not installed
// gopls still expects their edits to work.
var servers = map[string]server{
	".go":     {command: "gopls", languageID: "go"},
	".ts":     {command: "typescript-language-server", args: []string{"--stdio"}, languageID: "typescript"},
	".tsx":    {command: "typescript-language-server", args: []string{"--stdio"}, languageID: "typescriptreact"},
	".js":     {command: "typescript-language-server", args: []string{"--stdio"}, languageID: "javascript"},
	".jsx":    {command: "typescript-language-server", args: []string{"--stdio"}, languageID: "javascriptreact"},
	".svelte": {command: "svelteserver", args: []string{"--stdio"}, languageID: "svelte"},
	".py":     {command: "pylsp", languageID: "python"},
	".rs":     {command: "rust-analyzer", languageID: "rust"},
}

// Configured reports whether any server is known for this file type. It says
// nothing about whether that server exists on the machine.
func Configured(path string) bool {
	_, ok := servers[strings.ToLower(filepath.Ext(path))]
	return ok
}

// Available reports whether the server for this file type can actually run,
// installing it if it is missing and the toolchain to install it is present.
//
// Split from Configured on purpose: "we never check this language" and "we
// tried and could not" are different answers, and collapsing them lets a
// missing server be reported as a clean file — the most damaging possible
// wrong answer from a tool whose whole job is saying whether code is broken.
func Available(ctx context.Context, path string) bool {
	s, ok := servers[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return false
	}
	return ensureInstalled(ctx, s.command)
}

// Client owns the running servers for one workspace root. Servers are started
// on first use and reused: gopls spends seconds indexing a repo on startup, so
// paying that once per session rather than once per edit is the difference
// between this being usable and being abandoned.
type Client struct {
	root string

	mu      sync.Mutex
	running map[string]*conn // keyed by server command
}

func New(root string) *Client {
	return &Client{root: root, running: map[string]*conn{}}
}

// Close shuts every server down. Safe to call more than once.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, cn := range c.running {
		cn.close()
		delete(c.running, name)
	}
}

// Diagnose opens one file with the right language server and returns what it
// says about it. An unsupported or uninstalled language, or a server that
// never answers, yields no diagnostics and no error: this is an advisory
// signal layered on top of an edit that already succeeded, and it must never
// turn a good edit into a failure.
func (c *Client) Diagnose(ctx context.Context, path string, timeout time.Duration) ([]Diagnostic, error) {
	spec, ok := servers[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return nil, nil
	}
	// Installs it on first use if it is missing and the toolchain to do so is
	// there — see install.go for why that is here and not in the installer.
	if !ensureInstalled(ctx, spec.command) {
		return nil, nil // still unavailable — silence, not an error
	}

	cn, err := c.connFor(ctx, spec)
	if err != nil {
		debuglog.Msg("lsp: start %s: %v", spec.command, err)
		return nil, nil
	}

	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(c.root, path)
	}
	return cn.diagnose(ctx, abs, spec.languageID, timeout)
}

func (c *Client) connFor(ctx context.Context, spec server) (*conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cn, ok := c.running[spec.command]; ok && cn.alive() {
		return cn, nil
	}
	cn, err := startConn(ctx, c.root, spec)
	if err != nil {
		return nil, err
	}
	c.running[spec.command] = cn
	return cn, nil
}

// ---- one server process ----

type conn struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	// cancel tears the server's whole process tree down. It is the conn's own,
	// not the caller's: the ctx that reaches startConn belongs to one diagnose
	// call, and binding the server to it would kill gopls the moment the first
	// file finished checking. See close().
	cancel context.CancelFunc
	stdout *bufio.Reader

	mu      sync.Mutex
	nextID  int
	dead    bool
	diags   map[string][]Diagnostic // by file URI, replaced wholesale per notification
	updated chan string             // URIs that just got fresh diagnostics
	// pending correlates a reply with the call that asked for it. Diagnostics
	// arrive as notifications and need no id, which is why the handshake could
	// get away without this; hover and definition are requests, and a request
	// whose answer nobody is waiting for is just a slow no-op.
	pending map[int]chan json.RawMessage
}

func startConn(ctx context.Context, root string, spec server) (*conn, error) {
	// The server outlives the call that started it, so it gets a context of its
	// own. It has to be a cancellable one: proc.KillOnCancel installs cmd.Cancel,
	// which os/exec only ever calls from the goroutine it starts for a context
	// that can actually be done — with context.Background() the command starts
	// happily and the kill silently never happens.
	procCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, spec.command, spec.args...)
	cmd.Dir = root
	proc.HideConsole(cmd)
	// typescript-language-server forks tsserver, rust-analyzer spawns cargo,
	// svelteserver is node all the way down. Process.Kill() reaches the wrapper
	// and leaves the part doing the work — the same reason lsp/install.go gives
	// for npm, with "the rest of the session" in place of "five minutes".
	proc.KillOnCancel(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	cn := &conn{
		cmd:     cmd,
		stdin:   stdin,
		cancel:  cancel,
		stdout:  bufio.NewReader(stdout),
		diags:   map[string][]Diagnostic{},
		updated: make(chan string, 64),
		pending: map[int]chan json.RawMessage{},
	}
	go cn.readLoop()

	if err := cn.request(ctx, "initialize", map[string]any{
		"processId": nil,
		"rootUri":   pathToURI(root),
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"publishDiagnostics": map[string]any{"relatedInformation": false},
			},
		},
	}); err != nil {
		cn.close()
		return nil, err
	}
	if err := cn.notify("initialized", map[string]any{}); err != nil {
		cn.close()
		return nil, err
	}
	return cn, nil
}

func (c *conn) alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.dead
}

func (c *conn) close() {
	c.mu.Lock()
	if c.dead {
		c.mu.Unlock()
		return
	}
	c.dead = true
	c.mu.Unlock()
	// Closing stdin is how a language server is asked to stop, and the one it
	// should get first. Cancelling before Wait is what makes the kill a tree
	// kill: once Wait returns, os/exec's watcher goroutine has gone and
	// cmd.Cancel can never fire.
	_ = c.stdin.Close()
	if c.cancel != nil {
		c.cancel()
	}
	_ = c.cmd.Wait()
	// Belt and braces for the server that forked before it died: an orphan
	// still names its dead parent, which is what the second pass walks.
	if c.cmd.Process != nil {
		proc.KillTree(c.cmd.Process.Pid)
	}
}

// SymbolInfo is what a language server knows about one identifier: the type
// signature and doc comment it would show on hover, and where it is declared.
type SymbolInfo struct {
	Hover      string
	DefPath    string
	DefLine    int
	Occurrence int // 1-based line in the queried file where the name was found
}

// Symbol answers "what is this and where does it come from" for a name in a
// file.
//
// Located by name rather than by line and column, because that is what a model
// has: it reads code, not coordinates, and asking it to count characters
// invites an off-by-one that returns confidently wrong information about a
// neighbouring token. The first occurrence wins — for the case this serves,
// "what is this thing", any occurrence resolves to the same declaration.
func (c *Client) Symbol(ctx context.Context, path, name string, timeout time.Duration) (*SymbolInfo, error) {
	spec, ok := servers[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return nil, nil
	}
	if !ensureInstalled(ctx, spec.command) {
		return nil, nil
	}
	cn, err := c.connFor(ctx, spec)
	if err != nil {
		debuglog.Msg("lsp: start %s: %v", spec.command, err)
		return nil, nil
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(c.root, path)
	}
	return cn.symbol(ctx, abs, spec.languageID, name, timeout)
}

// findIdentifier returns the 0-based line and character of the first standalone
// occurrence of name. Standalone matters: searching for "Get" must not land
// inside "Getter", which would resolve to a different symbol entirely.
func findIdentifier(text, name string) (line, character int, ok bool) {
	isWord := func(r byte) bool {
		return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	}
	for i, l := range strings.Split(text, "\n") {
		from := 0
		for {
			at := strings.Index(l[from:], name)
			if at < 0 {
				break
			}
			at += from
			beforeOK := at == 0 || !isWord(l[at-1])
			end := at + len(name)
			afterOK := end >= len(l) || !isWord(l[end])
			if beforeOK && afterOK {
				return i, at, true
			}
			from = at + 1
		}
	}
	return 0, 0, false
}

func (c *conn) symbol(ctx context.Context, abs, languageID, name string, timeout time.Duration) (*SymbolInfo, error) {
	text, err := readFileString(abs)
	if err != nil {
		return nil, err
	}
	line, character, found := findIdentifier(text, name)
	if !found {
		return nil, fmt.Errorf("%q does not appear in %s", name, filepath.Base(abs))
	}

	uri := pathToURI(abs)
	c.mu.Lock()
	c.nextID++
	version := c.nextID
	c.mu.Unlock()
	if err := c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": languageID, "version": version, "text": text},
	}); err != nil {
		return nil, err
	}
	defer func() {
		_ = c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uri}})
	}()

	position := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
	}
	info := &SymbolInfo{Occurrence: line + 1}

	// Hover and definition are asked for separately and either may come back
	// empty: a local variable has a type but no interesting declaration
	// elsewhere, and a package name has a declaration but no hover text.
	if raw, err := c.call(ctx, "textDocument/hover", position, timeout); err == nil {
		info.Hover = parseHover(raw)
	}
	if raw, err := c.call(ctx, "textDocument/definition", position, timeout); err == nil {
		info.DefPath, info.DefLine = parseDefinition(raw)
	}
	if info.Hover == "" && info.DefPath == "" {
		return nil, fmt.Errorf("the language server knows nothing about %q here", name)
	}
	return info, nil
}

// parseHover copes with all three shapes the protocol has carried: a MarkupContent
// object, a bare string, and the old array of marked strings.
func parseHover(raw json.RawMessage) string {
	var wrapper struct {
		Contents json.RawMessage `json:"contents"`
	}
	if json.Unmarshal(raw, &wrapper) != nil || len(wrapper.Contents) == 0 {
		return ""
	}
	var markup struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(wrapper.Contents, &markup) == nil && markup.Value != "" {
		return strings.TrimSpace(markup.Value)
	}
	var plain string
	if json.Unmarshal(wrapper.Contents, &plain) == nil {
		return strings.TrimSpace(plain)
	}
	var parts []struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(wrapper.Contents, &parts) == nil {
		var out []string
		for _, p := range parts {
			if v := strings.TrimSpace(p.Value); v != "" {
				out = append(out, v)
			}
		}
		return strings.Join(out, "\n")
	}
	return ""
}

// parseDefinition takes either a single Location or a list of them; servers
// disagree and both are legal.
func parseDefinition(raw json.RawMessage) (string, int) {
	type location struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line int `json:"line"`
			} `json:"start"`
		} `json:"range"`
	}
	var one location
	if json.Unmarshal(raw, &one) == nil && one.URI != "" {
		return uriToPath(one.URI), one.Range.Start.Line + 1
	}
	var many []location
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return uriToPath(many[0].URI), many[0].Range.Start.Line + 1
	}
	return "", 0
}

func (c *conn) diagnose(ctx context.Context, abs, languageID string, timeout time.Duration) ([]Diagnostic, error) {
	uri := pathToURI(abs)
	text, err := readFileString(abs)
	if err != nil {
		return nil, nil
	}

	// Drain anything queued from an earlier file so a stale notification is
	// never mistaken for this file's answer.
	for {
		select {
		case <-c.updated:
			continue
		default:
		}
		break
	}

	// A version that always moves forward: reopening the same document with a
	// stale version makes servers ignore the content.
	c.mu.Lock()
	c.nextID++
	version := c.nextID
	c.mu.Unlock()

	if err := c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": languageID, "version": version, "text": text,
		},
	}); err != nil {
		return nil, nil
	}
	defer func() {
		_ = c.notify("textDocument/didClose", map[string]any{
			"textDocument": map[string]any{"uri": uri},
		})
	}()

	deadline := time.After(timeout)
	for {
		select {
		case got := <-c.updated:
			if got != uri {
				continue
			}
			c.mu.Lock()
			out := append([]Diagnostic(nil), c.diags[uri]...)
			c.mu.Unlock()
			return out, nil
		case <-deadline:
			// Silence usually means "nothing wrong" — servers are not required
			// to publish an empty list. Treating a timeout as an error would
			// cry wolf on every clean edit.
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *conn) readLoop() {
	defer c.close()
	for {
		payload, err := readMessage(c.stdout)
		if err != nil {
			return
		}
		var msg struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(payload, &msg) != nil {
			continue
		}
		// A reply to something we asked. Method is empty on a response, which
		// is what separates it from a server-initiated request carrying an id.
		if msg.ID != nil && msg.Method == "" {
			c.mu.Lock()
			waiter, ok := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.mu.Unlock()
			if ok {
				waiter <- msg.Result // buffered, so a caller that gave up cannot block this loop
			}
			continue
		}
		if msg.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params struct {
			URI         string `json:"uri"`
			Diagnostics []struct {
				Range struct {
					Start struct {
						Line      int `json:"line"`
						Character int `json:"character"`
					} `json:"start"`
				} `json:"range"`
				Severity int    `json:"severity"`
				Message  string `json:"message"`
				Source   string `json:"source"`
			} `json:"diagnostics"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			continue
		}
		out := make([]Diagnostic, 0, len(params.Diagnostics))
		for _, d := range params.Diagnostics {
			out = append(out, Diagnostic{
				Path:   uriToPath(params.URI),
				Line:   d.Range.Start.Line + 1, // LSP counts from 0; nothing else does
				Column: d.Range.Start.Character + 1,
				// 1..4 = error, warning, info, hint. 0 means the server did not
				// say, and an unlabelled problem is worth surfacing as an error
				// rather than swallowing.
				Severity: severityName(d.Severity),
				Message:  strings.TrimSpace(d.Message),
				Source:   d.Source,
			})
		}
		c.mu.Lock()
		c.diags[params.URI] = out
		c.mu.Unlock()
		select {
		case c.updated <- params.URI:
		default: // nobody waiting; the map already holds the latest
		}
	}
}

func severityName(s int) string {
	switch s {
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return "error"
	}
}

// ---- JSON-RPC framing ----

func (c *conn) send(payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead {
		return fmt.Errorf("language server is gone")
	}
	_, err = fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

func (c *conn) notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// request sends a call and waits for the server to become usable. The reply
// body is not inspected: initialize is the only request made, and all that
// matters is that it completed.
func (c *conn) request(ctx context.Context, method string, params any) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	if err := c.send(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	}); err != nil {
		return err
	}
	// The read loop owns stdout, so the handshake is confirmed by the server
	// staying alive rather than by matching the id — enough for one request,
	// and it keeps a second reader off the pipe.
	select {
	case <-time.After(150 * time.Millisecond):
		if !c.alive() {
			return fmt.Errorf("%s exited during %s", "language server", method)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// call is request with the answer kept. Used for hover and definition, where
// the reply is the whole point.
func (c *conn) call(ctx context.Context, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	reply := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.pending[id] = reply
	c.mu.Unlock()
	// Registered before sending, never after: a fast server can answer before
	// the sending goroutine gets back to the map.
	if err := c.send(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case result := <-reply:
		return result, nil
	case <-time.After(timeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("%s timed out", method)
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func readMessage(r *bufio.Reader) ([]byte, error) {
	length := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			length, _ = strconv.Atoi(strings.TrimSpace(value))
		}
	}
	if length <= 0 {
		return nil, fmt.Errorf("message with no Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// Shared returns the one client for a workspace root, creating it on first
// use. The engine re-bootstraps its registry on every model or project switch,
// and starting a fresh gopls each time would pile up processes that each spend
// seconds re-indexing the same repo. Keyed by root because a language server's
// whole job is understanding one workspace.
func Shared(root string) *Client {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if c, ok := sharedClients[root]; ok {
		return c
	}
	c := New(root)
	sharedClients[root] = c
	return c
}

// CloseShared stops every shared server. The process-wide job object already
// guarantees nothing outlives the app (internal/proc); this is for shutting
// down cleanly, and for tests.
func CloseShared() {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	for root, c := range sharedClients {
		c.Close()
		delete(sharedClients, root)
	}
}

var (
	sharedMu      sync.Mutex
	sharedClients = map[string]*Client{}
)
