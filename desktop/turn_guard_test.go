package main

// The turn in flight's identity, and the doors that respect it.
//
// Two bugs shipped together and were really one: a turn's answer had no home
// of its own. Reloading the window mid-turn lost the live work (the reply's
// only route back was the dead webview's promise), and switching chats
// mid-turn carried the answer into the newly opened session — persisted there,
// because appendTurn read a.sessionID at completion time. The fix is the stamp
// (beginTurn) plus one shared gate on every door that moves a.sessionID or the
// agent's context while a turn runs.

import (
	"strings"
	"testing"
)

// Stop pressed in the beginTurn → armTurnCancel gap (openTurn's DB writes sit
// there, seconds long on a busy database) used to land on a nil cancel func
// and silently do nothing — a Stop button that sometimes needed two presses.
// The press is remembered and consumed the moment the cancel func exists.
func TestStopPressedBeforeTheCancelFuncExistsStillStops(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	defer a.endTurn()

	a.CancelTurn() // turnCancel is nil here — the gap

	pressed := false
	if !a.armTurnCancel(func() { pressed = true }) {
		t.Fatal("armTurnCancel = false after a Stop in the gap, want true — the press was dropped")
	}
	// armTurnCancel reports; runTurn is the one that pulls the trigger.
	if pressed {
		t.Error("armTurnCancel called the cancel func itself — that is the caller's decision")
	}

	// And the flag is consumed: the next turn must not inherit this press.
	if a.armTurnCancel(func() {}) {
		t.Error("a second armTurnCancel = true, want false — one press stops one turn")
	}
}

func TestBeginTurnRefusesASecondTurn(t *testing.T) {
	a := newTestApp(t, t.TempDir())

	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() on an idle engine = %v, want nil", err)
	}
	if err := a.beginTurn(); err == nil {
		t.Fatal("a second beginTurn() while one runs = nil, want the busy refusal — two turns share one agent context")
	}
	a.endTurn()
	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() after endTurn() = %v, want nil — the gate must reopen", err)
	}
}

// The answer goes to the session the turn was born in, not to whatever
// a.sessionID has become by the time the turn finishes. The doors refuse to
// move it mid-turn, so this only fires if one is ever left unguarded — which
// is exactly when it must hold.
func TestAppendTurnWritesToTheSessionTheTurnWasBornIn(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	home := a.sessionID

	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	// An unguarded door moving the engine to another chat mid-turn.
	elsewhere := "20990101-000000.000"
	a.sessionID = elsewhere

	id := a.appendTurn(
		SessionMessage{Role: "user", Text: "คำถามของแชทเดิม"},
		SessionMessage{Role: "agent", Text: "คำตอบต้องกลับบ้านถูกหลัง"},
	)
	a.endTurn()
	if id == 0 {
		t.Fatal("appendTurn wrote nothing")
	}

	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	var atHome, strayed int
	_ = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, home).Scan(&atHome)
	_ = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, elsewhere).Scan(&strayed)
	if atHome != 2 {
		t.Errorf("messages in the turn's own session = %d, want 2", atHome)
	}
	if strayed != 0 {
		t.Errorf("messages in the chat opened mid-turn = %d, want 0 — this is the answer that used to follow the user", strayed)
	}
}

func TestSwitchDoorsRefuseWhileATurnRuns(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	current := a.sessionID
	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}

	busy := func(name string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "กำลังทำงานอยู่") {
			t.Errorf("%s mid-turn = %v, want the busy refusal", name, err)
		}
	}
	_, err := a.LoadSession("some-id")
	busy("LoadSession", err)
	_, err = a.LoadSessionAnyProject("some-id")
	busy("LoadSessionAnyProject", err)
	_, err = a.NewSession()
	busy("NewSession", err)
	_, err = a.NewSessionAt("assistant")
	busy("NewSessionAt", err)
	_, err = a.NewChairSession("someone")
	busy("NewChairSession", err)
	_, err = a.OpenProjectPath(t.TempDir())
	busy("OpenProjectPath", err)
	_, err = a.ClearProjectFocus()
	busy("ClearProjectFocus", err)
	busy("DeleteSession(open)", a.DeleteSession(current))
	// Restarting into a new build kills the process, and the process is where
	// the turn lives. Downloading one does not, and StageUpdate is deliberately
	// not on this list (§107) — bytes coming down interrupt nothing.
	busy("RestartToUpdate", a.RestartToUpdate())

	// Any OTHER session's row is not something the turn holds — deleting it
	// stays allowed, or a long turn would freeze the whole history list.
	if err := a.DeleteSession("someone-elses-old-chat"); err != nil {
		t.Errorf("DeleteSession(other) mid-turn = %v, want nil", err)
	}

	if a.sessionID != current {
		t.Errorf("a.sessionID moved to %q during the refusals, want it pinned at %q", a.sessionID, current)
	}

	a.endTurn()
	if _, err := a.NewSession(); err != nil {
		t.Errorf("NewSession after the turn = %v, want nil", err)
	}
}

// endTurn tells every window the turn is over — including the window that was
// reloaded mid-turn and has no promise left to resolve. The event names the
// session that finished, because the listener's own idea of "current" is
// exactly what a reload just wiped.
func TestEndTurnAnnouncesTheSessionThatFinished(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	home := a.sessionID

	var event string
	var got TurnStatus
	a.emit = func(name string, data ...any) {
		event = name
		if len(data) == 1 {
			if s, ok := data[0].(TurnStatus); ok {
				got = s
			}
		}
	}

	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	a.endTurn()

	if event != "agent:done" {
		t.Fatalf("event = %q, want agent:done", event)
	}
	if got.Running || got.SessionID != home {
		t.Errorf("agent:done payload = %+v, want Running=false SessionID=%q", got, home)
	}
}

func TestTurnInFlightReportsTheRunningTurn(t *testing.T) {
	a := newTestApp(t, t.TempDir())

	if s := a.TurnInFlight(); s.Running {
		t.Errorf("TurnInFlight on an idle engine = %+v, want Running=false", s)
	}
	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	if s := a.TurnInFlight(); !s.Running || s.SessionID != a.sessionID {
		t.Errorf("TurnInFlight mid-turn = %+v, want Running=true SessionID=%q", s, a.sessionID)
	}
	a.endTurn()
	if s := a.TurnInFlight(); s.Running {
		t.Errorf("TurnInFlight after endTurn = %+v, want Running=false", s)
	}
}

// SessionTranscript is a read, not a switch: the reloaded window uses it to put
// the conversation back on screen while the engine may still be working in it.
// It must not move a.sessionID, and it must answer even while a turn runs —
// refusing here would hand the reloaded window a welcome screen over a working
// agent, the exact bug it exists to end.
func TestSessionTranscriptReadsWithoutSwitching(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	first := a.sessionID
	a.appendTurn(
		SessionMessage{Role: "user", Text: "สวัสดี"},
		SessionMessage{Role: "agent", Text: "ครับ"},
	)
	a.startNewSession()
	second := a.sessionID

	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	defer a.endTurn()

	messages, err := a.SessionTranscript(first)
	if err != nil {
		t.Fatalf("SessionTranscript mid-turn = %v, want the rows", err)
	}
	if len(messages) != 2 || messages[0].Text != "สวัสดี" {
		t.Errorf("messages = %+v, want the stored pair", messages)
	}
	if a.sessionID != second {
		t.Errorf("a.sessionID = %q after the read, want it untouched at %q", a.sessionID, second)
	}

	// A session with no rows yet — opened, never spoken to — is an empty list,
	// not an error: the welcome screen is the honest answer for it.
	if empty, err := a.SessionTranscript("never-spoken-to"); err != nil || len(empty) != 0 {
		t.Errorf("SessionTranscript(unknown) = %v, %v — want an empty list, nil", empty, err)
	}
}
