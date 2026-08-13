package main

import (
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
	for _, path := range a.extraRoots {
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
	root := strings.TrimSpace(a.cfg.SandboxRoot)
	dir, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return a.WorkspaceFolders(), err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return a.WorkspaceFolders(), fmt.Errorf("ไม่ใช่โฟลเดอร์ที่เปิดได้: %s", dir)
	}
	// Refusing here rather than at the first tool call: a folder accepted onto
	// the list and then refused file by file is the app disagreeing with itself
	// in front of the user.
	if store := skill.CredentialStoreAt(dir); store != "" {
		return a.WorkspaceFolders(), fmt.Errorf("โฟลเดอร์นี้อยู่ในที่เก็บกุญแจ (%s) ซึ่งปิดตายทุกโต๊ะ", store)
	}
	// Saying "already reachable" beats accepting a row that changes nothing —
	// a list with a no-op entry on it stops describing what it grants.
	if withinFolder(dir, root) {
		return a.WorkspaceFolders(), fmt.Errorf("โฟลเดอร์นี้อยู่ในโปรเจกต์อยู่แล้ว เข้าถึงได้โดยไม่ต้องเพิ่ม")
	}
	for _, existing := range a.extraRoots {
		if withinFolder(dir, existing) {
			return a.WorkspaceFolders(), fmt.Errorf("อยู่ในโฟลเดอร์ที่เพิ่มไว้แล้ว: %s", existing)
		}
	}

	// Persist first, then reload: the engine is rebuilt from the stored list, so
	// a write that failed must not produce a session that can reach a folder no
	// restart would grant.
	if err := a.storeWorkspaceFolder(root, dir); err != nil {
		return a.WorkspaceFolders(), err
	}
	a.extraRoots = a.storedWorkspaceFolders(root)
	a.reloadWorkspace()
	return a.WorkspaceFolders(), nil
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
	a.extraRoots = a.storedWorkspaceFolders(root)
	a.reloadWorkspace()
	return a.WorkspaceFolders(), nil
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
	rows, err := db.Query(
		`SELECT path FROM project_folders WHERE project_key = ? ORDER BY added_at`,
		projectKey(root),
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var path string
		if rows.Scan(&path) == nil && strings.TrimSpace(path) != "" {
			out = append(out, path)
		}
	}
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
