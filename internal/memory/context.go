package memory

import (
	"strconv"
	"strings"

	"github.com/Mikedev115/Aetox/internal/model"
)

const defaultMaxChars = 128000

type Context struct {
	messages []model.Message
	maxTurns int
	maxChars int

	// What the two compaction layers have reclaimed this session, kept for the
	// context meter: the sweep happens invisibly inside a turn, and a meter
	// that shows usage FALLING with no line saying why reads as a bug report
	// against itself. Reset with the Context, which is per-agent — a model
	// switch starts the count over, and that is honest: the new engine's
	// window never held the swept bytes.
	sweptItems int
	sweptChars int
	summaries  int
}

// NewContext builds conversation memory. maxTurns <= 0 means no
// message-count cap — like OpenCode/Claude Code, the char budget (scaled to
// the model's context window by callers) is the only real constraint.
func NewContext(systemPrompt string, maxTurns, maxChars int) *Context {
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}

	return &Context{
		messages: []model.Message{
			{
				Role:    model.RoleSystem,
				Content: systemPrompt,
			},
		},
		maxTurns: maxTurns,
		maxChars: maxChars,
	}
}

func (c *Context) Add(role model.MessageRole, content string) {
	if c == nil {
		return
	}
	c.AddMessage(model.Message{
		Role:    role,
		Content: strings.TrimSpace(content),
	})
}

func (c *Context) AddMessage(message model.Message) {
	if c == nil {
		return
	}
	// Copy the message, then normalise — rather than rebuilding it field by
	// field. The old form dropped anything it did not name, and its own comment
	// warned about exactly that: "an attached image silently vanishing between
	// the composer and the provider is a bug with no visible symptom — the model
	// just answers about a picture it never got."
	//
	// Documents was added later and fell into that trap. Every PDF a user
	// attached was discarded here, one call after cognitive.addUserTurn put it
	// on the message, so no provider ever received one and the model answered
	// from the filename. Assigning the struct is what stops the next field
	// repeating it.
	stored := message
	stored.Name = strings.TrimSpace(message.Name)
	stored.ToolCallID = strings.TrimSpace(message.ToolCallID)
	stored.Content = strings.TrimSpace(message.Content)
	c.messages = append(c.messages, stored)

	c.enforceLimits()
}

func (c *Context) Messages() []model.Message {
	if c == nil {
		return nil
	}
	return append([]model.Message(nil), c.messages...)
}

func (c *Context) Reset(systemPrompt string) {
	if c == nil {
		return
	}
	c.messages = []model.Message{
		{
			Role:    model.RoleSystem,
			Content: strings.TrimSpace(systemPrompt),
		},
	}
}

func (c *Context) CompactSummary() string {
	if c == nil {
		return ""
	}

	if len(c.messages) <= 3 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Context compacted to ")
	builder.WriteString(formatTurnCount(len(c.messages) - 1))
	builder.WriteString(" conversation turns.")
	return builder.String()
}

func (c *Context) enforceLimits() {
	if c == nil {
		return
	}

	if c.maxTurns > 0 && len(c.messages) > c.maxTurns {
		system := c.messages[0]
		trimmed := append([]model.Message{system}, c.messages[len(c.messages)-(c.maxTurns-1):]...)
		c.messages = trimmed
	}

	for c.maxChars > 0 && totalChars(c.messages) > c.maxChars && len(c.messages) > 2 {
		c.dropOldestTurn()
	}

	if len(c.messages) > 1 {
		c.truncateLastIfNeeded()
	}
}

func (c *Context) dropOldestTurn() {
	if len(c.messages) < 3 {
		return
	}
	// keep system + first user message; drop the oldest assistant+tool block
	// an assistant+tool block = assistant message + all following tool messages
	start := 2 // skip system (0) + user (1)
	if start >= len(c.messages) {
		return
	}
	// find the end of this assistant+tool group:
	// after the assistant, consume all tool messages
	end := start
	// first message in group must be assistant
	if c.messages[end].Role != model.RoleAssistant {
		// unexpected state, just drop one message
		c.messages = append([]model.Message{c.messages[0], c.messages[1]}, c.messages[end+1:]...)
		return
	}
	end++ // skip assistant
	for end < len(c.messages) && c.messages[end].Role == model.RoleTool {
		end++
	}
	system := c.messages[0]
	user := c.messages[1]
	rest := c.messages[end:]
	c.messages = append([]model.Message{system, user}, rest...)
}

func (c *Context) truncateLastIfNeeded() {
	if len(c.messages) <= 1 {
		return
	}
	excess := totalChars(c.messages) - c.maxChars
	if excess <= 0 {
		return
	}
	last := c.messages[len(c.messages)-1]
	if excess >= len(last.Content) {
		last.Content = ""
	} else {
		last.Content = last.Content[:len(last.Content)-excess]
	}
	c.messages[len(c.messages)-1] = last
}

func totalChars(messages []model.Message) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	return total
}

func (c *Context) UsageStats() (messageCount int, usedChars int, maxChars int) {
	if c == nil {
		return 0, 0, 0
	}
	return len(c.messages), totalChars(c.messages), c.maxChars
}

// NeedsCompaction reports whether usage has crossed the given fraction of
// the char budget — the trigger for summarizing compaction, which preserves
// meaning where the enforceLimits trim would drop whole turns.
func (c *Context) NeedsCompaction(threshold float64) bool {
	if c == nil || c.maxChars <= 0 || threshold <= 0 {
		return false
	}
	return float64(totalChars(c.messages)) > threshold*float64(c.maxChars)
}

// SplitForCompaction returns the span to summarize (everything between the
// system prompt and the kept tail) plus the index where the kept tail
// starts. The boundary always lands on a RoleUser message — the start of a
// turn — so an assistant message is never separated from its tool results.
// Returns nil, 0 when the conversation is too short to be worth compacting.
func (c *Context) SplitForCompaction(keepRecent int) ([]model.Message, int) {
	candidate := c.compactionBoundary(keepRecent)
	if candidate == 0 {
		return nil, 0
	}
	old := append([]model.Message(nil), c.messages[1:candidate]...)
	return old, candidate
}

// compactionBoundary is where "old enough to compact" ends and the kept tail
// begins — shared by the summarizing compaction and the micro sweep so the two
// layers cannot disagree about what "recent" means. Returns 0 when the
// conversation is too short to have an old part.
func (c *Context) compactionBoundary(keepRecent int) int {
	if c == nil || keepRecent < 1 || len(c.messages) < keepRecent+4 {
		return 0
	}
	candidate := len(c.messages) - keepRecent
	// prefer a clean turn boundary at or after the candidate...
	for candidate < len(c.messages) && c.messages[candidate].Role != model.RoleUser {
		candidate++
	}
	// ...or walk back toward the system prompt if the tail has no user turn
	if candidate >= len(c.messages) {
		candidate = len(c.messages) - keepRecent
		for candidate > 1 && c.messages[candidate].Role != model.RoleUser {
			candidate--
		}
	}
	if candidate <= 1 || candidate >= len(c.messages) || c.messages[candidate].Role != model.RoleUser {
		return 0
	}
	return candidate
}

// microSweepMinChars is the floor under which sweeping is not worth the stub:
// replacing a 200-char result with a 100-char marker saves nothing and still
// costs the model the original text.
const microSweepMinChars = 400

// MicroCompact clears the BODIES of old tool results whose output can simply
// be asked for again — the lighter layer under the summarizing compaction.
// Where compact() spends a model call to preserve meaning, this spends nothing
// to drop what never had meaning to preserve: a file read three turns ago is
// still on disk, and the 30KB copy of it riding every later request is the
// single biggest thing the token audit found in a context.
//
// The message itself stays — role, name and tool_call_id intact — because
// providers reject a tool call whose result went missing; only the content is
// replaced with a marker that says what was cleared and how to get it back.
// Everything at or past the compaction boundary is untouched: the recent tail
// is what the model is actively working from.
//
// One deliberate cost: the first request after a sweep breaks the provider's
// prompt cache from the earliest swept message onward. That is why the caller
// triggers this in one batch at a pressure threshold instead of sweeping each
// turn — pay the break once, get the freed context for every round after
// (docs/aider-study/EXECUTION.md ระดับ 3).
func (c *Context) MicroCompact(keepRecent int, sweepable map[string]bool) (swept int, freed int) {
	if c == nil || len(sweepable) == 0 {
		return 0, 0
	}
	// NOT compactionBoundary, and the difference is the whole point of having
	// two layers. The summarizer's boundary snaps to a user message because it
	// REMOVES a span, and a turn cut in half lies about itself. The sweep only
	// rewrites content in place — every message keeps its role, id and
	// position — so its one obligation is recency: leave the last keepRecent
	// messages alone. Snapping to a user turn here made the sweep a no-op for
	// exactly the conversation that needs it most, the single giant turn that
	// reads forty files before its first answer (found writing the trigger
	// test, one day before 1.5.15).
	boundary := len(c.messages) - keepRecent
	if boundary <= 1 {
		return 0, 0
	}
	for i := 1; i < boundary; i++ {
		m := c.messages[i]
		if m.Role != model.RoleTool || !sweepable[strings.ToLower(m.Name)] {
			continue
		}
		if len(m.Content) < microSweepMinChars || strings.HasPrefix(m.Content, sweptMarkerPrefix) {
			continue
		}
		freed += len(m.Content)
		c.messages[i].Content = sweptMarker(m.Name, len(m.Content))
		freed -= len(c.messages[i].Content)
		swept++
	}
	c.sweptItems += swept
	c.sweptChars += freed
	return swept, freed
}

// MaintenanceStats reports what the compaction layers have reclaimed so far:
// tool outputs swept (and the chars they gave back) and how many times the
// history was summarized. For the context meter — see the fields' comment.
func (c *Context) MaintenanceStats() (sweptItems, sweptChars, summaries int) {
	if c == nil {
		return 0, 0, 0
	}
	return c.sweptItems, c.sweptChars, c.summaries
}

// sweptMarkerPrefix also guards re-sweeping: a marker is smaller than
// microSweepMinChars today, but that is an accident of two constants and not a
// contract worth relying on.
const sweptMarkerPrefix = "[cleared to save context]"

func sweptMarker(tool string, chars int) string {
	return sweptMarkerPrefix + " " + tool + " output (" + strconv.Itoa(chars) +
		" chars) was removed from this old turn — run the tool again if it is still needed"
}

// ReplaceWithSummary swaps the summarized span for a single summary message,
// keeping the system prompt and the tail starting at recentStart.
func (c *Context) ReplaceWithSummary(summary string, recentStart int) {
	if c == nil || recentStart <= 1 || recentStart > len(c.messages) {
		return
	}
	summaryMessage := model.Message{
		Role:    model.RoleUser,
		Content: "[Compacted summary of the earlier conversation — treat as prior context]\n" + strings.TrimSpace(summary),
	}
	rebuilt := make([]model.Message, 0, 2+len(c.messages)-recentStart)
	rebuilt = append(rebuilt, c.messages[0], summaryMessage)
	rebuilt = append(rebuilt, c.messages[recentStart:]...)
	c.messages = rebuilt
	c.summaries++
}

func formatTurnCount(messages int) string {
	if messages <= 0 {
		return "0"
	}
	if messages == 1 {
		return "1"
	}
	turns := messages / 2
	if messages%2 != 0 {
		turns++
	}
	return strconv.Itoa(turns)
}
