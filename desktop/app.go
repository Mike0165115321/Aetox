package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // imageTokens reads attachment headers for the context meter
	_ "image/jpeg" // (same set slides_write validates)
	_ "image/png"
	"io"
	"mime"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	aetoxapp "github.com/Mike0165115321/Aetox/internal/app"
	"github.com/Mike0165115321/Aetox/internal/bootstrap"
	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/connect"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/learned"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/oauth"
	"github.com/Mike0165115321/Aetox/internal/ooxml"
	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/prompt"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/snapshot"
	"github.com/Mike0165115321/Aetox/internal/subagent"
	"github.com/Mike0165115321/Aetox/internal/turn"
	"github.com/Mike0165115321/Aetox/internal/update"
	"github.com/Mike0165115321/Aetox/internal/version"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx         context.Context
	chat        *aetoxapp.App
	agent       *cognitive.Agent
	cfg         config.Config
	modelStatus string
	// modelErr is why the last bootstrap could not reach the configured
	// provider. Non-nil with a live chat means the engine is running the
	// built-in aetox fallback while the UI names something else — see
	// modelSwitchResult.
	modelErr    error
	toolHistory []string

	terminalsMu sync.Mutex
	terminals   map[string]*TerminalSession
	browsers    *browserHost

	// quotasMu guards quotas, which the model clients write from whatever
	// goroutine a turn is running on.
	quotasMu sync.RWMutex
	// quotas is the last rate-limit window each provider stated, keyed by
	// canonical provider name. Deliberately in memory only: a quota describes
	// a window measured in minutes to days, so a figure restored from disk at
	// the next launch would be a number that was true once, presented as if it
	// were true now. An absent key means "never heard from this provider",
	// which the UI shows as "not known yet" rather than as full.
	quotas map[string][]model.Quota

	// pricesMu guards the model catalog, which a background refresh replaces
	// while the stats page may be reading it.
	pricesMu sync.Mutex
	// prices holds what models.dev states about each model: rates, so recorded
	// tokens can be shown as money, and context windows, so the composer's
	// meter measures against the real one. Unlike quotas this IS restored from
	// disk, and for the opposite reason: a published list price is still
	// roughly true a week later, where a rate-limit window is not true a minute
	// later. nil means no catalog was ever fetched, which shows as no money
	// column at all rather than as zero.
	prices *model.ModelCatalog
	// pricesLoaded separates "read the cache and found nothing" from "have not
	// looked yet", so a missing catalog costs one disk miss per run and not one
	// per stats page open.
	pricesLoaded bool

	sessionID  string
	transcript []SessionMessage

	// desk is the mode the open session was created at (ARCHITECTURE.md §83),
	// nil for the full desk — every session from before modes existed, and the
	// state the app starts in. It is set when a session is opened and never
	// while one is running: a session is born at a desk and stays there, which
	// is the whole reason its context can be trusted never to have held another
	// desk's tools.
	//
	// Changing it re-bootstraps the engine (switchDesk), because everything the
	// desk decides — the dispatcher's filter, the system prompt's direction and
	// memory, the ceiling over sub-agents — is built once, at bootstrap, from
	// this one value.
	desk *mode.Mode

	// chair is the session's second coordinate (§85): which of the office's
	// agents the user is talking to directly, "" for every session held with
	// the main assistant. Only ever non-empty alongside desk = the office —
	// setStation is the single writer and enforces that pairing. Same
	// lifecycle rule as desk: set when a session opens, never while one runs.
	chair string

	// space is the session's third coordinate: which โปรเจกต์ (the storefront
	// kind — see spaces.go on why the word is `space` here) this chat is being
	// held inside, "" for a chat held outside every project. Same lifecycle as
	// desk and chair: set when a session opens, never while one runs, and
	// restored from the row when one is reopened.
	//
	// It moves no wall. The sandbox is exactly where it was, which is the line
	// COMPANY.md §84 draws between this and the workshop's project — all this
	// field changes is that the assistant is told which project it is working
	// in and where that project keeps its files.
	space string

	// stance is the session's fourth coordinate (DECISIONS.md §106): how the
	// turn runs, as opposed to what is on the desk. The zero value is ลงมือ.
	//
	// **The one coordinate with a different lifecycle, and that is the whole
	// point of it.** desk, chair and space are set when a session opens and
	// never while one runs, because each of them would change what the context
	// already holds. A stance only ever subtracts from the desk, so moving it
	// mid-conversation cannot put another desk's tools into this one — which is
	// what lets SetStance re-bootstrap in place (carrying the agent's context
	// over, as a model switch does) instead of opening a new session.
	stance mode.Stance

	// turnOpened is true between openTurn and appendTurn: the user message for
	// the turn now running is already in the store, so the closing write must
	// not add it a second time. See openTurn for why the pair stopped being one
	// transaction.
	turnOpened bool

	// projectFocused=false runs the engine "ไม่โฟกัสโปรเจกต์": rooted at the
	// user's home dir so every tool (files/git/terminal) still works on the
	// machine, but nothing is treated as a project (no tree walk, no recent-
	// projects entry, UI shows an unfocused chip). This is the startup default —
	// the app must not silently adopt whatever cwd it was launched from.
	projectFocused bool

	// extraRoots are the folders the user added to the focused project — the
	// whole of what widens the sandbox beyond it (desktop/workspace.go). Loaded
	// from the store on every focus switch and handed to the engine and the
	// system prompt from this one field, so the panel, the prompt and the gate
	// cannot disagree about what this session can reach.
	//
	// Read and written only through workspaceRoots/setWorkspaceRoots: the
	// workspace door adds a folder from the turn goroutine mid-tool-call, while
	// a binding call may be reading the same slice to draw the panel.
	extraRoots []string
	// wsMu guards extraRoots and workspaceDirty.
	wsMu sync.Mutex
	// workspaceDirty marks a folder added while a turn was running. The running
	// call gets the folder immediately (skill.SetWorkspaceFolders); the engine
	// gets it at endTurn, because rebuilding it underneath a turn in flight
	// would throw away the work the user is waiting for.
	workspaceDirty bool

	// shells is which shell this project's commands run in — the machine's own,
	// or a WSL distro. See desktop/shell_backend.go.
	shells config.ShellChoice

	turnMu     sync.Mutex
	turnCancel context.CancelFunc // cancels the chat turn in flight, nil when idle
	// turnCtx is that same turn's context, kept so a gate deep inside a tool
	// call can wait on the user without outliving the turn. The workspace door
	// (workspace.go) is the one that needs it: resolveSandboxPath carries no
	// context of its own, so without this a card left unanswered would hold the
	// tool call open and Stop would do nothing until somebody clicked it.
	turnCtx context.Context

	// turnRunning/turnSession are the turn in flight's identity: whether one is
	// running, and which session it belongs to. Stamped by beginTurn and read by
	// everything the turn's output lands in (openTurn, appendTurn), because the
	// alternative — reading a.sessionID at completion time — is how an answer
	// ended up persisted into whichever chat happened to be open when the turn
	// finished. The stamp is taken once, at birth, and cannot be moved.
	//
	// Both under turnMu: a switch door checks them from a binding goroutine
	// while the turn goroutine writes them.
	turnRunning bool
	turnSession string

	// turnStopEarly records a Stop that arrived in the window between beginTurn
	// and runTurn installing turnCancel — openTurn's DB writes sit in that gap,
	// and a busy database holds it open for whole seconds. Without this the
	// press lands on a nil cancel func and silently does nothing, which is a
	// Stop button that sometimes needs pressing twice. Consumed (and the turn
	// killed) the moment the cancel func exists.
	turnStopEarly bool

	// snapshots is the undo net (internal/snapshot). Nil whenever it cannot
	// work — no git, or a project that is not a repository — and every use of
	// it is written to carry on without it rather than refuse to run.
	snapshotMu   sync.Mutex
	snapshots    *snapshot.Store
	lastSnapshot string // the tree captured before the last turn, "" if none

	askMu sync.Mutex
	askCh chan string // the in-flight ask_user question's answer channel, nil when idle

	mcp      *mcp.Manager    // configured MCP servers; built once, survives re-bootstraps
	registry *skill.Registry // current skill/tool registry, for the Tools panel

	// delegations is the engine's register of sub-agent work. Held because a
	// delegate outlives the turn that started it (internal/subagent/runner.go),
	// which leaves this app's Stop button as the only thing that ends one early.
	// Replaced on every re-bootstrap along with everything else it belongs to.
	delegations *subagent.Delegations

	// A worker addressed with @name that stopped to ask something, and is still
	// holding everything it had done when it stopped. The next message answers
	// it (runAnswer) unless that message addresses somebody else.
	//
	// Session state, not engine state: it is cleared by a re-bootstrap along
	// with the runner that holds the run, which is why finishAddressed writes
	// both fields on every outcome rather than only on the waiting one.
	pendingTask  string
	pendingAgent string

	// When each engine's start command was fired, so a second call cannot start
	// a second server over the first (engine_server.go). On the App because the
	// tool that reads it is rebuilt by every re-bootstrap and the fact is not.
	engineStartMu   sync.Mutex
	engineStartedAt map[string]time.Time

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

	// taskChips holds the side work the agent has flagged with suggest_task
	// and the user has not yet started or dismissed (task_chips.go).
	taskChips taskChips

	// openTabs is what the frontend reports is open on the workbench, so the
	// agent can read its own desk (desk.go). Deliberately not named `desk` —
	// that field is the mode the session was opened at (§83), a different thing.
	openTabs deskState

	// remoteSrv is the phone's door into this process (remote.go), built on
	// first use and never without one: the listener stays down until the user
	// opens the pairing panel, so an install nobody asked to reach from a
	// phone opens no port at all.
	remoteOnce sync.Once
	remoteSrv  *remoteServer

	// staged is the downloaded, verified update waiting for the user to pick a
	// moment to restart into (§107). Held here rather than in internal/update
	// because it is one running app's state, not the package's — and guarded
	// because StageUpdate runs on whichever goroutine Wails hands it while
	// RestartToUpdate reads it from another.
	stagedMu sync.Mutex
	staged   update.Staged
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
	a.emitEvent("agent:tool", ev)
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
	a.emitEvent("agent:status", status)
}

// chatChunk is one write to the live answer bubble.
//
// Replace makes the payload the bubble's whole content instead of an addition,
// which is what lets one event name carry three different things safely: a
// streamed fragment (Replace false), an erased preview (Replace true, empty),
// and the finished reply (Replace true, whole text).
//
// It matters that all three ride the SAME event. Wails delivers emits of one
// event name in order, so "erase, then here is the answer" cannot arrive
// backwards — and because the delivery replaces rather than appends, the answer
// cannot end up printed twice even if a preview was mid-flight.
type chatChunk struct {
	Text    string `json:"text"`
	Replace bool   `json:"replace"`
}

// emitChatChunk is the one way anything reaches the live answer bubble.
func (a *App) emitChatChunk(text string, replace bool) {
	a.emitEvent("agent:chunk", chatChunk{Text: text, Replace: replace})
}

// previewAnswer shows the model's answer as it is written. Wired session-wide
// (aetoxapp.Options.OnContentPreview) rather than per turn, because a window can
// always draw a preview — see the doc on that field for why it is not a delivery.
func (a *App) previewAnswer(chunk string) { a.emitChatChunk(chunk, false) }

// discardAnswerPreview erases it, for every round whose text turns out not to be
// the answer.
func (a *App) discardAnswerPreview() { a.emitChatChunk("", true) }

// AppVersion is the release this build calls itself, for Settings → About.
//
// Until now the desktop app had no idea which version it was: the number was
// baked into the exe's Windows version resource by Wails and into a const in
// cmd/aetox, i.e. into a file this binary does not compile and a resource Go
// cannot read back. "Which version am I running?" was answerable only by
// right-clicking the exe — and an update check cannot be built on that at all.
func (a *App) AppVersion() string { return version.Current }

// RecentDebugLog is the evidence half of the About page's "ส่งปัญหาให้นักพัฒนา":
// the app's most recent internal complaints about itself, for prefilling into
// the GitHub issue body. Already secret-scrubbed at the moment each line was
// written (debuglog's single funnel), and the user reads every line on
// GitHub's own form before anything is submitted — the app itself sends
// nothing, to anyone, ever.
func (a *App) RecentDebugLog() []string { return debuglog.Recent(30) }

// CheckForUpdate asks GitHub whether a newer release exists. Explicitly, from
// the button in Settings → About — nothing calls it on a timer yet.
//
// ErrDisabled is folded into the returned Status rather than raised: the user
// switching the check off is not a failure, and rendering it as one would put
// a red error under a setting they chose. Every other failure does reject, so
// "could not reach GitHub" reads as what it is.
func (a *App) CheckForUpdate() (update.Status, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	st, err := update.Check(ctx, version.Current)
	if errors.Is(err, update.ErrDisabled) {
		return st, nil
	}
	return st, err
}

// StageUpdate downloads the newer release, verifies its signature and its
// bytes, and puts this machine one restart away from running it — without
// touching the window. Progress rides out as `update:progress` in bytes, which
// is what the download actually knows; turning that into a percentage or a
// megabyte count is the UI's business.
//
// It does not restart, on purpose: see internal/update's Stage. Downloading is
// cheap for the user, closing their window is not, and one act that did both
// would spend the second without asking.
//
// Deliberately not refused mid-turn. Nothing here interrupts anything — the
// agent keeps working while the bytes come down, and the gate belongs on
// RestartToUpdate, which is where the process actually ends.
func (a *App) StageUpdate() error {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	staged, err := update.Stage(ctx, version.Current, func(done, total int64) {
		a.emitEvent("update:progress", map[string]int64{"done": done, "total": total})
	})
	if err != nil {
		return err
	}
	a.stagedMu.Lock()
	a.staged = staged
	a.stagedMu.Unlock()
	return nil
}

// StagedUpdate is the version waiting for a restart, or "" if none is. What a
// window that just reloaded asks, so a staged update survives the webview
// coming back without the Go side having lost it.
func (a *App) StagedUpdate() string {
	a.stagedMu.Lock()
	defer a.stagedMu.Unlock()
	return a.staged.Version
}

// RestartToUpdate is the half the user times: close this build, let the waiter
// bring the new one up.
//
// Refused mid-turn for the same reason every session switch is — this ends the
// process, and the process is where the turn lives. Its own sentence
// (errTurnBusyUpdate) because the shared one ends in advice about switching
// chats, which is not the door the user is standing in.
func (a *App) RestartToUpdate() error {
	if a.turnBusy() {
		return errTurnBusyUpdate
	}
	a.stagedMu.Lock()
	staged := a.staged
	a.stagedMu.Unlock()
	if err := staged.Restart(); err != nil {
		return err
	}
	// The relauncher is waiting on this process. Quit on a short delay rather
	// than here: the frontend's await deserves its resolution first, so the
	// button can honestly say "restarting" instead of the window vanishing
	// mid-click.
	go func() {
		time.Sleep(400 * time.Millisecond)
		if a.ctx != nil {
			wailsruntime.Quit(a.ctx)
		}
	}()
	return nil
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

// OpenFileExternally hands a file inside the sandbox root to whatever program
// the operating system opens it with.
//
// It exists because the agent now produces files this app cannot render.
// `sheet_write` writes a real .xlsx (ARCHITECTURE.md §75) and ReadFile below
// correctly refuses it — a spreadsheet is a ZIP — so clicking the workbook the
// agent just announced opened an editor containing the words "binary file
// cannot be previewed". The app promised finished work and then declined to
// show it.
//
// Handing it to Excel rather than building a viewer is the whole positioning:
// the file is already on the user's machine and so is the program that opens
// it. A worse spreadsheet inside Aetox is not a feature, and rendering one
// would mean writing an OOXML *reader* — explicitly out of scope in
// OFFICE-EXPORT-PLAN.md §8.
//
// The sandbox check is not decoration. This launches a program of the OS's
// choosing on a path a caller supplies, so the path has to be one the user
// could have clicked in their own project.
// errFileGone is the one failure the file pane translates for itself rather
// than showing verbatim. Matched by the frontend, so the text is a contract.
var errFileGone = errors.New("file-gone")

// FileStillThere reports whether a path the app previously produced is still on
// disk.
//
// The file cards in a reply are history: they record what that turn made, and
// they are rebuilt from the message's own parts on reload (§80, §81), so they
// must not disappear when a file does — the turn still produced it. What must
// not survive is the *offer*. The pane used to say "this app cannot preview it,
// but a program on your machine can" about a file that was not there at all,
// and only the click revealed otherwise, as an OS error string.
//
// So: history says what happened, the pane says what is true now. This is how
// the pane finds out.
//
// A path outside the sandbox, or no project open, answers false — the same as
// missing, because in both cases there is nothing here to open.
func (a *App) FileStillThere(relPath string) bool {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return false
	}
	full, err := safeSandboxPath(root, relPath)
	if err != nil {
		return false
	}
	info, err := os.Stat(full)
	return err == nil && !info.IsDir()
}

func (a *App) OpenFileExternally(relPath string) error {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return fmt.Errorf("no project open")
	}
	full, err := safeSandboxPath(root, relPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(full)
	if os.IsNotExist(err) {
		// A file the agent produced can legitimately be gone: it can delete
		// files, and session output folders age out. Raising the OS error here
		// put "GetFileAttributesEx …: The system cannot find the file
		// specified" in front of the user, which reads as a crash rather than
		// as the ordinary thing it is. FileStillThere is what stops the
		// question being asked at all; this covers the gap between asking and
		// clicking.
		return errFileGone
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%q is a directory", relPath)
	}
	// Same one implementation the three reveal buttons share (speech.go): on
	// every platform the command it runs opens a file with its default program
	// just as it opens a folder in the file manager.
	return openInFileManager(full)
}

// ReadWorkbook renders a .xlsx inside the sandbox root as rows of display
// text, for the file pane's spreadsheet preview.
//
// It is a preview and not an editor, and the distinction is the whole design
// (ARCHITECTURE.md §79): every value comes back as a string already formatted
// the way Excel would show it, nothing round-trips, and the "open with my
// computer's app" button stays for anything past a glance. Aetox has no
// business being a worse Excel — but it should be able to answer "what did I
// just get?" without the user leaving the window.
//
// The 8 MB ceiling is about the round trip, not the file: the whole preview is
// marshalled to JSON and crosses an IPC boundary, and a workbook past that size
// is one to open in Excel anyway.
func (a *App) ReadWorkbook(relPath string) (*ooxml.WorkbookPreview, error) {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return nil, fmt.Errorf("no project open")
	}
	full, err := safeSandboxPath(root, relPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	const maxWorkbookBytes = 8 << 20
	if info.Size() > maxWorkbookBytes {
		return nil, fmt.Errorf("workbook too large to preview (%d bytes)", info.Size())
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return ooxml.ReadXLSX(data)
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

// SaveChatImageData is the same as SaveChatImage for an image that has no file
// behind it — one pasted from the clipboard.
//
// A screenshot, or a chart copied out of an answer with the drawing's own
// คัดลอก button, exists only as bytes on the clipboard: there is no path to
// hand SaveChatImage, which is why every attach route in this app used to
// require the picker. Ctrl+V is the obvious way to put a screenshot in front of
// the assistant, and it did nothing at all.
//
// Same destination, same session folder, same cap as a picked image — the only
// difference is where the bytes came from.
func (a *App) SaveChatImageData(dataURL string) (string, error) {
	data, ext, err := decodeImageDataURL(dataURL)
	if err != nil {
		return "", err
	}
	if int64(len(data)) > 20<<20 {
		return "", fmt.Errorf("รูปใหญ่เกินไป (%d MB, สูงสุด 20 MB)", len(data)>>20)
	}
	destPath, root, err := a.chatAttachmentDest(ext)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return "", err
	}
	return relativeToRoot(root, destPath)
}

// The image types the attach dialog already offers. An unknown type is refused
// rather than written with a guessed extension: the path this returns is handed
// to skills that open it by extension, and a .png holding something else is a
// file that fails later, somewhere less obvious than here.
var pastableImageExt = map[string]string{
	"png": ".png", "jpeg": ".jpg", "jpg": ".jpg",
	"gif": ".gif", "webp": ".webp", "bmp": ".bmp",
}

func decodeImageDataURL(dataURL string) (data []byte, ext string, err error) {
	raw, ok := strings.CutPrefix(strings.TrimSpace(dataURL), "data:image/")
	if !ok {
		return nil, "", fmt.Errorf("ไม่ใช่รูปภาพ")
	}
	kind, payload, ok := strings.Cut(raw, ";base64,")
	if !ok {
		return nil, "", fmt.Errorf("ไม่ใช่รูปภาพ")
	}
	ext, ok = pastableImageExt[strings.ToLower(kind)]
	if !ok {
		return nil, "", fmt.Errorf("ยังแนบรูปชนิด %s ไม่ได้", kind)
	}
	data, err = base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		return nil, "", fmt.Errorf("ข้อมูลรูปไม่สมบูรณ์")
	}
	return data, ext, nil
}

// chatAttachmentDest picks where the next attachment goes: one folder per
// session, a name that cannot collide. Shared by the copy-a-file path and the
// paste-bytes path so an attachment lands in exactly one place either way.
func (a *App) chatAttachmentDest(ext string) (destPath, root string, err error) {
	root = strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return "", "", fmt.Errorf("no project open")
	}
	// One subfolder per session, so an attachment lives and dies with its chat
	// (DeleteSession removes the folder, sweepAttachments catches orphans).
	// Before this, every session's attachments piled up in one shared folder
	// forever — a later chat could list and read documents attached to any
	// earlier one.
	if a.sessionID == "" {
		return "", "", fmt.Errorf("no active session")
	}
	destDir := filepath.Join(root, attachmentsDir, a.sessionID)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", "", err
	}
	seq := atomic.AddInt64(&attachmentSeq, 1)
	return filepath.Join(destDir, fmt.Sprintf("%d-%d%s", time.Now().UnixMilli(), seq, ext)), root, nil
}

func relativeToRoot(root, destPath string) (string, error) {
	rel, err := filepath.Rel(root, destPath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func (a *App) saveChatAttachment(sourcePath string, maxBytes int64) (string, error) {
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
	destPath, root, err := a.chatAttachmentDest(filepath.Ext(sourcePath))
	if err != nil {
		return "", err
	}

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
	return relativeToRoot(root, destPath)
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
	// A data: URL costs about 4/3 its file size as a JavaScript string that then
	// lives in the DOM. Attachments are already capped on the way in
	// (SaveChatImage), but the workbench opens whatever image is dropped on it,
	// and a 200MB scan would take the webview down rather than show anything.
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	const maxImageBytes = 20 << 20
	if info.Size() > maxImageBytes {
		return "", fmt.Errorf("รูปใหญ่เกินไปสำหรับการแสดงตัวอย่าง (%d MB, สูงสุด %d MB)", info.Size()>>20, maxImageBytes>>20)
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

// attachedImageRe matches the line the composer appends for a user-attached
// image (see cockpit.svelte.ts sendUserMessage). Both ends of that line are
// ours and the frontend already parses it back out on session restore, so it is
// a format we own rather than text we are guessing at.
var attachedImageRe = regexp.MustCompile(`\n*\[attachment: user-attached image — [^\]]*\] (\S+)`)

// visionAttachments answers the one question that decides how an attached image
// reaches the model: can this model look at it?
//
// Yes — the bytes are loaded and the marker line is rewritten, because a model
// holding the picture should not also be told to go OCR it. No — nothing
// changes at all: the marker stands, image_ocr runs, and the model reads the
// letters out of it exactly as it has since §22. That is the whole point of
// keeping OCR rather than replacing it; the fallback is not a lesser path, it
// is the only path for a model with no eyes.
//
// Returns the text to send and the images to attach.
func (a *App) visionAttachments(text string) (string, []model.Image) {
	matches := attachedImageRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	if !model.ResolveVision(a.cfg.ModelProvider, a.cfg.ModelName) {
		return text, nil
	}
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return text, nil
	}

	var images []model.Image
	rewritten := text
	for _, m := range matches {
		relPath := m[1]
		full, err := safeSandboxPath(root, relPath)
		if err != nil {
			continue // outside the sandbox: leave the OCR line, which is bounded too
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue // unreadable: the OCR path will report it in the model's own terms
		}
		mediaType := mime.TypeByExtension(filepath.Ext(full))
		if !strings.HasPrefix(mediaType, "image/") {
			continue
		}
		images = append(images, model.Image{MediaType: mediaType, Data: data})
		// The path stays in the text: the model still needs to know what the
		// file is called to talk about it, or to edit it later.
		rewritten = strings.Replace(rewritten, m[0],
			"\n\n[attachment: user-attached image, included below] "+relPath, 1)
	}
	if len(images) == 0 {
		return text, nil
	}
	return rewritten, images
}

// attachedDocumentRe matches the line the composer appends for a PDF. Same
// contract as attachedImageRe: both ends are ours, and the composer's wording
// is pinned by a test on each side.
var attachedDocumentRe = regexp.MustCompile(`\n*\[attachment: user-attached file — read it with pdf_read\] (\S+)`)

// documentMaxInlineBytes is where handing the model the whole document stops
// being the better trade.
//
// Unlike an image, a document has no ceiling on what it costs to read: pdf_read
// returns 220 lines whatever the input, while the file itself is the entire
// thing. A 200-page report inlined is tens of thousands of tokens on a question
// that a truncated extract might have answered. 10MB is generous for the
// documents people attach to a chat (a statement, a spec, a paper) and well
// under the size where the bill becomes the story.
const documentMaxInlineBytes = 10 << 20

// documentAttachments is visionAttachments for a PDF, and deliberately its
// twin: the model that can read a document itself is handed the document, and
// the model that cannot is left exactly where it was — the marker stands,
// pdf_read runs, and nothing about that path changes.
//
// The asymmetry worth knowing is the cost. Sending an image to a model that can
// see is strictly better than OCR. Sending a document is better *and* dearer:
// the layout, the tables and the pages pdf_read truncates all survive, and the
// token count goes up rather than down. Hence the size cap above, which the
// image path has no equivalent of and does not need.
func (a *App) documentAttachments(text string) (string, []model.Document) {
	matches := attachedDocumentRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	if !model.ResolveDocuments(a.cfg.ModelProvider, a.cfg.ModelName) {
		return text, nil
	}
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	if root == "" {
		return text, nil
	}

	var docs []model.Document
	rewritten := text
	for _, m := range matches {
		relPath := m[1]
		full, err := safeSandboxPath(root, relPath)
		if err != nil {
			continue // outside the sandbox: leave the pdf_read line, which is bounded too
		}
		info, err := os.Stat(full)
		if err != nil {
			continue // unreadable: pdf_read will report it in the model's own terms
		}
		if info.Size() > documentMaxInlineBytes {
			continue // too big to be the cheaper answer — pdf_read's truncation wins
		}
		mediaType := mime.TypeByExtension(filepath.Ext(full))
		if !model.SupportsDocumentType(mediaType) {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		docs = append(docs, model.Document{
			Name:      filepath.Base(full),
			MediaType: mediaType,
			Data:      data,
		})
		// The path stays in the text for the same reason it does for an image:
		// the model still has to be able to name the file it is talking about.
		rewritten = strings.Replace(rewritten, m[0],
			"\n\n[attachment: user-attached file, included below] "+relPath, 1)
	}
	if len(docs) == 0 {
		return text, nil
	}
	return rewritten, docs
}

// --- undo ------------------------------------------------------------------

// UndoResult is what the UI shows after an undo: which files went back, or why
// nothing did.
type UndoResult struct {
	Files  []string `json:"files"`
	Reason string   `json:"reason,omitempty"`
}

// captureSnapshot records the project before a turn runs. Failure is silent on
// purpose — a machine without git, or a folder that is not a repository, must
// still be able to hold a conversation, and an error banner about a safety net
// the user never asked for is worse than not having one.
func (a *App) captureSnapshot() {
	a.snapshotMu.Lock()
	store := a.snapshots
	a.snapshotMu.Unlock()
	if store == nil {
		return
	}
	id, err := store.Capture(a.ctx)
	if err != nil {
		debuglog.Msg("snapshot capture skipped: %v", err)
		return
	}
	a.snapshotMu.Lock()
	a.lastSnapshot = id
	a.snapshotMu.Unlock()
}

// UndoLastTurn puts every file the last turn changed back the way it was.
//
// One turn deep, deliberately. The question a user actually asks is "undo what
// it just did", asked immediately, and an undo stack invites the far more
// dangerous "undo the last six" long after the reasons are forgotten.
func (a *App) UndoLastTurn() (UndoResult, error) {
	a.snapshotMu.Lock()
	store, id := a.snapshots, a.lastSnapshot
	a.snapshotMu.Unlock()

	if store == nil {
		return UndoResult{Reason: "undo needs the project to be a git repository"}, nil
	}
	if id == "" {
		return UndoResult{Reason: "nothing to undo yet"}, nil
	}
	files, err := store.Restore(a.ctx, id, nil)
	if err != nil {
		return UndoResult{}, err
	}
	if len(files) == 0 {
		return UndoResult{Reason: "the last turn changed no files"}, nil
	}
	// The restore IS the new state, so undoing twice must not undo further —
	// re-capturing here is what makes the second press a no-op instead of a
	// second, silent step backwards.
	a.captureSnapshot()
	return UndoResult{Files: files}, nil
}

// PendingUndo reports what an undo would touch right now, so the UI can offer
// it only when there is something to offer.
func (a *App) PendingUndo() []string {
	a.snapshotMu.Lock()
	store, id := a.snapshots, a.lastSnapshot
	a.snapshotMu.Unlock()
	if store == nil || id == "" {
		return []string{}
	}
	current, err := store.Capture(a.ctx)
	if err != nil {
		return []string{}
	}
	files, err := store.Changed(a.ctx, id, current)
	if err != nil || files == nil {
		return []string{} // never nil: §34, a nil slice crashes the frontend
	}
	return files
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
	// Warning is why the named provider is not actually running: the engine
	// fell back to the built-in aetox provider so the app stays usable, and
	// the UI has to say so instead of showing a provider nobody is talking to.
	// Empty means the provider bootstrapped for real.
	Warning string `json:"warning"`
}

// desktopProviders is the curated subset of the full engine catalog
// (model.SupportedProviders()) exposed in the desktop UI's provider picker,
// in the order the picker shows them.
//
// It is an allowlist, so a provider added to the engine catalog is invisible in
// the desktop until it is named here — which is exactly how the sign-in
// providers shipped in §61/§62 stayed missing from the UI while every test
// passed. The sign-in group is listed first because "use the plan you already
// pay for" is the shorter path for most people than finding an API key.
var desktopProviders = []string{
	// Signed into, not keyed in (internal/oauth).
	"codex", "openrouter",
	// API key or a local server.
	"anthropic", "ollama", "lmstudio", "deepseek", "gemini", "openai", "qwen", "zai",
	// Same runtime as the row above (OpenAI-compatible, base URL, key), kept off
	// only because nobody had typed them here. Each endpoint verified up to the
	// auth wall against the live API (2026-08-14): base URL, path and body shape
	// all answered 401, so only a real key separates them from working.
	"groq", "mistral", "kimi", "minimax", "together", "perplexity",
	"aetox",
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Before anything else on this path: the window is created and centred by
	// the time startup runs, and shown only once the webview has content, so a
	// window bigger than the screen is corrected while nobody can see it move.
	a.fitToScreen()
	// Providers state their remaining window in the headers of turns the app
	// was running anyway. Nothing here fetches; this only stops the answer
	// from being thrown away, which is what happened until now — the headers
	// were read on 429 and nowhere else.
	model.SetQuotaObserver(a.rememberQuotas)
	// The desktop build never wired this up before, so every debuglog.Msg/Info/
	// Block call already sprinkled through the shared engine (turn executor
	// phases, provider HTTP round-trips, ...) was silently thrown away here —
	// unlike the CLI, which always enables it (cmd/aetox/main.go). Same
	// directory as model-preference.json etc. (internal/config.DataRoot).
	if dataRoot, err := config.DataRoot(); err == nil {
		debuglog.Init(dataRoot)
	}
	// Model facts (prices, context windows), off the main path: everything runs
	// on whatever is cached and this only replaces it. One document, once per
	// launch, and a failure leaves the previous table in place — see
	// App.RefreshModelFacts. Installed eagerly too, because the context meter
	// asks before anyone opens the stats page.
	a.modelCatalog()
	go a.RefreshModelFacts()
	// Explicit checkpoint, not just debuglog.Msg's usual error-only calls —
	// most of those never fire on a clean run, so without this the log stays
	// empty and gives no evidence either way for "why did first paint feel
	// stuck." This makes the log itself the answer next time it happens.
	defer debuglog.Block("App.startup")()
	// Agent files a user dropped into the shared folder before the homes split
	// (2026-08-05) move to the agents' home, before anything reads a roster.
	if moved := subagent.Migrate(); len(moved) > 0 {
		debuglog.Msg("subagent.Migrate moved: %s", strings.Join(moved, ", "))
	}
	// Engine placements written before the HomeAgent lock (2026-08-10) may
	// still point at desks; repaired before the first session builds a cut.
	connect.EnforceHomes()
	a.focusNone()
	a.startNewSession()
	a.openAtRememberedDesk()
	// The previous build's exe, renamed aside by a self-update, and the staging
	// download — this build is running, so by definition neither is needed.
	go update.RemoveLeftovers()
	// And the other end of the same feature: ask whether a newer build exists,
	// so the answer reaches the user without them going looking for it
	// (update_notify.go).
	go a.watchForUpdates()
	// Era cleanup: home itself was the unfocused root until 2026-07-26
	// (§19.1), and attachments copied there never expired. No session writes
	// there anymore, so this only ever drains the old pile.
	if home, err := os.UserHomeDir(); err == nil {
		go a.sweepAttachments(home)
	}
}

// openAtRememberedDesk points the fresh session at the desk the user was last
// at, or — first run only — at the product's entrance.
//
// Called from startup, and only from there: it is the one moment the session
// is known to be blank, so putting it at a desk cannot throw away a
// conversation. Later switches are the nav's business.
//
// Without this the engine booted at no desk at all, and a session's desk is
// stamped when its first message is written — so anything begun by opening the
// app and typing was recorded as belonging to no desk, while the sidebar drew
// a room as the one you were standing in. Two answers to the same question,
// and the sidebar's was the one the user could see.
//
// A remembered desk whose file has since been deleted falls back to the
// entrance rather than refusing: this runs before there is a window to show an
// error in, and a start that cannot be explained is worse than a start
// somewhere ordinary.
func (a *App) openAtRememberedDesk() {
	desk := mode.Default
	if pref, ok, err := config.LoadModelPreference(); err == nil && ok && pref.LastDesk != "" {
		desk = pref.LastDesk
	}
	if err := a.setStation(desk, ""); err != nil && desk != mode.Default {
		_ = a.setStation(mode.Default, "")
	}
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

// unfocusedRoot is the working root with no project open: <home>/aetox, not
// the home directory itself.
//
// The story of this folder is two decisions, in opposite directions. Home was
// the original root, until 2026-07-26 (§19.1) narrowed everything to this
// folder: unfocused runs full-access, web_fetch/browser sit in the same tool
// loop, so a fetched page could order a read of .ssh/.aws and carry it out —
// with no prompt in the way. Then 2026-08-04 reopened the machine on purpose
// (OpenSandbox in applyConfig): "no project" should mean the machine is the
// workspace, not this one folder — being unable to find a PDF the user knows
// is on disk made the mode useless. What did NOT reopen is the part the
// 07-26 change was actually protecting: the credential stores stay refused
// inside resolveSandboxPath itself (skill/sandbox_open.go), and new files
// still land in output/<session> (outputSubdir) so everything a chat produced
// stays inspectable in one place.
//
// This folder remains the working root — relative paths resolve here, and it
// is the parent of the output folders — so nothing on disk moved when either
// decision landed.
//
// Empty when home cannot be resolved, which config.Load turns into cwd — the
// same fallback as before.
func unfocusedRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		// Never "". config.Load answers an empty root with os.Getwd(), and an
		// installed desktop's cwd is C:\Program Files\Aetox — a root where the
		// session's first write dies on "Access is denied". This is a real
		// launch path, not a hypothetical: an installer's "เปิดเลย" checkbox
		// spawns the app with a stripped environment and no USERPROFILE (seen
		// 2026-08-08, tool_runs #533 — an assistant chat whose output folder
		// landed in Program Files). The data root resolves through its own
		// chain (env override → config dir → temp), all of them writable.
		if root, rootErr := config.DataRoot(); rootErr == nil && strings.TrimSpace(root) != "" {
			return filepath.Join(root, "workspace")
		}
		return ""
	}
	return filepath.Join(home, "aetox")
}

// focusNone re-roots the engine at unfocusedRoot and marks the app as not
// focused on any project.
//
// The flag and the folder list are both set BEFORE reload in all three focus
// switches: applyConfig reads them to build the sandbox, so setting either
// after would build one engine holding the outgoing project's reach.
func (a *App) focusNone() {
	root := unfocusedRoot()
	if root != "" {
		// A root that does not exist yet makes `list .` fail on a fresh
		// install — before the first write has created it.
		_ = os.MkdirAll(root, 0o755)
	}
	a.projectFocused = false
	// Cleared rather than carried: the added folders belong to the project that
	// is being left, and this mode reaches the machine anyway.
	a.setWorkspaceRoots(nil)
	a.reload(config.ConfigOptions{RootPath: root, ApprovalMode: string(safety.ApprovalFullAccess)})
}

// TurnReply is one finished turn as the UI receives it: the answer, and the
// sequence that produced it.
//
// Parts is what a bubble should actually draw — prose, thinking segments and
// tool calls in the order they happened (turn.TurnPart). Text is the
// concatenation of its prose, kept because it is what the store, the model's
// context and every older reader use, and because a turn from before the
// sequence existed has nothing else.
type TurnReply struct {
	Text  string          `json:"text"`
	Parts []turn.TurnPart `json:"parts,omitempty"`
	// MessageID is the stored row this reply became, so the bubble can be rated
	// without reloading the session first. 0 when the turn failed and nothing
	// was written — which is also the state in which there is nothing to rate.
	MessageID int64 `json:"messageId,omitempty"`
}

func replyOf(m SessionMessage) TurnReply {
	return TurnReply{Text: m.Text, Parts: m.Parts, MessageID: m.ID}
}

// errTurnBusy is the one sentence every door shares while a turn is running.
// One gate, one message: switching chats, switching projects, deleting the
// open session and re-running an answer are all the same refusal, because they
// are all the same fact — the engine has one brain, and a turn is using it.
var errTurnBusy = fmt.Errorf("เอเจนกำลังทำงานอยู่ — รอให้เสร็จ หรือกดหยุดก่อน แล้วค่อยสลับแชท")

// errTurnBusyUpdate is the same refusal for the same reason, with the right
// tail. An update kills the process, so the gate is identical — but "แล้วค่อย
// สลับแชท" is advice about a door the user is not standing in, and it arrives
// under the update dialog's "อัปเดตไม่สำเร็จ", where it reads as the update
// having broken rather than as having been postponed.
var errTurnBusyUpdate = fmt.Errorf("เอเจนกำลังทำงานอยู่ — รอให้เสร็จ หรือกดหยุดก่อน แล้วค่อยอัปเดต (การอัปเดตต้องปิดแอป)")

// beginTurn marks one turn in flight and stamps it with the session it was
// born in. Refuses when a turn is already running: two turns share one agent
// context, one turnOpened flag and one transcript, and interleaving them
// corrupts all three. The frontend's awaitingReply gate normally prevents this
// — the case that actually reaches here is a window reloaded mid-turn, whose
// fresh state no longer knows a turn exists.
func (a *App) beginTurn() error {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	if a.turnRunning {
		return errTurnBusy
	}
	a.turnRunning = true
	a.turnSession = a.sessionID
	a.turnStopEarly = false
	return nil
}

// armTurnCancel installs the turn's cancel func and reports whether a Stop
// already arrived while there was nothing to press it against — in which case
// the caller cancels immediately instead of running a turn the user has
// already refused.
func (a *App) armTurnCancel(ctx context.Context, cancel context.CancelFunc) (stopNow bool) {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	a.turnCancel = cancel
	a.turnCtx = ctx
	stopNow = a.turnStopEarly
	a.turnStopEarly = false
	return stopNow
}

// endTurn closes the turn and tells every window it is over. The event goes
// out even for a failed turn: a reloaded window is sitting on awaitingReply
// with no promise to resolve it, and "done" is the only signal it will get.
func (a *App) endTurn() {
	a.turnMu.Lock()
	done := a.turnSession
	a.turnRunning = false
	a.turnSession = ""
	a.turnMu.Unlock()
	a.emitEvent("agent:done", TurnStatus{Running: false, SessionID: done})
	// A folder approved mid-turn reached the running tool call directly; the
	// engine still has to be rebuilt for the system prompt to say the workspace
	// grew. Here rather than at the moment of approval: applyConfig swaps the
	// agent, the registry and the dispatcher, and doing that under a turn in
	// flight would discard the work the user is waiting on.
	if a.takeWorkspaceDirty() {
		a.reloadWorkspace()
	}
}

// turnBusy reports whether a chat turn is in flight.
func (a *App) turnBusy() bool {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	return a.turnRunning
}

// turnSessionID is where the turn in flight's rows belong: the session stamped
// at its birth, never whatever a.sessionID has become since. Falls back to the
// current session when no turn is running, which is what direct callers
// (tests, imports) have always meant.
func (a *App) turnSessionID() string {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	if a.turnRunning && a.turnSession != "" {
		return a.turnSession
	}
	return a.sessionID
}

// guardSessionSwitch is the door-check every session, desk and project switch
// shares. The engine has one agent context; every one of those switches
// rewrites it (ClearContext/RestoreHistory, or a full re-bootstrap), which
// mid-turn means the running turn continues on another conversation's memory —
// and its answer lands wherever a.sessionID points afterwards. Until the
// engine can hold one context per session, the honest capability is: one turn,
// one chat, finish or stop before you leave.
func (a *App) guardSessionSwitch() error {
	if a.turnBusy() {
		return errTurnBusy
	}
	return nil
}

// TurnStatus is the engine's answer to "are you busy, and with which chat" —
// what a window that just loaded asks before deciding what to draw. A webview
// reload does not touch the engine, so a turn started before the reload is
// still running after it; without this the fresh window drew an idle composer
// over a working agent.
type TurnStatus struct {
	Running   bool   `json:"running"`
	SessionID string `json:"sessionId"`
}

// TurnInFlight reports the turn currently running, if any.
func (a *App) TurnInFlight() TurnStatus {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	return TurnStatus{Running: a.turnRunning, SessionID: a.turnSession}
}

// SendMessage runs one chat turn through the Aetox engine and returns the reply.
// The turn is appended to the current session and persisted.
func (a *App) SendMessage(text string) (TurnReply, error) {
	if err := a.beginTurn(); err != nil {
		return TurnReply{}, err
	}
	defer a.endTurn()
	// Taken before the turn so the rows it writes can be told from everything
	// already in the store — see recordJobs. Cheap (one indexed MAX) and read
	// even when learning is off, because the alternative is a second capture
	// path that has to be kept in step with recordToolRun.
	mark := a.maxToolRunID()
	started := time.Now()

	// The question is written before the work starts, not after it finishes.
	// A turn can take minutes; a window reloaded inside one used to lose the
	// message that started it and the session with it (openTurn).
	a.openTurn(SessionMessage{Role: "user", Text: text, Time: time.Now().Format("15:04")})

	userMsg, agentMsg, err := a.runTurn(text)
	if err != nil {
		// The turn ends here, and it is written down. It used to end without a
		// row: the question openTurn had already stored sat alone forever, and
		// the red box with its ลองใหม่ button lived in the window and died with
		// the first reload. appendFailedTurn also lowers turnOpened — the flag
		// that, left standing, tells the NEXT turn's appendTurn that its
		// question is already stored.
		agentMsg.ID = a.appendFailedTurn(agentMsg, err)
		// Stamped on the in-memory copy as well as the stored one. Everything
		// that reasons about a failed turn asks this field — the context rebuild
		// skips the pair, the retry drops it — and a transcript entry that knew
		// it had failed only in the database would be a turn the store treats as
		// failed and the running engine treats as ordinary.
		agentMsg.ErrorText = err.Error()
		// In the transcript for the same reason the successful pair is: it is
		// what the model's memory is rebuilt from (restoreContext), and a turn
		// the engine acted on but the history does not mention is a turn the
		// user can see and the model cannot.
		a.transcript = append(a.transcript, userMsg, agentMsg)
		return replyOf(agentMsg), err
	}
	messageID := a.appendTurn(userMsg, agentMsg)
	agentMsg.ID = messageID
	a.transcript = append(a.transcript, userMsg, agentMsg)
	// After the transcript, never instead of it: a failure to record the work
	// for later learning must not cost the user their conversation.
	a.recordJobs(messageID, userMsg.Text, agentMsg.Text, mark, time.Since(started))
	return replyOf(agentMsg), nil
}

// runTurn runs one exchange through the engine and hands back the two messages
// it produced, recording nothing.
//
// Every entry point that answers a question shares it — a fresh send, a retry
// after a failure, a regenerate, an edited resend — because they differ only in
// what they do with the result. Keeping the engine call in one place is what
// stops them drifting apart on cancellation, attachments, snapshots or the
// thinking clock.
//
// On failure the returned agent message still carries whatever text arrived
// before the error, which is what the caller shows.
func (a *App) runTurn(text string) (SessionMessage, SessionMessage, error) {
	// Every caller must have marked the turn first (beginTurn): the stamp is
	// what keeps its rows home, and the busy gate is what keeps its memory
	// whole. A future entry point that forgets would run invisible to both —
	// loud in the log beats silently correct-looking.
	if !a.turnBusy() {
		debuglog.Msg("runTurn: unmarked turn — a caller skipped beginTurn")
	}
	if a.chat == nil {
		// Shaped like every other ending, not zero values. This is a real way
		// for a turn to fail — no key, no model — and the caller now writes the
		// failure down: a pair with no role and no text would be stored as a
		// question that lost its words and an answer nothing recognises as one.
		now := time.Now().Format("15:04")
		return SessionMessage{Role: "user", Text: text, Time: now},
			SessionMessage{Role: "agent", Time: now},
			fmt.Errorf("aetox core not ready: %s", a.modelStatus)
	}
	// Prompt presets ("/name args") expand into their prompt body before the
	// engine sees the text — bundled ones and the user's alike; unknown "/..."
	// passes through to the model unchanged, so nothing regresses.
	if expanded, ok := command.ExpandPreset(text); ok {
		text = expanded
	}
	ctx, cancel := context.WithCancel(a.ctx)
	if a.armTurnCancel(ctx, cancel) {
		// Stop was pressed before this cancel func existed (the beginTurn →
		// here gap, where openTurn writes to a possibly-busy database). The
		// press has to mean stop, not "stop if the timing was lucky".
		cancel()
	}
	defer func() {
		cancel()
		a.turnMu.Lock()
		a.turnCancel = nil
		a.turnCtx = nil
		a.turnMu.Unlock()
	}()
	// Accumulate reasoning at the source so it persists with the turn — the
	// live panel alone would vanish once the turn completes. First/last chunk
	// times give the "thought for Xs" label.
	var reasoning strings.Builder
	var firstThink, lastThink time.Time
	// An attachment goes to the model whole when the model can take it — a
	// picture when it can see, a document when it can read one — and as a path
	// for image_ocr / pdf_read when it cannot. `text` is rewritten only in the
	// first case, and only the model sees the rewrite: the transcript below
	// keeps what the user's composer actually sent. Chained, because one message
	// can carry both.
	// Before anything runs, so an undo has somewhere to go back to.
	a.captureSnapshot()
	// A message that names a worker goes to that worker, in the words it was
	// written in. This is the same act the model performs with `task`, and the
	// user gets to perform it directly (owner, 12 ส.ค.: one address for both) —
	// the point being not convenience but that a step which cannot mistranslate
	// is one that does not run. See subagent.Mention.
	if agent, addressed := subagent.Mention(text); addressed {
		return a.runAddressed(ctx, agent, text)
	}
	// A worker left waiting on a decision gets the next thing said, unless that
	// message addresses somebody else — which the branch above has already
	// taken. Answering is not a new job: the run is still open, holding
	// everything it had already read, so this costs one message where starting
	// again costs the whole run.
	if a.pendingTask != "" {
		return a.runAnswer(ctx, text)
	}
	sent, images := a.visionAttachments(text)
	sent, documents := a.documentAttachments(sent)
	result, err := a.chat.RunOnceStreamWithAttachments(ctx, sent, images, documents, func(chunk string) {
		// The authoritative delivery: replaces whatever the live preview holds,
		// so the answer lands exactly once no matter what streamed before it.
		a.emitChatChunk(chunk, true)
	}, func(chunk string) {
		if firstThink.IsZero() {
			firstThink = time.Now()
		}
		lastThink = time.Now()
		reasoning.WriteString(chunk)
		a.emitEvent("agent:reasoning", chunk)
	})
	// A message can land in the moment between the loop's last drain and the reply
	// arriving here. Hand it back to the UI instead of swallowing it — this is the
	// one case the composer's old queue still exists for.
	if missed := a.agent.DrainInterjections(); len(missed) > 0 {
		a.emitEvent("agent:interjection-missed", missed)
	}
	now := time.Now().Format("15:04")
	thinkSecs := 0
	if !firstThink.IsZero() {
		// round up so even a sub-second think shows as 1s, matching the label
		thinkSecs = int(lastThink.Sub(firstThink).Round(time.Second) / time.Second)
		if thinkSecs < 1 {
			thinkSecs = 1
		}
	}
	// Built once, for both endings. A turn that failed did not do less work than
	// one that succeeded — it did the same work and then hit a wall, and it is
	// the one the user most wants to open up and read: which tools ran, what the
	// model was thinking, how far it got. Computed above the error branch rather
	// than inside each, because the two used to diverge silently and the failing
	// side was the one nobody looked at.
	agentMsg := SessionMessage{
		Role: "agent", Text: result.Reply, Time: now,
		Reasoning: strings.TrimSpace(reasoning.String()), ThinkSecs: thinkSecs,
		// The turn as it happened, so reopening this session shows the work
		// and not just the sentence it ended on.
		Parts: result.Parts,
	}
	if err != nil {
		return SessionMessage{Role: "user", Text: text, Time: now}, agentMsg, err
	}
	return SessionMessage{Role: "user", Text: text, Time: now}, agentMsg, nil
}

// runAddressed answers a message that named a worker, by giving that worker the
// message.
//
// It goes through the session's own `task` registration rather than building a
// delegate here: the machinery for cutting a registry to a profile, mounting its
// prompt, connecting the servers it carries and gating what it may do is long,
// correct, and already written once (subagent.Dispatcher). A second copy of it
// living in the desktop is a second copy to keep in step, and the two would part
// company the first time either was touched.
//
// What the user sees is the worker's answer as this turn's reply, with its tool
// calls in the live timeline where every delegate's already appear. What they do
// not see is a paraphrase, because there is nowhere for one to happen.
//
// ponytail: Parts comes back empty, so reopening the session shows the answer
// without the steps under it — the parent executor that assembles Parts is the
// thing being skipped. The live view is complete; the stored one is not yet.
func (a *App) runAddressed(ctx context.Context, agent, text string) (SessionMessage, SessionMessage, error) {
	now := time.Now().Format("15:04")
	user := SessionMessage{Role: "user", Text: text, Time: now}

	dispatcher, ok := a.taskDispatcher()
	if !ok {
		// Every door into a worker is the same door, so if it is shut this one is
		// shut too — said plainly rather than falling back to the assistant,
		// which would answer as itself under a name the user addressed.
		return user, SessionMessage{Role: "agent", Time: now},
			fmt.Errorf("ยังส่งงานให้ %s ไม่ได้ตอนนี้ — เครื่องยนต์ยังไม่พร้อม", agent)
	}
	reply, err := dispatcher.Direct(ctx, agent, text)
	return a.finishAddressed(user, reply, err, text, now)
}

// runAnswer hands the waiting worker what the user just said.
func (a *App) runAnswer(ctx context.Context, text string) (SessionMessage, SessionMessage, error) {
	now := time.Now().Format("15:04")
	user := SessionMessage{Role: "user", Text: text, Time: now}
	dispatcher, ok := a.taskDispatcher()
	if !ok {
		a.pendingTask, a.pendingAgent = "", ""
		return user, SessionMessage{Role: "agent", Time: now},
			fmt.Errorf("คำถามที่ค้างอยู่หมดอายุแล้ว — เครื่องยนต์ถูกตั้งใหม่ระหว่างทาง")
	}
	reply, err := dispatcher.Answer(ctx, a.pendingTask, text)
	return a.finishAddressed(user, reply, err, text, now)
}

// finishAddressed turns a worker's reply into the turn's two messages, and is
// the one place the pending question is set or cleared — a second place would
// be a second chance to leave a run waiting on a message that never comes.
func (a *App) finishAddressed(
	user SessionMessage, reply subagent.Reply, err error, brief, now string,
) (SessionMessage, SessionMessage, error) {
	a.pendingTask, a.pendingAgent = reply.Pending, reply.Agent
	agentMsg := SessionMessage{Role: "agent", Time: now}
	if err != nil {
		return user, agentMsg, err
	}
	text := strings.TrimSpace(reply.Output.Content)
	if !reply.Output.Success {
		// A refusal is the worker's own sentence — it says what is missing and
		// what would fix it, which is more use to the user than a red box.
		if text == "" {
			text = strings.TrimSpace(reply.Output.Stderr)
		}
		agentMsg.Text = text
		return user, agentMsg, fmt.Errorf("%s: %s", reply.Agent, text)
	}
	agentMsg.Text = text
	// The work as one card, so reopening the session shows who did this and
	// what they were asked — the parent executor that would normally assemble
	// Parts is the thing this door skips, and a turn stored as bare prose loses
	// the fact that it was not the assistant who answered.
	agentMsg.Parts = []turn.TurnPart{{
		Kind: turn.PartTool,
		Tool: &turn.ToolPart{
			Name: "task", Subject: reply.Agent, Agent: reply.Agent, Brief: brief,
			AgentKind: subagent.KindOf(reply.Agent), OK: true,
			Secs: int(time.Duration(reply.Output.DurationMs) * time.Millisecond / time.Second),
		},
	}}
	// The assistant was not in this exchange and still has to be able to follow
	// it: the next thing said is as likely to be "แก้ย่อหน้าสอง" as a new
	// subject, and an assistant that never heard the first half answers as
	// though the conversation began there.
	//
	// What it gets is that the exchange happened and roughly what came back —
	// not the payload. §84's whole argument is that the baton between rooms is a
	// reference, not the material; importing a worker's full answer into the
	// assistant's context here would be shipping the context that dispatching
	// exists to avoid, and it would be paid for on every later turn.
	if a.agent != nil {
		a.agent.RestoreHistory([]model.Message{
			{Role: "user", Content: brief},
			{Role: "assistant", Content: fmt.Sprintf(
				"(@%s answered the user directly. The gist: %s. You did not see the full answer — address @%s again if this turn needs it.)",
				reply.Agent, gist(text), reply.Agent)},
		})
	}
	// The authoritative delivery, same as a streamed turn's last chunk: nothing
	// streamed here, so this is the only one.
	a.emitChatChunk(text, true)
	return user, agentMsg, nil
}

// gist is as much of a worker's answer as the assistant is given: enough to
// follow what happened, capped so a long one cannot quietly become a permanent
// tax on every turn after it.
func gist(answer string) string {
	const max = 300
	answer = strings.Join(strings.Fields(answer), " ")
	if len([]rune(answer)) <= max {
		return answer
	}
	return string([]rune(answer)[:max]) + "…"
}

// taskDispatcher asks the session's own `task` registration whether it can be
// spoken to directly. Read per call from the live registry, never held: a
// re-bootstrap replaces the tool along with the engine behind it, and a kept
// reference would be pointed at a dead one.
func (a *App) taskDispatcher() (subagent.Dispatcher, bool) {
	if a.registry == nil {
		return nil, false
	}
	tool, ok := a.registry.Get("task")
	if !ok {
		return nil, false
	}
	dispatcher, ok := tool.(subagent.Dispatcher)
	return dispatcher, ok
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
//
// It also ends every sub-agent still running, and that is not the same thing as
// ending the turn: a delegate's life is the session's now, so an ordinary reply
// leaves one working (internal/subagent/runner.go). Stop is the exception
// because Stop is a statement about the work rather than about the turn — a
// user who presses it and then watches an agent keep editing files has been
// told a lie by the button. Deliberately outside the idle check below rather
// than inside it: work started in an earlier turn is exactly the work still
// running when there is no turn left to cancel.
func (a *App) CancelTurn() {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	// Under the lock because applyConfig replaces the register on a re-bootstrap,
	// same as a.agent below. StopAll only cancels contexts — it calls nothing
	// back into this app, so it cannot reach round for this mutex.
	if a.delegations != nil {
		if stopped := a.delegations.StopAll(); stopped > 0 {
			debuglog.Msg("stop: ended %d running sub-agent(s)", stopped)
		}
	}
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
	} else if a.turnRunning {
		// The turn exists but its cancel func does not yet (openTurn's DB write
		// sits between the two). Remember the press; armTurnCancel consumes it
		// the moment there is something to cancel.
		a.turnStopEarly = true
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
	warning := ""
	if a.modelErr != nil {
		warning = a.modelErr.Error()
	}
	return ModelInfo{
		Provider:   a.cfg.ModelProvider,
		ModelName:  a.cfg.ModelName,
		ThinkLevel: a.cfg.ThinkLevel,
		// Normalized, never raw: before startup() has built a config this is
		// "", and the frontend CACHES what this reports (seedModelFromCache) —
		// so one early call painted an empty approval dropdown on every launch
		// after it, until a later fetch happened to overwrite the cache. "ask"
		// is also the honest answer for that window: nothing has widened yet.
		ApprovalMode: string(safety.NormalizeApprovalMode(a.cfg.ApprovalMode)),
		ContextUsed:  used,
		ContextMax:   a.contextWindowTokens(),
		WireFormat:   effectiveWireFormat(a.cfg.ModelProvider, a.cfg.ModelWireFormat),
		Warning:      warning,
	}
}

// modelSwitchResult reports the engine state every Switch* method ends on.
// `a.chat == nil` on its own used to be the whole check, which read a
// fallback bootstrap as success: picking an unreachable provider (LM Studio
// with its server off) silently left the engine on the built-in aetox
// provider while the picker showed LM Studio and no error anywhere. The
// fallback stays — the app must not go dead — but it now travels as
// ModelInfo.Warning so the UI can say which provider is really answering.
func (a *App) modelSwitchResult() (ModelInfo, error) {
	if a.chat == nil {
		return ModelInfo{}, fmt.Errorf("switch failed: %s", a.modelStatus)
	}
	return a.GetModelInfo(), nil
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
	// Measured reports whether UsedTokens is the provider's own count for a
	// round this session actually sent, or an estimate of what the next one
	// will cost. False on a chat nobody has typed into yet.
	Measured bool `json:"measured"`
	// CachedTokens is how much of the last round's input the provider served
	// from its prompt cache, at a fraction of the full price. Without it the
	// meter presents 12k as 12k paid, when most of it — the system prompt and
	// the tool block, the two parts that never change — was a cache hit. Zero
	// when nothing hit or the provider does no cache accounting; the UI shows
	// the note only when there is something to say.
	CachedTokens int `json:"cachedTokens"`
}

// GetContextBreakdown reports what is in the model's context window.
//
// Two different numbers wear the same label, and conflating them is what made
// this misleading: before anything is sent, the figure is a *forecast* of the
// first request — mostly the tool definitions, which ride along with every
// message. A fresh chat showing "10.1k used" reads as a bill already run up,
// and that is the reaction it actually got. Once a round has gone out, the
// provider's own count replaces the estimate and the number becomes a fact.
func (a *App) GetContextBreakdown() ContextBreakdown {
	est := func(chars int) int { return (chars + 3) / 4 }

	systemChars, msgChars, attachTokens := 0, 0, 0
	if a.agent != nil {
		for i, m := range a.agent.ContextMessages() {
			// Everything a message carries, not just Content: reasoning rides
			// back out on the wire for providers that take it (openai_compatible
			// resends it with the history), and an attached screenshot was the
			// single biggest thing this loop used to count as zero — a pasted
			// image read as a free message while costing more than the text
			// around it.
			chars := len(m.Content) + len(m.ReasoningContent)
			for _, tc := range m.ToolCalls {
				chars += len(tc.Function.Arguments)
			}
			for _, img := range m.Images {
				attachTokens += imageTokens(img)
			}
			// A document's real price is per page and nothing here can count
			// pages cheaply, so this is a floor — roughly one page — rather
			// than a guess dressed as a measurement. The measured total from
			// the next round absorbs the true cost either way.
			attachTokens += 1500 * len(m.Documents)
			if i == 0 && m.Role == model.RoleSystem {
				systemChars = chars
			} else {
				msgChars += chars
			}
		}
	}

	toolChars := 0
	if a.registry != nil {
		// Through the desk's filter, not the whole registry: the tool block is
		// what a narrower desk exists to shrink, and reporting the full pile
		// here would tell the user the one number the choice was meant to change
		// had not changed at all.
		if defs, err := json.Marshal(a.deskTools().ToolDefinitions()); err == nil {
			toolChars = len(defs)
		}
	}

	maxTokens := a.contextWindowTokens()

	system, tools, messages := est(systemChars), est(toolChars), est(msgChars)+attachTokens
	used := system + tools + messages

	// The provider counts tokens with its own tokenizer; chars/4 is an English
	// rule of thumb that Thai does not obey. Every completed round already
	// reports its real prompt size, so once this session has sent anything the
	// total stops being a guess — the per-slice split stays estimated, because
	// nobody reports that.
	//
	// When the real count exceeds the estimate, the whole gap belongs to
	// messages: the system prompt and the tool block are byte-for-byte the same
	// every round, so the underestimate is in the conversation. The earlier
	// version scaled all three slices by real/used, which made the tools bar
	// climb round after round while not one byte of it changed — a meter
	// steadily "proving" that Aetox's tool list eats the window. Only when the
	// estimate overshoots reality (rare — the guess errs low for Thai and for
	// JSON) is proportional scaling the honest split, because then there is no
	// single slice the error can be pinned on.
	measured := false
	real, cached := a.lastPromptUsage()
	if real > 0 {
		measured = true
		switch {
		case real >= system+tools:
			messages = real - system - tools
		case used > 0:
			system = system * real / used
			tools = tools * real / used
			messages = real - system - tools
		}
		used = real
	}

	free := maxTokens - used
	if free < 0 {
		free = 0
	}
	return ContextBreakdown{
		UsedTokens: used,
		MaxTokens:  maxTokens,
		// Measured is the difference between "this is what you have spent" and
		// "this is what your next message will cost". Before it is true nothing
		// has been sent at all, and a meter reading 10.1k on a chat the user has
		// not typed into reads as a bill they have already run up — which is
		// what it looked like, and why this field exists.
		Measured:     measured,
		CachedTokens: cached,
		Slices: []ContextSlice{
			{Key: "system", Tokens: system},
			{Key: "tools", Tokens: tools},
			{Key: "messages", Tokens: messages},
			{Key: "free", Tokens: free},
		},
	}
}

// imageTokens estimates what one attached image adds to the next request.
//
// Not bytes/4: a vision model prices an image by its pixels, not its file
// size, and the two disagree by an order of magnitude in both directions — a
// 100KB screenshot costs ~1.3k tokens, which bytes/4 would call 25k. The
// working rule is Anthropic's width×height/750, and providers downscale
// anything past ~1.6k tokens, hence the cap. DecodeConfig reads only the
// header, so this costs nothing per call.
//
// The flat fallback is for a format the header-read cannot parse (webp): the
// size of a typical screenshot, chosen over zero because zero is the exact lie
// this estimate exists to stop telling.
func imageTokens(img model.Image) int {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 1500
	}
	tok := (cfg.Width*cfg.Height + 749) / 750
	if tok > 1600 {
		tok = 1600
	}
	return tok
}

// lastPromptUsage is the real input size of this session's most recent round,
// as the provider counted it, and how much of that input was a prompt-cache
// hit. Zero-zero when the session has sent nothing yet, or when usage could
// not be read — both mean "no measurement", and the caller falls back to the
// estimate.
//
// The cache half was always in the same row (COALESCE because NULL there means
// "this provider does no cache accounting", db.go); not reading it is what let
// the meter present a mostly-cached prompt as fully paid.
func (a *App) lastPromptUsage() (prompt, cached int) {
	if strings.TrimSpace(a.sessionID) == "" {
		return 0, 0
	}
	db, err := a.database()
	if err != nil {
		return 0, 0
	}
	err = db.QueryRow(
		`SELECT prompt_tokens, COALESCE(cached_prompt_tokens, 0)
		 FROM token_usage WHERE session_id = ? ORDER BY id DESC LIMIT 1`,
		a.sessionID,
	).Scan(&prompt, &cached)
	if err != nil {
		return 0, 0
	}
	return prompt, cached
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
func (a *App) ClearProjectFocus() (ProjectStatus, error) {
	if err := a.guardSessionSwitch(); err != nil {
		return ProjectStatus{}, err
	}
	a.focusNone()
	a.startNewSession()
	return a.currentProjectStatus(), nil
}

// OpenProjectFolder lets the user pick a real folder via the native OS dialog, then
// re-bootstraps the engine to run inside it (same model/provider preference).
func (a *App) OpenProjectFolder() (ProjectStatus, error) {
	if err := a.guardSessionSwitch(); err != nil {
		return ProjectStatus{}, err
	}
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
	a.projectFocused = true
	a.setWorkspaceRoots(a.storedWorkspaceFolders(dir))
	a.reload(config.ConfigOptions{RootPath: dir, ApprovalMode: string(safety.ApprovalFullAccess)})
	a.startNewSession()
	a.touchProject(a.cfg.SandboxRoot)
	return a.currentProjectStatus(), nil
}

// OpenProjectPath switches straight to a previously-opened project by path —
// used by the sidebar's recent-projects list, skipping the OS folder dialog.
func (a *App) OpenProjectPath(root string) (ProjectStatus, error) {
	if err := a.guardSessionSwitch(); err != nil {
		return ProjectStatus{}, err
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return ProjectStatus{}, fmt.Errorf("empty project path")
	}
	a.projectFocused = true
	a.setWorkspaceRoots(a.storedWorkspaceFolders(root))
	a.reload(config.ConfigOptions{RootPath: root, ApprovalMode: string(safety.ApprovalFullAccess)})
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
	baseURL := resolveBaseURLForProvider(canonical)
	apiKey := resolveAPIKeyForProvider(canonical)
	if choices, err := model.ModelChoicesWithEndpointAndAPIKey(canonical, baseURL, apiKey); err == nil && len(choices) > 0 {
		return choices
	}
	if choices := model.ModelChoices(canonical); choices != nil {
		return choices
	}
	return []string{}
}

// ProviderAccount is what Settings shows under a provider's name: what is left
// in the account, and how much of the current rate-limit window remains.
//
// The two travel together because they are one line to the user, but they are
// not one fact: Balance was fetched just now, Quota is whatever the last real
// turn happened to state. Anything the UI needs to tell those apart —
// Balance.FetchedAt, Quota.ObservedAt — is carried, and none of it is
// flattened into a display string here, so Thai and English both come from the
// frontend locale files rather than from Go.
type ProviderAccount struct {
	Provider string        `json:"provider"`
	Balance  model.Balance `json:"balance"`

	// Quotas is nil in two different situations the UI must not merge:
	// QuotaKnown false means this provider has never answered a turn, so the
	// window is simply not known yet; QuotaKnown true with no entries means it
	// has answered and stated no limits, i.e. it does not report one.
	Quotas     []model.Quota `json:"quotas"`
	QuotaKnown bool          `json:"quotaKnown"`

	// ExpectsQuota is whether this provider states a window at all. Without it
	// the UI had to guess, and guessed by provider kind — which printed "the
	// limit is not known yet" under DeepSeek, a pay-as-you-go account that has
	// no window and never will.
	ExpectsQuota bool `json:"expectsQuota"`

	// Error is why Balance carries no figure. Empty when there was nothing to
	// fetch (a local runtime, a subscription) — that is not a failure, and the
	// UI says so with Balance.Kind instead.
	Error string `json:"error"`
}

// ProviderAccountFor answers for one provider: the Settings card the user
// opened, or the one provider actually in use for the profile menu.
//
// One at a time on purpose. Fetching every enabled provider at once meant
// spending a round trip on companies the user was not talking to, to fill rows
// that no longer exist — the menu shows the provider in use and nothing else.
//
// Never returns an error. A provider being unreachable is a fact about that
// provider, carried in the Error field, not a reason to blank the panel.
func (a *App) ProviderAccountFor(providerName string) ProviderAccount {
	return a.providerAccount(providerName)
}

func (a *App) providerAccount(providerName string) ProviderAccount {
	canonical := model.NormalizeProvider(providerName)
	account := ProviderAccount{
		Provider:     canonical,
		ExpectsQuota: model.StatesQuota(canonical),
	}

	balance, err := model.FetchBalance(
		a.ctx, canonical,
		resolveBaseURLForProvider(canonical),
		resolveAPIKeyForProvider(canonical),
	)
	account.Balance = balance
	if err != nil {
		account.Error = err.Error()
	}

	a.quotasMu.RLock()
	quotas, known := a.quotas[canonical]
	a.quotasMu.RUnlock()
	account.Quotas, account.QuotaKnown = quotas, known

	// OpenRouter is the one provider that states its window in the same
	// response as the credits, so it has an answer before any turn has run.
	if balance.Quota != nil {
		account.Quotas = []model.Quota{*balance.Quota}
		account.QuotaKnown = true
	}
	return account
}

// rememberQuotas is the sink installed on the model package at startup.
func (a *App) rememberQuotas(providerName string, quotas []model.Quota) {
	a.quotasMu.Lock()
	defer a.quotasMu.Unlock()
	if a.quotas == nil {
		a.quotas = make(map[string][]model.Quota, 4)
	}
	// The key is written even for an empty slice: "answered, reports no
	// limits" and "never answered" are different states on screen.
	a.quotas[model.NormalizeProvider(providerName)] = quotas
}

// ProviderAPIKeyURL is the page on the provider's own site where the user
// creates the key the Settings card is asking them to paste. Empty when there
// is nowhere to send them — a local runtime, or a sign-in provider whose row
// never shows a key field at all.
//
// The card asked for a key and left finding it as an exercise; every provider
// hides that page somewhere different, so the answer belongs next to the field.
func (a *App) ProviderAPIKeyURL(providerName string) string {
	return model.APIKeyURL(model.NormalizeProvider(providerName))
}

// ProviderBaseURL reports the API endpoint a provider will actually be called
// on: the user's override if there is one, else the catalog default.
func (a *App) ProviderBaseURL(providerName string) string {
	return resolveBaseURLForProvider(model.NormalizeProvider(providerName))
}

// ProviderBaseURLIsCustom says whether ProviderBaseURL is a user override
// rather than the catalog default — the UI needs it to offer a reset.
func (a *App) ProviderBaseURLIsCustom(providerName string) bool {
	canonical := model.NormalizeProvider(providerName)
	return resolveBaseURLForProvider(canonical) != model.DefaultBaseURL(canonical)
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
	baseURL := resolveBaseURLForProvider(canonical)
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
	return a.modelSwitchResult()
}

// HasAPIKey reports whether a key-requiring provider already has resolvable
// credentials — a cached key, an env var, or a sign-in. Always true for
// providers that don't need any.
func (a *App) HasAPIKey(providerName string) bool {
	canonical := model.NormalizeProvider(providerName)
	if !model.RequiresAPIKey(canonical) {
		return true
	}
	// A signed-in provider has no key to find and never will — asking the user
	// for one would be asking for something that does not exist.
	if oauth.Has(canonical) {
		return true
	}
	return resolveAPIKeyForProvider(canonical) != ""
}

// ProviderReady answers the question the sidebar dot is actually asking: can
// this provider be used right now?
//
// HasAPIKey was standing in for it and is not the same question. It returns
// true for anything that needs no key, which is every local runtime — so LM
// Studio and Ollama showed a green dot whether or not a server was listening,
// on a page that said "no models found" two inches to the right.
//
// What "ready" can honestly mean differs by kind, and the split is about what
// can be checked for free:
//
//   - A keyed provider: a key is set, or a sign-in exists. Whether that key
//     still works is only knowable by spending a request against it, and a
//     settings page that bills the user for opening it is worse than one that
//     says "configured".
//   - A local runtime: the server is answering. This costs a connection to
//     localhost, which is free, so there is no excuse for guessing — and it is
//     precisely the case that was lying.
//   - Aetox's own engine: always, it is built in.
func (a *App) ProviderReady(providerName string) bool {
	canonical := model.NormalizeProvider(providerName)
	if model.RequiresAPIKey(canonical) {
		return a.HasAPIKey(canonical)
	}
	if canonical == "aetox" {
		return true
	}
	// The same judgement the rest of the app makes about a local runtime — can
	// a model be got out of it — rather than a second definition of "up".
	return model.ResolveDefaultModel(canonical, resolveBaseURLForProvider(canonical), "") != ""
}

// RequiresAPIKey exposes model.RequiresAPIKey to the frontend.
func (a *App) RequiresAPIKey(providerName string) bool {
	return model.RequiresAPIKey(model.NormalizeProvider(providerName))
}

// AcceptsAPIKey says whether the provider's card should offer a key field at
// all. Codex is the one that must not: it is reached at chatgpt.com on a
// subscription, so the key a user would paste there is an api.openai.com key
// that answers 401 — a field whose only possible outcome is a failed login.
func (a *App) AcceptsAPIKey(providerName string) bool {
	return model.AcceptsAPIKey(model.NormalizeProvider(providerName))
}

// SetProviderBaseURL persists a custom endpoint for a provider and, if it's
// the active one, re-bootstraps onto it. An empty baseURL clears the override
// and returns the provider to its catalog default. Reported by a user running
// LM Studio's server on a port other than 1234, which the read-only field this
// replaces made unreachable — the catalog default was the only address Aetox
// would ever call.
func (a *App) SetProviderBaseURL(providerName, baseURL string) (ModelInfo, error) {
	canonical := model.NormalizeProvider(providerName)
	if _, ok := model.LookupProviderInfo(canonical); !ok {
		return ModelInfo{}, fmt.Errorf("unknown provider: %q", providerName)
	}
	trimmed := strings.TrimSpace(baseURL)
	// This value becomes an outbound request target, so it is validated here
	// rather than trusted: a bare "localhost:1234" or a "file://" would
	// otherwise be saved and fail later as something that reads like a
	// provider outage.
	if trimmed != "" {
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return ModelInfo{}, fmt.Errorf("base URL must be a full http:// or https:// address, got %q", baseURL)
		}
		trimmed = strings.TrimRight(trimmed, "/")
	}

	pref, ok, _ := config.LoadModelPreference()
	if !ok {
		pref = config.ModelPreference{}
	}
	pref.SetBaseURLForProvider(canonical, trimmed)
	if strings.EqualFold(a.cfg.ModelProvider, canonical) {
		// The legacy single slot is what resolveConfig reads first; leaving a
		// stale value there would fight the change on the next launch.
		pref.ModelBaseURL = trimmed
	}
	if err := config.SaveModelPreference(pref); err != nil {
		return ModelInfo{}, err
	}

	if strings.EqualFold(a.cfg.ModelProvider, canonical) {
		next := a.cfg
		next.ModelBaseURL = resolveBaseURLForProvider(canonical)
		// The model name came from the old endpoint's discovery, so it is a
		// guess about a server we have not spoken to yet — re-resolve it.
		next.ModelName = model.ResolveDefaultModel(canonical, next.ModelBaseURL, next.ModelAPIKey)
		next.ThinkLevel = model.NormalizeThinkingLevel(canonical, next.ModelName, next.ThinkLevel)
		a.applyConfig(next)
	}
	return a.modelSwitchResult()
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
	}
	return a.modelSwitchResult()
}

// resolveBaseURLForProvider is the one place that answers "where do we call
// this provider?" — the saved override, else the catalog default. Every
// discovery, connection test, and switch goes through it, so a custom endpoint
// cannot be honored on one path and ignored on another.
func resolveBaseURLForProvider(canonicalProvider string) string {
	if pref, ok, _ := config.LoadModelPreference(); ok {
		if v := pref.BaseURLForProvider(canonicalProvider); v != "" {
			return v
		}
	}
	return model.DefaultBaseURL(canonicalProvider)
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

// RetryActiveProvider re-bootstraps when the engine is sitting on the aetox
// fallback, so a provider that has come up since the switch is picked up
// without the user re-selecting it by hand. No-op when the last bootstrap
// succeeded.
//
// ModelInfo.Warning is a snapshot of switch time. Starting LM Studio's server
// a minute later left that snapshot nagging in red — next to a model list that
// had just been discovered from the very endpoint it called unreachable — and
// the engine really was still on the fallback, so hiding the banner alone
// would have been the lie. This un-sticks the engine, and the warning clears
// because it stopped being true.
func (a *App) RetryActiveProvider() ModelInfo {
	if a.modelErr == nil {
		return a.GetModelInfo()
	}
	next := a.cfg
	next.ModelBaseURL = resolveBaseURLForProvider(next.ModelProvider)
	next.ModelAPIKey = resolveAPIKeyForProvider(next.ModelProvider)
	// A failed bootstrap on a local runtime leaves the name empty (the server
	// had nothing to offer), and that empty name is what fails again.
	if strings.TrimSpace(next.ModelName) == "" {
		next.ModelName = model.ResolveDefaultModel(next.ModelProvider, next.ModelBaseURL, next.ModelAPIKey)
		next.ThinkLevel = model.NormalizeThinkingLevel(next.ModelProvider, next.ModelName, next.ThinkLevel)
	}
	a.applyConfig(next)
	return a.GetModelInfo()
}

// SwitchProvider re-bootstraps the engine on a different provider, using its default model.
func (a *App) SwitchProvider(provider string) (ModelInfo, error) {
	next := a.cfg
	next.ModelProvider = model.NormalizeProvider(provider)
	next.ModelBaseURL = resolveBaseURLForProvider(next.ModelProvider)
	next.ModelWireFormat = "" // reset to the new provider's default format
	next.ModelAPIKey = resolveAPIKeyForProvider(next.ModelProvider)
	next.ModelName = model.ResolveDefaultModel(next.ModelProvider, next.ModelBaseURL, next.ModelAPIKey)
	next.ThinkLevel = model.NormalizeThinkingLevel(next.ModelProvider, next.ModelName, "")
	a.applyConfig(next)
	return a.modelSwitchResult()
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
	return a.modelSwitchResult()
}

// SwitchThinkLevel changes the reasoning depth for the current provider/model.
func (a *App) SwitchThinkLevel(level string) (ModelInfo, error) {
	next := a.cfg
	next.ThinkLevel = model.NormalizeThinkingLevel(next.ModelProvider, next.ModelName, level)
	a.applyConfig(next)
	return a.modelSwitchResult()
}

// SwitchApprovalMode changes the safety approval mode the engine runs with.
//
// It used to go through applyConfig, which rebuilds the whole engine — new
// agent, new registry, new executor. The turn already running kept the old
// executor, so switching to full access mid-turn did nothing until the next
// turn: exactly when the user needs it, because what makes anyone reach for
// that dropdown is a prompt sitting on screen right now. Nothing about a mode
// change needs a new engine, so it no longer gets one.
func (a *App) SwitchApprovalMode(mode string) (ModelInfo, error) {
	normalized := safety.NormalizeApprovalMode(mode)
	a.cfg.ApprovalMode = string(normalized)
	if a.chat != nil {
		a.chat.SetApprovalMode(normalized)
	}
	persistModelPreference(a.cfg)
	return a.modelSwitchResult()
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
	} else {
		next := a.cfg
		next.SandboxRoot = config.Load(opts).SandboxRoot
		a.applyConfig(next)
	}
	go a.sweepAttachments(a.cfg.SandboxRoot)
}

// deskTools is the installed registry seen through the open session's desk —
// the same view the engine's own dispatcher was built with. Rebuilt per call
// rather than held, because it is cheap and because a held one would be a
// second thing to swap on every re-bootstrap.
//
// A chair session reports the chair's cut (profile ∩ office ceiling), built by
// the same AttendedRegistry the engine mounted — what the tools panel shows and
// what the model can call must be one list, not two readings of the rules. It
// was FilterRegistry here and that is exactly the drift the sentence above
// warns about: the engine grew `ask_user` for a chair chat and this line, four
// files away, kept reporting the list without it.
// The stance is folded in here too, and it is the same sentence one axis
// later: the panel and the model must agree about what this session carries,
// and a stance changes that without opening a new session. Left out, คู่คิด
// would empty the model's tool block while this list went on showing every
// tool the desk holds — the drift above, arriving through the newer door.
func (a *App) deskTools() *skill.Dispatcher {
	if p := a.chairProfile(); p != nil {
		return skill.NewDispatcherFor(subagent.AttendedRegistry(a.registry, *p, a.desk), a.stance.Carries)
	}
	return skill.NewDispatcherFor(a.registry, func(name string, source skill.Source) bool {
		return a.desk.Carries(name, source) && a.stance.Carries(name, source)
	})
}

// chairProfile resolves the open session's chair to its profile, nil when the
// session talks to the main assistant. Read from disk per call — profiles are
// files, and a held copy would be a second truth that survives an edit.
//
// A name that no longer resolves, or resolves to a profile that has stopped
// declaring the office as its desk, answers nil here; the doors that *open*
// chair sessions (NewChairSession, LoadSession) refuse those cases loudly
// first, so this quiet nil is only ever a race with a file deleted mid-session
// — and falling back to the main assistant's desk view is the readable answer
// for a tools panel that must render something.
func (a *App) chairProfile() *subagent.Profile {
	if a.chair == "" {
		return nil
	}
	p, ok := subagent.Load(a.chair)
	if !ok || p.Desk != mode.Office {
		return nil
	}
	return &p
}

// workbenchSkills are the tools only the desktop app can offer — they need a
// window, or a human, to mean anything. Everything else the agent gets comes
// from skill.NewDefaultRegistry.
//
// A function rather than a literal inside applyConfig because the tool coverage
// test has to assemble the exact same set: a list copied into a test is a list
// that silently stops matching the day someone adds a tool here.
func (a *App) workbenchSkills() []skill.Skill {
	return []skill.Skill{
		// One tool for the browser, four actions inside it (browser_tool.go).
		// The four old names are still what `tools:` and `categories:` speak —
		// they moved from being tools to being the actions' permission keys.
		&browserSkill{app: a},
		&deskOpenSkill{app: a},
		&deskTerminalSkill{app: a},
		&deskListSkill{app: a},
		&askUserSkill{app: a},
		&todoWriteSkill{app: a},
		&sessionSearchSkill{app: a},
		&suggestTaskSkill{app: a},
		// The engines' power switches. Registered for every session like the
		// rest of the workbench, and cut like a connection tool: each is owned
		// by its vendor's catalog entry, so the placement lock (HomeAgent)
		// delivers it to the automation agent holding that engine and to no
		// desk — same journey as n8n_workflow_create, one file over.
		&engineServerSkill{app: a, id: "n8n"},
		&engineServerSkill{app: a, id: "windmill"},
		// The main agent's own scope. A delegate does not inherit this one —
		// `task` builds it a replacement bound to its profile (internal/subagent),
		// so what a sub-agent learns never lands in this prompt.
		//
		// Desk is the second scope this one tool can write to, and it is empty
		// unless the session was opened at a desk: a fact about the user belongs
		// in the file every desk reads, while a lesson about this kind of work
		// belongs where only this kind of work pays for it (§83).
		&learned.MemoryTool{
			Scope:    learned.MainScope,
			Desk:     a.desk.DeskName(),
			Proposer: appProposer{app: a},
		},
	}
}

// applyConfig re-bootstraps the engine from an already-resolved config, then
// persists the model/approval choice so the CLI and desktop app share one preference.
func (a *App) applyConfig(cfg config.Config) {
	// Rebuilt with the engine, because the work tree it watches is the sandbox
	// root and that is exactly what a re-bootstrap can change. An error here is
	// the ordinary "this folder is not a repository" case — no undo, no fuss,
	// and every caller of a.snapshots is written for nil.
	a.snapshotMu.Lock()
	if store, err := snapshot.New(cfg.SandboxRoot); err == nil {
		a.snapshots = store
	} else {
		a.snapshots = nil
	}
	a.lastSnapshot = "" // a snapshot of the previous project is not an undo for this one
	a.snapshotMu.Unlock()

	workbenchTools := a.workbenchSkills()
	if a.mcp == nil {
		servers, err := config.LoadMCPServers()
		if err != nil {
			debuglog.Msg("mcp: load servers: %v", err)
		}
		a.mcp = mcp.NewManager(bootstrap.MCPServers(servers))
	}
	// Capture the outgoing agent's real context before it's replaced: it holds
	// what the text transcript doesn't — tool calls, tool results, compaction
	// summaries — so a model switch keeps the model's working memory intact
	// (OpenCode/Claude Code keep tool history across switches too).
	var priorContext []model.Message
	if a.agent != nil {
		priorContext = a.agent.ContextMessages()
	}
	// Named fields, not positional: four of these callbacks are func(string) or
	// func(), and two of them — the status reporter and the answer preview —
	// used to sit next to each other in the argument list. Swapping that pair
	// compiles cleanly and shows up only as the UI drawing the wrong text.
	res, bootErr := bootstrap.Engine(cfg, bootstrap.Options{
		Surface: prompt.SurfaceDesktop,
		Console: aetoxapp.NewStdIO(),
		// The desk the open session was created at. Nil — the full desk — until
		// a session says otherwise, which is what every session before §83 and
		// every unfiltered path still gets.
		Mode: a.desk,
		// How this turn runs (§106). Read here rather than snapshotted at
		// session open because SetStance re-bootstraps through this same
		// function — the dial and the engine cannot disagree if there is only
		// one place the engine reads it.
		Stance: a.stance,
		// The agent the open session talks to directly (§85), nil for the main
		// assistant. Resolved fresh from disk on every bootstrap so an edited
		// profile takes effect the next time its chair is sat at, like every
		// other manifest.
		Chair:        a.chairProfile(),
		Approve:      a.approveToolCall,
		Manager:      a.mcp,
		ExtraSkills:  workbenchTools,
		OutputSubdir: a.outputSubdir,
		// The session's whole reach, in two fields and no third: unfocused mode
		// roams the machine (minus credential stores), and a focused project
		// sees itself plus the folders the user added to it. See unfocusedRoot
		// and desktop/workspace.go.
		OpenSandbox: !a.projectFocused,
		ExtraRoots:  a.workspaceRoots(),
		// The door in that wall: a path outside the workspace asks to add the
		// folder it lives in rather than ending the work (workspace.go).
		AskWorkspace: a.askWorkspaceWiden,
		// The project this chat is being held inside, and the names of the
		// files it keeps — read fresh on every bootstrap, like every other
		// manifest, so a file dropped into the folder is known to the next
		// session without restarting anything.
		Space:        a.space,
		SpaceContext: a.spaceContextForPrompt(),
		// Which shell the agent's commands and the user's hooks run in. Read
		// per call so the composer's picker takes effect on the next command
		// rather than on the next restart.
		Shell:            a.shellBackend,
		OnToolAction:     a.recordToolAction,
		OnToolRun:        a.recordToolRun,
		Proposer:         appProposer{app: a},
		OnStatus:         a.emitAgentStatus,
		OnContentPreview: a.previewAnswer,
		OnContentReset:   a.discardAnswerPreview,
		OnUsage:          a.recordTokenUsage,
	})
	if bootErr == nil {
		// Fallback outlives a successful boot: the engine is up, but on the
		// built-in provider rather than the one asked for, and only this says why.
		bootErr = res.Fallback
	}
	agent, registry := res.Agent, res.Registry
	a.chat = res.App
	a.agent = agent
	a.cfg = cfg
	a.modelStatus = res.Status
	a.modelErr = bootErr
	a.registry = registry
	// A re-bootstrap builds a fresh register, so whatever the old engine still
	// had running is about to become unreachable — stop it rather than leave it
	// burning tokens for an engine nobody can collect from any more.
	if a.delegations != nil && a.delegations != res.Delegations {
		a.delegations.StopAll()
	}
	a.delegations = res.Delegations
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
				a.emitEvent("skills:updated", nil)
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
		if v := strings.TrimSpace(pref.SpeechModelPath); v != "" {
			cfg.SpeechModelPath = v
		}
		if key := pref.APIKeyForProvider(cfg.ModelProvider); key != "" {
			cfg.ModelAPIKey = key
		}
		// After pref.ModelBaseURL above, not before: the per-provider entry is
		// the one the user set for *this* provider, the legacy slot is whatever
		// provider happened to be active when it was written.
		if v := pref.BaseURLForProvider(cfg.ModelProvider); v != "" {
			cfg.ModelBaseURL = v
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
		baseURL = "" // matches the catalog — store nothing, so a later catalog change lands
	}
	pref.ModelBaseURL = baseURL
	pref.SetBaseURLForProvider(canonicalProvider, baseURL)
	pref.ModelWireFormat = strings.TrimSpace(cfg.ModelWireFormat)
	pref.ThinkLevel = model.NormalizeThinkingLevel(canonicalProvider, pref.ModelName, cfg.ThinkLevel)
	pref.ApprovalMode = string(safety.NormalizeApprovalMode(cfg.ApprovalMode))
	// Only overwrite when we actually have one: a model change must not wipe a
	// language the user already picked.
	if v := strings.TrimSpace(cfg.UILocale); v != "" {
		pref.UILocale = v
	}
	// Unlike the language, an empty value here is a real choice — it is how
	// "go back to picking whatever is on disk" is expressed — so it is written
	// through rather than treated as "nothing to say".
	pref.SpeechModelPath = strings.TrimSpace(cfg.SpeechModelPath)
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
