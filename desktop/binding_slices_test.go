package main

import (
	"reflect"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// Every slice a binding hands the frontend must be non-nil. A nil slice
// marshals to JSON null, the frontend does .length on it, and Svelte aborts
// the render mid-update — the symptom is a settings page that looks
// unclickable, not an error (ARCHITECTURE.md §34).
//
// The environment here is the crash case on purpose: empty data root, empty
// home, an empty non-repo sandbox. Everything below returns "nothing", which
// is exactly when nil shows up.
func TestBindingsNeverReturnNilSlices(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	app := &App{cfg: config.Config{SandboxRoot: t.TempDir()}}
	value := reflect.ValueOf(app)

	// Zero-argument bindings that are safe to call with no engine and no DB.
	noArgs := []string{
		"ListAgentProfiles",
		"ListExternalSkills",
		"ListPromptPresets",
		"ListSkills",
		"SupportedProviders",
		"EnabledProviders",
		"SupportedThinkLevels",
		"TerminalShells",
		"CommandHistory",
		"GitChangedFiles",
		"ProjectTree",
	}
	for _, name := range noArgs {
		method := value.MethodByName(name)
		if !method.IsValid() {
			t.Errorf("binding %s no longer exists — update this list", name)
			continue
		}
		assertNonNilSlices(t, name, method.Call(nil))
	}

	// Same rule, one string argument.
	withProvider := []string{"ProviderWireFormats"}
	for _, name := range withProvider {
		method := value.MethodByName(name)
		if !method.IsValid() {
			t.Errorf("binding %s no longer exists — update this list", name)
			continue
		}
		assertNonNilSlices(t, name, method.Call([]reflect.Value{reflect.ValueOf("deepseek")}))
	}
}

func assertNonNilSlices(t *testing.T, name string, results []reflect.Value) {
	t.Helper()
	for i, result := range results {
		if result.Kind() != reflect.Slice {
			continue
		}
		if result.IsNil() {
			t.Errorf("%s returned a nil slice (result %d) — marshals to JSON null and crashes the frontend; wrap it in jsonSlice()", name, i)
		}
	}
}
