package bootstrap

import (
	"context"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// approveNothing is a stand-in for a real approval gate. Tests must pass one:
// the whole point of ErrNoApprover is that nil is not a valid approver.
func approveNothing(context.Context, string, string) (bool, error) { return false, nil }

// testConfig is an engine that needs no network: the built-in aetox provider
// answers locally, so these tests exercise real wiring rather than mocks.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	return config.Config{
		ModelProvider: "aetox",
		ModelName:     "aetox",
		SandboxRoot:   t.TempDir(),
		ApprovalMode:  string(safety.ApprovalAsk),
	}
}

// A nil Approve must be refused rather than accepted. turn treats a nil
// approver as "approved", so accepting one here would silently run every tool
// without asking — on the hosts least likely to have a human watching.
func TestEngineRefusesANilApprover(t *testing.T) {
	res, err := Engine(testConfig(t), Options{})
	if err == nil {
		t.Fatal("Engine accepted a nil Approve; it must fail closed")
	}
	if res.App != nil || res.Agent != nil || res.Registry != nil {
		t.Errorf("Engine returned a usable engine alongside the error: %+v", res)
	}
}

// The three sub-agent tools must all be in the registry, and they must be there
// before the app snapshots its name list — a host that registered them later
// would silently omit them from the app's own skill and command sets.
func TestEngineRegistersEveryTaskTool(t *testing.T) {
	res, err := Engine(testConfig(t), Options{Approve: approveNothing})
	if err != nil {
		t.Fatalf("Engine: %v", err)
	}
	for _, name := range []string{"task", "task_result", "task_answer"} {
		if _, ok := res.Registry.Get(name); !ok {
			t.Errorf("%s missing from the registry", name)
		}
	}
	names := strings.Join(res.Dispatcher.Names(), " ")
	for _, name := range []string{"task", "task_result", "task_answer"} {
		if !strings.Contains(names, name) {
			t.Errorf("%s missing from the dispatcher's names: %s", name, names)
		}
	}
}

// Host-owned tools reach the registry tagged as their own source, so the UI can
// tell a workbench tool from a built-in one.
func TestEngineRegistersExtraSkills(t *testing.T) {
	res, err := Engine(testConfig(t), Options{
		Approve:     approveNothing,
		ExtraSkills: []skill.Skill{stubSkill{}},
	})
	if err != nil {
		t.Fatalf("Engine: %v", err)
	}
	if _, ok := res.Registry.Get("stub_workbench_tool"); !ok {
		t.Fatal("extra skill never reached the registry")
	}
}

// The MCP ask-rules must be installed synchronously and must come BEFORE the
// user's own rules, because the last matching rule wins: an MCP tool has to be
// gated from the first turn, while an explicit user choice still overrides it.
// Nothing tested PermissionRules' position before this.
func TestEnginePrependsMCPAskRules(t *testing.T) {
	mgr := mcp.NewManager([]mcp.Server{{Name: "demo", Command: []string{"nonexistent-binary"}}})
	want := mgr.PermissionRules()
	if len(want) == 0 {
		t.Skip("this manager produces no permission rules; nothing to order")
	}

	res, err := Engine(testConfig(t), Options{Approve: approveNothing, Manager: mgr})
	if err != nil {
		t.Fatalf("Engine: %v", err)
	}
	got := res.Permissions.Rules
	if len(got) < len(want) {
		t.Fatalf("engine has %d rules, fewer than the %d MCP rules alone", len(got), len(want))
	}
	for i, rule := range want {
		if got[i] != rule {
			t.Errorf("rule %d = %+v, want the MCP rule %+v — MCP rules must come first", i, got[i], rule)
		}
	}
}

// A nil Manager is the normal case for a host with no MCP configured, and must
// not be a crash: PermissionRules is called on it unconditionally.
func TestEngineToleratesANilMCPManager(t *testing.T) {
	if _, err := Engine(testConfig(t), Options{Approve: approveNothing, Manager: nil}); err != nil {
		t.Fatalf("Engine with no MCP manager: %v", err)
	}
}

// An unreachable provider is not a dead engine: it comes up on the built-in
// fallback, and Fallback is the only thing that says the caller did not get the
// provider they asked for. Distinct from Engine's error, which means no engine.
func TestEngineReportsAProviderFallbackWithoutFailing(t *testing.T) {
	cfg := testConfig(t)
	cfg.ModelProvider = "openai"
	cfg.ModelName = "gpt-4o"
	cfg.ModelAPIKey = "" // no key: provider init fails, aetox takes over

	res, err := Engine(cfg, Options{Approve: approveNothing})
	if err != nil {
		t.Fatalf("Engine should survive an unusable provider, got: %v", err)
	}
	if res.App == nil {
		t.Fatal("no app: the fallback provider should have produced a working engine")
	}
	if res.Fallback == nil {
		t.Error("Fallback is nil; nothing tells the caller they are not on the provider they asked for")
	}
	if !strings.Contains(res.Status, "fallback") {
		t.Errorf("Status = %q, expected it to mention the fallback", res.Status)
	}
}

// ContextChars is the one place the chars-per-token ratio lives. The main agent
// and every sub-agent read it, and they have to agree.
func TestContextCharsScalesWithTheConfiguredWindow(t *testing.T) {
	if got := ContextChars(config.Config{ModelContextTokens: 1000}); got != 4000 {
		t.Errorf("ContextChars(1000 tokens) = %d, want 4000", got)
	}
	// An unconfigured window falls through to the catalog, and a model the
	// catalog does not know yields 0. That 0 is the contract, not a bug: it
	// reaches cognitive.NewContext as "no explicit budget", which applies its
	// own 128k-char default. Pinned because the tempting "fix" — substituting a
	// number here — would silently override that default for every such model.
	if got := ContextChars(config.Config{ModelProvider: "aetox", ModelName: "aetox"}); got != 0 {
		t.Errorf("ContextChars for a model with no known window = %d, want 0 so NewContext applies its default", got)
	}
}

// The configured per-request timeout must actually be read. It was hardcoded to
// 30s in the old wiring while the setting sat unused, which a slow local model
// hits routinely.
func TestModelTimeoutHonoursTheConfiguredValue(t *testing.T) {
	if got := modelTimeout(config.Config{ModelTimeoutSec: 120}); got.Seconds() != 120 {
		t.Errorf("modelTimeout(120) = %v, want 2m", got)
	}
	if got := modelTimeout(config.Config{}); got.Seconds() != 30 {
		t.Errorf("modelTimeout(unset) = %v, want the 30s default", got)
	}
}

// DiscardConsole must fail a read rather than block or return an empty line: a
// host with no human attached should never reach a console read, and a silent
// empty answer would look like a real one.
func TestDiscardConsoleCannotRead(t *testing.T) {
	c := DiscardConsole()
	c.Print("swallowed")
	c.Printf("%s", "swallowed")
	c.Println("swallowed")
	c.Errorf("%s", "swallowed")
	if _, err := c.ReadLine(); err == nil {
		t.Error("DiscardConsole.ReadLine returned no error; a headless host must not get a fake answer")
	}
}

// --- helpers ----------------------------------------------------------------

type stubSkill struct{}

func (stubSkill) Name() string        { return "stub_workbench_tool" }
func (stubSkill) Description() string { return "a host-owned tool, for tests" }
func (stubSkill) Execute(context.Context, skill.Input) (skill.Output, error) {
	return skill.Output{}, nil
}
