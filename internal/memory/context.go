package memory

import (
	"strconv"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/model"
)

const (
	defaultMaxTurns = 80
	defaultMaxChars = 128000
)

type Context struct {
	messages []model.Message
	maxTurns int
	maxChars int
}

func NewContext(systemPrompt string, maxTurns, maxChars int) *Context {
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}
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
	c.messages = append(c.messages, model.Message{
		Role:       message.Role,
		Name:       strings.TrimSpace(message.Name),
		ToolCallID: strings.TrimSpace(message.ToolCallID),
		Content:    strings.TrimSpace(message.Content),
		ToolCalls:  message.ToolCalls,
	})

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
