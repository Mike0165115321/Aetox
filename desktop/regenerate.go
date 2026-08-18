package main

// Asking the same question again.
//
// Four things the user can do to a turn that already happened — retry one that
// failed, ask for another answer, edit what they said, and flip between the
// answers they have collected. They look like four features and are really one:
// put the conversation back the way it was before that turn, then run it again.
//
// "Put it back" is the whole difficulty, and it is why this is not a frontend
// concern. A turn lives in three places at once — the UI's bubble list, the
// `messages` table, and the model's own memory (internal/memory.Context) — and
// they do not fail together. A turn that errors out reaches the model's memory
// and neither of the other two: cognitive.Agent adds the user message to the
// context BEFORE it calls the provider, so the DNS failure that started this
// work left the question sitting in the model's head while the transcript and
// the store had never heard of it. Retrying by simply sending the text again
// would have asked it twice.
//
// So every entry point here rebuilds the model's memory from the transcript,
// which is the only record that is written after the fact and therefore the only
// one that is always true. Nothing tries to surgically remove messages from a
// live context.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/turn"
)

// RegenerateResult is what the UI needs after a re-run: the new answer, every
// answer this bubble now has, and which of them is live.
type RegenerateResult struct {
	Text string `json:"text"`
	// Parts is the re-run turn as a sequence, same as TurnReply.Parts.
	Parts    []turn.TurnPart  `json:"parts,omitempty"`
	Variants []SessionVariant `json:"variants"`
	Active   int              `json:"active"`
	// Reverted names the files put back before the re-run, when the caller asked
	// for that. The UI says so in the transcript — an answer that quietly undid
	// six files would be a worse surprise than the doubled edit it prevents.
	Reverted []string `json:"reverted,omitempty"`
}

// RetryFailedTurn re-runs a turn that never finished.
//
// The text still comes from the caller rather than from the stored question.
// The bubble on screen holds exactly what was sent — attachment marker lines and
// all — and re-deriving it from the row would be a reconstruction of something
// the caller already has verbatim.
//
// Rebuilding the context first is what removes the question the failed attempt
// left in the model's memory; transcriptToModelMessages drops the failed pair,
// so what comes back is the conversation up to the question, and SendMessage
// asks it once.
func (a *App) RetryFailedTurn(text string) (TurnReply, error) {
	if strings.TrimSpace(text) == "" {
		return TurnReply{}, fmt.Errorf("ไม่มีข้อความให้ลองใหม่")
	}
	// SendMessage would refuse on its own, but only after restoreContext had
	// already rewritten the memory the running turn is thinking with.
	if a.turnBusy() {
		return TurnReply{}, errTurnBusy
	}
	a.dropFailedTail()
	a.restoreContext(a.cur(), a.cur().transcript)
	return a.SendMessage(text)
}

// dropFailedTail removes the turn that failed at the end of this session — the
// row appendFailedTurn wrote and the question above it — from the store and the
// in-memory transcript alike.
//
// Retrying replaces an attempt; it does not file a second one beside it. The
// screen has always worked this way (the red bubble is spliced out the instant
// ลองใหม่ is pressed) and, now that the attempt is written down, the store has to
// agree — otherwise a reload after a successful retry would show the failure the
// user had already dealt with, above the answer that dealt with it.
//
// Guarded on the transcript's own tail rather than trusting the table: this
// deletes rows, and "the last two rows of this session" is only the failed pair
// if the failed pair is what the conversation actually ends with.
func (a *App) dropFailedTail() {
	n := len(a.cur().transcript)
	if n < 2 {
		return
	}
	last := a.cur().transcript[n-1]
	if last.Role != "agent" || last.ErrorText == "" {
		return
	}
	a.cur().transcript = a.cur().transcript[:n-2]
	a.dropLastTurnRows()
}

// RegenerateReply answers the last question again, keeping the previous answer
// alongside the new one so the user can compare and pick.
//
// revertFiles undoes what the previous attempt wrote before re-running. It is
// the caller's decision because it is destructive and because only the caller
// can ask: the UI checks PendingUndo and confirms. Answering again WITHOUT it is
// the genuinely dangerous option — the turn's tools run a second time on top of
// their own output — so the UI defaults to reverting when there is anything to
// revert.
func (a *App) RegenerateReply(revertFiles bool) (RegenerateResult, error) {
	// Its own turn, marked like one: RegenerateReply reaches runTurn without
	// going through SendMessage, and an unmarked turn is invisible to every
	// guard and to the reloaded window asking TurnInFlight.
	conv := a.cur()
	sessionID := conv.id
	if err := a.beginTurn(sessionID); err != nil {
		return RegenerateResult{}, err
	}
	defer a.endTurn(sessionID)
	question, ok := a.lastQuestion()
	if !ok {
		return RegenerateResult{}, fmt.Errorf("ยังไม่มีคำถามในเซสชันนี้ให้ตอบใหม่")
	}

	var reverted []string
	if revertFiles {
		// Best effort: a project that is not a git repository has no snapshot to
		// restore, and that must not stop the user getting another answer.
		if result, err := a.UndoLastTurn(); err == nil {
			reverted = result.Files
		}
	}

	// Asking again is the one negative signal that costs the user nothing to
	// give: they were going to press this button anyway. Recorded before the
	// second attempt runs, so it lands even if that attempt fails.
	replyID := a.lastAgentMessageID(conv)
	a.markTurnRedone(replyID)

	// The model must not see its own previous answer, or "ตอบใหม่" returns a
	// polite rewrite of it rather than another attempt.
	a.restoreContext(conv, conv.transcript[:len(conv.transcript)-2])
	mark := a.maxToolRunID(conv)
	started := time.Now()
	_, agentMsg, err := a.runTurn(conv, question)
	if err != nil {
		// The transcript was never touched, but the model's memory now holds a
		// conversation one turn shorter than the one on screen. Put it back, or
		// the next ordinary message is answered against a context missing the
		// exchange the user can still see.
		a.restoreContext(a.cur(), a.cur().transcript)
		return RegenerateResult{Text: agentMsg.Text, Reverted: reverted}, err
	}

	live := a.cur().transcript[len(a.cur().transcript)-1]
	variants := append(variantsOf(live), SessionVariant{
		Text: agentMsg.Text, Reasoning: agentMsg.Reasoning, ThinkSecs: agentMsg.ThinkSecs,
		Parts: agentMsg.Parts,
	})
	active := len(variants) - 1

	live.Text, live.Reasoning, live.ThinkSecs = agentMsg.Text, agentMsg.Reasoning, agentMsg.ThinkSecs
	live.Parts = agentMsg.Parts
	live.Variants, live.Active = variants, active
	conv.transcript[len(conv.transcript)-1] = live
	a.storeVariants(conv, variants, active)
	a.storeParts(conv, agentMsg.Parts)
	// A second attempt is a second job against the same bubble. Both rows stay:
	// "this shape of work needed two tries" is exactly the kind of thing a later
	// pass should be able to see, and it cannot if the retry overwrites the
	// attempt that provoked it.
	a.recordJobs(conv, replyID, question, agentMsg.Text, mark, time.Since(started))

	return RegenerateResult{
		Text: agentMsg.Text, Parts: agentMsg.Parts,
		Variants: variants, Active: active, Reverted: reverted,
	}, nil
}

// ResendEdited replaces the last question with a corrected one and answers it.
//
// The old exchange is deleted rather than kept as a variant: variants are two
// answers to the SAME question, and after an edit the old answer is a reply to
// something the user has said they did not mean. Its file changes are reverted
// on the same terms as RegenerateReply.
func (a *App) ResendEdited(text string, revertFiles bool) (TurnReply, error) {
	if strings.TrimSpace(text) == "" {
		return TurnReply{}, fmt.Errorf("ข้อความว่าง")
	}
	if _, ok := a.lastQuestion(); !ok {
		return TurnReply{}, fmt.Errorf("ยังไม่มีข้อความในเซสชันนี้ให้แก้")
	}
	// Same reason as RetryFailedTurn: everything below this line mutates the
	// transcript and the store before SendMessage's own gate would fire.
	if a.turnBusy() {
		return TurnReply{}, errTurnBusy
	}
	if revertFiles {
		_, _ = a.UndoLastTurn()
	}
	a.cur().transcript = a.cur().transcript[:len(a.cur().transcript)-2]
	a.dropLastTurnRows()
	a.restoreContext(a.cur(), a.cur().transcript)
	return a.SendMessage(text)
}

// SwitchVariant makes one of the stored answers the live one.
//
// It rewrites history on purpose: the chosen answer becomes `text` in the store
// and in the model's memory, so the conversation continues from the answer the
// user actually kept rather than from whichever one happened to be generated
// last. Picking an answer and then being replied to as if you had picked the
// other is the bug this exists to avoid.
func (a *App) SwitchVariant(index int) (RegenerateResult, error) {
	// restoreContext below rewrites the one agent context; mid-turn that is the
	// running turn's memory.
	if a.turnBusy() {
		return RegenerateResult{}, errTurnBusy
	}
	if len(a.cur().transcript) == 0 {
		return RegenerateResult{}, fmt.Errorf("ยังไม่มีคำตอบให้สลับ")
	}
	live := a.cur().transcript[len(a.cur().transcript)-1]
	if live.Role != "agent" || len(live.Variants) < 2 {
		return RegenerateResult{}, fmt.Errorf("คำตอบนี้มีแค่เวอร์ชันเดียว")
	}
	if index < 0 || index >= len(live.Variants) {
		return RegenerateResult{}, fmt.Errorf("ไม่มีคำตอบที่ %d", index+1)
	}

	chosen := live.Variants[index]
	live.Text, live.Reasoning, live.ThinkSecs = chosen.Text, chosen.Reasoning, chosen.ThinkSecs
	// The work moves with the answer. Without this the bubble showed the chosen
	// reply above the other attempt's tool calls.
	live.Parts = chosen.Parts
	live.Active = index
	a.cur().transcript[len(a.cur().transcript)-1] = live
	a.storeVariants(a.cur(), live.Variants, index)
	a.storeParts(a.cur(), chosen.Parts)
	a.restoreContext(a.cur(), a.cur().transcript)

	return RegenerateResult{Text: chosen.Text, Parts: chosen.Parts, Variants: live.Variants, Active: index}, nil
}

// lastQuestion returns the text of the last completed exchange's user message.
//
// The transcript is strictly user/agent pairs — appendTurn writes both or
// neither — so "the last turn" is the last two entries, and anything else means
// there is nothing to re-run.
func (a *App) lastQuestion() (string, bool) {
	if len(a.cur().transcript) < 2 {
		return "", false
	}
	user := a.cur().transcript[len(a.cur().transcript)-2]
	agent := a.cur().transcript[len(a.cur().transcript)-1]
	if user.Role != "user" || agent.Role != "agent" {
		return "", false
	}
	return user.Text, true
}

// restoreContext rebuilds the model's memory to match exactly these messages.
//
// Reusing LoadSession's mechanism rather than reaching into memory.Context:
// tool calls and mid-turn nudges are not in the transcript, so a rebuild is
// lossier than a surgical rewind would be — but it is the same loss a session
// reload has always taken, and it cannot leave the context in a state no
// transcript describes.
func (a *App) restoreContext(conv *conversation, messages []SessionMessage) {
	if a.cur().agent == nil {
		return
	}
	a.cur().agent.ClearContext()
	a.cur().agent.RestoreHistory(transcriptToModelMessages(messages))
}

// variantsOf reads a message's answer list, treating a bubble that has only
// ever been answered once as the one-element list it logically is.
func variantsOf(m SessionMessage) []SessionVariant {
	if len(m.Variants) > 0 {
		return append([]SessionVariant(nil), m.Variants...)
	}
	return []SessionVariant{{Text: m.Text, Reasoning: m.Reasoning, ThinkSecs: m.ThinkSecs, Parts: m.Parts}}
}

// storeVariants writes the answer list back onto the session's last agent row.
//
// The live variant is copied into text/reasoning/think_secs, which is what
// keeps every other reader — the FTS index, the session title, LoadSession's
// context rebuild — working without knowing variants exist.
//
// Addressed as "the newest agent row of this session" rather than by an id held
// in memory: the id would have to survive session switches and reloads to be
// worth anything, and only the last turn is ever re-answered.
func (a *App) storeVariants(conv *conversation, variants []SessionVariant, active int) {
	// The turn's stamped session, same as appendTurn — a re-answer is a turn
	// too, and its rows have the same one home.
	sessionID := conv.id
	db, err := a.database()
	if err != nil || sessionID == "" || active < 0 || active >= len(variants) {
		return
	}
	live := variants[active]
	_, _ = db.Exec(`
		UPDATE messages
		SET text = ?, reasoning = ?, think_secs = ?, variants = ?, variant_active = ?
		WHERE id = (SELECT MAX(id) FROM messages WHERE session_id = ? AND role = 'agent')`,
		live.Text, live.Reasoning, live.ThinkSecs, encodeVariants(variants), active, sessionID)
	// A re-answered session is a session that was just worked in, and the
	// sidebar orders by this. Without it, answering again would leave the
	// conversation sitting wherever it was in the history list.
	_, _ = db.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), sessionID)
}

// storeParts writes the sequence back onto the session's newest agent row,
// addressed the same way storeVariants addresses it. A re-answered turn produced
// different work, so the record of that work has to move with it.
func (a *App) storeParts(conv *conversation, parts []turn.TurnPart) {
	sessionID := conv.id
	db, err := a.database()
	if err != nil || sessionID == "" {
		return
	}
	_, _ = db.Exec(`
		UPDATE messages SET parts = ?
		WHERE id = (SELECT MAX(id) FROM messages WHERE session_id = ? AND role = 'agent')`,
		encodeParts(parts), sessionID)
}

// dropLastTurnRows deletes the newest user/agent pair of the current session.
// The messages_ad trigger takes them out of the FTS index with them.
func (a *App) dropLastTurnRows() {
	db, err := a.database()
	if err != nil || a.cur().id == "" {
		return
	}
	// The job rows for those messages go with them, and are NOT marked bad on the
	// way out. An edited question means the user asked for the wrong thing, not
	// that the agent did it badly — recording it as a failure would teach the
	// learning layer to avoid whatever the agent happened to do correctly.
	_, _ = db.Exec(`
		DELETE FROM jobs
		WHERE message_id IN (SELECT id FROM messages WHERE session_id = ? ORDER BY id DESC LIMIT 2)`,
		a.cur().id)
	_, _ = db.Exec(`
		DELETE FROM messages
		WHERE id IN (SELECT id FROM messages WHERE session_id = ? ORDER BY id DESC LIMIT 2)`,
		a.cur().id)
}

// encodeVariants renders the answer list for storage. A bubble answered once
// stores nothing: that is every message in almost every session, and an empty
// column says "no alternates" more cheaply than a one-element array.
func encodeVariants(variants []SessionVariant) string {
	if len(variants) < 2 {
		return ""
	}
	encoded, err := json.Marshal(variants)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// decodeVariants is the inverse, and is deliberately forgiving: a row written
// by a newer build, or one somehow corrupted, costs the switcher on that single
// bubble rather than the ability to open the session at all.
func decodeVariants(stored string) []SessionVariant {
	if strings.TrimSpace(stored) == "" {
		return nil
	}
	var variants []SessionVariant
	if json.Unmarshal([]byte(stored), &variants) != nil || len(variants) < 2 {
		return nil
	}
	return variants
}

// encodeParts stores the turn's sequence. Empty for a user message and for any
// turn that produced no parts — an empty column reads as "this predates the
// sequence", which is exactly right for every row already on disk.
func encodeParts(parts []turn.TurnPart) string {
	if len(parts) == 0 {
		return ""
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// decodeParts is the inverse, forgiving on the same terms: a row this build
// cannot read falls back to the plain text bubble rather than refusing to open
// the session.
func decodeParts(stored string) []turn.TurnPart {
	if strings.TrimSpace(stored) == "" {
		return nil
	}
	var parts []turn.TurnPart
	if json.Unmarshal([]byte(stored), &parts) != nil {
		return nil
	}
	return parts
}
