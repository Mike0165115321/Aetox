package cognitive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/memory"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"
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

// maxDSMLNudges caps corrective retries when the model leaks tool-call markup
// as text (see model.ContainsLeakedDSML). Each retry is one extra round-trip,
// so keep it small; past the cap we stop rather than surface raw markup.
const maxDSMLNudges = 2

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
// ponytail: per-provider floors, not per-model — swap for a models.dev-style
// catalog when one provider's newer models need more than its floor.
func (a *Agent) toolLoopMaxTokens() int {
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
	maxToolCalls       int
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
	execTool func(context.Context, model.ToolCall) (string, error),
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
	a.context.Add(model.RoleUser, msg)

	// OpenCode-style loop: run until the model stops calling tools. The brakes
	// are the permission/approval layer, ctx cancellation (Ctrl+C in the CLI,
	// the Stop button in the desktop app), and the doom-loop guard below — not
	// an arbitrary round cap. MaxToolCalls > 0 opts back into a hard cap.
	maxToolCalls := a.maxToolCalls
	debuglog.Info("maxToolCalls", fmt.Sprintf("%d (<=0 means unlimited)", maxToolCalls))
	anyToolUsed := false
	nudgedEmpty := false
	dsmlNudges := 0
	var lastCallKey string
	repeatedCalls := 0
	loopMaxTokens := a.toolLoopMaxTokens()
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
		response, err := a.completeToolLoop(ctx, a.buildRequest(a.context.Messages(), loopMaxTokens, 0.2, modelTools, "auto", opts), onReasoningChunk)
		if err != nil {
			debuglog.Msg("Complete() error: %v", err)
			if i == 0 {
				// The user message is already in context — respond over it
				// rather than via Respond(msg), which would add it a second time.
				reply, err := a.respondFromContext(ctx, opts)
				return reply, false, err
			}
			return "", false, err
		}
		a.recordUsage(response.Usage)

		content := strings.TrimSpace(response.Text)
		debuglog.Info("response.text", truncateStr(content, 100))
		debuglog.Info("response.toolCalls", fmt.Sprintf("%d", len(response.ToolCalls)))
		if len(response.ToolCalls) == 0 {
			// DeepSeek can leak DSML tool-call markup as plain text, and the
			// backstop (openai_compatible.go) only lifts out COMPLETE blocks —
			// a block cut off before its closing tag falls through here as raw
			// markup with no ToolCalls, bypassing the truncation guard below.
			// Never accept that as the answer: tell the model to call the tool
			// for real and retry, capped so a persistent leaker can't spin.
			if model.ContainsLeakedDSML(content) {
				if dsmlNudges < maxDSMLNudges {
					dsmlNudges++
					debuglog.Msg("leaked DSML tool call, nudging (%d/%d)", dsmlNudges, maxDSMLNudges)
					a.context.Add(model.RoleUser, dsmlLeakNudge)
					continue
				}
				content = dsmlLeakFallback // gave up — don't surface raw markup
			}
			if content == "" {
				// Nudge inside the loop, not via recoverEmptyReply: here the
				// model keeps its tools, so it can cover a missing capability
				// (e.g. reading an image) by calling a skill instead of refusing.
				if !nudgedEmpty {
					nudgedEmpty = true
					debuglog.Msg("empty reply in tool loop, nudging once")
					a.context.Add(model.RoleUser, emptyReplyNudge)
					continue
				}
				content = emptyReplyFallback
			}
			a.context.AddMessage(model.Message{
				Role:             model.RoleAssistant,
				Content:          content,
				ReasoningContent: strings.TrimSpace(response.ReasoningContent),
			})
			return content, anyToolUsed, nil
		}
		anyToolUsed = true

		a.context.AddMessage(model.Message{
			Role:             model.RoleAssistant,
			Content:          content,
			ReasoningContent: strings.TrimSpace(response.ReasoningContent),
			ToolCalls:        response.ToolCalls,
		})

		// Truncation guard (same failure OpenCode hit, sst/opencode#18108):
		// finish_reason "length" means the tool-call JSON was cut off at
		// MaxTokens. Executing it would fail with a misleading parse/path
		// error the model then "fixes" forever. Tell it the truth instead.
		if response.FinishReason == model.FinishReasonLength {
			debuglog.Msg("tool call truncated at max_tokens, telling the model")
			for _, toolCall := range response.ToolCalls {
				a.context.AddMessage(model.Message{
					Role:       model.RoleTool,
					Name:       toolCall.Function.Name,
					ToolCallID: toolCall.ID,
					Content: fmt.Sprintf(
						"tool call NOT executed: your %s arguments were truncated at the %d-token output limit. Produce a shorter version, or split the work into several smaller tool calls (e.g. write a skeleton file first, then extend it with edit).",
						toolCall.Function.Name, loopMaxTokens),
				})
			}
			continue
		}

		for _, toolCall := range response.ToolCalls {
			// Doom-loop guard (mirrors OpenCode session/prompt.ts): identical
			// (name, args) calls back to back — nudge at 3, stop at 5.
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

			debuglog.Msg("tool call: %s(%s)", toolCall.Function.Name, truncateStr(toolCall.Function.Arguments, 80))
			callOutput, toolErr := a.executeToolCall(ctx, toolCall, execTool)
			callOutput = strings.TrimSpace(callOutput)
			if callOutput == "" {
				if toolErr != nil {
					callOutput = toolErr.Error()
				} else {
					callOutput = "(no output)"
				}
			}
			if repeatedCalls == doomLoopWarn {
				callOutput += "\n[loop warning] You have now made this exact tool call " + fmt.Sprint(repeatedCalls) + " times in a row with the same result. Try a different approach instead of repeating it."
			}
			debuglog.Msg("tool result: %s (err=%v)", truncateStr(callOutput, 120), toolErr)
			a.context.AddMessage(model.Message{
				Role:       model.RoleTool,
				Name:       toolCall.Function.Name,
				ToolCallID: toolCall.ID,
				Content:    callOutput,
			})
			if toolErr != nil && ctx.Err() != nil {
				return callOutput, true, ctx.Err()
			}
		}
	}

	return ToolLoopExhausted, anyToolUsed, nil
}

// completeToolLoop runs one tool-loop request. When a reasoning handler is
// present and the provider can stream, it uses StreamComplete so the model's
// thinking reaches the UI live (the whole non-streaming call would otherwise
// show nothing until it finishes). Content is NOT streamed live — a nil content
// handler keeps any leaked DSML tool-call markup off-screen and lets the
// executor deliver the final text once at the end; StreamComplete still
// accumulates and parses that content. Non-streaming providers, or turns with
// no reasoning handler (e.g. the CLI), keep using Complete unchanged.
func (a *Agent) completeToolLoop(ctx context.Context, req model.Request, onReasoningChunk func(string) error) (model.Response, error) {
	if onReasoningChunk != nil {
		if streamer, ok := a.provider.(model.StreamingProvider); ok {
			return streamer.StreamComplete(ctx, req, nil, onReasoningChunk)
		}
	}
	return a.provider.Complete(ctx, req)
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
	if !a.context.NeedsCompaction(compactThresholdFraction) {
		return
	}
	old, recentStart := a.context.SplitForCompaction(compactKeepRecent)
	if len(old) == 0 {
		return
	}
	defer debuglog.Block(fmt.Sprintf("Agent.compact (%d msgs)", len(old)))()
	response, err := a.provider.Complete(ctx, model.Request{
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
		return
	}
	a.context.ReplaceWithSummary(summary, recentStart)
	debuglog.Info("compacted", fmt.Sprintf("%d old messages -> %d summary chars", len(old), len(summary)))
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

func (a *Agent) executeToolCall(ctx context.Context, toolCall model.ToolCall, execTool func(context.Context, model.ToolCall) (string, error)) (string, error) {
	if strings.TrimSpace(toolCall.Function.Name) == "" {
		return "tool-call-missing-name", errors.New("tool call missing function name")
	}

	output, err := execTool(ctx, toolCall)
	if err != nil {
		return output, err
	}
	return output, nil
}

// recoverEmptyReply nudges the model once after an empty reply. The nudge is
// ephemeral — never stored in context — so history stays clean either way.
func (a *Agent) recoverEmptyReply(ctx context.Context, opts turn.TurnOptions) string {
	msgs := append(a.context.Messages(), model.Message{Role: model.RoleUser, Content: emptyReplyNudge})
	response, err := a.provider.Complete(ctx, a.buildRequest(msgs, 768, 0.2, nil, "", opts))
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
	a.context.Add(model.RoleUser, msg)
	return a.respondFromContext(ctx, opts)
}

// respondFromContext completes over the context as-is — the user message must
// already be in history. Shared by Respond and the tool-loop fallback so the
// fallback can't add the user message a second time.
func (a *Agent) respondFromContext(ctx context.Context, opts turn.TurnOptions) (string, error) {
	response, err := a.provider.Complete(ctx, a.buildRequest(a.context.Messages(), a.toolLoopMaxTokens(), 0.2, nil, "", opts))
	if err != nil {
		return "", err
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
	response, err := a.provider.Complete(ctx, a.buildRequest(msgs, 768, 0.2, nil, "", opts))
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
		response, err := streamer.StreamComplete(ctx, req, onChunk, onReasoningChunk)
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

	response, err := a.provider.Complete(ctx, req)
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
	if a.provider != nil && model.NormalizeProvider(a.provider.Name()) == "deepseek" {
		switch think.NormalizeLevel(string(opts.ThinkLevel)) {
		case think.LevelNoThinking:
			req.Thinking = &model.ThinkingConfig{Type: "disabled"}
		default:
			req.Thinking = &model.ThinkingConfig{Type: "enabled"}
		}
	}
	return req
}
