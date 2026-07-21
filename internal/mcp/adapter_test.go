package mcp

import (
	"context"
	"testing"
	"time"
)

func TestToolName(t *testing.T) {
	cases := map[string][2]string{
		"fs_read_file":   {"fs", "read_file"},
		"my-server_echo": {"my-server", "echo"},
		"git_hub_list":   {"Git Hub", "list"},   // spaces + case → _ and lower
		"mcp_x":          {"", "x"},             // empty server falls back
		"a_b_c":          {"a.b", "c"},          // dot → _
	}
	for want, in := range cases {
		if got := toolName(in[0], in[1]); got != want {
			t.Errorf("toolName(%q,%q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

// Full bridge round-trip: build a real server, wrap its tool as a skill.Tool,
// and invoke it through the skill.Tool interface exactly as the dispatcher would.
func TestSkillToolBridge(t *testing.T) {
	bin := buildEchoServer(t)
	c := New(Server{Name: "echo srv", Command: []string{bin}, Timeout: 10 * time.Second})
	t.Cleanup(func() { c.Close() })

	tools, err := c.SkillTools(context.Background())
	if err != nil {
		t.Fatalf("SkillTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	tool := tools[0]
	if tool.Name() != "echo_srv_echo" {
		t.Fatalf("tool name = %q, want echo_srv_echo", tool.Name())
	}

	def := tool.ToolDefinition()
	if def.Function.Name != "echo_srv_echo" || len(def.Function.Parameters) == 0 {
		t.Fatalf("bad tool definition: %+v", def.Function)
	}

	out, err := tool.ExecuteTool(context.Background(), map[string]any{"text": "bridged"})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !out.Success || out.Content != "bridged" {
		t.Fatalf("output = %+v, want success with content %q", out, "bridged")
	}
}
