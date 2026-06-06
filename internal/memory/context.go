package memory

import (
	"strings"
	"strconv"

	"aetox-cli/internal/model"
)

const (
	defaultMaxTurns = 40
	defaultMaxChars = 12000
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
	c.messages = append(c.messages, model.Message{
		Role:    role,
		Content: strings.TrimSpace(content),
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
		// Keep system prompt and remove oldest user/assistant turn
		c.messages = append([]model.Message{c.messages[0]}, c.messages[2:]...)
	}

	if totalChars(c.messages) > c.maxChars && len(c.messages) > 1 {
		last := c.messages[len(c.messages)-1]
		excess := totalChars(c.messages) - c.maxChars
		if excess < 0 {
			excess = 0
		}
		if excess >= len(last.Content) {
			last.Content = ""
		} else {
			trimAt := len(last.Content) - excess
			if trimAt < 0 {
				trimAt = 0
			}
			last.Content = last.Content[:trimAt]
		}
		c.messages[len(c.messages)-1] = last
	}
}

func totalChars(messages []model.Message) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	return total
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
