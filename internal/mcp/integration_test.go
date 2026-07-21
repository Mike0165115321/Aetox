package mcp

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// buildEchoServer compiles testdata/echoserver into a temp binary and returns
// its path. Building a real subprocess (rather than an in-memory transport)
// exercises the actual code path: exec, stdio framing, env merge, cleanup.
func buildEchoServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "echoserver")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", bin, "./testdata/echoserver").CombinedOutput()
	if err != nil {
		t.Fatalf("build echoserver: %v\n%s", err, out)
	}
	return bin
}

// Happy path: connect a real stdio server, list its tool, call it, and confirm
// env merging reaches the subprocess. Covers the ~35% the failure-path tests
// can't (successful ensure/Tools/CallTool).
func TestConnectListCall(t *testing.T) {
	bin := buildEchoServer(t)
	c := New(Server{
		Name:        "echo",
		Command:     []string{bin},
		Environment: map[string]string{"AETOX_TEST": "merged"},
		Timeout:     10 * time.Second,
	})
	t.Cleanup(func() { c.Close() })

	ctx := context.Background()
	tools, err := c.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if c.Status() != StatusConnected {
		t.Fatalf("status = %q, want %q", c.Status(), StatusConnected)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v, want one named echo", tools)
	}

	res, err := c.CallTool(ctx, "echo", map[string]any{"text": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	got := textOf(t, res)
	if got != "hi|merged" {
		t.Fatalf("echo result = %q, want %q (env not merged?)", got, "hi|merged")
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.Status() != StatusIdle {
		t.Fatalf("post-close status = %q, want %q", c.Status(), StatusIdle)
	}
}

func textOf(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *TextContent", res.Content[0])
	}
	return tc.Text
}
