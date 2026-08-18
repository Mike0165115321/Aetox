package bootstrap

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// The foundation the per-session engine work stands on: this process can hold
// more than one engine at a time, and two of them do not share a memory.
//
// Aetox has always built exactly one and rebuilt it in place, which is why
// every door out of a running turn's chat had to refuse — a second
// conversation meant overwriting the first one's mind mid-thought. Before any
// of that is rewired, the question worth answering first is whether Engine can
// simply be called twice: if it carries hidden process-wide state, the plan
// changes shape, and finding that out after the refactor would be finding it
// out the expensive way.
//
// The MCP manager is deliberately the SAME pointer in both. That is the split
// the design depends on: what is expensive (servers, connections) is shared,
// what is the conversation (the agent and its context) is not.
func TestTwoEnginesCoexistWithSeparateMemories(t *testing.T) {
	cfg := testConfig(t)
	manager := mcp.NewManager(nil)

	first, err := Engine(cfg, Options{Approve: approveNothing, Manager: manager})
	if err != nil {
		t.Fatalf("first engine: %v", err)
	}
	second, err := Engine(cfg, Options{Approve: approveNothing, Manager: manager})
	if err != nil {
		t.Fatalf("second engine: %v", err)
	}

	if first.Agent == nil || second.Agent == nil {
		t.Fatal("an engine came back without an agent")
	}
	if first.Agent == second.Agent {
		t.Fatal("both engines share one agent — there would be one memory between two conversations")
	}
	if first.App == second.App {
		t.Fatal("both engines share one app")
	}
	// Registries are separate objects (each Engine builds and fills its own),
	// which is what lets a session at another desk carry different tools.
	if first.Registry == second.Registry {
		t.Error("both engines share one registry; a per-desk cut would leak between sessions")
	}

	// The memory is the thing that must not be shared. Say something to one.
	first.Agent.RestoreHistory([]model.Message{
		{Role: model.RoleUser, Content: "ไล่บั๊คให้หน่อย"},
		{Role: model.RoleAssistant, Content: "เจอแล้วครับ"},
	})

	if !contextMentions(first.Agent.ContextMessages(), "ไล่บั๊คให้หน่อย") {
		t.Fatal("the first engine did not keep what it was told")
	}
	if contextMentions(second.Agent.ContextMessages(), "ไล่บั๊คให้หน่อย") {
		t.Error("the second engine can read the first one's conversation — the memories are not separate")
	}
}

func contextMentions(msgs []model.Message, want string) bool {
	for _, m := range msgs {
		if strings.Contains(m.Content, want) {
			return true
		}
	}
	return false
}
