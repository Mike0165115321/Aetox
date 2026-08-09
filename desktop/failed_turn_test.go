package main

import (
	"errors"
	"strings"
	"testing"
)

// A turn that failed used to be persisted as half a turn: openTurn had written
// the question, SendMessage returned before appendTurn, and the answer — with
// the fact that anything had gone wrong — existed only in the window. Reload,
// and what was left was a question sitting alone with nothing saying why.
func TestAFailedTurnIsWrittenDownWithItsReason(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn: %v", err)
	}
	a.openTurn(SessionMessage{Role: "user", Text: "เทสๆ", Time: "22:10"})
	id := a.appendFailedTurn(
		SessionMessage{Role: "agent", Text: "อ่านไฟล์ครบแล้ว", Time: "22:10", ThinkSecs: 3},
		errors.New("codex: the free plan's limit is used up"),
	)
	a.endTurn()
	if id == 0 {
		t.Fatal("appendFailedTurn wrote nothing")
	}

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("stored %d messages, want the question and the answer it never got", len(messages))
	}
	if messages[0].Text != "เทสๆ" {
		t.Errorf("question = %q; want the user's own words", messages[0].Text)
	}
	// Whatever streamed before the wall is part of the record — often nothing,
	// sometimes half an answer the user watched arrive.
	if messages[1].Text != "อ่านไฟล์ครบแล้ว" {
		t.Errorf("partial answer = %q; want what had streamed", messages[1].Text)
	}
	if !strings.Contains(messages[1].ErrorText, "limit is used up") {
		t.Errorf("errorText = %q; want the reason the turn stopped", messages[1].ErrorText)
	}
	// The flag openTurn raised has to come down here too, or the NEXT turn's
	// appendTurn reads it as "the question is already stored" and skips writing.
	if a.turnOpened {
		t.Error("turnOpened survived a failed turn — the next question could be dropped silently")
	}
}

// Non-empty error_text IS "this turn failed". A turn that worked must never come
// back looking like one that did not.
func TestASuccessfulTurnCarriesNoError(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "ถาม", Time: "09:00"},
		SessionMessage{Role: "agent", Text: "ตอบ", Time: "09:00"},
	)
	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	for _, m := range messages {
		if m.ErrorText != "" {
			t.Errorf("%s message came back marked failed: %q", m.Role, m.ErrorText)
		}
	}
}

// An error that stringifies to nothing must still read as a failure. An empty
// column means the turn succeeded, and a failure recorded as a success is worse
// than one recorded with a vague reason.
func TestAFailureIsNeverStoredAsBlank(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn: %v", err)
	}
	a.openTurn(SessionMessage{Role: "user", Text: "q", Time: "09:00"})
	a.appendFailedTurn(SessionMessage{Role: "agent", Time: "09:00"}, errors.New("   "))
	a.endTurn()

	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if messages[1].ErrorText == "" {
		t.Error("a failed turn was stored as a successful one")
	}
}

// The model was never told the answer it gave, because it never gave one.
// Replaying the question with an empty reply under it would teach it that it had
// ignored the user; replaying the question alone would leave it dangling in a
// memory the very next message is about to ask again.
func TestAFailedTurnIsLeftOutOfTheModelsMemory(t *testing.T) {
	transcript := []SessionMessage{
		{Role: "user", Text: "คำถามแรก"},
		{Role: "agent", Text: "คำตอบแรก"},
		{Role: "user", Text: "เทสๆ"},
		{Role: "agent", Text: "", ErrorText: "limit is used up"},
	}
	got := transcriptToModelMessages(transcript)
	if len(got) != 2 {
		t.Fatalf("rebuilt %d messages, want only the turn that completed", len(got))
	}
	for _, m := range got {
		if strings.Contains(m.Content, "เทสๆ") {
			t.Error("the failed question is back in the model's memory — the retry would ask it twice")
		}
	}
}

// End to end through the real door, because the two halves of a failed turn are
// easy to fix one at a time: the row can be written correctly while the
// in-memory copy beside it forgets it failed, and then the retry — which reads
// the transcript, not the table — quietly stops dropping the attempt it is
// supposed to replace.
//
// No model is configured in a test app, so SendMessage fails inside runTurn.
// That is a genuine failure taking the genuine path, which is the point.
func TestSendMessageRecordsItsOwnFailureOnBothSides(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if _, err := a.SendMessage("เทสๆ"); err == nil {
		t.Fatal("SendMessage succeeded with no model configured; this test needs the failing path")
	}

	if n := len(a.transcript); n != 2 {
		t.Fatalf("transcript holds %d messages, want the question and the answer it never got", n)
	}
	if got := a.transcript[1].ErrorText; got == "" {
		t.Error("the in-memory answer does not know it failed — the retry would not drop it")
	}
	stored, err := a.SessionTranscript(a.sessionID)
	if err != nil {
		t.Fatalf("SessionTranscript: %v", err)
	}
	if len(stored) != 2 || stored[1].ErrorText == "" {
		t.Fatalf("stored %d rows, error_text = %q; want the failure on disk too",
			len(stored), lastErrorText(stored))
	}

	// And the pair is droppable, which is what pressing ลองใหม่ relies on.
	a.dropFailedTail()
	if len(a.transcript) != 0 {
		t.Errorf("the failed pair survived the retry's cleanup: %d messages left", len(a.transcript))
	}
}

func lastErrorText(messages []SessionMessage) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].ErrorText
}

// Retrying replaces an attempt; it does not file a second one beside it. The
// screen has always spliced the red bubble out, and now that the attempt is
// written down the store has to agree — or a reload after a successful retry
// shows the failure the user already dealt with, above the answer that dealt
// with it.
func TestRetryDropsTheFailedAttemptFromTheStore(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn: %v", err)
	}
	a.openTurn(SessionMessage{Role: "user", Text: "เทสๆ", Time: "22:10"})
	agentMsg := SessionMessage{Role: "agent", Text: "", Time: "22:10"}
	a.appendFailedTurn(agentMsg, errors.New("limit is used up"))
	a.transcript = append(a.transcript,
		SessionMessage{Role: "user", Text: "เทสๆ", Time: "22:10"},
		SessionMessage{Role: "agent", Text: "", Time: "22:10", ErrorText: "limit is used up"},
	)
	a.endTurn()

	a.dropFailedTail()

	if len(a.transcript) != 0 {
		t.Errorf("transcript still holds %d messages after the retry cleared it", len(a.transcript))
	}
	// SessionTranscript, not LoadSession: an empty conversation is the expected
	// state here, and LoadSession reports "no such session in this project" for
	// one — a door refusing to open an empty chat, which is not what is under
	// test.
	messages, err := a.SessionTranscript(a.sessionID)
	if err != nil {
		t.Fatalf("SessionTranscript: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("store still holds %d rows of the attempt being replaced", len(messages))
	}
}

// The guard matters more than the delete: this removes rows, and "the last two
// rows" is only the failed pair when the conversation actually ends with one.
func TestDropFailedTailLeavesACompletedTurnAlone(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.appendTurn(
		SessionMessage{Role: "user", Text: "ถาม", Time: "09:00"},
		SessionMessage{Role: "agent", Text: "ตอบ", Time: "09:00"},
	)
	a.transcript = []SessionMessage{
		{Role: "user", Text: "ถาม"},
		{Role: "agent", Text: "ตอบ"},
	}

	a.dropFailedTail()

	if len(a.transcript) != 2 {
		t.Fatalf("a completed turn was dropped: %d messages left", len(a.transcript))
	}
	messages, err := a.LoadSession(a.sessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("a completed turn's rows were deleted: %d left", len(messages))
	}
}
