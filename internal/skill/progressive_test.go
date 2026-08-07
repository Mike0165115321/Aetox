package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The whole point of progressive loading in one test: a discovered skill is
// still dispatchable, but its definition no longer rides in the tool block —
// only the two flat entries do. Before this, fifty installed skills meant
// fifty schemas in the head of every request, relevant or not.
func TestDiscoveredSkillsStayOutOfTheToolBlock(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&markdownSkill{
		name:        "deploy_notes",
		description: "How we deploy",
		body:        "step one",
	}, SourceSkill); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := registry.Register(&echoSkill{}, SourceBuiltin); err != nil {
		t.Fatalf("register echo: %v", err)
	}

	dispatcher := NewDispatcher(registry)

	for _, def := range dispatcher.ToolDefinitions() {
		if def.Function.Name == "deploy_notes" {
			t.Fatalf("discovered skill leaked into the tool definitions")
		}
	}

	// Still registered: the user's /deploy_notes and the settings page both
	// resolve it, and a name collision with a new skill is still caught.
	if _, ok := registry.Get("deploy_notes"); !ok {
		t.Fatalf("skill fell out of the registry entirely; only its definition should be withheld")
	}
	out, handled, err := dispatcher.ExecuteTool(context.Background(), "deploy_notes", nil)
	if err != nil || !handled {
		t.Fatalf("ExecuteTool = %v, %v; a withheld definition must not break execution", handled, err)
	}
	if !strings.Contains(out.Content, "step one") {
		t.Fatalf("skill body missing from output: %q", out.Content)
	}
}

func TestSkillsListReportsNameAndDescription(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "deploy", "---\nname: deploy_notes\ndescription: How we deploy\n---\nstep one\n")
	writeSkillFixture(t, root, "invoice", "---\nname: invoice_filing\ndescription: Where receipts go\n---\nsort by month\n")

	list := &skillsListSkill{paths: []string{root}}
	out, err := list.ExecuteTool(context.Background(), nil)
	if err != nil {
		t.Fatalf("skills_list: %v", err)
	}
	if !containsAll(out.Content, "deploy_notes — How we deploy", "invoice_filing — Where receipts go") {
		t.Fatalf("listing missing entries: %q", out.Content)
	}
	// The listing is L0 — names and descriptions only. A body leaking in here
	// means every skill's full text is paid for on every list call.
	if strings.Contains(out.Content, "step one") || strings.Contains(out.Content, "sort by month") {
		t.Fatalf("skill body leaked into the listing: %q", out.Content)
	}
}

// This used to assert "No skills installed" on a fresh machine, and that is no
// longer reachable: the bundled skills (bundled_skills.go) ship inside the
// binary, so the library is never empty. What is worth pinning instead is that
// a user who has installed nothing still gets a real listing with the reading
// instruction on the end — the state a first-run session is actually in.
func TestSkillsListOnAFreshMachineListsTheBundledSkills(t *testing.T) {
	list := &skillsListSkill{paths: []string{t.TempDir()}}
	out, err := list.ExecuteTool(context.Background(), nil)
	if err != nil {
		t.Fatalf("skills_list on empty dir: %v", err)
	}
	if strings.Contains(out.Content, "No skills installed") {
		t.Fatalf("the bundled skills should always be listed: %q", out.Content)
	}
	for _, b := range bundledSkills() {
		if !strings.Contains(out.Content, b.Name) {
			t.Errorf("bundled skill %q is missing from skills_list: %q", b.Name, out.Content)
		}
	}
	if !strings.Contains(out.Content, "skill_view") {
		t.Errorf("listing does not say how to open one: %q", out.Content)
	}
}

func TestSkillViewReturnsBody(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "deploy", "---\nname: deploy_notes\ndescription: How we deploy\n---\nstep one\nstep two\n")

	view := &skillViewSkill{paths: []string{root}}
	out, err := view.ExecuteTool(context.Background(), map[string]any{"name": "deploy_notes"})
	if err != nil {
		t.Fatalf("skill_view: %v", err)
	}
	if !containsAll(out.Content, "step one", "step two") {
		t.Fatalf("body missing: %q", out.Content)
	}
}

// The wrong-name error carries the real list so the model recovers in zero
// extra rounds instead of one.
func TestSkillViewUnknownNameNamesTheAlternatives(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "deploy", "---\nname: deploy_notes\ndescription: d\n---\nbody\n")

	view := &skillViewSkill{paths: []string{root}}
	_, err := view.ExecuteTool(context.Background(), map[string]any{"name": "deploy_nots"})
	if err == nil {
		t.Fatalf("want an error for an unknown skill name")
	}
	if !strings.Contains(err.Error(), "deploy_notes") {
		t.Fatalf("error should name the installed skills: %v", err)
	}
}

// A skill installed mid-session (plugin_install writes the directory) must be
// visible to the very next skills_list call — no re-bootstrap in between.
func TestSkillsListSeesSkillsInstalledAfterRegistryBuild(t *testing.T) {
	root := t.TempDir()
	list := &skillsListSkill{paths: []string{root}}

	out, err := list.ExecuteTool(context.Background(), nil)
	if err != nil || strings.Contains(out.Content, "fresh_skill") {
		t.Fatalf("precondition: fresh_skill should not exist yet, got %q (%v)", out.Content, err)
	}

	writeSkillFixture(t, root, "fresh", "---\nname: fresh_skill\ndescription: just installed\n---\nbody\n")

	out, err = list.ExecuteTool(context.Background(), nil)
	if err != nil {
		t.Fatalf("skills_list after install: %v", err)
	}
	if !strings.Contains(out.Content, "fresh_skill") {
		t.Fatalf("freshly installed skill invisible without a re-bootstrap: %q", out.Content)
	}
}

// writeSkillFile puts a supporting file inside an existing skill fixture.
func writeSkillFile(t *testing.T, root, dirName, rel, content string) {
	t.Helper()
	path := filepath.Join(root, dirName, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// L2. Without it, a skill too long for one file is a dead end: the body can
// name its references, but nothing can open them — and a skill written from
// real work splits into files precisely because it outgrew one.
func TestSkillViewReadsASupportingFile(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "deploy", "---\nname: deploy_notes\ndescription: d\n---\nsee references/rollback.md\n")
	writeSkillFile(t, root, "deploy", "references/rollback.md", "step one: stop the service")

	view := &skillViewSkill{paths: []string{root}}
	out, err := view.ExecuteTool(context.Background(), map[string]any{
		"name": "deploy_notes", "path": "references/rollback.md",
	})
	if err != nil {
		t.Fatalf("skill_view L2: %v", err)
	}
	if !strings.Contains(out.Content, "stop the service") {
		t.Fatalf("reference body missing: %q", out.Content)
	}
}

// The model cannot open a file it was never told exists, so L1 lists what else
// the skill carries.
func TestSkillViewListsWhatElseTheSkillCarries(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "deploy", "---\nname: deploy_notes\ndescription: d\n---\nbody\n")
	writeSkillFile(t, root, "deploy", "references/rollback.md", "x")
	writeSkillFile(t, root, "deploy", "scripts/check.sh", "y")

	view := &skillViewSkill{paths: []string{root}}
	out, err := view.ExecuteTool(context.Background(), map[string]any{"name": "deploy_notes"})
	if err != nil {
		t.Fatalf("skill_view: %v", err)
	}
	if !containsAll(out.Content, "references/rollback.md", "scripts/check.sh") {
		t.Fatalf("supporting files not listed: %q", out.Content)
	}
	// A skill with nothing but SKILL.md must not grow a stray empty section.
	writeSkillFixture(t, root, "solo", "---\nname: solo\ndescription: d\n---\nbody\n")
	plain, err := view.ExecuteTool(context.Background(), map[string]any{"name": "solo"})
	if err != nil {
		t.Fatalf("skill_view: %v", err)
	}
	if strings.Contains(plain.Content, "Files in this skill") {
		t.Errorf("a single-file skill should list nothing: %q", plain.Content)
	}
}

// `path` is a string the model wrote, joined onto a directory. What the gate
// judges is where the path lands, never how it was spelled.
func TestSkillViewRefusesAPathOutsideTheSkill(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "deploy", "---\nname: deploy_notes\ndescription: d\n---\nbody\n")
	writeSkillFixture(t, root, "other", "---\nname: other\ndescription: d\n---\nSECRET\n")

	view := &skillViewSkill{paths: []string{root}}
	for _, bad := range []string{"../other/SKILL.md", "references/../../other/SKILL.md", `..\other\SKILL.md`} {
		out, err := view.ExecuteTool(context.Background(), map[string]any{"name": "deploy_notes", "path": bad})
		if err == nil {
			t.Errorf("path %q was accepted: %q", bad, out.Content)
			continue
		}
		if strings.Contains(out.Content, "SECRET") {
			t.Errorf("path %q leaked another skill's body", bad)
		}
	}
}

// Naming a file that is not there is a mistake the model can recover from
// only if the answer says so.
func TestSkillViewMissingFileSaysWhereToLook(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "deploy", "---\nname: deploy_notes\ndescription: d\n---\nbody\n")

	view := &skillViewSkill{paths: []string{root}}
	_, err := view.ExecuteTool(context.Background(), map[string]any{"name": "deploy_notes", "path": "references/nope.md"})
	if err == nil {
		t.Fatal("want an error for a file the skill does not have")
	}
	if !strings.Contains(err.Error(), "not in this skill") {
		t.Errorf("error should point at the skill body: %v", err)
	}
}

// A model handed a listing of `consumer-props.md` asked for
// `references/consumer-props.md` instead — following the example that used to
// end this parameter's description ("e.g. references/formats.md") rather than
// the data it had just received. An example in a tool description is not an
// illustration, it is an instruction, and it outranks anything that arrives
// later in the conversation. Same family as the word-trigger rule in
// defaults_test.go: what a description says is routing, not documentation.
func TestSkillViewDoesNotInventAPathShapeForTheModelToCopy(t *testing.T) {
	def := (&skillViewSkill{}).ToolDefinition()
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
		t.Fatalf("parameters do not parse: %v", err)
	}
	desc := schema.Properties["path"].Description
	if desc == "" {
		t.Fatal("the path parameter lost its description")
	}
	// Any concrete-looking path in here is a shape the model will reproduce.
	for _, invented := range []string{"references/", "scripts/", "assets/", "e.g.", ".md"} {
		if strings.Contains(desc, invented) {
			t.Errorf("path description carries %q, which a model will copy instead of reading the listing: %q",
				invented, desc)
		}
	}
	// It has to say where the real answer is instead.
	if !strings.Contains(desc, "listed at the end of the skill body") {
		t.Errorf("path description does not point at the listing: %q", desc)
	}
}
