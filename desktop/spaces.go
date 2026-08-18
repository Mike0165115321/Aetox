package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Mike0165115321/Aetox/internal/bootstrap"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/mode"
)

// A โปรเจกต์ at the storefront door: a named folder that groups chats and
// carries a few files into every session held inside it (COMPANY.md §84).
//
// **`space` in code, โปรเจกต์ on screen, and the two words are not the same
// word by accident.** The workshop door already has a โปรเจกต์: a folder on the
// user's disk that the sandbox is rooted in (`projectKey`, the `projects`
// table, `App.projectFocused`). That one is a fence — rooting the engine in it
// is the entire point. This one is the opposite: it is a folder for
// conversations and it moves no wall, the assistant still reaches the whole
// machine. Two meanings needed two identifiers, and taking the free one for the
// new concept beats renaming a shipped schema. The mapping is written down in
// COMPANY.md §8 so the second word stays a translation instead of becoming a
// drift.
//
// **The folder is the truth.** A space exists because `<DataRoot>/project/<name>`
// exists — there is no table of spaces to fall out of step with the disk, the
// same rule ผลงาน already follows (§84). A user who makes the folder by hand has
// made a project; one who deletes it has deleted one, and nothing here has to be
// told. The only thing the database holds is which space a session was held in,
// which is a fact about the session and lives on the session's own row.
type Space struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	ContextPath  string   `json:"contextPath"`
	ContextFiles []string `json:"contextFiles"`
	Chats        int      `json:"chats"`
	UpdatedAt    string   `json:"updatedAt"`
}

// contextDirName is the folder inside a space that holds what every session in
// it should start knowing. Its own folder rather than loose files at the top,
// so a space can grow other folders later without the context becoming "the
// files that happen to not be in a subfolder".
const contextDirName = "context"

// spacesRoot is <DataRoot>/project — the owner named this path, and it is
// deliberately beside modes/, agents/ and subagents/ rather than inside the
// database: everything the user is meant to be able to open, edit and back up
// by hand lives as files under DataRoot (ARCHITECTURE.md §14).
func spacesRoot() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "project"), nil
}

// spaceFolderName is the name the user typed, once it has been checked that a
// folder can be called that.
//
// It **refuses** rather than repairs. Stripping the characters a filesystem
// cannot hold is the tempting version and it is wrong twice over: "a/b" would
// become a folder called "ab" that the user never named and cannot find by the
// name on their screen, and "../escape" would become "..escape", a name that
// looks like it did something and did not. The promise of putting projects on
// disk is that the folder is the one they named — so a name that cannot be a
// folder is an error with the reason in it, not a name quietly turned into a
// different one.
//
// Surrounding whitespace is the exception, and only because it is invisible: a
// user who typed a trailing space did not name their project that.
func spaceFolderName(name string) (string, error) {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" {
		return "", fmt.Errorf("ตั้งชื่อโปรเจกต์ก่อนนะครับ")
	}
	if len([]rune(cleaned)) > 64 {
		return "", fmt.Errorf("ชื่อโปรเจกต์ยาวเกินไป (ไม่เกิน 64 ตัวอักษร)")
	}
	if cleaned == "." || cleaned == ".." {
		return "", fmt.Errorf("ชื่อนี้ใช้เป็นชื่อโฟลเดอร์ไม่ได้")
	}
	for _, r := range cleaned {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("ชื่อโปรเจกต์มีอักขระที่ใช้เป็นชื่อโฟลเดอร์ไม่ได้")
		}
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return "", fmt.Errorf("ชื่อโปรเจกต์ใช้ %q ไม่ได้ เพราะมันเป็นชื่อโฟลเดอร์จริงบนเครื่อง", string(r))
		}
	}
	// Windows stores neither, and stores them by silently dropping the
	// character — the folder would exist under a name that is not the one on
	// screen, which is the failure this whole function is about.
	if strings.HasSuffix(cleaned, ".") {
		return "", fmt.Errorf("ชื่อโปรเจกต์ลงท้ายด้วยจุดไม่ได้")
	}
	// CON, PRN, NUL and friends are devices on Windows, not names. Creating one
	// fails in a way that reads like a bug in Aetox rather than a name that
	// cannot be used.
	base := strings.ToUpper(cleaned)
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "", fmt.Errorf("%q เป็นชื่อที่ Windows จองไว้ ตั้งเป็นชื่อโฟลเดอร์ไม่ได้", cleaned)
	}
	return cleaned, nil
}

// spacePath resolves a space's folder and refuses anything that is not directly
// inside the spaces root. The name arrives from the frontend, so this is the
// gate, not the caller.
func spacePath(name string) (string, error) {
	folder, err := spaceFolderName(name)
	if err != nil {
		return "", err
	}
	root, err := spacesRoot()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, folder)
	if filepath.Dir(path) != filepath.Clean(root) {
		return "", fmt.Errorf("ชื่อโปรเจกต์ไม่ถูกต้อง")
	}
	return path, nil
}

// Spaces lists every โปรเจกต์, newest activity first. Reading the disk rather
// than a table is the point (see the type comment) — and it is why this answers
// correctly for a folder the user created in Explorer a second ago.
func (a *App) Spaces() []Space {
	out := []Space{}
	root, err := spacesRoot()
	if err != nil {
		return out
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out // no spaces folder yet is not an error, it is zero projects
	}
	counts := a.spaceChatCounts()
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		out = append(out, a.describeSpace(filepath.Join(root, entry.Name()), entry.Name(), counts[entry.Name()]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

func (a *App) describeSpace(path, name string, chats int) Space {
	space := Space{
		Name:         name,
		Path:         path,
		ContextPath:  filepath.Join(path, contextDirName),
		ContextFiles: []string{},
		Chats:        chats,
	}
	if info, err := os.Stat(path); err == nil {
		space.UpdatedAt = info.ModTime().Format(time.RFC3339)
	}
	if entries, err := os.ReadDir(space.ContextPath); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			space.ContextFiles = append(space.ContextFiles, entry.Name())
		}
		sort.Strings(space.ContextFiles)
	}
	return space
}

// spaceChatCounts is one query for the whole page rather than one per row.
func (a *App) spaceChatCounts() map[string]int {
	counts := map[string]int{}
	db, err := a.database()
	if err != nil {
		return counts
	}
	_ = eachRow(db, "spaces: chat counts",
		`SELECT space, COUNT(*) FROM sessions WHERE space <> '' GROUP BY space`, nil,
		func(rows *sql.Rows) error {
			var name string
			var n int
			if err := rows.Scan(&name, &n); err != nil {
				return err
			}
			counts[name] = n
			return nil
		})
	return counts
}

// CreateSpace makes the folder and the context folder inside it.
//
// Both, in one call, on purpose: a space whose context folder appears only once
// something is put in it is a space where the user has to be told where to put
// things. The folder being there *is* the instruction.
func (a *App) CreateSpace(name string) (Space, error) {
	path, err := spacePath(name)
	if err != nil {
		return Space{}, err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return Space{}, fmt.Errorf("มีโปรเจกต์ชื่อนี้อยู่แล้ว")
	}
	if err := os.MkdirAll(filepath.Join(path, contextDirName), 0o755); err != nil {
		return Space{}, err
	}
	return a.describeSpace(path, filepath.Base(path), 0), nil
}

// OpenSpaceFolder shows the folder in the file manager — the answer to "where
// do I put the files?", given rather than described.
func (a *App) OpenSpaceFolder(name string) error {
	path, err := spacePath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("ยังไม่มีโฟลเดอร์ของโปรเจกต์นี้")
	}
	return a.revealInFileManager(path)
}

// AddSpaceContext copies files the user picks into the project's context
// folder, and answers with the folder's new contents.
//
// Copied, not linked. A link would make the project's context depend on a file
// staying where it was on a machine the user reorganises — and the promise this
// room makes is that the project *carries* its material. What is in the folder
// is what every chat in the project knows about, and that has to survive the
// original being moved into a different folder tomorrow.
//
// Returns the file list rather than nothing so the page redraws from the disk
// it just changed, instead of from what the frontend assumes happened.
func (a *App) AddSpaceContext(name string) ([]string, error) {
	path, err := spacePath(name)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("ยังไม่มีโปรเจกต์ชื่อนี้")
	}
	picked, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "เพิ่มไฟล์บริบทของโปรเจกต์",
	})
	if err != nil {
		return nil, err
	}
	contextDir := filepath.Join(path, contextDirName)
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return nil, err
	}
	for _, source := range picked {
		if err := copyIntoContext(source, contextDir); err != nil {
			return nil, err
		}
	}
	return a.describeSpace(path, filepath.Base(path), 0).ContextFiles, nil
}

// copyIntoContext copies one picked file in, never overwriting: a second
// "รายงาน.pdf" becomes "รายงาน (2).pdf" rather than replacing the one already
// there. The user picked a file to add — silently losing the previous one is
// not something they asked for, and it would be unrecoverable.
func copyIntoContext(source, contextDir string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	base := filepath.Base(source)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	target := filepath.Join(contextDir, base)
	for n := 2; ; n++ {
		if _, err := os.Stat(target); err != nil {
			break
		}
		target = filepath.Join(contextDir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
	}
	return os.WriteFile(target, data, 0o644)
}

// RemoveSpaceContext deletes one file out of a project's context folder.
//
// The name is a bare filename and is checked to resolve inside that folder —
// it arrives from the frontend, and the frontend is not the gate. Deleting is
// the user's own gesture on their own file (the page asks twice, as ผลงาน
// does); nothing here is reachable by the model.
func (a *App) RemoveSpaceContext(name, file string) ([]string, error) {
	path, err := spacePath(name)
	if err != nil {
		return nil, err
	}
	contextDir := filepath.Join(path, contextDirName)
	target := filepath.Join(contextDir, filepath.Base(strings.TrimSpace(file)))
	if filepath.Dir(target) != filepath.Clean(contextDir) || filepath.Base(target) == "" {
		return nil, fmt.Errorf("ไฟล์นี้ไม่ได้อยู่ในโฟลเดอร์บริบทของโปรเจกต์")
	}
	if err := os.Remove(target); err != nil {
		return nil, err
	}
	return a.describeSpace(path, filepath.Base(path), 0).ContextFiles, nil
}

// NewSessionInSpace opens a chat inside a space, at the assistant desk.
//
// The space is held on the App and written onto the session's row when the
// first message lands, exactly as the desk and the agent are (§83, §85): a
// session is born inside a project and stays there. Nothing about the sandbox
// moves — that is what separates this from the workshop's project and it is the
// promise the room's own description makes.
func (a *App) NewSessionInSpace(name string) (string, error) {
	path, err := spacePath(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("ยังไม่มีโปรเจกต์ชื่อนี้")
	}
	id, err := a.NewSessionAt(mode.Default)
	if err != nil {
		return "", err
	}
	a.cur().space = filepath.Base(path)
	// Re-bootstrap so the system prompt is built with the project in it.
	// NewSessionAt already did one, and it ran before this line — without a
	// second the assistant would be told about the project one message late,
	// which is the message where it matters most.
	a.applyConfig(a.cur(), a.cfg)
	return id, nil
}

// resolvedSpace answers what a stored space name means now: itself if the
// folder is still there, "" if it is not. The disk is the only record that a
// project exists, so a name that no longer resolves is a project that no longer
// exists — and telling the assistant it is working inside one that is gone
// would have it looking for a context folder nobody can put anything into.
func (a *App) resolvedSpace(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	path, err := spacePath(name)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return filepath.Base(path)
}

// SessionsInSpace is the space's own chat history, newest first.
func (a *App) SessionsInSpace(name string) []SessionMeta {
	out := []SessionMeta{}
	folder, err := spaceFolderName(name)
	if err != nil {
		return out
	}
	db, dbErr := a.database()
	if dbErr != nil {
		return out
	}
	out, _ = queryAll(db, "spaces: sessions", `
		SELECT id, title, updated_at, mode, agent FROM sessions
		WHERE project_key = ? AND space = ? ORDER BY updated_at DESC LIMIT 200`,
		[]any{projectKey(a.cfg.SandboxRoot), folder},
		func(rows *sql.Rows) (SessionMeta, error) {
			var m SessionMeta
			err := rows.Scan(&m.ID, &m.Title, &m.UpdatedAt, &m.Mode, &m.Agent)
			return m, err
		})
	return out
}

// CurrentSpace is which project this session is being held in, "" for a chat
// held outside every project.
func (a *App) CurrentSpace() string { return a.cur().space }

// spaceContextForPrompt is what the system prompt names: where the project
// keeps its files and which files those are — never their contents.
//
// Naming them costs a line and buys the only thing the assistant is missing,
// which is knowing they exist; it reads the ones a question needs with the tools
// it already has. See prompt.workingIn for why the other design — pasting them
// in — makes the assistant worse at everything else.
func (a *App) spaceContextForPrompt() bootstrap.SpaceContext {
	if a.cur().space == "" {
		return bootstrap.SpaceContext{}
	}
	path, err := spacePath(a.cur().space)
	if err != nil {
		return bootstrap.SpaceContext{}
	}
	space := a.describeSpace(path, a.cur().space, 0)
	return bootstrap.SpaceContext{Path: space.ContextPath, Files: space.ContextFiles}
}
