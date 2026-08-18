package main

// แหล่งที่มา — what this conversation read to get its answer.
//
// The transcript already contains every one of these, buried in a tool timeline
// that is tens of screens long by the time anyone wants to look. That is fine
// for "what happened" and useless for "which file was that again", which is the
// question people actually come back with — and in an app that reads the user's
// own disk, it is also the question that decides whether they trust the answer.
//
// Read off tool_runs rather than kept in a table of its own, for the reason
// written at the head of artifacts.go: a second place recording what the first
// one already knows goes stale the moment somebody moves a file, and then it
// lies about the machine. tool_runs is the record of what was called; this is a
// reading of it, and it cannot disagree with what happened.

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Source is one thing the room looked at, as the panel lists it.
type Source struct {
	Kind string `json:"kind"` // "file" | "url"
	// Label is what the row reads. A file's base name, a URL's host+path —
	// never the whole absolute path, which is unreadable at panel width and
	// puts the part that distinguishes two rows off the right-hand edge.
	Label string `json:"label"`
	// Path is the whole of it: what opening the row needs, and what the title
	// attribute shows for a label that had to be shortened.
	Path string `json:"path"`
	// Dir is filled in only for a Label that collides with another row's, and
	// then for every row in the colliding group rather than only the later
	// ones. Two rows both reading `code.html` is the failure this exists to
	// prevent: the one job of a list like this is pointing at the right thing.
	Dir  string `json:"dir,omitempty"`
	Time string `json:"time"`
}

// maxSources caps what the panel is handed. The count of everything is returned
// alongside it (SessionSourceCount) so "ดูทั้งหมด" can say how much is behind
// it — a truncated list that will not say how truncated reads as complete.
const maxSources = 50

// readingTools are the tools whose arguments name something the room *read*.
//
// Writers are deliberately absent. A file this conversation produced is not a
// source for it, and the ผลงาน page already owns that question — listing them
// here would be the second place answering it, which is the thing this file was
// written to avoid.
//
// glob and grep are absent too, and that is a judgement rather than an
// oversight: their argument is a pattern, not a file. "**/*.ts" in a list of
// sources tells nobody which file mattered, and the reads that followed the
// search are in here on their own account anyway.
var readingTools = map[string]string{
	"read":             "path",
	"pdf_read":         "path",
	"image_ocr":        "path",
	"video_ocr":        "path",
	"audio_transcribe": "path",
	"web_fetch":        "url",
	"browser_open":     "url",
	"browser":          "url", // the packed tool; open is the action that carries one
	"github_read_file": "path",
}

// SessionSources reports what the given session read, newest first.
//
// A file that is no longer on disk is left out. The user moved or deleted it,
// and a row that opens nothing is worse than no row — the same rule the ผลงาน
// gallery follows by sweeping the folder instead of trusting an index.
func (a *App) SessionSources(sessionID string) []Source {
	out := []Source{}
	if strings.TrimSpace(sessionID) == "" {
		return out
	}
	db, err := a.database()
	if err != nil {
		return out
	}
	// Keyed by Path so a file read eleven times is one row, and keeping the
	// later hit means the row's time answers "when did this room last touch
	// it" rather than "when did it first".
	seen := map[string]Source{}
	order := []string{}
	_ = eachRow(db, "sources", `
		SELECT tool, args, time FROM tool_runs WHERE session_id = ? ORDER BY id`,
		[]any{sessionID},
		func(rows *sql.Rows) error {
			var tool, args, at string
			if err := rows.Scan(&tool, &args, &at); err != nil {
				return err
			}
			src, ok := sourceFromRun(tool, args, at)
			if !ok {
				return nil
			}
			if _, dup := seen[src.Path]; !dup {
				order = append(order, src.Path)
			}
			seen[src.Path] = src
			return nil
		})

	// Newest first: what you are looking for is nearly always what you were
	// just doing.
	for i := len(order) - 1; i >= 0; i-- {
		src := seen[order[i]]
		if src.Kind == "file" {
			if _, err := os.Stat(src.Path); err != nil {
				continue
			}
		}
		out = append(out, src)
		if len(out) == maxSources {
			break
		}
	}
	markCollisions(out)
	return out
}

// SessionSourceCount is how many the session has in total, so a list cut at
// maxSources can say what it is hiding. Counted the same way the list is built,
// missing files and all, or the number would promise rows the list cannot show.
func (a *App) SessionSourceCount(sessionID string) int {
	return len(a.allSources(sessionID))
}

func (a *App) allSources(sessionID string) []Source {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	db, err := a.database()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := []Source{}
	_ = eachRow(db, "sources: all", `
		SELECT tool, args, time FROM tool_runs WHERE session_id = ? ORDER BY id`,
		[]any{sessionID},
		func(rows *sql.Rows) error {
			var tool, args, at string
			if err := rows.Scan(&tool, &args, &at); err != nil {
				return err
			}
			src, ok := sourceFromRun(tool, args, at)
			if !ok || seen[src.Path] {
				return nil
			}
			if src.Kind == "file" {
				if _, statErr := os.Stat(src.Path); statErr != nil {
					return nil
				}
			}
			seen[src.Path] = true
			out = append(out, src)
			return nil
		})
	return out
}

// sourceFromRun turns one recorded call into a row, or reports that it was not
// a reading call at all.
//
// Args is the model's raw JSON, unparsed by design (turn.ToolRun) — so a
// malformed call reaches here as unparseable text, and the answer to that is to
// skip it. A tool call nobody can read the arguments of read nothing anybody
// can name.
func sourceFromRun(tool, args, at string) (Source, bool) {
	key, ok := readingTools[strings.ToLower(strings.TrimSpace(tool))]
	if !ok {
		return Source{}, false
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(args), &parsed) != nil {
		return Source{}, false
	}
	raw, _ := parsed[key].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Source{}, false
	}
	// The packed browser tool carries every action through one schema, and only
	// `open` names something read. Without this, a click's coordinates or a
	// type's text would arrive as a source.
	if action, has := parsed["action"].(string); has && !strings.EqualFold(action, "open") {
		return Source{}, false
	}
	if isURL(raw) {
		return Source{Kind: "url", Label: urlLabel(raw), Path: raw, Time: at}, true
	}
	return Source{
		Kind:  "file",
		Label: filepath.Base(raw),
		Path:  raw,
		Time:  at,
	}, true
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// urlLabel keeps the host and the tail of the path.
//
// A URL's identity is at both ends and a plain truncation takes the wrong one:
// two Railway deployments differ only after the host, so cutting the tail off
// leaves two rows that read identically — which is the exact failure a list of
// sources exists to prevent.
func urlLabel(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	path := strings.TrimSuffix(u.Path, "/")
	if path == "" {
		return u.Host
	}
	return u.Host + path
}

// markCollisions fills Dir on every row whose Label another row also carries.
//
// The whole group is marked, not the later members: telling two rows apart
// needs both of them to say where they are, and a `code.html` sitting next to
// `src/code.html` still leaves the reader working out which one the bare name
// belongs to.
func markCollisions(list []Source) {
	byLabel := map[string][]int{}
	for i, s := range list {
		byLabel[s.Label] = append(byLabel[s.Label], i)
	}
	labels := make([]string, 0, len(byLabel))
	for label := range byLabel {
		labels = append(labels, label)
	}
	sort.Strings(labels) // deterministic, so a test can rely on it
	for _, label := range labels {
		idx := byLabel[label]
		if len(idx) < 2 {
			continue
		}
		for _, i := range idx {
			if list[i].Kind == "file" {
				list[i].Dir = filepath.Dir(list[i].Path)
			}
		}
	}
}
