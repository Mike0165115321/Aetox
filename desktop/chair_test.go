package main

// What a chair does to a real engine (§85).
//
// internal/subagent proves FilterRegistry cuts a profile against a ceiling;
// none of that would catch a chair chat mounted on the wrong prompt, or a
// deleted profile quietly answered by the main assistant wearing the chair's
// history. So, like desk_test.go, every assertion here goes through the
// engine the app actually builds.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// A direct chat with a chair must send the model the chair's cut — profile ∩
// office ceiling — and run on the chair's own prompt, not the office desk's.
func TestAChairSessionSendsTheChairsCutOnly(t *testing.T) {
	a := bootDeskApp(t, "")
	if _, err := a.NewChairSession("deck"); err != nil {
		t.Fatalf("NewChairSession(deck): %v", err)
	}

	got := toolNames(a)
	for _, want := range []string{"slides_write", "read", "pdf_read", "web_search"} {
		if !slices.Contains(got, want) {
			t.Errorf("deck's chat is missing %s — its own profile asks for it: %v", want, got)
		}
	}
	// task: a leaf stays a leaf, even when spoken to. shell: the office
	// ceiling, in person. ask_user/todo_write: forced denials shared with the
	// delegate path — one invariant, not two.
	for _, banned := range []string{"task", "task_result", "shell", "diagnostics", "ask_user", "todo_write"} {
		if slices.Contains(got, banned) {
			t.Errorf("deck's chat carries %s — the chair's cut must match its delegate runs", banned)
		}
	}

	messages := a.agent.ContextMessages()
	if len(messages) == 0 {
		t.Fatal("no system prompt")
	}
	sys := messages[0].Content
	if !strings.Contains(sys, "You build one presentation") {
		t.Error("the chair chat does not run on the chair's own prompt")
	}
	if strings.Contains(sys, "This session is deliverable work") {
		t.Error("the office desk's direction leaked into a chair chat — one brief, not two masters")
	}
}

// The door validates loudly: a name nothing answers to, and a profile that
// never declared the office, are both refused — not fallen back on. A picker
// with a stale name must never quietly open a chat as somebody else.
func TestNewChairSessionRefusesStrangers(t *testing.T) {
	a := bootDeskApp(t, "")
	if _, err := a.NewChairSession("no-such-agent"); err == nil {
		t.Error("an unknown agent name opened a session")
	}
	// `explore` is a real bundled profile — but a delegate, not a chair.
	if _, err := a.NewChairSession("explore"); err == nil {
		t.Error("a non-office profile opened a direct chat")
	}
	if a.chair != "" {
		t.Errorf("a refused door still seated the session at %q", a.chair)
	}
}

// Reopening a chair session restores the chair; reopening one whose profile
// file is gone refuses, exactly as a deleted desk manifest does. The transcript
// is intact either way — what must never happen is the main assistant
// answering under the chair's name.
func TestReopeningAChairSessionRestoresTheChairOrRefuses(t *testing.T) {
	a := bootDeskApp(t, "")

	// A user-authored chair, so the file can actually be deleted — the bundled
	// ones are compiled in and cannot go missing. In the agents' home: since
	// the homes split, that is what makes it a chair at all.
	dir, err := subagent.AgentsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(dir, "painter.md")
	if err := os.WriteFile(profile, []byte("---\ndescription: วาดภาพ\ndesk: specialized\ntools: read, list\n---\npaint things"), 0o644); err != nil {
		t.Fatal(err)
	}

	id, err := a.NewChairSession("painter")
	if err != nil {
		t.Fatalf("NewChairSession(painter): %v", err)
	}
	now := time.Now().Format(time.RFC3339)
	a.appendTurn(
		SessionMessage{Role: "user", Text: "สวัสดี", Time: now},
		SessionMessage{Role: "agent", Text: "สวัสดีครับ", Time: now},
	)

	// Walk away to the plain desk, then come back.
	if err := a.setStation("", ""); err != nil {
		t.Fatalf("leaving the chair: %v", err)
	}
	if _, err := a.LoadSession(id); err != nil {
		t.Fatalf("reopening the chair session: %v", err)
	}
	if a.chair != "painter" {
		t.Fatalf("reopened session sits at %q, want painter", a.chair)
	}

	// Delete the profile and try again: refusal, with the chair named.
	if err := a.setStation("", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(profile); err != nil {
		t.Fatal(err)
	}
	if _, err := a.LoadSession(id); err == nil {
		t.Fatal("a chair session reopened without its profile file")
	}
	if a.chair != "" {
		t.Errorf("a refused reopen still seated the session at %q", a.chair)
	}
}
