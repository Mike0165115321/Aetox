package subagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// writeSkill drops one SKILL.md into a worker's own skills folder.
func writeSkill(t *testing.T, agent, name, description, body string) {
	t.Helper()
	dir, err := config.AgentSkillsPath(agent)
	if err != nil {
		t.Fatalf("skills path for %s: %v", agent, err)
	}
	dir = filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	doc := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(doc), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func agent(name string) Profile  { return Profile{Name: name, Desk: "specialized"} }
func helper(name string) Profile { return Profile{Name: name} }

// The claim the whole per-agent skills folder exists to make: a specialist
// document spec reaches the worker whose folder it is in, and nobody else.
// Written as one test because the three assertions are one sentence — a version
// of this that only checked the first would pass on the arrangement this
// replaced, where the shared shelf handed the same set to everyone.
func TestASkillReachesItsOwnWorkerAndNoOther(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	writeSkill(t, "doc", "tax-invoice", "the fields a full-form Thai tax invoice must carry", "8 required fields")

	if _, ok := FilterRegistry(parent, agent("doc"), nil).Get("tax-invoice"); !ok {
		t.Error("doc cannot reach a skill in its own folder")
	}
	if _, ok := FilterRegistry(parent, agent("deck"), nil).Get("tax-invoice"); ok {
		t.Error("doc's skill reached deck")
	}
	if _, ok := FilterRegistry(parent, helper("explore"), nil).Get("tax-invoice"); ok {
		t.Error("an agent's skill reached a helper")
	}
	// The parent registry is the main assistant's. It knows the office has a
	// document writer; it must not know what a tax invoice needs.
	if _, ok := parent.Get("tax-invoice"); ok {
		t.Error("an agent's skill leaked into the main assistant's registry")
	}
}

// The half that was missing: reaching it.
//
// Everything above asserts the skill is *registered* for the right worker, and
// all of it passed while the model could not see the skill at all. Progressive
// loading withholds every SourceSkill tool definition — that is what keeps a
// shelf of fifty from being charged to every request — and the two tools that
// exist to replace those definitions, skills_list and skill_view, scanned
// ~/.aetox/skills and nothing else. So a worker's own knowledge was loaded,
// executable by name, and listed behind no door the model could open: asked what
// it carried, `doc` answered with the machine's shared shelf and no tax-invoice
// on it (found 2026-08-16).
//
// Registry.Get is therefore not enough to prove this feature works, and this
// test asks the question the model asks.
func TestAWorkerCanOpenItsOwnShelfThroughTheSkillDoors(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	writeSkill(t, "doc", "payroll-columns", "the columns a payroll workbook must carry", "column A is the employee id")

	ctx := context.Background()
	shelfOf := func(p Profile) string {
		t.Helper()
		out, _, err := skill.NewDispatcher(FilterRegistry(parent, p, nil)).ExecuteTool(ctx, "skills_list", nil)
		if err != nil {
			t.Fatalf("skills_list for %s: %v", p.Name, err)
		}
		return out.Content
	}

	if listing := shelfOf(agent("doc")); !strings.Contains(listing, "payroll-columns") {
		t.Errorf("doc's own skill is not in the list it is told to read:\n%s", listing)
	}
	// A shipped skill is on the same shelf and has to arrive by the same door.
	if listing := shelfOf(agent("doc")); !strings.Contains(listing, "tax-invoice") {
		t.Errorf("doc's bundled skill is not listed:\n%s", listing)
	}
	if listing := shelfOf(agent("deck")); strings.Contains(listing, "payroll-columns") {
		t.Errorf("doc's shelf is visible to deck:\n%s", listing)
	}

	// L2: the body, by name, out of the worker's own folder.
	body, _, err := skill.NewDispatcher(FilterRegistry(parent, agent("doc"), nil)).
		ExecuteTool(ctx, "skill_view", map[string]any{"name": "payroll-columns"})
	if err != nil {
		t.Fatalf("skill_view: %v", err)
	}
	if !strings.Contains(body.Content, "column A is the employee id") {
		t.Errorf("skill_view did not return the worker's own document:\n%s", body.Content)
	}

	// And the thing that must NOT change: opening the door is not the same as
	// putting every skill back in the tool block, which is the cost progressive
	// loading exists to avoid.
	for _, def := range skill.NewDispatcher(FilterRegistry(parent, agent("doc"), nil)).ToolDefinitions() {
		if def.Function.Name == "payroll-columns" {
			t.Error("a skill is back in the tool block — progressive loading pays for the doors instead")
		}
	}
}

// Names are per-home, so two workers may each carry a skill called `invoice`
// and neither is the other's. On the shared shelf this is a collision one of
// them loses; here it is the normal case.
func TestTwoWorkersMayHoldTheSameSkillName(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	writeSkill(t, "doc", "invoice", "a document's required fields", "DOC-BODY")
	writeSkill(t, "sheet", "invoice", "a workbook's columns and formulas", "SHEET-BODY")

	docSkill, ok := FilterRegistry(parent, agent("doc"), nil).Get("invoice")
	if !ok {
		t.Fatal("doc lost its own invoice skill")
	}
	sheetSkill, ok := FilterRegistry(parent, agent("sheet"), nil).Get("invoice")
	if !ok {
		t.Fatal("sheet lost its own invoice skill")
	}
	if docSkill.Description() == sheetSkill.Description() {
		t.Errorf("both workers were handed the same invoice skill: %q", docSkill.Description())
	}
}

// The shared shelf's own skills are already in the parent registry, and the
// per-worker scan must not fold them in a second time — a scan that quietly
// added the shelf would hand every agent the same set, which is the one thing
// this folder exists to prevent.
func TestTheSharedShelfIsNotFoldedIntoAWorkersOwnScan(t *testing.T) {
	isolate(t)
	shelf, _ := skill.DiscoverSkills(nil) // bundled only
	own, _ := skill.DiscoverScoped(nil)   // nothing, and nothing borrowed
	if len(shelf) == 0 {
		t.Skip("no bundled skills ship, so there is nothing to leak")
	}
	if len(own) != 0 {
		t.Errorf("a scoped scan of nowhere returned %d skills", len(own))
	}
}

// A file the user is halfway through writing must not cost them the worker. The
// broken one is skipped and the rest of the folder still loads.
func TestAMalformedSkillDoesNotCostTheWorkerItsOthers(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	writeSkill(t, "doc", "quotation", "what a quotation must state", "body")
	broken, err := config.AgentSkillsPath("doc")
	if err != nil {
		t.Fatalf("skills path: %v", err)
	}
	broken = filepath.Join(broken, "half-written")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(broken, "SKILL.md"), []byte("---\nname: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, ok := FilterRegistry(parent, agent("doc"), nil).Get("quotation"); !ok {
		t.Error("one malformed file took the whole folder down with it")
	}
}

// A dropped file must not be able to take a built-in tool's name and answer in
// its place. The worker's tools were decided by the rules above the attach
// point; a folder cannot overrule them.
func TestASkillCannotImpersonateATool(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	writeSkill(t, "doc", "read", "pretending to be the file reader", "body")

	child := FilterRegistry(parent, agent("doc"), nil)
	got, ok := child.Get("read")
	if !ok {
		t.Fatal("the real read tool went missing")
	}
	if src, _ := child.SourceOf("read"); src != skill.SourceBuiltin {
		t.Errorf("read is now sourced %q — a dropped file took a built-in's name", src)
	}
	if got.Description() == "pretending to be the file reader" {
		t.Error("a skill answered in a built-in tool's place")
	}
}

// A worker with an empty home costs nothing and still runs. The shipped agents
// have no folder on disk at all until the user edits one, so this is the normal
// state rather than an edge.
func TestAWorkerWithNoSkillsFolderIsUnaffected(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})

	child := FilterRegistry(parent, agent("doc"), nil)
	if _, ok := child.Get("read"); !ok {
		t.Error("a worker with no skills folder lost its ordinary tools")
	}
}

// A shipped worker brings its knowledge with it: a fresh install with an empty
// data root still has doc's tax-invoice spec, and still does not give it to
// anybody else.
func TestAShippedWorkerBringsItsOwnSkills(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})

	if _, ok := FilterRegistry(parent, agent("doc"), nil).Get("tax-invoice"); !ok {
		t.Error("the bundled tax-invoice skill did not reach doc on a fresh install")
	}
	if _, ok := FilterRegistry(parent, agent("deck"), nil).Get("tax-invoice"); ok {
		t.Error("a skill shipped with doc reached deck")
	}
}

// Editing a shipped skill is copying it out and changing it — the rule AGENT.md
// already lives by, carried to the rest of the package. The user's file runs;
// the bundled one of the same name steps aside rather than colliding.
func TestAUsersOwnCopyShadowsTheShippedSkill(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	writeSkill(t, "doc", "tax-invoice", "our company's own invoice rules", "MINE")

	got, ok := FilterRegistry(parent, agent("doc"), nil).Get("tax-invoice")
	if !ok {
		t.Fatal("doc lost tax-invoice entirely")
	}
	if got.Description() != "our company's own invoice rules" {
		t.Errorf("the bundled copy won over the user's: %q", got.Description())
	}
}

// The shipped profiles carry an explicit `tools:` allowlist, and a worker's own
// skills must not be filtered by it — the allowlist names tools inherited from
// the parent, and a worker's knowledge is not inherited. Asserted against the
// profile that actually loads rather than a bare one, because a hand-built
// Profile{} has no allowlist and would pass this even if the wiring were wrong.
func TestAnAllowlistDoesNotSilenceAWorkersOwnKnowledge(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	doc, ok := Load("doc")
	if !ok {
		t.Fatal("the shipped doc profile did not load")
	}
	if len(doc.Tools) == 0 {
		t.Fatal("doc no longer has an allowlist — this test is checking nothing")
	}

	if _, got := FilterRegistry(parent, doc, nil).Get("tax-invoice"); !got {
		t.Errorf("doc's own tax-invoice skill was filtered out by its tools: %v", doc.Tools)
	}
}
