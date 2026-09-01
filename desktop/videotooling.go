package main

// What this machine can already do with video, and how the editor is spawned.
//
// **Nothing here starts the editor.** Owner, 30 ส.ค.: *"kino จะทำงานก็ต่อเมื่อ
// เอเจนเรียกนะครับ ไม่ผูกกับระบบนะครับ"* — kinocut is a program the agent drives
// over MCP, and Aetox starting it to interview it makes every page open wait on
// a third party's startup. It did, briefly, once per agent card.
//
// One program is run in this file and only one: `ffmpeg -encoders`, inside the
// readiness check, when a person pressed the button that asks the question it
// answers. Everything else is a filesystem lookup.
//
// **Installing is internal/capability's job.** There is one way Aetox puts a
// program on someone's machine — a pinned, SHA256-checked archive fetched into
// DataRoot, no elevation, no package manager, nothing written to PATH — and
// every button here presses that one. A second mechanism beside it is the shape
// of the mistake that got the NSIS installer classified as Wacapew.C!ml.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/capability"
	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/proc"
)

// VideoEditorServer is the MCP server name the two video agents name in their
// own `needs:` lines, and the name the shelf entry in Settings installs under.
//
// Nothing in this file answers "is it connected" — `AgentNeeds` already does,
// off the agents' own `needs:` lines and the server's `for:` placement. Two
// questions, two owners: whether the program is on the machine is this file's,
// whether an agent can reach it is that one's.
const VideoEditorServer = "kinocut"

// The three capability ids video work installs under, and the split is the
// point: one job's download must not drag the other job's along.
//
//   - VideoEncodeCapability is the ffmpeg both jobs encode with.
//   - VideoMakeCapability is the scene renderer and the browser it drives.
//   - VideoEditCapability is the editor for footage that already exists.
//
// They were one id called "video" until 30 ส.ค., which meant pressing install
// on the card that makes videos also fetched a Python interpreter for cutting
// footage the user had not shot yet.
const (
	VideoEncodeCapability = "video"
	VideoMakeCapability   = "video-make"
	VideoEditCapability   = "video-edit"

	// VideoEditorCapability is the old name for the shared one, kept because
	// the editor's own card and Settings still speak it.
	VideoEditorCapability = VideoEncodeCapability
)

// videoEditorNeeds is what the editor cannot work without.
//
// Written here rather than asked of the editor, deliberately — see the file
// comment. ffprobe is listed beside ffmpeg because they ship together, so a
// tree carrying only one is a broken install rather than a partial one.
var videoEditorNeeds = []string{"ffmpeg", "ffprobe"}

// ---------------------------------------------------------------------------
// Finding things
// ---------------------------------------------------------------------------

// ffmpegSearchDirs is everywhere a real ffmpeg tends to be on Windows, past
// PATH — which is the whole point of the list.
//
// A machine that installed ffmpeg through scoop, chocolatey or winget very
// often has it working in a shell and invisible to a GUI process started before
// the install, because PATH is read at launch. Checking only PATH and then
// offering to download 90MB the user already has is exactly the wrong answer
// the readiness panel exists to avoid (owner, 30 ส.ค.: *"เผื่อมันมีอยู่แล้วแต่
// ระบบตรวจไม่เจออีก"*).
//
// Aetox's own GPL copy is first because it is the one that was pinned and
// proved.
//
// **Aetox's own LGPL copy is deliberately not in this list at all.** It is a
// real ffmpeg and it is right there, and finding it is worse than finding
// nothing: it carries no libx264, so the panel drew a green tick beside a
// program that cannot render, and with nothing left to offer the reader there
// was no way forward from that screen. A copy that can never do this job is not
// a copy of this job's tool.
func ffmpegSearchDirs() []string {
	var dirs []string
	root, rootErr := config.DataRoot()
	if rootErr == nil {
		dirs = append(dirs, filepath.Join(root, "tools", "ffmpeg-gpl", "bin"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "scoop", "shims"))
	}
	if pd := os.Getenv("ProgramData"); pd != "" {
		dirs = append(dirs, filepath.Join(pd, "chocolatey", "bin"))
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		dirs = append(dirs, filepath.Join(la, "Microsoft", "WinGet", "Links"))
	}
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(key); pf != "" {
			dirs = append(dirs, filepath.Join(pf, "ffmpeg", "bin"))
		}
	}
	return dirs
}

// findProgram looks in Aetox's own folder, then PATH, then the places an
// installer would have left it. Empty when it is genuinely not here.
func findProgram(name string) string {
	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	dirs := ffmpegSearchDirs()
	if len(dirs) > 0 {
		if candidate := filepath.Join(dirs[0], exe); isFile(candidate) {
			return candidate
		}
		dirs = dirs[1:]
	}
	if found, err := exec.LookPath(name); err == nil {
		return found
	}
	for _, dir := range dirs {
		if candidate := filepath.Join(dir, exe); isFile(candidate) {
			return candidate
		}
	}
	return ""
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// bundledEditorPython is where the capability install puts the interpreter.
func bundledEditorPython() string {
	root, err := config.DataRoot()
	if err != nil {
		return ""
	}
	name := "python"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(root, "tools", "kinocut", name)
}

// videoEditorCommand resolves how to start the editor, most trusted first.
//
// The bundle wins over PATH for the same reason bundledBinary's does
// (internal/skill/bundled.go): the copy Aetox fetched is the version that was
// pinned and proved, and a stale one already on the machine should not quietly
// become the one that runs. The PATH fallback keeps a developer's own
// `pip install kinocut` working.
//
// `python.exe -m kinocut` rather than a `kino` shim inside the bundle: the
// embeddable Python writes no Scripts folder, and its own `._pth` is what puts
// the library on sys.path. Running the module is the only entry point there.
func videoEditorCommand() []string {
	if candidate := bundledEditorPython(); candidate != "" && isFile(candidate) {
		return []string{candidate, "-m", "kinocut", "--mcp"}
	}
	// The resolved path, not the bare name: the readiness panel prints this so
	// somebody can disagree with it, and "kino" is not an answer to "where".
	if found, err := exec.LookPath("kino"); err == nil {
		return []string{found, "--mcp"}
	}
	return nil
}

// VideoEditorCommand is what to write into the MCP entry so it starts the copy
// this machine will have.
//
// Exported because the shelf preset in Settings cannot know it: every other
// preset is a URL that is the same string everywhere, and this one is an
// absolute path into a per-user data folder.
//
// **Will have, not has.** A server entry is written once and read at every
// launch, so it has to be right after the download rather than only before it.
func (a *App) VideoEditorCommand() []string {
	if cmd := videoEditorCommand(); cmd != nil {
		return cmd
	}
	if p := bundledEditorPython(); p != "" {
		return []string{p, "-m", "kinocut", "--mcp"}
	}
	return []string{"kino", "--mcp"}
}

// hyperframesCommand is the scene renderer as kinocut should be told to start
// it: the bundled Node running the bundled entry point.
//
// A path to the `.cmd` shim npm writes would be shorter and would break — the
// shim looks for `node` on PATH, and the whole point of a portable bundle is
// that nothing has to be on PATH for it to work.
//
// Empty when the bundle is not installed, so the caller can fall back to
// whatever the machine already has rather than naming a file that is not there.
func hyperframesCommand() string {
	node, entry := hyperframesParts()
	if node == "" {
		return ""
	}
	return node + " " + entry
}

// hyperframesParts is the same answer unjoined, for the caller that runs it
// rather than writes it down.
//
// Two shapes of one fact, and the split is not cosmetic: kinocut is configured
// with a command LINE (one string in an environment variable), while
// scene_tool.go has to hand exec an argv where the entry point is the first
// argument. Joining and re-splitting would break on the first user whose
// DataRoot has a space in it, which on Windows is most of them.
//
// Empty when the bundle is not installed, so the caller can say so rather than
// naming a file that is not there.
func hyperframesParts() (node, entry string) {
	root, err := config.DataRoot()
	if err != nil {
		return "", ""
	}
	node = filepath.Join(root, "tools", "hyperframes", "node", "node")
	if runtime.GOOS == "windows" {
		node += ".exe"
	}
	entry = filepath.Join(root, "tools", "hyperframes", "node_modules", "hyperframes", "bin", "hyperframes.mjs")
	if !isFile(node) || !isFile(entry) {
		return "", ""
	}
	return node, entry
}

// gsapCDNURL is the one address the library's nine composition scenes load
// their animation library from, spelled here because two places have to agree
// about it: the copy that rewrites it into a local path, and the test that
// refuses a scene naming a version we did not pin.
const gsapCDNURL = "https://cdn.jsdelivr.net/npm/gsap@3.14.2/dist/gsap.min.js"

// gsapFile is the copy of that library on this machine, or "" if the user has
// not installed it. Named after the pinned version for the reason the manifest
// entry gives.
func gsapFile() string {
	root, err := config.DataRoot()
	if err != nil {
		return ""
	}
	path := filepath.Join(root, "tools", "gsap", "gsap-3.14.2.min.js")
	if !isFile(path) {
		return ""
	}
	return path
}

// findHyperframes is the renderer as the readiness panel looks for it: ours
// first, then whatever the user installed themselves.
func findHyperframes() string {
	if cmd := hyperframesCommand(); cmd != "" {
		return cmd
	}
	for _, name := range []string{"hyperframes", "hyperframes.cmd"} {
		if found, err := exec.LookPath(name); err == nil {
			return found
		}
	}
	return ""
}

// VideoEditorEnvironment tells the editor where its ffmpeg is, in its own
// vocabulary.
//
// kinocut reads KINOCUT_FFMPEG_EXECUTABLE and KINOCUT_FFPROBE_EXECUTABLE before
// it looks at PATH (kinocut/engine_runtime_utils.py `_find_executable`), which
// is why nothing here touches the machine's PATH: the editor is told where its
// tools are and every other program on the computer is left as it was.
//
// It names the path Aetox installs to even when nothing is there yet, for the
// same reason VideoEditorCommand does. A machine with its own working ffmpeg
// gets that one named instead, which is what the user already chose.
func (a *App) VideoEditorEnvironment() map[string]string {
	env := map[string]string{}
	root, rootErr := config.DataRoot()
	for _, name := range videoEditorNeeds {
		key := "KINOCUT_" + strings.ToUpper(name) + "_EXECUTABLE"
		if found := findProgram(name); found != "" {
			env[key] = found
			continue
		}
		if rootErr != nil {
			continue
		}
		exe := name
		if runtime.GOOS == "windows" {
			exe += ".exe"
		}
		env[key] = filepath.Join(root, "tools", "ffmpeg-gpl", "bin", exe)
	}
	// And where the scene renderer is, the same way. kinocut reads this before
	// it looks for `hyperframes` on PATH (kinocut/hyperframes_engine.py
	// HYPERFRAMES_COMMAND_ENV), so the bundle is used without anything being
	// installed globally.
	if cmd := hyperframesCommand(); cmd != "" {
		env["MCP_VIDEO_HYPERFRAMES_COMMAND"] = cmd
	}
	for key, value := range hyperframesEnvironment(root, rootErr == nil, findProgram) {
		env[key] = value
	}
	return env
}

// hyperframesEnvironment is everything the scene renderer is told, in its own
// vocabulary, so that none of it has to be discovered by the renderer at run
// time on somebody else's machine.
//
// Six settings, and each one closes a door that is open by default. Read
// together they are the whole of what "ปรับให้เข้าบริบทเรา" means for this
// engine: it is somebody else's program, it works, and the things it would
// otherwise do on its own are the things Aetox has already promised not to do.
//
// Split out of VideoEditorEnvironment and given the lookup as a parameter so it
// can be checked without a DataRoot and without an ffmpeg on the machine — the
// bug this shape prevents is a test that passes because the runner happened to
// have one.
func hyperframesEnvironment(root string, haveRoot bool, lookup func(string) string) map[string]string {
	env := map[string]string{
		// Nothing goes back to HeyGen. The CLI reports its own invocations by
		// default, and a renderer that phones home about the user's work is not
		// a renderer that runs on the user's machine.
		"HYPERFRAMES_NO_TELEMETRY": "1",
		// It does not go looking for a newer copy of itself either. The version
		// that ships is the version that was pinned, checksummed and proved by
		// the tools workflow, and a self-update would replace exactly that.
		"HYPERFRAMES_NO_UPDATE_CHECK": "1",
		// And it installs nothing. This is the load-bearing one: left alone it
		// downloads its own browser on the first render, which would be a second
		// install mechanism beside internal/capability — no checksum we chose,
		// at a moment nobody pressed anything. Aetox has one way of putting a
		// program on a machine and this keeps it that way.
		"HYPERFRAMES_NO_AUTO_INSTALL": "1",
		// The fast capture path stays off, and this is the one switch here that
		// is about the picture rather than about a promise.
		//
		// `--experimental-fast-capture` reads Chrome's paint records instead of
		// screenshotting, and to do it the renderer moves the composition root
		// into a canvas it makes — `document.querySelector("[data-composition-id]")`,
		// and it throws HF_DE_COMPOSITION_ROOT_MISSING when there is none.
		// Thirteen of the twenty-two scenes in the library are a single
		// hand-written file with no composition root at all, so on a machine
		// where this engages the render dies at the first frame with an error
		// about a selector nobody wrote.
		//
		// Upstream calls it "on where it can engage (macOS + hardware-GPU
		// browser)", which is the worse half of the problem: whether it engages
		// depends on a GPU probe, so the same scene renders on one Windows
		// machine and fails on the next. Off is the only answer that is the same
		// answer twice. Measured 31 ส.ค.: statement-title renders in 12s on the
		// screenshot path, and fails in 28s on this one.
		"PRODUCER_EXPERIMENTAL_FAST_CAPTURE": "false",
	}
	if !haveRoot {
		return env
	}
	// The browser we pinned, named whether or not it is there yet — same reason
	// VideoEditorCommand names a path before the download: this is written into
	// the environment once and read at every launch, so it has to be right after
	// the install rather than only before it.
	//
	// chrome-headless-shell rather than a Chrome or the machine's Edge, because
	// hyperframes' fast capture path lives only in that binary. See the
	// chrome-headless-shell job in .github/workflows/tools.yml for the rest of
	// the argument, including why WebView2 cannot stand in.
	browser := filepath.Join(root, "tools", "chrome-headless-shell", "chrome-headless-shell")
	if runtime.GOOS == "windows" {
		browser += ".exe"
	}
	env["HYPERFRAMES_BROWSER_PATH"] = browser

	// The same ffmpeg the editor was given, found the same way, so the two
	// halves of one video job never disagree about which encoder they have.
	// findProgram is deliberately the GPL-carrying search: hyperframes writes
	// libx264 like kinocut does, and refuses outright on a build without it
	// ("This FFmpeg build has neither libx264 nor VideoToolbox").
	if found := lookup("ffmpeg"); found != "" {
		env["HYPERFRAMES_FFMPEG_PATH"] = found
	}
	if found := lookup("ffprobe"); found != "" {
		env["HYPERFRAMES_FFPROBE_PATH"] = found
	}

	// Its caches, inside our folder. Left alone it writes them to
	// ~/.cache/hyperframes and ~/.hyperframes, which are two more places on the
	// user's disk that Aetox put something and never mentions again — and which
	// uninstalling Aetox would not take with it.
	cache := filepath.Join(root, "tools", "hyperframes", "cache")
	env["HYPERFRAMES_FONT_CACHE_DIR"] = filepath.Join(cache, "fonts")
	env["HYPERFRAMES_EXTRACT_CACHE_DIR"] = filepath.Join(cache, "extract")
	return env
}

// VideoToolingStatus is the short answer, for the lock on a card.
type VideoToolingStatus struct {
	// Installed is whether the editor could be spawned, by either route. Not
	// whether it was: nothing here starts it.
	Installed bool `json:"installed"`
	// MissingRequired is whichever of videoEditorNeeds is not on this machine.
	MissingRequired []string `json:"missingRequired"`
}

// VideoToolingStatus answers in lookups only, so every card can ask it.
func (a *App) VideoToolingStatus() VideoToolingStatus {
	out := VideoToolingStatus{MissingRequired: []string{}} // never nil: §34
	out.Installed = videoEditorCommand() != nil
	if !out.Installed {
		return out
	}
	for _, name := range videoEditorNeeds {
		if findProgram(name) == "" {
			out.MissingRequired = append(out.MissingRequired, name)
		}
	}
	return out
}

// videoEditorTools is the tool allowlist the kinocut entry is written with.
//
// **Not tidiness — the bill.** Measured 30 ส.ค. against kinocut 1.15.0: the
// server offers 196 tools whose schemas are ~37,600 tokens, on every request of
// any agent holding it — over three times the whole fresh-install tool block
// (tool_budget_test.go). The 54 named here are the video agents' actual job and
// cost ~12,400. `search_tools` is deliberately among them: it is how the agent
// finds out what was left out and can say so, instead of the trim being silent.
//
// This is the one copy. The shelf preset in Settings reads it over the bridge
// (VideoEditorTools) for the same reason it reads the command and environment
// from here: two lists would be answering one question, and the one in a
// .svelte file is the one nobody re-measures.
var videoEditorTools = []string{
	"search_tools",
	"video_info", "video_info_detailed", "video_read_metadata", "video_trim", "video_merge",
	"video_crop", "video_rotate", "video_resize", "video_speed", "video_fade", "video_convert",
	"video_export", "video_edit", "video_batch", "video_reverse",
	"video_extract_audio", "video_normalize_audio", "video_add_audio", "video_duck_audio",
	"video_subtitles", "video_generate_subtitles", "video_ai_transcribe", "video_translate_captions",
	"video_thumbnail", "video_preview", "video_storyboard", "video_extract_frame", "video_export_frames",
	"video_detect_scenes", "video_ai_remove_silence", "video_find_moments",
	"video_add_text", "video_watermark", "video_overlay", "video_split_screen",
	"video_repurpose_plan", "video_repurpose", "video_create_from_images", "video_composite_layers",
	"video_quality_check", "video_validate_text_layout", "video_compare_quality",
	"hyperframes_render", "hyperframes_preview", "hyperframes_still", "hyperframes_inspect",
	"hyperframes_catalog", "hyperframes_compositions", "hyperframes_validate", "hyperframes_doctor",
	"hyperframes_capture", "hyperframes_tts", "hyperframes_remove_background",
}

// VideoEditorTools is the allowlist as the settings shelf asks for it.
func (a *App) VideoEditorTools() []string {
	return append([]string(nil), videoEditorTools...)
}

// connectVideoEditor writes the kinocut MCP entry and places the editor agent
// on it, when nobody has yet.
//
// Called when the kinocut download lands (runCapabilityInstall), because the
// two halves are one thing: the bundle without the entry is a program no agent
// can reach, and the entry without the bundle is a command naming a file that
// is not there. Before this, pressing ติดตั้งทั้งหมด in ห้องงานวิดีโอ finished
// the download and left the card veiled, and the missing half was a shelf
// preset in a settings section the person had no reason to know about.
//
// This does not cross the rule written on the shelf ("a button, not a
// default"): that rule keeps a fresh install from being wired to a third-party
// endpoint nobody chose. kinocut is a local subprocess this app just fetched
// because the user pressed install on the card of the agent that needs it —
// the press is the consent, and what is removed is the matching.
//
// Idempotent, and it never overrides a decision: an entry that already exists
// keeps its command, tools and disabled state untouched — placement is added
// only when the editor is on nobody's list, and nothing else is written.
func (a *App) connectVideoEditor() error {
	servers, err := config.LoadMCPServers()
	if err != nil {
		return err
	}
	target := config.MCPAgentPrefix + "editor"
	for i := range servers {
		if !strings.EqualFold(strings.TrimSpace(servers[i].Name), VideoEditorServer) {
			continue
		}
		if slices.Contains(servers[i].For, target) {
			return nil
		}
		servers[i].For = append(servers[i].For, target)
		if err := config.SaveMCPServers(servers); err != nil {
			return err
		}
		a.rebuildMCP()
		return nil
	}
	servers = append(servers, config.MCPServerConfig{
		Name:        VideoEditorServer,
		Command:     a.VideoEditorCommand(),
		Environment: a.VideoEditorEnvironment(),
		Tools:       a.VideoEditorTools(),
		For:         []string{target},
	})
	if err := config.SaveMCPServers(servers); err != nil {
		return err
	}
	a.rebuildMCP()
	return nil
}

// ---------------------------------------------------------------------------
// The readiness check
// ---------------------------------------------------------------------------

// ReadyRow is one line of the readiness panel.
type ReadyRow struct {
	// ID is what the row is about. The screen holds the words for each; this
	// side holds only the verdict, so the two languages stay in the locale
	// files where they belong.
	ID string `json:"id"`
	// State is "ok", "missing", "warn" or "optional".
	State string `json:"state"`
	// Where the check actually landed, shown so the answer can be disagreed
	// with. Empty when nothing was found.
	Where string `json:"where,omitempty"`
	// Bytes is what installing this would cost, 0 when it is not ours to
	// install.
	Bytes int64 `json:"bytes"`
}

// VideoReadiness is the whole answer: what is here, what is not, and what one
// press would fetch.
type VideoReadiness struct {
	Rows []ReadyRow `json:"rows"`
	// Capabilities is what InstallCapabilities should be called with, empty
	// when there is nothing this app can fetch.
	//
	// A list rather than one name since the two video jobs stopped sharing a
	// download (videoCapabilitiesFor). InstallCapabilities already took a list;
	// what changed is that this side stopped pretending there was only ever one
	// answer.
	Capabilities []string `json:"capabilities"`
	// MissingBytes is the sum of what would be fetched.
	MissingBytes int64 `json:"missingBytes"`
	// Ready is whether video work can actually be done right now.
	Ready bool `json:"ready"`
}

// videoEditorEncoder asks a found ffmpeg whether it can actually encode H.264.
//
// **Finding the file is not the answer, and this is the trap.** Aetox already
// ships an ffmpeg, deliberately the LGPL build, which carries no libx264 — and
// kinocut writes `-c:v libx264` into 38 call sites. A check that stopped at
// "there is a file called ffmpeg" would draw a green tick and then fail on the
// first render with "Unknown encoder", which is worse than saying nothing.
//
// So one program is run, once, and only because a person pressed the button
// that asks this question.
func videoEditorEncoder(ffmpeg string) bool {
	if ffmpeg == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-encoders")
	proc.HideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "libx264")
}

// videoAgentNeedsScenes reports whether this agent renders HTML scenes, which
// is the one requirement the two video agents do not share.
//
// Asked per agent because the panel is opened from one card and answers for
// that card. `editor` cuts footage that already exists and never touches
// Hyperframes or the template library; telling somebody cutting a clip that
// they are missing a scene renderer is a fault report about a job they are not
// doing (owner, 30 ส.ค.: *"ทำไมมันไม่ตรวจจับเฉพาะเอเจนล่ะ"*).
func videoAgentNeedsScenes(agent string) bool {
	return !strings.EqualFold(strings.TrimSpace(agent), "editor")
}

// videoCapabilitiesFor is what one card's button should fetch.
//
// Two jobs, two lists, one shared piece. Making a video needs the scene
// renderer and the browser it drives; cutting one needs the editor; both encode
// with the same ffmpeg. Before this split there was a single `video` list, so
// pressing install on the card that makes videos downloaded a Python
// interpreter and an editor for footage that does not exist yet — owner,
// 30 ส.ค.: *"มันคนละส่วนกัน ทำไมถึงเอามาปนกัน"*.
func videoCapabilitiesFor(agent string) []string {
	if videoAgentNeedsScenes(agent) {
		return []string{VideoEncodeCapability, VideoMakeCapability}
	}
	return []string{VideoEncodeCapability, VideoEditCapability}
}

// VideoReadiness is the check behind the panel, for one agent. Run when
// somebody presses, never on a page open.
func (a *App) VideoReadiness(agent string) VideoReadiness {
	out := VideoReadiness{Rows: []ReadyRow{}, Capabilities: []string{}} // never nil: §34
	scenes := videoAgentNeedsScenes(agent)
	out.Capabilities = videoCapabilitiesFor(agent)
	missing := map[string]int64{}
	for _, c := range capability.MissingFor(out.Capabilities) {
		missing[c.ID] = c.ApproxBytes
	}

	// The editor is the cutting job's own program and is not asked about on the
	// making card at all. A missing thing that the job never uses is not a fault
	// report, it is noise on a screen whose whole purpose is telling somebody
	// what is wrong.
	editor := ""
	if !scenes {
		if cmd := videoEditorCommand(); cmd != nil {
			editor = cmd[0]
		}
		out.Rows = append(out.Rows, ReadyRow{
			ID: "editor", State: stateOf(editor != ""), Where: editor, Bytes: missing["kinocut"],
		})
	}

	// Ships inside the binary, so it is never absent — and listed anyway. A
	// panel that shows only problems reads as a list of faults rather than as
	// an account of what this machine can do. Only for the agent that draws
	// scenes; the one that cuts footage has no use for them.
	if scenes {
		out.Rows = append(out.Rows, ReadyRow{ID: "templates", State: "ok"})
	}

	ffmpeg := findProgram("ffmpeg")
	ffprobe := findProgram("ffprobe")
	out.Rows = append(out.Rows, ReadyRow{
		ID: "ffmpeg", State: stateOf(ffmpeg != "" && ffprobe != ""), Where: ffmpeg, Bytes: missing["ffmpeg-gpl"],
	})

	// Only worth asking once there is something to ask it of.
	encoder := false
	if ffmpeg != "" {
		encoder = videoEditorEncoder(ffmpeg)
		state := "ok"
		if !encoder {
			state = "warn"
		}
		out.Rows = append(out.Rows, ReadyRow{ID: "h264", State: state, Where: ffmpeg, Bytes: missing["ffmpeg-gpl"]})
	}

	// The scene renderer, and it is required rather than reported.
	//
	// It was optional for one day, on the reasoning that Aetox would render its
	// own scenes off the WebView2 it already carries. That reasoning held for
	// the seven scenes in the library that are CSS and nothing else, and fell
	// over on the other fifteen: nine of them are compositions with sub-scenes,
	// audio tracks and footage slots, which is a product rather than a hundred
	// lines of seeking. Owner's call, 30 ส.ค.: Hyperframes is the engine.
	hyper := ""
	browser := ""
	if scenes {
		hyper, _ = hyperframesParts()
		out.Rows = append(out.Rows, ReadyRow{
			ID: "renderer", State: stateOf(hyper != ""), Where: hyper, Bytes: missing["hyperframes"],
		})
		// Its own row rather than folded into the renderer's, because they fail
		// separately and the sentence differs: a renderer with no browser starts
		// and then cannot draw anything.
		browser = findSceneBrowser()
		out.Rows = append(out.Rows, ReadyRow{
			ID: "browser", State: stateOf(browser != ""), Where: browser, Bytes: missing["chrome-headless-shell"],
		})
		// A warning rather than a cross, because it is the honest severity: 13
		// of the 22 scenes never touch GSAP, and the 9 that do still render
		// while there is a network to fetch it from. It earns a row anyway —
		// without it those 9 quietly become a still picture the first time
		// somebody renders on a train.
		gsap := gsapFile()
		state := "ok"
		if gsap == "" {
			state = "warn"
		}
		out.Rows = append(out.Rows, ReadyRow{ID: "gsap", State: state, Where: gsap, Bytes: missing["gsap"]})
	}

	// Whether anything is missing and how big it is are two questions, and this
	// used to answer the first with the second.
	//
	// A row's Bytes comes from the manifest, so a component that is not in the
	// manifest yet reports zero — and the panel then summed zero, concluded
	// there was nothing to fetch, and drew "ตรวจใหม่" and "ปิด" beside two red
	// crosses. A screen that shows a fault and offers no way out of it is worse
	// than one that shows nothing. What decides the button is now the state of
	// the rows; the size is only ever what the button SAYS.
	missingSomething := false
	for _, row := range out.Rows {
		if row.State == "ok" || row.State == "optional" {
			continue
		}
		missingSomething = true
		out.MissingBytes += row.Bytes
	}
	// Offered only when there is something for it to fetch.
	//
	// The button used to appear whenever a row was red, and pressing it on a
	// machine whose missing piece is not in the manifest ran an install of
	// nothing, succeeded at it, and raised "เพิ่มความสามารถแล้ว" over two rows
	// that were still crossed out. A success message for work that did not
	// happen is worse than an honest refusal: the honest one sends somebody to
	// ask why, and this one sends them to press again.
	if !missingSomething || len(capability.MissingFor(out.Capabilities)) == 0 {
		out.Capabilities = []string{}
	}
	if scenes {
		out.Ready = hyper != "" && browser != "" && ffmpeg != "" && ffprobe != "" && encoder
	} else {
		out.Ready = editor != "" && ffmpeg != "" && ffprobe != "" && encoder
	}
	return out
}

// findSceneBrowser is the browser the renderer is pointed at, looked for where
// Aetox puts it.
//
// Only our own copy counts. A Chrome or an Edge the user happens to have is a
// browser the renderer was never tested against, and HYPERFRAMES_BROWSER_PATH
// names ours whether or not it is there yet — so reporting somebody else's as
// "found" would draw a green tick beside a file the renderer will not open.
func findSceneBrowser() string {
	root, err := config.DataRoot()
	if err != nil {
		return ""
	}
	name := "chrome-headless-shell"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(root, "tools", "chrome-headless-shell", name)
	if !isFile(path) {
		return ""
	}
	return path
}

func stateOf(ok bool) string {
	if ok {
		return "ok"
	}
	return "missing"
}

// videoCheckedMarker is the file that remembers the panel has been seen, so it
// opens itself once and is a button after that.
func videoCheckedMarker() string {
	root, err := config.DataRoot()
	if err != nil {
		return ""
	}
	return filepath.Join(root, "video-checked.ok")
}

// VideoCheckSeen reports whether the readiness panel has ever been shown.
func (a *App) VideoCheckSeen() bool {
	path := videoCheckedMarker()
	if path == "" {
		return true // cannot remember it, so do not keep reopening it
	}
	return isFile(path)
}

// MarkVideoCheckSeen records that it has been. Written when the panel opens
// rather than when it is acted on: opening once is the promise, and somebody
// who closed it without pressing anything has still been told.
func (a *App) MarkVideoCheckSeen() {
	if path := videoCheckedMarker(); path != "" {
		_ = os.WriteFile(path, nil, 0o644)
	}
}

// VideoEditorHelpURL is the last resort, for a platform whose manifest offers
// none of this. Everything Aetox can install for itself goes through the
// readiness panel instead — owner, 30 ส.ค.: *"ไปหน้าติดตั้งดิ แสดงหน้าต่าง
// ติดตั้งของเราเลย"*. Sending somebody to a download page for something we
// fetch ourselves was the wrong answer to a question we can answer.
func (a *App) VideoEditorHelpURL(what string) string {
	switch strings.ToLower(strings.TrimSpace(what)) {
	case "ffmpeg", "ffprobe":
		switch runtime.GOOS {
		case "darwin":
			return "https://formulae.brew.sh/formula/ffmpeg"
		case "linux":
			return "https://ffmpeg.org/download.html#build-linux"
		default:
			return "https://www.gyan.dev/ffmpeg/builds/"
		}
	}
	return "https://kinocut.dev"
}
