package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	aetoxapp "github.com/Mike0165115321/Aetox/internal/app"
	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/prompt"
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

	// projectFocused=false runs the engine "ไม่โฟกัสโปรเจกต์": rooted at the
	// user's home dir so every tool (files/git/terminal) still works on the
	// machine, but nothing is treated as a project (no tree walk, no recent-
	// projects entry, UI shows an unfocused chip). This is the startup default —
	// the app must not silently adopt whatever cwd it was launched from.
	projectFocused bool

	turnMu     sync.Mutex
	turnCancel context.CancelFunc // cancels the chat turn in flight, nil when idle

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
	// Relay every call/result live to the chat's tool timeline.
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "agent:tool", map[string]string{"action": action, "detail": detail})
	}
	if action != "call" {
		return
	}
	a.toolHistory = append(a.toolHistory, detail)
	if len(a.toolHistory) > maxToolHistory {
		a.toolHistory = a.toolHistory[len(a.toolHistory)-maxToolHistory:]
	}
}

// emitAgentStatus relays the turn executor's phase messages ("กำลังคิดคำตอบ...",
// "กำลังรันเครื่องมือ...", then "" when done) to the frontend as a live typing/
// thinking indicator, so the chat doesn't look frozen during a turn.
func (a *App) emitAgentStatus(status string) {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "agent:status", status)
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
	// Unfocused mode: home is not a project — even if it happens to sit inside
	// a git repo, its status is noise for the Files Changed panel.
	if !a.projectFocused {
		return out
	}
	cmd := exec.Command("git", "-C", a.cfg.SandboxRoot, "status", "--porcelain")
	proc.HideConsole(cmd)
	raw, err := cmd.Output()
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
	// Unfocused mode is rooted at the user's home dir — eagerly walking that
	// (Documents, Downloads, ...) would be huge and meaningless as a "project
	// tree", so the tree is simply empty until a project is focused.
	if !a.projectFocused {
		return []TreeNode{}
	}
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

// RelativizePath converts an absolute OS path (e.g. from a native file drop)
// into a path relative to the open project's sandbox root, so it can be
// passed to ReadFile/WriteFile. Errors if the path is outside the project.
func (a *App) RelativizePath(absPath string) (string, error) {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return "", fmt.Errorf("no project open")
	}
	safeRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(safeRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("path is outside project root")
	}
	return filepath.ToSlash(rel), nil
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

// IdentityFile is one markdown file in the user's cross-project "AI
// Identity" directory (config.IdentityDir) — e.g. context.md, skills.md.
// Every file here rides along with the AI into every system prompt build,
// regardless of which project is open (internal/prompt's "Personal
// instructions" layer, ARCHITECTURE.md §11 row 3).
type IdentityFile struct {
	Name string `json:"name"`
}

// ensureIdentityDir returns config.IdentityDir(), creating it on first use
// and migrating the old single-file AETOX.md (pre-multi-file AI Identity)
// into identity/context.md if one exists.
func ensureIdentityDir() (string, error) {
	dir, err := config.IdentityDir()
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		if legacyPath, lerr := config.UserGlobalContextPath(); lerr == nil {
			if data, rerr := os.ReadFile(legacyPath); rerr == nil && len(data) > 0 {
				_ = os.WriteFile(filepath.Join(dir, "context.md"), data, 0o644)
				_ = os.Remove(legacyPath)
			}
		}
	}
	return dir, nil
}

// safeIdentityName rejects path traversal and appends .md if the caller left
// the extension off, so every identity file stays a plain, flat filename.
func safeIdentityName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid file name: %q", name)
	}
	if !strings.EqualFold(filepath.Ext(name), ".md") {
		name += ".md"
	}
	return name, nil
}

// ListIdentityFiles lists the markdown files in the AI Identity directory,
// sorted by name. Empty (not error) if none exist yet.
func (a *App) ListIdentityFiles() ([]IdentityFile, error) {
	dir, err := ensureIdentityDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := []IdentityFile{} // non-nil so the frontend gets [] not null
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		files = append(files, IdentityFile{Name: e.Name()})
	}
	return files, nil
}

// ReadIdentityFile reads one file from the AI Identity directory by name.
func (a *App) ReadIdentityFile(name string) (string, error) {
	dir, err := ensureIdentityDir()
	if err != nil {
		return "", err
	}
	safeName, err := safeIdentityName(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, safeName))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// SaveIdentityFile creates or overwrites one file in the AI Identity directory.
func (a *App) SaveIdentityFile(name, content string) error {
	dir, err := ensureIdentityDir()
	if err != nil {
		return err
	}
	safeName, err := safeIdentityName(name)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, safeName), []byte(content), 0o644)
}

// DeleteIdentityFile removes one file from the AI Identity directory.
func (a *App) DeleteIdentityFile(name string) error {
	dir, err := ensureIdentityDir()
	if err != nil {
		return err
	}
	safeName, err := safeIdentityName(name)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, safeName))
}

const attachmentsDir = ".aetox-attachments"

var attachmentSeq int64

// PickAttachmentImage prompts the user to pick an image file (native dialog)
// for chat attachment, returning its absolute OS path, or "" if cancelled.
func (a *App) PickAttachmentImage() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "แนบรูปภาพ",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Images (*.png, *.jpg, *.jpeg, *.gif, *.webp, *.bmp)", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp"},
		},
	})
}

// SaveChatImage copies an image (picked via PickAttachmentImage, or dropped —
// both give a real absolute OS path) into the project's sandbox root, so it
// becomes a normal relative path any sandboxed skill (image_ocr, read, ...)
// can already operate on, with no path-escaping special case.
func (a *App) SaveChatImage(sourcePath string) (string, error) {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return "", fmt.Errorf("no project open")
	}
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return "", fmt.Errorf("no source path given")
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory", sourcePath)
	}
	const maxBytes = 20 << 20 // 20MB — generous for a chat-attached photo/screenshot
	if info.Size() > maxBytes {
		return "", fmt.Errorf("image too large (%d bytes, max 20MB)", info.Size())
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}

	destDir := filepath.Join(root, attachmentsDir)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}
	seq := atomic.AddInt64(&attachmentSeq, 1)
	destName := fmt.Sprintf("%d-%d%s", time.Now().UnixMilli(), seq, filepath.Ext(sourcePath))
	destPath := filepath.Join(destDir, destName)
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return "", err
	}

	rel, err := filepath.Rel(root, destPath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// ReadImageDataURL reads a sandboxed image back as a data: URL, for inline
// preview in the chat UI (the frontend only has an OS path, not the bytes).
func (a *App) ReadImageDataURL(relPath string) (string, error) {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return "", fmt.Errorf("no project open")
	}
	full, err := safeSandboxPath(root, relPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	mimeType := mime.TypeByExtension(filepath.Ext(full))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// ProjectStatus is the real project/git state for the sandbox root the engine runs in.
type ProjectStatus struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Branch           string `json:"branch"`
	Focused          bool   `json:"focused"` // false = "ไม่โฟกัสโปรเจกต์" mode (engine rooted at home)
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
	// The desktop build never wired this up before, so every debuglog.Msg/Info/
	// Block call already sprinkled through the shared engine (turn executor
	// phases, provider HTTP round-trips, ...) was silently thrown away here —
	// unlike the CLI, which always enables it (cmd/aetox/main.go). Same
	// directory as model-preference.json etc. (internal/config.DataRoot).
	if dataRoot, err := config.DataRoot(); err == nil {
		debuglog.Init(dataRoot)
	}
	// Explicit checkpoint, not just debuglog.Msg's usual error-only calls —
	// most of those never fire on a clean run, so without this the log stays
	// empty and gives no evidence either way for "why did first paint feel
	// stuck." This makes the log itself the answer next time it happens.
	defer debuglog.Block("App.startup")()
	a.focusNone()
	a.startNewSession()
}

// focusNone re-roots the engine at the user's home dir and marks the app as
// not focused on any project. Falls back to cwd only if home can't be resolved.
func (a *App) focusNone() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	a.reload(config.ConfigOptions{RootPath: home, ApprovalMode: string(safety.ApprovalFullAccess)})
	a.projectFocused = false
}

// SendMessage runs one chat turn through the Aetox engine and returns the reply.
// The turn is appended to the current session and persisted.
func (a *App) SendMessage(text string) (string, error) {
	if a.chat == nil {
		return "", fmt.Errorf("aetox core not ready: %s", a.modelStatus)
	}
	// Custom slash commands (<DataRoot>/commands/<name>.md) expand into their
	// prompt body before the engine sees the text; unknown "/..." passes
	// through to the model unchanged, so nothing regresses.
	if expanded, ok := command.ExpandCustom(text); ok {
		text = expanded
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.turnMu.Lock()
	a.turnCancel = cancel
	a.turnMu.Unlock()
	defer func() {
		cancel()
		a.turnMu.Lock()
		a.turnCancel = nil
		a.turnMu.Unlock()
	}()
	reply, err := a.chat.RunOnceStream(ctx, text, func(chunk string) {
		wailsruntime.EventsEmit(a.ctx, "agent:chunk", chunk)
	}, func(chunk string) {
		wailsruntime.EventsEmit(a.ctx, "agent:reasoning", chunk)
	})
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

// CancelTurn aborts the chat turn in flight (the tool loop is unbounded, so
// this Stop button is the user's brake, same role as Ctrl+C in the CLI).
// No-op when idle.
func (a *App) CancelTurn() {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	if a.turnCancel != nil {
		a.turnCancel()
	}
}

// ModelStatus reports which provider/model the engine is running, as a display string.
func (a *App) ModelStatus() string {
	return a.modelStatus
}

// contextWindowTokens resolves the model's real context window: an explicit
// user override wins, then the curated per-model catalog, then the agent's
// own char budget as the honest floor (what the engine will actually keep).
func (a *App) contextWindowTokens() int {
	if a.cfg.ModelContextTokens > 0 {
		return a.cfg.ModelContextTokens
	}
	if tokens := model.ContextWindowTokens(a.cfg.ModelProvider, a.cfg.ModelName); tokens > 0 {
		return tokens
	}
	if a.agent != nil {
		_, _, maxChars := a.agent.ContextUsage()
		return (maxChars + 3) / 4
	}
	return 0
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
		ContextMax:   a.contextWindowTokens(),
	}
}

// ContextSlice is one labeled share of the context window. Key is stable for
// the frontend to translate: system | tools | messages | free.
type ContextSlice struct {
	Key    string `json:"key"`
	Tokens int    `json:"tokens"`
}

// ContextBreakdown backs the composer's context meter (Claude Code-style):
// how full the window is and what fills it.
type ContextBreakdown struct {
	UsedTokens int            `json:"usedTokens"`
	MaxTokens  int            `json:"maxTokens"`
	Slices     []ContextSlice `json:"slices"`
}

// GetContextBreakdown estimates token usage per category. Same chars/4
// heuristic as GetModelInfo — an estimate for orientation, not billing.
func (a *App) GetContextBreakdown() ContextBreakdown {
	est := func(chars int) int { return (chars + 3) / 4 }

	systemChars, msgChars := 0, 0
	if a.agent != nil {
		for i, m := range a.agent.ContextMessages() {
			chars := len(m.Content)
			for _, tc := range m.ToolCalls {
				chars += len(tc.Function.Arguments)
			}
			if i == 0 && m.Role == model.RoleSystem {
				systemChars = chars
			} else {
				msgChars += chars
			}
		}
	}

	toolChars := 0
	if a.registry != nil {
		if defs, err := json.Marshal(skill.NewDispatcher(a.registry).ToolDefinitions()); err == nil {
			toolChars = len(defs)
		}
	}

	maxTokens := a.contextWindowTokens()

	used := est(systemChars) + est(toolChars) + est(msgChars)
	free := maxTokens - used
	if free < 0 {
		free = 0
	}
	return ContextBreakdown{
		UsedTokens: used,
		MaxTokens:  maxTokens,
		Slices: []ContextSlice{
			{Key: "system", Tokens: est(systemChars)},
			{Key: "tools", Tokens: est(toolChars)},
			{Key: "messages", Tokens: est(msgChars)},
			{Key: "free", Tokens: free},
		},
	}
}

// currentProjectStatus stamps the focus flag onto the raw status; unfocused
// mode hides the home dir's name/branch so the UI never presents it as a project.
func (a *App) currentProjectStatus() ProjectStatus {
	ps := projectStatus(a.cfg.SandboxRoot)
	ps.Focused = a.projectFocused
	if !a.projectFocused {
		ps.Name = ""
		ps.Branch = ""
	}
	return ps
}

// GetProjectStatus reports the real project/git state for the current sandbox root.
func (a *App) GetProjectStatus() ProjectStatus {
	return a.currentProjectStatus()
}

// ClearProjectFocus switches to "no project" mode: tools keep full access to
// the machine (rooted at home), but the chat is no longer tied to any project.
// Starts a fresh session, same as switching projects does.
func (a *App) ClearProjectFocus() ProjectStatus {
	a.focusNone()
	a.startNewSession()
	return a.currentProjectStatus()
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
	a.projectFocused = true
	a.startNewSession()
	a.touchProject(a.cfg.SandboxRoot)
	return a.currentProjectStatus(), nil
}

// OpenProjectPath switches straight to a previously-opened project by path —
// used by the sidebar's recent-projects list, skipping the OS folder dialog.
func (a *App) OpenProjectPath(root string) (ProjectStatus, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return ProjectStatus{}, fmt.Errorf("empty project path")
	}
	a.reload(config.ConfigOptions{RootPath: root, ApprovalMode: string(safety.ApprovalFullAccess)})
	a.projectFocused = true
	a.startNewSession()
	a.touchProject(a.cfg.SandboxRoot)
	return a.currentProjectStatus(), nil
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
	if choices := model.ModelChoices(canonical); choices != nil {
		return choices
	}
	return []string{}
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
	// Never nil: a nil slice serializes to JSON null, which the frontend
	// (thinkLevels.length) crashes on mid-render.
	caps := model.ResolveThinkingCapabilities(a.cfg.ModelProvider, a.cfg.ModelName)
	if !caps.Native || caps.Levels == nil {
		return []string{}
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
		&browserClickSkill{app: a},
		&browserTypeSkill{app: a},
	}
	if a.mcp == nil {
		servers, err := config.LoadMCPServers()
		if err != nil {
			debuglog.Msg("mcp: load servers: %v", err)
		}
		a.mcp = mcp.NewManager(toMCPServers(servers))
	}
	chatApp, agent, status, registry := bootstrapFromConfig(cfg, a.recordToolAction, a.emitAgentStatus, workbenchTools, a.mcp)
	a.chat = chatApp
	a.agent = agent
	a.cfg = cfg
	a.modelStatus = status
	a.registry = registry
	if a.agent != nil {
		a.agent.SetUsageReporter(a.recordTokenUsage)
	}
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
		if v := strings.TrimSpace(pref.ApprovalMode); v != "" {
			cfg.ApprovalMode = v
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
			URL:         c.URL,
			Headers:     c.Headers,
			Timeout:     time.Duration(c.TimeoutMs) * time.Millisecond,
			Disabled:    c.Disabled,
		})
	}
	return out
}

func bootstrapFromConfig(cfg config.Config, onToolAction func(action, detail string), onStatus func(string), extraSkills []skill.Skill, mcpMgr *mcp.Manager) (*aetoxapp.App, *cognitive.Agent, string, *skill.Registry) {
	defer debuglog.Block("bootstrapFromConfig")()
	status := model.ResolveStatus(cfg.ModelProvider, cfg.ModelName, nil)

	providerDone := debuglog.Block("model.BootstrapProvider")
	bootstrapResult := model.BootstrapProvider(model.BootstrapOptions{
		Provider: cfg.ModelProvider,
		Model:    cfg.ModelName,
		APIKey:   cfg.ModelAPIKey,
		BaseURL:  cfg.ModelBaseURL,
		Timeout:  30 * time.Second,
	})
	providerDone()
	if bootstrapResult.Provider == nil {
		return nil, nil, status + " (init failed: " + bootstrapResult.Error.Error() + ")", nil
	}
	if bootstrapResult.Warning != "" {
		status += " (" + bootstrapResult.Warning + ")"
	}

	ctxTokens := cfg.ModelContextTokens
	if ctxTokens <= 0 {
		ctxTokens = model.ContextWindowTokens(cfg.ModelProvider, cfg.ModelName)
	}
	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     bootstrapResult.Provider,
		Model:        cfg.ModelName,
		SystemPrompt: prompt.Build(prompt.SurfaceDesktop, cfg.SandboxRoot),
		// Scale the retained-history budget to the model's real window
		// (0 → NewContext's 128k-char default). ponytail: trims oldest turns
		// when over budget — upgrade to summarizing compaction if losing old
		// turns verbatim starts to hurt long sessions.
		MaxChars: ctxTokens * 4,
	})

	registry := skill.NewDefaultRegistry(skill.RegistryOptions{
		SandboxRoot: cfg.SandboxRoot,
	})
	for _, s := range extraSkills {
		if err := registry.Register(s, skill.SourceExternal); err != nil {
			debuglog.Msg("skill registration skipped: %v", err)
		}
	}
	// Scans ~/.agents/skills and ~/.claude/skills — a real filesystem walk,
	// not a fixed-cost lookup; timed because a large/slow-disk skills
	// directory is a plausible, easy-to-overlook source of startup latency.
	discoverDone := debuglog.Block("skill.RegisterDiscovered")
	for _, discErr := range skill.RegisterDiscovered(registry, skill.DefaultDiscoveryPaths()) {
		debuglog.Msg("skill discovery: %v", discErr)
	}
	discoverDone()
	// Register MCP tools before the dispatcher snapshots the registry. Bounded,
	// not unlimited: this runs synchronously on every app startup/model switch,
	// and a server like `npx -y pkg@latest` can take up to its own 30s default
	// timeout to resolve on a cold cache — that used to block the whole UI
	// (desktop's GetModelInfo etc. wouldn't resolve until this returned). Capped
	// here regardless of how many servers are configured or how slow any one of
	// them is; a server that doesn't make it just contributes no tools this
	// session (already-existing per-server error handling below).
	mcpDone := debuglog.Block("mcpMgr.Register")
	mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 8*time.Second)
	mcpRules, mcpErrs := mcpMgr.Register(mcpCtx, registry)
	mcpCancel()
	mcpDone()
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
		Agent:          agent,
		Console:        aetoxapp.NewStdIO(),
		Dispatcher:     dispatcher,
		ApprovalMode:   safety.ApprovalFullAccess,
		Permissions:    permissions,
		OnToolAction:   onToolAction,
		StatusReporter: onStatus,
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

// projectStatus reports the governance file the prompt layer would actually
// load for this root (internal/prompt.ProjectContextFile), so the UI badge
// reflects reality instead of just stat-ing a hardcoded name.
func projectStatus(root string) ProjectStatus {
	root = strings.TrimSpace(root)
	name := ""
	if root != "" && root != "." {
		name = filepath.Base(root)
	}
	governancePath := prompt.ProjectContextFile(root)
	governanceFile := prompt.ProjectContextFileNames[0]
	if governancePath != "" {
		governanceFile = filepath.Base(governancePath)
	}
	return ProjectStatus{
		Name:             name,
		Path:             root,
		Branch:           readGitBranch(root),
		GovernanceFile:   governanceFile,
		GovernanceLoaded: governancePath != "",
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

