package main

// The ผลงาน page (COMPANY.md §2): every file Aetox has made, gathered from
// where it actually made them.
//
// The disk is the index. Nothing writes a row when a tool creates a file, and
// nothing should: an index and a folder that can disagree is a gallery that
// shows files that are gone and hides files that are there, and the folder is
// the half users move, rename and delete without telling us. Reading it live
// costs one directory walk on a page nobody opens in a loop.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// outputDir is the folder outputSubdir writes into, relative to a root. Named
// here rather than spelled out twice — the two halves are checked together in
// app_test.go, and this reader has to look exactly where the writer put things.
const outputDir = "output"

// maxArtifacts bounds one sweep. A gallery is something a person looks
// through, and past a few hundred rows the page is the wrong tool anyway — the
// files are in a folder the user can open (they are ordinary files, which is
// the point).
const maxArtifacts = 500

// Artifact is one file the agent produced, as the gallery shows it.
type Artifact struct {
	Name string `json:"name"`
	Path string `json:"path"` // absolute — what the open button needs
	// SessionID is the chat that made it, read off the folder name. It is what
	// makes "jump to the conversation this came from" possible without a table
	// recording it, and it is empty for a file sitting loose in output/.
	SessionID string `json:"sessionId,omitempty"`
	Size      int64  `json:"size"`
	Modified  string `json:"modified"` // RFC3339
	// Root is the workspace it was found under, so a gallery spanning the
	// machine can say where a file lives without printing the whole path.
	Root string `json:"root"`
}

// ListArtifacts returns every file under <root>/output/<session> for each root
// this install knows about, newest first.
//
// The roots are the unfocused working folder (where every chat with no project
// open writes) and every project ever opened. A project that has been deleted
// or moved simply reads as nothing there, which is the truth about it.
func (a *App) ListArtifacts() []Artifact {
	out := []Artifact{}
	seen := map[string]bool{}
	for _, root := range a.artifactRoots() {
		if root == "" || seen[strings.ToLower(root)] {
			continue
		}
		seen[strings.ToLower(root)] = true
		out = append(out, sweepArtifacts(root)...)
		if len(out) >= maxArtifacts {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	if len(out) > maxArtifacts {
		out = out[:maxArtifacts]
	}
	return out
}

// artifactRoots is every workspace whose output folder is ours to read: the
// unfocused root first (where the overwhelming majority of artifacts land,
// because a focused project writes into the project itself), then the projects
// the user has opened, most recent first.
func (a *App) artifactRoots() []string {
	roots := []string{unfocusedRoot(), a.cfg.SandboxRoot}
	for _, p := range a.RecentProjects() {
		roots = append(roots, p.RootPath)
	}
	return roots
}

// sweepArtifacts lists the files under one root's output folder. A missing
// folder is the normal state — a fresh install, or a project whose chats never
// wrote anything — and reads as no artifacts rather than as an error.
func sweepArtifacts(root string) []Artifact {
	dir := filepath.Join(root, outputDir)
	sessions, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Artifact
	for _, entry := range sessions {
		path := filepath.Join(dir, entry.Name())
		if !entry.IsDir() {
			// A file loose in output/ predates per-session folders. It is still
			// something the agent made, so it is still shown; it just has no
			// session to jump back to.
			if art, ok := describeArtifact(path, "", root); ok {
				out = append(out, art)
			}
			continue
		}
		out = append(out, sweepSession(path, entry.Name(), root)...)
		if len(out) >= maxArtifacts {
			break
		}
	}
	return out
}

// sweepSession walks one session's output folder. Nested folders are walked
// because a deliverable can legitimately bring its own — an exported site, a
// folder of frames — and a gallery that showed only the top level would hide
// exactly the work that took longest.
func sweepSession(dir, sessionID, root string) []Artifact {
	var out []Artifact
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil //lint:ignore nilerr an unreadable entry is skipped, not fatal
		}
		if len(out) >= maxArtifacts {
			return filepath.SkipAll
		}
		if art, ok := describeArtifact(path, sessionID, root); ok {
			out = append(out, art)
		}
		return nil
	})
	return out
}

// insideOutput resolves path and reports whether it sits inside one of this
// install's `<root>/output/` trees.
//
// Every artifact path arrives back over a JS binding, so it is checked against
// the folders the gallery actually swept rather than trusted. Nothing else can
// be named through these two doors — not the project's own source, not a path
// assembled with `..`, not a root the user never opened.
func (a *App) insideOutput(path string) (string, bool) {
	clean, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return "", false
	}
	for _, root := range a.artifactRoots() {
		if strings.TrimSpace(root) == "" {
			continue
		}
		dir, err := filepath.Abs(filepath.Join(root, outputDir))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(dir, clean)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return clean, true
	}
	return "", false
}

// OpenArtifact hands one produced file to the program that owns it.
//
// Separate from OpenFileExternally, which takes a path relative to the *open
// project's* sandbox root: an artifact is absolute and routinely belongs to
// another project's output folder, so routing it through that door would either
// fail or need the sandbox opened up. This one is bounded by the gallery's own
// roots instead.
func (a *App) OpenArtifact(path string) error {
	full, ok := a.insideOutput(path)
	if !ok {
		return fmt.Errorf("เปิดได้เฉพาะไฟล์ในโฟลเดอร์ผลงานเท่านั้น")
	}
	info, err := os.Stat(full)
	if os.IsNotExist(err) {
		// The gallery reads the disk live, but a file can go between the sweep
		// and the click. Say so plainly rather than passing up a Win32 error.
		return fmt.Errorf("ไฟล์นี้ไม่อยู่แล้ว")
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("นี่เป็นโฟลเดอร์ ไม่ใช่ไฟล์ผลงาน")
	}
	return openInFileManager(full)
}

// DeleteArtifact removes one produced file. The ผลงาน page is the only place a
// file dies (COMPANY.md §6.7): deleting a conversation leaves its work alone,
// so this is the door that had to exist for the rule to be liveable.
//
// A file that is already gone is a success: the user asked for it not to be
// there, and it is not there.
func (a *App) DeleteArtifact(path string) error {
	full, ok := a.insideOutput(path)
	if !ok {
		return fmt.Errorf("ลบได้เฉพาะไฟล์ในโฟลเดอร์ผลงานเท่านั้น")
	}
	info, err := os.Stat(full)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("นี่เป็นโฟลเดอร์ ไม่ใช่ไฟล์ผลงาน")
	}
	return os.Remove(full)
}

func describeArtifact(path, sessionID, root string) (Artifact, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return Artifact{}, false
	}
	return Artifact{
		Name:      filepath.Base(path),
		Path:      path,
		SessionID: sessionID,
		Size:      info.Size(),
		Modified:  info.ModTime().Format(time.RFC3339),
		Root:      root,
	}, true
}
