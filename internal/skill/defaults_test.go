package skill

import "testing"

func TestNewDefaultRegistryRegistersAllBuiltins(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})

	want := []string{
		"help", "echo", "time", "list", "read", "github_repo_summary",
		"git", "fs", "shell", "write", "edit", "grep", "delete", "plugin_install", "image_ocr",
		"web_fetch", "web_search",
	}
	for _, name := range want {
		if _, ok := registry.Get(name); !ok {
			t.Errorf("built-in skill %q not registered", name)
		}
		if src, ok := registry.SourceOf(name); !ok || src != SourceBuiltin {
			t.Errorf("SourceOf(%q) = %v, %v, want %v, true", name, src, ok, SourceBuiltin)
		}
	}
	if got := len(registry.Names()); got != len(want) {
		t.Errorf("registry has %d skills, want %d", got, len(want))
	}
}

func TestRegisterDefaultsNilRegistryIsSafe(t *testing.T) {
	RegisterDefaults(nil, RegistryOptions{}) // must not panic
}
