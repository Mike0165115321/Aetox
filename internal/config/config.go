package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"aetox-cli/internal/model"
)

type Config struct {
	SandboxRoot        string
	AutoApprove        bool
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
		thinkLevel = "medium"
	}

	return Config{
		SandboxRoot:        root,
		AutoApprove:        opt.AutoApprove,
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

func PreferencePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		configDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "aetox")
		if configDir == "" {
			configDir = filepath.Join(os.TempDir(), "aetox")
		}
	}
	return filepath.Join(configDir, "aetox-cli", "model-preference.json"), nil
}

func LoadModelPreference() (ModelPreference, bool, error) {
	var pref ModelPreference
	path, err := PreferencePath()
	if err != nil {
		return pref, false, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return pref, false, nil
		}
		return pref, false, err
	}

	if err := json.Unmarshal(raw, &pref); err != nil {
		return pref, false, err
	}
	return pref, true, nil
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
