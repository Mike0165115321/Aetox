package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// A focused project works in its own folder — that is what focusing means, and
// the guarantee is worth keeping. But a problem in one project regularly starts
// in another: a shared library, a generated client, a config that lives
// somewhere else. Before this, the honest answer to "go look at what the API
// repo actually returns" was to unfocus the project, which trades one folder
// for the whole machine — an absurd swap for wanting to read one directory.
//
// So the workspace is the project folder plus a list the user keeps. The list
// is the entire mechanism: nothing infers a folder from the project's imports,
// nothing remembers a folder "temporarily", nothing here is enabled by a
// heuristic. What the user can see in the panel is exactly what the tools can
// reach, and a folder that is not on the list is not reachable by any route the
// app knows — which is the only version of this that stays true a year from now
// when nobody remembers the rules.

// WorkspaceFolder is one entry as the UI shows it.
type WorkspaceFolder struct {
	Path string `json:"path"`
	Name string `json:"name"`
	// Missing marks a folder that is no longer on disk. It stays on the list
	// (a detached drive should not silently drop the user's choice) but the UI
	// has to say so, or the folder looks reachable when nothing in it is.
	Missing bool `json:"missing"`
}

// WorkspaceFolders lists the folders added to the focused project.
func (a *App) WorkspaceFolders() []WorkspaceFolder {
	out := []WorkspaceFolder{} // never nil: §34, a nil slice crashes the frontend
	for _, path := range a.workspaceRoots() {
		info, err := os.Stat(path)
		out = append(out, WorkspaceFolder{
			Path:    path,
			Name:    filepath.Base(filepath.Clean(path)),
			Missing: err != nil || !info.IsDir(),
		})
	}
	return out
}

// AddWorkspaceFolder asks the user for a folder and gives it the same rights
// the project folder has, for this project, until they remove it.
func (a *App) AddWorkspaceFolder() ([]WorkspaceFolder, error) {
	if !a.projectFocused {
		// Not a wall to work around — with no project focused the tools already
		// reach the whole machine, so there is nothing this list could add.
		return a.WorkspaceFolders(), fmt.Errorf("ไม่มีโปรเจกต์ที่โฟกัสอยู่ — โต๊ะนี้เข้าถึงทั้งเครื่องอยู่แล้ว")
	}
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "เพิ่มโฟลเดอร์เข้าโปรเจกต์นี้",
	})
	if err != nil {
		return a.WorkspaceFolders(), err
	}
	if strings.TrimSpace(dir) == "" {
		return a.WorkspaceFolders(), nil // cancelled
	}
	return a.addWorkspaceFolder(dir)
}

// addWorkspaceFolder is AddWorkspaceFolder minus the dialog, so the rules below
// are reachable from a test without a window.
func (a *App) addWorkspaceFolder(dir string) ([]WorkspaceFolder, error) {
	if err := a.recordWorkspaceFolder(dir); err != nil {
		return a.WorkspaceFolders(), err
	}
	a.reloadWorkspace()
	return a.WorkspaceFolders(), nil
}

// recordWorkspaceFolder checks one folder against every rule and, if it passes,
// writes it to the project's list and refreshes the copy this process holds.
//
// Split out from addWorkspaceFolder because the folder now arrives by two
// routes — the panel, and the card the workspace door raises mid-tool-call
// (askWorkspaceWiden) — and those two differ in exactly one thing: whether the
// engine can be rebuilt right now. The rules must not differ at all, so they
// live here and nowhere else.
func (a *App) recordWorkspaceFolder(dir string) error {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	dir, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("ไม่ใช่โฟลเดอร์ที่เปิดได้: %s", dir)
	}
	// Refusing here rather than at the first tool call: a folder accepted onto
	// the list and then refused file by file is the app disagreeing with itself
	// in front of the user.
	if store := skill.CredentialStoreAt(dir); store != "" {
		return fmt.Errorf("โฟลเดอร์นี้อยู่ในที่เก็บกุญแจ (%s) ซึ่งปิดตายทุกโต๊ะ", store)
	}
	// Saying "already reachable" beats accepting a row that changes nothing —
	// a list with a no-op entry on it stops describing what it grants.
	if withinFolder(dir, root) {
		return fmt.Errorf("โฟลเดอร์นี้อยู่ในโปรเจกต์อยู่แล้ว เข้าถึงได้โดยไม่ต้องเพิ่ม")
	}
	for _, existing := range a.workspaceRoots() {
		if withinFolder(dir, existing) {
			return fmt.Errorf("อยู่ในโฟลเดอร์ที่เพิ่มไว้แล้ว: %s", existing)
		}
	}

	// Persist first, then read back: the engine is rebuilt from the stored list,
	// so a write that failed must not produce a session that can reach a folder
	// no restart would grant.
	if err := a.storeWorkspaceFolder(root, dir); err != nil {
		return err
	}
	a.setWorkspaceRoots(a.storedWorkspaceFolders(root))
	return nil
}

// RemoveWorkspaceFolder drops a folder and narrows the running session in the
// same call — a folder removed in the panel while the agent is mid-conversation
// has to be out of reach before the next tool call, not after a restart.
func (a *App) RemoveWorkspaceFolder(path string) ([]WorkspaceFolder, error) {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	path = strings.TrimSpace(path)
	if path == "" {
		return a.WorkspaceFolders(), fmt.Errorf("empty folder path")
	}
	db, err := a.database()
	if err != nil {
		return a.WorkspaceFolders(), err
	}
	if _, err := db.Exec(
		`DELETE FROM project_folders WHERE project_key = ? AND path = ?`,
		projectKey(root), path,
	); err != nil {
		return a.WorkspaceFolders(), err
	}
	a.setWorkspaceRoots(a.storedWorkspaceFolders(root))
	a.reloadWorkspace()
	return a.WorkspaceFolders(), nil
}

// workspaceRoots / setWorkspaceRoots are the only readers and writers of the
// folder list this process holds.
//
// They exist because the list stopped being touched by one goroutine. It used
// to change only when the user clicked something, which is the Wails binding
// goroutine; the workspace door adds a folder from inside a tool call, which is
// the turn goroutine, while a binding call can be reading the same slice to draw
// the panel.
func (a *App) workspaceRoots() []string {
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	return append([]string(nil), a.extraRoots...)
}

func (a *App) setWorkspaceRoots(roots []string) {
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	a.extraRoots = roots
}

// widenAdd / widenRefuse are the exact option strings the workspace card sends
// to the UI and compares the answer against. Anything else — including free
// text — reads as "no", like every other gate in this app.
const (
	widenAdd     = "เพิ่มเข้าโปรเจกต์ / Add folder"
	widenRefuse  = "ไม่ / No"
	widenSkipped = "" // no folder worth offering; see folderToOffer
)

// askWorkspaceWiden is the desktop's skill.WidenFunc: the door in the wall.
//
// A path outside the project used to be the end of the work. The agent knew the
// folder it needed and said it could not have it, and the user went to fetch the
// same folder by hand through a menu and an OS dialog, then asked again. This
// asks instead — once, naming the folder — and on a yes the folder joins the
// project's own list, where it stays visible and removable like every other
// entry. There is deliberately no "allow once": a right that is not on the list
// is a right the panel has stopped describing.
//
// What it does NOT decide is whether the path is then allowed. It adds a folder;
// resolveSandboxPath re-reads the list and rules on the path itself, and the
// credential stores are refused after that regardless of the answer here.
func (a *App) askWorkspaceWiden(conv *conversation, target string) bool {
	// With no project focused there is no wall, so nothing ever asks — but the
	// check is cheap and the alternative is a card offering to widen a workspace
	// that is already the whole machine.
	if !a.projectFocused {
		return false
	}
	if strings.TrimSpace(a.cfg.SandboxRoot) == "" {
		return false
	}
	// No window, no question — the same test emitEvent makes before it reaches
	// the Wails runtime. No emitter and no runtime context means nothing is
	// listening: a headless run, and every test that builds an engine without a
	// frontend. Asking there parks the tool call on an answer that can never
	// arrive, so the flat refusal stands, exactly as it did before this door.
	if a.emit == nil && a.ctx == nil {
		return false
	}
	folder := folderToOffer(target)
	if folder == widenSkipped {
		return false
	}
	// Asked before the card, not after: a question whose yes cannot be honoured
	// trains the user to distrust the question. recordWorkspaceFolder refuses
	// these too — this is the same rule, stated early enough to stay silent.
	if skill.CredentialStoreAt(folder) != "" {
		return false
	}

	// The turn's context, so a card nobody answers cannot outlive the turn it
	// belongs to. Without it Stop would appear dead until somebody clicked the
	// card, because the tool call is parked inside resolveSandboxPath.
	a.turnMu.Lock()
	var turnCtx context.Context
	if live := a.turns[a.cur().id]; live != nil {
		turnCtx = live.ctx
	}
	a.turnMu.Unlock()
	if turnCtx == nil {
		// No turn in flight — a tool running outside one. The app's own context
		// still closes when the window does, which is the only cancellation left
		// to respect.
		turnCtx = a.ctx
	}
	if turnCtx == nil {
		turnCtx = context.Background()
	}

	question := fmt.Sprintf(
		"งานนี้ต้องใช้ `%s` ซึ่งอยู่นอกโปรเจกต์\n\nเพิ่มโฟลเดอร์นี้เข้าโปรเจกต์ไหม (อ่านและแก้ไขได้ เอาออกได้ทีหลังในเมนูโปรเจกต์)",
		folder)
	ch, err := a.beginUserQuestion(conv, question, []string{widenAdd, widenRefuse})
	if err != nil {
		// Another question already has the floor — two cards competing for one
		// answer channel is worse than a refusal the model can report and retry.
		return false
	}
	defer a.endUserQuestion(conv)

	select {
	case answer := <-ch:
		if answer != widenAdd {
			return false
		}
	case <-turnCtx.Done():
		return false
	}

	if err := a.recordWorkspaceFolder(folder); err != nil {
		// The rules refused what the user just accepted — a folder deleted
		// between the card and the click, or a store that would not write. The
		// path stays refused and the tool error carries the reason into the
		// transcript, which is where the user is already looking.
		return false
	}
	// Reaches the call that is parked on this answer without rebuilding the
	// engine underneath it. The engine picks the same list up when the turn ends
	// (endTurn → reloadWorkspace), which is what puts the new folder in the
	// system prompt; both read the list this call just wrote.
	skill.SetWorkspaceFolders(a.cfg.SandboxRoot, a.workspaceRoots())
	a.markWorkspaceDirty()
	a.emitEvent("workspace:changed", a.WorkspaceFolders())
	return true
}

// folderToOffer turns the refused path into the folder worth putting on the
// list: the path itself when it is a directory, otherwise the directory holding
// it. A file's own name on the list would grant one file and refuse its
// siblings, which is not how anyone thinks about "let it see the API repo".
//
// A volume root ("D:\", "/") is refused rather than offered. It is what
// filepath.Dir answers for a loose file at the top of a drive, and one click
// would hand over every project on that drive — a grant nobody means to make
// from a card about one file.
func folderToOffer(target string) string {
	target = strings.TrimSpace(target)
	if target == "" || !filepath.IsAbs(target) {
		return widenSkipped
	}
	folder := filepath.Clean(target)
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		folder = filepath.Dir(folder)
	}
	if folder == filepath.Dir(folder) {
		return widenSkipped
	}
	return folder
}

// markWorkspaceDirty records that the folder list changed under a running turn,
// so endTurn rebuilds the engine once the turn is out of the way.
func (a *App) markWorkspaceDirty() {
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	a.workspaceDirty = true
}

// takeWorkspaceDirty clears the flag and reports whether a reload is owed.
func (a *App) takeWorkspaceDirty() bool {
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	dirty := a.workspaceDirty
	a.workspaceDirty = false
	return dirty
}

// reloadWorkspace re-bootstraps the engine so the sandbox gate and the system
// prompt both pick up the new list. The conversation survives it (applyConfig
// carries the outgoing agent's context across), which is the point: the user
// adds the folder because of the question they are already asking.
func (a *App) reloadWorkspace() {
	a.reload(config.ConfigOptions{
		RootPath:     a.cfg.SandboxRoot,
		ApprovalMode: a.cfg.ApprovalMode,
	})
}

// storeWorkspaceFolder records one folder for one project.
func (a *App) storeWorkspaceFolder(root, dir string) error {
	db, err := a.database()
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO project_folders(project_key, path, added_at)
		VALUES(?,?,?)
		ON CONFLICT(project_key, path) DO NOTHING`,
		projectKey(root), dir, time.Now().Format(time.RFC3339))
	return err
}

// storedWorkspaceFolders reads a project's folder list. A database that cannot
// be opened yields an empty list rather than an error: failing closed here
// means a session confined to its project, which is the safe direction and the
// one the user can see (the panel shows nothing) rather than guess at.
func (a *App) storedWorkspaceFolders(root string) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	db, err := a.database()
	if err != nil {
		return nil
	}
	var out []string
	_ = eachRow(db, "workspace folders",
		`SELECT path FROM project_folders WHERE project_key = ? ORDER BY added_at`,
		[]any{projectKey(root)},
		func(rows *sql.Rows) error {
			var path string
			if err := rows.Scan(&path); err != nil {
				return err
			}
			if strings.TrimSpace(path) != "" {
				out = append(out, path)
			}
			return nil
		})
	// Left in the order they were added, which is the order the user watched
	// the list grow in — and stable across restarts, so the system prompt built
	// from it does not churn between sessions.
	return out
}

// withinFolder reports whether target sits at or under folder, on the same
// case rule the sandbox gate uses (NTFS is case-insensitive, so D:\Lib and
// d:\lib are one folder).
func withinFolder(target, folder string) bool {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return false
	}
	target, folder = filepath.Clean(target), filepath.Clean(folder)
	if runtime.GOOS == "windows" {
		target, folder = strings.ToLower(target), strings.ToLower(folder)
	}
	rel, err := filepath.Rel(folder, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
