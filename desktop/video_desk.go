package main

// The video desk opens itself.
//
// Every other file the agent puts in front of someone gets there because the
// agent decided to: `desk_open` is a tool, and a model that forgets to call it
// leaves the user reading "ตัดให้แล้วครับ" with an empty panel beside it. That
// is the right shape for a general tool and the wrong one for this job, because
// cutting video is the one kind of work where the artifact IS the answer. A
// sentence describing a cut cannot be checked. The clip can.
//
// kinocut is also the one toolset where the app can know, without being told,
// which file to show — its tools take an absolute `input_path` and return an
// absolute `output_path`, and both are in the record of the call. So the two
// beats here are read off the call rather than asked for:
//
//	source — the first time this chat touches a file, it opens. The user can
//	         watch the thing being worked on while it is being worked on.
//	result — the moment a tool returns a new file, it opens beside the source
//	         and takes focus. Switching tabs is then the before/after.
//
// **No new rule decides when this is allowed to happen.** The `kinocut` server
// is placed `for: agent:editor` (mcp-servers.json), so these tool names cannot
// be called from anywhere else in the app, and the event is stamped with the
// calling conversation's id, so it lands on that chat's desk under the rule
// every desk event already follows (§187). "When does it show" and "where did
// the work happen" are the same question, and it was already answered.
//
// **Nothing here reads the agent's prose.** The origin line under the player is
// built from the tool's own JSON — operation, duration, size, resolution — and
// the plan bar from the arguments the model actually sent plus the lengths
// kinocut itself reported. An agent that says it cut 18 seconds and cut 8
// cannot make this say 18.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/mcp"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// MediaSpan is one stretch of a clip's timeline, in seconds from its start.
type MediaSpan struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	// Label names the span for the reader — a clip's filename in a merge, a
	// scene's number in a detection. Empty when the span speaks for itself.
	Label string `json:"label,omitempty"`
}

// MediaPlan is the cut drawn as a bar: the whole source, and which parts of it
// survived into the file the user is looking at.
//
// It exists because it answers the question a person has BEFORE they press
// play, and pressing play cannot answer it: what got thrown away. A 20-second
// clip that should be 18 looks exactly like a correct one until you have
// watched all of it.
//
// Nil whenever the answer is not actually known — see planFor. A bar drawn
// from a guessed total would be the one thing this must never be.
type MediaPlan struct {
	// Total is the source's full length in seconds, as kinocut measured it.
	Total float64     `json:"total"`
	Kept  []MediaSpan `json:"kept"`
	// Marks are scene boundaries the agent detected on this same source, drawn
	// under the bar. Empty unless `video_detect_scenes` was actually run — this
	// is a report of work that happened, not an analysis this file performs.
	Marks []MediaSpan `json:"marks,omitempty"`
}

// MediaOrigin is what one file on the desk came from, in the tool's own words.
//
// Numbers rather than sentences, deliberately: the wording is the window's job
// (it has three locales), and this side's job is to be unable to say anything
// the tool did not.
type MediaOrigin struct {
	Path string `json:"path"`
	Name string `json:"name"`
	// Role is "source" or "result" — which of the two beats opened this tab.
	Role string `json:"role"`
	// Tool is the namespaced name of the call, kept so the line can say what
	// was run even for a tool whose result carries no `operation`.
	Tool string `json:"tool,omitempty"`
	// Operation is kinocut's own name for what it did ("trim", "storyboard",
	// "thumbnail"), straight out of the result.
	Operation  string     `json:"operation,omitempty"`
	Duration   float64    `json:"duration,omitempty"`
	SizeMB     float64    `json:"sizeMB,omitempty"`
	Resolution string     `json:"resolution,omitempty"`
	Plan       *MediaPlan `json:"plan,omitempty"`
}

// videoDesk is what this chat has learned watching its own editor work.
//
// Per conversation for the reason every other desk state is (§187): two chats
// editing at once would otherwise share one memory of "how long is that file",
// and answer each other's questions with it.
type videoDesk struct {
	mu sync.Mutex
	// length is every clip kinocut has told this chat the duration of, keyed by
	// normalised path. Built up from whatever calls happen to be made rather
	// than probed for: this file starts no programs, and a length nobody
	// measured is a length the plan bar does without.
	length map[string]float64
	// scenes is the same, for `video_detect_scenes`.
	scenes map[string][]MediaSpan
	// sourced is the paths already opened as a source, so the tab appears on
	// the first touch and never fights the user for focus on the twelfth.
	sourced map[string]bool
}

func (d *videoDesk) init() {
	if d.length == nil {
		d.length = map[string]float64{}
		d.scenes = map[string][]MediaSpan{}
		d.sourced = map[string]bool{}
	}
}

// mediaKey is one path's identity in those maps. Case-folded and cleaned
// because Windows hands the same file back spelled several ways, and a cache
// that misses on `C:\X\a.mp4` after storing `C:/x/A.mp4` is a plan bar that
// silently stops being drawn.
func mediaKey(path string) string {
	return strings.ToLower(filepath.Clean(strings.TrimSpace(path)))
}

// autoOpenMedia is the whole feature, hung off the same OnToolRun the file
// cards already use.
func (a *App) autoOpenMedia(conv *conversation, run turn.ToolRun) {
	if conv == nil || !run.OK {
		return
	}
	// The one gate, and it is the naming rule itself rather than a copy of it
	// (mcp.ToolBelongsTo). A prefix loop written here would be a second
	// definition of which tools are the editor's.
	if !mcp.ToolBelongsTo(run.Name, []string{VideoEditorServer}) {
		return
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(run.Args), &args); err != nil {
		// A call whose arguments nobody can read names no file anybody can
		// open — the same judgement editsFromRun makes, for the same reason.
		args = nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(run.Output), &result); err != nil {
		return
	}
	// kinocut's own wrapper puts `success` on every result, including the ones
	// it builds out of an exception (server_app._result). A tool that reported
	// failure has no file to show, whatever the transport thought.
	if ok, _ := result["success"].(bool); !ok {
		return
	}

	conv.video.mu.Lock()
	conv.video.init()
	conv.video.learn(args, result)
	source := sourcePath(args)
	firstTouch := source != "" && !conv.video.sourced[mediaKey(source)]
	if firstTouch {
		conv.video.sourced[mediaKey(source)] = true
	}
	plan := conv.video.planFor(run.Name, args, result)
	conv.video.mu.Unlock()

	// Source first, so the result is what ends up in front of the user.
	if firstTouch {
		a.openMediaTab(conv, MediaOrigin{Path: source, Role: "source", Tool: run.Name})
	}
	for _, path := range resultPaths(result) {
		if source != "" && mediaKey(path) == mediaKey(source) {
			// A read-only tool that echoes its input as `output_path` must not
			// re-announce the source as a result of itself.
			continue
		}
		a.openMediaTab(conv, MediaOrigin{
			Path:       path,
			Role:       "result",
			Tool:       run.Name,
			Operation:  text(result["operation"]),
			Duration:   number(result["duration"]),
			SizeMB:     number(result["size_mb"]),
			Resolution: text(result["resolution"]),
			Plan:       plan,
		})
	}
}

// openMediaTab resolves the path and hands it to the desk, or does nothing.
//
// The extension check is what keeps this to files the desk can actually draw.
// kinocut returns .srt, .json and project folders too, and a tab that opens on
// one of those is a tab the user has to close — the point of opening a file
// unasked is that looking at it IS the reading.
func (a *App) openMediaTab(conv *conversation, origin MediaOrigin) {
	raw := strings.TrimSpace(origin.Path)
	if raw == "" || !isDeskMedia(raw) {
		return
	}
	path, exists := a.onDisk(conv.id, raw)
	if !exists {
		return
	}
	origin.Path = path
	origin.Name = filepath.Base(filepath.FromSlash(path))
	a.deskMediaOpened(conv, origin)
}

// deskMediaExts is what the workbench draws from a URL without reading the
// bytes, and it is deliberately the video/image/audio thirds of the frontend's
// own viewByExt rather than all four: a .pdf out of a video editor would be a
// report, and a report is reading rather than watching.
//
// Kept in step with desktop/frontend/src/lib/stores/workbench.svelte.ts by
// TestDeskMediaMatchesThePanes, which reads that file.
var deskMediaExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".avif": true, ".ico": true, ".svg": true,
	".mp4": true, ".webm": true, ".mov": true, ".mkv": true, ".avi": true,
	".mp3": true, ".wav": true, ".m4a": true, ".flac": true, ".ogg": true,
}

func isDeskMedia(path string) bool {
	return deskMediaExts[strings.ToLower(filepath.Ext(path))]
}

// ---------------------------------------------------------------------------
// cutting_room — the room, opened on purpose
// ---------------------------------------------------------------------------

// cuttingRoomSkill lets the editor put its own room on the desk.
//
// The room opens itself when work starts (autoOpenMedia), so most sessions
// never need this call — it exists for the two moments the automatic beat
// cannot cover: the user closed the room and asks to see it again, and the
// model wants the ledger up before the first cut so the user watches the work
// arrive from the start.
//
// It is NOT in the desk pack, and the reason is the pack's own budget rule
// (§99): every line in `desk`'s description is paid for by every session in
// the app, and this room means something only where the editor's tools are.
// So the skill is registered per conversation, only when the chair this chat
// is held with has the kinocut server placed for it (workbenchSkills) — the
// same placement that decides where the `kinocut_*` tools themselves exist,
// asked of the same list. One rule, two readers, zero tokens everywhere else.
type cuttingRoomSkill struct {
	app  *App
	conv *conversation
}

// conversationHasEditor is that gate. The chair's name is compared against the
// server's placement rather than against the literal "editor", so a user who
// points kinocut at an agent of their own gets the room's tool with it.
func conversationHasEditor(conv *conversation) bool {
	for _, server := range config.MCPServersForAgent(conv.chair) {
		if strings.EqualFold(server, VideoEditorServer) {
			return true
		}
	}
	return false
}

func (*cuttingRoomSkill) Name() string { return "cutting_room" }

func (*cuttingRoomSkill) Description() string {
	return "เปิดห้องตัดบนโต๊ะ — บัญชีไฟล์ของงานตัดรอบนี้ พร้อมเครื่องเล่น"
}

func (*cuttingRoomSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("cutting_room",
		"Open the cutting room on the user's desk: the ledger of this session's cut — source and every result, with a player. It also opens itself when your first video tool runs, so call this only to bring it back after the user closed it, or to have it up before you start.",
		map[string]any{"type": "object", "properties": map[string]any{}})
}

func (s *cuttingRoomSkill) ExecuteTool(_ context.Context, _ map[string]any) (skill.Output, error) {
	return s.open()
}

func (s *cuttingRoomSkill) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	return s.open()
}

func (s *cuttingRoomSkill) open() (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: "cutting_room", Command: "cutting_room"}
	if s.app.ctx == nil {
		err := fmt.Errorf("UI not ready")
		out.Content = "เปิดห้องตัดไม่สำเร็จ: " + err.Error()
		out.Stderr = err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}
	// Through the desk's one door, like everything that lands on it (§187.3):
	// a background chat's room opens on THAT chat's desk.
	s.app.deskEvent(s.conv.id, "open-cutroom", nil)
	out.Success = true
	out.DurationMs = time.Since(start).Milliseconds()
	out.Content = "เปิดห้องตัดบนโต๊ะแล้ว ผู้ใช้เห็นบัญชีงานตัดรอบนี้อยู่"
	out.RawOutput = out.Content
	return out, nil
}

// ---------------------------------------------------------------------------
// reading one call
// ---------------------------------------------------------------------------

// sourceKeys is where kinocut spells "the file I was pointed at", most common
// first. A list rather than a per-tool table because there are 54 of these
// tools and they agree on their vocabulary — and a table would have to be
// revisited on the editor's every release, which is exactly the maintenance
// this app does not take on for somebody else's server.
var sourceKeys = []string{"input_path", "video_path", "clips", "input_paths"}

func sourcePath(args map[string]any) string {
	for _, key := range sourceKeys {
		switch v := args[key].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case []any:
			// An ordered list of clips: the first one is the file the work
			// starts at, which is what "source" means on this desk.
			if len(v) > 0 {
				if s, ok := v[0].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

// resultKeys is the same for what came back. `output_path` alone would miss
// the two results that carry a second file worth seeing: a storyboard's grid
// and a subtitle run's burned-in video, both of which kinocut's own models
// alias into output_path only when the other is absent (models.py).
var resultKeys = []string{"output_path", "grid", "video_path", "frame_path"}

// resultPaths is every file this call produced that is worth a tab.
//
// A storyboard's individual `frames` are deliberately not among them: they are
// the same picture cut up, and opening twelve tabs of them would bury
// everything else on the desk. The grid is the one that answers the question.
func resultPaths(result map[string]any) []string {
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[mediaKey(raw)] {
			return
		}
		seen[mediaKey(raw)] = true
		out = append(out, raw)
	}
	for _, key := range resultKeys {
		if s, ok := result[key].(string); ok {
			add(s)
		}
	}
	return out
}

// learn files away what this call happened to reveal about the clips involved.
//
// Called under the lock, from autoOpenMedia only.
func (d *videoDesk) learn(args, result map[string]any) {
	// video_info's whole purpose, and the usual way a length first becomes
	// known: {success, info:{duration, ...}}.
	if info, ok := result["info"].(map[string]any); ok {
		if secs := number(info["duration"]); secs > 0 {
			if src := text(args["input_path"]); src != "" {
				d.length[mediaKey(src)] = secs
			}
		}
	}
	// Every rendering tool reports the length of what it just wrote, which is
	// how a clip made two calls ago can be measured in a merge today.
	if secs := number(result["duration"]); secs > 0 {
		if outPath := text(result["output_path"]); outPath != "" {
			d.length[mediaKey(outPath)] = secs
		}
	}
	// detect_scenes measures the source it was given, so it teaches both.
	if scenes, ok := result["scenes"].([]any); ok {
		src := text(args["input_path"])
		if src == "" {
			return
		}
		if secs := number(result["duration"]); secs > 0 {
			d.length[mediaKey(src)] = secs
		}
		var spans []MediaSpan
		for i, item := range scenes {
			row, isMap := item.(map[string]any)
			if !isMap {
				continue
			}
			spans = append(spans, MediaSpan{
				Start: number(row["start"]),
				End:   number(row["end"]),
				Label: strconv.Itoa(i + 1),
			})
		}
		if len(spans) > 0 {
			d.scenes[mediaKey(src)] = spans
		}
	}
}

// planFor draws the bar, for the two calls whose arguments actually describe a
// cut, and returns nil for everything else.
//
// Two, not fifty-four, and the restraint is the honesty: `video_trim` and
// `video_merge` say in their own arguments which parts of which files survive,
// so the bar is a redrawing of the call. A crop or a speed change alters the
// clip without removing a span, and a bar over those would be decoration that
// looks like a measurement.
//
// Called under the lock.
func (d *videoDesk) planFor(tool string, args, result map[string]any) *MediaPlan {
	switch strings.TrimPrefix(strings.ToLower(tool), mcp.ToolPrefix(VideoEditorServer)) {
	case "video_trim":
		return d.trimPlan(args, result)
	case "video_merge":
		return d.mergePlan(args)
	}
	return nil
}

// trimPlan is the whole source with the kept stretch marked on it.
//
// Nil without a total, and that is the point of the whole type: "18 seconds
// were kept" is a fact, "18 of 45 were kept" is the answer, and the difference
// is a number this app must have been told rather than assumed.
func (d *videoDesk) trimPlan(args, result map[string]any) *MediaPlan {
	src := text(args["input_path"])
	total := d.length[mediaKey(src)]
	if src == "" || total <= 0 {
		return nil
	}
	start := seconds(args["start"])
	end := total
	switch {
	case text(args["end"]) != "":
		end = seconds(args["end"])
	case args["duration"] != nil:
		end = start + seconds(args["duration"])
	case number(result["duration"]) > 0:
		// The renderer's own measurement of what it wrote, which is the most
		// truthful of the three: fast seeking lands on a keyframe, so a clip
		// asked for at 10.0 can honestly begin at 9.7.
		end = start + number(result["duration"])
	}
	if end > total {
		end = total
	}
	if end <= start {
		return nil
	}
	return &MediaPlan{Total: total, Kept: []MediaSpan{{Start: start, End: end}}, Marks: d.scenes[mediaKey(src)]}
}

// mergePlan lays the clips end to end, each as long as kinocut said it was.
//
// All or nothing: one clip of unknown length would shift every clip after it,
// so a bar missing a single measurement is a bar drawn in the wrong places.
func (d *videoDesk) mergePlan(args map[string]any) *MediaPlan {
	clips, ok := args["clips"].([]any)
	if !ok || len(clips) == 0 {
		return nil
	}
	plan := &MediaPlan{}
	at := 0.0
	for _, item := range clips {
		path, isText := item.(string)
		if !isText {
			return nil
		}
		length := d.length[mediaKey(path)]
		if length <= 0 {
			return nil
		}
		plan.Kept = append(plan.Kept, MediaSpan{
			Start: at,
			End:   at + length,
			Label: filepath.Base(filepath.FromSlash(strings.TrimSpace(path))),
		})
		at += length
	}
	plan.Total = at
	return plan
}

// ---------------------------------------------------------------------------
// reading JSON somebody else wrote
// ---------------------------------------------------------------------------

func text(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// number reads a JSON number, and a number that arrived as a string too:
// kinocut's timestamps are strings by contract and its measurements are not,
// and a reader that trusted the distinction would drop a value on the first
// tool that disagreed.
func number(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

// seconds reads a kinocut timestamp: "90", "90.5", "01:30", "00:01:30.5".
//
// Its own parser rather than time.ParseDuration, because none of those spell a
// Go duration and the ones with colons are the shape a person types.
func seconds(v any) float64 {
	raw := text(v)
	if raw == "" {
		return number(v)
	}
	if !strings.Contains(raw, ":") {
		return number(raw)
	}
	var total float64
	for _, part := range strings.Split(raw, ":") {
		f, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return 0
		}
		total = total*60 + f
	}
	return total
}
