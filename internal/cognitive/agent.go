package cognitive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/memory"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/think"
	"github.com/Mikedev115/Aetox/internal/turn"
)

const (
	// toolLoopOutputTokenMax mirrors OpenCode's OUTPUT_TOKEN_MAX: the global
	// ceiling for tool-loop output, clamped down per provider in
	// toolLoopMaxTokens where an API rejects values this large.
	toolLoopOutputTokenMax = 32000

	// deepseekV4OutputTokenMax lets DeepSeek V4 write a whole file in one call
	// instead of truncating and forcing a skeleton-then-edit split (each split
	// resends the full context = wasted tokens). V4 accepts up to 384K output;
	// 64K comfortably fits any single hand-written file. max_tokens is only a
	// ceiling — you pay for tokens generated, not the cap — so raising it costs
	// nothing until the model actually produces a large file.
	deepseekV4OutputTokenMax = 65536

	// minToolLoopMaxTokens is the floor clampToWindow will not go below. Room
	// this tight means the turn is about to be compacted anyway; asking for a
	// handful of tokens still lets the model say why it stopped, where asking
	// for zero is a 400 of its own.
	minToolLoopMaxTokens = 512

	// Doom-loop guard thresholds, same as OpenCode's (warn at 3 identical
	// consecutive calls, hard-stop at 5).
	doomLoopWarn = 3
	doomLoopStop = 5

	// Compaction (OpenCode/Claude Code-style): when usage crosses the
	// threshold fraction of the char budget, older turns are summarized by
	// the model into one message instead of being trimmed away whole.
	compactThresholdFraction = 0.8
	compactKeepRecent        = 6
	compactSummaryMaxTokens  = 2048

	// The micro sweep (Claude Code-style micro-compact) runs earlier, at 60%:
	// old re-obtainable tool outputs are cleared in one batch, which often
	// keeps the session from reaching 80% at all — a sweep costs nothing where
	// a summary costs a model call and loses detail. One batch at a threshold
	// rather than every turn, because each sweep breaks the provider's prompt
	// cache from the first cleared message onward: pay that once per crossing,
	// not per round (docs/aider-study/EXECUTION.md ระดับ 3).
	microCompactThresholdFraction = 0.6

	// How many times one turn may answer a provider's "too long" by summarizing
	// and trying again.
	//
	// Two, not one, because the first summary can leave the recent turns it is
	// required to keep still over the line; and not more than two, because each
	// attempt costs a model call and the third would be evidence that the
	// problem is a single oversized message rather than an accumulated history.
	// Summarizing cannot fix that one, and spinning on it turns a clear failure
	// into a slow one.
	maxOverflowCompactions = 2

	// maxDroppedConnectionRetries caps how many times one round may be asked
	// again after the connection died under it.
	//
	// Two, matching the transport's own budget (model.retryTransport), because
	// this is the same retry: the transport covers a socket that dies before
	// the headers, this covers one that dies after them, and a user who has
	// lost three connections in a row has a network problem that a fourth
	// attempt will not out-wait.
	maxDroppedConnectionRetries = 2

	// maxEmptyCompletionReplays caps how many times one round may be sent again
	// after the provider answered with nothing at all (model.ErrEmptyCompletion).
	//
	// Two, like the dropped connection above, and for a reason that reads the
	// same way from the other end: an empty answer that survives three
	// identical asks is not a hiccup at the gateway, it is this conversation —
	// and a fourth ask would spend the same input tokens to be told the same
	// nothing. What comes after the two is a DIFFERENT request (the nudge),
	// because repeating a question that has been answered the same way three
	// times is not a strategy.
	maxEmptyCompletionReplays = 2
)

// Small local models (Ollama-scale) sometimes reply with nothing at all,
// typically right after a large tool result. One nudge usually revives them —
// and lets the model phrase the answer (or the "beyond my limits" admission)
// in the user's own language. Tools-first wording: a model that lacks a
// capability natively (e.g. reading images) must reach for a tool that covers
// it, not refuse. The bilingual line below is the floor for when even the
// nudge comes back empty.
const emptyReplyNudge = "[system] Your previous reply was empty. Respond now, in the same language the user writes in. If tools are available that can do what you cannot do natively (reading images, files, searching, etc.), call one now instead of refusing. Only if no tool can help and the task is truly impossible for you, tell the user briefly that it exceeds the current model's capability."

const emptyReplyFallback = "เกินขีดจำกัดของโมเดลปัจจุบัน — โมเดลตอบกลับว่างเปล่า ลองแบ่งงานให้เล็กลงหรือเปลี่ยนโมเดล (Beyond the current model's limits — it returned an empty reply. Try a smaller task or a stronger model.)"

// interjectionNote marks a message that arrived while the turn was already
// running. Without it the message is indistinguishable from one the user typed
// before anything started — and the two want opposite behaviour: read as a fresh
// instruction, "ใส่สีน้ำเงินด้วยนะ" becomes a reason to abandon a half-written file.
//
// What it does NOT do is decide for the model. The owner's own description of
// what makes this good is that the model judges — *"โคตรเจ๋ง โมเดลฉลาดเลือกด้วยนะว่า
// ที่บอกกลางคันนั้นคืออะไร จะทำตอนนี้เลย เช่นบอกปรับสี หรือไว้ทำตอนเสร็จงานใหญ่ที่ทำอยู่ก่อน"*. So
// the note supplies the one fact the model cannot infer (this arrived mid-work)
// and names the choices, rather than picking one. Same principle as §17: the
// model is told what is true and left to decide, never pre-judged.
const interjectionNote = "[system] The user sent this WHILE you were working on the task above — they did not wait for you to finish. Judge which kind it is and act accordingly: small enough to fold into what you are doing right now (a colour, a name, a correction) — do it now; a change to the job in hand — adjust course and carry on; something separate or larger — finish what you are already doing first, then do it, and say that is what you are doing. Only drop the current work if the message plainly tells you to stop. Either way, acknowledge it in your final answer.\n\n"

// maxDSMLNudges caps corrective retries when the model leaks tool-call markup
// as text (see model.ContainsLeakedDSML). Each retry is one extra round-trip,
// so keep it small; past the cap we stop rather than surface raw markup.
const maxDSMLNudges = 2

// maxAnswerContinuations caps how many times one answer may be picked up again
// after the model ran out of output tokens mid-sentence.
//
// Three, which is a different judgement from the two above: those cap a retry
// of something that FAILED, this caps the continuation of something that is
// working. A long answer being written in four pieces is the feature doing its
// job, not a loop, and the model has to stop on its own eventually because each
// piece starts nearer the end. The cap is here only so a model that answers
// every continuation with a fresh essay cannot run forever.
const maxAnswerContinuations = 3

// The instruction that joins the pieces. Every clause in it is load-bearing:
// left to itself a model asked to continue will re-introduce the topic, or
// apologise, or start the list again from item one — and the two halves are
// glued with no separator, so anything it adds lands in the middle of a word.
const truncatedAnswerNudge = "[system] Your reply was cut off mid-answer because it reached the output token limit. It was NOT finished, and the user can see what you had written so far. Carry straight on from the last character you wrote, in the same language and the same formatting. Do not repeat any of it, do not start over, do not summarise what you already said, do not apologise and do not announce that you are continuing: the two pieces are joined into one message with nothing between them. If the answer is long, prefer finishing the point you were making over starting new ones."

const dsmlLeakNudge = "[system] Your previous reply wrote a tool call as plain-text markup, so nothing ran and no file was created. Do NOT write tool calls as text or invent your own tool-call format. Call the tool through the normal tool interface — e.g. the write tool with a `path` argument (not `file_path`) and a `content` argument. You may issue several tool calls at once. Do it now."

// The two ways a tool loop can end without the model choosing to stop. Both are
// returned as ordinary replies (no error) because the user has to see them — but
// a *caller* has to be able to recognise them too, and matching the prose would
// break the first time it is reworded or translated (the §27 lesson). A sub-agent
// turns both into something its parent can act on (internal/subagent).
const (
	// ToolLoopExhausted is the reply when MaxToolCalls is reached.
	ToolLoopExhausted = "agent tool loop reached maximum iterations"
	// DoomLoopStopPrefix begins the reply when the doom-loop guard aborts an
	// identical call repeated with no progress.
	DoomLoopStopPrefix = "หยุดการทำงาน:"
)

const dsmlLeakFallback = "โมเดลพยายามเรียกเครื่องมือแต่ส่งออกมาเป็นข้อความแทนคำสั่งจริง จึงไม่มีอะไรทำงานและไม่มีไฟล์ถูกสร้าง — ลองสั่งใหม่หรือเปลี่ยนโมเดลครับ (The model wrote a tool call as text instead of a real call, so nothing ran. Try again or switch models.)"

const compactionPrompt = "You are compacting a long conversation so it can continue in less context. " +
	"Write a faithful, information-dense summary of the conversation you are given, covering: " +
	"the user's goals and every decision made; important facts and constraints; " +
	"files or paths created/modified and how; key tool results; unresolved tasks and agreed next steps; " +
	"the user's language and preferences. Output only the summary text, in the user's language."

// toolLoopMaxTokens returns the max_tokens sent on every tool-loop request —
// always explicit (Anthropic requires the field; the rest get a value their
// API accepts instead of a provider default that may be as low as 4096).
//
// The provider floor is a ceiling, not the answer. max_tokens is checked by
// the API against what is LEFT in the window, not against the window, so a
// floor alone is a 400 waiting for a small model: ThaiLLM's THaLLE-8B serves
// 16,384 tokens, and at 9,791 tokens of input it rejected the whole request
// for asking 8,192 back when only 6,593 remained (measured 2026-08-20). Every
// provider has this cliff; ThaiLLM is simply the first row whose window is
// small enough to reach it in ordinary use.
//
// So the floor is clamped by whatever the window can still hold. Recompute it
// each round rather than once per turn: a tool loop grows its own input, and
// the value that fitted on the first call is the one that 400s on the ninth.
func (a *Agent) toolLoopMaxTokens() int {
	return a.clampToWindow(a.providerOutputCeiling())
}

// clampToWindow cuts an output ceiling down to the room actually left in the
// model's context window, and is a no-op for the case that used to be the only
// one: a big window with a short conversation in it.
//
// Input size is measured, not guessed, whenever the provider has already told
// us — lastUsage.PromptTokens is the real count from this same conversation one
// round ago, and it is the only number here that a tokenizer cannot embarrass.
// Before the first reply there is nothing to measure, and the char estimate
// that stands in is deliberately pessimistic: Thai runs closer to one token
// per two characters than the one-per-four an English-shaped guess assumes,
// and this row exists to serve Thai. Guessing high costs a shorter reply;
// guessing low costs the whole request.
func (a *Agent) clampToWindow(ceiling int) int {
	name := ""
	if a.provider != nil {
		name = model.NormalizeProvider(a.provider.Name())
	}
	window := model.ContextWindowTokens(name, a.model)
	if window <= 0 {
		return ceiling // no promise about this model's window; nothing to clamp against
	}

	input := a.lastUsage.PromptTokens
	if input <= 0 {
		_, usedChars, _ := a.context.UsageStats()
		input = usedChars / 2
	}
	// Headroom for the turn being added on top of what was measured, and for
	// the tokenizer disagreeing with the estimate. A reply that is shorter than
	// it could have been is invisible; a 400 ends the turn.
	room := window - input - window/16

	switch {
	case room >= ceiling:
		return ceiling
	case room > minToolLoopMaxTokens:
		return room
	default:
		// Already at or past the window. Ask for the smallest useful reply and
		// let the compaction check at the top of the loop reclaim the room.
		return minToolLoopMaxTokens
	}
}

// providerOutputCeiling is the per-provider floor this used to return on its
// own: the largest max_tokens each API accepts, before the window is consulted.
func (a *Agent) providerOutputCeiling() int {
	name := ""
	if a.provider != nil {
		name = model.NormalizeProvider(a.provider.Name())
	}
	switch name {
	case "deepseek":
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.model)), "deepseek-v4") {
			return deepseekV4OutputTokenMax // V4 series allows up to 384K output
		}
		return 8192 // V3-era API max; larger values are rejected with 400
	case "openai":
		return 16384 // gpt-4o floor; newer models allow more
	case "anthropic", "gemini", "zai":
		return toolLoopOutputTokenMax
	default:
		return 8192 // openrouter/groq/unknown route mixed models — stay safe
	}
}

type Agent struct {
	provider  model.Provider
	model     string
	context   *memory.Context
	lastUsage model.Usage
	onUsage   func(model.Usage) // observer for every API response's usage; nil = off
	// onToolCallStart is told a tool call is coming while its arguments are
	// still streaming — the UI's only signal during the long silence of a
	// model writing a large file. nil = off.
	onToolCallProgress func(id, name, subject string, lines int)
	// onToolCallRefused closes a row onToolCallProgress opened. A call the tool
	// loop refuses never reaches the executor, so the "result" event that would
	// end its live row is never sent and the row spins for the rest of the
	// session — the owner watched a write sit at "+237" forever while the retry
	// completed underneath it (23 ส.ค. 2026). Drawing a call as it is written
	// means owning the case where it is never run.
	onToolCallRefused func(id, name, subject string)
	maxToolCalls      int

	// interjections are messages the user typed while a turn was already
	// running. Guarded because they arrive from the UI's goroutine while the
	// tool loop is inside a provider call on another.
	interjectMu  sync.Mutex
	interjection []string
}

// Interject hands the running turn a message the user typed while it was still
// working. It is picked up at the start of the next tool-loop round, or — if the
// model was already writing its final answer — it keeps the turn going instead
// of letting it end (see Chat).
//
// Queueing it until the turn finished was the old behaviour and the owner's
// complaint: *"เวลาพิมพ์อะไรลงไป มันต้องส่งต่อได้ทันทีดิ ไม่ใช่ต้องรอให้มันทำงานเสร็จก่อน"*.
// Nothing here checks whether a turn is actually in flight — the buffer is drained
// by whoever runs next, and the host takes back what was left over
// (DrainInterjections) so a message can never be silently swallowed.
func (a *Agent) Interject(text string) {
	if a == nil {
		return
	}
	if text = strings.TrimSpace(text); text == "" {
		return
	}
	a.interjectMu.Lock()
	a.interjection = append(a.interjection, text)
	a.interjectMu.Unlock()
}

// DrainInterjections empties the buffer and returns what was in it. The tool loop
// calls it every round; the host calls it once more after a turn returns, to catch
// anything that arrived in the moment between the last round and the reply.
func (a *Agent) DrainInterjections() []string {
	if a == nil {
		return nil
	}
	a.interjectMu.Lock()
	defer a.interjectMu.Unlock()
	if len(a.interjection) == 0 {
		return nil
	}
	taken := a.interjection
	a.interjection = nil
	return taken
}

// SetUsageReporter registers fn to receive every model response's token
// usage as it arrives (including each round of a tool loop) — the hook the
// desktop layer uses to persist usage stats. Pass nil to disable.
func (a *Agent) SetUsageReporter(fn func(model.Usage)) {
	if a == nil {
		return
	}
	a.onUsage = fn
}

// SetToolCallStartReporter wires the "a tool call is being written" signal —
// see the onToolCallStart field. Set alongside SetUsageReporter after a
// bootstrap; a fresh agent starts with none.
func (a *Agent) SetToolCallProgressReporter(fn func(id, name, subject string, lines int)) {
	if a == nil {
		return
	}
	a.onToolCallProgress = fn
}

// SetToolCallRefusedReporter wires the "that call will never run" signal — see
// the onToolCallRefused field. Wired alongside the progress reporter, because a
// caller that draws the row is the caller that has to be told to stop.
func (a *Agent) SetToolCallRefusedReporter(fn func(id, name, subject string)) {
	if a == nil {
		return
	}
	a.onToolCallRefused = fn
}

// recordUsage is the one place a response's usage is taken in: it keeps
// LastUsage's per-call semantics and fans out to the reporter.
func (a *Agent) recordUsage(u *model.Usage) {
	if u == nil {
		return
	}
	a.lastUsage = *u
	if a.onUsage != nil && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
		a.onUsage(*u)
	}
}

type AgentConfig struct {
	Provider     model.Provider
	Model        string
	SystemPrompt string
	MaxChars     int
	MaxToolCalls int
}

func NewAgent(cfg AgentConfig) *Agent {
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = "You are Aetox, a concise and helpful terminal assistant."
	}
	return &Agent{
		provider:     cfg.Provider,
		model:        cfg.Model,
		lastUsage:    model.Usage{},
		maxToolCalls: cfg.MaxToolCalls,
		context:      memory.NewContext(systemPrompt, 0, cfg.MaxChars),
	}
}

func (a *Agent) RespondWithTools(
	ctx context.Context,
	modelTools []model.ToolDefinition,
	userMessage string,
	execTool func(context.Context, model.ToolCall) (string, []model.Image, error),
	onReasoningChunk func(string) error,
	opts turn.TurnOptions,
) (string, bool, error) {
	defer debuglog.Block(fmt.Sprintf("Agent.RespondWithTools (tools=%d)", len(modelTools)))()

	if len(modelTools) == 0 || execTool == nil || !a.supportsToolCalling() {
		debuglog.Msg("fallback to Respond (tools=%d supportsToolCalling=%v)", len(modelTools), a.supportsToolCalling())
		reply, err := a.Respond(ctx, userMessage, opts)
		return reply, false, err
	}
	if a.provider == nil {
		return "", false, errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", false, errors.New("input is empty")
	}
	a.compactIfNeeded(ctx)
	a.addUserTurn(msg, opts)

	// OpenCode-style loop: run until the model stops calling tools. The brakes
	// are the permission/approval layer, ctx cancellation (Ctrl+C in the CLI,
	// the Stop button in the desktop app), and the doom-loop guard below — not
	// an arbitrary round cap. MaxToolCalls > 0 opts back into a hard cap.
	maxToolCalls := a.maxToolCalls
	debuglog.Info("maxToolCalls", fmt.Sprintf("%d (<=0 means unlimited)", maxToolCalls))
	anyToolUsed := false
	nudgedEmpty := false
	dsmlNudges := 0
	overflowCompactions := 0
	droppedConnections := 0
	emptyCompletions := 0
	// The pieces of an answer the output limit cut in half, waiting to be
	// joined back together. Empty for every turn that fits in one round, which
	// is nearly all of them. Held raw rather than trimmed: the limit cuts
	// mid-token, so the whitespace at the seam is part of the word.
	carried := ""
	continuations := 0
	var lastCallKey string
	repeatedCalls := 0
	for i := 0; maxToolCalls <= 0 || i < maxToolCalls; i++ {
		debuglog.Msg("tool loop iteration %d (max=%d)", i+1, maxToolCalls)
		if ctx.Err() != nil {
			return "", anyToolUsed, ctx.Err()
		}
		// Re-check every round (OpenCode checks per step): a single long tool
		// loop can cross the budget mid-turn, and without this the context
		// falls back to dropping old turns verbatim instead of summarizing.
		// SplitForCompaction can't split the in-flight turn — its boundary
		// lands on a RoleUser message — so live tool results stay verbatim.
		if i > 0 {
			a.compactIfNeeded(ctx)
		}
		// Anything the user typed under the running turn goes in before the next
		// request is built, so the model sees it on this round rather than after
		// the turn ends. Consecutive user messages are merged by the providers
		// that require alternating roles (see convertMessagesToAnthropic).
		for _, text := range a.DrainInterjections() {
			debuglog.Msg("interjection folded in before round %d (%d chars)", i+1, len(text))
			a.context.Add(model.RoleUser, interjectionNote+text)
		}
		// Recomputed here, not before the loop: each round adds its own tool
		// results to the input, so the room left for output shrinks as it runs.
		loopMaxTokens := a.toolLoopMaxTokens()
		response, err := a.completeToolLoop(ctx, a.buildRequest(a.context.Messages(), loopMaxTokens, 0.2, modelTools, "auto", opts), onReasoningChunk, opts)
		if err != nil {
			debuglog.Msg("Complete() error: %v", err)
			// A half-written answer must not survive the failure that
			// interrupted it — whatever replaces it says something different.
			discardPreview(opts)
			// The connection went away, and this is judged before anything that
			// reads the error's words because there are no words to read: the
			// provider never answered, so nothing it says can be quoted back at
			// the user and nothing about the request was wrong.
			//
			// Safe to replay from here, and the two lines above are why: the
			// failed round was never added to the context, and discardPreview
			// has taken back whatever streamed. So a second attempt sends the
			// same bytes to the same endpoint and its answer replaces rather
			// than doubles.
			//
			// i-- so a network blip does not spend one of MaxToolCalls, the same
			// bookkeeping the overflow path below uses for the same reason.
			again, endWith := askAgainAfterDrop(ctx, err, droppedConnections)
			if endWith != nil {
				return "", anyToolUsed, endWith
			}
			if again {
				droppedConnections++
				i--
				continue
			}
			// The provider answered, and what it said was nothing: no text, no
			// reasoning, no tool call, and not a truncation either. Nothing
			// about the request was wrong — it is the same round that succeeded
			// six times before it — so it gets asked again instead of ending
			// the turn. A turn that had already run for 350 seconds and
			// collected eighteen tool results is what this used to throw away,
			// on one blank stream frame from a gateway.
			//
			// Same bookkeeping as the drop above, for the same three reasons:
			// discardPreview has taken back whatever streamed, the failed round
			// was never added to the context, and i-- keeps a silent provider
			// from spending one of MaxToolCalls.
			again, endWith = askAgainAfterEmpty(ctx, err, emptyCompletions)
			if endWith != nil {
				return "", anyToolUsed, endWith
			}
			if again {
				emptyCompletions++
				i--
				continue
			}
			if model.IsEmptyCompletion(err) {
				// Out of replays, so change the question rather than ask it
				// again: this is the same nudge the no-error empty reply gets
				// below, and it shares that path's flag because "this turn has
				// been nudged once" is one fact about the turn, not two.
				if !nudgedEmpty {
					nudgedEmpty = true
					debuglog.Msg("provider still answering with nothing, nudging once: %v", err)
					a.context.Add(model.RoleUser, emptyReplyNudge)
					i--
					continue
				}
				// Nudged and still silent. Ended as an error rather than as
				// emptyReplyFallback because that string is an answer, and an
				// answer cannot be retried — the error can, and everything the
				// turn did up to here is still in context for the retry to
				// build on.
				return "", anyToolUsed, err
			}
			// The one failure in here with a mechanical fix. Everything else is
			// the model, the network or the account; this one is just too many
			// bytes, and the provider has told us so with more authority than
			// the char budget has ever had.
			//
			// Until this existed the turn simply died: nothing anywhere in the
			// engine reacted to a context-length rejection, so a session that
			// crossed the line lost the answer AND kept the history that caused
			// it, failing identically on every retry the user typed by hand.
			//
			// i-- so the round that never happened does not spend one of
			// MaxToolCalls, and does not push a first-round failure past the
			// recovery below.
			if model.IsContextLengthError(err) && overflowCompactions < maxOverflowCompactions {
				overflowCompactions++
				if a.compactNow(ctx) {
					debuglog.Msg("prompt over the model's window, summarized and retrying (%d/%d)",
						overflowCompactions, maxOverflowCompactions)
					i--
					continue
				}
				// Nothing left to summarize, so retrying would send the same
				// bytes to the same refusal. Say what is actually wrong.
				return "", anyToolUsed, fmt.Errorf(
					"this conversation no longer fits %s's context window, and there is nothing older left to summarize: %w",
					a.model, err)
			}
			// The second failure with a mechanical fix, and it is a much
			// narrower one than it used to be. Asking again without tools helps
			// exactly when the tools were the problem; on a quota wall, a
			// rejected sign-in or a dropped connection it is a second full-context
			// call that fails the same way, spends the same money and leaves no
			// trace — the turn simply took twice as long for no stated reason
			// (see model.IsToolBlockRejection for what that measured).
			//
			// Worse than the cost: when the retry ANSWERED, it answered without
			// tools and the turn was recorded as a success. A dropped connection
			// came back as "เกินขีดจำกัดของโมเดลปัจจุบัน ... ลองเปลี่ยนโมเดล" —
			// blaming the user's model, with no error and no ลองใหม่ button, for
			// a Wi-Fi blip. An error the user can see and retry beats an answer
			// that cannot do the work and does not say so.
			if i == 0 && model.IsToolBlockRejection(err) {
				// Logged, because this is a second call to the provider and
				// every other one in this loop announces itself. The gap it used
				// to leave in the debug log is what made it so hard to find.
				debuglog.Msg("provider refused the tool block, asking once more as plain chat: %v", err)
				// The user message is already in context — respond over it
				// rather than via Respond(msg), which would add it a second time.
				reply, retryErr := a.respondFromContext(ctx, opts)
				return reply, false, retryErr
			}
			return "", false, err
		}
		a.recordUsage(response.Usage)

		// Two different things from here on, and they were one before an answer
		// could arrive in pieces:
		//
		//   recorded — what THIS round said, which is what history gets. The
		//              earlier pieces are already in context as their own
		//              messages, so stitching them in again would say the first
		//              half twice.
		//   content  — the whole answer, which is what the user is shown and
		//              what the turn is recorded as having replied.
		//
		// Joined with nothing between them: the limit cuts mid-token, so any
		// separator inserted here lands inside a word.
		recorded := strings.TrimSpace(response.Text)
		whole := carried + response.Text
		content := strings.TrimSpace(whole)
		// Consumed. Cleared here rather than where the answer is handed over,
		// because a turn does not end at its first answer: an interjection keeps
		// it alive, and a round that calls tools carries straight on. Either
		// would have found the previous answer's first half still sitting here
		// and glued it to the front of the next one.
		carried = ""
		debuglog.Info("response.text", truncateStr(content, 100))
		debuglog.Info("response.toolCalls", fmt.Sprintf("%d", len(response.ToolCalls)))
		// Logged because its absence cost a diagnosis. An empty round is either
		// a model that spent its whole output budget thinking (finish_reason
		// "length") or a provider that answered with nothing at all, and the
		// two want opposite remedies — shrink the round, or change model. The
		// log said "response.text = " for both, twice in one session, for two
		// minutes each (owner's log, 13:20 and 13:21 on 23 ส.ค. 2026).
		spent := 0
		if response.Usage != nil {
			spent = response.Usage.CompletionTokens
		}
		debuglog.Info("response.finish", fmt.Sprintf("%q, out %d of %d", response.FinishReason, spent, loopMaxTokens))
		if len(response.ToolCalls) == 0 {
			// The answer did not end, it ran out of room. finish_reason
			// "length" on a round with no tool calls means the model was still
			// writing when the output budget ran out, and until this existed
			// that half-sentence was simply handed over as the reply — the
			// answer stopped mid-word and nothing anywhere asked for the rest.
			//
			// The preview is deliberately NOT discarded: what is on screen is
			// the first half of this same answer and the continuation appends to
			// it, so the user watches one answer being written rather than
			// seeing it vanish and restart. That is the whole difference between
			// this and every other retry in this loop, all of which replace what
			// they took back.
			//
			// i-- because a continuation is not a round of work, it is the same
			// round still being written.
			if response.FinishReason == model.FinishReasonLength && recorded != "" &&
				continuations < maxAnswerContinuations {
				continuations++
				debuglog.Msg("answer cut at the output limit, asking for the rest (%d/%d)",
					continuations, maxAnswerContinuations)
				carried = whole
				a.context.AddMessage(model.Message{
					Role:             model.RoleAssistant,
					Content:          recorded,
					ReasoningContent: strings.TrimSpace(response.ReasoningContent),
				})
				a.context.Add(model.RoleUser, truncatedAnswerNudge)
				i--
				continue
			}
			// DeepSeek can leak DSML tool-call markup as plain text, and the
			// backstop (openai_compatible.go) only lifts out COMPLETE blocks —
			// a block cut off before its closing tag falls through here as raw
			// markup with no ToolCalls, bypassing the truncation guard below.
			// Never accept that as the answer: tell the model to call the tool
			// for real and retry, capped so a persistent leaker can't spin.
			if model.ContainsLeakedDSML(content) {
				// The gate kept the markup itself off screen, but the prose in
				// front of it streamed — and this round is about to be thrown
				// away, so that prose is not the answer either.
				discardPreview(opts)
				if dsmlNudges < maxDSMLNudges {
					dsmlNudges++
					debuglog.Msg("leaked DSML tool call, nudging (%d/%d)", dsmlNudges, maxDSMLNudges)
					a.context.Add(model.RoleUser, dsmlLeakNudge)
					continue
				}
				// Both, because from here the two have to say the same thing:
				// a fallback that reached the user but not the history would
				// leave the next round answering a question it cannot see.
				content = dsmlLeakFallback // gave up — don't surface raw markup
				recorded = content
			}
			if content == "" {
				// Nudge inside the loop, not via recoverEmptyReply: here the
				// model keeps its tools, so it can cover a missing capability
				// (e.g. reading an image) by calling a skill instead of refusing.
				// Whitespace-only text is empty to this check but was streamed,
				// so the preview is cleared here too.
				discardPreview(opts)
				if !nudgedEmpty {
					nudgedEmpty = true
					debuglog.Msg("empty reply in tool loop, nudging once")
					a.context.Add(model.RoleUser, emptyReplyNudge)
					continue
				}
				content = emptyReplyFallback
				recorded = content
			}
			a.context.AddMessage(model.Message{
				Role:             model.RoleAssistant,
				Content:          recorded,
				ReasoningContent: strings.TrimSpace(response.ReasoningContent),
			})
			// The model was ready to stop, but the user typed while it was writing
			// this. Keeping the turn alive is the whole point of Interject: ending
			// here would make them wait out a reply they have already moved past.
			// The answer it just gave stays in context, so the next round builds on
			// it instead of repeating it.
			if pending := a.DrainInterjections(); len(pending) > 0 {
				// The turn stays alive, so this answer is not the reply — but it
				// is still something the model said, so it goes into the
				// sequence as a non-final part rather than being retracted. That
				// is the difference the parts model makes: nothing has to be
				// un-said, only re-placed.
				//
				// Demoted, not merely non-final: what this round wrote is a
				// finished answer, and the round that carries no tool calls is
				// the only place that can still say so. Downstream the two look
				// identical, and the executor drew this one as a narration row.
				if opts.OnRound != nil {
					opts.OnRound(turn.RoundEvent{Text: content, Demoted: true})
				}
				for _, text := range pending {
					debuglog.Msg("interjection kept the turn alive (%d chars)", len(text))
					a.context.Add(model.RoleUser, interjectionNote+text)
				}
				continue
			}
			if opts.OnRound != nil {
				opts.OnRound(turn.RoundEvent{Text: content, Final: true})
			}
			return content, anyToolUsed, nil
		}
		anyToolUsed = true

		// Sanitized on the way in, never raw: a tool call whose arguments were
		// cut off is refused below, but the message carrying it is re-sent for
		// the rest of the conversation, and a provider that validates history
		// 400s on the fragment long after the round it came from. See
		// model.SanitizeToolCallArguments for the turn this cost.
		a.context.AddMessage(model.Message{
			Role:             model.RoleAssistant,
			Content:          recorded,
			ReasoningContent: strings.TrimSpace(response.ReasoningContent),
			ToolCalls:        model.SanitizeToolCallArguments(response.ToolCalls),
		})
		// Before the tool calls run, so the narration lands in the sequence
		// above the work it announces. It is a part of the turn, not a draft to
		// be taken back — the executor's OnRound is what records it and hands
		// the live preview over.
		if opts.OnRound != nil {
			opts.OnRound(turn.RoundEvent{Text: content})
		}

		// Truncation guard (same failure OpenCode hit, sst/opencode#18108):
		// a tool call the model did not finish writing. Executing it would
		// fail with a misleading parse/path error the model then "fixes"
		// forever. Tell it the truth instead.
		//
		// The judgement moved into model.ToolCallTruncated so it stops
		// depending on finish_reason, which not every provider sets when it
		// cuts a call; the round's own ceiling and the arguments themselves
		// answer without asking the provider anything. Every call in the round
		// still gets a result, since the ids in the assistant message above
		// require one, but only the call actually cut is told to write less.
		if cut, atLimit := model.ToolCallTruncated(response, loopMaxTokens); cut {
			debuglog.Msg("tool call truncated (at_limit=%v, ceiling=%d, finish_reason=%q), telling the model",
				atLimit, loopMaxTokens, response.FinishReason)
			for _, toolCall := range response.ToolCalls {
				content := model.UnfinishedRoundRefusal(toolCall.Function.Name, loopMaxTokens)
				if model.ToolCallUnfinished(toolCall) {
					content = model.TruncatedToolCallRefusal(toolCall.Function.Name, loopMaxTokens, atLimit, nil)
				}
				a.context.AddMessage(model.Message{
					Role:       model.RoleTool,
					Name:       toolCall.Function.Name,
					ToolCallID: toolCall.ID,
					Content:    content,
				})
				// The row was drawn while the arguments streamed. Nothing below
				// will run this call, so this is the only place left that can
				// say so.
				if a.onToolCallRefused != nil {
					subject, _ := model.SubjectFromPartialArgs(toolCall.Function.Arguments)
					a.onToolCallRefused(toolCall.ID, toolCall.Function.Name, subject)
				}
			}
			continue
		}

		calls := response.ToolCalls
		for i := 0; i < len(calls); {
			// How many of the calls starting here may run at the same time.
			// Always at least one, so the sequential path below is the same
			// path it has always been.
			group := parallelGroup(calls[i:])

			// Doom-loop guard (mirrors OpenCode session/prompt.ts): identical
			// (name, args) calls back to back — nudge at 3, stop at 5. Read in
			// call order whether or not the calls then run together: the guard
			// is about what the model asked for, and the model asked for these
			// in this order.
			repeats := make([]int, group)
			for k := 0; k < group; k++ {
				toolCall := calls[i+k]
				callKey := toolCall.Function.Name + "\x00" + toolCall.Function.Arguments
				if callKey == lastCallKey {
					repeatedCalls++
				} else {
					lastCallKey, repeatedCalls = callKey, 1
				}
				if repeatedCalls >= doomLoopStop {
					debuglog.Msg("doom loop: %s repeated %d times, stopping", toolCall.Function.Name, repeatedCalls)
					stopMsg := fmt.Sprintf("%s เรียกเครื่องมือ %s ด้วยค่าเดิมซ้ำ %d ครั้งติดกันโดยไม่คืบหน้า — ลองสั่งใหม่หรือปรับคำสั่งดูครับ", DoomLoopStopPrefix, toolCall.Function.Name, repeatedCalls)
					a.context.AddMessage(model.Message{
						Role:       model.RoleTool,
						Name:       toolCall.Function.Name,
						ToolCallID: toolCall.ID,
						Content:    "aborted: identical tool call repeated " + fmt.Sprint(repeatedCalls) + " times",
					})
					return stopMsg, true, nil
				}
				repeats[k] = repeatedCalls
			}

			outputs := make([]string, group)
			images := make([][]model.Image, group)
			toolErrs := make([]error, group)
			if group == 1 {
				debuglog.Msg("tool call: %s(%s)", calls[i].Function.Name, truncateStr(calls[i].Function.Arguments, 80))
				outputs[0], images[0], toolErrs[0] = a.executeToolCall(ctx, calls[i], execTool)
			} else {
				debuglog.Msg("running %d read-only tool calls together", group)
				var wg sync.WaitGroup
				for k := 0; k < group; k++ {
					wg.Add(1)
					go func(k int) {
						defer wg.Done()
						toolCall := calls[i+k]
						debuglog.Msg("tool call: %s(%s)", toolCall.Function.Name, truncateStr(toolCall.Function.Arguments, 80))
						outputs[k], images[k], toolErrs[k] = a.executeToolCall(ctx, toolCall, execTool)
					}(k)
				}
				wg.Wait()
			}

			// Results enter the context in call order however they finished, so
			// the history a provider receives is the one it dictated: a
			// tool_calls message followed by its results in the order it wrote
			// them. Which one came back first is this machine's business.
			cancelled := ""
			for k := 0; k < group; k++ {
				toolCall := calls[i+k]
				callOutput := strings.TrimSpace(outputs[k])
				if callOutput == "" {
					if toolErrs[k] != nil {
						callOutput = toolErrs[k].Error()
					} else {
						callOutput = "(no output)"
					}
				}
				if repeats[k] == doomLoopWarn {
					callOutput += "\n[loop warning] You have now made this exact tool call " + fmt.Sprint(repeats[k]) + " times in a row with the same result. Try a different approach instead of repeating it."
				}
				debuglog.Msg("tool result: %s (err=%v)", truncateStr(callOutput, 120), toolErrs[k])
				a.context.AddMessage(model.Message{
					Role:       model.RoleTool,
					Name:       toolCall.Function.Name,
					ToolCallID: toolCall.ID,
					Content:    callOutput,
				})
				// A picture cannot travel in the tool result itself: Anthropic
				// allows an image block inside tool_result, the OpenAI-compatible
				// APIs do not, and Ollama has no tool_result shape at all. One
				// follow-up user message carrying the image works on all three, so
				// that is what this is — a single path rather than three, at the
				// cost of one extra message in history.
				//
				// It says which call it belongs to, because a user message the user
				// did not send is otherwise indistinguishable from one they did.
				if len(images[k]) > 0 {
					a.context.AddMessage(model.Message{
						Role:    model.RoleUser,
						Content: "[the image returned by " + toolCall.Function.Name + " follows]",
						Images:  images[k],
					})
				}
				if toolErrs[k] != nil && cancelled == "" {
					cancelled = callOutput
				}
			}
			// Checked once the whole group is written down rather than at the
			// first failure: a Stop lands on every call in flight at the same
			// instant, and leaving the others' results out would hand the
			// provider a tool_calls message with holes in its replies.
			if cancelled != "" && ctx.Err() != nil {
				return cancelled, true, ctx.Err()
			}
			i += group
		}
	}

	return ToolLoopExhausted, anyToolUsed, nil
}

// completeToolLoop runs one tool-loop request. When a reasoning handler is
// present and the provider can stream, it uses StreamComplete so the model's
// thinking — and, when the caller asked for it, the answer as it is written —
// reaches the UI live. Non-streaming providers, or turns with no reasoning
// handler (e.g. the CLI), keep using Complete unchanged.
//
// Content used to be withheld outright: a nil handler was the only thing
// keeping leaked DSML tool-call markup off-screen, at the price of the answer
// arriving in one silent jump. model.GateLeakedDSML now does that job at the
// chunk level, so the text can stream and the markup still cannot.
//
// What the gate does NOT decide is whether this round's text is the answer at
// all — only the loop knows that, and it says so by erasing the preview
// (opts.OnContentReset) on every path that does not surface `content`.
func (a *Agent) completeToolLoop(ctx context.Context, req model.Request, onReasoningChunk func(string) error, opts turn.TurnOptions) (model.Response, error) {
	if onReasoningChunk != nil {
		if streamer, ok := a.provider.(model.StreamingProvider); ok {
			var onContent model.StreamChunkHandler
			if opts.OnContent != nil {
				onContent = model.GateLeakedDSML(func(chunk string) error {
					opts.OnContent(chunk)
					return nil
				})
			}
			return streamer.StreamComplete(ctx, req, onContent, onReasoningChunk)
		}
	}
	return a.provider.Complete(ctx, req)
}

// discardPreview erases whatever the live preview has shown for the round that
// just finished.
//
// Only three callers are left, and they are the only rounds whose text never
// enters the turn at all: one that leaked markup and is about to be nudged, one
// that came back empty, and one that failed. Narration and an interjected-over
// answer used to be here too — they are parts of the turn now, recorded rather
// than retracted, which is the point of the sequence.
//
// A no-op when nobody asked for a preview, which is every caller but the
// desktop.
func discardPreview(opts turn.TurnOptions) {
	if opts.OnContentReset != nil {
		opts.OnContentReset()
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// compactIfNeeded summarizes older history into one message when the context
// budget is nearly full, so long coding sessions keep their early decisions
// instead of losing whole turns to the char trim. Failure is non-fatal: on
// any error the turn proceeds and enforceLimits still guards the budget.
func (a *Agent) compactIfNeeded(ctx context.Context) {
	if a == nil || a.context == nil || a.provider == nil {
		return
	}
	// Sweep before summarizing, and only summarize if sweeping was not enough:
	// the sweep is free and lossless-in-practice (everything it clears can be
	// re-run), the summary costs a model call and keeps only what the
	// summarizer thought mattered.
	if a.context.NeedsCompaction(microCompactThresholdFraction) {
		if swept, freed := a.context.MicroCompact(compactKeepRecent, sweepableToolOutputs); swept > 0 {
			debuglog.Info("micro-compacted", fmt.Sprintf("%d old tool outputs cleared, %d chars freed", swept, freed))
		}
	}
	if !a.context.NeedsCompaction(compactThresholdFraction) {
		return
	}
	a.compact(ctx)
}

// sweepableToolOutputs is every tool whose old output the micro sweep may
// clear: the parallel-safe read tools (same judgement — somebody wrote the
// name down knowing the tool only reads and can be called again), plus the
// re-viewable documents and the repo map. Absent on purpose: `shell` — a
// marker inviting the model to "run it again" on a command that mutated
// something is an invitation to mutate it twice — and `browser`, whose output
// is a page state that may no longer exist to re-fetch.
var sweepableToolOutputs = func() map[string]bool {
	m := map[string]bool{"skill_view": true, "skills_list": true, "repo_map": true}
	for name := range parallelToolCalls {
		m[name] = true
	}
	return m
}()

// compactNow summarizes whether or not the char budget asked for it, and says
// whether anything actually moved.
//
// The budget is measured in bytes and the limit it stands in for is measured in
// tokens, and the ratio between them is not a constant: across three real
// sessions on one machine it ranged 3.45 to 5.46 bytes per token depending on
// how much of the turn was Thai prose and how much was English tool output. So
// bootstrap.ContextChars multiplying by a flat 4 is a decent centre and a poor
// guarantee, and on the low side a budget that reads as full-and-fine is
// already past what the model will accept.
//
// Which makes the provider the only party that actually knows. This is the door
// for its answer: when it rejects a prompt for length, the history really is too
// long whatever the budget believed, and it gets summarized on the spot.
//
// The summarizer's own request is the old messages minus the recent few, with no
// tool block attached, so it is materially smaller than the request that just
// failed and normally fits where that did not. When it does not, it fails the
// way it always has (err -> "compaction skipped") and this returns false, which
// the caller reads as "no point retrying".
func (a *Agent) compactNow(ctx context.Context) bool {
	if a == nil || a.context == nil || a.provider == nil {
		return false
	}
	return a.compact(ctx)
}

// compact is the summarization itself, with no opinion about whether it should
// happen. Shared so the budget-driven path and the provider-driven one cannot
// summarize differently.
func (a *Agent) compact(ctx context.Context) bool {
	old, recentStart := a.context.SplitForCompaction(compactKeepRecent)
	if len(old) == 0 {
		return false
	}
	defer debuglog.Block(fmt.Sprintf("Agent.compact (%d msgs)", len(old)))()
	response, err := a.completeWithReconnect(ctx, model.Request{
		Model: a.model,
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: compactionPrompt},
			{Role: model.RoleUser, Content: renderCompactionTranscript(old)},
		},
		MaxTokens:   compactSummaryMaxTokens,
		Temperature: 0.2,
	})
	summary := ""
	if response.Text != "" {
		summary = strings.TrimSpace(response.Text)
	}
	if err != nil || summary == "" {
		debuglog.Msg("compaction skipped (err=%v, empty=%v)", err, summary == "")
		return false
	}
	a.context.ReplaceWithSummary(summary, recentStart)
	debuglog.Info("compacted", fmt.Sprintf("%d old messages -> %d summary chars", len(old), len(summary)))
	return true
}

// renderCompactionTranscript flattens messages into plain text for the
// summarizer — roles marked, tool calls noted with truncated arguments.
func renderCompactionTranscript(messages []model.Message) string {
	var b strings.Builder
	for _, m := range messages {
		b.WriteString("--- ")
		b.WriteString(string(m.Role))
		if m.Name != "" {
			b.WriteString(" (" + m.Name + ")")
		}
		b.WriteString(" ---\n")
		if m.Content != "" {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
		for _, tc := range m.ToolCalls {
			b.WriteString("[tool call] " + tc.Function.Name + " " + truncateStr(tc.Function.Arguments, 400) + "\n")
		}
	}
	return b.String()
}

// parallelToolCalls names the tools that may run beside each other when the
// model asks for several in one round.
//
// A model that wants to read six files says so in one round — every provider
// supports it and the prompt already invites it — and this loop then ran them
// strictly one after another, so six 200ms reads cost 1.2s of wall clock for
// work that has no order in it. Running them together is the whole gain, and it
// is only available to calls that cannot see each other's effects.
//
// An allow-list, not a deny-list, and that direction is the load-bearing part.
// The obvious spelling — "run anything safety.AssessCommand calls harmless
// together" — cannot be used: that function's catch-all answers RiskLow with no
// effects for every name it does not recognize, which is every MCP tool and
// every skill added after it was written. A deny-list would silently
// parallelize a payment API the day someone connects one. A name has to be
// written here, by somebody who knows the tool only reads, before it runs
// beside anything.
//
// So this list is exactly the read tools of this repo. Everything else — shell,
// write, edit, delete, git, task, every MCP tool — keeps the sequential path it
// has always had, because a write and the read after it are the one pair whose
// order is the answer. The media tools (image_ocr, video_ocr,
// audio_transcribe) are read-only too and are deliberately absent: they run
// ffmpeg and whisper, and four of those at once is a machine that stops
// answering, not a faster turn.
var parallelToolCalls = map[string]bool{
	"read": true, "list": true, "glob": true, "grep": true, "tree": true,
	"pdf_read": true, "web_fetch": true, "web_search": true, "calc": true,
}

// maxParallelToolCalls caps one group, keeping the ceiling on open files,
// sockets and live timeline rows somewhere a person can picture.
//
// Raised from four to five on 2026-08-20 (owner's call). The number has never
// actually bound: across 1,769 debug logs the loop grouped calls 24 times, 19 of
// them two calls and 5 of them three, and nothing has ever asked for four. So
// this buys headroom rather than throughput, and it is recorded that way — the
// thing worth measuring is how rarely a model batches at all, not where the
// ceiling sits.
const maxParallelToolCalls = 5

// parallelGroup counts how many calls from the front of the slice may run
// together: a run of parallel-safe calls, capped, or 1 for anything else.
//
// Consecutive, never gathered from across the round. A round of
// read, write, read has an order that matters, and pulling the two reads out to
// run beside each other would move the second one in front of the write.
func parallelGroup(calls []model.ToolCall) int {
	safe := func(c model.ToolCall) bool {
		return parallelToolCalls[strings.ToLower(strings.TrimSpace(c.Function.Name))]
	}
	if len(calls) < 2 || !safe(calls[0]) {
		return 1
	}
	n := 1
	for n < len(calls) && n < maxParallelToolCalls && safe(calls[n]) {
		n++
	}
	return n
}

func (a *Agent) executeToolCall(ctx context.Context, toolCall model.ToolCall, execTool func(context.Context, model.ToolCall) (string, []model.Image, error)) (string, []model.Image, error) {
	if strings.TrimSpace(toolCall.Function.Name) == "" {
		return "tool-call-missing-name", nil, errors.New("tool call missing function name")
	}
	return execTool(ctx, toolCall)
}

// recoverEmptyReply nudges the model once after an empty reply. The nudge is
// ephemeral — never stored in context — so history stays clean either way.
func (a *Agent) recoverEmptyReply(ctx context.Context, opts turn.TurnOptions) string {
	msgs := append(a.context.Messages(), model.Message{Role: model.RoleUser, Content: emptyReplyNudge})
	response, err := a.completeWithReconnect(ctx, a.buildRequest(msgs, 768, 0.2, nil, "", opts))
	if err != nil {
		debuglog.Msg("empty-reply nudge failed: %v", err)
		return emptyReplyFallback
	}
	if reply := strings.TrimSpace(response.Text); reply != "" {
		return reply
	}
	return emptyReplyFallback
}

func (a *Agent) Respond(ctx context.Context, userMessage string, opts turn.TurnOptions) (string, error) {
	if a.provider == nil {
		return "", errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", errors.New("input is empty")
	}

	a.compactIfNeeded(ctx)
	a.addUserTurn(msg, opts)
	return a.respondFromContext(ctx, opts)
}

// addUserTurn writes the message that opens a turn, with whatever the user
// attached to it. Separate from context.Add because the images belong to this
// one message only: every later RoleUser entry in the same loop — an
// interjection, the empty-reply nudge, the DSML nudge — is Aetox talking to the
// model, not the user handing it a picture, and re-attaching there would send
// the image again on every round.
func (a *Agent) addUserTurn(msg string, opts turn.TurnOptions) {
	if len(opts.Images) == 0 && len(opts.Documents) == 0 {
		a.context.Add(model.RoleUser, msg)
		return
	}
	a.context.AddMessage(model.Message{
		Role:      model.RoleUser,
		Content:   msg,
		Images:    opts.Images,
		Documents: opts.Documents,
	})
}

// askAgainAfterDrop decides what a failed model call means when the failure was
// the connection rather than the provider.
//
// Three answers, because there are three things that can be true.
//
//   - (true, nil) — a blip worth replaying, and the pause has already been
//     waited out by the time this returns.
//   - (false, nil) — not this kind of failure. The caller carries on with its
//     own handling, which is where a context-length rejection or a refused tool
//     block gets read.
//   - (false, err) — done with it, and err is what the turn ends with.
//
// That last one carries two different endings and both have to be exact. Stop
// pressed during the pause must be reported as Stop, because the desktop tells
// a cancelled turn from a failed one by the words in the error and would
// otherwise draw a red retry box over a button the user chose to press. A
// connection that never came back is labelled as one (model.AsDroppedConnection)
// so the window can say what happened in the user's own language instead of
// showing them "wsarecv".
//
// Returning early on the give-up is deliberate rather than incidental: a
// dropped connection is not a context-length rejection and not a tool-block
// refusal, so there is nothing below for the caller to fall through to. The
// provider never answered, and every one of those checks reads an answer.
//
// One function, every caller, because a dropped socket is not a property of
// which route was taken to the provider.
func askAgainAfterDrop(ctx context.Context, err error, spent int) (bool, error) {
	if !model.IsDroppedConnection(ctx, err) {
		return false, nil
	}
	if spent >= maxDroppedConnectionRetries {
		return false, model.AsDroppedConnection(err)
	}
	// A pause, and a short one. The socket was cut a moment ago, so asking
	// again in the same millisecond usually finds whatever cut it still there;
	// much longer and the wait itself reads as the app having hung, which is
	// the complaint this whole path came from.
	wait := time.Duration(spent+1) * time.Second
	debuglog.Msg("the connection dropped mid-answer, asking again in %s (%d/%d): %v",
		wait, spent+1, maxDroppedConnectionRetries, err)
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(wait):
		return true, nil
	}
}

// askAgainAfterEmpty is askAgainAfterDrop for the other silence: the provider
// answered on time, with nothing in it.
//
// Same three answers, with one deliberate difference at the end. A spent budget
// returns (false, nil) rather than an error, because unlike a dropped
// connection there IS something for the caller to fall through to — the nudge
// in the tool loop, recoverEmptyReply in the plain one. This function decides
// only whether the identical request is still worth repeating.
func askAgainAfterEmpty(ctx context.Context, err error, spent int) (bool, error) {
	if !model.IsEmptyCompletion(err) {
		return false, nil
	}
	if spent >= maxEmptyCompletionReplays {
		return false, nil
	}
	// The same short pause the drop gets, for a different reason: an empty
	// stream is usually a gateway shedding one request, and the shedding lasts
	// about as long as it takes to say so.
	wait := time.Duration(spent+1) * time.Second
	debuglog.Msg("the provider answered with nothing, asking again in %s (%d/%d): %v",
		wait, spent+1, maxEmptyCompletionReplays, err)
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(wait):
		return true, nil
	}
}

// completeWithReconnect is provider.Complete with the rule above applied, and
// it is what every completion in this file goes through bar one.
//
// The exception is completeToolLoop's, and it is an exception because the tool
// loop has to do two things around its own retry that nothing here can do for
// it: take back the half-answer that streamed (discardPreview) and un-spend the
// round (i--). Everywhere else there is no preview and no round to account for,
// so the rule is the whole of the handling and belongs in one place rather than
// copied into five.
//
// Those five are not incidental. The compaction call is the one that decides
// whether the turn even fits, and losing it to a blip is how a network failure
// comes back to the user as "this conversation no longer fits the context
// window" — the model blamed for the wifi, which §166 is the long version of.
func (a *Agent) completeWithReconnect(ctx context.Context, req model.Request) (model.Response, error) {
	// Two budgets, not one: a socket that keeps dying and a gateway that keeps
	// answering with nothing are different failures, and neither should be able
	// to spend the other's attempts.
	for dropped, empties := 0, 0; ; {
		response, err := a.provider.Complete(ctx, req)
		if err == nil {
			return response, nil
		}
		again, endWith := askAgainAfterDrop(ctx, err, dropped)
		if endWith != nil {
			return model.Response{}, endWith
		}
		if again {
			dropped++
			continue
		}
		again, endWith = askAgainAfterEmpty(ctx, err, empties)
		if endWith != nil {
			return model.Response{}, endWith
		}
		if again {
			empties++
			continue
		}
		return model.Response{}, err
	}
}

// respondFromContext completes over the context as-is — the user message must
// already be in history. Shared by Respond and the tool-loop fallback so the
// fallback can't add the user message a second time.
func (a *Agent) respondFromContext(ctx context.Context, opts turn.TurnOptions) (string, error) {
	response, err := a.completeWithReconnect(ctx, a.buildRequest(a.context.Messages(), a.toolLoopMaxTokens(), 0.2, nil, "", opts))
	if err != nil {
		// An empty answer that outlasted its replays is not a failure to report
		// — it is the empty reply the line below has always known how to
		// handle. It only ever arrived as a Response with no text; providers
		// that state it as an error (model.ErrEmptyCompletion) were skipping
		// the recovery entirely and ending the turn red instead.
		if !model.IsEmptyCompletion(err) {
			return "", err
		}
		response = model.Response{}
	}

	reply := strings.TrimSpace(response.Text)
	if reply == "" {
		reply = a.recoverEmptyReply(ctx, opts)
	}
	a.lastUsage = model.Usage{}
	a.recordUsage(response.Usage)

	a.context.AddMessage(model.Message{
		Role:             model.RoleAssistant,
		Content:          reply,
		ReasoningContent: strings.TrimSpace(response.ReasoningContent),
	})
	return reply, nil
}

// RespondEphemeral answers a one-shot prompt over the current conversation
// WITHOUT writing anything into history — the Claude Code/OpenCode pattern for
// meta work (tool-run summaries, titles): the session transcript must never
// contain fabricated user messages carrying kilobytes of tool output.
func (a *Agent) RespondEphemeral(ctx context.Context, prompt string, opts turn.TurnOptions) (string, error) {
	if a.provider == nil {
		return "", errors.New("agent provider is not initialized")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("input is empty")
	}
	msgs := append(a.context.Messages(), model.Message{Role: model.RoleUser, Content: prompt})
	response, err := a.completeWithReconnect(ctx, a.buildRequest(msgs, 768, 0.2, nil, "", opts))
	if err != nil {
		return "", err
	}
	a.recordUsage(response.Usage)
	return strings.TrimSpace(response.Text), nil
}

func (a *Agent) RespondStream(ctx context.Context, userMessage string, onChunk func(string) error, onReasoningChunk func(string) error, opts turn.TurnOptions) (string, bool, error) {
	if a.provider == nil {
		return "", false, errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", false, errors.New("input is empty")
	}

	a.compactIfNeeded(ctx)
	a.context.Add(model.RoleUser, msg)

	// Same per-provider output ceiling as the tool loop (OpenCode's
	// OUTPUT_TOKEN_MAX approach) — a ceiling costs nothing until used, and 768
	// used to truncate long answers mid-sentence for non-tool-calling models.
	req := a.buildRequest(a.context.Messages(), a.toolLoopMaxTokens(), 0.2, nil, "", opts)

	if streamer, ok := a.provider.(model.StreamingProvider); ok {
		// Gated for the same reason the tool loop is: this path has no DSML
		// backstop at all (it never inspects the reply), and it tells its caller
		// the text is already on screen — so raw markup here would be permanent,
		// not merely early.
		response, err := streamer.StreamComplete(ctx, req, model.GateLeakedDSML(onChunk), onReasoningChunk)
		if err == nil {
			reply := strings.TrimSpace(response.Text)
			streamed := true
			if reply == "" {
				reply = a.recoverEmptyReply(ctx, opts)
				streamed = false // nothing reached onChunk — caller must render the reply itself
			}
			a.lastUsage = model.Usage{}
			a.recordUsage(response.Usage)
			a.context.AddMessage(model.Message{
				Role:             model.RoleAssistant,
				Content:          reply,
				ReasoningContent: strings.TrimSpace(response.ReasoningContent),
			})
			return reply, streamed, nil
		}
		// fallback to non-streaming when streaming path fails
	}

	response, err := a.completeWithReconnect(ctx, req)
	if err != nil {
		return "", false, err
	}

	reply := strings.TrimSpace(response.Text)
	if reply == "" {
		reply = a.recoverEmptyReply(ctx, opts)
	}
	a.lastUsage = model.Usage{}
	a.recordUsage(response.Usage)
	a.context.AddMessage(model.Message{
		Role:             model.RoleAssistant,
		Content:          reply,
		ReasoningContent: strings.TrimSpace(response.ReasoningContent),
	})
	return reply, false, nil
}

func (a *Agent) supportsToolCalling() bool {
	provider, ok := a.provider.(interface{ SupportsToolCalling() bool })
	return ok && provider.SupportsToolCalling()
}

func (a *Agent) SupportsToolCalling() bool {
	return a.supportsToolCalling()
}

func (a *Agent) ResolveThinkProfile(level think.Level) think.Profile {
	return think.Resolve(level, model.ProviderSupportsReasoning(a.provider))
}

func (a *Agent) ReplaceModel(provider model.Provider, modelName string) {
	a.provider = provider
	if modelName != "" {
		a.model = modelName
	}
}

func (a *Agent) ClearContext() {
	if a.context == nil {
		return
	}
	messages := a.context.Messages()
	systemPrompt := "You are Aetox, a concise and helpful terminal assistant."
	if len(messages) > 0 {
		systemPrompt = messages[0].Content
	}
	a.context.Reset(systemPrompt)
	a.lastUsage = model.Usage{}
}

// RestoreHistory appends prior conversation turns into the agent's context,
// so a reloaded chat session continues with its memory intact.
func (a *Agent) RestoreHistory(messages []model.Message) {
	if a == nil || a.context == nil {
		return
	}
	for _, m := range messages {
		a.context.AddMessage(m)
	}
}

func (a *Agent) ContextUsage() (messageCount int, usedChars int, maxChars int) {
	if a == nil || a.context == nil {
		return 0, 0, 0
	}
	return a.context.UsageStats()
}

// ContextMessages returns a copy of the conversation as currently held in
// memory (system prompt first) — for UI features like the context meter.
func (a *Agent) ContextMessages() []model.Message {
	if a == nil || a.context == nil {
		return nil
	}
	return a.context.Messages()
}

func (a *Agent) LastUsage() model.Usage {
	return a.lastUsage
}

func (a *Agent) buildRequest(messages []model.Message, maxTokens int, temperature float64, tools []model.ToolDefinition, toolChoice string, opts turn.TurnOptions) model.Request {
	req := model.Request{
		Model:       a.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		Tools:       tools,
		ToolChoice:  toolChoice,
		// Only meaningful when tools are on the table — see the field's doc.
		OnToolCallProgress: a.onToolCallProgress,
	}
	profile := a.ResolveThinkProfile(opts.ThinkLevel)
	if effort := profile.ReasoningEffort(); effort != "" {
		req.Reasoning = &model.ReasoningConfig{Effort: effort}
	}
	// Some providers carry the thinking switch in a `thinking` block rather than
	// in an effort field. Which ones, and what the block should say, is the
	// capability table's business — this used to be a check for "deepseek" by
	// name here, so the second provider of that shape would have needed another
	// branch in this function.
	if a.provider != nil {
		if blockType, ok := model.ThinkingBlockType(a.provider.Name(), a.model, string(opts.ThinkLevel)); ok {
			req.Thinking = &model.ThinkingConfig{Type: blockType}
		}
	}
	return req
}

// HistoryChars is the conversation budget this agent measures itself against —
// the same number compactIfNeeded takes its 80% of.
//
// Exported so the turn executor can size its output backstop from it. Asking
// the agent is the point: the budget is one fact with one owner, and the two
// previous attempts to carry it somewhere else both went wrong the same way —
// desktop/app.go's contextWindowTokens has a comment counting three shipped
// bugs from "two facts with one name", and sizing the backstop off
// config.ModelContextTokens (an optional override, usually zero) made it four.
func (a *Agent) HistoryChars() int {
	if a == nil || a.context == nil {
		return 0
	}
	_, _, maxChars := a.context.UsageStats()
	return maxChars
}
