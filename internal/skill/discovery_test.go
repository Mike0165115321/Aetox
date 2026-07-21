package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkillFixture(t *testing.T, root, dirName, content string) {
	t.Helper()
	skillDir := filepath.Join(root, dirName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
}

func TestDiscoverSkills_ParsesFrontmatterAndBody(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "commit-helper", "---\nname: commit_helper\ndescription: Draft a commit message from staged changes\n---\n# Commit Helper\n\nLook at `git diff --staged` and write a message.\n")

	found, errs := DiscoverSkills([]string{root})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(found))
	}
	if found[0].Name() != "commit_helper" {
		t.Fatalf("expected name commit_helper, got %q", found[0].Name())
	}
	if found[0].Description() != "Draft a commit message from staged changes" {
		t.Fatalf("unexpected description: %q", found[0].Description())
	}
	out, err := found[0].Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !containsAll(out.Content, "Commit Helper", "git diff --staged") {
		t.Fatalf("expected body content in output, got %q", out.Content)
	}
}

func TestDiscoverSkills_MissingNameFallsBackToDirName(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "my-skill", "---\ndescription: no name field\n---\nbody\n")

	found, errs := DiscoverSkills([]string{root})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(found) != 1 || found[0].Name() != "my-skill" {
		t.Fatalf("expected fallback name my-skill, got %+v", found)
	}
}

func TestDiscoverSkills_NoFrontmatterUsesWholeFileAsBody(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "plain", "just plain instructions, no frontmatter\n")

	found, errs := DiscoverSkills([]string{root})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(found))
	}
	out, _ := found[0].Execute(context.Background(), nil)
	if out.Content != "just plain instructions, no frontmatter" {
		t.Fatalf("unexpected body: %q", out.Content)
	}
}

func TestDiscoverSkills_UnterminatedFrontmatterIsReportedNotFatal(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "broken", "---\nname: broken\nno closing marker\n")
	writeSkillFixture(t, root, "ok-one", "---\nname: ok_one\ndescription: fine\n---\nbody\n")

	found, errs := DiscoverSkills([]string{root})
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 parse error, got %d: %v", len(errs), errs)
	}
	if len(found) != 1 || found[0].Name() != "ok_one" {
		t.Fatalf("expected the well-formed skill to still be discovered, got %+v", found)
	}
}

func TestDiscoverSkills_MissingDirIsNotAnError(t *testing.T) {
	found, errs := DiscoverSkills([]string{filepath.Join(t.TempDir(), "does-not-exist")})
	if len(errs) != 0 {
		t.Fatalf("missing scan dir should not be an error, got %v", errs)
	}
	if len(found) != 0 {
		t.Fatalf("expected no skills, got %d", len(found))
	}
}

func TestRegisterDiscovered_CollisionWithBuiltinIsSkippedNotFatal(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "read", "---\nname: read\ndescription: shadow attempt\n---\nbody\n")

	registry := NewRegistry()
	if err := registry.Register(&stubSkill{name: "read"}, SourceBuiltin); err != nil {
		t.Fatalf("seed builtin failed: %v", err)
	}

	errs := RegisterDiscovered(registry, []string{root})
	if len(errs) != 1 {
		t.Fatalf("expected 1 collision error, got %d: %v", len(errs), errs)
	}
	if src, _ := registry.SourceOf("read"); src != SourceBuiltin {
		t.Fatalf("builtin 'read' must survive the collision, got source %v", src)
	}
}

func TestRegisterDiscovered_RegistersAsExternal(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, "helper", "---\nname: helper_skill\ndescription: d\n---\nbody\n")

	registry := NewRegistry()
	if errs := RegisterDiscovered(registry, []string{root}); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	src, ok := registry.SourceOf("helper_skill")
	if !ok || src != SourceExternal {
		t.Fatalf("SourceOf(helper_skill) = %v, %v; want %v, true", src, ok, SourceExternal)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
