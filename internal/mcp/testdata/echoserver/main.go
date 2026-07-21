// Command echoserver is a minimal stdio MCP server used by the internal/mcp
// integration test: one "echo" tool that returns its text argument, plus it
// writes the AETOX_TEST env var it was launched with into the reply so the test
// can assert environment merging. Not shipped — testdata only.
package main

import (
	"context"
	"os"

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
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "echo", Version: "1"}, nil)
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "echo", Description: "echoes text"}, echo)
	if err := s.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
