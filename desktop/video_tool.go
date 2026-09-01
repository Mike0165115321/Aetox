package main

// The agent's hands on the scene renderer.
//
// **Why this is a tool of ours and not somebody else's MCP server.** Every
// external program Aetox uses is reached the same way: a Go skill runs it and
// names its parameters after what they mean. `image_ocr` knows tesseract's
// flags, `video_ocr` knows ffmpeg's, `audio_transcribe` knows whisper's, and
// none of the three asks a model to remember a command line. Making a video was
// about to be the first exception — routed through kinocut, a Python MCP server
// whose own job is cutting footage that already exists, which then forwards to
// the same Hyperframes CLI this file calls directly. Owner, 30 ส.ค.: *"มันคนละ
// ส่วนกัน ทำไมถึงเอามาปนกัน"*. So: making a video talks to Hyperframes, cutting
// one talks to kinocut, and neither carries the other's runtime.
//
// **Three actions, because the job has three moves.** Take a scene out of the
// library and make it a project, look at it before spending the render, then
// render. That is the loop Hyperframes' own documentation describes, and the
// middle one is the one an agent skips and should not: a render is minutes of
// somebody's machine, and `check` opens the page once and reports the text that
// ran past the edge of the frame.
//
// **What this file does NOT do** is decide anything about the video. Length,
// wording, which scene, how many — those are the agent's judgement, and they
// arrive as parameters. This file resolves paths, runs one program, and reports
// what came back.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/proc"
	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/statereport"
	"github.com/Mikedev115/Aetox/internal/subagent"
)

// videoToolName is the one name this pack answers to, spelled once for the same
// reason browserToolName is.
const videoToolName = "video"

const (
	// The agent whose shelf the scene library sits on, and the skill on it.
	// Named rather than searched: a library that could be found under any agent
	// is a library any agent could quietly render from.
	videoLibraryAgent = "video"
	videoLibrarySkill = "video-templates"

	// Where the motion scenes live inside that skill. The still ones are
	// deliberately not reachable here — they do not move, so rendering one
	// produces a video of a photograph, which is a thing somebody asked for by
	// mistake every time.
	videoLibraryDir = "motion"

	// A render is the longest thing this app runs on purpose. Bounded anyway,
	// because a scene that has stopped answering should end as an error rather
	// than as a chat that never comes back.
	videoRenderTimeout = 30 * time.Minute
	// Checking opens one browser session and seeks through the scene once.
	videoCheckTimeout = 5 * time.Minute
)

type videoToolSkill struct {
	app *App
	// actions this caller may use, nil for all of them. Set only by Narrow.
	actions []string
}

func (s *videoToolSkill) allowedActions() []string {
	if s == nil || len(s.actions) == 0 {
		out := make([]string, 0, len(skill.PackedCalls(videoToolName)))
		for _, call := range skill.PackedCalls(videoToolName) {
			out = append(out, call.Action)
		}
		return out
	}
	return s.actions
}

func (s *videoToolSkill) Actions() []string { return skill.PackedActions(videoToolName) }

// Narrow hands back a scene tool offering only the named actions — a copy, for
// the same shared-registry reason the browser's and the shell's are.
func (s *videoToolSkill) Narrow(named []string) skill.Skill {
	narrowed := *s
	want := map[string]bool{}
	for _, n := range named {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var actions []string
	for _, call := range skill.PackedCalls(videoToolName) {
		if want[call.Permission] {
			actions = append(actions, call.Action)
		}
	}
	if len(actions) == 0 {
		return s // silence is the whole tool, not an empty one
	}
	narrowed.actions = actions
	return &narrowed
}

func (*videoToolSkill) Name() string { return videoToolName }

func (*videoToolSkill) Description() string {
	return "สร้างคลิปจากฉาก HTML — หยิบฉากจากคลังมาเป็นโปรเจกต์ ตรวจก่อนเรนเดอร์ แล้วเรนเดอร์เป็นไฟล์วิดีโอ"
}

func (s *videoToolSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()
	// Signatures only. Every "when should I reach for this" sentence belongs in
	// Guidance, sent once on the first call — the same split browser_tool.go
	// made when its entry went from 766 tokens to a fifth of that.
	lines := map[string]string{
		"new":    "`new` (template, path, seconds?, footage?) — copy a scene out of the library into its own project folder, ready to edit.",
		"check":  "`check` (path) — open the project once and report runtime errors, text that overflows the frame, and contrast failures.",
		"render": "`render` (path, output?, seconds?, fps?, quality?, composition?, proof?) — render the project to a video file; a fresh check report rides along, and proof:true also reads the on-screen text back via OCR in the same reply.",
	}
	var b strings.Builder
	b.WriteString("Build a video from an HTML scene. Actions:\n")
	for _, action := range allowed {
		b.WriteString(lines[action] + "\n")
	}
	b.WriteString("\nEvery path is relative to the project root. " +
		"Read the `video-templates` skill for what is in the library and what each scene needs.")

	return toolDef(videoToolName, b.String(), map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":      map[string]any{"type": "string", "enum": allowed},
			"template":    map[string]any{"type": "string"},
			"path":        map[string]any{"type": "string"},
			"output":      map[string]any{"type": "string"},
			"footage":     map[string]any{"type": "string"},
			"composition": map[string]any{"type": "string"},
			"seconds":     map[string]any{"type": "number"},
			"fps":         map[string]any{"type": "integer"},
			"quality":     map[string]any{"type": "string", "enum": []string{"draft", "standard", "high"}},
			"proof":       map[string]any{"type": "boolean"},
		},
		"required": []string{"action"},
	})
}

func (s *videoToolSkill) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	return s.run(ctx, args)
}

func (s *videoToolSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	return s.run(ctx, map[string]any(input))
}

func (s *videoToolSkill) run(ctx context.Context, args map[string]any) (skill.Output, error) {
	action := strings.ToLower(strings.TrimSpace(argString(args, "action")))
	if action == "" {
		err := fmt.Errorf("video ต้องบอกว่าจะทำอะไร: %s", strings.Join(s.allowedActions(), ", "))
		return videoFailed("video", err), err
	}
	allowed := false
	for _, a := range s.allowedActions() {
		if a == action {
			allowed = true
			break
		}
	}
	if !allowed {
		err := fmt.Errorf("video %s ใช้ไม่ได้ที่นี่", action)
		return videoFailed("video "+action, err), err
	}
	switch action {
	case "new":
		return s.newProject(args)
	case "check":
		return s.check(ctx, args)
	case "render":
		return s.render(ctx, args)
	}
	err := fmt.Errorf("video ไม่รู้จักคำสั่ง %q", action)
	return videoFailed("video "+action, err), err
}

// ---------------------------------------------------------------------------
// new
// ---------------------------------------------------------------------------

// newProject copies one scene out of the library and leaves a folder the agent
// can edit and the renderer can read.
//
// A copy rather than the agent writing the files itself, and the reason is
// bytes: nine of the library's scenes carry mp3 and png beside their HTML, and
// a model asked to reproduce those is a model asked to invent them.
func (s *videoToolSkill) newProject(args map[string]any) (skill.Output, error) {
	template := strings.TrimSpace(argString(args, "template"))
	dest := strings.TrimSpace(argString(args, "path"))
	command := "video new " + template
	if template == "" || dest == "" {
		err := fmt.Errorf("video new ต้องบอกทั้ง template และ path")
		return videoFailed(command, err), err
	}
	root, err := s.sandbox()
	if err != nil {
		return videoFailed(command, err), err
	}
	// The same placement rule every file-producing tool follows (write.go): an
	// unfocused session's project lands in output/<session>, not at the root.
	// This tool skipped the rule, so every session's video work piled up in one
	// folder and a test in one chat globbed up the renders of another — the
	// receipt below echoes the placed path, and check/render resolve it back.
	dest = skill.PlacedWrite(s.app.outputSubdir, dest)
	full, err := safeSandboxPath(root, dest)
	if err != nil {
		return videoFailed(command, err), err
	}

	written, err := videoCopyTemplate(template, full)
	if err != nil {
		return videoFailed(command, err), err
	}

	// The two slots the library ships unfilled. Upstream fills them when it
	// scaffolds a project from a video file; nothing fills them on this path,
	// so a scene left with the placeholders in it renders with a duration of
	// the literal string "__VIDEO_DURATION__".
	seconds := argFloat(args, "seconds")
	if seconds <= 0 {
		seconds = videoDefaultSeconds(full)
	}
	filled, err := videoPatchPlaceholders(full, seconds, strings.TrimSpace(argString(args, "footage")))
	if err != nil {
		return videoFailed(command, err), err
	}

	localised, err := videoUseLocalGSAP(full)
	if err != nil {
		return videoFailed(command, err), err
	}

	report := fmt.Sprintf("วางฉาก %s ไว้ที่ %s แล้ว (%d ไฟล์", template, dest, written)
	if filled > 0 {
		report += fmt.Sprintf(", แทนค่าความยาว %.4g วินาที ใน %d ไฟล์", seconds, filled)
	}
	if localised > 0 {
		report += fmt.Sprintf(", ชี้ GSAP ไปที่ไฟล์ในเครื่องใน %d ไฟล์", localised)
	}
	report += ")\n" + videoProjectInventory(full)
	report += "\nแก้ข้อความข้างบนให้เป็นของงานนี้ทั้งหมด (ที่เห็นคือสำเนาตัวอย่าง) แล้วเรียก video check ก่อนเรนเดอร์" +
		"\nฟอนต์ของฉากไม่มีชุดตัวอักษรไทย — ข้อความไทยจะออกมาเป็นฟอนต์ระบบ ถ้าไม่ใช่ที่ตั้งใจให้เพิ่มฟอนต์ไทยใน <link> เอง"
	return skill.Output{Name: "video_new", Command: command, Content: report, Success: true}, nil
}

// videoProjectInventory reads the freshly copied project so the agent does not
// have to. Measured 31 ส.ค. (session 164630): after `video new` the agent spent
// six calls of glob/list/read learning what the copy held, and eight
// `skill_view` calls before that reading sub-scenes in the library to find
// where the words live. The tool wrote every one of those files; it can say
// what is in them.
func videoProjectInventory(dir string) string {
	var b strings.Builder
	b.WriteString("ไฟล์ในโปรเจกต์ และข้อความที่อยู่ในแต่ละไฟล์:")
	var others []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "vendor/") {
			return nil // the GSAP copy `video new` itself put there
		}
		if !strings.EqualFold(filepath.Ext(path), ".html") {
			others = append(others, rel)
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		snippets := videoVisibleText(string(data), 10)
		if len(snippets) == 0 {
			b.WriteString("\n- " + rel + " — (ไม่มีข้อความ)")
			return nil
		}
		b.WriteString("\n- " + rel + ": " + strings.Join(snippets, " | "))
		return nil
	})
	if len(others) > 0 {
		b.WriteString("\n- ไฟล์ประกอบ: " + strings.Join(others, ", "))
	}
	return b.String()
}

// videoVisibleText is the words a browser would paint, pulled out of markup the
// cheap way: style, script and comments dropped, tags stripped, whitespace
// folded. Cheap is enough — this feeds an editor deciding what to rewrite, not
// a renderer deciding where it lands.
func videoVisibleText(markup string, max int) []string {
	for _, re := range videoInvisibleParts {
		markup = re.ReplaceAllString(markup, " ")
	}
	markup = videoTagRe.ReplaceAllString(markup, "\n")
	replacer := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&nbsp;", " ")
	var out []string
	for _, line := range strings.Split(markup, "\n") {
		line = strings.Join(strings.Fields(replacer.Replace(line)), " ")
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > 80 {
			line = string(r[:80]) + "…"
		}
		out = append(out, line)
		if len(out) == max {
			out = append(out, "…")
			break
		}
	}
	return out
}

var (
	videoInvisibleParts = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?s)<!--.*?-->`),
		regexp.MustCompile(`(?is)<(?:head|title)[^>]*>.*?</(?:head|title)>`),
	}
	videoTagRe = regexp.MustCompile(`<[^>]*>`)
)

// videoCopyTemplate resolves a library name to whichever shape it has.
//
// Thirteen of the twenty-two motion scenes are one self-contained file and nine
// are folders with their own sub-scenes and assets. Both become a project
// folder here, so nothing downstream has to know which kind it started as —
// that difference belongs to the library, not to the renderer.
//
// The shape is decided by asking the shelf, not by which copy fails. The old
// try-the-folder-then-fall-through order turned any folder-copy failure into
// "ไม่มีฉากชื่อ ... ในคลัง" — including the one where the destination already
// existed, which sent an agent retrying a template the refusal itself listed
// (tool_runs, 31 ส.ค. 16:49). A refusal has to name the caller's actual
// mistake, or it manufactures the next one.
func videoCopyTemplate(template, dest string) (int, error) {
	// The path shapes are accepted, because WE teach them: the SKILL.md table
	// spells every scene `motion/<name>.html` or `motion/<name>/index.html`,
	// and agents copy that spelling verbatim into `template` — three times in
	// one evening (problem queue, 31 ส.ค.), each answered by a refusal that
	// blamed the caller for reading our own index. One unambiguous meaning,
	// so normalise instead of refusing.
	template = strings.TrimPrefix(strings.ReplaceAll(template, `\`, "/"), videoLibraryDir+"/")
	template = strings.TrimSuffix(template, "/index.html")
	template = strings.TrimSuffix(template, ".html")
	if strings.ContainsAny(template, `/\`) {
		return 0, fmt.Errorf("template คือชื่อฉากในคลัง ไม่ใช่พาธ — ดูรายชื่อในตาราง SKILL.md ของ video-templates")
	}
	if _, err := os.Stat(dest); err == nil {
		return 0, fmt.Errorf("%s มีอยู่แล้ว", filepath.Base(dest))
	}
	shelf, err := subagent.ListSkillDir(videoLibraryAgent, videoLibrarySkill, videoLibraryDir)
	if err != nil {
		return 0, err
	}
	dir := videoLibraryDir + "/" + template
	if !slices.Contains(shelf, template) {
		if !slices.Contains(shelf, template+".html") {
			return 0, fmt.Errorf("ไม่มีฉากชื่อ %s ในคลัง — มีให้เลือกคือ %s", template, videoLibraryNames())
		}
		data, err := subagent.ReadSkillFile(videoLibraryAgent, videoLibrarySkill, dir+".html")
		if err != nil {
			return 0, err
		}
		return videoWriteFlatScene(dest, data)
	}
	return subagent.CopySkillDir(videoLibraryAgent, videoLibrarySkill, dir, dest)
}

// videoWriteFlatScene turns one self-contained library file into a project
// folder.
func videoWriteFlatScene(dest string, data []byte) (int, error) {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, err
	}
	// index.html, because that is the name the renderer looks for in a project
	// directory. The library's flat scenes are named after themselves, which is
	// right on a shelf and wrong in a project.
	if err := os.WriteFile(filepath.Join(dest, "index.html"), data, 0o644); err != nil {
		return 0, err
	}
	return 1, nil
}

// videoLibraryNames is the shelf, spelled the way `template` has to be spelled.
//
// It is in the refusal rather than in the tool's description on purpose. The
// listing costs tokens on every request if it lives in the schema, and it is
// only ever needed by a caller that has already guessed wrong — which is any
// agent that does not carry the `video-templates` skill and therefore cannot
// take the description's advice to go and read it. Guess once, be told, get it
// right, instead of a loop of "no such scene" with nowhere to look.
func videoLibraryNames() string {
	entries, err := subagent.ListSkillDir(videoLibraryAgent, videoLibrarySkill, videoLibraryDir)
	if err != nil {
		return "เปิดสกิล video-templates ดูรายชื่อ"
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e, ".html"))
	}
	return strings.Join(names, ", ")
}

var (
	videoDurationSlot = "__VIDEO_DURATION__"
	videoFootageSlot  = "__VIDEO_SRC__"
	// The four shapes upstream strips when a scene has a footage slot and
	// nobody supplied footage. Copied from hyperframes' own patchVideoSrc
	// (Apache-2.0, HeyGen) rather than reasoned out, because a scene left
	// holding <video src="__VIDEO_SRC__"> renders a broken element rather than
	// nothing, and they already found that out.
	videoFootageStrip = []*regexp.Regexp{
		regexp.MustCompile(`(?s)<video[^>]*src="__VIDEO_SRC__"[^>]*>.*?</video>`),
		regexp.MustCompile(`<video[^>]*src="__VIDEO_SRC__"[^>]*>`),
		regexp.MustCompile(`(?s)<audio[^>]*src="__VIDEO_SRC__"[^>]*>.*?</audio>`),
		regexp.MustCompile(`<audio[^>]*src="__VIDEO_SRC__"[^>]*>`),
	}
	videoDefaultSecRe = regexp.MustCompile(`(?m)^\s*default_sec:\s*([0-9.]+)`)
)

// videoDefaultSeconds reads the length the scene's own manifest suggests.
//
// A regex rather than a YAML parser, and deliberately: one number is wanted out
// of a file this app never writes, and adding a YAML dependency to read it
// would be the larger change. A miss falls through to the caller's default,
// which is a number the agent was going to have to justify anyway.
func videoDefaultSeconds(dir string) float64 {
	data, err := os.ReadFile(filepath.Join(dir, "template.html-video.yaml"))
	if err != nil {
		return 0
	}
	m := videoDefaultSecRe.FindSubmatch(data)
	if m == nil {
		return 0
	}
	seconds, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0
	}
	return seconds
}

// videoUseLocalGSAP points a copied project's script tags at the copy of GSAP
// on this machine, and answers with how many files it rewrote.
//
// **Why the rewrite happens here and not in the library.** Nine scenes load
// their animation library from a CDN, which means nine of the twenty-two need
// the network up at render time or they produce a still picture with nothing in
// the output saying why. The fix cannot be to edit those nine in place: the file
// that arrives on disk has to keep working when somebody opens it in a browser
// straight out of the shelf, and CREDITS.md promises the folders are
// byte-for-byte upstream's. So the library keeps the CDN address and the copy
// gets the local one — the same split `__VIDEO_DURATION__` already uses.
//
// **A missing GSAP is not an error.** It leaves the CDN address in place, which
// is exactly what upstream shipped and works whenever there is a network. The
// tool says what it did; it does not refuse to make a project because an
// optional download has not happened.
func videoUseLocalGSAP(dir string) (int, error) {
	local := gsapFile()
	if local == "" {
		return 0, nil
	}
	// One copy per project, beside the scene rather than inside it, so a
	// composition folder's own files are still only the ones upstream wrote.
	vendor := filepath.Join(dir, "vendor")
	copied := false
	rewritten := 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".html") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if !strings.Contains(text, gsapCDNURL) {
			return nil
		}
		if !copied {
			if err := os.MkdirAll(vendor, 0o755); err != nil {
				return err
			}
			blob, err := os.ReadFile(local)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(vendor, "gsap.min.js"), blob, 0o644); err != nil {
				return err
			}
			copied = true
		}
		// Relative to the file doing the loading, because the renderer serves
		// the project directory and a sub-composition sits one level down.
		rel, err := filepath.Rel(filepath.Dir(path), filepath.Join(vendor, "gsap.min.js"))
		if err != nil {
			return err
		}
		href := filepath.ToSlash(rel)
		if !strings.HasPrefix(href, ".") {
			href = "./" + href
		}
		if err := os.WriteFile(path, []byte(strings.ReplaceAll(text, gsapCDNURL, href)), 0o644); err != nil {
			return err
		}
		rewritten++
		return nil
	})
	return rewritten, err
}

// videoPatchPlaceholders fills the two slots across every HTML file in the
// project and answers with how many files it touched.
func videoPatchPlaceholders(dir string, seconds float64, footage string) (int, error) {
	touched := 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".html") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if !strings.Contains(text, videoDurationSlot) && !strings.Contains(text, videoFootageSlot) {
			return nil
		}
		if footage != "" {
			text = strings.ReplaceAll(text, videoFootageSlot, footage)
		} else {
			for _, re := range videoFootageStrip {
				text = re.ReplaceAllString(text, "")
			}
		}
		if seconds > 0 {
			text = strings.ReplaceAll(text, videoDurationSlot, strconv.FormatFloat(seconds, 'g', -1, 64))
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return err
		}
		touched++
		return nil
	})
	return touched, err
}

// ---------------------------------------------------------------------------
// check and render
// ---------------------------------------------------------------------------

func (s *videoToolSkill) check(ctx context.Context, args map[string]any) (skill.Output, error) {
	dir := strings.TrimSpace(argString(args, "path"))
	command := "video check " + dir
	full, err := s.projectDir(dir)
	if err != nil {
		return videoFailed(command, err), err
	}
	// **Findings are the answer, not a failure.** `check` exits non-zero the
	// moment it has anything to say, and everything it has to say is on stdout —
	// so treating the exit code as an error threw the report away and handed the
	// agent the last line of stderr instead, which is a note about which fonts
	// were fetched. Measured 31 ส.ค.: `statement-title` came back as
	// "check ไม่สำเร็จ: Fetched 5 font face(s) for Space Grotesk", with five
	// layout findings on the floor.
	//
	// It is also not a gate. Half of what the layout pass reports on this
	// library is deliberate — tilted headline lines that overlap each other are
	// the design of the scene — so the judgement belongs to the agent reading
	// it, the way it belongs to a person looking at the frame. The report ends
	// with its own verdict, and videoTail keeps the end.
	report, ran, err := runHyperframesReport(ctx, videoCheckTimeout, "check", full)
	if !ran {
		return videoFailed(command, err), err
	}
	return skill.Output{Name: "video_check", Command: command, Content: report, Success: true}, nil
}

func (s *videoToolSkill) render(ctx context.Context, args map[string]any) (skill.Output, error) {
	dir := strings.TrimSpace(argString(args, "path"))
	command := "video render " + dir
	full, err := s.projectDir(dir)
	if err != nil {
		return videoFailed(command, err), err
	}

	argv := []string{"render", full}
	out := strings.TrimSpace(argString(args, "output"))
	if out == "" {
		// Beside the project, named after it. Upstream's own default is
		// renders/<name>.mp4 inside the project, which buries the one file the
		// user actually wants under the working folder.
		out = filepath.Base(full) + ".mp4"
	}
	root, err := s.sandbox()
	if err != nil {
		return videoFailed(command, err), err
	}
	// Beside the project as projectDir RESOLVED it, not as the caller spelled
	// it — with the session placement above, those are different folders, and
	// the render must land next to the project it came from.
	outFull, err := safeSandboxPath(root, filepath.Join(filepath.Dir(skill.PlacedPath(root, s.app.outputSubdir, dir)), out))
	if err != nil {
		return videoFailed(command, err), err
	}
	argv = append(argv, "--output", outFull)

	if fps := argInt(args, "fps"); fps > 0 {
		argv = append(argv, "--fps", strconv.Itoa(fps))
	}
	if quality := strings.TrimSpace(argString(args, "quality")); quality != "" {
		argv = append(argv, "--quality", quality)
	}
	if composition := strings.TrimSpace(argString(args, "composition")); composition != "" {
		argv = append(argv, "--composition", composition)
	}

	// The check rides inside the render, and it gates nothing. Measured on the
	// clean run of 31 ส.ค. (session 192150): every clip pays a final
	// check-then-render as two adjacent model rounds, the second check warm at
	// ~9s and its round carrying ~90k of context. Folding it here makes
	// "edit → render" the whole closing move: the findings arrive beside the
	// file in one reply, and the agent that wants a look BEFORE spending the
	// render still has `video check` untouched. Not a gate, because half of
	// what the layout pass reports on this library is the design of the scene
	// (see check above) — a render blocked on a deliberate overlap would be
	// the tool overruling the craft. Best effort: a check that cannot run
	// costs the render nothing.
	checkReport, checkRan, _ := runHyperframesReport(ctx, videoCheckTimeout, "check", full)

	if _, err := runHyperframes(ctx, videoRenderTimeout, argv...); err != nil {
		return videoFailed(command, err), err
	}
	info, err := os.Stat(outFull)
	if err != nil || info.Size() == 0 {
		// The renderer said it succeeded and there is no file, or an empty one.
		// Reporting that as success is how a chat ends with a path nobody can
		// open.
		failure := fmt.Errorf("เรนเดอร์จบแล้วแต่ไม่มีไฟล์ที่ %s", out)
		return videoFailed(command, failure), failure
	}
	report := fmt.Sprintf("เรนเดอร์เสร็จ: %s (%.1f MB)", out, float64(info.Size())/(1<<20))
	if checkRan && strings.TrimSpace(checkReport) != "" {
		report += "\nรายงาน check ก่อนเรนเดอร์ (อ่านแบบเดียวกับ video check — ที่ตั้งใจซ้อนกันไม่ใช่ปัญหา):\n" + checkReport
	}

	// proof folds the read-back into the same reply. Rendering a draft and then
	// OCR-ing it were two model turns for one deterministic sequence — the
	// sequence belongs here, the judgement on what the letters say stays with
	// the agent. Best effort: a machine that cannot OCR still rendered.
	if argBool(args, "proof") {
		if text, ocrErr := skill.OCRVideoFile(ctx, outFull, 1); ocrErr != nil {
			report += "\nอ่านข้อความบนจอกลับไม่ได้: " + ocrErr.Error()
		} else {
			report += "\nข้อความบนจอ (OCR ทุก 1 วิ):\n" + text
		}
	}
	report += "\nเปิดให้ผู้ใช้ดูด้วย desk open"
	return skill.Output{Name: "video_render", Command: command, Content: report, Success: true}, nil
}

// projectDir resolves a project path and refuses one that is not a project.
//
// The existence check is not politeness. `render` on a directory with no
// index.html spends a browser launch to find out, and the sentence it comes
// back with is about a composition rather than about a wrong path.
func (s *videoToolSkill) projectDir(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("ต้องบอก path ของโปรเจกต์")
	}
	root, err := s.sandbox()
	if err != nil {
		return "", err
	}
	// The read side of newProject's placement: a caller repeating the short
	// name it originally asked for still finds the project the session folder
	// holds (write.go's PlacedPath — literal path wins, fallback only fires
	// when nothing is there).
	dir = skill.PlacedPath(root, s.app.outputSubdir, dir)
	full, err := safeSandboxPath(root, dir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(full, "index.html")); err != nil {
		return "", fmt.Errorf("%s ไม่ใช่โปรเจกต์ฉาก — ไม่มี index.html อยู่ในนั้น", dir)
	}
	return full, nil
}

func (s *videoToolSkill) sandbox() (string, error) {
	root := strings.TrimSpace(s.app.cur().cfg.SandboxRoot)
	if root == "" {
		return "", fmt.Errorf("ยังไม่ได้เปิดโปรเจกต์")
	}
	return root, nil
}

// ---------------------------------------------------------------------------
// running the renderer
// ---------------------------------------------------------------------------

// runHyperframes runs one command and hands back what it said.
//
// The bundled Node running the bundled entry point, never a shim: npm's `.cmd`
// wrapper looks for `node` on PATH, and the whole point of a portable bundle is
// that nothing has to be on PATH for it to work.
func runHyperframes(ctx context.Context, budget time.Duration, argv ...string) (string, error) {
	stdout, stderr, err := spawnHyperframes(ctx, budget, argv...)
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail == "" {
			detail = err.Error()
		}
		if statereport.Is(err) {
			return "", err
		}
		return "", fmt.Errorf("%s ไม่สำเร็จ: %s", argv[0], videoTail(detail))
	}
	answer := strings.TrimSpace(stdout)
	if answer == "" {
		answer = strings.TrimSpace(stderr)
	}
	return videoTail(answer), nil
}

// runHyperframesReport is the same run for a command whose non-zero exit means
// "I found something" rather than "I failed".
//
// `ran` is the question the caller actually has: did the program get to say
// anything. False means the renderer is missing, could not start, or ran out of
// its budget — and then err is the sentence to show. True means there is a
// report, whatever the exit code was.
func runHyperframesReport(ctx context.Context, budget time.Duration, argv ...string) (report string, ran bool, err error) {
	stdout, stderr, runErr := spawnHyperframes(ctx, budget, argv...)
	said := strings.TrimSpace(stdout)
	if said == "" {
		said = strings.TrimSpace(stderr)
	}
	if runErr != nil {
		var exit *exec.ExitError
		// An exit code with something written under it is the program
		// answering. Anything else — a missing renderer, a binary that would
		// not start, a deadline — is not.
		if !errors.As(runErr, &exit) || said == "" {
			return "", false, runErr
		}
	}
	return videoTail(said), true, nil
}

// spawnHyperframes runs the renderer once and hands back both streams.
func spawnHyperframes(ctx context.Context, budget time.Duration, argv ...string) (stdout, stderr string, err error) {
	node, entry := hyperframesParts()
	if node == "" {
		return "", "", statereport.New("ยังเรนเดอร์ไม่ได้ เพราะเครื่องนี้ยังไม่มีตัวเรนเดอร์ฉาก — เปิดหน้างานวิดีโอแล้วกดติดตั้ง")
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	cmd := exec.CommandContext(ctx, node, append([]string{entry}, argv...)...)
	proc.HideConsole(cmd)
	root, rootErr := config.DataRoot()
	cmd.Env = append(os.Environ(), videoEnvPairs(root, rootErr == nil)...)
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs

	runErr := cmd.Run()
	// A budget that ran out is its own sentence: the exit error alone says
	// "signal: killed", which reads as a crash rather than as a wait nobody
	// wanted to keep waiting for.
	if ctx.Err() != nil {
		return out.String(), errs.String(), fmt.Errorf("%s ใช้เวลาเกิน %s จึงหยุดไว้", argv[0], budget)
	}
	return out.String(), errs.String(), runErr
}

// videoEnvPairs is hyperframesEnvironment in the shape exec wants.
func videoEnvPairs(root string, haveRoot bool) []string {
	env := hyperframesEnvironment(root, haveRoot, findProgram)
	pairs := make([]string, 0, len(env))
	for key, value := range env {
		pairs = append(pairs, key+"="+value)
	}
	return pairs
}

// videoTail keeps the end of a long answer rather than the start.
//
// A renderer's useful sentence is its last one — what failed, or where the file
// landed. Truncating from the front would keep the banner and drop the answer.
func videoTail(s string) string {
	const most = 4000
	if len(s) <= most {
		return s
	}
	return "…\n" + s[len(s)-most:]
}

func videoFailed(command string, err error) skill.Output {
	return skill.Output{
		Name:    "video",
		Command: command,
		Content: err.Error(),
		Stderr:  err.Error(),
		Success: false,
		// A missing renderer is a fact about this machine rather than about
		// anything the agent did, and the learning floor reads the difference.
		FromWorld: statereport.Is(err),
	}
}

// argString, argFloat and argInt read one loosely-typed argument. A model sends
// a number as a string as often as not, and refusing that is refusing the call
// over its punctuation.
func argString(args map[string]any, key string) string {
	switch v := args[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case json.Number:
		return v.String()
	}
	return ""
}

func argFloat(args map[string]any, key string) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

func argInt(args map[string]any, key string) int { return int(argFloat(args, key)) }

func argBool(args map[string]any, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return false
}
