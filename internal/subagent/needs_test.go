package subagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

func needsRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	return root
}

// connectGitHub fakes an attached account by writing the credential the vault
// reads. The real Connect proves the token against GitHub first, which a test
// must not do.
func connectGitHub(t *testing.T) {
	t.Helper()
	writeCredential(t, `{"github":{"type":"api","key":"t","account":"mike"}}`)
}

func writeCredential(t *testing.T, body string) {
	t.Helper()
	root := os.Getenv("AETOX_DATA_ROOT")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "oauth.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write oauth.json: %v", err)
	}
}

func githubAgent(needs ...string) Profile {
	return Profile{Name: "github", Description: "GitHub", Needs: needs, Prompt: "You handle GitHub."}
}

// Every agent shipping today declares nothing, and must keep its prompt byte
// for byte — the prefix cache keys on the leading bytes.
func TestAnAgentThatNeedsNothingIsUnchanged(t *testing.T) {
	needsRoot(t)

	p := Profile{Name: "doc", Prompt: "You write one document."}
	if got := UnmetNeeds(p); len(got) != 0 {
		t.Fatalf("UnmetNeeds = %v, want none", got)
	}
	if got := PromptFor(p); got != p.Prompt {
		t.Fatalf("PromptFor changed a prompt with no needs:\n%q", got)
	}
}

func TestAnUnconnectedAccountIsReportedAsUnconnected(t *testing.T) {
	needsRoot(t)

	unmet := UnmetNeeds(githubAgent("connection:github"))
	if len(unmet) != 1 {
		t.Fatalf("unmet = %v, want one", unmet)
	}
	if unmet[0].Reason != ReasonUnconnected {
		t.Fatalf("reason = %q, want %q", unmet[0].Reason, ReasonUnconnected)
	}
	// Nothing the app can do on the user's behalf: it cannot connect an account
	// for them, so the roster must not offer a button that would lie.
	if unmet[0].Fixable() {
		t.Fatal("an unconnected account was reported as one click away")
	}
	if unmet[0].Label != "GitHub" {
		t.Fatalf("label = %q, want the service's display name", unmet[0].Label)
	}
}

// Connected but not switched on for this agent is the one unmet state that is a
// deliberate choice, and the only one a click can undo.
func TestConnectedButUnplacedIsFixable(t *testing.T) {
	needsRoot(t)
	connectGitHub(t)
	if err := config.SetConnectionTargets("github", []string{"coding"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}

	unmet := UnmetNeeds(githubAgent("connection:github"))
	if len(unmet) != 1 || unmet[0].Reason != ReasonUnplaced {
		t.Fatalf("unmet = %+v, want one unplaced", unmet)
	}
	if !unmet[0].Fixable() {
		t.Fatal("a placement Aetox can write was not reported as fixable")
	}
}

func TestConnectedAndPlacedIsMet(t *testing.T) {
	needsRoot(t)
	connectGitHub(t)
	if err := config.SetConnectionTargets("github", []string{config.MCPAgentPrefix + "github"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}

	if unmet := UnmetNeeds(githubAgent("connection:github")); len(unmet) != 0 {
		t.Fatalf("unmet = %+v, want none", unmet)
	}
}

func TestAMissingServerIsNotConfusedWithAnUnplacedOne(t *testing.T) {
	needsRoot(t)

	unmet := UnmetNeeds(githubAgent("mcp:github"))
	if len(unmet) != 1 || unmet[0].Reason != ReasonMissing {
		t.Fatalf("unmet = %+v, want one missing", unmet)
	}

	// Now it exists, switched on, pointed at nobody.
	if err := config.SaveMCPServers([]config.MCPServerConfig{
		{Name: "github", Command: []string{"docker", "run", "ghcr.io/github/github-mcp-server"}, For: []string{}},
	}); err != nil {
		t.Fatalf("SaveMCPServers: %v", err)
	}
	unmet = UnmetNeeds(githubAgent("mcp:github"))
	if len(unmet) != 1 || unmet[0].Reason != ReasonUnplaced {
		t.Fatalf("unmet = %+v, want one unplaced", unmet)
	}
	if !unmet[0].Fixable() {
		t.Fatal("an unplaced server was not reported as fixable")
	}
}

func TestADisabledServerSaysSoRatherThanLookingAbsent(t *testing.T) {
	needsRoot(t)
	if err := config.SaveMCPServers([]config.MCPServerConfig{
		{Name: "github", Command: []string{"x"}, Disabled: true, For: []string{config.MCPAgentPrefix + "github"}},
	}); err != nil {
		t.Fatalf("SaveMCPServers: %v", err)
	}

	unmet := UnmetNeeds(githubAgent("mcp:github"))
	if len(unmet) != 1 || unmet[0].Reason != ReasonDisabled {
		t.Fatalf("unmet = %+v, want one disabled", unmet)
	}
}

func TestServerPlacedForThisAgentIsMet(t *testing.T) {
	needsRoot(t)
	if err := config.SaveMCPServers([]config.MCPServerConfig{
		{Name: "github", Command: []string{"x"}, For: []string{config.MCPAgentPrefix + "github"}},
	}); err != nil {
		t.Fatalf("SaveMCPServers: %v", err)
	}

	if unmet := UnmetNeeds(githubAgent("mcp:github")); len(unmet) != 0 {
		t.Fatalf("unmet = %+v, want none", unmet)
	}
}

// A hand-written file with a typo must show up on the card. Dropping the line
// would leave the agent looking ready while missing the thing it named.
func TestAnUnknownNeedIsReportedRatherThanDropped(t *testing.T) {
	needsRoot(t)

	unmet := UnmetNeeds(githubAgent("conection:github", "connection:myspace"))
	if len(unmet) != 2 {
		t.Fatalf("unmet = %+v, want both reported", unmet)
	}
	for _, need := range unmet {
		if need.Reason != ReasonUnknown {
			t.Fatalf("need %+v, want reason %q", need, ReasonUnknown)
		}
		if need.Fixable() {
			t.Fatal("a need nothing recognises was offered a one-click fix")
		}
	}
}

func TestAnEntryWithNoIDIsIgnored(t *testing.T) {
	needsRoot(t)

	if unmet := UnmetNeeds(githubAgent("connection:", "  ")); len(unmet) != 0 {
		t.Fatalf("unmet = %+v, want the empty entries skipped", unmet)
	}
}

// The runtime half, and the shape of it matters more than the fact of it.
//
// Two earlier versions both wrote out a move: "say what is missing, then stop"
// (which turned a specialist into a door nobody could get through) and then its
// mirror, "ask in one line, then do the rest". The second read better and was
// the same kind of thing — a rule choosing for an agent that can choose. What it
// carries now is the fact and nothing else, because what an agent cannot work
// out for itself is that a server is unconnected and where it gets switched on;
// what to do about that is its own call (owner's call, 2026-08-16).
//
// So this test guards the fact arriving, the one standard that is not a move,
// and — the part that keeps the rule from growing back — the absence of
// choreography.
func TestTheNoticeCarriesTheFactAndNotTheMove(t *testing.T) {
	needsRoot(t)

	prompt := PromptFor(githubAgent("connection:github"))
	if !strings.Contains(prompt, "You handle GitHub.") {
		t.Fatal("the agent's own prompt was lost")
	}
	// What is missing, and where the user turns it on — the half an agent has no
	// way of knowing, and the half it needs to say anything useful about it.
	for _, want := range []string{"GitHub", "การเชื่อมต่อ"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the notice does not say what is missing — no %q", want)
		}
	}
	// A standard, not a procedure: the one outcome nobody downstream can catch.
	if !strings.Contains(prompt, "as though you had it") {
		t.Error("the notice no longer forbids answering as if the missing thing were there")
	}
	// Neither old rule may come back, in either direction. An agent that reads
	// this decides whether to ask first, work around it, or say it cannot be
	// done — and a sentence here that picks for it is the debt returning.
	for _, gone := range []string{"then stop", "ask for it", "does not need it", "Refusing work"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("the notice is telling the agent what to do again — %q is back", gone)
		}
	}
}

func TestTheNoticeGoesAwayOnceTheNeedIsMet(t *testing.T) {
	needsRoot(t)
	connectGitHub(t)
	if err := config.SetConnectionTargets("github", []string{config.MCPAgentPrefix + "github"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}

	p := githubAgent("connection:github")
	if got := PromptFor(p); got != p.Prompt {
		t.Fatalf("a satisfied agent still carries a notice:\n%q", got)
	}
}

// `needs:` declares; it never grants. An agent that names a connection must not
// thereby be able to use it — the switch is still the only thing that decides.
func TestDeclaringANeedGrantsNothing(t *testing.T) {
	needsRoot(t)
	connectGitHub(t)
	if err := config.SetConnectionTargets("github", []string{"coding"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}

	held := config.ConnectionsForAgent("github", []string{"github"})
	if len(held) != 0 {
		t.Fatalf("the agent holds %v having only declared a need for it", held)
	}
}

// ---------------------------------------------------------------------------
// The github agent — the first specialist that leans on things outside its own
// folder, and so the first customer of everything above.
// ---------------------------------------------------------------------------

func TestTheGitHubAgentShipsWhole(t *testing.T) {
	needsRoot(t)

	p, ok := Load("github")
	if !ok {
		t.Fatal("the github agent is not bundled")
	}
	// The office, like every other agent. It is what makes all three ways in
	// work at once — the roster lists it, the chat page's picker offers it, and
	// the assistant can hire it across the counter (COMPANY.md §4). A specialist
	// at any other desk is unreachable from all three while looking correct in
	// its own file.
	if p.Desk != "specialized" {
		t.Fatalf("desk = %q, want specialized", p.Desk)
	}
	// It must narrow. An agent that names no tools carries everything its desk
	// has, and this one also brings ~90 more from its server.
	if len(p.Tools) == 0 {
		t.Fatal("the github agent names no tools, so it carries the whole desk")
	}
	if len(p.Needs) != 2 {
		t.Fatalf("needs = %v, want the account and the server", p.Needs)
	}
	for _, want := range []string{"connection:github", "mcp:github"} {
		found := false
		for _, need := range p.Needs {
			if need == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("needs = %v, missing %q", p.Needs, want)
		}
	}
}

// Its expertise is documents in its own folder, not sentences in its prompt —
// which is the whole reason an agent has a skills/ directory. A skill on the
// shared shelf would make every agent a GitHub expert by the thirtieth day.
func TestTheGitHubAgentCarriesItsOwnKnowledge(t *testing.T) {
	needsRoot(t)

	p, ok := Load("github")
	if !ok {
		t.Fatal("the github agent is not bundled")
	}
	registry := skill.NewRegistry()
	attachOwnSkills(registry, p, nil)
	names := map[string]bool{}
	for name := range registry.Snapshot() {
		names[name] = true
	}
	for _, want := range []string{"repo-standards", "pr-workflow", "ci-triage", "issue-hygiene"} {
		if !names[want] {
			t.Fatalf("the github agent cannot reach its %q skill; it holds %v", want, names)
		}
	}
}

// engine is an agent that works on either of two automation engines — the shape
// §92.3 committed Aetox to, written as one requirement with two answers.
func engine(needs ...string) Profile {
	return Profile{Name: "automation", Description: "automation", Needs: needs, Prompt: "You build automations."}
}

// The bug this alternation exists to prevent, stated as a test: a user who runs
// Windmill and has never wanted n8n was told they were missing n8n. The
// declaration was true of the build and false of them, and a readiness warning
// that is false is worse than none — it teaches the user to ignore the roster.
func TestEitherEngineSatisfiesTheSameRequirement(t *testing.T) {
	needsRoot(t)
	if err := config.SaveMCPServers([]config.MCPServerConfig{
		{Name: "windmill", URL: "http://localhost:8000/api/mcp/w/main/mcp", For: []string{config.MCPAgentPrefix + "automation"}},
	}); err != nil {
		t.Fatalf("SaveMCPServers: %v", err)
	}

	if unmet := UnmetNeeds(engine("connection:n8n | mcp:windmill")); len(unmet) != 0 {
		t.Fatalf("unmet = %+v — Windmill answers this need, and n8n was never wanted", unmet)
	}
}

// With neither, the agent still has to say so — the alternation must not become
// a way for a requirement to disappear.
func TestWithNeitherEngineTheNeedIsStillReportedOnce(t *testing.T) {
	needsRoot(t)

	unmet := UnmetNeeds(engine("connection:n8n | mcp:windmill"))
	if len(unmet) != 1 {
		t.Fatalf("unmet = %+v, want exactly one — two lines read as two missing things", unmet)
	}
	if len(unmet[0].OneOf) != 1 {
		t.Fatalf("OneOf = %+v, want the other engine carried alongside", unmet[0].OneOf)
	}
	// And the notice has to say they are alternatives. Told them as a list, the
	// agent asks the user to set up both, and the second is one they may have
	// deliberately not chosen.
	notice := PromptFor(engine("connection:n8n | mcp:windmill"))
	if !strings.Contains(notice, "\n    or ") {
		t.Errorf("the notice does not mark the alternative as one:\n%s", notice)
	}
}

// Which of the two gets reported is not the order they were written in. An
// account already connected and one toggle from working must beat a product the
// user has never installed, or the fix offered is the expensive one.
func TestTheNearestAlternativeIsTheOneReported(t *testing.T) {
	needsRoot(t)
	// n8n: configured, switched on, pointed at nobody — one click away.
	if err := config.SaveMCPServers([]config.MCPServerConfig{
		{Name: "windmill", URL: "http://localhost:8000", For: []string{}},
	}); err != nil {
		t.Fatalf("SaveMCPServers: %v", err)
	}

	// Written windmill-last, and windmill is the one that is nearly there.
	unmet := UnmetNeeds(engine("mcp:absent-engine | mcp:windmill"))
	if len(unmet) != 1 {
		t.Fatalf("unmet = %+v, want one", unmet)
	}
	if unmet[0].ID != "windmill" || unmet[0].Reason != ReasonUnplaced {
		t.Fatalf("reported %q/%q, want windmill/unplaced — the reachable fix must be the one offered", unmet[0].ID, unmet[0].Reason)
	}
	if !unmet[0].Fixable() {
		t.Error("the one-click fix was not offered on a need that has one")
	}
}

// The seam, walked for this agent the way automation's was on 2026-08-10: the
// write the engine picker makes (the connection placed on `agent:github`), then
// the question dispatch asks (FilterRegistry under the office ceiling). The
// agent's own remit runs through its MCP server, but the built-in `github` tool
// is the half that works from the connection alone — and until 2026-08-10 the
// profile simply did not name it, so the GitHub agent could not read a
// repository without its server running. Every screen said connected; the
// toolbox disagreed.
func TestTheGitHubAgentReadsRepositoriesFromItsConnectionAlone(t *testing.T) {
	needsRoot(t)

	if err := config.SetConnectionTargets("github", []string{config.MCPAgentPrefix + "github"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}
	connectGitHub(t)

	p, ok := Load("github")
	if !ok {
		t.Fatal("the github agent is not bundled")
	}
	ceiling, _ := mode.Load(mode.Office)
	reg := FilterRegistry(skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()}), p, ceiling)
	if _, found := reg.Get("github"); !found {
		t.Error("the github tool did not reach the github agent — connected and placed on it, and it still cannot read a repository")
	}
}
