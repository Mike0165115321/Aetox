package subagent

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/connect"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/oauth"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// flat collapses runs of whitespace so an assertion about a sentence survives
// the file being re-wrapped. These prompts are prose kept at ~80 columns, and a
// phrase that happens to straddle a line break is not a phrase that has been
// removed — a test that cannot tell the difference fails for editing, which
// teaches the next author to stop asserting on the words that matter.
func flat(s string) string { return strings.Join(strings.Fields(s), " ") }

// declares reports whether `want` is one of the things that would satisfy some
// entry in a `needs:` list — including as one branch of an alternation.
//
// Reads the list the way needs.go does rather than comparing whole strings,
// because "the agent asks for n8n" and "the agent asks for n8n and nothing else"
// are different claims and only the first one is true.
func declares(needs []string, want string) bool {
	for _, entry := range needs {
		for _, part := range strings.Split(entry, "|") {
			if strings.TrimSpace(part) == want {
				return true
			}
		}
	}
	return false
}

// toolsNamedIn finds the tool names a prompt tells the agent to call.
//
// Backticked identifiers only — prose about "the workflow list" is not an
// instruction, and a check that guessed from prose would fire on every rewrite.
//
// Then filtered against the tools this build actually has, which is the part
// that matters. A prompt about automation is full of snake_case that is not a
// tool — `input_transforms`, `parent_hash`, `flow_input` are fields of the thing
// being built — and a checker that could not tell those from a tool name would
// fail every time somebody documented a field properly. Asking the registry is
// the only way to know, and it is one call.
var toolNamePattern = regexp.MustCompile("`([a-z][a-z0-9]*(?:_[a-z0-9]+)+)`")

func toolsNamedIn(prompt string, known []string) []string {
	var out []string
	for _, m := range toolNamePattern.FindAllStringSubmatch(prompt, -1) {
		if slices.Contains(known, m[1]) && !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

// The automation agent is the room ระบบออโตเมชั่น opens into: clicking it starts
// a direct chat with this profile rather than with the assistant. Three things
// have to hold for that room to work at all, and each of them is silent when it
// breaks — the chat opens, the agent answers, and it simply cannot do the one
// thing it exists for.
func TestTheAutomationAgentIsReachableAndEquipped(t *testing.T) {
	isolate(t)

	p, ok := Load("automation")
	if !ok {
		t.Fatal("the automation agent did not load — the ระบบออโตเมชั่น room opens into nothing")
	}

	// 1. It has to sit in the office. A profile at any other desk still parses,
	//    still validates, still appears in List(), and is unreachable from every
	//    door a user could walk through.
	if p.Desk != mode.Office {
		t.Errorf("Desk = %q, want %q — an agent anywhere else cannot be chatted with", p.Desk, mode.Office)
	}
	if _, found := find(Chairs(mode.Office), "automation"); !found {
		t.Error("the automation agent is not on the office roster")
	}

	// 2. It has to carry the tools it was hired for. Without these it is a
	//    conversation about automation rather than an agent that builds one.
	for _, want := range []string{
		"n8n_workflow_list", "n8n_workflow_read",
		"n8n_workflow_create", "n8n_workflow_update", "n8n_workflow_activate",
	} {
		if !slices.Contains(p.Tools, want) {
			t.Errorf("missing %s — it cannot do the job it is named for: %v", want, p.Tools)
		}
	}

	// 2a. It is a full-rank worker (owner's call, 2026-08-10: "มันควรจะมียศ
	//     เท่าเมน เพราะงานมันก็ไม่ใช่เล็ก ๆ"): it shows its work in the browser and
	//     can act on the page, starts its own engine, and holds a real
	//     workstation — shell, a visible terminal, files, notes, memory.
	//
	//     `browser` is one name for four actions since the packing (desktop/
	//     browser_tool.go). A profile that names it whole gets all four, which
	//     is what this agent wants: pressing Execute on a manual test is work
	//     only the editor can do.
	for _, want := range []string{
		"browser",
		"n8n_server_start", "windmill_server_start",
		"shell", "desk_terminal", "write", "todo_write", "memory",
	} {
		if !slices.Contains(p.Tools, want) {
			t.Errorf("missing %s — the full-rank worker lost part of its workstation", want)
		}
	}

	// 2b. Every tool its own prompt tells it to call has to be in that list.
	//
	//     `tools:` is a whitelist, so an instruction naming a tool the profile
	//     does not carry is an instruction to do something impossible — the
	//     agent reads it, reaches for the tool, and finds nothing. It is the
	//     same fault the desks were fixed for on 2026-08-09 (ffb58f8, "โต๊ะที่ไม่
	//     มีเครื่องมือ ต้องไม่ถูกสั่งให้ใช้มัน") and it arrived here the moment the
	//     prompt started pointing at the per-engine skills.
	for _, named := range toolsNamedIn(p.Prompt, skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()}).Names()) {
		if !slices.Contains(p.Tools, named) {
			t.Errorf("the prompt tells it to use %s, which is not in its tools: %v", named, p.Tools)
		}
	}

	// 3. It has to DECLARE that it needs the connection. The declaration is not
	//    what grants it — placement is — but it is what makes an unplaced or
	//    unconnected n8n show up on the roster as a sentence with a fix, instead
	//    of the agent quietly having no tools and no explanation.
	//
	//     Written as an alternation now (`connection:n8n | mcp:windmill`), which
	//     is the shape this agent always wanted: it needs *an* engine, not that
	//     one. So the assertion is that n8n is among the things that satisfy the
	//     requirement — not that it IS the requirement, which would forbid the
	//     second engine all over again.
	if !declares(p.Needs, "connection:n8n") {
		t.Errorf("Needs = %v, want n8n among the things that satisfy it — an agent short of its engine must say so on the roster", p.Needs)
	}
}

// The prompt carries the n8n rules that cannot be discovered at runtime: there
// is no node-schema endpoint, connections are keyed by node NAME with a doubled
// array, and an update replaces the whole workflow. Each one is a way to produce
// a workflow that saves successfully and does not work, which is the failure
// mode this agent exists to avoid — so they are pinned here rather than left to
// survive the next edit by luck.
func TestTheAutomationPromptCarriesWhatTheAPICannotTell(t *testing.T) {
	isolate(t)

	p, ok := Load("automation")
	if !ok {
		t.Fatal("the automation agent did not load")
	}
	for _, must := range []string{
		"no endpoint that returns node types", // no schema discovery
		"n8n_workflow_read",                   // and what to do instead
		"keyed by node *name*",                // not id
		"replaces the whole workflow",         // no partial edit
		"cannot be created already running",   // create, then activate
	} {
		if !strings.Contains(flat(p.Prompt), must) {
			t.Errorf("the prompt no longer says %q — that rule is not discoverable at runtime", must)
		}
	}
}

// The agent belongs to no vendor. §92.3 settled that Aetox supports more than
// one automation engine "from the first line of code", because an integration
// shaped around one makes the second a rewrite — and an agent whose identity is
// one product is the same mistake in prose. n8n is a dialect it speaks, not who
// it is.
//
// The line this test draws: the description and the parts of the prompt that
// say what the agent IS must name no product. The dialect section is where a
// product name belongs, and adding Windmill should mean adding a section beside
// it rather than rewriting anything above.
func TestTheAutomationAgentBelongsToNoVendor(t *testing.T) {
	isolate(t)

	p, ok := Load("automation")
	if !ok {
		t.Fatal("the automation agent did not load")
	}
	if strings.Contains(strings.ToLower(p.Description), "n8n") {
		t.Errorf("description names one engine: %q — the room it opens is not n8n's room", p.Description)
	}

	// Everything before the first dialect heading is the job, and the job is the
	// same on every engine.
	identity, _, found := strings.Cut(flat(p.Prompt), "## Dialect:")
	if !found {
		t.Fatal("the prompt has no dialect section — engine-specific rules must be marked as such, or the next engine is a rewrite")
	}
	for _, claim := range []string{
		"whichever automation engine",  // it works on what you have
		"the same job on every engine", // and the craft does not change
		"Read it off your own tools",   // how it knows which engine, without guessing
	} {
		if !strings.Contains(identity, claim) {
			t.Errorf("the identity no longer says %q — neutrality is a property somebody has to restate", claim)
		}
	}
}

// The two dialect skills have to reach the agent, and each has to carry the
// node-level facts that neither the API nor the prompt can supply.
//
// Why this is a test and not a reading: the skills ship inside the binary, so
// nothing on disk proves they loaded, and the failure is silent in the way that
// matters — the agent still answers, still builds, and simply writes a node
// from memory. Both files exist to stop exactly that.
func TestTheAutomationAgentCarriesItsDialectSkills(t *testing.T) {
	isolate(t)

	p, ok := Load("automation")
	if !ok {
		t.Fatal("the automation agent did not load")
	}
	reg := FilterRegistry(skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()}), p, nil)

	// Each fact below is one that is accepted at write time and only found out
	// on a run — which is why it has to be written down somewhere rather than
	// discovered.
	for name, musts := range map[string][]string{
		"n8n-nodes": {
			"typeVersion", // the same node at two versions takes different fields
			"starts with", // a computed parameter without the leading = is a literal
			"index **1**", // the error port's place in connections
		},
		"windmill-steps": {
			"input_transforms",                  // Windmill's wiring is a map, not a line
			"^[ufg]",                            // the path rule the server enforces unhelpfully
			"deleting it and creating it again", // what destroys schedules
		},
	} {
		s, found := reg.Get(name)
		if !found {
			t.Errorf("%s did not reach the automation agent — it will write steps from memory", name)
			continue
		}
		out, err := s.Execute(context.Background(), nil)
		if err != nil {
			t.Errorf("%s could not be opened: %v", name, err)
			continue
		}
		for _, must := range musts {
			if !strings.Contains(flat(out.Content), must) {
				t.Errorf("%s no longer says %q — that is not discoverable at run time", name, must)
			}
		}
	}
}

// Windmill's workspace id is the one thing a model gets wrong by being
// reasonable: the interface shows a name, the API takes an id, and the two are
// routinely different. What comes back is a 404, which reads like a permissions
// problem — so the hour after it goes on checking the token, while the flow was
// being looked for in a workspace that was never there.
//
// Pinned because it is the costliest mistake per occurrence on this engine, and
// the only defence against it is a sentence in a document.
func TestTheWindmillSkillSendsTheAgentForTheWorkspaceID(t *testing.T) {
	isolate(t)

	p, ok := Load("automation")
	if !ok {
		t.Fatal("the automation agent did not load")
	}
	reg := FilterRegistry(skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()}), p, nil)
	s, found := reg.Get("windmill-steps")
	if !found {
		t.Fatal("windmill-steps did not load")
	}
	out, err := s.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("windmill-steps could not be opened: %v", err)
	}
	for _, must := range []string{
		"windmill_workspace_list",        // where the real id comes from
		"not the name shown in Windmill", // and what it is not
		"windmill_flow_read",             // read before you write, named as the tool that does it
	} {
		if !strings.Contains(flat(out.Content), must) {
			t.Errorf("windmill-steps no longer says %q — that mistake is silent until it is expensive", must)
		}
	}
}

// The wiring, end to end: the placement UseEngine writes (`agent:automation`)
// has to reach the one function that hands the agent its tools.
//
// This is the test the 2026-08-10 incident was missing. Every piece had a test
// and every piece passed — the engine chip read `agent:` placement, the needs
// gate read it, engine_pick_test fed it to connect.Allows by hand and got yes.
// What nothing walked was the pipe: FilterRegistry asked the desk's ceiling,
// the desk had never heard of `agent:automation`, and an agent every screen
// called equipped opened its toolbox to nothing. So this test makes the exact
// writes the app makes and then asks the exact function the dispatch asks.
func TestAConnectionPlacedOnTheAgentReachesItsTools(t *testing.T) {
	isolate(t)

	// The write UseEngine makes when the user picks n8n in the chat chip.
	if err := connect.SetTargets("n8n", []string{"agent:automation"}); err != nil {
		t.Fatalf("SetTargets: %v", err)
	}

	p, ok := Load("automation")
	if !ok {
		t.Fatal("the automation agent did not load")
	}
	// The ceiling a chair dispatch runs under: the office desk, whose own
	// connection list is all the old code ever asked. Placed on the agent,
	// the desk does not carry n8n — the exact state the bug lived in.
	ceiling := &mode.Mode{
		Name:        mode.Office,
		Connections: config.ConnectionsForDesk(mode.Office, connect.IDs()),
	}
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})

	// No account attached yet: placement alone must not grant. Widening past
	// the desk must not carry the tools around the RequiresAccount half of the
	// same gate — five tools in the block with no key behind them all fail.
	if _, found := FilterRegistry(parent, p, ceiling).Get("n8n_workflow_list"); found {
		t.Fatal("n8n tools handed out with no account attached — placement alone must not be a grant")
	}

	// The write Connect makes when the key is accepted.
	if err := oauth.Set("n8n", oauth.Credential{Type: "api", Key: "k", Account: "localhost:5678"}); err != nil {
		t.Fatalf("oauth.Set: %v", err)
	}
	reg := FilterRegistry(parent, p, ceiling)
	for _, want := range []string{
		"n8n_workflow_list", "n8n_workflow_read",
		"n8n_workflow_create", "n8n_workflow_update", "n8n_workflow_activate",
	} {
		if _, found := reg.Get(want); !found {
			t.Errorf("%s did not reach the agent — placed on it and connected, and the cut still asked only the desk", want)
		}
	}

	// And the tools must not leak past the account the user placed: the same
	// registry still refuses windmill's, whose connection nobody attached.
	if _, found := reg.Get("windmill_flow_create"); found {
		t.Error("windmill tools handed out through n8n's placement — the widening is reading the family, not the connection")
	}
}

// The Manual Trigger trap, pinned where the model meets it.
//
// 2026-08-10, from the debug log of a real session: the agent checked the
// server, read an existing workflow, built a test one and saved it — then
// called activate, got "has no trigger node" about a workflow that visibly had
// one, and reported the whole job as impossible. Every step but the last had
// worked. That is the shape of "it says it can't do anything" the owner kept
// hitting.
//
// Two places have to carry it, because they are read at different moments: the
// tool description is what the model sees at the instant it decides to call
// activate, and the skill is what it reads while building. A fact in only one
// of them is a fact the model meets half the time.
func TestTheManualTriggerTrapIsTaughtWhereItIsMet(t *testing.T) {
	isolate(t)

	reg := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	activate, found := reg.Get("n8n_workflow_activate")
	if !found {
		t.Fatal("n8n_workflow_activate is not in the registry")
	}
	tool, isTool := activate.(skill.Tool)
	if !isTool {
		t.Fatal("n8n_workflow_activate is not offered to the model as a tool")
	}
	desc := flat(tool.ToolDefinition().Function.Description)
	for _, must := range []string{
		"Manual Trigger is NOT one of those", // the node it will actually have
		"does not need to be",                // so the call is the mistake
		"not a fault in the workflow",        // and the refusal is not a failure
	} {
		if !strings.Contains(desc, must) {
			t.Errorf("the activate description no longer says %q — the model reads this at the moment it makes the mistake", must)
		}
	}

	p, ok := Load("automation")
	if !ok {
		t.Fatal("the automation agent did not load")
	}
	// And the prompt carries the lesson that outlives n8n: a refused step that
	// was never required is not a failed job. Written engine-neutrally on
	// purpose — the next engine will refuse different things for the same
	// reason, and a rule naming this one case would not travel.
	for _, must := range []string{
		"was that step actually required",
		"Reporting \"I could not do it\" over an unnecessary step",
	} {
		if !strings.Contains(flat(p.Prompt), must) {
			t.Errorf("the prompt no longer says %q — the agent goes back to calling a finished job impossible", must)
		}
	}

	// The skill spells out what to do instead, including the wrong repair.
	child := FilterRegistry(reg, p, nil)
	s, found := child.Get("n8n-nodes")
	if !found {
		t.Fatal("n8n-nodes did not reach the agent")
	}
	out, err := s.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("n8n-nodes could not be opened: %v", err)
	}
	for _, must := range []string{
		"still not an activatable trigger", // names the trap
		"finished the moment it saved",     // what the state actually is
		"adding a schedule nobody asked",   // and the repair that would be worse
	} {
		if !strings.Contains(flat(out.Content), must) {
			t.Errorf("n8n-nodes no longer says %q", must)
		}
	}
}
