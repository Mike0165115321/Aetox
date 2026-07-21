package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolAdapter bridges one MCP tool to the skill.Tool interface so the existing
// dispatcher/tool-loop can call it with no changes. The model sees a namespaced
// name (server_tool) to avoid collisions across servers and with built-ins.
type toolAdapter struct {
	client *Client
	remote string          // tool name as the server knows it
	name   string          // namespaced name the model calls
	desc   string
	schema json.RawMessage // JSON schema for the model's tool definition
}

func newToolAdapter(c *Client, t *mcpsdk.Tool) *toolAdapter {
	schema, err := json.Marshal(t.InputSchema)
	if err != nil || len(schema) == 0 || string(schema) == "null" {
		// Every provider expects an object schema; a tool with no inputs still
		// needs a valid empty one.
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return &toolAdapter{
		client: c,
		remote: t.Name,
		name:   toolName(c.Name(), t.Name),
		desc:   t.Description,
		schema: schema,
	}
}

func (a *toolAdapter) Name() string        { return a.name }
func (a *toolAdapter) Description() string { return a.desc }

func (a *toolAdapter) ToolDefinition() model.ToolDefinition {
	desc := a.desc
	if desc == "" {
		desc = "MCP tool " + a.remote
	}
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        a.name,
			Description: desc,
			Parameters:  a.schema,
		},
	}
}

// ExecuteTool is the real entry point: forward args to the server and flatten
// the MCP result into skill.Output. A tool-level error (IsError) is returned as
// output+error so the model sees it and can self-correct, matching how the SDK
// documents CallToolResult.IsError.
func (a *toolAdapter) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	start := time.Now()
	res, err := a.client.CallTool(ctx, a.remote, args)
	if err != nil {
		return skill.Output{
			Name:       a.name,
			Command:    a.name,
			Success:    false,
			Stderr:     err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		}, err
	}

	text := contentText(res)
	if text == "" {
		text = "(no output)"
	}
	out := skill.Output{
		Name:       a.name,
		Command:    a.name,
		Content:    text,
		RawOutput:  text,
		Success:    !res.IsError,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if res.IsError {
		out.Stderr = text
		return out, fmt.Errorf("mcp tool %q returned an error", a.name)
	}
	return out, nil
}

// Execute satisfies skill.Skill for the slash-command path. MCP tools are
// model-invoked, so there are no positional args to forward here.
func (a *toolAdapter) Execute(ctx context.Context, _ skill.Input) (skill.Output, error) {
	return a.ExecuteTool(ctx, nil)
}

// SkillTools connects (lazily) and returns one skill.Tool per MCP tool the
// server exposes. A connect/enumeration failure returns the error; the caller
// treats that as "this server contributes no tools" and moves on.
func (c *Client) SkillTools(ctx context.Context) ([]skill.Tool, error) {
	tools, err := c.Tools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]skill.Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, newToolAdapter(c, t))
	}
	return out, nil
}

// toolName builds the namespaced, provider-safe tool name server_tool. Tool
// names must match ^[A-Za-z0-9_-]+$ for the model APIs, so any other rune
// becomes '_'.
func toolName(server, tool string) string {
	return sanitize(server) + "_" + sanitize(tool)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "mcp"
	}
	return b.String()
}

// contentText flattens an MCP result's content blocks to plain text. Non-text
// blocks (image/audio/resource) are noted by type rather than dropped silently,
// so the model at least knows something came back.
func contentText(res *mcpsdk.CallToolResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcpsdk.TextContent:
			parts = append(parts, v.Text)
		default:
			parts = append(parts, fmt.Sprintf("(%T content omitted)", c))
		}
	}
	return strings.Join(parts, "\n")
}
