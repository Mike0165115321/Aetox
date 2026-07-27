package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// Live end-to-end against the real provider, skipped unless AETOX_LIVE=1:
//
//	AETOX_LIVE=1 go test ./desktop/ -run TestLiveAllToolsAccepted -v -count=1
//
// The unit checks prove each tool definition is well-formed on its own. This
// proves the thing that actually matters at runtime: the provider accepts the
// whole batch the app really sends, and picks the right tool out of it. A
// schema a validator tolerates but an API rejects fails every tool in the turn,
// not just its own, so it is worth one real request to know.
func TestLiveAllToolsAccepted(t *testing.T) {
	key := liveDeepSeekKey(t)

	// Exactly the batch applyConfig assembles: every built-in plus the
	// workbench tools the desktop app adds.
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	app := &App{}
	for _, s := range app.workbenchSkills() {
		if err := registry.Register(s, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", s.Name(), err)
		}
	}
	defs := skill.NewDispatcher(registry).ToolDefinitions()
	if len(defs) < 20 {
		t.Fatalf("only %d tool definitions — the batch is not representative", len(defs))
	}
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Function.Name)
	}
	t.Logf("sending %d tools: %s", len(defs), strings.Join(names, ", "))

	p, err := model.NewProvider(model.ProviderOptions{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   key,
		Timeout:  5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	resp, err := p.Complete(context.Background(), model.Request{
		Model:     "deepseek-v4-flash",
		MaxTokens: 2000,
		Tools:     defs,
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: "You are a coding agent. Use tools. Do not explain."},
			{Role: model.RoleUser, Content: "Read the file notes.txt in the sandbox root."},
		},
	})
	if err != nil {
		t.Fatalf("provider rejected the tool batch: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Fatalf("no tool call chosen from %d tools; text was %q", len(defs), resp.Text)
	}
	call := resp.ToolCalls[0]
	t.Logf("model chose: %s(%s)", call.Function.Name, call.Function.Arguments)

	// It should reach for read — the point is not that it must, but that a tool
	// picked out of the full batch comes back dispatchable.
	if _, ok := registry.Get(call.Function.Name); !ok {
		t.Errorf("model called %q, which is not a registered skill", call.Function.Name)
	}
	if !json.Valid([]byte(call.Function.Arguments)) {
		t.Errorf("arguments are not valid JSON: %q", call.Function.Arguments)
	}
}
