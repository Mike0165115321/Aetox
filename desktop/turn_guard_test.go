package main

// The turn in flight's identity, and the doors that respect it.
//
// Two bugs shipped together and were really one: a turn's answer had no home
// of its own. Reloading the window mid-turn lost the live work (the reply's
// only route back was the dead webview's promise), and switching chats
// mid-turn carried the answer into the newly opened session — persisted there,
// because appendTurn read a.cur().id at completion time. The fix is the stamp
// (beginTurn) plus one shared gate on every door that moves a.cur().id or the
// agent's context while a turn runs.

import (
	"context"
	"strings"
	"testing"
)

// Stop pressed in the beginTurn → armTurnCancel gap (openTurn's DB writes sit
// there, seconds long on a busy database) used to land on a nil cancel func
// and silently do nothing — a Stop button that sometimes needed two presses.
// The press is remembered and consumed the moment the cancel func exists.
func TestStopPressedBeforeTheCancelFuncExistsStillStops(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if err := a.beginTurn(a.cur().id); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	defer a.endTurn(a.cur().id)

	a.CancelTurn() // turnCancel is nil here — the gap

	pressed := false
	if !a.armTurnCancel(a.cur().id, context.Background(), func() { pressed = true }) {
		t.Fatal("armTurnCancel = false after a Stop in the gap, want true — the press was dropped")
	}
	// armTurnCancel reports; runTurn is the one that pulls the trigger.
	if pressed {
		t.Error("armTurnCancel called the cancel func itself — that is the caller's decision")
	}

	// And the flag is consumed: the next turn must not inherit this press.
	if a.armTurnCancel(a.cur().id, context.Background(), func() {}) {
		t.Error("a second armTurnCancel = true, want false — one press stops one turn")
	}
}

func TestBeginTurnRefusesASecondTurn(t *testing.T) {
	a := newTestApp(t, t.TempDir())

	if err := a.beginTurn(a.cur().id); err != nil {
		t.Fatalf("beginTurn() on an idle engine = %v, want nil", err)
	}
	if err := a.beginTurn(a.cur().id); err == nil {
		t.Fatal("a second beginTurn() while one runs = nil, want the busy refusal — two turns share one agent context")
	}
	a.endTurn(a.cur().id)
	if err := a.beginTurn(a.cur().id); err != nil {
		t.Fatalf("beginTurn() after endTurn() = %v, want nil — the gate must reopen", err)
	}
}

// The answer goes to the session the turn was born in, not to whatever
// a.cur().id has become by the time the turn finishes. The doors refuse to
// move it mid-turn, so this only fires if one is ever left unguarded — which
// is exactly when it must hold.
func TestAppendTurnWritesToTheSessionTheTurnWasBornIn(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	// What SendMessage does on its first line: hold the conversation, not a
	// cursor to be read again later.
	conv := a.cur()
	home := conv.id

	if err := a.beginTurn(home); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	// The window moves to another chat mid-turn — which is an ordinary thing to
	// do now rather than a door left unguarded. It puts a different
	// conversation on screen and touches the running turn's not at all.
	elsewhere := "20990101-000000.000"
	a.convs.show(&conversation{id: elsewhere})

	id := a.appendTurn(conv,
		SessionMessage{Role: "user", Text: "คำถามของแชทเดิม"},
		SessionMessage{Role: "agent", Text: "คำตอบต้องกลับบ้านถูกหลัง"},
	)
	a.endTurn(home)
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

// Which doors still refuse while a turn is running, and — the half this test
// was rewritten for on 2026-08-19 — which ones stopped.
//
// It used to assert that every one of them refused. That was right when there
// was one agent context: opening or starting a chat rewrote the memory the
// running turn was thinking with, so the only honest answer was "finish or stop
// first". Each chat has its own engine now, so opening one builds or attaches
// beside the turn instead of on top of it, and the refusal became a wall around
// a hazard that no longer exists.
//
// What is left refusing is the state that really is shared: the project. It
// moves the sandbox root, the workspace and the shell backend, which belong to
// the machine rather than to any conversation — a turn running anywhere would
// find the ground moved under it mid-tool-call. Same for restarting into a new
// build, which ends the process the turn lives in, and for deleting the row the
// turn is writing into.
func TestTheDoorsThatStillRefuseWhileATurnRuns(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	working := a.cur().id
	if err := a.beginTurn(working); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}

	busy := func(name string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "กำลังทำงานอยู่") {
			t.Errorf("%s mid-turn = %v, want the busy refusal", name, err)
		}
	}
	_, err := a.LoadSessionAnyProject("some-id")
	busy("LoadSessionAnyProject", err)
	_, err = a.OpenProjectPath(t.TempDir())
	busy("OpenProjectPath", err)
	_, err = a.ClearProjectFocus()
	busy("ClearProjectFocus", err)
	busy("DeleteSession(open)", a.DeleteSession(working))
	// Restarting into a new build kills the process, and the process is where
	// the turn lives. Downloading one does not, and StageUpdate is deliberately
	// not on this list (§107) — bytes coming down interrupt nothing.
	busy("RestartToUpdate", a.RestartToUpdate())
	// The stance rebuilds the engine of the chat on screen, carrying its
	// context over — which is the same hazard, aimed at one conversation. The
	// chat on screen IS the working one here.
	if _, err := a.SetStance("วางแผน"); err == nil {
		t.Error("SetStance rebuilt the engine of the chat the turn is running in")
	}

	// Any OTHER session's row is not something the turn holds — deleting it
	// stays allowed, or a long turn would freeze the whole history list.
	if err := a.DeleteSession("someone-elses-old-chat"); err != nil {
		t.Errorf("DeleteSession(other) mid-turn = %v, want nil", err)
	}

	if a.cur().id != working {
		t.Errorf("a.cur().id moved to %q during the refusals, want it pinned at %q", a.cur().id, working)
	}
	a.endTurn(working)
}

// And the doors that stopped refusing, which is the capability the owner asked
// for three times (§134.4, 19 ส.ค.): a turn running in one chat, and the user
// working in another.
func TestOpeningAnotherChatLeavesTheRunningTurnAlone(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	working := a.cur()
	if err := a.beginTurn(working.id); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	defer a.endTurn(working.id)

	if _, err := a.NewSession(); err != nil {
		t.Fatalf("starting a chat while another works = %v, want nil", err)
	}
	fresh := a.cur()
	if fresh == working {
		t.Fatal("the new chat is the working one — startNewSession emptied it instead of opening another")
	}
	if fresh.id == working.id {
		t.Errorf("the new chat took the working chat's id %q", working.id)
	}
	// The turn is still where it was, and its conversation is untouched: same
	// object, same id, still marked as working.
	if !a.turnRunningIn(working.id) {
		t.Error("the running turn was lost when another chat opened")
	}
	// And the working chat is still held, because work is what holds it — the
	// window is not looking at it any more.
	if a.convs.find(working.id) != working {
		t.Error("the working chat's engine was let go of while its turn was still running")
	}
}

// endTurn tells every window the turn is over — including the window that was
// reloaded mid-turn and has no promise left to resolve. The event names the
// session that finished, because the listener's own idea of "current" is
// exactly what a reload just wiped.
func TestEndTurnAnnouncesTheSessionThatFinished(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	home := a.cur().id

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

	if err := a.beginTurn(a.cur().id); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	a.endTurn(a.cur().id)

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
	if err := a.beginTurn(a.cur().id); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	if s := a.TurnInFlight(); !s.Running || s.SessionID != a.cur().id {
		t.Errorf("TurnInFlight mid-turn = %+v, want Running=true SessionID=%q", s, a.cur().id)
	}
	a.endTurn(a.cur().id)
	if s := a.TurnInFlight(); s.Running {
		t.Errorf("TurnInFlight after endTurn = %+v, want Running=false", s)
	}
}

// SessionTranscript is a read, not a switch: the reloaded window uses it to put
// the conversation back on screen while the engine may still be working in it.
// It must not move a.cur().id, and it must answer even while a turn runs —
// refusing here would hand the reloaded window a welcome screen over a working
// agent, the exact bug it exists to end.
func TestSessionTranscriptReadsWithoutSwitching(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	first := a.cur().id
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "สวัสดี"},
		SessionMessage{Role: "agent", Text: "ครับ"},
	)
	a.startNewSession()
	second := a.cur().id

	if err := a.beginTurn(a.cur().id); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	defer a.endTurn(a.cur().id)

	messages, err := a.SessionTranscript(first)
	if err != nil {
		t.Fatalf("SessionTranscript mid-turn = %v, want the rows", err)
	}
	if len(messages) != 2 || messages[0].Text != "สวัสดี" {
		t.Errorf("messages = %+v, want the stored pair", messages)
	}
	if a.cur().id != second {
		t.Errorf("a.cur().id = %q after the read, want it untouched at %q", a.cur().id, second)
	}

	// A session with no rows yet — opened, never spoken to — is an empty list,
	// not an error: the welcome screen is the honest answer for it.
	if empty, err := a.SessionTranscript("never-spoken-to"); err != nil || len(empty) != 0 {
		t.Errorf("SessionTranscript(unknown) = %v, %v — want an empty list, nil", empty, err)
	}
}
