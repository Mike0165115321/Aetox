package main

// Tests for the agent's reach onto the workbench (workbench_desk.go).
//
// Named for the workbench, not the desk, because `desk_test.go` next door is
// about a different thing entirely — the mode a session was opened at (§83).
// The word carries two meanings in this repo; the file names should not make
// that worse.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/skill"
)

type emitted struct {
	Name string
	Data []any
}

// captureEvents replaces the Wails emitter with a recorder. The real one calls
// log.Fatalf when ctx is not Wails-bound, which is never in a unit test — see
// the `emit` field's comment in app.go.
func captureEvents(a *App) *[]emitted {
	events := &[]emitted{}
	a.emit = func(name string, data ...any) {
		*events = append(*events, emitted{name, data})
	}
	a.ctx = context.Background()
	return events
}

func TestDeskOpenEmitsForAnExistingFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "report.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.cur().cfg.SandboxRoot = root
	events := captureEvents(a)

	out, err := (&deskOpenSkill{app: a, conv: a.cur()}).open("report.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !out.Success {
		t.Error("Success = false")
	}
	if len(*events) != 1 || (*events)[0].Name != "workbench:open-file" {
		t.Fatalf("events = %+v, want one workbench:open-file", *events)
	}
	payload, ok := (*events)[0].Data[0].(map[string]string)
	if !ok || payload["path"] != "report.pdf" || payload["name"] != "report.pdf" {
		t.Errorf("payload = %+v", (*events)[0].Data[0])
	}
}

// The error belongs in the turn, not on screen: a tab opened onto a missing
// path shows a card reading "this file is gone", which reads as the agent
// having lost the file it just made.
func TestDeskOpenRefusesBeforeOpeningATab(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "desk-secret.txt")
	_ = os.WriteFile(outside, []byte("private"), 0o644)
	t.Cleanup(func() { os.Remove(outside) })

	for name, path := range map[string]string{
		"missing":  "nope.pdf",
		"escaping": "../desk-secret.txt",
		"empty":    "",
		"dir":      ".",
	} {
		t.Run(name, func(t *testing.T) {
			a := &App{}
			a.cur().cfg.SandboxRoot = root
			events := captureEvents(a)

			if _, err := (&deskOpenSkill{app: a, conv: a.cur()}).open(path); err == nil {
				t.Error("err = nil, want a refusal")
			}
			if len(*events) != 0 {
				t.Errorf("emitted %+v, want nothing", *events)
			}
		})
	}
}

func TestDeskOpenWithoutProject(t *testing.T) {
	a := &App{}
	captureEvents(a)
	if _, err := (&deskOpenSkill{app: a, conv: a.cur()}).open("a.png"); err == nil {
		t.Error("err = nil, want 'no project open'")
	}
}

// §81 kept the user's browsing out of the agent's reach on purpose. desk_list
// must not become the door it walks in through.
func TestDeskListRedactsTheUsersOwnBrowsing(t *testing.T) {
	lines := describeDesk([]DeskTab{
		{Kind: "browser", Name: "mail.google.com", URL: "https://mail.google.com/u/0", Mine: false},
		{Kind: "browser", Name: "localhost", URL: "http://localhost:5173", Mine: true},
		{Kind: "file", Name: "report.pdf", Path: "out/report.pdf"},
		{Kind: "terminal", Name: "PowerShell"},
	})
	all := strings.Join(lines, "\n")

	for _, leaked := range []string{"mail.google.com", "https://mail.google.com/u/0"} {
		if strings.Contains(all, leaked) {
			t.Errorf("leaked %q from a tab the user opened:\n%s", leaked, all)
		}
	}
	// ...while everything the agent put there, and everything that is not
	// browsing at all, still reports in full — a redaction that hid the file it
	// just opened would make the tool useless.
	for _, want := range []string{"http://localhost:5173", "out/report.pdf", "PowerShell"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q:\n%s", want, all)
		}
	}
	if len(lines) != 4 {
		t.Errorf("got %d lines, want 4 — a redacted tab still has to be reported as existing", len(lines))
	}
}

func TestDeskListEmpty(t *testing.T) {
	out, err := (&deskListSkill{app: NewApp(), conv: newConversation()}).list()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "โต๊ะว่าง") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestWorkbenchTabsChangedRoundTrip(t *testing.T) {
	a := &App{}
	a.WorkbenchTabsChanged(a.cur().id, []DeskTab{{Kind: "file", Name: "a.md", Path: "a.md"}})
	if got := deskTabsOf(a.cur()); len(got) != 1 || got[0].Path != "a.md" {
		t.Fatalf("deskTabs = %+v", got)
	}
	// The frontend is the source of truth, so a later report replaces rather
	// than appends — a desk the user emptied must come back empty.
	a.WorkbenchTabsChanged(a.cur().id, nil)
	if got := deskTabsOf(a.cur()); len(got) != 0 {
		t.Errorf("deskTabs = %+v, want empty", got)
	}
}

// desk_terminal runs a real command in a real shell. If it did not reach the
// same gate as `shell`, it would be a second and quieter way to run anything.
func TestDeskTerminalIsAssessedAsShell(t *testing.T) {
	risky := safety.AssessCommand("desk_terminal", []string{"rm", "-rf", "/"})
	if risky.Risk != safety.RiskHigh {
		t.Errorf("rm -rf / assessed %v, want RiskHigh", risky.Risk)
	}
	if risky.SkillName != "desk_terminal" {
		t.Errorf("SkillName = %q, want desk_terminal — the prompt has to name the tool being run", risky.SkillName)
	}
	if shell := safety.AssessCommand("shell", []string{"rm", "-rf", "/"}); shell.Risk != risky.Risk {
		t.Errorf("desk_terminal %v vs shell %v — the same command must be judged the same way", risky.Risk, shell.Risk)
	}

	// An empty terminal runs nothing until the user types, and what they type is
	// theirs. Assessing it as "shell with no command" would put an approval
	// prompt in front of opening a window.
	if empty := safety.AssessCommand("desk_terminal", nil); empty.Risk != safety.RiskLow {
		t.Errorf("empty terminal assessed %v, want RiskLow", empty.Risk)
	}
}

// The bug the owner hit on 2026-08-20, one line after the model wrote the file:
//
//	desk_open xiaomi-17t-pro-sales.html
//	xiaomi-17t-pro-sales.html does not exist
//
// Unfocused, `write` steers a new relative file into output/<session> and its
// receipt echoes the path the model ASKED for — which is also what this tool's
// description tells the model to pass. desk_open resolved that name straight
// off the sandbox root, found nothing, and reported the file missing while it
// sat one folder down.
//
// `browser open` has resolved this correctly since it was written
// (normalizeWorkbenchURL). Two tools answering "where is this file" two
// different ways is the debt this repo calls หนี้ในระบบ; both now go through
// skill.PlacedPath.
func TestDeskOpenFindsWhatWriteJustPlacedInTheOutputFolder(t *testing.T) {
	root := t.TempDir()
	a := &App{}
	a.cur().cfg.SandboxRoot = root
	a.cur().id = "20260820-063040.715"
	subdir := filepath.Join(root, "output", a.cur().id)
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "deck.html"), []byte("<section class=\"slide\"></section>"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := captureEvents(a)

	out, err := (&deskOpenSkill{app: a, conv: a.cur()}).open("deck.html")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false, content = %q", out.Content)
	}
	payload, ok := (*events)[0].Data[0].(map[string]string)
	if !ok {
		t.Fatalf("payload = %+v", (*events)[0].Data[0])
	}
	// The PLACED path travels on, because nothing downstream knows the rule:
	// the tab, ReadFile and the file host all resolve straight off the root.
	if payload["path"] != "output/20260820-063040.715/deck.html" {
		t.Errorf("path = %q, want the placed path", payload["path"])
	}
	if payload["name"] != "deck.html" {
		t.Errorf("name = %q, want the bare file name", payload["name"])
	}
}

// A file that really is at the root still wins, so an artifact of the same name
// in the output folder cannot shadow it (PlacedPath's own rule).
func TestDeskOpenPrefersTheLiteralPathOverTheOutputFolder(t *testing.T) {
	root := t.TempDir()
	a := &App{}
	a.cur().cfg.SandboxRoot = root
	a.cur().id = "20260820-063040.715"
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(root, "output", a.cur().id)
	_ = os.MkdirAll(subdir, 0o755)
	_ = os.WriteFile(filepath.Join(subdir, "notes.md"), []byte("artifact"), 0o644)
	events := captureEvents(a)

	if _, err := (&deskOpenSkill{app: a, conv: a.cur()}).open("notes.md"); err != nil {
		t.Fatalf("open: %v", err)
	}
	payload := (*events)[0].Data[0].(map[string]string)
	if payload["path"] != "notes.md" {
		t.Errorf("path = %q, want the literal path to win", payload["path"])
	}
}

// ---------------------------------------------------------------------------
// the desk as one packed tool (2026-08-20)
// ---------------------------------------------------------------------------

// One name in the block, three rights inside it. What the block shows is
// `desk`; what every gate below it judges is still desk_open / desk_list /
// desk_close, because the act has not changed (§99.1).
func TestTheDeskIsOneToolWithThreeActionsInside(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.cur().cfg.SandboxRoot = root
	events := captureEvents(a)
	tool := &deskSkill{app: a, conv: a.cur()}

	if got := tool.Name(); got != "desk" {
		t.Errorf("Name = %q, want desk", got)
	}
	if got := skill.Unpack("desk", map[string]any{"action": "close"}); got != "desk_close" {
		t.Errorf("Unpack = %q, want desk_close — the gates below the block judge the act, not the pack", got)
	}

	// A call with no action at all is an open, because that is the call every
	// habit makes and the one the fallback exists for.
	out, err := tool.run(map[string]any{"path": "note.md"})
	if err != nil || !out.Success {
		t.Fatalf("bare call: %v / %+v", err, out)
	}
	if out.Name != "desk_open" {
		t.Errorf("Name = %q, want the per-action name in the timeline", out.Name)
	}

	out, err = tool.run(map[string]any{"action": "list"})
	if err != nil || !out.Success {
		t.Fatalf("list: %v / %+v", err, out)
	}
	if len(*events) != 1 {
		t.Errorf("events = %d, want one (the open) — list touches no window", len(*events))
	}
}

// A profile naming only some of the actions gets exactly those, and the
// description advertises only those — the rule that makes a pack a set of
// rights rather than one (§99.2). An AGENT.md written before the packing says
// `tools: desk_open, desk_list`, and that has to keep meaning what it said.
func TestADeskNarrowedToTheActionsAProfileNamesRefusesTheRest(t *testing.T) {
	a := &App{}
	a.cur().cfg.SandboxRoot = t.TempDir()
	captureEvents(a)

	narrowed, ok := (&deskSkill{app: a, conv: a.cur()}).Narrow([]string{"desk_open", "desk_list"}).(*deskSkill)
	if !ok {
		t.Fatal("Narrow did not return a desk")
	}
	if got := narrowed.allowedActions(); !slices.Equal(got, []string{"open", "list"}) {
		t.Fatalf("actions = %v, want open+list", got)
	}
	body, _ := json.Marshal(narrowed.ToolDefinition())
	if strings.Contains(string(body), "`close`") {
		t.Error("the description advertises an action this caller would be refused")
	}
	if _, err := narrowed.run(map[string]any{"action": "close", "path": "note.md"}); err == nil {
		t.Error("close ran on a desk narrowed to open+list")
	}

	// Naming nothing asks for the tool whole, not for an empty one — the
	// silence rule, and the failure it prevents is a tool that refuses every
	// call while every screen says the agent is equipped.
	whole := (&deskSkill{app: a, conv: a.cur()}).Narrow(nil).(*deskSkill)
	if got := whole.allowedActions(); len(got) != 3 {
		t.Errorf("actions = %v, want all three", got)
	}
}

// §81 says what the user is doing on their own machine is not the agent's to
// read. Taking a file off their desk is the same rule with a heavier hand.
func TestDeskCloseOnlyTakesBackWhatTheAgentPutThere(t *testing.T) {
	root := t.TempDir()
	a := &App{}
	a.cur().cfg.SandboxRoot = root
	events := captureEvents(a)

	a.WorkbenchTabsChanged(a.cur().id, []DeskTab{
		{Kind: "file", Name: "mine.md", Path: "mine.md", Mine: true},
		{Kind: "file", Name: "theirs.md", Path: "theirs.md"},
	})
	tool := &deskCloseSkill{app: a, conv: a.cur()}

	if _, err := tool.close("theirs.md"); err == nil {
		t.Error("closed a file the user opened")
	}
	if _, err := tool.close("gone.md"); err == nil {
		t.Error("closed a file that is not on the desk")
	}
	if len(*events) != 0 {
		t.Fatalf("events = %+v, want none — both calls were refused", *events)
	}

	out, err := tool.close("mine.md")
	if err != nil || !out.Success {
		t.Fatalf("close: %v / %+v", err, out)
	}
	if len(*events) != 1 || (*events)[0].Name != "workbench:close-file" {
		t.Fatalf("events = %+v, want one workbench:close-file", *events)
	}
}

// The anatomy of a deck reaches the writer exactly once, on the first `open` of
// a session (internal/skill/guidance.go). §149 found the marker living "in no
// prompt, no profile and no tool description" and closed it for the document
// agents; the assistant wrote a deck with its own navigation on 2026-08-20
// because nothing had told it what the room does with the file.
func TestDeskOpenTeachesWhatMakesAnHTMLFileADeck(t *testing.T) {
	guidance := (&deskSkill{}).Guidance(map[string]any{"action": "open"})
	// Three things, and the third is why the other two can stay this short: the
	// marker, the half today's bug turned on (the room pages the deck, so a deck
	// that pages itself is one the room cannot drive), and where the rest is.
	// The full recipe — sizing, assets, the skeleton — is the bundled
	// `aetox-slides` skill, which is read BEFORE a deck is written; this arrives
	// with the first desk_open, which is after.
	for _, want := range []string{`<section class="slide">`, "navigation", "aetox-slides"} {
		if !strings.Contains(guidance, want) {
			t.Errorf("guidance does not mention %s:\n%s", want, guidance)
		}
	}
	// Nothing to say once about an action whose signature already says it all.
	if got := (&deskSkill{}).Guidance(map[string]any{"action": "nonsense"}); got != "" {
		t.Errorf("guidance for an unknown action = %q, want empty", got)
	}
}
