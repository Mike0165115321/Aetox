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

// maxArtifacts is a safety bound on one reply, and nothing to do with how much
// the page draws.
//
// **The distinction is the owner\'s, 2026-08-14:** *"อันไหนเกินเพดานก็เอามาแต่
// ยังไม่แสดง แต่ขึ้นโหลดเพื่อแสดงเอา จะได้ไม่หายและไม่หนักในตอนที่เปิดตอนแรก"*.
// A file the gallery refuses to send is a file the user cannot find; a file it
// sends but has not drawn yet costs a row of metadata. Those are not the same
// trade, and one number was being asked to make both.
//
// So this is now only what stops a pathological folder from serialising tens of
// megabytes across the binding in one go. It is high enough that no real
// install reaches it, and ArtifactPage.Total still reports the true count, so
// the one install that does reach it is told rather than quietly shortened.
// What keeps the first paint cheap is the frontend drawing a screenful at a
// time, and previews that only load for the cards on screen.
//
// It used to be 500, applied three times on the way *in* — before the sort —
// which made it decide which files you saw rather than how many (see
// ListArtifactsIn).
const maxArtifacts = 5000

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
	return a.ListArtifactsIn(RangeAll).Files
}

// The ranges the gallery opens at. Days rather than calendar weeks/months, and
// the same numbers dayBucket.ts already buckets by (<=7, <=30), so the range a
// person picks and the headings they then read agree about where the line is.
const (
	RangeWeek  = "week"
	RangeMonth = "month"
	RangeAll   = "all"
)

// ArtifactPage is one range of the gallery, and what it could not fit.
//
// Range is the range actually served, which is not always the one asked for:
// see ListArtifactsIn. Total counts what exists in that range before the cap,
// so the page can say "500 of 1,240" instead of quietly ending at 500.
type ArtifactPage struct {
	Files []Artifact `json:"files"`
	Range string     `json:"range"`
	Total int        `json:"total"`
}

// ListArtifactsIn returns the produced files inside one time range, newest
// first (COMPANY.md §2).
//
// **A range rather than a count, owner's call 2026-08-14**, and it fixed a real
// bug rather than only trimming the page. maxArtifacts used to be applied three
// times on the way in — inside a session walk, inside a root walk, and across
// roots — every one of them *before* the sort. os.ReadDir returns names in
// order and a session folder is named for its timestamp, so the sweep ran
// oldest-first and stopped at five hundred. Past that many files the gallery
// showed the five hundred **oldest** artifacts, sorted newest-first, under a
// heading promising the newest: today's work was not on the page at all.
//
// Now nothing stops early. Every root is walked, the whole list is sorted, the
// range decides what is shown and the cap is a backstop applied last, to a set
// that is already in the right order. A full walk is readdir plus a stat per
// file on a page nobody opens in a loop, which is what the file header already
// says this design is paying for.
//
// **It widens when the range it was given is empty.** Opening ผลงาน on a quiet
// Monday and being shown nothing is indistinguishable from the feature being
// broken, so an empty week falls through to the month and then to everything.
// Range comes back saying which one answered, and the picker follows it: the
// control has to keep telling the truth about what is on screen.
func (a *App) ListArtifactsIn(want string) ArtifactPage {
	all := []Artifact{}
	seen := map[string]bool{}
	for _, root := range a.artifactRoots() {
		if root == "" || seen[strings.ToLower(root)] {
			continue
		}
		seen[strings.ToLower(root)] = true
		all = append(all, sweepArtifacts(root)...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Modified > all[j].Modified })

	for _, name := range widenFrom(want) {
		in := within(all, name)
		if len(in) == 0 && name != RangeAll {
			continue
		}
		page := ArtifactPage{Range: name, Total: len(in), Files: in}
		if len(page.Files) > maxArtifacts {
			// Total keeps the real number. A page that shortens itself and says
			// nothing is the failure this whole method was rewritten to remove.
			page.Files = page.Files[:maxArtifacts]
		}
		return page
	}
	return ArtifactPage{Range: RangeAll, Files: []Artifact{}}
}

// widenFrom is the fall-through order for a range that turns out to be empty.
// An unknown name starts at the week, which is what a fresh window asks for.
func widenFrom(want string) []string {
	switch want {
	case RangeMonth:
		return []string{RangeMonth, RangeAll}
	case RangeAll:
		return []string{RangeAll}
	default:
		return []string{RangeWeek, RangeMonth, RangeAll}
	}
}

// within filters an already-sorted list to one range.
//
// Counted in calendar days from this morning, not in 24-hour blocks from now:
// a file written at 11pm last night is yesterday's at 1am whatever the
// arithmetic says, and dayBucket.ts draws its headings by the same rule. Two
// places deciding "which day" differently is the drift dayBucket's own comment
// was written to prevent, and this is the third caller of that question.
func within(all []Artifact, name string) []Artifact {
	days, bounded := rangeDays(name)
	if !bounded {
		return all
	}
	today := time.Now()
	midnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	cutoff := midnight.AddDate(0, 0, -days)
	out := make([]Artifact, 0, len(all))
	for _, art := range all {
		when, err := time.Parse(time.RFC3339, art.Modified)
		// A timestamp that will not parse is kept rather than dropped: the file
		// is real, and hiding it because its clock is odd is the worse failure.
		if err != nil || !when.Before(cutoff) {
			out = append(out, art)
		}
	}
	return out
}

func rangeDays(name string) (int, bool) {
	switch name {
	case RangeWeek:
		return 7, true
	case RangeMonth:
		return 30, true
	}
	return 0, false
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
