package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/hook"
	"github.com/Mike0165115321/Aetox/internal/model"
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
	// EnabledProviders is the set of providers shown in the Settings sidebar
	// and the chat composer's picker. Empty means "never customized" — callers
	// resolve that case via ResolvedEnabledProviders rather than persisting a
	// default here, so old preference files don't need migrating.
	EnabledProviders []string `json:"enabled_providers,omitempty"`
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
}

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
	return pref, true, nil
}

func sanitizePreference(pref ModelPreference) ModelPreference {
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

func SaveModelPreference(pref ModelPreference) error {
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

	// Write-then-rename, not a plain WriteFile: this file holds the API keys,
	// and a truncate-then-write loses all of them if a second Aetox process
	// (another window, the CLI) writes at the same moment or the app dies
	// mid-write. Rename is atomic, so a reader sees the old file or the new
	// one, never half of either.
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
