package config

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/hook"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/safety"
)

type Config struct {
	SandboxRoot        string
	AutoApprove        bool
	ApprovalMode       string
	MaxRetries         int
	MaxPlanRetries     int
	ApprovalTimeoutSec int
	MaxOutputFiles     int
	ThinkLevel         string
	ModelProvider      string
	ModelName          string
	ModelAPIKey        string
	ModelBaseURL       string
	// ModelWireFormat picks between a provider's alternate wire formats when
	// it has one (e.g. DeepSeek's OpenAI-compatible vs Anthropic-format
	// endpoints) — see provider.Spec.AltRuntime. Empty uses the default.
	ModelWireFormat    string
	ModelTimeoutSec    int
	ModelContextTokens int
	// SpeechModelPath pins which speech model audio_transcribe uses. Empty
	// means "whatever is on disk", which is what shipped first — fine until a
	// machine has more than one, and no way at all to trade accuracy for size
	// (ggml-tiny ~31MB against ggml-base ~141MB) without moving files by hand.
	SpeechModelPath string
	// UILocale is the language the desktop UI is showing ("th", "en"). The
	// engine has no business with language — the one exception is Aetox's own
	// built-in provider, which is an onboarding surface wearing a Provider
	// interface and must speak to the user in their language (ARCHITECTURE.md
	// §40). Empty means "not set", and the built-in falls back to Thai.
	UILocale string
	// The assistant's reach, one field per kind (COMPANY.md §4) — and they are
	// spelled opposite ways on purpose, because they ship opposite ways.
	//
	// DelegateAgents is POSITIVE: handing a whole job to an เอเจน is off until
	// the user asks for it, so a zero Config has it off. DelegateHelpersOff is
	// NEGATIVE: a ซับเอเจน is the assistant's own hands in a second context, it
	// is on from the start, so a zero Config — and a preference file that has
	// never heard of the field — has it on (owner, 20 ส.ค.: "ซับเอเจน ควรจะไป
	// ระบุที่หน้าตั้งค่า และเปิดเป็นค่าเริ่มต้น").
	//
	// The alternative was two positive fields and a default applied wherever
	// somebody remembered to. That is what the first version of DelegateOn did,
	// and it meant a Config built anywhere else — a test, another entry point —
	// silently got the wrong answer. A default that lives in a code path is a
	// default that is true wherever somebody wrote it; both of these live in the
	// value, which is why the two are spelled differently rather than tidily.
	//
	// They were one field, DelegateOn, until 2026-08-20: one switch for both
	// kinds, which lived on the เอเจน settings page while quietly taking every
	// ซับเอเจน away too.
	//
	// internal/subagent keeps its own defaults (both reachable), because a
	// library that does nothing until you opt in is a library that gets called
	// wrong. The one translation sits at the boundary in internal/bootstrap.
	//
	// Measured 2026-08-20: `task` costs 710 tokens in every request with both
	// on, 629 with เอเจน alone, 599 with ซับเอเจน alone, and is not built at all
	// with neither — which is what the two switches are weighing.
	DelegateAgents     bool
	DelegateHelpersOff bool
	// WorkersOff names individual workers the ASSISTANT may not hand work to,
	// either kind. Each one left out saves ~21 tokens per message.
	//
	// One list rather than two, deliberately, where the switches above are two:
	// this is keyed by NAME, and a name is unique across both homes
	// (subagent.Conflicts refuses a collision), so the kind is always derivable
	// and a second list would be a second place to get it wrong. The switches
	// above are not keyed by anything — they are the two acts themselves.
	//
	// It does not disable a worker. The user still opens a chat with it and
	// still writes @name at it, because that is the user's own door and no
	// setting here reaches it. Anything shown for this in the UI has to say
	// whose reach is narrowed, or somebody will read "off" as "gone".
	WorkersOff []string
	// DelegateSet says somebody has answered the delegation question on this
	// machine. False means nobody has, and the shipped default applies instead of
	// the zero values above (App.resolveConfig, shippedDelegation).
	//
	// A flag rather than a sentinel value, because every state these three fields
	// can hold is also a real answer: delegation off is an answer, and an empty
	// WorkersOff is the answer "everybody is in reach". EnabledProviders solves the
	// same problem with an empty list because empty is not a real answer there.
	DelegateSet bool
}

type ConfigOptions struct {
	RootPath           string
	AutoApprove        bool
	ApprovalMode       string
	MaxRetries         int
	MaxPlanRetries     int
	ApprovalTimeout    int
	ThinkLevel         string
	ModelProvider      string
	ModelName          string
	ModelAPIKey        string
	ModelBaseURL       string
	ModelWireFormat    string
	ModelTimeout       int
	ModelContextTokens int
}

type ModelPreference struct {
	ModelProvider   string `json:"provider"`
	ModelName       string `json:"model"`
	ModelBaseURL    string `json:"base_url"`
	ModelWireFormat string `json:"wire_format,omitempty"`
	ThinkLevel      string `json:"think_level,omitempty"`
	ApprovalMode    string `json:"approval_mode,omitempty"`
	// UILocale sits next to ApprovalMode because it is the same kind of thing:
	// a choice the user made in the UI that the engine needs on next start.
	UILocale string `json:"ui_locale,omitempty"`
	// SpeechModelPath is the speech model the user picked, by absolute path
	// rather than by name: models legitimately live outside Aetox's own folder
	// (Ollama's and LM Studio's stores are scanned too, see internal/stt), and
	// a bare filename could not tell two of those apart.
	SpeechModelPath string `json:"speech_model_path,omitempty"`
	// UserName is what the user calls themselves in the sidebar footer. It
	// lives here rather than in the webview's localStorage because that store
	// is keyed by origin — a name typed under `wails dev` (…:34115) was a
	// different bucket from the built app's, so it had to be typed again on
	// every switch.
	UserName string `json:"user_name,omitempty"`
	// LastDesk is the desk the window was last at, so relaunching lands where
	// the user left off instead of at whatever the product's entrance happens
	// to be. Same reason UILocale is here: a choice made in the UI that the
	// engine needs before the first paint.
	//
	// Only ever holds a named desk. The legacy full desk is "" — the same value
	// as "nothing remembered" — and reopening a pre-desk conversation is not a
	// statement about where you want to start next time.
	LastDesk     string            `json:"last_desk,omitempty"`
	ModelAPIKeys map[string]string `json:"provider_api_keys,omitempty"`
	// ModelBaseURLs holds per-provider endpoint overrides. ModelBaseURL above
	// is the older single-slot version, which only ever held the *active*
	// provider's URL — switching away reset it to the catalog default and the
	// custom one was gone. Keyed per provider it survives, which is what a
	// local runtime on a non-default port (LM Studio's server port is
	// user-configurable) actually needs. An absent entry means "catalog
	// default"; ModelBaseURL is still read as a fallback for old files.
	ModelBaseURLs map[string]string `json:"provider_base_urls,omitempty"`
	// LearningDisabled turns off the whole learning layer: no job rows are
	// recorded and nothing can be queued for approval. Stored as the negative
	// so an absent field means enabled — the switch was added after people had
	// preference files, and defaulting it off would have made the feature
	// invisible to everyone who already had one.
	LearningDisabled bool `json:"learning_disabled,omitempty"`
	// The assistant's reach: one switch per kind, plus the per-worker trim. Each
	// is spelled so that ABSENT means what the product ships — the same rule
	// LearningDisabled above follows, applied twice with opposite answers.
	//
	// `delegate_agents_on` is positive: เอเจน delegation ships off, so a file
	// that says nothing means off. `delegate_helpers_off` is negative: ซับเอเจน
	// ship on, so a file that says nothing — every file written before today —
	// means on. Spelling either one the other way would hand somebody a setting
	// they never chose the first time they upgraded.
	//
	// DelegateOn is the field they replaced (one switch for both kinds, until
	// 2026-08-20). It is still READ — LoadModelPreference folds it in — and
	// never written again, so a file written by the old build keeps working and
	// the first save after an upgrade replaces it. Deleting it would silently
	// switch delegation off for everybody who had turned it on.
	//
	// WorkersOff stays negative and stays one list: a worker the user has said
	// nothing about is one the assistant may reach, and the trim is keyed by a
	// name that is unique across both homes.
	//
	// They persist here rather than per project, because "may my assistant hand
	// work to a specialist" is a fact about how somebody works and not about
	// which folder is open — the same reason UILocale sits here.
	DelegateAgents     bool     `json:"delegate_agents_on,omitempty"`
	DelegateHelpersOff bool     `json:"delegate_helpers_off,omitempty"`
	DelegateOn         bool     `json:"delegate_on,omitempty"` // read-only, pre-2026-08-20 files
	WorkersOff         []string `json:"agents_off,omitempty"`
	// DelegateSet records that the three fields above are an answer rather than a
	// blank. Written the first time somebody flips one of these switches; a file
	// from before it existed is read for the same fact in sanitizePreference.
	DelegateSet bool `json:"delegate_set,omitempty"`
	// EnabledProviders is the set of providers shown in the Settings sidebar
	// and the chat composer's picker. Empty means "never customized" — callers
	// resolve that case via ResolvedEnabledProviders rather than persisting a
	// default here, so old preference files don't need migrating.
	EnabledProviders []string `json:"enabled_providers,omitempty"`
	// Shells records which shell each project's commands run in — the machine's
	// own, or a WSL distro (proc.ParseBackend for the spelling).
	//
	// Keyed by project folder rather than held as one setting, because it is a
	// fact about the project and not about the person: a repo on D:\ is built
	// by the Windows toolchain and a repo under \\wsl.localhost is built by the
	// distro's, and someone who works on both would otherwise be re-picking
	// every time they switch. The "" key is the fallback for a folder that has
	// never been chosen for, which is how a user who wants WSL everywhere says
	// so once.
	Shells map[string]string `json:"shells,omitempty"`
}

// ShellFor answers which shell a project's commands should run in: the folder's
// own choice, else the default the user set for everything, else what the
// folder itself implies (a project inside a distro is built by that distro).
//
// The lookup is case-insensitive on the folder, because Windows hands the same
// directory back under different spellings depending on who asked, and a
// setting that silently stops applying when a path arrives capitalised
// differently is worse than no setting.
func (p ModelPreference) ShellFor(root string) proc.Backend {
	if setting, ok := p.shellSetting(root); ok {
		return proc.ParseBackend(setting)
	}
	if setting := strings.TrimSpace(p.Shells[""]); setting != "" {
		return proc.ParseBackend(setting)
	}
	return proc.DefaultBackendFor(root)
}

func (p ModelPreference) shellSetting(root string) (string, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", false
	}
	for key, setting := range p.Shells {
		if key != "" && strings.EqualFold(filepath.Clean(key), filepath.Clean(root)) {
			return setting, strings.TrimSpace(setting) != ""
		}
	}
	return "", false
}

// SetShellFor records a choice. An empty root sets the default for every folder
// that has not been chosen for.
func (p *ModelPreference) SetShellFor(root string, backend proc.Backend) {
	if p.Shells == nil {
		p.Shells = make(map[string]string, 1)
	}
	root = strings.TrimSpace(root)
	if root != "" {
		root = filepath.Clean(root)
		// Replace whatever spelling of this folder is already in there, so the
		// map cannot end up holding two answers for one directory.
		for key := range p.Shells {
			if key != "" && strings.EqualFold(filepath.Clean(key), root) {
				delete(p.Shells, key)
			}
		}
	}
	p.Shells[root] = proc.FormatBackend(backend)
}

// ResolvedEnabledProviders returns the providers that should actually be shown.
// Only the untouched-install case (enabled is empty — the user has never
// customized it) falls back to activeProvider, so a fresh install shows just
// what's already configured instead of the full catalog — including "aetox"
// itself (Aetox's own built-in engine, needs no key), since that is exactly
// what a genuinely fresh install is running on and what removing an active
// provider falls back to; hiding it here would contradict both. Once the user
// has customized the set at all, it is respected exactly as given
// (deduped/normalized) — the active provider is NOT force-appended, so
// explicitly disabling it (e.g. to switch away and hide it) actually takes
// effect instead of being fought by this function on every read.
func ResolvedEnabledProviders(enabled []string, activeProvider string) []string {
	if len(enabled) == 0 {
		active := strings.TrimSpace(model.NormalizeProvider(activeProvider))
		if active == "" {
			return []string{}
		}
		return []string{active}
	}
	seen := make(map[string]struct{}, len(enabled))
	out := make([]string, 0, len(enabled))
	for _, p := range enabled {
		p = strings.TrimSpace(model.NormalizeProvider(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func (p *ModelPreference) normalizeProviderKey(provider string) string {
	return strings.ToLower(strings.TrimSpace(model.NormalizeProvider(provider)))
}

func (p *ModelPreference) EnsureProviderMap() map[string]string {
	if p.ModelAPIKeys == nil {
		p.ModelAPIKeys = make(map[string]string)
	}
	return p.ModelAPIKeys
}

func (p ModelPreference) APIKeyForProvider(provider string) string {
	key := p.normalizeProviderKey(provider)
	if key == "" {
		return ""
	}
	for providerKey, value := range p.ModelAPIKeys {
		if strings.EqualFold(strings.TrimSpace(providerKey), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (p *ModelPreference) SetAPIKeyForProvider(provider, apiKey string) {
	key := p.normalizeProviderKey(provider)
	if key == "" {
		return
	}
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return
	}
	p.EnsureProviderMap()
	p.ModelAPIKeys[key] = trimmed
}

func (p ModelPreference) BaseURLForProvider(provider string) string {
	key := p.normalizeProviderKey(provider)
	if key == "" {
		return ""
	}
	for providerKey, value := range p.ModelBaseURLs {
		if strings.EqualFold(strings.TrimSpace(providerKey), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// SetBaseURLForProvider records a custom endpoint. Unlike SetAPIKeyForProvider,
// an empty value is meaningful — it deletes the override so the provider goes
// back to its catalog default, which is the only way to undo a typo'd port.
func (p *ModelPreference) SetBaseURLForProvider(provider, baseURL string) {
	key := p.normalizeProviderKey(provider)
	if key == "" {
		return
	}
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		delete(p.ModelBaseURLs, key)
		return
	}
	if p.ModelBaseURLs == nil {
		p.ModelBaseURLs = make(map[string]string)
	}
	p.ModelBaseURLs[key] = trimmed
}

func Load(opt ConfigOptions) Config {
	loadDotEnv()

	root := opt.RootPath
	if root == "" {
		root, _ = os.Getwd()
	}

	maxRetries := opt.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}

	maxPlanRetries := opt.MaxPlanRetries
	if maxPlanRetries < 0 {
		maxPlanRetries = 0
	}

	timeout := opt.ApprovalTimeout
	if timeout <= 0 {
		timeout = 60
	}

	provider := model.NormalizeProvider(opt.ModelProvider)
	if provider == "" {
		provider = "aetox"
	}

	modelName := strings.TrimSpace(opt.ModelName)
	modelAPIKey := strings.TrimSpace(opt.ModelAPIKey)
	if modelAPIKey == "" {
		modelAPIKey = model.ResolveModelAPIKey(provider)
	}
	// The second place a key enters the process, after LoadCredentials: this
	// one can come from an environment variable or the .env file, which the
	// credentials file never sees. Registering here as well means every path a
	// key arrives by is covered, and the debug log cannot print one whichever
	// way the user supplied it.
	debuglog.Redact(modelAPIKey)
	baseURL := strings.TrimSpace(opt.ModelBaseURL)
	wireFormat := strings.TrimSpace(opt.ModelWireFormat)
	modelTimeout := opt.ModelTimeout
	if modelTimeout <= 0 {
		modelTimeout = 30
	}
	modelContextTokens := opt.ModelContextTokens
	if modelContextTokens < 0 {
		modelContextTokens = 0
	}
	thinkLevel := strings.ToLower(strings.TrimSpace(opt.ThinkLevel))
	if thinkLevel == "" {
		thinkLevel = "low"
	}

	approvalMode := strings.ToLower(strings.TrimSpace(opt.ApprovalMode))
	if approvalMode == "" {
		approvalMode = string(safety.NormalizeApprovalMode(""))
	}

	return Config{
		SandboxRoot:        root,
		AutoApprove:        opt.AutoApprove,
		ApprovalMode:       approvalMode,
		MaxRetries:         maxRetries,
		MaxPlanRetries:     maxPlanRetries,
		ApprovalTimeoutSec: timeout,
		MaxOutputFiles:     2000,
		ThinkLevel:         thinkLevel,
		ModelProvider:      provider,
		ModelName:          modelName,
		ModelAPIKey:        modelAPIKey,
		ModelBaseURL:       baseURL,
		ModelWireFormat:    wireFormat,
		ModelTimeoutSec:    modelTimeout,
		ModelContextTokens: modelContextTokens,
	}
}

// DataRoot is the single directory every piece of Aetox's own persisted data
// lives under — preferences, permissions, sessions (desktop/db.go), the
// downloaded rtk binary (internal/rtk/install.go), WebView2 profiles
// (desktop/main.go, browser.go), audit logs. One well-defined location we
// design and own, rather than each subsystem picking its own OS convention
// (ARCHITECTURE.md §14).
//
// AETOX_DATA_ROOT overrides it — set by desktop/wails-dev.bat during dev so
// repeated `wails dev` runs don't grow session/webview/preference data
// unbounded on the system drive. Unset (the production default) resolves to
// <UserConfigDir>/aetox — normal, expected behavior for an installed app.
//
// Skills are Aetox-owned but live under their own home-level dotdir
// ~/.aetox/skills (skill.DefaultSkillsDir), not here — the same convention as
// ~/.agents (opencode) and ~/.claude (Claude Code), so plugin_install and
// discovery stay in Aetox's own directory instead of sharing another tool's.
func DataRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv("AETOX_DATA_ROOT")); override != "" {
		return override, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			configDir = filepath.Join(home, ".config")
		} else {
			configDir = os.TempDir()
		}
	}
	return filepath.Join(configDir, "aetox"), nil
}

// ProjectKey is one folder's stable identity: its readable base name plus a
// short hash of the full path, so two folders both called "app" never share a
// key. Case-insensitive and path-cleaned, because a project reopened as
// `d:\work\app` is the same project as one opened as `D:/Work/App/`.
//
// It lives here rather than in the desktop because two subsystems now key on
// it and they must agree to the byte: the store files a session's history
// under it (desktop/sessions.go), and the agent's per-project memory is the
// file named by it (internal/learned). A second implementation that drifted
// would file the memory of a project under a name its own history does not
// use, and nothing would report an error — the memory would simply never be
// read again.
func ProjectKey(root string) string {
	root = strings.TrimSpace(root)
	sum := sha1.Sum([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Base(root) + "-" + hex.EncodeToString(sum[:4])
}

func PreferencePath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "model-preference.json"), nil
}

func LegacyPreferencePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		configDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "aetox")
	}
	return filepath.Join(configDir, "aetox-cli", "model-preference.json")
}

// UserGlobalContextPath is the old single-file location for cross-project
// instructions, pre-dating IdentityDir. Kept only so the desktop app can
// migrate its content into IdentityDir on first run; new code should use
// IdentityDir instead (ARCHITECTURE.md §11 row 3).
func UserGlobalContextPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "AETOX.md"), nil
}

// IdentityDir holds every markdown file that always rides along with the AI
// across every project — the "AI Identity" layer edited from the desktop
// sidebar (e.g. context.md, skills.md, whatever the user wants always
// attached). Every *.md file in it is folded into every system prompt build
// (internal/prompt.BuildWithReport, ARCHITECTURE.md §11 row 3). Replaces the
// single-file UserGlobalContextPath.
func IdentityDir() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "identity"), nil
}

func PermissionsPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "permissions.json"), nil
}

// HooksPath is where the user's own PreToolUse/PostToolUse commands live.
// Beside permissions.json rather than inside it: one file answers "may this
// run", the other "what else should run", and a user editing one should not
// risk the other.
func HooksPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "hooks.json"), nil
}

// LoadHooks reads the user's tool hooks. Missing file is the normal case.
func LoadHooks() (hook.Config, error) {
	path, err := HooksPath()
	if err != nil {
		return hook.Config{}, err
	}
	return hook.Load(path)
}

// LoadPermissions reads the user's per-tool permission overrides, if any.
// Missing file is not an error — it just means no rules are configured yet.
func LoadPermissions() (safety.PermissionConfig, error) {
	path, err := PermissionsPath()
	if err != nil {
		return safety.PermissionConfig{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return safety.PermissionConfig{}, nil
		}
		return safety.PermissionConfig{}, err
	}
	var cfg safety.PermissionConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return safety.PermissionConfig{}, err
	}
	return cfg, nil
}

// MCPServerConfig is the persisted, provider-agnostic description of one MCP
// server. It is a plain DTO so this package needn't depend on internal/mcp;
// the wiring layer translates it into an mcp.Server. A non-empty URL means a
// remote (streamable HTTP) server, otherwise Command is a local stdio server —
// deliberately no separate "type" field that could fall out of sync.
type MCPServerConfig struct {
	Name        string            `json:"name"`
	Command     []string          `json:"command,omitempty"`
	Cwd         string            `json:"cwd,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	TimeoutMs   int               `json:"timeout_ms,omitempty"`
	// Disabled (not Enabled) so the zero value keeps servers in pre-existing
	// config files switched on without any migration.
	Disabled bool `json:"disabled,omitempty"`
	// For names who this server belongs to — the desks that carry its tools,
	// and (planned) the agents that bring it along. Owner's call, 2026-08-06.
	//
	// The server says where it goes, rather than each desk listing the servers
	// it wants, because only one of those can be written at the right moment. A
	// desk manifest ships compiled into the app, before any of the user's
	// servers exist — so `mcp:` in a bundled manifest could name nothing, every
	// desk carried no servers, and a server the user had configured connected,
	// registered, and was filtered off every desk. The model then reported
	// having no MCP tools, which from where it sat was true.
	//
	// This is also the line that keeps a server the user installed *for an
	// agent* off the main assistant's tool block. "Attach everything" would
	// have merged the two the moment a second server existed.
	//
	// Disabled and For are different switches and both are wanted: Disabled
	// means do not connect at all, For means who sees it once connected.
	//
	// No `omitempty`, unlike every other optional field here. An empty list is
	// a decision — "connected, shown to nobody" — and omitting it on write
	// would make it indistinguishable from a file predating the field, which
	// the migration fills in with the default. Switching a server off for
	// everyone would then switch itself back on at the next launch.
	For []string `json:"for"`
	// Tools, when non-empty, is the only tools taken from this server — see
	// mcp.Server.Tools for why one would write such a list.
	//
	// `omitempty`, unlike For, and the difference is not an oversight. An empty
	// For means "shown to nobody", which is a decision worth persisting; an
	// empty Tools means "take everything", which is the absence of a decision.
	Tools []string `json:"tools,omitempty"`
	// Source records that this entry arrived with an agent package rather than
	// being written by the user — "agent:<name>", the same spelling `for:` uses.
	//
	// It exists so that removing a bought agent can remove exactly what it
	// brought and nothing else. `for:` cannot answer that: a user who later
	// places the same server on a second agent has not thereby made it theirs
	// to keep, and a server the user added by hand and then pointed at an agent
	// looks identical from the placement alone.
	//
	// Empty is the normal state and means the user owns this entry. An install
	// that reuses a server somebody already had leaves it empty on purpose —
	// what was already yours stays yours.
	Source string `json:"source,omitempty"`
}

// MCPDefaultDesks is where a server with no `for:` lands: the two general
// desks. It is applied once, by MigrateMCPServerOwners, and written back into
// the file — not treated as a standing default.
//
// Writing it back is the whole point. A silent default is a rule the user
// cannot see, edit or switch off, and this field exists to be a switch. After
// the migration the file says exactly what is true, and an empty `for` from
// then on means what it says: attached nowhere.
var MCPDefaultDesks = []string{"assistant", "coding"}

func MCPServersPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "mcp-servers.json"), nil
}

// LoadMCPServers reads the configured MCP servers. A missing file is not an
// error — it just means none are configured yet.
func LoadMCPServers() ([]MCPServerConfig, error) {
	path, err := MCPServersPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var servers []MCPServerConfig
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, err
	}
	return servers, nil
}

// MCPAgentPrefix marks an entry in `for:` as naming one of the team rather than
// a desk. Defined here, next to the file it is written into, because three
// places say it — the settings page building the toggles, the resolver reading
// them, and the agent asking what it may carry — and a prefix spelled by hand
// in any of them is a switch that silently stops matching.
const MCPAgentPrefix = "agent:"

// MCPServersForDesk returns the servers that desk carries.
//
// The empty desk name is the pre-modes full desk and carries everything, which
// is what a nil *mode.Mode has always meant.
func MCPServersForDesk(desk string) []string {
	return mcpServersFor(strings.TrimSpace(desk), strings.TrimSpace(desk) == "")
}

// MCPServersForAgent returns the servers the user pointed at this agent — the
// tools it brings to a job that the desk's own assistant never carries.
//
// An empty name carries nothing, the opposite of the desk rule above and
// deliberately so: "no desk" is a real state that means the legacy full desk,
// while "no agent" is a caller that failed to say who is asking, and answering
// that with every server would hand a nameless delegate the lot.
func MCPServersForAgent(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return mcpServersFor(MCPAgentPrefix+name, false)
}

// mcpServersFor is the one matcher behind both. A desk name and an
// "agent:<name>" entry are the same kind of thing — an owner id, exactly as the
// settings page writes it — so they are compared the same way rather than by
// two rules that could drift apart.
//
// Disabled servers are left out of both: one that is never connected has no
// tools to carry, whoever it was pointed at.
func mcpServersFor(owner string, all bool) []string {
	MigrateMCPServerOwners()
	servers, err := LoadMCPServers()
	if err != nil {
		return nil
	}
	var out []string
	for _, s := range servers {
		if s.Disabled || strings.TrimSpace(s.Name) == "" {
			continue
		}
		if all {
			out = append(out, s.Name)
			continue
		}
		for _, entry := range s.For {
			if strings.EqualFold(strings.TrimSpace(entry), owner) {
				out = append(out, s.Name)
				break
			}
		}
	}
	return out
}

var migratedMCPOwners sync.Once

// MigrateMCPServerOwners fills in `for:` on servers configured before the field
// existed, and writes the file back so the user can see and change it.
//
// Only servers with the key entirely absent are touched. An explicit `"for":
// []` is a switch the user turned off and must survive — which is why the
// decision is made on the raw JSON rather than on the decoded struct, where
// absent and empty are the same nil.
func MigrateMCPServerOwners() {
	migratedMCPOwners.Do(migrateMCPServerOwners)
}

func migrateMCPServerOwners() {
	path, err := MCPServersPath()
	if err != nil {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return // no file is the normal state, not an error
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return // a file we cannot parse is the user's to fix, not ours to rewrite
	}
	changed := false
	for _, e := range entries {
		if _, ok := e["for"]; ok {
			continue
		}
		owners, err := json.Marshal(MCPDefaultDesks)
		if err != nil {
			return
		}
		e["for"] = owners
		changed = true
	}
	if !changed {
		return
	}
	payload, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, payload, 0o600)
}

func SaveMCPServers(servers []MCPServerConfig) error {
	path, err := MCPServersPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func EnvFilePath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".env"), nil
}

func LoadModelPreference() (ModelPreference, bool, error) {
	var pref ModelPreference
	path, err := PreferencePath()
	if err != nil {
		return pref, false, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return pref, false, err
		}
		// try migrating from old path
		legacy := LegacyPreferencePath()
		if legacyRaw, legacyErr := os.ReadFile(legacy); legacyErr == nil {
			if unmarshalErr := json.Unmarshal(legacyRaw, &pref); unmarshalErr == nil {
				pref = sanitizePreference(pref)
				_ = SaveModelPreference(pref)
				_ = os.Remove(legacy)
				return pref, true, nil
			}
		}
		return pref, false, nil
	}

	if err := json.Unmarshal(raw, &pref); err != nil {
		return pref, false, err
	}
	pref = sanitizePreference(pref)
	// Keys live in credentials.json now; anything still in the settings file is
	// from before the split and gets moved on the way past. Callers get one
	// struct with everything in it either way — the storage split is this
	// package's business, not theirs.
	migrateCredentialsOutOfPreferences(&pref)
	if creds, err := LoadCredentials(); err == nil && len(creds.ModelAPIKeys) > 0 {
		pref.ModelAPIKeys = creds.ModelAPIKeys
	}
	return pref, true, nil
}

func sanitizePreference(pref ModelPreference) ModelPreference {
	// `delegate_on` was one switch for both kinds until 2026-08-20, and it meant
	// both. Folding it into the เอเจน half is the whole migration: the ซับเอเจน
	// half is on for everybody now, so somebody who had delegation on keeps
	// exactly what they had, and somebody who never turned it on gains the hands
	// they were silently missing.
	//
	// Cleared on the way past so the next save writes the new key and drops the
	// old one. Folding on every load rather than migrating the file in place is
	// the same choice the credentials split made: a user who never opens
	// settings again keeps working, and the file is rewritten the first time
	// they do.
	if pref.DelegateOn {
		pref.DelegateAgents = true
		pref.DelegateOn = false
	}
	// A file written before delegate_set existed still says whether anybody ever
	// answered: any of the three delegation fields carrying a non-zero value is
	// somebody's choice, and a machine at every default never made one. Read on
	// the way past rather than migrated, same as the fold above — so an install
	// that had turned delegation on keeps exactly what it had, and one that never
	// touched it gets the shipped default instead of the zero values.
	if pref.DelegateAgents || pref.DelegateHelpersOff || len(pref.WorkersOff) > 0 {
		pref.DelegateSet = true
	}
	pref.ModelName = strings.TrimSpace(pref.ModelName)
	if looksLikeAPIKey(pref.ModelName) {
		pref.ModelName = ""
	}
	pref.ModelBaseURL = strings.TrimSpace(pref.ModelBaseURL)
	if looksLikeAPIKey(pref.ModelBaseURL) {
		pref.ModelBaseURL = ""
	}
	return pref
}

func looksLikeAPIKey(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 20 {
		return false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "sk-") {
		return true
	}
	if strings.HasPrefix(lower, "sk-") && len(s) > 30 {
		return true
	}
	return false
}

// SaveModelPreference persists the user's settings, sending the API keys to
// the credentials file on the way through (see credentials.go).
//
// Callers still hand over one struct with the keys in it, because splitting the
// storage is not a reason to split every call site — a caller that had to
// remember two saves is a caller that can do one and lose the other. The keys
// are cleared from the copy that reaches the preference file and left on the
// caller's struct, which is still the whole picture in memory.
func SaveModelPreference(pref ModelPreference) error {
	if len(pref.ModelAPIKeys) > 0 {
		if err := SaveCredentials(Credentials{ModelAPIKeys: pref.ModelAPIKeys}); err != nil {
			return err
		}
		pref.ModelAPIKeys = nil // a value copy: the caller's struct is untouched
	}
	return saveModelPreferenceFile(pref)
}

// saveModelPreferenceFile writes the settings file exactly as given. Separate
// from SaveModelPreference so the migration can rewrite the file *without*
// re-saving the credentials it is in the middle of moving.
func saveModelPreferenceFile(pref ModelPreference) error {
	path, err := PreferencePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	payload, err := json.Marshal(pref)
	if err != nil {
		return err
	}

	// Write-then-rename, not a plain WriteFile: a truncate-then-write loses the
	// file's contents if a second Aetox process (another window, the CLI)
	// writes at the same moment or the app dies mid-write. Rename is atomic, so
	// a reader sees the old file or the new one, never half of either.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func loadDotEnv() {
	envPath, err := EnvFilePath()
	if err != nil {
		return
	}
	raw, err := os.ReadFile(envPath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, `"'`)
			if key != "" && value != "" {
				os.Setenv(key, value)
			}
		}
	}
}
