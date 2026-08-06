package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/safety"
)

// isolateUserDirs points every "where does this user's data live" lookup at a
// temp directory, on every platform.
//
// Setting APPDATA/LOCALAPPDATA alone only isolates Windows. os.UserConfigDir
// reads XDG_CONFIG_HOME on Linux and $HOME/Library/Application Support on
// macOS, so on those platforms these tests were reading and writing the real
// ~/.config/aetox — which is how a "missing file returns nil" test started
// failing the moment another package's tests ran first and left a file there.
func isolateUserDirs(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("APPDATA", base)         // windows: os.UserConfigDir
	t.Setenv("LOCALAPPDATA", base)    // windows: the legacy preference path
	t.Setenv("USERPROFILE", base)     // windows: os.UserHomeDir
	t.Setenv("XDG_CONFIG_HOME", base) // linux: os.UserConfigDir
	t.Setenv("HOME", base)            // linux + macos: os.UserHomeDir, and macOS' config dir
	return base
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "env-key")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	cfg := Load(ConfigOptions{
		ModelProvider: "openrouter",
	})

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		t.Fatalf("abs cwd failed: %v", err)
	}
	absRoot, err := filepath.Abs(cfg.SandboxRoot)
	if err != nil {
		t.Fatalf("abs root failed: %v", err)
	}

	if absRoot != absCwd {
		t.Fatalf("expected root fallback to cwd, got %q", cfg.SandboxRoot)
	}
	if cfg.MaxRetries != 2 {
		t.Fatalf("expected max retries 2, got %d", cfg.MaxRetries)
	}
	if cfg.MaxPlanRetries != 0 {
		t.Fatalf("expected max plan retries 0, got %d", cfg.MaxPlanRetries)
	}
	if cfg.ApprovalTimeoutSec != 60 {
		t.Fatalf("expected approval timeout 60, got %d", cfg.ApprovalTimeoutSec)
	}
	if cfg.ModelTimeoutSec != 30 {
		t.Fatalf("expected model timeout 30, got %d", cfg.ModelTimeoutSec)
	}
	if cfg.ModelProvider != "openrouter" {
		t.Fatalf("expected model provider openrouter, got %q", cfg.ModelProvider)
	}
	if cfg.ModelAPIKey != "env-key" {
		t.Fatalf("expected API key from env, got %q", cfg.ModelAPIKey)
	}
	if cfg.ThinkLevel != "low" {
		t.Fatalf("expected default think level low, got %q", cfg.ThinkLevel)
	}
}

func TestLoadInvalidValues(t *testing.T) {
	root := t.TempDir()

	cfg := Load(ConfigOptions{
		RootPath:        root,
		MaxRetries:      0,
		MaxPlanRetries:  -1,
		ApprovalTimeout: 0,
		ModelTimeout:    0,
		ModelProvider:   "",
	})

	if cfg.SandboxRoot != root {
		t.Fatalf("expected configured root %q, got %q", root, cfg.SandboxRoot)
	}
	if cfg.MaxRetries != 2 {
		t.Fatalf("expected fallback max retries 2, got %d", cfg.MaxRetries)
	}
	if cfg.MaxPlanRetries != 0 {
		t.Fatalf("expected fallback max plan retries 0, got %d", cfg.MaxPlanRetries)
	}
	if cfg.ApprovalTimeoutSec != 60 {
		t.Fatalf("expected fallback approval timeout 60, got %d", cfg.ApprovalTimeoutSec)
	}
	if cfg.ModelTimeoutSec != 30 {
		t.Fatalf("expected fallback model timeout 30, got %d", cfg.ModelTimeoutSec)
	}
	if cfg.ModelProvider != "aetox" {
		t.Fatalf("expected model provider fallback aetox, got %q", cfg.ModelProvider)
	}
	if cfg.ThinkLevel != "low" {
		t.Fatalf("expected fallback think level low, got %q", cfg.ThinkLevel)
	}
}

func TestSaveAndLoadModelPreferenceThinkLevel(t *testing.T) {
	isolateUserDirs(t)

	want := ModelPreference{
		ModelProvider: "openrouter",
		ModelName:     "deepseek/deepseek-r1",
		ThinkLevel:    "high",
	}
	if err := SaveModelPreference(want); err != nil {
		t.Fatalf("save preference failed: %v", err)
	}

	got, ok, err := LoadModelPreference()
	if err != nil {
		t.Fatalf("load preference failed: %v", err)
	}
	if !ok {
		t.Fatal("expected saved preference to exist")
	}
	if got.ThinkLevel != want.ThinkLevel {
		t.Fatalf("expected think level %q, got %q", want.ThinkLevel, got.ThinkLevel)
	}
}

func TestResolvedEnabledProvidersDefaultsToActiveProvider(t *testing.T) {
	// Never customized (empty slice) — an install must still show something,
	// and it must be exactly the provider already configured, not the whole catalog.
	got := ResolvedEnabledProviders(nil, "deepseek")
	if len(got) != 1 || got[0] != "deepseek" {
		t.Fatalf("ResolvedEnabledProviders(nil, deepseek) = %v, want [deepseek]", got)
	}
}

// A genuinely fresh install (never customized, never picked a real provider)
// runs on "aetox" (Aetox's own built-in engine) — it must show up by default,
// not be hidden, since that's exactly what removing an active provider falls
// back to and what the onboarding reply points a user back at.
func TestResolvedEnabledProvidersShowsAetoxByDefault(t *testing.T) {
	got := ResolvedEnabledProviders(nil, "aetox")
	if len(got) != 1 || got[0] != "aetox" {
		t.Fatalf("ResolvedEnabledProviders(nil, aetox) = %v, want [aetox]", got)
	}
	// "noop" is a backward-compat alias — must resolve the same way.
	got = ResolvedEnabledProviders(nil, "noop")
	if len(got) != 1 || got[0] != "aetox" {
		t.Fatalf("ResolvedEnabledProviders(nil, noop) = %v, want [aetox] (noop normalizes to aetox)", got)
	}
}

func TestResolvedEnabledProvidersRespectsCustomSetEvenWithoutActiveProvider(t *testing.T) {
	// A customized set wins outright — the active provider is NOT force-appended.
	// Otherwise explicitly disabling the currently-active provider (to switch
	// away and hide it) would be silently undone on every subsequent read.
	got := ResolvedEnabledProviders([]string{"openai", "anthropic"}, "deepseek")
	want := []string{"openai", "anthropic"}
	if len(got) != len(want) {
		t.Fatalf("ResolvedEnabledProviders = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ResolvedEnabledProviders = %v, want %v", got, want)
		}
	}
}

func TestResolvedEnabledProvidersDedupesAndNormalizes(t *testing.T) {
	got := ResolvedEnabledProviders([]string{"OpenAI", "openai", " anthropic "}, "openai")
	want := []string{"openai", "anthropic"}
	if len(got) != len(want) {
		t.Fatalf("ResolvedEnabledProviders = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ResolvedEnabledProviders = %v, want %v", got, want)
		}
	}
}

func TestLoadPermissionsMissingFileReturnsEmpty(t *testing.T) {
	isolateUserDirs(t)

	got, err := LoadPermissions()
	if err != nil {
		t.Fatalf("load permissions failed: %v", err)
	}
	if len(got.Rules) != 0 {
		t.Fatalf("expected no rules when file is missing, got %v", got.Rules)
	}
}

func TestLoadMCPServersMissingFileReturnsNil(t *testing.T) {
	isolateUserDirs(t)

	got, err := LoadMCPServers()
	if err != nil {
		t.Fatalf("load mcp servers failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no servers when file is missing, got %v", got)
	}
}

func TestSaveAndLoadMCPServers(t *testing.T) {
	isolateUserDirs(t)

	want := []MCPServerConfig{
		{Name: "fs", Command: []string{"npx", "-y", "server-filesystem", "/tmp"}, TimeoutMs: 5000},
		{Name: "git", Command: []string{"uvx", "mcp-git"}, Environment: map[string]string{"TOKEN": "x"}},
	}
	if err := SaveMCPServers(want); err != nil {
		t.Fatalf("save mcp servers failed: %v", err)
	}

	got, err := LoadMCPServers()
	if err != nil {
		t.Fatalf("load mcp servers failed: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d servers, got %d", len(want), len(got))
	}
	if got[0].Name != "fs" || len(got[0].Command) != 4 || got[0].TimeoutMs != 5000 {
		t.Fatalf("server 0 round-trip mismatch: %+v", got[0])
	}
	if got[1].Environment["TOKEN"] != "x" {
		t.Fatalf("server 1 environment not preserved: %+v", got[1])
	}
}

func TestLoadPermissions(t *testing.T) {
	isolateUserDirs(t)

	want := safety.PermissionConfig{Rules: []safety.PermissionRule{
		{Tool: "shell", Pattern: "rm *", Action: safety.PermissionDeny},
		{Tool: "git", Pattern: "status", Action: safety.PermissionAllow},
	}}
	path, err := PermissionsPath()
	if err != nil {
		t.Fatalf("permissions path: %v", err)
	}
	payload, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write permissions file: %v", err)
	}

	got, err := LoadPermissions()
	if err != nil {
		t.Fatalf("load permissions failed: %v", err)
	}
	if len(got.Rules) != len(want.Rules) {
		t.Fatalf("expected %d rules, got %d", len(want.Rules), len(got.Rules))
	}
	for i, rule := range want.Rules {
		if got.Rules[i] != rule {
			t.Fatalf("rule %d mismatch: want %+v, got %+v", i, rule, got.Rules[i])
		}
	}
}

// A server the user configured used to connect, register, and then be filtered
// off every desk, because a desk manifest ships compiled in and could not name
// a server that did not exist yet. The assistant reported having no MCP tools,
// which from where it stood was true. Ownership now sits on the server.
func TestMCPServersReachTheDesksTheyNameAndNoOthers(t *testing.T) {
	withServers(t, `[
	  {"name":"sequential-thinking","command":["npx","x"],"for":["assistant","coding"]},
	  {"name":"video-tools","command":["npx","y"],"for":["agent:วิดีโอ"]},
	  {"name":"off","command":["npx","z"],"for":["assistant"],"disabled":true},
	  {"name":"shelved","command":["npx","w"],"for":[]}
	]`)

	if got := MCPServersForDesk("assistant"); !slices.Equal(got, []string{"sequential-thinking"}) {
		t.Errorf("assistant desk got %v, want just the server that names it", got)
	}
	// The separation this field exists for: a server installed for an agent is
	// never on the main assistant's tool block.
	for _, desk := range []string{"assistant", "coding", "specialized"} {
		if slices.Contains(MCPServersForDesk(desk), "video-tools") {
			t.Errorf("an agent's server leaked onto the %s desk", desk)
		}
	}
	// Two switches, both real: disabled is never connected, empty `for` is
	// connected and attached nowhere.
	if got := MCPServersForDesk("assistant"); slices.Contains(got, "off") || slices.Contains(got, "shelved") {
		t.Errorf("a switched-off server was carried: %v", got)
	}
	// The pre-modes full desk still carries everything it can reach.
	if got := MCPServersForDesk(""); len(got) != 3 {
		t.Errorf("the full desk got %v, want every enabled server", got)
	}
}

// An install written before the field existed must not go dark, and the value
// it gets must be visible in the file rather than applied invisibly — this
// field's whole purpose is to be a switch the user can flip.
func TestMCPOwnersMigrateOnceAndAreWrittenBack(t *testing.T) {
	path := withServers(t, `[{"name":"sequential-thinking","command":["npx","x"]}]`)

	if got := MCPServersForDesk("assistant"); !slices.Contains(got, "sequential-thinking") {
		t.Fatalf("a pre-field server went dark: %v", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(raw), `"for"`) {
		t.Fatalf("the default was applied invisibly — the user cannot switch what they cannot see:\n%s", raw)
	}

	// An explicit empty list is a switch the user turned off. Absent and empty
	// decode to the same nil, so the migration reads the raw JSON; if it did
	// not, turning a server off would silently turn it back on.
	withServers(t, `[{"name":"shelved","command":["npx","x"],"for":[]}]`)
	if got := MCPServersForDesk("assistant"); len(got) != 0 {
		t.Errorf("an explicitly emptied `for` was overwritten by the default: %v", got)
	}
}

// withServers points the data root at a temp dir holding one servers file, and
// resets the once-guard so each test migrates its own fixture.
func withServers(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	path := filepath.Join(root, "mcp-servers.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write servers: %v", err)
	}
	migratedMCPOwners = sync.Once{}
	return path
}

// Turning a server off for everyone must survive a round trip through the
// file. An empty `for` written with omitempty disappears, the migration reads
// absence as "predates the field", and the switch the user just turned off is
// back on at the next launch — a setting that silently undoes itself.
func TestSwitchingAServerOffForEveryoneSurvivesASave(t *testing.T) {
	withServers(t, `[{"name":"s","command":["npx","x"],"for":["assistant"]}]`)

	servers, err := LoadMCPServers()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	servers[0].For = []string{}
	if err := SaveMCPServers(servers); err != nil {
		t.Fatalf("save: %v", err)
	}

	migratedMCPOwners = sync.Once{} // the next launch
	if got := MCPServersForDesk("assistant"); len(got) != 0 {
		t.Errorf("the server switched itself back on: %v", got)
	}
}

// Secrets and settings need different handling, and one file cannot have two.
// The preference file is opened to check a locale or a last desk, pasted into
// bug reports and screenshotted; while the keys lived in it, every one of those
// ordinary acts leaked them — which is how a key reached a debugging transcript
// on the day this was split (2026-08-06).
func TestAPIKeysAreNotInThePreferenceFile(t *testing.T) {
	isolateUserDirs(t)

	const key = "sk-do-not-let-this-into-the-settings-file"
	if err := SaveModelPreference(ModelPreference{
		ModelProvider: "deepseek",
		UILocale:      "th",
		LastDesk:      "specialized",
		ModelAPIKeys:  map[string]string{"deepseek": key},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	prefPath, _ := PreferencePath()
	raw, err := os.ReadFile(prefPath)
	if err != nil {
		t.Fatalf("read preferences: %v", err)
	}
	if strings.Contains(string(raw), key) {
		t.Fatalf("the API key is still in the settings file:\n%s", raw)
	}
	// The settings themselves must still be there and still readable by hand —
	// that readability is the whole reason the secrets left.
	if !strings.Contains(string(raw), "specialized") {
		t.Fatalf("the settings did not survive the split:\n%s", raw)
	}

	// Callers hand over one struct and get one back: the storage split is this
	// package's business, not every call site's.
	got, ok, err := LoadModelPreference()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.ModelAPIKeys["deepseek"] != key {
		t.Fatalf("the key did not come back: %+v", got.ModelAPIKeys)
	}
	if got.LastDesk != "specialized" {
		t.Errorf("settings did not come back: %+v", got)
	}
}

// An install written before the split must not lose its keys — losing one means
// the user cannot reach their own provider until they find and retype it.
func TestKeysInAnOldPreferenceFileMoveOutOnLoad(t *testing.T) {
	isolateUserDirs(t)

	const key = "sk-written-before-the-split"
	prefPath, _ := PreferencePath()
	if err := os.MkdirAll(filepath.Dir(prefPath), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"provider":"deepseek","last_desk":"coding","provider_api_keys":{"deepseek":"` + key + `"}}`
	if err := os.WriteFile(prefPath, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok, err := LoadModelPreference()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.ModelAPIKeys["deepseek"] != key {
		t.Fatalf("the migration lost the key: %+v", got.ModelAPIKeys)
	}
	raw, err := os.ReadFile(prefPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(raw), key) {
		t.Fatalf("the key is still in the settings file after migrating:\n%s", raw)
	}
	// Two copies of a secret is worse than one: the stripped file must not be
	// the only thing that happened.
	creds, err := LoadCredentials()
	if err != nil {
		t.Fatalf("load credentials: %v", err)
	}
	if creds.ModelAPIKeys["deepseek"] != key {
		t.Fatalf("the key did not land in the credentials file: %+v", creds)
	}
}

// Round trip through whatever the platform gives us. On Windows the file is
// DPAPI-wrapped and must not contain the key in the clear; everywhere else it
// is plaintext by design (see secret_other.go) and this still pins that the
// value survives the trip.
func TestCredentialsRoundTrip(t *testing.T) {
	isolateUserDirs(t)

	const key = "sk-round-trip"
	if err := SaveCredentials(Credentials{ModelAPIKeys: map[string]string{"deepseek": key}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadCredentials()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ModelAPIKeys["deepseek"] != key {
		t.Fatalf("round trip lost the key: %+v", got)
	}
	if runtime.GOOS == "windows" {
		path, _ := CredentialsPath()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(string(raw), key) {
			t.Errorf("the key is in the clear on disk despite DPAPI:\n%s", raw)
		}
	}
}
