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
	"errors"
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
	_, err := a.OpenProjectPath(t.TempDir())
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

// And every chat that is working, not only the one on screen.
//
// A reloaded window has no memory of anything: it asks this and draws what it
// is told. Told about one chat, it marked one and forgot the rest — rings that
// never came back on over turns that were still running, and no way to see from
// the list that they were.
func TestTurnInFlightNamesEveryWorkingChat(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	onScreen := a.cur().id
	elsewhere := "20990101-000000.000"

	if err := a.beginTurn(onScreen); err != nil {
		t.Fatalf("beginTurn(onScreen) = %v", err)
	}
	if err := a.beginTurn(elsewhere); err != nil {
		t.Fatalf("beginTurn(elsewhere) = %v", err)
	}

	got := a.TurnInFlight()
	if !got.Running || got.SessionID != onScreen {
		t.Errorf("the chat on screen = %+v, want Running=true SessionID=%q", got, onScreen)
	}
	if len(got.Working) != 2 {
		t.Fatalf("Working = %v, want both chats", got.Working)
	}
	if got.Working[0] != onScreen && got.Working[1] != onScreen {
		t.Errorf("Working = %v, missing the chat on screen %q", got.Working, onScreen)
	}
	if got.Working[0] != elsewhere && got.Working[1] != elsewhere {
		t.Errorf("Working = %v, missing the chat off screen %q", got.Working, elsewhere)
	}

	// The chat on screen finishes; the other one is still going, and the window
	// has to keep being told so.
	a.endTurn(onScreen)
	got = a.TurnInFlight()
	if got.Running || got.SessionID != "" {
		t.Errorf("after the open chat's turn ended = %+v, want Running=false with no session", got)
	}
	if len(got.Working) != 1 || got.Working[0] != elsewhere {
		t.Errorf("Working = %v, want only the chat still working", got.Working)
	}
	a.endTurn(elsewhere)
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

// The history list is clickable while the agent works — every row of it.
//
// The gate that made this false was at the top of LoadSessionAnyProject, which
// is the door the sidebar sends EVERY click through. It is there for the branch
// that re-roots the project, and re-rooting really does move the ground under a
// running turn — but the overwhelming majority of clicks are chats in the
// project already open, where nothing is re-rooted and nothing was ever at risk.
// The switch had been opened one door down and this one was still shut in front
// of it (owner, 20 ส.ค.: "ตอนมันทำงานผมกดไปเซสชั่นอื่นไม่ได้เลย").
func TestTheHistoryListOpensWhileATurnRuns(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	working := a.cur().id
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "คำถามเก่า", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "คำตอบเก่า", Time: "10:00"},
	)

	// A second chat in the SAME project, which is what a history row usually is.
	other := a.cur()
	// Same project — which is what a history row usually is, and the whole
	// point of the case. A conversation built by hand inherits nothing, so it
	// is given the config applyConfig would have given it.
	a.convs.show(&conversation{id: "20990101-000000.000", cfg: other.cfg})
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "คำถามอีกแชท", Time: "10:01"},
		SessionMessage{Role: "agent", Text: "คำตอบอีกแชท", Time: "10:01"},
	)
	elsewhere := a.cur().id
	a.convs.show(other)

	if err := a.beginTurn(working); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	defer a.endTurn(working)

	msgs, err := a.LoadSessionAnyProject(elsewhere)
	if err != nil {
		t.Fatalf("a history row refused to open mid-turn: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("opened with %d messages, want the stored pair", len(msgs))
	}
	if a.cur().id != elsewhere {
		t.Errorf("the window is on %q, want the chat that was clicked %q", a.cur().id, elsewhere)
	}
	// And the turn is untouched: still running, still in its own conversation.
	if !a.turnRunningIn(working) {
		t.Error("opening another chat ended the running turn")
	}
}

// LoadSessionAnyProject is two doors wearing one name, and only one of them is
// shut. Opening a chat in the project already open re-roots nothing and stays
// open mid-turn (TestTheHistoryListOpensWhileATurnRuns). Opening one that lives
// in ANOTHER project moves the sandbox, the workspace and the shell — the
// machine the running turn is working on — and that half still refuses.
func TestOpeningAChatInAnotherProjectStillRefusesMidTurn(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "คำถามในโปรเจกต์เก่า", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "คำตอบ", Time: "10:00"},
	)
	elsewhere := a.cur().id

	// The app moves to a different project, so that chat is now in another one.
	// Both roots, because they are two facts now: what a new chat is born with
	// (App.cfg) and what a chat is actually running in (conversation.cfg).
	moved := t.TempDir()
	a.cfg.SandboxRoot = moved
	a.cur().cfg.SandboxRoot = moved
	a.projectFocused = true

	working := "20990101-000000.000"
	a.convs.show(&conversation{id: working, cfg: a.cfg})
	if err := a.beginTurn(working); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	defer a.endTurn(working)

	_, err := a.LoadSessionAnyProject(elsewhere)
	if err == nil || !strings.Contains(err.Error(), "กำลังทำงานอยู่") {
		t.Errorf("a cross-project open mid-turn = %v, want the busy refusal", err)
	}
}

// A chat keeps the model it was left on, across a switch and across a restart.
//
// §155 put the dials on the conversation and gave them the lifetime of an
// engine, which is not long enough: a chat that goes idle and off screen has
// its engine let go, and reopening it rebuilt from App.cfg — the last model
// anyone chose anywhere. So the model still followed the user between chats,
// which is exactly what §155 was written to stop. The owner did not believe the
// claim and was right not to (20 ส.ค.: "เช็คดีๆ เหมือนจะไม่ใช่นะ").
//
// The lifetime that is long enough is the one desk, chair, space and stance
// already use: a column on the session row. An engine is a derivative that gets
// thrown away and rebuilt; what identifies a conversation cannot live in it.
func TestAChatKeepsTheModelItWasLeftOn(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	first := a.cur().id
	a.cur().cfg.ModelName = "model-of-chat-one"
	a.cur().cfg.ModelProvider = "provider-one"
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "คำถามแชทหนึ่ง", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "คำตอบ", Time: "10:00"},
	)

	// A second chat picks a different model. SwitchModel writes both: the chat
	// on screen, and the template a new chat is born with.
	a.startNewSession()
	a.cur().cfg.ModelName = "model-of-chat-two"
	a.cfg.ModelName = "model-of-chat-two"
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "คำถามแชทสอง", Time: "10:01"},
		SessionMessage{Role: "agent", Text: "คำตอบ", Time: "10:01"},
	)
	second := a.cur().id

	if _, err := a.LoadSession(first); err != nil {
		t.Fatalf("reopening the first chat: %v", err)
	}
	if got := a.cur().cfg.ModelName; got != "model-of-chat-one" {
		t.Errorf("the first chat came back on %q, want the model it was left on", got)
	}
	if got := a.cur().cfg.ModelProvider; got != "provider-one" {
		t.Errorf("the first chat came back on provider %q, want its own", got)
	}
	if _, err := a.LoadSession(second); err != nil {
		t.Fatalf("reopening the second chat: %v", err)
	}
	if got := a.cur().cfg.ModelName; got != "model-of-chat-two" {
		t.Errorf("the second chat came back on %q, want its own", got)
	}
}

// A chat from before the columns existed opens on the app's default, because
// that is what it has always opened on. An empty column is "never recorded",
// and a default is not a lie — a wrong specific value would be.
func TestAChatThatRecordedNoModelOpensOnTheDefault(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	id := a.cur().id
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "เก่า", Time: "09:00"},
		SessionMessage{Role: "agent", Text: "ครับ", Time: "09:00"},
	)
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	// A row as an older build left it.
	if _, err := db.Exec(`UPDATE sessions SET provider = '', model = '' WHERE id = ?`, id); err != nil {
		t.Fatalf("blank the columns: %v", err)
	}

	a.cfg.ModelName = "the-default"
	a.startNewSession()
	if _, err := a.LoadSession(id); err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if got := a.cur().cfg.ModelName; got != "the-default" {
		t.Errorf("a session with no recorded model opened on %q, want the app default", got)
	}
}

// Restoring a provider restores its credentials with it.
//
// The first cut of §157 set the provider name off the row and left the key and
// the base URL as the app's template had them — one provider's name over
// another's credentials, which the engine reports as "missing model API key"
// before falling back to the built-in one. The owner saw that message rather
// than his model (20 ส.ค.).
// PROBE: a chat recorded on another provider must come back with that
// provider's key, not the last one the app happened to hold.
func TestAReopenedChatGetsItsOwnProvidersKey(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	first := a.cur().id
	a.cur().cfg.ModelProvider = "openai"
	a.cur().cfg.ModelName = "gpt-4o"
	a.cur().cfg.ModelAPIKey = "key-for-openai"
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "q", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "a", Time: "10:00"},
	)

	a.startNewSession()
	a.cur().cfg.ModelProvider = "deepseek"
	a.cur().cfg.ModelAPIKey = "key-for-deepseek"
	a.cfg = a.cur().cfg

	if _, err := a.LoadSession(first); err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	t.Logf("PROBE: provider=%q key=%q", a.cur().cfg.ModelProvider, a.cur().cfg.ModelAPIKey)
	if a.cur().cfg.ModelAPIKey == "key-for-deepseek" {
		t.Error("the reopened chat kept the other provider's key")
	}
}

// A chat older than the columns keeps the app's default until it answers again.
//
// The first cut of this stamped the row the moment the chat was OPENED, which
// wrote configuration into the user's data because they looked at something.
// Browse twenty old chats while the app happens to be on one provider and all
// twenty are pinned to it, silently, by an accident of timing — and then every
// one of them insists on it. That is not a default becoming specific, it is a
// choice being made on the user's behalf and recorded as if they made it.
//
// What the row records is what actually ANSWERED. The dials go down with every
// turn the session stores, so a chat carries the setup that produced its last
// reply, and looking at a conversation changes nothing about it.
func TestAnOldChatRecordsItsDialsWhenItAnswers(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	id := a.cur().id
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "เก่า", Time: "09:00"},
		SessionMessage{Role: "agent", Text: "ครับ", Time: "09:00"},
	)
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	// A row as an older build left it.
	if _, err := db.Exec(`UPDATE sessions SET provider = '', model = '' WHERE id = ?`, id); err != nil {
		t.Fatalf("blank the columns: %v", err)
	}

	// Merely opening it records nothing.
	a.cfg.ModelName = "whatever-was-current"
	a.startNewSession()
	if _, err := a.LoadSession(id); err != nil {
		t.Fatalf("first open: %v", err)
	}
	var recorded string
	_ = db.QueryRow(`SELECT model FROM sessions WHERE id = ?`, id).Scan(&recorded)
	if recorded != "" {
		t.Errorf("opening the chat recorded %q, want nothing written by a look", recorded)
	}

	// Answering in it does.
	a.cur().cfg.ModelName = "the-one-that-answered"
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "ถามใหม่", Time: "11:00"},
		SessionMessage{Role: "agent", Text: "ตอบ", Time: "11:00"},
	)
	_ = db.QueryRow(`SELECT model FROM sessions WHERE id = ?`, id).Scan(&recorded)
	if recorded != "the-one-that-answered" {
		t.Errorf("the row records %q, want the model that produced the reply", recorded)
	}

	// And from then on it is its own, whatever anybody else picks.
	a.startNewSession()
	a.cur().cfg.ModelName = "someone-elses-choice"
	a.cfg.ModelName = "someone-elses-choice"
	if _, err := a.LoadSession(id); err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if got := a.cur().cfg.ModelName; got != "the-one-that-answered" {
		t.Errorf("the chat came back on %q, want the model it answered with", got)
	}
}

// A chat whose recorded model cannot be started says so, in its own words.
//
// The model a conversation remembers can stop existing under it: a provider slot
// repointed at another endpoint (the owner's deepseek → OpenRouter), a local
// runtime that is not running, a revoked key. What the window said before was
// the generic "เชื่อมต่อไม่ได้ ตอนนี้ใช้โมเดลในตัวของ Aetox แทน", which is true
// and useless — it never says the model being asked for is one THIS CHAT
// remembers rather than one the user just picked.
//
// Nothing is corrected automatically (owner's call, 20 ส.ค.): the chat keeps
// what it recorded, the banner names it, and choosing another is the user's
// click. Silence is what made the earlier bug in this file so hard to find.
func TestAChatSaysWhenItsOwnRecordedModelWillNotStart(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	conv := a.cur()

	// No error at all: nothing to explain, and no banner invented.
	a.explainARecordedModelThatWillNotStart(conv, "deepseek", "gone-model")
	if conv.modelErr != nil {
		t.Errorf("a healthy engine grew a warning: %v", conv.modelErr)
	}

	conv.modelErr = errors.New("model not found")
	a.explainARecordedModelThatWillNotStart(conv, "deepseek", "deepseek-v4-flash")
	got := conv.modelErr.Error()
	if !strings.Contains(got, "deepseek-v4-flash") {
		t.Errorf("the warning is %q, want it to name the model this chat recorded", got)
	}
	if !strings.Contains(got, "model not found") {
		t.Errorf("the warning is %q, want the engine's own reason kept", got)
	}

	// A session that recorded no model has nothing of its own to blame, so the
	// engine's plain reason stands.
	conv.modelErr = errors.New("model not found")
	a.explainARecordedModelThatWillNotStart(conv, "", "")
	if conv.modelErr.Error() != "model not found" {
		t.Errorf("an unrecorded chat's warning became %q, want the engine's own", conv.modelErr)
	}
}

// The last three things the app was still holding for everybody at once
// (§155's own leftover list), each proved separately.

// Undo goes back to THIS chat's last turn. One snapshot point for the app meant
// the second turn's capture overwrote the first's, so undo in one conversation
// restored the other one's work — the only one of the three that damaged data
// rather than confusing a screen.
func TestEachChatHasItsOwnUndoPoint(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	one := a.cur()
	one.lastSnapshot = "snapshot-of-chat-one"

	a.startNewSession()
	two := a.cur()
	if two.lastSnapshot != "" {
		t.Errorf("a new chat starts on %q, want no undo point of its own yet", two.lastSnapshot)
	}
	two.lastSnapshot = "snapshot-of-chat-two"

	if one.lastSnapshot != "snapshot-of-chat-one" {
		t.Errorf("the first chat's undo point moved to %q — the second turn overwrote it", one.lastSnapshot)
	}
}

// desk_list describes the desk of the chat that asked. The window has kept a
// workbench per session all along; what was one per app was the mirror on this
// side, so a tool call from a background chat read the tab strip of the chat on
// screen and described somebody else's desk as its own.
func TestDeskListDescribesItsOwnChatsDesk(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	onScreen := a.cur()
	other := &conversation{id: "20990101-000000.000", cfg: a.cfg}
	a.convs.show(other)
	a.convs.show(onScreen)

	a.WorkbenchTabsChanged(onScreen.id, []DeskTab{{Kind: "file", Name: "on-screen.go", Path: "on-screen.go", Mine: true}})
	a.WorkbenchTabsChanged(other.id, []DeskTab{{Kind: "file", Name: "background.go", Path: "background.go", Mine: true}})

	got, err := (&deskListSkill{app: a, conv: other}).list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(got.Content, "background.go") {
		t.Errorf("the background chat's desk reads %q, want its own tab", got.Content)
	}
	if strings.Contains(got.Content, "on-screen.go") {
		t.Errorf("the background chat's desk reads %q — that is the other chat's tab", got.Content)
	}
}

// A suggest_task chip belongs to the chat that noticed the work. Its prompt is
// written to stand alone from THAT conversation, so shown under another one it
// is a suggestion with no visible origin — and with two chats working, two
// agents were filling one tray.
func TestATaskChipStaysInTheChatThatRaisedIt(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	first := a.cur()
	first.taskChips.add("ลบ config ที่ตายแล้ว", "", "ลบให้หน่อย")

	a.startNewSession()
	if got := a.ListTaskChips(); len(got) != 0 {
		t.Errorf("a new chat opened holding %d chips, want none of the other chat's", len(got))
	}
	a.cur().taskChips.add("อีกงานหนึ่ง", "", "ทำให้ที")
	if got := a.ListTaskChips(); len(got) != 1 || got[0].Title != "อีกงานหนึ่ง" {
		t.Errorf("this chat's tray = %+v, want only what it raised itself", got)
	}
	if got := first.taskChips.list(); len(got) != 1 || got[0].Title != "ลบ config ที่ตายแล้ว" {
		t.Errorf("the first chat's tray = %+v, want what it raised kept", got)
	}
}
