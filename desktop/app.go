package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/Mike0165115321/Aetox/internal/subagent"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"

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

	askMu sync.Mutex
	askCh chan string // the in-flight ask_user question's answer channel, nil when idle

	mcp      *mcp.Manager    // configured MCP servers; built once, survives re-bootstraps
	registry *skill.Registry // current skill/tool registry, for the Tools panel

	// toolHistoryMu guards toolHistory. Until sub-agents existed every tool event
	// arrived on the one turn goroutine; a delegate runs in its own (§44.11), so
	// two writers are now normal rather than impossible.
	toolHistoryMu sync.Mutex

	dbInit sync.Once
	db     *sql.DB
	dbErr  error
	dbDir  string // overrides the default <UserConfigDir>/aetox directory; empty means production default. Test seam only.

	// emit stands in for wailsruntime.EventsEmit. The indirection exists
	// because EventsEmit calls log.Fatalf — a hard os.Exit, not an error a
	// test can recover from — whenever ctx is not Wails-bound, which it never
	// is in a unit test. That is why the terminal read loop had no test at
	// all; see emitEvent in terminal.go. nil means the real thing. Test seam
	// only.
	emit func(event string, data ...any)
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
//
// The event goes to the UI as a struct rather than a formatted string: the
// frontend used to decide success by matching the Thai word "สำเร็จ" at the end
// of a detail line, so localizing that word (or appending anything to it) would
// have silently marked every tool call as failed.
func (a *App) recordToolAction(ev turn.ToolEvent) {
	// Relay every call/result live to the chat's tool timeline.
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "agent:tool", ev)
	}
	if ev.Action != "call" {
		return
	}
	// A sub-agent's calls are stamped with the `task` call that caused them
	// (§44.5). They belong to the chat timeline, not to this session's command
	// log, which is a list of what the agent itself did.
	if ev.Parent != "" {
		return
	}
	a.toolHistoryMu.Lock()
	defer a.toolHistoryMu.Unlock()
	a.toolHistory = append(a.toolHistory, ev.Label())
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
	a.toolHistoryMu.Lock()
	defer a.toolHistoryMu.Unlock()
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
// It is skill.IgnoredDirs — the same set grep refuses to search — so the tree
// and the search never disagree about what counts as the user's code.
var treeIgnore = skill.IgnoredDirs

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
	return a.saveChatAttachment(sourcePath, 20<<20) // generous for a photo/screenshot
}

// SaveChatFile is the same for anything else the user attaches — a clip to
// transcribe, a PDF to read. The cap is high because the point of attaching a
// video is that it is a video; the copy streams rather than loading it whole.
func (a *App) SaveChatFile(sourcePath string) (string, error) {
	return a.saveChatAttachment(sourcePath, 2<<30) // 2GB
}

func (a *App) saveChatAttachment(sourcePath string, maxBytes int64) (string, error) {
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
	if info.Size() > maxBytes {
		return "", fmt.Errorf("ไฟล์ใหญ่เกินไป (%d MB, สูงสุด %d MB)", info.Size()>>20, maxBytes>>20)
	}

	destDir := filepath.Join(root, attachmentsDir)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}
	seq := atomic.AddInt64(&attachmentSeq, 1)
	destName := fmt.Sprintf("%d-%d%s", time.Now().UnixMilli(), seq, filepath.Ext(sourcePath))
	destPath := filepath.Join(destDir, destName)

	// Streamed, not ReadFile: a 1GB clip must not have to fit in memory first.
	src, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(destPath) // a half-copied attachment is worse than none
		return "", err
	}
	if err := dst.Close(); err != nil {
		os.Remove(destPath)
		return "", err
	}

	rel, err := filepath.Rel(root, destPath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// PickAttachment prompts for any file to attach — image, clip, document. The
// image-only picker stays for the paths that specifically want one.
func (a *App) PickAttachment() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "แนบไฟล์",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "ไฟล์ที่แนบได้ทั้งหมด", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp;*.mp4;*.mov;*.mkv;*.webm;*.avi;*.mp3;*.wav;*.m4a;*.flac;*.ogg;*.pdf;*.txt;*.md;*.csv;*.json"},
			{DisplayName: "รูปภาพ", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp"},
			{DisplayName: "วิดีโอ / เสียง", Pattern: "*.mp4;*.mov;*.mkv;*.webm;*.avi;*.mp3;*.wav;*.m4a;*.flac;*.ogg"},
			{DisplayName: "ทุกไฟล์", Pattern: "*.*"},
		},
	})
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
	// WireFormat is the active runtime format for providers with more than
	// one (e.g. DeepSeek's "anthropic" vs "openai-compatible"). Empty when
	// the provider has only one format or uses the catalog default.
	WireFormat string `json:"wireFormat"`
}

// desktopProviders is the curated subset of the full engine catalog
// (model.SupportedProviders()) exposed in the desktop UI's provider picker.
var desktopProviders = []string{"ollama", "lmstudio", "deepseek", "gemini", "openai", "openrouter", "zai", "anthropic", "aetox"}

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

// outputSubdir is where a brand new file goes, relative to the sandbox root.
//
// Focused on a project, that is the project itself — the whole point of
// focusing is that the AI works inside it. Unfocused, every chat gets its own
// folder, so "write index.html" cannot be overwritten by the next chat that
// writes index.html; the session id is already a timestamp, so the folders
// sort by when the work happened.
//
// This is relative to unfocusedRoot, which is itself <home>/aetox — so the
// absolute destination is <home>/aetox/output/<session>. Changing either half
// alone moves every artifact or doubles the folder name; they are checked
// together in app_test.go.
//
// Read as a func at call time, not baked in at bootstrap: it changes every
// time the user starts or opens a chat, and re-bootstrapping the engine to
// change one folder name would be an absurd price.
func (a *App) outputSubdir() string {
	if a.projectFocused || a.sessionID == "" {
		return ""
	}
	return "output/" + a.sessionID
}

// unfocusedRoot is the sandbox root with no project open: <home>/aetox, not
// the home directory itself.
//
// Home was the original default, and it made the sandbox everything the user
// owns — .ssh, .aws, AppData with its browser token stores, Documents — all of
// it readable by read/grep/glob on every turn and writable without a prompt,
// since unfocused runs full-access. What made that indefensible rather than
// merely broad is that web_fetch, web_search and browser_read now sit in the
// same tool loop: a page can carry an instruction in, and the same loop can
// carry an answer back out. Reaching anything outside this folder is now a
// deliberate act — open it as a project, or attach the file.
//
// It is the parent of the folder writes already landed in (see outputSubdir),
// so nothing on disk moved when this changed.
//
// Empty when home cannot be resolved, which config.Load turns into cwd — the
// same fallback as before.
func unfocusedRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "aetox")
}

// focusNone re-roots the engine at unfocusedRoot and marks the app as not
// focused on any project.
func (a *App) focusNone() {
	root := unfocusedRoot()
	if root != "" {
		// A root that does not exist yet makes `list .` fail on a fresh
		// install — before the first write has created it.
		_ = os.MkdirAll(root, 0o755)
	}
	a.reload(config.ConfigOptions{RootPath: root, ApprovalMode: string(safety.ApprovalFullAccess)})
	a.projectFocused = false
}

// SendMessage runs one chat turn through the Aetox engine and returns the reply.
// The turn is appended to the current session and persisted.
func (a *App) SendMessage(text string) (string, error) {
	if a.chat == nil {
		return "", fmt.Errorf("aetox core not ready: %s", a.modelStatus)
	}
	// Prompt presets ("/name args") expand into their prompt body before the
	// engine sees the text — bundled ones and the user's alike; unknown "/..."
	// passes through to the model unchanged, so nothing regresses.
	if expanded, ok := command.ExpandPreset(text); ok {
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
	// Accumulate reasoning at the source so it persists with the turn — the
	// live panel alone would vanish once the turn completes. First/last chunk
	// times give the "thought for Xs" label.
	var reasoning strings.Builder
	var firstThink, lastThink time.Time
	reply, err := a.chat.RunOnceStream(ctx, text, func(chunk string) {
		wailsruntime.EventsEmit(a.ctx, "agent:chunk", chunk)
	}, func(chunk string) {
		if firstThink.IsZero() {
			firstThink = time.Now()
		}
		lastThink = time.Now()
		reasoning.WriteString(chunk)
		wailsruntime.EventsEmit(a.ctx, "agent:reasoning", chunk)
	})
	// A message can land in the moment between the loop's last drain and the reply
	// arriving here. Hand it back to the UI instead of swallowing it — this is the
	// one case the composer's old queue still exists for.
	if missed := a.agent.DrainInterjections(); len(missed) > 0 {
		wailsruntime.EventsEmit(a.ctx, "agent:interjection-missed", missed)
	}
	if err != nil {
		return reply, err
	}
	thinkSecs := 0
	if !firstThink.IsZero() {
		// round up so even a sub-second think shows as 1s, matching the label
		thinkSecs = int(lastThink.Sub(firstThink).Round(time.Second) / time.Second)
		if thinkSecs < 1 {
			thinkSecs = 1
		}
	}
	now := time.Now().Format("15:04")
	userMsg := SessionMessage{Role: "user", Text: text, Time: now}
	agentMsg := SessionMessage{Role: "agent", Text: reply, Time: now, Reasoning: strings.TrimSpace(reasoning.String()), ThinkSecs: thinkSecs}
	a.transcript = append(a.transcript, userMsg, agentMsg)
	a.appendTurn(userMsg, agentMsg)
	return reply, nil
}

// Interject hands the turn already in flight something the user just typed,
// instead of parking it until the turn ends. The engine picks it up on its next
// tool-loop round, or keeps the turn going if it was already writing the answer
// (internal/cognitive.Agent.Interject).
//
// It returns nothing to wait on: the text folds into the turn that is running, so
// its answer arrives as part of that turn's reply. Preset expansion happens here
// too, or "/name" would work only when the engine was idle.
func (a *App) Interject(text string) error {
	if a.agent == nil {
		return fmt.Errorf("aetox core not ready: %s", a.modelStatus)
	}
	if expanded, ok := command.ExpandPreset(text); ok {
		text = expanded
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	a.agent.Interject(text)
	return nil
}

// CancelTurn aborts the chat turn in flight (the tool loop is unbounded, so
// this Stop button is the user's brake, same role as Ctrl+C in the CLI).
// No-op when idle.
func (a *App) CancelTurn() {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	// Stop has to mean stop, including whatever was typed under the turn being
	// stopped. Dropped here rather than left in the buffer: the loop checks ctx
	// before it drains, so a cancelled turn returns with the message still
	// pending, SendMessage would hand it back as a straggler, and the composer
	// would send the thing the user just cancelled as a fresh turn.
	if a.agent != nil {
		a.agent.DrainInterjections()
	}
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
		WireFormat:   effectiveWireFormat(a.cfg.ModelProvider, a.cfg.ModelWireFormat),
	}
}

// effectiveWireFormat resolves the format actually in effect: the explicit
// preference if set, otherwise the provider's catalog-default runtime — so
// the UI can highlight the right toggle option even when nothing was ever
// saved (a fresh install, or a provider with only one format).
func effectiveWireFormat(providerName, explicit string) string {
	if v := strings.TrimSpace(explicit); v != "" {
		return v
	}
	info, ok := model.LookupProviderInfo(model.NormalizeProvider(providerName))
	if !ok {
		return ""
	}
	return info.Runtime
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

// ClearProjectFocus switches to "no project" mode: tools keep working, rooted
// at unfocusedRoot, but the chat is no longer tied to any project.
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

// EnabledProviders is the subset of SupportedProviders the Settings sidebar
// and the chat composer's picker should actually show — everything else stays
// reachable via the "+" add flow in Settings. See config.ResolvedEnabledProviders
// for the default-to-active-provider rule an untouched install falls back to.
func (a *App) EnabledProviders() []string {
	pref, _, _ := config.LoadModelPreference()
	return config.ResolvedEnabledProviders(pref.EnabledProviders, a.cfg.ModelProvider)
}

// SetProviderEnabled adds or removes providerName from the enabled set and
// returns the refreshed list. Removing the last remaining entry is refused
// (there must always be at least one provider visible); adding is a no-op if
// providerName is already enabled.
func (a *App) SetProviderEnabled(providerName string, enabled bool) ([]string, error) {
	if strings.TrimSpace(providerName) == "" {
		return nil, fmt.Errorf("provider name is required")
	}
	canonical := model.NormalizeProvider(providerName)
	if _, ok := model.LookupProviderInfo(canonical); !ok {
		return nil, fmt.Errorf("unknown provider: %q", providerName)
	}
	pref, ok, _ := config.LoadModelPreference()
	if !ok {
		pref = config.ModelPreference{}
	}
	// Materialize the resolved (possibly default) set before mutating, so
	// toggling one provider never silently drops the implicit active one.
	current := config.ResolvedEnabledProviders(pref.EnabledProviders, a.cfg.ModelProvider)

	next := make([]string, 0, len(current)+1)
	found := false
	for _, p := range current {
		if p == canonical {
			found = true
			if !enabled {
				continue // drop it
			}
		}
		next = append(next, p)
	}
	if enabled && !found {
		next = append(next, canonical)
	}
	if !enabled && len(next) == 0 {
		return current, fmt.Errorf("cannot disable %s: at least one provider must stay enabled", canonical)
	}

	pref.EnabledProviders = next
	if err := config.SaveModelPreference(pref); err != nil {
		return current, err
	}
	return next, nil
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

// TestProviderConnection proves a provider is actually reachable by running a
// minimal 1-token completion through the same client chat uses — endpoint,
// key, and wire format all verified in one shot. modelName picks which model
// to ping, so a model can be proven before switching to it; empty falls back
// to the active model for this provider, else the catalog default. Returns the
// latency label on success; the error carries the provider's real failure
// message.
func (a *App) TestProviderConnection(providerName, modelName string) (string, error) {
	canonical := model.NormalizeProvider(providerName)
	baseURL := model.DefaultBaseURL(canonical)
	apiKey := resolveAPIKeyForProvider(canonical)
	wireFormat := ""
	fallback := ""
	if canonical == model.NormalizeProvider(a.cfg.ModelProvider) {
		fallback = strings.TrimSpace(a.cfg.ModelName)
		wireFormat = a.cfg.ModelWireFormat
	}
	if fallback == "" {
		fallback = model.ResolveDefaultModel(canonical, baseURL, apiKey)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = fallback
	}
	p, err := model.NewProvider(model.ProviderOptions{
		Provider:   canonical,
		Model:      modelName,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Timeout:    15 * time.Second,
		WireFormat: wireFormat,
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := time.Now()
	_, err = p.Complete(ctx, model.Request{
		Model:     modelName,
		Messages:  []model.Message{{Role: model.RoleUser, Content: "ping"}},
		MaxTokens: 1,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s · %dms", modelName, time.Since(start).Milliseconds()), nil
}

// SwitchModel re-bootstraps the engine on a specific model name for the
// current provider.
func (a *App) SwitchModel(modelName string) (ModelInfo, error) {
	next := a.cfg
	next.ModelName = strings.TrimSpace(modelName)
	if next.ModelName == "" {
		next.ModelName = model.ResolveDefaultModel(next.ModelProvider, next.ModelBaseURL, next.ModelAPIKey)
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
	next.ModelBaseURL = model.DefaultBaseURL(next.ModelProvider)
	next.ModelWireFormat = "" // reset to the new provider's default format
	next.ModelAPIKey = resolveAPIKeyForProvider(next.ModelProvider)
	next.ModelName = model.ResolveDefaultModel(next.ModelProvider, next.ModelBaseURL, next.ModelAPIKey)
	next.ThinkLevel = model.NormalizeThinkingLevel(next.ModelProvider, next.ModelName, "")
	a.applyConfig(next)
	if a.chat == nil {
		return ModelInfo{}, fmt.Errorf("switch failed: %s", a.modelStatus)
	}
	return a.GetModelInfo(), nil
}

// ProviderWireFormats lists the wire formats providerName can speak — e.g.
// DeepSeek offers both an Anthropic-format endpoint and a plain
// OpenAI-compatible one for the same account. Empty when the provider has
// only one format (nothing to toggle). The first element is always the
// catalog default.
func (a *App) ProviderWireFormats(providerName string) []string {
	info, ok := model.LookupProviderInfo(model.NormalizeProvider(providerName))
	if !ok || info.AltRuntime == "" {
		return []string{}
	}
	return []string{info.Runtime, info.AltRuntime}
}

// SetProviderWireFormat switches the currently active provider between its
// available wire formats (see ProviderWireFormats) without changing the
// selected model. A no-op format (provider has no alt, or format is already
// current) still re-bootstraps — cheap, and keeps behavior predictable.
func (a *App) SetProviderWireFormat(format string) (ModelInfo, error) {
	next := a.cfg
	format = strings.TrimSpace(format)
	if info, ok := model.LookupProviderInfo(model.NormalizeProvider(next.ModelProvider)); ok && format == info.Runtime {
		format = "" // matches the catalog default — store nothing
	}
	next.ModelWireFormat = format
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

// reload re-points the engine at a different project root. Only the root
// changes — the model this window is running on stays put.
//
// It used to re-run resolveConfig on every switch, which re-reads
// model-preference.json: a single global file (config.PreferencePath is under
// DataRoot, not per-project) that the CLI and every other open Aetox window
// also write. So opening a project silently adopted whoever wrote it last, and
// applyConfig then persisted that value back — one window's test model spread
// to the rest and stuck. The log signature was a bootstrap that flipped model
// *and* approval mode in one line, which no Switch* call can produce (they all
// copy a.cfg).
//
// The first bootstrap has no running model to keep, so startup still resolves
// from disk — that is how the user's saved model gets loaded at launch.
func (a *App) reload(opts config.ConfigOptions) {
	if a.cfg.ModelProvider == "" {
		a.applyConfig(resolveConfig(opts))
		return
	}
	next := a.cfg
	next.SandboxRoot = config.Load(opts).SandboxRoot
	a.applyConfig(next)
}

// applyConfig re-bootstraps the engine from an already-resolved config, then
// persists the model/approval choice so the CLI and desktop app share one preference.
func (a *App) applyConfig(cfg config.Config) {
	workbenchTools := []skill.Skill{
		&browserOpenSkill{app: a},
		&browserReadSkill{app: a},
		&browserClickSkill{app: a},
		&browserTypeSkill{app: a},
		&askUserSkill{app: a},
		&todoWriteSkill{app: a},
	}
	if a.mcp == nil {
		servers, err := config.LoadMCPServers()
		if err != nil {
			debuglog.Msg("mcp: load servers: %v", err)
		}
		a.mcp = mcp.NewManager(toMCPServers(servers))
	}
	// Capture the outgoing agent's real context before it's replaced: it holds
	// what the text transcript doesn't — tool calls, tool results, compaction
	// summaries — so a model switch keeps the model's working memory intact
	// (OpenCode/Claude Code keep tool history across switches too).
	var priorContext []model.Message
	if a.agent != nil {
		priorContext = a.agent.ContextMessages()
	}
	chatApp, agent, status, registry := bootstrapFromConfig(cfg, a.recordToolAction, a.emitAgentStatus, a.recordTokenUsage, workbenchTools, a.mcp, a.outputSubdir)
	a.chat = chatApp
	a.agent = agent
	a.cfg = cfg
	a.modelStatus = status
	a.registry = registry
	if a.agent != nil {
		a.agent.SetUsageReporter(a.recordTokenUsage)
		// Draw the row while the model is still writing the call, not after,
		// and tick its line count up as the content arrives. The executor emits
		// the same Ref when the call actually runs, so the UI reuses the row
		// rather than drawing the call twice — including when the early updates
		// carried no subject yet and the label filled itself in later.
		a.agent.SetToolCallProgressReporter(func(id, name, subject string, lines int) {
			a.recordToolAction(turn.ToolEvent{Action: "call", Ref: id, Name: name, Subject: subject, Added: lines})
		})
	}
	// A re-bootstrap (model/provider switch) creates a fresh agent — replay the
	// old agent's context (minus its system prompt; the new agent builds its
	// own). Falls back to the persisted text transcript when there is no live
	// agent to inherit from (e.g. first bootstrap after loading a session).
	if a.agent != nil {
		if len(priorContext) > 1 {
			a.agent.RestoreHistory(priorContext[1:])
		} else if len(a.transcript) > 0 {
			a.agent.RestoreHistory(transcriptToModelMessages(a.transcript))
		}
	}
	persistModelPreference(cfg)

	// Connect MCP servers and register their tools OFF the startup path: a cold
	// `npx -y pkg@latest` resolve took ~5s and used to block first paint. The
	// permission gate is already installed synchronously above (from server
	// names), and the dispatcher reads the registry live, so tools just appear
	// mid-session when their server finishes connecting. Captures this specific
	// registry — a later model switch swaps in a new one and starts its own
	// registration; tools landing in a superseded registry are simply unused.
	if a.mcp != nil && registry != nil {
		mgr, reg := a.mcp, registry
		go func() {
			defer debuglog.Block("mcpMgr.Register (background)")()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, errs := mgr.Register(ctx, reg)
			for _, err := range errs {
				debuglog.Msg("mcp: %v", err)
			}
			if a.ctx != nil {
				wailsruntime.EventsEmit(a.ctx, "skills:updated", nil)
			}
		}()
	}
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
		if v := strings.TrimSpace(pref.ModelWireFormat); v != "" {
			cfg.ModelWireFormat = v
		}
		if v := strings.TrimSpace(pref.ThinkLevel); v != "" {
			cfg.ThinkLevel = v
		}
		if v := strings.TrimSpace(pref.ApprovalMode); v != "" {
			cfg.ApprovalMode = v
		}
		if v := strings.TrimSpace(pref.UILocale); v != "" {
			cfg.UILocale = v
		}
		if key := pref.APIKeyForProvider(cfg.ModelProvider); key != "" {
			cfg.ModelAPIKey = key
		}
	}
	if cfg.ModelAPIKey == "" {
		cfg.ModelAPIKey = model.ResolveModelAPIKey(cfg.ModelProvider)
	}
	// Every provider gets its catalog default, aetox included. It used to be
	// excluded, from when its models were only test fixtures and a made-up name
	// in the picker would have been noise — but aetox-grid is now a real
	// default with a real job (it answers the guide, §42), so a fresh install
	// that shows no model name at all is the wrong end of that trade.
	if cfg.ModelName == "" {
		cfg.ModelName = model.ResolveDefaultModel(cfg.ModelProvider, cfg.ModelBaseURL, cfg.ModelAPIKey)
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

func bootstrapFromConfig(cfg config.Config, onToolAction func(turn.ToolEvent), onStatus func(string), onUsage func(model.Usage), extraSkills []skill.Skill, mcpMgr *mcp.Manager, outputSubdir func() string) (*aetoxapp.App, *cognitive.Agent, string, *skill.Registry) {
	defer debuglog.Block("bootstrapFromConfig")()
	// Which model this actually resolved to. The log recorded that a bootstrap
	// happened but never what came out of it, so "it switched models on its
	// own" had no evidence either way — the same reason startup logs a
	// checkpoint at all.
	debuglog.Info("resolved model", fmt.Sprintf("%s / %s (think=%s, approval=%s)",
		cfg.ModelProvider, cfg.ModelName, cfg.ThinkLevel, cfg.ApprovalMode))
	status := model.ResolveStatus(cfg.ModelProvider, cfg.ModelName, nil)

	providerDone := debuglog.Block("model.BootstrapProvider")
	bootstrapResult := model.BootstrapProvider(model.BootstrapOptions{
		Provider:   cfg.ModelProvider,
		Model:      cfg.ModelName,
		APIKey:     cfg.ModelAPIKey,
		BaseURL:    cfg.ModelBaseURL,
		Timeout:    30 * time.Second,
		WireFormat: cfg.ModelWireFormat,
		Locale:     cfg.UILocale,
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
		SandboxRoot:  cfg.SandboxRoot,
		OutputSubdir: outputSubdir,
	})
	for _, s := range extraSkills {
		if err := registry.Register(s, skill.SourceExternal); err != nil {
			debuglog.Msg("skill registration skipped: %v", err)
		}
	}
	// Scans ~/.aetox/skills — a real filesystem walk, not a fixed-cost lookup;
	// timed because a large/slow-disk skills directory is a plausible,
	// easy-to-overlook source of startup latency.
	discoverDone := debuglog.Block("skill.RegisterDiscovered")
	for _, discErr := range skill.RegisterDiscovered(registry, skill.DefaultDiscoveryPaths()) {
		debuglog.Msg("skill discovery: %v", discErr)
	}
	discoverDone()
	dispatcher := skill.NewDispatcher(registry)

	permissions, permErr := config.LoadPermissions()
	if permErr != nil {
		debuglog.Msg("permissions load failed: %v", permErr)
	}
	// Prepend the default MCP "ask" rules so MCP tools never auto-run. These are
	// derived from configured server names WITHOUT connecting — so the safety
	// gate is in place synchronously here, even though the tools themselves are
	// registered later by a background connect (applyConfig), which is what used
	// to block startup ~5s on a cold `npx` resolve. A tool named "<server>_x"
	// can't be called before its "<server>_*" ask-rule exists, so nothing races.
	// User rules stay last (last-match-wins) so explicit choices still win.
	permissions.Rules = append(mcpMgr.PermissionRules(), permissions.Rules...)

	// `task` + `task_result` — the only way a sub-agent runs (§44.4, §44.11:
	// starting one never blocks the turn). Registered here rather than
	// in skill.RegisterDefaults because it needs turn+cognitive, which skill
	// cannot import. It holds the live provider/registry/permissions, so a
	// re-bootstrap replaces it along with everything else instead of leaving it
	// pointed at a dead engine. FilterRegistry drops it from every child, which
	// is what keeps delegation depth at 1.
	for _, tool := range subagent.NewTaskTools(subagent.TaskOptions{
		Provider:     bootstrapResult.Provider,
		Model:        cfg.ModelName,
		Registry:     registry,
		Permissions:  permissions,
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: onToolAction,
		OnUsage:      onUsage,
		MaxChars:     ctxTokens * 4,
		ThinkLevel:   think.NormalizeLevel(cfg.ThinkLevel),
	}) {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			debuglog.Msg("%s registration skipped: %v", tool.Name(), err)
		}
	}

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

// UserName / SetUserName back the sidebar footer's display name. They go
// through the preference file rather than localStorage for the reason spelled
// out on config.ModelPreference.UserName. persistModelPreference is a
// load-modify-save, so a later model switch leaves this field alone.
func (a *App) UserName() string {
	pref, _, _ := config.LoadModelPreference()
	return pref.UserName
}

func (a *App) SetUserName(name string) error {
	pref, _, err := config.LoadModelPreference()
	if err != nil {
		return err
	}
	pref.UserName = strings.TrimSpace(name)
	return config.SaveModelPreference(pref)
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
	pref.ModelWireFormat = strings.TrimSpace(cfg.ModelWireFormat)
	pref.ThinkLevel = model.NormalizeThinkingLevel(canonicalProvider, pref.ModelName, cfg.ThinkLevel)
	pref.ApprovalMode = string(safety.NormalizeApprovalMode(cfg.ApprovalMode))
	// Only overwrite when we actually have one: a model change must not wipe a
	// language the user already picked.
	if v := strings.TrimSpace(cfg.UILocale); v != "" {
		pref.UILocale = v
	}
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
