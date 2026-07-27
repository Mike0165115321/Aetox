// Command echoserver is a minimal stdio MCP server used by the internal/mcp
// integration test: one "echo" tool that returns its text argument, plus it
// writes the AETOX_TEST env var it was launched with into the reply so the test
// can assert environment merging. Not shipped — testdata only.
package main

import (
	"context"
	"os"
	"strconv"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoArgs struct {
	Text string `json:"text"`
}

func echo(_ context.Context, _ *mcpsdk.CallToolRequest, args echoArgs) (*mcpsdk.CallToolResult, any, error) {
	reply := args.Text
	if v := os.Getenv("AETOX_TEST"); v != "" {
		reply += "|" + v
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: reply}},
	}, nil, nil
}

func main() {
	// AETOX_TEST_DELAY_MS simulates a slow-to-start server (e.g. npx resolving
	// a package on a cold cache) — used by TestManagerRegisterRunsConcurrently
	// to prove Register connects clients in parallel, not one after another.
	if ms, err := strconv.Atoi(os.Getenv("AETOX_TEST_DELAY_MS")); err == nil && ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "echo", Version: "1"}, nil)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "echo", Description: "echoes text"}, echo)
	// One resource, so the resource bridge has something real to enumerate and
	// read. AETOX_TEST_NO_RESOURCES drops it, which is how the test proves the
	// resource tools are not registered for a server that has none.
	if os.Getenv("AETOX_TEST_NO_RESOURCES") == "" {
		s.AddResource(
			&mcpsdk.Resource{URI: "echo://greeting", Name: "greeting", Description: "a fixed greeting"},
			func(_ context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
				return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{
					{URI: req.Params.URI, MIMEType: "text/plain", Text: "hello from a resource"},
				}}, nil
			})
	}
	if err := s.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
