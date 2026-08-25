package main

// ไฟล์ที่สร้างหรือแก้ — what this conversation wrote.
//
// The sibling of sources.go, and the other half of the same question. That file
// answers "what did the room read to get here"; a person looking at a finished
// turn asks the second half just as often, and louder: which files on my
// machine did it actually change.
//
// Nothing on screen answered it. ที่เก็บโค้ด lists `git status` — the whole
// working tree, every file the user themselves left dirty, with no way to tell
// the agent's work from their own — and it says nothing at all outside a git
// repository. ผลงาน answers a narrower question than sources.go's comment
// credited it with: it sweeps output/<session>, so it knows about a file the
// agent created for the user and nothing about the source file it edited in
// place.
//
// Read off tool_runs for the reason written at the head of sources.go: a second
// table recording what the first one already knows goes stale the moment a file
// moves, and then it lies about the machine. This is a reading of the record of
// what was called, so it cannot disagree with what happened.

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// EditedFile is one file this conversation changed, as the panel lists it.
type EditedFile struct {
	// Path is where the file is now, resolved the way the agent's own tools
	// resolve it (onDisk) rather than the string the model typed. A row that
	// opens is the whole point of the section, and "notes.md" typed into
	// `write` is on disk at output/<session>/notes.md.
	Path  string `json:"path"`
	Label string `json:"label"`
	// Dir is filled only when another row carries the same Label, and then for
	// every row in the colliding group — same rule as แหล่งที่มา.
	Dir string `json:"dir,omitempty"`
	// Status is what was done to it, as one letter the row can wear:
	// W wrote it whole, M edited part of it, D deleted it. That is the same
	// slot ที่เก็บโค้ด already puts git's letter in.
	Status string `json:"status"`
	// Gone is true when nothing is at Path any more — the agent deleted it, or
	// the user did afterwards. The row still belongs in the list (this section
	// reports what the room DID, and deleting a file is the loudest thing it
	// can do to one) but it must not offer a door that opens nothing.
	Gone bool   `json:"gone"`
	Time string `json:"time"`
}

// writingTools are the tools whose arguments name something the room *changed*,
// and the letter each one earns.
//
// A call that failed changed nothing, so only ok rows are read (SessionEdits'
// query). Without that, a refused write would appear as work that was done —
// the one mistake a list like this must never make.
//
// `delete` is in here rather than left out with the readers: the file it names
// is gone, which is exactly the case somebody opens this panel to confirm.
var writingTools = map[string]string{
	"write":         "W",
	"doc_write":     "W",
	"sheet_write":   "W",
	"notebook_edit": "W",
	"edit":          "M",
	"edits":   "M", // several paths in one call; see editsFromRun
	"delete":        "D",
}

// maxEdits caps what the panel is handed, for the same reason maxSources does.
const maxEdits = 50

// EditPage is the list and how many there are, in one answer.
//
// แหล่งที่มา asks for its count through a second binding, and pays for it with
// a second full scan of the same rows. One call cannot disagree with itself
// about how much it is hiding, which is the whole point of carrying the number.
type EditPage struct {
	Files []EditedFile `json:"files"`
	// Total is every distinct file, including the ones past maxEdits — so a cut
	// list can say what it is cutting. A list that will not say how truncated
	// it is reads as complete.
	Total int `json:"total"`
}

// SessionEdits reports what the given session wrote, newest first.
//
// Keyed by the path on disk, so a file written eleven times is one row and the
// last thing done to it is what the row says: a file written and then deleted
// reads as deleted, which is the truth about it.
func (a *App) SessionEdits(sessionID string) EditPage {
	page := EditPage{Files: []EditedFile{}}
	if strings.TrimSpace(sessionID) == "" {
		return page
	}
	db, err := a.database()
	if err != nil {
		return page
	}
	seen := map[string]EditedFile{}
	order := []string{}
	_ = eachRow(db, "edits", `
		SELECT tool, args, time FROM tool_runs WHERE session_id = ? AND ok = 1 ORDER BY id`,
		[]any{sessionID},
		func(rows *sql.Rows) error {
			var tool, args, at string
			if err := rows.Scan(&tool, &args, &at); err != nil {
				return err
			}
			for _, ed := range a.editsFromRun(sessionID, tool, args, at) {
				if _, dup := seen[ed.Path]; !dup {
					order = append(order, ed.Path)
				}
				seen[ed.Path] = ed
			}
			return nil
		})

	// Newest first: what you are looking for is nearly always what was just
	// done.
	page.Total = len(order)
	for i := len(order) - 1; i >= 0 && len(page.Files) < maxEdits; i-- {
		page.Files = append(page.Files, seen[order[i]])
	}
	markEditCollisions(page.Files)
	return page
}

// notifyFilesChanged tells the window which files a finished call just changed,
// so a pane already showing one stops showing yesterday's bytes.
//
// Owner, 24 ส.ค., with a file tab open beside a working turn: *"ผมทำงานอยู่ มัน
// ปรับเนื้อหาในเอกสารแล้วผมยังเห็นอันเก่าอยู่"*. The pane read the file once,
// when it was opened, and nothing ever told it the file had moved on — so the
// panel whose job is showing the user what the agent produced was showing them
// what it had produced before.
//
// Off the same parse the panel uses (editsFromRun), not a second list of tool
// names: the two questions are "which files did this call change" asked live
// and asked later, and they must not be able to answer differently.
//
// A failed call changed nothing and is not announced. Paths are resolved
// against the writing conversation's own root, which is what makes a background
// chat's write land on the right tab rather than on a same-named file.
func (a *App) notifyFilesChanged(conv *conversation, run turn.ToolRun) {
	if conv == nil || !run.OK {
		return
	}
	edits := a.editsFromRun(conv.id, run.Name, run.Args, "")
	if len(edits) == 0 {
		return
	}
	paths := make([]string, 0, len(edits))
	for _, ed := range edits {
		paths = append(paths, ed.Path)
	}
	a.emitEvent("workbench:files-changed", sessionEvent[[]string]{SessionID: conv.id, Data: paths})
}

// editsFromRun turns one recorded call into the rows it changed, or none when
// it was not a writing call at all.
//
// Args is the model's raw JSON, unparsed by design (turn.ToolRun), so a
// malformed call reaches here as unreadable text and is skipped — a call nobody
// can read the arguments of names no file anybody can open.
func (a *App) editsFromRun(sessionID, tool, args, at string) []EditedFile {
	status, ok := writingTools[strings.ToLower(strings.TrimSpace(tool))]
	if !ok {
		return nil
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(args), &parsed) != nil {
		return nil
	}
	// edits is the one writer that names several files in one call, and
	// every one of them was changed by it. Reading only a top-level "path"
	// would drop the lot.
	raws := []string{}
	if list, isList := parsed["edits"].([]any); isList {
		for _, item := range list {
			step, isMap := item.(map[string]any)
			if !isMap {
				continue
			}
			if p, isText := step["path"].(string); isText {
				raws = append(raws, p)
			}
		}
	} else if p, isText := parsed["path"].(string); isText {
		raws = append(raws, p)
	}

	out := []EditedFile{}
	for _, raw := range raws {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		path, exists := a.onDisk(sessionID, raw)
		out = append(out, EditedFile{
			Path:   path,
			Label:  filepath.Base(path),
			Status: status,
			Gone:   !exists,
			Time:   at,
		})
	}
	return out
}

// onDisk turns a path a tool call named into where that file actually is, and
// says whether anything is there.
//
// The stored argument is what the model typed, and the model types relative
// paths: resolving one against this process's working directory — wherever the
// user happened to launch the app from — answers about a folder nobody in the
// conversation ever mentioned. The agent's own tools resolve against the
// session's sandbox root, and so must anything reading their record back, or
// the panel and the agent disagree about which file is which.
//
// skill.PlacedPath is the second half, and the reason this is not one os.Stat:
// a relative `write` in an unfocused chat lands in output/<session>, so the name
// in the record is not the name on disk. That rule has one definition and this
// borrows it rather than restating it.
func (a *App) onDisk(sessionID, raw string) (string, bool) {
	root := strings.TrimSpace(a.rootFor(sessionID))
	if root == "" {
		// Nothing to resolve against. Report the path as recorded, and let an
		// absolute one still answer for itself.
		_, err := os.Stat(raw)
		return raw, err == nil
	}
	subdir := func() string { return a.outputSubdirFor(sessionID) }
	placed := skill.PlacedPath(root, subdir, raw)
	full, err := safeSandboxPath(root, placed)
	if err != nil {
		return placed, false
	}
	_, err = os.Stat(full)
	return placed, err == nil
}

// rootFor is the sandbox root the named chat runs in. A chat this process holds
// no engine for falls back to the one on screen: the panel only ever asks about
// that one, and an approximate root beats no answer at all.
func (a *App) rootFor(sessionID string) string {
	if conv := a.convs.find(sessionID); conv != nil {
		return conv.cfg.SandboxRoot
	}
	return a.cur().cfg.SandboxRoot
}

// outputSubdirFor is outputSubdirOf's answer for a session id rather than a
// live conversation, so a stored chat's files resolve the way its own tools
// resolved them.
func (a *App) outputSubdirFor(sessionID string) string {
	if a.projectFocused || strings.TrimSpace(sessionID) == "" {
		return ""
	}
	return "output/" + sessionID
}

// markEditCollisions fills Dir on every row whose Label another row also
// carries, the whole group rather than the later members — the same rule and
// the same reason as markCollisions in sources.go, which see.
func markEditCollisions(list []EditedFile) {
	for _, i := range collidingRows(len(list), func(i int) string { return list[i].Label }) {
		list[i].Dir = filepath.Dir(list[i].Path)
	}
}
