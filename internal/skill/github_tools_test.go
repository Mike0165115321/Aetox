package skill

import "testing"

func TestValidatePluginManifestName(t *testing.T) {
	base := func(name string) *aetoxPluginManifest {
		return &aetoxPluginManifest{
			Name:  name,
			Files: []aetoxPluginFileEntry{{Source: "skill.md", Target: "skill.md"}},
		}
	}

	for _, bad := range []string{"", "..", "../escape", "..\\escape", "/abs", "a/../..", "nested/name", "C:\\evil"} {
		if err := validatePluginManifest(base(bad)); err == nil {
			t.Errorf("name %q should be rejected", bad)
		}
	}

	m := base("  my-plugin  ")
	if err := validatePluginManifest(m); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	if m.Name != "my-plugin" {
		t.Errorf("name not normalized: %q", m.Name)
	}
}
