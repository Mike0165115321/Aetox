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
	remote string // tool name as the server knows it
	name   string // namespaced name the model calls
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

// ToolDefinition stamps the origin onto every bridged tool's description.
//
// Without it the model has no way to know an MCP tool is one. The name is
// `<server>_<tool>` and the description is whatever the remote server wrote
// about itself — neither says "MCP" anywhere, and the system prompt never
// mentions the protocol either. So a user asking "do you have MCP connected?"
// got "no" from an assistant that was holding the server's tools at that
// moment: it looked for something labelled MCP, found nothing, and answered
// honestly about what it could see. Measured on 2026-08-06 with
// sequential-thinking in the tool block; the model went on to hand-build a
// test client over stdio to prove MCP worked, having never noticed it was
// already holding the tool.
//
// One line, at the front, in the model's own working vocabulary. The remote's
// description follows unchanged — it is that server's own words about what the
// tool does, and this only says where it came from.
func (a *toolAdapter) ToolDefinition() model.ToolDefinition {
	desc := "From the MCP server \"" + a.client.Name() + "\"."
	if a.desc != "" {
		desc += " " + a.desc
	} else {
		desc += " Tool: " + a.remote + "."
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
	// Resources are the server's nouns, and a server that is a source of data
	// rather than a set of verbs looked empty without them. The pair is added
	// only when there is something to list — see resourceTools.
	out = append(out, resourceTools(ctx, c)...)
	return out, nil
}

// toolName builds the namespaced, provider-safe tool name server_tool. Tool
// names must match ^[A-Za-z0-9_-]+$ for the model APIs, so any other rune
// becomes '_'.
func toolName(server, tool string) string {
	return ToolPrefix(server) + sanitize(tool)
}

// ToolPrefix is what every tool bridged from server is named with. Exported
// because a caller holding only a tool name has to be able to ask which server
// it came from — internal/mode decides per-desk MCP visibility that way, and a
// second copy of this rule anywhere else is a rule that can disagree with the
// one that does the naming. Keep it the same string toolName builds from.
func ToolPrefix(server string) string {
	return sanitize(server) + "_"
}

// ToolBelongsTo reports whether a bridged tool name came from one of the named
// servers. The question "is this MCP tool mine?" is now asked from two places —
// a desk (mode.CarriesMCP) and an agent the user pointed a server at
// (subagent.FilterRegistry) — and both have to answer it the same way as
// toolName builds it. Two open-coded prefix loops would be two chances to
// disagree with the naming rule three functions up.
func ToolBelongsTo(tool string, servers []string) bool {
	tool = strings.ToLower(strings.TrimSpace(tool))
	for _, server := range servers {
		if strings.HasPrefix(tool, ToolPrefix(server)) {
			return true
		}
	}
	return false
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
