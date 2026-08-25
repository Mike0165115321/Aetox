package main

import (
	"reflect"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
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

	app := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, newConversation())
	// Some of these open the database and App caches the handle. On Windows an
	// open file cannot be removed, so t.TempDir's own cleanup fails the test
	// unless the handle is closed first (Cleanup is LIFO, and the temp dirs
	// registered theirs above).
	t.Cleanup(func() {
		if app.db != nil {
			_ = app.db.Close()
		}
	})
	value := reflect.ValueOf(app)

	// Zero-argument bindings that are safe to call with no engine and no DB.
	noArgs := []string{
		"ListSubagentProfiles",
		"ListExternalSkills",
		"ListPromptPresets",
		"ListSkills",
		"SupportedProviders",
		"EnabledProviders",
		"SupportedThinkLevels",
		"TerminalShells",
		"CommandHistory",
		"GitChangedFiles",
		"GitWorkingTree",
		"ProjectTree",
		"WorkspaceFolders",
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

	// Same rule, one count argument. RecentAgentPages is the browser tab's
	// start page, which does .length on the result the moment it resolves.
	withLimit := []string{"RecentAgentPages"}
	for _, name := range withLimit {
		method := value.MethodByName(name)
		if !method.IsValid() {
			t.Errorf("binding %s no longer exists — update this list", name)
			continue
		}
		assertNonNilSlices(t, name, method.Call([]reflect.Value{reflect.ValueOf(5)}))
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
