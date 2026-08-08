// Package connect is the register of external accounts Aetox can work on the
// user's behalf with, and the one place that answers "which desk may use this".
//
// It is the thin half. Everything about *how* a service authenticates lives in
// that service's own package (internal/github, and its siblings as they arrive),
// because those grow — GitHub alone will end up holding pull requests, issues
// and checks. What lives here is only what every service has in common: an id,
// a label, the tools it contributes, and the four verbs the settings page calls.
//
// Adding a service is adding one entry to the catalog below plus its package.
// Nothing else in the app is edited, which is the property the whole shape
// exists for.
//
// Deliberately not internal/oauth's Methods: those are model sign-ins, and a
// model sign-in buys thinking while a connection buys reach. They share a
// credential vault and nothing else.
package connect

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	gh "github.com/Mike0165115321/Aetox/internal/github"
)

// Kind is how an account is attached. Only one today; the field exists because
// the settings page has to know which screen to draw before the flow starts,
// and finding that out by special-casing an id is how a second catalog begins.
type Kind string

const (
	// KindToken: the user pastes a token they minted themselves.
	KindToken Kind = "token"
	// KindOAuth: a flow hands one back. No service uses it yet — GitHub's
	// device flow needs a registered OAuth App first (internal/oauth/device.go
	// is written and waiting).
	KindOAuth Kind = "oauth"
)

// Account is who a token belongs to, flattened from whatever the service calls
// it so the desktop layer sees one shape however many services there are.
type Account struct {
	Login  string   `json:"login"`
	Name   string   `json:"name,omitempty"`
	Scopes []string `json:"scopes,omitempty"`
}

// Status is one connection as a settings page renders it. Never a token.
type Status struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Kind      Kind   `json:"kind"`
	TokenURL  string `json:"token_url,omitempty"`
	Connected bool   `json:"connected"`
	Login     string `json:"login,omitempty"`
	// Source is "connection" (stored in Aetox) or "environment" (a token set
	// outside it), so "disconnect" and "still works" stay explainable together.
	Source      string `json:"source,omitempty"`
	EnvOverride bool   `json:"env_override"`
	// For is the placement: desk names and "agent:<name>", the same vocabulary
	// an MCP server's `for:` uses. Configured says whether the user has ever set
	// it — absent means every desk, and the page must not draw that as "off".
	For        []string `json:"for"`
	Configured bool     `json:"configured"`
	// Tools are the tool names this connection contributes, so the page can say
	// what a switch actually turns off.
	Tools []string `json:"tools"`
}

// Provider is one connectable service.
type Provider struct {
	ID       string
	Label    string
	Kind     Kind
	TokenURL string
	// Tools are every tool name that exists because of this connection. A desk
	// that does not carry the connection does not see them at all — the model
	// should never be shown a door it cannot open.
	Tools []string

	connect func(context.Context, string) (Account, error)
	// verify re-checks whatever credential is in play and says whose it is.
	// Separate from status because status must never touch the network — a
	// settings page renders on every keystroke of its search box.
	verify     func(context.Context) (Account, error)
	status     func() (connected bool, login, source string, envOverride bool)
	disconnect func() error
}

// catalog is the register. One entry per service, written out rather than
// assembled by init() side effects: the list of what this build can connect to
// should be readable in one screen, and a registry populated by imports is a
// list nobody can see.
var catalog = []Provider{
	{
		ID:       "github",
		Label:    "GitHub",
		Kind:     KindToken,
		TokenURL: "https://github.com/settings/tokens/new?scopes=repo,read:org&description=Aetox",
		// plugin_install is here on purpose. It reaches GitHub like the rest,
		// so a desk with the connection switched off should not keep one tool
		// that still goes there. Installing a skill by hand is unaffected —
		// that is a button in Settings, not this tool.
		Tools: []string{
			"github_search", "github_repo_summary", "github_list_files",
			"github_read_file", "plugin_install",
		},
		connect: func(ctx context.Context, token string) (Account, error) {
			account, err := gh.Connect(ctx, token)
			return Account{Login: account.Login, Name: account.Name, Scopes: account.Scopes}, err
		},
		verify: func(ctx context.Context) (Account, error) {
			account, err := gh.Verify(ctx)
			return Account{Login: account.Login, Name: account.Name, Scopes: account.Scopes}, err
		},
		status: func() (bool, string, string, bool) {
			s := gh.CurrentStatus()
			return s.Connected, s.Login, string(s.Source), s.EnvOverride
		},
		disconnect: gh.Disconnect,
	},
}

// IDs lists every connection this build knows, which is what the config matcher
// needs so a stale row cannot resurrect an id nothing recognises.
func IDs() []string {
	out := make([]string, 0, len(catalog))
	for _, p := range catalog {
		out = append(out, p.ID)
	}
	return out
}

// Find returns one provider by id.
func Find(id string) (Provider, bool) {
	id = strings.TrimSpace(id)
	for _, p := range catalog {
		if strings.EqualFold(p.ID, id) {
			return p, true
		}
	}
	return Provider{}, false
}

// List reports every connection with its account state and placement, in
// catalog order — the page's row order is a property of the build, not of
// whichever one the user connected first.
func List() []Status {
	out := make([]Status, 0, len(catalog))
	for _, p := range catalog {
		out = append(out, statusOf(p))
	}
	return out
}

// StatusOf reports one connection.
func StatusOf(id string) (Status, bool) {
	p, ok := Find(id)
	if !ok {
		return Status{}, false
	}
	return statusOf(p), true
}

func statusOf(p Provider) Status {
	connected, login, source, envOverride := p.status()
	targets, configured := config.ConnectionTargets(p.ID)
	if targets == nil {
		targets = []string{}
	}
	return Status{
		ID: p.ID, Label: p.Label, Kind: p.Kind, TokenURL: p.TokenURL,
		Connected: connected, Login: login, Source: source, EnvOverride: envOverride,
		For: targets, Configured: configured, Tools: p.Tools,
	}
}

// Connect attaches an account and places it in one call.
//
// One call rather than two because the user chose both on one screen (owner's
// call, 2026-08-08): a connect that succeeded and a placement that failed would
// leave an account attached to nothing, looking connected and reaching nobody.
// Placement is written first for the same reason — if it fails, nothing was
// stored and the page can say so honestly.
func Connect(ctx context.Context, id, token string, targets []string) (Account, error) {
	p, ok := Find(id)
	if !ok {
		return Account{}, fmt.Errorf("unknown connection: %q", id)
	}
	if err := config.SetConnectionTargets(p.ID, targets); err != nil {
		return Account{}, err
	}
	return p.connect(ctx, token)
}

// Verify re-checks one connection against the service and reports the account.
func Verify(ctx context.Context, id string) (Account, error) {
	p, ok := Find(id)
	if !ok {
		return Account{}, fmt.Errorf("unknown connection: %q", id)
	}
	return p.verify(ctx)
}

// SetTargets moves an existing connection between desks.
func SetTargets(id string, targets []string) error {
	p, ok := Find(id)
	if !ok {
		return fmt.Errorf("unknown connection: %q", id)
	}
	return config.SetConnectionTargets(p.ID, targets)
}

// Disconnect forgets the account but keeps the placement. Reconnecting the same
// service should not make the user choose its desks again — they did not change
// their mind about where it belongs, only about which token it holds.
func Disconnect(id string) error {
	p, ok := Find(id)
	if !ok {
		return fmt.Errorf("unknown connection: %q", id)
	}
	return p.disconnect()
}

// Allows reports whether a tool may be carried, given the connection ids a desk
// or agent holds.
//
// A tool that belongs to no connection is not this package's business and is
// always allowed — the desk's own categories and deny list judge it, as they
// always have. Only a tool that exists *because* of a connection is gated here.
func Allows(tool string, enabled []string) bool {
	owner, ok := ProviderOfTool(tool)
	if !ok {
		return true
	}
	for _, id := range enabled {
		if strings.EqualFold(strings.TrimSpace(id), owner) {
			return true
		}
	}
	return false
}

// ProviderOfTool reports which connection a tool came from, if any.
func ProviderOfTool(tool string) (string, bool) {
	tool = strings.ToLower(strings.TrimSpace(tool))
	for _, p := range catalog {
		for _, name := range p.Tools {
			if strings.ToLower(name) == tool {
				return p.ID, true
			}
		}
	}
	return "", false
}

// ToolNames lists every tool that belongs to some connection, sorted. Used by
// tests and by anything that needs to reason about the gated set as a whole.
func ToolNames() []string {
	var out []string
	for _, p := range catalog {
		out = append(out, p.Tools...)
	}
	sort.Strings(out)
	return out
}
