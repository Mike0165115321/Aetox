package skill

import (
	"context"
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

func TestSkillsListEmpty(t *testing.T) {
	list := &skillsListSkill{paths: []string{t.TempDir()}}
	out, err := list.ExecuteTool(context.Background(), nil)
	if err != nil {
		t.Fatalf("skills_list on empty dir: %v", err)
	}
	if !strings.Contains(out.Content, "No skills installed") {
		t.Fatalf("empty library should say so plainly: %q", out.Content)
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
	if err != nil || !strings.Contains(out.Content, "No skills installed") {
		t.Fatalf("precondition: expected empty library, got %q (%v)", out.Content, err)
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
