package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolName(t *testing.T) {
	cases := map[string][2]string{
		"fs_read_file":   {"fs", "read_file"},
		"my-server_echo": {"my-server", "echo"},
		"git_hub_list":   {"Git Hub", "list"}, // spaces + case → _ and lower
		"mcp_x":          {"", "x"},           // empty server falls back
		"a_b_c":          {"a.b", "c"},        // dot → _
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
	// One real tool plus the resource pair, which this server also offers.
	if len(tools) != 3 {
		t.Fatalf("got %d tools, want 3 (echo + the resource pair)", len(tools))
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

// The model has no other way to know a tool came from MCP. The name is
// `<server>_<tool>` and the description is the remote server's own words;
// neither says "MCP", and the system prompt never mentions the protocol.
//
// Measured 2026-08-06: asked whether MCP was connected, the assistant answered
// "no" while holding sequential-thinking's tool in its tool block, then
// hand-built a stdio client to prove MCP worked — never noticing it was already
// holding the tool. It was not confused; it was told nothing.
func TestBridgedToolSaysWhichServerItCameFrom(t *testing.T) {
	c := New(Server{Name: "sequential-thinking", Command: []string{"npx", "x"}})

	described := newToolAdapter(c, &mcpsdk.Tool{
		Name:        "sequentialthinking",
		Description: "A tool for reflective problem solving.",
	}).ToolDefinition()
	if !strings.Contains(described.Function.Description, "MCP server") {
		t.Errorf("nothing marks this as an MCP tool: %q", described.Function.Description)
	}
	if !strings.Contains(described.Function.Description, "sequential-thinking") {
		t.Errorf("the description does not name the server: %q", described.Function.Description)
	}
	// The server's own words survive: this says where the tool came from, not
	// what it does.
	if !strings.Contains(described.Function.Description, "reflective problem solving") {
		t.Errorf("the server's description was lost: %q", described.Function.Description)
	}

	// A server that documents nothing still produces a usable definition.
	bare := newToolAdapter(c, &mcpsdk.Tool{Name: "ping"}).ToolDefinition()
	if !strings.Contains(bare.Function.Description, "MCP server") || !strings.Contains(bare.Function.Description, "ping") {
		t.Errorf("a tool with no description lost its origin too: %q", bare.Function.Description)
	}
}

// An allowlist is written by a person reading a screen, and there are two
// screens: the server's own documentation, which calls it `echo`, and Aetox's
// tool list, which calls it `demo_echo`. Both have to work — a list copied from
// the more natural of the two places and silently selecting nothing is the
// failure this whole mechanism exists to avoid.
func TestAnAllowlistTakesOnlyWhatItNames(t *testing.T) {
	c := New(Server{Name: "demo", Tools: []string{"echo", "demo_search"}})

	keep, err := c.selected([]string{"echo", "search", "delete_everything"})
	if err != nil {
		t.Fatalf("selected: %v", err)
	}
	for _, tool := range []string{"echo", "search"} {
		if !keep[tool] {
			t.Errorf("%q was named in the allowlist and did not survive it", tool)
		}
	}
	if keep["delete_everything"] {
		t.Error("a tool nobody listed came through — an allowlist that leaks is a denial list")
	}
}

// No list is the case every server configured today is in, and it has to mean
// "take everything" rather than "take nothing".
func TestNoAllowlistTakesTheWholeServer(t *testing.T) {
	c := New(Server{Name: "demo"})
	keep, err := c.selected([]string{"anything_at_all"})
	if err != nil {
		t.Fatalf("selected: %v", err)
	}
	if keep != nil {
		t.Errorf("a server with no allowlist filtered its tools: %v", keep)
	}
}

// The collision is real and it is n8n-mcp's, which is the server this whole
// mechanism was built for: its own tool names begin with `n8n_`, so on a server
// the user named `n8n` one entry can mean a documentation tool by its bridged
// name or a workflow-writing tool by its raw one.
//
// Guessing would sometimes admit a write tool to a list written to exclude
// every one of them, and do it silently. So it refuses and says which two.
func TestAnAmbiguousEntryIsRefusedRatherThanGuessed(t *testing.T) {
	c := New(Server{Name: "n8n", Tools: []string{"n8n_validate_workflow"}})

	_, err := c.selected([]string{"validate_workflow", "n8n_validate_workflow"})
	if err == nil {
		t.Fatal("an entry naming both a docs tool and a write tool was resolved by guessing")
	}
	for _, want := range []string{"ambiguous", "validate_workflow"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not say %q, so the user cannot fix it: %v", want, err)
		}
	}
}

// The same two tools, on a server named anything else, are not ambiguous at
// all — so the refusal must not fire on the ordinary case.
func TestTheSameTwoToolsAreFineOnADifferentlyNamedServer(t *testing.T) {
	c := New(Server{Name: "n8n-docs", Tools: []string{"validate_workflow"}})

	keep, err := c.selected([]string{"validate_workflow", "n8n_validate_workflow"})
	if err != nil {
		t.Fatalf("selected: %v", err)
	}
	if !keep["validate_workflow"] || keep["n8n_validate_workflow"] {
		t.Errorf("kept = %v, want only the docs tool", keep)
	}
}
