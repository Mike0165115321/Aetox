package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	aetoxapp "github.com/Mike0165115321/Aetox/internal/app"
	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx         context.Context
	chat        *aetoxapp.App
	agent       *cognitive.Agent
	cfg         config.Config
	modelStatus string
}

// ProjectStatus is the real project/git state for the sandbox root the engine runs in.
type ProjectStatus struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Branch           string `json:"branch"`
	GovernanceFile   string `json:"governanceFile"`
	GovernanceLoaded bool   `json:"governanceLoaded"`
}

// ModelInfo is the real model/context state behind the top bar.
type ModelInfo struct {
	Provider     string `json:"provider"`
	ModelName    string `json:"modelName"`
	ThinkLevel   string `json:"thinkLevel"`
	ApprovalMode string `json:"approvalMode"`
	ContextUsed  int    `json:"contextUsed"`
	ContextMax   int    `json:"contextMax"`
}

// desktopProviders is the curated subset of the full engine catalog
// (model.SupportedProviders()) exposed in the desktop UI's provider picker.
var desktopProviders = []string{"ollama", "deepseek", "gemini", "openai", "openrouter", "zai"}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.reload(config.ConfigOptions{ApprovalMode: string(safety.ApprovalFullAccess)})
}

// SendMessage runs one chat turn through the Aetox engine and returns the reply.
func (a *App) SendMessage(text string) (string, error) {
	if a.chat == nil {
		return "", fmt.Errorf("aetox core not ready: %s", a.modelStatus)
	}
	return a.chat.RunOnce(a.ctx, text)
}

// ModelStatus reports which provider/model the engine is running, as a display string.
func (a *App) ModelStatus() string {
	return a.modelStatus
}

// GetModelInfo reports the real model/context state for the UI top bar.
func (a *App) GetModelInfo() ModelInfo {
	used := 0
	if a.agent != nil {
		_, usedChars, _ := a.agent.ContextUsage()
		used = (usedChars + 3) / 4
	}
	return ModelInfo{
		Provider:     a.cfg.ModelProvider,
		ModelName:    a.cfg.ModelName,
		ThinkLevel:   a.cfg.ThinkLevel,
		ApprovalMode: a.cfg.ApprovalMode,
		ContextUsed:  used,
		ContextMax:   a.cfg.ModelContextTokens,
	}
}

// GetProjectStatus reports the real project/git state for the current sandbox root.
func (a *App) GetProjectStatus() ProjectStatus {
	return projectStatus(a.cfg.SandboxRoot)
}

// OpenProjectFolder lets the user pick a real folder via the native OS dialog, then
// re-bootstraps the engine to run inside it (same model/provider preference).
func (a *App) OpenProjectFolder() (ProjectStatus, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Open Aetox Project Folder",
	})
	if err != nil {
		return ProjectStatus{}, err
	}
	if strings.TrimSpace(dir) == "" {
		return projectStatus(a.cfg.SandboxRoot), nil
	}
	a.reload(config.ConfigOptions{RootPath: dir, ApprovalMode: string(safety.ApprovalFullAccess)})
	return projectStatus(a.cfg.SandboxRoot), nil
}

// SupportedProviders lists the model providers exposed in the desktop UI — a
// curated subset of the full engine catalog (model.SupportedProviders()),
// which stays untouched so the CLI keeps its full provider list.
func (a *App) SupportedProviders() []string {
	all := make(map[string]bool, len(desktopProviders))
	for _, p := range model.SupportedProviders() {
		all[p] = true
	}
	out := make([]string, 0, len(desktopProviders))
	for _, p := range desktopProviders {
		if all[p] {
			out = append(out, p)
		}
	}
	return out
}

// ListModelsForProvider mirrors the CLI's model-selection discovery chain:
// live API discovery first, falling back to the static recommended list.
// An empty result means "no known models" — the frontend should offer a
// free-text input for a custom model id.
func (a *App) ListModelsForProvider(providerName string) []string {
	canonical := model.NormalizeProvider(providerName)
	baseURL := model.DefaultBaseURL(canonical)
	apiKey := resolveAPIKeyForProvider(canonical)
	if choices, err := model.ModelChoicesWithEndpointAndAPIKey(canonical, baseURL, apiKey); err == nil && len(choices) > 0 {
		return choices
	}
	return model.ModelChoices(canonical)
}

// SwitchModel re-bootstraps the engine on a specific model name for the
// current provider.
func (a *App) SwitchModel(modelName string) (ModelInfo, error) {
	next := a.cfg
	next.ModelName = strings.TrimSpace(modelName)
	if next.ModelName == "" {
		next.ModelName = model.DefaultModel(next.ModelProvider)
	}
	next.ThinkLevel = model.NormalizeThinkingLevel(next.ModelProvider, next.ModelName, next.ThinkLevel)
	a.applyConfig(next)
	if a.chat == nil {
		return ModelInfo{}, fmt.Errorf("switch failed: %s", a.modelStatus)
	}
	return a.GetModelInfo(), nil
}

// HasAPIKey reports whether a key-requiring provider already has a resolvable
// key (cached preference or env var). Always true for providers that don't
// need one.
func (a *App) HasAPIKey(providerName string) bool {
	canonical := model.NormalizeProvider(providerName)
	if !model.RequiresAPIKey(canonical) {
		return true
	}
	return resolveAPIKeyForProvider(canonical) != ""
}

// RequiresAPIKey exposes model.RequiresAPIKey to the frontend.
func (a *App) RequiresAPIKey(providerName string) bool {
	return model.RequiresAPIKey(model.NormalizeProvider(providerName))
}

// SetAPIKey persists an API key for a provider and, if it's the active
// provider, immediately re-bootstraps the engine with it.
func (a *App) SetAPIKey(providerName, apiKey string) (ModelInfo, error) {
	canonical := model.NormalizeProvider(providerName)
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return ModelInfo{}, fmt.Errorf("API key cannot be empty")
	}

	pref, ok, _ := config.LoadModelPreference()
	if !ok {
		pref = config.ModelPreference{}
	}
	pref.SetAPIKeyForProvider(canonical, key)
	if err := config.SaveModelPreference(pref); err != nil {
		return ModelInfo{}, err
	}

	if strings.EqualFold(a.cfg.ModelProvider, canonical) {
		next := a.cfg
		next.ModelAPIKey = key
		a.applyConfig(next)
		if a.chat == nil {
			return ModelInfo{}, fmt.Errorf("switch failed: %s", a.modelStatus)
		}
	}
	return a.GetModelInfo(), nil
}

func resolveAPIKeyForProvider(canonicalProvider string) string {
	if pref, ok, _ := config.LoadModelPreference(); ok {
		if key := pref.APIKeyForProvider(canonicalProvider); key != "" {
			return key
		}
	}
	return model.ResolveModelAPIKey(canonicalProvider)
}

// SupportedThinkLevels lists the thinking levels confirmed real for the current
// provider/model. Providers Aetox has no curated capability data for only get a
// generic guessed fallback internally (caps.Native == false) — that guess is not
// shown here, since we can't promise the API actually honors those levels.
func (a *App) SupportedThinkLevels() []string {
	caps := model.ResolveThinkingCapabilities(a.cfg.ModelProvider, a.cfg.ModelName)
	if !caps.Native {
		return nil
	}
	return caps.Levels
}

// SwitchProvider re-bootstraps the engine on a different provider, using its default model.
func (a *App) SwitchProvider(provider string) (ModelInfo, error) {
	next := a.cfg
	next.ModelProvider = model.NormalizeProvider(provider)
	next.ModelName = model.DefaultModel(next.ModelProvider)
	next.ModelBaseURL = model.DefaultBaseURL(next.ModelProvider)
	next.ModelAPIKey = resolveAPIKeyForProvider(next.ModelProvider)
	next.ThinkLevel = model.NormalizeThinkingLevel(next.ModelProvider, next.ModelName, "")
	a.applyConfig(next)
	if a.chat == nil {
		return ModelInfo{}, fmt.Errorf("switch failed: %s", a.modelStatus)
	}
	return a.GetModelInfo(), nil
}

// SwitchThinkLevel changes the reasoning depth for the current provider/model.
func (a *App) SwitchThinkLevel(level string) (ModelInfo, error) {
	next := a.cfg
	next.ThinkLevel = model.NormalizeThinkingLevel(next.ModelProvider, next.ModelName, level)
	a.applyConfig(next)
	if a.chat == nil {
		return ModelInfo{}, fmt.Errorf("switch failed: %s", a.modelStatus)
	}
	return a.GetModelInfo(), nil
}

// SwitchApprovalMode changes the safety approval mode the engine runs with.
func (a *App) SwitchApprovalMode(mode string) (ModelInfo, error) {
	next := a.cfg
	next.ApprovalMode = string(safety.NormalizeApprovalMode(mode))
	a.applyConfig(next)
	if a.chat == nil {
		return ModelInfo{}, fmt.Errorf("switch failed: %s", a.modelStatus)
	}
	return a.GetModelInfo(), nil
}

func (a *App) reload(opts config.ConfigOptions) {
	a.applyConfig(resolveConfig(opts))
}

// applyConfig re-bootstraps the engine from an already-resolved config, then
// persists the model/approval choice so the CLI and desktop app share one preference.
func (a *App) applyConfig(cfg config.Config) {
	chatApp, agent, status := bootstrapFromConfig(cfg)
	a.chat = chatApp
	a.agent = agent
	a.cfg = cfg
	a.modelStatus = status
	persistModelPreference(cfg)
}

func resolveConfig(opts config.ConfigOptions) config.Config {
	cfg := config.Load(opts)

	if pref, ok, _ := config.LoadModelPreference(); ok {
		if v := strings.TrimSpace(pref.ModelProvider); v != "" {
			cfg.ModelProvider = v
		}
		if v := strings.TrimSpace(pref.ModelName); v != "" {
			cfg.ModelName = v
		}
		if v := strings.TrimSpace(pref.ModelBaseURL); v != "" {
			cfg.ModelBaseURL = v
		}
		if v := strings.TrimSpace(pref.ThinkLevel); v != "" {
			cfg.ThinkLevel = v
		}
		if key := pref.APIKeyForProvider(cfg.ModelProvider); key != "" {
			cfg.ModelAPIKey = key
		}
	}
	if cfg.ModelAPIKey == "" {
		cfg.ModelAPIKey = model.ResolveModelAPIKey(cfg.ModelProvider)
	}
	if cfg.ModelName == "" && !strings.EqualFold(cfg.ModelProvider, "noop") {
		cfg.ModelName = model.DefaultModel(cfg.ModelProvider)
	}
	cfg.ThinkLevel = model.NormalizeThinkingLevel(cfg.ModelProvider, cfg.ModelName, cfg.ThinkLevel)
	return cfg
}

func bootstrapFromConfig(cfg config.Config) (*aetoxapp.App, *cognitive.Agent, string) {
	status := model.ResolveStatus(cfg.ModelProvider, cfg.ModelName, nil)

	bootstrapResult := model.BootstrapProvider(model.BootstrapOptions{
		Provider: cfg.ModelProvider,
		Model:    cfg.ModelName,
		APIKey:   cfg.ModelAPIKey,
		BaseURL:  cfg.ModelBaseURL,
		Timeout:  30 * time.Second,
	})
	if bootstrapResult.Provider == nil {
		return nil, nil, status + " (init failed: " + bootstrapResult.Error.Error() + ")"
	}
	if bootstrapResult.Warning != "" {
		status += " (" + bootstrapResult.Warning + ")"
	}

	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     bootstrapResult.Provider,
		Model:        cfg.ModelName,
		SystemPrompt: buildSystemPrompt(cfg.SandboxRoot),
	})

	dispatcher := skill.NewDispatcher(skill.NewDefaultRegistry(skill.RegistryOptions{
		SandboxRoot: cfg.SandboxRoot,
	}))

	chatApp, err := aetoxapp.NewApp(aetoxapp.Options{
		Agent:        agent,
		Console:      aetoxapp.NewStdIO(),
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess,
	})
	if err != nil {
		return nil, nil, status + " (init failed: " + err.Error() + ")"
	}
	return chatApp, agent, status
}

// persistModelPreference saves the current model/approval choice to the same
// preference file the CLI reads, so both surfaces stay in sync.
func persistModelPreference(cfg config.Config) {
	provider := strings.TrimSpace(cfg.ModelProvider)
	if provider == "" {
		return
	}
	canonicalProvider := model.NormalizeProvider(provider)
	pref, ok, _ := config.LoadModelPreference()
	if !ok {
		pref = config.ModelPreference{}
	}
	if strings.TrimSpace(cfg.ModelAPIKey) != "" {
		pref.SetAPIKeyForProvider(canonicalProvider, cfg.ModelAPIKey)
	}
	pref.ModelProvider = canonicalProvider
	pref.ModelName = strings.TrimSpace(cfg.ModelName)
	baseURL := strings.TrimSpace(cfg.ModelBaseURL)
	if baseURL == model.DefaultBaseURL(canonicalProvider) {
		baseURL = ""
	}
	pref.ModelBaseURL = baseURL
	pref.ThinkLevel = model.NormalizeThinkingLevel(canonicalProvider, pref.ModelName, cfg.ThinkLevel)
	pref.ApprovalMode = string(safety.NormalizeApprovalMode(cfg.ApprovalMode))
	_ = config.SaveModelPreference(pref)
}

func buildSystemPrompt(root string) string {
	sandboxRoot := strings.TrimSpace(root)
	if sandboxRoot == "" {
		sandboxRoot = "(unknown)"
	}
	return "You are Aetox, a concise assistant in Thai and English " +
		"that helps users through a desktop chat UI.\n" +
		"Current working sandbox root is: " + sandboxRoot + ".\n" +
		"Do NOT proactively mention or leak this path to the user in general greetings or unrelated conversation " +
		"unless they explicitly ask about files, directories, paths, or workspace locations."
}

const governanceFileName = "Aetox.md"

func projectStatus(root string) ProjectStatus {
	root = strings.TrimSpace(root)
	name := ""
	if root != "" && root != "." {
		name = filepath.Base(root)
	}
	_, statErr := os.Stat(filepath.Join(root, governanceFileName))
	return ProjectStatus{
		Name:             name,
		Path:             root,
		Branch:           readGitBranch(root),
		GovernanceFile:   governanceFileName,
		GovernanceLoaded: statErr == nil,
	}
}

// readGitBranch reads .git/HEAD directly rather than shelling out to git, so a
// missing git executable on PATH can't break project status.
func readGitBranch(root string) string {
	data, err := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	const prefix = "ref: refs/heads/"
	if strings.HasPrefix(head, prefix) {
		return strings.TrimPrefix(head, prefix)
	}
	if len(head) > 7 {
		return head[:7] // detached HEAD: short commit hash
	}
	return head
}
