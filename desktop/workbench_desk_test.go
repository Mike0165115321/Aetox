package main

// Tests for the agent's reach onto the workbench (workbench_desk.go).
//
// Named for the workbench, not the desk, because `desk_test.go` next door is
// about a different thing entirely — the mode a session was opened at (§83).
// The word carries two meanings in this repo; the file names should not make
// that worse.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/safety"
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
	a.cfg.SandboxRoot = root
	events := captureEvents(a)

	out, err := (&deskOpenSkill{app: a}).open("report.pdf")
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
			a.cfg.SandboxRoot = root
			events := captureEvents(a)

			if _, err := (&deskOpenSkill{app: a}).open(path); err == nil {
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
	if _, err := (&deskOpenSkill{app: a}).open("a.png"); err == nil {
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
	out, err := (&deskListSkill{app: &App{}}).list()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "โต๊ะว่าง") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestWorkbenchTabsChangedRoundTrip(t *testing.T) {
	a := &App{}
	a.WorkbenchTabsChanged([]DeskTab{{Kind: "file", Name: "a.md", Path: "a.md"}})
	if got := a.deskTabs(); len(got) != 1 || got[0].Path != "a.md" {
		t.Fatalf("deskTabs = %+v", got)
	}
	// The frontend is the source of truth, so a later report replaces rather
	// than appends — a desk the user emptied must come back empty.
	a.WorkbenchTabsChanged(nil)
	if got := a.deskTabs(); len(got) != 0 {
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
