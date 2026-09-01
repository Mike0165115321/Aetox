package main

// What the video desk is allowed to say, measured against real kinocut
// results. The JSON in these tests is the shape kinocut's own models produce
// (models.py: EditResult and friends) — an invented shape would prove only
// that the parser reads itself.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/turn"
)

// run builds the record the executor would hand OnToolRun.
func videoRun(name string, args, result any) turn.ToolRun {
	a, _ := json.Marshal(args)
	r, _ := json.Marshal(result)
	return turn.ToolRun{Name: name, Args: string(a), Output: string(r), OK: true}
}

// deskApp is an App whose desk events are collected instead of emitted.
func deskApp(t *testing.T) (*App, *conversation, *[]MediaOrigin) {
	t.Helper()
	root := t.TempDir()
	var opened []MediaOrigin
	a := &App{}
	a.emit = func(event string, data ...any) {
		if event != "workbench:open-media" || len(data) == 0 {
			return
		}
		ev, ok := data[0].(sessionEvent[MediaOrigin])
		if !ok {
			t.Fatalf("open-media carried %T, want a session-stamped MediaOrigin", data[0])
		}
		opened = append(opened, ev.Data)
	}
	conv := &conversation{id: "s1"}
	conv.cfg.SandboxRoot = root
	return seed(a, conv), conv, &opened
}

// The two beats: the source opens when the agent first touches it, the result
// opens when it exists, and the result is last so it is what has focus.
func TestVideoDeskOpensSourceThenResult(t *testing.T) {
	a, conv, opened := deskApp(t)
	dir := conv.cfg.SandboxRoot
	src := touch(t, dir, "talk.mp4")
	out := touch(t, dir, "talk-cut.mp4")

	a.autoOpenMedia(conv, videoRun("kinocut_video_trim",
		map[string]any{"input_path": src, "start": "0", "duration": "18"},
		map[string]any{"success": true, "output_path": out, "operation": "trim", "duration": 18.0, "size_mb": 4.2, "resolution": "1280x720"},
	))

	if len(*opened) != 2 {
		t.Fatalf("opened %d tabs, want the source and the result", len(*opened))
	}
	if (*opened)[0].Role != "source" || (*opened)[0].Path != src {
		t.Errorf("first tab = %+v, want the source %q", (*opened)[0], src)
	}
	got := (*opened)[1]
	if got.Role != "result" || got.Path != out {
		t.Errorf("second tab = %+v, want the result %q", got, out)
	}
	if got.Operation != "trim" || got.Duration != 18 || got.SizeMB != 4.2 || got.Resolution != "1280x720" {
		t.Errorf("origin = %+v, want every fact carried off the tool's own JSON", got)
	}
}

// The source opens on the FIRST touch and never again: a second cut of the
// same footage must not drag the user back to the tab they left.
func TestVideoDeskOpensTheSourceOnlyOnce(t *testing.T) {
	a, conv, opened := deskApp(t)
	dir := conv.cfg.SandboxRoot
	src := touch(t, dir, "talk.mp4")
	first := touch(t, dir, "a.mp4")
	second := touch(t, dir, "b.mp4")

	for _, out := range []string{first, second} {
		a.autoOpenMedia(conv, videoRun("kinocut_video_trim",
			map[string]any{"input_path": src, "start": "0", "duration": "5"},
			map[string]any{"success": true, "output_path": out, "operation": "trim"},
		))
	}

	sources := 0
	for _, o := range *opened {
		if o.Role == "source" {
			sources++
		}
	}
	if sources != 1 {
		t.Errorf("the source opened %d times, want once", sources)
	}
}

// The plan bar needs a total somebody measured. Without a video_info there is
// no honest bar to draw, and a guessed one is the single thing this must never
// produce.
func TestVideoDeskDrawsNoPlanWithoutAMeasuredTotal(t *testing.T) {
	a, conv, opened := deskApp(t)
	dir := conv.cfg.SandboxRoot
	src := touch(t, dir, "talk.mp4")
	out := touch(t, dir, "cut.mp4")

	a.autoOpenMedia(conv, videoRun("kinocut_video_trim",
		map[string]any{"input_path": src, "start": "0", "duration": "18"},
		map[string]any{"success": true, "output_path": out, "operation": "trim", "duration": 18.0},
	))

	for _, o := range *opened {
		if o.Plan != nil {
			t.Errorf("drew a plan from %+v with nothing having measured the source", o)
		}
	}
}

// With one, the bar is the call redrawn: the whole source, the stretch kept,
// and the scene marks the agent actually detected on it.
func TestVideoDeskDrawsThePlanFromWhatWasMeasured(t *testing.T) {
	a, conv, opened := deskApp(t)
	dir := conv.cfg.SandboxRoot
	src := touch(t, dir, "talk.mp4")
	out := touch(t, dir, "cut.mp4")

	a.autoOpenMedia(conv, videoRun("kinocut_video_info",
		map[string]any{"input_path": src},
		map[string]any{"success": true, "info": map[string]any{"duration": 45.0}},
	))
	a.autoOpenMedia(conv, videoRun("kinocut_video_detect_scenes",
		map[string]any{"input_path": src},
		map[string]any{"success": true, "scene_count": 2, "duration": 45.0, "scenes": []any{
			map[string]any{"start": 0.0, "end": 20.0},
			map[string]any{"start": 20.0, "end": 45.0},
		}},
	))
	a.autoOpenMedia(conv, videoRun("kinocut_video_trim",
		map[string]any{"input_path": src, "start": "00:00:05", "duration": "18"},
		map[string]any{"success": true, "output_path": out, "operation": "trim", "duration": 18.0},
	))

	var plan *MediaPlan
	for _, o := range *opened {
		if o.Role == "result" && o.Plan != nil {
			plan = o.Plan
		}
	}
	if plan == nil {
		t.Fatal("no plan on the result, with the source measured at 45s")
	}
	if plan.Total != 45 {
		t.Errorf("total = %v, want the 45 video_info reported", plan.Total)
	}
	if len(plan.Kept) != 1 || plan.Kept[0].Start != 5 || plan.Kept[0].End != 23 {
		t.Errorf("kept = %+v, want 5s to 23s off the arguments the model sent", plan.Kept)
	}
	if len(plan.Marks) != 2 {
		t.Errorf("marks = %+v, want the two scenes detect_scenes found", plan.Marks)
	}
}

// A merge lays the clips end to end at the lengths kinocut reported for them.
func TestVideoDeskDrawsAMergeFromTheClipLengths(t *testing.T) {
	a, conv, opened := deskApp(t)
	dir := conv.cfg.SandboxRoot
	one := touch(t, dir, "one.mp4")
	two := touch(t, dir, "two.mp4")
	out := touch(t, dir, "joined.mp4")

	for _, clip := range []struct {
		path string
		secs float64
	}{{one, 9}, {two, 6}} {
		a.autoOpenMedia(conv, videoRun("kinocut_video_info",
			map[string]any{"input_path": clip.path},
			map[string]any{"success": true, "info": map[string]any{"duration": clip.secs}},
		))
	}
	a.autoOpenMedia(conv, videoRun("kinocut_video_merge",
		map[string]any{"clips": []any{one, two}},
		map[string]any{"success": true, "output_path": out, "operation": "merge", "duration": 15.0},
	))

	var plan *MediaPlan
	for _, o := range *opened {
		if o.Path == out {
			plan = o.Plan
		}
	}
	if plan == nil || plan.Total != 15 || len(plan.Kept) != 2 {
		t.Fatalf("plan = %+v, want two clips totalling 15s", plan)
	}
	if plan.Kept[1].Start != 9 || plan.Kept[1].End != 15 {
		t.Errorf("second clip = %+v, want it to start where the first ended", plan.Kept[1])
	}
}

// One unmeasured clip shifts every clip after it, so the bar is not drawn at
// all rather than drawn in the wrong places.
func TestVideoDeskRefusesAMergeItCannotPlace(t *testing.T) {
	a, conv, opened := deskApp(t)
	dir := conv.cfg.SandboxRoot
	one := touch(t, dir, "one.mp4")
	two := touch(t, dir, "two.mp4")
	out := touch(t, dir, "joined.mp4")

	a.autoOpenMedia(conv, videoRun("kinocut_video_info",
		map[string]any{"input_path": one},
		map[string]any{"success": true, "info": map[string]any{"duration": 9.0}},
	))
	a.autoOpenMedia(conv, videoRun("kinocut_video_merge",
		map[string]any{"clips": []any{one, two}},
		map[string]any{"success": true, "output_path": out, "operation": "merge"},
	))

	for _, o := range *opened {
		if o.Plan != nil {
			t.Errorf("drew %+v with one clip never measured", o.Plan)
		}
	}
}

// A failed call has no file to show, whatever the transport thought of it.
func TestVideoDeskIgnoresAFailedCall(t *testing.T) {
	a, conv, opened := deskApp(t)
	src := touch(t, conv.cfg.SandboxRoot, "talk.mp4")

	a.autoOpenMedia(conv, videoRun("kinocut_video_trim",
		map[string]any{"input_path": src},
		map[string]any{"success": false, "error": map[string]any{"message": "bad timestamp"}},
	))

	if len(*opened) != 0 {
		t.Errorf("opened %+v for a call that failed", *opened)
	}
}

// The desk is the editor's, and only the editor's. Nothing else in the app can
// reach this event, and the check is the tool's own name.
func TestVideoDeskIgnoresEverybodyElse(t *testing.T) {
	a, conv, opened := deskApp(t)
	src := touch(t, conv.cfg.SandboxRoot, "talk.mp4")

	a.autoOpenMedia(conv, videoRun("write",
		map[string]any{"input_path": src},
		map[string]any{"success": true, "output_path": src},
	))

	if len(*opened) != 0 {
		t.Errorf("opened %+v for a tool that is not the editor's", *opened)
	}
}

// A .srt and a .json are results too, and neither is something to watch. A tab
// onto one is a tab the user has to close.
func TestVideoDeskOpensOnlyWhatThePanesDraw(t *testing.T) {
	a, conv, opened := deskApp(t)
	dir := conv.cfg.SandboxRoot
	src := touch(t, dir, "talk.mp4")
	srt := touch(t, dir, "talk.srt")

	a.autoOpenMedia(conv, videoRun("kinocut_video_generate_subtitles",
		map[string]any{"input_path": src},
		map[string]any{"success": true, "output_path": srt, "srt_path": srt, "operation": "generate_subtitles"},
	))

	for _, o := range *opened {
		if strings.EqualFold(filepath.Ext(o.Path), ".srt") {
			t.Errorf("opened %q, which no pane on this desk draws", o.Path)
		}
	}
}

// A read-only tool that echoes its input back as output_path must not
// re-announce the source as a result of itself.
func TestVideoDeskDoesNotEchoTheSourceAsAResult(t *testing.T) {
	a, conv, opened := deskApp(t)
	src := touch(t, conv.cfg.SandboxRoot, "talk.mp4")

	a.autoOpenMedia(conv, videoRun("kinocut_video_quality_check",
		map[string]any{"input_path": src},
		map[string]any{"success": true, "output_path": src, "operation": "quality_check"},
	))

	for _, o := range *opened {
		if o.Role == "result" {
			t.Errorf("announced %+v as a result of reading it", o)
		}
	}
}

// kinocut spells timestamps three ways and its own docs say so ("'00:02:15' or
// seconds as string like '10.5'"). A parser that read one of them would put
// the kept stretch in the wrong place on the bar without ever erroring.
func TestSecondsReadsEveryTimestampKinocutAccepts(t *testing.T) {
	for _, c := range []struct {
		in   any
		want float64
	}{
		{"90", 90},
		{"90.5", 90.5},
		{"01:30", 90},
		{"00:01:30.5", 90.5},
		{"1:00:00", 3600},
		{90.5, 90.5},
		{"", 0},
	} {
		if got := seconds(c.in); got != c.want {
			t.Errorf("seconds(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// The room's tool exists exactly where the editor's own tools do: the chair
// with the kinocut server placed for it. Every other session pays nothing for
// it — the desk pack's budget rule, kept by registration rather than by a
// description growing a caveat.
func TestCuttingRoomToolOnlyWhereTheEditorIs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	servers := `[{"name":"kinocut","command":["python","-m","kinocut","--mcp"],"for":["agent:editor"]}]`
	if err := os.WriteFile(filepath.Join(root, "mcp-servers.json"), []byte(servers), 0o644); err != nil {
		t.Fatal(err)
	}

	holds := func(chair string) bool {
		conv := &conversation{id: "s1", chair: chair}
		a := seed(&App{}, conv)
		for _, s := range a.workbenchSkills(conv, t.TempDir()) {
			if s.Name() == "cutting_room" {
				return true
			}
		}
		return false
	}
	if !holds("editor") {
		t.Error("the editor's chair does not hold cutting_room, with kinocut placed for it")
	}
	if holds("") {
		t.Error("a plain chat holds cutting_room — every session is paying for one agent's room")
	}
	if holds("doc") {
		t.Error("another agent's chair holds cutting_room without the editor's server")
	}
}

// The tool is one event through the desk's one door, stamped with its session.
func TestCuttingRoomToolOpensTheRoom(t *testing.T) {
	var got []string
	a := seed(&App{emit: func(event string, data ...any) {
		if len(data) > 0 {
			if p, ok := data[0].(map[string]string); ok {
				got = append(got, event+" for "+p["sessionId"])
			}
		}
	}}, &conversation{id: "s1"})
	a.ctx = t.Context()

	out, err := (&cuttingRoomSkill{app: a, conv: a.cur()}).open()
	if err != nil || !out.Success {
		t.Fatalf("open() = %+v, %v", out, err)
	}
	if len(got) != 1 || got[0] != "workbench:open-cutroom for s1" {
		t.Errorf("emitted %v, want the room event stamped with its session", got)
	}
}

// The Go list of what auto-opens and the frontend's list of what it can draw
// are one decision in two files, and the day they disagree is the day a clip
// opens onto a pane that renders nothing. Read out of the store rather than
// restated here, so this measures the real table.
func TestDeskMediaMatchesThePanes(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("frontend", "src", "lib", "stores", "workbench.svelte.ts"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)viewByExt: Record<string, FileView> = \{(.*?)\n\}`).FindSubmatch(src)
	if block == nil {
		t.Fatal("viewByExt is not where this test looks for it — the store was restructured")
	}
	pairs := regexp.MustCompile(`(\w+):\s*'(image|video|audio|pdf)'`).FindAllStringSubmatch(string(block[1]), -1)
	if len(pairs) == 0 {
		t.Fatal("viewByExt parsed to nothing")
	}
	for _, p := range pairs {
		ext, view := "."+p[1], p[2]
		// pdf is deliberately out: a report out of a video editor is reading
		// rather than watching (deskMediaExts says so).
		want := view != "pdf"
		if deskMediaExts[ext] != want {
			t.Errorf("%s is %q to the panes but %v to the desk", ext, view, deskMediaExts[ext])
		}
	}
	for ext := range deskMediaExts {
		found := false
		for _, p := range pairs {
			if "."+p[1] == ext {
				found = true
			}
		}
		if !found {
			t.Errorf("the desk auto-opens %s and no pane draws it", ext)
		}
	}
}
