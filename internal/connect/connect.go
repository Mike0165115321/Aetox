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
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/Mikedev115/Aetox/internal/automation/n8n"
	"github.com/Mikedev115/Aetox/internal/automation/windmill"
	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/debuglog"
	gh "github.com/Mikedev115/Aetox/internal/github"
)

// Family groups the services that are interchangeable — several products doing
// the one job, where a user may hold more than one and works on one at a time.
//
// It exists so that "which automation engines do I have" is a question the
// catalog answers, rather than a list of ids written down a second time in the
// UI and left to rot. GitHub belongs to no family: nothing substitutes for it,
// so grouping it would be inventing a set with one member.
//
// Also the shape the `needs:` line of an agent profile will eventually want —
// an automation agent needs *an* engine, not n8n specifically (see
// docs/AUTOMATION-ENGINES.md).
type Family string

const (
	// FamilyAutomation: n8n, Windmill, and whatever comes next. They are the
	// clock; Aetox is the hands (§92.3).
	FamilyAutomation Family = "automation"
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
	// Family groups interchangeable services, so a picker can offer "the
	// automation engines you have" without keeping its own list of ids.
	Family string `json:"family,omitempty"`
	// HomeAgent, when set, says placement is locked to that agent (see
	// Provider.HomeAgent): the page draws the fact instead of a picker.
	HomeAgent string `json:"home_agent,omitempty"`
	// DefaultAgents are the agents an unplaced connection already reaches (see
	// Provider.DefaultAgents). The page ticks them, because the engine acts on
	// them: a chip drawn unticked over a grant that is really in force is the
	// page telling the user something that is not true.
	DefaultAgents []string `json:"default_agents,omitempty"`
	// NeedsBaseURL says this service has no address of its own — the user runs
	// it, and only the user knows where. The page has to know before it draws
	// the form, which is why it is a field rather than something inferred from
	// the id.
	NeedsBaseURL bool `json:"needs_base_url"`
	// BaseURL is the address currently stored, so a reconnect shows what was
	// typed last time instead of an empty box. Never a secret.
	BaseURL string `json:"base_url,omitempty"`
	// BaseURLHint is the example to show in the empty field — the address the
	// service listens on out of the box, which is what it will be for most
	// people the first time.
	BaseURLHint string `json:"base_url_hint,omitempty"`
	// StartCommand is how to bring this service up, as the user typed it. Empty
	// means Aetox has not been told and the page draws no start button.
	StartCommand string `json:"start_command,omitempty"`
	// Reachable is whether the address answered the last time the page asked.
	// Never filled by List/StatusOf — those must not touch the network — and
	// only by the deliberate check the page runs.
	Reachable bool `json:"reachable,omitempty"`
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
	//
	// Tool names are prefixed with the provider's own id — `n8n_workflow_create`,
	// not `workflow_create`. Two services that do the same job would otherwise
	// want the same name, and ProviderOfTool answers with the first match in
	// catalog order: whichever vendor was added first would silently own the
	// other's tools, and a desk that switched Windmill off would keep them.
	Tools []string

	// Family groups interchangeable services. "" for a service that stands
	// alone. See the Family type.
	Family Family
	// NeedsBaseURL marks a service the user hosts. See Status.NeedsBaseURL.
	NeedsBaseURL bool
	BaseURLHint  string
	// RequiresAccount says these tools cannot do anything at all without an
	// attached account, so they are withheld until there is one.
	//
	// Not true of every connection, which is why it is a field. GitHub's read
	// tools work unauthenticated at a lower rate limit — withholding them from
	// somebody who has not connected an account would take away something that
	// used to work. n8n's cannot: there is no public n8n, no default host, and
	// no anonymous mode; every one of its tools would fail on its first call
	// with "not connected".
	//
	// The cost of getting this wrong is not just a wasted turn. Every tool
	// offered to the model is paid for in the tool block of every single
	// request, whether or not it is ever called (see the budget guard in
	// desktop/tool_budget_test.go), so a tool nobody can use is a tax on
	// everybody who cannot use it.
	RequiresAccount bool
	// TokenPath is where the key is minted INSIDE a self-hosted service. There
	// is no fixed URL to put in TokenURL for these — the page is on the user's
	// own machine — so the address is joined on once they have said where that
	// is, and the settings row grows its "create a token" button at the moment
	// it can actually go somewhere.
	TokenPath string
	// HomeAgent locks placement to one of the office's agents: this connection
	// is that agent's workstation, and `for:` may name it and nobody else
	// (owner's call, 2026-08-10: "หน้าออโตเมชั่นใช้ได้แค่เอเจนออโตเมชั่น นั่นคือ
	// โต๊ะทำงานของมัน").
	//
	// It exists because the general placement vocabulary was wrong for engines.
	// A connection with eight possible audiences and one real reader is not
	// flexibility, it is eight ways to stand in a state where every screen says
	// "connected" and the one agent that could use it holds nothing — which is
	// exactly the state 2026-08-10 was spent in. Locked, the placement has two
	// values: at its agent, or switched off. Both are states the room's own
	// picker writes, and neither can strand the tools somewhere nothing reads.
	//
	// "" for every connection that is not an agent's workstation; those keep
	// the full vocabulary.
	HomeAgent string

	// DefaultAgents are the office's agents that carry this connection when
	// nobody has placed it yet. HomeAgent's softer relative: a lock draws a
	// fact instead of a picker and admits no other audience, this only decides
	// where an untouched connection starts and every chip stays clickable.
	//
	// It exists because "unplaced" was one answer for every service — every
	// desk, no agent — and that is wrong for a connection whose whole reason to
	// exist is one specialist. GitHub's tools are the github agent's trade
	// (owner, 19 ส.ค.: "ค่าเริ่มต้นเป็นเอเจน กิตฮับเลยสิครับ"), and having to
	// hand them to it by hand on every install is a default that disagrees with
	// the product.
	//
	// It ADDS, never subtracts. The desks keep an unplaced connection exactly as
	// before, because taking one away from the main assistant on an install
	// nobody has touched would be a capability removed by an upgrade — the one
	// thing the placement rules were written to prevent. Narrowing it to the
	// agent alone is one click on this page, and it is the user's click.
	DefaultAgents []string

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
		//
		// `github` is the packed tool the model is actually offered and is what
		// this gate has to answer about, or the connection would be switched on
		// and hand over nothing. The four names under it stay listed because
		// they are still the vocabulary a profile narrows with, and a desk that
		// holds the connection should carry them by either spelling.
		Tools: []string{
			"github",
			"github_search", "github_repo_summary", "github_list_files",
			"github_read_file", "plugin_install",
		},
		// The agent whose whole trade this is. See Provider.DefaultAgents.
		DefaultAgents: []string{"github"},
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
	{
		// The automation room's first engine (§92.3). It is in this catalog and
		// not in a register of its own because "an account Aetox acts on your
		// behalf with" is exactly what it is — the room shows the state, this
		// file owns it, and there is one place a token is typed.
		ID:     "n8n",
		Label:  "n8n",
		Kind:   KindToken,
		Family: FamilyAutomation,
		// No fixed TokenURL: the page that mints a key is inside the user's own
		// instance. Settings → n8n API, which is this path once we know the host.
		NeedsBaseURL: true,
		BaseURLHint:  "http://localhost:5678",
		TokenPath:    "/settings/api",
		// The automation room is a chat with the automation agent (desks.ts:
		// `chair: 'automation'`), so that agent is the engines' one audience.
		HomeAgent: "automation",
		// Nothing here works without an account: no public n8n, no default
		// host, no anonymous mode. Until one is attached these five stay out of
		// the tool block entirely rather than sitting in it failing.
		RequiresAccount: true,
		// Prefixed with the vendor id, and that is not decoration — see
		// Provider.Tools. Windmill will want `workflow_create` too, and
		// ProviderOfTool answers with the first match in catalog order.
		// `n8n` is the packed tool the model is offered (skill/packed.go); the
		// five workflow names under it stay listed because they are still the
		// vocabulary a profile narrows with — same both-spellings rule as the
		// github provider above.
		Tools: []string{
			"n8n",
			"n8n_workflow_list", "n8n_workflow_read",
			"n8n_workflow_create", "n8n_workflow_update", "n8n_workflow_activate",
			// The desktop's power switch (engine_server.go). In this list so it
			// travels the same gate as the API tools: to the engine's agent,
			// never to a desk.
			"n8n_server_start",
		},
		connect: func(ctx context.Context, token string) (Account, error) {
			account, err := n8n.Connect(ctx, token)
			return Account{Login: account.Login, Name: account.Name}, err
		},
		verify: func(ctx context.Context) (Account, error) {
			account, err := n8n.Verify(ctx)
			return Account{Login: account.Login, Name: account.Name}, err
		},
		status: func() (bool, string, string, bool) {
			s := n8n.CurrentStatus()
			// No env override: n8n has no conventional environment variable, and
			// inventing one would be a second way to configure the same thing.
			return s.Connected, s.Login, string(s.Source), false
		},
		disconnect: n8n.Disconnect,
	},
	{
		// The second engine, and the one the shape was built for. §92.3 settled
		// that Aetox supports more than one "from the first line of code",
		// because an integration shaped around n8n alone makes this a rewrite.
		// It is not: everything below is one catalog entry and one package.
		ID:     "windmill",
		Label:  "Windmill",
		Kind:   KindToken,
		Family: FamilyAutomation,
		// The token page is inside the user's own instance, like n8n's. It is a
		// fragment route, which is why the path carries the `#`.
		NeedsBaseURL:    true,
		BaseURLHint:     "http://localhost:8000",
		TokenPath:       "/#user-settings",
		RequiresAccount: true,
		HomeAgent:       "automation",
		Tools: []string{
			"windmill",
			"windmill_workspace_list", "windmill_flow_list",
			"windmill_flow_read", "windmill_flow_create", "windmill_flow_update",
			"windmill_server_start",
		},
		connect: func(ctx context.Context, token string) (Account, error) {
			account, err := windmill.Connect(ctx, token)
			return Account{Login: account.Login, Name: account.Name}, err
		},
		verify: func(ctx context.Context) (Account, error) {
			account, err := windmill.Verify(ctx)
			return Account{Login: account.Login, Name: account.Name}, err
		},
		status: func() (bool, string, string, bool) {
			s := windmill.CurrentStatus()
			return s.Connected, s.Login, string(s.Source), false
		},
		disconnect: windmill.Disconnect,
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

// AgentDefaults is which agents carry each connection before anybody places it.
//
// Handed to the resolver rather than read by it: the readers live in config,
// which cannot import this package (this one imports config), and that boundary
// is the reason the placement rules have stayed one file. Empty for every
// service that has no opinion, which is most of them.
func AgentDefaults() map[string][]string {
	out := map[string][]string{}
	for _, p := range catalog {
		if len(p.DefaultAgents) > 0 {
			out[p.ID] = append([]string(nil), p.DefaultAgents...)
		}
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
	baseURL, startCommand := "", ""
	tokenURL := p.TokenURL
	if p.NeedsBaseURL {
		baseURL = config.ConnectionBaseURL(p.ID)
		startCommand = config.ConnectionStartCommand(p.ID)
		// Only once there is somewhere to point. A button labelled "create a
		// token" that opens `/settings/api` on no host at all is worse than no
		// button: it looks broken rather than not-yet-applicable.
		if baseURL != "" && p.TokenPath != "" {
			tokenURL = baseURL + p.TokenPath
		}
	}
	return Status{
		ID: p.ID, Label: p.Label, Kind: p.Kind, TokenURL: tokenURL,
		Connected: connected, Login: login, Source: source, EnvOverride: envOverride,
		For: targets, Configured: configured, Tools: p.Tools, Family: string(p.Family),
		HomeAgent:     p.HomeAgent,
		DefaultAgents: p.DefaultAgents,
		NeedsBaseURL:  p.NeedsBaseURL, BaseURL: baseURL, BaseURLHint: p.BaseURLHint,
		StartCommand: startCommand,
	}
}

// SetStartCommand records how to bring a self-hosted service up.
func SetStartCommand(id, command string) error {
	p, ok := Find(id)
	if !ok {
		return fmt.Errorf("unknown connection: %q", id)
	}
	if !p.NeedsBaseURL {
		// A hosted service is not something this machine starts. Storing a
		// command for one would put a button on a row that can only fail.
		return fmt.Errorf("%s ไม่ใช่บริการที่รันบนเครื่องนี้", p.Label)
	}
	return config.SetConnectionStartCommand(p.ID, command)
}

// AgentTarget spells one of the team as a `for:` entry. The prefix belongs to
// config and is read from there, so the two halves of the vocabulary cannot
// drift — a placement written with the wrong prefix matches nothing and grants
// nothing, silently.
func AgentTarget(agent string) string {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return ""
	}
	return config.MCPAgentPrefix + agent
}

// InFamily reports every service in one family, in catalog order.
//
// The caller that needs this is a picker: "which automation engine am I working
// on" is a real question the moment somebody holds two, and answering it from a
// list of ids kept in the UI would be a second catalog that goes stale the day a
// third engine ships.
func InFamily(family Family) []Status {
	out := []Status{}
	for _, p := range catalog {
		if p.Family == family && family != "" {
			out = append(out, statusOf(p))
		}
	}
	return out
}

// NormalizeBaseURL turns what a person types into something a request can be
// built from, or says why it cannot be.
//
// People paste `localhost:5678`, `http://localhost:5678/`, and occasionally the
// address of the page they were looking at when they made the key. Scheme is
// filled in rather than demanded — nobody types http:// by choice — and a
// trailing slash is dropped so that joining a path never produces `//`.
//
// What it refuses is what would send the user's token somewhere unintended: a
// scheme that is not http(s), and a URL with no host at all.
func NormalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("ต้องระบุที่อยู่ของเซิร์ฟเวอร์")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("ที่อยู่ไม่ถูกต้อง: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", fmt.Errorf("รองรับเฉพาะ http และ https — ไม่ใช่ %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("ที่อยู่นี้ไม่มีชื่อเครื่อง")
	}
	// Path is kept: a Windmill behind a reverse proxy can genuinely live under
	// a prefix. Query and fragment are not — they are the tail of a page URL
	// somebody copied out of their browser, and carrying them into every request
	// would break each one.
	return strings.TrimRight((&url.URL{
		Scheme: strings.ToLower(parsed.Scheme),
		Host:   parsed.Host,
		Path:   parsed.Path,
	}).String(), "/"), nil
}

// Connect attaches an account and places it in one call.
//
// One call rather than two because the user chose both on one screen (owner's
// call, 2026-08-08): a connect that succeeded and a placement that failed would
// leave an account attached to nothing, looking connected and reaching nobody.
// Placement is written first for the same reason — if it fails, nothing was
// stored and the page can say so honestly.
//
// baseURL joins them for the same reason and is written before either: for a
// self-hosted service it is not a setting the connect step could be done
// without — it IS where the connect request goes, and the provider reads it
// back from storage the moment this hands over.
func Connect(ctx context.Context, id, token, baseURL string, targets []string) (Account, error) {
	p, ok := Find(id)
	if !ok {
		return Account{}, fmt.Errorf("unknown connection: %q", id)
	}
	if p.NeedsBaseURL {
		normalized, err := NormalizeBaseURL(baseURL)
		if err != nil {
			return Account{}, err
		}
		if err := config.SetConnectionBaseURL(p.ID, normalized); err != nil {
			return Account{}, err
		}
	}
	if err := config.SetConnectionTargets(p.ID, p.lockTargets(targets)); err != nil {
		return Account{}, err
	}
	return p.connect(ctx, token)
}

// lockTargets applies the provider's HomeAgent lock to a placement before it
// is written. On the two functions that write placement rather than on the
// readers, because the readers live in config, which cannot know a family —
// and because a rule enforced at one narrow door is a rule that cannot drift.
//
// The two shapes that come out, and why each is what it is:
//
//   - nil (never placed) becomes the home itself. The general default for nil
//     is "every desk", which for a locked connection is precisely the state
//     the lock exists to forbid — and a workstation nobody has moved belongs
//     to its owner, not to everybody.
//   - anything else keeps at most the home, in a NON-nil slice: an empty list
//     must survive as "switched off everywhere" (see ConnectionItem.For) and
//     must never collapse into the nil that means the opposite.
func (p Provider) lockTargets(targets []string) []string {
	if p.HomeAgent == "" {
		return targets
	}
	home := AgentTarget(p.HomeAgent)
	if targets == nil {
		return []string{home}
	}
	out := []string{}
	for _, t := range targets {
		if strings.EqualFold(strings.TrimSpace(t), home) {
			out = append(out, home)
			break
		}
	}
	return out
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
	return config.SetConnectionTargets(p.ID, p.lockTargets(targets))
}

// EnforceHomes repairs any stored placement that predates its provider's
// HomeAgent lock — a row written back when n8n could be pointed at any desk
// stays in the file forever, and the lock would otherwise hold only for users
// who never touched the old picker.
//
// Called once at startup. Rows the user never configured are left alone: there
// is nothing written to repair, and the first write will arrive through a
// locked door anyway. A repaired row keeps what the lock permits — at its
// agent if the old list included it, otherwise switched off — rather than
// guessing that a placement the user pointed elsewhere was meant for the home.
func EnforceHomes() {
	for _, p := range catalog {
		if p.HomeAgent == "" {
			continue
		}
		targets, configured := config.ConnectionTargets(p.ID)
		if !configured {
			continue
		}
		locked := p.lockTargets(targets)
		if slices.Equal(locked, targets) {
			continue
		}
		if err := config.SetConnectionTargets(p.ID, locked); err != nil {
			debuglog.Msg("connect: enforcing %s home placement: %v", p.ID, err)
		}
	}
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
	placed := false
	for _, id := range enabled {
		if strings.EqualFold(strings.TrimSpace(id), owner) {
			placed = true
			break
		}
	}
	if !placed {
		return false
	}
	// Placed is not the same as usable. A connection nobody has attached an
	// account to yet is carried by every desk (config.ConnectionsForDesk: an
	// unplaced connection defaults to everywhere), which is right for tools
	// that work without one and wrong for tools that cannot exist without one.
	if p, found := Find(owner); found && p.RequiresAccount {
		connected, _, _, _ := p.status()
		return connected
	}
	return true
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
