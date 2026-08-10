package main

// One test that runs every tool the agent is given, through the real path.
//
// Not "the definitions look well-formed" (tool_definitions_test.go already does
// that) and not "the provider accepts the batch" (live_tools_test.go does that
// against a real API). This one answers the question those two leave open: if
// the model calls this tool right now, does anything actually happen?
//
// The path is the real one. skill.NewDefaultRegistry plus App.workbenchSkills is
// exactly the set applyConfig hands the agent, and dispatcher.ExecuteTool is the
// same call internal/turn/executor.go makes when a model's tool call arrives —
// so what runs below is the tool's own implementation against a real sandbox,
// real subprocesses and real files, with nothing stubbed in between.
//
// Two guards, both aimed at drift rather than at today's code:
//
//  1. Every tool in the registry must appear in the table. A tool added without
//     a case fails the test instead of quietly never being exercised.
//  2. The set of skills deliberately kept off the model's tool surface is
//     pinned. A tool that loses its ToolDefinition — and with it any chance of
//     the model ever calling it — stops being an invisible regression.

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/lsp"
	"github.com/Mike0165115321/Aetox/internal/automation/n8n"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/stt"
	"github.com/Mike0165115321/Aetox/internal/automation/windmill"
)

// cliOnlySkills are registered but have no ToolDefinition, so the model is
// never offered them: CLI-era surface kept for the console. Pinned here so that
// dropping a real tool off the model's surface fails loudly instead of looking
// like nothing changed.
//
// shell and git left this list on 2026-07-28 (ARCHITECTURE.md §49). ADR 0001
// had held them back as phase-1 caution; the narrower tools it was waiting for
// shipped long ago, and until then the agent could edit code without ever being
// able to run it.
var cliOnlySkills = []string{"echo", "fs", "help"}

// toolCase is how one tool gets driven. args are what the model would send.
type toolCase struct {
	args map[string]any
	// available reports whether this machine can run the tool at all. nil means
	// it always can, and then the tool MUST succeed — no excuses accepted.
	available func() bool
	// why names the missing piece, for the report line when available is false.
	why string
	// drive runs alongside the call, for tools that block on something else
	// happening (ask_user waits for a human).
	drive func(t *testing.T, a *App)
	// realUserProfile puts %USERPROFILE% back for the duration of this one
	// call. See useRealUserProfile — it is about a third-party binary, not
	// about the tool.
	realUserProfile bool
	// check looks for evidence the tool did its job, rather than returning an
	// empty success.
	check func(t *testing.T, out skill.Output, root string)
}

func TestEveryToolRunsThroughTheRealDispatcher(t *testing.T) {
	// Captured before the isolation replaces it — the one tool that spawns a
	// third-party binary needs the real value back for the length of its call.
	realUserProfile := os.Getenv("USERPROFILE")
	isolateUserDirs(t)
	root := t.TempDir()
	// diagnostics starts a real gopls and leaves it running on purpose (see
	// lsp.Shared). Registered after the temp dirs so LIFO shuts it down first:
	// otherwise it still holds files inside them and the cleanup fails.
	t.Cleanup(lsp.CloseShared)
	writeToolFixtures(t, root)

	app := &App{}
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root})
	for _, s := range app.workbenchSkills() {
		if err := registry.Register(s, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", s.Name(), err)
		}
	}
	dispatcher := skill.NewDispatcher(registry)

	assertModelSurfaceIsIntact(t, registry, dispatcher)

	cases := toolCases(t, root, dispatcher)
	var ran, skipped []string

	for _, def := range dispatcher.ToolDefinitions() {
		// A packed tool is one entry in the block and several acts inside it,
		// so it is driven once per action rather than once (skill.PackedCalls).
		// Packing was meant to cost the tool block, not the coverage: `github`
		// answering for one case while three of its four actions ship
		// unexercised is exactly the quiet gap this test exists to close.
		//
		// Each action is looked up under the name it had before the packing,
		// which is also the name every gate still judges it by — so the table
		// below did not have to be rewritten, and a reader of the report still
		// sees which act ran.
		for _, call := range callsOf(def.Function.Name) {
			name := call.Permission
			tc, ok := cases[name]
			if !ok {
				t.Errorf("%s is offered to the model but this test does not run it — add a case, or the tool ships unexercised", name)
				continue
			}
			tc.args = withAction(tc.args, call.Action)

			if tc.available != nil && !tc.available() {
				skipped = append(skipped, name+" ("+tc.why+")")
				// Still has to be reachable: a tool the dispatcher cannot route
				// is broken whether or not its dependency is installed.
				assertReachable(t, dispatcher, def.Function.Name, tc)
				continue
			}

			restore := useRealUserProfile(t, tc, realUserProfile)
			out := runTool(t, dispatcher, app, def.Function.Name, tc)
			restore()
			if !out.Success {
				t.Errorf("%s failed on a machine that can run it: %s", name, firstLine(out.Stderr+out.Content))
				continue
			}
			if tc.check != nil {
				tc.check(t, out, root)
			}
			ran = append(ran, name)
		}
	}

	sort.Strings(ran)
	sort.Strings(skipped)
	t.Logf("ran for real (%d): %s", len(ran), strings.Join(ran, ", "))
	if len(skipped) > 0 {
		t.Logf("reachable but not runnable here (%d): %s", len(skipped), strings.Join(skipped, ", "))
	}
}

// callsOf expands one offered tool into the calls that have to be driven: every
// action of a packed tool, or the tool itself for everything else.
func callsOf(tool string) []skill.PackedCall {
	if calls := skill.PackedCalls(tool); len(calls) > 0 {
		return calls
	}
	return []skill.PackedCall{{Permission: tool}}
}

// withAction copies a case's arguments with the action word added.
//
// A copy because the table is built once and its maps are shared: writing the
// action into the original would leave whichever action ran last sitting in the
// arguments of a case that never asked for one.
func withAction(args map[string]any, action string) map[string]any {
	if action == "" {
		return args
	}
	out := make(map[string]any, len(args)+1)
	for key, value := range args {
		out[key] = value
	}
	out["action"] = action
	return out
}

// assertModelSurfaceIsIntact catches a tool silently dropping off the list the
// model is sent — the failure mode where everything still compiles, every unit
// test still passes, and the agent simply never uses that tool again.
func assertModelSurfaceIsIntact(t *testing.T, registry *skill.Registry, dispatcher *skill.Dispatcher) {
	t.Helper()
	exposed := map[string]bool{}
	for _, d := range dispatcher.ToolDefinitions() {
		exposed[d.Function.Name] = true
	}
	var hidden []string
	for _, name := range registry.Names() {
		if !exposed[name] {
			hidden = append(hidden, name)
		}
	}
	sort.Strings(hidden)
	if strings.Join(hidden, ",") != strings.Join(cliOnlySkills, ",") {
		t.Errorf("skills hidden from the model changed\n  now:      %v\n  expected: %v\nA tool losing its ToolDefinition can never be called again — confirm that is intended, then update cliOnlySkills.", hidden, cliOnlySkills)
	}
}

// runTool calls the dispatcher exactly the way turn/executor.go does, under a
// deadline so a hung tool fails the test instead of the suite.
func runTool(t *testing.T, d *skill.Dispatcher, app *App, name string, tc toolCase) skill.Output {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if tc.drive != nil {
		go tc.drive(t, app)
	}

	type result struct {
		out     skill.Output
		handled bool
		err     error
	}
	done := make(chan result, 1)
	go func() {
		out, handled, err := d.ExecuteTool(ctx, name, tc.args)
		done <- result{out, handled, err}
	}()

	select {
	case r := <-done:
		if !r.handled {
			t.Fatalf("%s: the dispatcher did not route it — the model would get 'tool is not exposed to agent'", name)
		}
		if r.err != nil && r.out.Success {
			t.Errorf("%s returned an error and Success=true at once: %v", name, r.err)
		}
		return r.out
	case <-ctx.Done():
		t.Fatalf("%s never returned — a tool that hangs blocks the whole turn", name)
		return skill.Output{}
	}
}

// assertReachable is the reduced check for a tool whose dependency is absent:
// it must still be routed and must still come back, rather than hanging or
// leaking a raw exec error.
func assertReachable(t *testing.T, d *skill.Dispatcher, name string, tc toolCase) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan bool, 1)
	go func() {
		_, handled, _ := d.ExecuteTool(ctx, name, tc.args)
		done <- handled
	}()
	select {
	case handled := <-done:
		if !handled {
			t.Errorf("%s is offered to the model but the dispatcher will not route it", name)
		}
	case <-ctx.Done():
		t.Errorf("%s hung with its dependency missing — it must report, not block the turn", name)
	}
}

// startBackgroundFixture launches one long-running command through the real
// dispatcher and returns its handle, so shell_output and shell_kill are driven
// against a process that genuinely exists. Without it the only way to exercise
// them is an invented id, which tests the error path and nothing else.
func startBackgroundFixture(t *testing.T, d *skill.Dispatcher) string {
	t.Helper()
	command := `ping -n 60 127.0.0.1 >nul & echo aetox-bg-done`
	if runtime.GOOS != "windows" {
		command = `sleep 60; echo aetox-bg-done`
	}
	out, _, err := d.ExecuteTool(context.Background(), "shell", map[string]any{
		"command":           command,
		"run_in_background": true,
	})
	if err != nil {
		t.Fatalf("background fixture: %v", err)
	}
	id := regexp.MustCompile(`\bbg_\d+\b`).FindString(out.Content)
	if id == "" {
		t.Fatalf("background fixture returned no handle: %q", out.Content)
	}
	return id
}

func toolCases(t *testing.T, root string, dispatcher *skill.Dispatcher) map[string]toolCase {
	t.Helper()
	// Started once and shared by both cases below, which are driven in the
	// pack's own order — read, then kill. Either way round works on purpose:
	// a read before the kill finds a job that is still running, and a read
	// after it finds one that is not, because the handle outlives the process.
	backgroundID := startBackgroundFixture(t, dispatcher)
	writeCoverageSkill(t)
	return map[string]toolCase{
		// --- pure local: no excuse for these to fail anywhere ---
		"time": {args: map[string]any{}},
		// The interpreter is compiled in, so this one has no excuse to fail on
		// any machine — that is the whole reason it is not `python script.py`.
		"calc": {args: map[string]any{"script": "1234 * 5678"}, check: outputContains("7006652")},
		"list": {args: map[string]any{}, check: outputContains("notes.txt")},
		"read": {args: map[string]any{"path": "notes.txt"}, check: outputContains("alpha")},
		"grep": {args: map[string]any{"pattern": "alpha"}, check: outputContains("notes.txt")},
		"glob": {args: map[string]any{"pattern": "*.txt"}, check: outputContains("notes.txt")},
		"write": {
			args:  map[string]any{"path": "written.txt", "content": "from the tool\n"},
			check: fileContains("written.txt", "from the tool"),
		},
		"sheet_write": {
			args: map[string]any{
				"path": "book.xlsx",
				"sheets": []any{map[string]any{
					"name":    "รายการ",
					"columns": []any{"ชื่อ", "ยอด"},
					"rows":    []any{[]any{"กาแฟ", 185.5}},
				}},
			},
			check: workbookHasNumericCell("book.xlsx", "185.5"),
		},
		"slides_write": {
			args: map[string]any{
				"path": "deck.pptx",
				"slides": []any{map[string]any{
					"title":   "สรุปยอดขาย",
					"bullets": []any{"โตขึ้น ๑๒%"},
					"notes":   "พูดถึงลูกค้าเก่า",
				}},
			},
			check: packageHasPart("deck.pptx", "ppt/slides/slide1.xml", "สรุปยอดขาย"),
		},
		"doc_write": {
			args: map[string]any{
				"path": "memo.docx",
				"blocks": []any{
					map[string]any{"type": "heading", "text": "บันทึกข้อความ", "level": 1},
					map[string]any{"type": "paragraph", "text": "เรียน ผู้จัดการฝ่ายขาย"},
					map[string]any{"type": "bullets", "items": []any{"ยอดโตขึ้น ๑๒%"}},
					map[string]any{"type": "table", "columns": []any{"หมวด", "ยอด"}, "rows": []any{[]any{"อาหาร", "95"}}},
				},
			},
			check: packageHasPart("memo.docx", "word/document.xml", "บันทึกข้อความ"),
		},
		"edit": {
			args:  map[string]any{"path": "notes.txt", "old_string": "beta", "new_string": "BETA"},
			check: fileContains("notes.txt", "BETA"),
		},
		"apply_patch": {
			args: map[string]any{"edits": []any{
				map[string]any{"path": "patch-me.txt", "old_string": "two", "new_string": "TWO"},
			}},
			check: fileContains("patch-me.txt", "TWO"),
		},
		"notebook_edit": {
			args:  map[string]any{"path": "nb.ipynb", "cell": 0, "source": "y = 99\n"},
			check: fileContains("nb.ipynb", "y = 99"),
		},
		"delete": {
			args: map[string]any{"path": "victim.txt"},
			check: func(t *testing.T, _ skill.Output, root string) {
				if _, err := os.Stat(filepath.Join(root, "victim.txt")); !os.IsNotExist(err) {
					t.Errorf("delete reported success but the file is still there")
				}
			},
		},
		"todo_write": {args: map[string]any{"todos": []any{
			map[string]any{"content": "prove the tool runs", "status": "completed"},
		}}},
		// Progressive skill loading: a fixture skill is installed into the
		// (isolated) discovery dir by writeCoverageSkill, so the list has one
		// real entry and the view has a real body to return.
		"skills_list": {
			args:  map[string]any{},
			check: outputContains("coverage-skill"),
		},
		"skill_view": {
			args:  map[string]any{"name": "coverage-skill"},
			check: outputContains("the coverage body"),
		},
		// Empty history is this test's reality — "no matches" IS the success
		// case, proving the query ran against a real database.
		"session_search": {
			args:  map[string]any{"query": "nothing-recorded-yet"},
			check: outputContains("No history matches"),
		},
		// The receipt has to say the change is waiting, not that it happened:
		// a model told "saved" would stop proposing and start assuming, and
		// nothing reaches memory until a human approves it.
		"memory": {
			args: map[string]any{
				"text": "เครื่องนี้ไม่มี Excel ติดตั้ง",
				"why":  "เปิดไฟล์ .xlsx แล้วไม่มีโปรแกรมรับ",
			},
			check: func(t *testing.T, out skill.Output, _ string) {
				if !strings.Contains(out.Content, "approve") {
					t.Errorf("memory receipt should say it is waiting for approval: %q", out.Content)
				}
			},
		},
		"suggest_task": {
			args: map[string]any{
				"title":  "Fix the stale badge",
				"prompt": "In README.md the CI badge points at a workflow that was renamed; update the URL to the current one.",
				"tldr":   "The README badge is dead.",
			},
			check: func(t *testing.T, out skill.Output, _ string) {
				if !strings.Contains(out.Content, "Noted") {
					t.Errorf("suggest_task receipt missing: %q", out.Content)
				}
			},
		},
		// echo is the one command cmd.exe and sh both spell the same way, and
		// the point here is the plumbing — a real child process, its output
		// captured and handed back — not the command itself.
		"shell": {
			args:  map[string]any{"command": "echo aetox-shell-ok"},
			check: outputContains("aetox-shell-ok"),
		},
		"shell_kill": {
			args:  map[string]any{"shell_id": backgroundID},
			check: outputContains("killed"),
		},
		"shell_output": {
			args: map[string]any{"shell_id": backgroundID},
			// Reading a killed job still works and still says what happened to
			// it — the handle outlives the process on purpose.
			check: outputContains(backgroundID),
		},
		"shell_list": {
			args: map[string]any{},
			// Runs after kill in the pack's order, which is the interesting
			// case: a finished job is still listed while it is remembered,
			// because the model recovering a lost handle is usually looking
			// for one whose work has already ended.
			check: outputContains(backgroundID),
		},

		// ask_user blocks until a human answers — so the test answers, through
		// the same binding the UI calls. This is the one tool whose real path
		// includes a round trip back out of the engine.
		"ask_user": {
			args:  map[string]any{"question": "does this tool work?", "options": []any{"yes", "no"}},
			drive: answerTheQuestion("yes"),
			check: outputContains("yes"),
		},

		// --- needs a local binary ---
		"pdf_read": {
			args:            map[string]any{"path": "doc.pdf"},
			available:       haveBinary("pdftotext"),
			why:             "no poppler",
			realUserProfile: true,
			check:           outputContains("AETOX PDF OK"),
		},
		"image_ocr": {
			args:      map[string]any{"path": "shot.png"},
			available: haveBinary("tesseract"),
			why:       "no tesseract",
		},
		"video_ocr": {
			// interval_seconds is explicit: the default sampling interval is
			// longer than a fixture clip, and "no frames extracted" would read
			// as a broken tool rather than a fixture too short to sample.
			args:      map[string]any{"path": "clip.mp4", "interval_seconds": 1},
			available: bothOf(haveBinary("ffmpeg"), fileExists(filepath.Join(root, "clip.mp4"))),
			why:       "no ffmpeg",
		},
		"audio_transcribe": {
			args:      map[string]any{"path": "tone.wav"},
			available: bothOf(haveBinary("ffmpeg"), haveSpeechEngine),
			why:       "no ffmpeg, or no speech model on this machine",
		},
		"diagnostics": {
			args:      map[string]any{"path": "main.go"},
			available: haveBinary("gopls"),
			why:       "no gopls",
		},
		"symbol": {
			args:      map[string]any{"path": "main.go", "name": "main"},
			available: haveBinary("gopls"),
			why:       "no gopls",
		},
		// No check: Success already means git was found, the action passed the
		// read-only allowlist, ensureGitRepo confirmed a real repository and
		// output came back. Matching on the text would pin git's own wording,
		// which is localized and version-dependent.
		"git": {
			args:      map[string]any{"action": "status", "args": []any{"--short"}},
			available: haveBinary("git"),
			why:       "no git",
		},

		// --- needs the network ---
		"web_fetch": {
			args:      map[string]any{"url": "https://example.com"},
			available: online,
			why:       "offline",
			check:     outputContains("Example Domain"),
		},
		"web_search":          {args: map[string]any{"query": "golang"}, available: online, why: "offline"},
		"github_repo_summary": {args: map[string]any{"repo_url": "https://github.com/golang/example"}, available: githubUsable, why: "offline, or out of GitHub API quota"},
		"github_search":       {args: map[string]any{"query": "aetox"}, available: githubUsable, why: "offline, or out of GitHub API quota"},
		"github_list_files":   {args: map[string]any{"repo_url": "https://github.com/golang/example"}, available: githubUsable, why: "offline, or out of GitHub API quota"},
		"github_read_file": {
			args:      map[string]any{"repo_url": "https://github.com/golang/example", "path": "README.md"},
			available: githubUsable,
			why:       "offline, or out of GitHub API quota",
		},

		// The n8n tools reach a server only the user has. There is no public
		// instance to point a test at and standing one up would mean running
		// Docker in CI, so what is exercised here is the half that is ours: the
		// dispatcher routes the call, the tool validates its arguments, and the
		// not-connected refusal is the sentence the user would actually read.
		// assertReachable does that on the skipped path.
		"n8n_workflow_list":     {args: map[string]any{}, available: n8nConnected, why: "no n8n connected on this machine"},
		"n8n_workflow_read":     {args: map[string]any{"id": "1"}, available: n8nConnected, why: "no n8n connected on this machine"},
		"n8n_workflow_create":   {args: map[string]any{"name": "aetox test"}, available: n8nConnected, why: "no n8n connected on this machine"},
		"n8n_workflow_update":   {args: map[string]any{"id": "1", "name": "aetox test"}, available: n8nConnected, why: "no n8n connected on this machine"},
		"n8n_workflow_activate": {args: map[string]any{"id": "1", "active": false}, available: n8nConnected, why: "no n8n connected on this machine"},

		// Same reasoning for the second engine. Windmill's paths carry one extra
		// trap worth exercising even here: `u/a/b` is the shape the server
		// enforces, and the arguments below are valid ones, so a failure on a
		// connected machine is the client's fault rather than the test's.
		"windmill_workspace_list": {args: map[string]any{}, available: windmillConnected, why: "no Windmill connected on this machine"},
		"windmill_flow_list":      {args: map[string]any{"workspace": "dev"}, available: windmillConnected, why: "no Windmill connected on this machine"},
		"windmill_flow_read":      {args: map[string]any{"workspace": "dev", "path": "u/aetox/probe"}, available: windmillConnected, why: "no Windmill connected on this machine"},
		"windmill_flow_create":    {args: map[string]any{"workspace": "dev", "path": "u/aetox/probe"}, available: windmillConnected, why: "no Windmill connected on this machine"},
		"windmill_flow_update":    {args: map[string]any{"workspace": "dev", "path": "u/aetox/probe"}, available: windmillConnected, why: "no Windmill connected on this machine"},

		// The power switches are never thrown from a test: starting the user's
		// real server would spawn a process this test does not own and cannot
		// clean up. The reachable half — dispatch routes the call, and with
		// nothing configured the refusal names the missing address — is what
		// assertReachable exercises.
		"n8n_server_start":      {args: map[string]any{}, available: never, why: "not starting the user's server from a test"},
		"windmill_server_start": {args: map[string]any{}, available: never, why: "not starting the user's server from a test"},

		// plugin_install is driven at its own validation rather than through a
		// real install: the point here is that the call reaches the tool, and a
		// test that downloads and installs a plugin to prove it is a test that
		// fetches arbitrary code from the internet on every run.
		"plugin_install": {
			args:      map[string]any{"repo_url": "not-a-repo-url"},
			available: never,
			why:       "not installing a plugin from a test",
		},

		// --- needs the running desktop window ---
		// These reach a real webview or nothing. Headless they must fail
		// cleanly and immediately, which is what assertReachable checks: the
		// regression worth catching is a browser tool that hangs the turn.
		// One tool, four actions (desktop/browser_tool.go). Driven at `open`
		// because that is the action a session starts with and the one whose
		// argument handling is worth reaching; the other three refuse for the
		// same reason on the same path — there is no window here.
		// One case per action since browser joined the packs table — the same
		// per-act coverage every other packed tool gets. All unrunnable here
		// for the same one reason, but each action still has to route.
		"browser_open":  {args: map[string]any{"url": "https://example.com"}, available: never, why: "needs the app window"},
		"browser_read":  {args: map[string]any{}, available: never, why: "needs the app window"},
		"browser_click": {args: map[string]any{"ref": 1}, available: never, why: "needs the app window"},
		"browser_type":  {args: map[string]any{"ref": 1, "text": "x"}, available: never, why: "needs the app window"},

		// The agent's reach onto the desk (workbench_desk.go). desk_open and
		// desk_terminal both end in an event the frontend answers, so there is
		// nothing here to answer them; their behaviour is covered in
		// workbench_desk_test.go, and what this file adds is the guarantee that
		// the dispatcher can still route them.
		//
		// desk_list is the exception and runs for real: it reads state this
		// process owns, so it needs no window at all — and a tool that answers
		// "the desk is empty" is exactly right for a test with no desk.
		"desk_open":     {args: map[string]any{"path": "note.md"}, available: never, why: "needs the app window"},
		"desk_terminal": {args: map[string]any{"command": "echo hi"}, available: never, why: "needs the app window"},
		"desk_list":     {args: map[string]any{}, check: outputContains("โต๊ะ")},
	}
}

// --- drivers and predicates -------------------------------------------------

// answerTheQuestion plays the user: it waits for ask_user to register its
// question, then answers through AnswerUserQuestion, the same binding the UI
// calls when someone clicks an option.
func answerTheQuestion(answer string) func(*testing.T, *App) {
	return func(t *testing.T, a *App) {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			a.askMu.Lock()
			pending := a.askCh != nil
			a.askMu.Unlock()
			if pending {
				a.AnswerUserQuestion(answer)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func haveBinary(name string) func() bool {
	return func() bool {
		_, err := exec.LookPath(name)
		return err == nil
	}
}

func fileExists(path string) func() bool {
	return func() bool {
		_, err := os.Stat(path)
		return err == nil
	}
}

func bothOf(a, b func() bool) func() bool {
	return func() bool { return a() && b() }
}

func never() bool { return false }

// haveSpeechEngine asks the speech package the same question audio_transcribe
// asks it — a binary alone is not enough, there has to be a model file too, and
// guessing at either from here would drift from what the tool actually needs.
//
// Note this is normally false under test: isolateUserDirs moves the data root,
// and the model file lives in the real one. That is the isolation working, not
// a gap — internal/stt has its own live test for the engine itself.
func haveSpeechEngine() bool {
	_, err := stt.New(stt.Options{})
	return err == nil
}

// online is a real reachability check, not a guess from an env var: the network
// tools below are about to make real requests.
func online() bool {
	conn, err := net.DialTimeout("tcp", "example.com:443", 4*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// n8nConnected reports whether this machine has an n8n attached to run the
// automation tools against.
//
// Almost always false, and deliberately not faked: an httptest server would
// prove that our client can talk to our own stub, which is the one thing never
// in doubt. What the real risk is — n8n rejecting a body over a read-only field
// it will not accept — can only be found against a real instance, and this
// returning false is the honest record that it has not been.
func n8nConnected() bool {
	return n8n.BaseURL() != "" && n8n.Token() != ""
}

func windmillConnected() bool {
	return windmill.BaseURL() != "" && windmill.Token() != ""
}

// githubUsable asks GitHub whether it will answer before four tools are judged
// on whether it did. Unauthenticated calls are capped at 60 an hour per IP, and
// running this suite a few times is enough to spend them — after which every
// github tool returns 403 and looks broken when the only thing wrong is the
// quota. The rate_limit endpoint itself is not counted against that budget.
func githubUsable() bool {
	if !online() {
		return false
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://api.github.com/rate_limit")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var body struct {
		Resources struct {
			Core struct {
				Remaining int `json:"remaining"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}
	// A margin, not zero: four tools are about to spend one call each, and one
	// of them spends two (metadata, then the file).
	return body.Resources.Core.Remaining > 6
}

func outputContains(want string) func(*testing.T, skill.Output, string) {
	return func(t *testing.T, out skill.Output, _ string) {
		t.Helper()
		if !strings.Contains(out.Content, want) {
			t.Errorf("output does not contain %q — the tool succeeded without doing its job\ngot: %s", want, firstLine(out.Content))
		}
	}
}

func fileContains(rel, want string) func(*testing.T, skill.Output, string) {
	return func(t *testing.T, _ skill.Output, root string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("reading %s back: %v", rel, err)
			return
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s on disk does not contain %q — the tool reported success without writing", rel, want)
		}
	}
}

// workbookHasNumericCell proves sheet_write produced a real workbook rather
// than a file with the right name. fileContains cannot: an .xlsx is a ZIP, so
// the value is inside a compressed part and no substring of the raw bytes finds
// it. Checking for the <v> form specifically is the point — a value that landed
// as an inline string looks identical to the user and cannot be summed.
func workbookHasNumericCell(rel, value string) func(*testing.T, skill.Output, string) {
	// Checking for the <v> form specifically is the point — a value that landed
	// as an inline string looks identical to the user and cannot be summed.
	return packageHasPart(rel, "xl/worksheets/sheet1.xml", "<v>"+value+"</v>")
}

// packageHasPart proves an Office tool produced a real package rather than a
// file with the right name. fileContains cannot: .xlsx and .pptx are ZIPs, so
// the content lives inside a compressed part and no substring of the raw bytes
// finds it.
func packageHasPart(rel, part, want string) func(*testing.T, skill.Output, string) {
	return func(t *testing.T, _ skill.Output, root string) {
		t.Helper()
		zr, err := zip.OpenReader(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("%s is not a readable package: %v", rel, err)
			return
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.Name != part {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				t.Errorf("opening %s in %s: %v", part, rel, err)
				return
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Errorf("reading %s in %s: %v", part, rel, err)
				return
			}
			if !strings.Contains(string(body), want) {
				t.Errorf("%s: %s does not contain %q — the tool reported success without doing its job", rel, part, want)
			}
			return
		}
		t.Errorf("%s has no part %s", rel, part)
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// useRealUserProfile puts the machine's own %USERPROFILE% back for one tool
// call and returns the undo.
//
// The isolation this reverses is not optional in general: os.UserHomeDir reads
// USERPROFILE on Windows, and app.go's attachment sweep walks the home folder
// it returns — a test that ran that against a real home would be deleting the
// user's files, which is the exact class of bug DECISIONS §14.4 records.
//
// But it isolates Aetox, and pdftotext is not Aetox. Measured 2026-08-09 on
// this machine, one variable at a time, against a valid PDF:
//
//	USERPROFILE redirected  → exit status 0xc0000005 (access violation), no output
//	USERPROFILE real        → "AETOX PDF OK", clean
//
// HOME, APPDATA, LOCALAPPDATA and XDG_CONFIG_HOME are all fine redirected; it
// is USERPROFILE alone, and the failure is Git for Windows' pdftotext.exe
// *crashing* rather than reporting anything — poppler's Windows build reaching
// for something under the home it was handed. Nothing about that is Aetox's
// behaviour to assert, and it cost a real debugging session once already
// because the symptom reads as "pdf_read is broken".
//
// Scoped to the single call rather than the whole test so every other tool
// still runs fully isolated. pdf_read writes nothing outside the sandbox root
// — it spawns pdftotext and reads its stdout — so there is nothing here for
// the restored home to be exposed to.
func useRealUserProfile(t *testing.T, tc toolCase, real string) func() {
	t.Helper()
	if !tc.realUserProfile || real == "" {
		return func() {}
	}
	isolated := os.Getenv("USERPROFILE")
	if err := os.Setenv("USERPROFILE", real); err != nil {
		t.Fatalf("restoring USERPROFILE: %v", err)
	}
	return func() { _ = os.Setenv("USERPROFILE", isolated) }
}

// --- fixtures ---------------------------------------------------------------

// minimalPDF builds a *valid* one-page PDF around text — real xref table, real
// byte offsets, real startxref.
//
// It used to be a const holding the same deliberately-damaged PDF the pdf_read
// unit test uses: no xref, on the theory that poppler rebuilds one. It does,
// usually. On 2026-08-09 this case failed here with `exit status 0xc0000005` —
// an access violation, which is pdftotext.exe *crashing* rather than reporting
// anything — while the identical file, flags and binary ran fine from a shell.
// Whatever the trigger, the reconstruct-a-broken-file path is not what this
// test is for: the toolbox sweep asks whether pdf_read extracts the text of a
// PDF, and reaching that answer through a third-party binary's damage-recovery
// code makes the sweep as reliable as the least-tested branch of poppler.
//
// The unit test in internal/skill keeps the damaged one on purpose — its
// subject is that poppler's stderr warnings never reach the model, which needs
// a file that produces warnings. Two fixtures, two questions; the comment that
// used to claim they were the same file was describing an accident.
//
// Built rather than written out because an xref is a table of byte offsets:
// hand-maintaining it means any edit to any object silently corrupts the file,
// and a raw string literal in a CRLF checkout does not even hold the bytes the
// author counted.
func minimalPDF(text string) string {
	stream := "BT /F1 18 Tf 20 100 Td (" + text + ") Tj ET\n"
	objects := []string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R]/Count 1>>",
		"<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>",
		fmt.Sprintf("<</Length %d>>stream\n%sendstream", len(stream), stream),
		"<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>",
	}

	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects))
	for i, body := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	startXref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(objects)+1)
	// Every entry is exactly 20 bytes, free list head first. A reader seeks by
	// multiplying, so a byte off here is a file that will not open.
	b.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1, startXref)
	return b.String()
}

func writeToolFixtures(t *testing.T, root string) {
	t.Helper()
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatalf("fixture %s: %v", rel, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("fixture dir: %v", err)
	}
	write("notes.txt", "alpha\nbeta\ngamma\n")
	write("patch-me.txt", "one\ntwo\nthree\n")
	write("victim.txt", "delete me\n")
	write("main.go", "package main\n\nfunc main() {}\n")
	write(filepath.Join("sub", "inner.txt"), "alpha inside\n")
	write("doc.pdf", minimalPDF("AETOX PDF OK"))
	// A minimal but genuine nbformat 4 notebook — notebook_edit parses it as
	// JSON and writes it back, so a hand-waved fixture would fail for the wrong
	// reason.
	write("nb.ipynb", `{
 "cells": [{"cell_type": "code", "execution_count": null, "metadata": {}, "outputs": [], "source": ["x = 1\n"]}],
 "metadata": {},
 "nbformat": 4,
 "nbformat_minor": 5
}
`)
	writeTestPNG(t, filepath.Join(root, "shot.png"))
	writeTestWAV(t, filepath.Join(root, "tone.wav"))

	// git refuses to run outside a work tree, so the sandbox has to be one. A
	// real init rather than a faked .git: ensureGitRepo asks git itself.
	if _, err := exec.LookPath("git"); err == nil {
		if err := exec.Command("git", "-C", root, "init", "-q").Run(); err != nil {
			t.Logf("could not init the git fixture (%v) — git will report as unavailable", err)
		}
	}

	// A real one-second video, built from the PNG by the same ffmpeg video_ocr
	// will use. Skipped silently when ffmpeg is absent — the case above then
	// reports "no ffmpeg" rather than failing on a missing fixture.
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error",
			"-loop", "1", "-i", filepath.Join(root, "shot.png"),
			"-t", "3", "-pix_fmt", "yuv420p", filepath.Join(root, "clip.mp4"))
		if err := cmd.Run(); err != nil {
			t.Logf("could not build the video fixture (%v) — video_ocr will report as unavailable", err)
		}
	}
}

// writeTestPNG draws black bars on white. Not text, so OCR legitimately finds
// nothing; the point is a real decodable image rather than bytes named .png.
func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 120, 60))
	for x := 0; x < 120; x++ {
		for y := 0; y < 60; y++ {
			img.Set(x, y, color.White)
		}
	}
	for x := 10; x < 110; x++ {
		for y := 25; y < 35; y++ {
			img.Set(x, y, color.Black)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("png fixture: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("png fixture: %v", err)
	}
}

// writeTestWAV emits a real 16-bit mono PCM header plus a short silence, so
// ffmpeg and the speech engine get something they can genuinely decode.
func writeTestWAV(t *testing.T, path string) {
	t.Helper()
	const (
		sampleRate = 16000
		samples    = sampleRate / 2 // half a second
		dataBytes  = samples * 2
	)
	buf := make([]byte, 0, 44+dataBytes)
	le32 := func(v uint32) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		return b
	}
	le16 := func(v uint16) []byte {
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, v)
		return b
	}
	buf = append(buf, []byte("RIFF")...)
	buf = append(buf, le32(36+dataBytes)...)
	buf = append(buf, []byte("WAVEfmt ")...)
	buf = append(buf, le32(16)...) // PCM header size
	buf = append(buf, le16(1)...)  // PCM
	buf = append(buf, le16(1)...)  // mono
	buf = append(buf, le32(sampleRate)...)
	buf = append(buf, le32(sampleRate*2)...) // byte rate
	buf = append(buf, le16(2)...)            // block align
	buf = append(buf, le16(16)...)           // bits per sample
	buf = append(buf, []byte("data")...)
	buf = append(buf, le32(dataBytes)...)
	buf = append(buf, make([]byte, dataBytes)...)

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("wav fixture: %v", err)
	}
}

// writeCoverageSkill installs one SKILL.md into the isolated discovery dir so
// skills_list has a real entry and skill_view a real body. Written to
// DefaultSkillsDir — under the temp home isolateUserDirs set up — because the
// progressive tools scan the disk at call time, not a path frozen at registry
// build.
func writeCoverageSkill(t *testing.T) {
	t.Helper()
	dir := filepath.Join(skill.DefaultSkillsDir(), "coverage-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("skill fixture dir: %v", err)
	}
	content := "---\nname: coverage-skill\ndescription: proves discovery works\n---\nthe coverage body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("skill fixture: %v", err)
	}
}
