package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	aetoxapp "github.com/Mike0165115321/Aetox/internal/app"
	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/mcp"
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
	toolHistory []string

	terminalsMu sync.Mutex
	terminals   map[string]*TerminalSession
	browsers    *browserHost

	sessionID  string
	transcript []SessionMessage

	mcp      *mcp.Manager    // configured MCP servers; built once, survives re-bootstraps
	registry *skill.Registry // current skill/tool registry, for the Tools panel

	dbInit sync.Once
	db     *sql.DB
	dbErr  error
	dbDir  string // overrides the default <UserConfigDir>/aetox directory; empty means production default. Test seam only.
}

// ChangedFile is one working-tree change reported by `git status`.
type ChangedFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

const maxToolHistory = 50

// recordToolAction is the engine's live tool-call feed for this session,
// kept for the Inspector's Command History panel. Only "call" events are
// recorded — "result" events are noise for a command-log view.
func (a *App) recordToolAction(action, detail string) {
	if action != "call" {
		return
	}
	a.toolHistory = append(a.toolHistory, detail)
	if len(a.toolHistory) > maxToolHistory {
		a.toolHistory = a.toolHistory[len(a.toolHistory)-maxToolHistory:]
	}
}

// CommandHistory returns this session's real tool-call history, most recent first.
func (a *App) CommandHistory() []string {
	out := make([]string, len(a.toolHistory))
	for i, c := range a.toolHistory {
		out[len(out)-1-i] = c
	}
	return out
}

// GitChangedFiles reports the working-tree status for the sandbox root via
// `git status --porcelain`. Returns an empty slice if git isn't on PATH or
// the root isn't a repo — the panel just shows no changes.
func (a *App) GitChangedFiles() []ChangedFile {
	out := []ChangedFile{}
	raw, err := exec.Command("git", "-C", a.cfg.SandboxRoot, "status", "--porcelain").Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		status := "M"
		if strings.Contains(code, "?") || strings.Contains(code, "A") {
			status = "U"
		}
		out = append(out, ChangedFile{Path: strings.TrimSpace(line[3:]), Status: status})
	}
	return out
}

// TreeNode is one row of the sidebar's project file tree.
type TreeNode struct {
	Label  string `json:"label"`
	Path   string `json:"path"` // relative to the sandbox root, forward-slashed
	Kind   string `json:"kind"` // "dir" | "file"
	Depth  int    `json:"depth"`
	Status string `json:"status,omitempty"` // "M" | "U" | ""
	Icon   string `json:"icon,omitempty"`
}

// treeIgnore skips VCS/build/dependency noise a dev never wants in the sidebar.
var treeIgnore = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "build": true,
	".vs": true, ".idea": true, "bin": true, "obj": true,
}

// ProjectTree walks the sandbox root and returns a flat, depth-first file
// tree for the sidebar (dirs collapsed by default, matching Sidebar.svelte's
// toggle logic). Git status per file reuses GitChangedFiles so the M/U
// badges match the Inspector's Files Changed panel exactly.
//
// ponytail: walks the whole tree eagerly on every call rather than lazily
// per folder-expand — fine for a normal repo, revisit if it's ever slow on
// a huge one.
func (a *App) ProjectTree() []TreeNode {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return []TreeNode{}
	}

	statusByPath := make(map[string]string)
	for _, f := range a.GitChangedFiles() {
		statusByPath[filepath.ToSlash(f.Path)] = f.Status
	}

	out := []TreeNode{}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})
		for _, entry := range entries {
			name := entry.Name()
			if treeIgnore[name] || strings.HasPrefix(name, ".") {
				continue
			}
			full := filepath.Join(dir, name)
			rel, _ := filepath.Rel(root, full)
			relSlash := filepath.ToSlash(rel)
			if entry.IsDir() {
				out = append(out, TreeNode{Label: name, Path: relSlash, Kind: "dir", Depth: depth, Icon: "📁"})
				walk(full, depth+1)
				continue
			}
			out = append(out, TreeNode{
				Label: name, Path: relSlash, Kind: "file", Depth: depth, Icon: "📄",
				Status: statusByPath[relSlash],
			})
		}
	}
	walk(root, 0)
	return out
}

// safeSandboxPath resolves relPath against root and rejects anything that
// would escape it (e.g. "../../etc/passwd"), so the file viewer can't be
// used to read outside the open project.
func safeSandboxPath(root, relPath string) (string, error) {
	safeRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(safeRoot, relPath)
	safeTarget, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", err
	}
	if safeTarget != safeRoot && !strings.HasPrefix(safeTarget+string(filepath.Separator), safeRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside project root")
	}
	return safeTarget, nil
}

// ReadFile returns the text content of a file inside the sandbox root, for
// the sidebar's file viewer.
func (a *App) ReadFile(relPath string) (string, error) {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return "", fmt.Errorf("no project open")
	}
	full, err := safeSandboxPath(root, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory", relPath)
	}

	const maxBytes = 1 << 20 // 1MB — plenty for a source file, keeps huge files out of the UI
	if info.Size() > maxBytes {
		return "", fmt.Errorf("file too large to preview (%d bytes)", info.Size())
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if bytes.Contains(data, []byte{0}) {
		return "", fmt.Errorf("binary file cannot be previewed")
	}
	return string(data), nil
}

// WriteFile saves text content to a file inside the sandbox root, for the
// dock's file editor. Same path-escape guard as ReadFile.
func (a *App) WriteFile(relPath, content string) error {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return fmt.Errorf("no project open")
	}
	full, err := safeSandboxPath(root, relPath)
	if err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
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
var desktopProviders = []string{"ollama", "deepseek", "gemini", "openai", "openrouter", "zai", "anthropic", "noop"}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.reload(config.ConfigOptions{ApprovalMode: string(safety.ApprovalFullAccess)})
	a.startNewSession()
}

// SendMessage runs one chat turn through the Aetox engine and returns the reply.
// The turn is appended to the current session and persisted.
func (a *App) SendMessage(text string) (string, error) {
	if a.chat == nil {
		return "", fmt.Errorf("aetox core not ready: %s", a.modelStatus)
	}
	reply, err := a.chat.RunOnce(a.ctx, text)
	if err != nil {
		return reply, err
	}
	now := time.Now().Format("15:04")
	userMsg := SessionMessage{Role: "user", Text: text, Time: now}
	agentMsg := SessionMessage{Role: "agent", Text: reply, Time: now}
	a.transcript = append(a.transcript, userMsg, agentMsg)
	a.appendTurn(userMsg, agentMsg)
	return reply, nil
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
	// Sessions are per project — turns are already persisted incrementally, so
	// just re-point the engine and start a fresh session for the new project.
	a.reload(config.ConfigOptions{RootPath: dir, ApprovalMode: string(safety.ApprovalFullAccess)})
	a.startNewSession()
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

// ProviderBaseURL reports the default API endpoint for a provider, for
// read-only display in the settings UI.
func (a *App) ProviderBaseURL(providerName string) string {
	return model.DefaultBaseURL(model.NormalizeProvider(providerName))
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
	workbenchTools := []skill.Skill{
		&browserOpenSkill{app: a},
		&browserReadSkill{app: a},
	}
	if a.mcp == nil {
		servers, err := config.LoadMCPServers()
		if err != nil {
			debuglog.Msg("mcp: load servers: %v", err)
		}
		a.mcp = mcp.NewManager(toMCPServers(servers))
	}
	chatApp, agent, status, registry := bootstrapFromConfig(cfg, a.recordToolAction, workbenchTools, a.mcp)
	a.chat = chatApp
	a.agent = agent
	a.cfg = cfg
	a.modelStatus = status
	a.registry = registry
	// A re-bootstrap (model/provider switch) creates a fresh agent — replay the
	// current session so the conversation's memory survives the switch.
	if a.agent != nil && len(a.transcript) > 0 {
		a.agent.RestoreHistory(transcriptToModelMessages(a.transcript))
	}
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

// toMCPServers translates the persisted config DTOs into mcp.Server values.
func toMCPServers(cfgs []config.MCPServerConfig) []mcp.Server {
	out := make([]mcp.Server, 0, len(cfgs))
	for _, c := range cfgs {
		out = append(out, mcp.Server{
			Name:        c.Name,
			Command:     c.Command,
			Cwd:         c.Cwd,
			Environment: c.Environment,
			Timeout:     time.Duration(c.TimeoutMs) * time.Millisecond,
		})
	}
	return out
}

func bootstrapFromConfig(cfg config.Config, onToolAction func(action, detail string), extraSkills []skill.Skill, mcpMgr *mcp.Manager) (*aetoxapp.App, *cognitive.Agent, string, *skill.Registry) {
	status := model.ResolveStatus(cfg.ModelProvider, cfg.ModelName, nil)

	bootstrapResult := model.BootstrapProvider(model.BootstrapOptions{
		Provider: cfg.ModelProvider,
		Model:    cfg.ModelName,
		APIKey:   cfg.ModelAPIKey,
		BaseURL:  cfg.ModelBaseURL,
		Timeout:  30 * time.Second,
	})
	if bootstrapResult.Provider == nil {
		return nil, nil, status + " (init failed: " + bootstrapResult.Error.Error() + ")", nil
	}
	if bootstrapResult.Warning != "" {
		status += " (" + bootstrapResult.Warning + ")"
	}

	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     bootstrapResult.Provider,
		Model:        cfg.ModelName,
		SystemPrompt: buildSystemPrompt(cfg.SandboxRoot),
	})

	registry := skill.NewDefaultRegistry(skill.RegistryOptions{
		SandboxRoot: cfg.SandboxRoot,
	})
	for _, s := range extraSkills {
		if err := registry.Register(s, skill.SourceExternal); err != nil {
			debuglog.Msg("skill registration skipped: %v", err)
		}
	}
	for _, discErr := range skill.RegisterDiscovered(registry, skill.DefaultDiscoveryPaths()) {
		debuglog.Msg("skill discovery: %v", discErr)
	}
	// Register MCP tools before the dispatcher snapshots the registry.
	mcpRules, mcpErrs := mcpMgr.Register(context.Background(), registry)
	for _, mcpErr := range mcpErrs {
		debuglog.Msg("mcp: %v", mcpErr)
	}
	dispatcher := skill.NewDispatcher(registry)

	permissions, permErr := config.LoadPermissions()
	if permErr != nil {
		debuglog.Msg("permissions load failed: %v", permErr)
	}
	// Prepend the default MCP "ask" rules so a user's explicit rule (later in
	// the list) still wins under last-match-wins.
	permissions.Rules = append(mcpRules, permissions.Rules...)

	chatApp, err := aetoxapp.NewApp(aetoxapp.Options{
		Agent:        agent,
		Console:      aetoxapp.NewStdIO(),
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess,
		Permissions:  permissions,
		OnToolAction: onToolAction,
	})
	if err != nil {
		return nil, nil, status + " (init failed: " + err.Error() + ")", nil
	}
	return chatApp, agent, status, registry
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

