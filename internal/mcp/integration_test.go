package mcp

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/skill"

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
		// No Timeout override: the production default (30s) is the one worth
		// exercising, and a tighter one only made this flaky. Connecting takes
		// ~2s alone but spawns a subprocess and handshakes over stdio, and
		// `go test ./...` runs this alongside packages that saturate the
		// machine — under that load 10s was not enough and the whole suite
		// went red for no defect.
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

// Resources are the server's nouns — a document, a record, a config it is the
// authority on. A server that is a source of data rather than a set of verbs
// looked empty before these were bridged (ARCHITECTURE.md §55).
func TestResourceToolsBridgeAServersResources(t *testing.T) {
	bin := buildEchoServer(t)
	c := New(Server{Name: "echo", Command: []string{bin}})
	t.Cleanup(func() { c.Close() })
	ctx := context.Background()

	tools, err := c.SkillTools(ctx)
	if err != nil {
		t.Fatalf("SkillTools: %v", err)
	}
	byName := map[string]skill.Tool{}
	for _, tool := range tools {
		byName[tool.Name()] = tool
	}
	list, hasList := byName["echo_resources"]
	read, hasRead := byName["echo_resource"]
	if !hasList || !hasRead {
		t.Fatalf("resource tools missing; got %v", keysOf(byName))
	}

	listed, err := list.ExecuteTool(ctx, nil)
	if err != nil {
		t.Fatalf("echo_resources: %v", err)
	}
	if !strings.Contains(listed.Content, "echo://greeting") {
		t.Errorf("the listing does not name the resource:\n%s", listed.Content)
	}

	got, err := read.ExecuteTool(ctx, map[string]any{"uri": "echo://greeting"})
	if err != nil {
		t.Fatalf("echo_resource: %v", err)
	}
	if !strings.Contains(got.Content, "hello from a resource") {
		t.Errorf("the resource body did not come back:\n%s", got.Content)
	}
}

// A server with no resources must not gain two tools that always answer
// "none" — they would be paid for in the tool block of every single request.
func TestNoResourceToolsForAServerWithoutResources(t *testing.T) {
	bin := buildEchoServer(t)
	c := New(Server{
		Name:        "echo",
		Command:     []string{bin},
		Environment: map[string]string{"AETOX_TEST_NO_RESOURCES": "1"},
	})
	t.Cleanup(func() { c.Close() })

	tools, err := c.SkillTools(context.Background())
	if err != nil {
		t.Fatalf("SkillTools: %v", err)
	}
	for _, tool := range tools {
		if strings.HasPrefix(tool.Name(), "echo_resource") {
			t.Errorf("%s was registered for a server with no resources", tool.Name())
		}
	}
}

func keysOf(m map[string]skill.Tool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
