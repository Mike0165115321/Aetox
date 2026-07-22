package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

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
	ModelTimeoutSec    int
	ModelContextTokens int
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
	ModelTimeout       int
	ModelContextTokens int
}

type ModelPreference struct {
	ModelProvider string            `json:"provider"`
	ModelName     string            `json:"model"`
	ModelBaseURL  string            `json:"base_url"`
	ThinkLevel    string            `json:"think_level,omitempty"`
	ApprovalMode  string            `json:"approval_mode,omitempty"`
	ModelAPIKeys  map[string]string `json:"provider_api_keys,omitempty"`
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
		provider = "noop"
	}

	modelName := strings.TrimSpace(opt.ModelName)
	modelAPIKey := strings.TrimSpace(opt.ModelAPIKey)
	if modelAPIKey == "" {
		modelAPIKey = model.ResolveModelAPIKey(provider)
	}
	baseURL := strings.TrimSpace(opt.ModelBaseURL)
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
// Deliberately NOT used for things designed to be shared with the wider
// ecosystem: skill discovery scans ~/.agents/skills and ~/.claude/skills
// (internal/skill/discovery.go), and plugin_install writes into
// ~/.agents/skills on purpose (internal/skill/github_tools.go) — those are
// intentionally external, shared conventions (the same paths OpenCode/Claude
// Code use), not ours to own or relocate.
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

// UserGlobalContextPath is where a user's cross-project AETOX.md instructions
// live (the prompt layer's "user global" layer — ARCHITECTURE.md §11), same
// directory as PreferencePath/PermissionsPath.
func UserGlobalContextPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "AETOX.md"), nil
}

func PermissionsPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "permissions.json"), nil
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

func SavePermissions(cfg safety.PermissionConfig) error {
	path, err := PermissionsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

// MCPServerConfig is the persisted, provider-agnostic description of one local
// MCP server (phase 1: stdio only — see MCP-SUPPORT-PLAN.md §4). It is a plain
// DTO so this package needn't depend on internal/mcp; the wiring layer
// translates it into an mcp.Server.
type MCPServerConfig struct {
	Name        string            `json:"name"`
	Command     []string          `json:"command"`
	Cwd         string            `json:"cwd,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	TimeoutMs   int               `json:"timeout_ms,omitempty"`
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

	return os.WriteFile(path, payload, 0o600)
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
